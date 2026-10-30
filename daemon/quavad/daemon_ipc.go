package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	clip "golang.design/x/clipboard"

	"libquava/config"
	"libquava/discovery"
	lq "libquava/models"
	"libquava/pairing"
	"libquava/session"
	"quavad/models"
	"quavad/utils"
)

func (d *Daemon) handle(connection net.Conn) {
	d.connMu.Lock()
	d.connections[connection] = struct{}{}
	d.connMu.Unlock()
	defer func() {
		d.connMu.Lock()
		delete(d.connections, connection)
		d.connMu.Unlock()
		_ = connection.Close()
	}()

	// Bound IPC request size and idle connections. Pairing confirmation is
	// interactive, so allow up to three minutes for the full request exchange.
	_ = connection.SetDeadline(time.Now().Add(3 * time.Minute))
	decoder, encoder := json.NewDecoder(io.LimitReader(connection, 1<<20)), json.NewEncoder(connection)
	var req models.Request

	if err := decoder.Decode(&req); err != nil {
		_ = encoder.Encode(models.Response{Error: err.Error()})
		return
	}

	req.PeerDeviceID = strings.ToLower(strings.TrimSpace(req.PeerDeviceID))
	if req.Command != "devices" && req.Command != "discover" && !strings.HasPrefix(req.Command, "media-") && req.PeerDeviceID == "" {
		_ = encoder.Encode(models.Response{Error: "peer_device_id is required"})
		return
	}

	switch req.Command {
	case "devices":
		d.devices(req, encoder)

	case "discover":
		d.discover(req, encoder)

	case "media-list":
		d.mediaList(req, encoder)

	case "media-active":
		d.mediaActive(req, encoder)

	case "media-set-active":
		d.mediaSetActive(req, encoder)

	case "media-play-pause", "media-next", "media-previous", "media-play", "media-pause", "media-stop", "media-seek", "media-shuffle", "media-loop", "media-volume":
		d.mediaControl(req, encoder)

	case "pair":
		d.pair(req, decoder, encoder)

	case "connect":
		d.connect(req, encoder)

	case "disconnect":
		d.disconnect(req, encoder)

	case "ping":
		d.ping(req, encoder)

	case "ring":
		d.ring(req, encoder)

	case "clipboard-push":
		d.clipboardPush(req, encoder)

	case "clipboard-service-start":
		d.clipboardServiceStart(req, encoder)

	case "clipboard-service-stop":
		d.clipboardServiceStop(req, encoder)

	case "clipboard-sync-on":
		d.setClipboardSync(req, encoder, true)

	case "clipboard-sync-off":
		d.setClipboardSync(req, encoder, false)

	case "input-enable":
		d.setInputEnabled(req, encoder, true)

	case "input-disable":
		d.setInputEnabled(req, encoder, false)

	case "notif-sync-both":
		d.setNotificationSync(req, encoder, config.NotificationSyncBoth)

	case "notif-sync-stop":
		d.setNotificationSync(req, encoder, config.NotificationSyncNone)

	case "notif-sync-toclient":
		d.setNotificationSync(req, encoder, config.NotificationSyncToClient)

	case "notif-sync-tohost":
		d.setNotificationSync(req, encoder, config.NotificationSyncToHost)

	default:
		_ = encoder.Encode(models.Response{Error: fmt.Sprintf("unsupported daemon command %q", req.Command)})
	}
}

func (d *Daemon) devices(req models.Request, enc *json.Encoder) {
	d.mu.Lock()
	defer d.mu.Unlock()

	connected := make([]string, 0, len(d.sessions))
	for deviceID, managed := range d.sessions {
		if managed.Active != nil {
			connected = append(connected, deviceID)
		}
	}

	if req.PeerDeviceID == "" {
		_ = enc.Encode(models.Response{Devices: d.store.Devices(), ConnectedDeviceIDs: connected})
		return
	}

	device, ok := d.store.Device(req.PeerDeviceID)
	if !ok {
		_ = enc.Encode(models.Response{Error: "peer device not found"})
		return
	}

	_ = enc.Encode(models.Response{Device: &device})
}

func (d *Daemon) discover(req models.Request, enc *json.Encoder) {
	ctx, cancel := context.WithTimeout(d.ctx, utils.Duration(req.TimeoutMS, 10*time.Second))
	defer cancel()

	devices, err := discovery.Discover(ctx)
	if err != nil && ctx.Err() == nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	_ = enc.Encode(models.Response{Discovered: devices})
}

