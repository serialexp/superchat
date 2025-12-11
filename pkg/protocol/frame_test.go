package protocol

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeFrame(t *testing.T) {
	tests := []struct {
		name    string
		frame   Frame
		wantErr bool
	}{
		{
			name: "valid frame - empty payload",
			frame: Frame{
				Version: 1,
				Type:    TypeSetNickname,
				Flags:   0,
				Payload: []byte{},
			},
			wantErr: false,
		},
		{
			name: "valid frame - with payload",
			frame: Frame{
				Version: 1,
				Type:    TypeSetNickname,
				Flags:   0,
				Payload: []byte("alice"),
			},
			wantErr: false,
		},
		{
			name: "encryption flag set",
			frame: Frame{
				Version: 1,
				Type:    TypePostMessage,
				Flags:   FlagEncrypted,
				Payload: []byte("encrypted data here"),
			},
			wantErr: false,
		},
		{
			name: "max payload size (1MB)",
			frame: Frame{
				Version: 1,
				Type:    TypePostMessage,
				Flags:   0,
				Payload: make([]byte, MaxFrameSize-3), // Subtract version, type, flags
			},
			wantErr: false,
		},
		{
			name: "oversized payload (should fail)",
			frame: Frame{
				Version: 1,
				Type:    TypePostMessage,
				Flags:   FlagCompressed, // Mark as already compressed to skip compression attempt
				Payload: make([]byte, MaxFrameSize), // Too large (exceeds with header)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			buf := new(bytes.Buffer)
			err := EncodeFrame(buf, &tt.frame)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, ErrFrameTooLarge, err)
				return
			}
			require.NoError(t, err)

			// Decode
			decoded, err := DecodeFrame(buf)
			require.NoError(t, err)

			// Verify round-trip
			assert.Equal(t, tt.frame.Version, decoded.Version)
			assert.Equal(t, tt.frame.Type, decoded.Type)
			assert.Equal(t, tt.frame.Flags, decoded.Flags)
			assert.Equal(t, tt.frame.Payload, decoded.Payload)
		})
	}
}

func TestDecodeFrameErrors(t *testing.T) {
	t.Run("empty buffer", func(t *testing.T) {
		buf := bytes.NewReader([]byte{})
		_, err := DecodeFrame(buf)
		assert.Error(t, err)
	})

	t.Run("oversized frame", func(t *testing.T) {
		// Length field indicates frame larger than MaxFrameSize
		buf := new(bytes.Buffer)
		WriteUint32(buf, MaxFrameSize+1)

		_, err := DecodeFrame(buf)
		assert.Error(t, err)
		assert.Equal(t, ErrFrameTooLarge, err)
	})

	t.Run("invalid frame length (too small)", func(t *testing.T) {
		// Length must be at least 3 (version + type + flags)
		buf := new(bytes.Buffer)
		WriteUint32(buf, 2) // Too small

		_, err := DecodeFrame(buf)
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidFrameLength, err)
	})

	t.Run("incomplete frame - missing version", func(t *testing.T) {
		buf := new(bytes.Buffer)
		WriteUint32(buf, 3) // Valid length
		// But no data follows

		_, err := DecodeFrame(buf)
		assert.Error(t, err)
	})

	t.Run("incomplete frame - missing type", func(t *testing.T) {
		buf := new(bytes.Buffer)
		WriteUint32(buf, 3)     // Valid length
		WriteUint8(buf, 1)      // Version
		// Type missing

		_, err := DecodeFrame(buf)
		assert.Error(t, err)
	})

	t.Run("incomplete frame - missing flags", func(t *testing.T) {
		buf := new(bytes.Buffer)
		WriteUint32(buf, 3)     // Valid length
		WriteUint8(buf, 1)      // Version
		WriteUint8(buf, 0x02)   // Type
		// Flags missing

		_, err := DecodeFrame(buf)
		assert.Error(t, err)
	})

	t.Run("incomplete frame - missing payload", func(t *testing.T) {
		buf := new(bytes.Buffer)
		WriteUint32(buf, 10)    // Length indicates 10 bytes (including 7 bytes of payload)
		WriteUint8(buf, 1)      // Version
		WriteUint8(buf, 0x02)   // Type
		WriteUint8(buf, 0)      // Flags
		buf.Write([]byte{0x01, 0x02}) // Only 2 bytes instead of 7

		_, err := DecodeFrame(buf)
		assert.Error(t, err)
	})
}

