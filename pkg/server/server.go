package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aeolun/superchat/pkg/database"
	"github.com/aeolun/superchat/pkg/protocol"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultDirectoryPort         = 6465
	directoryHealthCheckInterval = 5 * time.Minute
)

var (
	errorLog *log.Logger
	debugLog *log.Logger
)

// Server represents the SuperChat server
type Server struct {
	db          *database.MemDB
	listener    net.Listener
	sshListener net.Listener
	sessions    *SessionManager
	config      ServerConfig
	configPath  string
	shutdown    chan struct{}
	wg          sync.WaitGroup
	metrics     *Metrics
	startTime   time.Time // Server start time for uptime calculation

	// Connection deltas for periodic reporting
	connectionsSinceReport    atomic.Int64
	disconnectionsSinceReport atomic.Int64

	// Server discovery
	verificationMu         sync.Mutex
	verificationChallenges map[uint64]uint64 // sessionID -> challenge
	discoveryRateLimits    map[string]*discoveryRateLimiter
	discoveryRateLimitMu   sync.Mutex
	autoRegisterMu         sync.Mutex
	autoRegisterAttempts   map[string][]time.Time
}

// ServerConfig holds server configuration
type ServerConfig struct {
	TCPPort                 int
	SSHPort                 int
	HTTPPort                int // Public HTTP port for /servers.json (default: 8080, 0 = disabled)
	SSHHostKeyPath          string
	MaxConnectionsPerIP     uint8
	MessageRateLimit        uint16
	MaxChannelCreates       uint16
	InactiveCleanupDays     uint16
	MaxMessageLength        uint32
	SessionTimeoutSeconds   int
	ProtocolVersion         uint8
	MaxThreadSubscriptions  uint16
	MaxChannelSubscriptions uint16
	DirectoryEnabled        bool

	// Server discovery metadata (used when DirectoryEnabled=true)
	PublicHostname string // Public hostname/IP for clients to connect
	ServerName     string // Display name in server list
	ServerDesc     string // Description in server list
	MaxUsers       uint32 // Max concurrent users (0 = unlimited)

	// Admin configuration
	AdminUsers []string // List of admin user nicknames
}

// DefaultConfig returns default server configuration
func DefaultConfig() ServerConfig {
	return ServerConfig{
		TCPPort:                 6465,
		SSHPort:                 6466,
		HTTPPort:                8080, // Public HTTP server for /servers.json
		SSHHostKeyPath:          "~/.superchat/ssh_host_key",
		MaxConnectionsPerIP:     10,
		MessageRateLimit:        10,   // per minute
		MaxChannelCreates:       5,    // per hour
		InactiveCleanupDays:     90,   // days
		MaxMessageLength:        4096, // bytes
		SessionTimeoutSeconds:   120,  // 2 minutes
		ProtocolVersion:         1,
		MaxThreadSubscriptions:  50,   // max thread subscriptions per session
		MaxChannelSubscriptions: 10,   // max channel subscriptions per session
		DirectoryEnabled:        true, // Default: directory mode enabled

		// Server discovery metadata
		PublicHostname: "localhost",
		ServerName:     "SuperChat Server",
		ServerDesc:     "A SuperChat server",
		MaxUsers:       0, // unlimited
	}
}

// NewServer creates a new server instance
func NewServer(dbPath string, config ServerConfig, configPath string) (*Server, error) {
	// Open underlying SQLite database for snapshots
	sqliteDB, err := database.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Seed default channels if they don't exist
	if err := sqliteDB.SeedDefaultChannels(); err != nil {
		sqliteDB.Close()
		return nil, fmt.Errorf("failed to seed channels: %w", err)
	}

	// Create in-memory database with 30-second snapshot interval
	memDB, err := database.NewMemDB(sqliteDB, 30*time.Second)
	if err != nil {
		sqliteDB.Close()
		return nil, fmt.Errorf("failed to create in-memory database: %w", err)
	}

	// Initialize loggers
	if err := initLoggers(); err != nil {
		memDB.Close()
		sqliteDB.Close()
		return nil, fmt.Errorf("failed to initialize loggers: %w", err)
	}

	metrics := NewMetrics()
	sessions := NewSessionManager(memDB, config.SessionTimeoutSeconds)
	sessions.SetMetrics(metrics)

	server := &Server{
		db:                     memDB,
		sessions:               sessions,
		config:                 config,
		configPath:             configPath,
		shutdown:               make(chan struct{}),
		metrics:                metrics,
		startTime:              time.Now(),
		verificationChallenges: make(map[uint64]uint64),
		discoveryRateLimits:    make(map[string]*discoveryRateLimiter),
		autoRegisterAttempts:   make(map[string][]time.Time),
	}

	return server, nil
}

// getServerDataDir returns the server data directory, creating it if needed
func getServerDataDir() (string, error) {
	var dataDir string
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		dataDir = filepath.Join(xdg, "superchat")
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		dataDir = filepath.Join(homeDir, ".local", "share", "superchat")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create data directory: %w", err)
	}

	return dataDir, nil
}

