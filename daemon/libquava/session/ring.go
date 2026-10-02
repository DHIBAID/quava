package session

import (
	"bytes"
	"context"
	"errors"
	"libquava/crypto"
	"libquava/protocol"
)

func (s *Session) SendRing(ctx context.Context) error {
	if s == nil || s.conn == nil {
		return errors.New("session: closed")
	}
	s.pingMutex.Lock()
	defer s.pingMutex.Unlock()

	transactionID, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	if err := s.conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypeRing, transactionID, nil)); err != nil {
		return err
	}

	// Check for RING_ACK response
	response, err := s.conn.ReadMessage(ctx)
	if err != nil {
		return err
	}

	if response.Type != protocol.MessageTypeRingAck || !bytes.Equal(response.TransactionID, transactionID) {
		return errors.New("session: invalid RING_ACK response")
	}
	return nil
}
