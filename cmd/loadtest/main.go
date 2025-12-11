package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aeolun/superchat/pkg/client"
	"github.com/aeolun/superchat/pkg/protocol"
)

const loremIpsum = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum."

var loremWords []string
var usernameWords []string

func init() {
	// Split lorem ipsum into words for random message generation
	loremWords = strings.Fields(loremIpsum)

	// Load words for username generation
	wordsData, err := os.ReadFile("cmd/loadtest/words.txt")
	if err != nil {
		log.Fatalf("Failed to load words.txt: %v", err)
	}
	// Split by newlines and filter out empty lines
	lines := strings.Split(string(wordsData), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			usernameWords = append(usernameWords, line)
		}
	}
	if len(usernameWords) == 0 {
		log.Fatal("words.txt is empty")
	}
}

// generateUsername creates a realistic-looking username by combining fragments of two random words
// getCPULoad returns the 1-minute load average
func getCPULoad() float64 {
	// Read /proc/loadavg on Linux
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}

	// Format: "0.52 0.58 0.59 1/285 12345"
	var load1, load5, load15 float64
	fmt.Sscanf(string(data), "%f %f %f", &load1, &load5, &load15)
	return load1
}

func generateUsername() string {
	// Pick two random words
	word1 := usernameWords[rand.Intn(len(usernameWords))]
	word2 := usernameWords[rand.Intn(len(usernameWords))]

	// Take a random fragment from each word (3-6 characters)
	// Word 1: from start
	len1 := len(word1)
	fragLen1 := 3
	if len1 > 6 {
		fragLen1 = 3 + rand.Intn(4) // 3-6 chars
	} else if len1 > 3 {
		fragLen1 = 3
	} else {
		fragLen1 = len1
	}
	if fragLen1 > len1 {
		fragLen1 = len1
	}

	// Word 2: from start
	len2 := len(word2)
	fragLen2 := 3
	if len2 > 6 {
		fragLen2 = 3 + rand.Intn(4) // 3-6 chars
	} else if len2 > 3 {
		fragLen2 = 3
	} else {
		fragLen2 = len2
	}
	if fragLen2 > len2 {
		fragLen2 = len2
	}

	frag1 := word1[:fragLen1]
	frag2 := word2[:fragLen2]

	// Combine and lowercase
	username := strings.ToLower(frag1 + frag2)

	// Ensure it's valid (3-20 chars, alphanumeric)
	if len(username) < 3 {
		username = username + "user"
	}
	if len(username) > 20 {
		username = username[:20]
	}

	return username
}

// Stats tracks performance metrics
type Stats struct {
	messagesPosted    atomic.Int64
	messagesFailed    atomic.Int64
	totalResponseTime atomic.Int64 // in microseconds
	connectionErrors  atomic.Int64
	successfulClients atomic.Int64 // clients that successfully connected and started running

	// Detailed failure tracking
	postFailures     atomic.Int64
	fetchFailures    atomic.Int64
	timeouts         atomic.Int64
	disconnections   atomic.Int64

	// Setup failure breakdown
	setupListChannelsFailed  atomic.Int64
	setupNoChannelsAvailable atomic.Int64
	setupJoinChannelFailed   atomic.Int64
	setupSubscribeFailed     atomic.Int64

	// Connect phase failure breakdown
	connectNewClientFailed     atomic.Int64
	connectConnectFailed       atomic.Int64
	connectServerConfigTimeout atomic.Int64
	connectNicknameSetFailed   atomic.Int64
	connectNicknameRejected    atomic.Int64
}

func (s *Stats) recordSuccess(responseTimeUs int64) {
	s.messagesPosted.Add(1)
	s.totalResponseTime.Add(responseTimeUs)
}

func (s *Stats) recordFailure() {
	s.messagesFailed.Add(1)
}

func (s *Stats) recordPostFailure() {
	s.messagesFailed.Add(1)
	s.postFailures.Add(1)
}

func (s *Stats) recordFetchFailure() {
	s.fetchFailures.Add(1)
}

func (s *Stats) recordTimeout() {
	s.messagesFailed.Add(1)
	s.timeouts.Add(1)
}

func (s *Stats) recordConnectionError() {
	s.connectionErrors.Add(1)
}

