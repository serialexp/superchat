package archiver

import (
	"os"
	"path/filepath"
	"testing"

	gen "github.com/aeolun/superchat/pkg/archive/generated"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// createTestServer is a helper that creates a server and returns its ID.
func createTestServer(t *testing.T, store *Store, name string) int64 {
	t.Helper()
	id, err := store.GetOrCreateServer(name)
	if err != nil {
		t.Fatalf("GetOrCreateServer(%q): %v", name, err)
	}
	return id
}

func TestStoreGetOrCreateServer(t *testing.T) {
	store := newTestStore(t)

	id1, err := store.GetOrCreateServer("Server A")
	if err != nil {
		t.Fatal(err)
	}
	if id1 == 0 {
		t.Fatal("expected non-zero server ID")
	}

	// Same name returns same ID
	id2, err := store.GetOrCreateServer("Server A")
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id1 {
		t.Errorf("same name returned different IDs: %d vs %d", id1, id2)
	}

	// Different name returns different ID
	id3, err := store.GetOrCreateServer("Server B")
	if err != nil {
		t.Fatal(err)
	}
	if id3 == id1 {
		t.Error("different name returned same ID")
	}
}

func TestStoreUpsertChannel(t *testing.T) {
	store := newTestStore(t)
	srvID := createTestServer(t, store, "TestServer")

	err := store.UpsertChannel(srvID, &gen.ChannelInfo{
		ChannelId: 1, Name: "general", Description: "General chat",
		ChannelType: 0, RetentionHours: 168, ArchiveEnabled: 1,
	})
	if err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}

	channels, err := store.GetChannelsByServer(srvID)
	if err != nil {
		t.Fatalf("GetChannelsByServer: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("len(channels) = %d, want 1", len(channels))
	}
	if channels[0].Name != "general" {
		t.Errorf("Name = %q, want %q", channels[0].Name, "general")
	}

	// Upsert same channel with updated name
	err = store.UpsertChannel(srvID, &gen.ChannelInfo{
		ChannelId: 1, Name: "general-v2", Description: "Updated",
		ChannelType: 0, RetentionHours: 168, ArchiveEnabled: 1,
	})
	if err != nil {
		t.Fatalf("UpsertChannel (update): %v", err)
	}

	channels, err = store.GetChannelsByServer(srvID)
	if err != nil {
		t.Fatalf("GetChannelsByServer after update: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("len(channels) = %d, want 1 after upsert", len(channels))
	}
	if channels[0].Name != "general-v2" {
		t.Errorf("Name after update = %q, want %q", channels[0].Name, "general-v2")
	}
}

func TestStoreUpsertMessage(t *testing.T) {
	store := newTestStore(t)
	srvID := createTestServer(t, store, "TestServer")

	// Create channel first
	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "test"}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}

	channels, _ := store.GetChannelsByServer(srvID)
	chID := channels[0].ID

	// Insert message
	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1,
		AuthorNickname: "alice", Content: "hello",
		CreatedAt: 1700000000000,
	}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}

	msgs, err := store.GetRootMessages(chID, 50, 0)
	if err != nil {
		t.Fatalf("GetRootMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "hello")
	}
	if msgs[0].AuthorNickname != "alice" {
		t.Errorf("AuthorNickname = %q, want %q", msgs[0].AuthorNickname, "alice")
	}
}

func TestStoreUpdateMessage(t *testing.T) {
	store := newTestStore(t)
	srvID := createTestServer(t, store, "TestServer")

	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "test"}); err != nil {
		t.Fatal(err)
	}
	channels, _ := store.GetChannelsByServer(srvID)
	chID := channels[0].ID

	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1, AuthorNickname: "alice",
		Content: "original", CreatedAt: 1700000000000,
	}); err != nil {
		t.Fatal(err)
	}

	// Edit message
	if err := store.UpdateMessage(srvID, &gen.MessageEdited{
		MessageId: 100, ChannelId: 1, NewContent: "edited", EditedAt: 1700000001000,
	}); err != nil {
		t.Fatal(err)
	}

	msgs, err := store.GetRootMessages(chID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if msgs[0].Content != "edited" {
		t.Errorf("Content after edit = %q, want %q", msgs[0].Content, "edited")
	}
	if msgs[0].EditedAt == nil || *msgs[0].EditedAt != 1700000001000 {
		t.Errorf("EditedAt = %v, want 1700000001000", msgs[0].EditedAt)
	}
}

