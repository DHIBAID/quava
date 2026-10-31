package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	clip "golang.design/x/clipboard"

	"libquava/discovery"
	"libquava/inputs"
	lq "libquava/models"
	"libquava/protocol"
	"libquava/session"
	"quavad/models"
)

func (d *Daemon) shutdown() {
	d.connMu.Lock()
	for conn := range d.connections {
		_ = conn.Close()
	}
	d.connMu.Unlock()
	d.mu.Lock()
	for _, managed := range d.sessions {
		if managed.Cancel != nil {
			managed.Cancel()
		}
	}
	d.inputOwner = ""
	for id := range d.inputEnabled {
		d.inputEnabled[id] = false
	}
	d.mu.Unlock()
	if err := d.resetRemoteInput(); err != nil {
		log.Printf("input: shutdown cleanup: %v", err)
	}
}

func (d *Daemon) connect(req models.Request, enc *json.Encoder) {
	if err := d.startSession(req.PeerDeviceID); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	_ = enc.Encode(models.Response{})
}

// startSession begins keeping a session with a paired peer alive. It is a no-op
// when one is already running. The session retries on its own until the peer
// is reachable, so this can be called for phones that are not on the network.
func (d *Daemon) startSession(peerID string) error {
	d.mu.Lock()
	peer, ok := d.store.Device(peerID)
	if !ok {
		d.mu.Unlock()
		return errors.New("peer device is not paired")
	}

	identity, ok := d.store.Identity()
	if !ok {
		d.mu.Unlock()
		return errors.New("local identity not found")
	}

	if _, exists := d.sessions[peer.PeerDeviceID]; exists {
		d.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(d.ctx)
	d.sessions[peer.PeerDeviceID] = &models.ManagedSession{Cancel: cancel}
	d.mu.Unlock()

	go d.runSession(ctx, identity, peer)

	return nil
}

// startAutoConnect connects to every paired device that has automatic
// connecting on, which is the default.
func (d *Daemon) startAutoConnect() {
	d.mu.Lock()
	var ids []string
	for _, device := range d.store.Devices() {
		if d.store.AutoConnect(device.PeerDeviceID) {
			ids = append(ids, device.PeerDeviceID)
		}
	}
	d.mu.Unlock()

	for _, id := range ids {
		if err := d.startSession(id); err != nil {
			log.Printf("auto-connect %s: %v", id, err)
		}
	}
}

// setAutoConnect changes the saved preference. Turning it on also starts a
// session; turning it off leaves any current session alone.
func (d *Daemon) setAutoConnect(req models.Request, enc *json.Encoder, enabled bool) {
	d.mu.Lock()
	err := d.store.SetAutoConnect(req.PeerDeviceID, enabled)
	if err == nil {
		err = d.store.Save()
	}
	d.mu.Unlock()

	if err == nil && enabled {
		err = d.startSession(req.PeerDeviceID)
	}
	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	_ = enc.Encode(models.Response{})
}

// disconnect cancels the managed session for one peer. runSession performs the
// remaining cleanup and removes the entry once the protocol connection closes.

func (d *Daemon) disconnect(req models.Request, enc *json.Encoder) {
	d.mu.Lock()
	managed := d.sessions[req.PeerDeviceID]
	if managed == nil {
		d.mu.Unlock()
		_ = enc.Encode(models.Response{Error: "peer device is not connected"})
		return
	}
	cancel := managed.Cancel
	d.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	_ = enc.Encode(models.Response{})
}

func (d *Daemon) runSession(ctx context.Context, identity lq.IdentityFile, peer lq.PairResult) {
	// The shared watcher listens for the phone to announce itself. Sessions
	// that cannot use it (no multicast-capable network at startup) fall back
	// to a private client.
	var client *discovery.Client
	if d.watcher == nil {
		var err error
		client, err = discovery.NewClient()
		if err != nil {
			d.clearSession(peer.PeerDeviceID)
			return
		}

		defer func() { _ = client.Close() }()
	}

	if err := session.RunWithCallbacks(ctx, identity, peer, func(dialCtx context.Context) (*protocol.Conn, error) {
		var device discovery.Device
		var err error
		if d.watcher != nil {
			// Waits quietly until the phone is heard; asks only occasionally.
			device, err = d.watcher.Find(dialCtx, peer.PeerDeviceID)
		} else {
			findCtx, cancelFind := context.WithTimeout(dialCtx, 8*time.Second)
			device, err = client.Find(findCtx, peer.PeerDeviceID)
			cancelFind()
		}
		if err != nil {
			return nil, fmt.Errorf("find %s: %w", peer.PeerDeviceID, err)
		}

		connection, err := device.Connect(dialCtx)
		if err != nil {
			// The address may be stale (new IP, phone gone); wait for a fresh announcement.
			if d.watcher != nil {
				d.watcher.Forget(peer.PeerDeviceID)
			}
			return nil, err
		}

		return connection.ProtocolConn(), nil
	}, func(active *session.Session) {
		active.SetRemoteInputHandler(func(inputCtx context.Context, event lq.RemoteInput) error {
			d.mu.Lock()
			enabled := d.inputEnabled[peer.PeerDeviceID] && d.inputOwner == peer.PeerDeviceID
			d.mu.Unlock()
			if !enabled {
				return errors.New("remote input is disabled for this device")
			}
			return d.applyRemoteInput(inputCtx, event)
		})
		active.SetClipboardHandler(func(clipboardCtx context.Context, content lq.ClipboardContent) error {
			d.mu.Lock()
			ready := d.clipboardReady
			d.mu.Unlock()
			if !ready {
				return errors.New("host clipboard service is unavailable in this session")
			}
			if content.ContentType != lq.Text {
				return fmt.Errorf("unsupported clipboard content type %q", content.ContentType)
			}
			if len(content.Content) > protocol.MaxClipboardBytes {
				return errors.New("clipboard payload exceeds 48 KiB sync limit")
			}
			d.clipboard.RememberContent(content)
			_, writeErr := clip.Write(clipboardCtx, clip.FmtText, content.Content)
			return writeErr
		})
		active.SetUnpairHandler(func() {
			log.Printf("session: %s asked to be unpaired", peer.PeerDeviceID)
			if err := d.forgetDevice(peer.PeerDeviceID); err != nil {
				log.Printf("session: unpair %s: %v", peer.PeerDeviceID, err)
			}
		})
		active.SetAndroidNotificationHandler(func(ctx context.Context, notification lq.AndroidNotificationContent) error {
			if d.androidNotifications == nil || !d.notificationSyncsToHost(peer.PeerDeviceID) {
				return nil
			}
			return d.androidNotifications.Show(ctx, notification)
		})
	}, func(active *session.Session) {
		d.mu.Lock()
		if managed := d.sessions[peer.PeerDeviceID]; managed != nil {
			managed.Active = active
		}
		// A phone the user chose to always allow gets remote input back on its
		// own, unless another device currently owns it.
		if d.store.InputAuto(peer.PeerDeviceID) && (d.inputOwner == "" || d.inputOwner == peer.PeerDeviceID) {
			d.inputOwner = peer.PeerDeviceID
			d.inputEnabled[peer.PeerDeviceID] = true
		}
		d.mu.Unlock()
	}, func(cause error) {
		if cause != nil && !errors.Is(cause, context.Canceled) {
			log.Printf("session: %s unavailable, will retry: %v", peer.PeerDeviceID, cause)
		}
		d.mu.Lock()
		if managed := d.sessions[peer.PeerDeviceID]; managed != nil {
			managed.Active = nil
		}
		wasInputOwner := d.inputOwner == peer.PeerDeviceID
		if wasInputOwner {
			d.inputOwner = ""
			d.inputEnabled[peer.PeerDeviceID] = false
		}
		d.mu.Unlock()
		if wasInputOwner {
			if resetErr := d.resetRemoteInput(); resetErr != nil {
				log.Printf("input: release state after session loss: %v", resetErr)
			}
		}
	}); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("session: %s ended: %v", peer.PeerDeviceID, err)
	}

	d.clearSession(peer.PeerDeviceID)
}