func (s *Stats) recordDisconnection() {
	s.messagesFailed.Add(1)
	s.disconnections.Add(1)
}

func (s *Stats) snapshot() (posted, failed, connErrors int64, avgResponseUs float64) {
	posted = s.messagesPosted.Load()
	failed = s.messagesFailed.Load()
	connErrors = s.connectionErrors.Load()

	if posted > 0 {
		avgResponseUs = float64(s.totalResponseTime.Load()) / float64(posted)
	}

	return
}

// BotClient represents a fake client for load testing
type BotClient struct {
	id                 int
	nickname           string
	conn               *client.LoadTestConnection
	stats              *Stats
	channelID          uint64
	messages           []uint64 // Cache of message IDs we've seen
	messagesMu         sync.Mutex
	currentThreadID    *uint64 // Currently subscribed thread (nil if not subscribed)
	currentThreadIDMu  sync.Mutex
}

func NewBotClient(id int, serverAddr string, stats *Stats) (*BotClient, error) {
	nickname := generateUsername()

	conn := client.NewLoadTestConnection(serverAddr)

	return &BotClient{
		id:       id,
		nickname: nickname,
		conn:     conn,
		stats:    stats,
		messages: make([]uint64, 0, 100),
	}, nil
}

func (bc *BotClient) Connect() error {
	if err := bc.conn.Connect(); err != nil {
		bc.stats.connectConnectFailed.Add(1)
		return fmt.Errorf("conn.Connect: %w", err)
	}

	// Wait for server config
	frame, err := bc.conn.ReceiveMessage(5 * time.Second)
	if err != nil {
		bc.stats.connectServerConfigTimeout.Add(1)
		return fmt.Errorf("receive server config: %w", err)
	}
	if frame.Type == protocol.TypeError {
		errMsg := &protocol.ErrorMessage{}
		errMsg.Decode(frame.Payload)
		bc.stats.connectServerConfigTimeout.Add(1)
		return fmt.Errorf("server config error: %s", errMsg.Message)
	}
	if frame.Type != protocol.TypeServerConfig {
		bc.stats.connectServerConfigTimeout.Add(1)
		return fmt.Errorf("unexpected message type: 0x%02X, expected server config", frame.Type)
	}

	// Decode server config and store version for compression decisions
	serverConfig := &protocol.ServerConfigMessage{}
	if err := serverConfig.Decode(frame.Payload); err == nil {
		bc.conn.SetServerProtocolVersion(serverConfig.ProtocolVersion)
	}

	// Set nickname
	msg := &protocol.SetNicknameMessage{Nickname: bc.nickname}
	if err := bc.conn.SendMessage(protocol.TypeSetNickname, msg); err != nil {
		bc.stats.connectNicknameSetFailed.Add(1)
		return fmt.Errorf("send nickname: %w", err)
	}

	// Wait for nickname response
	frame, err = bc.conn.ReceiveMessage(5 * time.Second)
	if err != nil {
		bc.stats.connectNicknameRejected.Add(1)
		return fmt.Errorf("receive nickname response: %w", err)
	}
	if frame.Type == protocol.TypeError {
		errMsg := &protocol.ErrorMessage{}
		errMsg.Decode(frame.Payload)
		bc.stats.connectNicknameRejected.Add(1)
		return fmt.Errorf("nickname rejected: %s", errMsg.Message)
	}
	if frame.Type != protocol.TypeNicknameResponse {
		bc.stats.connectNicknameRejected.Add(1)
		return fmt.Errorf("unexpected message type: 0x%02X, expected nickname response", frame.Type)
	}

	resp := &protocol.NicknameResponseMessage{}
	if err := resp.Decode(frame.Payload); err != nil {
		bc.stats.connectNicknameRejected.Add(1)
		return fmt.Errorf("failed to decode nickname response: %w", err)
	}
	if !resp.Success {
		bc.stats.connectNicknameRejected.Add(1)
		return fmt.Errorf("nickname rejected: %s", resp.Message)
	}

	return nil
}

