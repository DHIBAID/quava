package session

import (
	"context"
	"crypto/hmac"
	"errors"
	"io"
	"log"

	"libquava/protocol"
)

// SSHOpenRequest carries what the peer declared when asking to open an SSH
// tunnel: the ephemeral public key blob it will authenticate with. There is
// no destination/host/port — the tunnel always terminates in the handler's
// own local SSH server.
type SSHOpenRequest struct {
	ClientPublicKey []byte
}

// SetSSHOpenHandler installs the host-side callback that grants or denies an
// SSH tunnel request and, on success, returns the local end of the tunnel
// plus the host's SSH key fingerprint to report back to the peer. Install it
// before the session dispatcher starts.
func (s *Session) SetSSHOpenHandler(handler func(context.Context, SSHOpenRequest) (io.ReadWriteCloser, [32]byte, error)) {
	s.sshOpenHandler = handler
}

func (s *Session) runSSHWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case job := <-s.sshQueue:
			switch job.msg.Type {
			case protocol.MessageTypeSSHOpen:
				if err := s.handleSSHOpen(job.ctx, job.msg); err != nil {
					log.Printf("session: handle ssh open failed: %v", err)
				}

			case protocol.MessageTypeSSHData:
				if err := s.handleSSHData(job.ctx, job.msg); err != nil {
					log.Printf("session: handle ssh data failed: %v", err)
				}

			case protocol.MessageTypeSSHClose:
				s.handleSSHClose(job.ctx, job.msg)
			}
		}
	}
}

func (s *Session) handleSSHOpen(ctx context.Context, msg protocol.Message) error {
	clientKey, err := protocol.PayloadBytes(msg.Payload, 0)
	if err != nil {
		return s.denySSHOpen(ctx, msg.TransactionID, "invalid request")
	}

	s.sshMu.Lock()
	alreadyOpen := s.sshTunnel != nil
	s.sshMu.Unlock()
	if alreadyOpen {
		return s.denySSHOpen(ctx, msg.TransactionID, "a tunnel is already open")
	}

	if s.sshOpenHandler == nil {
		return s.denySSHOpen(ctx, msg.TransactionID, "ssh is unavailable")
	}

	tunnel, fingerprint, err := s.sshOpenHandler(ctx, SSHOpenRequest{ClientPublicKey: clientKey})
	if err != nil {
		return s.denySSHOpen(ctx, msg.TransactionID, err.Error())
	}

	s.sshMu.Lock()
	s.sshTunnel = tunnel
	s.sshTunnelID = append([]byte(nil), msg.TransactionID...)
	s.sshMu.Unlock()

	ack := protocol.NewMessage(protocol.MessageTypeSSHOpenAck, msg.TransactionID, map[uint64]any{
		0: true,
		2: fingerprint[:],
	})

	s.pingMutex.Lock()
	writeErr := s.conn.WriteMessage(ctx, ack)
	s.pingMutex.Unlock()
	if writeErr != nil {
		s.closeSSHTunnel(ctx, msg.TransactionID, "")
		return writeErr
	}

	go s.runSSHOutbound(ctx, tunnel, append([]byte(nil), msg.TransactionID...))
	return nil
}

func (s *Session) denySSHOpen(ctx context.Context, transactionID []byte, reason string) error {
	ack := protocol.NewMessage(protocol.MessageTypeSSHOpenAck, transactionID, map[uint64]any{
		0: false,
		1: reason,
	})

	s.pingMutex.Lock()
	err := s.conn.WriteMessage(ctx, ack)
	s.pingMutex.Unlock()
	return err
}

func (s *Session) handleSSHData(ctx context.Context, msg protocol.Message) error {
	chunk, err := protocol.PayloadBytes(msg.Payload, 0)
	if err != nil {
		return err
	}

	s.sshMu.Lock()
	tunnel := s.sshTunnel
	matches := tunnel != nil && hmac.Equal(msg.TransactionID, s.sshTunnelID)
	s.sshMu.Unlock()
	if !matches {
		// Stray frame from an already torn-down or never-opened tunnel.
		return nil
	}

	if _, err := tunnel.Write(chunk); err != nil {
		s.closeSSHTunnel(ctx, msg.TransactionID, "")
		return err
	}

	return nil
}

// handleSSHClose is the peer-initiated teardown path: the peer told us it's
// closing the tunnel, so we just close our side locally. It deliberately
// doesn't call closeSSHTunnel or write anything back - the peer already
// knows, and echoing a message it already sent would be pure noise.
func (s *Session) handleSSHClose(_ context.Context, msg protocol.Message) {
	s.sshMu.Lock()
	tunnel := s.sshTunnel
	matches := tunnel != nil && hmac.Equal(msg.TransactionID, s.sshTunnelID)
	if matches {
		s.sshTunnel = nil
		s.sshTunnelID = nil
	}

	s.sshMu.Unlock()
	if matches {
		_ = tunnel.Close()
	}
}

// closeSSHTunnel is the daemon-initiated teardown path: it fires when the
// tunnel's own read loop hits EOF/an error (the shell exited, or a write
// into the tunnel failed), and tells the peer the tunnel is gone. If the
// tunnel was already cleared - by this racing with another call, or by a
// peer-initiated handleSSHClose that got there first - matches is false and
// nothing is sent, so there's no separate flag to track: clearing
// s.sshTunnel under sshMu is what makes "who tore it down first" unambiguous.
func (s *Session) closeSSHTunnel(ctx context.Context, tunnelID []byte, reason string) {
	s.sshMu.Lock()
	tunnel := s.sshTunnel
	matches := tunnel != nil && hmac.Equal(tunnelID, s.sshTunnelID)
	if matches {
		s.sshTunnel = nil
		s.sshTunnelID = nil
	}

	s.sshMu.Unlock()
	if !matches {
		return
	}
	_ = tunnel.Close()

	closeMsg := protocol.NewMessage(protocol.MessageTypeSSHClose, tunnelID, map[uint64]any{0: reason})
	s.pingMutex.Lock()
	_ = s.conn.WriteMessage(ctx, closeMsg)
	s.pingMutex.Unlock()
}

func (s *Session) runSSHOutbound(ctx context.Context, tunnel io.ReadWriteCloser, tunnelID []byte) {
	buf := make([]byte, protocol.SSHDataChunkSize)
	for {
		n, err := tunnel.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			data := protocol.NewMessage(protocol.MessageTypeSSHData, tunnelID, map[uint64]any{0: chunk})
			s.pingMutex.Lock()
			writeErr := s.conn.WriteMessage(ctx, data)
			s.pingMutex.Unlock()
			if writeErr != nil {
				s.closeSSHTunnel(ctx, tunnelID, "")
				return
			}
		}
		if err != nil {
			reason := ""
			if !errors.Is(err, io.EOF) {
				reason = err.Error()
			}

			s.closeSSHTunnel(ctx, tunnelID, reason)
			return
		}
	}
}