// initLoggers sets up error and debug loggers
func initLoggers() error {
	// Get server data directory
	dataDir, err := getServerDataDir()
	if err != nil {
		return err
	}

	// Error log goes to stderr and errors.log
	errorLogPath := filepath.Join(dataDir, "errors.log")
	errorFile, err := os.OpenFile(errorLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	// Write startup marker to errors.log (for distinguishing between runs)
	startupMsg := fmt.Sprintf("=== Server started at %s ===\n", time.Now().Format(time.RFC3339))
	if _, err := errorFile.WriteString(startupMsg); err != nil {
		return err
	}

	errorLog = log.New(io.MultiWriter(os.Stderr, errorFile), "ERROR: ", log.LstdFlags)

	// Debug log goes to /dev/null by default (can be enabled via EnableDebugLogging)
	debugLog = log.New(io.Discard, "DEBUG: ", log.LstdFlags)

	// Redirect standard log (used by database package) to stdout and server.log
	// Truncate server.log on startup to avoid confusion from multiple runs
	serverLogPath := filepath.Join(dataDir, "server.log")
	serverLogFile, err := os.OpenFile(serverLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	log.SetOutput(io.MultiWriter(os.Stdout, serverLogFile))

	return nil
}

// EnableDebugLogging enables debug logging to debug.log
func (s *Server) EnableDebugLogging() {
	// Get server data directory
	dataDir, err := getServerDataDir()
	if err != nil {
		log.Printf("Failed to get data directory: %v", err)
		return
	}

	// Create/truncate debug.log
	debugLogPath := filepath.Join(dataDir, "debug.log")
	debugLogFile, err := os.OpenFile(debugLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		log.Printf("Failed to open debug.log: %v", err)
		return
	}

	debugLog = log.New(debugLogFile, "DEBUG: ", log.LstdFlags)
	debugLog.Println("Debug logging enabled")
}

// Start starts the TCP and SSH servers
func (s *Server) Start() error {
	// Start TCP server
	addr := fmt.Sprintf(":%d", s.config.TCPPort)

	// Use ListenConfig to enable SO_REUSEADDR
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				opErr = setSocketOptions(fd)
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}

	listener, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.listener = listener
	logListenBacklog(addr)

	// Start listen overflow monitor (Linux only)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.monitorListenOverflows()
	}()

	// Start SSH server
	if err := s.startSSHServer(); err != nil {
		s.listener.Close()
		return fmt.Errorf("failed to start SSH server: %w", err)
	}

	// Start metrics HTTP server (internal only - never expose publicly!)
	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsMux.HandleFunc("/health", s.HealthHandler)
		log.Printf("Metrics server listening on :9090 (/metrics, /health) - INTERNAL ONLY")
		if err := http.ListenAndServe(":9090", metricsMux); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// Start public HTTP server for /servers.json and WebSocket (safe to expose publicly)
	if s.config.HTTPPort > 0 {
		go func() {
			publicMux := http.NewServeMux()
			if s.config.DirectoryEnabled {
				publicMux.HandleFunc("/servers.json", s.ServersJSONHandler)
			}
			publicMux.HandleFunc("/ws", s.HandleWebSocket)
			addr := fmt.Sprintf(":%d", s.config.HTTPPort)

			endpoints := "/ws"
			if s.config.DirectoryEnabled {
				endpoints = "/servers.json, /ws"
			}
			log.Printf("Public HTTP server listening on %s (%s)", addr, endpoints)

			if err := http.ListenAndServe(addr, publicMux); err != nil {
				log.Printf("Public HTTP server error: %v", err)
			}
		}()
	}

	// Start metrics logging goroutine (log metrics every 5 seconds)
	s.wg.Add(1)
	go s.metricsLoggingLoop()

	// Start session cleanup goroutine
	s.wg.Add(1)
	go s.sessionCleanupLoop()

	// Start message retention cleanup goroutine
	s.wg.Add(1)
	go s.retentionCleanupLoop()

	// Start directory health checks (only when running as directory)
	if s.config.DirectoryEnabled {
		s.wg.Add(1)
		go s.directoryHealthCheckLoop()
	}

	// Accept TCP connections
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// GetChannels returns the list of channels from the database
func (s *Server) GetChannels() ([]*database.Channel, error) {
	return s.db.ListChannels()
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	log.Println("Graceful shutdown initiated...")

	// Signal shutdown to all goroutines
	close(s.shutdown)

	// Stop accepting new connections
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
		log.Println("TCP listener closed")
	}

	if s.sshListener != nil {
		s.sshListener.Close()
		s.sshListener = nil
		log.Println("SSH listener closed")
	}

	// Notify all connected clients before closing connections
	log.Println("Notifying connected clients of shutdown...")
	s.notifyClientsOfShutdown()

	// Close all sessions
	log.Println("Closing all client sessions...")
	s.sessions.CloseAll()

	// Wait for goroutines to finish (with timeout)
	log.Println("Waiting for background goroutines to finish...")
	s.wg.Wait()

	// Close in-memory database (triggers final snapshot to SQLite)
	log.Println("Flushing in-memory database to disk...")
	if err := s.db.Close(); err != nil {
		log.Printf("Error during database close: %v", err)
		return err
	}

	log.Println("Graceful shutdown complete")
	return nil
}

