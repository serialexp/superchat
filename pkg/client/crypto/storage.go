package crypto

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// KeysDirName is the subdirectory name for storing encryption keys
	KeysDirName = "keys"

	// KeyFileExtension is the extension for key files
	KeyFileExtension = ".x25519"

	// KeyFileMode is the file permission for key files (owner read/write only)
	KeyFileMode = 0600

	// KeyDirMode is the directory permission for the keys directory
	KeyDirMode = 0700
)

var (
	ErrKeyNotFound    = errors.New("encryption key not found")
	ErrKeyFileCorrupt = errors.New("key file is corrupt")
	ErrInvalidUserID  = errors.New("invalid user ID")
)

// KeyStore manages encryption key storage for password users.
// SSH users derive their keys from their SSH key and don't need storage.
type KeyStore struct {
	baseDir string // Base config directory (e.g., ~/.config/superchat-client)
}

// NewKeyStore creates a new KeyStore with the given base configuration directory.
func NewKeyStore(configDir string) *KeyStore {
	return &KeyStore{
		baseDir: configDir,
	}
}

// keysDir returns the path to the keys directory, creating it if necessary.
func (ks *KeyStore) keysDir() (string, error) {
	dir := filepath.Join(ks.baseDir, KeysDirName)
	if err := os.MkdirAll(dir, KeyDirMode); err != nil {
		return "", fmt.Errorf("failed to create keys directory: %w", err)
	}
	return dir, nil
}

// keyFilePath returns the path to a key file for the given server and user.
// Format: {keysDir}/{serverHost}_{userID}.x25519
func (ks *KeyStore) keyFilePath(serverHost string, userID uint64) (string, error) {
	if userID == 0 {
		return "", ErrInvalidUserID
	}

	dir, err := ks.keysDir()
	if err != nil {
		return "", err
	}

	// Sanitize server host for use in filename
	safeHost := sanitizeHostForFilename(serverHost)
	filename := fmt.Sprintf("%s_%d%s", safeHost, userID, KeyFileExtension)

	return filepath.Join(dir, filename), nil
}

// anonKeyFilePath returns the path to a key file for an anonymous user.
// Format: {keysDir}/{serverHost}_anon_{nickname}.x25519
func (ks *KeyStore) anonKeyFilePath(serverHost string, nickname string) (string, error) {
	if nickname == "" {
		return "", errors.New("nickname cannot be empty")
	}

	dir, err := ks.keysDir()
	if err != nil {
		return "", err
	}

	// Sanitize server host and nickname for use in filename
	safeHost := sanitizeHostForFilename(serverHost)
	safeNickname := sanitizeHostForFilename(nickname) // Reuse same sanitization
	filename := fmt.Sprintf("%s_anon_%s%s", safeHost, safeNickname, KeyFileExtension)

	return filepath.Join(dir, filename), nil
}

// sanitizeHostForFilename converts a server host to a safe filename component.
// Replaces : with _ and removes any path separators.
func sanitizeHostForFilename(host string) string {
	// Replace colons (from port) with underscores
	safe := strings.ReplaceAll(host, ":", "_")
	// Remove any path separators
	safe = strings.ReplaceAll(safe, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	// Remove any other potentially problematic characters
	safe = strings.ReplaceAll(safe, "..", "_")
	return safe
}

// SaveKey saves an X25519 private key for a user on a specific server.
// The key is stored securely with restrictive file permissions.
func (ks *KeyStore) SaveKey(serverHost string, userID uint64, privateKey []byte) error {
	if len(privateKey) != X25519KeySize {
		return fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidKeySize, X25519KeySize, len(privateKey))
	}

	path, err := ks.keyFilePath(serverHost, userID)
	if err != nil {
		return err
	}

	// Write atomically by writing to temp file first
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, privateKey, KeyFileMode); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	// Rename to final path (atomic on POSIX)
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to save key file: %w", err)
	}

	return nil
}

// LoadKey loads an X25519 private key for a user on a specific server.
func (ks *KeyStore) LoadKey(serverHost string, userID uint64) ([]byte, error) {
	path, err := ks.keyFilePath(serverHost, userID)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	if len(data) != X25519KeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", ErrKeyFileCorrupt, X25519KeySize, len(data))
	}

	return data, nil
}

