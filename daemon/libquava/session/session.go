package session

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"libquava/crypto"
	"libquava/models"
	"libquava/protocol"
)

const (
	sessionTimeout    = 15 * time.Second
	heartbeatInterval = 20 * time.Second
)

type Session struct {
	conn        *protocol.Conn
	peer        models.PairResult
	localID     []byte
	localPublic ed25519.PublicKey
	txID        []byte
}

func Connect(ctx context.Context, conn *protocol.Conn, identity models.IdentityFile, peer models.PairResult) (*Session, error) {
	if conn == nil {
		return nil, errors.New("session: nil connection")
	}
	if len(identity.PrivateKey) != ed25519.PrivateKeySize || len(identity.PublicKey) != ed25519.PublicKeySize {
		return nil, errors.New("session: invalid local identity")
	}
	if len(peer.PeerPublicKey) != ed25519.PublicKeySize || len(peer.PeerCredential) == 0 {
		return nil, errors.New("session: incomplete trust record")
	}
	if peer.PeerDeviceID != fmt.Sprintf("%x", crypto.DeviceIDBytes(ed25519.PublicKey(peer.PeerPublicKey))) {
		return nil, errors.New("session: trusted peer device ID does not match public key")
	}

	txID, err := crypto.GenerateTransactionID()
	if err != nil {
		return nil, err
	}
	localNonce, err := crypto.GenerateNonce()
	if err != nil {
		return nil, err
	}
	localID := crypto.DeviceIDBytes(identity.PublicKey)
	hello := protocol.NewMessage(protocol.MessageTypeSessionHello, txID, map[uint64]any{0: localID, 1: []byte(identity.PublicKey), 2: localNonce, 3: uint64(protocol.ProtocolVersion)})
	if err := conn.WriteMessage(ctx, hello); err != nil {
		return nil, err
	}

	challenge, err := conn.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	if challenge.Version != protocol.ProtocolVersion || challenge.Type != protocol.MessageTypeSessionChallenge || !hmac.Equal(challenge.TransactionID, txID) {
		return nil, errors.New("session: invalid challenge")
	}
	remoteID, err := protocol.PayloadBytes(challenge.Payload, 0)
	if err != nil {
		return nil, err
	}
	remotePub, err := protocol.PayloadBytes(challenge.Payload, 1)
	if err != nil {
		return nil, err
	}
	remoteNonce, err := protocol.PayloadBytes(challenge.Payload, 2)
	if err != nil {
		return nil, err
	}
	challengeMAC, err := protocol.PayloadBytes(challenge.Payload, 3)
	if err != nil {
		return nil, err
	}
	if len(remotePub) != ed25519.PublicKeySize || !hmac.Equal(remoteID, crypto.DeviceIDBytes(ed25519.PublicKey(remotePub))) || !hmac.Equal(remoteID, crypto.DeviceIDBytes(ed25519.PublicKey(peer.PeerPublicKey))) {
		return nil, errors.New("session: peer identity mismatch")
	}
	transcript := transcript(txID, localID, remoteID, identity.PublicKey, remotePub, localNonce, remoteNonce)
	if !hmac.Equal(challengeMAC, mac(peer.PeerCredential, "challenge", transcript)) {
		return nil, errors.New("session: challenge authentication failed")
	}

	if err := conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypeSessionAuthenticate, txID, map[uint64]any{0: mac(peer.PeerCredential, "authenticate", transcript)})); err != nil {
		return nil, err
	}
	ready, err := conn.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	if ready.Version != protocol.ProtocolVersion || ready.Type != protocol.MessageTypeSessionReady || !hmac.Equal(ready.TransactionID, txID) {
		return nil, errors.New("session: invalid ready")
	}
	readyMAC, err := protocol.PayloadBytes(ready.Payload, 0)
	if err != nil {
		return nil, err
	}

	if !hmac.Equal(
		readyMAC,
		mac(peer.PeerCredential, "ready", transcript),
	) {
		return nil, errors.New("session: ready authentication failed")
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown PC"
	}

	log.Printf(
		"SESSION_READY: sending hostname=%q tx=%x",
		hostname,
		txID,
	)

	payload := map[uint64]any{
		protocol.SessionReadyMAC:      mac(peer.PeerCredential, "ready", transcript),
		protocol.SessionReadyHostname: hostname,
	}

	if err := conn.WriteMessage(
		ctx,
		protocol.NewMessage(
			protocol.MessageTypeSessionReady,
			txID,
			payload,
		),
	); err != nil {
		return nil, err
	}

	log.Printf("SESSION_READY: write succeeded")

	return &Session{
		conn:        conn,
		peer:        peer,
		localID:     localID,
		localPublic: identity.PublicKey,
		txID:        txID,
	}, nil
}

