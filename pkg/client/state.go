package client

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// State manages client-side persistent state
type State struct {
	db  *sql.DB
	dir string // Directory where state is stored
}

// OpenState opens or creates the client state database
func OpenState(path string) (*State, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open state database: %w", err)
	}

	// Configure for better reliability
	db.SetMaxOpenConns(1) // Client only needs one connection
	db.SetMaxIdleConns(1)

	// Enable WAL mode
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set busy timeout
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	state := &State{
		db:  db,
		dir: dir,
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return state, nil
}

// Close closes the state database
func (s *State) Close() error {
	return s.db.Close()
}

// GetConfig retrieves a configuration value
func (s *State) GetConfig(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM Config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetConfig stores a configuration value
func (s *State) SetConfig(key, value string) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO Config (key, value) VALUES (?, ?)
	`, key, value)
	return err
}

// GetLastNickname returns the last used nickname
func (s *State) GetLastNickname() string {
	nickname, _ := s.GetConfig("last_nickname")
	return nickname
}

// SetLastNickname stores the last used nickname
func (s *State) SetLastNickname(nickname string) error {
	return s.SetConfig("last_nickname", nickname)
}

// GetUserID returns the authenticated user ID (V2)
func (s *State) GetUserID() *uint64 {
	userIDStr, _ := s.GetConfig("user_id")
	if userIDStr == "" {
		return nil
	}
	var userID uint64
	if _, err := fmt.Sscanf(userIDStr, "%d", &userID); err != nil {
		return nil
	}
	return &userID
}

// SetUserID stores the authenticated user ID (V2)
func (s *State) SetUserID(userID *uint64) error {
	if userID == nil {
		return s.SetConfig("user_id", "")
	}
	return s.SetConfig("user_id", fmt.Sprintf("%d", *userID))
}

// GetReadState returns the read state for a channel/subchannel/thread
// Returns 0 if no state exists (never read)
func (s *State) GetReadState(channelID uint64, subchannelID *uint64, threadID *uint64) (int64, error) {
	var lastReadAt int64
	err := s.db.QueryRow(`
		SELECT last_read_at
		FROM ReadState
		WHERE channel_id = ? AND subchannel_id IS ? AND thread_id IS ?
	`, channelID, subchannelID, threadID).Scan(&lastReadAt)

	if err == sql.ErrNoRows {
		return 0, nil // Never read
	}
	if err != nil {
		return 0, err
	}

	return lastReadAt, nil
}

// UpdateReadState updates the read state for a channel/subchannel/thread
func (s *State) UpdateReadState(channelID uint64, subchannelID *uint64, threadID *uint64, timestamp int64) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO ReadState (channel_id, subchannel_id, thread_id, last_read_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, channelID, subchannelID, threadID, timestamp, time.Now().Unix())

	return err
}

// GetLastSuccessfulMethod retrieves the last successful connection method for a server
func (s *State) GetLastSuccessfulMethod(serverAddress string) (string, error) {
	var method string
	err := s.db.QueryRow(`
		SELECT last_successful_method
		FROM ConnectionHistory
		WHERE server_address = ?
	`, serverAddress).Scan(&method)

	if err == sql.ErrNoRows {
		return "", nil // No history for this server
	}
	return method, err
}

// SaveSuccessfulConnection records a successful connection method for a server
func (s *State) SaveSuccessfulConnection(serverAddress string, method string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO ConnectionHistory (server_address, last_successful_method, last_success_at)
		VALUES (?, ?, ?)
	`, serverAddress, method, now)
	return err
}

// GetFirstRun checks if this is the first time running the client
func (s *State) GetFirstRun() bool {
	val, _ := s.GetConfig("first_run_complete")
	return val != "true"
}

// SetFirstRunComplete marks first run as complete
func (s *State) SetFirstRunComplete() error {
	return s.SetConfig("first_run_complete", "true")
}

// GetStateDir returns the directory where state is stored
func (s *State) GetStateDir() string {
	return s.dir
}

// GetFirstPostWarningDismissed checks if the user has permanently dismissed the first post warning
func (s *State) GetFirstPostWarningDismissed() bool {
	val, _ := s.GetConfig("first_post_warning_dismissed")
	return val == "true"
}

// SetFirstPostWarningDismissed marks the first post warning as permanently dismissed
func (s *State) SetFirstPostWarningDismissed() error {
	return s.SetConfig("first_post_warning_dismissed", "true")
}

// GetLastSeenTimestamp returns the timestamp when the client was last active (in milliseconds)
// Returns 0 if no timestamp has been stored
func (s *State) GetLastSeenTimestamp() int64 {
	timestampStr, _ := s.GetConfig("last_seen_timestamp")
	if timestampStr == "" {
		return 0
	}
	var timestamp int64
	if _, err := fmt.Sscanf(timestampStr, "%d", &timestamp); err != nil {
		return 0
	}
	return timestamp
}

// SetLastSeenTimestamp stores the current timestamp as the last active time (in milliseconds)
func (s *State) SetLastSeenTimestamp(timestamp int64) error {
	return s.SetConfig("last_seen_timestamp", fmt.Sprintf("%d", timestamp))
}

// UpdateLastSeenTimestamp updates the last seen timestamp to now
func (s *State) UpdateLastSeenTimestamp() error {
	return s.SetLastSeenTimestamp(time.Now().UnixMilli())
}
