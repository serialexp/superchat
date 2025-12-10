package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestGenerateX25519KeyPair(t *testing.T) {
	kp1, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() error = %v", err)
	}

	if len(kp1.PublicKey) != X25519KeySize {
		t.Errorf("PublicKey size = %d, want %d", len(kp1.PublicKey), X25519KeySize)
	}
	if len(kp1.PrivateKey) != X25519KeySize {
		t.Errorf("PrivateKey size = %d, want %d", len(kp1.PrivateKey), X25519KeySize)
	}

	// Check that private key is clamped correctly
	if kp1.PrivateKey[0]&7 != 0 {
		t.Error("PrivateKey not correctly clamped (bottom 3 bits should be 0)")
	}
	if kp1.PrivateKey[31]&128 != 0 {
		t.Error("PrivateKey not correctly clamped (top bit should be 0)")
	}
	if kp1.PrivateKey[31]&64 == 0 {
		t.Error("PrivateKey not correctly clamped (second-to-top bit should be 1)")
	}

	// Generate second key pair and ensure they're different
	kp2, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() second call error = %v", err)
	}

	if kp1.PublicKey == kp2.PublicKey {
		t.Error("Two generated key pairs have identical public keys")
	}
	if kp1.PrivateKey == kp2.PrivateKey {
		t.Error("Two generated key pairs have identical private keys")
	}
}

func TestGenerateX25519KeyPair_PublicKeyDerivation(t *testing.T) {
	kp, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() error = %v", err)
	}

	// Verify public key can be re-derived from private key
	derivedPub, err := X25519PrivateToPublic(kp.PrivateKey[:])
	if err != nil {
		t.Fatalf("X25519PrivateToPublic() error = %v", err)
	}

	if !bytes.Equal(kp.PublicKey[:], derivedPub) {
		t.Error("Public key doesn't match re-derived public key")
	}
}

func TestEd25519PrivateToX25519(t *testing.T) {
	// Generate an Ed25519 key pair
	_, ed25519Priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey() error = %v", err)
	}

	// The seed is the first 32 bytes of the Ed25519 private key
	seed := ed25519Priv[:32]

	// Convert to X25519
	x25519Priv, err := Ed25519PrivateToX25519(seed)
	if err != nil {
		t.Fatalf("Ed25519PrivateToX25519() error = %v", err)
	}

	if len(x25519Priv) != X25519KeySize {
		t.Errorf("X25519 private key size = %d, want %d", len(x25519Priv), X25519KeySize)
	}

	// Derive public key and ensure it's valid
	x25519Pub, err := X25519PrivateToPublic(x25519Priv)
	if err != nil {
		t.Fatalf("X25519PrivateToPublic() error = %v", err)
	}

	if len(x25519Pub) != X25519KeySize {
		t.Errorf("X25519 public key size = %d, want %d", len(x25519Pub), X25519KeySize)
	}
}

func TestEd25519PrivateToX25519_InvalidSize(t *testing.T) {
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
			_, err := Ed25519PrivateToX25519(key)
			if err == nil {
				t.Error("Ed25519PrivateToX25519() expected error for invalid size")
			}
		})
	}
}

func TestEd25519PrivateToX25519_Deterministic(t *testing.T) {
	// Same Ed25519 seed should always produce same X25519 key
	seed := make([]byte, 32)
	rand.Read(seed)

	x1, err := Ed25519PrivateToX25519(seed)
	if err != nil {
		t.Fatalf("Ed25519PrivateToX25519() first call error = %v", err)
	}

	x2, err := Ed25519PrivateToX25519(seed)
	if err != nil {
		t.Fatalf("Ed25519PrivateToX25519() second call error = %v", err)
	}

	if !bytes.Equal(x1, x2) {
		t.Error("Ed25519PrivateToX25519() not deterministic")
	}
}

