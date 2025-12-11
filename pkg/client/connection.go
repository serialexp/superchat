package client

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aeolun/superchat/pkg/protocol"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ConnectionStateType represents the connection status
type ConnectionStateType int

const (
	StateTypeConnected ConnectionStateType = iota
	StateTypeDisconnected
	StateTypeReconnecting
)

// ConnectionStateUpdate represents a connection state change
type ConnectionStateUpdate struct {
	State   ConnectionStateType
	Attempt int
	Err     error
}

// DisconnectReason indicates why a connection was lost
type DisconnectReason int

const (
	DisconnectUnknown DisconnectReason = iota
	DisconnectIdle                      // No activity, normal keepalive cycle
	DisconnectError                     // Read/write error
	DisconnectServerDown                // Server closed connection
	DisconnectUserRequested             // User explicitly disconnected
)

// Connection represents a client connection to the server
type Connection struct {
	addr            string // Display address with scheme (e.g., "ws://server:6467")
	rawAddr         string // Raw host:port without scheme (e.g., "server:6467")
	dial            func() (net.Conn, error)
	conn            net.Conn
	mu              sync.RWMutex
	connected       bool
	reconnecting    bool
	securityWarning string
	warningOnce     sync.Once
	connectionType  string // "tcp", "ssh", or "websocket"

	// Channels for communication
	incoming    chan *protocol.Frame
	outgoing    chan *protocol.Frame
	errors      chan error
	stateChange chan ConnectionStateUpdate

	// Auto-reconnect settings
	autoReconnect     bool
	reconnectDelay    time.Duration
	maxReconnectDelay time.Duration

	// Disconnect tracking
	lastDisconnectReason DisconnectReason
	lastSuccessfulMethod string // Track what worked for auto-reconnect

	// Protocol validation
	protocolTimeout       time.Duration
	serverProtocolVersion uint8 // Server's protocol version from SERVER_CONFIG

	// Traffic counters (bytes on the wire)
	bytesSent     atomic.Uint64
	bytesReceived atomic.Uint64

	// Bandwidth throttling (for testing)
	throttleBytesPerSec int // 0 = no throttle

	// Logging
	logger *log.Logger

	// Shutdown
	shutdown chan struct{}
	closed   bool // Track if Close() has been called
	wg       sync.WaitGroup
}

// NewConnection creates a new client connection
func NewConnection(addr string) (*Connection, error) {
	dialConfig, err := parseServerAddress(addr)
	if err != nil {
		return nil, err
	}

	return &Connection{
		addr:              dialConfig.display,
		rawAddr:           dialConfig.raw,
		dial:              dialConfig.dial,
		securityWarning:   dialConfig.warning,
		incoming:          make(chan *protocol.Frame, 100),
		outgoing:          make(chan *protocol.Frame, 100),
		errors:            make(chan error, 10),
		stateChange:       make(chan ConnectionStateUpdate, 10),
		autoReconnect:     true,
		reconnectDelay:    1 * time.Second,
		maxReconnectDelay: 30 * time.Second,
		protocolTimeout:   1 * time.Second,
		shutdown:          make(chan struct{}),
	}, nil
}

// SetLogger sets a logger for debugging connection events
func (c *Connection) SetLogger(logger *log.Logger) {
	c.logger = logger
}

// SetThrottle sets bandwidth throttling in bytes per second (0 = no throttle)
// Example: SetThrottle(3600) simulates 28.8kbps dial-up modem
func (c *Connection) SetThrottle(bytesPerSec int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.throttleBytesPerSec = bytesPerSec
	if bytesPerSec > 0 {
		c.logf("Bandwidth throttling enabled: %d bytes/sec (~%.1f kbps)", bytesPerSec, float64(bytesPerSec*8)/1000)
	} else {
		c.logf("Bandwidth throttling disabled")
	}
}

// DisableAutoReconnect disables automatic reconnection on connection loss
func (c *Connection) DisableAutoReconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.autoReconnect = false
}

