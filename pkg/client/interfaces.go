package client

import (
	"github.com/aeolun/superchat/pkg/protocol"
)

// ConnectionInterface defines the interface for client connections
// This allows for mocking in tests while the real Connection implements all these methods
type ConnectionInterface interface {
	// Connection management
	Connect() error
	Disconnect()
	Close()
	IsConnected() bool
	GetAddress() string
	GetRawAddress() string

	// Message sending
	Send(frame *protocol.Frame) error
	SendMessage(msgType uint8, msg interface{}) error

	// Channels for receiving data
	Incoming() <-chan *protocol.Frame
	Errors() <-chan error
	StateChanges() <-chan ConnectionStateUpdate

	// Configuration
	DisableAutoReconnect()
	EnableAutoReconnect()
	SetThrottle(bytesPerSec int)

	// Traffic statistics
	GetBytesSent() uint64
	GetBytesReceived() uint64

	// Connection information
	GetConnectionType() string
}

// StateInterface defines the interface for client state persistence
// This allows for mocking in tests while the real State implements all these methods
type StateInterface interface {
	// Configuration
	GetConfig(key string) (string, error)
	SetConfig(key, value string) error

	// Nickname management
	GetLastNickname() string
	SetLastNickname(nickname string) error

	// Authentication (V2)
	GetUserID() *uint64
	SetUserID(userID *uint64) error

	// Read state tracking
	GetReadState(channelID uint64, subchannelID *uint64, threadID *uint64) (int64, error)
	UpdateReadState(channelID uint64, subchannelID *uint64, threadID *uint64, timestamp int64) error

	// First run tracking
	GetFirstRun() bool
	SetFirstRunComplete() error

	// First post warning tracking
	GetFirstPostWarningDismissed() bool
	SetFirstPostWarningDismissed() error

	// Connection history
	GetLastSuccessfulMethod(serverAddress string) (string, error)
	SaveSuccessfulConnection(serverAddress string, method string) error

	// Last seen timestamp (for anonymous user unread counts)
	GetLastSeenTimestamp() int64
	SetLastSeenTimestamp(timestamp int64) error
	UpdateLastSeenTimestamp() error

	// State directory
	GetStateDir() string

	// Close the state
	Close() error
}
