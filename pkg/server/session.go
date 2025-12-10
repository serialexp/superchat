package server

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/aeolun/superchat/pkg/database"
)

// ChannelSubscription represents a channel/subchannel subscription
type ChannelSubscription struct {
	ChannelID    uint64
	SubchannelID *uint64
}

// Session represents an active client connection
type Session struct {
	ID                     uint64
	DBSessionID            int64        // Database session record ID
	UserID                 *int64       // Registered user ID (nil for anonymous)
	Nickname               string       // Current nickname
	UserFlags              uint8        // Cached user flags (0 for anonymous, updated on login/register)
	Shadowbanned           bool         // True if user is shadowbanned (messages hidden from other users)
	Conn                   *SafeConn    // TCP connection with automatic write synchronization
	RemoteAddr             string       // Remote address (for rate limiting)
	JoinedChannel          *int64       // Currently joined channel ID
	mu                     sync.RWMutex // Protects Nickname, UserFlags, Shadowbanned, and JoinedChannel
	lastActivityUpdateTime int64        // Last time we wrote activity to DB (milliseconds, atomic)

	// Subscriptions for selective message broadcasting
	subscribedThreads  map[uint64]ChannelSubscription // thread_id -> channel subscription
	subscribedChannels map[ChannelSubscription]bool   // channel/subchannel -> true
	subMu              sync.RWMutex                   // Protects subscription maps

	// V3 DM encryption (for anonymous users with ephemeral keys)
	EncryptionPublicKey []byte // X25519 public key (32 bytes, session-only for anonymous)
}

// SessionManager manages all active sessions
type SessionManager struct {
	db                       *database.MemDB
	sessions                 map[uint64]*Session
	nextID                   uint64
	mu                       sync.RWMutex
	metrics                  *Metrics
	activityUpdateIntervalMs int64 // Half of session timeout in milliseconds

	// Reverse subscription indices for fast broadcast lookups
	threadSubscribers  map[uint64]map[uint64]*Session              // threadID -> sessionID -> session
	channelSubscribers map[ChannelSubscription]map[uint64]*Session // channelSub -> sessionID -> session
	subIndexMu         sync.RWMutex                                // Protects subscription indices
}

// NewSessionManager creates a new session manager
func NewSessionManager(db *database.MemDB, sessionTimeoutSeconds int) *SessionManager {
	// Activity update interval is half the session timeout
	activityIntervalMs := int64(sessionTimeoutSeconds) * 500 // half in milliseconds

	sm := &SessionManager{
		db:                       db,
		activityUpdateIntervalMs: activityIntervalMs,
		sessions:                 make(map[uint64]*Session),
		nextID:                   1,
		threadSubscribers:        make(map[uint64]map[uint64]*Session),
		channelSubscribers:       make(map[ChannelSubscription]map[uint64]*Session),
	}

	return sm
}

// SetMetrics attaches metrics to the session manager
func (sm *SessionManager) SetMetrics(metrics *Metrics) {
	sm.metrics = metrics
}

// CreateSession creates a new session
func (sm *SessionManager) CreateSession(userID *int64, nickname, connType string, conn net.Conn) (*Session, error) {
	// Create database session record (instant in-memory)
	dbSessionID, err := sm.db.CreateSession(userID, nickname, connType)
	if err != nil {
		return nil, fmt.Errorf("failed to create DB session: %w", err)
	}

	// Allocate session ID atomically (no lock needed)
	sessionID := atomic.AddUint64(&sm.nextID, 1) - 1

	// Create session object (no lock needed)
	sess := &Session{
		ID:                     sessionID,
		DBSessionID:            dbSessionID,
		UserID:                 userID,
		Nickname:               nickname,
		Conn:                   NewSafeConn(conn),
		RemoteAddr:             conn.RemoteAddr().String(),
		lastActivityUpdateTime: 0, // Will be set on first activity update
		subscribedThreads:      make(map[uint64]ChannelSubscription),
		subscribedChannels:     make(map[ChannelSubscription]bool),
	}

	// Only acquire lock for map insertion (critical section)
	sm.mu.Lock()
	sm.sessions[sessionID] = sess
	sessionCount := len(sm.sessions)
	sm.mu.Unlock()

	// Update metrics outside lock
	if sm.metrics != nil {
		sm.metrics.RecordActiveSessions(sessionCount)
		sm.metrics.RecordSessionCreated()
	}

	return sess, nil
}

// GetSession returns a session by ID
func (sm *SessionManager) GetSession(sessionID uint64) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sess, ok := sm.sessions[sessionID]
	return sess, ok
}

