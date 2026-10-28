package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"libquava/clipboard"
	"libquava/config"
	lq "libquava/models"
	"libquava/session"
	"quavad/models"
)

func (d *Daemon) pushClipboardContent(ctx context.Context, content lq.ClipboardContent) error {
	d.mu.Lock()

	activeSessions := make([]*session.Session, 0, len(d.sessions))

	for deviceID, managed := range d.sessions {
		if managed.Active != nil && d.store.ClipboardSync(deviceID) {
			activeSessions = append(activeSessions, managed.Active)
		}
	}

	d.mu.Unlock()

	var errs []error

	for _, active := range activeSessions {
		if err := active.SyncClipboard(ctx, content); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (d *Daemon) pushNotificationContent(ctx context.Context, content lq.NotificationContent) error {
	d.mu.Lock()

	activeSessions := make([]*session.Session, 0, len(d.sessions))

	for deviceID, managed := range d.sessions {
		if managed.Active != nil && notificationSyncsToClient(d.store.NotificationSync(deviceID)) {
			activeSessions = append(activeSessions, managed.Active)
		}
	}

	d.mu.Unlock()

	var errs []error

	for _, active := range activeSessions {
		if err := active.SyncNotifications(ctx, content); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (d *Daemon) clearNotificationContent(ctx context.Context, id uint32) error {
	d.mu.Lock()

	activeSessions := make([]*session.Session, 0, len(d.sessions))

	for deviceID, managed := range d.sessions {
		if managed.Active != nil && notificationSyncsToClient(d.store.NotificationSync(deviceID)) {
			activeSessions = append(activeSessions, managed.Active)
		}
	}

	d.mu.Unlock()

	var errs []error

	for _, active := range activeSessions {
		if err := active.ClearNotification(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// systemd passes an already-bound listener as file descriptor 3. Keeping the
// socket owned by systemd lets a CLI connection activate the daemon safely.

func (d *Daemon) sendAndroidNotificationAction(ctx context.Context, key string, action int, reply string) error {
	return d.withActiveSession(func(active *session.Session) error {
		return active.SendAndroidNotificationAction(ctx, key, action, reply)
	})
}

func (d *Daemon) dismissAndroidNotification(ctx context.Context, key string) error {
	return d.withActiveSession(func(active *session.Session) error { return active.DismissAndroidNotification(ctx, key) })
}

func (d *Daemon) withActiveSession(fn func(*session.Session) error) error {
	d.mu.Lock()
	active := make([]*session.Session, 0, len(d.sessions))
	for _, managed := range d.sessions {
		if managed.Active != nil {
			active = append(active, managed.Active)
		}
	}
	d.mu.Unlock()
	var errs []error
	for _, s := range active {
		if err := fn(s); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (d *Daemon) clipboardServiceStart(_ models.Request, enc *json.Encoder) {
	if d.clipboard == nil {
		d.clipboard = clipboard.NewClipboardService(d.pushClipboardContent)
	}

	if err := d.clipboard.StartClipboardService(d.ctx); err != nil {
		if err.Error() != "clipboard: service already running" {
			log.Printf("clipboard: failed to start: %v", err)
			_ = enc.Encode(models.Response{Error: err.Error()})
			return
		}
	}
	d.mu.Lock()
	d.clipboardReady = true
	d.mu.Unlock()

	_ = enc.Encode(models.Response{})
}

func (d *Daemon) clipboardServiceStop(_ models.Request, enc *json.Encoder) {
	if d.clipboard == nil {
		_ = enc.Encode(models.Response{})
		return
	}

	d.clipboard.StopClipboardService()
	// Stop disables automatic watching, not clipboard read/write capability.
	_ = enc.Encode(models.Response{})
}

func (d *Daemon) setClipboardSync(req models.Request, enc *json.Encoder, enabled bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.store.SetClipboardSync(req.PeerDeviceID, enabled); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	if err := d.store.Save(); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	_ = enc.Encode(models.Response{})
}

func (d *Daemon) setNotificationSync(req models.Request, enc *json.Encoder, direction config.NotificationSyncDirection) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, paired := d.store.Device(req.PeerDeviceID); !paired {
		_ = enc.Encode(models.Response{Error: "peer device is not paired"})
		return
	}
	if err := d.store.SetNotificationSync(req.PeerDeviceID, direction); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	if err := d.store.Save(); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	_ = enc.Encode(models.Response{})
}

func notificationSyncsToHost(direction config.NotificationSyncDirection) bool {
	return direction == config.NotificationSyncBoth || direction == config.NotificationSyncToHost
}

func notificationSyncsToClient(direction config.NotificationSyncDirection) bool {
	return direction == config.NotificationSyncBoth || direction == config.NotificationSyncToClient
}

// notificationSyncsToHost returns the persisted per-device direction under the
// daemon state lock before applying the shared direction predicate.
func (d *Daemon) notificationSyncsToHost(deviceID string) bool {
	d.mu.Lock()
	direction := d.store.NotificationSync(deviceID)
	d.mu.Unlock()
	return notificationSyncsToHost(direction)
}

func (d *Daemon) mediaList(_ models.Request, enc *json.Encoder) {
	players, err := session.ListMediaPlayers()
	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	_ = enc.Encode(models.Response{Players: players})
}

func (d *Daemon) mediaActive(_ models.Request, enc *json.Encoder) {
	player, err := session.ActiveMediaPlayer()
	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	_ = enc.Encode(models.Response{Player: &player})
}

func (d *Daemon) mediaSetActive(req models.Request, enc *json.Encoder) {
	player, err := session.SetActiveMediaPlayer(req.BusName)
	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	_ = enc.Encode(models.Response{Player: &player})
}

func (d *Daemon) mediaControl(req models.Request, enc *json.Encoder) {
	err := session.ControlMedia(session.MediaCommand{Command: req.Command, BusName: req.BusName, OffsetMS: req.OffsetMS, Enabled: req.Enabled, LoopStatus: req.LoopStatus, Volume: req.Volume})
	if err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}
	_ = enc.Encode(models.Response{})
}
