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
	d.mu.Lock()
	peer, ok := d.store.Device(req.PeerDeviceID)

	if !ok {
		d.mu.Unlock()
		_ = enc.Encode(models.Response{Error: "peer device is not paired"})
		return
	}

	identity, ok := d.store.Identity()
	if !ok {
		d.mu.Unlock()
		_ = enc.Encode(models.Response{Error: "local identity not found"})
		return
	}

	if _, exists := d.sessions[peer.PeerDeviceID]; exists {
		d.mu.Unlock()
		_ = enc.Encode(models.Response{})
		return
	}
	ctx, cancel := context.WithCancel(d.ctx)
	d.sessions[peer.PeerDeviceID] = &models.ManagedSession{Cancel: cancel}
	d.mu.Unlock()

	go d.runSession(ctx, identity, peer)

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
	client, err := discovery.NewClient()

	if err != nil {
		d.clearSession(peer.PeerDeviceID)
		return
	}

	defer func() { _ = client.Close() }()

	if err := session.RunWithCallbacks(ctx, identity, peer, func(dialCtx context.Context) (*protocol.Conn, error) {
		device, err := client.Find(dialCtx, peer.PeerDeviceID)
		if err != nil {
			return nil, err
		}

		connection, err := device.Connect(dialCtx)
		if err != nil {
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
			if len(content.Content) > 48*1024 {
				return errors.New("clipboard payload exceeds 48 KiB sync limit")
			}
			d.clipboard.RememberContent(content)
			_, writeErr := clip.Write(clipboardCtx, clip.FmtText, content.Content)
			return writeErr
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
		d.mu.Unlock()
	}, func(error) {
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
