package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"github.com/pierrec/lz4/v4"
)

const (
	// MaxFrameSize is the maximum allowed frame size (1 MB)
	MaxFrameSize = 1024 * 1024

	// ProtocolVersion is the current protocol version
	// v1: Initial protocol
	// v2: Added LZ4 compression support (FlagCompressed)
	ProtocolVersion = 2

	// CompressionThreshold is the minimum payload size to consider compression (512 bytes)
	CompressionThreshold = 512
)

// Flag constants
const (
	FlagCompressed = 0x01 // Bit 0: compression
	FlagEncrypted  = 0x02 // Bit 1: encryption
)

var (
	ErrFrameTooLarge        = errors.New("frame exceeds maximum size (1 MB)")
	ErrInvalidVersion       = errors.New("invalid protocol version")
	ErrInvalidFrameLength   = errors.New("invalid frame length")
	ErrDecompressionFailed  = errors.New("decompression failed")
	ErrInvalidCompressedLen = errors.New("invalid compressed payload length")
)

// Frame represents a protocol frame
// Format: [Length (4 bytes)][Version (1 byte)][Type (1 byte)][Flags (1 byte)][Payload (N bytes)]
type Frame struct {
	Version uint8  // Protocol version (currently 1)
	Type    uint8  // Message type
	Flags   uint8  // Flags byte (compression, encryption, etc.)
	Payload []byte // Message payload
}

// CompressPayload compresses data using LZ4 and prepends the uncompressed size.
// Format: [Uncompressed Size (4 bytes, big-endian)][LZ4 Compressed Data]
// Returns the original data if compression doesn't reduce size.
func CompressPayload(data []byte) ([]byte, bool) {
	if len(data) == 0 {
		return data, false
	}

	// Allocate buffer for compressed data: 4 bytes for uncompressed size + compressed data
	maxCompressedSize := lz4.CompressBlockBound(len(data))
	compressed := make([]byte, 4+maxCompressedSize)

	// Write uncompressed size as first 4 bytes (big-endian)
	binary.BigEndian.PutUint32(compressed[:4], uint32(len(data)))

	// Compress the data
	n, err := lz4.CompressBlock(data, compressed[4:], nil)
	if err != nil || n == 0 {
		// Compression failed or data is incompressible
		return data, false
	}

	// Only use compression if it actually saves space
	compressedTotal := 4 + n
	if compressedTotal >= len(data) {
		return data, false
	}

	return compressed[:compressedTotal], true
}

// DecompressPayload decompresses LZ4-compressed data.
// Expects format: [Uncompressed Size (4 bytes, big-endian)][LZ4 Compressed Data]
func DecompressPayload(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, ErrInvalidCompressedLen
	}

	// Read uncompressed size from first 4 bytes
	uncompressedSize := binary.BigEndian.Uint32(data[:4])

	// Sanity check: don't allocate more than MaxFrameSize
	if uncompressedSize > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}

	// Allocate output buffer
	decompressed := make([]byte, uncompressedSize)

	// Decompress
	n, err := lz4.UncompressBlock(data[4:], decompressed)
	if err != nil {
		return nil, ErrDecompressionFailed
	}

	if n != int(uncompressedSize) {
		return nil, ErrDecompressionFailed
	}

	return decompressed, nil
}

