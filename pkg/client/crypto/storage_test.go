package crypto

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyStore_SaveAndLoadKey(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"
	userID := uint64(123)

	// Generate a key
	kp, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() error = %v", err)
	}

	// Save it
	err = ks.SaveKey(serverHost, userID, kp.PrivateKey[:])
	if err != nil {
		t.Fatalf("SaveKey() error = %v", err)
	}

	// Load it back
	loaded, err := ks.LoadKey(serverHost, userID)
	if err != nil {
		t.Fatalf("LoadKey() error = %v", err)
	}

	if !bytes.Equal(kp.PrivateKey[:], loaded) {
		t.Error("Loaded key doesn't match saved key")
	}
}

func TestKeyStore_HasKey(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"
	userID := uint64(456)

	// Initially no key
	if ks.HasKey(serverHost, userID) {
		t.Error("HasKey() should return false for non-existent key")
	}

	// Save a key
	kp, _ := GenerateX25519KeyPair()
	ks.SaveKey(serverHost, userID, kp.PrivateKey[:])

	// Now should exist
	if !ks.HasKey(serverHost, userID) {
		t.Error("HasKey() should return true after saving key")
	}
}

func TestKeyStore_DeleteKey(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"
	userID := uint64(789)

	// Save a key
	kp, _ := GenerateX25519KeyPair()
	ks.SaveKey(serverHost, userID, kp.PrivateKey[:])

	// Delete it
	err := ks.DeleteKey(serverHost, userID)
	if err != nil {
		t.Fatalf("DeleteKey() error = %v", err)
	}

	// Should no longer exist
	if ks.HasKey(serverHost, userID) {
		t.Error("HasKey() should return false after deleting key")
	}

	// Load should fail
	_, err = ks.LoadKey(serverHost, userID)
	if err == nil {
		t.Error("LoadKey() should fail after deleting key")
	}
}

func TestKeyStore_DeleteKey_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	// Deleting non-existent key should not error
	err := ks.DeleteKey("example.com", 999)
	if err != nil {
		t.Errorf("DeleteKey() for non-existent key error = %v", err)
	}
}

func TestKeyStore_LoadKey_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	_, err := ks.LoadKey("example.com", 999)
	if err != ErrKeyNotFound {
		t.Errorf("LoadKey() expected ErrKeyNotFound, got %v", err)
	}
}

func TestKeyStore_LoadOrGenerateKey_Existing(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"
	userID := uint64(100)

	// Generate and save a key first
	original, _ := GenerateX25519KeyPair()
	ks.SaveKey(serverHost, userID, original.PrivateKey[:])

	// LoadOrGenerateKey should load existing key
	kp, generated, err := ks.LoadOrGenerateKey(serverHost, userID)
	if err != nil {
		t.Fatalf("LoadOrGenerateKey() error = %v", err)
	}

	if generated {
		t.Error("LoadOrGenerateKey() should return generated=false for existing key")
	}

	if !bytes.Equal(original.PrivateKey[:], kp.PrivateKey[:]) {
		t.Error("LoadOrGenerateKey() returned different key than saved")
	}
}

func TestKeyStore_LoadOrGenerateKey_New(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"
	userID := uint64(200)

	// LoadOrGenerateKey should generate new key
	kp, generated, err := ks.LoadOrGenerateKey(serverHost, userID)
	if err != nil {
		t.Fatalf("LoadOrGenerateKey() error = %v", err)
	}

	if !generated {
		t.Error("LoadOrGenerateKey() should return generated=true for new key")
	}

	// Verify key was saved
	loaded, err := ks.LoadKey(serverHost, userID)
	if err != nil {
		t.Fatalf("LoadKey() error = %v", err)
	}

	if !bytes.Equal(kp.PrivateKey[:], loaded) {
		t.Error("Generated key was not saved correctly")
	}
}

func TestKeyStore_MultipleServers(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	userID := uint64(300)

	// Save keys for different servers
	kp1, _ := GenerateX25519KeyPair()
	kp2, _ := GenerateX25519KeyPair()

	ks.SaveKey("server1.com:6465", userID, kp1.PrivateKey[:])
	ks.SaveKey("server2.com:6465", userID, kp2.PrivateKey[:])

	// Load and verify they're different
	loaded1, _ := ks.LoadKey("server1.com:6465", userID)
	loaded2, _ := ks.LoadKey("server2.com:6465", userID)

	if bytes.Equal(loaded1, loaded2) {
		t.Error("Keys for different servers should be different")
	}
}