// logf logs a message if a logger is set
func (c *Connection) logf(format string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

// Connect establishes connection to the server with WebSocket fallback
func (c *Connection) Connect() error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	c.mu.Unlock()

	c.logf("Connecting to %s...", c.addr)

	if c.dial == nil {
		return fmt.Errorf("no dialer configured")
	}

	// Try primary connection method (TCP, SSH, or WebSocket)
	conn, err := c.dial()

	// Determine connection type from address
	connType := "tcp" // Default
	if strings.HasPrefix(c.addr, "ssh://") {
		connType = "ssh"
	} else if strings.HasPrefix(c.addr, "ws://") {
		connType = "websocket"
	}

	if err != nil {
		c.logf("Primary connection failed: %v", err)

		// Only try WebSocket fallback if not already trying WebSocket
		if connType != "websocket" {
			c.logf("Attempting WebSocket fallback...")
			wsConn, wsAddr, wsErr := c.tryWebSocketFallback()
			if wsErr != nil {
				c.logf("WebSocket fallback also failed: %v", wsErr)
				// Set connectionType even on failure so caller knows what we tried
				c.mu.Lock()
				c.connectionType = connType
				c.mu.Unlock()
				return fmt.Errorf("all connection methods failed - Primary: %w, WebSocket: %v", err, wsErr)
			}
			conn = wsConn
			connType = "websocket"
			c.mu.Lock()
			c.addr = wsAddr // Update display address to show actual connection
			c.mu.Unlock()
			c.logf("WebSocket fallback successful!")
		} else {
			// Already tried WebSocket explicitly, no fallback
			// Set connectionType even on failure so caller knows what we tried
			c.mu.Lock()
			c.connectionType = connType
			c.mu.Unlock()
			return err
		}
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.connectionType = connType
	c.mu.Unlock()

	c.logf("Connected successfully to %s (%s)", c.addr, connType)

	// Validate protocol by sending PING and waiting for response
	c.logf("Validating protocol...")
	if err := c.validateProtocol(); err != nil {
		c.logf("Protocol validation failed: %v", err)
		conn.Close()

		// Only try WebSocket fallback if not already using it
		if connType != "websocket" {
			c.logf("Attempting WebSocket fallback after protocol failure...")
			wsConn, wsAddr, wsErr := c.tryWebSocketFallback()
			if wsErr != nil {
				c.logf("WebSocket fallback also failed: %v", wsErr)
				return fmt.Errorf("protocol validation failed and WebSocket fallback failed: %w", err)
			}

			// Update connection and try validation again
			conn = wsConn
			connType = "websocket"
			c.mu.Lock()
			c.conn = conn
			c.connectionType = connType
			c.addr = wsAddr // Update display address to show actual connection
			c.mu.Unlock()

			c.logf("WebSocket fallback connected, validating protocol...")
			if err := c.validateProtocol(); err != nil {
				conn.Close()
				return fmt.Errorf("protocol validation failed on WebSocket: %w", err)
			}
		} else {
			return fmt.Errorf("protocol validation failed: %w", err)
		}
	}
	c.logf("Protocol validation successful")

	// Record successful connection method
	c.mu.Lock()
	c.lastSuccessfulMethod = connType
	c.mu.Unlock()

	c.warningOnce.Do(func() {
		if c.securityWarning != "" {
			c.logf("WARNING: %s", c.securityWarning)
		}
	})

	// Start reader and writer goroutines
	c.wg.Add(2)
	go c.readLoop()
	go c.writeLoop()

	return nil
}

// tryWebSocketFallback attempts to connect via WebSocket on port 8080
// Returns the connection and the display address (with scheme)
func (c *Connection) tryWebSocketFallback() (net.Conn, string, error) {
	// Parse the address to extract just the hostname
	var host string

	// If address has a scheme, parse as URL
	if strings.Contains(c.addr, "://") {
		u, err := url.Parse(c.addr)
		if err != nil {
			// Fallback to using addr directly if parse fails
			host = c.addr
		} else {
			// Use Hostname() to strip port and get clean hostname
			host = u.Hostname()
		}
	} else {
		// No scheme, parse as host:port
		parsedHost, _, err := net.SplitHostPort(c.addr)
		if err != nil {
			// No port either, use as-is
			host = c.addr
		} else {
			host = parsedHost
		}
	}

	// Try WebSocket on port 8080
	wsAddr := fmt.Sprintf("%s:8080", host)
	c.logf("Attempting WebSocket connection to %s", wsAddr)

	// Try both WSS and WS - WSS first (more secure), then WS
	var lastErr error

	// Try WSS
	conn, err := DialWebSocket(wsAddr, true)
	if err == nil {
		c.logf("Connected via WSS")
		return conn, fmt.Sprintf("wss://%s", wsAddr), nil
	}
	c.logf("WSS failed: %v", err)
	lastErr = fmt.Errorf("WSS: %w", err)

	// Try plain WS
	conn, err = DialWebSocket(wsAddr, false)
	if err == nil {
		c.logf("Connected via WS (insecure)")
		return conn, fmt.Sprintf("ws://%s", wsAddr), nil
	}
	c.logf("WS failed: %v", err)

	// Both failed
	return nil, "", fmt.Errorf("both WSS and WS failed - %v, WS: %w", lastErr, err)
}

// GetConnectionType returns the current connection type (tcp, ssh, or websocket)
func (c *Connection) GetConnectionType() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connectionType
}

