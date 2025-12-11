package botlib

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/aeolun/superchat/pkg/protocol"
)

// connection handles the low-level protocol communication.
type connection struct {
	addr                  string
	conn                  net.Conn
	sendMu                sync.Mutex
	closed                bool
	mu                    sync.RWMutex
	serverProtocolVersion uint8 // Server's protocol version from SERVER_CONFIG

	// Response channels for request/response patterns
	responseMu  sync.Mutex
	responsesCh chan *protocol.Frame

	// Broadcast handler
	onFrame func(*protocol.Frame)
}

func newConnection(addr string) *connection {
	return &connection{
		addr:        addr,
		responsesCh: make(chan *protocol.Frame, 10),
	}
}

func (c *connection) connect() error {
	conn, err := net.Dial("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
	}

	c.conn = conn
	return nil
}

func (c *connection) close() error {
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

func (c *connection) isClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

func (c *connection) send(msgType uint8, msg protocol.ProtocolMessage) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if c.isClosed() {
		return fmt.Errorf("connection closed")
	}

	payload, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Flags:   0,
		Payload: payload,
	}

	// Pass server version for compression decisions
	c.mu.RLock()
	serverVersion := c.serverProtocolVersion
	c.mu.RUnlock()

	if err := protocol.EncodeFrame(c.conn, frame, serverVersion); err != nil {
		return fmt.Errorf("write frame failed: %w", err)
	}

	return nil
}

// setServerProtocolVersion sets the server's protocol version (from SERVER_CONFIG)
func (c *connection) setServerProtocolVersion(version uint8) {
	c.mu.Lock()
	c.serverProtocolVersion = version
	c.mu.Unlock()
}

// receiveLoop reads frames from the connection and dispatches them.
// Responses to requests (MESSAGE_POSTED, etc.) go to responsesCh.
// Broadcasts (NEW_MESSAGE, etc.) go to onFrame handler.
func (c *connection) receiveLoop() {
	for {
		if c.isClosed() {
			return
		}

		frame, err := protocol.DecodeFrame(c.conn)
		if err != nil {
			if c.isClosed() {
				return
			}
			// Connection error - could log or notify
			return
		}

		// Dispatch based on message type
		switch frame.Type {
		// Response types - send to response channel
		case protocol.TypeServerConfig,
			protocol.TypeNicknameResponse,
			protocol.TypeChannelList,
			protocol.TypeJoinResponse,
			protocol.TypeMessageList,
			protocol.TypeMessagePosted,
			protocol.TypeSubscribeOk,
			protocol.TypeError:
			select {
			case c.responsesCh <- frame:
			default:
				// Response channel full, drop oldest
				select {
				case <-c.responsesCh:
				default:
				}
				c.responsesCh <- frame
			}

		// Broadcast types - send to handler
		default:
			if c.onFrame != nil {
				c.onFrame(frame)
			}
		}
	}
}

// waitForResponse waits for a response with timeout.
func (c *connection) waitForResponse(timeout time.Duration) (*protocol.Frame, error) {
	select {
	case frame := <-c.responsesCh:
		return frame, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for response")
	}
}

// sendAndWait sends a message and waits for the response.
func (c *connection) sendAndWait(msgType uint8, msg protocol.ProtocolMessage, timeout time.Duration) (*protocol.Frame, error) {
	if err := c.send(msgType, msg); err != nil {
		return nil, err
	}
	return c.waitForResponse(timeout)
}

// expectType checks if the frame is of the expected type, handling errors.
func expectType(frame *protocol.Frame, expected uint8) error {
	if frame.Type == protocol.TypeError {
		errMsg := &protocol.ErrorMessage{}
		if err := errMsg.Decode(frame.Payload); err != nil {
			return fmt.Errorf("error response (decode failed)")
		}
		return fmt.Errorf("server error %d: %s", errMsg.ErrorCode, errMsg.Message)
	}
	if frame.Type != expected {
		return fmt.Errorf("unexpected response type 0x%02X, expected 0x%02X", frame.Type, expected)
	}
	return nil
}
