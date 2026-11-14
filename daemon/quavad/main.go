package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"sync"
	"syscall"
	"time"

	"libquava/clipboard"
	"libquava/config"
	"libquava/discovery"
	"libquava/inputs"
	"libquava/notifications"

	"libquava/profiling"
	"libquava/session"
	quavassh "libquava/ssh"
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
	watcher              *discovery.Watcher
	sshService           *quavassh.Service
	audioAutoMuted       bool // true while a phone call put the host's audio into a mute WE applied
	audioWasMutedBefore  bool // the host's own mute state right before we auto-muted it
}

// startProfiling starts the profiling server and returns its shutdown function.
// The caller must defer the returned function: deferring inside this function
// would stop the server as soon as it returned.
func startProfiling() func() {
	profiler, err := profiling.Start(
		profiling.Config{
			Address:              "127.0.0.1:6060",
			MutexProfileFraction: 5,
			BlockProfileRate:     1,
		},
		slog.Default(),
	)

	if err != nil {
		slog.Error("profiling initialization failed", "error", err)
		return func() {}
	}

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := profiler.Stop(ctx); err != nil {
			slog.Error("profiling shutdown failed", "error", err)
		}
	}
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
	if service, err := quavassh.NewService(filepath.Join(store.Dir(), "ssh_host_key")); err != nil {
		log.Printf("SSH service unavailable: %v", err)
	} else {
		d.sshService = service
	}

	// pprof exposes process memory and goroutines to other local users, so
	// enable its HTTP listener only when explicitly requested for debugging.
	if os.Getenv("QUAVA_ENABLE_PPROF") == "1" {
		defer startProfiling()()
	}

	if err := session.StartMedia(runCtx); err != nil {
		log.Printf("mpris: service unavailable: %v", err)
	} else {
		defer func() {
			if err := session.StopMedia(); err != nil {
				log.Printf("mpris: shutdown failed: %v", err)
			}
		}()
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
		d.androidNotifications.SetFilters(d.store.NotificationFilters())
		defer func() {
			if err := d.androidNotifications.Close(); err != nil {
				log.Printf("failed to stop Android Notification service: %v", err)
			}
		}()
	}

	notifCtx, notifCancel := context.WithCancel(runCtx)
	defer notifCancel()

	if err := d.dbusNotifications.StartNotificationService(notifCtx); err != nil {
		log.Printf("notificaions: failed to start: %v", err)
	}

	defer func() {
		if err := d.dbusNotifications.StopNotificationService(); err != nil {
			log.Printf("failed to close D-Bus connection: %v", err)
		}
	}()

	defer func() {
		_ = listener.Close()
		if !socketActivated {
			_ = os.Remove(path)
		}
	}()

	// Reconnect to paired phones on its own. The watcher listens for their
	// announcements instead of polling the network.
	if watcher, err := discovery.NewWatcher(); err != nil {
		log.Printf("discovery: watcher unavailable, falling back to per-session discovery: %v", err)
	} else {
		watcher.Start(runCtx)
		d.watcher = watcher
	}
	d.startAutoConnect()

	var handlers sync.WaitGroup
	go func() {
		<-runCtx.Done()
		_ = listener.Close()

		// Shutdown watchdog: if cleanup blocks (a hung D-Bus call, a stuck
		// handler), a stopping daemon would otherwise sit in "deactivating"
		// until systemd's SIGKILL, with the control socket unanswered and every
		// CLI call hanging. Dump goroutines so the cause shows up in the journal.
		time.Sleep(8 * time.Second)
		log.Printf("shutdown: cleanup did not finish within 8s; dumping goroutines and exiting")
		_ = pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
		os.Exit(1)
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
		// Not started by systemd (the variable is unset or not a number); the
		// caller opens the socket itself.
		return nil, false, nil //nolint:nilerr
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
