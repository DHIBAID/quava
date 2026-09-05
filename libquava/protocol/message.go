package protocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/fxamacker/cbor/v2"
)

const DefaultMaxFrameSize = 64 * 1024

const (
	ProtocolName    = "quava-pairing"
	ProtocolVersion = 1

	MessageTypePairRequest      = 0x01
	MessageTypePairChallenge    = 0x02
	MessageTypePairAuthenticate = 0x03
	MessageTypePairConfirm      = 0x04
	MessageTypePairComplete     = 0x05
	MessageTypePairReject       = 0x06
	MessageTypePairCancel       = 0x07
	MessageTypePing             = 0x20
	MessageTypePong             = 0x21

	ReasonUserRejected         = 0x01
	ReasonAuthenticationFailed = 0x02
	ReasonUnsupportedVersion   = 0x03
	ReasonInvalidMessage       = 0x04
	ReasonTimeout              = 0x05
	ReasonCancelled            = 0x06
	ReasonIdentityConflict     = 0x07
	ReasonInternalError        = 0x08
)

var (
	ErrZeroLengthFrame = errors.New("protocol: zero-length frame")
	ErrFrameTooLarge   = errors.New("protocol: frame too large")
	ErrInvalidCBOR     = errors.New("protocol: invalid cbor")
	ErrInvalidMessage  = errors.New("protocol: invalid message")
	ErrUnexpectedType  = errors.New("protocol: unexpected message type")
	ErrUnexpectedID    = errors.New("protocol: unexpected transaction id")
)

// Message is a Quava CBOR message using integer keys.
type Message struct {
	Version       uint64
	Type          uint64
	TransactionID []byte
	Payload       map[uint64]any
}

// Framer encodes and decodes length-prefixed CBOR messages.
type Framer struct {
	maxFrameSize uint32
}

// Conn wraps a TCP connection with Quava framing.
type Conn struct {
	conn   net.Conn
	framer Framer
}

// NewConn creates a framed Quava TCP connection wrapper.
func NewConn(conn net.Conn) *Conn {
	return &Conn{conn: conn, framer: NewFramer(DefaultMaxFrameSize)}
}

// NewFramer creates a framer with the provided maximum payload size.
func NewFramer(maxFrameSize uint32) Framer {
	if maxFrameSize == 0 {
		maxFrameSize = DefaultMaxFrameSize
	}
	return Framer{maxFrameSize: maxFrameSize}
}