// applyRemoteInput initializes the compositor backend on demand. A daemon can
// be socket-activated before the graphical session is fully ready, so eagerly
// selecting a backend at startup would make remote input permanently fail.

func (d *Daemon) applyRemoteInput(ctx context.Context, event lq.RemoteInput) error {
	d.inputMu.Lock()
	if d.input == nil {
		emulator, err := inputs.New()
		if err != nil {
			d.inputMu.Unlock()
			return err
		}
		d.input = emulator
	}
	emulator := d.input
	d.inputMu.Unlock()

	eventCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	d.inputDispatchMu.Lock()
	defer d.inputDispatchMu.Unlock()
	return emulator.Apply(eventCtx, event)
}

func (d *Daemon) resetRemoteInput() error {
	d.inputMu.Lock()
	emulator := d.input
	d.inputMu.Unlock()
	if emulator == nil {
		return nil
	}
	d.inputDispatchMu.Lock()
	defer d.inputDispatchMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return emulator.Reset(ctx)
}

func (d *Daemon) clearSession(id string) {
	d.mu.Lock()
	delete(d.sessions, id)
	wasInputOwner := d.inputOwner == id
	if wasInputOwner {
		d.inputOwner = ""
		d.inputEnabled[id] = false
	}
	d.mu.Unlock()
	if wasInputOwner {
		if err := d.resetRemoteInput(); err != nil {
			log.Printf("input: release state after disconnect: %v", err)
		}
	}
}

