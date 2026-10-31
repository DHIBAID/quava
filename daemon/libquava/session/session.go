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
	multimediaOnce      sync.Once
	multimediaWG        sync.WaitGroup
	readDone            chan error
	closeNotification   func(uint32) error
	androidNotification func(context.Context, models.AndroidNotificationContent) error
	remoteInput         func(context.Context, models.RemoteInput) error
	clipboardHandler    func(context.Context, models.ClipboardContent) error
	inputQueue          chan inputJob
	eventQueue          chan inputJob
	inputTokens         float64
	inputLast           time.Time
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

// RunWithCallbacks allows callers to install message handlers before the
// dispatcher starts, then publish the session only after the dispatcher is live.
func RunWithCallbacks(ctx context.Context, identity models.IdentityFile, peer models.PairResult, dial func(context.Context) (*protocol.Conn, error), onPrepare func(*Session), onConnect func(*Session), onDisconnect func(error)) error {
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
		// Each transport connection gets its own context. A dropped connection
		// must stop its reader and input worker even while Run retries discovery.
		sessionCtx, sessionCancel := context.WithCancel(ctx)
		// Install handlers before the reader can dispatch unsolicited messages.
		if onPrepare != nil {
			onPrepare(s)
		}
		s.startDispatcher(sessionCtx)
		// Publish only after the request dispatcher is ready, so IPC requests
		// cannot race the reader startup.
		if onConnect != nil {
			onConnect(s)
		}
		s.startMultimediaSync(sessionCtx)
		err = s.RunHeartbeat(sessionCtx)
		sessionCancel()
		s.waitMultimediaSync()
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

type inputJob struct {
	ctx context.Context
	msg protocol.Message
}

func (s *Session) startDispatcher(ctx context.Context) {
	s.readerOnce.Do(func() {
		s.readDone = make(chan error, 1)
		s.inputQueue = make(chan inputJob, 128)
		s.eventQueue = make(chan inputJob, 64)
		go s.runInputWorker(ctx)
		go s.runEventWorker(ctx)
		go func() {
			for {
				msg, err := s.conn.ReadMessage(ctx)
				if err != nil {
					s.readDone <- err
					return
				}
				s.dispatchIncoming(ctx, msg)
			}
		}()
	})
}

func (s *Session) dispatchIncoming(ctx context.Context, msg protocol.Message) {
	if s.deliverResponse(msg) {
		return
	}
	job := inputJob{ctx: ctx, msg: msg}
	if msg.Type == protocol.MessageTypeInputEvent {
		if !s.allowInputEvent(time.Now()) {
			_ = s.ackInput(ctx, msg, false)
			return
		}
		select {
		case s.inputQueue <- job:
		default:
			log.Printf("session: input queue full; dropping input event")
			_ = s.ackInput(ctx, msg, false)
		}
		return
	}
	select {
	case s.eventQueue <- job:
	default:
		log.Printf("session: unsolicited event queue full; dropping message type %d", msg.Type)
		var ackType uint64
		switch msg.Type {
		case protocol.MessageTypeSyncClipboard:
			ackType = protocol.MessageTypeSyncClipboardAck
		case protocol.MessageTypeSyncNotification:
			ackType = protocol.MessageTypeSyncNotificationAck
		case protocol.MessageTypeClearNotification:
			ackType = protocol.MessageTypeClearNotificationAck
		}
		if ackType != 0 {
			ack := protocol.NewMessage(ackType, msg.TransactionID, map[uint64]any{0: false})
			s.pingMutex.Lock()
			_ = s.conn.WriteMessage(ctx, ack)
			s.pingMutex.Unlock()
		}
	}
}

func (s *Session) runEventWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.eventQueue:
			s.handleUnsolicited(job.ctx, job.msg)
		}
	}
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
	case protocol.MessageTypeSyncClipboard:
		if err := s.handleIncomingClipboard(ctx, msg); err != nil {
			log.Printf("session: handle incoming clipboard failed: %v", err)
		}
	case protocol.MessageTypeMultimedia:
		if err := s.handleMultimediaCommand(ctx, msg); err != nil {
			log.Printf("session: handle multimedia command failed: %v", err)
		}
	default:
		// Ignore other unsolicited messages
	}
}

func (s *Session) allowInputEvent(now time.Time) bool {
	const rate = 500.0 // sustained events per second
	const burst = 100.0
	if s.inputLast.IsZero() {
		s.inputLast = now
		s.inputTokens = burst
	}
	elapsed := now.Sub(s.inputLast).Seconds()
	if elapsed > 0 {
		s.inputTokens += elapsed * rate
	}
	if s.inputTokens > burst {
		s.inputTokens = burst
	}
	s.inputLast = now
	if s.inputTokens < 1 {
		return false
	}
	s.inputTokens--
	return true
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