// notifyClientsOfShutdown sends DISCONNECT message to all connected clients
func (s *Server) notifyClientsOfShutdown() {
	sessions := s.sessions.GetAllSessions()

	if len(sessions) == 0 {
		log.Println("No active sessions to notify")
		return
	}

	log.Printf("Sending shutdown notification to %d active sessions...", len(sessions))

	// Create DISCONNECT message frame with reason
	reason := "Server shutting down for maintenance"
	disconnectMsg := &protocol.DisconnectMessage{
		Reason: &reason,
	}
	payload, err := disconnectMsg.Encode()
	if err != nil {
		log.Printf("Failed to encode disconnect message: %v", err)
		return
	}

	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeDisconnect,
		Flags:   0,
		Payload: payload,
	}

	// Send to all sessions concurrently (best effort)
	sent := 0
	for _, sess := range sessions {
		if err := sess.Conn.EncodeFrame(frame); err == nil {
			sent++
		}
	}

	log.Printf("Shutdown notification sent to %d/%d sessions", sent, len(sessions))
}

// acceptLoop accepts incoming connections
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		// Handle connection directly in goroutine
		go s.handleConnection(conn)
	}
}

// handleConnection handles initial connection setup, then spawns message loop goroutine
func (s *Server) handleConnection(conn net.Conn) {
	startTime := time.Now()

	// Disable Nagle's algorithm for immediate sends
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
	}

	afterTCP := time.Now()

	// Create session
	sess, err := s.sessions.CreateSession(nil, "", "tcp", conn)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		conn.Close()
		return
	}

	afterCreateSession := time.Now()

	// Track connection for periodic metrics
	s.connectionsSinceReport.Add(1)
	debugLog.Printf("New connection from %s (session %d)", conn.RemoteAddr(), sess.ID)

	// Send SERVER_CONFIG immediately after connection
	if err := s.sendServerConfig(sess); err != nil {
		// Debug log already shows the send attempt, clean up and return
		s.removeSession(sess.ID)
		conn.Close()
		return
	}

	afterServerConfig := time.Now()

	// Log timing if it took more than 100ms
	totalTime := afterServerConfig.Sub(startTime)
	if totalTime > 100*time.Millisecond {
		debugLog.Printf("Session %d: SLOW connection setup: total=%v (tcp=%v, createSess=%v, sendConfig=%v)",
			sess.ID,
			totalTime,
			afterTCP.Sub(startTime),
			afterCreateSession.Sub(afterTCP),
			afterServerConfig.Sub(afterCreateSession))
	}

	// Spawn goroutine for message loop (worker returns to pool)
	go s.messageLoop(sess, conn)
}

// messageLoop handles messages for an established connection
func (s *Server) messageLoop(sess *Session, conn net.Conn) {
	defer conn.Close()
	defer s.removeSession(sess.ID)

	// Message loop
	for {
		// Read frame
		frame, err := protocol.DecodeFrame(conn)
		if err != nil {
			// Check if session still exists (if not, it was closed by stale cleanup)
			_, exists := s.sessions.GetSession(sess.ID)

			// Remove from sessions map immediately to prevent broadcast attempts
			s.removeSession(sess.ID)

			// Only log if we're the ones who discovered the error (session existed)
			if exists {
				s.disconnectionsSinceReport.Add(1)
				if err == io.EOF {
					debugLog.Printf("Session %d: Client disconnected (message loop read)", sess.ID)
				} else {
					debugLog.Printf("Session %d: Message loop read error: %v", sess.ID, err)
				}
			}
			return
		}

		debugLog.Printf("Session %d ← RECV: Type=0x%02X Flags=0x%02X PayloadLen=%d", sess.ID, frame.Type, frame.Flags, len(frame.Payload))

		// Update session activity (buffered write, rate-limited to half of session timeout)
		s.sessions.UpdateSessionActivity(sess, time.Now().UnixMilli())

		// Track message received
		if s.metrics != nil {
			s.metrics.RecordMessageReceived(messageTypeToString(frame.Type))
		}

		// Handle message
		if err := s.handleMessage(sess, frame); err != nil {
			// If it's a graceful disconnect, exit cleanly
			if errors.Is(err, ErrClientDisconnecting) {
				s.disconnectionsSinceReport.Add(1)
				debugLog.Printf("Session %d disconnected gracefully", sess.ID)
				return
			}
			// Log and send error response for other errors
			log.Printf("Session %d handle error: %v", sess.ID, err)
			s.sendError(sess, 9000, fmt.Sprintf("Internal error: %v", err))
		}
	}
}