func TestKeyStore_MultipleUsers(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"

	// Save keys for different users
	kp1, _ := GenerateX25519KeyPair()
	kp2, _ := GenerateX25519KeyPair()

	ks.SaveKey(serverHost, 1, kp1.PrivateKey[:])
	ks.SaveKey(serverHost, 2, kp2.PrivateKey[:])

	// Load and verify they're different
	loaded1, _ := ks.LoadKey(serverHost, 1)
	loaded2, _ := ks.LoadKey(serverHost, 2)

	if bytes.Equal(loaded1, loaded2) {
		t.Error("Keys for different users should be different")
	}
}

func TestKeyStore_SaveKey_InvalidSize(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	tests := []struct {
		name    string
		keySize int
	}{
		{"too short", 16},
		{"too long", 64},
		{"empty", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keySize)
			err := ks.SaveKey("example.com", 1, key)
			if err == nil {
				t.Error("SaveKey() should fail with invalid key size")
			}
		})
	}
}

func TestKeyStore_SaveKey_InvalidUserID(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	kp, _ := GenerateX25519KeyPair()
	err := ks.SaveKey("example.com", 0, kp.PrivateKey[:])
	if err != ErrInvalidUserID {
		t.Errorf("SaveKey() expected ErrInvalidUserID, got %v", err)
	}
}

func TestKeyStore_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"
	userID := uint64(400)

	kp, _ := GenerateX25519KeyPair()
	ks.SaveKey(serverHost, userID, kp.PrivateKey[:])

	// Check file permissions
	path, _ := ks.keyFilePath(serverHost, userID)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	// On Unix, check permissions are 0600
	mode := info.Mode().Perm()
	if mode != KeyFileMode {
		t.Errorf("Key file permissions = %o, want %o", mode, KeyFileMode)
	}
}

func TestKeyStore_ListKeys(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	// Initially empty
	keys, err := ks.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("ListKeys() expected 0 keys, got %d", len(keys))
	}

	// Save some keys
	kp, _ := GenerateX25519KeyPair()
	ks.SaveKey("server1.com:6465", 1, kp.PrivateKey[:])
	ks.SaveKey("server2.com:6465", 2, kp.PrivateKey[:])

	keys, err = ks.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("ListKeys() expected 2 keys, got %d", len(keys))
	}
}

func TestKeyStore_GenerateAndSaveKey(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"
	userID := uint64(500)

	kp, err := ks.GenerateAndSaveKey(serverHost, userID)
	if err != nil {
		t.Fatalf("GenerateAndSaveKey() error = %v", err)
	}

	// Verify key was saved
	loaded, err := ks.LoadKey(serverHost, userID)
	if err != nil {
		t.Fatalf("LoadKey() error = %v", err)
	}

	if !bytes.Equal(kp.PrivateKey[:], loaded) {
		t.Error("Generated and saved key doesn't match loaded key")
	}
}

func TestSanitizeHostForFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"example.com:6465", "example.com_6465"},
		{"192.168.1.1:6465", "192.168.1.1_6465"},
		{"[::1]:6465", "[__1]_6465"},
		{"server/path", "server_path"},
		{"../attack", "__attack"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeHostForFilename(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeHostForFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestKeyStore_KeysDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"
	userID := uint64(700)

	kp, _ := GenerateX25519KeyPair()
	ks.SaveKey(serverHost, userID, kp.PrivateKey[:])

	// Check that keys directory was created
	keysDir := filepath.Join(tmpDir, KeysDirName)
	info, err := os.Stat(keysDir)
	if err != nil {
		t.Fatalf("Keys directory not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("Keys path is not a directory")
	}

	// Check directory permissions
	mode := info.Mode().Perm()
	if mode != KeyDirMode {
		t.Errorf("Keys directory permissions = %o, want %o", mode, KeyDirMode)
	}
}

func TestKeyStore_CorruptKeyFile(t *testing.T) {
	tmpDir := t.TempDir()
	ks := NewKeyStore(tmpDir)

	serverHost := "example.com:6465"
	userID := uint64(800)

	// Manually create a corrupt key file (wrong size)
	path, _ := ks.keyFilePath(serverHost, userID)
	os.MkdirAll(filepath.Dir(path), KeyDirMode)
	os.WriteFile(path, []byte("too short"), KeyFileMode)

	_, err := ks.LoadKey(serverHost, userID)
	if err == nil {
		t.Error("LoadKey() should fail for corrupt key file")
	}
}
