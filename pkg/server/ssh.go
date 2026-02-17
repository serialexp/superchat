package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aeolun/superchat/pkg/database"
	"github.com/aeolun/superchat/pkg/protocol"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"
)

const (
	maxAutoRegistrationsPerHour = 10
	autoRegisterWindow          = time.Hour
)

// startSSHServer starts the SSH server on the configured port
func (s *Server) startSSHServer() error {
	if s.config.SSHPort <= 0 {
		log.Printf("SSH server disabled (ssh_port=%d)", s.config.SSHPort)
		return nil
	}

	// Load or generate host key
	hostKey, err := s.loadOrGenerateHostKey()
	if err != nil {
		return fmt.Errorf("failed to load host key: %w", err)
	}

	// Configure SSH server with public key authentication (V2)
	config := &ssh.ServerConfig{
		PublicKeyCallback: s.authenticateSSHKey,
		ServerVersion:     "SSH-2.0-SuperChat",
	}
	config.AddHostKey(hostKey)

	// Listen on SSH port
	addr := fmt.Sprintf(":%d", s.config.SSHPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.sshListener = listener

	log.Printf("SSH server listening on %s", addr)

	// Accept connections in a goroutine
	s.wg.Add(1)
	go s.acceptSSHLoop(listener, config)

	return nil
}

// acceptSSHLoop accepts incoming SSH connections
func (s *Server) acceptSSHLoop(listener net.Listener, config *ssh.ServerConfig) {
	defer s.wg.Done()
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return
			default:
				log.Printf("SSH accept error: %v", err)
				continue
			}
		}

		// Handle SSH connection in a goroutine
		s.wg.Add(1)
		go s.handleSSHConnection(conn, config)
	}
}

// handleSSHConnection handles a single SSH connection
func (s *Server) handleSSHConnection(conn net.Conn, config *ssh.ServerConfig) {
	defer s.wg.Done()
	defer conn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		log.Printf("SSH handshake failed: %v", err)
		return
	}
	defer sshConn.Close()

	// Discard global out-of-band requests
	go ssh.DiscardRequests(reqs)

	// Handle incoming channels
	for newChannel := range chans {
		// We only accept "session" channels for our binary protocol
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Could not accept channel: %v", err)
			continue
		}

		// Handle the session in the existing handler
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			go s.handleSSHChannelRequests(requests)
			// Pass SSH permissions (contains authenticated user info)
			s.handleSSHSession(channel, sshConn.Permissions)
		}()
	}
}

