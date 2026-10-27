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
	if len(clipboard.Content) > 48*1024 {
		return errors.New("session: clipboard content exceeds 48 KiB sync limit")
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
	if accepted, ok := response.Payload[0].(bool); ok && !accepted {
		return errors.New("session: peer rejected clipboard synchronization")
	}

	return nil
}

// SetClipboardHandler installs the host-side callback for clipboard content
// received from the peer. Install it before the session dispatcher starts.
func (s *Session) SetClipboardHandler(handler func(context.Context, models.ClipboardContent) error) {
	s.clipboardHandler = handler
}

func (s *Session) handleIncomingClipboard(ctx context.Context, msg protocol.Message) error {
	contentType, err := protocol.PayloadString(msg.Payload, 1)
	if err != nil {
		return err
	}
	content, err := protocol.PayloadBytes(msg.Payload, 2)
	if err != nil {
		return err
	}
	if len(content) > 48*1024 {
		err = errors.New("clipboard payload exceeds 48 KiB sync limit")
	} else if s.clipboardHandler == nil {
		err = errors.New("clipboard receive is unavailable")
	} else {
		err = s.clipboardHandler(ctx, models.ClipboardContent{ContentType: models.ClipboardContentTypes(contentType), Content: content})
	}
	ack := protocol.NewMessage(protocol.MessageTypeSyncClipboardAck, msg.TransactionID, map[uint64]any{0: err == nil})
	s.pingMutex.Lock()
	writeErr := s.conn.WriteMessage(ctx, ack)
	s.pingMutex.Unlock()
	if writeErr != nil {
		return writeErr
	}
	return err
}