func TestStoreDeleteMessage(t *testing.T) {
	store := newTestStore(t)
	srvID := createTestServer(t, store, "TestServer")

	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "test"}); err != nil {
		t.Fatal(err)
	}
	channels, _ := store.GetChannelsByServer(srvID)
	chID := channels[0].ID

	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1, AuthorNickname: "alice",
		Content: "to-be-deleted", CreatedAt: 1700000000000,
	}); err != nil {
		t.Fatal(err)
	}

	// Delete message
	if err := store.DeleteMessage(srvID, &gen.MessageDeleted{
		MessageId: 100, ChannelId: 1, Content: "[deleted by ~alice]", DeletedAt: 1700000002000,
	}); err != nil {
		t.Fatal(err)
	}

	msgs, err := store.GetRootMessages(chID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if msgs[0].DeletedAt == nil || *msgs[0].DeletedAt != 1700000002000 {
		t.Errorf("DeletedAt = %v, want 1700000002000", msgs[0].DeletedAt)
	}
	if msgs[0].Content != "[deleted by ~alice]" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "[deleted by ~alice]")
	}
}

func TestStoreChannelStates(t *testing.T) {
	store := newTestStore(t)
	srvID := createTestServer(t, store, "TestServer")

	// Create channels with messages
	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "ch1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 2, Name: "ch2"}); err != nil {
		t.Fatal(err)
	}

	// Insert messages
	for _, msg := range []*gen.MessageSync{
		{MessageId: 10, ChannelId: 1, AuthorNickname: "a", Content: "1", CreatedAt: 1},
		{MessageId: 20, ChannelId: 1, AuthorNickname: "a", Content: "2", CreatedAt: 2},
		{MessageId: 30, ChannelId: 2, AuthorNickname: "a", Content: "3", CreatedAt: 3},
	} {
		if err := store.UpsertMessage(srvID, msg); err != nil {
			t.Fatal(err)
		}
	}

	states, err := store.GetChannelStates(srvID)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("len(states) = %d, want 2", len(states))
	}

	// Build map for easier assertion (these should be remote IDs)
	stateMap := make(map[int64]int64)
	for _, s := range states {
		stateMap[s.ChannelID] = s.LastMessageID
	}

	if stateMap[1] != 20 {
		t.Errorf("channel 1 last_message_id = %d, want 20", stateMap[1])
	}
	if stateMap[2] != 30 {
		t.Errorf("channel 2 last_message_id = %d, want 30", stateMap[2])
	}
}

func TestStoreGetThreadMessages(t *testing.T) {
	store := newTestStore(t)
	srvID := createTestServer(t, store, "TestServer")

	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "test"}); err != nil {
		t.Fatal(err)
	}

	threadRoot := uint64(100)
	// Root message
	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1, AuthorNickname: "alice",
		Content: "root", CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	// Reply (parent_id and thread_root_id reference remote IDs)
	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 101, ChannelId: 1, ParentId: &threadRoot, ThreadRootId: &threadRoot,
		AuthorNickname: "bob", Content: "reply", CreatedAt: 2000,
	}); err != nil {
		t.Fatal(err)
	}

	// Get the local ID of the root message for thread lookup
	channels, _ := store.GetChannelsByServer(srvID)
	rootMsgs, _ := store.GetRootMessages(channels[0].ID, 50, 0)
	rootLocalID := rootMsgs[0].ID

	msgs, err := store.GetThreadMessages(rootLocalID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(thread msgs) = %d, want 2", len(msgs))
	}
	// Should be ordered by created_at ASC with correct depth
	if msgs[0].Content != "root" {
		t.Errorf("first message content = %q, want %q", msgs[0].Content, "root")
	}
	if msgs[0].Depth != 0 {
		t.Errorf("root message depth = %d, want 0", msgs[0].Depth)
	}
	if msgs[1].Content != "reply" {
		t.Errorf("second message content = %q, want %q", msgs[1].Content, "reply")
	}
	if msgs[1].Depth != 1 {
		t.Errorf("reply message depth = %d, want 1", msgs[1].Depth)
	}
}

