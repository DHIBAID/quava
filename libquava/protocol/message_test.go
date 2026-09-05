package protocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

func TestReadMessageComplete(t *testing.T) {
	transactionID := []byte("transaction-123456")
	reader := bytes.NewReader(encodeTestFrame(t, NewMessage(MessageTypePing, transactionID, map[uint64]any{1: "hello"})))
	message, err := NewFramer(DefaultMaxFrameSize).ReadMessage(context.Background(), reader)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if message.Type != MessageTypePing {
		t.Fatalf("unexpected message type: %d", message.Type)
	}
	if !bytes.Equal(message.TransactionID, transactionID) {
		t.Fatalf("unexpected transaction id: %q", message.TransactionID)
	}
	if got, err := PayloadString(message.Payload, 1); err != nil || got != "hello" {
		t.Fatalf("payload mismatch: %q %v", got, err)
	}
}

func TestReadMessagePartialHeader(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		frame := encodeTestFrame(t, NewMessage(MessageTypePing, []byte("partial-header"), nil))
		_, _ = server.Write(frame[:2])
		time.Sleep(20 * time.Millisecond)
		_, _ = server.Write(frame[2:])
	}()

	message, err := NewConn(client).ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if !bytes.Equal(message.TransactionID, []byte("partial-header")) {
		t.Fatalf("unexpected transaction id: %q", message.TransactionID)
	}
}

func TestReadMessagePartialPayload(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		frame := encodeTestFrame(t, NewMessage(MessageTypePing, []byte("partial-payload"), nil))
		_, _ = server.Write(frame[:4])
		_, _ = server.Write(frame[4:10])
		time.Sleep(20 * time.Millisecond)
		_, _ = server.Write(frame[10:])
	}()

	message, err := NewConn(client).ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if !bytes.Equal(message.TransactionID, []byte("partial-payload")) {
		t.Fatalf("unexpected transaction id: %q", message.TransactionID)
	}
}

func TestReadMultipleMessages(t *testing.T) {
	frame1 := encodeTestFrame(t, NewMessage(MessageTypePing, []byte("one"), nil))
	frame2 := encodeTestFrame(t, NewMessage(MessageTypePong, []byte("two"), nil))
	reader := bytes.NewReader(append(frame1, frame2...))
	framer := NewFramer(DefaultMaxFrameSize)

	message1, err := framer.ReadMessage(context.Background(), reader)
	if err != nil {
		t.Fatalf("first ReadMessage failed: %v", err)
	}
	message2, err := framer.ReadMessage(context.Background(), reader)
	if err != nil {
		t.Fatalf("second ReadMessage failed: %v", err)
	}
	if !bytes.Equal(message1.TransactionID, []byte("one")) || !bytes.Equal(message2.TransactionID, []byte("two")) {
		t.Fatalf("unexpected messages: %#v %#v", message1, message2)
	}
}

func TestZeroLengthFrame(t *testing.T) {
	reader := bytes.NewReader([]byte{0, 0, 0, 0})
	_, err := NewFramer(DefaultMaxFrameSize).ReadMessage(context.Background(), reader)
	if !errors.Is(err, ErrZeroLengthFrame) {
		t.Fatalf("expected ErrZeroLengthFrame, got %v", err)
	}
}

func TestOversizedFrame(t *testing.T) {
	frame := make([]byte, 4)
	binary.BigEndian.PutUint32(frame[0:4], 1024)
	reader := bytes.NewReader(frame)
	_, err := NewFramer(16).ReadMessage(context.Background(), reader)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func TestMalformedCBOR(t *testing.T) {
	payload := []byte("not-cbor")
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(payload)))
	copy(frame[4:], payload)
	_, err := NewFramer(DefaultMaxFrameSize).ReadMessage(context.Background(), bytes.NewReader(frame))
	if !errors.Is(err, ErrInvalidCBOR) {
		t.Fatalf("expected ErrInvalidCBOR, got %v", err)
	}
}

func TestValidatePongMatchesRequestID(t *testing.T) {
	requestID := []byte("match")
	if err := ValidatePong(Message{Type: MessageTypePong, TransactionID: requestID}, requestID); err != nil {
		t.Fatalf("ValidatePong failed: %v", err)
	}
	if err := ValidatePong(Message{Type: MessageTypePong, TransactionID: []byte("wrong")}, requestID); !errors.Is(err, ErrUnexpectedID) {
		t.Fatalf("expected ErrUnexpectedID, got %v", err)
	}
}

func encodeTestFrame(t *testing.T, message Message) []byte {
	t.Helper()
	payload, err := EncodeMessage(message)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(payload)))
	copy(frame[4:], payload)
	return frame
}