// handleMessage dispatches a frame to the appropriate handler
func (s *Server) handleMessage(sess *Session, frame *protocol.Frame) error {
	switch frame.Type {
	case protocol.TypeAuthRequest:
		return s.handleAuthRequest(sess, frame)
	case protocol.TypeSetNickname:
		return s.handleSetNickname(sess, frame)
	case protocol.TypeRegisterUser:
		return s.handleRegisterUser(sess, frame)
	case protocol.TypeLogout:
		return s.handleLogout(sess, frame)
	case protocol.TypeListChannels:
		return s.handleListChannels(sess, frame)
	case protocol.TypeJoinChannel:
		return s.handleJoinChannel(sess, frame)
	case protocol.TypeLeaveChannel:
		return s.handleLeaveChannel(sess, frame)
	case protocol.TypeCreateChannel:
		return s.handleCreateChannel(sess, frame)
	case protocol.TypeCreateSubchannel:
		return s.handleCreateSubchannel(sess, frame)
	case protocol.TypeGetSubchannels:
		return s.handleGetSubchannels(sess, frame)
	case protocol.TypeListMessages:
		return s.handleListMessages(sess, frame)
	case protocol.TypePostMessage:
		return s.handlePostMessage(sess, frame)
	case protocol.TypeEditMessage:
		return s.handleEditMessage(sess, frame)
	case protocol.TypeDeleteMessage:
		return s.handleDeleteMessage(sess, frame)
	case protocol.TypeChangePassword:
		return s.handleChangePassword(sess, frame)
	case protocol.TypeAddSSHKey:
		return s.handleAddSSHKey(sess, frame)
	case protocol.TypeListSSHKeys:
		return s.handleListSSHKeys(sess, frame)
	case protocol.TypeUpdateSSHKeyLabel:
		return s.handleUpdateSSHKeyLabel(sess, frame)
	case protocol.TypeDeleteSSHKey:
		return s.handleDeleteSSHKey(sess, frame)
	case protocol.TypeGetUserInfo:
		return s.handleGetUserInfo(sess, frame)
	case protocol.TypeListUsers:
		return s.handleListUsers(sess, frame)
	case protocol.TypeListChannelUsers:
		return s.handleListChannelUsers(sess, frame)
	case protocol.TypeGetUnreadCounts:
		return s.handleGetUnreadCounts(sess, frame)
	case protocol.TypeUpdateReadState:
		return s.handleUpdateReadState(sess, frame)
	case protocol.TypePing:
		return s.handlePing(sess, frame)
	case protocol.TypeDisconnect:
		return s.handleDisconnect(sess, frame)
	case protocol.TypeSubscribeThread:
		return s.handleSubscribeThread(sess, frame)
	case protocol.TypeUnsubscribeThread:
		return s.handleUnsubscribeThread(sess, frame)
	case protocol.TypeSubscribeChannel:
		return s.handleSubscribeChannel(sess, frame)
	case protocol.TypeUnsubscribeChannel:
		return s.handleUnsubscribeChannel(sess, frame)
	case protocol.TypeListServers:
		return s.handleListServers(sess, frame)
	case protocol.TypeRegisterServer:
		return s.handleRegisterServer(sess, frame)
	case protocol.TypeVerifyRegistration:
		return s.handleVerifyRegistration(sess, frame)
	case protocol.TypeVerifyResponse:
		return s.handleVerifyResponse(sess, frame)
	case protocol.TypeHeartbeat:
		return s.handleHeartbeat(sess, frame)
	case protocol.TypeBanUser:
		return s.handleBanUser(sess, frame)
	case protocol.TypeBanIP:
		return s.handleBanIP(sess, frame)
	case protocol.TypeUnbanUser:
		return s.handleUnbanUser(sess, frame)
	case protocol.TypeUnbanIP:
		return s.handleUnbanIP(sess, frame)
	case protocol.TypeListBans:
		return s.handleListBans(sess, frame)
	case protocol.TypeDeleteUser:
		return s.handleDeleteUser(sess, frame)
	case protocol.TypeDeleteChannel:
		return s.handleDeleteChannel(sess, frame)

	// V3 DM messages
	case protocol.TypeStartDM:
		return s.handleStartDM(sess, frame)
	case protocol.TypeProvidePublicKey:
		return s.handleProvidePublicKey(sess, frame)
	case protocol.TypeAllowUnencrypted:
		return s.handleAllowUnencrypted(sess, frame)

	default:
		// Unknown or unimplemented message type
		return s.sendError(sess, 1001, "Unsupported message type")
	}
}

func (s *Server) removeSession(sessionID uint64) {
	sess, ok := s.sessions.GetSession(sessionID)
	var joined *int64
	if ok {
		sess.mu.RLock()
		joined = sess.JoinedChannel
		sess.mu.RUnlock()
	}

	s.sessions.RemoveSession(sessionID)

	if ok {
		if joined != nil {
			s.notifyChannelPresence(*joined, sess, false)
		}
		s.notifyServerPresence(sess, false)
	}
}

