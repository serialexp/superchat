package botlib

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aeolun/superchat/pkg/protocol"
)

// MessageHandler is called when a new message is received.
type MessageHandler func(ctx *Context, msg *Message)

// Config holds the bot configuration.
type Config struct {
	// Server address (host:port)
	Server string

	// Nickname for the bot (e.g., "[Bot] Claude")
	Nickname string

	// Channels to join and monitor
	Channels []string

	// Logger for debug output (optional, defaults to stdout)
	Logger *log.Logger

	// ResponseTimeout for request/response operations (default: 10s)
	ResponseTimeout time.Duration

	// PingInterval for keepalive (default: 30s)
	PingInterval time.Duration
}

// Bot represents a SuperChat bot instance.
type Bot struct {
	config   Config
	conn     *connection
	logger   *log.Logger
	nickname string

	// Channel state
	channels   map[string]uint64 // name -> ID
	channelsMu sync.RWMutex

	// Track threads the bot has participated in
	myThreads   map[uint64]bool // threadID -> participated
	myThreadsMu sync.RWMutex

	// Handlers
	onMessage     MessageHandler
	onThreadReply MessageHandler
	onMention     MessageHandler

	// Lifecycle
	running bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// New creates a new Bot with the given configuration.
func New(config Config) *Bot {
	if config.Logger == nil {
		config.Logger = log.New(os.Stdout, "[bot] ", log.LstdFlags)
	}
	if config.ResponseTimeout == 0 {
		config.ResponseTimeout = 10 * time.Second
	}
	if config.PingInterval == 0 {
		config.PingInterval = 30 * time.Second
	}

	return &Bot{
		config:    config,
		logger:    config.Logger,
		nickname:  config.Nickname,
		channels:  make(map[string]uint64),
		myThreads: make(map[uint64]bool),
		stopCh:    make(chan struct{}),
	}
}

// OnMessage registers a handler for all new messages.
func (b *Bot) OnMessage(handler MessageHandler) {
	b.onMessage = handler
}

// OnThreadReply registers a handler for replies to threads the bot participated in.
func (b *Bot) OnThreadReply(handler MessageHandler) {
	b.onThreadReply = handler
}

// OnMention registers a handler for messages that mention the bot.
func (b *Bot) OnMention(handler MessageHandler) {
	b.onMention = handler
}

// Run connects to the server and starts processing messages.
// Blocks until Stop() is called or the connection is lost.
func (b *Bot) Run() error {
	b.conn = newConnection(b.config.Server)

	// Connect
	b.logger.Printf("Connecting to %s...", b.config.Server)
	if err := b.conn.connect(); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	// Set up frame handler for broadcasts
	b.conn.onFrame = b.handleFrame

	// Start receive loop
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.conn.receiveLoop()
	}()

	// Wait for server config
	frame, err := b.conn.waitForResponse(b.config.ResponseTimeout)
	if err != nil {
		b.conn.close()
		return fmt.Errorf("waiting for server config: %w", err)
	}
	if err := expectType(frame, protocol.TypeServerConfig); err != nil {
		b.conn.close()
		return fmt.Errorf("server config: %w", err)
	}
	b.logger.Printf("Received server config")

	// Set nickname
	b.logger.Printf("Setting nickname: %s", b.nickname)
	nickMsg := &protocol.SetNicknameMessage{Nickname: b.nickname}
	frame, err = b.conn.sendAndWait(protocol.TypeSetNickname, nickMsg, b.config.ResponseTimeout)
	if err != nil {
		b.conn.close()
		return fmt.Errorf("set nickname: %w", err)
	}
	if err := expectType(frame, protocol.TypeNicknameResponse); err != nil {
		b.conn.close()
		return fmt.Errorf("nickname response: %w", err)
	}
	resp := &protocol.NicknameResponseMessage{}
	if err := resp.Decode(frame.Payload); err != nil {
		b.conn.close()
		return fmt.Errorf("decode nickname response: %w", err)
	}
	if !resp.Success {
		b.conn.close()
		return fmt.Errorf("nickname rejected: %s", resp.Message)
	}
	b.logger.Printf("Nickname set successfully")

	// List and join channels
	if err := b.joinChannels(); err != nil {
		b.conn.close()
		return fmt.Errorf("join channels: %w", err)
	}

	// Start ping loop
	b.wg.Add(1)
	go b.pingLoop()

	b.running = true
	b.logger.Printf("Bot is running. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		b.logger.Printf("Shutdown signal received")
	case <-b.stopCh:
		b.logger.Printf("Stop requested")
	}

	return b.shutdown()
}

