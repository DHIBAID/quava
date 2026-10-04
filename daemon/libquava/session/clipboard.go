package session

import (
	"bytes"
	"context"
	"errors"

	"libquava/crypto"
	"libquava/models"
	"libquava/protocol"
)

func (s *Session) SyncClipboard(
	ctx context.Context,
	clipboard models.ClipboardContent,
) error {
	if s == nil || s.conn == nil {
		return errors.New("session: closed")
	}

	transactionID, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	payload := map[uint64]interface{}{
		1: string(clipboard.ContentType),
		2: clipboard.Content,
	}

	msg := protocol.NewMessage(protocol.MessageTypeSyncClipboard, transactionID, payload)

	response, err := s.sendRequest(ctx, msg)
	if err != nil {
		return err
	}

	if response.Type != protocol.MessageTypeSyncClipboardAck ||
		!bytes.Equal(response.TransactionID, transactionID) {
		return errors.New("session: invalid SYNC_CLIPBOARD_ACK response")
	}

	return nil
}
