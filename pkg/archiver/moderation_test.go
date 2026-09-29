package archiver

import (
	"context"
	"strings"
	"testing"

	gen "github.com/aeolun/superchat/pkg/archive/generated"
	"github.com/aeolun/superchat/pkg/moderation"
)

func filteredStore(t *testing.T) *Store {
	t.Helper()
	f, err := moderation.New(moderation.Options{})
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}
	return newStoreWithFilter(t, f)
}

// seedChannel creates a channel and returns its local id.
func seedChannel(t *testing.T, store *Store, srvID int64) int64 {
	t.Helper()
	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "test"}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	channels, err := store.GetChannelsByServer(srvID)
	if err != nil {
		t.Fatalf("GetChannelsByServer: %v", err)
	}
	return channels[0].ID
}

// The archiver keeps its own copy and publishes it as public HTML, so it must
// not depend on the feeding server having censored anything.
func TestArchiverCensorsOnIngest(t *testing.T) {
	store := filteredStore(t)
	srvID := createTestServer(t, store, "TestServer")
	chID := seedChannel(t, store, srvID)

	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1,
		AuthorNickname: "alice", Content: "hey nigger",
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
	if msgs[0].Content != "hey ******" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "hey ******")
	}
}

func TestArchiverCensorsOnEdit(t *testing.T) {
	store := filteredStore(t)
	srvID := createTestServer(t, store, "TestServer")
	chID := seedChannel(t, store, srvID)

	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1, AuthorNickname: "alice",
		Content: "harmless", CreatedAt: 1700000000000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMessage(srvID, &gen.MessageEdited{
		MessageId: 100, ChannelId: 1, NewContent: "you kike!", EditedAt: 1700000001000,
	}); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}

	msgs, _ := store.GetRootMessages(chID, 50, 0)
	if len(msgs) != 1 || msgs[0].Content != "you ****!" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "you ****!")
	}
}

// The delete path carries content too, so it is a third way in.
func TestArchiverCensorsOnDelete(t *testing.T) {
	store := filteredStore(t)
	srvID := createTestServer(t, store, "TestServer")
	chID := seedChannel(t, store, srvID)

	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1, AuthorNickname: "alice",
		Content: "harmless", CreatedAt: 1700000000000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteMessage(srvID, &gen.MessageDeleted{
		MessageId: 100, ChannelId: 1, Content: "gook", DeletedAt: 1700000002000,
	}); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	msgs, _ := store.GetRootMessages(chID, 50, 0)
	if len(msgs) != 1 || msgs[0].Content != "****" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "****")
	}
}

// The case Bart actually has: the archive already ingested everything before the
// filter existed. An unfiltered store takes the slurs, then CensorExisting on a
// filtered store over the same database cleans them.
func TestArchiverCensorExistingCleansPriorContent(t *testing.T) {
	// Ingest with no filter, the way the archive was populated originally.
	store := newTestStore(t)
	srvID := createTestServer(t, store, "TestServer")
	chID := seedChannel(t, store, srvID)

	raw := []string{
		"hey nigger",
		"perfectly fine message",
		"you kike!",
		"the niggardly landlord was niggling",
	}
	for i, content := range raw {
		if err := store.UpsertMessage(srvID, &gen.MessageSync{
			MessageId: uint64(100 + i), ChannelId: 1,
			AuthorNickname: "alice", Content: content,
			CreatedAt: int64(1700000000000 + i),
		}); err != nil {
			t.Fatalf("UpsertMessage: %v", err)
		}
	}

	// Confirm the archive really is dirty before the pass, or the test proves
	// nothing.
	msgs, _ := store.GetRootMessages(chID, 50, 0)
	if len(msgs) != len(raw) {
		t.Fatalf("len(msgs) = %d, want %d", len(msgs), len(raw))
	}
	foundRaw := false
	for _, m := range msgs {
		if strings.Contains(m.Content, "nigger") {
			foundRaw = true
		}
	}
	if !foundRaw {
		t.Fatal("fixture did not store uncensored content; test would be vacuous")
	}

	// Now attach a filter and run the retroactive pass.
	f, err := moderation.New(moderation.Options{})
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}
	store.filter = f

	stats, err := store.CensorExisting(context.Background())
	if err != nil {
		t.Fatalf("CensorExisting: %v", err)
	}
	if stats.Scanned != len(raw) {
		t.Errorf("Scanned = %d, want %d", stats.Scanned, len(raw))
	}
	if stats.Changed != 2 {
		t.Errorf("Changed = %d, want 2", stats.Changed)
	}

	want := map[string]bool{
		"hey ******":                          true,
		"perfectly fine message":              true,
		"you ****!":                           true,
		"the niggardly landlord was niggling": true,
	}
	msgs, _ = store.GetRootMessages(chID, 50, 0)
	for _, m := range msgs {
		if !want[m.Content] {
			t.Errorf("unexpected content after backfill: %q", m.Content)
		}
	}
}