// sendServerConfig sends the SERVER_CONFIG message to a session
func (s *Server) sendServerConfig(sess *Session) error {
	msg := &protocol.ServerConfigMessage{
		ProtocolVersion:         s.config.ProtocolVersion,
		MaxMessageRate:          s.config.MessageRateLimit,
		MaxChannelCreates:       s.config.MaxChannelCreates,
		InactiveCleanupDays:     s.config.InactiveCleanupDays,
		MaxConnectionsPerIP:     s.config.MaxConnectionsPerIP,
		MaxMessageLength:        s.config.MaxMessageLength,
		MaxThreadSubscriptions:  s.config.MaxThreadSubscriptions,
		MaxChannelSubscriptions: s.config.MaxChannelSubscriptions,
		DirectoryEnabled:        s.config.DirectoryEnabled,
	}

	payload, err := msg.Encode()
	if err != nil {
		return err
	}

	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeServerConfig,
		Flags:   0,
		Payload: payload,
	}

	debugLog.Printf("Session %d → SEND: Type=0x%02X (SERVER_CONFIG) Flags=0x%02X PayloadLen=%d", sess.ID, protocol.TypeServerConfig, 0, len(payload))
	if s.metrics != nil {
		s.metrics.RecordMessageSent(messageTypeToString(protocol.TypeServerConfig))
	}
	return sess.Conn.EncodeFrame(frame)
}

// sendError sends an ERROR message to a session
func (s *Server) sendError(sess *Session, code uint16, message string) error {
	msg := &protocol.ErrorMessage{
		ErrorCode: code,
		Message:   message,
	}

	payload, err := msg.Encode()
	if err != nil {
		return err
	}

	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeError,
		Flags:   0,
		Payload: payload,
	}

	if s.metrics != nil {
		s.metrics.RecordMessageSent(messageTypeToString(protocol.TypeError))
	}
	return sess.Conn.EncodeFrame(frame)
}

// isAdmin checks if a session belongs to an admin user
// Returns false for anonymous users (nil UserID)
// Returns true if session nickname matches any admin user in config
func (s *Server) isAdmin(sess *Session) bool {
	// Anonymous users can never be admin
	if sess.UserID == nil {
		return false
	}

	// Check if nickname is in admin list
	for _, adminNick := range s.config.AdminUsers {
		if sess.Nickname == adminNick {
			return true
		}
	}

	return false
}

// metricsLoggingLoop periodically logs key metrics
func (s *Server) metricsLoggingLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			// Get current counts
			activeSessions := s.sessions.CountOnlineUsers()
			goroutines := runtime.NumGoroutine()

			// Get deltas and reset
			connected := s.connectionsSinceReport.Swap(0)
			disconnected := s.disconnectionsSinceReport.Swap(0)

			log.Printf("[METRICS] Active sessions: %d, connected since last: %d, disconnected since last: %d, goroutines: %d",
				activeSessions, connected, disconnected, goroutines)
		}
	}
}

// sessionCleanupLoop periodically cleans up stale sessions
func (s *Server) sessionCleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.cleanupStaleSessions()
		}
	}
}

// cleanupStaleSessions removes sessions that have been inactive
func (s *Server) cleanupStaleSessions() {
	timeout := time.Duration(s.config.SessionTimeoutSeconds) * time.Second
	cutoff := time.Now().Add(-timeout).UnixMilli()

	sessions := s.sessions.GetAllSessions()
	for _, sess := range sessions {
		dbSess, err := s.db.GetSession(sess.DBSessionID)
		if err != nil {
			continue
		}

		if dbSess.LastActivity < cutoff {
			s.disconnectionsSinceReport.Add(1)
			debugLog.Printf("Closing stale session %d (inactive for %v)", sess.ID, timeout)
			s.removeSession(sess.ID)
		}
	}
}

// retentionCleanupLoop periodically cleans up old messages based on channel retention policies
func (s *Server) retentionCleanupLoop() {
	defer s.wg.Done()

	// Run cleanup every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run cleanup immediately on startup
	s.cleanupExpiredMessages()

	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.cleanupExpiredMessages()
		}
	}
}

// cleanupExpiredMessages deletes messages older than their channel's retention policy
func (s *Server) cleanupExpiredMessages() {
	count, err := s.db.CleanupExpiredMessages()
	if err != nil {
		log.Printf("Error cleaning up expired messages: %v", err)
		return
	}

	if count > 0 {
		log.Printf("Cleaned up %d expired messages", count)
	}

	// Also cleanup idle sessions from the database
	sessionTimeout := int64(s.config.SessionTimeoutSeconds)
	sessionCount, err := s.db.CleanupIdleSessions(sessionTimeout)
	if err != nil {
		log.Printf("Error cleaning up idle sessions from database: %v", err)
		return
	}

	if sessionCount > 0 {
		log.Printf("Cleaned up %d idle database sessions", sessionCount)
	}
}

// directoryHealthCheckLoop periodically verifies registered servers are reachable.
func (s *Server) directoryHealthCheckLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(directoryHealthCheckInterval)
	defer ticker.Stop()

	// Run an immediate check so newly registered servers are validated promptly.
	s.runDirectoryHealthCheck()

	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.runDirectoryHealthCheck()
		}
	}
}

// runDirectoryHealthCheck verifies all known servers and refreshes their heartbeat timestamps.
func (s *Server) runDirectoryHealthCheck() {
	servers, err := s.db.ListDiscoveredServers(^uint16(0))
	if err != nil {
		log.Printf("Directory health check: failed to list servers: %v", err)
		return
	}

	if len(servers) == 0 {
		return
	}

	var successes, failures int
	for _, entry := range servers {
		if err := s.verifyRegisteredServer(entry); err != nil {
			failures++
			log.Printf("Directory health check: %s:%d verification failed: %v", entry.Hostname, entry.Port, err)
			continue
		}
		successes++
	}

	log.Printf("Directory health check complete: %d verified, %d failed", successes, failures)
}

