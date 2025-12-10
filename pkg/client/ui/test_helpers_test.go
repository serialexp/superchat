package ui

import (
	"io"
	"log"
	"time"

	"github.com/aeolun/superchat/pkg/client"
	"github.com/aeolun/superchat/pkg/protocol"
	"github.com/charmbracelet/bubbles/viewport"
)

// NewTestModel creates a Model with mock dependencies for testing
func NewTestModel() Model {
	conn := client.NewMockConnection("localhost:6465")
	conn.Connect()
	state := client.NewMockState()
	logger := log.New(io.Discard, "", 0) // Discard logs in tests

	return NewModel(conn, state, "0.0.0-test", false, 0, logger, "", nil)
}

// NewTestModelWithMocks creates a Model with provided mocks
func NewTestModelWithMocks(conn client.ConnectionInterface, state client.StateInterface) Model {
	logger := log.New(io.Discard, "", 0) // Discard logs in tests
	return NewModel(conn, state, "0.0.0-test", false, 0, logger, "", nil)
}

// SetupTestModelWithDimensions creates a test model with window dimensions set
func SetupTestModelWithDimensions(width, height int) Model {
	m := NewTestModel()
	m.width = width
	m.height = height

	// Initialize viewports
	m.threadViewport = viewport.New(width-2, height-6)
	m.threadListViewport = viewport.New(width-width/4-1-4, height-6)

	return m
}

// CreateTestChannel creates a test channel
func CreateTestChannel(id uint64, name string) protocol.Channel {
	return protocol.Channel{
		ID:          id,
		Name:        name,
		Description: "Test channel",
		UserCount:   0,
		IsOperator:  false,
		Type:        0,
		RetentionHours: 168,
	}
}

// CreateTestMessage creates a test message
func CreateTestMessage(id uint64, channelID uint64, author string, content string, parentID *uint64) protocol.Message {
	return protocol.Message{
		ID:             id,
		ChannelID:      channelID,
		SubchannelID:   nil,
		ParentID:       parentID,
		AuthorUserID:   nil,
		AuthorNickname: author,
		Content:        content,
		CreatedAt:      time.Now(),
		EditedAt:       nil,
		ReplyCount:     0,
	}
}

// CreateTestServerConfigFrame creates a SERVER_CONFIG frame
func CreateTestServerConfigFrame() *protocol.Frame {
	msg := &protocol.ServerConfigMessage{
		ProtocolVersion:         protocol.ProtocolVersion,
		MaxMessageRate:          10,
		MaxChannelCreates:       5,
		InactiveCleanupDays:     30,
		MaxConnectionsPerIP:     10,
		MaxMessageLength:        1024000,
		MaxThreadSubscriptions:  100,
		MaxChannelSubscriptions: 50,
	}

	payload, _ := msg.Encode()
	return &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeServerConfig,
		Flags:   0,
		Payload: payload,
	}
}

// CreateTestChannelListFrame creates a CHANNEL_LIST frame
func CreateTestChannelListFrame(channels []protocol.Channel) *protocol.Frame {
	msg := &protocol.ChannelListMessage{
		Channels: channels,
	}

	payload, _ := msg.Encode()
	return &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeChannelList,
		Flags:   0,
		Payload: payload,
	}
}

// CreateTestMessageListFrame creates a MESSAGE_LIST frame
func CreateTestMessageListFrame(messages []protocol.Message, parentID *uint64) *protocol.Frame {
	msg := &protocol.MessageListMessage{
		ParentID: parentID,
		Messages: messages,
	}

	payload, _ := msg.Encode()
	return &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeMessageList,
		Flags:   0,
		Payload: payload,
	}
}

// CreateTestNewMessageFrame creates a NEW_MESSAGE frame
func CreateTestNewMessageFrame(message protocol.Message) *protocol.Frame {
	msg := protocol.NewMessageMessage(message)

	payload, _ := msg.Encode()
	return &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeNewMessage,
		Flags:   0,
		Payload: payload,
	}
}

// CreateTestErrorFrame creates an ERROR frame
func CreateTestErrorFrame(code uint16, message string) *protocol.Frame {
	msg := &protocol.ErrorMessage{
		ErrorCode: code,
		Message:   message,
	}

	payload, _ := msg.Encode()
	return &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeError,
		Flags:   0,
		Payload: payload,
	}
}

// GetMockConnection extracts the mock connection from a model (for assertions)
func GetMockConnection(m Model) *client.MockConnection {
	if mock, ok := m.conn.(*client.MockConnection); ok {
		return mock
	}
	return nil
}

// GetMockState extracts the mock state from a model (for assertions)
func GetMockState(m Model) *client.MockState {
	if mock, ok := m.state.(*client.MockState); ok {
		return mock
	}
	return nil
}
