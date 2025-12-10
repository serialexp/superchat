// Package botlib provides a simple library for building SuperChat bots.
package botlib

import (
	"strings"
	"time"
)

// Message represents a chat message received by the bot.
type Message struct {
	ID             uint64
	ChannelID      uint64
	ParentID       *uint64 // nil for root messages/threads
	AuthorUserID   *uint64 // nil for anonymous users
	AuthorNickname string
	Content        string
	CreatedAt      time.Time
	ReplyCount     uint32

	// Internal: the bot's nickname for mention detection
	botNickname string
}

// IsThread returns true if this message is a thread root (has no parent).
func (m *Message) IsThread() bool {
	return m.ParentID == nil
}

// IsReply returns true if this message is a reply to another message.
func (m *Message) IsReply() bool {
	return m.ParentID != nil
}

// ThreadID returns the thread root ID. For replies, this is the ParentID.
// For thread roots, this is the message's own ID.
func (m *Message) ThreadID() uint64 {
	if m.ParentID != nil {
		return *m.ParentID
	}
	return m.ID
}

// MentionsMe returns true if the message content mentions the bot.
// Checks for @nickname patterns (case-insensitive).
func (m *Message) MentionsMe() bool {
	if m.botNickname == "" {
		return false
	}

	content := strings.ToLower(m.Content)
	nickname := strings.ToLower(m.botNickname)

	// Check for @nickname mention
	if strings.Contains(content, "@"+nickname) {
		return true
	}

	// Also check for nickname at start of message (common pattern)
	if strings.HasPrefix(content, nickname+":") ||
		strings.HasPrefix(content, nickname+",") ||
		strings.HasPrefix(content, nickname+" ") {
		return true
	}

	return false
}

// MentionedContent returns the message content with the bot mention removed.
// Useful for extracting the actual query/command.
func (m *Message) MentionedContent() string {
	if m.botNickname == "" {
		return m.Content
	}

	content := m.Content
	nickname := m.botNickname

	// Remove @nickname mentions
	content = strings.ReplaceAll(content, "@"+nickname, "")
	content = strings.ReplaceAll(content, "@"+strings.ToLower(nickname), "")

	// Remove nickname: or nickname, prefix
	lower := strings.ToLower(content)
	lowerNick := strings.ToLower(nickname)
	if strings.HasPrefix(lower, lowerNick+":") {
		content = content[len(nickname)+1:]
	} else if strings.HasPrefix(lower, lowerNick+",") {
		content = content[len(nickname)+1:]
	} else if strings.HasPrefix(lower, lowerNick+" ") {
		content = content[len(nickname)+1:]
	}

	return strings.TrimSpace(content)
}

// IsFromUser returns true if the message is from a registered user (not anonymous).
func (m *Message) IsFromUser() bool {
	return m.AuthorUserID != nil
}

// Channel represents a chat channel.
type Channel struct {
	ID              uint64
	Name            string
	Description     string
	UserCount       uint32
	Type            uint8  // 0 = threaded forum, 1 = linear chat
	RetentionHours  uint32
	HasSubchannels  bool
	SubchannelCount uint16
}

// IsChat returns true if this is a linear chat channel (not threaded).
func (c *Channel) IsChat() bool {
	return c.Type == 1
}

// IsForum returns true if this is a threaded forum channel.
func (c *Channel) IsForum() bool {
	return c.Type == 0
}

// Subchannel represents a subchannel within a channel.
type Subchannel struct {
	ID             uint64
	Name           string
	Description    string
	Type           uint8
	RetentionHours uint32
}