// validateProtocol reads the initial SERVER_CONFIG to verify protocol version
func (c *Connection) validateProtocol() error {
	c.mu.RLock()
	conn := c.conn
	timeout := c.protocolTimeout
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("no connection available")
	}

	// Set read deadline
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	// Read the first frame (server sends SERVER_CONFIG immediately)
	responseFrame, err := protocol.DecodeFrame(conn)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Clear read deadline
	conn.SetReadDeadline(time.Time{})

	// Server should send SERVER_CONFIG as the first message
	if responseFrame.Type != protocol.TypeServerConfig {
		return fmt.Errorf("unexpected response type: got %d, expected SERVER_CONFIG (%d)", responseFrame.Type, protocol.TypeServerConfig)
	}

	// Decode SERVER_CONFIG to check protocol version
	serverConfig := &protocol.ServerConfigMessage{}
	if err := serverConfig.Decode(responseFrame.Payload); err != nil {
		return fmt.Errorf("failed to decode SERVER_CONFIG: %w", err)
	}

	// Store server's protocol version for compression decisions
	c.serverProtocolVersion = serverConfig.ProtocolVersion

	// Best effort: try to talk to servers of any version
	// Core protocol is stable; just don't use features the server might not understand
	if serverConfig.ProtocolVersion > protocol.ProtocolVersion {
		c.logf("Note: server uses newer protocol v%d (we are v%d), some features may not work",
			serverConfig.ProtocolVersion, protocol.ProtocolVersion)
	}

	// Protocol validation successful - put the SERVER_CONFIG frame into the incoming channel
	// so it can be processed normally by the message loop
	select {
	case c.incoming <- responseFrame:
	default:
		// Channel full, this shouldn't happen during initial connection
		c.logf("Warning: incoming channel full during protocol validation")
	}

	return nil
}

// Disconnect closes the connection
func (c *Connection) Disconnect() {
	c.disconnectWithReason(DisconnectUserRequested)
}

// disconnectWithReason closes the connection and records why
func (c *Connection) disconnectWithReason(reason DisconnectReason) {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return
	}
	c.logf("Disconnecting from %s (reason: %v)", c.addr, reason)
	c.connected = false
	c.lastDisconnectReason = reason
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()
}

// Close shuts down the connection permanently
func (c *Connection) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return // Already closed
	}
	c.closed = true
	c.mu.Unlock()

	c.logf("DEBUG: Closing connection - sending shutdown signal")
	close(c.shutdown)
	c.Disconnect()
	c.logf("DEBUG: Waiting for goroutines to finish")
	c.wg.Wait()
	c.logf("DEBUG: Closing channels")
	close(c.incoming)
	close(c.outgoing)
	close(c.errors)
	close(c.stateChange)
	c.logf("DEBUG: Connection fully closed")
}

// Send sends a frame to the server
func (c *Connection) Send(frame *protocol.Frame) error {
	select {
	case c.outgoing <- frame:
		return nil
	case <-c.shutdown:
		return fmt.Errorf("connection closed")
	default:
		return fmt.Errorf("outgoing queue full")
	}
}

// Incoming returns the channel for receiving frames from server
func (c *Connection) Incoming() <-chan *protocol.Frame {
	return c.incoming
}

// Errors returns the channel for connection errors
func (c *Connection) Errors() <-chan error {
	return c.errors
}

// StateChanges returns the channel for connection state updates
func (c *Connection) StateChanges() <-chan ConnectionStateUpdate {
	return c.stateChange
}

// IsConnected returns whether the connection is active
func (c *Connection) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetAddress returns the server address
func (c *Connection) GetAddress() string {
	return c.addr
}

// GetRawAddress returns the raw address without scheme (e.g., "server:6467" or "user@server:6466")
func (c *Connection) GetRawAddress() string {
	return c.rawAddr
}

// GetBytesSent returns the total bytes sent
func (c *Connection) GetBytesSent() uint64 {
	return c.bytesSent.Load()
}

// GetBytesReceived returns the total bytes received
func (c *Connection) GetBytesReceived() uint64 {
	return c.bytesReceived.Load()
}

// readLoop reads frames from the connection
func (c *Connection) readLoop() {
	defer c.wg.Done()

	for {
		c.mu.RLock()
		conn := c.conn
		connected := c.connected
		throttle := c.throttleBytesPerSec
		c.mu.RUnlock()

		if !connected || conn == nil {
			break
		}

		// Build reader chain: conn -> throttle (optional) -> counter
		var reader io.Reader = conn
		if throttle > 0 {
			reader = newThrottledReader(reader, throttle)
		}
		// Always count bytes at the lowest level
		reader = &countingReader{r: reader, counter: &c.bytesReceived}

		frame, err := protocol.DecodeFrame(reader)

		if err != nil {
			if err == io.EOF {
				c.logf("Connection closed by server (EOF)")
				c.handleDisconnectWithReason(DisconnectServerDown)
				return
			}
			c.logf("Read error: %v", err)
			c.errors <- fmt.Errorf("read error: %w", err)
			c.handleDisconnectWithReason(DisconnectError)
			return
		}

		c.logf("← RECV: Type=0x%02X Flags=0x%02X PayloadLen=%d", frame.Type, frame.Flags, len(frame.Payload))

		select {
		case c.incoming <- frame:
		case <-c.shutdown:
			return
		}
	}
}

