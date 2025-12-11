package server

import (
	"net"
	"sync"

	"github.com/aeolun/superchat/pkg/protocol"
)

// SafeConn wraps a net.Conn with automatic write synchronization to prevent
// concurrent writes from corrupting the wire protocol frames.
//
// Under load, multiple goroutines (request handlers and broadcast senders)
// may try to write to the same connection simultaneously. Without synchronization,
// their frame bytes interleave on the wire, causing frame corruption.
//
// SafeConn solves this by encapsulating both the connection and its write mutex,
// making it impossible to write without proper synchronization.
type SafeConn struct {
	conn net.Conn
	mu   sync.Mutex // Protects writes to conn
}

// NewSafeConn wraps a net.Conn with write synchronization
func NewSafeConn(conn net.Conn) *SafeConn {
	return &SafeConn{
		conn: conn,
	}
}

// EncodeFrame encodes and sends a protocol frame with automatic write synchronization.
// This is the ONLY way to write frames to the connection - the raw conn is private.
// Optional peerVersion controls compression (see protocol.EncodeFrame).
func (sc *SafeConn) EncodeFrame(frame *protocol.Frame, peerVersion ...uint8) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return protocol.EncodeFrame(sc.conn, frame, peerVersion...)
}

// ReadFrame reads a protocol frame from the connection.
// Reads don't need write synchronization.
func (sc *SafeConn) ReadFrame() (*protocol.Frame, error) {
	return protocol.DecodeFrame(sc.conn)
}

// Close closes the underlying connection
func (sc *SafeConn) Close() error {
	return sc.conn.Close()
}

// RemoteAddr returns the remote network address
func (sc *SafeConn) RemoteAddr() net.Addr {
	return sc.conn.RemoteAddr()
}

// WriteBytes writes raw bytes to the connection with synchronization.
// Used for pre-encoded frames in broadcast operations.
func (sc *SafeConn) WriteBytes(data []byte) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	_, err := sc.conn.Write(data)
	return err
}
