package client

import (
	"log"
	"testing"
)

// MockState implements StateInterface for testing
type MockStateForHelpers struct {
	connectionHistory map[string]string
}

func (m *MockStateForHelpers) GetLastSuccessfulMethod(serverAddress string) (string, error) {
	if method, ok := m.connectionHistory[serverAddress]; ok {
		return method, nil
	}
	return "", nil
}

// Implement other required StateInterface methods with stubs
func (m *MockStateForHelpers) GetConfig(key string) (string, error) { return "", nil }
func (m *MockStateForHelpers) SetConfig(key, value string) error { return nil }
func (m *MockStateForHelpers) GetLastNickname() string { return "" }
func (m *MockStateForHelpers) SetLastNickname(nickname string) error { return nil }
func (m *MockStateForHelpers) GetUserID() *uint64 { return nil }
func (m *MockStateForHelpers) SetUserID(userID *uint64) error { return nil }
func (m *MockStateForHelpers) GetReadState(channelID uint64, subchannelID *uint64, threadID *uint64) (int64, error) { return 0, nil }
func (m *MockStateForHelpers) UpdateReadState(channelID uint64, subchannelID *uint64, threadID *uint64, timestamp int64) error { return nil }
func (m *MockStateForHelpers) GetFirstRun() bool { return false }
func (m *MockStateForHelpers) SetFirstRunComplete() error { return nil }
func (m *MockStateForHelpers) SaveSuccessfulConnection(serverAddress string, method string) error { return nil }
func (m *MockStateForHelpers) GetStateDir() string { return "" }
func (m *MockStateForHelpers) GetFirstPostWarningDismissed() bool { return false }
func (m *MockStateForHelpers) SetFirstPostWarningDismissed() error { return nil }
func (m *MockStateForHelpers) GetLastSeenTimestamp() int64 { return 0 }
func (m *MockStateForHelpers) SetLastSeenTimestamp(timestamp int64) error { return nil }
func (m *MockStateForHelpers) UpdateLastSeenTimestamp() error { return nil }
func (m *MockStateForHelpers) Close() error { return nil }

func TestResolveConnectionMethod(t *testing.T) {
	tests := []struct {
		name              string
		address           string
		connectionHistory map[string]string
		expected          string
	}{
		{
			name:     "address with scheme unchanged",
			address:  "ssh://example.com",
			expected: "ssh://example.com",
		},
		{
			name:     "no connection history returns original",
			address:  "example.com:6465",
			expected: "example.com:6465",
		},
		{
			name:    "exact match for SSH",
			address: "example.com:6465",
			connectionHistory: map[string]string{
				"example.com:6465": "ssh",
			},
			expected: "ssh://example.com:6465",
		},
		{
			name:    "match with default port added",
			address: "example.com",
			connectionHistory: map[string]string{
				"example.com:6465": "tcp",
			},
			expected: "example.com",
		},
		{
			name:    "match with WebSocket",
			address: "example.com",
			connectionHistory: map[string]string{
				"example.com:8080": "ws",
			},
			expected: "ws://example.com",
		},
		{
			name:    "match with secure WebSocket",
			address: "example.com:8080",
			connectionHistory: map[string]string{
				"example.com:8080": "wss",
			},
			expected: "wss://example.com:8080",
		},
		{
			name:    "host extracted from address with port",
			address: "example.com:9999",
			connectionHistory: map[string]string{
				"example.com": "ssh",
			},
			expected: "ssh://example.com:9999",
		},
		{
			name:    "prefer exact match over variations",
			address: "example.com:6465",
			connectionHistory: map[string]string{
				"example.com:6465": "tcp",
				"example.com":      "ssh",
			},
			expected: "example.com:6465", // TCP returns without scheme
		},
		{
			name:    "websocket alias handled correctly",
			address: "example.com",
			connectionHistory: map[string]string{
				"example.com": "websocket",
			},
			expected: "ws://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &MockStateForHelpers{
				connectionHistory: tt.connectionHistory,
			}
			
			result := ResolveConnectionMethod(tt.address, state, nil)
			if result != tt.expected {
				t.Errorf("ResolveConnectionMethod(%q) = %q, want %q", tt.address, result, tt.expected)
			}
		})
	}
}

func TestBuildLookupAddresses(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		expected []string
	}{
		{
			name:    "address without port",
			address: "example.com",
			expected: []string{
				"example.com",
				"example.com:6465",
				"example.com:8080",
			},
		},
		{
			name:    "address with port 6465",
			address: "example.com:6465",
			expected: []string{
				"example.com:6465",
				"example.com",
				"example.com:8080",
			},
		},
		{
			name:    "address with port 8080",
			address: "example.com:8080",
			expected: []string{
				"example.com:8080",
				"example.com",
				"example.com:6465",
			},
		},
		{
			name:    "address with custom port",
			address: "example.com:9999",
			expected: []string{
				"example.com:9999",
				"example.com",
				"example.com:8080",
				"example.com:6465",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildLookupAddresses(tt.address)
			
			if len(result) != len(tt.expected) {
				t.Errorf("buildLookupAddresses(%q) returned %d addresses, want %d", 
					tt.address, len(result), len(tt.expected))
				t.Errorf("Got: %v", result)
				t.Errorf("Want: %v", tt.expected)
				return
			}
			
			for i, addr := range result {
				if addr != tt.expected[i] {
					t.Errorf("buildLookupAddresses(%q)[%d] = %q, want %q", 
						tt.address, i, addr, tt.expected[i])
				}
			}
		})
	}
}

func TestResolveConnectionMethodWithLogger(t *testing.T) {
	// Test that logger is used when provided
	var logOutput string
	logger := log.New(&testWriter{&logOutput}, "", 0)
	
	state := &MockStateForHelpers{
		connectionHistory: map[string]string{
			"example.com": "ssh",
		},
	}
	
	result := ResolveConnectionMethod("example.com", state, logger)
	
	if result != "ssh://example.com" {
		t.Errorf("Expected ssh://example.com, got %s", result)
	}
	
	if !contains(logOutput, "Found connection history") {
		t.Errorf("Expected log output to contain 'Found connection history', got: %s", logOutput)
	}
}

type testWriter struct {
	output *string
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	*w.output += string(p)
	return len(p), nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || 
		   len(s) > len(substr) && contains(s[1:], substr)
}
