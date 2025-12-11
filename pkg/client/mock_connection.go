package client

import (
	"fmt"
	"sync"

	"github.com/aeolun/superchat/pkg/protocol"
)

// MockConnection is a test implementation of ConnectionInterface
type MockConnection struct {
	mu sync.RWMutex

	// State
	connected       bool
	address         string
	autoReconnect   bool
	connectErr      error
	sendErr         error
	sendMessageErr  error

	// Channels for communication
	incoming    chan *protocol.Frame
	errors      chan error
	stateChange chan ConnectionStateUpdate

	// Sent frames for verification
	SentFrames   []*protocol.Frame
	SentMessages []MockSentMessage
}

// MockSentMessage tracks messages sent via SendMessage
type MockSentMessage struct {
	Type uint8
	Msg  interface{}
}

// NewMockConnection creates a new mock connection
func NewMockConnection(address string) *MockConnection {
	return &MockConnection{
		connected:   false,
		address:     address,
		incoming:    make(chan *protocol.Frame, 100),
		errors:      make(chan error, 10),
		stateChange: make(chan ConnectionStateUpdate, 10),
		SentFrames:  make([]*protocol.Frame, 0),
		SentMessages: make([]MockSentMessage, 0),
	}
}

// Connect simulates connecting to the server
func (m *MockConnection) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connectErr != nil {
		return m.connectErr
	}

	m.connected = true
	return nil
}

// Disconnect simulates disconnecting from the server
func (m *MockConnection) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
}

// Close closes the mock connection
func (m *MockConnection) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	close(m.incoming)
	close(m.errors)
	close(m.stateChange)
}

// IsConnected returns the connection status
func (m *MockConnection) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// GetAddress returns the mock address
func (m *MockConnection) GetAddress() string {
	return m.address
}

// GetRawAddress returns the raw address without scheme
func (m *MockConnection) GetRawAddress() string {
	return m.address
}

// GetConnectionType returns the connection type (always "tcp" for mock)
func (m *MockConnection) GetConnectionType() string {
	return "tcp"
}

// Send sends a frame (records it for verification)
func (m *MockConnection) Send(frame *protocol.Frame) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendErr != nil {
		return m.sendErr
	}

	m.SentFrames = append(m.SentFrames, frame)
	return nil
}

// SendMessage sends a message (records it for verification)
func (m *MockConnection) SendMessage(msgType uint8, msg interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendMessageErr != nil {
		return m.sendMessageErr
	}

	m.SentMessages = append(m.SentMessages, MockSentMessage{
		Type: msgType,
		Msg:  msg,
	})
	return nil
}

// Incoming returns the incoming frame channel
func (m *MockConnection) Incoming() <-chan *protocol.Frame {
	return m.incoming
}

// Errors returns the error channel
func (m *MockConnection) Errors() <-chan error {
	return m.errors
}

// StateChanges returns the state change channel
func (m *MockConnection) StateChanges() <-chan ConnectionStateUpdate {
	return m.stateChange
}

// DisableAutoReconnect disables auto-reconnect
func (m *MockConnection) DisableAutoReconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoReconnect = false
}

// EnableAutoReconnect enables automatic reconnection
func (m *MockConnection) EnableAutoReconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoReconnect = true
}

// SetThrottle is a no-op for mock
func (m *MockConnection) SetThrottle(bytesPerSec int) {
	// No-op for mock
}

// GetBytesSent returns 0 for mock
func (m *MockConnection) GetBytesSent() uint64 {
	return 0
}

// GetBytesReceived returns 0 for mock
func (m *MockConnection) GetBytesReceived() uint64 {
	return 0
}

// Test helpers

// SetConnectError sets an error to return from Connect()
func (m *MockConnection) SetConnectError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectErr = err
}

// SetSendError sets an error to return from Send()
func (m *MockConnection) SetSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

// SetSendMessageError sets an error to return from SendMessage()
func (m *MockConnection) SetSendMessageError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendMessageErr = err
}

// SimulateIncomingFrame sends a frame to the incoming channel
func (m *MockConnection) SimulateIncomingFrame(frame *protocol.Frame) {
	m.incoming <- frame
}

// SimulateError sends an error to the errors channel
func (m *MockConnection) SimulateError(err error) {
	m.errors <- err
}

// SimulateStateChange sends a state change to the stateChange channel
func (m *MockConnection) SimulateStateChange(state ConnectionStateUpdate) {
	m.stateChange <- state
}

// GetSentMessageCount returns the number of messages sent
func (m *MockConnection) GetSentMessageCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.SentMessages)
}

// GetLastSentMessage returns the last message sent, or error if none
func (m *MockConnection) GetLastSentMessage() (MockSentMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.SentMessages) == 0 {
		return MockSentMessage{}, fmt.Errorf("no messages sent")
	}

	return m.SentMessages[len(m.SentMessages)-1], nil
}

// ClearSentMessages clears the sent messages list
func (m *MockConnection) ClearSentMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SentMessages = make([]MockSentMessage, 0)
	m.SentFrames = make([]*protocol.Frame, 0)
}
