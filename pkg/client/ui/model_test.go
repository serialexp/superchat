package ui

import (
	"io"
	"log"
	"testing"

	"github.com/aeolun/superchat/pkg/client"
	"github.com/aeolun/superchat/pkg/protocol"
)

func TestNewModel(t *testing.T) {
	conn := client.NewMockConnection("localhost:6465")
	state := client.NewMockState()
	state.SetFirstRun(false) // Not first run
	logger := log.New(io.Discard, "", 0)

	m := NewModel(conn, state, "1.0.0", false, 0, logger, "", nil)

	if m.conn == nil {
		t.Error("NewModel() conn is nil")
	}

	if m.state == nil {
		t.Error("NewModel() state is nil")
	}

	if m.currentVersion != "1.0.0" {
		t.Errorf("NewModel() currentVersion = %q, want %q", m.currentVersion, "1.0.0")
	}

	if m.connectionState != StateConnected {
		t.Errorf("NewModel() connectionState = %v, want StateConnected", m.connectionState)
	}

	if m.currentView != ViewChannelList {
		t.Errorf("NewModel() currentView = %v, want ViewChannelList", m.currentView)
	}
}

func TestNewModelFirstRun(t *testing.T) {
	conn := client.NewMockConnection("localhost:6465")
	state := client.NewMockState()
	state.SetFirstRun(true)
	logger := log.New(io.Discard, "", 0)

	m := NewModel(conn, state, "1.0.0", false, 0, logger, "", nil)

	if !m.firstRun {
		t.Error("NewModel() firstRun = false, want true")
	}

	if m.currentView != ViewSplash {
		t.Errorf("NewModel() currentView = %v, want ViewSplash", m.currentView)
	}
}

func TestNewModelWithNickname(t *testing.T) {
	conn := client.NewMockConnection("localhost:6465")
	state := client.NewMockState()
	state.SetLastNickname("testuser")
	logger := log.New(io.Discard, "", 0)

	m := NewModel(conn, state, "1.0.0", false, 0, logger, "", nil)

	if m.nickname != "testuser" {
		t.Errorf("NewModel() nickname = %q, want %q", m.nickname, "testuser")
	}
}

func TestHandleServerConfig(t *testing.T) {
	m := NewTestModel()
	frame := CreateTestServerConfigFrame()

	updatedModel, cmd := m.handleServerFrame(frame)
	m = updatedModel.(Model)

	if m.serverConfig == nil {
		t.Fatal("handleServerConfig() did not set serverConfig")
	}

	if m.serverConfig.ProtocolVersion != protocol.ProtocolVersion {
		t.Errorf("serverConfig.ProtocolVersion = %d, want %d",
			m.serverConfig.ProtocolVersion, protocol.ProtocolVersion)
	}

	if m.statusMessage == "" {
		t.Error("handleServerConfig() did not set status message")
	}

	if cmd == nil {
		t.Error("handleServerConfig() returned nil cmd")
	}
}

func TestHandleChannelList(t *testing.T) {
	m := NewTestModel()

	channels := []protocol.Channel{
		CreateTestChannel(1, "general"),
		CreateTestChannel(2, "random"),
	}
	frame := CreateTestChannelListFrame(channels)

	updatedModel, cmd := m.handleServerFrame(frame)
	m = updatedModel.(Model)

	if len(m.channels) != 2 {
		t.Fatalf("handleChannelList() channels = %d, want 2", len(m.channels))
	}

	if m.channels[0].Name != "general" {
		t.Errorf("channels[0].Name = %q, want %q", m.channels[0].Name, "general")
	}

	if m.channels[1].Name != "random" {
		t.Errorf("channels[1].Name = %q, want %q", m.channels[1].Name, "random")
	}

	if cmd == nil {
		t.Error("handleChannelList() returned nil cmd")
	}
}