// Stop gracefully stops the bot.
func (b *Bot) Stop() {
	close(b.stopCh)
}

func (b *Bot) shutdown() error {
	b.running = false

	// Send disconnect
	b.conn.send(protocol.TypeDisconnect, &protocol.DisconnectMessage{})
	time.Sleep(100 * time.Millisecond)

	// Close connection
	b.conn.close()

	// Wait for goroutines
	b.wg.Wait()

	b.logger.Printf("Bot stopped")
	return nil
}

func (b *Bot) joinChannels() error {
	// List available channels
	frame, err := b.conn.sendAndWait(protocol.TypeListChannels, &protocol.ListChannelsMessage{}, b.config.ResponseTimeout)
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}
	if err := expectType(frame, protocol.TypeChannelList); err != nil {
		return err
	}

	channelList := &protocol.ChannelListMessage{}
	if err := channelList.Decode(frame.Payload); err != nil {
		return fmt.Errorf("decode channel list: %w", err)
	}

	// Build channel name -> ID map
	channelMap := make(map[string]*protocol.Channel)
	for i := range channelList.Channels {
		ch := &channelList.Channels[i]
		channelMap[ch.Name] = ch
	}

	// Join requested channels
	for _, name := range b.config.Channels {
		ch, ok := channelMap[name]
		if !ok {
			b.logger.Printf("Warning: channel %q not found", name)
			continue
		}

		// Join channel
		joinMsg := &protocol.JoinChannelMessage{ChannelID: ch.ID}
		frame, err := b.conn.sendAndWait(protocol.TypeJoinChannel, joinMsg, b.config.ResponseTimeout)
		if err != nil {
			b.logger.Printf("Warning: failed to join %q: %v", name, err)
			continue
		}
		if err := expectType(frame, protocol.TypeJoinResponse); err != nil {
			b.logger.Printf("Warning: join %q failed: %v", name, err)
			continue
		}

		// Subscribe to channel for new message notifications
		subMsg := &protocol.SubscribeChannelMessage{ChannelID: ch.ID}
		frame, err = b.conn.sendAndWait(protocol.TypeSubscribeChannel, subMsg, b.config.ResponseTimeout)
		if err != nil {
			b.logger.Printf("Warning: failed to subscribe to %q: %v", name, err)
			// Continue anyway, subscription isn't critical
		}

		b.channelsMu.Lock()
		b.channels[name] = ch.ID
		b.channelsMu.Unlock()

		b.logger.Printf("Joined channel: %s (ID: %d)", name, ch.ID)
	}

	if len(b.channels) == 0 {
		return fmt.Errorf("no channels joined")
	}

	return nil
}

func (b *Bot) pingLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if b.conn.isClosed() {
				return
			}
			pingMsg := &protocol.PingMessage{Timestamp: time.Now().UnixMilli()}
			b.conn.send(protocol.TypePing, pingMsg)
		case <-b.stopCh:
			return
		}
	}
}

func (b *Bot) handleFrame(frame *protocol.Frame) {
	switch frame.Type {
	case protocol.TypeNewMessage:
		b.handleNewMessage(frame)
	case protocol.TypePong:
		// Ignore pong responses
	default:
		// Log unknown broadcast types for debugging
		b.logger.Printf("Received broadcast type 0x%02X", frame.Type)
	}
}

