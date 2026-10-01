package session

import (
	"context"
	"errors"
	"libquava/crypto"
	"libquava/protocol"
)

// SendPing sends a ping over the authenticated session and waits for its
// matching acknowledgement. The caller owns the timeout through ctx.
func (s *Session) SendPing(ctx context.Context) error {
	if s == nil || s.conn == nil {
		return errors.New("session: closed")
	}
	s.pingMutex.Lock()
	defer s.pingMutex.Unlock()

	transactionID, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	if err := s.conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePing, transactionID, nil)); err != nil {
		return err
	}

	response, err := s.conn.ReadMessage(ctx)
	if err != nil {
		return err
	}
	return protocol.ValidatePong(response, transactionID)
}
