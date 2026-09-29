package archiver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	gen "github.com/aeolun/superchat/pkg/archive/generated"
	"github.com/aeolun/superchat/pkg/moderation"
	_ "modernc.org/sqlite"
)

// ChannelStateRow is the per-channel state returned to the server during handshake.
// IDs are remote (server-side) values so the server can determine what to backfill.
type ChannelStateRow struct {
	ChannelID     int64
	LastMessageID int64
}

// ServerRow is a server from the archive database.
type ServerRow struct {
	ID   int64
	Name string
	Slug string
}

// ChannelRow is a channel from the archive database.
type ChannelRow struct {
	ID             int64
	Name           string
	Description    string
	ChannelType    int
	RetentionHours int
	ArchiveEnabled int
}

// MessageRow is a message from the archive database.
type MessageRow struct {
	ID             int64
	ChannelID      int64
	ParentID       *int64
	ThreadRootID   *int64
	AuthorUserID   *int64
	AuthorNickname string
	Content        string
	CreatedAt      int64
	EditedAt       *int64
	DeletedAt      *int64
	Depth          int // 0 for root, computed via recursive CTE in thread queries
}

// Store manages the archiver's persistent SQLite database.
type Store struct {
	db *sql.DB

	// filter censors content on the way in. The superchat server already
	// censors before forwarding, so this normally finds nothing — but the
	// archiver can be fed by a server with the filter switched off, or an older
	// build, and its output is published as public HTML. A nil filter is a no-op.
	filter *moderation.Filter
}

// NewStore opens (or creates) the archive database. filter may be nil.
func NewStore(path string, filter *moderation.Filter) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("archiver: open db: %w", err)
	}

	// WAL mode for concurrent reads
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("archiver: set WAL: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("archiver: migrate: %w", err)
	}

	return &Store{db: db, filter: filter}, nil
}

// CensorExisting applies the word filter to content already in the archive.
//
// The archiver has its own copy of every message, so censoring the superchat
// server alone leaves the archive — and the static HTML generated from it —
// showing the original text. Callers should regenerate HTML afterwards if this
// reports any changes; Server.Start does that unconditionally on boot.
func (s *Store) CensorExisting(ctx context.Context) (moderation.BackfillStats, error) {
	return s.filter.Backfill(ctx, moderation.BackfillTarget{
		Read:     s.db,
		Write:    s.db,
		Table:    "Message",
		IDColumn: "id",
		// author_nickname is rendered into every message header in the published
		// HTML, so it needs the same treatment as the body.
		ContentColumns: []string{"content", "author_nickname"},
	})
}

// Close closes the database.
func (s *Store) Close() {
	s.db.Close()
}

// migrate creates the schema if it doesn't exist.
// Detects old single-server schema and recreates for multi-server support.
func migrate(db *sql.DB) error {
	// Check if we need to migrate from old schema (no Server table)
	var serverTableExists int
	_ = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='Server'").Scan(&serverTableExists)

	if serverTableExists == 0 {
		// Drop old tables if they exist (will be re-backfilled from server)
		db.Exec("DROP TABLE IF EXISTS Message")
		db.Exec("DROP TABLE IF EXISTS Channel")
	}

	schema := `
	CREATE TABLE IF NOT EXISTS Server (
		id   INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL
	);

	CREATE TABLE IF NOT EXISTS Channel (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id       INTEGER NOT NULL REFERENCES Server(id),
		remote_id       INTEGER NOT NULL,
		name            TEXT NOT NULL,
		description     TEXT NOT NULL DEFAULT '',
		channel_type    INTEGER NOT NULL DEFAULT 0,
		retention_hours INTEGER NOT NULL DEFAULT 168,
		archive_enabled INTEGER NOT NULL DEFAULT 1,
		UNIQUE(server_id, remote_id)
	);

	CREATE TABLE IF NOT EXISTS Message (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id       INTEGER NOT NULL REFERENCES Server(id),
		remote_id       INTEGER NOT NULL,
		channel_id      INTEGER NOT NULL REFERENCES Channel(id),
		parent_id       INTEGER REFERENCES Message(id),
		thread_root_id  INTEGER REFERENCES Message(id),
		author_user_id  INTEGER,
		author_nickname TEXT NOT NULL DEFAULT '',
		content         TEXT NOT NULL DEFAULT '',
		created_at      INTEGER NOT NULL,
		edited_at       INTEGER,
		deleted_at      INTEGER,
		UNIQUE(server_id, remote_id)
	);

	CREATE INDEX IF NOT EXISTS idx_message_channel ON Message(channel_id, id);
	CREATE INDEX IF NOT EXISTS idx_message_thread ON Message(thread_root_id);
	CREATE INDEX IF NOT EXISTS idx_channel_server ON Channel(server_id);
	CREATE INDEX IF NOT EXISTS idx_message_server_remote ON Message(server_id, remote_id);
	`
	_, err := db.Exec(schema)
	return err
}