// GetAllSessions returns all active sessions
func (sm *SessionManager) GetAllSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]*Session, 0, len(sm.sessions))
	for _, sess := range sm.sessions {
		sessions = append(sessions, sess)
	}
	return sessions
}

// RemoveSession removes a session and closes the connection
func (sm *SessionManager) RemoveSession(sessionID uint64) {
	sm.mu.Lock()
	sess, ok := sm.sessions[sessionID]
	if !ok {
		sm.mu.Unlock()
		return
	}
	delete(sm.sessions, sessionID)
	sessionCount := len(sm.sessions)
	sm.mu.Unlock()

	// Update metrics
	if sm.metrics != nil {
		sm.metrics.RecordActiveSessions(sessionCount)
		sm.metrics.RecordSessionDisconnected()
	}

	// Clean up reverse subscription indices
	sess.subMu.Lock()
	threadIDs := make([]uint64, 0, len(sess.subscribedThreads))
	for threadID := range sess.subscribedThreads {
		threadIDs = append(threadIDs, threadID)
	}
	channelSubs := make([]ChannelSubscription, 0, len(sess.subscribedChannels))
	for channelSub := range sess.subscribedChannels {
		channelSubs = append(channelSubs, channelSub)
	}
	sess.subscribedThreads = nil
	sess.subscribedChannels = nil
	sess.subMu.Unlock()

	// Update reverse indices
	sm.subIndexMu.Lock()
	for _, threadID := range threadIDs {
		if subscribers := sm.threadSubscribers[threadID]; subscribers != nil {
			delete(subscribers, sessionID)
			if len(subscribers) == 0 {
				delete(sm.threadSubscribers, threadID)
			}
		}
	}
	for _, channelSub := range channelSubs {
		if subscribers := sm.channelSubscribers[channelSub]; subscribers != nil {
			delete(subscribers, sessionID)
			if len(subscribers) == 0 {
				delete(sm.channelSubscribers, channelSub)
			}
		}
	}
	sm.subIndexMu.Unlock()

	// Close connection
	sess.Conn.Close()

	// Queue DB session deletion (buffered)
	sm.db.DeleteSession(sess.DBSessionID)
}

// UpdateNickname updates a session's nickname
func (sm *SessionManager) UpdateNickname(sessionID uint64, nickname string) error {
	sess, ok := sm.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("session not found")
	}

	sess.mu.Lock()
	sess.Nickname = nickname
	sess.mu.Unlock()

	// Update in database (no error to return - queued in buffer)
	sm.db.UpdateSessionNickname(sess.DBSessionID, nickname)
	return nil
}

// UpdateSessionActivity updates session activity only if the configured interval has passed
func (sm *SessionManager) UpdateSessionActivity(sess *Session, now int64) {
	lastUpdate := atomic.LoadInt64(&sess.lastActivityUpdateTime)

	// Only update if the configured interval has passed (half of session timeout)
	if now-lastUpdate >= sm.activityUpdateIntervalMs {
		// Try to atomically update the timestamp
		if atomic.CompareAndSwapInt64(&sess.lastActivityUpdateTime, lastUpdate, now) {
			sm.db.UpdateSessionActivity(sess.DBSessionID)
		}
	}
}

// SetJoinedChannel sets the currently joined channel for a session
func (sm *SessionManager) SetJoinedChannel(sessionID uint64, channelID *int64) error {
	sess, ok := sm.GetSession(sessionID)
	if !ok {
		return fmt.Errorf("session not found")
	}

	sess.mu.Lock()
	sess.JoinedChannel = channelID
	sess.mu.Unlock()

	return nil
}

// CountOnlineUsers returns the number of currently connected users
func (sm *SessionManager) CountOnlineUsers() uint32 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return uint32(len(sm.sessions))
}

// CloseAll closes all sessions
func (sm *SessionManager) CloseAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, sess := range sm.sessions {
		sess.Conn.Close()
		sm.db.DeleteSession(sess.DBSessionID)
	}

	sm.sessions = make(map[uint64]*Session)
}

// SubscribeToThread subscribes the session to a thread and updates reverse index
func (sm *SessionManager) SubscribeToThread(sess *Session, threadID uint64, channelSub ChannelSubscription) {
	// Update session's subscription map
	sess.subMu.Lock()
	sess.subscribedThreads[threadID] = channelSub
	sess.subMu.Unlock()

	// Update reverse index
	sm.subIndexMu.Lock()
	if sm.threadSubscribers[threadID] == nil {
		sm.threadSubscribers[threadID] = make(map[uint64]*Session)
	}
	sm.threadSubscribers[threadID][sess.ID] = sess
	sm.subIndexMu.Unlock()
}