func (bc *BotClient) Setup() error {
	// List channels
	if err := bc.conn.SendMessage(protocol.TypeListChannels, &protocol.ListChannelsMessage{}); err != nil {
		bc.stats.setupListChannelsFailed.Add(1)
		return fmt.Errorf("send list channels: %w", err)
	}

	// Wait for channel list
	frame, err := bc.conn.ReceiveMessage(5 * time.Second)
	if err != nil {
		bc.stats.setupListChannelsFailed.Add(1)
		return fmt.Errorf("receive channel list: %w", err)
	}
	if frame.Type == protocol.TypeError {
		errMsg := &protocol.ErrorMessage{}
		errMsg.Decode(frame.Payload)
		bc.stats.setupListChannelsFailed.Add(1)
		return fmt.Errorf("list channels error: %s", errMsg.Message)
	}
	if frame.Type != protocol.TypeChannelList {
		bc.stats.setupListChannelsFailed.Add(1)
		return fmt.Errorf("unexpected message type: 0x%02X, expected channel list", frame.Type)
	}

	resp := &protocol.ChannelListMessage{}
	if err := resp.Decode(frame.Payload); err != nil {
		bc.stats.setupListChannelsFailed.Add(1)
		return fmt.Errorf("decode channel list: %w", err)
	}

	if len(resp.Channels) == 0 {
		bc.stats.setupNoChannelsAvailable.Add(1)
		return fmt.Errorf("no channels available")
	}

	// Pick random channel
	channel := resp.Channels[rand.Intn(len(resp.Channels))]
	bc.channelID = channel.ID

	// Join channel
	joinMsg := &protocol.JoinChannelMessage{ChannelID: channel.ID}
	if err := bc.conn.SendMessage(protocol.TypeJoinChannel, joinMsg); err != nil {
		bc.stats.setupJoinChannelFailed.Add(1)
		return fmt.Errorf("send join channel: %w", err)
	}

	// Wait for join response
	frame, err = bc.conn.ReceiveMessage(5 * time.Second)
	if err != nil {
		bc.stats.setupJoinChannelFailed.Add(1)
		return fmt.Errorf("receive join response: %w", err)
	}
	if frame.Type == protocol.TypeError {
		errMsg := &protocol.ErrorMessage{}
		errMsg.Decode(frame.Payload)
		bc.stats.setupJoinChannelFailed.Add(1)
		return fmt.Errorf("join channel error: %s", errMsg.Message)
	}
	if frame.Type != protocol.TypeJoinResponse {
		bc.stats.setupJoinChannelFailed.Add(1)
		return fmt.Errorf("unexpected message type: 0x%02X, expected join response", frame.Type)
	}

	// Subscribe to channel for new thread notifications
	subscribeMsg := &protocol.SubscribeChannelMessage{
		ChannelID:    channel.ID,
		SubchannelID: nil,
	}
	if err := bc.conn.SendMessage(protocol.TypeSubscribeChannel, subscribeMsg); err != nil {
		bc.stats.setupSubscribeFailed.Add(1)
		return fmt.Errorf("send subscribe channel: %w", err)
	}

	// Wait for subscribe confirmation (not critical if it fails)
	frame, err = bc.conn.ReceiveMessage(5 * time.Second)
	if err != nil {
		bc.stats.setupSubscribeFailed.Add(1)
		// Subscription timeout, but not critical - continue anyway
	} else if frame.Type == protocol.TypeError {
		bc.stats.setupSubscribeFailed.Add(1)
		// Subscription error, but not critical - continue anyway
	} else if frame.Type != protocol.TypeSubscribeOk {
		bc.stats.setupSubscribeFailed.Add(1)
		// Unexpected response, but not critical - continue anyway
	}

	return nil
}