func TestEncodeMessage(t *testing.T) {
	payload := []byte("test payload")
	data, err := EncodeMessage(1, TypeSetNickname, 0, payload)
	require.NoError(t, err)

	// Decode it back
	frame, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, uint8(1), frame.Version)
	assert.Equal(t, uint8(TypeSetNickname), frame.Type)
	assert.Equal(t, uint8(0), frame.Flags)
	assert.Equal(t, payload, frame.Payload)
}

func TestFrameConstants(t *testing.T) {
	assert.Equal(t, 1024*1024, MaxFrameSize)
	assert.Equal(t, 2, ProtocolVersion) // v2 adds compression support
	assert.Equal(t, 0x01, FlagCompressed)
	assert.Equal(t, 0x02, FlagEncrypted)
}

func TestFrameStructure(t *testing.T) {
	t.Run("frame with all fields", func(t *testing.T) {
		frame := &Frame{
			Version: 1,
			Type:    TypePostMessage,
			Flags:   FlagCompressed,
			Payload: []byte("Hello, world!"),
		}

		buf := new(bytes.Buffer)
		err := EncodeFrame(buf, frame)
		require.NoError(t, err)

		// Check the binary structure manually
		data := buf.Bytes()

		// First 4 bytes: length (big-endian)
		length := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
		expectedLength := uint32(1 + 1 + 1 + len(frame.Payload)) // version + type + flags + payload
		assert.Equal(t, expectedLength, length)

		// Next byte: version
		assert.Equal(t, frame.Version, data[4])

		// Next byte: type
		assert.Equal(t, frame.Type, data[5])

		// Next byte: flags
		assert.Equal(t, frame.Flags, data[6])

		// Remaining bytes: payload
		assert.Equal(t, frame.Payload, data[7:])
	})
}

func TestZeroLengthPayload(t *testing.T) {
	frame := &Frame{
		Version: 1,
		Type:    TypeListChannels,
		Flags:   0,
		Payload: nil, // No payload
	}

	buf := new(bytes.Buffer)
	err := EncodeFrame(buf, frame)
	require.NoError(t, err)

	decoded, err := DecodeFrame(buf)
	require.NoError(t, err)

	assert.Equal(t, frame.Version, decoded.Version)
	assert.Equal(t, frame.Type, decoded.Type)
	assert.Equal(t, frame.Flags, decoded.Flags)
	assert.Equal(t, 0, len(decoded.Payload))
}

// Compression tests

func TestCompressPayload(t *testing.T) {
	tests := []struct {
		name           string
		input          []byte
		expectCompress bool
	}{
		{
			name:           "empty data",
			input:          []byte{},
			expectCompress: false,
		},
		{
			name:           "small data - no compression benefit",
			input:          []byte("hello"),
			expectCompress: false,
		},
		{
			name:           "highly compressible data",
			input:          bytes.Repeat([]byte("a"), 1000),
			expectCompress: true,
		},
		{
			name:           "random-like data - may not compress well",
			input:          make([]byte, 1000), // zeros compress well
			expectCompress: true,
		},
		{
			name:           "repeated pattern",
			input:          bytes.Repeat([]byte("hello world "), 100),
			expectCompress: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed, wasCompressed := CompressPayload(tt.input)

			if tt.expectCompress {
				assert.True(t, wasCompressed, "expected compression to succeed")
				assert.Less(t, len(compressed), len(tt.input), "compressed should be smaller")
			}

			// If compressed, verify we can decompress
			if wasCompressed {
				decompressed, err := DecompressPayload(compressed)
				require.NoError(t, err)
				assert.Equal(t, tt.input, decompressed)
			}
		})
	}
}