// UnsubscribeFromThread unsubscribes the session from a thread and updates reverse index
func (sm *SessionManager) UnsubscribeFromThread(sess *Session, threadID uint64) {
	// Update session's subscription map
	sess.subMu.Lock()
	delete(sess.subscribedThreads, threadID)
	sess.subMu.Unlock()

	// Update reverse index
	sm.subIndexMu.Lock()
	if subscribers := sm.threadSubscribers[threadID]; subscribers != nil {
		delete(subscribers, sess.ID)
		if len(subscribers) == 0 {
			delete(sm.threadSubscribers, threadID)
		}
	}
	sm.subIndexMu.Unlock()
}

// SubscribeToChannel subscribes the session to a channel/subchannel and updates reverse index
func (sm *SessionManager) SubscribeToChannel(sess *Session, channelSub ChannelSubscription) {
	// Update session's subscription map
	sess.subMu.Lock()
	sess.subscribedChannels[channelSub] = true
	sess.subMu.Unlock()

	// Update reverse index
	sm.subIndexMu.Lock()
	if sm.channelSubscribers[channelSub] == nil {
		sm.channelSubscribers[channelSub] = make(map[uint64]*Session)
	}
	sm.channelSubscribers[channelSub][sess.ID] = sess
	sm.subIndexMu.Unlock()
}

// UnsubscribeFromChannel unsubscribes the session from a channel/subchannel and updates reverse index
func (sm *SessionManager) UnsubscribeFromChannel(sess *Session, channelSub ChannelSubscription) {
	// Update session's subscription map
	sess.subMu.Lock()
	delete(sess.subscribedChannels, channelSub)
	sess.subMu.Unlock()

	// Update reverse index
	sm.subIndexMu.Lock()
	if subscribers := sm.channelSubscribers[channelSub]; subscribers != nil {
		delete(subscribers, sess.ID)
		if len(subscribers) == 0 {
			delete(sm.channelSubscribers, channelSub)
		}
	}
	sm.subIndexMu.Unlock()
}

// GetThreadSubscribers returns all sessions subscribed to a thread (optimized via reverse index)
func (sm *SessionManager) GetThreadSubscribers(threadID uint64) []*Session {
	sm.subIndexMu.RLock()
	defer sm.subIndexMu.RUnlock()

	subscribers := sm.threadSubscribers[threadID]
	if len(subscribers) == 0 {
		return nil
	}

	result := make([]*Session, 0, len(subscribers))
	for _, sess := range subscribers {
		result = append(result, sess)
	}
	return result
}

// GetChannelSubscribers returns all sessions subscribed to a channel (optimized via reverse index)
func (sm *SessionManager) GetChannelSubscribers(channelSub ChannelSubscription) []*Session {
	sm.subIndexMu.RLock()
	defer sm.subIndexMu.RUnlock()

	subscribers := sm.channelSubscribers[channelSub]
	if len(subscribers) == 0 {
		return nil
	}

	result := make([]*Session, 0, len(subscribers))
	for _, sess := range subscribers {
		result = append(result, sess)
	}
	return result
}

// IsSubscribedToThread checks if the session is subscribed to a thread (thread-safe)
func (s *Session) IsSubscribedToThread(threadID uint64) (ChannelSubscription, bool) {
	s.subMu.RLock()
	defer s.subMu.RUnlock()

	channelSub, ok := s.subscribedThreads[threadID]
	return channelSub, ok
}

// IsSubscribedToChannel checks if the session is subscribed to a channel/subchannel (thread-safe)
func (s *Session) IsSubscribedToChannel(channelSub ChannelSubscription) bool {
	s.subMu.RLock()
	defer s.subMu.RUnlock()

	return s.subscribedChannels[channelSub]
}

// ThreadSubscriptionCount returns the number of thread subscriptions (thread-safe)
func (s *Session) ThreadSubscriptionCount() int {
	s.subMu.RLock()
	defer s.subMu.RUnlock()

	return len(s.subscribedThreads)
}

// ChannelSubscriptionCount returns the number of channel subscriptions (thread-safe)
func (s *Session) ChannelSubscriptionCount() int {
	s.subMu.RLock()
	defer s.subMu.RUnlock()

	return len(s.subscribedChannels)
}
