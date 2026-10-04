package notifications

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"libquava/models"

	"github.com/godbus/dbus/v5"
)

type DBusNotificationService struct {
	LastUpdated           time.Time
	Notification          models.NotificationContent
	pendingMutex          sync.Mutex
	pendingNotifs         map[string]models.NotificationContent // Notify caller + call serial -> content
	notificationIDMapLock sync.Mutex
	notificationIDMap     map[uint32]uint32 // Android notification ID -> Linux notification ID
	lifecycle             sync.Mutex
	push                  DBusPushFunc
	clear                 DBusClearFunc
	cancel                context.CancelFunc
	conn                  *dbus.Conn
	messages              chan *dbus.Message
	done                  chan struct{}
	started               bool
}

type DBusPushFunc func(context.Context, models.NotificationContent) error

type DBusClearFunc func(context.Context, uint32) error

var notificationIDs = struct {
	sync.Mutex
	androidToLinux map[uint32]uint32
}{
	androidToLinux: make(map[uint32]uint32),
}

func NewDBusNotificationWatcher(push DBusPushFunc, clear DBusClearFunc) *DBusNotificationService {
	return &DBusNotificationService{
		push:              push,
		clear:             clear,
		pendingNotifs:     make(map[string]models.NotificationContent),
		notificationIDMap: make(map[uint32]uint32),
	}
}

func (d *DBusNotificationService) StartNotificationService(ctx context.Context) error {
	d.lifecycle.Lock()
	defer d.lifecycle.Unlock()
	if d.started {
		return errors.New("notifications: service already running")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("notifications: connect to session bus: %w", err)
	}
	// DBus configuration for becoming a notification watcher.
	matches := []string{
		// All standard notification submissions, regardless of sender or path.
		"interface='org.freedesktop.Notifications',member='Notify'",

		// Replies and errors needed to associate Notify calls with server IDs.
		"type='method_return'",
		"type='error'",

		// Notification dismissal events.
		"type='signal',interface='org.freedesktop.Notifications',member='NotificationClosed'",
	}

	err = conn.BusObject().Call("org.freedesktop.DBus.Monitoring.BecomeMonitor", 0, matches, uint32(0)).Err
	if err != nil {
		conn.Close()
		return fmt.Errorf("notifications: become monitor: %w", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	messages := make(chan *dbus.Message, 100)

	done := make(chan struct{})
	conn.Eavesdrop(messages)

	d.conn = conn
	d.cancel = cancel
	d.messages = messages
	d.done = done
	d.started = true

	go d.run(workerCtx, conn, messages, done)

	return nil
}

func (d *DBusNotificationService) PushNotificationContent(ctx context.Context, content models.NotificationContent) error {
	d.pendingMutex.Lock()
	d.Notification = content
	d.LastUpdated = time.Now()
	d.pendingMutex.Unlock()
	if d.push == nil {
		return nil
	}
	return d.push(ctx, content)
}

func (d *DBusNotificationService) ClearNotificationContent(ctx context.Context, id uint32) error {
	if d.clear == nil {
		return nil
	}
	return d.clear(ctx, id)
}

func (d *DBusNotificationService) GetLatestNotification() (models.NotificationContent, time.Time) {
	d.pendingMutex.Lock()
	defer d.pendingMutex.Unlock()
	return d.Notification, d.LastUpdated
}

func (d *DBusNotificationService) StopNotificationService() error {
	d.lifecycle.Lock()
	if !d.started {
		d.lifecycle.Unlock()
		return nil
	}
	cancel := d.cancel
	conn := d.conn
	done := d.done
	d.lifecycle.Unlock()
	if cancel != nil {
		cancel()
	}
	// Closing the connection also terminates D-Bus monitoring.
	if conn != nil {
		conn.Close()
	}
	<-done
	return nil
}

func (d *DBusNotificationService) run(ctx context.Context, conn *dbus.Conn, messages <-chan *dbus.Message, done chan<- struct{}) {
	defer close(done)
	defer conn.Close()
	defer d.clearAllNotificationIDMappings()
	defer func() {
		d.lifecycle.Lock()
		d.started = false
		d.conn = nil
		d.cancel = nil
		d.messages = nil
		d.done = nil
		d.lifecycle.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}
			if msg.Type == dbus.TypeError {
				if replySerial, ok := getHeaderUint32(msg, dbus.FieldReplySerial); ok {
					destination, hasDestination := getHeaderString(msg, dbus.FieldDestination)
					if !hasDestination {
						continue
					}
					requestKey := fmt.Sprintf("%s:%d", destination, replySerial)
					d.pendingMutex.Lock()
					_, found := d.pendingNotifs[requestKey]
					delete(d.pendingNotifs, requestKey)
					d.pendingMutex.Unlock()
					if found {
						log.Printf("notifications: Notify request failed: %v", msg)
					}
				}
				continue
			}
			// A monitor sees replies for all method calls. Match a Notify reply by
			// reply serial; a FIFO queue would associate unrelated uint32 replies
			// with notifications.
			if actualID, replySerial, err := parseNotificationReturn(msg); err == nil {
				destination, hasDestination := getHeaderString(msg, dbus.FieldDestination)
				if !hasDestination {
					continue
				}
				requestKey := fmt.Sprintf("%s:%d", destination, replySerial)
				d.pendingMutex.Lock()
				content, found := d.pendingNotifs[requestKey]
				if found {
					delete(d.pendingNotifs, requestKey)
				}
				d.pendingMutex.Unlock()
				if !found {
					continue
				}
				previousID := content.ReplacesID
				content.ReplacesID = actualID
				d.storeNotificationIDMapping(actualID, actualID)
				if previousID != 0 && previousID != actualID {
					d.clearNotificationIDMapping(previousID)
				}
				if err := d.PushNotificationContent(ctx, content); err != nil {
					if ctx.Err() == nil {
						log.Printf("notifications: sync failed: %v", err)
					}
				}
				continue
			}
			if id, err := parseNotificationClosed(msg); err == nil {
				if isAndroidOriginNotification(id) {
					continue
				}
				d.clearNotificationIDMapping(id)
				if err := d.ClearNotificationContent(ctx, id); err != nil {
					if ctx.Err() == nil {
						log.Printf("notifications: clear failed: %v", err)
					}
				}
				continue
			}
			content, err := parseNotification(msg)
			if err != nil {
				continue
			}
			// Notifications rendered by AndroidNotificationBridge are already
			// Android-originated. Forwarding them again creates a host <-> phone
			// notification loop.
			if origin, ok := content.Hints["x-quava-android"]; ok {
				if marked, _ := origin.Value().(bool); marked {
					continue
				}
			}
			sender, hasSender := getHeaderString(msg, dbus.FieldSender)
			if !hasSender {
				continue
			}
			content.IconData = loadNotificationIcon(notificationIconPath(content))
			// Never expose replaces_id as the Android notification ID. Hold every
			// Notify call until its reply gives us the server-assigned ID.
			d.pendingMutex.Lock()
			d.pendingNotifs[fmt.Sprintf("%s:%d", sender, msg.Serial())] = content
			d.pendingMutex.Unlock()
		}
	}
}

