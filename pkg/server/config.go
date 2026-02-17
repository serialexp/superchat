package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// TOMLConfig represents the structure of the server config file
type TOMLConfig struct {
	Server    ServerSection    `toml:"server"`
	Limits    LimitsSection    `toml:"limits"`
	Retention RetentionSection `toml:"retention"`
	Channels  ChannelsSection  `toml:"channels"`
	Discovery DiscoverySection `toml:"discovery"`
}

type ServerSection struct {
	TCPPort       int      `toml:"tcp_port"`
	SSHPort       int      `toml:"ssh_port"`
	HTTPPort      int      `toml:"http_port"`
	SSHHostKey    string   `toml:"ssh_host_key"`
	DatabasePath  string   `toml:"database_path"`
	AdminUsers    []string `toml:"admin_users"`
	AdminPassword string   `toml:"admin_password"`
}

type LimitsSection struct {
	MaxConnectionsPerIP     int `toml:"max_connections_per_ip"`
	MessageRateLimit        int `toml:"message_rate_limit"`
	MaxChannelCreates       int `toml:"max_channel_creates"`
	MaxMessageLength        int `toml:"max_message_length"`
	MaxNicknameLength       int `toml:"max_nickname_length"`
	SessionTimeoutSeconds   int `toml:"session_timeout_seconds"`
	MaxThreadSubscriptions  int `toml:"max_thread_subscriptions"`
	MaxChannelSubscriptions int `toml:"max_channel_subscriptions"`
}

type RetentionSection struct {
	DefaultRetentionHours  int `toml:"default_retention_hours"`
	CleanupIntervalMinutes int `toml:"cleanup_interval_minutes"`
}

type ChannelsSection struct {
	SeedChannels []SeedChannel `toml:"seed_channels"`
}

type SeedChannel struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

type DiscoverySection struct {
	DirectoryEnabled bool   `toml:"directory_enabled"`
	PublicHostname   string `toml:"public_hostname"`
	ServerName       string `toml:"server_name"`
	ServerDescription string `toml:"server_description"`
	MaxUsers         int    `toml:"max_users"`
}

// DefaultTOMLConfig returns the default TOML configuration
func DefaultTOMLConfig() TOMLConfig {
	return TOMLConfig{
		Server: ServerSection{
			TCPPort:      6465,
			SSHPort:      6466,
			HTTPPort:     8080,
			SSHHostKey:   "~/.superchat/ssh_host_key",
			DatabasePath: "~/.superchat/superchat.db",
		},
		Limits: LimitsSection{
			MaxConnectionsPerIP:     10,
			MessageRateLimit:        10,
			MaxChannelCreates:       5,
			MaxMessageLength:        4096,
			MaxNicknameLength:       20,
			SessionTimeoutSeconds:   120,
			MaxThreadSubscriptions:  50,
			MaxChannelSubscriptions: 10,
		},
		Retention: RetentionSection{
			DefaultRetentionHours:  168, // 7 days
			CleanupIntervalMinutes: 60,
		},
		Channels: ChannelsSection{
			SeedChannels: []SeedChannel{
				{Name: "general", Description: "General discussion"},
				{Name: "tech", Description: "Technical topics"},
				{Name: "random", Description: "Off-topic chat"},
				{Name: "feedback", Description: "Bug reports and feature requests"},
			},
		},
		Discovery: DiscoverySection{
			DirectoryEnabled: true,
			PublicHostname:   "", // Auto-detect if empty
			ServerName:       "SuperChat Server",
			ServerDescription: "A SuperChat community server",
			MaxUsers:         0, // 0 = unlimited
		},
	}
}

// LoadConfig loads configuration from a TOML file, creates default if not found,
// and applies environment variable overrides
func LoadConfig(path string) (TOMLConfig, error) {
	// Expand ~ in path
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return TOMLConfig{}, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// File doesn't exist, create default config
		config := DefaultTOMLConfig()
		if err := writeDefaultConfig(path, config); err != nil {
			// If we can't write, just return defaults without error
			// (might be a permissions issue, but we can still run)
			config = applyEnvOverrides(config)
			return config, nil
		}
		config = applyEnvOverrides(config)
		return config, nil
	}

	// Load from file
	var config TOMLConfig
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return TOMLConfig{}, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply environment variable overrides
	config = applyEnvOverrides(config)

	return config, nil
}

