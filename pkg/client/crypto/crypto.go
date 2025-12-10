// Package crypto provides end-to-end encryption for SuperChat DMs using
// X25519 key agreement and AES-256-GCM message encryption.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

const (
	// X25519KeySize is the size of X25519 public and private keys
	X25519KeySize = 32

	// AESKeySize is the size of AES-256 keys
	AESKeySize = 32

	// NonceSize is the size of AES-GCM nonces
	NonceSize = 12

	// TagSize is the size of AES-GCM authentication tags
	TagSize = 16

	// HKDFSalt is the salt used for HKDF key derivation
	HKDFSalt = "superchat-dm-v1"
)

var (
	ErrInvalidKeySize       = errors.New("invalid key size")
	ErrInvalidCiphertext    = errors.New("ciphertext too short")
	ErrDecryptionFailed     = errors.New("decryption failed: authentication error")
	ErrKeyGenerationFailed  = errors.New("key generation failed")
	ErrSharedSecretFailed   = errors.New("shared secret computation failed")
	ErrInvalidPublicKey     = errors.New("invalid public key")
	ErrInvalidEd25519Key    = errors.New("invalid Ed25519 private key")
)

// X25519KeyPair represents an X25519 key pair for DH key exchange
type X25519KeyPair struct {
	PublicKey  [X25519KeySize]byte
	PrivateKey [X25519KeySize]byte
}

// GenerateX25519KeyPair generates a new X25519 key pair for encryption.
// The private key is generated using crypto/rand and the public key
// is derived using curve25519.ScalarBaseMult.
func GenerateX25519KeyPair() (*X25519KeyPair, error) {
	var privateKey [X25519KeySize]byte
	if _, err := io.ReadFull(rand.Reader, privateKey[:]); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyGenerationFailed, err)
	}

	// Clamp the private key (standard X25519 clamping)
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	publicKey, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyGenerationFailed, err)
	}

	kp := &X25519KeyPair{}
	copy(kp.PrivateKey[:], privateKey[:])
	copy(kp.PublicKey[:], publicKey)

	return kp, nil
}

// Ed25519PrivateToX25519 converts an Ed25519 private key (seed) to an X25519 private key.
// This allows SSH Ed25519 keys to be used for encryption without storing a separate key.
//
// The conversion works because Ed25519 and X25519 use the same underlying curve
// (Curve25519), just in different representations:
// - Ed25519: twisted Edwards form (for signatures)
// - X25519: Montgomery form (for key exchange)
//
// The ed25519Seed should be the 32-byte seed (first half of the 64-byte Ed25519 private key).
func Ed25519PrivateToX25519(ed25519Seed []byte) ([]byte, error) {
	if len(ed25519Seed) != 32 {
		return nil, fmt.Errorf("%w: expected 32 bytes, got %d", ErrInvalidEd25519Key, len(ed25519Seed))
	}

	// Hash the seed to get the scalar (same as Ed25519 key derivation)
	h := sha512.Sum512(ed25519Seed)

	// Clamp the scalar (standard X25519 clamping)
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64

	// Return the first 32 bytes as the X25519 private key
	result := make([]byte, X25519KeySize)
	copy(result, h[:32])
	return result, nil
}

// X25519PrivateToPublic derives the X25519 public key from a private key.
func X25519PrivateToPublic(privateKey []byte) ([]byte, error) {
	if len(privateKey) != X25519KeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidKeySize, X25519KeySize, len(privateKey))
	}

	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyGenerationFailed, err)
	}

	return publicKey, nil
}