const maxNotificationIconBytes = 512 * 1024

func notificationIconPath(content models.NotificationContent) string {
	if content.AppIcon != "" {
		return content.AppIcon
	}
	if imagePath, ok := content.Hints["image-path"]; ok {
		if path, ok := imagePath.Value().(string); ok {
			return path
		}
	}
	return ""
}

// loadNotificationIcon copies only small, decodable raster icons. The path in
// Notify belongs to the Linux host and cannot be resolved by Android.

func loadNotificationIcon(path string) []byte {
	if !filepath.IsAbs(path) {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxNotificationIconBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxNotificationIconBytes {
		return nil
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil
	}
	return data
}

func (d *DBusNotificationService) storeNotificationIDMapping(androidID, linuxID uint32) {
	d.notificationIDMapLock.Lock()
	d.notificationIDMap[androidID] = linuxID
	d.notificationIDMapLock.Unlock()
	notificationIDs.Lock()
	notificationIDs.androidToLinux[androidID] = linuxID
	notificationIDs.Unlock()
}

func (d *DBusNotificationService) clearNotificationIDMapping(androidID uint32) {
	d.notificationIDMapLock.Lock()
	delete(d.notificationIDMap, androidID)
	d.notificationIDMapLock.Unlock()
	notificationIDs.Lock()
	delete(notificationIDs.androidToLinux, androidID)
	notificationIDs.Unlock()
}

func (d *DBusNotificationService) clearAllNotificationIDMappings() {
	d.notificationIDMapLock.Lock()
	ids := make([]uint32, 0, len(d.notificationIDMap))
	for id := range d.notificationIDMap {
		ids = append(ids, id)
	}
	clear(d.notificationIDMap)
	d.notificationIDMapLock.Unlock()
	notificationIDs.Lock()
	for _, id := range ids {
		delete(notificationIDs.androidToLinux, id)
	}
	notificationIDs.Unlock()
}

func CloseNotification(id uint32) error {
	notificationIDs.Lock()
	linuxID, ok := notificationIDs.androidToLinux[id]
	notificationIDs.Unlock()
	if ok {
		id = linuxID
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("notifications: connect to session bus: %w", err)
	}
	defer conn.Close()
	obj := conn.Object("org.freedesktop.Notifications", dbus.ObjectPath("/org/freedesktop/Notifications"))
	return obj.Call("org.freedesktop.Notifications.CloseNotification", 0, id).Err
}

func parseNotificationClosed(msg *dbus.Message) (uint32, error) {
	if msg == nil || msg.Type != dbus.TypeSignal {
		return 0, errors.New("not a signal")
	}
	iface, ok := getHeaderString(msg, dbus.FieldInterface)
	if !ok || iface != "org.freedesktop.Notifications" {
		return 0, errors.New("not a notification interface")
	}
	member, ok := getHeaderString(msg, dbus.FieldMember)
	if !ok || member != "NotificationClosed" {
		return 0, errors.New("not a NotificationClosed signal")
	}
	if len(msg.Body) != 2 {
		return 0, errors.New("invalid NotificationClosed argument count")
	}
	id, ok := msg.Body[0].(uint32)
	if !ok {
		return 0, errors.New("invalid notification ID")
	}
	return id, nil
}

func parseNotification(msg *dbus.Message) (models.NotificationContent, error) {
	var content models.NotificationContent
	if msg == nil || msg.Type != dbus.TypeMethodCall {
		return content, errors.New("not a method call")
	}
	iface, ok := getHeaderString(msg, dbus.FieldInterface)
	if !ok || iface != "org.freedesktop.Notifications" {
		return content, errors.New("not a notification interface")
	}
	member, ok := getHeaderString(msg, dbus.FieldMember)
	if !ok || member != "Notify" {
		return content, errors.New("not a Notify call")
	}
	if len(msg.Body) != 8 {
		return content, errors.New("invalid Notify argument count")
	}
	var valid bool
	if content.AppName, valid = msg.Body[0].(string); !valid {
		return content, errors.New("invalid application name")
	}
	if content.ReplacesID, valid = msg.Body[1].(uint32); !valid {
		return content, errors.New("invalid replacement ID")
	}
	if content.AppIcon, valid = msg.Body[2].(string); !valid {
		return content, errors.New("invalid icon")
	}
	if content.Summary, valid = msg.Body[3].(string); !valid {
		return content, errors.New("invalid summary")
	}
	if content.Body, valid = msg.Body[4].(string); !valid {
		return content, errors.New("invalid body")
	}
	if content.Actions, valid = msg.Body[5].([]string); !valid {
		return content, errors.New("invalid actions")
	}
	if content.Hints, valid = msg.Body[6].(map[string]dbus.Variant); !valid {
		return content, errors.New("invalid hints")
	}
	if content.ExpireTimeout, valid = msg.Body[7].(int32); !valid {
		return content, errors.New("invalid expiration timeout")
	}
	return content, nil
}

func getHeaderString(msg *dbus.Message, field dbus.HeaderField) (string, bool) {
	header, ok := msg.Headers[field]
	if !ok {
		return "", false
	}
	value, ok := header.Value().(string)
	return value, ok
}

func parseNotificationReturn(msg *dbus.Message) (uint32, uint32, error) {
	if msg == nil {
		return 0, 0, errors.New("nil message")
	}
	if msg.Type != dbus.TypeMethodReply {
		return 0, 0, errors.New("not a method reply")
	}
	replySerial, ok := getHeaderUint32(msg, dbus.FieldReplySerial)
	if !ok {
		return 0, 0, errors.New("missing Notify reply serial")
	}
	// Check if this could be a return from Notify() by looking at the body
	// Notify() returns a single uint32: the notification ID
	if len(msg.Body) != 1 {
		return 0, 0, errors.New("invalid Notify return argument count")
	}
	id, ok := msg.Body[0].(uint32)
	if !ok {
		return 0, 0, errors.New("invalid Notify return ID type")
	}
	return id, replySerial, nil
}

func getHeaderUint32(msg *dbus.Message, field dbus.HeaderField) (uint32, bool) {
	header, ok := msg.Headers[field]
	if !ok {
		return 0, false
	}
	value, ok := header.Value().(uint32)
	return value, ok
}
