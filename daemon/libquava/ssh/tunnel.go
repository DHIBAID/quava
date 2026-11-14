package ssh

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/ssh"
)

// OpenTunnel builds a fresh, single-use SSH server for one tunnel and
// returns the caller's end of an in-process pipe to pump bytes through. No
// socket is ever opened: the only way in is to already hold the pipe this
// call hands back, and the only key ssh.ServerConfig's PublicKeyCallback
// will accept is clientPublicKeyBlob — the exact wire-format blob
// (ssh.PublicKey.Marshal() / JSch KeyPair.getPublicKeyBlob()) the caller
// already took from an authenticated-session request. There is nothing
// durable or shared to revoke: the credential's lifetime is the tunnel's.
//
// Terminal type and size are not parameters here: the SSH client negotiates
// those itself over the tunnel via the ordinary "pty-req"/"window-change"
// channel requests, which pty.go already handles.
func (s *Service) OpenTunnel(ctx context.Context, clientPublicKeyBlob []byte) (io.ReadWriteCloser, [32]byte, error) {
	if s == nil || s.hostKey == nil {
		return nil, [32]byte{}, errors.New("ssh: service has no host key")
	}

	if len(clientPublicKeyBlob) == 0 {
		return nil, [32]byte{}, errors.New("ssh: empty client public key")
	}

	expectedKey := append([]byte(nil), clientPublicKeyBlob...)

	config := &ssh.ServerConfig{
		KeyExchanges:            []string{ssh.KeyExchangeCurve25519},
		Ciphers:                 []string{ssh.CipherChaCha20Poly1305, ssh.CipherAES256GCM},
		PublicKeyAuthAlgorithms: []string{ssh.KeyAlgoED25519},
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if !publicKeyMatches(key, expectedKey) {
				return nil, errors.New("ssh: presented key does not match this tunnel's authorized key")
			}
			return nil, nil
		},
	}

	config.AddHostKey(s.hostKey)

	serverSide, tunnelSide, err := newTunnelPipe()
	if err != nil {
		return nil, [32]byte{}, fmt.Errorf("ssh: open tunnel pipe: %w", err)
	}

	_ = serverSide.SetDeadline(time.Now().Add(sshHandshakeTimeout))

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = serverSide.Close()
			_ = tunnelSide.Close()
		case <-done:
		}
	}()

	// ssh.NewServerConn blocks for the full handshake. Running it (and the
	// one-channel session loop that follows) in its own goroutine lets
	// OpenTunnel hand back tunnelSide immediately so the caller can start
	// pumping it, rather than only once the handshake and shell finish.
	go func() {
		defer close(done)
		defer func() { _ = serverSide.Close() }()

		sshConn, chans, reqs, err := ssh.NewServerConn(serverSide, config)
		if err != nil {
			return
		}

		_ = serverSide.SetDeadline(time.Time{})
		defer func() { _ = sshConn.Close() }()

		go ssh.DiscardRequests(reqs)

		// Exactly one PTY/shell per tunnel: accept the first "session"
		// channel, run it to completion, then tear the whole tunnel down.
		// Anything else the client opens afterwards is rejected outright.
		newChannel, ok := <-chans
		if !ok {
			return
		}

		go func() {
			for extra := range chans {
				_ = extra.Reject(ssh.ResourceShortage, "only one session per tunnel")
			}
		}()

		handleChannel(newChannel)
	}()

	return tunnelSide, s.HostKeyFingerprint(), nil
}

// publicKeyMatches reports whether the presented key's wire-format blob is
// byte-for-byte the one key this tunnel was authorized for.
func publicKeyMatches(presented ssh.PublicKey, expected []byte) bool {
	return hmac.Equal(presented.Marshal(), expected)
}