// applyEnvOverrides applies environment variable overrides to the config
// Environment variables follow the pattern: SUPERCHAT_SECTION_KEY
// Example: SUPERCHAT_SERVER_TCP_PORT=8080
func applyEnvOverrides(config TOMLConfig) TOMLConfig {
	// Server section
	if val := os.Getenv("SUPERCHAT_SERVER_TCP_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			config.Server.TCPPort = port
		}
	}
	if val := os.Getenv("SUPERCHAT_SERVER_SSH_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			config.Server.SSHPort = port
		}
	}
	if val := os.Getenv("SUPERCHAT_SERVER_SSH_HOST_KEY"); val != "" {
		config.Server.SSHHostKey = val
	}
	if val := os.Getenv("SUPERCHAT_SERVER_DATABASE_PATH"); val != "" {
		config.Server.DatabasePath = val
	}
	if val := os.Getenv("SUPERCHAT_SERVER_ADMIN_USERS"); val != "" {
		// Parse comma-separated list of admin nicknames
		adminUsers := strings.Split(val, ",")
		// Trim whitespace from each nickname
		for i, user := range adminUsers {
			adminUsers[i] = strings.TrimSpace(user)
		}
		config.Server.AdminUsers = adminUsers
	}
	if val := os.Getenv("SUPERCHAT_SERVER_ADMIN_PASSWORD"); val != "" {
		config.Server.AdminPassword = val
	}

	// Limits section
	if val := os.Getenv("SUPERCHAT_LIMITS_MAX_CONNECTIONS_PER_IP"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Limits.MaxConnectionsPerIP = limit
		}
	}
	if val := os.Getenv("SUPERCHAT_LIMITS_MESSAGE_RATE_LIMIT"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Limits.MessageRateLimit = limit
		}
	}
	if val := os.Getenv("SUPERCHAT_LIMITS_MAX_CHANNEL_CREATES"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Limits.MaxChannelCreates = limit
		}
	}
	if val := os.Getenv("SUPERCHAT_LIMITS_MAX_MESSAGE_LENGTH"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Limits.MaxMessageLength = limit
		}
	}
	if val := os.Getenv("SUPERCHAT_LIMITS_MAX_NICKNAME_LENGTH"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Limits.MaxNicknameLength = limit
		}
	}
	if val := os.Getenv("SUPERCHAT_LIMITS_SESSION_TIMEOUT_SECONDS"); val != "" {
		if timeout, err := strconv.Atoi(val); err == nil {
			config.Limits.SessionTimeoutSeconds = timeout
		}
	}
	if val := os.Getenv("SUPERCHAT_LIMITS_MAX_THREAD_SUBSCRIPTIONS"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Limits.MaxThreadSubscriptions = limit
		}
	}
	if val := os.Getenv("SUPERCHAT_LIMITS_MAX_CHANNEL_SUBSCRIPTIONS"); val != "" {
		if limit, err := strconv.Atoi(val); err == nil {
			config.Limits.MaxChannelSubscriptions = limit
		}
	}

	// Retention section
	if val := os.Getenv("SUPERCHAT_RETENTION_DEFAULT_RETENTION_HOURS"); val != "" {
		if hours, err := strconv.Atoi(val); err == nil {
			config.Retention.DefaultRetentionHours = hours
		}
	}
	if val := os.Getenv("SUPERCHAT_RETENTION_CLEANUP_INTERVAL_MINUTES"); val != "" {
		if minutes, err := strconv.Atoi(val); err == nil {
			config.Retention.CleanupIntervalMinutes = minutes
		}
	}

	// Discovery section
	if val := os.Getenv("SUPERCHAT_DISCOVERY_DIRECTORY_ENABLED"); val != "" {
		if enabled, err := strconv.ParseBool(val); err == nil {
			config.Discovery.DirectoryEnabled = enabled
		}
	}
	if val := os.Getenv("SUPERCHAT_DISCOVERY_PUBLIC_HOSTNAME"); val != "" {
		config.Discovery.PublicHostname = val
	}
	if val := os.Getenv("SUPERCHAT_DISCOVERY_SERVER_NAME"); val != "" {
		config.Discovery.ServerName = val
	}
	if val := os.Getenv("SUPERCHAT_DISCOVERY_SERVER_DESCRIPTION"); val != "" {
		config.Discovery.ServerDescription = val
	}
	if val := os.Getenv("SUPERCHAT_DISCOVERY_MAX_USERS"); val != "" {
		if maxUsers, err := strconv.Atoi(val); err == nil {
			config.Discovery.MaxUsers = maxUsers
		}
	}

	return config
}