func TestComputeSharedSecret(t *testing.T) {
	// Generate two key pairs
	alice, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() alice error = %v", err)
	}

	bob, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() bob error = %v", err)
	}

	// Both should compute the same shared secret
	aliceSecret, err := ComputeSharedSecret(alice.PrivateKey[:], bob.PublicKey[:])
	if err != nil {
		t.Fatalf("ComputeSharedSecret() alice error = %v", err)
	}

	bobSecret, err := ComputeSharedSecret(bob.PrivateKey[:], alice.PublicKey[:])
	if err != nil {
		t.Fatalf("ComputeSharedSecret() bob error = %v", err)
	}

	if !bytes.Equal(aliceSecret, bobSecret) {
		t.Error("Alice and Bob computed different shared secrets")
	}

	if len(aliceSecret) != X25519KeySize {
		t.Errorf("Shared secret size = %d, want %d", len(aliceSecret), X25519KeySize)
	}
}

func TestComputeSharedSecret_InvalidKeySizes(t *testing.T) {
	validKey := make([]byte, X25519KeySize)
	rand.Read(validKey)

	tests := []struct {
		name       string
		privateKey []byte
		publicKey  []byte
	}{
		{"short private key", make([]byte, 16), validKey},
		{"long private key", make([]byte, 64), validKey},
		{"short public key", validKey, make([]byte, 16)},
		{"long public key", validKey, make([]byte, 64)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ComputeSharedSecret(tt.privateKey, tt.publicKey)
			if err == nil {
				t.Error("ComputeSharedSecret() expected error for invalid key size")
			}
		})
	}
}

func TestComputeSharedSecret_RejectsLowOrderPoints(t *testing.T) {
	kp, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() error = %v", err)
	}

	for i, lowOrder := range lowOrderPoints {
		_, err := ComputeSharedSecret(kp.PrivateKey[:], lowOrder[:])
		if err == nil {
			t.Errorf("ComputeSharedSecret() should reject low order point %d", i)
		}
	}
}

func TestDeriveChannelKey(t *testing.T) {
	sharedSecret := make([]byte, X25519KeySize)
	rand.Read(sharedSecret)

	key1, err := DeriveChannelKey(sharedSecret, 1)
	if err != nil {
		t.Fatalf("DeriveChannelKey() error = %v", err)
	}

	if len(key1) != AESKeySize {
		t.Errorf("Channel key size = %d, want %d", len(key1), AESKeySize)
	}

	// Same inputs should produce same key
	key1Again, err := DeriveChannelKey(sharedSecret, 1)
	if err != nil {
		t.Fatalf("DeriveChannelKey() second call error = %v", err)
	}
	if !bytes.Equal(key1, key1Again) {
		t.Error("DeriveChannelKey() not deterministic")
	}

	// Different channel IDs should produce different keys
	key2, err := DeriveChannelKey(sharedSecret, 2)
	if err != nil {
		t.Fatalf("DeriveChannelKey() channel 2 error = %v", err)
	}
	if bytes.Equal(key1, key2) {
		t.Error("Different channel IDs should produce different keys")
	}

	// Different shared secrets should produce different keys
	otherSecret := make([]byte, X25519KeySize)
	rand.Read(otherSecret)
	key3, err := DeriveChannelKey(otherSecret, 1)
	if err != nil {
		t.Fatalf("DeriveChannelKey() different secret error = %v", err)
	}
	if bytes.Equal(key1, key3) {
		t.Error("Different shared secrets should produce different keys")
	}
}

func TestDeriveChannelKey_InvalidSecretSize(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"too short", 16},
		{"too long", 64},
		{"empty", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := make([]byte, tt.size)
			_, err := DeriveChannelKey(secret, 1)
			if err == nil {
				t.Error("DeriveChannelKey() expected error for invalid secret size")
			}
		})
	}
}

