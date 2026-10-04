package session

import (
	"context"
	"net"
	"testing"

	"libquava/protocol"
)

func TestHandleClearNotificationClosesLinuxNotificationAndAcknowledges(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()

	closedID := uint32(0)
	s := &Session{
		conn: protocol.NewConn(local),
		closeNotification: func(id uint32) error {
			closedID = id
			return nil
		},
	}
	txID := []byte("0123456789abcdef")
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.handleClearNotification(context.Background(), protocol.NewMessage(
			protocol.MessageTypeClearNotification,
			txID,
			map[uint64]any{1: uint64(77)},
		))
	}()

	ack, err := protocol.NewConn(remote).ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("read acknowledgement: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("handleClearNotification() error = %v", err)
	}
	if closedID != 77 {
		t.Fatalf("closed notification ID = %d, want 77", closedID)
	}
	if ack.Type != protocol.MessageTypeClearNotificationAck {
		t.Fatalf("ack type = %x, want %x", ack.Type, protocol.MessageTypeClearNotificationAck)
	}
	if string(ack.TransactionID) != string(txID) {
		t.Fatalf("ack transaction ID = %x, want %x", ack.TransactionID, txID)
	}
}
