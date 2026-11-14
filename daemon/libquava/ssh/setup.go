package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
)

const sshHandshakeTimeout = 10 * time.Second

// Service mints hardened, single-use SSH server configurations. It never
// listens on a socket: every tunnel is opened explicitly by the session
// layer in response to an authenticated peer request. The only state it
// carries across tunnels is the daemon's own persistent host identity.
type Service struct {
	hostKey ssh.Signer
}

// NewService loads or generates the daemon's persistent SSH host key.
func NewService(keyPath string) (*Service, error) {
	signer, err := loadOrGenerateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("ssh: load host key: %w", err)
	}
	return &Service{hostKey: signer}, nil
}

// HostKeyFingerprint returns the SHA-256 digest of the host's public key
// blob, so a peer can pin it instead of trusting it on first use.
func (s *Service) HostKeyFingerprint() [32]byte {
	return sha256.Sum256(s.hostKey.PublicKey().Marshal())
}

// handleChannel accepts the one "session" channel a tunnel is allowed to
// carry and rejects anything else.
func handleChannel(newChannel ssh.NewChannel) {
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

	handleSession(channel, requests)
}

// handleSession handles incoming SSH channel requests.
func handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
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
				runPTYShell(channel, requests, *requestedPTY)
			} else {
				runShell(channel)
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
			runCommand(channel, payload.Command)
			return

		default:
			_ = req.Reply(false, nil)
		}
	}
}

// runShell starts an interactive login shell.
func runShell(channel ssh.Channel) {
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
func runCommand(channel ssh.Channel, command string) {
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
		if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
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
