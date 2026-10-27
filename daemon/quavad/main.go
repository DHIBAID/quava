package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"libquava/clipboard"
	"libquava/config"
	"libquava/inputs"
	"libquava/notifications"
	"quavad/models"
)

type Daemon struct {
	ctx                  context.Context
	store                *config.Store
	listener             net.Listener
	mu                   sync.Mutex
	sessions             map[string]*models.ManagedSession
	clipboard            *clipboard.ClipboardService
	dbusNotifications    *notifications.DBusNotificationService
	androidNotifications *notifications.AndroidNotificationBridge
	inputMu              sync.Mutex
	inputDispatchMu      sync.Mutex
	input                inputs.Emulator
	inputEnabled         map[string]bool
	inputOwner           string
	clipboardReady       bool
	connMu               sync.Mutex
	connections          map[net.Conn]struct{}
}

func main() {
	runCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	store, err := config.Open("")
	if err != nil {
		fatal(err)
	}

	path := filepath.Join(store.Dir(), models.ControlSocketName)
	listener, socketActivated, err := systemdListener()
	if err != nil {
		fatal(err)
	}

	if !socketActivated {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			fatal(err)
		}

		listener, err = net.Listen("unix", path)
		if err != nil {
			fatal(err)
		}

		if err := os.Chmod(path, 0o600); err != nil {
			_ = listener.Close()
			fatal(err)
		}
	}

	d := &Daemon{
		ctx:          runCtx,
		store:        store,
		listener:     listener,
		sessions:     map[string]*models.ManagedSession{},
		inputEnabled: map[string]bool{},
		connections:  map[net.Conn]struct{}{},
	}

	// Init clipboard service
	d.clipboard = clipboard.NewClipboardService(d.pushClipboardContent)

	clipboardCtx, clipboardCancel := context.WithCancel(runCtx)
	defer clipboardCancel()

	if err := d.clipboard.StartClipboardService(clipboardCtx); err != nil {
		log.Printf("clipboard: failed to start: %v", err)
	} else {
		d.clipboardReady = true
	}
	defer d.clipboard.StopClipboardService()

	// Init notifications service
	d.dbusNotifications = notifications.NewDBusNotificationWatcher(d.pushNotificationContent, d.clearNotificationContent)
	d.androidNotifications, err = notifications.NewAndroidNotificationBridge(d.sendAndroidNotificationAction, d.dismissAndroidNotification)
	if err != nil {
		log.Printf("notifications: Android notification bridge failed: %v", err)
	}
	if d.androidNotifications != nil {
		defer d.androidNotifications.Close()
	}

	notifCtx, notifCancel := context.WithCancel(runCtx)
	defer notifCancel()

	if err := d.dbusNotifications.StartNotificationService(notifCtx); err != nil {
		log.Printf("notificaions: failed to start: %v", err)
	}
	defer d.dbusNotifications.StopNotificationService()

	defer func() {
		_ = listener.Close()
		if !socketActivated {
			_ = os.Remove(path)
		}
	}()

	var handlers sync.WaitGroup
	go func() {
		<-runCtx.Done()
		_ = listener.Close()
	}()
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if runCtx.Err() != nil || errors.Is(acceptErr, net.ErrClosed) {
				break
			}
			log.Printf("ipc: accept failed: %v", acceptErr)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		handlers.Add(1)
		go func(conn net.Conn) {
			defer handlers.Done()
			d.handle(conn)
		}(connection)
	}
	d.shutdown()
	handlers.Wait()
}

func systemdListener() (net.Listener, bool, error) {
	fds, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil || fds == 0 {
		return nil, false, nil
	}

	pid, err := strconv.Atoi(os.Getenv("LISTEN_PID"))
	if err != nil || pid != os.Getpid() || fds != 1 {
		return nil, false, errors.New("invalid systemd socket activation environment")
	}

	file := os.NewFile(uintptr(3), "quavad.socket")
	listener, err := net.FileListener(file)
	_ = file.Close()

	if err != nil {
		return nil, false, fmt.Errorf("use systemd socket: %w", err)
	}

	return listener, true, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "quavad:", err)
	os.Exit(1)
}