// Close closes the underlying TCP connection.
func (c *Conn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// WriteMessage writes a single framed CBOR message.
func (c *Conn) WriteMessage(ctx context.Context, message Message) error {
	if c == nil || c.conn == nil {
		return errors.New("protocol: nil connection")
	}
	return c.framer.WriteMessage(ctx, c.conn, message)
}

// ReadMessage reads a single framed CBOR message.
func (c *Conn) ReadMessage(ctx context.Context) (Message, error) {
	if c == nil || c.conn == nil {
		return Message{}, errors.New("protocol: nil connection")
	}
	return c.framer.ReadMessage(ctx, c.conn)
}

// NewMessage constructs a protocol message.
func NewMessage(messageType uint64, transactionID []byte, payload map[uint64]any) Message {
	if payload == nil {
		payload = map[uint64]any{}
	}
	return Message{Version: ProtocolVersion, Type: messageType, TransactionID: append([]byte(nil), transactionID...), Payload: payload}
}

// EncodeMessage encodes a message as CBOR.
func EncodeMessage(message Message) ([]byte, error) {
	encoded := map[uint64]any{
		0: message.Version,
		1: message.Type,
		2: append([]byte(nil), message.TransactionID...),
		3: message.Payload,
	}
	return cbor.Marshal(encoded)
}

// DecodeMessage decodes a CBOR message.
func DecodeMessage(payload []byte) (Message, error) {
	var encoded map[uint64]any
	if err := cbor.Unmarshal(payload, &encoded); err != nil {
		return Message{}, fmt.Errorf("%w: %v", ErrInvalidCBOR, err)
	}

	message := Message{Payload: map[uint64]any{}}
	if rawVersion, ok := encoded[0]; ok {
		version, err := uint64Value(rawVersion)
		if err != nil {
			return Message{}, fmt.Errorf("%w: version: %v", ErrInvalidMessage, err)
		}
		message.Version = version
	}
	if rawType, ok := encoded[1]; ok {
		typeValue, err := uint64Value(rawType)
		if err != nil {
			return Message{}, fmt.Errorf("%w: type: %v", ErrInvalidMessage, err)
		}
		message.Type = typeValue
	}
	if rawTransactionID, ok := encoded[2]; ok {
		transactionID, err := bytesValue(rawTransactionID)
		if err != nil {
			return Message{}, fmt.Errorf("%w: transaction_id: %v", ErrInvalidMessage, err)
		}
		message.TransactionID = transactionID
	}
	if rawPayload, ok := encoded[3]; ok {
		switch typed := rawPayload.(type) {
		case map[any]any:
			message.Payload = make(map[uint64]any, len(typed))
			for key, value := range typed {
				uintKey, err := uint64Value(key)
				if err != nil {
					return Message{}, fmt.Errorf("%w: payload key: %v", ErrInvalidMessage, err)
				}
				message.Payload[uintKey] = value
			}
		case map[uint64]any:
			message.Payload = typed
		default:
			return Message{}, fmt.Errorf("%w: payload", ErrInvalidMessage)
		}
	}

	return message, nil
}

// ReadMessage reads a message from an io.Reader.
func (f Framer) ReadMessage(ctx context.Context, reader io.Reader) (Message, error) {
	payload, err := f.ReadFrame(ctx, reader)
	if err != nil {
		return Message{}, err
	}
	return DecodeMessage(payload)
}

// WriteMessage writes a message to an io.Writer.
func (f Framer) WriteMessage(ctx context.Context, writer io.Writer, message Message) error {
	payload, err := EncodeMessage(message)
	if err != nil {
		return fmt.Errorf("protocol: marshal message: %w", err)
	}
	return f.WriteFrame(ctx, writer, payload)
}

// ReadFrame reads a framed payload from an io.Reader.
func (f Framer) ReadFrame(ctx context.Context, reader io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if err := readExact(ctx, reader, header); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(header)
	if length == 0 {
		return nil, ErrZeroLengthFrame
	}
	if length > f.maxFrameSize {
		return nil, fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, length, f.maxFrameSize)
	}

	payload := make([]byte, int(length))
	if err := readExact(ctx, reader, payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// WriteFrame writes a payload with a 4-byte big-endian length prefix.
func (f Framer) WriteFrame(ctx context.Context, writer io.Writer, payload []byte) error {
	if uint32(len(payload)) > f.maxFrameSize {
		return fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, len(payload), f.maxFrameSize)
	}

	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(payload)))
	copy(frame[4:], payload)

	return writeExact(ctx, writer, frame)
}

func uint64Value(value any) (uint64, error) {
	switch typed := value.(type) {
	case uint64:
		return typed, nil
	case uint32:
		return uint64(typed), nil
	case uint16:
		return uint64(typed), nil
	case uint8:
		return uint64(typed), nil
	case int:
		if typed < 0 {
			return 0, fmt.Errorf("negative integer %d", typed)
		}
		return uint64(typed), nil
	case int64:
		if typed < 0 {
			return 0, fmt.Errorf("negative integer %d", typed)
		}
		return uint64(typed), nil
	case int32:
		if typed < 0 {
			return 0, fmt.Errorf("negative integer %d", typed)
		}
		return uint64(typed), nil
	default:
		return 0, fmt.Errorf("unexpected %T", value)
	}
}

func bytesValue(value any) ([]byte, error) {
	switch typed := value.(type) {
	case []byte:
		return append([]byte(nil), typed...), nil
	case [16]byte:
		return typed[:], nil
	default:
		return nil, fmt.Errorf("unexpected %T", value)
	}
}

