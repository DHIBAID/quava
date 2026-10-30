package notifications

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"libquava/models"

	"github.com/godbus/dbus/v5"
)

// TODO: Allow supressing notifications based on types
// TODO: Allow enabling/disabling notifications for specific Apps
// TODO: Allow forwarding of notification types (slient notif is silent on host aswell)

// AndroidNotificationBridge renders Android notifications locally and turns
// D-Bus action/close signals back into Android operations.
type AndroidNotificationBridge struct {
	mu       sync.Mutex
	conn     *dbus.Conn
	byLinux  map[uint32]androidNotification
	byKey    map[string]uint32
	replying map[string]bool
	action   func(context.Context, string, int, string) error
	dismiss  func(context.Context, string) error
}

// dbusImageData is the standard freedesktop image-data struct (iiibiiay).
// It gives the desktop server real pixels instead of an Android-only icon URI.
type dbusImageData struct {
	Width         int32
	Height        int32
	RowStride     int32
	HasAlpha      bool
	BitsPerSample int32
	Channels      int32
	Data          []byte
}

type androidNotification struct {
	key     string
	appName string
	actions []models.AndroidNotificationAction
}

// Android-originated D-Bus notification IDs must never re-enter the ordinary
// Linux-to-Android watcher. Keep IDs for the daemon lifetime: servers do not
// normally reuse IDs, and retaining them prevents a close-event race between
// the bridge and the watcher from reintroducing the forwarding loop.
var androidOriginIDs = struct {
	sync.RWMutex
	ids map[uint32]struct{}
}{ids: make(map[uint32]struct{})}

func NewAndroidNotificationBridge(action func(context.Context, string, int, string) error, dismiss func(context.Context, string) error) (*AndroidNotificationBridge, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("notifications: connect to session bus: %w", err)
	}
	b := &AndroidNotificationBridge{conn: conn, byLinux: make(map[uint32]androidNotification), byKey: make(map[string]uint32), replying: make(map[string]bool), action: action, dismiss: dismiss}
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.Notifications")); err != nil {
		if err := conn.Close(); err != nil {
			return nil, err
		}

		return nil, err
	}
	signals := make(chan *dbus.Signal, 32)
	conn.Signal(signals)
	go b.watch(signals)
	return b, nil
}

func (b *AndroidNotificationBridge) Show(_ context.Context, n models.AndroidNotificationContent) error {
	actions := make([]string, 0, len(n.Actions)*2)
	for i, action := range n.Actions {
		actions = append(actions, fmt.Sprintf("quava-action-%d", i), action.Title)
	}
	hints := map[string]dbus.Variant{"x-quava-android": dbus.MakeVariant(true)}
	if icon, ok := decodeIcon(n.IconData); ok {
		hints["image-data"] = dbus.MakeVariant(icon)
	}
	if anyReplyAction(n.Actions) {
		hints["x-kde-reply-placeholder-text"] = dbus.MakeVariant("Reply")
	}
	b.mu.Lock()
	replacesID := b.byKey[n.Key]
	b.mu.Unlock()
	var id uint32
	call := b.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").Call("org.freedesktop.Notifications.Notify", 0,
		"quava", replacesID, "", n.Title, n.Body, actions, hints, int32(-1))
	if call.Err != nil {
		return call.Err
	}
	if err := call.Store(&id); err != nil {
		return err
	}
	b.mu.Lock()
	if replacesID != 0 && replacesID != id {
		delete(b.byLinux, replacesID)
	}
	b.byLinux[id] = androidNotification{key: n.Key, appName: n.AppName, actions: n.Actions}
	b.byKey[n.Key] = id
	b.mu.Unlock()
	androidOriginIDs.Lock()
	androidOriginIDs.ids[id] = struct{}{}
	androidOriginIDs.Unlock()
	log.Printf("notifications: rendered Android notification id=%d actions=%d key=%q", id, len(n.Actions), n.Key)
	return nil
}