func TestEncryptDecryptMessage(t *testing.T) {
	key := make([]byte, AESKeySize)
	rand.Read(key)

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"empty message", []byte{}},
		{"short message", []byte("hello")},
		{"unicode message", []byte("Hello, 世界! 🎉")},
		{"long message", bytes.Repeat([]byte("a"), 10000)},
		{"binary data", func() []byte {
			data := make([]byte, 256)
			for i := range data {
				data[i] = byte(i)
			}
			return data
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := EncryptMessage(key, tt.plaintext)
			if err != nil {
				t.Fatalf("EncryptMessage() error = %v", err)
			}

			// Ciphertext should be larger than plaintext (nonce + tag)
			if len(ciphertext) < len(tt.plaintext)+NonceSize+TagSize {
				t.Error("Ciphertext too short")
			}

			// Decrypt and verify
			decrypted, err := DecryptMessage(key, ciphertext)
			if err != nil {
				t.Fatalf("DecryptMessage() error = %v", err)
			}

			if !bytes.Equal(tt.plaintext, decrypted) {
				t.Error("Decrypted message doesn't match original")
			}
		})
	}
}

func TestEncryptMessage_UniqueNonces(t *testing.T) {
	key := make([]byte, AESKeySize)
	rand.Read(key)
	plaintext := []byte("test message")

	// Encrypt the same message multiple times
	nonces := make(map[string]bool)
	for i := 0; i < 100; i++ {
		ciphertext, err := EncryptMessage(key, plaintext)
		if err != nil {
			t.Fatalf("EncryptMessage() iteration %d error = %v", i, err)
		}

		// Extract nonce (first 12 bytes)
		nonce := string(ciphertext[:NonceSize])
		if nonces[nonce] {
			t.Error("Duplicate nonce detected")
		}
		nonces[nonce] = true
	}
}

func TestDecryptMessage_InvalidCiphertext(t *testing.T) {
	key := make([]byte, AESKeySize)
	rand.Read(key)

	tests := []struct {
		name       string
		ciphertext []byte
	}{
		{"empty", []byte{}},
		{"too short", make([]byte, NonceSize+TagSize-1)},
		{"just nonce", make([]byte, NonceSize)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecryptMessage(key, tt.ciphertext)
			if err == nil {
				t.Error("DecryptMessage() expected error for invalid ciphertext")
			}
		})
	}
}

func TestDecryptMessage_WrongKey(t *testing.T) {
	key1 := make([]byte, AESKeySize)
	key2 := make([]byte, AESKeySize)
	rand.Read(key1)
	rand.Read(key2)

	plaintext := []byte("secret message")
	ciphertext, err := EncryptMessage(key1, plaintext)
	if err != nil {
		t.Fatalf("EncryptMessage() error = %v", err)
	}

	_, err = DecryptMessage(key2, ciphertext)
	if err == nil {
		t.Error("DecryptMessage() should fail with wrong key")
	}
}

func TestDecryptMessage_TamperedCiphertext(t *testing.T) {
	key := make([]byte, AESKeySize)
	rand.Read(key)

	plaintext := []byte("secret message")
	ciphertext, err := EncryptMessage(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptMessage() error = %v", err)
	}

	// Tamper with ciphertext (flip a bit)
	tamperedCiphertext := make([]byte, len(ciphertext))
	copy(tamperedCiphertext, ciphertext)
	tamperedCiphertext[NonceSize+5] ^= 0xFF

	_, err = DecryptMessage(key, tamperedCiphertext)
	if err == nil {
		t.Error("DecryptMessage() should fail with tampered ciphertext")
	}
}

func TestDecryptMessage_TamperedTag(t *testing.T) {
	key := make([]byte, AESKeySize)
	rand.Read(key)

	plaintext := []byte("secret message")
	ciphertext, err := EncryptMessage(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptMessage() error = %v", err)
	}

	// Tamper with authentication tag (last 16 bytes)
	tamperedCiphertext := make([]byte, len(ciphertext))
	copy(tamperedCiphertext, ciphertext)
	tamperedCiphertext[len(tamperedCiphertext)-1] ^= 0xFF

	_, err = DecryptMessage(key, tamperedCiphertext)
	if err == nil {
		t.Error("DecryptMessage() should fail with tampered tag")
	}
}