// countingReader wraps an io.Reader and counts bytes read using atomic counter
type countingReader struct {
	r       io.Reader
	counter *atomic.Uint64
}

func (cr *countingReader) Read(p []byte) (n int, err error) {
	n, err = cr.r.Read(p)
	if n > 0 && cr.counter != nil {
		cr.counter.Add(uint64(n))
	}
	return n, err
}

// countingWriter wraps an io.Writer and counts bytes written using atomic counter
type countingWriter struct {
	w       io.Writer
	counter *atomic.Uint64
}

func (cw *countingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	if n > 0 && cw.counter != nil {
		cw.counter.Add(uint64(n))
	}
	return n, err
}

// throttledReader wraps an io.Reader and limits read rate to bytesPerSec
type throttledReader struct {
	r            io.Reader
	bytesPerSec  int
	lastReadTime time.Time
	mu           sync.Mutex
}

func newThrottledReader(r io.Reader, bytesPerSec int) *throttledReader {
	return &throttledReader{
		r:            r,
		bytesPerSec:  bytesPerSec,
		lastReadTime: time.Now(),
	}
}

func (tr *throttledReader) Read(p []byte) (n int, err error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	// Limit read size to avoid overshooting rate
	maxChunkSize := tr.bytesPerSec / 10 // Read in smaller chunks for smoother throttling
	if maxChunkSize < 1 {
		maxChunkSize = 1
	}
	if len(p) > maxChunkSize {
		p = p[:maxChunkSize]
	}

	n, err = tr.r.Read(p)
	if n > 0 {
		// Calculate required delay based on bytes read
		elapsed := time.Since(tr.lastReadTime)
		expectedDuration := time.Duration(float64(n) / float64(tr.bytesPerSec) * float64(time.Second))

		if expectedDuration > elapsed {
			time.Sleep(expectedDuration - elapsed)
		}

		tr.lastReadTime = time.Now()
	}

	return n, err
}

// throttledWriter wraps an io.Writer and limits write rate to bytesPerSec
type throttledWriter struct {
	w             io.Writer
	bytesPerSec   int
	lastWriteTime time.Time
	mu            sync.Mutex
}

func newThrottledWriter(w io.Writer, bytesPerSec int) *throttledWriter {
	return &throttledWriter{
		w:             w,
		bytesPerSec:   bytesPerSec,
		lastWriteTime: time.Now(),
	}
}

func (tw *throttledWriter) Write(p []byte) (n int, err error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	// Write in chunks to maintain rate limit
	totalWritten := 0
	for totalWritten < len(p) {
		// Calculate chunk size
		chunkSize := tw.bytesPerSec / 10 // Write in smaller chunks for smoother throttling
		if chunkSize < 1 {
			chunkSize = 1
		}
		remaining := len(p) - totalWritten
		if chunkSize > remaining {
			chunkSize = remaining
		}

		// Write chunk
		written, err := tw.w.Write(p[totalWritten : totalWritten+chunkSize])
		totalWritten += written

		if err != nil {
			return totalWritten, err
		}

		// Throttle if needed
		if totalWritten < len(p) {
			elapsed := time.Since(tw.lastWriteTime)
			expectedDuration := time.Duration(float64(written) / float64(tw.bytesPerSec) * float64(time.Second))

			if expectedDuration > elapsed {
				time.Sleep(expectedDuration - elapsed)
			}

			tw.lastWriteTime = time.Now()
		}
	}

	return totalWritten, nil
}

// writeLoop sends frames to the connection
func (c *Connection) writeLoop() {
	defer c.wg.Done()

	for {
		select {
		case frame := <-c.outgoing:
			c.mu.RLock()
			conn := c.conn
			connected := c.connected
			throttle := c.throttleBytesPerSec
			serverVersion := c.serverProtocolVersion
			c.mu.RUnlock()

			if !connected || conn == nil {
				continue
			}

			// Encode to buffer first, passing server version for compression decisions
			var buf bytes.Buffer
			if err := protocol.EncodeFrame(&buf, frame, serverVersion); err != nil {
				c.logf("Encode error: %v", err)
				c.errors <- fmt.Errorf("encode error: %w", err)
				continue
			}

			frameBytes := buf.Bytes()

			// Build writer chain: conn -> throttle (optional) -> counter
			var writer io.Writer = conn
			if throttle > 0 {
				writer = newThrottledWriter(writer, throttle)
			}
			// Always count bytes at the lowest level
			writer = &countingWriter{w: writer, counter: &c.bytesSent}

			if _, err := writer.Write(frameBytes); err != nil {
				c.logf("Write error: %v", err)
				c.errors <- fmt.Errorf("write error: %w", err)
				c.handleDisconnectWithReason(DisconnectError)
				return
			}

			c.logf("→ SEND: Type=0x%02X Flags=0x%02X PayloadLen=%d", frame.Type, frame.Flags, len(frame.Payload))

		case <-c.shutdown:
			return
		}
	}
}