// GetOrCreateServer returns the server ID for the given name, creating it if needed.
func (s *Store) GetOrCreateServer(name string) (int64, error) {
	var id int64
	err := s.db.QueryRow("SELECT id FROM Server WHERE name = ?", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	result, err := s.db.Exec("INSERT INTO Server (name) VALUES (?)", name)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetServers returns all servers.
func (s *Store) GetServers() ([]ServerRow, error) {
	rows, err := s.db.Query("SELECT id, name FROM Server ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []ServerRow
	for rows.Next() {
		var srv ServerRow
		if err := rows.Scan(&srv.ID, &srv.Name); err != nil {
			return nil, err
		}
		srv.Slug = Slugify(srv.Name)
		servers = append(servers, srv)
	}
	return servers, rows.Err()
}

// GetServerByID returns a single server by ID.
func (s *Store) GetServerByID(id int64) (*ServerRow, error) {
	var srv ServerRow
	err := s.db.QueryRow("SELECT id, name FROM Server WHERE id = ?", id).Scan(&srv.ID, &srv.Name)
	if err != nil {
		return nil, err
	}
	srv.Slug = Slugify(srv.Name)
	return &srv, nil
}

// GetChannelStates returns the last remote message ID per channel for a specific server.
// Returns remote IDs so the server can determine what to backfill.
func (s *Store) GetChannelStates(serverID int64) ([]ChannelStateRow, error) {
	rows, err := s.db.Query(`
		SELECT c.remote_id, COALESCE(MAX(m.remote_id), 0) as last_message_id
		FROM Channel c
		LEFT JOIN Message m ON m.channel_id = c.id
		WHERE c.server_id = ?
		GROUP BY c.id
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []ChannelStateRow
	for rows.Next() {
		var cs ChannelStateRow
		if err := rows.Scan(&cs.ChannelID, &cs.LastMessageID); err != nil {
			return nil, err
		}
		states = append(states, cs)
	}
	return states, rows.Err()
}

// UpsertChannel inserts or updates a channel for a specific server.
func (s *Store) UpsertChannel(serverID int64, msg *gen.ChannelInfo) error {
	_, err := s.db.Exec(`
		INSERT INTO Channel (server_id, remote_id, name, description, channel_type, retention_hours, archive_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_id, remote_id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			channel_type = excluded.channel_type,
			retention_hours = excluded.retention_hours,
			archive_enabled = excluded.archive_enabled
	`, serverID, msg.ChannelId, msg.Name, msg.Description, msg.ChannelType, msg.RetentionHours, msg.ArchiveEnabled)
	return err
}

// UpsertMessage inserts or updates a message for a specific server.
// Resolves remote channel/parent/thread IDs to local archiver IDs.
func (s *Store) UpsertMessage(serverID int64, msg *gen.MessageSync) error {
	// Resolve remote channel ID to local channel ID
	var channelID int64
	err := s.db.QueryRow(
		"SELECT id FROM Channel WHERE server_id = ? AND remote_id = ?",
		serverID, msg.ChannelId,
	).Scan(&channelID)
	if err != nil {
		return fmt.Errorf("resolve channel %d: %w", msg.ChannelId, err)
	}

	// Resolve remote parent_id to local ID (may not exist yet)
	var parentID *int64
	if msg.ParentId != nil {
		var pid int64
		err := s.db.QueryRow(
			"SELECT id FROM Message WHERE server_id = ? AND remote_id = ?",
			serverID, *msg.ParentId,
		).Scan(&pid)
		if err == nil {
			parentID = &pid
		}
	}

	// Resolve remote thread_root_id to local ID
	var threadRootID *int64
	if msg.ThreadRootId != nil {
		var tid int64
		err := s.db.QueryRow(
			"SELECT id FROM Message WHERE server_id = ? AND remote_id = ?",
			serverID, *msg.ThreadRootId,
		).Scan(&tid)
		if err == nil {
			threadRootID = &tid
		}
	}

	content, _ := s.filter.Censor(msg.Content)
	// The nickname is censored rather than refused here: by the time a message
	// reaches the archive the name is a historical fact, and rejecting the row
	// would lose the message instead of the name.
	nickname, _ := s.filter.Censor(msg.AuthorNickname)

	_, err = s.db.Exec(`
		INSERT INTO Message (server_id, remote_id, channel_id, parent_id, thread_root_id,
			author_user_id, author_nickname, content, created_at, edited_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_id, remote_id) DO UPDATE SET
			content = excluded.content,
			edited_at = excluded.edited_at,
			deleted_at = excluded.deleted_at
	`, serverID, msg.MessageId, channelID, parentID, threadRootID,
		msg.AuthorUserId, nickname, content, msg.CreatedAt, msg.EditedAt, msg.DeletedAt)
	return err
}

// UpdateMessage applies an edit to a message identified by server and remote ID.
func (s *Store) UpdateMessage(serverID int64, msg *gen.MessageEdited) error {
	content, _ := s.filter.Censor(msg.NewContent)

	_, err := s.db.Exec(`
		UPDATE Message SET content = ?, edited_at = ?
		WHERE server_id = ? AND remote_id = ?
	`, content, msg.EditedAt, serverID, msg.MessageId)
	return err
}

// DeleteMessage applies a soft delete to a message identified by server and remote ID.
func (s *Store) DeleteMessage(serverID int64, msg *gen.MessageDeleted) error {
	content, _ := s.filter.Censor(msg.Content)

	_, err := s.db.Exec(`
		UPDATE Message SET deleted_at = ?, content = ?
		WHERE server_id = ? AND remote_id = ?
	`, msg.DeletedAt, content, serverID, msg.MessageId)
	return err
}

// GetChannelsByServer returns all channels for a specific server.
func (s *Store) GetChannelsByServer(serverID int64) ([]ChannelRow, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, channel_type, retention_hours, archive_enabled
		FROM Channel WHERE server_id = ? ORDER BY name
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []ChannelRow
	for rows.Next() {
		var ch ChannelRow
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Description, &ch.ChannelType, &ch.RetentionHours, &ch.ArchiveEnabled); err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

// GetChannelByID returns a single channel by local ID.
func (s *Store) GetChannelByID(id int64) (*ChannelRow, error) {
	var ch ChannelRow
	err := s.db.QueryRow(`
		SELECT id, name, description, channel_type, retention_hours, archive_enabled
		FROM Channel WHERE id = ?
	`, id).Scan(&ch.ID, &ch.Name, &ch.Description, &ch.ChannelType, &ch.RetentionHours, &ch.ArchiveEnabled)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

// GetRootMessages returns top-level messages (no parent) for a channel, ordered by created_at.
func (s *Store) GetRootMessages(channelID int64, limit, offset int) ([]MessageRow, error) {
	rows, err := s.db.Query(`
		SELECT id, channel_id, parent_id, thread_root_id, author_user_id, author_nickname,
			content, created_at, edited_at, deleted_at
		FROM Message
		WHERE channel_id = ? AND parent_id IS NULL
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, channelID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// GetThreadMessages returns all messages in a thread (root + replies), ordered by created_at.
// Computes depth via recursive CTE (0 for root, 1+ for replies).
func (s *Store) GetThreadMessages(threadRootID int64) ([]MessageRow, error) {
	rows, err := s.db.Query(`
		WITH RECURSIVE thread_tree AS (
			SELECT id, channel_id, parent_id, thread_root_id, author_user_id, author_nickname,
				content, created_at, edited_at, deleted_at, 0 AS depth
			FROM Message
			WHERE id = ?
			UNION ALL
			SELECT m.id, m.channel_id, m.parent_id, m.thread_root_id, m.author_user_id, m.author_nickname,
				m.content, m.created_at, m.edited_at, m.deleted_at, tt.depth + 1
			FROM Message m
			JOIN thread_tree tt ON m.parent_id = tt.id
		)
		SELECT id, channel_id, parent_id, thread_root_id, author_user_id, author_nickname,
			content, created_at, edited_at, deleted_at, depth
		FROM thread_tree
		ORDER BY created_at ASC
	`, threadRootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessagesWithDepth(rows)
}

// GetMessageCount returns the total number of root messages in a channel.
func (s *Store) GetMessageCount(channelID int64) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM Message WHERE channel_id = ? AND parent_id IS NULL", channelID).Scan(&count)
	return count, err
}

func scanMessages(rows *sql.Rows) ([]MessageRow, error) {
	var messages []MessageRow
	for rows.Next() {
		var m MessageRow
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.ParentID, &m.ThreadRootID, &m.AuthorUserID, &m.AuthorNickname, &m.Content, &m.CreatedAt, &m.EditedAt, &m.DeletedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func scanMessagesWithDepth(rows *sql.Rows) ([]MessageRow, error) {
	var messages []MessageRow
	for rows.Next() {
		var m MessageRow
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.ParentID, &m.ThreadRootID, &m.AuthorUserID, &m.AuthorNickname, &m.Content, &m.CreatedAt, &m.EditedAt, &m.DeletedAt, &m.Depth); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// Slugify converts a server name to a URL-safe slug.
func Slugify(name string) string {
	s := strings.ToLower(name)
	var buf strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			buf.WriteRune(r)
		} else {
			buf.WriteRune('-')
		}
	}
	result := buf.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}