func TestArchiverCensorExistingWithoutFilterIsNoOp(t *testing.T) {
	store := newTestStore(t)
	srvID := createTestServer(t, store, "TestServer")
	chID := seedChannel(t, store, srvID)

	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1, AuthorNickname: "alice",
		Content: "hey nigger", CreatedAt: 1700000000000,
	}); err != nil {
		t.Fatal(err)
	}

	stats, err := store.CensorExisting(context.Background())
	if err != nil {
		t.Fatalf("CensorExisting: %v", err)
	}
	if stats.Scanned != 0 || stats.Changed != 0 {
		t.Errorf("stats = %+v, want zero with no filter", stats)
	}

	msgs, _ := store.GetRootMessages(chID, 50, 0)
	if len(msgs) != 1 || msgs[0].Content != "hey nigger" {
		t.Errorf("Content = %q, want it untouched", msgs[0].Content)
	}
}

// New(...) must reject a bad word list rather than silently archiving unfiltered.
func TestArchiverNewRejectsInvalidWordList(t *testing.T) {
	_, err := New(Config{
		DBPath:          t.TempDir() + "/archive.db",
		OutputDir:       t.TempDir(),
		WordFilter:      true,
		WordFilterExtra: []string{".,;"},
	})
	if err == nil {
		t.Fatal("New succeeded with an unusable word list")
	}
	if !strings.Contains(err.Error(), "word list") {
		t.Errorf("error = %q, want it to mention the word list", err)
	}
}

// New runs the retroactive pass, which is what makes a plain archiver restart
// enough to clean an archive that predates the filter.
func TestArchiverNewRunsBackfill(t *testing.T) {
	dbPath := t.TempDir() + "/archive.db"

	// Populate without a filter.
	store, err := NewStore(dbPath, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	srvID, err := store.GetOrCreateServer("TestServer")
	if err != nil {
		t.Fatalf("GetOrCreateServer: %v", err)
	}
	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "test"}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1, AuthorNickname: "alice",
		Content: "hey nigger", CreatedAt: 1700000000000,
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	// Restart with the filter on: New should clean the archive.
	srv, err := New(Config{
		DBPath:             dbPath,
		OutputDir:          t.TempDir(),
		WordFilter:         true,
		WordFilterBackfill: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.store.Close()

	channels, _ := srv.store.GetChannelsByServer(srvID)
	msgs, _ := srv.store.GetRootMessages(channels[0].ID, 50, 0)
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	if msgs[0].Content != "hey ******" {
		t.Errorf("Content = %q, want %q; New did not run the backfill", msgs[0].Content, "hey ******")
	}
}

func TestArchiverNewSkipsBackfillWhenDisabled(t *testing.T) {
	dbPath := t.TempDir() + "/archive.db"

	store, err := NewStore(dbPath, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	srvID, _ := store.GetOrCreateServer("TestServer")
	if err := store.UpsertChannel(srvID, &gen.ChannelInfo{ChannelId: 1, Name: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMessage(srvID, &gen.MessageSync{
		MessageId: 100, ChannelId: 1, AuthorNickname: "alice",
		Content: "hey nigger", CreatedAt: 1700000000000,
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	srv, err := New(Config{
		DBPath:             dbPath,
		OutputDir:          t.TempDir(),
		WordFilter:         true,
		WordFilterBackfill: false,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.store.Close()

	channels, _ := srv.store.GetChannelsByServer(srvID)
	msgs, _ := srv.store.GetRootMessages(channels[0].ID, 50, 0)
	if len(msgs) != 1 || msgs[0].Content != "hey nigger" {
		t.Errorf("Content = %q, want it untouched with backfill disabled", msgs[0].Content)
	}
}