func (bc *BotClient) PostRandomMessage() error {
	// Decide: new thread (10%) or reply (90%)
	createNewThread := rand.Float32() < 0.1

	var parentID *uint64
	if !createNewThread && len(bc.messages) > 0 {
		// Pick random message to reply to
		// NOTE: All cached messages are thread roots (from FetchMessages)
		bc.messagesMu.Lock()
		randomMsg := bc.messages[rand.Intn(len(bc.messages))]
		bc.messagesMu.Unlock()
		parentID = &randomMsg
	}

	// Generate random message content (5-20 words)
	wordCount := 5 + rand.Intn(16)
	var words []string
	for i := 0; i < wordCount; i++ {
		words = append(words, loremWords[rand.Intn(len(loremWords))])
	}
	content := strings.Join(words, " ")

	// Post message
	start := time.Now()
	postMsg := &protocol.PostMessageMessage{
		ChannelID: bc.channelID,
		ParentID:  parentID,
		Content:   content,
	}

	if err := bc.conn.SendMessage(protocol.TypePostMessage, postMsg); err != nil {
		// Check if it's a connection error (broken pipe, connection reset, etc)
		if strings.Contains(err.Error(), "broken pipe") ||
		   strings.Contains(err.Error(), "connection reset") ||
		   strings.Contains(err.Error(), "EOF") {
			bc.stats.recordDisconnection()
		} else {
			bc.stats.recordFailure()
		}
		return err
	}

	// Wait for response
	frame, err := bc.conn.ReceiveMessage(10 * time.Second)
	if err != nil {
		bc.stats.recordTimeout()
		return fmt.Errorf("receive post response: %w", err)
	}
	if frame.Type == protocol.TypeError {
		errMsg := &protocol.ErrorMessage{}
		if decodeErr := errMsg.Decode(frame.Payload); decodeErr == nil {
			log.Printf("[Bot %d] POST failed with error %d: %s", bc.id, errMsg.ErrorCode, errMsg.Message)
		}
		bc.stats.recordPostFailure()
		return fmt.Errorf("post message failed")
	}
	if frame.Type != protocol.TypeMessagePosted {
		bc.stats.recordPostFailure()
		return fmt.Errorf("unexpected message type: 0x%02X, expected message posted", frame.Type)
	}

	responseTime := time.Since(start).Microseconds()
	resp := &protocol.MessagePostedMessage{}
	if err := resp.Decode(frame.Payload); err == nil {
		// Cache the message ID
		bc.messagesMu.Lock()
		bc.messages = append(bc.messages, resp.MessageID)
		bc.messagesMu.Unlock()
	}
	bc.stats.recordSuccess(responseTime)
	return nil
}

func (bc *BotClient) FetchMessages() error {
	// Periodically fetch messages to update our cache
	listMsg := &protocol.ListMessagesMessage{
		ChannelID: bc.channelID,
		ParentID:  nil, // Fetch root messages only (thread starters)
		Limit:     50,
	}

	if err := bc.conn.SendMessage(protocol.TypeListMessages, listMsg); err != nil {
		return err
	}

	// Wait for response
	frame, err := bc.conn.ReceiveMessage(5 * time.Second)
	if err != nil {
		bc.stats.recordFetchFailure()
		return fmt.Errorf("receive message list: %w", err)
	}
	if frame.Type == protocol.TypeError {
		bc.stats.recordFetchFailure()
		return fmt.Errorf("failed to fetch messages")
	}
	if frame.Type != protocol.TypeMessageList {
		bc.stats.recordFetchFailure()
		return fmt.Errorf("unexpected message type: 0x%02X, expected message list", frame.Type)
	}

	resp := &protocol.MessageListMessage{}
	if err := resp.Decode(frame.Payload); err != nil {
		bc.stats.recordFetchFailure()
		return fmt.Errorf("decode message list: %w", err)
	}

	bc.messagesMu.Lock()
	// Update cache with root messages
	bc.messages = bc.messages[:0] // Clear old messages
	for _, msg := range resp.Messages {
		bc.messages = append(bc.messages, msg.ID)
	}

	// Pick a random thread to subscribe to
	var newThreadID *uint64
	if len(bc.messages) > 0 {
		randomThread := bc.messages[rand.Intn(len(bc.messages))]
		newThreadID = &randomThread
	}
	bc.messagesMu.Unlock()

	// Handle thread subscription switching
	bc.currentThreadIDMu.Lock()
	oldThreadID := bc.currentThreadID
	bc.currentThreadID = newThreadID
	bc.currentThreadIDMu.Unlock()

	// Unsubscribe from old thread
	if oldThreadID != nil {
		unsubMsg := &protocol.UnsubscribeThreadMessage{ThreadID: *oldThreadID}
		bc.conn.SendMessage(protocol.TypeUnsubscribeThread, unsubMsg)
	}

	// Subscribe to new thread
	if newThreadID != nil {
		subMsg := &protocol.SubscribeThreadMessage{ThreadID: *newThreadID}
		bc.conn.SendMessage(protocol.TypeSubscribeThread, subMsg)
	}

	return nil
}