func (b *Bot) handleNewMessage(frame *protocol.Frame) {
	protoMsg := &protocol.NewMessageMessage{}
	if err := protoMsg.Decode(frame.Payload); err != nil {
		b.logger.Printf("Failed to decode NEW_MESSAGE: %v", err)
		return
	}

	// Skip our own messages
	if protoMsg.AuthorNickname == b.nickname {
		return
	}

	msg := &Message{
		ID:             protoMsg.ID,
		ChannelID:      protoMsg.ChannelID,
		ParentID:       protoMsg.ParentID,
		AuthorUserID:   protoMsg.AuthorUserID,
		AuthorNickname: protoMsg.AuthorNickname,
		Content:        protoMsg.Content,
		CreatedAt:      protoMsg.CreatedAt,
		ReplyCount:     protoMsg.ReplyCount,
		botNickname:    b.nickname,
	}

	ctx := &Context{bot: b, message: msg}

	// Check if this is a reply to a thread we participated in
	if msg.ParentID != nil {
		b.myThreadsMu.RLock()
		participated := b.myThreads[*msg.ParentID]
		b.myThreadsMu.RUnlock()

		if participated && b.onThreadReply != nil {
			b.onThreadReply(ctx, msg)
			return
		}
	}

	// Check for mentions
	if msg.MentionsMe() && b.onMention != nil {
		b.onMention(ctx, msg)
		return
	}

	// General message handler
	if b.onMessage != nil {
		b.onMessage(ctx, msg)
	}
}

func (b *Bot) postMessage(channelID uint64, parentID *uint64, content string) error {
	_, err := b.postMessageWithResult(channelID, parentID, content)
	return err
}

func (b *Bot) postMessageWithResult(channelID uint64, parentID *uint64, content string) (*PostMessageResult, error) {
	msg := &protocol.PostMessageMessage{
		ChannelID: channelID,
		ParentID:  parentID,
		Content:   content,
	}

	frame, err := b.conn.sendAndWait(protocol.TypePostMessage, msg, b.config.ResponseTimeout)
	if err != nil {
		return nil, fmt.Errorf("post message: %w", err)
	}
	if err := expectType(frame, protocol.TypeMessagePosted); err != nil {
		return nil, err
	}

	resp := &protocol.MessagePostedMessage{}
	if err := resp.Decode(frame.Payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Track that we participated in this thread
	threadID := resp.MessageID
	if parentID != nil {
		threadID = *parentID
	}
	b.myThreadsMu.Lock()
	b.myThreads[threadID] = true
	b.myThreadsMu.Unlock()

	return &PostMessageResult{
		MessageID: resp.MessageID,
		Timestamp: time.Now(), // MessagePosted doesn't include timestamp
	}, nil
}

func (b *Bot) fetchMessages(channelID uint64, parentID *uint64, limit uint16) ([]Message, error) {
	msg := &protocol.ListMessagesMessage{
		ChannelID: channelID,
		ParentID:  parentID,
		Limit:     limit,
	}

	frame, err := b.conn.sendAndWait(protocol.TypeListMessages, msg, b.config.ResponseTimeout)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	if err := expectType(frame, protocol.TypeMessageList); err != nil {
		return nil, err
	}

	resp := &protocol.MessageListMessage{}
	if err := resp.Decode(frame.Payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	messages := make([]Message, len(resp.Messages))
	for i, m := range resp.Messages {
		messages[i] = Message{
			ID:             m.ID,
			ChannelID:      m.ChannelID,
			ParentID:       m.ParentID,
			AuthorUserID:   m.AuthorUserID,
			AuthorNickname: m.AuthorNickname,
			Content:        m.Content,
			CreatedAt:      m.CreatedAt,
			ReplyCount:     m.ReplyCount,
			botNickname:    b.nickname,
		}
	}

	return messages, nil
}
