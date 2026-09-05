package discovery

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"strconv"
	"strings"

	"libquava/protocol"
)

// Discover returns devices using the default discovery client.
func Discover(ctx context.Context) ([]Device, error) {
	client, err := NewClient()
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.Discover(ctx)
}

// Connection is a TCP connection to a discovered Quava device.
type Connection struct {
	device Device
	conn   *protocol.Conn
}

// ProtocolConn returns the framed protocol connection.
func (c *Connection) ProtocolConn() *protocol.Conn {
	if c == nil {
		return nil
	}
	return c.conn
}

// Connect opens a TCP connection to the discovered device.
func (d Device) Connect(ctx context.Context) (*Connection, error) {
	if strings.TrimSpace(d.Host) == "" {
		return nil, fmt.Errorf("discovery: empty host for %q", d.Name)
	}
	if d.Port <= 0 || d.Port > 65535 {
		return nil, fmt.Errorf("discovery: invalid port %d", d.Port)
	}

	dialer := net.Dialer{}
	targetHost := strings.TrimSpace(d.Address)
	if targetHost == "" {
		targetHost = strings.TrimSuffix(d.Host, ".")
	}
	address := net.JoinHostPort(targetHost, strconv.Itoa(d.Port))
	netConn, err := dialer.DialContext(ctx, "tcp4", address)
	if err != nil {
		return nil, err
	}

	return &Connection{device: d, conn: protocol.NewConn(netConn)}, nil
}

// Close closes the TCP connection.
func (c *Connection) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Ping sends a PING message and waits for the matching PONG.
func (c *Connection) Ping(ctx context.Context) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("discovery: nil connection")
	}

	requestID, err := generateRequestID()
	if err != nil {
		return err
	}

	if err := c.conn.WriteMessage(ctx, protocol.NewMessage(protocol.MessageTypePing, requestID, nil)); err != nil {
		return err
	}

	response, err := c.conn.ReadMessage(ctx)
	if err != nil {
		return err
	}
	return protocol.ValidatePong(response, requestID)
}

func generateRequestID() ([]byte, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, err
	}
	return append([]byte(nil), raw[:]...), nil
}
