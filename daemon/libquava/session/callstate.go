package session

import (
	"context"
	"errors"
	"libquava/protocol"
)

// handleCallState processes an incoming CallState notification from the
// phone and acknowledges whether the host-side mute callback succeeded.
func (s *Session) handleCallState(ctx context.Context, msg protocol.Message) error {
	inCall, ok := msg.Payload[0].(bool)
	if !ok {
		return errors.New("session: invalid call state payload")
	}

	var callbackErr error
	if s.callMute != nil {
		callbackErr = s.callMute(ctx, inCall)
	}

	ack := protocol.NewMessage(protocol.MessageTypeCallStateAck, msg.TransactionID, map[uint64]any{0: callbackErr == nil})
	s.pingMutex.Lock()
	ackErr := s.conn.WriteMessage(ctx, ack)
	s.pingMutex.Unlock()
	return errors.Join(callbackErr, ackErr)
}