func (s *Server) handleSSHChannelRequests(requests <-chan *ssh.Request) {
	for req := range requests {
		switch req.Type {
		case "shell", "pty-req", "env", "window-change":
			if req.WantReply {
				req.Reply(true, nil)
			}
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// handleSSHSession wraps an SSH channel and uses the existing protocol handler
func (s *Server) handleSSHSession(channel ssh.Channel, permissions *ssh.Permissions) {
	defer channel.Close()

	// Wrap the SSH channel as a net.Conn-like interface
	conn := &sshChannelConn{channel: channel}

	// Extract authenticated user info from SSH permissions (V2 feature)
	var userID *int64
	var nickname string
	var userFlags uint8
	if permissions != nil && permissions.Extensions != nil {
		if uidStr := permissions.Extensions["user_id"]; uidStr != "" {
			if uid, err := strconv.ParseInt(uidStr, 10, 64); err == nil {
				userID = &uid
			}
		}
		nickname = permissions.Extensions["nickname"]
		if flagsStr := permissions.Extensions["user_flags"]; flagsStr != "" {
			if flags, err := strconv.ParseUint(flagsStr, 10, 8); err == nil {
				userFlags = uint8(flags)
			}
		}
	}

	// Create authenticated session
	sess, err := s.sessions.CreateSession(userID, nickname, "ssh", conn)
	if err != nil {
		log.Printf("Failed to create SSH session: %v", err)
		return
	}
	defer s.removeSession(sess.ID)

	// Track connection for periodic metrics
	s.connectionsSinceReport.Add(1)
	debugLog.Printf("New SSH connection (session %d)", sess.ID)

	// Send SERVER_CONFIG immediately after connection
	if err := s.sendServerConfig(sess); err != nil {
		// Debug log already shows the send attempt, just return on error
		return
	}

	// Automatically send AUTH_RESPONSE for SSH-authenticated users (V2 feature)
	if userID != nil {
		// Check if user is shadowbanned
		ban, err := s.db.GetActiveBanForUser(userID, &nickname)
		if err != nil {
			log.Printf("Session %d: failed to check ban status for SSH user %s (ID: %d): %v", sess.ID, nickname, *userID, err)
			// Continue - don't block on ban check failures
		}

		// Update session with user flags and shadowban status
		sess.mu.Lock()
		sess.UserFlags = userFlags
		sess.Shadowbanned = ban != nil && ban.Shadowban
		sess.mu.Unlock()

		if sess.Shadowbanned {
			debugLog.Printf("Session %d: SSH user %s (ID: %d) is shadowbanned", sess.ID, nickname, *userID)
		}

		flags := protocol.UserFlags(userFlags)
		authResp := &protocol.AuthResponseMessage{
			Success:   true,
			UserID:    uint64(*userID),
			Nickname:  nickname,
			Message:   fmt.Sprintf("Authenticated via SSH as %s", nickname),
			UserFlags: &flags,
		}
		if err := s.sendMessage(sess, protocol.TypeAuthResponse, authResp); err != nil {
			log.Printf("Failed to send AUTH_RESPONSE for SSH user %s: %v", nickname, err)
			return
		}
		debugLog.Printf("Session %d: Auto-authenticated SSH user %s (ID: %d)", sess.ID, nickname, *userID)
		s.sendServerPresenceSnapshot(sess)
		s.notifyServerPresence(sess, true)
	}

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

// sshChannelConn wraps ssh.Channel to implement net.Conn interface
type sshChannelConn struct {
	channel ssh.Channel
}

func (c *sshChannelConn) Read(b []byte) (int, error) {
	return c.channel.Read(b)
}

func (c *sshChannelConn) Write(b []byte) (int, error) {
	return c.channel.Write(b)
}

func (c *sshChannelConn) Close() error {
	return c.channel.Close()
}

func (c *sshChannelConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (c *sshChannelConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (c *sshChannelConn) SetDeadline(t time.Time) error      { return nil }
func (c *sshChannelConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *sshChannelConn) SetWriteDeadline(t time.Time) error { return nil }

// loadOrGenerateHostKey loads the SSH host key or generates one if it doesn't exist
func (s *Server) loadOrGenerateHostKey() (ssh.Signer, error) {
	// Expand ~ in path
	keyPath := s.config.SSHHostKeyPath
	if strings.HasPrefix(keyPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		keyPath = filepath.Join(homeDir, keyPath[2:])
	}

	if strings.TrimSpace(keyPath) == "" {
		configTarget := "server config file"
		if strings.TrimSpace(s.configPath) != "" {
			configTarget = s.configPath
		}
		return nil, fmt.Errorf("ssh host key path is empty; update [server].ssh_host_key in %s or remove it to use the default (%s)", configTarget, DefaultConfig().SSHHostKeyPath)
	}

	// Try to load existing key
	keyBytes, err := os.ReadFile(keyPath)
	if err == nil {
		// Parse the key
		key, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse host key: %w", err)
		}
		log.Printf("Loaded SSH host key from %s", keyPath)
		return key, nil
	}

	// Generate new key if file doesn't exist
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read host key: %w", err)
	}

	log.Printf("Generating new SSH host key at %s...", keyPath)

	// Generate RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Encode to PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Write key to file
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	if err := pem.Encode(keyFile, privateKeyPEM); err != nil {
		return nil, fmt.Errorf("failed to write key: %w", err)
	}

	// Parse the generated key
	key, err := ssh.ParsePrivateKey(pem.EncodeToMemory(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated key: %w", err)
	}

	log.Printf("Generated and saved new SSH host key")
	return key, nil
}

// authenticateSSHKey validates SSH public keys and auto-registers new users (V2 feature)
func (s *Server) authenticateSSHKey(conn ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
	// Compute fingerprint (SHA256 format like OpenSSH)
	fingerprint := ssh.FingerprintSHA256(pubKey)

	// Look up key in database
	sshKey, err := s.db.GetSSHKeyByFingerprint(fingerprint)
	if err == nil {
		// Known key - authenticate as existing user
		user, err := s.db.GetUserByID(sshKey.UserID)
		if err != nil {
			log.Printf("SSH auth failed: user not found for key %s", fingerprint)
			return nil, fmt.Errorf("user not found for SSH key")
		}

		// Update last used timestamp
		if err := s.db.UpdateSSHKeyLastUsed(fingerprint); err != nil {
			log.Printf("Failed to update SSH key last_used for %s: %v", fingerprint, err)
		}

		// Check if user is banned
		ban, err := s.db.GetActiveBanForUser(&user.ID, &user.Nickname)
		if err != nil {
			log.Printf("SSH auth: failed to check ban status for user %s (ID: %d): %v", user.Nickname, user.ID, err)
			// Continue with auth - don't block on ban check failures
		}

		if ban != nil && !ban.Shadowban {
			// Regular ban - reject SSH authentication
			bannedUntil := "permanently"
			if ban.BannedUntil != nil {
				bannedUntil = fmt.Sprintf("until %s", time.Unix(*ban.BannedUntil/1000, 0).Format(time.RFC3339))
			}
			log.Printf("SSH auth rejected: user %s (ID: %d) is banned %s. Reason: %s", user.Nickname, user.ID, bannedUntil, ban.Reason)
			return nil, fmt.Errorf("account banned %s", bannedUntil)
		}

		if ban != nil && ban.Shadowban {
			log.Printf("SSH auth: user %s (ID: %d) is shadowbanned", user.Nickname, user.ID)
			// Continue with auth - shadowban is enforced during message broadcasting
		}

		// Sync admin flag from config (config is source of truth)
		expectedAdmin := s.isAdminNickname(user.Nickname)
		hasAdmin := user.UserFlags&uint8(protocol.UserFlagAdmin) != 0
		if expectedAdmin != hasAdmin {
			if expectedAdmin {
				user.UserFlags |= uint8(protocol.UserFlagAdmin)
			} else {
				user.UserFlags &^= uint8(protocol.UserFlagAdmin)
			}
			if err := s.db.UpdateUserFlags(user.ID, user.UserFlags); err != nil {
				log.Printf("SSH auth: failed to sync admin flags for user %s: %v", user.Nickname, err)
			}
		}

		log.Printf("SSH auth: user %s (ID: %d, fingerprint: %s)", user.Nickname, user.ID, fingerprint)

		// Return permissions with user info
		return &ssh.Permissions{
			Extensions: map[string]string{
				"user_id":    fmt.Sprintf("%d", user.ID),
				"nickname":   user.Nickname,
				"user_flags": fmt.Sprintf("%d", user.UserFlags),
				"pubkey_fp":  fingerprint,
			},
		}, nil
	}

	// Unknown key - auto-register new user
	username := conn.User() // From ssh username@host
	if username == "" {
		username = "user" // Fallback
	}

	// Reject if nickname is already registered (prevent impersonation)
	existingUser, _ := s.db.GetUserByNickname(username)
	if existingUser != nil {
		log.Printf("SSH auto-register rejected: nickname %q already registered (user ID: %d)", username, existingUser.ID)
		return nil, fmt.Errorf("nickname %q is already registered; add your SSH key via the client", username)
	}

	// Check rate limiting (max 10 auto-registers per hour from same IP)
	if !s.checkAutoRegisterRateLimit(conn.RemoteAddr().String()) {
		log.Printf("SSH auto-register rate limit exceeded from %s", conn.RemoteAddr())
		return nil, fmt.Errorf("auto-registration rate limit exceeded")
	}

	// Create new user with random password (user can change later via CHANGE_PASSWORD)
	randomPassword := generateSecureRandomPassword(32)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash auto-register password: %v", err)
		return nil, fmt.Errorf("failed to hash password")
	}

	var userFlags uint8
	if s.isAdminNickname(username) {
		userFlags = uint8(protocol.UserFlagAdmin)
	}
	userID, err := s.db.CreateUser(username, string(hashedPassword), userFlags)
	if err != nil {
		log.Printf("Failed to auto-register user %s: %v", username, err)
		return nil, fmt.Errorf("failed to auto-register user: %w", err)
	}

	// Store SSH key
	newKey := &database.SSHKey{
		UserID:      userID,
		Fingerprint: fingerprint,
		PublicKey:   string(ssh.MarshalAuthorizedKey(pubKey)),
		KeyType:     pubKey.Type(),
		Label:       stringPtr("Auto-registered"),
		AddedAt:     time.Now().UnixMilli(),
	}
	if err := s.db.CreateSSHKey(newKey); err != nil {
		// Rollback user creation would be ideal, but challenging with current DB structure
		// User will exist but have no keys - they can still register via password
		log.Printf("Failed to store SSH key for user %s (ID: %d): %v", username, userID, err)
		return nil, fmt.Errorf("failed to store SSH key: %w", err)
	}

	log.Printf("Auto-registered new user %s (ID: %d) via SSH (fingerprint: %s)", username, userID, fingerprint)

	return &ssh.Permissions{
		Extensions: map[string]string{
			"user_id":    fmt.Sprintf("%d", userID),
			"nickname":   username,
			"user_flags": fmt.Sprintf("%d", userFlags),
			"pubkey_fp":  fingerprint,
		},
	}, nil
}

// generateSecureRandomPassword generates a cryptographically secure random password
func generateSecureRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic(err) // Crypto rand failure is unrecoverable
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

// checkAutoRegisterRateLimit checks if auto-registration is allowed from this IP
func (s *Server) checkAutoRegisterRateLimit(remoteAddr string) bool {
	host := strings.TrimSpace(remoteAddr)
	if host == "" {
		return true
	}

	if parsedIP := net.ParseIP(host); parsedIP == nil {
		if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
			host = h
		}
	}

	ip := net.ParseIP(host)
	if ip == nil {
		log.Printf("SSH auto-register: unable to parse remote address %q; skipping rate limit", remoteAddr)
		return true
	}

	now := time.Now()
	cutoff := now.Add(-autoRegisterWindow)

	s.autoRegisterMu.Lock()
	defer s.autoRegisterMu.Unlock()

	attempts := s.autoRegisterAttempts[ip.String()]
	pruned := attempts[:0]
	for _, ts := range attempts {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}

	if len(pruned) >= maxAutoRegistrationsPerHour {
		s.autoRegisterAttempts[ip.String()] = pruned
		return false
	}

	pruned = append(pruned, now)
	s.autoRegisterAttempts[ip.String()] = pruned

	return true
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}