// verifyRegisteredServer confirms the server is reachable and updates its heartbeat metadata.
func (s *Server) verifyRegisteredServer(entry *database.DiscoveredServer) error {
	if _, err := s.verifyServerReachability(entry.Hostname, entry.Port); err != nil {
		return err
	}

	intervalSeconds := uint32(directoryHealthCheckInterval.Seconds())
	if intervalSeconds == 0 {
		intervalSeconds = 300
	}

	if err := s.db.UpdateHeartbeat(entry.Hostname, entry.Port, entry.UserCount, entry.UptimeSeconds, entry.ChannelCount, intervalSeconds); err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	return nil
}

// verifyServerReachability performs the VERIFY_REGISTRATION handshake against a remote server.
func (s *Server) verifyServerReachability(host string, port uint16) (*protocol.ServerConfigMessage, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(int(port)))

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not connect: %w", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}

	handshakeFrame, err := protocol.DecodeFrame(conn)
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		return nil, fmt.Errorf("failed to read SERVER_CONFIG: %w", err)
	}

	if handshakeFrame.Type != protocol.TypeServerConfig {
		return nil, fmt.Errorf("unexpected frame type 0x%02x (expected SERVER_CONFIG)", handshakeFrame.Type)
	}

	serverConfigMsg := &protocol.ServerConfigMessage{}
	if err := serverConfigMsg.Decode(handshakeFrame.Payload); err != nil {
		return nil, fmt.Errorf("failed to decode SERVER_CONFIG: %w", err)
	}

	if serverConfigMsg.ProtocolVersion != protocol.ProtocolVersion {
		return nil, fmt.Errorf("protocol mismatch (remote=%d, local=%d)", serverConfigMsg.ProtocolVersion, protocol.ProtocolVersion)
	}

	challenge := uint64(time.Now().UnixNano())
	verifyMsg := &protocol.VerifyRegistrationMessage{
		Challenge: challenge,
	}

	payload, err := verifyMsg.Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to encode VERIFY_REGISTRATION: %w", err)
	}

	verifyFrame := &protocol.Frame{
		Version: 1,
		Type:    protocol.TypeVerifyRegistration,
		Flags:   0,
		Payload: payload,
	}

	if err := protocol.EncodeFrame(conn, verifyFrame); err != nil {
		return nil, fmt.Errorf("failed to send VERIFY_REGISTRATION: %w", err)
	}

	for {
		if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return nil, fmt.Errorf("failed to set read deadline: %w", err)
		}

		respFrame, err := protocol.DecodeFrame(conn)
		if err != nil {
			return nil, fmt.Errorf("failed to read verification response: %w", err)
		}

		switch respFrame.Type {
		case protocol.TypeVerifyResponse:
			conn.SetReadDeadline(time.Time{})

			respMsg := &protocol.VerifyResponseMessage{}
			if err := respMsg.Decode(respFrame.Payload); err != nil {
				return nil, fmt.Errorf("failed to decode VERIFY_RESPONSE: %w", err)
			}

			if respMsg.Challenge != challenge {
				return nil, fmt.Errorf("verification failed: wrong challenge (expected %d, got %d)", challenge, respMsg.Challenge)
			}

			// Try to close the connection gracefully; ignore errors.
			_ = sendDisconnectFrame(conn, "Directory verification complete")
			return serverConfigMsg, nil

		case protocol.TypeError:
			errMsg := &protocol.ErrorMessage{}
			if err := errMsg.Decode(respFrame.Payload); err != nil {
				return nil, fmt.Errorf("verification failed: could not decode ERROR frame: %w", err)
			}
			return nil, fmt.Errorf("verification failed: remote error (code=%d, message=%s)", errMsg.ErrorCode, errMsg.Message)

		case protocol.TypeServerConfig:
			// Duplicate SERVER_CONFIG; ignore.
			continue

		case protocol.TypeRegisterAck:
			// Some implementations might send REGISTER_ACK; ignore and continue waiting.
			continue

		default:
			return nil, fmt.Errorf("verification failed: unexpected response type 0x%02x", respFrame.Type)
		}
	}
}

// sendDisconnectFrame attempts to send a DISCONNECT frame with an optional reason.
func sendDisconnectFrame(conn net.Conn, reason string) error {
	var msg protocol.DisconnectMessage
	if reason != "" {
		msg.Reason = &reason
	}

	payload, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("failed to encode DISCONNECT message: %w", err)
	}

	frame := &protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.TypeDisconnect,
		Flags:   0,
		Payload: payload,
	}

	return protocol.EncodeFrame(conn, frame)
}

// ===== Server Discovery Methods =====

// DisableDirectory disables directory mode (server won't accept registrations)
func (s *Server) DisableDirectory() {
	s.config.DirectoryEnabled = false
}

// EnableDirectory enables directory mode (server will accept registrations)
func (s *Server) EnableDirectory() {
	s.config.DirectoryEnabled = true
}