// ComputeSharedSecret performs X25519 Diffie-Hellman to compute a shared secret.
// Both parties will compute the same 32-byte shared secret independently.
func ComputeSharedSecret(myPrivateKey, theirPublicKey []byte) ([]byte, error) {
	if len(myPrivateKey) != X25519KeySize {
		return nil, fmt.Errorf("%w: private key must be %d bytes", ErrInvalidKeySize, X25519KeySize)
	}
	if len(theirPublicKey) != X25519KeySize {
		return nil, fmt.Errorf("%w: public key must be %d bytes", ErrInvalidKeySize, X25519KeySize)
	}

	// Check for low-order public key points (potential attack)
	if isLowOrderPoint(theirPublicKey) {
		return nil, ErrInvalidPublicKey
	}

	sharedSecret, err := curve25519.X25519(myPrivateKey, theirPublicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSharedSecretFailed, err)
	}

	return sharedSecret, nil
}

// DeriveChannelKey derives a unique AES-256 key for a specific DM channel
// using HKDF-SHA256. This ensures each channel has a unique encryption key
// even when the same two users create multiple DM channels.
func DeriveChannelKey(sharedSecret []byte, channelID uint64) ([]byte, error) {
	if len(sharedSecret) != X25519KeySize {
		return nil, fmt.Errorf("%w: shared secret must be %d bytes", ErrInvalidKeySize, X25519KeySize)
	}

	// Convert channelID to bytes for the info parameter
	channelIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(channelIDBytes, channelID)

	// Use HKDF to derive a 32-byte AES key
	hkdfReader := hkdf.New(sha512.New, sharedSecret, []byte(HKDFSalt), channelIDBytes)

	key := make([]byte, AESKeySize)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("HKDF key derivation failed: %w", err)
	}

	return key, nil
}

// EncryptMessage encrypts a plaintext message using AES-256-GCM.
// Returns: nonce (12 bytes) || ciphertext || tag (16 bytes)
func EncryptMessage(key, plaintext []byte) ([]byte, error) {
	if len(key) != AESKeySize {
		return nil, fmt.Errorf("%w: key must be %d bytes", ErrInvalidKeySize, AESKeySize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt: nonce || ciphertext || tag
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// DecryptMessage decrypts a ciphertext encrypted with EncryptMessage.
// Expects: nonce (12 bytes) || ciphertext || tag (16 bytes)
func DecryptMessage(key, ciphertext []byte) ([]byte, error) {
	if len(key) != AESKeySize {
		return nil, fmt.Errorf("%w: key must be %d bytes", ErrInvalidKeySize, AESKeySize)
	}

	if len(ciphertext) < NonceSize+TagSize {
		return nil, ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := ciphertext[:NonceSize]
	encrypted := ciphertext[NonceSize:]

	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// isLowOrderPoint checks if the public key is a low-order point.
// These are weak keys that should be rejected.
var lowOrderPoints = [][32]byte{
	// Point at infinity (all zeros)
	{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	// Order 2 point
	{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	// Order 4 points
	{0xe0, 0xeb, 0x7a, 0x7c, 0x3b, 0x41, 0xb8, 0xae, 0x16, 0x56, 0xe3, 0xfa, 0xf1, 0x9f, 0xc4, 0x6a, 0xda, 0x09, 0x8d, 0xeb, 0x9c, 0x32, 0xb1, 0xfd, 0x86, 0x62, 0x05, 0x16, 0x5f, 0x49, 0xb8, 0x00},
	{0x5f, 0x9c, 0x95, 0xbc, 0xa3, 0x50, 0x8c, 0x24, 0xb1, 0xd0, 0xb1, 0x55, 0x9c, 0x83, 0xef, 0x5b, 0x04, 0x44, 0x5c, 0xc4, 0x58, 0x1c, 0x8e, 0x86, 0xd8, 0x22, 0x4e, 0xdd, 0xd0, 0x9f, 0x11, 0x57},
	// Order 8 points
	{0xec, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
	{0xed, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
	{0xee, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
}

func isLowOrderPoint(key []byte) bool {
	if len(key) != 32 {
		return true // Invalid length, reject
	}

	var keyArray [32]byte
	copy(keyArray[:], key)

	for _, lowOrder := range lowOrderPoints {
		if keyArray == lowOrder {
			return true
		}
	}
	return false
}
