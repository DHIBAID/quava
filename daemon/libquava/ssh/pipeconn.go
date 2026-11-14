package ssh

import (
	"errors"
	"net"
	"os"
	"time"
)

type pipeConn struct {
	r *os.File
	w *os.File
}

// newTunnelPipe returns two connected, in-process net.Conn endpoints backed
// by OS pipes rather than net.Pipe. net.Pipe is a synchronous rendezvous
// with no internal buffering, and the SSH handshake has both sides write
// their identification string before reading the other's - a pattern
// net.Pipe is documented to deadlock on (golang.org/x/crypto/ssh's own test
// suite builds a buffered loopback-TCP helper for exactly this reason). A
// real loopback TCP socket would reopen the listening-socket exposure this
// package exists to close, so this uses kernel-buffered OS pipes instead:
// buffered like a real connection, but never reachable by anything except
// the two file descriptors this process already holds.
func newTunnelPipe() (serverSide, tunnelSide net.Conn, err error) {
	clientToServerRead, clientToServerWrite, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	serverToClientRead, serverToClientWrite, err := os.Pipe()
	if err != nil {
		_ = clientToServerRead.Close()
		_ = clientToServerWrite.Close()
		return nil, nil, err
	}

	serverSide = &pipeConn{r: clientToServerRead, w: serverToClientWrite}
	tunnelSide = &pipeConn{r: serverToClientRead, w: clientToServerWrite}
	return serverSide, tunnelSide, nil
}

func (c *pipeConn) Read(b []byte) (int, error)  { return c.r.Read(b) }
func (c *pipeConn) Write(b []byte) (int, error) { return c.w.Write(b) }

func (c *pipeConn) Close() error {
	return errors.Join(c.r.Close(), c.w.Close())
}

func (c *pipeConn) LocalAddr() net.Addr  { return pipeAddr{} }
func (c *pipeConn) RemoteAddr() net.Addr { return pipeAddr{} }

func (c *pipeConn) SetDeadline(t time.Time) error {
	return errors.Join(c.r.SetDeadline(t), c.w.SetDeadline(t))
}

func (c *pipeConn) SetReadDeadline(t time.Time) error  { return c.r.SetReadDeadline(t) }
func (c *pipeConn) SetWriteDeadline(t time.Time) error { return c.w.SetWriteDeadline(t) }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "quava-ssh-tunnel" }