func (b *AndroidNotificationBridge) watch(signals <-chan *dbus.Signal) {
	for signal := range signals {
		if signal == nil || len(signal.Body) == 0 {
			continue
		}
		id, ok := signal.Body[0].(uint32)
		if !ok {
			continue
		}
		b.mu.Lock()
		notification, found := b.byLinux[id]
		isReplying := b.replying[notification.key]
		if signal.Name == "org.freedesktop.Notifications.NotificationClosed" {
			delete(b.byLinux, id)
			delete(b.byKey, notification.key)
		}
		b.mu.Unlock()
		if !found {
			continue
		}
		switch signal.Name {
		case "org.freedesktop.Notifications.ActionInvoked":
			if len(signal.Body) < 2 {
				continue
			}
			name, _ := signal.Body[1].(string)
			var index int
			if _, err := fmt.Sscanf(name, "quava-action-%d", &index); err == nil && b.action != nil {
				log.Printf("notifications: Android notification action id=%d action=%d", id, index)
				if index >= 0 && index < len(notification.actions) && notification.actions[index].HasRemoteInput {
					b.mu.Lock()
					b.replying[notification.key] = true
					b.mu.Unlock()
					go b.promptAndReply(notification, index)
				} else {
					_ = b.action(context.Background(), notification.key, index, "")
				}
			}
		case "org.freedesktop.Notifications.NotificationReplied":
			if len(signal.Body) < 2 || b.action == nil {
				continue
			}
			reply, _ := signal.Body[1].(string)
			for index, action := range notification.actions {
				if action.HasRemoteInput {
					_ = b.action(context.Background(), notification.key, index, reply)
					break
				}
			}
		case "org.freedesktop.Notifications.NotificationClosed":
			if b.dismiss != nil && !isReplying {
				log.Printf("notifications: Android notification dismissed on Linux id=%d", id)
				_ = b.dismiss(context.Background(), notification.key)
			}
		}
	}
}

// Small native GTK entry dialog and send its text as Android RemoteInput.
func (b *AndroidNotificationBridge) promptAndReply(notification androidNotification, action int) {
	defer func() { b.mu.Lock(); delete(b.replying, notification.key); b.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "zenity", "--entry", "--title=Quava — Reply", "--text="+notification.actions[action].Title+" to "+notification.appName, "--width=420").Output()
	if err != nil {
		log.Printf("notifications: reply prompt cancelled or unavailable: %v", err)
		return
	}
	reply := strings.TrimSpace(string(output))
	if reply == "" || b.action == nil {
		return
	}
	if err := b.action(ctx, notification.key, action, reply); err != nil {
		log.Printf("notifications: send reply failed: %v", err)
	}
}

func decodeIcon(encoded []byte) (dbusImageData, bool) {
	if len(encoded) == 0 {
		return dbusImageData{}, false
	}
	decoded, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return dbusImageData{}, false
	}
	bounds := decoded.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 || bounds.Dx() > 256 || bounds.Dy() > 256 {
		return dbusImageData{}, false
	}
	rgba := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), decoded, bounds.Min, draw.Src)
	return dbusImageData{Width: int32(bounds.Dx()), Height: int32(bounds.Dy()), RowStride: int32(rgba.Stride), HasAlpha: true, BitsPerSample: 8, Channels: 4, Data: rgba.Pix}, true
}

func anyReplyAction(actions []models.AndroidNotificationAction) bool {
	for _, action := range actions {
		if action.HasRemoteInput {
			return true
		}
	}
	return false
}

func (b *AndroidNotificationBridge) Close() error {
	if b == nil || b.conn == nil {
		return nil
	}
	return b.conn.Close()
}

func isAndroidOriginNotification(id uint32) bool {
	androidOriginIDs.RLock()
	_, found := androidOriginIDs.ids[id]
	androidOriginIDs.RUnlock()
	return found
}
