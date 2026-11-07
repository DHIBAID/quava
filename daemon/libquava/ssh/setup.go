package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const sshListenAddr = ":2222"

const sshHandshakeTimeout = 10 * time.Second

var ErrAlreadyRunning = errors.New("ssh service is already running")

// SSHService manages the SSH server listener and configuration.
type SSHService struct {
	mu         sync.Mutex
	listenAddr string
	config     *ssh.ServerConfig
	listener   net.Listener
	conns      map[net.Conn]struct{}
	allowed    map[string]string
}

// NewSSHService creates a new SSH service.
func NewSSHService(listenAddr string, config *ssh.ServerConfig) *SSHService {
	if listenAddr == "" {
		listenAddr = sshListenAddr
	}
	return &SSHService{
		listenAddr: listenAddr,
		config:     config,
		conns:      make(map[net.Conn]struct{}),
		allowed:    make(map[string]string),
	}
}

// SetAllowedAddress associates a Quava peer with the address it used for the
// authenticated transport. SSH accepts connections only from these addresses.
func (s *SSHService) SetAllowedAddress(peerID string, address net.Addr) {
	host := addressHost(address)
	if host == "" {
		s.RemoveAllowedAddress(peerID)
		return
	}

	s.mu.Lock()
	s.allowed[peerID] = host
	s.mu.Unlock()
}

// RemoveAllowedAddress removes a peer from the SSH source allowlist.
func (s *SSHService) RemoveAllowedAddress(peerID string) {
	s.mu.Lock()
	delete(s.allowed, peerID)
	s.mu.Unlock()
}

// NewSSHServiceWithKey creates an SSH service using a persistent host key.
func NewSSHServiceWithKey(listenAddr, keyPath string) (*SSHService, error) {
	signer, err := loadOrGenerateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("ssh: load host key: %w", err)
	}

	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	return NewSSHService(listenAddr, config), nil
}

// Start starts the SSH server and returns once its listener is ready.
func (s *SSHService) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ssh: start canceled: %w", err)
	}
	if s.config == nil {
		return errors.New("ssh service has no server configuration")
	}

	s.mu.Lock()
	if s.listener != nil {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}

	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("ssh: listen: %w", err)
	}
	s.listener = listener
	s.mu.Unlock()

	log.Printf("SSH server listening on %s", listener.Addr())
	go func() {
		<-ctx.Done()
		_ = s.Stop()
	}()
	go s.acceptConnections(listener)
	return nil
}

func (s *SSHService) acceptConnections(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			s.mu.Lock()
			stopped := s.listener != listener
			if !stopped {
				s.listener = nil
				clear(s.allowed)
			}
			s.mu.Unlock()
			if stopped {
				return
			}
			log.Printf("ssh accept failed: %v", err)
			return
		}

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		go s.handleConnection(conn)
	}
}

// Stop closes the listener and all active SSH connections.
func (s *SSHService) Stop() error {
	s.mu.Lock()
	listener := s.listener
	s.listener = nil
	for conn := range s.conns {
		_ = conn.Close()
	}
	s.conns = make(map[net.Conn]struct{})
	s.mu.Unlock()

	if listener == nil {
		return nil
	}
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("ssh: stop: %w", err)
	}
	log.Printf("SSH server stopped")
	return nil
}

// handleConnection creates and manages an SSH session for each connection.
func (s *SSHService) handleConnection(conn net.Conn) {
	defer func() {
		_ = conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()

	if !s.isAllowed(conn.RemoteAddr()) {
		log.Printf("ssh connection rejected from unauthorized address %s", conn.RemoteAddr())
		return
	}

	_ = conn.SetDeadline(time.Now().Add(sshHandshakeTimeout))

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		log.Printf("ssh handshake failed from %s: %v", conn.RemoteAddr(), err)
		return
	}
	defer func() { _ = sshConn.Close() }()
	_ = conn.SetDeadline(time.Time{})

	log.Printf(
		"ssh connection established from %s (user: %s)",
		sshConn.RemoteAddr(),
		sshConn.User(),
	)

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		go s.handleChannel(newChannel)
	}

	log.Printf("ssh connection closed: %s", sshConn.RemoteAddr())
}

// isAllowed reports whether the peer address belongs to an authenticated
// Quava session.
func (s *SSHService) isAllowed(address net.Addr) bool {
	host := addressHost(address)
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, allowedHost := range s.allowed {
		if allowedHost == host {
			return true
		}
	}
	return false
}

func addressHost(address net.Addr) string {
	if address == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(address.String())
	if err == nil {
		return host
	}
	return address.String()
}

// handleChannel handles incoming SSH channels.
func (s *SSHService) handleChannel(newChannel ssh.NewChannel) {
	if newChannel.ChannelType() != "session" {
		_ = newChannel.Reject(
			ssh.UnknownChannelType,
			"unsupported channel type",
		)
		return
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Printf("ssh channel accept failed: %v", err)
		return
	}

	go s.handleSession(channel, requests)
}

func (s *SSHService) handleSession(
	channel ssh.Channel,
	requests <-chan *ssh.Request,
) {
	defer func() { _ = channel.Close() }()

	var requestedPTY *ptyRequest

	for req := range requests {
		switch req.Type {
		case "pty-req":
			var payload ptyRequest
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				_ = req.Reply(false, nil)
				continue
			}

			requestedPTY = &payload
			_ = req.Reply(true, nil)

		case "env":
			_ = req.Reply(false, nil)

		case "shell":
			_ = req.Reply(true, nil)

			if requestedPTY != nil {
				s.runPTYShell(channel, requests, *requestedPTY)
			} else {
				s.runShell(channel)
			}
			return

		case "exec":
			var payload struct {
				Command string
			}

			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				_ = req.Reply(false, nil)
				continue
			}

			_ = req.Reply(true, nil)
			s.runCommand(channel, payload.Command)
			return

		default:
			_ = req.Reply(false, nil)
		}
	}
}

// runShell starts an interactive login shell.
func (s *SSHService) runShell(channel ssh.Channel) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell, "-l")
	cmd.Stdin = channel
	cmd.Stdout = channel
	cmd.Stderr = channel

	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(
			channel,
			"Failed to start shell: %v\n",
			err,
		)
		return
	}

	if err := cmd.Wait(); err != nil {
		log.Printf("SSH shell exited: %v", err)
	}
}

// runCommand executes a command through the user's shell.
func (s *SSHService) runCommand(channel ssh.Channel, command string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell, "-c", command)
	cmd.Stdin = channel
	cmd.Stdout = channel
	cmd.Stderr = channel

	err := cmd.Run()
	if err != nil {
		log.Printf("ssh command exited: %v", err)
	}
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(exitCode)}))
}

func loadOrGenerateKey(path string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		return ssh.ParsePrivateKey(key)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	pemKey, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		return nil, err
	}

	keyBytes := pem.EncodeToMemory(pemKey)

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			key, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, fmt.Errorf("read concurrently created %s: %w", path, readErr)
			}
			return ssh.ParsePrivateKey(key)
		}
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := file.Write(keyBytes); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close %s: %w", path, err)
	}

	return ssh.ParsePrivateKey(keyBytes)
}