// handleDisconnect handles unexpected disconnection
func (c *Connection) handleDisconnect() {
	c.handleDisconnectWithReason(DisconnectUnknown)
}

func (c *Connection) handleDisconnectWithReason(reason DisconnectReason) {
	c.mu.Lock()
	wasConnected := c.connected
	c.connected = false
	c.lastDisconnectReason = reason
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	if !wasConnected {
		return
	}

	c.logf("Disconnected from server (reason: %v)", reason)

	disconnectErr := fmt.Errorf("disconnected from server")
	c.errors <- disconnectErr

	// Send disconnected state
	c.logf("DEBUG: Attempting to send StateTypeDisconnected on stateChange channel")
	select {
	case c.stateChange <- ConnectionStateUpdate{State: StateTypeDisconnected, Err: disconnectErr}:
		c.logf("DEBUG: Successfully sent StateTypeDisconnected")
	default:
		c.logf("DEBUG: Failed to send StateTypeDisconnected (channel full or closed)")
	}

	// Auto-reconnect if enabled
	// For idle disconnects, reconnect immediately without modal
	if c.autoReconnect {
		c.logf("Auto-reconnect enabled, starting reconnect loop")
		go c.reconnectLoop()
	}
}

// reconnectLoop attempts to reconnect with exponential backoff
func (c *Connection) reconnectLoop() {
	c.mu.Lock()
	if c.reconnecting {
		c.mu.Unlock()
		return
	}
	c.reconnecting = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.reconnecting = false
		c.mu.Unlock()
	}()

	delay := c.reconnectDelay
	attempt := 1

	for {
		select {
		case <-c.shutdown:
			c.logf("Reconnect loop cancelled (shutdown)")
			return
		case <-time.After(delay):
			c.logf("Reconnect attempt %d to %s", attempt, c.addr)

			// Send reconnecting state
			select {
			case c.stateChange <- ConnectionStateUpdate{State: StateTypeReconnecting, Attempt: attempt}:
			default:
			}

			if err := c.Connect(); err != nil {
				c.logf("Reconnect attempt %d failed: %v", attempt, err)

				// Exponential backoff
				delay = delay * 2
				if delay > c.maxReconnectDelay {
					delay = c.maxReconnectDelay
				}
				c.logf("Next reconnect attempt in %v", delay)
				attempt++
				continue
			}

			c.logf("Reconnected successfully after %d attempts", attempt)

			// Send connected state
			select {
			case c.stateChange <- ConnectionStateUpdate{State: StateTypeConnected}:
			default:
			}

			return
		}
	}
}

// SendMessage is a helper to send a protocol message
func (c *Connection) SendMessage(msgType uint8, msg interface{}) error {
	var payload []byte
	var err error

	switch m := msg.(type) {
	case interface{ Encode() ([]byte, error) }:
		payload, err = m.Encode()
	default:
		return fmt.Errorf("message type does not implement Encode()")
	}

	if err != nil {
		return err
	}

	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Flags:   0,
		Payload: payload,
	}

	return c.Send(frame)
}

type dialConfig struct {
	display string // Display address with scheme
	raw     string // Raw host:port without scheme
	dial    func() (net.Conn, error)
	warning string
}

const (
	defaultTCPPort            = "6465"
	defaultSSHPort            = "6466"
	defaultHTTPPort           = "8080"
	superChatSSHVersionPrefix = "SSH-2.0-SuperChat"
)

func parseServerAddress(raw string) (*dialConfig, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("server address is empty")
	}

	scheme := "tcp"
	user := ""
	hostPort := trimmed
	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid server address %q: %w", raw, err)
		}

		if u.Scheme != "" {
			scheme = strings.ToLower(u.Scheme)
		}

		if u.User != nil {
			user = u.User.Username()
		}

		if u.Host != "" {
			hostPort = u.Host
		} else if u.Path != "" {
			hostPort = u.Path
		}

		hostPort = strings.TrimPrefix(hostPort, "//")
	}

	switch scheme {
	case "tcp", "sc", "":
		host, port, err := splitHostPortWithDefault(hostPort, defaultTCPPort)
		if err != nil {
			return nil, err
		}

		address := net.JoinHostPort(host, port)
		dial := func() (net.Conn, error) {
			return net.DialTimeout("tcp", address, 1*time.Second)
		}

		return &dialConfig{
			display: address,
			raw:     address,
			dial:    dial,
		}, nil

	case "ssh":
		host, port, err := splitHostPortWithDefault(hostPort, defaultSSHPort)
		if err != nil {
			return nil, err
		}

		if user == "" {
			user = defaultSSHUser()
		}

		verifier := newHostKeyVerifier(host, port)
		address := net.JoinHostPort(host, port)

		dial := func() (net.Conn, error) {
			return dialSSH(user, host, port, verifier)
		}

		display := fmt.Sprintf("ssh://%s", address)
		rawAddr := address
		if user != "" {
			display = fmt.Sprintf("ssh://%s@%s", user, address)
			rawAddr = fmt.Sprintf("%s@%s", user, address) // Preserve user@ for SSH
		}

		return &dialConfig{
			display: display,
			raw:     rawAddr,
			dial:    dial,
			warning: verifier.warning,
		}, nil

	case "ws", "wss":
		// WebSocket connections (ws:// or wss://)
		host, port, err := splitHostPortWithDefault(hostPort, defaultHTTPPort)
		if err != nil {
			return nil, err
		}

		address := net.JoinHostPort(host, port)
		useTLS := scheme == "wss"

		dial := func() (net.Conn, error) {
			return DialWebSocket(address, useTLS)
		}

		displayScheme := "ws"
		if useTLS {
			displayScheme = "wss"
		}

		return &dialConfig{
			display: fmt.Sprintf("%s://%s", displayScheme, address),
			raw:     address,
			dial:    dial,
			warning: "",
		}, nil

	default:
		return nil, fmt.Errorf("unsupported server scheme %q", scheme)
	}
}

