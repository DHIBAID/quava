// quavad owns Quava's durable state, discovery, pairing, and authenticated sessions.
// It is intended to run continuously; clients use its owner-only Unix socket.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"quavad/models"
	"quavad/utils"
	"strconv"
	"strings"
	"sync"
	"time"

	"libquava/config"
	"libquava/discovery"
	lq "libquava/models"

	"libquava/pairing"
	"libquava/protocol"
	"libquava/session"
)

type Daemon struct {
	store    *config.Store
	listener net.Listener
	mu       sync.Mutex
	sessions map[string]*models.ManagedSession
}

func main() {
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
		store:    store,
		listener: listener,
		sessions: map[string]*models.ManagedSession{},
	}

	defer func() {
		_ = listener.Close()
		if !socketActivated {
			_ = os.Remove(path)
		}
	}()

	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}

		go d.handle(connection)
	}
}

// systemd passes an already-bound listener as file descriptor 3. Keeping the
// socket owned by systemd lets a CLI connection activate the daemon safely.
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

func (d *Daemon) handle(connection net.Conn) {
	defer func() { _ = connection.Close() }()

	decoder, encoder := json.NewDecoder(connection), json.NewEncoder(connection)
	var req models.Request

	if err := decoder.Decode(&req); err != nil {
		_ = encoder.Encode(models.Response{Error: err.Error()})
		return
	}

	req.PeerDeviceID = strings.ToLower(strings.TrimSpace(req.PeerDeviceID))
	if req.Command != "devices" && req.Command != "discover" && req.PeerDeviceID == "" {
		_ = encoder.Encode(models.Response{Error: "peer_device_id is required"})
		return
	}

	switch req.Command {
	case "devices":
		d.devices(req, encoder)

	case "discover":
		d.discover(req, encoder)

	case "pair":
		d.pair(req, decoder, encoder)

	case "connect":
		d.connect(req, encoder)

	case "ping":
		d.ping(req, encoder)

	case "ring":
		d.ring(req, encoder)

	default:
		_ = encoder.Encode(models.Response{Error: fmt.Sprintf("unsupported daemon command %q", req.Command)})
	}
}

func (d *Daemon) devices(req models.Request, enc *json.Encoder) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if req.PeerDeviceID == "" {
		_ = enc.Encode(models.Response{Devices: d.store.Devices()})
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
	ctx, cancel := context.WithTimeout(context.Background(), utils.Duration(req.TimeoutMS, 10*time.Second))
	defer cancel()

	devices, err := discovery.Discover(ctx)
	if err != nil && ctx.Err() == nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	_ = enc.Encode(models.Response{Discovered: devices})
}

func (d *Daemon) pair(req models.Request, dec *json.Decoder, enc *json.Encoder) {
	ctx, cancel := context.WithTimeout(context.Background(), utils.Duration(req.TimeoutMS, 2*time.Minute))
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
	ctx, cancel := context.WithCancel(context.Background())
	d.sessions[peer.PeerDeviceID] = &models.ManagedSession{Cancel: cancel}
	d.mu.Unlock()

	go d.runSession(ctx, identity, peer)

	_ = enc.Encode(models.Response{})
}

func (d *Daemon) runSession(ctx context.Context, identity lq.IdentityFile, peer lq.PairResult) {
	client, err := discovery.NewClient()

	if err != nil {
		d.clearSession(peer.PeerDeviceID)
		return
	}

	defer func() { _ = client.Close() }()

	_ = session.Run(ctx, identity, peer, func(dialCtx context.Context) (*protocol.Conn, error) {
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

		d.mu.Unlock()
	})

	d.clearSession(peer.PeerDeviceID)
}

func (d *Daemon) clearSession(id string) {
	d.mu.Lock()
	delete(d.sessions, id)
	d.mu.Unlock()
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

	ctx, cancel := context.WithTimeout(context.Background(), utils.Duration(req.TimeoutMS, 15*time.Second))
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

	ctx, cancel := context.WithTimeout(context.Background(), utils.Duration(req.TimeoutMS, 15*time.Second))
	defer cancel()

	if err := active.SendRing(ctx); err != nil {
		_ = enc.Encode(models.Response{Error: err.Error()})
		return
	}

	_ = enc.Encode(models.Response{})
}
