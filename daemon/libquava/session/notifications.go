package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"libquava/crypto"
	"libquava/models"
	"libquava/protocol"
	"log"
	"unicode/utf8"
)

func (s *Session) handleAndroidNotification(ctx context.Context, msg protocol.Message) error {
	key, err := protocol.PayloadString(msg.Payload, 1)
	if err != nil || key == "" {
		return errors.New("session: invalid Android notification key")
	}
	appName, err := protocol.PayloadString(msg.Payload, 2)
	if err != nil {
		return err
	}
	title, err := protocol.PayloadString(msg.Payload, 3)
	if err != nil {
		return err
	}
	body, _ := protocol.PayloadString(msg.Payload, 4)
	var icon []byte
	if value, ok := msg.Payload[5]; ok {
		icon, _ = protocol.PayloadBytes(map[uint64]any{0: value}, 0)
	}
	content := models.AndroidNotificationContent{Key: key, AppName: appName, Title: title, Body: body, IconData: icon}
	if rawActions, ok := msg.Payload[6].([]any); ok {
		for _, raw := range rawActions {
			name, reply := androidNotificationAction(raw)
			if name != "" {
				content.Actions = append(content.Actions, models.AndroidNotificationAction{Title: name, HasRemoteInput: reply})
			}
		}
	}
	log.Printf("session: received Android notification key=%q app=%q actions=%d icon_bytes=%d", key, appName, len(content.Actions), len(icon))
	var callbackErr error
	if s.androidNotification != nil {
		callbackErr = s.androidNotification(ctx, content)
	}
	ack := protocol.NewMessage(protocol.MessageTypeSyncNotificationAck, msg.TransactionID, map[uint64]any{0: callbackErr == nil})
	s.pingMutex.Lock()
	ackErr := s.conn.WriteMessage(ctx, ack)
	s.pingMutex.Unlock()
	return errors.Join(callbackErr, ackErr)
}

// Nested CBOR maps are decoded as map[any]any on some peers and map[uint64]any
// on others. Accept both rather than silently dropping every notification action.
func androidNotificationAction(raw any) (string, bool) {
	switch fields := raw.(type) {
	case map[any]any:
		name, _ := fields[uint64(1)].(string)
		reply, _ := fields[uint64(2)].(bool)
		return name, reply
	case map[uint64]any:
		name, _ := fields[1].(string)
		reply, _ := fields[2].(bool)
		return name, reply
	default:
		return "", false
	}
}

// SendAndroidNotificationAction requests that Android invoke an original
// notification action. reply is supplied for RemoteInput actions.
func (s *Session) SendAndroidNotificationAction(ctx context.Context, key string, action int, reply string) error {
	tx, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}
	response, err := s.sendRequest(ctx, protocol.NewMessage(protocol.MessageTypeNotificationAction, tx, map[uint64]any{1: key, 2: action, 3: reply}))
	if err != nil {
		return err
	}
	if response.Type != protocol.MessageTypeNotificationActionAck || !bytes.Equal(response.TransactionID, tx) {
		return errors.New("session: invalid notification action acknowledgement")
	}
	return nil
}

func (s *Session) DismissAndroidNotification(ctx context.Context, key string) error {
	tx, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}
	response, err := s.sendRequest(ctx, protocol.NewMessage(protocol.MessageTypeDismissAndroidNotification, tx, map[uint64]any{1: key}))
	if err != nil {
		return err
	}
	if response.Type != protocol.MessageTypeDismissAndroidNotificationAck || !bytes.Equal(response.TransactionID, tx) {
		return errors.New("session: invalid Android dismissal acknowledgement")
	}
	return nil
}

func (s *Session) SyncNotifications(ctx context.Context, notification models.NotificationContent) error {
	if s == nil || s.conn == nil {
		return errors.New("session: closed")
	}

	transactionID, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	// Keep optional notification fields below the protocol's 64 KiB frame
	// ceiling. Oversized icons are omitted; text is bounded before encoding.
	if len(notification.IconData) > 8*1024 {
		notification.IconData = nil
	}
	if len(notification.AppName) > 1024 {
		notification.AppName = truncateUTF8(notification.AppName, 1024)
	}
	if len(notification.AppIcon) > 1024 {
		notification.AppIcon = truncateUTF8(notification.AppIcon, 1024)
	}
	if len(notification.Summary) > 4*1024 {
		notification.Summary = truncateUTF8(notification.Summary, 4*1024)
	}
	if len(notification.Body) > 16*1024 {
		notification.Body = truncateUTF8(notification.Body, 16*1024)
	}
	if len(notification.Actions) > 16 {
		notification.Actions = notification.Actions[:16]
	}
	if len(notification.Hints) > 32 {
		notification.Hints = nil
	}

	payload := map[uint64]interface{}{
		1: notification.AppName,
		2: notification.ReplacesID,
		3: notification.AppIcon,
		4: notification.Summary,
		5: notification.Body,
		6: notification.Actions,
		7: notification.Hints,
		8: notification.ExpireTimeout,
		9: notification.IconData,
	}

	msg := protocol.NewMessage(protocol.MessageTypeSyncNotification, transactionID, payload)
	encoded, err := protocol.EncodeMessage(msg)
	if err != nil {
		return fmt.Errorf("session: encode notification: %w", err)
	}
	if len(encoded) > int(protocol.DefaultMaxFrameSize) {
		// Hints and icons are optional and can dominate the serialized size.
		msg.Payload[7] = map[string]any{}
		msg.Payload[9] = []byte(nil)
		msg.Payload[6] = []string{}
		msg.Payload[5] = truncateUTF8(notification.Body, 8*1024)
		msg.Payload[4] = truncateUTF8(notification.Summary, 2*1024)
		encoded, err = protocol.EncodeMessage(msg)
		if err != nil {
			return fmt.Errorf("session: encode bounded notification: %w", err)
		}
		if len(encoded) > int(protocol.DefaultMaxFrameSize) {
			return errors.New("session: notification payload exceeds protocol frame limit")
		}
	}

	response, err := s.sendRequest(ctx, msg)
	if err != nil {
		return err
	}

	if response.Type != protocol.MessageTypeSyncNotificationAck ||
		!bytes.Equal(response.TransactionID, transactionID) {
		return errors.New("session: invalid SYNC_NOTIFICATION_ACK response")
	}
	if accepted, ok := response.Payload[0].(bool); ok && !accepted {
		return errors.New("session: peer rejected notification synchronization")
	}

	return nil
}

func (s *Session) ClearNotification(ctx context.Context, id uint32) error {
	if s == nil || s.conn == nil {
		return errors.New("session: closed")
	}

	transactionID, err := crypto.GenerateTransactionID()
	if err != nil {
		return err
	}

	payload := map[uint64]interface{}{
		1: id,
	}

	msg := protocol.NewMessage(protocol.MessageTypeClearNotification, transactionID, payload)
	response, err := s.sendRequest(ctx, msg)
	if err != nil {
		return err
	}

	if response.Type != protocol.MessageTypeClearNotificationAck ||
		!bytes.Equal(response.TransactionID, transactionID) {
		return errors.New("session: invalid CLEAR_NOTIFICATION_ACK response")
	}
	if accepted, ok := response.Payload[0].(bool); ok && !accepted {
		return errors.New("session: peer rejected notification clear")
	}

	return nil
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