func NewServerSession(ctx context.Context, conn *protocol.Conn, identity models.IdentityFile, peer models.PairResult, first protocol.Message) (*Session, error) {
	if conn == nil {
		return nil, errors.New("session: nil connection")
	}
	if first.Version != protocol.ProtocolVersion || first.Type != protocol.MessageTypeSessionHello {
		return nil, errors.New("session: expected session hello")
	}
	if len(identity.PrivateKey) != ed25519.PrivateKeySize || len(identity.PublicKey) != ed25519.PublicKeySize {
		return nil, errors.New("session: invalid local identity")
	}
	if len(peer.PeerPublicKey) != ed25519.PublicKeySize || len(peer.PeerCredential) == 0 {
		return nil, errors.New("session: incomplete trust record")
	}

	remoteID, err := protocol.PayloadBytes(first.Payload, 0)
	if err != nil {
		return nil, err
	}
	remotePub, err := protocol.PayloadBytes(first.Payload, 1)
	if err != nil {
		return nil, err
	}
	remoteNonce, err := protocol.PayloadBytes(first.Payload, 2)
	if err != nil {
		return nil, err
	}
	if len(remotePub) != ed25519.PublicKeySize || !hmac.Equal(remoteID, crypto.DeviceIDBytes(ed25519.PublicKey(remotePub))) || !hmac.Equal(remoteID, crypto.DeviceIDBytes(ed25519.PublicKey(peer.PeerPublicKey))) {
		return nil, errors.New("session: peer identity mismatch")
	}
	localNonce, err := crypto.GenerateNonce()
	if err != nil {
		return nil, err
	}
	localID := crypto.DeviceIDBytes(identity.PublicKey)
	transcript := transcript(first.TransactionID, remoteID, localID, remotePub, identity.PublicKey, remoteNonce, localNonce)
	if err := conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypeSessionChallenge, first.TransactionID, map[uint64]any{0: localID, 1: []byte(identity.PublicKey), 2: localNonce, 3: mac(peer.PeerCredential, "challenge", transcript)})); err != nil {
		return nil, err
	}

	auth, err := conn.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	if auth.Version != protocol.ProtocolVersion || auth.Type != protocol.MessageTypeSessionAuthenticate || !hmac.Equal(auth.TransactionID, first.TransactionID) {
		return nil, errors.New("session: invalid authentication")
	}
	authMAC, err := protocol.PayloadBytes(auth.Payload, 0)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(authMAC, mac(peer.PeerCredential, "authenticate", transcript)) {
		return nil, errors.New("session: peer authentication failed")
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown PC"
	}

	log.Printf(
		"SESSION_READY: sending hostname=%q tx=%x",
		hostname,
		first.TransactionID,
	)

	payload := map[uint64]any{
		protocol.SessionReadyMAC:      mac(peer.PeerCredential, "ready", transcript),
		protocol.SessionReadyHostname: hostname,
	}

	if err := conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypeSessionReady, first.TransactionID, payload)); err != nil {
		return nil, err
	}
	return &Session{conn: conn, peer: peer, localID: localID, localPublic: identity.PublicKey, txID: append([]byte(nil), first.TransactionID...)}, nil
}

// Run maintains a session to a trusted peer. It reconnects after transport loss
// with bounded exponential backoff. Pairing trust is never modified by reconnects.
func Run(ctx context.Context, identity models.IdentityFile, peer models.PairResult, dial func(context.Context) (*protocol.Conn, error), onConnect func(*Session), onDisconnect func(error)) error {
	if dial == nil {
		return errors.New("session: nil dial function")
	}
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := dial(ctx)
		if err != nil {
			if onDisconnect != nil {
				onDisconnect(err)
			}
			t := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
			if backoff < 30*time.Second {
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
			}
			continue
		}
		s, err := Connect(ctx, conn, identity, peer)
		if err != nil {
			_ = conn.Close()
			if onDisconnect != nil {
				onDisconnect(err)
			}
			t := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
			if backoff < 30*time.Second {
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
			}
			continue
		}
		backoff = time.Second
		if onConnect != nil {
			onConnect(s)
		}
		err = s.RunHeartbeat(ctx)
		_ = s.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if onDisconnect != nil {
			onDisconnect(err)
		}
	}
}

func (s *Session) Ping(ctx context.Context) error {
	if s == nil || s.conn == nil {
		return errors.New("session: closed")
	}
	id, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}
	if err := s.conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePing, id, nil)); err != nil {
		return err
	}
	msg, err := s.conn.ReadMessage(ctx)
	if err != nil {
		return err
	}
	return protocol.ValidatePong(msg, id)
}

func (s *Session) RunHeartbeat(ctx context.Context) error {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, sessionTimeout)
			err := s.Ping(pingCtx)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

func (s *Session) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *Session) Peer() models.PairResult { return s.peer }

func transcript(txID, localID, remoteID, localPub, remotePub, localNonce, remoteNonce []byte) []byte {
	out := make([]byte, 0, len(protocol.SessionProtocolName)+len(txID)+len(localID)+len(remoteID)+len(localPub)+len(remotePub)+len(localNonce)+len(remoteNonce))
	out = append(out, []byte(protocol.SessionProtocolName)...)
	out = append(out, txID...)
	out = append(out, localID...)
	out = append(out, remoteID...)
	out = append(out, localPub...)
	out = append(out, remotePub...)
	out = append(out, localNonce...)
	out = append(out, remoteNonce...)
	return out
}

func mac(key []byte, label string, transcript []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(label))
	h.Write(transcript)
	return h.Sum(nil)
}
