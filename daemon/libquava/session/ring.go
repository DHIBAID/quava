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
	transactionID, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	response, err := s.sendRequest(ctx, protocol.NewMessage(protocol.MessageTypeRing, transactionID, nil))
	if err != nil {
		return err
	}

	if response.Type != protocol.MessageTypeRingAck || !bytes.Equal(response.TransactionID, transactionID) {
		return errors.New("session: invalid RING_ACK response")
	}
	return nil
}
