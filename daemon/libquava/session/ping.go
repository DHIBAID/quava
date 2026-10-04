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
	transactionID, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	response, err := s.sendRequest(ctx, protocol.NewMessage(protocol.MessageTypePing, transactionID, nil))
	if err != nil {
		return err
	}
	return protocol.ValidatePong(response, transactionID)
}
