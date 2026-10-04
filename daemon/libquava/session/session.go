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
	"sync"
	"time"

	"libquava/crypto"
	"libquava/models"
	"libquava/notifications"
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

	pingMutex           sync.Mutex
	pendingMutex        sync.Mutex
	pending             map[string]chan protocol.Message
	readerOnce          sync.Once
	readDone            chan error
	closeNotification   func(uint32) error
	androidNotification func(context.Context, models.AndroidNotificationContent) error
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
		conn:              conn,
		peer:              peer,
		localID:           localID,
		localPublic:       identity.PublicKey,
		txID:              txID,
		pending:           make(map[string]chan protocol.Message),
		closeNotification: notifications.CloseNotification,
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
	return &Session{
		conn:              conn,
		peer:              peer,
		localID:           localID,
		localPublic:       identity.PublicKey,
		txID:              append([]byte(nil), first.TransactionID...),
		pending:           make(map[string]chan protocol.Message),
		closeNotification: notifications.CloseNotification,
	}, nil
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
		s.startDispatcher(ctx)
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

func (s *Session) RunHeartbeat(ctx context.Context) error {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	s.startDispatcher(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-s.readDone:
			return err
		case <-ticker.C:
			heartbeatCtx, cancel := context.WithTimeout(ctx, sessionTimeout)
			err := s.sendHeartbeat(heartbeatCtx)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

func (s *Session) sendHeartbeat(ctx context.Context) error {
	if s == nil || s.conn == nil {
		return errors.New("session: closed")
	}
	transactionID, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	response, err := s.sendRequest(ctx, protocol.NewMessage(protocol.MessageTypePing, transactionID, map[uint64]any{protocol.PingPayloadSilent: true}))
	if err != nil {
		return err
	}
	return protocol.ValidatePong(response, transactionID)
}

func (s *Session) startDispatcher(ctx context.Context) {
	s.readerOnce.Do(func() {
		s.readDone = make(chan error, 1)
		go func() {
			for {
				msg, err := s.conn.ReadMessage(ctx)
				if err != nil {
					s.readDone <- err
					return
				}
				if !s.deliverResponse(msg) {
					s.handleUnsolicited(ctx, msg)
				}
			}
		}()
	})
}

// sendRequest registers its response channel before writing, so the sole
// connection reader cannot receive and discard a fast acknowledgement.
func (s *Session) sendRequest(ctx context.Context, msg protocol.Message) (protocol.Message, error) {
	transactionID := msg.TransactionID
	txIDStr := fmt.Sprintf("%x", transactionID)
	responseChan := make(chan protocol.Message, 1)

	s.pendingMutex.Lock()
	s.pending[txIDStr] = responseChan
	s.pendingMutex.Unlock()

	cleanup := func() {
		s.pendingMutex.Lock()
		delete(s.pending, txIDStr)
		s.pendingMutex.Unlock()
	}

	s.pingMutex.Lock()
	err := s.conn.WriteMessage(ctx, msg)
	s.pingMutex.Unlock()
	if err != nil {
		cleanup()
		return protocol.Message{}, err
	}

	select {
	case <-ctx.Done():
		cleanup()
		return protocol.Message{}, ctx.Err()
	case response := <-responseChan:
		cleanup()
		return response, nil
	}
}

func (s *Session) deliverResponse(msg protocol.Message) bool {
	txIDStr := fmt.Sprintf("%x", msg.TransactionID)

	s.pendingMutex.Lock()
	responseChan, found := s.pending[txIDStr]
	s.pendingMutex.Unlock()

	if found && responseChan != nil {
		select {
		case responseChan <- msg:
			return true
		default:
			// Channel not ready or closed, log and continue
			return false
		}
	}
	return false
}

func (s *Session) handleUnsolicited(ctx context.Context, msg protocol.Message) {
	switch msg.Type {
	case protocol.MessageTypeSyncNotification:
		if err := s.handleAndroidNotification(ctx, msg); err != nil {
			log.Printf("session: handle Android notification failed: %v", err)
		}
	case protocol.MessageTypeClearNotification:
		if err := s.handleClearNotification(ctx, msg); err != nil {
			log.Printf("session: handle clear notification failed: %v", err)
		}
	default:
		// Ignore other unsolicited messages
	}
}

// SetAndroidNotificationHandler sets the host-side renderer for notifications
// received from Android. It must be installed before the session is exposed.
func (s *Session) SetAndroidNotificationHandler(handler func(context.Context, models.AndroidNotificationContent) error) {
	s.androidNotification = handler
}

func (s *Session) handleClearNotification(ctx context.Context, msg protocol.Message) error {
	if s == nil || s.conn == nil {
		return errors.New("session: closed")
	}

	// Extract notification ID from payload (field 1)
	id, err := protocol.PayloadUint(msg.Payload, 1)
	if err != nil {
		return fmt.Errorf("session: invalid clear notification payload: %w", err)
	}

	// Close the notification on Linux via D-Bus
	closeNotification := s.closeNotification
	if closeNotification == nil {
		closeNotification = notifications.CloseNotification
	}
	if err := closeNotification(uint32(id)); err != nil {
		log.Printf("session: close notification %d failed: %v", id, err)
		// Don't fail here - we still need to send the ack
	}

	// Send acknowledgment back to Android
	ackMsg := protocol.NewMessage(
		protocol.MessageTypeClearNotificationAck,
		msg.TransactionID,
		map[uint64]interface{}{},
	)

	s.pingMutex.Lock()
	err = s.conn.WriteMessage(ctx, ackMsg)
	s.pingMutex.Unlock()
	if err != nil {
		return fmt.Errorf("session: send clear notification ack failed: %w", err)
	}

	return nil
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