func splitHostPortWithDefault(hostPort, defaultPort string) (string, string, error) {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return "", "", errors.New("missing host in server address")
	}

	host, port, err := net.SplitHostPort(hostPort)
	if err == nil {
		return host, port, nil
	}

	var addrErr *net.AddrError
	if errors.As(err, &addrErr) && strings.Contains(strings.ToLower(addrErr.Err), "missing port") {
		host = hostPort
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
		}
		return host, defaultPort, nil
	}

	return "", "", err
}

func defaultSSHUser() string {
	if user := os.Getenv("SUPERCHAT_SSH_USER"); user != "" {
		return user
	}
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return "anonymous"
}

type hostKeyVerifier struct {
	host         string
	port         string
	paths        []string
	callbacks    []ssh.HostKeyCallback
	acceptedFP   map[string]string
	acceptedKeys map[string]ssh.PublicKey
	warning      string
}

var errUserRejectedHostKey = errors.New("user rejected ssh host key")

func newHostKeyVerifier(host, port string) *hostKeyVerifier {
	paths := knownHostPaths()
	var callbacks []ssh.HostKeyCallback
	for _, path := range paths {
		if cb, err := knownhosts.New(path); err == nil {
			callbacks = append(callbacks, cb)
		}
	}

	warning := ""
	if len(callbacks) == 0 {
		warning = "SSH host key verification is disabled (known_hosts not found); connection is vulnerable to MITM attacks"
	}

	return &hostKeyVerifier{
		host:         host,
		port:         port,
		paths:        paths,
		callbacks:    callbacks,
		acceptedFP:   make(map[string]string),
		acceptedKeys: make(map[string]ssh.PublicKey),
		warning:      warning,
	}
}

func (v *hostKeyVerifier) callback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	if len(v.callbacks) == 0 {
		return v.handleUnknownHostKey(hostname, remote, key)
	}

	var lastErr error
	for _, cb := range v.callbacks {
		if err := cb(hostname, remote, key); err != nil {
			lastErr = err
			continue
		}
		return nil
	}

	var keyErr *knownhosts.KeyError
	if errors.As(lastErr, &keyErr) {
		if len(keyErr.Want) == 0 {
			return v.handleUnknownHostKey(hostname, remote, key)
		}
		return v.handleMismatchedHostKey(hostname, keyErr, key)
	}

	return lastErr
}

func (v *hostKeyVerifier) handleUnknownHostKey(hostname string, remote net.Addr, key ssh.PublicKey) error {
	fingerprint := ssh.FingerprintSHA256(key)
	if acceptedFP, ok := v.acceptedFP[hostname]; ok && acceptedFP == fingerprint {
		return nil
	}

	if !isInteractive() {
		return fmt.Errorf("ssh host key verification failed for %s: key %s is not trusted and interactive approval is not possible. Add it with `ssh-keyscan -p %s %s >> %s` and retry", hostname, fingerprint, v.port, v.host, v.preferredKnownHostsPath())
	}

	accepted, err := promptAcceptHostKey(hostname, remote, fingerprint, key, v.paths)
	if err != nil {
		return err
	}
	if !accepted {
		return errUserRejectedHostKey
	}

	v.acceptedFP[hostname] = fingerprint
	v.acceptedKeys[hostname] = key
	return nil
}