func readExact(ctx context.Context, reader io.Reader, buf []byte) error {
	read := 0
	for read < len(buf) {
		if err := ctx.Err(); err != nil {
			return err
		}

		if deadlineReader, ok := reader.(interface{ SetReadDeadline(time.Time) error }); ok {
			deadline := time.Now().Add(250 * time.Millisecond)
			if ctxDeadline, ok := ctx.Deadline(); ok {
				deadline = ctxDeadline
			}
			_ = deadlineReader.SetReadDeadline(deadline)
		}

		n, err := reader.Read(buf[read:])
		if n > 0 {
			read += n
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			if err := ctx.Err(); err != nil {
				return err
			}
			continue
		}
		return err
	}
	return nil
}

func writeExact(ctx context.Context, writer io.Writer, buf []byte) error {
	written := 0
	for written < len(buf) {
		if err := ctx.Err(); err != nil {
			return err
		}

		if deadlineWriter, ok := writer.(interface{ SetWriteDeadline(time.Time) error }); ok {
			deadline := time.Now().Add(250 * time.Millisecond)
			if ctxDeadline, ok := ctx.Deadline(); ok {
				deadline = ctxDeadline
			}
			_ = deadlineWriter.SetWriteDeadline(deadline)
		}

		n, err := writer.Write(buf[written:])
		if n > 0 {
			written += n
		}
		if err == nil {
			continue
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			if err := ctx.Err(); err != nil {
				return err
			}
			continue
		}
		return err
	}
	return nil
}

// PayloadBytes extracts a byte slice from a payload map.
func PayloadBytes(payload map[uint64]any, key uint64) ([]byte, error) {
	value, ok := payload[key]
	if !ok {
		return nil, fmt.Errorf("protocol: missing field %d", key)
	}
	return bytesValue(value)
}

// PayloadString extracts a string from a payload map.
func PayloadString(payload map[uint64]any, key uint64) (string, error) {
	value, ok := payload[key]
	if !ok {
		return "", fmt.Errorf("protocol: missing field %d", key)
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("protocol: field %d has unexpected type %T", key, value)
	}
	return stringValue, nil
}

// PayloadBool extracts a bool from a payload map.
func PayloadBool(payload map[uint64]any, key uint64) (bool, error) {
	value, ok := payload[key]
	if !ok {
		return false, fmt.Errorf("protocol: missing field %d", key)
	}
	boolean, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("protocol: field %d has unexpected type %T", key, value)
	}
	return boolean, nil
}

// PayloadUint extracts an unsigned integer from a payload map.
func PayloadUint(payload map[uint64]any, key uint64) (uint64, error) {
	value, ok := payload[key]
	if !ok {
		return 0, fmt.Errorf("protocol: missing field %d", key)
	}
	return uint64Value(value)
}

// PayloadUintSlice extracts an array of unsigned integers from a payload map.
func PayloadUintSlice(payload map[uint64]any, key uint64) ([]uint64, error) {
	value, ok := payload[key]
	if !ok {
		return nil, fmt.Errorf("protocol: missing field %d", key)
	}
	switch typed := value.(type) {
	case []uint64:
		return append([]uint64(nil), typed...), nil
	case []any:
		result := make([]uint64, 0, len(typed))
		for _, item := range typed {
			converted, err := uint64Value(item)
			if err != nil {
				return nil, fmt.Errorf("protocol: field %d contains invalid integer: %w", key, err)
			}
			result = append(result, converted)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("protocol: field %d has unexpected type %T", key, value)
	}
}

// ValidatePong checks that a pong response matches the request transaction ID.
func ValidatePong(message Message, requestTransactionID []byte) error {
	if message.Type != MessageTypePong {
		return fmt.Errorf("%w: got %d", ErrUnexpectedType, message.Type)
	}
	if !bytes.Equal(message.TransactionID, requestTransactionID) {
		return fmt.Errorf("%w", ErrUnexpectedID)
	}
	return nil
}
