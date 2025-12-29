package server

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"

	"github.com/aeolun/superchat/pkg/database"
	"github.com/aeolun/superchat/pkg/protocol"
)

var (
	nicknameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,20}$`)

	// ErrClientDisconnecting is returned when client sends graceful disconnect
	ErrClientDisconnecting = errors.New("client disconnecting")
)

// encodedFrame holds pre-encoded frame bytes for different protocol versions
type encodedFrame struct {
	v1Bytes []byte // Uncompressed encoding for v1 clients
	v2Bytes []byte // Compressed encoding for v2+ clients (nil if compression didn't help)
}

// encodeFrameVersionAware encodes a frame for both v1 and v2+ clients.
// Returns v1 (uncompressed) and optionally v2 (compressed) encodings.
// v2Bytes will be nil if compression didn't reduce size.
func encodeFrameVersionAware(frame *protocol.Frame) (*encodedFrame, error) {
	// Encode for v1 (no compression)
	var v1Buf bytes.Buffer
	if err := protocol.EncodeFrame(&v1Buf, frame, 1); err != nil {
		return nil, fmt.Errorf("failed to encode v1 frame: %w", err)
	}

	// Encode for v2 (with compression if beneficial)
	var v2Buf bytes.Buffer
	if err := protocol.EncodeFrame(&v2Buf, frame, 2); err != nil {
		return nil, fmt.Errorf("failed to encode v2 frame: %w", err)
	}

	result := &encodedFrame{
		v1Bytes: v1Buf.Bytes(),
	}

	// Only use v2 encoding if it's actually different (compression was applied)
	if v2Buf.Len() < v1Buf.Len() {
		result.v2Bytes = v2Buf.Bytes()
	}

	return result, nil
}

// dbError logs a database error and sends an error response to the client
func (s *Server) dbError(sess *Session, operation string, err error) error {
	errorLog.Printf("Session %d: %s failed: %v", sess.ID, operation, err)
	return s.sendError(sess, 9001, "Database error")
}

func optionalUint64FromInt64Ptr(v *int64) *uint64 {
	if v == nil {
		return nil
	}
	converted := uint64(*v)
	return &converted
}

func (s *Server) buildServerPresenceMessage(sess *Session, online bool) *protocol.ServerPresenceMessage {
	sess.mu.RLock()
	nickname := sess.Nickname
	userID := sess.UserID
	userFlags := sess.UserFlags
	sess.mu.RUnlock()

	if nickname == "" {
		return nil
	}

	msg := &protocol.ServerPresenceMessage{
		SessionID:    sess.ID,
		Nickname:     nickname,
		IsRegistered: userID != nil,
		UserFlags:    protocol.UserFlags(userFlags),
		Online:       online,
	}

	if userID != nil {
		msg.UserID = optionalUint64FromInt64Ptr(userID)
	}

	return msg
}

func (s *Server) notifyServerPresence(sess *Session, online bool) {
	msg := s.buildServerPresenceMessage(sess, online)
	if msg == nil {
		return
	}
	targets := s.sessions.GetAllSessions()
	for _, target := range targets {
		if err := s.sendMessage(target, protocol.TypeServerPresence, msg); err != nil {
			log.Printf("Failed to send SERVER_PRESENCE to session %d: %v", target.ID, err)
			s.removeSession(target.ID)
		}
	}
}

func (s *Server) sendServerPresenceSnapshot(target *Session) {
	sessions := s.sessions.GetAllSessions()
	for _, sess := range sessions {
		if sess.ID == target.ID {
			continue
		}
		msg := s.buildServerPresenceMessage(sess, true)
		if msg == nil {
			continue
		}
		if err := s.sendMessage(target, protocol.TypeServerPresence, msg); err != nil {
			log.Printf("Failed to send SERVER_PRESENCE snapshot to session %d: %v", target.ID, err)
		}
	}
}

func (s *Server) buildChannelPresenceMessage(channelID int64, subchannelID *uint64, sess *Session, joined bool) *protocol.ChannelPresenceMessage {
	sess.mu.RLock()
	nickname := sess.Nickname
	userID := sess.UserID
	userFlags := sess.UserFlags
	sess.mu.RUnlock()

	if nickname == "" {
		return nil
	}

	msg := &protocol.ChannelPresenceMessage{
		ChannelID:    uint64(channelID),
		SubchannelID: subchannelID,
		SessionID:    sess.ID,
		Nickname:     nickname,
		IsRegistered: userID != nil,
		UserFlags:    protocol.UserFlags(userFlags),
		Joined:       joined,
	}

	if userID != nil {
		msg.UserID = optionalUint64FromInt64Ptr(userID)
	}

	return msg
}

func (s *Server) notifyChannelPresence(channelID int64, sess *Session, joined bool) {
	msg := s.buildChannelPresenceMessage(channelID, nil, sess, joined)
	if msg == nil {
		return
	}
	if err := s.broadcastToChannel(channelID, protocol.TypeChannelPresence, msg); err != nil {
		log.Printf("Failed to broadcast CHANNEL_PRESENCE for channel %d: %v", channelID, err)
	}
	if !joined {
		// Leaving sessions won't receive the broadcast (they are removed before send). Send directly.
		if err := s.sendMessage(sess, protocol.TypeChannelPresence, msg); err != nil {
			log.Printf("Failed to send CHANNEL_PRESENCE to leaving session %d: %v", sess.ID, err)
		}
	}
}

// handleAuthRequest handles AUTH_REQUEST message (login)
func (s *Server) handleAuthRequest(sess *Session, frame *protocol.Frame) error {
	// Decode message
	msg := &protocol.AuthRequestMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		log.Printf("Session %d: AUTH_REQUEST decode failed: %v", sess.ID, err)
		return s.sendError(sess, 1000, "Invalid message format")
	}

	log.Printf("Session %d: AUTH_REQUEST for nickname %s", sess.ID, msg.Nickname)

	// Get user from database
	user, err := s.db.GetUserByNickname(msg.Nickname)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Session %d: AUTH_REQUEST failed - nickname %s not registered", sess.ID, msg.Nickname)
			resp := &protocol.AuthResponseMessage{
				Success: false,
				Message: "Invalid credentials",
			}
			return s.sendMessage(sess, protocol.TypeAuthResponse, resp)
		}
		return s.dbError(sess, "GetUserByNickname", err)
	}

	// Check if user has removed password (SSH-only authentication)
	if user.PasswordHash == "" {
		log.Printf("Session %d: AUTH_REQUEST failed - user %s requires SSH authentication", sess.ID, msg.Nickname)
		resp := &protocol.AuthResponseMessage{
			Success: false,
			Message: "This account requires SSH authentication. Please connect via SSH.",
		}
		return s.sendMessage(sess, protocol.TypeAuthResponse, resp)
	}

	// Verify password hash
	// msg.Password contains the client-side argon2id hash
	// user.PasswordHash contains bcrypt(client_hash)
	// So we compare: bcrypt(stored_hash, client_hash)
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(msg.Password))
	if err != nil {
		log.Printf("Session %d: AUTH_REQUEST failed - password verification failed for user %s (client_hash_len=%d)", sess.ID, msg.Nickname, len(msg.Password))
		resp := &protocol.AuthResponseMessage{
			Success: false,
			Message: "Invalid credentials",
		}
		return s.sendMessage(sess, protocol.TypeAuthResponse, resp)
	}

	// Check if user is banned
	ban, err := s.db.GetActiveBanForUser(&user.ID, &user.Nickname)
	if err != nil {
		log.Printf("Session %d: failed to check ban status: %v", sess.ID, err)
		// Continue with login - don't block on ban check failures
	}

	if ban != nil {
		// If shadowban, allow login but mark session (filtering happens during broadcasts)
		if ban.Shadowban {
			log.Printf("Session %d: user %s (id=%d) is shadowbanned", sess.ID, user.Nickname, user.ID)
			// Continue with login - shadowban is enforced during message broadcasting
		} else {
			// Regular ban - reject authentication
			bannedUntil := "permanently"
			if ban.BannedUntil != nil {
				bannedUntil = fmt.Sprintf("until %s", time.Unix(*ban.BannedUntil/1000, 0).Format(time.RFC3339))
			}
			resp := &protocol.AuthResponseMessage{
				Success: false,
				Message: fmt.Sprintf("Account banned %s. Reason: %s", bannedUntil, ban.Reason),
			}
			log.Printf("Session %d: rejected login for banned user %s (id=%d)", sess.ID, user.Nickname, user.ID)
			return s.sendMessage(sess, protocol.TypeAuthResponse, resp)
		}
	}

	// Update session with user ID and flags
	sess.mu.Lock()
	sess.UserID = &user.ID
	sess.Nickname = user.Nickname
	sess.UserFlags = user.UserFlags
	sess.Shadowbanned = ban != nil && ban.Shadowban // Mark session as shadowbanned
	sess.mu.Unlock()

	// Update database session
	if err := s.db.UpdateSessionUserID(sess.DBSessionID, user.ID); err != nil {
		log.Printf("Session %d: failed to update session user_id: %v", sess.ID, err)
	}

	// Update last_seen
	if err := s.db.UpdateUserLastSeen(user.ID); err != nil {
		log.Printf("Session %d: failed to update user last_seen: %v", sess.ID, err)
	}

	// Send success response
	log.Printf("Session %d: AUTH_REQUEST succeeded for user %s (id=%d)", sess.ID, user.Nickname, user.ID)
	flags := protocol.UserFlags(user.UserFlags)
	resp := &protocol.AuthResponseMessage{
		Success:   true,
		UserID:    uint64(user.ID),
		Nickname:  user.Nickname,
		Message:   fmt.Sprintf("Welcome back, %s!", user.Nickname),
		UserFlags: &flags,
	}
	if err := s.sendMessage(sess, protocol.TypeAuthResponse, resp); err != nil {
		return err
	}
	s.sendServerPresenceSnapshot(sess)
	s.notifyServerPresence(sess, true)
	return nil
}

// handleRegisterUser handles REGISTER_USER message
func (s *Server) handleRegisterUser(sess *Session, frame *protocol.Frame) error {
	// Decode message
	msg := &protocol.RegisterUserMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Check if session has a nickname set
	sess.mu.RLock()
	nickname := sess.Nickname
	sess.mu.RUnlock()

	if nickname == "" {
		resp := &protocol.RegisterResponseMessage{
			Success: false,
			Message: "Must set nickname before registering",
		}
		return s.sendMessage(sess, protocol.TypeRegisterResponse, resp)
	}

	// Validate client hash (should be 43 characters for argon2id base64-encoded 32 bytes)
	// Note: msg.Password actually contains the client-side argon2id hash
	if len(msg.Password) < 40 || len(msg.Password) > 50 {
		resp := &protocol.RegisterResponseMessage{
			Success: false,
			Message: "Invalid password hash format",
		}
		return s.sendMessage(sess, protocol.TypeRegisterResponse, resp)
	}

	// Double-hash: bcrypt the client hash for storage
	// This provides defense-in-depth against database breaches
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(msg.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Session %d: bcrypt.GenerateFromPassword failed: %v", sess.ID, err)
		return s.sendError(sess, 9000, "Failed to hash password")
	}

	// Create user in database
	userID, err := s.db.CreateUser(nickname, string(hashedPassword), 0) // 0 = no special flags
	if err != nil {
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			resp := &protocol.RegisterResponseMessage{
				Success: false,
				Message: "Nickname already registered",
			}
			return s.sendMessage(sess, protocol.TypeRegisterResponse, resp)
		}
		return s.dbError(sess, "CreateUser", err)
	}

	// Update session with user ID
	sess.mu.Lock()
	sess.UserID = &userID
	sess.UserFlags = 0 // Regular user
	sess.mu.Unlock()

	// Update database session
	if err := s.db.UpdateSessionUserID(sess.DBSessionID, userID); err != nil {
		log.Printf("Session %d: failed to update session user_id: %v", sess.ID, err)
	}

	// Broadcast updated presence to all clients
	s.notifyServerPresence(sess, true)

	// Send success response
	resp := &protocol.RegisterResponseMessage{
		Success: true,
		UserID:  uint64(userID),
		Message: fmt.Sprintf("Successfully registered %s!", nickname),
	}
	return s.sendMessage(sess, protocol.TypeRegisterResponse, resp)
}

// handleLogout handles LOGOUT message
func (s *Server) handleLogout(sess *Session, frame *protocol.Frame) error {
	log.Printf("Session %d: LOGOUT request received", sess.ID)

	// Decode message (empty, but verify payload is valid)
	msg := &protocol.LogoutMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		log.Printf("Session %d: LOGOUT decode failed: %v", sess.ID, err)
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Clear the session's authentication
	sess.mu.Lock()
	oldUserID := sess.UserID
	sess.UserID = nil
	sess.mu.Unlock()

	if oldUserID != nil {
		log.Printf("Session %d: Logged out (was user_id=%d), now anonymous with nickname %s", sess.ID, *oldUserID, sess.Nickname)
	} else {
		log.Printf("Session %d: LOGOUT received but already anonymous", sess.ID)
	}

	// No response message - silent success
	return nil
}

// handleSetNickname handles SET_NICKNAME message
func (s *Server) handleSetNickname(sess *Session, frame *protocol.Frame) error {
	// Decode message
	msg := &protocol.SetNicknameMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Validate nickname
	if !nicknameRegex.MatchString(msg.Nickname) {
		resp := &protocol.NicknameResponseMessage{
			Success: false,
			Message: "Invalid nickname. Must be 3-20 characters, alphanumeric plus - and _",
		}
		return s.sendMessage(sess, protocol.TypeNicknameResponse, resp)
	}

	// Check if nickname is registered
	existingUser, err := s.db.GetUserByNickname(msg.Nickname)
	isRegistered := (err == nil)

	// If nickname is registered and session is not authenticated as that user
	if isRegistered && (sess.UserID == nil || *sess.UserID != existingUser.ID) {
		resp := &protocol.NicknameResponseMessage{
			Success: false,
			Message: "Nickname registered, password required",
		}
		return s.sendMessage(sess, protocol.TypeNicknameResponse, resp)
	}

	// Determine if this is a change or initial set
	oldNickname := sess.Nickname
	isChange := oldNickname != "" && oldNickname != msg.Nickname

	// For registered users changing nickname, update database
	if sess.UserID != nil && isChange {
		if err := s.db.UpdateUserNickname(*sess.UserID, msg.Nickname); err != nil {
			log.Printf("Session %d: UpdateUserNickname failed: %v", sess.ID, err)
			resp := &protocol.NicknameResponseMessage{
				Success: false,
				Message: "Nickname already in use",
			}
			return s.sendMessage(sess, protocol.TypeNicknameResponse, resp)
		}
	}

	// Update session nickname
	if err := s.sessions.UpdateNickname(sess.ID, msg.Nickname); err != nil {
		log.Printf("Session %d: UpdateNickname failed: %v", sess.ID, err)
		return s.sendError(sess, 9000, "Failed to update nickname")
	}

	// Send success response
	var message string
	if isChange {
		message = fmt.Sprintf("Nickname changed to %s", msg.Nickname)
	} else {
		message = fmt.Sprintf("Nickname set to %s", msg.Nickname)
	}

	resp := &protocol.NicknameResponseMessage{
		Success: true,
		Message: message,
	}
	if err := s.sendMessage(sess, protocol.TypeNicknameResponse, resp); err != nil {
		return err
	}

	if oldNickname == "" {
		s.sendServerPresenceSnapshot(sess)
	}
	s.notifyServerPresence(sess, true)
	sess.mu.RLock()
	joined := sess.JoinedChannel
	sess.mu.RUnlock()
	if joined != nil {
		s.notifyChannelPresence(*joined, sess, true)
	}

	return nil
}

// handleListChannels handles LIST_CHANNELS message
func (s *Server) handleListChannels(sess *Session, frame *protocol.Frame) error {
	// Decode message
	msg := &protocol.ListChannelsMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Get channels from MemDB (already in memory, instant)
	dbChannels, err := s.db.ListChannels()
	if err != nil {
		return s.sendError(sess, 1002, "Failed to list channels")
	}

	// Apply pagination
	channelList := make([]protocol.Channel, 0, len(dbChannels))
	for _, dbCh := range dbChannels {
		// Skip DM channels - they're handled separately
		if dbCh.IsDM {
			continue
		}

		// Skip channels before cursor
		if msg.FromChannelID > 0 && uint64(dbCh.ID) <= msg.FromChannelID {
			continue
		}

		channelSub := ChannelSubscription{ChannelID: uint64(dbCh.ID)}
		userCount := uint32(len(s.sessions.GetChannelSubscribers(channelSub)))

		// Get subchannel count for V3
		subchannelCount, _ := s.db.GetSubchannelCount(dbCh.ID)

		// Convert to protocol format
		ch := protocol.Channel{
			ID:              uint64(dbCh.ID),
			Name:            dbCh.Name,
			Description:     safeDeref(dbCh.Description, ""),
			UserCount:       userCount,
			IsOperator:      false,
			Type:            dbCh.ChannelType,
			RetentionHours:  dbCh.MessageRetentionHours,
			HasSubchannels:  subchannelCount > 0,
			SubchannelCount: uint16(subchannelCount),
		}
		channelList = append(channelList, ch)

		// Stop if we've reached the limit
		if msg.Limit > 0 && len(channelList) >= int(msg.Limit) {
			break
		}
	}

	// Send response
	resp := &protocol.ChannelListMessage{
		Channels: channelList,
	}

	return s.sendMessage(sess, protocol.TypeChannelList, resp)
}

// handleJoinChannel handles JOIN_CHANNEL message
func (s *Server) handleJoinChannel(sess *Session, frame *protocol.Frame) error {
	// Decode message
	msg := &protocol.JoinChannelMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Check if channel exists
	channel, err := s.db.GetChannel(int64(msg.ChannelID))
	if err != nil || channel == nil {
		errorLog.Printf("Session %d: Channel %d not found", sess.ID, msg.ChannelID)
		resp := &protocol.JoinResponseMessage{
			Success:      false,
			ChannelID:    msg.ChannelID,
			SubchannelID: nil,
			Message:      "Channel not found",
		}
		return s.sendMessage(sess, protocol.TypeJoinResponse, resp)
	}

	// For DM channels, check that the user is a participant
	if channel.IsDM {
		sess.mu.RLock()
		userID := sess.UserID
		sessionID := sess.DBSessionID
		sess.mu.RUnlock()

		isParticipant, err := s.db.IsChannelParticipant(channel.ID, userID, sessionID)
		if err != nil || !isParticipant {
			log.Printf("[DM] Session %d tried to join DM channel %d but is not a participant", sess.ID, channel.ID)
			resp := &protocol.JoinResponseMessage{
				Success:      false,
				ChannelID:    msg.ChannelID,
				SubchannelID: nil,
				Message:      "Not a participant in this DM",
			}
			return s.sendMessage(sess, protocol.TypeJoinResponse, resp)
		}
	}

	sess.mu.RLock()
	previousJoined := sess.JoinedChannel
	sess.mu.RUnlock()
	channelID := int64(msg.ChannelID)
	if previousJoined != nil && *previousJoined != channelID {
		s.notifyChannelPresence(*previousJoined, sess, false)
	}

	// Update session's joined channel
	if err := s.sessions.SetJoinedChannel(sess.ID, &channelID); err != nil {
		return s.sendError(sess, 9000, "Failed to join channel")
	}

	// Send success response
	resp := &protocol.JoinResponseMessage{
		Success:      true,
		ChannelID:    msg.ChannelID,
		SubchannelID: msg.SubchannelID,
		Message:      "Joined channel",
	}

	if err := s.sendMessage(sess, protocol.TypeJoinResponse, resp); err != nil {
		return err
	}

	s.notifyChannelPresence(channelID, sess, true)
	return nil
}

// handleLeaveChannel handles LEAVE_CHANNEL message
func (s *Server) handleLeaveChannel(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.LeaveChannelMessage{}
	if len(frame.Payload) > 0 {
		if err := msg.Decode(frame.Payload); err != nil {
			return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
		}
	}

	sess.mu.RLock()
	current := sess.JoinedChannel
	sess.mu.RUnlock()
	if current == nil {
		resp := &protocol.LeaveResponseMessage{
			Success:   false,
			ChannelID: msg.ChannelID,
			Message:   "Not currently joined to any channel",
		}
		return s.sendMessage(sess, protocol.TypeLeaveResponse, resp)
	}

	targetChannelID := *current
	if msg.ChannelID != 0 && int64(msg.ChannelID) != targetChannelID {
		resp := &protocol.LeaveResponseMessage{
			Success:   false,
			ChannelID: msg.ChannelID,
			Message:   "Session is not joined to the requested channel",
		}
		return s.sendMessage(sess, protocol.TypeLeaveResponse, resp)
	}

	if err := s.sessions.SetJoinedChannel(sess.ID, nil); err != nil {
		return s.sendError(sess, 9000, "Failed to leave channel")
	}

	// For DM channels with permanent=true, remove the participant and notify others
	if msg.Permanent {
		channel, err := s.db.GetChannel(targetChannelID)
		if err == nil && channel != nil && channel.IsDM {
			sess.mu.RLock()
			userID := sess.UserID
			sessionID := sess.DBSessionID
			nickname := sess.Nickname
			sess.mu.RUnlock()

			// Get other participants BEFORE removing the current one
			participants, _ := s.db.GetChannelParticipants(targetChannelID)

			// Remove participant based on user ID (registered) or session ID (anonymous)
			if userID != nil {
				s.db.RemoveParticipantByUserID(targetChannelID, *userID)
				log.Printf("[DM] Permanently removed registered user %d from DM channel %d", *userID, targetChannelID)
			} else {
				s.db.RemoveParticipantBySessionID(targetChannelID, sessionID)
				log.Printf("[DM] Permanently removed anonymous session %d from DM channel %d", sessionID, targetChannelID)
			}

			// Create a system message so it persists in history
			systemContent := fmt.Sprintf("%s has left the conversation", nickname)
			s.db.CreateSystemMessage(targetChannelID, systemContent)

			// Notify other participants that this user has left (real-time)
			leftMsg := &protocol.DMParticipantLeftMessage{
				DMChannelID: uint64(targetChannelID),
				UserID:      toUint64Ptr(userID),
				Nickname:    nickname,
			}
			for _, p := range participants {
				// Skip the leaving user
				if userID != nil && p.UserID != nil && *p.UserID == *userID {
					continue
				}
				if userID == nil && p.SessionID != nil && *p.SessionID == sessionID {
					continue
				}

				// Send to participant by user ID (registered) or session ID (anonymous)
				if p.UserID != nil {
					s.sendToUser(*p.UserID, protocol.TypeDMParticipantLeft, leftMsg)
				} else if p.SessionID != nil {
					if otherSess, ok := s.sessions.GetSessionByDBID(*p.SessionID); ok {
						s.sendMessage(otherSess, protocol.TypeDMParticipantLeft, leftMsg)
					}
				}
			}

			// If no participants remain, delete the DM channel entirely
			remainingParticipants, _ := s.db.GetChannelParticipants(targetChannelID)
			if len(remainingParticipants) == 0 {
				if err := s.db.DeleteChannel(uint64(targetChannelID)); err != nil {
					log.Printf("[DM] Failed to delete empty DM channel %d: %v", targetChannelID, err)
				} else {
					log.Printf("[DM] Deleted empty DM channel %d", targetChannelID)
				}
			}
		}
	}

	resp := &protocol.LeaveResponseMessage{
		Success:   true,
		ChannelID: uint64(targetChannelID),
		Message:   "Left channel",
	}
	if err := s.sendMessage(sess, protocol.TypeLeaveResponse, resp); err != nil {
		return err
	}

	s.notifyChannelPresence(targetChannelID, sess, false)
	return nil
}

// handleCreateChannel handles CREATE_CHANNEL message (V2+)
func (s *Server) handleCreateChannel(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.CreateChannelMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendMessage(sess, protocol.TypeChannelCreated, &protocol.ChannelCreatedMessage{
			Success: false,
			Message: "Invalid request format",
		})
	}

	// V2 feature: Only registered users can create channels
	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	if userID == nil {
		return s.sendMessage(sess, protocol.TypeChannelCreated, &protocol.ChannelCreatedMessage{
			Success: false,
			Message: "Only registered users can create channels. Please register or log in.",
		})
	}

	// Validate channel name (must be URL-friendly)
	if len(msg.Name) < 3 || len(msg.Name) > 50 {
		return s.sendMessage(sess, protocol.TypeChannelCreated, &protocol.ChannelCreatedMessage{
			Success: false,
			Message: "Channel name must be 3-50 characters",
		})
	}

	// Validate display name
	if len(msg.DisplayName) < 1 || len(msg.DisplayName) > 100 {
		return s.sendMessage(sess, protocol.TypeChannelCreated, &protocol.ChannelCreatedMessage{
			Success: false,
			Message: "Display name must be 1-100 characters",
		})
	}

	// Validate description (optional, max 500 chars)
	if msg.Description != nil && len(*msg.Description) > 500 {
		return s.sendMessage(sess, protocol.TypeChannelCreated, &protocol.ChannelCreatedMessage{
			Success: false,
			Message: "Description must be at most 500 characters",
		})
	}

	// Validate channel type (0=chat, 1=forum)
	if msg.ChannelType != 0 && msg.ChannelType != 1 {
		return s.sendMessage(sess, protocol.TypeChannelCreated, &protocol.ChannelCreatedMessage{
			Success: false,
			Message: "Invalid channel type (must be 0=chat or 1=forum)",
		})
	}

	// Validate retention hours (1 hour to 1 year)
	if msg.RetentionHours < 1 || msg.RetentionHours > 8760 {
		return s.sendMessage(sess, protocol.TypeChannelCreated, &protocol.ChannelCreatedMessage{
			Success: false,
			Message: "Retention hours must be between 1 and 8760 (1 year)",
		})
	}

	// Create channel in database
	channelID, err := s.db.CreateChannel(msg.Name, msg.DisplayName, msg.Description, msg.ChannelType, msg.RetentionHours, userID)
	if err != nil {
		// Check if it's a duplicate name error
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "already exists") {
			return s.sendMessage(sess, protocol.TypeChannelCreated, &protocol.ChannelCreatedMessage{
				Success: false,
				Message: "Channel name already exists",
			})
		}
		return s.dbError(sess, "CreateChannel", err)
	}

	// Build CHANNEL_CREATED message (hybrid response + broadcast)
	channelCreatedMsg := &protocol.ChannelCreatedMessage{
		Success:        true,
		ChannelID:      uint64(channelID),
		Name:           msg.Name,
		Description:    safeDeref(msg.Description, ""),
		Type:           msg.ChannelType,
		RetentionHours: msg.RetentionHours,
		Message:        fmt.Sprintf("Channel '%s' created successfully", msg.DisplayName),
	}

	// Send to creator as confirmation
	if err := s.sendMessage(sess, protocol.TypeChannelCreated, channelCreatedMsg); err != nil {
		return err
	}

	// Construct channel object for broadcast (we have all the data)
	now := time.Now().UnixMilli()
	createdChannel := &database.Channel{
		ID:                    channelID,
		Name:                  msg.Name,
		DisplayName:           msg.DisplayName,
		Description:           msg.Description,
		ChannelType:           msg.ChannelType,
		MessageRetentionHours: msg.RetentionHours,
		CreatedBy:             userID,
		CreatedAt:             now,
		IsPrivate:             false,
	}

	// Broadcast to all OTHER connected users (not the creator again)
	s.broadcastChannelCreated(createdChannel, sess.ID)

	return nil
}

// handleCreateSubchannel handles CREATE_SUBCHANNEL message
func (s *Server) handleCreateSubchannel(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.CreateSubchannelMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
			Success: false,
			Message: "Invalid request format",
		})
	}

	// V3 feature: Only registered users can create subchannels
	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	if userID == nil {
		return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
			Success: false,
			Message: "Only registered users can create subchannels. Please register or log in.",
		})
	}

	// Verify parent channel exists
	parentChannel, err := s.db.GetChannel(int64(msg.ChannelID))
	if err != nil {
		return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
			Success: false,
			Message: "Parent channel not found",
		})
	}

	// Check permission: must be channel owner or server admin
	isOwner := parentChannel.CreatedBy != nil && *parentChannel.CreatedBy == *userID
	isAdmin := s.isAdmin(sess)
	if !isOwner && !isAdmin {
		return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
			Success: false,
			Message: "Only the channel owner or admins can create subchannels",
		})
	}

	// Don't allow creating sub-subchannels (only one level of nesting)
	if parentChannel.ParentID != nil {
		return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
			Success: false,
			Message: "Cannot create subchannels within subchannels (only one level of nesting allowed)",
		})
	}

	// Validate subchannel name (must be URL-friendly)
	if len(msg.Name) < 3 || len(msg.Name) > 50 {
		return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
			Success: false,
			Message: "Subchannel name must be 3-50 characters",
		})
	}

	// Validate description (max 500 chars)
	if len(msg.Description) > 500 {
		return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
			Success: false,
			Message: "Description must be at most 500 characters",
		})
	}

	// Validate channel type (0=chat, 1=forum)
	if msg.Type != 0 && msg.Type != 1 {
		return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
			Success: false,
			Message: "Invalid channel type (must be 0=chat or 1=forum)",
		})
	}

	// Validate retention hours (1 hour to 1 year)
	if msg.RetentionHours < 1 || msg.RetentionHours > 8760 {
		return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
			Success: false,
			Message: "Retention hours must be between 1 and 8760 (1 year)",
		})
	}

	// Create subchannel display name (prefix with parent name)
	displayName := fmt.Sprintf("%s/%s", parentChannel.DisplayName, msg.Name)

	// Create subchannel in database
	var descPtr *string
	if msg.Description != "" {
		descPtr = &msg.Description
	}
	subchannelID, err := s.db.CreateSubchannel(int64(msg.ChannelID), msg.Name, displayName, descPtr, msg.Type, msg.RetentionHours, userID)
	if err != nil {
		// Check if it's a duplicate name error
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "already exists") {
			return s.sendMessage(sess, protocol.TypeSubchannelCreated, &protocol.SubchannelCreatedMessage{
				Success: false,
				Message: "Subchannel name already exists in this channel",
			})
		}
		return s.dbError(sess, "CreateSubchannel", err)
	}

	// Build SUBCHANNEL_CREATED message (hybrid response + broadcast)
	subchannelCreatedMsg := &protocol.SubchannelCreatedMessage{
		Success:        true,
		ChannelID:      msg.ChannelID,
		SubchannelID:   uint64(subchannelID),
		Name:           msg.Name,
		Description:    msg.Description,
		Type:           msg.Type,
		RetentionHours: msg.RetentionHours,
		Message:        fmt.Sprintf("Subchannel '%s' created successfully", msg.Name),
	}

	// Send to creator as confirmation
	if err := s.sendMessage(sess, protocol.TypeSubchannelCreated, subchannelCreatedMsg); err != nil {
		return err
	}

	// Broadcast to all OTHER connected users (not the creator again)
	s.broadcastSubchannelCreated(subchannelCreatedMsg, sess.ID)

	return nil
}

// handleGetSubchannels handles GET_SUBCHANNELS message
func (s *Server) handleGetSubchannels(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.GetSubchannelsMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid request format")
	}

	// Verify parent channel exists
	_, err := s.db.GetChannel(int64(msg.ChannelID))
	if err != nil {
		return s.sendError(sess, protocol.ErrCodeChannelNotFound, "Channel not found")
	}

	// Get subchannels
	subchannels, err := s.db.GetSubchannels(int64(msg.ChannelID))
	if err != nil {
		return s.dbError(sess, "GetSubchannels", err)
	}

	// Build response
	subchannelInfos := make([]protocol.SubchannelInfo, len(subchannels))
	for i, sub := range subchannels {
		desc := ""
		if sub.Description != nil {
			desc = *sub.Description
		}
		subchannelInfos[i] = protocol.SubchannelInfo{
			ID:             uint64(sub.ID),
			Name:           sub.Name,
			Description:    desc,
			Type:           sub.ChannelType,
			RetentionHours: sub.MessageRetentionHours,
		}
	}

	return s.sendMessage(sess, protocol.TypeSubchannelList, &protocol.SubchannelListMessage{
		ChannelID:   msg.ChannelID,
		Subchannels: subchannelInfos,
	})
}

// broadcastSubchannelCreated broadcasts a subchannel creation to all connected users
func (s *Server) broadcastSubchannelCreated(msg *protocol.SubchannelCreatedMessage, excludeSessionID uint64) {
	sessions := s.sessions.GetAllSessions()
	for _, target := range sessions {
		if target.ID == excludeSessionID {
			continue
		}
		if err := s.sendMessage(target, protocol.TypeSubchannelCreated, msg); err != nil {
			log.Printf("Failed to broadcast SUBCHANNEL_CREATED to session %d: %v", target.ID, err)
		}
	}
}

// handleListMessages handles LIST_MESSAGES message
func (s *Server) handleListMessages(sess *Session, frame *protocol.Frame) error {
	// Decode message
	msg := &protocol.ListMessagesMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	var messages []protocol.Message

	if msg.ParentID != nil {
		// Get thread replies
		dbMessages, err := s.db.ListThreadReplies(*msg.ParentID, msg.Limit, msg.BeforeID, msg.AfterID)
		if err != nil {
			return s.dbError(sess, "ListThreadReplies", err)
		}
		messages = convertDBMessagesToProtocol(dbMessages, s.db)
	} else {
		// Get root messages
		var subchannelID *int64
		if msg.SubchannelID != nil {
			id := int64(*msg.SubchannelID)
			subchannelID = &id
		}

		dbMessages, err := s.db.ListRootMessages(int64(msg.ChannelID), subchannelID, msg.Limit, msg.BeforeID, msg.AfterID)
		if err != nil {
			return s.dbError(sess, "ListRootMessages", err)
		}
		messages = convertDBMessagesToProtocol(dbMessages, s.db)
	}

	// Send response
	resp := &protocol.MessageListMessage{
		ChannelID:    msg.ChannelID,
		SubchannelID: msg.SubchannelID,
		ParentID:     msg.ParentID,
		Messages:     messages,
	}

	return s.sendMessage(sess, protocol.TypeMessageList, resp)
}

// handlePostMessage handles POST_MESSAGE message
func (s *Server) handlePostMessage(sess *Session, frame *protocol.Frame) error {
	// Decode message
	msg := &protocol.PostMessageMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Check if session has a nickname
	sess.mu.RLock()
	nickname := sess.Nickname
	sess.mu.RUnlock()

	if nickname == "" {
		log.Printf("Session %d tried to POST without nickname set", sess.ID)
		return s.sendError(sess, 2000, "Nickname required. Use SET_NICKNAME first.")
	}

	// Validate message length
	if uint32(len(msg.Content)) > s.config.MaxMessageLength {
		return s.sendError(sess, 6001, fmt.Sprintf("Message too long (max %d bytes)", s.config.MaxMessageLength))
	}

	// Convert IDs
	var subchannelID, parentID *int64
	if msg.SubchannelID != nil {
		id := int64(*msg.SubchannelID)
		subchannelID = &id
	}
	if msg.ParentID != nil {
		id := int64(*msg.ParentID)
		parentID = &id
	}

	// Get channel and check access
	channel, err := s.db.GetChannel(int64(msg.ChannelID))
	if err != nil {
		return s.dbError(sess, "GetChannel", err)
	}

	// For DM channels, verify sender is a participant
	if channel.IsDM {
		sess.mu.RLock()
		userID := sess.UserID
		sessionID := sess.DBSessionID
		sess.mu.RUnlock()

		isParticipant, err := s.db.IsChannelParticipant(channel.ID, userID, sessionID)
		if err != nil || !isParticipant {
			log.Printf("[DM] Session %d tried to post to DM channel %d but is not a participant", sess.ID, channel.ID)
			return s.sendError(sess, protocol.ErrCodePermissionDenied, "Not a participant in this DM")
		}
	}

	if channel.ChannelType == 0 && parentID != nil {
		return s.sendError(sess, 6000, "Chat channels do not support threaded replies")
	}

	// Post message to in-memory database (instant)
	messageID, dbMsg, err := s.db.PostMessage(
		int64(msg.ChannelID),
		subchannelID,
		parentID,
		sess.UserID,
		nickname,
		msg.Content,
	)

	if err != nil {
		return s.dbError(sess, "PostMessage", err)
	}

	// Send confirmation
	resp := &protocol.MessagePostedMessage{
		Success:   true,
		MessageID: uint64(messageID),
		Message:   "Message posted",
	}

	if err := s.sendMessage(sess, protocol.TypeMessagePosted, resp); err != nil {
		return err
	}

	// Broadcast NEW_MESSAGE to subscribed sessions
	newMsg := convertDBMessageToProtocol(dbMsg, s.db)
	broadcastMsg := (*protocol.NewMessageMessage)(newMsg)

	// Use thread_root_id from database (server owns thread hierarchy)
	var threadRootID *uint64
	if dbMsg.ThreadRootID != nil {
		id := uint64(*dbMsg.ThreadRootID)
		threadRootID = &id
	}

	if err := s.broadcastNewMessage(sess, broadcastMsg, threadRootID); err != nil {
		// Log but don't fail - message was posted successfully
		fmt.Printf("Failed to broadcast new message: %v\n", err)
	}

	return nil
}

// handleEditMessage handles EDIT_MESSAGE message (V2, registered users only)
func (s *Server) handleEditMessage(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.EditMessageMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Check if user is registered (anonymous users cannot edit)
	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	if userID == nil {
		log.Printf("Session %d (anonymous) tried to EDIT_MESSAGE", sess.ID)
		return s.sendError(sess, protocol.ErrCodeAuthRequired, "Authentication required. Register to edit messages.")
	}

	// Validate message length
	if uint32(len(msg.NewContent)) > s.config.MaxMessageLength {
		return s.sendError(sess, protocol.ErrCodeMessageTooLong, fmt.Sprintf("Message too long (max %d bytes)", s.config.MaxMessageLength))
	}

	// Check if user is admin - admins can edit any message
	isAdmin := s.isAdmin(sess)

	// Update message in database
	var dbMsg *database.Message
	var err error

	if isAdmin {
		// Admin edit: bypass ownership check
		dbMsg, err = s.db.AdminUpdateMessage(msg.MessageID, uint64(*userID), msg.NewContent)
	} else {
		// Regular edit: check ownership
		dbMsg, err = s.db.UpdateMessage(msg.MessageID, uint64(*userID), msg.NewContent)
	}
	if err != nil {
		switch {
		case errors.Is(err, database.ErrMessageNotFound):
			return s.sendError(sess, protocol.ErrCodeMessageNotFound, "Message not found")
		case errors.Is(err, database.ErrMessageNotOwned):
			return s.sendError(sess, protocol.ErrCodePermissionDenied, "You can only edit your own messages")
		case err.Error() == "cannot edit anonymous messages":
			return s.sendError(sess, protocol.ErrCodePermissionDenied, "Cannot edit anonymous messages")
		case err.Error() == "cannot edit deleted message":
			return s.sendError(sess, protocol.ErrCodeInvalidInput, "Cannot edit deleted message")
		default:
			return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to edit message")
		}
	}

	// EditedAt should always be set by UpdateMessage
	editedAtMs := safeDeref(dbMsg.EditedAt, time.Now().UnixMilli())
	editedAt := time.UnixMilli(editedAtMs)

	// Send confirmation to editing user
	resp := &protocol.MessageEditedMessage{
		Success:    true,
		MessageID:  msg.MessageID,
		EditedAt:   editedAt,
		NewContent: msg.NewContent,
		Message:    "",
	}

	if err := s.sendMessage(sess, protocol.TypeMessageEdited, resp); err != nil {
		return err
	}

	// Broadcast MESSAGE_EDITED to all users in the channel
	if err := s.broadcastToChannel(dbMsg.ChannelID, protocol.TypeMessageEdited, resp); err != nil {
		log.Printf("Failed to broadcast message edit: %v", err)
	}

	return nil
}

// handleDeleteMessage handles DELETE_MESSAGE message
func (s *Server) handleDeleteMessage(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.DeleteMessageMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	sess.mu.RLock()
	nickname := sess.Nickname
	sess.mu.RUnlock()

	if nickname == "" {
		return s.sendError(sess, protocol.ErrCodeNicknameRequired, "Nickname required. Use SET_NICKNAME first.")
	}

	// Check if user is admin - admins can delete any message
	isAdmin := s.isAdmin(sess)

	var dbMsg *database.Message
	var err error

	if isAdmin {
		// Admin delete: bypass ownership check
		dbMsg, err = s.db.AdminSoftDeleteMessage(msg.MessageID, nickname)
	} else {
		// Regular delete: check ownership
		dbMsg, err = s.db.SoftDeleteMessage(msg.MessageID, nickname)
	}

	if err != nil {
		switch {
		case errors.Is(err, database.ErrMessageNotFound):
			return s.sendError(sess, protocol.ErrCodeMessageNotFound, "Message not found")
		case errors.Is(err, database.ErrMessageNotOwned):
			return s.sendError(sess, protocol.ErrCodePermissionDenied, "You can only delete your own messages")
		case errors.Is(err, database.ErrMessageAlreadyDeleted):
			return s.sendError(sess, protocol.ErrCodeInvalidInput, "Message already deleted")
		default:
			return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to delete message")
		}
	}

	// DeletedAt should always be set by SoftDeleteMessage, but add defensive check
	deletedAtMs := safeDeref(dbMsg.DeletedAt, time.Now().UnixMilli())
	deletedAt := time.UnixMilli(deletedAtMs)
	resp := &protocol.MessageDeletedMessage{
		Success:   true,
		MessageID: msg.MessageID,
		DeletedAt: deletedAt,
		Message:   dbMsg.Content,
	}

	if err := s.sendMessage(sess, protocol.TypeMessageDeleted, resp); err != nil {
		return err
	}

	if err := s.broadcastToChannel(dbMsg.ChannelID, protocol.TypeMessageDeleted, resp); err != nil {
		log.Printf("Failed to broadcast message deletion: %v", err)
	}

	return nil
}

// handleChangePassword handles CHANGE_PASSWORD message (V2 feature)
func (s *Server) handleChangePassword(sess *Session, frame *protocol.Frame) error {
	// Must be authenticated
	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	if userID == nil {
		return s.sendError(sess, protocol.ErrCodeAuthRequired, "Must be authenticated to change password")
	}

	// Decode request
	req := &protocol.ChangePasswordRequest{}
	if err := req.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid change password request")
	}

	// Get user from database
	user, err := s.db.GetUserByID(*userID)
	if err != nil {
		log.Printf("Failed to get user %d for password change: %v", *userID, err)
		return s.sendPasswordChanged(sess, false, "User not found")
	}

	// Verify old password hash (skip if user has no password set - SSH-registered)
	// req.OldPassword contains client-side argon2id hash (or empty for SSH users)
	if user.PasswordHash != "" && req.OldPassword != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
			return s.sendPasswordChanged(sess, false, "Incorrect current password")
		}
	}

	// Check if this is a password removal request (empty new password)
	if req.NewPassword == "" {
		// Password removal: only allowed if user has SSH keys
		sshKeys, err := s.db.GetSSHKeysByUserID(int64(*userID))
		if err != nil {
			log.Printf("Failed to check SSH keys for user %d: %v", *userID, err)
			return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to check SSH keys")
		}
		if len(sshKeys) == 0 {
			return s.sendPasswordChanged(sess, false, "Cannot remove password without SSH keys. Add an SSH key first.")
		}

		// Remove password by setting to empty string
		if err := s.db.UpdateUserPassword(*userID, ""); err != nil {
			log.Printf("Failed to remove password for user %d: %v", *userID, err)
			return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to remove password")
		}

		sess.mu.RLock()
		nickname := sess.Nickname
		sess.mu.RUnlock()

		log.Printf("User %s (ID: %d) removed password (SSH-only authentication)", nickname, *userID)
		return s.sendPasswordChanged(sess, true, "")
	}

	// Validate new password hash (should be 43 characters for argon2id base64-encoded 32 bytes)
	// req.NewPassword contains client-side argon2id hash
	if len(req.NewPassword) < 40 || len(req.NewPassword) > 50 {
		return s.sendPasswordChanged(sess, false, "Invalid password hash format")
	}

	// Double-hash: bcrypt the client hash for storage
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password for user %d: %v", *userID, err)
		return s.sendError(sess, protocol.ErrCodeInternalError, "Failed to hash password")
	}

	// Update password
	if err := s.db.UpdateUserPassword(*userID, string(newHash)); err != nil {
		log.Printf("Failed to update password for user %d: %v", *userID, err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to update password")
	}

	sess.mu.RLock()
	nickname := sess.Nickname
	sess.mu.RUnlock()

	log.Printf("User %s (ID: %d) changed password", nickname, *userID)
	return s.sendPasswordChanged(sess, true, "")
}

// sendPasswordChanged sends a PASSWORD_CHANGED response
func (s *Server) sendPasswordChanged(sess *Session, success bool, errorMessage string) error {
	resp := &protocol.PasswordChangedResponse{
		Success:      success,
		ErrorMessage: errorMessage,
	}
	return s.sendMessage(sess, protocol.TypePasswordChanged, resp)
}

// handleAddSSHKey handles ADD_SSH_KEY message
func (s *Server) handleAddSSHKey(sess *Session, frame *protocol.Frame) error {
	// Must be authenticated
	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	if userID == nil {
		return s.sendError(sess, protocol.ErrCodeAuthRequired, "Must be authenticated to add SSH key")
	}

	// Decode request
	req := &protocol.AddSSHKeyRequest{}
	if err := req.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid request format")
	}

	// Validate public key format
	if req.PublicKey == "" {
		return s.sendSSHKeyAdded(sess, false, 0, "", "Public key cannot be empty")
	}

	// Parse SSH public key
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(req.PublicKey))
	if err != nil {
		return s.sendSSHKeyAdded(sess, false, 0, "", fmt.Sprintf("Invalid SSH public key: %v", err))
	}

	// Compute fingerprint
	fingerprint := ssh.FingerprintSHA256(pubKey)

	// Check for duplicate
	existing, err := s.db.GetSSHKeyByFingerprint(fingerprint)
	if err == nil && existing != nil {
		return s.sendSSHKeyAdded(sess, false, 0, "", "SSH key already exists")
	}

	// Create SSH key record
	sshKey := &database.SSHKey{
		UserID:      *userID,
		Fingerprint: fingerprint,
		PublicKey:   req.PublicKey,
		KeyType:     pubKey.Type(),
		AddedAt:     time.Now().UnixMilli(),
	}
	if req.Label != "" {
		sshKey.Label = &req.Label
	}

	// Store in database
	if err := s.db.CreateSSHKey(sshKey); err != nil {
		log.Printf("Failed to create SSH key for user %d: %v", *userID, err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to add SSH key")
	}

	log.Printf("User %d added SSH key %s", *userID, fingerprint)
	return s.sendSSHKeyAdded(sess, true, sshKey.ID, fingerprint, "")
}

// sendSSHKeyAdded sends SSH_KEY_ADDED response
func (s *Server) sendSSHKeyAdded(sess *Session, success bool, keyID int64, fingerprint, errorMessage string) error {
	resp := &protocol.SSHKeyAddedResponse{
		Success:      success,
		KeyID:        keyID,
		Fingerprint:  fingerprint,
		ErrorMessage: errorMessage,
	}
	return s.sendMessage(sess, protocol.TypeSSHKeyAdded, resp)
}

// handleListSSHKeys handles LIST_SSH_KEYS message
func (s *Server) handleListSSHKeys(sess *Session, frame *protocol.Frame) error {
	// Must be authenticated
	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	if userID == nil {
		return s.sendError(sess, protocol.ErrCodeAuthRequired, "Must be authenticated to list SSH keys")
	}

	// Decode request (no payload, but need to decode for consistency)
	req := &protocol.ListSSHKeysRequest{}
	if err := req.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid request format")
	}

	// Get SSH keys from database
	keys, err := s.db.GetSSHKeysByUserID(*userID)
	if err != nil {
		log.Printf("Failed to get SSH keys for user %d: %v", *userID, err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to retrieve SSH keys")
	}

	// Convert to protocol format
	keyInfos := make([]protocol.SSHKeyInfo, len(keys))
	for i, key := range keys {
		label := ""
		if key.Label != nil {
			label = *key.Label
		}
		lastUsed := int64(0)
		if key.LastUsedAt != nil {
			lastUsed = *key.LastUsedAt
		}

		keyInfos[i] = protocol.SSHKeyInfo{
			ID:          key.ID,
			Fingerprint: key.Fingerprint,
			KeyType:     key.KeyType,
			Label:       label,
			AddedAt:     key.AddedAt,
			LastUsedAt:  lastUsed,
		}
	}

	resp := &protocol.SSHKeyListResponse{
		Keys: keyInfos,
	}
	return s.sendMessage(sess, protocol.TypeSSHKeyList, resp)
}

// handleUpdateSSHKeyLabel handles UPDATE_SSH_KEY_LABEL message
func (s *Server) handleUpdateSSHKeyLabel(sess *Session, frame *protocol.Frame) error {
	// Must be authenticated
	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	if userID == nil {
		return s.sendError(sess, protocol.ErrCodeAuthRequired, "Must be authenticated to update SSH key")
	}

	// Decode request
	req := &protocol.UpdateSSHKeyLabelRequest{}
	if err := req.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid request format")
	}

	// Verify key belongs to user
	keys, err := s.db.GetSSHKeysByUserID(*userID)
	if err != nil {
		log.Printf("Failed to get SSH keys for user %d: %v", *userID, err)
		return s.sendSSHKeyLabelUpdated(sess, false, "Failed to retrieve SSH keys")
	}

	found := false
	for _, key := range keys {
		if key.ID == req.KeyID {
			found = true
			break
		}
	}

	if !found {
		return s.sendSSHKeyLabelUpdated(sess, false, "SSH key not found or does not belong to you")
	}

	// Update label
	if err := s.db.UpdateSSHKeyLabel(req.KeyID, *userID, req.NewLabel); err != nil {
		log.Printf("Failed to update SSH key label for key %d: %v", req.KeyID, err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to update SSH key label")
	}

	log.Printf("User %d updated label for SSH key %d", *userID, req.KeyID)
	return s.sendSSHKeyLabelUpdated(sess, true, "")
}

// sendSSHKeyLabelUpdated sends SSH_KEY_LABEL_UPDATED response
func (s *Server) sendSSHKeyLabelUpdated(sess *Session, success bool, errorMessage string) error {
	resp := &protocol.SSHKeyLabelUpdatedResponse{
		Success:      success,
		ErrorMessage: errorMessage,
	}
	return s.sendMessage(sess, protocol.TypeSSHKeyLabelUpdated, resp)
}

// handleDeleteSSHKey handles DELETE_SSH_KEY message
func (s *Server) handleDeleteSSHKey(sess *Session, frame *protocol.Frame) error {
	// Must be authenticated
	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	if userID == nil {
		return s.sendError(sess, protocol.ErrCodeAuthRequired, "Must be authenticated to delete SSH key")
	}

	// Decode request
	req := &protocol.DeleteSSHKeyRequest{}
	if err := req.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid request format")
	}

	// Get user's SSH keys
	keys, err := s.db.GetSSHKeysByUserID(*userID)
	if err != nil {
		log.Printf("Failed to get SSH keys for user %d: %v", *userID, err)
		return s.sendSSHKeyDeleted(sess, false, "Failed to retrieve SSH keys")
	}

	// Verify key belongs to user
	found := false
	for _, key := range keys {
		if key.ID == req.KeyID {
			found = true
			break
		}
	}

	if !found {
		return s.sendSSHKeyDeleted(sess, false, "SSH key not found or does not belong to you")
	}

	// Check if user has password (can't delete last SSH key if no password)
	user, err := s.db.GetUserByID(*userID)
	if err != nil {
		log.Printf("Failed to get user %d: %v", *userID, err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to verify user")
	}

	if len(keys) == 1 && user.PasswordHash == "" {
		return s.sendSSHKeyDeleted(sess, false, "Cannot delete last SSH key when no password is set")
	}

	// Delete SSH key
	if err := s.db.DeleteSSHKey(req.KeyID, *userID); err != nil {
		log.Printf("Failed to delete SSH key %d: %v", req.KeyID, err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to delete SSH key")
	}

	log.Printf("User %d deleted SSH key %d", *userID, req.KeyID)
	return s.sendSSHKeyDeleted(sess, true, "")
}

// sendSSHKeyDeleted sends SSH_KEY_DELETED response
func (s *Server) sendSSHKeyDeleted(sess *Session, success bool, errorMessage string) error {
	resp := &protocol.SSHKeyDeletedResponse{
		Success:      success,
		ErrorMessage: errorMessage,
	}
	return s.sendMessage(sess, protocol.TypeSSHKeyDeleted, resp)
}

// handlePing handles PING message
func (s *Server) handlePing(sess *Session, frame *protocol.Frame) error {
	// Decode message
	msg := &protocol.PingMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Update session activity on PING (for idle detection, rate-limited based on session timeout)
	s.sessions.UpdateSessionActivity(sess, time.Now().UnixMilli())

	// Send PONG
	resp := &protocol.PongMessage{
		ClientTimestamp: msg.Timestamp,
	}

	return s.sendMessage(sess, protocol.TypePong, resp)
}

// handleDisconnect handles graceful client disconnect
func (s *Server) handleDisconnect(sess *Session, frame *protocol.Frame) error {
	// Client is disconnecting gracefully - remove from sessions map immediately
	// to prevent broadcasts during the 100ms grace period before connection closes
	s.removeSession(sess.ID)

	// Return error to close the connection
	return ErrClientDisconnecting
}

// sendMessage sends a protocol message to a session
func (s *Server) sendMessage(sess *Session, msgType uint8, msg interface{}) error {
	// Encode message payload
	var payload []byte
	var err error

	switch m := msg.(type) {
	case interface{ Encode() ([]byte, error) }:
		payload, err = m.Encode()
	default:
		return fmt.Errorf("message type does not implement Encode()")
	}

	if err != nil {
		return err
	}

	// Create frame
	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Flags:   0,
		Payload: payload,
	}

	// Send frame (SafeConn automatically handles write synchronization)
	debugLog.Printf("Session %d → SEND: Type=0x%02X Flags=0x%02X PayloadLen=%d", sess.ID, msgType, 0, len(payload))
	if err := sess.Conn.EncodeFrame(frame, sess.ProtocolVersion); err != nil {
		errorLog.Printf("Session %d: EncodeFrame failed (Type=0x%02X): %v", sess.ID, msgType, err)
		return err
	}
	return nil
}

// broadcastToChannel sends a message to all sessions in a channel
func (s *Server) broadcastToChannel(channelID int64, msgType uint8, msg interface{}) error {
	// Encode message payload
	var payload []byte
	var err error

	switch m := msg.(type) {
	case interface{ Encode() ([]byte, error) }:
		payload, err = m.Encode()
	default:
		return fmt.Errorf("message type does not implement Encode()")
	}

	if err != nil {
		return err
	}

	// Create frame
	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Flags:   0,
		Payload: payload,
	}

	// Pre-encode for both v1 and v2 clients
	encoded, err := encodeFrameVersionAware(frame)
	if err != nil {
		return err
	}

	// Collect target sessions: both joined sessions AND channel subscribers
	targetSessionsMap := make(map[uint64]*Session)

	// 1. Get sessions that have joined this channel
	s.sessions.mu.RLock()
	for _, sess := range s.sessions.sessions {
		sess.mu.RLock()
		joined := sess.JoinedChannel
		sess.mu.RUnlock()

		if joined != nil && *joined == channelID {
			targetSessionsMap[sess.ID] = sess
		}
	}
	s.sessions.mu.RUnlock()

	// 2. Get sessions subscribed to this channel (using subscription index)
	channelSub := ChannelSubscription{
		ChannelID:    uint64(channelID),
		SubchannelID: nil,
	}
	subscribedSessions := s.sessions.GetChannelSubscribers(channelSub)
	for _, sess := range subscribedSessions {
		targetSessionsMap[sess.ID] = sess
	}

	// Convert map to slice
	targetSessions := make([]*Session, 0, len(targetSessionsMap))
	for _, sess := range targetSessionsMap {
		targetSessions = append(targetSessions, sess)
	}

	// Broadcast to target sessions using version-aware worker pool
	deadSessions := s.broadcastToSessionsVersionAware(targetSessions, encoded.v1Bytes, encoded.v2Bytes)

	// Remove dead sessions
	for _, sessID := range deadSessions {
		s.removeSession(sessID)
	}

	return nil
}

// broadcastToSessionsParallel broadcasts frameBytes to sessions using a worker pool
// Returns list of session IDs that had write errors
func (s *Server) broadcastToSessionsParallel(sessions []*Session, frameBytes []byte) []uint64 {
	// Use version-aware broadcast with only uncompressed bytes (legacy compatibility)
	return s.broadcastToSessionsVersionAware(sessions, frameBytes, nil)
}

// broadcastToSessionsVersionAware broadcasts to sessions with version-aware encoding.
// v1Bytes is sent to v1 clients, v2Bytes is sent to v2+ clients.
// If v2Bytes is nil, v1Bytes is sent to all clients.
// Returns list of session IDs that had write errors.
func (s *Server) broadcastToSessionsVersionAware(sessions []*Session, v1Bytes, v2Bytes []byte) []uint64 {
	const maxWorkers = 40
	const sessionsPerWorker = 50

	if len(sessions) == 0 {
		return nil
	}

	// Determine actual worker count
	numWorkers := (len(sessions) + sessionsPerWorker - 1) / sessionsPerWorker
	if numWorkers > maxWorkers {
		numWorkers = maxWorkers
	}

	// Calculate chunk size
	chunkSize := (len(sessions) + numWorkers - 1) / numWorkers

	// Broadcast in parallel chunks
	var wg sync.WaitGroup
	var deadSessionsMu sync.Mutex
	deadSessions := make([]uint64, 0)

	for i := 0; i < numWorkers; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(sessions) {
			end = len(sessions)
		}

		chunk := sessions[start:end]
		wg.Add(1)
		go func(sessionChunk []*Session) {
			defer wg.Done()
			for _, sess := range sessionChunk {
				// Choose appropriate encoding based on client's protocol version
				frameBytes := v1Bytes
				if v2Bytes != nil && sess.ProtocolVersion >= 2 {
					frameBytes = v2Bytes
				}

				if writeErr := sess.Conn.WriteBytes(frameBytes); writeErr != nil {
					debugLog.Printf("Session %d: Broadcast write failed: %v", sess.ID, writeErr)
					deadSessionsMu.Lock()
					deadSessions = append(deadSessions, sess.ID)
					deadSessionsMu.Unlock()
				}
			}
		}(chunk)
	}

	wg.Wait()
	return deadSessions
}

// broadcastNewMessage sends a NEW_MESSAGE to subscribed sessions only (subscription-aware)
// If authorSess is shadowbanned, the message is only sent to the author and admins
func (s *Server) broadcastNewMessage(authorSess *Session, msg *protocol.NewMessageMessage, threadRootID *uint64) error {
	startTime := time.Now()

	// Encode message payload ONCE (not per recipient)
	payload, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	// Create frame
	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeNewMessage,
		Flags:   0,
		Payload: payload,
	}

	// Pre-encode for both v1 and v2 clients
	encoded, err := encodeFrameVersionAware(frame)
	if err != nil {
		return err
	}

	// Build channel subscription key
	var subchannelID *uint64
	if msg.SubchannelID != nil {
		id := uint64(*msg.SubchannelID)
		subchannelID = &id
	}

	channelSub := ChannelSubscription{
		ChannelID:    msg.ChannelID,
		SubchannelID: subchannelID,
	}

	// Determine if this is a top-level message
	isTopLevel := msg.ParentID == nil || (msg.ParentID != nil && *msg.ParentID == 0)

	// Metrics: track broadcast
	if s.metrics != nil {
		s.metrics.RecordMessageBroadcast()
	}
	recipientCount := 0
	broadcastType := "thread"
	if isTopLevel {
		broadcastType = "channel"
	}

	// Get subscribers using reverse index (no iteration through all sessions!)
	var targetSessions []*Session
	if isTopLevel {
		// Top-level message: get channel subscribers
		targetSessions = s.sessions.GetChannelSubscribers(channelSub)
		debugLog.Printf("Broadcasting top-level message %d to channel %d: %d subscribers", msg.ID, msg.ChannelID, len(targetSessions))
	} else if threadRootID != nil {
		// Reply: get thread subscribers
		targetSessions = s.sessions.GetThreadSubscribers(*threadRootID)
		debugLog.Printf("Broadcasting reply message %d to thread %d: %d subscribers", msg.ID, *threadRootID, len(targetSessions))
	} else {
		debugLog.Printf("WARNING: Reply message %d has no threadRootID - will not be broadcast!", msg.ID)
	}

	// Filter recipients if author is shadowbanned
	authorSess.mu.RLock()
	isShadowbanned := authorSess.Shadowbanned
	authorSess.mu.RUnlock()

	if isShadowbanned {
		// Shadowbanned: only send to author and admins
		filteredSessions := make([]*Session, 0)
		for _, sess := range targetSessions {
			sess.mu.RLock()
			isAuthor := sess.ID == authorSess.ID
			isAdmin := sess.UserID != nil && (sess.UserFlags&1) != 0 // Check admin flag
			sess.mu.RUnlock()

			if isAuthor || isAdmin {
				filteredSessions = append(filteredSessions, sess)
			}
		}
		debugLog.Printf("Shadowban: filtering message %d from %d recipients to %d (author + admins only)", msg.ID, len(targetSessions), len(filteredSessions))
		targetSessions = filteredSessions
	}

	// Broadcast to target sessions using version-aware worker pool
	recipientCount = len(targetSessions)
	deadSessions := s.broadcastToSessionsVersionAware(targetSessions, encoded.v1Bytes, encoded.v2Bytes)

	// Remove dead sessions
	for _, sessID := range deadSessions {
		s.removeSession(sessID)
	}

	// Metrics: record fan-out and duration
	if s.metrics != nil {
		s.metrics.RecordBroadcastFanout(broadcastType, recipientCount)
		s.metrics.RecordBroadcastDuration(broadcastType, time.Since(startTime).Seconds())
	}

	return nil
}

// convertDBMessagesToProtocol converts database messages to protocol messages
func convertDBMessagesToProtocol(dbMessages []*database.Message, db *database.MemDB) []protocol.Message {
	messages := make([]protocol.Message, len(dbMessages))
	for i, dbMsg := range dbMessages {
		messages[i] = *convertDBMessageToProtocol(dbMsg, db)
	}
	return messages
}

// convertDBMessageToProtocol converts a database message to protocol message
func convertDBMessageToProtocol(dbMsg *database.Message, db *database.MemDB) *protocol.Message {
	var subchannelID, parentID, authorUserID *uint64
	var editedAt *time.Time

	if dbMsg.SubchannelID != nil {
		id := uint64(*dbMsg.SubchannelID)
		subchannelID = &id
	}
	if dbMsg.ParentID != nil {
		id := uint64(*dbMsg.ParentID)
		parentID = &id
	}
	if dbMsg.AuthorUserID != nil {
		id := uint64(*dbMsg.AuthorUserID)
		authorUserID = &id
	}
	if dbMsg.EditedAt != nil {
		t := time.UnixMilli(*dbMsg.EditedAt)
		editedAt = &t
	}

	// Determine display nickname (with prefix)
	nickname := dbMsg.AuthorNickname
	if dbMsg.AuthorUserID != nil {
		// Registered user - lookup and apply prefix based on flags
		user, err := db.GetUserByID(*dbMsg.AuthorUserID)
		if err == nil {
			prefix := protocol.UserFlags(user.UserFlags).DisplayPrefix()
			nickname = prefix + user.Nickname
		} else {
			// Fallback if user lookup fails (shouldn't happen)
			nickname = "<user:" + fmt.Sprint(*dbMsg.AuthorUserID) + ">"
		}
	} else if dbMsg.AuthorNickname != "" {
		// Anonymous user - prefix with tilde (but not for system messages with empty author)
		nickname = "~" + dbMsg.AuthorNickname
	}

	// Count replies (only for root messages)
	replyCount := uint32(0)
	if dbMsg.ParentID == nil {
		count, err := db.CountReplies(dbMsg.ID)
		if err == nil {
			replyCount = count
		}
	}

	return &protocol.Message{
		ID:             uint64(dbMsg.ID),
		ChannelID:      uint64(dbMsg.ChannelID),
		SubchannelID:   subchannelID,
		ParentID:       parentID,
		AuthorUserID:   authorUserID,
		AuthorNickname: nickname, // Prefixed for registered users, as-is for anonymous
		Content:        dbMsg.Content,
		CreatedAt:      time.UnixMilli(dbMsg.CreatedAt),
		EditedAt:       editedAt,
		ReplyCount:     replyCount,
	}
}

// handleSubscribeThread handles SUBSCRIBE_THREAD message
func (s *Server) handleSubscribeThread(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.SubscribeThreadMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Validate thread exists
	exists, err := s.db.MessageExists(int64(msg.ThreadID))
	if err != nil {
		return s.dbError(sess, "MessageExists", err)
	}
	if !exists {
		return s.sendError(sess, protocol.ErrCodeThreadNotFound, "Thread does not exist")
	}

	// Get thread's channel for tracking
	threadMsg, err := s.db.GetMessage(int64(msg.ThreadID))
	if err != nil {
		return s.dbError(sess, "GetMessage", err)
	}

	var subchannelID *uint64
	if threadMsg.SubchannelID != nil {
		id := uint64(*threadMsg.SubchannelID)
		subchannelID = &id
	}

	channelSub := ChannelSubscription{
		ChannelID:    uint64(threadMsg.ChannelID),
		SubchannelID: subchannelID,
	}

	// Add subscription with limit check
	if sess.ThreadSubscriptionCount() >= int(s.config.MaxThreadSubscriptions) {
		return s.sendError(sess, protocol.ErrCodeThreadSubscriptionLimit, fmt.Sprintf("Thread subscription limit exceeded (max %d per session)", s.config.MaxThreadSubscriptions))
	}

	s.sessions.SubscribeToThread(sess, msg.ThreadID, channelSub)

	// Send success response
	resp := &protocol.SubscribeOkMessage{
		Type:         1, // 1=thread
		ID:           msg.ThreadID,
		SubchannelID: subchannelID,
	}
	return s.sendMessage(sess, protocol.TypeSubscribeOk, resp)
}

// handleUnsubscribeThread handles UNSUBSCRIBE_THREAD message
func (s *Server) handleUnsubscribeThread(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.UnsubscribeThreadMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Remove subscription (idempotent - no error if not subscribed)
	s.sessions.UnsubscribeFromThread(sess, msg.ThreadID)

	// Send success response
	resp := &protocol.SubscribeOkMessage{
		Type: 1, // 1=thread
		ID:   msg.ThreadID,
	}
	return s.sendMessage(sess, protocol.TypeSubscribeOk, resp)
}

// handleSubscribeChannel handles SUBSCRIBE_CHANNEL message
func (s *Server) handleSubscribeChannel(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.SubscribeChannelMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Validate channel exists in MemDB (instant lookup)
	exists, err := s.db.ChannelExists(int64(msg.ChannelID))
	if err != nil || !exists {
		return s.sendError(sess, protocol.ErrCodeChannelNotFound, "Channel does not exist")
	}

	// Validate subchannel if provided (still uses DB - subchannels not cached yet)
	if msg.SubchannelID != nil {
		exists, err := s.db.SubchannelExists(int64(*msg.SubchannelID))
		if err != nil {
			return s.dbError(sess, "SubchannelExists", err)
		}
		if !exists {
			return s.sendError(sess, protocol.ErrCodeSubchannelNotFound, "Subchannel does not exist")
		}
	}

	channelSub := ChannelSubscription{
		ChannelID:    msg.ChannelID,
		SubchannelID: msg.SubchannelID,
	}

	// Add subscription with limit check
	if sess.ChannelSubscriptionCount() >= int(s.config.MaxChannelSubscriptions) {
		return s.sendError(sess, protocol.ErrCodeChannelSubscriptionLimit, fmt.Sprintf("Channel subscription limit exceeded (max %d per session)", s.config.MaxChannelSubscriptions))
	}

	s.sessions.SubscribeToChannel(sess, channelSub)

	// Send success response
	resp := &protocol.SubscribeOkMessage{
		Type:         2, // 2=channel
		ID:           msg.ChannelID,
		SubchannelID: msg.SubchannelID,
	}
	return s.sendMessage(sess, protocol.TypeSubscribeOk, resp)
}

// handleUnsubscribeChannel handles UNSUBSCRIBE_CHANNEL message
func (s *Server) handleUnsubscribeChannel(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.UnsubscribeChannelMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	channelSub := ChannelSubscription{
		ChannelID:    msg.ChannelID,
		SubchannelID: msg.SubchannelID,
	}

	// Remove subscription (idempotent - no error if not subscribed)
	s.sessions.UnsubscribeFromChannel(sess, channelSub)

	// Send success response
	resp := &protocol.SubscribeOkMessage{
		Type:         2, // 2=channel
		ID:           msg.ChannelID,
		SubchannelID: msg.SubchannelID,
	}
	return s.sendMessage(sess, protocol.TypeSubscribeOk, resp)
}

// handleGetUserInfo handles GET_USER_INFO message
func (s *Server) handleGetUserInfo(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.GetUserInfoMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	log.Printf("Session %d: GET_USER_INFO request for nickname=%s", sess.ID, msg.Nickname)

	// Check if user is registered in database
	user, err := s.db.GetUserByNickname(msg.Nickname)
	isRegistered := false
	var userID *uint64

	if err == nil {
		// User is registered
		isRegistered = true
		uid := uint64(user.ID)
		userID = &uid
		log.Printf("Session %d: User '%s' is registered (user_id=%d)", sess.ID, msg.Nickname, uid)
	} else if err != sql.ErrNoRows {
		// Database error (not just "user not found")
		return s.dbError(sess, "GetUserByNickname", err)
	} else {
		log.Printf("Session %d: User '%s' is not registered", sess.ID, msg.Nickname)
	}

	// Check if user is currently online (check all sessions for matching nickname)
	online := false
	allSessions := s.sessions.GetAllSessions()
	for _, s := range allSessions {
		s.mu.RLock()
		if s.Nickname == msg.Nickname {
			online = true
			s.mu.RUnlock()
			break
		}
		s.mu.RUnlock()
	}

	// Send response
	resp := &protocol.UserInfoMessage{
		Nickname:     msg.Nickname,
		IsRegistered: isRegistered,
		UserID:       userID,
		Online:       online,
	}
	log.Printf("Session %d: Sending USER_INFO response: nickname=%s, is_registered=%v, online=%v", sess.ID, msg.Nickname, isRegistered, online)
	return s.sendMessage(sess, protocol.TypeUserInfo, resp)
}

// handleListUsers handles LIST_USERS message
func (s *Server) handleListUsers(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.ListUsersMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Check if include_offline is requested
	if msg.IncludeOffline {
		// Verify admin permission
		if !s.isAdmin(sess) {
			return s.sendError(sess, protocol.ErrCodePermissionDenied, "Admin permission required for include_offline")
		}
	}

	// Apply limit constraints
	limit := msg.Limit
	if limit == 0 {
		limit = 100 // Default
	}
	if limit > 500 {
		limit = 500 // Max
	}

	var users []protocol.UserListEntry

	if msg.IncludeOffline {
		// Admin requested all registered users (online + offline)
		allUsers, err := s.db.ListAllUsers(int(limit))
		if err != nil {
			log.Printf("[ERROR] Failed to list all users: %v", err)
			return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to retrieve user list")
		}

		// Get currently online user IDs
		onlineUserIDs := make(map[int64]bool)
		allSessions := s.sessions.GetAllSessions()
		for _, session := range allSessions {
			session.mu.RLock()
			if session.UserID != nil {
				onlineUserIDs[*session.UserID] = true
			}
			session.mu.RUnlock()
		}

		// Build user list with online status
		for _, user := range allUsers {
			uid := uint64(user.ID)
			users = append(users, protocol.UserListEntry{
				Nickname:     user.Nickname,
				IsRegistered: true, // All users from DB are registered
				UserID:       &uid,
				Online:       onlineUserIDs[user.ID],
			})
		}
	} else {
		// Standard request: online users only
		allSessions := s.sessions.GetAllSessions()

		// Build user list (deduplicate by nickname)
		seenNicknames := make(map[string]bool)

		for _, session := range allSessions {
			session.mu.RLock()
			nickname := session.Nickname
			userID := session.UserID
			session.mu.RUnlock()

			// Skip if we've already added this nickname
			if seenNicknames[nickname] {
				continue
			}
			seenNicknames[nickname] = true

			// Determine if registered and get user_id
			isRegistered := userID != nil
			var uid *uint64
			if isRegistered {
				u := uint64(*userID)
				uid = &u
			}

			users = append(users, protocol.UserListEntry{
				Nickname:     nickname,
				IsRegistered: isRegistered,
				UserID:       uid,
				Online:       true, // All users from sessions are online
			})

			// Stop if we've reached the limit
			if len(users) >= int(limit) {
				break
			}
		}
	}

	// Send response
	resp := &protocol.UserListMessage{
		Users: users,
	}
	return s.sendMessage(sess, protocol.TypeUserList, resp)
}

// handleListChannelUsers returns the current roster for a channel/subchannel
func (s *Server) handleListChannelUsers(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.ListChannelUsersMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	channelID := int64(msg.ChannelID)
	if channelID == 0 {
		return s.sendError(sess, protocol.ErrCodeChannelNotFound, "Channel not found")
	}

	exists, err := s.db.ChannelExists(channelID)
	if err != nil {
		return s.dbError(sess, "ChannelExists", err)
	}
	if !exists {
		return s.sendError(sess, protocol.ErrCodeChannelNotFound, "Channel does not exist")
	}

	if msg.SubchannelID != nil {
		subExists, err := s.db.SubchannelExists(int64(*msg.SubchannelID))
		if err != nil {
			return s.dbError(sess, "SubchannelExists", err)
		}
		if !subExists {
			return s.sendError(sess, protocol.ErrCodeSubchannelNotFound, "Subchannel does not exist")
		}
	}

	allSessions := s.sessions.GetAllSessions()
	users := make([]protocol.ChannelUserEntry, 0)
	for _, other := range allSessions {
		other.mu.RLock()
		joined := other.JoinedChannel
		nickname := other.Nickname
		userID := other.UserID
		userFlags := other.UserFlags
		other.mu.RUnlock()

		if joined == nil || *joined != channelID {
			continue
		}

		entry := protocol.ChannelUserEntry{
			SessionID:    other.ID,
			Nickname:     nickname,
			IsRegistered: userID != nil,
			UserFlags:    protocol.UserFlags(userFlags),
		}
		if userID != nil {
			entry.UserID = optionalUint64FromInt64Ptr(userID)
		}
		users = append(users, entry)
	}

	resp := &protocol.ChannelUserListMessage{
		ChannelID:    msg.ChannelID,
		SubchannelID: msg.SubchannelID,
		Users:        users,
	}
	return s.sendMessage(sess, protocol.TypeChannelUserList, resp)
}

// broadcastChannelCreated broadcasts a CHANNEL_CREATED message to all connected users (except creator)
func (s *Server) broadcastChannelCreated(ch *database.Channel, creatorSessionID uint64) {
	msg := &protocol.ChannelCreatedMessage{
		Success:        true,
		ChannelID:      uint64(ch.ID),
		Name:           ch.Name,
		Description:    safeDeref(ch.Description, ""),
		Type:           ch.ChannelType,
		RetentionHours: ch.MessageRetentionHours,
		Message:        fmt.Sprintf("New channel '%s' created", ch.DisplayName),
	}

	// Broadcast to all connected sessions EXCEPT the creator (they already got the response)
	allSessions := s.sessions.GetAllSessions()
	for _, sess := range allSessions {
		if sess.ID == creatorSessionID {
			continue // Skip creator - they already received the response
		}
		if err := s.sendMessage(sess, protocol.TypeChannelCreated, msg); err != nil {
			log.Printf("Failed to broadcast CHANNEL_CREATED to session %d: %v", sess.ID, err)
		}
	}
}

// broadcastToAll broadcasts a message to all connected clients
func (s *Server) broadcastToAll(msgType uint8, msg interface{}) error {
	// Encode message payload
	var payload []byte
	var err error

	switch m := msg.(type) {
	case interface{ Encode() ([]byte, error) }:
		payload, err = m.Encode()
	default:
		return fmt.Errorf("message type does not implement Encode()")
	}

	if err != nil {
		return err
	}

	// Create frame
	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Flags:   0,
		Payload: payload,
	}

	// Pre-encode for both v1 and v2 clients
	encoded, err := encodeFrameVersionAware(frame)
	if err != nil {
		return err
	}

	// Get all sessions
	allSessions := s.sessions.GetAllSessions()

	// Broadcast to target sessions using version-aware worker pool
	deadSessions := s.broadcastToSessionsVersionAware(allSessions, encoded.v1Bytes, encoded.v2Bytes)

	// Remove dead sessions
	for _, sessID := range deadSessions {
		s.removeSession(sessID)
	}

	return nil
}

// ===== Server Discovery Handlers =====

// handleListServers handles LIST_SERVERS message (request server directory)
func (s *Server) handleListServers(sess *Session, frame *protocol.Frame) error {
	log.Printf("[DEBUG] handleListServers: DirectoryEnabled=%v", s.config.DirectoryEnabled)

	// Only respond if directory mode is enabled
	if !s.config.DirectoryEnabled {
		log.Printf("[DEBUG] handleListServers: Directory not enabled, returning empty list")
		// Return empty list for non-directory servers
		resp := &protocol.ServerListMessage{
			Servers: []protocol.ServerInfo{},
		}
		return s.sendMessage(sess, protocol.TypeServerList, resp)
	}

	// Decode message
	msg := &protocol.ListServersMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Validate limit
	limit := msg.Limit
	if limit == 0 {
		limit = 100 // default
	}
	if limit > 500 {
		limit = 500 // max
	}

	// Get servers from database
	servers, err := s.db.ListDiscoveredServers(limit)
	if err != nil {
		return s.dbError(sess, "ListDiscoveredServers", err)
	}

	// Include the directory server itself as the first entry
	serverInfos := make([]protocol.ServerInfo, 0, len(servers)+1)

	// Add self (directory server)
	selfInfo := protocol.ServerInfo{
		Hostname:      s.config.PublicHostname,
		Port:          uint16(s.config.TCPPort),
		Name:          s.config.ServerName,
		Description:   s.config.ServerDesc,
		UserCount:     s.sessions.CountOnlineUsers(),
		MaxUsers:      s.config.MaxUsers,
		UptimeSeconds: uint64(time.Since(s.startTime).Seconds()),
		IsPublic:      true,
		ChannelCount:  s.db.CountChannels(),
	}
	serverInfos = append(serverInfos, selfInfo)

	log.Printf("[DEBUG] handleListServers: Returning %d servers (including self: %s)", len(serverInfos), selfInfo.Name)

	// Add registered servers from database
	for _, server := range servers {
		serverInfos = append(serverInfos, protocol.ServerInfo{
			Hostname:      server.Hostname,
			Port:          server.Port,
			Name:          server.Name,
			Description:   server.Description,
			UserCount:     server.UserCount,
			MaxUsers:      server.MaxUsers,
			UptimeSeconds: server.UptimeSeconds,
			IsPublic:      server.IsPublic,
			ChannelCount:  server.ChannelCount,
		})
	}

	// Send response
	resp := &protocol.ServerListMessage{
		Servers: serverInfos,
	}
	return s.sendMessage(sess, protocol.TypeServerList, resp)
}

// handleRegisterServer handles REGISTER_SERVER message (server registration)
func (s *Server) handleRegisterServer(sess *Session, frame *protocol.Frame) error {
	// Only accept if directory mode is enabled
	if !s.config.DirectoryEnabled {
		return s.sendError(sess, 1001, "Directory mode not enabled on this server")
	}

	// Check rate limit (30 requests/hour per IP)
	if !s.checkDiscoveryRateLimit(sess.RemoteAddr) {
		resp := &protocol.RegisterAckMessage{
			Success: false,
			Message: "Rate limit exceeded (30 requests/hour)",
		}
		return s.sendMessage(sess, protocol.TypeRegisterAck, resp)
	}

	// Decode message
	msg := &protocol.RegisterServerMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Validate hostname and port
	if msg.Hostname == "" || msg.Port == 0 {
		resp := &protocol.RegisterAckMessage{
			Success: false,
			Message: "Invalid hostname or port",
		}
		return s.sendMessage(sess, protocol.TypeRegisterAck, resp)
	}

	// Start verification in background
	go s.verifyAndRegisterServer(msg, sess.RemoteAddr)

	// Send immediate response (verification happens async)
	// Server will be added after successful verification
	resp := &protocol.RegisterAckMessage{
		Success: false,
		Message: "Verification in progress...",
	}
	return s.sendMessage(sess, protocol.TypeRegisterAck, resp)
}

// handleVerifyRegistration responds to verification challenges from directories
func (s *Server) handleVerifyRegistration(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.VerifyRegistrationMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	log.Printf("Received VERIFY_REGISTRATION challenge %d from %s", msg.Challenge, sess.RemoteAddr)

	resp := &protocol.VerifyResponseMessage{
		Challenge: msg.Challenge,
	}

	if err := s.sendMessage(sess, protocol.TypeVerifyResponse, resp); err != nil {
		return err
	}

	log.Printf("Sent VERIFY_RESPONSE (challenge %d) to %s", msg.Challenge, sess.RemoteAddr)
	return nil
}

// handleVerifyResponse handles VERIFY_RESPONSE message (verification challenge response)
func (s *Server) handleVerifyResponse(sess *Session, frame *protocol.Frame) error {
	// Decode message
	msg := &protocol.VerifyResponseMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Check if we have a pending verification for this session
	s.verificationMu.Lock()
	expectedChallenge, exists := s.verificationChallenges[sess.ID]
	if exists {
		delete(s.verificationChallenges, sess.ID)
	}
	s.verificationMu.Unlock()

	if !exists {
		// No pending verification for this session
		return s.sendError(sess, 6000, "No pending verification")
	}

	if msg.Challenge != expectedChallenge {
		// Wrong challenge response
		log.Printf("Verification failed for session %d: wrong challenge (expected %d, got %d)",
			sess.ID, expectedChallenge, msg.Challenge)
		return s.sendError(sess, 6000, "Verification failed")
	}

	// Verification succeeded! Mark this session as verified
	log.Printf("Verification succeeded for session %d", sess.ID)

	// Note: The actual server registration happens in verifyAndRegisterServer
	// This handler just validates the challenge response

	return nil
}

// handleHeartbeat handles HEARTBEAT message (periodic keepalive from registered servers)
func (s *Server) handleHeartbeat(sess *Session, frame *protocol.Frame) error {
	// Only accept if directory mode is enabled
	if !s.config.DirectoryEnabled {
		return s.sendError(sess, 1001, "Directory mode not enabled on this server")
	}

	// Decode message
	msg := &protocol.HeartbeatMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, 1000, "Invalid message format")
	}

	// Check if server exists in directory
	server, err := s.db.GetDiscoveredServer(msg.Hostname, msg.Port)
	if err != nil {
		if err == sql.ErrNoRows {
			return s.sendError(sess, 4000, "Server not registered")
		}
		return s.dbError(sess, "GetDiscoveredServer", err)
	}

	// Calculate new heartbeat interval based on directory load
	serverCount, err := s.db.CountDiscoveredServers()
	if err != nil {
		serverCount = 0 // fallback
	}
	newInterval := s.calculateHeartbeatInterval(serverCount)

	// Update heartbeat
	err = s.db.UpdateHeartbeat(msg.Hostname, msg.Port, msg.UserCount, msg.UptimeSeconds, msg.ChannelCount, newInterval)
	if err != nil {
		return s.dbError(sess, "UpdateHeartbeat", err)
	}

	// Log interval change if different
	if newInterval != server.HeartbeatInterval {
		log.Printf("Updated heartbeat interval for %s:%d from %ds to %ds (directory has %d servers)",
			msg.Hostname, msg.Port, server.HeartbeatInterval, newInterval, serverCount)
	}

	// Send acknowledgment with new interval
	resp := &protocol.HeartbeatAckMessage{
		HeartbeatInterval: newInterval,
	}
	return s.sendMessage(sess, protocol.TypeHeartbeatAck, resp)
}

// Helper methods for server discovery

type discoveryRateLimiter struct {
	requests []time.Time
	mu       sync.Mutex
}

// checkDiscoveryRateLimit checks if IP is within rate limit (30 requests/hour)
func (s *Server) checkDiscoveryRateLimit(remoteAddr string) bool {
	// Extract IP from address
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	s.discoveryRateLimitMu.Lock()
	defer s.discoveryRateLimitMu.Unlock()

	limiter, exists := s.discoveryRateLimits[host]
	if !exists {
		limiter = &discoveryRateLimiter{
			requests: []time.Time{},
		}
		s.discoveryRateLimits[host] = limiter
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)

	// Remove requests older than 1 hour
	filtered := make([]time.Time, 0, len(limiter.requests))
	for _, t := range limiter.requests {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	limiter.requests = filtered

	// Check if under limit
	if len(limiter.requests) >= 30 {
		return false
	}

	// Add this request
	limiter.requests = append(limiter.requests, now)
	return true
}

// calculateHeartbeatInterval returns heartbeat interval based on server count
func (s *Server) calculateHeartbeatInterval(serverCount uint32) uint32 {
	switch {
	case serverCount < 100:
		return 300 // 5 minutes
	case serverCount < 1000:
		return 600 // 10 minutes
	case serverCount < 5000:
		return 1800 // 30 minutes
	default:
		return 3600 // 1 hour
	}
}

// verifyAndRegisterServer verifies a server is reachable and registers it
func (s *Server) verifyAndRegisterServer(msg *protocol.RegisterServerMessage, sourceIP string) {
	addr := fmt.Sprintf("%s:%d", msg.Hostname, msg.Port)

	serverConfig, err := s.verifyServerReachability(msg.Hostname, msg.Port)
	if err != nil {
		log.Printf("Verification failed for %s: %v", addr, err)
		return
	}

	log.Printf("Verification handshake with %s succeeded (protocol v%d)", addr, serverConfig.ProtocolVersion)

	// Verification succeeded! Register the server
	_, err = s.db.RegisterDiscoveredServer(
		msg.Hostname,
		msg.Port,
		msg.Name,
		msg.Description,
		msg.MaxUsers,
		msg.IsPublic,
		msg.ChannelCount,
		sourceIP,
		"registration",
	)
	if err != nil {
		log.Printf("Failed to register server %s: %v", addr, err)
		return
	}

	// Record an initial heartbeat using the health-check interval so the server remains visible.
	intervalSeconds := uint32(directoryHealthCheckInterval.Seconds())
	if intervalSeconds == 0 {
		intervalSeconds = 300
	}
	if err := s.db.UpdateHeartbeat(msg.Hostname, msg.Port, 0, 0, msg.ChannelCount, intervalSeconds); err != nil {
		log.Printf("Verification succeeded for %s but failed to update heartbeat metadata: %v", addr, err)
		return
	}

	log.Printf("Successfully registered server %s (health check interval: %ds)", addr, intervalSeconds)
}

// ===== Admin System Handlers =====

// handleBanUser handles BAN_USER message (admin only)
func (s *Server) handleBanUser(sess *Session, frame *protocol.Frame) error {
	// Check admin permissions
	if !s.isAdmin(sess) {
		return s.sendMessage(sess, protocol.TypeUserBanned, &protocol.UserBannedMessage{
			Success: false,
			Message: "Permission denied: admin access required",
		})
	}

	// Decode message
	msg := &protocol.BanUserMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Validate: must provide either UserID or Nickname
	if msg.UserID == nil && msg.Nickname == nil {
		return s.sendMessage(sess, protocol.TypeUserBanned, &protocol.UserBannedMessage{
			Success: false,
			Message: "Must provide either UserID or Nickname",
		})
	}

	// Get admin info for audit log
	sess.mu.RLock()
	adminNickname := sess.Nickname
	sess.mu.RUnlock()
	adminIP, _, _ := net.SplitHostPort(sess.RemoteAddr)

	// Convert UserID if provided
	var userID *int64
	if msg.UserID != nil {
		id := int64(*msg.UserID)
		userID = &id
	}

	// Create ban in database
	banID, err := s.db.CreateUserBan(userID, msg.Nickname, msg.Reason, msg.Shadowban, msg.DurationSeconds, adminNickname, adminIP)
	if err != nil {
		log.Printf("Failed to create user ban: %v", err)
		return s.sendMessage(sess, protocol.TypeUserBanned, &protocol.UserBannedMessage{
			Success: false,
			Message: "Failed to create ban",
		})
	}

	targetIdentifier := ""
	if msg.Nickname != nil {
		targetIdentifier = *msg.Nickname
	} else if msg.UserID != nil {
		targetIdentifier = fmt.Sprintf("user_id:%d", *msg.UserID)
	}

	log.Printf("Admin %s banned user %s (ban_id=%d, reason=%s, shadowban=%v)",
		adminNickname, targetIdentifier, banID, msg.Reason, msg.Shadowban)

	// Send success response
	return s.sendMessage(sess, protocol.TypeUserBanned, &protocol.UserBannedMessage{
		Success: true,
		BanID:   uint64(banID),
		Message: fmt.Sprintf("User %s banned successfully", targetIdentifier),
	})
}

// handleBanIP handles BAN_IP message (admin only)
func (s *Server) handleBanIP(sess *Session, frame *protocol.Frame) error {
	// Check admin permissions
	if !s.isAdmin(sess) {
		return s.sendMessage(sess, protocol.TypeIPBanned, &protocol.IPBannedMessage{
			Success: false,
			Message: "Permission denied: admin access required",
		})
	}

	// Decode message
	msg := &protocol.BanIPMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Validate IPCIDR
	if msg.IPCIDR == "" {
		return s.sendMessage(sess, protocol.TypeIPBanned, &protocol.IPBannedMessage{
			Success: false,
			Message: "IP/CIDR address required",
		})
	}

	// Get admin info for audit log
	sess.mu.RLock()
	adminNickname := sess.Nickname
	sess.mu.RUnlock()
	adminIP, _, _ := net.SplitHostPort(sess.RemoteAddr)

	// Create ban in database
	banID, err := s.db.CreateIPBan(msg.IPCIDR, msg.Reason, msg.DurationSeconds, adminNickname, adminIP)
	if err != nil {
		log.Printf("Failed to create IP ban: %v", err)
		return s.sendMessage(sess, protocol.TypeIPBanned, &protocol.IPBannedMessage{
			Success: false,
			Message: "Failed to create ban",
		})
	}

	log.Printf("Admin %s banned IP %s (ban_id=%d, reason=%s)", adminNickname, msg.IPCIDR, banID, msg.Reason)

	// Send success response
	return s.sendMessage(sess, protocol.TypeIPBanned, &protocol.IPBannedMessage{
		Success: true,
		BanID:   uint64(banID),
		Message: fmt.Sprintf("IP %s banned successfully", msg.IPCIDR),
	})
}

// handleUnbanUser handles UNBAN_USER message (admin only)
func (s *Server) handleUnbanUser(sess *Session, frame *protocol.Frame) error {
	// Check admin permissions
	if !s.isAdmin(sess) {
		return s.sendMessage(sess, protocol.TypeUserUnbanned, &protocol.UserUnbannedMessage{
			Success: false,
			Message: "Permission denied: admin access required",
		})
	}

	// Decode message
	msg := &protocol.UnbanUserMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Validate: must provide either UserID or Nickname
	if msg.UserID == nil && msg.Nickname == nil {
		return s.sendMessage(sess, protocol.TypeUserUnbanned, &protocol.UserUnbannedMessage{
			Success: false,
			Message: "Must provide either UserID or Nickname",
		})
	}

	// Get admin info for audit log
	sess.mu.RLock()
	adminNickname := sess.Nickname
	sess.mu.RUnlock()
	adminIP, _, _ := net.SplitHostPort(sess.RemoteAddr)

	// Convert UserID if provided
	var userID *int64
	if msg.UserID != nil {
		id := int64(*msg.UserID)
		userID = &id
	}

	// Delete ban from database
	rowsAffected, err := s.db.DeleteUserBan(userID, msg.Nickname, adminNickname, adminIP)
	if err != nil {
		log.Printf("Failed to delete user ban: %v", err)
		return s.sendMessage(sess, protocol.TypeUserUnbanned, &protocol.UserUnbannedMessage{
			Success: false,
			Message: "Failed to remove ban",
		})
	}

	if rowsAffected == 0 {
		return s.sendMessage(sess, protocol.TypeUserUnbanned, &protocol.UserUnbannedMessage{
			Success: false,
			Message: "No active ban found for this user",
		})
	}

	targetIdentifier := ""
	if msg.Nickname != nil {
		targetIdentifier = *msg.Nickname
	} else if msg.UserID != nil {
		targetIdentifier = fmt.Sprintf("user_id:%d", *msg.UserID)
	}

	log.Printf("Admin %s unbanned user %s (%d bans removed)", adminNickname, targetIdentifier, rowsAffected)

	// Send success response
	return s.sendMessage(sess, protocol.TypeUserUnbanned, &protocol.UserUnbannedMessage{
		Success: true,
		Message: fmt.Sprintf("User %s unbanned successfully (%d bans removed)", targetIdentifier, rowsAffected),
	})
}

// handleUnbanIP handles UNBAN_IP message (admin only)
func (s *Server) handleUnbanIP(sess *Session, frame *protocol.Frame) error {
	// Check admin permissions
	if !s.isAdmin(sess) {
		return s.sendMessage(sess, protocol.TypeIPUnbanned, &protocol.IPUnbannedMessage{
			Success: false,
			Message: "Permission denied: admin access required",
		})
	}

	// Decode message
	msg := &protocol.UnbanIPMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Validate IPCIDR
	if msg.IPCIDR == "" {
		return s.sendMessage(sess, protocol.TypeIPUnbanned, &protocol.IPUnbannedMessage{
			Success: false,
			Message: "IP/CIDR address required",
		})
	}

	// Get admin info for audit log
	sess.mu.RLock()
	adminNickname := sess.Nickname
	sess.mu.RUnlock()
	adminIP, _, _ := net.SplitHostPort(sess.RemoteAddr)

	// Delete ban from database
	rowsAffected, err := s.db.DeleteIPBan(msg.IPCIDR, adminNickname, adminIP)
	if err != nil {
		log.Printf("Failed to delete IP ban: %v", err)
		return s.sendMessage(sess, protocol.TypeIPUnbanned, &protocol.IPUnbannedMessage{
			Success: false,
			Message: "Failed to remove ban",
		})
	}

	if rowsAffected == 0 {
		return s.sendMessage(sess, protocol.TypeIPUnbanned, &protocol.IPUnbannedMessage{
			Success: false,
			Message: "No active ban found for this IP",
		})
	}

	log.Printf("Admin %s unbanned IP %s (%d bans removed)", adminNickname, msg.IPCIDR, rowsAffected)

	// Send success response
	return s.sendMessage(sess, protocol.TypeIPUnbanned, &protocol.IPUnbannedMessage{
		Success: true,
		Message: fmt.Sprintf("IP %s unbanned successfully (%d bans removed)", msg.IPCIDR, rowsAffected),
	})
}

// handleListBans handles LIST_BANS message (admin only)
func (s *Server) handleListBans(sess *Session, frame *protocol.Frame) error {
	// Check admin permissions
	if !s.isAdmin(sess) {
		return s.sendError(sess, protocol.ErrCodePermissionDenied, "Permission denied: admin access required")
	}

	// Decode message
	msg := &protocol.ListBansMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Get bans from database
	bans, err := s.db.ListBans(msg.IncludeExpired)
	if err != nil {
		log.Printf("Failed to list bans: %v", err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to retrieve ban list")
	}

	// Convert to protocol format
	banEntries := make([]protocol.BanEntry, len(bans))
	for i, ban := range bans {
		var userID *uint64
		if ban.UserID != nil {
			id := uint64(*ban.UserID)
			userID = &id
		}

		banEntries[i] = protocol.BanEntry{
			ID:          uint64(ban.ID),
			Type:        ban.BanType,
			UserID:      userID,
			Nickname:    ban.Nickname,
			IPCIDR:      ban.IPCIDR,
			Reason:      ban.Reason,
			Shadowban:   ban.Shadowban,
			BannedAt:    ban.BannedAt,
			BannedUntil: ban.BannedUntil,
			BannedBy:    ban.BannedBy,
		}
	}

	// Send response
	resp := &protocol.BanListMessage{
		Bans: banEntries,
	}
	return s.sendMessage(sess, protocol.TypeBanList, resp)
}

// handleDeleteUser handles DELETE_USER message (admin only)
func (s *Server) handleDeleteUser(sess *Session, frame *protocol.Frame) error {
	// Check admin permissions
	if !s.isAdmin(sess) {
		return s.sendMessage(sess, protocol.TypeUserDeleted, &protocol.UserDeletedMessage{
			Success: false,
			Message: "Permission denied: admin access required",
		})
	}

	// Decode message
	msg := &protocol.DeleteUserMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Get user info before deletion (for logging and session cleanup)
	user, err := s.db.GetUserByID(int64(msg.UserID))
	if err != nil {
		return s.sendMessage(sess, protocol.TypeUserDeleted, &protocol.UserDeletedMessage{
			Success: false,
			Message: "User not found",
		})
	}

	// Don't allow admins to delete themselves
	sess.mu.RLock()
	adminUserID := sess.UserID
	sess.mu.RUnlock()

	if adminUserID != nil && uint64(*adminUserID) == msg.UserID {
		return s.sendMessage(sess, protocol.TypeUserDeleted, &protocol.UserDeletedMessage{
			Success: false,
			Message: "Cannot delete your own account",
		})
	}

	// Get all active sessions for this user (for disconnection)
	allSessions := s.sessions.GetAllSessions()
	targetSessions := make([]*Session, 0)
	for _, targetSess := range allSessions {
		targetSess.mu.RLock()
		if targetSess.UserID != nil && uint64(*targetSess.UserID) == msg.UserID {
			targetSessions = append(targetSessions, targetSess)
		}
		targetSess.mu.RUnlock()
	}

	// Log admin action
	if adminUserID != nil {
		if err := s.db.LogAdminAction(uint64(*adminUserID), sess.Nickname, "DELETE_USER",
			fmt.Sprintf("user_id=%d nickname=%s", msg.UserID, user.Nickname)); err != nil {
			log.Printf("Failed to log admin action: %v", err)
		}
	}

	// Delete user (anonymizes messages, removes from DB)
	deletedNickname, err := s.db.DeleteUser(msg.UserID)
	if err != nil {
		return s.sendMessage(sess, protocol.TypeUserDeleted, &protocol.UserDeletedMessage{
			Success: false,
			Message: fmt.Sprintf("Failed to delete user: %v", err),
		})
	}

	// Disconnect all active sessions for this user
	for _, targetSess := range targetSessions {
		log.Printf("Disconnecting session %d for deleted user %s (id=%d)", targetSess.ID, deletedNickname, msg.UserID)
		s.removeSession(targetSess.ID)
	}

	// Send success response
	resp := &protocol.UserDeletedMessage{
		Success: true,
		Message: fmt.Sprintf("User '%s' deleted successfully (messages anonymized, %d sessions disconnected)", deletedNickname, len(targetSessions)),
	}

	if err := s.sendMessage(sess, protocol.TypeUserDeleted, resp); err != nil {
		return err
	}

	// Broadcast to all connected clients
	if err := s.broadcastToAll(protocol.TypeUserDeleted, resp); err != nil {
		log.Printf("Failed to broadcast user deletion: %v", err)
	}

	return nil
}

// handleDeleteChannel handles DELETE_CHANNEL message (admin only)
func (s *Server) handleDeleteChannel(sess *Session, frame *protocol.Frame) error {
	// Check admin permissions
	if !s.isAdmin(sess) {
		return s.sendMessage(sess, protocol.TypeChannelDeleted, &protocol.ChannelDeletedMessage{
			Success:   false,
			ChannelID: 0,
			Message:   "Permission denied: admin access required",
		})
	}

	// Decode message
	msg := &protocol.DeleteChannelMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Get channel info before deletion (for logging and broadcast message)
	channel, err := s.db.GetChannel(int64(msg.ChannelID))
	if err != nil {
		return s.sendMessage(sess, protocol.TypeChannelDeleted, &protocol.ChannelDeletedMessage{
			Success:   false,
			ChannelID: msg.ChannelID,
			Message:   "Channel not found",
		})
	}

	// Log admin action
	sess.mu.RLock()
	adminNickname := sess.Nickname
	adminUserID := sess.UserID
	sess.mu.RUnlock()

	if adminUserID != nil {
		if err := s.db.LogAdminAction(uint64(*adminUserID), adminNickname, "DELETE_CHANNEL",
			fmt.Sprintf("channel_id=%d name=%s reason=%s", msg.ChannelID, channel.Name, msg.Reason)); err != nil {
			log.Printf("Failed to log admin action: %v", err)
		}
	}

	// Delete the channel (cascades to messages, subchannels, subscriptions)
	if err := s.db.DeleteChannel(uint64(msg.ChannelID)); err != nil {
		return s.sendMessage(sess, protocol.TypeChannelDeleted, &protocol.ChannelDeletedMessage{
			Success:   false,
			ChannelID: msg.ChannelID,
			Message:   fmt.Sprintf("Failed to delete channel: %v", err),
		})
	}

	// Send success response
	resp := &protocol.ChannelDeletedMessage{
		Success:   true,
		ChannelID: msg.ChannelID,
		Message:   fmt.Sprintf("Channel '%s' deleted successfully", channel.Name),
	}

	if err := s.sendMessage(sess, protocol.TypeChannelDeleted, resp); err != nil {
		return err
	}

	// Broadcast to all connected clients
	if err := s.broadcastToAll(protocol.TypeChannelDeleted, resp); err != nil {
		log.Printf("Failed to broadcast channel deletion: %v", err)
	}

	return nil
}

func (s *Server) handleGetUnreadCounts(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.GetUnreadCountsMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	// Determine the reference timestamp
	var sinceTimestamp int64
	if msg.SinceTimestamp != nil {
		// Client provided explicit timestamp
		sinceTimestamp = *msg.SinceTimestamp
	} else {
		// Use server-stored state (registered users only)
		sess.mu.RLock()
		userID := sess.UserID
		sess.mu.RUnlock()

		if userID == nil {
			return s.sendError(sess, protocol.ErrCodeAuthRequired, "Anonymous users must provide since_timestamp")
		}
	}

	// Build response with counts for each target
	counts := make([]protocol.UnreadCount, 0, len(msg.Targets))

	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	for _, target := range msg.Targets {
		var count uint32
		var err error

		// If no explicit timestamp provided, get user's last read timestamp for this target
		timestamp := sinceTimestamp
		if msg.SinceTimestamp == nil && userID != nil {
			timestamp, err = s.db.GetUserChannelState(uint64(*userID), target.ChannelID, target.SubchannelID)
			if err != nil {
				log.Printf("[ERROR] Failed to get user channel state: %v", err)
				return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to retrieve read state")
			}
		}

		// Count based on whether thread_id is specified
		if target.ThreadID != nil {
			// Thread-specific count
			count, err = s.db.GetUnreadCountForThread(*target.ThreadID, timestamp)
		} else {
			// Channel-wide count
			count, err = s.db.GetUnreadCountForChannel(target.ChannelID, target.SubchannelID, timestamp)
		}

		if err != nil {
			log.Printf("[ERROR] Failed to get unread count: %v", err)
			return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to count unread messages")
		}

		counts = append(counts, protocol.UnreadCount{
			ChannelID:    target.ChannelID,
			SubchannelID: target.SubchannelID,
			ThreadID:     target.ThreadID,
			UnreadCount:  count,
		})
	}

	// Send response
	resp := &protocol.UnreadCountsMessage{
		Counts: counts,
	}
	return s.sendMessage(sess, protocol.TypeUnreadCounts, resp)
}

func (s *Server) handleUpdateReadState(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.UpdateReadStateMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid message format")
	}

	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	// Only registered users can update read state on server
	// Anonymous users handle this locally
	if userID == nil {
		// Silent success for anonymous users (they track locally)
		return nil
	}

	// Update the user's read state
	if err := s.db.UpdateUserChannelState(uint64(*userID), msg.ChannelID, msg.SubchannelID, msg.Timestamp); err != nil {
		log.Printf("[ERROR] Failed to update read state: %v", err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to update read state")
	}

	// Silent success (no response message defined for UPDATE_READ_STATE)
	return nil
}

// ============================================================================
// V3 Direct Message (DM) Handlers
// ============================================================================

// handleStartDM initiates a DM conversation with another user
func (s *Server) handleStartDM(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.StartDMMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid START_DM format")
	}

	sess.mu.RLock()
	initiatorUserID := sess.UserID
	initiatorNickname := sess.Nickname
	sess.mu.RUnlock()

	// Find target user
	var targetUser *database.User
	var targetSession *Session
	var err error

	switch msg.TargetType {
	case protocol.DMTargetByUserID:
		targetUser, err = s.db.GetUserByID(int64(msg.TargetUserID))
		if err != nil {
			return s.sendError(sess, protocol.ErrCodeNotFound, "User not found")
		}
	case protocol.DMTargetByNickname:
		// First try registered user
		targetUser, err = s.db.GetUserByNickname(msg.TargetNickname)
		if err != nil {
			// Try to find an online session with this nickname
			for _, session := range s.sessions.GetAllSessions() {
				session.mu.RLock()
				if session.Nickname == msg.TargetNickname {
					targetSession = session
					session.mu.RUnlock()
					break
				}
				session.mu.RUnlock()
			}

			if targetSession == nil {
				return s.sendError(sess, protocol.ErrCodeNotFound, "User not found")
			}
		}
	case protocol.DMTargetBySessionID:
		// Find session by ID
		allSessions := s.sessions.GetAllSessions()
		for _, session := range allSessions {
			if session.ID == msg.TargetUserID {
				targetSession = session
				break
			}
		}
		if targetSession == nil {
			return s.sendError(sess, protocol.ErrCodeNotFound, "Session not found")
		}
	default:
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid target type")
	}

	// Get target user ID if we found a registered user
	var targetUserID *int64
	var targetNickname string
	if targetUser != nil {
		targetUserID = &targetUser.ID
		targetNickname = targetUser.Nickname
	} else if targetSession != nil {
		targetSession.mu.RLock()
		targetUserID = targetSession.UserID
		targetNickname = targetSession.Nickname
		targetSession.mu.RUnlock()
	}

	// Check if DM already exists between these users
	if initiatorUserID != nil && targetUserID != nil {
		existingDM, err := s.db.GetDMChannelBetweenUsers(*initiatorUserID, *targetUserID)
		if err == nil && existingDM != nil {
			// DM already exists, send DM_READY with existing channel
			return s.sendExistingDMReady(sess, existingDM, targetUser)
		}
	}

	// Check encryption keys
	var initiatorHasKey, targetHasKey bool
	var initiatorPubKey, targetPubKey []byte

	if initiatorUserID != nil {
		initiatorPubKey, _ = s.db.GetUserEncryptionKey(*initiatorUserID)
		initiatorHasKey = len(initiatorPubKey) == 32
	}
	if targetUserID != nil {
		targetPubKey, _ = s.db.GetUserEncryptionKey(*targetUserID)
		targetHasKey = len(targetPubKey) == 32
	}

	// Determine if encryption is possible
	canEncrypt := initiatorHasKey && targetHasKey

	// If initiator doesn't have a key and doesn't allow unencrypted
	if !initiatorHasKey && !msg.AllowUnencrypted {
		return s.sendKeyRequired(sess, "You need to set up an encryption key before starting encrypted DMs", nil)
	}

	// If both have keys, create encrypted DM immediately
	if canEncrypt && initiatorUserID != nil && targetUserID != nil {
		channelID, err := s.db.CreateDMChannel(*initiatorUserID, *targetUserID, true)
		if err != nil {
			log.Printf("[ERROR] Failed to create DM channel: %v", err)
			return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to create DM channel")
		}

		// Send DM_READY to initiator
		var targetPubKeyArr [32]byte
		copy(targetPubKeyArr[:], targetPubKey)
		initiatorReady := &protocol.DMReadyMessage{
			ChannelID:      uint64(channelID),
			OtherUserID:    toUint64Ptr(targetUserID),
			OtherNickname:  targetNickname,
			IsEncrypted:    true,
			OtherPublicKey: targetPubKeyArr,
		}
		if err := s.sendMessage(sess, protocol.TypeDMReady, initiatorReady); err != nil {
			return err
		}

		// Send DM_READY to target if online
		if targetSession != nil || targetUserID != nil {
			var initiatorPubKeyArr [32]byte
			copy(initiatorPubKeyArr[:], initiatorPubKey)
			targetReady := &protocol.DMReadyMessage{
				ChannelID:      uint64(channelID),
				OtherUserID:    toUint64Ptr(initiatorUserID),
				OtherNickname:  initiatorNickname,
				IsEncrypted:    true,
				OtherPublicKey: initiatorPubKeyArr,
			}
			s.sendToUserOrSession(targetUserID, targetSession, protocol.TypeDMReady, targetReady)
		}

		return nil
	}

	// Need consent flow - create invite
	if initiatorUserID == nil || targetUserID == nil {
		// Anonymous DMs require both to be online
		if targetSession == nil {
			return s.sendError(sess, protocol.ErrCodeNotFound, "Target user must be online for anonymous DMs")
		}
	}

	// Create DM invite (for unencrypted or key setup flow)
	isEncrypted := canEncrypt
	var targetSessID int64
	if targetSession != nil {
		targetSessID = targetSession.DBSessionID
	}
	dbInviteID, err := s.db.CreateDMInviteWithSessions(initiatorUserID, targetUserID, sess.DBSessionID, targetSessID, isEncrypted)
	if err != nil {
		log.Printf("[ERROR] Failed to create DM invite: %v", err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to create DM invite")
	}
	inviteID := uint64(dbInviteID)

	// Send DM_PENDING to initiator
	pending := &protocol.DMPendingMessage{
		DMChannelID:        inviteID,
		WaitingForUserID:   toUint64Ptr(targetUserID),
		WaitingForNickname: targetNickname,
		Reason:             "Waiting for " + targetNickname + " to accept",
	}
	if err := s.sendMessage(sess, protocol.TypeDMPending, pending); err != nil {
		return err
	}

	// Send DM_REQUEST to target
	// Determine encryption status for the recipient
	var encryptionStatus uint8
	if !initiatorHasKey {
		// Initiator has no key - encryption is not possible
		encryptionStatus = protocol.DMEncryptionNotPossible
	} else if !msg.AllowUnencrypted {
		// Initiator has key and requires encryption
		encryptionStatus = protocol.DMEncryptionRequired
	} else {
		// Initiator has key but allows unencrypted
		encryptionStatus = protocol.DMEncryptionOptional
	}

	request := &protocol.DMRequestMessage{
		DMChannelID:      inviteID,
		FromUserID:       toUint64Ptr(initiatorUserID),
		FromNickname:     initiatorNickname,
		EncryptionStatus: encryptionStatus,
	}
	s.sendToUserOrSession(targetUserID, targetSession, protocol.TypeDMRequest, request)

	return nil
}

// handleProvidePublicKey stores a user's encryption public key
func (s *Server) handleProvidePublicKey(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.ProvidePublicKeyMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid PROVIDE_PUBLIC_KEY format")
	}

	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	// Anonymous users can have ephemeral keys (session-only)
	if userID == nil {
		if msg.KeyType != protocol.KeyTypeEphemeral {
			return s.sendError(sess, protocol.ErrCodePermissionDenied, "Anonymous users can only use ephemeral keys")
		}
		// Store in session (not database)
		sess.mu.Lock()
		sess.EncryptionPublicKey = msg.PublicKey[:]
		sess.mu.Unlock()

		// Check for any pending DM invites that were waiting for this key
		// (Would need session-based invite tracking for anonymous users)
		return nil
	}

	// Store key for registered user
	if err := s.db.SetUserEncryptionKey(*userID, msg.PublicKey[:]); err != nil {
		log.Printf("[ERROR] Failed to store encryption key: %v", err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to store encryption key")
	}

	// Check for pending DM invites where this user is the target
	invites, err := s.db.GetPendingDMInvitesForUser(*userID)
	if err != nil {
		log.Printf("[WARN] Failed to get pending invites: %v", err)
	}

	// Process any pending invites that were waiting for this user's key
	for _, invite := range invites {
		s.processPendingInviteAfterKey(sess, invite)
	}

	return nil
}

// handleAllowUnencrypted accepts an unencrypted DM request
func (s *Server) handleAllowUnencrypted(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.AllowUnencryptedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid ALLOW_UNENCRYPTED format")
	}

	// Get the invite from database
	invite, err := s.db.GetDMInvite(int64(msg.DMChannelID))
	if err != nil || invite == nil {
		return s.sendError(sess, protocol.ErrCodeNotFound, "DM invite not found")
	}

	// Verify this session is the target of the invite
	sess.mu.RLock()
	userID := sess.UserID
	sess.mu.RUnlock()

	isAuthorized := false
	if invite.TargetSessionID != nil && *invite.TargetSessionID == sess.DBSessionID {
		// Session-based authorization (for anonymous users)
		isAuthorized = true
	} else if invite.TargetUserID != nil && userID != nil && *userID == *invite.TargetUserID {
		// User-based authorization (for registered users)
		isAuthorized = true
	}
	if !isAuthorized {
		return s.sendError(sess, protocol.ErrCodePermissionDenied, "Not authorized for this invite")
	}

	// Get nicknames from sessions
	var initiatorNickname, targetNickname string
	if invite.InitiatorSessionID != nil {
		if initSess, ok := s.sessions.GetSessionByDBID(*invite.InitiatorSessionID); ok {
			initSess.mu.RLock()
			initiatorNickname = initSess.Nickname
			initSess.mu.RUnlock()
		}
	}
	if invite.TargetSessionID != nil {
		if targetSess, ok := s.sessions.GetSessionByDBID(*invite.TargetSessionID); ok {
			targetSess.mu.RLock()
			targetNickname = targetSess.Nickname
			targetSess.mu.RUnlock()
		}
	}

	// For registered users, also try to get nickname from User table
	if initiatorNickname == "" && invite.InitiatorUserID != nil {
		if user, _ := s.db.GetUserByID(*invite.InitiatorUserID); user != nil {
			initiatorNickname = user.Nickname
		}
	}
	if targetNickname == "" && invite.TargetUserID != nil {
		if user, _ := s.db.GetUserByID(*invite.TargetUserID); user != nil {
			targetNickname = user.Nickname
		}
	}

	// Create a real DM channel with participants (works for both registered and anonymous users)
	var initSessionID int64
	if invite.InitiatorSessionID != nil {
		initSessionID = *invite.InitiatorSessionID
	}
	targetSessionID := sess.DBSessionID

	channelID, err := s.db.CreateDMChannelWithParticipants(
		invite.InitiatorUserID, initSessionID, initiatorNickname,
		invite.TargetUserID, targetSessionID, targetNickname,
	)
	if err != nil {
		log.Printf("[ERROR] Failed to create DM channel: %v", err)
		return s.sendError(sess, protocol.ErrCodeDatabaseError, "Failed to create DM")
	}

	// Delete the invite
	s.db.DeleteDMInvite(invite.ID)

	// Send DM_READY to target (this user)
	var initiatorUserIDPtr *uint64
	if invite.InitiatorUserID != nil {
		initiatorUserIDPtr = toUint64Ptr(invite.InitiatorUserID)
	}
	targetReady := &protocol.DMReadyMessage{
		ChannelID:     uint64(channelID),
		OtherUserID:   initiatorUserIDPtr,
		OtherNickname: initiatorNickname,
		IsEncrypted:   false,
	}
	if err := s.sendMessage(sess, protocol.TypeDMReady, targetReady); err != nil {
		return err
	}

	// Send DM_READY to initiator
	var targetUserIDPtr *uint64
	if invite.TargetUserID != nil {
		targetUserIDPtr = toUint64Ptr(invite.TargetUserID)
	}
	initiatorReady := &protocol.DMReadyMessage{
		ChannelID:     uint64(channelID),
		OtherUserID:   targetUserIDPtr,
		OtherNickname: targetNickname,
		IsEncrypted:   false,
	}

	// Send to initiator - try by user ID first, then by session ID
	if invite.InitiatorUserID != nil {
		s.sendToUser(*invite.InitiatorUserID, protocol.TypeDMReady, initiatorReady)
	} else if invite.InitiatorSessionID != nil {
		if initSess, ok := s.sessions.GetSessionByDBID(*invite.InitiatorSessionID); ok {
			s.sendMessage(initSess, protocol.TypeDMReady, initiatorReady)
		}
	}

	return nil
}

// handleDeclineDM handles DECLINE_DM message - when target declines an incoming DM request
func (s *Server) handleDeclineDM(sess *Session, frame *protocol.Frame) error {
	msg := &protocol.DeclineDMMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		return s.sendError(sess, protocol.ErrCodeInvalidFormat, "Invalid DECLINE_DM format")
	}

	// Get the invite from database
	invite, err := s.db.GetDMInvite(int64(msg.DMChannelID))
	if err != nil || invite == nil {
		// Invite might already be deleted, just return success
		return nil
	}

	// Verify this session is the target of the invite
	sess.mu.RLock()
	userID := sess.UserID
	nickname := sess.Nickname
	sess.mu.RUnlock()

	isAuthorized := false
	if invite.TargetSessionID != nil && *invite.TargetSessionID == sess.DBSessionID {
		isAuthorized = true
	} else if invite.TargetUserID != nil && userID != nil && *userID == *invite.TargetUserID {
		isAuthorized = true
	}
	if !isAuthorized {
		return s.sendError(sess, protocol.ErrCodePermissionDenied, "Not authorized for this invite")
	}

	// Delete the invite
	s.db.DeleteDMInvite(invite.ID)

	// Notify the initiator that their DM request was declined
	declinedMsg := &protocol.DMDeclinedMessage{
		DMChannelID: msg.DMChannelID,
		UserID:      toUint64Ptr(userID),
		Nickname:    nickname,
	}

	// Send to initiator - try by user ID first, then by session ID
	if invite.InitiatorUserID != nil {
		s.sendToUser(*invite.InitiatorUserID, protocol.TypeDMDeclined, declinedMsg)
	} else if invite.InitiatorSessionID != nil {
		if initSess, ok := s.sessions.GetSessionByDBID(*invite.InitiatorSessionID); ok {
			s.sendMessage(initSess, protocol.TypeDMDeclined, declinedMsg)
		}
	}

	log.Printf("[DM] User %s declined DM invite %d", nickname, invite.ID)
	return nil
}

// Helper: send DM_READY for an existing DM channel
func (s *Server) sendExistingDMReady(sess *Session, dm *database.Channel, otherUser *database.User) error {
	sess.mu.RLock()
	currentUserID := sess.UserID
	sess.mu.RUnlock()

	var otherPubKey [32]byte
	isEncrypted := false

	if otherUser != nil && currentUserID != nil {
		// Get encryption keys if both users have them
		currentKey, _ := s.db.GetUserEncryptionKey(*currentUserID)
		otherKey, _ := s.db.GetUserEncryptionKey(otherUser.ID)
		if len(currentKey) == 32 && len(otherKey) == 32 {
			isEncrypted = true
			copy(otherPubKey[:], otherKey)
		}
	}

	var otherUserID *uint64
	var otherNickname string
	if otherUser != nil {
		uid := uint64(otherUser.ID)
		otherUserID = &uid
		otherNickname = otherUser.Nickname
	}

	ready := &protocol.DMReadyMessage{
		ChannelID:      uint64(dm.ID),
		OtherUserID:    otherUserID,
		OtherNickname:  otherNickname,
		IsEncrypted:    isEncrypted,
		OtherPublicKey: otherPubKey,
	}
	return s.sendMessage(sess, protocol.TypeDMReady, ready)
}

// Helper: send KEY_REQUIRED message
func (s *Server) sendKeyRequired(sess *Session, reason string, channelID *uint64) error {
	msg := &protocol.KeyRequiredMessage{
		Reason:      reason,
		DMChannelID: channelID,
	}
	return s.sendMessage(sess, protocol.TypeKeyRequired, msg)
}

// Helper: send message to a user by ID (find their session)
func (s *Server) sendToUser(userID int64, msgType byte, msg protocol.ProtocolMessage) {
	for _, session := range s.sessions.GetAllSessions() {
		session.mu.RLock()
		if session.UserID != nil && *session.UserID == userID {
			session.mu.RUnlock()
			s.sendMessage(session, msgType, msg)
			return
		}
		session.mu.RUnlock()
	}
}

// Helper: send message to user by ID or session
func (s *Server) sendToUserOrSession(userID *int64, targetSession *Session, msgType byte, msg protocol.ProtocolMessage) {
	if targetSession != nil {
		s.sendMessage(targetSession, msgType, msg)
		return
	}
	if userID != nil {
		s.sendToUser(*userID, msgType, msg)
	}
}

// Helper: convert *int64 to *uint64
func toUint64Ptr(i *int64) *uint64 {
	if i == nil {
		return nil
	}
	u := uint64(*i)
	return &u
}

// Helper: process pending invite after user provides key
func (s *Server) processPendingInviteAfterKey(sess *Session, invite *database.DMInvite) {
	// Encrypted DMs only apply to registered users
	if invite.InitiatorUserID == nil || invite.TargetUserID == nil {
		return
	}

	// Check if initiator also has a key now
	initiatorKey, _ := s.db.GetUserEncryptionKey(*invite.InitiatorUserID)
	targetKey, _ := s.db.GetUserEncryptionKey(*invite.TargetUserID)

	if len(initiatorKey) == 32 && len(targetKey) == 32 {
		// Both have keys, create encrypted DM
		channelID, err := s.db.CreateDMChannel(*invite.InitiatorUserID, *invite.TargetUserID, true)
		if err != nil {
			log.Printf("[ERROR] Failed to create DM after key setup: %v", err)
			return
		}

		// Delete the invite
		s.db.DeleteDMInvite(invite.ID)

		// Get user info
		initiatorUser, _ := s.db.GetUserByID(*invite.InitiatorUserID)
		targetUser, _ := s.db.GetUserByID(*invite.TargetUserID)

		// Send DM_READY to both
		var initiatorPubKey, targetPubKey [32]byte
		copy(initiatorPubKey[:], initiatorKey)
		copy(targetPubKey[:], targetKey)

		if targetUser != nil {
			targetReady := &protocol.DMReadyMessage{
				ChannelID:      uint64(channelID),
				OtherUserID:    toUint64Ptr(invite.InitiatorUserID),
				OtherNickname:  initiatorUser.Nickname,
				IsEncrypted:    true,
				OtherPublicKey: initiatorPubKey,
			}
			s.sendMessage(sess, protocol.TypeDMReady, targetReady)
		}

		if initiatorUser != nil {
			initiatorReady := &protocol.DMReadyMessage{
				ChannelID:      uint64(channelID),
				OtherUserID:    toUint64Ptr(invite.TargetUserID),
				OtherNickname:  targetUser.Nickname,
				IsEncrypted:    true,
				OtherPublicKey: targetPubKey,
			}
			s.sendToUser(*invite.InitiatorUserID, protocol.TypeDMReady, initiatorReady)
		}
	}
}

// notifyDMParticipantsOfDisconnect notifies DM participants when a user disconnects
func (s *Server) notifyDMParticipantsOfDisconnect(userID *int64, sessionID int64, nickname string) {
	// Find all DM channels this user/session is a participant in
	dmChannels, err := s.db.GetDMChannelsForParticipant(userID, sessionID)
	if err != nil {
		log.Printf("[DM] Failed to get DM channels for disconnecting user: %v", err)
		return
	}

	for _, channelID := range dmChannels {
		// Get other participants
		participants, err := s.db.GetChannelParticipants(channelID)
		if err != nil {
			continue
		}

		// For anonymous users, remove them from the channel
		if userID == nil {
			s.db.RemoveParticipantBySessionID(channelID, sessionID)
		}

		// Create a system message so it persists in history
		systemContent := fmt.Sprintf("%s has left the conversation", nickname)
		s.db.CreateSystemMessage(channelID, systemContent)

		// Notify other participants (real-time)
		leftMsg := &protocol.DMParticipantLeftMessage{
			DMChannelID: uint64(channelID),
			UserID:      toUint64Ptr(userID),
			Nickname:    nickname,
		}

		for _, p := range participants {
			// Skip the disconnecting user
			if userID != nil && p.UserID != nil && *p.UserID == *userID {
				continue
			}
			if userID == nil && p.SessionID != nil && *p.SessionID == sessionID {
				continue
			}

			// Send notification
			if p.UserID != nil {
				s.sendToUser(*p.UserID, protocol.TypeDMParticipantLeft, leftMsg)
			} else if p.SessionID != nil {
				if sess, ok := s.sessions.GetSessionByDBID(*p.SessionID); ok {
					s.sendMessage(sess, protocol.TypeDMParticipantLeft, leftMsg)
				}
			}
		}

		// Check if channel is now empty (for anonymous users)
		if userID == nil {
			remaining, _ := s.db.GetChannelParticipants(channelID)
			if len(remaining) == 0 {
				s.db.DeleteChannel(uint64(channelID))
				log.Printf("[DM] Deleted empty DM channel %d after disconnect", channelID)
			}
		}
	}
}