func (d *Daemon) setInputEnabled(req models.Request, enc *json.Encoder, enabled bool) {
	d.mu.Lock()
	if _, paired := d.store.Device(req.PeerDeviceID); !paired {
		d.mu.Unlock()
		_ = enc.Encode(models.Response{Error: "peer device is not paired"})
		return
	}
	managed := d.sessions[req.PeerDeviceID]
	if enabled && (managed == nil || managed.Active == nil) {
		d.mu.Unlock()
		_ = enc.Encode(models.Response{Error: "connect to the peer before enabling remote input"})
		return
	}
	if enabled && d.inputOwner != "" && d.inputOwner != req.PeerDeviceID {
		d.mu.Unlock()
		_ = enc.Encode(models.Response{Error: "remote input is already enabled for another device"})
		return
	}
	if enabled {
		d.inputOwner = req.PeerDeviceID
		d.inputEnabled[req.PeerDeviceID] = true
		d.mu.Unlock()
		_ = enc.Encode(models.Response{})
		return
	}
	d.inputEnabled[req.PeerDeviceID] = false
	wasOwner := d.inputOwner == req.PeerDeviceID
	if wasOwner {
		d.inputOwner = ""
	}
	d.mu.Unlock()
	if wasOwner {
		if err := d.resetRemoteInput(); err != nil {
			_ = enc.Encode(models.Response{Error: err.Error()})
			return
		}
	}
	_ = enc.Encode(models.Response{})
}

// setInputAuto remembers (or forgets) that a paired phone may use remote input
// every time it connects. Turning it on also enables input right away when the
// phone is already connected.
func (d *Daemon) setInputAuto(req models.Request, enc *json.Encoder, enabled bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.store.SetInputAuto(req.PeerDeviceID, enabled); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	if err := d.store.Save(); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	if enabled {
		managed := d.sessions[req.PeerDeviceID]
		if managed != nil && managed.Active != nil && (d.inputOwner == "" || d.inputOwner == req.PeerDeviceID) {
			d.inputOwner = req.PeerDeviceID
			d.inputEnabled[req.PeerDeviceID] = true
		}
	}
	_ = enc.Encode(models.Response{})
}

// forgetDevice ends any session with the device, then removes its trust record
// and settings from the config.
func (d *Daemon) forgetDevice(id string) error {
	d.mu.Lock()
	managed := d.sessions[id]
	wasInputOwner := d.inputOwner == id
	if wasInputOwner {
		d.inputOwner = ""
	}
	delete(d.inputEnabled, id)

	err := d.store.RemoveDevice(id)
	if err == nil {
		err = d.store.Save()
	}
	d.mu.Unlock()

	if managed != nil && managed.Cancel != nil {
		managed.Cancel()
	}
	if wasInputOwner {
		if resetErr := d.resetRemoteInput(); resetErr != nil {
			log.Printf("input: release state after unpair: %v", resetErr)
		}
	}
	return err
}

// unpair asks a connected phone to forget this host (best effort), then forgets
// the phone locally.
func (d *Daemon) unpair(req models.Request, enc *json.Encoder) {
	d.mu.Lock()
	_, paired := d.store.Device(req.PeerDeviceID)
	var active *session.Session
	if managed := d.sessions[req.PeerDeviceID]; managed != nil {
		active = managed.Active
	}
	d.mu.Unlock()

	if !paired {
		_ = enc.Encode(models.Response{Error: "peer device is not paired"})
		return
	}

	if active != nil {
		ctx, cancel := context.WithTimeout(d.ctx, 3*time.Second)
		if err := active.Unpair(ctx); err != nil {
			log.Printf("unpair: %s did not acknowledge: %v", req.PeerDeviceID, err)
		}
		cancel()
	}

	if err := d.forgetDevice(req.PeerDeviceID); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	_ = enc.Encode(models.Response{})
}