// AnnounceToDirectory announces this server to a directory server using a transient connection.
func (s *Server) AnnounceToDirectory(directoryAddr, serverName, serverDescription string) {
	// Check if we're listening on localhost only
	if s.listener != nil {
		addr := s.listener.Addr().String()
		// Check if listening on localhost/127.0.0.1
		if strings.HasPrefix(addr, "127.0.0.1:") || strings.HasPrefix(addr, "[::1]:") || strings.HasPrefix(addr, "localhost:") {
			log.Printf("WARNING: Server is listening on localhost only (%s) - not announcing to public directory %s", addr, directoryAddr)
			return
		}
	}

	directoryAddr = strings.TrimSpace(directoryAddr)
	if directoryAddr == "" {
		log.Printf("Directory address is empty; skipping announcement")
		return
	}

	// Parse directory address (allow host-only, defaulting to SuperChat TCP port)
	host, portStr, err := net.SplitHostPort(directoryAddr)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			host = directoryAddr
			portStr = strconv.Itoa(defaultDirectoryPort)
			log.Printf("No port specified for directory %s; defaulting to port %d", directoryAddr, defaultDirectoryPort)
		} else {
			log.Printf("Failed to parse directory address %s: %v", directoryAddr, err)
			return
		}
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Printf("Invalid port in directory address %s: %v", directoryAddr, err)
		return
	}
	if port <= 0 || port > int(^uint16(0)) {
		log.Printf("Invalid port in directory address %s: %d (out of range)", directoryAddr, port)
		return
	}

	normalizedAddr := net.JoinHostPort(host, strconv.Itoa(port))

	// Determine the hostname we advertise to the directory.
	ourHostname := strings.TrimSpace(s.config.PublicHostname)
	if ourHostname == "" {
		ourHostname = host
	}

	// Determine the port we actually listen on (handles dynamic ports)
	var ourPort uint16
	if s.listener != nil {
		if tcpAddr, ok := s.listener.Addr().(*net.TCPAddr); ok && tcpAddr.Port > 0 {
			ourPort = uint16(tcpAddr.Port)
		} else if addr := s.listener.Addr().String(); addr != "" {
			if _, portStr, err := net.SplitHostPort(addr); err == nil {
				if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= int(^uint16(0)) {
					ourPort = uint16(p)
				}
			}
		}
	}
	if ourPort == 0 {
		ourPort = uint16(s.config.TCPPort)
	}

	// Start announcement loop
	go s.maintainDirectoryAnnouncement(normalizedAddr, ourHostname, ourPort, serverName, serverDescription)
}