func TestDecompressPayload(t *testing.T) {
	t.Run("valid compressed data", func(t *testing.T) {
		original := bytes.Repeat([]byte("test data "), 100)
		compressed, ok := CompressPayload(original)
		require.True(t, ok, "should compress")

		decompressed, err := DecompressPayload(compressed)
		require.NoError(t, err)
		assert.Equal(t, original, decompressed)
	})

	t.Run("too short data", func(t *testing.T) {
		_, err := DecompressPayload([]byte{0x01, 0x02, 0x03})
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidCompressedLen, err)
	})

	t.Run("empty data", func(t *testing.T) {
		_, err := DecompressPayload([]byte{})
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidCompressedLen, err)
	})

	t.Run("invalid compressed data", func(t *testing.T) {
		// Valid length header but garbage compressed data
		data := []byte{0x00, 0x00, 0x00, 0x64, 0xFF, 0xFF, 0xFF} // claims 100 bytes uncompressed
		_, err := DecompressPayload(data)
		assert.Error(t, err)
		assert.Equal(t, ErrDecompressionFailed, err)
	})

	t.Run("size exceeds max frame size", func(t *testing.T) {
		// Claim uncompressed size > MaxFrameSize
		data := make([]byte, 8)
		data[0] = 0xFF
		data[1] = 0xFF
		data[2] = 0xFF
		data[3] = 0xFF // uint32 max = 4GB > MaxFrameSize
		_, err := DecompressPayload(data)
		assert.Error(t, err)
		assert.Equal(t, ErrFrameTooLarge, err)
	})
}

func TestEncodeFrameAutoCompression(t *testing.T) {
	t.Run("small payload - no compression", func(t *testing.T) {
		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: []byte("small"),
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame)
		require.NoError(t, err)

		decoded, err := DecodeFrame(&buf)
		require.NoError(t, err)

		assert.Equal(t, frame.Payload, decoded.Payload)
		assert.Equal(t, uint8(0), decoded.Flags&FlagCompressed)
	})

	t.Run("large compressible payload - compression applied", func(t *testing.T) {
		originalPayload := bytes.Repeat([]byte("compressible data "), 100) // 1800 bytes

		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: originalPayload,
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame)
		require.NoError(t, err)

		// Decode - DecodeFrame should auto-decompress
		decoded, err := DecodeFrame(&buf)
		require.NoError(t, err)

		// Payload should match original (decompressed)
		assert.Equal(t, originalPayload, decoded.Payload)
		// Flag should be cleared after decompression
		assert.Equal(t, uint8(0), decoded.Flags&FlagCompressed)
	})

	t.Run("already compressed flag set - no double compression", func(t *testing.T) {
		// Create already compressed data
		original := bytes.Repeat([]byte("test "), 200)
		compressed, ok := CompressPayload(original)
		require.True(t, ok)

		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   FlagCompressed,
			Payload: compressed,
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame)
		require.NoError(t, err)

		decoded, err := DecodeFrame(&buf)
		require.NoError(t, err)

		// Should get original data back
		assert.Equal(t, original, decoded.Payload)
	})

	t.Run("preserves other flags", func(t *testing.T) {
		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   FlagEncrypted,
			Payload: bytes.Repeat([]byte("data "), 200),
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame)
		require.NoError(t, err)

		decoded, err := DecodeFrame(&buf)
		require.NoError(t, err)

		// Encryption flag should be preserved
		assert.Equal(t, uint8(FlagEncrypted), decoded.Flags&FlagEncrypted)
	})
}

func TestCompressionRoundTrip(t *testing.T) {
	t.Run("large message round trip", func(t *testing.T) {
		// Simulate a large chat message
		content := "This is a long message that would benefit from compression. "
		content = content + content + content + content // Repeat to make it larger
		content = content + content + content + content // 16x original

		original := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: []byte(content),
		}

		// Encode with compression
		var buf bytes.Buffer
		err := EncodeFrame(&buf, original)
		require.NoError(t, err)

		// Decode
		decoded, err := DecodeFrame(&buf)
		require.NoError(t, err)

		// Verify content matches
		assert.Equal(t, original.Version, decoded.Version)
		assert.Equal(t, original.Type, decoded.Type)
		assert.Equal(t, string(original.Payload), string(decoded.Payload))
	})

	t.Run("incompressible data falls back gracefully", func(t *testing.T) {
		// Random-ish data that doesn't compress well
		payload := make([]byte, 600)
		for i := range payload {
			payload[i] = byte(i * 17) // pseudo-random pattern
		}

		original := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: payload,
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, original)
		require.NoError(t, err)

		decoded, err := DecodeFrame(&buf)
		require.NoError(t, err)

		assert.Equal(t, original.Payload, decoded.Payload)
	})
}