func TestEncryptDecryptMessage_InvalidKeySize(t *testing.T) {
	plaintext := []byte("test")

	tests := []struct {
		name    string
		keySize int
	}{
		{"too short", 16},
		{"too long", 64},
		{"empty", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name+" encrypt", func(t *testing.T) {
			key := make([]byte, tt.keySize)
			_, err := EncryptMessage(key, plaintext)
			if err == nil {
				t.Error("EncryptMessage() expected error for invalid key size")
			}
		})

		t.Run(tt.name+" decrypt", func(t *testing.T) {
			key := make([]byte, tt.keySize)
			ciphertext := make([]byte, NonceSize+TagSize+10)
			_, err := DecryptMessage(key, ciphertext)
			if err == nil {
				t.Error("DecryptMessage() expected error for invalid key size")
			}
		})
	}
}

func TestX25519PrivateToPublic(t *testing.T) {
	kp, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() error = %v", err)
	}

	derivedPub, err := X25519PrivateToPublic(kp.PrivateKey[:])
	if err != nil {
		t.Fatalf("X25519PrivateToPublic() error = %v", err)
	}

	if !bytes.Equal(kp.PublicKey[:], derivedPub) {
		t.Error("Derived public key doesn't match generated public key")
	}
}

func TestX25519PrivateToPublic_InvalidSize(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"too short", 16},
		{"too long", 64},
		{"empty", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.size)
			_, err := X25519PrivateToPublic(key)
			if err == nil {
				t.Error("X25519PrivateToPublic() expected error for invalid key size")
			}
		})
	}
}

func TestIsLowOrderPoint(t *testing.T) {
	// All known low-order points should be detected
	for i, lowOrder := range lowOrderPoints {
		if !isLowOrderPoint(lowOrder[:]) {
			t.Errorf("Low order point %d not detected", i)
		}
	}

	// Valid points should not be flagged
	for i := 0; i < 10; i++ {
		kp, err := GenerateX25519KeyPair()
		if err != nil {
			t.Fatalf("GenerateX25519KeyPair() error = %v", err)
		}
		if isLowOrderPoint(kp.PublicKey[:]) {
			t.Error("Valid public key incorrectly flagged as low order point")
		}
	}

	// Invalid length should be flagged
	if !isLowOrderPoint(make([]byte, 16)) {
		t.Error("Invalid length key should be flagged")
	}
}

// TestFullDMFlow simulates a complete DM encryption flow between two users
func TestFullDMFlow(t *testing.T) {
	// Alice and Bob generate their key pairs
	alice, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() alice error = %v", err)
	}

	bob, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() bob error = %v", err)
	}

	// They exchange public keys and compute shared secrets
	aliceShared, err := ComputeSharedSecret(alice.PrivateKey[:], bob.PublicKey[:])
	if err != nil {
		t.Fatalf("ComputeSharedSecret() alice error = %v", err)
	}

	bobShared, err := ComputeSharedSecret(bob.PrivateKey[:], alice.PublicKey[:])
	if err != nil {
		t.Fatalf("ComputeSharedSecret() bob error = %v", err)
	}

	// Both derive the same channel key for channel ID 42
	channelID := uint64(42)
	aliceKey, err := DeriveChannelKey(aliceShared, channelID)
	if err != nil {
		t.Fatalf("DeriveChannelKey() alice error = %v", err)
	}

	bobKey, err := DeriveChannelKey(bobShared, channelID)
	if err != nil {
		t.Fatalf("DeriveChannelKey() bob error = %v", err)
	}

	if !bytes.Equal(aliceKey, bobKey) {
		t.Fatal("Alice and Bob derived different channel keys")
	}

	// Alice sends a message to Bob
	message := []byte("Hello Bob! This is a secret message.")
	ciphertext, err := EncryptMessage(aliceKey, message)
	if err != nil {
		t.Fatalf("EncryptMessage() error = %v", err)
	}

	// Bob decrypts it
	decrypted, err := DecryptMessage(bobKey, ciphertext)
	if err != nil {
		t.Fatalf("DecryptMessage() error = %v", err)
	}

	if !bytes.Equal(message, decrypted) {
		t.Error("Bob received a different message than Alice sent")
	}

	// Bob replies to Alice
	reply := []byte("Hi Alice! Got your message!")
	replyCiphertext, err := EncryptMessage(bobKey, reply)
	if err != nil {
		t.Fatalf("EncryptMessage() bob reply error = %v", err)
	}

	// Alice decrypts Bob's reply
	decryptedReply, err := DecryptMessage(aliceKey, replyCiphertext)
	if err != nil {
		t.Fatalf("DecryptMessage() alice error = %v", err)
	}

	if !bytes.Equal(reply, decryptedReply) {
		t.Error("Alice received a different reply than Bob sent")
	}
}