func TestStoreGetMessageCount(t *testing.T) {
	store := newTestStore(t)
	srvID := createTestServer(t, store, "TestServer")

	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "test"}); err != nil {
		t.Fatal(err)
	}
	channels, _ := store.GetChannelsByServer(srvID)
	chID := channels[0].ID

	count, err := store.GetMessageCount(chID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	for i := range 5 {
		if err := store.UpsertMessage(srvID, &gen.MessageSync{
			MessageId: uint64(i + 1), ChannelId: 1,
			AuthorNickname: "a", Content: "msg", CreatedAt: int64(i * 1000),
		}); err != nil {
			t.Fatal(err)
		}
	}

	count, err = store.GetMessageCount(chID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

func TestStoreMultiServerIsolation(t *testing.T) {
	store := newTestStore(t)
	srvA := createTestServer(t, store, "Server A")
	srvB := createTestServer(t, store, "Server B")

	// Both servers have channel with remote ID 1
	if err := store.UpsertChannel(srvA, &gen.ChannelInfo{ChannelId: 1, Name: "general", Description: "A's general"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertChannel(srvB, &gen.ChannelInfo{ChannelId: 1, Name: "lobby", Description: "B's lobby"}); err != nil {
		t.Fatal(err)
	}

	// Channels should be isolated
	chA, err := store.GetChannelsByServer(srvA)
	if err != nil {
		t.Fatal(err)
	}
	if len(chA) != 1 || chA[0].Name != "general" {
		t.Errorf("Server A channels: got %v, want [general]", chA)
	}

	chB, err := store.GetChannelsByServer(srvB)
	if err != nil {
		t.Fatal(err)
	}
	if len(chB) != 1 || chB[0].Name != "lobby" {
		t.Errorf("Server B channels: got %v, want [lobby]", chB)
	}

	// Both servers have message with remote ID 1
	if err := store.UpsertMessage(srvA, &gen.MessageSync{
		MessageId: 1, ChannelId: 1, AuthorNickname: "alice", Content: "from A", CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMessage(srvB, &gen.MessageSync{
		MessageId: 1, ChannelId: 1, AuthorNickname: "bob", Content: "from B", CreatedAt: 2000,
	}); err != nil {
		t.Fatal(err)
	}

	// Messages should be isolated
	msgsA, _ := store.GetRootMessages(chA[0].ID, 50, 0)
	if len(msgsA) != 1 || msgsA[0].Content != "from A" {
		t.Errorf("Server A messages: got %v", msgsA)
	}

	msgsB, _ := store.GetRootMessages(chB[0].ID, 50, 0)
	if len(msgsB) != 1 || msgsB[0].Content != "from B" {
		t.Errorf("Server B messages: got %v", msgsB)
	}

	// Channel states should be isolated
	statesA, _ := store.GetChannelStates(srvA)
	statesB, _ := store.GetChannelStates(srvB)

	if len(statesA) != 1 || statesA[0].ChannelID != 1 || statesA[0].LastMessageID != 1 {
		t.Errorf("Server A states: %v", statesA)
	}
	if len(statesB) != 1 || statesB[0].ChannelID != 1 || statesB[0].LastMessageID != 1 {
		t.Errorf("Server B states: %v", statesB)
	}
}

func TestHTMLGeneratorCreatesDirs(t *testing.T) {
	store := newTestStore(t)
	srvID := createTestServer(t, store, "Test Server")
	outputDir := filepath.Join(t.TempDir(), "html-output")

	htmlGen := NewHTMLGenerator(outputDir, store)

	// Create a channel with some messages
	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "test-channel", Description: "Test"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 1, ChannelId: 1, AuthorNickname: "alice",
		Content: "hello world", CreatedAt: 1700000000000,
	}); err != nil {
		t.Fatal(err)
	}

	htmlGen.GenerateAll()

	// Verify root index.html was created
	if _, err := os.Stat(filepath.Join(outputDir, "index.html")); os.IsNotExist(err) {
		t.Error("index.html not created")
	}

	// Verify server index was created
	serverSlug := Slugify("Test Server")
	if _, err := os.Stat(filepath.Join(outputDir, serverSlug, "index.html")); os.IsNotExist(err) {
		t.Errorf("%s/index.html not created", serverSlug)
	}

	// Verify channel dir was created
	channelDir := filepath.Join(outputDir, serverSlug, "channel", "test-channel")
	if _, err := os.Stat(filepath.Join(channelDir, "index.html")); os.IsNotExist(err) {
		t.Errorf("%s/channel/test-channel/index.html not created", serverSlug)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Simple", "simple"},
		{"My Server", "my-server"},
		{"Superchat [dot] Win", "superchat-dot-win"},
		{"Hello   World", "hello-world"},
		{"---test---", "test"},
		{"IRC #general", "irc-general"},
	}

	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
