package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/aeolun/superchat/pkg/database"
	"github.com/aeolun/superchat/pkg/moderation"
	"github.com/aeolun/superchat/pkg/protocol"
	_ "modernc.org/sqlite"
)

// enableWordFilter installs a filter on a test server built by testServer,
// which constructs Server directly and so starts out with none.
func enableWordFilter(t *testing.T, srv *Server, opts moderation.Options) {
	t.Helper()
	f, err := moderation.New(opts)
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}
	srv.wordFilter = f
	srv.config.WordFilterEnabled = true
	srv.config.WordFilterLog = false // keep test output clean
}

func encodeEditMessageMessage(msg *protocol.EditMessageMessage) (*protocol.Frame, error) {
	var buf bytes.Buffer
	if err := msg.EncodeTo(&buf); err != nil {
		return nil, err
	}
	return &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeEditMessage,
		Flags:   0,
		Payload: buf.Bytes(),
	}, nil
}

// The filter runs before the database write, so the stored row must already be
// censored. That is what makes every read path — history, broadcasts, the
// archiver and its HTML — safe without touching any of them.
func TestPostMessageStoresCensoredContent(t *testing.T) {
	srv, db := testServer(t)
	defer db.Close()

	channelID := createTestChannel(t, db, "general", "General")
	reloadMemDB(t, srv, db)
	enableWordFilter(t, srv, moderation.Options{})

	sess := testSession(srv)
	srv.sessions.UpdateNickname(sess.ID, "testuser")

	msg := &protocol.PostMessageMessage{
		ChannelID: uint64(channelID),
		Content:   "hey nigger, what's up",
	}
	frame, err := encodePostMessageMessage(msg)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := srv.handlePostMessage(sess, frame); err != nil {
		t.Fatalf("handlePostMessage: %v", err)
	}

	msgs, err := srv.db.ListRootMessages(channelID, nil, 10, nil, nil)
	if err != nil {
		t.Fatalf("ListRootMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}

	const want = "hey ******, what's up"
	if msgs[0].Content != want {
		t.Errorf("stored content = %q, want %q", msgs[0].Content, want)
	}
	if strings.Contains(msgs[0].Content, "nigger") {
		t.Error("uncensored slur reached the database")
	}
}

func TestPostMessageLeavesCleanContentAlone(t *testing.T) {
	srv, db := testServer(t)
	defer db.Close()

	channelID := createTestChannel(t, db, "general", "General")
	reloadMemDB(t, srv, db)
	enableWordFilter(t, srv, moderation.Options{})

	sess := testSession(srv)
	srv.sessions.UpdateNickname(sess.ID, "testuser")

	const content = "the niggardly landlord is niggling about the rent again"
	frame, err := encodePostMessageMessage(&protocol.PostMessageMessage{
		ChannelID: uint64(channelID),
		Content:   content,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := srv.handlePostMessage(sess, frame); err != nil {
		t.Fatalf("handlePostMessage: %v", err)
	}

	msgs, err := srv.db.ListRootMessages(channelID, nil, 10, nil, nil)
	if err != nil {
		t.Fatalf("ListRootMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if msgs[0].Content != content {
		t.Errorf("stored content = %q, want it unchanged", msgs[0].Content)
	}
}

// With no filter configured the handler must be a pure passthrough, which is
// also what keeps every pre-existing server test unaffected.
func TestPostMessageWithoutFilterIsUnchanged(t *testing.T) {
	srv, db := testServer(t)
	defer db.Close()

	channelID := createTestChannel(t, db, "general", "General")
	reloadMemDB(t, srv, db)
	if srv.wordFilter != nil {
		t.Fatal("testServer should not install a word filter")
	}

	sess := testSession(srv)
	srv.sessions.UpdateNickname(sess.ID, "testuser")

	const content = "hey nigger"
	frame, err := encodePostMessageMessage(&protocol.PostMessageMessage{
		ChannelID: uint64(channelID),
		Content:   content,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := srv.handlePostMessage(sess, frame); err != nil {
		t.Fatalf("handlePostMessage: %v", err)
	}

	msgs, err := srv.db.ListRootMessages(channelID, nil, 10, nil, nil)
	if err != nil {
		t.Fatalf("ListRootMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != content {
		t.Errorf("stored content = %q, want %q unchanged with filter off", msgs[0].Content, content)
	}
}

// An edit must not be a way around the filter.
func TestEditMessageStoresCensoredContent(t *testing.T) {
	srv, db := testServer(t)
	defer db.Close()

	channelID := createTestChannel(t, db, "general", "General")
	userID, err := db.CreateUser("editor", "hash", 0)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	reloadMemDB(t, srv, db)
	enableWordFilter(t, srv, moderation.Options{})

	// Post as the registered user so the edit passes the ownership check.
	messageID, _, err := srv.db.PostMessage(channelID, nil, nil, &userID, "editor", "original text")
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	sess := testSession(srv)
	sess.mu.Lock()
	sess.UserID = &userID
	sess.Nickname = "editor"
	sess.mu.Unlock()

	frame, err := encodeEditMessageMessage(&protocol.EditMessageMessage{
		MessageID:  uint64(messageID),
		NewContent: "actually you are a kike",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := srv.handleEditMessage(sess, frame); err != nil {
		t.Fatalf("handleEditMessage: %v", err)
	}

	stored, err := srv.db.GetMessage(messageID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	const want = "actually you are a ****"
	if stored.Content != want {
		t.Errorf("stored content = %q, want %q", stored.Content, want)
	}
}

func TestWordFilterConfigPlumbing(t *testing.T) {
	t.Run("absent section leaves the filter on", func(t *testing.T) {
		tc := TOMLConfig{}
		cfg := tc.ToServerConfig()
		if !cfg.WordFilterEnabled {
			t.Error("WordFilterEnabled = false, want true when [moderation] is absent")
		}
		if !cfg.WordFilterLog {
			t.Error("WordFilterLog = false, want true when [moderation] is absent")
		}
	})

	t.Run("explicit false turns it off", func(t *testing.T) {
		off := false
		tc := TOMLConfig{Moderation: ModerationSection{WordFilter: &off, LogFiltered: &off}}
		cfg := tc.ToServerConfig()
		if cfg.WordFilterEnabled {
			t.Error("WordFilterEnabled = true, want false")
		}
		if cfg.WordFilterLog {
			t.Error("WordFilterLog = true, want false")
		}
	})

	t.Run("word lists are carried through", func(t *testing.T) {
		tc := TOMLConfig{Moderation: ModerationSection{
			ExtraWords:  []string{"frobnicate"},
			RemoveWords: []string{"chink"},
			AllowWords:  []string{"coon"},
		}}
		cfg := tc.ToServerConfig()
		if len(cfg.WordFilterExtra) != 1 || cfg.WordFilterExtra[0] != "frobnicate" {
			t.Errorf("WordFilterExtra = %v", cfg.WordFilterExtra)
		}
		if len(cfg.WordFilterRemove) != 1 || cfg.WordFilterRemove[0] != "chink" {
			t.Errorf("WordFilterRemove = %v", cfg.WordFilterRemove)
		}
		if len(cfg.WordFilterAllow) != 1 || cfg.WordFilterAllow[0] != "coon" {
			t.Errorf("WordFilterAllow = %v", cfg.WordFilterAllow)
		}
	})
}

// A nickname is refused, not starred: "~******" in every message header and
// presence entry still says what it was meant to say.
func TestSetNicknameRejectsFilteredNames(t *testing.T) {
	srv, db := testServer(t)
	defer db.Close()
	enableWordFilter(t, srv, moderation.Options{})

	sess := testSession(srv)

	rejected := []string{
		"nigger",
		"NIGGER",
		"n1gg3r",
		"sandnigger",
		"kike",
		// Padded forms. These are the obvious next move once the bare name is
		// refused, and a word-boundary match would let every one of them past.
		"xxkikexx",
		"thegook",
		"kike123",
		"Mr_Kike",
		"Maine_Coon", // collision term, but bounded by the underscore
	}
	for _, name := range rejected {
		frame, err := encodeSetNicknameMessage(&protocol.SetNicknameMessage{Nickname: name})
		if err != nil {
			t.Fatalf("encode %q: %v", name, err)
		}
		if err := srv.handleSetNickname(sess, frame); err != nil {
			t.Fatalf("handleSetNickname(%q): %v", name, err)
		}

		current, ok := srv.sessions.GetSession(sess.ID)
		if ok && current.Nickname == name {
			t.Errorf("nickname %q was accepted; want it refused", name)
		}
	}
}

func TestSetNicknameAcceptsCleanNames(t *testing.T) {
	srv, db := testServer(t)
	defer db.Close()
	enableWordFilter(t, srv, moderation.Options{})

	sess := testSession(srv)

	// Allowlisted stems, and names that only collide with a collision term as an
	// unbounded substring -- those keep their word boundaries so ordinary names
	// stay usable.
	for _, name := range []string{"alice", "niggardly", "snigger", "RaccoonFan", "SpiceGirl", "Japan_Fan"} {
		frame, err := encodeSetNicknameMessage(&protocol.SetNicknameMessage{Nickname: name})
		if err != nil {
			t.Fatalf("encode %q: %v", name, err)
		}
		if err := srv.handleSetNickname(sess, frame); err != nil {
			t.Fatalf("handleSetNickname(%q): %v", name, err)
		}

		current, ok := srv.sessions.GetSession(sess.ID)
		if !ok || current.Nickname != name {
			got := ""
			if ok {
				got = current.Nickname
			}
			t.Errorf("nickname %q was refused (session has %q); want it accepted", name, got)
		}
	}
}

// MatchesName is deliberately stricter than Matches. Pin the difference so the
// two do not quietly converge.
func TestMatchesNameIsStricterThanMatches(t *testing.T) {
	f, err := moderation.New(moderation.Options{})
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}

	// Padded slurs: fine as prose (no word boundary), refused as a name.
	for _, s := range []string{"xxkikexx", "thegook", "mywetbackaccount"} {
		if f.Matches(s) {
			t.Errorf("Matches(%q) = true; word-mode terms should need boundaries in prose", s)
		}
		if !f.MatchesName(s) {
			t.Errorf("MatchesName(%q) = false; a padded slur must not be usable as a name", s)
		}
	}

	// Collision terms keep their boundaries in both, so ordinary names survive.
	for _, s := range []string{"RaccoonFan", "SpiceGirl", "Japan_Fan", "doowop_lover"} {
		if f.MatchesName(s) {
			t.Errorf("MatchesName(%q) = true; collision terms must stay word-bounded", s)
		}
	}

	// ...but a collision term standing alone in a name is still refused.
	for _, s := range []string{"coon", "Maine_Coon", "spic"} {
		if !f.MatchesName(s) {
			t.Errorf("MatchesName(%q) = false; a bounded collision term should be refused", s)
		}
	}

	// The allowlist wins over MatchesName too.
	for _, s := range []string{"niggardly", "snigger"} {
		if f.MatchesName(s) {
			t.Errorf("MatchesName(%q) = true; allowlisted words must stay usable", s)
		}
	}
}

// The retroactive pass is the thing that cleans up messages posted before the
// filter existed — Bart's actual channels, not just new posts.
func TestCensorExistingMessages(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := database.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	channelID := createTestChannel(t, db, "general", "General")

	// Post directly through the SQLite layer, bypassing the handler, which is
	// how the existing history got there.
	raw := []string{
		"hey nigger, what's up",
		"perfectly ordinary message",
		"you kike!",
		"the niggardly landlord was niggling again",
	}
	ids := make([]int64, 0, len(raw))
	for _, content := range raw {
		ids = append(ids, postTestMessage(t, db, channelID, nil, "someone", content))
	}

	f, err := moderation.New(moderation.Options{})
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}

	stats, err := db.CensorMessages(context.Background(), f)
	if err != nil {
		t.Fatalf("CensorMessages: %v", err)
	}
	if stats.Scanned != len(raw) {
		t.Errorf("Scanned = %d, want %d", stats.Scanned, len(raw))
	}
	if stats.Changed != 2 {
		t.Errorf("Changed = %d, want 2", stats.Changed)
	}

	want := []string{
		"hey ******, what's up",
		"perfectly ordinary message",
		"you ****!",
		"the niggardly landlord was niggling again",
	}
	for i, id := range ids {
		msg, err := db.GetMessage(uint64(id))
		if err != nil {
			t.Fatalf("GetMessage(%d): %v", id, err)
		}
		if msg.Content != want[i] {
			t.Errorf("message %d content = %q, want %q", id, msg.Content, want[i])
		}
	}
}

// MessageVersion is insert-only and never served to a client, and it is the only
// place the original wording survives. The pass must leave it alone.
func TestCensorExistingMessagesLeavesVersionHistoryAlone(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	channelID := createTestChannel(t, db, "general", "General")
	messageID := postTestMessage(t, db, channelID, nil, "someone", "hey nigger")

	f, err := moderation.New(moderation.Options{})
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}
	if _, err := db.CensorMessages(context.Background(), f); err != nil {
		t.Fatalf("CensorMessages: %v", err)
	}

	// The Message row is censored...
	msg, err := db.GetMessage(uint64(messageID))
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if msg.Content != "hey ******" {
		t.Fatalf("Message content = %q, want it censored", msg.Content)
	}
	db.Close()

	// ...while MessageVersion keeps the original. Read it with a plain handle
	// rather than adding a test-only accessor to the database package.
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer raw.Close()

	var versionContent string
	err = raw.QueryRow(
		`SELECT content FROM MessageVersion WHERE message_id = ? ORDER BY id LIMIT 1`,
		messageID).Scan(&versionContent)
	if errors.Is(err, sql.ErrNoRows) {
		t.Skip("PostMessage did not record a MessageVersion row; nothing to assert")
	}
	if err != nil {
		t.Fatalf("query MessageVersion: %v", err)
	}
	if versionContent != "hey nigger" {
		t.Errorf("MessageVersion content = %q, want the original preserved", versionContent)
	}
}

func TestNewWordFilter(t *testing.T) {
	t.Run("installed by default", func(t *testing.T) {
		f, err := newWordFilter(DefaultConfig())
		if err != nil {
			t.Fatalf("newWordFilter: %v", err)
		}
		if f == nil {
			t.Fatal("filter is nil; want it built by default")
		}
		if f.NumTerms() == 0 {
			t.Error("filter has no terms")
		}
	})

	t.Run("omitted when disabled", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.WordFilterEnabled = false

		f, err := newWordFilter(cfg)
		if err != nil {
			t.Fatalf("newWordFilter: %v", err)
		}
		if f != nil {
			t.Error("filter built despite WordFilterEnabled = false")
		}
	})

	// A word list the operator got wrong must stop the server, not be skipped:
	// a filter that silently is not filtering is worse than one that refuses to
	// boot. NewServer propagates this error.
	t.Run("invalid word list is an error", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.WordFilterExtra = []string{".,;"} // normalizes to nothing matchable

		_, err := newWordFilter(cfg)
		if err == nil {
			t.Fatal("newWordFilter accepted an unusable word list")
		}
		if !strings.Contains(err.Error(), "[moderation]") {
			t.Errorf("error = %q, want it to name the [moderation] section", err)
		}
	})

	t.Run("configured lists take effect", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.WordFilterExtra = []string{"frobnicate"}
		cfg.WordFilterRemove = []string{"chink", "chinks"}

		f, err := newWordFilter(cfg)
		if err != nil {
			t.Fatalf("newWordFilter: %v", err)
		}
		if _, changed := f.Censor("please frobnicate it"); !changed {
			t.Error("extra_words term did not take effect")
		}
		if _, changed := f.Censor("a chink in the armour"); changed {
			t.Error("remove_words term still matched")
		}
	})
}