func TestHandleMessageList_RootMessages(t *testing.T) {
	m := NewTestModel()
	m = SetupTestModelWithDimensions(100, 30)

	messages := []protocol.Message{
		CreateTestMessage(1, 1, "alice", "First thread", nil),
		CreateTestMessage(2, 1, "bob", "Second thread", nil),
	}
	frame := CreateTestMessageListFrame(messages, nil) // nil parentID = root messages

	updatedModel, cmd := m.handleServerFrame(frame)
	m = updatedModel.(Model)

	if len(m.threads) != 2 {
		t.Fatalf("handleMessageList() threads = %d, want 2", len(m.threads))
	}

	if m.threads[0].Content != "First thread" {
		t.Errorf("threads[0].Content = %q, want %q", m.threads[0].Content, "First thread")
	}

	if cmd == nil {
		t.Error("handleMessageList() returned nil cmd")
	}
}

func TestHandleMessageList_Replies(t *testing.T) {
	m := NewTestModel()
	m = SetupTestModelWithDimensions(100, 30)

	rootID := uint64(1)
	m.currentThread = &protocol.Message{
		ID:             rootID,
		ChannelID:      1,
		AuthorNickname: "alice",
		Content:        "Root message",
	}

	reply1ID := rootID
	messages := []protocol.Message{
		CreateTestMessage(2, 1, "bob", "Reply 1", &reply1ID),
		CreateTestMessage(3, 1, "charlie", "Reply 2", &reply1ID),
	}
	frame := CreateTestMessageListFrame(messages, &rootID)

	updatedModel, cmd := m.handleServerFrame(frame)
	m = updatedModel.(Model)

	if len(m.threadReplies) != 2 {
		t.Fatalf("handleMessageList() threadReplies = %d, want 2", len(m.threadReplies))
	}

	if cmd == nil {
		t.Error("handleMessageList() returned nil cmd")
	}
}

func TestHandleNewMessage_RootMessage(t *testing.T) {
	m := NewTestModel()
	m = SetupTestModelWithDimensions(100, 30)

	channel := CreateTestChannel(1, "general")
	m.currentChannel = &channel
	m.threads = []protocol.Message{}

	newMsg := CreateTestMessage(100, 1, "alice", "New thread!", nil)
	frame := CreateTestNewMessageFrame(newMsg)

	updatedModel, cmd := m.handleServerFrame(frame)
	m = updatedModel.(Model)

	if len(m.threads) != 1 {
		t.Fatalf("handleNewMessage() threads = %d, want 1", len(m.threads))
	}

	if m.threads[0].ID != 100 {
		t.Errorf("threads[0].ID = %d, want 100", m.threads[0].ID)
	}

	if cmd == nil {
		t.Error("handleNewMessage() returned nil cmd")
	}
}

func TestHandleNewMessage_Reply(t *testing.T) {
	m := NewTestModel()
	m = SetupTestModelWithDimensions(100, 30)

	channel := CreateTestChannel(1, "general")
	m.currentChannel = &channel

	rootID := uint64(1)
	m.currentThread = &protocol.Message{
		ID:             rootID,
		ChannelID:      1,
		AuthorNickname: "alice",
		Content:        "Root message",
	}
	m.threadReplies = []protocol.Message{}

	newMsg := CreateTestMessage(100, 1, "bob", "A reply!", &rootID)
	frame := CreateTestNewMessageFrame(newMsg)

	updatedModel, cmd := m.handleServerFrame(frame)
	m = updatedModel.(Model)

	if len(m.threadReplies) != 1 {
		t.Fatalf("handleNewMessage() threadReplies = %d, want 1", len(m.threadReplies))
	}

	if m.threadReplies[0].ID != 100 {
		t.Errorf("threadReplies[0].ID = %d, want 100", m.threadReplies[0].ID)
	}

	if cmd == nil {
		t.Error("handleNewMessage() returned nil cmd")
	}
}

func TestHandleError(t *testing.T) {
	m := NewTestModel()

	frame := CreateTestErrorFrame(1001, "Not implemented")

	updatedModel, cmd := m.handleServerFrame(frame)
	m = updatedModel.(Model)

	if m.errorMessage == "" {
		t.Error("handleError() did not set error message")
	}

	expectedMsg := "Error 1001: Not implemented"
	if m.errorMessage != expectedMsg {
		t.Errorf("errorMessage = %q, want %q", m.errorMessage, expectedMsg)
	}

	if cmd == nil {
		t.Error("handleError() returned nil cmd")
	}
}

