package server

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/aeolun/superchat/pkg/database"
	"github.com/aeolun/superchat/pkg/protocol"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

// ---------------------------------------------------------------------------
// Transport abstraction
// ---------------------------------------------------------------------------

// transportClient provides a uniform interface for sending/receiving protocol
// messages over TCP, SSH, or WebSocket connections.
type transportClient interface {
	// send encodes and sends a protocol message.
	send(t *testing.T, msgType uint8, msg interface{ EncodeTo(io.Writer) error })
	// expect reads the next protocol frame, skipping presence broadcasts,
	// and asserts that its type matches expectedType.
	expect(t *testing.T, expectedType uint8, timeout time.Duration) *protocol.Frame
	// tryRead attempts to read one frame within timeout. Returns nil if
	// nothing arrived (no fatal on timeout).
	tryRead(t *testing.T, timeout time.Duration) *protocol.Frame
	// close tears down the connection.
	close()
}

// ignoredBroadcast returns true for message types that may arrive
// asynchronously and should be skipped when waiting for a specific response.
func ignoredBroadcast(msgType uint8) bool {
	switch msgType {
	case protocol.TypeServerPresence, protocol.TypeChannelPresence:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// TCP transport
// ---------------------------------------------------------------------------

type tcpClient struct {
	conn      net.Conn
	closeOnce sync.Once
}

func newTCPClient(t *testing.T, addr string) *tcpClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("TCP connect to %s failed: %v", addr, err)
	}
	return &tcpClient{conn: conn}
}

func (c *tcpClient) send(t *testing.T, msgType uint8, msg interface{ EncodeTo(io.Writer) error }) {
	t.Helper()
	var buf bytes.Buffer
	if err := msg.EncodeTo(&buf); err != nil {
		t.Fatalf("TCP encode 0x%02X: %v", msgType, err)
	}
	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Flags:   0,
		Payload: buf.Bytes(),
	}
	if err := protocol.EncodeFrame(c.conn, frame); err != nil {
		t.Fatalf("TCP send 0x%02X: %v", msgType, err)
	}
}

func (c *tcpClient) expect(t *testing.T, expectedType uint8, timeout time.Duration) *protocol.Frame {
	t.Helper()
	for {
		c.conn.SetReadDeadline(time.Now().Add(timeout))
		frame, err := protocol.DecodeFrame(c.conn)
		c.conn.SetReadDeadline(time.Time{})
		if err != nil {
			t.Fatalf("TCP expect 0x%02X: read error: %v", expectedType, err)
		}
		if ignoredBroadcast(frame.Type) {
			continue
		}
		if frame.Type != expectedType {
			t.Fatalf("TCP expected 0x%02X, got 0x%02X", expectedType, frame.Type)
		}
		return frame
	}
}

func (c *tcpClient) tryRead(t *testing.T, timeout time.Duration) *protocol.Frame {
	t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	frame, err := protocol.DecodeFrame(c.conn)
	c.conn.SetReadDeadline(time.Time{})
	if err != nil {
		return nil
	}
	return frame
}

func (c *tcpClient) close() {
	c.closeOnce.Do(func() {
		c.conn.Close()
	})
}

// ---------------------------------------------------------------------------
// SSH transport
//
// SSH channels don't support deadlines, so we use a persistent reader
// goroutine that feeds decoded frames into a buffered channel. This avoids
// the data-race that would occur if multiple goroutines tried to read from
// the same ssh.Channel concurrently.
// ---------------------------------------------------------------------------

type sshClient struct {
	client   *ssh.Client
	channel  ssh.Channel
	frames   chan *protocol.Frame
	errors   chan error
	done     chan struct{}
	closeOnce sync.Once
}

func newSSHClient(t *testing.T, addr string) *sshClient {
	t.Helper()

	// Generate a fresh RSA key for this client
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("SSH keygen: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("SSH signer: %v", err)
	}

	config := &ssh.ClientConfig{
		User:            "testuser",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		t.Fatalf("SSH dial %s: %v", addr, err)
	}
	channel, requests, err := client.OpenChannel("session", nil)
	if err != nil {
		client.Close()
		t.Fatalf("SSH open channel: %v", err)
	}
	go ssh.DiscardRequests(requests)

	sc := &sshClient{
		client:  client,
		channel: channel,
		frames:  make(chan *protocol.Frame, 64),
		errors:  make(chan error, 1),
		done:    make(chan struct{}),
	}

	// Single persistent reader goroutine
	go func() {
		defer close(sc.done)
		for {
			frame, err := protocol.DecodeFrame(channel)
			if err != nil {
				sc.errors <- err
				return
			}
			sc.frames <- frame
		}
	}()

	return sc
}

func (c *sshClient) send(t *testing.T, msgType uint8, msg interface{ EncodeTo(io.Writer) error }) {
	t.Helper()
	var buf bytes.Buffer
	if err := msg.EncodeTo(&buf); err != nil {
		t.Fatalf("SSH encode 0x%02X: %v", msgType, err)
	}
	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Flags:   0,
		Payload: buf.Bytes(),
	}
	if err := protocol.EncodeFrame(c.channel, frame); err != nil {
		t.Fatalf("SSH send 0x%02X: %v", msgType, err)
	}
}

func (c *sshClient) expect(t *testing.T, expectedType uint8, timeout time.Duration) *protocol.Frame {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case frame := <-c.frames:
			if ignoredBroadcast(frame.Type) {
				continue
			}
			if frame.Type != expectedType {
				t.Fatalf("SSH expected 0x%02X, got 0x%02X", expectedType, frame.Type)
			}
			return frame
		case err := <-c.errors:
			t.Fatalf("SSH expect 0x%02X: read error: %v", expectedType, err)
			return nil
		case <-deadline:
			t.Fatalf("SSH expect 0x%02X: timeout after %v", expectedType, timeout)
			return nil
		}
	}
}

func (c *sshClient) tryRead(t *testing.T, timeout time.Duration) *protocol.Frame {
	t.Helper()
	select {
	case frame := <-c.frames:
		return frame
	case <-c.errors:
		return nil
	case <-time.After(timeout):
		return nil
	}
}

func (c *sshClient) close() {
	c.closeOnce.Do(func() {
		c.channel.Close()
		c.client.Close()
		// Wait for reader goroutine to exit (channel close unblocks DecodeFrame)
		<-c.done
	})
}

// ---------------------------------------------------------------------------
// WebSocket transport
//
// The server's WebSocketConn.Write is called multiple times per frame by
// protocol.EncodeFrame (length prefix, then headers+payload). Each Write
// becomes a separate WebSocket binary message. We use a persistent reader
// goroutine that accumulates WS messages into a buffer and decodes protocol
// frames, feeding them into a channel. This avoids gorilla/websocket's
// limitation where a read deadline timeout corrupts the connection state.
// ---------------------------------------------------------------------------

type wsClient struct {
	conn      *websocket.Conn
	frames    chan *protocol.Frame
	errors    chan error
	done      chan struct{}
	closeOnce sync.Once
}