// TestSSHKeyIntegration tests the Ed25519 SSH key conversion flow
func TestSSHKeyIntegration(t *testing.T) {
	// Simulate Alice with SSH key and Bob with generated key
	_, aliceEd25519Priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey() error = %v", err)
	}

	// Alice converts her SSH key to X25519
	aliceX25519Priv, err := Ed25519PrivateToX25519(aliceEd25519Priv[:32])
	if err != nil {
		t.Fatalf("Ed25519PrivateToX25519() error = %v", err)
	}

	aliceX25519Pub, err := X25519PrivateToPublic(aliceX25519Priv)
	if err != nil {
		t.Fatalf("X25519PrivateToPublic() error = %v", err)
	}

	// Bob generates a new X25519 key pair
	bob, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair() error = %v", err)
	}

	// They compute shared secrets
	aliceShared, err := ComputeSharedSecret(aliceX25519Priv, bob.PublicKey[:])
	if err != nil {
		t.Fatalf("ComputeSharedSecret() alice error = %v", err)
	}

	bobShared, err := ComputeSharedSecret(bob.PrivateKey[:], aliceX25519Pub)
	if err != nil {
		t.Fatalf("ComputeSharedSecret() bob error = %v", err)
	}

	if !bytes.Equal(aliceShared, bobShared) {
		t.Fatal("Shared secrets don't match between SSH key and generated key")
	}

	// Verify they can exchange encrypted messages
	channelID := uint64(123)
	aliceKey, _ := DeriveChannelKey(aliceShared, channelID)
	bobKey, _ := DeriveChannelKey(bobShared, channelID)

	message := []byte("SSH key encryption works!")
	ciphertext, _ := EncryptMessage(aliceKey, message)
	decrypted, err := DecryptMessage(bobKey, ciphertext)
	if err != nil {
		t.Fatalf("DecryptMessage() error = %v", err)
	}

	if !bytes.Equal(message, decrypted) {
		t.Error("Message roundtrip failed with SSH key derived encryption")
	}
}

// Benchmarks
func BenchmarkGenerateX25519KeyPair(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GenerateX25519KeyPair()
	}
}

func BenchmarkComputeSharedSecret(b *testing.B) {
	alice, _ := GenerateX25519KeyPair()
	bob, _ := GenerateX25519KeyPair()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ComputeSharedSecret(alice.PrivateKey[:], bob.PublicKey[:])
	}
}

func BenchmarkDeriveChannelKey(b *testing.B) {
	sharedSecret := make([]byte, X25519KeySize)
	rand.Read(sharedSecret)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DeriveChannelKey(sharedSecret, uint64(i))
	}
}

func BenchmarkEncryptMessage(b *testing.B) {
	key := make([]byte, AESKeySize)
	rand.Read(key)
	plaintext := []byte("This is a typical chat message of moderate length.")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = EncryptMessage(key, plaintext)
	}
}

func BenchmarkDecryptMessage(b *testing.B) {
	key := make([]byte, AESKeySize)
	rand.Read(key)
	plaintext := []byte("This is a typical chat message of moderate length.")
	ciphertext, _ := EncryptMessage(key, plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecryptMessage(key, ciphertext)
	}
}