// HasKey checks if a key exists for a user on a specific server.
func (ks *KeyStore) HasKey(serverHost string, userID uint64) bool {
	path, err := ks.keyFilePath(serverHost, userID)
	if err != nil {
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.Size() == X25519KeySize
}

// DeleteKey removes a stored key for a user on a specific server.
func (ks *KeyStore) DeleteKey(serverHost string, userID uint64) error {
	path, err := ks.keyFilePath(serverHost, userID)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete key file: %w", err)
	}

	return nil
}

// SaveAnonKey saves an X25519 private key for an anonymous user on a specific server.
// The key is identified by the user's nickname.
func (ks *KeyStore) SaveAnonKey(serverHost string, nickname string, privateKey []byte) error {
	if len(privateKey) != X25519KeySize {
		return fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidKeySize, X25519KeySize, len(privateKey))
	}

	path, err := ks.anonKeyFilePath(serverHost, nickname)
	if err != nil {
		return err
	}

	// Write atomically by writing to temp file first
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, privateKey, KeyFileMode); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	// Rename to final path (atomic on POSIX)
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to save key file: %w", err)
	}

	return nil
}

// LoadAnonKey loads an X25519 private key for an anonymous user on a specific server.
func (ks *KeyStore) LoadAnonKey(serverHost string, nickname string) ([]byte, error) {
	path, err := ks.anonKeyFilePath(serverHost, nickname)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	if len(data) != X25519KeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", ErrKeyFileCorrupt, X25519KeySize, len(data))
	}

	return data, nil
}

// HasAnonKey checks if a key exists for an anonymous user on a specific server.
func (ks *KeyStore) HasAnonKey(serverHost string, nickname string) bool {
	path, err := ks.anonKeyFilePath(serverHost, nickname)
	if err != nil {
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.Size() == X25519KeySize
}

// DeleteAnonKey removes a stored key for an anonymous user on a specific server.
func (ks *KeyStore) DeleteAnonKey(serverHost string, nickname string) error {
	path, err := ks.anonKeyFilePath(serverHost, nickname)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete key file: %w", err)
	}

	return nil
}

// LoadOrGenerateKey loads an existing key or generates a new one if not found.
// Returns the key pair (public and private) and whether the key was newly generated.
func (ks *KeyStore) LoadOrGenerateKey(serverHost string, userID uint64) (*X25519KeyPair, bool, error) {
	// Try to load existing key
	privateKey, err := ks.LoadKey(serverHost, userID)
	if err == nil {
		// Key exists, derive public key
		publicKey, err := X25519PrivateToPublic(privateKey)
		if err != nil {
			return nil, false, err
		}

		kp := &X25519KeyPair{}
		copy(kp.PrivateKey[:], privateKey)
		copy(kp.PublicKey[:], publicKey)
		return kp, false, nil
	}

	if !errors.Is(err, ErrKeyNotFound) {
		return nil, false, err
	}

	// Generate new key
	kp, err := GenerateX25519KeyPair()
	if err != nil {
		return nil, false, err
	}

	// Save it
	if err := ks.SaveKey(serverHost, userID, kp.PrivateKey[:]); err != nil {
		return nil, false, err
	}

	return kp, true, nil
}

// ListKeys returns all stored key files (for debugging/management).
func (ks *KeyStore) ListKeys() ([]string, error) {
	dir, err := ks.keysDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var keys []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), KeyFileExtension) {
			keys = append(keys, entry.Name())
		}
	}

	return keys, nil
}

// GenerateAndSaveKey generates a new key pair and saves it.
// This is a convenience function for first-time key setup.
func (ks *KeyStore) GenerateAndSaveKey(serverHost string, userID uint64) (*X25519KeyPair, error) {
	kp, err := GenerateX25519KeyPair()
	if err != nil {
		return nil, err
	}

	if err := ks.SaveKey(serverHost, userID, kp.PrivateKey[:]); err != nil {
		return nil, err
	}

	return kp, nil
}