func newWSClient(t *testing.T, addr string) *wsClient {
	t.Helper()
	url := fmt.Sprintf("ws://%s/ws", addr)
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("WebSocket dial %s: %v", url, err)
	}

	wc := &wsClient{
		conn:   conn,
		frames: make(chan *protocol.Frame, 64),
		errors: make(chan error, 1),
		done:   make(chan struct{}),
	}

	// Persistent reader goroutine: reads WS messages, accumulates into
	// buffer, decodes protocol frames, sends to channel.
	go func() {
		defer close(wc.done)
		var readBuf bytes.Buffer
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				wc.errors <- err
				return
			}
			readBuf.Write(data)

			// Try to decode as many complete frames as possible
			for readBuf.Len() > 0 {
				snapshot := make([]byte, readBuf.Len())
				copy(snapshot, readBuf.Bytes())
				reader := bytes.NewReader(snapshot)
				frame, err := protocol.DecodeFrame(reader)
				if err != nil {
					// Not enough data for a complete frame yet
					break
				}
				consumed := len(snapshot) - reader.Len()
				readBuf.Next(consumed)
				wc.frames <- frame
			}
		}
	}()

	return wc
}

func (c *wsClient) send(t *testing.T, msgType uint8, msg interface{ EncodeTo(io.Writer) error }) {
	t.Helper()
	// Encode the protocol frame into a buffer, then send as a single WS binary message
	var payload bytes.Buffer
	if err := msg.EncodeTo(&payload); err != nil {
		t.Fatalf("WS encode 0x%02X: %v", msgType, err)
	}
	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Flags:   0,
		Payload: payload.Bytes(),
	}
	var frameBuf bytes.Buffer
	if err := protocol.EncodeFrame(&frameBuf, frame); err != nil {
		t.Fatalf("WS frame encode 0x%02X: %v", msgType, err)
	}
	if err := c.conn.WriteMessage(websocket.BinaryMessage, frameBuf.Bytes()); err != nil {
		t.Fatalf("WS send 0x%02X: %v", msgType, err)
	}
}

func (c *wsClient) expect(t *testing.T, expectedType uint8, timeout time.Duration) *protocol.Frame {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case frame := <-c.frames:
			if ignoredBroadcast(frame.Type) {
				continue
			}
			if frame.Type != expectedType {
				t.Fatalf("WS expected 0x%02X, got 0x%02X", expectedType, frame.Type)
			}
			return frame
		case err := <-c.errors:
			t.Fatalf("WS expect 0x%02X: read error: %v", expectedType, err)
			return nil
		case <-deadline:
			t.Fatalf("WS expect 0x%02X: timeout after %v", expectedType, timeout)
			return nil
		}
	}
}

func (c *wsClient) tryRead(t *testing.T, timeout time.Duration) *protocol.Frame {
	t.Helper()
	select {
	case frame := <-c.frames:
		return frame
	case <-c.errors:
		return nil
	case <-time.After(timeout):
		return nil
	}
}

func (c *wsClient) close() {
	c.closeOnce.Do(func() {
		c.conn.Close()
		<-c.done
	})
}

// ---------------------------------------------------------------------------
// Server setup for journey tests
// ---------------------------------------------------------------------------

type journeyServers struct {
	srv     *Server
	tcpAddr string
	sshAddr string
	wsAddr  string
}

// setupJourneyServer creates a single server with TCP, SSH (NoClientAuth),
// and WebSocket listeners on random ports. Returns addresses for each.
// Constructs the server manually (like testServer) to avoid Prometheus
// registration conflicts and logger races with other tests.
func setupJourneyServer(t *testing.T) *journeyServers {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := tmpDir + "/journey.db"

	// Open database and seed default channels
	sqliteDB, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	if err := sqliteDB.SeedDefaultChannels(); err != nil {
		sqliteDB.Close()
		t.Fatalf("SeedDefaultChannels: %v", err)
	}

	// Create MemDB with short snapshot interval for tests
	memDB, err := database.NewMemDB(sqliteDB, 100*time.Millisecond)
	if err != nil {
		sqliteDB.Close()
		t.Fatalf("NewMemDB: %v", err)
	}

	config := DefaultConfig()
	config.HTTPPort = 0
	config.SSHPort = 0
	config.TCPPort = 0
	config.SessionTimeoutSeconds = 60
	config.DirectoryEnabled = false

	sessions := NewSessionManager(memDB, config.SessionTimeoutSeconds)

	srv := &Server{
		db:                     memDB,
		sessions:               sessions,
		config:                 config,
		shutdown:               make(chan struct{}),
		metrics:                nil, // Skip metrics to avoid Prometheus registration conflicts
		startTime:              time.Now(),
		verificationChallenges: make(map[uint64]uint64),
		discoveryRateLimits:    make(map[string]*discoveryRateLimiter),
		autoRegisterAttempts:   make(map[string][]time.Time),
	}

	// Start server (TCP only — SSH and HTTP disabled)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	tcpAddr := srv.listener.Addr().String()

	// --- Manually start SSH with NoClientAuth for testing ---
	srv.config.SSHHostKeyPath = tmpDir + "/ssh_host_key"
	hostKey, err := srv.loadOrGenerateHostKey()
	if err != nil {
		t.Fatalf("SSH host key: %v", err)
	}
	sshConfig := &ssh.ServerConfig{NoClientAuth: true}
	sshConfig.ServerVersion = "SSH-2.0-SuperChat"
	sshConfig.AddHostKey(hostKey)

	sshListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("SSH listen: %v", err)
	}
	srv.sshListener = sshListener
	sshAddr := sshListener.Addr().String()

	srv.wg.Add(1)
	go srv.acceptSSHLoop(sshListener, sshConfig)

	// --- Manually start WebSocket HTTP server ---
	wsMux := http.NewServeMux()
	wsMux.HandleFunc("/ws", srv.HandleWebSocket)
	wsListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("WS listen: %v", err)
	}
	wsAddr := wsListener.Addr().String()
	wsServer := &http.Server{Handler: wsMux}
	go wsServer.Serve(wsListener)

	t.Cleanup(func() {
		wsServer.Close()
		srv.Stop()
	})

	return &journeyServers{
		srv:     srv,
		tcpAddr: tcpAddr,
		sshAddr: sshAddr,
		wsAddr:  wsAddr,
	}
}

// ---------------------------------------------------------------------------
// Transport factories
// ---------------------------------------------------------------------------

type transportFactory struct {
	name    string
	connect func(t *testing.T, servers *journeyServers) transportClient
}

func allTransports() []transportFactory {
	return []transportFactory{
		{"tcp", func(t *testing.T, s *journeyServers) transportClient { return newTCPClient(t, s.tcpAddr) }},
		{"ssh", func(t *testing.T, s *journeyServers) transportClient { return newSSHClient(t, s.sshAddr) }},
		{"websocket", func(t *testing.T, s *journeyServers) transportClient { return newWSClient(t, s.wsAddr) }},
	}
}

// ---------------------------------------------------------------------------
// Drain helper — read and discard frames for a short window
// ---------------------------------------------------------------------------

