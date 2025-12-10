package botlib

import (
	"fmt"
	"time"
)

// Context provides methods for responding to messages.
// It is passed to message handlers and provides a convenient API
// for common bot actions.
type Context struct {
	bot     *Bot
	message *Message
}

// Message returns the message that triggered this context.
func (c *Context) Message() *Message {
	return c.message
}

// Reply sends a reply to the current message's thread.
// For thread roots, this creates a reply in that thread.
// For replies, this also replies to the same thread (not nested).
func (c *Context) Reply(content string) error {
	threadID := c.message.ThreadID()
	return c.bot.postMessage(c.message.ChannelID, &threadID, content)
}

// ReplyTo sends a reply to a specific message ID.
func (c *Context) ReplyTo(messageID uint64, content string) error {
	return c.bot.postMessage(c.message.ChannelID, &messageID, content)
}

// NewThread creates a new thread in the current channel.
func (c *Context) NewThread(content string) error {
	return c.bot.postMessage(c.message.ChannelID, nil, content)
}

// ChannelID returns the channel ID where the message was received.
func (c *Context) ChannelID() uint64 {
	return c.message.ChannelID
}

// Author returns the nickname of the message author.
func (c *Context) Author() string {
	return c.message.AuthorNickname
}

// BotNickname returns the bot's current nickname.
func (c *Context) BotNickname() string {
	return c.bot.nickname
}

// Log logs a message using the bot's logger.
func (c *Context) Log(format string, args ...interface{}) {
	if c.bot.logger != nil {
		c.bot.logger.Printf(format, args...)
	}
}

// PostMessageResult contains the result of posting a message.
type PostMessageResult struct {
	MessageID uint64
	Timestamp time.Time
}

// ReplyWithResult sends a reply and returns the posted message details.
func (c *Context) ReplyWithResult(content string) (*PostMessageResult, error) {
	threadID := c.message.ThreadID()
	return c.bot.postMessageWithResult(c.message.ChannelID, &threadID, content)
}

// NewThreadWithResult creates a new thread and returns the posted message details.
func (c *Context) NewThreadWithResult(content string) (*PostMessageResult, error) {
	return c.bot.postMessageWithResult(c.message.ChannelID, nil, content)
}

// FetchThread fetches all messages in the current thread.
func (c *Context) FetchThread() ([]Message, error) {
	threadID := c.message.ThreadID()
	return c.bot.fetchMessages(c.message.ChannelID, &threadID, 100)
}

// FetchThreadMessages fetches messages from a specific thread.
func (c *Context) FetchThreadMessages(threadID uint64, limit uint16) ([]Message, error) {
	return c.bot.fetchMessages(c.message.ChannelID, &threadID, limit)
}

// String returns a debug representation of the context.
func (c *Context) String() string {
	return fmt.Sprintf("Context{channel=%d, message=%d, author=%s}",
		c.message.ChannelID, c.message.ID, c.message.AuthorNickname)
}