func (bc *BotClient) Run(duration time.Duration, minDelay, maxDelay time.Duration, shutdownDelay time.Duration, disconnectTimes chan<- time.Time) {
	defer func() {
		// Send graceful disconnect before closing
		bc.conn.SendMessage(protocol.TypeDisconnect, &protocol.DisconnectMessage{})
		time.Sleep(100 * time.Millisecond)
		bc.conn.Close()

		// Record disconnect time
		select {
		case disconnectTimes <- time.Now():
		default:
		}
	}()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Bot %d] PANIC: %v", bc.id, r)
		}
	}()

	// Initial message fetch
	if err := bc.FetchMessages(); err != nil {
		// Silently ignore initial fetch failures
	}

	endTime := time.Now().Add(duration)
	iteration := 0

	for time.Now().Before(endTime) {
		iteration++

		// Post a message
		if err := bc.PostRandomMessage(); err != nil {
			// Only log critical errors
		}

		// Refresh message list every 3 iterations to discover new threads
		if iteration%3 == 0 {
			if err := bc.FetchMessages(); err != nil {
				// Silently ignore fetch failures
			}
		}

		// Random delay between posts
		delay := minDelay + time.Duration(rand.Int63n(int64(maxDelay-minDelay)))
		time.Sleep(delay)
	}

	// Stagger shutdown to avoid thundering herd on disconnect
	if shutdownDelay > 0 {
		time.Sleep(shutdownDelay)
	}

	// Send graceful disconnect message
	bc.conn.SendMessage(protocol.TypeDisconnect, &protocol.DisconnectMessage{})

	// Give server time to process disconnect before closing connection
	time.Sleep(100 * time.Millisecond)
}

var debugLogger *log.Logger

func initLogging() error {
	// Create loadtest.log file (truncate on each run to avoid confusion)
	logFile, err := os.OpenFile("loadtest.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("failed to create loadtest.log: %w", err)
	}

	// Create loadtest_debug.log file for detailed bot communication logs
	debugLogFile, err := os.OpenFile("loadtest_debug.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("failed to create loadtest_debug.log: %w", err)
	}

	// Configure standard log to write to both stdout and file
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.LstdFlags)

	// Configure debug logger to write only to debug file
	debugLogger = log.New(debugLogFile, "", log.LstdFlags|log.Lmicroseconds)

	return nil
}