func (v *hostKeyVerifier) handleMismatchedHostKey(hostname string, keyErr *knownhosts.KeyError, presented ssh.PublicKey) error {
	actual := fingerprintForKey(presented)
	expected := "unknown"
	if len(keyErr.Want) > 0 && keyErr.Want[0].Key != nil {
		expected = ssh.FingerprintSHA256(keyErr.Want[0].Key)
	}

	return fmt.Errorf("ssh host key verification failed for %s: the server presented key %s but an existing known_hosts entry expects %s (%s). This could indicate a man-in-the-middle attack. Update or remove the known_hosts entry before retrying", hostname, actual, expected, v.describeSources())
}

func (v *hostKeyVerifier) describeSources() string {
	if len(v.paths) == 0 {
		return "no known_hosts files were found"
	}
	return fmt.Sprintf("checked known_hosts files: %s", strings.Join(v.paths, ", "))
}

func (v *hostKeyVerifier) preferredKnownHostsPath() string {
	if len(v.paths) > 0 {
		return v.paths[0]
	}
	return filepath.Join(userHomeDir(), ".ssh", "known_hosts")
}

func (v *hostKeyVerifier) wrapError(err error) error {
	if errors.Is(err, errUserRejectedHostKey) {
		return fmt.Errorf("connection aborted: rejected SSH host key for %s", net.JoinHostPort(v.host, v.port))
	}

	var keyErr *knownhosts.KeyError
	if errors.As(err, &keyErr) {
		if len(keyErr.Want) == 0 {
			return fmt.Errorf("ssh host key verification failed for %s:%s: the key is not trusted and was not accepted", v.host, v.port)
		}
		return v.handleMismatchedHostKey(net.JoinHostPort(v.host, v.port), keyErr, nil)
	}

	if strings.Contains(err.Error(), "unable to authenticate") {
		return fmt.Errorf("ssh authentication failed for %s:%s: the remote server requires credentials. SuperChat's SSH endpoint does not use passwords or SSH-agent auth, so double-check the address (expected banner prefix %q).", v.host, v.port, superChatSSHVersionPrefix)
	}

	return err
}

func (v *hostKeyVerifier) persistAccepted(serverVersion string) {
	if len(v.acceptedKeys) == 0 {
		return
	}

	if len(v.paths) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: accepted SSH host key for %s:%s but no known_hosts path is writable; it will need to be trusted again next time\n", v.host, v.port)
		return
	}

	for host, key := range v.acceptedKeys {
		saved := false
		for _, path := range v.paths {
			if err := appendKnownHost(path, host, serverVersion, key); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to persist SSH host key for %s in %s: %v\n", host, path, err)
				continue
			}
			saved = true
			break
		}
		if !saved {
			fmt.Fprintf(os.Stderr, "Warning: could not persist SSH host key for %s; it will need to be trusted again next time\n", host)
		}
	}

	// Clear accepted cache to avoid duplicate writes on reconnection attempts
	v.acceptedKeys = make(map[string]ssh.PublicKey)
	v.acceptedFP = make(map[string]string)
}

func knownHostPaths() []string {
	if env := os.Getenv("SSH_KNOWN_HOSTS"); env != "" {
		split := strings.Split(env, string(os.PathListSeparator))
		var paths []string
		for _, p := range split {
			p = strings.TrimSpace(p)
			if p != "" {
				paths = append(paths, p)
			}
		}
		return paths
	}

	home := userHomeDir()
	if home == "" {
		return nil
	}

	return []string{filepath.Join(home, ".ssh", "known_hosts")}
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func dialSSH(user, host, port string, verifier *hostKeyVerifier) (net.Conn, error) {
	address := net.JoinHostPort(host, port)
	netConn, err := net.DialTimeout("tcp", address, 1*time.Second)
	if err != nil {
		return nil, err
	}

	// Load SSH keys for authentication
	authMethods, err := loadSSHAuthMethods()
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("failed to load SSH keys: %w", err)
	}

	if len(authMethods) == 0 {
		netConn.Close()
		return nil, errors.New("no SSH keys found - add your key to ssh-agent with: ssh-add ~/.ssh/id_rsa\nOr generate a new unencrypted key with: ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -N ''")
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: verifier.callback,
		Timeout:         1 * time.Second,
	}

	// Set a deadline for the SSH handshake to enforce the timeout
	if err := netConn.SetDeadline(time.Now().Add(1 * time.Second)); err != nil {
		netConn.Close()
		return nil, fmt.Errorf("failed to set connection deadline: %w", err)
	}

	localAddr := netConn.LocalAddr()
	remoteAddr := netConn.RemoteAddr()

	clientConn, chans, reqs, err := ssh.NewClientConn(netConn, address, config)
	if err != nil {
		netConn.Close()
		return nil, verifier.wrapError(err)
	}

	// Clear the deadline after handshake completes
	if err := netConn.SetDeadline(time.Time{}); err != nil {
		clientConn.Close()
		return nil, fmt.Errorf("failed to clear connection deadline: %w", err)
	}

	serverBanner := string(clientConn.ServerVersion())
	if !strings.HasPrefix(serverBanner, superChatSSHVersionPrefix) {
		clientConn.Close()
		return nil, fmt.Errorf("ssh handshake completed but remote server advertised %q; expected a SuperChat server (banner prefix %q)", serverBanner, superChatSSHVersionPrefix)
	}

	verifier.persistAccepted(serverBanner)

	client := ssh.NewClient(clientConn, chans, reqs)
	channel, requests, err := client.OpenChannel("session", nil)
	if err != nil {
		client.Close()
		return nil, verifier.wrapError(err)
	}

	go ssh.DiscardRequests(requests)

	return &sshClientConn{
		channel:    channel,
		client:     client,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}, nil
}