func drain(t *testing.T, c transportClient, duration time.Duration) {
	t.Helper()
	for {
		f := c.tryRead(t, duration)
		if f == nil {
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Main test entry point (single TestJourney avoids Prometheus conflicts)
// ---------------------------------------------------------------------------

func TestJourney(t *testing.T) {
	servers := setupJourneyServer(t)

	for _, tf := range allTransports() {
		t.Run("full_user_journey/"+tf.name, func(t *testing.T) {
			runFullUserJourney(t, servers, tf)
		})
	}

	for _, tf := range allTransports() {
		t.Run("multi_client_broadcast/"+tf.name, func(t *testing.T) {
			runMultiClientBroadcast(t, servers, tf)
		})
	}

	t.Run("cross_transport_broadcast", func(t *testing.T) {
		runCrossTransportBroadcast(t, servers)
	})

	for _, tf := range allTransports() {
		t.Run("dm_unencrypted/"+tf.name, func(t *testing.T) {
			runDMUnencrypted(t, servers, tf)
		})
	}

	for _, tf := range allTransports() {
		t.Run("dm_encrypted/"+tf.name, func(t *testing.T) {
			runDMEncrypted(t, servers, tf)
		})
	}

	for _, tf := range allTransports() {
		t.Run("dm_key_provisioning/"+tf.name, func(t *testing.T) {
			runDMKeyProvisioning(t, servers, tf)
		})
	}

	for _, tf := range allTransports() {
		t.Run("dm_decline/"+tf.name, func(t *testing.T) {
			runDMDecline(t, servers, tf)
		})
	}

	for _, tf := range allTransports() {
		t.Run("dm_no_key_required/"+tf.name, func(t *testing.T) {
			runDMNoKeyRequired(t, servers, tf)
		})
	}

	for _, tf := range allTransports() {
		t.Run("dm_existing_channel/"+tf.name, func(t *testing.T) {
			runDMExistingChannel(t, servers, tf)
		})
	}
}

// ---------------------------------------------------------------------------
// Full user journey (18 steps)
// ---------------------------------------------------------------------------

func runFullUserJourney(t *testing.T, servers *journeyServers, tf transportFactory) {
	timeout := 5 * time.Second
	nickname := fmt.Sprintf("journey_%s", tf.name)
	password := "TestPassword123!"
	clientHash := hashPasswordForTest(password, nickname)
	channelName := fmt.Sprintf("journey_%s", tf.name)

	// Track all clients for cleanup (reconnect creates a second client)
	var clients []transportClient
	cleanup := func() {
		for _, c := range clients {
			c.close()
		}
	}
	defer cleanup()

	connect := func() transportClient {
		c := tf.connect(t, servers)
		clients = append(clients, c)
		return c
	}

	// Step 1: Connect — receive SERVER_CONFIG
	client := connect()

	configFrame := client.expect(t, protocol.TypeServerConfig, timeout)
	var serverConfig protocol.ServerConfigMessage
	if err := serverConfig.Decode(configFrame.Payload); err != nil {
		t.Fatalf("Decode SERVER_CONFIG: %v", err)
	}
	if serverConfig.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("Protocol version: got %d, want %d", serverConfig.ProtocolVersion, protocol.ProtocolVersion)
	}

	// Step 2: Set nickname
	client.send(t, protocol.TypeSetNickname, &protocol.SetNicknameMessage{Nickname: nickname})
	nickFrame := client.expect(t, protocol.TypeNicknameResponse, timeout)
	var nickResp protocol.NicknameResponseMessage
	if err := nickResp.Decode(nickFrame.Payload); err != nil {
		t.Fatalf("Decode NICKNAME_RESPONSE: %v", err)
	}
	if !nickResp.Success {
		t.Fatalf("Set nickname failed: %s", nickResp.Message)
	}

	// Step 3: Register
	client.send(t, protocol.TypeRegisterUser, &protocol.RegisterUserMessage{Password: clientHash})
	regFrame := client.expect(t, protocol.TypeRegisterResponse, timeout)
	var regResp protocol.RegisterResponseMessage
	if err := regResp.Decode(regFrame.Payload); err != nil {
		t.Fatalf("Decode REGISTER_RESPONSE: %v", err)
	}
	if !regResp.Success {
		t.Fatalf("Register failed: %s", regResp.Message)
	}
	if regResp.UserID == 0 {
		t.Fatal("Register returned UserID 0")
	}
	registeredUserID := regResp.UserID

	// Verify user exists in DB
	user, err := servers.srv.db.GetUserByNickname(nickname)
	if err != nil {
		t.Fatalf("DB GetUserByNickname: %v", err)
	}
	if user == nil {
		t.Fatal("User not found in DB after registration")
	}

	// Step 4: Reconnect and re-authenticate
	client.close()
	client = connect()

	client.expect(t, protocol.TypeServerConfig, timeout)

	client.send(t, protocol.TypeAuthRequest, &protocol.AuthRequestMessage{
		Nickname: nickname,
		Password: clientHash,
	})
	authFrame := client.expect(t, protocol.TypeAuthResponse, timeout)
	var authResp protocol.AuthResponseMessage
	if err := authResp.Decode(authFrame.Payload); err != nil {
		t.Fatalf("Decode AUTH_RESPONSE: %v", err)
	}
	if !authResp.Success {
		t.Fatalf("Auth failed: %s", authResp.Message)
	}
	if authResp.UserID != registeredUserID {
		t.Fatalf("Auth UserID: got %d, want %d", authResp.UserID, registeredUserID)
	}

	// Step 5: List channels
	client.send(t, protocol.TypeListChannels, &protocol.ListChannelsMessage{Limit: 50})
	chanListFrame := client.expect(t, protocol.TypeChannelList, timeout)
	var chanList protocol.ChannelListMessage
	if err := chanList.Decode(chanListFrame.Payload); err != nil {
		t.Fatalf("Decode CHANNEL_LIST: %v", err)
	}
	if len(chanList.Channels) == 0 {
		t.Fatal("Expected at least 1 channel (default seed)")
	}

	// Step 6: Create channel
	desc := "Journey test channel"
	client.send(t, protocol.TypeCreateChannel, &protocol.CreateChannelMessage{
		Name:           channelName,
		DisplayName:    "#" + channelName,
		Description:    &desc,
		ChannelType:    1, // forum
		RetentionHours: 168,
	})
	createFrame := client.expect(t, protocol.TypeChannelCreated, timeout)
	var createResp protocol.ChannelCreatedMessage
	if err := createResp.Decode(createFrame.Payload); err != nil {
		t.Fatalf("Decode CHANNEL_CREATED: %v", err)
	}
	if !createResp.Success {
		t.Fatalf("Create channel failed: %s", createResp.Message)
	}
	channelID := createResp.ChannelID

	// Step 7: Join channel
	client.send(t, protocol.TypeJoinChannel, &protocol.JoinChannelMessage{ChannelID: channelID})
	joinFrame := client.expect(t, protocol.TypeJoinResponse, timeout)
	var joinResp protocol.JoinResponseMessage
	if err := joinResp.Decode(joinFrame.Payload); err != nil {
		t.Fatalf("Decode JOIN_RESPONSE: %v", err)
	}
	if !joinResp.Success {
		t.Fatalf("Join channel failed: %s", joinResp.Message)
	}

	// Step 8: Subscribe channel
	client.send(t, protocol.TypeSubscribeChannel, &protocol.SubscribeChannelMessage{ChannelID: channelID})
	client.expect(t, protocol.TypeSubscribeOk, timeout)

	// Step 9: Post root message
	client.send(t, protocol.TypePostMessage, &protocol.PostMessageMessage{
		ChannelID: channelID,
		Content:   "Root message from " + tf.name,
	})
	postFrame := client.expect(t, protocol.TypeMessagePosted, timeout)
	var postResp protocol.MessagePostedMessage
	if err := postResp.Decode(postFrame.Payload); err != nil {
		t.Fatalf("Decode MESSAGE_POSTED: %v", err)
	}
	if !postResp.Success {
		t.Fatalf("Post root message failed: %s", postResp.Message)
	}
	rootMsgID := postResp.MessageID

	// Drain self-broadcast (we're subscribed to this channel)
	drain(t, client, 200*time.Millisecond)

	// Step 10: Post reply
	client.send(t, protocol.TypePostMessage, &protocol.PostMessageMessage{
		ChannelID: channelID,
		ParentID:  &rootMsgID,
		Content:   "Reply from " + tf.name,
	})
	replyFrame := client.expect(t, protocol.TypeMessagePosted, timeout)
	var replyResp protocol.MessagePostedMessage
	if err := replyResp.Decode(replyFrame.Payload); err != nil {
		t.Fatalf("Decode MESSAGE_POSTED (reply): %v", err)
	}
	if !replyResp.Success {
		t.Fatalf("Post reply failed: %s", replyResp.Message)
	}
	replyMsgID := replyResp.MessageID

	drain(t, client, 200*time.Millisecond)

	// Step 11: List root messages
	client.send(t, protocol.TypeListMessages, &protocol.ListMessagesMessage{
		ChannelID: channelID,
		Limit:     50,
	})
	listFrame := client.expect(t, protocol.TypeMessageList, timeout)
	var msgList protocol.MessageListMessage
	if err := msgList.Decode(listFrame.Payload); err != nil {
		t.Fatalf("Decode MESSAGE_LIST: %v", err)
	}
	foundRoot := false
	for _, m := range msgList.Messages {
		if m.ID == rootMsgID {
			foundRoot = true
			break
		}
	}
	if !foundRoot {
		t.Fatalf("Root message %d not found in message list", rootMsgID)
	}

	// Step 12: List thread replies
	client.send(t, protocol.TypeListMessages, &protocol.ListMessagesMessage{
		ChannelID: channelID,
		ParentID:  &rootMsgID,
		Limit:     50,
	})
	threadFrame := client.expect(t, protocol.TypeMessageList, timeout)
	var threadList protocol.MessageListMessage
	if err := threadList.Decode(threadFrame.Payload); err != nil {
		t.Fatalf("Decode MESSAGE_LIST (thread): %v", err)
	}
	foundReply := false
	for _, m := range threadList.Messages {
		if m.ID == replyMsgID {
			foundReply = true
			break
		}
	}
	if !foundReply {
		t.Fatalf("Reply message %d not found in thread list", replyMsgID)
	}

	// Step 13: Subscribe thread
	client.send(t, protocol.TypeSubscribeThread, &protocol.SubscribeThreadMessage{ThreadID: rootMsgID})
	client.expect(t, protocol.TypeSubscribeOk, timeout)

	// Step 14: Edit message
	client.send(t, protocol.TypeEditMessage, &protocol.EditMessageMessage{
		MessageID:  rootMsgID,
		NewContent: "Edited root message from " + tf.name,
	})
	editFrame := client.expect(t, protocol.TypeMessageEdited, timeout)
	var editResp protocol.MessageEditedMessage
	if err := editResp.Decode(editFrame.Payload); err != nil {
		t.Fatalf("Decode MESSAGE_EDITED: %v", err)
	}
	if !editResp.Success {
		t.Fatalf("Edit message failed: %s", editResp.Message)
	}
	if editResp.MessageID != rootMsgID {
		t.Fatalf("Edit MessageID: got %d, want %d", editResp.MessageID, rootMsgID)
	}

	drain(t, client, 200*time.Millisecond)

	// Step 15: Delete reply
	client.send(t, protocol.TypeDeleteMessage, &protocol.DeleteMessageMessage{MessageID: replyMsgID})
	deleteFrame := client.expect(t, protocol.TypeMessageDeleted, timeout)
	var deleteResp protocol.MessageDeletedMessage
	if err := deleteResp.Decode(deleteFrame.Payload); err != nil {
		t.Fatalf("Decode MESSAGE_DELETED: %v", err)
	}
	if !deleteResp.Success {
		t.Fatalf("Delete message failed: %s", deleteResp.Message)
	}
	if deleteResp.MessageID != replyMsgID {
		t.Fatalf("Delete MessageID: got %d, want %d", deleteResp.MessageID, replyMsgID)
	}

	drain(t, client, 200*time.Millisecond)

	// Step 16: Leave channel
	client.send(t, protocol.TypeLeaveChannel, &protocol.LeaveChannelMessage{
		ChannelID: channelID,
		Permanent: false,
	})
	leaveFrame := client.expect(t, protocol.TypeLeaveResponse, timeout)
	var leaveResp protocol.LeaveResponseMessage
	if err := leaveResp.Decode(leaveFrame.Payload); err != nil {
		t.Fatalf("Decode LEAVE_RESPONSE: %v", err)
	}
	if !leaveResp.Success {
		t.Fatalf("Leave channel failed: %s", leaveResp.Message)
	}

	// Step 17: Ping/Pong
	client.send(t, protocol.TypePing, &protocol.PingMessage{Timestamp: time.Now().UnixMilli()})
	client.expect(t, protocol.TypePong, timeout)

	// Step 18: Disconnect
	client.send(t, protocol.TypeDisconnect, &protocol.DisconnectMessage{})

	// Give server time to clean up
	time.Sleep(100 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Multi-client broadcast test
// ---------------------------------------------------------------------------

func runMultiClientBroadcast(t *testing.T, servers *journeyServers, tf transportFactory) {
	timeout := 5 * time.Second

	// Find "general" channel from the seeded defaults
	channels, err := servers.srv.db.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	var channelID uint64
	for _, ch := range channels {
		if ch.Name == "general" {
			channelID = uint64(ch.ID)
			break
		}
	}
	if channelID == 0 {
		t.Fatal("Default 'general' channel not found")
	}

	// Connect 3 clients
	clients := make([]transportClient, 3)
	for i := range clients {
		clients[i] = tf.connect(t, servers)
		defer clients[i].close()
	}

	// All receive SERVER_CONFIG, set nickname, subscribe
	for i, c := range clients {
		c.expect(t, protocol.TypeServerConfig, timeout)

		nick := fmt.Sprintf("bc_%s_%d", tf.name, i)
		c.send(t, protocol.TypeSetNickname, &protocol.SetNicknameMessage{Nickname: nick})
		c.expect(t, protocol.TypeNicknameResponse, timeout)

		c.send(t, protocol.TypeSubscribeChannel, &protocol.SubscribeChannelMessage{ChannelID: channelID})
		c.expect(t, protocol.TypeSubscribeOk, timeout)
	}

	// Client 0 posts a message
	content := fmt.Sprintf("Broadcast from %s", tf.name)
	clients[0].send(t, protocol.TypePostMessage, &protocol.PostMessageMessage{
		ChannelID: channelID,
		Content:   content,
	})
	clients[0].expect(t, protocol.TypeMessagePosted, timeout)

	// Clients 1 and 2 should receive NEW_MESSAGE broadcast
	var wg sync.WaitGroup
	for i := 1; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			frame := clients[idx].expect(t, protocol.TypeNewMessage, timeout)
			var msg protocol.NewMessageMessage
			if err := msg.Decode(frame.Payload); err != nil {
				t.Errorf("Client %d: decode NEW_MESSAGE: %v", idx, err)
				return
			}
			if msg.Content != content {
				t.Errorf("Client %d: content = %q, want %q", idx, msg.Content, content)
			}
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Cross-transport broadcast test
// ---------------------------------------------------------------------------

func runCrossTransportBroadcast(t *testing.T, servers *journeyServers) {
	timeout := 5 * time.Second

	// Find "general" channel
	channels, err := servers.srv.db.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	var channelID uint64
	for _, ch := range channels {
		if ch.Name == "general" {
			channelID = uint64(ch.ID)
			break
		}
	}
	if channelID == 0 {
		t.Fatal("Default 'general' channel not found")
	}

	// One client per transport
	tcpC := newTCPClient(t, servers.tcpAddr)
	defer tcpC.close()
	sshC := newSSHClient(t, servers.sshAddr)
	defer sshC.close()
	wsC := newWSClient(t, servers.wsAddr)
	defer wsC.close()

	type namedClient struct {
		name   string
		client transportClient
	}
	all := []namedClient{
		{"tcp", tcpC},
		{"ssh", sshC},
		{"ws", wsC},
	}

	// All receive SERVER_CONFIG, set nickname, subscribe
	for i, nc := range all {
		nc.client.expect(t, protocol.TypeServerConfig, timeout)

		nick := fmt.Sprintf("cross_%s_%d", nc.name, i)
		nc.client.send(t, protocol.TypeSetNickname, &protocol.SetNicknameMessage{Nickname: nick})
		nc.client.expect(t, protocol.TypeNicknameResponse, timeout)

		nc.client.send(t, protocol.TypeSubscribeChannel, &protocol.SubscribeChannelMessage{ChannelID: channelID})
		nc.client.expect(t, protocol.TypeSubscribeOk, timeout)
	}

	// TCP client posts
	content := "Cross-transport broadcast test"
	tcpC.send(t, protocol.TypePostMessage, &protocol.PostMessageMessage{
		ChannelID: channelID,
		Content:   content,
	})
	tcpC.expect(t, protocol.TypeMessagePosted, timeout)

	// SSH and WS should receive NEW_MESSAGE
	receivers := []namedClient{all[1], all[2]}
	var wg sync.WaitGroup
	for _, nc := range receivers {
		wg.Add(1)
		go func(nc namedClient) {
			defer wg.Done()
			frame := nc.client.expect(t, protocol.TypeNewMessage, timeout)
			var msg protocol.NewMessageMessage
			if err := msg.Decode(frame.Payload); err != nil {
				t.Errorf("%s: decode NEW_MESSAGE: %v", nc.name, err)
				return
			}
			if msg.Content != content {
				t.Errorf("%s: content = %q, want %q", nc.name, msg.Content, content)
			}
		}(nc)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// DM helper: setup two registered, authenticated clients
// ---------------------------------------------------------------------------

type twoClients struct {
	alice       transportClient
	bob         transportClient
	aliceUserID uint64
	bobUserID   uint64
	aliceNick   string
	bobNick     string
}

func setupTwoClients(t *testing.T, servers *journeyServers, tf transportFactory, testName string) (*twoClients, func()) {
	t.Helper()
	timeout := 5 * time.Second

	var clients []transportClient
	cleanup := func() {
		for _, c := range clients {
			c.close()
		}
	}

	connect := func() transportClient {
		c := tf.connect(t, servers)
		clients = append(clients, c)
		return c
	}

	registerClient := func(nick, password string) (transportClient, uint64) {
		c := connect()

		// Receive SERVER_CONFIG
		c.expect(t, protocol.TypeServerConfig, timeout)

		// Set nickname
		c.send(t, protocol.TypeSetNickname, &protocol.SetNicknameMessage{Nickname: nick})
		nickFrame := c.expect(t, protocol.TypeNicknameResponse, timeout)
		var nickResp protocol.NicknameResponseMessage
		if err := nickResp.Decode(nickFrame.Payload); err != nil {
			t.Fatalf("Decode NICKNAME_RESPONSE for %s: %v", nick, err)
		}
		if !nickResp.Success {
			t.Fatalf("Set nickname for %s failed: %s", nick, nickResp.Message)
		}

		// Register
		clientHash := hashPasswordForTest(password, nick)
		c.send(t, protocol.TypeRegisterUser, &protocol.RegisterUserMessage{Password: clientHash})
		regFrame := c.expect(t, protocol.TypeRegisterResponse, timeout)
		var regResp protocol.RegisterResponseMessage
		if err := regResp.Decode(regFrame.Payload); err != nil {
			t.Fatalf("Decode REGISTER_RESPONSE for %s: %v", nick, err)
		}
		if !regResp.Success {
			t.Fatalf("Register %s failed: %s", nick, regResp.Message)
		}
		if regResp.UserID == 0 {
			t.Fatalf("Register %s returned UserID 0", nick)
		}

		return c, regResp.UserID
	}

	// Keep nicknames ≤20 chars: shorten transport name
	tn := tf.name
	if tn == "websocket" {
		tn = "ws"
	}
	aliceNick := fmt.Sprintf("dma_%s_%s", tn, testName)
	bobNick := fmt.Sprintf("dmb_%s_%s", tn, testName)
	password := "TestDMPassword123!"

	alice, aliceUserID := registerClient(aliceNick, password)
	bob, bobUserID := registerClient(bobNick, password)

	return &twoClients{
		alice:       alice,
		bob:         bob,
		aliceUserID: aliceUserID,
		bobUserID:   bobUserID,
		aliceNick:   aliceNick,
		bobNick:     bobNick,
	}, cleanup
}

// generateTestKey returns a 32-byte random key suitable for X25519 testing.
func generateTestKey(t *testing.T) [32]byte {
	t.Helper()
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("Generate test key: %v", err)
	}
	return key
}

// ---------------------------------------------------------------------------
// DM journey: unencrypted flow
// ---------------------------------------------------------------------------

func runDMUnencrypted(t *testing.T, servers *journeyServers, tf transportFactory) {
	timeout := 5 * time.Second

	tc, cleanup := setupTwoClients(t, servers, tf, "unenc")
	defer cleanup()
	alice, bob := tc.alice, tc.bob
	aliceNick, bobNick := tc.aliceNick, tc.bobNick

	// Step 1: Alice sends START_DM (by nickname, allow unencrypted)
	alice.send(t, protocol.TypeStartDM, &protocol.StartDMMessage{
		TargetType:       protocol.DMTargetByNickname,
		TargetNickname:   bobNick,
		AllowUnencrypted: true,
	})

	// Step 2: Alice gets DM_PENDING
	pendingFrame := alice.expect(t, protocol.TypeDMPending, timeout)
	var pending protocol.DMPendingMessage
	if err := pending.Decode(pendingFrame.Payload); err != nil {
		t.Fatalf("Decode DM_PENDING: %v", err)
	}
	if pending.DMChannelID == 0 {
		t.Fatal("DM_PENDING: invite ID is 0")
	}
	inviteID := pending.DMChannelID

	// Step 3: Bob gets DM_REQUEST
	requestFrame := bob.expect(t, protocol.TypeDMRequest, timeout)
	var request protocol.DMRequestMessage
	if err := request.Decode(requestFrame.Payload); err != nil {
		t.Fatalf("Decode DM_REQUEST: %v", err)
	}
	if request.DMChannelID != inviteID {
		t.Fatalf("DM_REQUEST invite ID: got %d, want %d", request.DMChannelID, inviteID)
	}
	if request.EncryptionStatus != protocol.DMEncryptionNotPossible {
		t.Fatalf("DM_REQUEST encryption_status: got %d, want %d", request.EncryptionStatus, protocol.DMEncryptionNotPossible)
	}

	// Step 4: Bob sends ALLOW_UNENCRYPTED
	bob.send(t, protocol.TypeAllowUnencrypted, &protocol.AllowUnencryptedMessage{
		DMChannelID: inviteID,
		Permanent:   false,
	})

	// Step 5: Bob gets DM_READY
	bobReadyFrame := bob.expect(t, protocol.TypeDMReady, timeout)
	var bobReady protocol.DMReadyMessage
	if err := bobReady.Decode(bobReadyFrame.Payload); err != nil {
		t.Fatalf("Decode DM_READY (Bob): %v", err)
	}
	if bobReady.ChannelID == 0 {
		t.Fatal("DM_READY (Bob): channel ID is 0")
	}
	if bobReady.IsEncrypted {
		t.Fatal("DM_READY (Bob): expected unencrypted")
	}
	if bobReady.OtherNickname != aliceNick {
		t.Fatalf("DM_READY (Bob): OtherNickname = %q, want %q", bobReady.OtherNickname, aliceNick)
	}
	channelID := bobReady.ChannelID

	// Step 6: Alice gets DM_READY
	aliceReadyFrame := alice.expect(t, protocol.TypeDMReady, timeout)
	var aliceReady protocol.DMReadyMessage
	if err := aliceReady.Decode(aliceReadyFrame.Payload); err != nil {
		t.Fatalf("Decode DM_READY (Alice): %v", err)
	}
	if aliceReady.ChannelID != channelID {
		t.Fatalf("DM_READY channel mismatch: Alice=%d, Bob=%d", aliceReady.ChannelID, channelID)
	}
	if aliceReady.IsEncrypted {
		t.Fatal("DM_READY (Alice): expected unencrypted")
	}
	if aliceReady.OtherNickname != bobNick {
		t.Fatalf("DM_READY (Alice): OtherNickname = %q, want %q", aliceReady.OtherNickname, bobNick)
	}

	// Step 7-8: Both join the DM channel
	alice.send(t, protocol.TypeJoinChannel, &protocol.JoinChannelMessage{ChannelID: channelID})
	joinFrame := alice.expect(t, protocol.TypeJoinResponse, timeout)
	var joinResp protocol.JoinResponseMessage
	if err := joinResp.Decode(joinFrame.Payload); err != nil {
		t.Fatalf("Decode JOIN_RESPONSE (Alice): %v", err)
	}
	if !joinResp.Success {
		t.Fatalf("Alice join DM channel failed: %s", joinResp.Message)
	}

	bob.send(t, protocol.TypeJoinChannel, &protocol.JoinChannelMessage{ChannelID: channelID})
	bobJoinFrame := bob.expect(t, protocol.TypeJoinResponse, timeout)
	var bobJoinResp protocol.JoinResponseMessage
	if err := bobJoinResp.Decode(bobJoinFrame.Payload); err != nil {
		t.Fatalf("Decode JOIN_RESPONSE (Bob): %v", err)
	}
	if !bobJoinResp.Success {
		t.Fatalf("Bob join DM channel failed: %s", bobJoinResp.Message)
	}

	// Step 9: Bob subscribes
	bob.send(t, protocol.TypeSubscribeChannel, &protocol.SubscribeChannelMessage{ChannelID: channelID})
	bob.expect(t, protocol.TypeSubscribeOk, timeout)

	// Step 10: Alice posts a message
	alice.send(t, protocol.TypePostMessage, &protocol.PostMessageMessage{
		ChannelID: channelID,
		Content:   "Hello Bob",
	})
	postFrame := alice.expect(t, protocol.TypeMessagePosted, timeout)
	var postResp protocol.MessagePostedMessage
	if err := postResp.Decode(postFrame.Payload); err != nil {
		t.Fatalf("Decode MESSAGE_POSTED: %v", err)
	}
	if !postResp.Success {
		t.Fatalf("Post message failed: %s", postResp.Message)
	}

	// Step 11: Bob receives NEW_MESSAGE
	newMsgFrame := bob.expect(t, protocol.TypeNewMessage, timeout)
	var newMsg protocol.NewMessageMessage
	if err := newMsg.Decode(newMsgFrame.Payload); err != nil {
		t.Fatalf("Decode NEW_MESSAGE: %v", err)
	}
	if newMsg.Content != "Hello Bob" {
		t.Fatalf("NEW_MESSAGE content: got %q, want %q", newMsg.Content, "Hello Bob")
	}

	// Step 12: Alice permanently leaves the DM
	alice.send(t, protocol.TypeLeaveChannel, &protocol.LeaveChannelMessage{
		ChannelID: channelID,
		Permanent: true,
	})
	leaveFrame := alice.expect(t, protocol.TypeLeaveResponse, timeout)
	var leaveResp protocol.LeaveResponseMessage
	if err := leaveResp.Decode(leaveFrame.Payload); err != nil {
		t.Fatalf("Decode LEAVE_RESPONSE: %v", err)
	}
	if !leaveResp.Success {
		t.Fatalf("Leave DM failed: %s", leaveResp.Message)
	}

	// Step 13: Bob gets DM_PARTICIPANT_LEFT
	leftFrame := bob.expect(t, protocol.TypeDMParticipantLeft, timeout)
	var leftMsg protocol.DMParticipantLeftMessage
	if err := leftMsg.Decode(leftFrame.Payload); err != nil {
		t.Fatalf("Decode DM_PARTICIPANT_LEFT: %v", err)
	}
	if leftMsg.DMChannelID != channelID {
		t.Fatalf("DM_PARTICIPANT_LEFT channel: got %d, want %d", leftMsg.DMChannelID, channelID)
	}
	if leftMsg.Nickname != aliceNick {
		t.Fatalf("DM_PARTICIPANT_LEFT nickname: got %q, want %q", leftMsg.Nickname, aliceNick)
	}
}

// ---------------------------------------------------------------------------
// DM journey: encrypted flow (both users have keys beforehand)
// ---------------------------------------------------------------------------

func runDMEncrypted(t *testing.T, servers *journeyServers, tf transportFactory) {
	timeout := 5 * time.Second

	tc, cleanup := setupTwoClients(t, servers, tf, "enc")
	defer cleanup()
	alice, bob := tc.alice, tc.bob
	aliceNick, bobNick := tc.aliceNick, tc.bobNick

	// Step 1: Both provide encryption keys
	alicePub := generateTestKey(t)
	alice.send(t, protocol.TypeProvidePublicKey, &protocol.ProvidePublicKeyMessage{
		KeyType:   protocol.KeyTypeGenerated,
		PublicKey: alicePub,
		Label:     "alice-test",
	})
	// PROVIDE_PUBLIC_KEY has no response, small drain
	drain(t, alice, 200*time.Millisecond)

	bobPub := generateTestKey(t)
	bob.send(t, protocol.TypeProvidePublicKey, &protocol.ProvidePublicKeyMessage{
		KeyType:   protocol.KeyTypeGenerated,
		PublicKey: bobPub,
		Label:     "bob-test",
	})
	drain(t, bob, 200*time.Millisecond)

	// Step 2: Alice sends START_DM (require encryption)
	alice.send(t, protocol.TypeStartDM, &protocol.StartDMMessage{
		TargetType:       protocol.DMTargetByNickname,
		TargetNickname:   bobNick,
		AllowUnencrypted: false,
	})

	// Step 3: Alice gets DM_READY (immediate — both have keys)
	aliceReadyFrame := alice.expect(t, protocol.TypeDMReady, timeout)
	var aliceReady protocol.DMReadyMessage
	if err := aliceReady.Decode(aliceReadyFrame.Payload); err != nil {
		t.Fatalf("Decode DM_READY (Alice): %v", err)
	}
	if aliceReady.ChannelID == 0 {
		t.Fatal("DM_READY (Alice): channel ID is 0")
	}
	if !aliceReady.IsEncrypted {
		t.Fatal("DM_READY (Alice): expected encrypted")
	}
	if aliceReady.OtherNickname != bobNick {
		t.Fatalf("DM_READY (Alice): OtherNickname = %q, want %q", aliceReady.OtherNickname, bobNick)
	}
	if aliceReady.OtherPublicKey != bobPub {
		t.Fatal("DM_READY (Alice): OtherPublicKey does not match Bob's key")
	}
	channelID := aliceReady.ChannelID

	// Step 4: Bob gets DM_READY
	bobReadyFrame := bob.expect(t, protocol.TypeDMReady, timeout)
	var bobReady protocol.DMReadyMessage
	if err := bobReady.Decode(bobReadyFrame.Payload); err != nil {
		t.Fatalf("Decode DM_READY (Bob): %v", err)
	}
	if bobReady.ChannelID != channelID {
		t.Fatalf("DM_READY channel mismatch: Alice=%d, Bob=%d", channelID, bobReady.ChannelID)
	}
	if !bobReady.IsEncrypted {
		t.Fatal("DM_READY (Bob): expected encrypted")
	}
	if bobReady.OtherNickname != aliceNick {
		t.Fatalf("DM_READY (Bob): OtherNickname = %q, want %q", bobReady.OtherNickname, aliceNick)
	}
	if bobReady.OtherPublicKey != alicePub {
		t.Fatal("DM_READY (Bob): OtherPublicKey does not match Alice's key")
	}

	// Step 5-6: Both join the DM channel
	alice.send(t, protocol.TypeJoinChannel, &protocol.JoinChannelMessage{ChannelID: channelID})
	joinFrame := alice.expect(t, protocol.TypeJoinResponse, timeout)
	var joinResp protocol.JoinResponseMessage
	if err := joinResp.Decode(joinFrame.Payload); err != nil {
		t.Fatalf("Decode JOIN_RESPONSE (Alice): %v", err)
	}
	if !joinResp.Success {
		t.Fatalf("Alice join DM channel failed: %s", joinResp.Message)
	}

	bob.send(t, protocol.TypeJoinChannel, &protocol.JoinChannelMessage{ChannelID: channelID})
	bobJoinFrame := bob.expect(t, protocol.TypeJoinResponse, timeout)
	var bobJoinResp protocol.JoinResponseMessage
	if err := bobJoinResp.Decode(bobJoinFrame.Payload); err != nil {
		t.Fatalf("Decode JOIN_RESPONSE (Bob): %v", err)
	}
	if !bobJoinResp.Success {
		t.Fatalf("Bob join DM channel failed: %s", bobJoinResp.Message)
	}

	// Step 7: Bob subscribes
	bob.send(t, protocol.TypeSubscribeChannel, &protocol.SubscribeChannelMessage{ChannelID: channelID})
	bob.expect(t, protocol.TypeSubscribeOk, timeout)

	// Step 8: Alice posts a secret message
	alice.send(t, protocol.TypePostMessage, &protocol.PostMessageMessage{
		ChannelID: channelID,
		Content:   "Secret message",
	})
	postFrame := alice.expect(t, protocol.TypeMessagePosted, timeout)
	var postResp protocol.MessagePostedMessage
	if err := postResp.Decode(postFrame.Payload); err != nil {
		t.Fatalf("Decode MESSAGE_POSTED: %v", err)
	}
	if !postResp.Success {
		t.Fatalf("Post message failed: %s", postResp.Message)
	}

	// Step 9: Bob receives NEW_MESSAGE
	newMsgFrame := bob.expect(t, protocol.TypeNewMessage, timeout)
	var newMsg protocol.NewMessageMessage
	if err := newMsg.Decode(newMsgFrame.Payload); err != nil {
		t.Fatalf("Decode NEW_MESSAGE: %v", err)
	}
	if newMsg.Content != "Secret message" {
		t.Fatalf("NEW_MESSAGE content: got %q, want %q", newMsg.Content, "Secret message")
	}
}

// ---------------------------------------------------------------------------
// DM journey: key provisioning mid-flow
// ---------------------------------------------------------------------------

func runDMKeyProvisioning(t *testing.T, servers *journeyServers, tf transportFactory) {
	timeout := 5 * time.Second

	tc, cleanup := setupTwoClients(t, servers, tf, "keyprov")
	defer cleanup()
	alice, bob := tc.alice, tc.bob
	aliceNick, bobNick := tc.aliceNick, tc.bobNick

	// Step 1: Only Alice provides a key
	alicePub := generateTestKey(t)
	alice.send(t, protocol.TypeProvidePublicKey, &protocol.ProvidePublicKeyMessage{
		KeyType:   protocol.KeyTypeGenerated,
		PublicKey: alicePub,
		Label:     "alice-test",
	})
	drain(t, alice, 200*time.Millisecond)

	// Step 2: Alice sends START_DM by user ID, require encryption
	alice.send(t, protocol.TypeStartDM, &protocol.StartDMMessage{
		TargetType:       protocol.DMTargetByUserID,
		TargetUserID:     tc.bobUserID,
		AllowUnencrypted: false,
	})

	// Step 3: Alice gets DM_PENDING (Bob has no key → consent flow)
	pendingFrame := alice.expect(t, protocol.TypeDMPending, timeout)
	var pending protocol.DMPendingMessage
	if err := pending.Decode(pendingFrame.Payload); err != nil {
		t.Fatalf("Decode DM_PENDING: %v", err)
	}
	if pending.DMChannelID == 0 {
		t.Fatal("DM_PENDING: invite ID is 0")
	}
	if pending.WaitingForNickname != bobNick {
		t.Fatalf("DM_PENDING: WaitingForNickname = %q, want %q", pending.WaitingForNickname, bobNick)
	}

	// Step 4: Bob gets DM_REQUEST with encryption_required
	requestFrame := bob.expect(t, protocol.TypeDMRequest, timeout)
	var request protocol.DMRequestMessage
	if err := request.Decode(requestFrame.Payload); err != nil {
		t.Fatalf("Decode DM_REQUEST: %v", err)
	}
	if request.EncryptionStatus != protocol.DMEncryptionRequired {
		t.Fatalf("DM_REQUEST encryption_status: got %d, want %d", request.EncryptionStatus, protocol.DMEncryptionRequired)
	}
	if request.FromNickname != aliceNick {
		t.Fatalf("DM_REQUEST from_nickname: got %q, want %q", request.FromNickname, aliceNick)
	}

	// Step 5: Bob provides his key → server auto-resolves the invite
	bobPub := generateTestKey(t)
	bob.send(t, protocol.TypeProvidePublicKey, &protocol.ProvidePublicKeyMessage{
		KeyType:   protocol.KeyTypeGenerated,
		PublicKey: bobPub,
		Label:     "bob-test",
	})

	// Step 6: Bob gets DM_READY (auto-resolved by processPendingInviteAfterKey)
	bobReadyFrame := bob.expect(t, protocol.TypeDMReady, timeout)
	var bobReady protocol.DMReadyMessage
	if err := bobReady.Decode(bobReadyFrame.Payload); err != nil {
		t.Fatalf("Decode DM_READY (Bob): %v", err)
	}
	if bobReady.ChannelID == 0 {
		t.Fatal("DM_READY (Bob): channel ID is 0")
	}
	if !bobReady.IsEncrypted {
		t.Fatal("DM_READY (Bob): expected encrypted")
	}
	if bobReady.OtherNickname != aliceNick {
		t.Fatalf("DM_READY (Bob): OtherNickname = %q, want %q", bobReady.OtherNickname, aliceNick)
	}
	if bobReady.OtherPublicKey != alicePub {
		t.Fatal("DM_READY (Bob): OtherPublicKey does not match Alice's key")
	}
	channelID := bobReady.ChannelID

	// Step 7: Alice gets DM_READY
	aliceReadyFrame := alice.expect(t, protocol.TypeDMReady, timeout)
	var aliceReady protocol.DMReadyMessage
	if err := aliceReady.Decode(aliceReadyFrame.Payload); err != nil {
		t.Fatalf("Decode DM_READY (Alice): %v", err)
	}
	if aliceReady.ChannelID != channelID {
		t.Fatalf("DM_READY channel mismatch: Alice=%d, Bob=%d", aliceReady.ChannelID, channelID)
	}
	if !aliceReady.IsEncrypted {
		t.Fatal("DM_READY (Alice): expected encrypted")
	}
	if aliceReady.OtherPublicKey != bobPub {
		t.Fatal("DM_READY (Alice): OtherPublicKey does not match Bob's key")
	}
}

// ---------------------------------------------------------------------------
// DM journey: decline
// ---------------------------------------------------------------------------

func runDMDecline(t *testing.T, servers *journeyServers, tf transportFactory) {
	timeout := 5 * time.Second

	tc, cleanup := setupTwoClients(t, servers, tf, "decline")
	defer cleanup()
	alice, bob := tc.alice, tc.bob
	bobNick := tc.bobNick

	// Step 1: Alice sends START_DM (allow unencrypted)
	alice.send(t, protocol.TypeStartDM, &protocol.StartDMMessage{
		TargetType:       protocol.DMTargetByNickname,
		TargetNickname:   bobNick,
		AllowUnencrypted: true,
	})

	// Step 2: Alice gets DM_PENDING
	pendingFrame := alice.expect(t, protocol.TypeDMPending, timeout)
	var pending protocol.DMPendingMessage
	if err := pending.Decode(pendingFrame.Payload); err != nil {
		t.Fatalf("Decode DM_PENDING: %v", err)
	}
	inviteID := pending.DMChannelID

	// Step 3: Bob gets DM_REQUEST
	requestFrame := bob.expect(t, protocol.TypeDMRequest, timeout)
	var request protocol.DMRequestMessage
	if err := request.Decode(requestFrame.Payload); err != nil {
		t.Fatalf("Decode DM_REQUEST: %v", err)
	}
	if request.DMChannelID != inviteID {
		t.Fatalf("DM_REQUEST invite ID: got %d, want %d", request.DMChannelID, inviteID)
	}

	// Step 4: Bob declines
	bob.send(t, protocol.TypeDeclineDM, &protocol.DeclineDMMessage{
		DMChannelID: inviteID,
	})

	// Step 5: Alice gets DM_DECLINED
	declinedFrame := alice.expect(t, protocol.TypeDMDeclined, timeout)
	var declined protocol.DMDeclinedMessage
	if err := declined.Decode(declinedFrame.Payload); err != nil {
		t.Fatalf("Decode DM_DECLINED: %v", err)
	}
	if declined.DMChannelID != inviteID {
		t.Fatalf("DM_DECLINED invite ID: got %d, want %d", declined.DMChannelID, inviteID)
	}
	if declined.Nickname != bobNick {
		t.Fatalf("DM_DECLINED nickname: got %q, want %q", declined.Nickname, bobNick)
	}
}

// ---------------------------------------------------------------------------
// DM journey: no key required (server rejects immediately)
// ---------------------------------------------------------------------------

func runDMNoKeyRequired(t *testing.T, servers *journeyServers, tf transportFactory) {
	timeout := 5 * time.Second

	tc, cleanup := setupTwoClients(t, servers, tf, "nokey")
	defer cleanup()
	alice := tc.alice
	bobNick := tc.bobNick

	// Alice has no key, sends START_DM with allow_unencrypted=false
	alice.send(t, protocol.TypeStartDM, &protocol.StartDMMessage{
		TargetType:       protocol.DMTargetByNickname,
		TargetNickname:   bobNick,
		AllowUnencrypted: false,
	})

	// Server should respond with KEY_REQUIRED
	keyReqFrame := alice.expect(t, protocol.TypeKeyRequired, timeout)
	var keyReq protocol.KeyRequiredMessage
	if err := keyReq.Decode(keyReqFrame.Payload); err != nil {
		t.Fatalf("Decode KEY_REQUIRED: %v", err)
	}
	if keyReq.Reason == "" {
		t.Fatal("KEY_REQUIRED: reason is empty")
	}
}

// ---------------------------------------------------------------------------
// DM journey: existing channel reuse
// ---------------------------------------------------------------------------

func runDMExistingChannel(t *testing.T, servers *journeyServers, tf transportFactory) {
	timeout := 5 * time.Second

	tc, cleanup := setupTwoClients(t, servers, tf, "exist")
	defer cleanup()
	alice, bob := tc.alice, tc.bob
	bobNick := tc.bobNick

	// First: create an encrypted DM (same flow as runDMEncrypted)
	alicePub := generateTestKey(t)
	alice.send(t, protocol.TypeProvidePublicKey, &protocol.ProvidePublicKeyMessage{
		KeyType:   protocol.KeyTypeGenerated,
		PublicKey: alicePub,
		Label:     "alice-test",
	})
	drain(t, alice, 200*time.Millisecond)

	bobPub := generateTestKey(t)
	bob.send(t, protocol.TypeProvidePublicKey, &protocol.ProvidePublicKeyMessage{
		KeyType:   protocol.KeyTypeGenerated,
		PublicKey: bobPub,
		Label:     "bob-test",
	})
	drain(t, bob, 200*time.Millisecond)

	// Alice initiates DM
	alice.send(t, protocol.TypeStartDM, &protocol.StartDMMessage{
		TargetType:       protocol.DMTargetByNickname,
		TargetNickname:   bobNick,
		AllowUnencrypted: false,
	})

	// Alice gets DM_READY
	aliceReadyFrame := alice.expect(t, protocol.TypeDMReady, timeout)
	var aliceReady protocol.DMReadyMessage
	if err := aliceReady.Decode(aliceReadyFrame.Payload); err != nil {
		t.Fatalf("Decode DM_READY (first): %v", err)
	}
	firstChannelID := aliceReady.ChannelID

	// Bob gets DM_READY
	bob.expect(t, protocol.TypeDMReady, timeout)

	// Now: Alice sends START_DM again — should get same channel
	alice.send(t, protocol.TypeStartDM, &protocol.StartDMMessage{
		TargetType:       protocol.DMTargetByNickname,
		TargetNickname:   bobNick,
		AllowUnencrypted: false,
	})

	aliceReady2Frame := alice.expect(t, protocol.TypeDMReady, timeout)
	var aliceReady2 protocol.DMReadyMessage
	if err := aliceReady2.Decode(aliceReady2Frame.Payload); err != nil {
		t.Fatalf("Decode DM_READY (second): %v", err)
	}
	if aliceReady2.ChannelID != firstChannelID {
		t.Fatalf("Existing channel reuse: got %d, want %d", aliceReady2.ChannelID, firstChannelID)
	}
	if !aliceReady2.IsEncrypted {
		t.Fatal("DM_READY (reuse): expected encrypted")
	}
}
