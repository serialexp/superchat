package client

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/aeolun/superchat/pkg/protocol"
)

// LoadTestConnection is a simplified connection for load testing that avoids
// goroutine overhead by reading/writing synchronously. Unlike the full Connection,
// it does not spawn readLoop/writeLoop goroutines and does not support auto-reconnect.
//
// This design reduces per-client goroutine count from 3 (readLoop + writeLoop + messageReader)
// to 0, allowing load tests to scale to 20k+ concurrent clients.
type LoadTestConnection struct {
	addr                  string
	conn                  net.Conn
	sendMu                sync.Mutex // Protects concurrent writes
	recvMu                sync.Mutex // Protects concurrent reads
	closed                bool
	mu                    sync.Mutex // Protects closed flag
	serverProtocolVersion uint8      // Server's protocol version from SERVER_CONFIG
}

// NewLoadTestConnection creates a new load test connection
func NewLoadTestConnection(addr string) *LoadTestConnection {
	return &LoadTestConnection{
		addr: addr,
	}
}

// Connect establishes a TCP connection to the server
func (c *LoadTestConnection) Connect() error {
	conn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	// Enable TCP_NODELAY for low latency
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
	}

	c.conn = conn
	return nil
}

// Close closes the connection
func (c *LoadTestConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}

// SendMessage encodes and sends a message synchronously
func (c *LoadTestConnection) SendMessage(msgType uint8, msg interface{}) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	// Check if closed
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("connection closed")
	}
	c.mu.Unlock()

	// Encode message
	var payload []byte
	var err error

	// Handle messages that implement Encode() method
	type Encoder interface {
		Encode() ([]byte, error)
	}

	if encoder, ok := msg.(Encoder); ok {
		payload, err = encoder.Encode()
		if err != nil {
			return fmt.Errorf("encode failed: %w", err)
		}
	} else {
		return fmt.Errorf("message does not implement Encode()")
	}

	// Create frame
	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Flags:   0,
		Payload: payload,
	}

	// Write frame to connection, passing server version for compression decisions
	if err := protocol.EncodeFrame(c.conn, frame, c.serverProtocolVersion); err != nil {
		return fmt.Errorf("write frame failed: %w", err)
	}

	return nil
}

// SetServerProtocolVersion sets the server's protocol version (from SERVER_CONFIG)
func (c *LoadTestConnection) SetServerProtocolVersion(version uint8) {
	c.serverProtocolVersion = version
}

// ReceiveMessage reads a frame from the connection with timeout
func (c *LoadTestConnection) ReceiveMessage(timeout time.Duration) (*protocol.Frame, error) {
	c.recvMu.Lock()
	defer c.recvMu.Unlock()

	// Check if closed
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("connection closed")
	}
	c.mu.Unlock()

	// Set read deadline
	if timeout > 0 {
		if err := c.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return nil, fmt.Errorf("set read deadline failed: %w", err)
		}
		// Clear deadline after read
		defer c.conn.SetReadDeadline(time.Time{})
	}

	// Read frame
	frame, err := protocol.DecodeFrame(c.conn)
	if err != nil {
		return nil, fmt.Errorf("read frame failed: %w", err)
	}

	return frame, nil
}

// Addr returns the connection address
func (c *LoadTestConnection) Addr() string {
	return c.addr
}
