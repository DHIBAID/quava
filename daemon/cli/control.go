package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"libquava/session"
)

const controlSocketName = "control.sock"

type controlRequest struct {
	Command   string `json:"command"`
	DeviceID  string `json:"device_id"`
	TimeoutMS int64  `json:"timeout_ms"`
}

type controlResponse struct {
	Error string `json:"error,omitempty"`
}

// controlServer exposes commands to local CLI invocations while connect owns
// the sole authenticated TCP session to the Android device.
type controlServer struct {
	listener net.Listener
	peerID   string

	mu      sync.RWMutex
	session *session.Session
}

func newControlServer(path, peerID string) (*controlServer, error) {
	if err := removeStaleControlSocket(path); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &controlServer{listener: listener, peerID: peerID}, nil
}

func removeStaleControlSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("control path %q is not a socket", path)
	}

	connection, err := net.DialTimeout("unix", path, 250*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		return fmt.Errorf("another Quava daemon is already running")
	}
	return os.Remove(path)
}

func (s *controlServer) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	for {
		connection, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(ctx, connection)
	}
}

func (s *controlServer) Close() error {
	if s == nil || s.listener == nil {
		return nil
	}
	path := s.listener.Addr().String()
	err := s.listener.Close()
	if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && err == nil {
		err = removeErr
	}
	return err
}

func (s *controlServer) SetSession(active *session.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = active
}

func (s *controlServer) ClearSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = nil
}

func (s *controlServer) handle(parent context.Context, connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(time.Minute))

	var request controlRequest
	if err := json.NewDecoder(connection).Decode(&request); err != nil {
		s.respond(connection, err)
		return
	}
	if request.Command != "ping" {
		s.respond(connection, fmt.Errorf("unsupported daemon command %q", request.Command))
		return
	}
	if request.DeviceID != s.peerID {
		s.respond(connection, errors.New("requested device is not the active session peer"))
		return
	}

	s.mu.RLock()
	active := s.session
	s.mu.RUnlock()
	if active == nil {
		s.respond(connection, errors.New("no active session; wait for the daemon to connect"))
		return
	}

	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > time.Minute {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	s.respond(connection, active.SendPing(ctx))
}

func (s *controlServer) respond(connection net.Conn, err error) {
	response := controlResponse{}
	if err != nil {
		response.Error = err.Error()
	}
	_ = json.NewEncoder(connection).Encode(response)
}

func sendControlPing(ctx context.Context, path, deviceID string) error {
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return fmt.Errorf("no active daemon session: %w", err)
	}
	defer connection.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	timeoutMS := int64(15_000)
	if deadline, ok := ctx.Deadline(); ok {
		timeoutMS = time.Until(deadline).Milliseconds()
	}
	request := controlRequest{Command: "ping", DeviceID: deviceID, TimeoutMS: timeoutMS}
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return err
	}

	var response controlResponse
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return err
	}
	if response.Error != "" {
		return errors.New(response.Error)
	}
	return nil
}