// loadSSHAuthMethods loads SSH private keys from SSH agent and standard locations
func loadSSHAuthMethods() ([]ssh.AuthMethod, error) {
	var authMethods []ssh.AuthMethod

	// Try SSH agent first (handles encrypted keys automatically)
	if agentAuth := trySSHAgent(); agentAuth != nil {
		authMethods = append(authMethods, agentAuth)
	}

	// Also try loading unencrypted keys from disk as fallback
	if diskAuth := tryLoadKeysFromDisk(); diskAuth != nil {
		authMethods = append(authMethods, diskAuth)
	}

	return authMethods, nil
}

// trySSHAgent attempts to connect to SSH agent and use its keys
func trySSHAgent() ssh.AuthMethod {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil
	}

	agentClient := agent.NewClient(conn)
	return ssh.PublicKeysCallback(agentClient.Signers)
}

// tryLoadKeysFromDisk loads unencrypted SSH keys from standard locations
func tryLoadKeysFromDisk() ssh.AuthMethod {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return nil
	}

	sshDir := filepath.Join(homeDir, ".ssh")

	// Try common key file names in order of preference
	keyFiles := []string{
		"id_ed25519", // Modern default
		"id_ecdsa",   // Modern ECDSA
		"id_rsa",     // Traditional RSA
		"id_dsa",     // Legacy DSA (still check for compatibility)
	}

	var signers []ssh.Signer

	for _, keyFile := range keyFiles {
		keyPath := filepath.Join(sshDir, keyFile)

		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			continue
		}

		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			// Skip encrypted keys - user should use SSH agent for those
			continue
		}

		signers = append(signers, signer)
	}

	if len(signers) == 0 {
		return nil
	}

	return ssh.PublicKeys(signers...)
}

func promptAcceptHostKey(hostname string, remote net.Addr, fingerprint string, key ssh.PublicKey, paths []string) (bool, error) {
	fmt.Printf("\nThe authenticity of host '%s' (%s) can't be established.\n", hostname, remoteString(remote))
	fmt.Printf("SSH key fingerprint is %s.\n", fingerprint)
	if len(paths) > 0 {
		fmt.Printf("If you accept, the key will be written to %s once the connection is verified.\n", paths[0])
	} else {
		fmt.Println("No writable known_hosts file detected; acceptance will apply to this session only.")
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Do you trust this host? (yes/no) [no]: ")
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "yes" && answer != "y" {
		return false, nil
	}

	return true, nil
}

func appendKnownHost(path, hostname, serverVersion string, key ssh.PublicKey) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line := knownhosts.Line([]string{hostname}, key)
	comment := fmt.Sprintf("SuperChat server banner=%s added=%s", serverVersion, time.Now().Format(time.RFC3339))
	if comment != "" {
		line = fmt.Sprintf("%s %s", line, comment)
	}
	if _, err := fmt.Fprintln(f, line); err != nil {
		return err
	}
	return nil
}

func remoteString(remote net.Addr) string {
	if remote == nil {
		return "unknown"
	}
	return remote.String()
}

func isInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func fingerprintForKey(key ssh.PublicKey) string {
	if key == nil {
		return "unknown"
	}
	return ssh.FingerprintSHA256(key)
}

type sshClientConn struct {
	channel    ssh.Channel
	client     *ssh.Client
	localAddr  net.Addr
	remoteAddr net.Addr
	once       sync.Once
}

func (c *sshClientConn) Read(b []byte) (int, error) {
	return c.channel.Read(b)
}

func (c *sshClientConn) Write(b []byte) (int, error) {
	return c.channel.Write(b)
}

func (c *sshClientConn) Close() error {
	var err error
	c.once.Do(func() {
		if closeErr := c.channel.Close(); closeErr != nil && !errors.Is(closeErr, io.EOF) {
			err = closeErr
		}
		c.client.Close()
	})
	return err
}

func (c *sshClientConn) LocalAddr() net.Addr {
	if c.localAddr != nil {
		return c.localAddr
	}
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (c *sshClientConn) RemoteAddr() net.Addr {
	if c.remoteAddr != nil {
		return c.remoteAddr
	}
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (c *sshClientConn) SetDeadline(t time.Time) error      { return nil }
func (c *sshClientConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *sshClientConn) SetWriteDeadline(t time.Time) error { return nil }