func TestCompressionThreshold(t *testing.T) {
	t.Run("just below threshold - no compression", func(t *testing.T) {
		payload := bytes.Repeat([]byte("a"), CompressionThreshold-1)

		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: payload,
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame)
		require.NoError(t, err)

		// Read raw frame to check if compression was applied
		rawBuf := buf.Bytes()
		// Flags byte is at offset 6 (4 length + 1 version + 1 type)
		assert.Equal(t, uint8(0), rawBuf[6]&FlagCompressed, "should not be compressed")
	})

	t.Run("at threshold - compression considered", func(t *testing.T) {
		payload := bytes.Repeat([]byte("a"), CompressionThreshold)

		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: payload,
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame)
		require.NoError(t, err)

		// Read raw frame to check if compression was applied
		rawBuf := buf.Bytes()
		// Highly compressible data at threshold should be compressed
		assert.Equal(t, uint8(FlagCompressed), rawBuf[6]&FlagCompressed, "should be compressed")
	})

	t.Run("threshold constant value", func(t *testing.T) {
		assert.Equal(t, 512, CompressionThreshold, "threshold should be 512 bytes as per spec")
	})
}

func TestVersionAwareCompression(t *testing.T) {
	t.Run("v1 peer - no compression", func(t *testing.T) {
		payload := bytes.Repeat([]byte("compressible data "), 100)

		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: payload,
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame, 1) // v1 peer
		require.NoError(t, err)

		// Check raw frame - flags should NOT have compression bit
		rawBuf := buf.Bytes()
		assert.Equal(t, uint8(0), rawBuf[6]&FlagCompressed, "v1 peer should not receive compressed frames")
	})

	t.Run("v2 peer - compression applied", func(t *testing.T) {
		payload := bytes.Repeat([]byte("compressible data "), 100)

		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: payload,
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame, 2) // v2 peer
		require.NoError(t, err)

		// Check raw frame - flags should have compression bit
		rawBuf := buf.Bytes()
		assert.Equal(t, uint8(FlagCompressed), rawBuf[6]&FlagCompressed, "v2 peer should receive compressed frames")
	})

	t.Run("no peer version - compression applied (default)", func(t *testing.T) {
		payload := bytes.Repeat([]byte("compressible data "), 100)

		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: payload,
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame) // no peer version specified
		require.NoError(t, err)

		// Check raw frame - flags should have compression bit (default behavior)
		rawBuf := buf.Bytes()
		assert.Equal(t, uint8(FlagCompressed), rawBuf[6]&FlagCompressed, "default should apply compression")
	})

	t.Run("v1 peer round-trip works without compression", func(t *testing.T) {
		originalPayload := bytes.Repeat([]byte("test data "), 100)

		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: originalPayload,
		}

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame, 1) // v1 peer - no compression
		require.NoError(t, err)

		decoded, err := DecodeFrame(&buf)
		require.NoError(t, err)

		assert.Equal(t, originalPayload, decoded.Payload)
		assert.Equal(t, uint8(0), decoded.Flags&FlagCompressed)
	})

	t.Run("future version peer (> ProtocolVersion) - no compression", func(t *testing.T) {
		payload := bytes.Repeat([]byte("compressible data "), 100)

		frame := &Frame{
			Version: ProtocolVersion,
			Type:    TypePostMessage,
			Flags:   0,
			Payload: payload,
		}

		// Use ProtocolVersion + 1 to test unknown future version
		futureVersion := uint8(ProtocolVersion + 1)

		var buf bytes.Buffer
		err := EncodeFrame(&buf, frame, futureVersion)
		require.NoError(t, err)

		// Check raw frame - flags should NOT have compression bit
		rawBuf := buf.Bytes()
		assert.Equal(t, uint8(0), rawBuf[6]&FlagCompressed, "unknown future version should not receive compressed frames")
	})
}