// EncodeFrame writes a frame to the writer, automatically compressing
// payloads larger than CompressionThreshold if compression saves space.
//
// Optional peerVersion parameter controls compression:
//   - Not provided: compress if beneficial (for internal/test usage)
//   - v2 to ProtocolVersion: compress if beneficial (known to support LZ4)
//   - v1: never compress (lacks support)
//   - > ProtocolVersion: never compress (unknown future version)
//
// Example (assuming ProtocolVersion is 2):
//
//	EncodeFrame(w, frame)           // compress if beneficial (default)
//	EncodeFrame(w, frame, 2)        // compress if beneficial (peer is v2)
//	EncodeFrame(w, frame, 1)        // never compress (peer is v1)
//	EncodeFrame(w, frame, 3)        // never compress (unknown future version)
func EncodeFrame(w io.Writer, f *Frame, peerVersion ...uint8) error {
	payload := f.Payload
	flags := f.Flags

	// Determine if peer supports our compression format
	// - No peerVersion: assume compression OK (internal/test usage)
	// - v2 to ProtocolVersion: supports LZ4 compression
	// - v1: no compression (lacks support)
	// - > ProtocolVersion: no compression (unknown future version)
	peerSupportsCompression := true
	if len(peerVersion) > 0 {
		v := peerVersion[0]
		peerSupportsCompression = (v >= 2 && v <= ProtocolVersion)
	}

	// Auto-compress if payload is large enough, not already compressed, and peer supports it
	if peerSupportsCompression && len(payload) >= CompressionThreshold && flags&FlagCompressed == 0 {
		compressed, wasCompressed := CompressPayload(payload)
		if wasCompressed {
			payload = compressed
			flags |= FlagCompressed
		}
	}

	// Calculate length: Version (1) + Type (1) + Flags (1) + Payload (N)
	length := uint32(1 + 1 + 1 + len(payload))

	// Check max frame size (excluding the 4-byte length field itself)
	if length > MaxFrameSize {
		return ErrFrameTooLarge
	}

	// Write length (4 bytes, big-endian)
	if err := WriteUint32(w, length); err != nil {
		return err
	}

	// Write version (1 byte)
	if err := WriteUint8(w, f.Version); err != nil {
		return err
	}

	// Write type (1 byte)
	if err := WriteUint8(w, f.Type); err != nil {
		return err
	}

	// Write flags (1 byte)
	if err := WriteUint8(w, flags); err != nil {
		return err
	}

	// Write payload
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}

	// Flush if the writer supports it (e.g., *bufio.Writer or net.TCPConn)
	type flusher interface {
		Flush() error
	}
	if fl, ok := w.(flusher); ok {
		return fl.Flush()
	}

	return nil
}

// DecodeFrame reads a frame from the reader
func DecodeFrame(r io.Reader) (*Frame, error) {
	// Read length (4 bytes)
	length, err := ReadUint32(r)
	if err != nil {
		return nil, err
	}

	// Validate length
	if length > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}

	// Length must be at least 3 (version + type + flags)
	if length < 3 {
		return nil, ErrInvalidFrameLength
	}

	// Read version (1 byte)
	version, err := ReadUint8(r)
	if err != nil {
		return nil, err
	}

	// Read type (1 byte)
	msgType, err := ReadUint8(r)
	if err != nil {
		return nil, err
	}

	// Read flags (1 byte)
	flags, err := ReadUint8(r)
	if err != nil {
		return nil, err
	}

	// Read payload (remaining bytes)
	payloadLen := length - 3 // Subtract version, type, flags
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}

	// Handle decompression if FlagCompressed is set
	if flags&FlagCompressed != 0 && len(payload) > 0 {
		decompressed, err := DecompressPayload(payload)
		if err != nil {
			return nil, err
		}
		payload = decompressed
		// Clear compression flag since payload is now decompressed
		flags &^= FlagCompressed
	}

	return &Frame{
		Version: version,
		Type:    msgType,
		Flags:   flags,
		Payload: payload,
	}, nil
}

// EncodeMessage is a helper that encodes a message to a byte slice.
// Optional peerVersion parameter controls compression (see EncodeFrame).
func EncodeMessage(version, msgType uint8, flags uint8, payload []byte, peerVersion ...uint8) ([]byte, error) {
	frame := &Frame{
		Version: version,
		Type:    msgType,
		Flags:   flags,
		Payload: payload,
	}

	buf := new(bytes.Buffer)
	if err := EncodeFrame(buf, frame, peerVersion...); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DecodeMessage is a helper that decodes a frame from a byte slice
func DecodeMessage(data []byte) (*Frame, error) {
	buf := bytes.NewReader(data)
	return DecodeFrame(buf)
}