func (d *Daemon) pair(req models.Request, dec *json.Decoder, enc *json.Encoder) {
	ctx, cancel := context.WithTimeout(d.ctx, utils.Duration(req.TimeoutMS, 2*time.Minute))
	defer cancel()

	client, err := discovery.NewClient()
	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	defer func() { _ = client.Close() }()
	remote, err := client.Find(ctx, req.PeerDeviceID)

	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	connection, err := remote.Connect(ctx)
	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	defer func() { _ = connection.Close() }()
	result, err := pairing.Initiate(ctx, connection.ProtocolConn(), lq.InitiatorOptions{DeviceName: utils.LocalDeviceName(), RemoteDeviceName: remote.Name, ExpectedPeerDeviceID: req.PeerDeviceID}, func(code uint32, peerName string) (bool, error) {
		if err := enc.Encode(models.Response{Event: "pairing_code", Code: code, PeerName: peerName}); err != nil {
			return false, err
		}

		var confirmation models.Request
		if err := dec.Decode(&confirmation); err != nil {
			return false, err
		}

		if confirmation.Command != "confirm_pairing" || confirmation.Confirm == nil {
			return false, errors.New("expected pairing confirmation")
		}

		return *confirmation.Confirm, nil
	})

	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	if result.PeerDeviceID != req.PeerDeviceID {
		_ = enc.Encode(models.Response{Error: "advertised peer_device_id does not match pairing identity"})
		return
	}

	// pairing persists through its own config transaction; refresh our daemon view
	// before accepting a follow-up connect request.
	if refreshed, refreshErr := config.Open(""); refreshErr == nil {
		d.mu.Lock()
		d.store = refreshed
		d.mu.Unlock()
	}

	_ = enc.Encode(models.Response{Device: result})
}

func (d *Daemon) ping(req models.Request, enc *json.Encoder) {
	d.mu.Lock()

	managed := d.sessions[req.PeerDeviceID]
	var active *session.Session

	if managed != nil {
		active = managed.Active
	}

	d.mu.Unlock()

	if active == nil {
		_ = enc.Encode(models.Response{Error: "no active session; use connect first"})
		return
	}

	ctx, cancel := context.WithTimeout(d.ctx, utils.Duration(req.TimeoutMS, 15*time.Second))
	defer cancel()

	if err := active.SendPing(ctx); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	_ = enc.Encode(models.Response{})
}

func (d *Daemon) ring(req models.Request, enc *json.Encoder) {
	d.mu.Lock()

	managed := d.sessions[req.PeerDeviceID]
	var active *session.Session

	if managed != nil {
		active = managed.Active
	}

	d.mu.Unlock()

	if active == nil {
		_ = enc.Encode(models.Response{Error: "no active session; use connect first"})
		return
	}

	ctx, cancel := context.WithTimeout(d.ctx, utils.Duration(req.TimeoutMS, 15*time.Second))
	defer cancel()

	if err := active.SendRing(ctx); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	_ = enc.Encode(models.Response{})
}

func (d *Daemon) clipboardPush(req models.Request, enc *json.Encoder) {
	d.mu.Lock()

	managed := d.sessions[req.PeerDeviceID]
	var active *session.Session

	if managed != nil {
		active = managed.Active
	}

	d.mu.Unlock()

	if active == nil {
		_ = enc.Encode(models.Response{Error: "no active session; use connect first"})
		return
	}

	d.mu.Lock()
	clipboardReady := d.clipboardReady
	d.mu.Unlock()
	if !clipboardReady {
		_ = enc.Encode(models.Response{Error: "host clipboard is unavailable in this graphical session"})
		return
	}
	ctx, cancel := context.WithTimeout(d.ctx, utils.Duration(req.TimeoutMS, 15*time.Second))
	defer cancel()

	content, err := clip.Read(ctx, clip.FmtText)
	if err != nil {
		_ = enc.Encode(models.Response{Error: "failed to read clipboard content"})
		return
	}

	if len(content) > 48*1024 {
		_ = enc.Encode(models.Response{Error: "clipboard content exceeds 48 KiB sync limit"})
		return
	}
	clipboardContent := lq.ClipboardContent{
		ContentType: lq.Text,
		Content:     content,
	}

	// This command explicitly names one peer; do not route it through the
	// automatic clipboard broadcast callback.
	err = active.SyncClipboard(ctx, clipboardContent)
	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	_ = enc.Encode(models.Response{})

}