// writeDefaultConfig writes the default config to a file with all options documented
func writeDefaultConfig(path string, config TOMLConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create file
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	// Build comprehensive config file manually
	// Active settings use defaults, commented settings show available options
	content := `# SuperChat Server Configuration
# This file was auto-generated with default values
# Settings below are active - modify them to change server behavior
# Commented settings show available options with their defaults
# Restart the server for changes to take effect
#
# Environment variables can override these settings:
# SUPERCHAT_SECTION_KEY (e.g., SUPERCHAT_SERVER_TCP_PORT=8080)

[server]
# Port for TCP connections
tcp_port = 6465

# Port for SSH connections
ssh_port = 6466

# Port for public HTTP server (/servers.json, /ws endpoints)
# Set to 0 to disable
http_port = 8080

# Path to SSH host key file
ssh_host_key = "~/.superchat/ssh_host_key"

# Path to SQLite database file
database_path = "~/.superchat/superchat.db"

# List of admin user nicknames (admins can ban users, delete channels, etc.)
# Uncomment and add nicknames to grant admin privileges:
# admin_users = ["alice", "bob"]

[limits]
# Maximum concurrent connections per IP address
max_connections_per_ip = 10

# Maximum messages per minute per user
message_rate_limit = 10

# Maximum channels a user can create per hour
max_channel_creates = 5

# Maximum message length in bytes
max_message_length = 4096

# Maximum nickname length in characters (default: 20)
# Uncomment to change:
# max_nickname_length = 20

# Session timeout in seconds (sessions idle longer than this are disconnected)
session_timeout_seconds = 120

# Maximum thread subscriptions per session (for real-time NEW_MESSAGE broadcasts)
# Uncomment to change from default (50):
# max_thread_subscriptions = 50

# Maximum channel subscriptions per session (for real-time NEW_MESSAGE broadcasts)
# Uncomment to change from default (10):
# max_channel_subscriptions = 10

[retention]
# Default message retention in hours (messages older than this are deleted)
default_retention_hours = 168  # 7 days

# How often to run cleanup job in minutes
# Uncomment to change from default (60 minutes):
# cleanup_interval_minutes = 60

[channels]
# Seed channels created on first startup if database is empty
# Modify this list to customize your server's default channels
seed_channels = [
  { name = "general", description = "General discussion" },
  { name = "tech", description = "Technical topics" },
  { name = "random", description = "Off-topic chat" },
  { name = "feedback", description = "Bug reports and feature requests" },
]

[discovery]
# Enable server directory mode (when enabled, server can register with directories
# and accept incoming server registrations)
directory_enabled = true

# Public hostname for client connections (leave empty to auto-detect)
# Uncomment and set your public IP/hostname:
# public_hostname = "chat.example.com"

# Display name shown in server lists
server_name = "SuperChat Server"

# Description shown in server lists
server_description = "A SuperChat community server"

# Maximum concurrent users (0 = unlimited)
# Uncomment to set a limit:
# max_users = 100
`

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// ToServerConfig converts TOMLConfig to ServerConfig
func (c *TOMLConfig) ToServerConfig() ServerConfig {
	cfg := DefaultConfig()

	if c.Server.TCPPort != 0 {
		cfg.TCPPort = c.Server.TCPPort
	}

	if c.Server.SSHPort != 0 {
		cfg.SSHPort = c.Server.SSHPort
	}

	if c.Server.HTTPPort != 0 {
		cfg.HTTPPort = c.Server.HTTPPort
	}

	if strings.TrimSpace(c.Server.SSHHostKey) != "" {
		cfg.SSHHostKeyPath = c.Server.SSHHostKey
	}

	if c.Limits.MaxConnectionsPerIP != 0 {
		cfg.MaxConnectionsPerIP = uint8(c.Limits.MaxConnectionsPerIP)
	}

	if c.Limits.MessageRateLimit != 0 {
		cfg.MessageRateLimit = uint16(c.Limits.MessageRateLimit)
	}

	if c.Limits.MaxChannelCreates != 0 {
		cfg.MaxChannelCreates = uint16(c.Limits.MaxChannelCreates)
	}

	if c.Limits.MaxMessageLength != 0 {
		cfg.MaxMessageLength = uint32(c.Limits.MaxMessageLength)
	}

	if c.Limits.SessionTimeoutSeconds != 0 {
		cfg.SessionTimeoutSeconds = c.Limits.SessionTimeoutSeconds
	}

	if c.Limits.MaxThreadSubscriptions != 0 {
		cfg.MaxThreadSubscriptions = uint16(c.Limits.MaxThreadSubscriptions)
	}

	if c.Limits.MaxChannelSubscriptions != 0 {
		cfg.MaxChannelSubscriptions = uint16(c.Limits.MaxChannelSubscriptions)
	}

	// Discovery section
	// Check if Discovery section exists in config file (vs missing in old configs)
	// If ServerName and ServerDescription are both empty, the section is likely missing
	// (defaults have non-empty values, so zero values indicate missing section)
	discoveryExists := c.Discovery.ServerName != "" || c.Discovery.ServerDescription != ""

	if discoveryExists {
		// Discovery section exists (or env vars set values), honor DirectoryEnabled
		cfg.DirectoryEnabled = c.Discovery.DirectoryEnabled
	}
	// Otherwise: Discovery section missing, keep DirectoryEnabled = true from DefaultConfig()

	if strings.TrimSpace(c.Discovery.PublicHostname) != "" {
		cfg.PublicHostname = c.Discovery.PublicHostname
	}

	if strings.TrimSpace(c.Discovery.ServerName) != "" {
		cfg.ServerName = c.Discovery.ServerName
	}

	if strings.TrimSpace(c.Discovery.ServerDescription) != "" {
		cfg.ServerDesc = c.Discovery.ServerDescription
	}

	if c.Discovery.MaxUsers != 0 {
		cfg.MaxUsers = uint32(c.Discovery.MaxUsers)
	}

	// Admin configuration
	if len(c.Server.AdminUsers) > 0 {
		cfg.AdminUsers = c.Server.AdminUsers
	}
	if c.Server.AdminPassword != "" {
		cfg.AdminPassword = c.Server.AdminPassword
	}

	return cfg
}

// GetDatabasePath returns the database path with ~ expanded
func (c *TOMLConfig) GetDatabasePath() (string, error) {
	path := c.Server.DatabasePath
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}
	return path, nil
}