func main() {
	// Command-line flags
	serverAddr := flag.String("server", "localhost:6465", "Server address (host:port)")
	numClients := flag.Int("clients", 10, "Number of concurrent clients")
	duration := flag.Duration("duration", 1*time.Minute, "Test duration")
	minDelay := flag.Duration("min-delay", 100*time.Millisecond, "Minimum delay between posts")
	maxDelay := flag.Duration("max-delay", 1*time.Second, "Maximum delay between posts")
	flag.Parse()

	// Initialize logging to both stdout and file
	if err := initLogging(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	log.Printf("Load test logs will be written to loadtest.log")
	log.Printf("Detailed bot communication logs in loadtest_debug.log")

	// Calculate stagger delay: ramp up over 25% of test duration
	rampUpDuration := *duration / 4
	staggerDelay := rampUpDuration / time.Duration(*numClients)
	if staggerDelay < 1*time.Millisecond {
		staggerDelay = 1 * time.Millisecond
	}

	log.Printf("Starting load test:")
	log.Printf("  Server: %s", *serverAddr)
	log.Printf("  Clients: %d", *numClients)
	log.Printf("  Duration: %v", *duration)
	log.Printf("  Ramp-up: %v (%v per client)", rampUpDuration, staggerDelay)
	log.Printf("  Delay: %v - %v", *minDelay, *maxDelay)
	log.Printf("")

	stats := &Stats{}
	var wg sync.WaitGroup

	// Start stats reporter
	stopStats := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		startTime := time.Now()
		for {
			select {
			case <-ticker.C:
				posted, failed, connErrors, avgUs := stats.snapshot()
				elapsed := time.Since(startTime).Seconds()
				rate := float64(posted) / elapsed
				avgMs := avgUs / 1000.0
				load := getCPULoad()
				goroutines := runtime.NumGoroutine()

				log.Printf("Stats: %d posted (%.1f/s), %d failed, %d conn errors, avg %.2fms, load %.2f, goroutines %d",
					posted, rate, failed, connErrors, avgMs, load, goroutines)
			case <-stopStats:
				return
			}
		}
	}()

	// Track ramp-up and ramp-down timing
	rampUpStart := time.Now()
	var firstConnectTime, lastConnectTime atomic.Value
	var firstDisconnectTime, lastDisconnectTime atomic.Value
	connectTimes := make(chan time.Time, *numClients)
	disconnectTimes := make(chan time.Time, *numClients)

	// Spawn clients
	for i := 0; i < *numClients; i++ {
		wg.Add(1)

		// Calculate shutdown delay for this bot (reverse order for ramp-down)
		shutdownDelay := staggerDelay * time.Duration(*numClients-i-1)

		go func(id int, shutdownDelay time.Duration) {
			defer wg.Done()

			bot, err := NewBotClient(id, *serverAddr, stats)
			if err != nil {
				stats.recordConnectionError()
				stats.connectNewClientFailed.Add(1)
				return
			}

			if err := bot.Connect(); err != nil {
				stats.recordConnectionError()
				// Clean up connection if connection fails
				bot.conn.Close()
				return
			}

			if err := bot.Setup(); err != nil {
				stats.recordConnectionError()
				// Clean up connection if setup fails
				bot.conn.SendMessage(protocol.TypeDisconnect, &protocol.DisconnectMessage{})
				time.Sleep(100 * time.Millisecond)
				bot.conn.Close()
				return
			}

			// Record successful client connection
			stats.successfulClients.Add(1)

			// Record connection time
			connectTime := time.Now()
			select {
			case connectTimes <- connectTime:
			default:
			}

			// Only log every 100th client during ramp-up
			if id%100 == 0 {
				log.Printf("[Bot %d] Connected", id)
			}

			bot.Run(*duration, *minDelay, *maxDelay, shutdownDelay, disconnectTimes)
		}(i, shutdownDelay)

		// Stagger client connections based on calculated delay
		time.Sleep(staggerDelay)
	}

	// Track connection and disconnection times in background
	go func() {
		for t := range connectTimes {
			if firstConnectTime.Load() == nil {
				firstConnectTime.Store(t)
			}
			lastConnectTime.Store(t)
		}
	}()

	go func() {
		for t := range disconnectTimes {
			if firstDisconnectTime.Load() == nil {
				firstDisconnectTime.Store(t)
			}
			lastDisconnectTime.Store(t)
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Printf("\nShutdown signal received, stopping test...")
		close(stopStats)
	}()

	// Wait for all clients to finish
	wg.Wait()
	close(stopStats)
	close(connectTimes)
	close(disconnectTimes)

	// Check ramp-up timing
	if lastConnectTime.Load() != nil && firstConnectTime.Load() != nil {
		first := firstConnectTime.Load().(time.Time)
		last := lastConnectTime.Load().(time.Time)
		actualRampUp := last.Sub(first)
		expectedRampUp := rampUpDuration

		tolerance := 1 * time.Second
		withinTolerance := actualRampUp >= expectedRampUp-tolerance && actualRampUp <= expectedRampUp+tolerance

		status := "✓"
		if !withinTolerance {
			status = "✗"
		}

		log.Printf("\n%s Ramp-up timing: expected %v, took %v (first: %v, last: %v)",
			status, expectedRampUp.Round(time.Second), actualRampUp.Round(time.Second),
			first.Sub(rampUpStart).Round(time.Millisecond), last.Sub(rampUpStart).Round(time.Millisecond))
	}

	// Check ramp-down timing
	if lastDisconnectTime.Load() != nil && firstDisconnectTime.Load() != nil {
		first := firstDisconnectTime.Load().(time.Time)
		last := lastDisconnectTime.Load().(time.Time)
		actualRampDown := last.Sub(first)
		expectedRampDown := rampUpDuration // Same as ramp-up

		tolerance := 1 * time.Second
		withinTolerance := actualRampDown >= expectedRampDown-tolerance && actualRampDown <= expectedRampDown+tolerance

		status := "✓"
		if !withinTolerance {
			status = "✗"
		}

		log.Printf("%s Ramp-down timing: expected %v, took %v (first: %v after start, last: %v after start)",
			status, expectedRampDown.Round(time.Second), actualRampDown.Round(time.Second),
			first.Sub(rampUpStart).Round(time.Second), last.Sub(rampUpStart).Round(time.Second))
	}

	// Total test duration (from first connect to last disconnect)
	if firstConnectTime.Load() != nil && lastDisconnectTime.Load() != nil {
		first := firstConnectTime.Load().(time.Time)
		last := lastDisconnectTime.Load().(time.Time)
		totalTestDuration := last.Sub(first)
		expectedTotal := *duration + rampUpDuration // ramp-up + test duration, ramp-down overlaps
		log.Printf("Total test duration: %v (expected: ~%v)\n",
			totalTestDuration.Round(time.Second),
			expectedTotal.Round(time.Second))
	}

	// Final stats
	posted, failed, connErrors, avgUs := stats.snapshot()
	successfulClients := stats.successfulClients.Load()
	totalDuration := *duration
	rate := float64(posted) / totalDuration.Seconds()
	avgMs := avgUs / 1000.0

	// Calculate expected throughput based on successful clients
	avgDelay := (*minDelay + *maxDelay) / 2
	expectedPerClient := float64(totalDuration) / float64(avgDelay)
	expectedTotal := expectedPerClient * float64(successfulClients)
	efficiency := 0.0
	if expectedTotal > 0 {
		efficiency = float64(posted) / expectedTotal * 100
	}

	// Detailed failure breakdown
	postFails := stats.postFailures.Load()
	fetchFails := stats.fetchFailures.Load()
	timeouts := stats.timeouts.Load()
	disconnects := stats.disconnections.Load()

	// Setup failure breakdown
	setupListChannelsFails := stats.setupListChannelsFailed.Load()
	setupNoChannels := stats.setupNoChannelsAvailable.Load()
	setupJoinFails := stats.setupJoinChannelFailed.Load()
	setupSubscribeFails := stats.setupSubscribeFailed.Load()

	// Connect phase failure breakdown
	connectNewClientFails := stats.connectNewClientFailed.Load()
	connectConnectFails := stats.connectConnectFailed.Load()
	connectServerConfigTimeouts := stats.connectServerConfigTimeout.Load()
	connectNicknameSetFails := stats.connectNicknameSetFailed.Load()
	connectNicknameRejects := stats.connectNicknameRejected.Load()

	log.Printf("\n=== Final Results ===")
	log.Printf("Clients: %d attempted, %d successful (%.1f%%)", *numClients, successfulClients, float64(successfulClients)/float64(*numClients)*100)
	log.Printf("Duration: %v", totalDuration)
	log.Printf("Messages posted: %d (%.1f/s)", posted, rate)
	log.Printf("Messages failed: %d", failed)
	log.Printf("  - Post failures: %d", postFails)
	log.Printf("  - Fetch failures: %d", fetchFails)
	log.Printf("  - Timeouts: %d", timeouts)
	log.Printf("  - Disconnections: %d", disconnects)
	log.Printf("Connection errors: %d", connErrors)
	if connErrors > 0 {
		log.Printf("  Connect phase breakdown:")
		log.Printf("    - NewBotClient failed: %d", connectNewClientFails)
		log.Printf("    - conn.Connect failed: %d", connectConnectFails)
		log.Printf("    - Server config timeout: %d", connectServerConfigTimeouts)
		log.Printf("    - Nickname set failed: %d", connectNicknameSetFails)
		log.Printf("    - Nickname rejected: %d", connectNicknameRejects)
		log.Printf("  Setup phase breakdown:")
		log.Printf("    - List channels failed: %d", setupListChannelsFails)
		log.Printf("    - No channels available: %d", setupNoChannels)
		log.Printf("    - Join channel failed: %d", setupJoinFails)
		log.Printf("    - Subscribe failed: %d", setupSubscribeFails)
	}
	log.Printf("Average response time: %.2fms", avgMs)
	log.Printf("Expected throughput: %.0f messages (%.1f per client)", expectedTotal, expectedPerClient)
	log.Printf("Actual vs expected: %.1f%% efficiency", efficiency)

	if posted > 0 {
		successRate := float64(posted) / float64(posted+failed) * 100
		log.Printf("Success rate: %.1f%%", successRate)
	}
}