// maintainDirectoryAnnouncement performs a one-shot registration handshake with the directory
// and then disconnects gracefully. No persistent heartbeat connection is maintained.
func (s *Server) maintainDirectoryAnnouncement(directoryAddr, ourHostname string, ourPort uint16, serverName, serverDescription string) {
	for {
		select {
		case <-s.shutdown:
			log.Printf("Announcement to %s cancelled (server shutting down)", directoryAddr)
			return
		default:
		}

		// Connect to directory
		conn, err := net.DialTimeout("tcp", directoryAddr, 10*time.Second)
		if err != nil {
			log.Printf("Failed to connect to directory %s: %v (retrying in 60s)", directoryAddr, err)
			if !s.waitForDirectoryRetry() {
				return
			}
			continue
		}

		log.Printf("Connected to directory %s", directoryAddr)

		// Read initial SERVER_CONFIG frame to validate protocol compatibility
		if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			log.Printf("Failed to set read deadline for %s: %v", directoryAddr, err)
			conn.Close()
			if !s.waitForDirectoryRetry() {
				return
			}
			continue
		}

		handshakeFrame, err := protocol.DecodeFrame(conn)
		conn.SetReadDeadline(time.Time{})
		if err != nil {
			log.Printf("Failed to read SERVER_CONFIG from %s: %v (retrying in 60s)", directoryAddr, err)
			conn.Close()
			if !s.waitForDirectoryRetry() {
				return
			}
			continue
		}

		if handshakeFrame.Type != protocol.TypeServerConfig {
			log.Printf("Expected SERVER_CONFIG from %s, got 0x%02x", directoryAddr, handshakeFrame.Type)
			conn.Close()
			if !s.waitForDirectoryRetry() {
				return
			}
			continue
		}

		serverConfigMsg := &protocol.ServerConfigMessage{}
		if err := serverConfigMsg.Decode(handshakeFrame.Payload); err != nil {
			log.Printf("Failed to decode SERVER_CONFIG from %s: %v", directoryAddr, err)
			conn.Close()
			if !s.waitForDirectoryRetry() {
				return
			}
			continue
		}

		if serverConfigMsg.ProtocolVersion != protocol.ProtocolVersion {
			log.Printf("Protocol version mismatch with %s (server=%d, directory=%d)", directoryAddr, serverConfigMsg.ProtocolVersion, protocol.ProtocolVersion)
			conn.Close()
			if !s.waitForDirectoryRetry() {
				return
			}
			continue
		}

		log.Printf("Handshake with directory %s succeeded (protocol v%d)", directoryAddr, serverConfigMsg.ProtocolVersion)

		// Send REGISTER_SERVER
		registerMsg := &protocol.RegisterServerMessage{
			Hostname:    ourHostname,
			Port:        ourPort,
			Name:        serverName,
			Description: serverDescription,
			MaxUsers:    0, // 0 = unlimited
			IsPublic:    true,
		}

		payload, err := registerMsg.Encode()
		if err != nil {
			log.Printf("Failed to encode REGISTER_SERVER: %v", err)
			conn.Close()
			if !s.waitForDirectoryRetry() {
				return
			}
			continue
		}

		frame := &protocol.Frame{
			Version: 1,
			Type:    protocol.TypeRegisterServer,
			Flags:   0,
			Payload: payload,
		}

		if err := protocol.EncodeFrame(conn, frame); err != nil {
			log.Printf("Failed to send REGISTER_SERVER to %s: %v", directoryAddr, err)
			conn.Close()
			if !s.waitForDirectoryRetry() {
				return
			}
			continue
		}

		log.Printf("Sent REGISTER_SERVER to %s", directoryAddr)

		// Wait for optional REGISTER_ACK or verification challenge.
		ackHandled := false
	verificationLoop:
		for {
			if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
				log.Printf("Failed to set read deadline for %s: %v", directoryAddr, err)
				break
			}

			frame, err := protocol.DecodeFrame(conn)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					log.Printf("Directory %s did not send ACK within 30s; assuming registration accepted", directoryAddr)
					break verificationLoop
				}
				if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("Directory %s closed registration connection; assuming registration accepted", directoryAddr)
					break verificationLoop
				}

				log.Printf("Failed to receive registration response from %s: %v (retrying in 60s)", directoryAddr, err)
				conn.Close()
				if !s.waitForDirectoryRetry() {
					return
				}
				continue
			}

			switch frame.Type {
			case protocol.TypeRegisterAck:
				ackMsg := &protocol.RegisterAckMessage{}
				if err := ackMsg.Decode(frame.Payload); err != nil {
					log.Printf("Failed to decode REGISTER_ACK from %s: %v", directoryAddr, err)
					break verificationLoop
				}

				if ackMsg.Success {
					log.Printf("Directory %s accepted registration: %s", directoryAddr, ackMsg.Message)
				} else {
					log.Printf("Directory %s acknowledged registration (pending verification): %s", directoryAddr, ackMsg.Message)
				}
				ackHandled = true
				break verificationLoop

			case protocol.TypeVerifyRegistration:
				verifyMsg := &protocol.VerifyRegistrationMessage{}
				if err := verifyMsg.Decode(frame.Payload); err != nil {
					log.Printf("Failed to decode VERIFY_REGISTRATION from %s: %v", directoryAddr, err)
					break verificationLoop
				}

				log.Printf("Directory %s issued inline verification challenge: %d", directoryAddr, verifyMsg.Challenge)

				responseMsg := &protocol.VerifyResponseMessage{
					Challenge: verifyMsg.Challenge,
				}
				respPayload, err := responseMsg.Encode()
				if err != nil {
					log.Printf("Failed to encode VERIFY_RESPONSE for %s: %v", directoryAddr, err)
					break verificationLoop
				}

				respFrame := &protocol.Frame{
					Version: 1,
					Type:    protocol.TypeVerifyResponse,
					Flags:   0,
					Payload: respPayload,
				}

				if err := protocol.EncodeFrame(conn, respFrame); err != nil {
					log.Printf("Failed to send VERIFY_RESPONSE to %s: %v", directoryAddr, err)
				} else {
					log.Printf("Sent VERIFY_RESPONSE to %s", directoryAddr)
				}
				ackHandled = true
				break verificationLoop

			case protocol.TypeError:
				errorMsg := &protocol.ErrorMessage{}
				if err := errorMsg.Decode(frame.Payload); err != nil {
					log.Printf("Directory %s responded with ERROR (decode failed): %v", directoryAddr, err)
				} else {
					log.Printf("Directory %s rejected registration: code=%d message=%s", directoryAddr, errorMsg.ErrorCode, errorMsg.Message)
				}
				conn.Close()
				if !s.waitForDirectoryRetry() {
					return
				}
				continue

			default:
				log.Printf("Unexpected frame 0x%02x from %s during registration; closing connection", frame.Type, directoryAddr)
				break verificationLoop
			}
		}

		// Send graceful disconnect and finish.
		if err := sendDisconnectFrame(conn, "Registration complete"); err != nil {
			log.Printf("Failed to send DISCONNECT to %s: %v", directoryAddr, err)
		}
		conn.Close()

		if ackHandled {
			log.Printf("Registration handshake with directory %s finished; connection closed", directoryAddr)
		} else {
			log.Printf("Registration handshake with directory %s finished without explicit ACK; proceeding without heartbeat", directoryAddr)
		}

		return
	}
}

// waitForDirectoryRetry waits for the retry delay unless the server is shutting down.
func (s *Server) waitForDirectoryRetry() bool {
	select {
	case <-s.shutdown:
		return false
	case <-time.After(60 * time.Second):
		return true
	}
}