func TestSelectedMessage_Root(t *testing.T) {
	m := NewTestModel()

	rootMsg := CreateTestMessage(1, 1, "alice", "Root message", nil)
	m.currentThread = &rootMsg
	m.replyCursor = 0 // Root selected

	msg, ok := m.selectedMessage()
	if !ok {
		t.Fatal("selectedMessage() returned false, want true")
	}

	if msg.ID != 1 {
		t.Errorf("selectedMessage().ID = %d, want 1", msg.ID)
	}
}

func TestSelectedMessage_Reply(t *testing.T) {
	m := NewTestModel()

	rootID := uint64(1)
	rootMsg := CreateTestMessage(1, 1, "alice", "Root message", nil)
	m.currentThread = &rootMsg

	reply1 := CreateTestMessage(2, 1, "bob", "Reply 1", &rootID)
	reply2 := CreateTestMessage(3, 1, "charlie", "Reply 2", &rootID)
	m.threadReplies = []protocol.Message{reply1, reply2}
	m.replyCursor = 2 // Second reply (index 1)

	msg, ok := m.selectedMessage()
	if !ok {
		t.Fatal("selectedMessage() returned false, want true")
	}

	if msg.ID != 3 {
		t.Errorf("selectedMessage().ID = %d, want 3", msg.ID)
	}
}

func TestSelectedMessage_NoThread(t *testing.T) {
	m := NewTestModel()
	m.currentThread = nil

	_, ok := m.selectedMessage()
	if ok {
		t.Error("selectedMessage() returned true when currentThread is nil, want false")
	}
}

func TestSelectedMessage_OutOfBounds(t *testing.T) {
	m := NewTestModel()

	rootMsg := CreateTestMessage(1, 1, "alice", "Root message", nil)
	m.currentThread = &rootMsg
	m.threadReplies = []protocol.Message{}
	m.replyCursor = 5 // Out of bounds

	_, ok := m.selectedMessage()
	if ok {
		t.Error("selectedMessage() returned true when cursor is out of bounds, want false")
	}
}

func TestApplyMessageDeletion(t *testing.T) {
	m := NewTestModel()

	rootID := uint64(1)
	rootMsg := CreateTestMessage(1, 1, "alice", "Root message", nil)
	m.currentThread = &rootMsg

	reply1 := CreateTestMessage(2, 1, "bob", "Reply 1", &rootID)
	m.threadReplies = []protocol.Message{reply1}
	m.threads = []protocol.Message{rootMsg}

	// Delete the reply
	m.applyMessageDeletion(2, "[deleted by author]")

	if m.threadReplies[0].Content != "[deleted by author]" {
		t.Errorf("threadReplies[0].Content = %q, want %q",
			m.threadReplies[0].Content, "[deleted by author]")
	}
}

func TestCalculateThreadDepths(t *testing.T) {
	m := NewTestModel()

	rootID := uint64(1)
	rootMsg := CreateTestMessage(1, 1, "alice", "Root", nil)
	m.currentThread = &rootMsg

	msg2ID := uint64(2)
	msg3ID := uint64(3)

	m.threadReplies = []protocol.Message{
		CreateTestMessage(2, 1, "bob", "Reply to root", &rootID),
		CreateTestMessage(3, 1, "charlie", "Another reply to root", &rootID),
		CreateTestMessage(4, 1, "dave", "Reply to msg 2", &msg2ID),
		CreateTestMessage(5, 1, "eve", "Reply to msg 3", &msg3ID),
		CreateTestMessage(6, 1, "frank", "Another reply to msg 2", &msg2ID),
	}

	depths := client.CalculateThreadDepths(m.currentThread.ID, m.threadReplies)

	expectedDepths := map[uint64]int{
		1: 0, // Root
		2: 1, // Reply to root
		3: 1, // Reply to root
		4: 2, // Reply to msg 2
		5: 2, // Reply to msg 3
		6: 2, // Reply to msg 2
	}

	for id, expectedDepth := range expectedDepths {
		if depths[id] != expectedDepth {
			t.Errorf("depth[%d] = %d, want %d", id, depths[id], expectedDepth)
		}
	}
}
