package protocol

import (
	"bytes"
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// TestFrameRoundTrip tests that any valid frame can be encoded and decoded
func TestFrameRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random frame components
		msgType := rapid.Byte().Draw(t, "type")
		// Mask out compression flag - compressed frames require valid LZ4 data
		// which we test separately in TestCompressionRoundTrip
		flags := rapid.Byte().Draw(t, "flags") &^ FlagCompressed
		payloadLen := rapid.IntRange(0, 1024).Draw(t, "payloadLen")
		payload := rapid.SliceOfN(rapid.Byte(), payloadLen, payloadLen).Draw(t, "payload")

		// Create frame
		original := &Frame{
			Version: ProtocolVersion,
			Type:    msgType,
			Flags:   flags,
			Payload: payload,
		}

		// Encode
		var buf bytes.Buffer
		err := EncodeFrame(&buf, original)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded, err := DecodeFrame(&buf)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if decoded.Version != original.Version {
			t.Fatalf("version mismatch: got %d, want %d", decoded.Version, original.Version)
		}
		if decoded.Type != original.Type {
			t.Fatalf("type mismatch: got %d, want %d", decoded.Type, original.Type)
		}
		if decoded.Flags != original.Flags {
			t.Fatalf("flags mismatch: got %d, want %d", decoded.Flags, original.Flags)
		}
		if !bytes.Equal(decoded.Payload, original.Payload) {
			t.Fatalf("payload mismatch")
		}
	})
}

// TestCompressionRoundTripRapid tests that any frame with compression enabled round-trips correctly
func TestCompressionRoundTripRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random frame components
		msgType := rapid.Byte().Draw(t, "type")
		// Generate other flags (but not compression - we handle that)
		otherFlags := rapid.Byte().Draw(t, "otherFlags") &^ FlagCompressed
		// Generate compressible payload (repeated pattern)
		patternLen := rapid.IntRange(1, 50).Draw(t, "patternLen")
		pattern := rapid.SliceOfN(rapid.Byte(), patternLen, patternLen).Draw(t, "pattern")
		repeatCount := rapid.IntRange(10, 100).Draw(t, "repeatCount")

		payload := bytes.Repeat(pattern, repeatCount)

		// Create frame
		original := &Frame{
			Version: ProtocolVersion,
			Type:    msgType,
			Flags:   otherFlags,
			Payload: payload,
		}

		// Encode with compression
		var buf bytes.Buffer
		err := EncodeFrame(&buf, original)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode (should auto-decompress)
		decoded, err := DecodeFrame(&buf)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if decoded.Version != original.Version {
			t.Fatalf("version mismatch: got %d, want %d", decoded.Version, original.Version)
		}
		if decoded.Type != original.Type {
			t.Fatalf("type mismatch: got %d, want %d", decoded.Type, original.Type)
		}
		// Other flags should be preserved, compression flag should be cleared after decompress
		if decoded.Flags != otherFlags {
			t.Fatalf("flags mismatch: got %d, want %d", decoded.Flags, otherFlags)
		}
		if !bytes.Equal(decoded.Payload, original.Payload) {
			t.Fatalf("payload mismatch: got %d bytes, want %d bytes", len(decoded.Payload), len(original.Payload))
		}
	})
}

// TestStringRoundTrip tests that any valid string can be encoded and decoded
func TestStringRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := rapid.String().Draw(t, "string")

		// Encode
		var buf bytes.Buffer
		err := WriteString(&buf, original)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded, err := ReadString(&buf)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if decoded != original {
			t.Fatalf("string mismatch: got %q, want %q", decoded, original)
		}
	})
}

// TestUint64RoundTrip tests that any uint64 can be encoded and decoded
func TestUint64RoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := rapid.Uint64().Draw(t, "uint64")

		// Encode
		var buf bytes.Buffer
		err := WriteUint64(&buf, original)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded, err := ReadUint64(&buf)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if decoded != original {
			t.Fatalf("uint64 mismatch: got %d, want %d", decoded, original)
		}
	})
}

// TestOptionalUint64RoundTrip tests optional uint64 encoding
func TestOptionalUint64RoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		hasValue := rapid.Bool().Draw(t, "hasValue")
		var original *uint64
		if hasValue {
			v := rapid.Uint64().Draw(t, "uint64")
			original = &v
		}

		// Encode
		var buf bytes.Buffer
		err := WriteOptionalUint64(&buf, original)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded, err := ReadOptionalUint64(&buf)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if original == nil && decoded != nil {
			t.Fatalf("expected nil, got %d", *decoded)
		}
		if original != nil && decoded == nil {
			t.Fatalf("expected %d, got nil", *original)
		}
		if original != nil && decoded != nil && *decoded != *original {
			t.Fatalf("uint64 mismatch: got %d, want %d", *decoded, *original)
		}
	})
}

// TestSetNicknameRoundTrip tests SetNicknameMessage encoding
func TestSetNicknameRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := &SetNicknameMessage{
			Nickname: rapid.StringMatching(`[a-zA-Z0-9_-]{3,20}`).Draw(t, "nickname"),
		}

		// Encode
		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded := &SetNicknameMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if decoded.Nickname != original.Nickname {
			t.Fatalf("nickname mismatch: got %q, want %q", decoded.Nickname, original.Nickname)
		}
	})
}

// TestPostMessageRoundTrip tests PostMessageMessage encoding
func TestPostMessageRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		hasParent := rapid.Bool().Draw(t, "hasParent")
		var parentID *uint64
		if hasParent {
			p := rapid.Uint64().Draw(t, "parentID")
			parentID = &p
		}

		original := &PostMessageMessage{
			ChannelID: rapid.Uint64().Draw(t, "channelID"),
			ParentID:  parentID,
			Content:   rapid.StringOfN(rapid.Rune(), 1, 4096, -1).Draw(t, "content"),
		}

		// Encode
		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded := &PostMessageMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if decoded.ChannelID != original.ChannelID {
			t.Fatalf("channelID mismatch: got %d, want %d", decoded.ChannelID, original.ChannelID)
		}
		if (decoded.ParentID == nil) != (original.ParentID == nil) {
			t.Fatalf("parentID presence mismatch")
		}
		if original.ParentID != nil && *decoded.ParentID != *original.ParentID {
			t.Fatalf("parentID mismatch: got %d, want %d", *decoded.ParentID, *original.ParentID)
		}
		if decoded.Content != original.Content {
			t.Fatalf("content mismatch")
		}
	})
}

// TestGetUserInfoRoundTrip tests GetUserInfoMessage encoding
func TestGetUserInfoRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nickname := rapid.StringN(3, 20, 256).Draw(t, "nickname")

		original := &GetUserInfoMessage{
			Nickname: nickname,
		}

		// Encode
		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded := &GetUserInfoMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if decoded.Nickname != original.Nickname {
			t.Fatalf("nickname mismatch: got %s, want %s", decoded.Nickname, original.Nickname)
		}
	})
}

// TestUserInfoRoundTrip tests UserInfoMessage encoding
func TestUserInfoRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nickname := rapid.StringN(1, 50, 256).Draw(t, "nickname")
		isRegistered := rapid.Bool().Draw(t, "is_registered")
		var userID *uint64
		if isRegistered {
			id := rapid.Uint64().Draw(t, "user_id")
			userID = &id
		}
		online := rapid.Bool().Draw(t, "online")

		original := &UserInfoMessage{
			Nickname:     nickname,
			IsRegistered: isRegistered,
			UserID:       userID,
			Online:       online,
		}

		// Encode
		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded := &UserInfoMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if decoded.Nickname != original.Nickname {
			t.Fatalf("nickname mismatch: got %s, want %s", decoded.Nickname, original.Nickname)
		}
		if decoded.IsRegistered != original.IsRegistered {
			t.Fatalf("is_registered mismatch: got %v, want %v", decoded.IsRegistered, original.IsRegistered)
		}
		if (decoded.UserID == nil) != (original.UserID == nil) {
			t.Fatalf("user_id presence mismatch")
		}
		if original.UserID != nil && *decoded.UserID != *original.UserID {
			t.Fatalf("user_id mismatch: got %d, want %d", *decoded.UserID, *original.UserID)
		}
		if decoded.Online != original.Online {
			t.Fatalf("online mismatch: got %v, want %v", decoded.Online, original.Online)
		}
	})
}

// TestListUsersRoundTrip tests ListUsersMessage encoding
func TestListUsersRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.Uint16().Draw(t, "limit")

		original := &ListUsersMessage{
			Limit: limit,
		}

		// Encode
		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded := &ListUsersMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if decoded.Limit != original.Limit {
			t.Fatalf("limit mismatch: got %d, want %d", decoded.Limit, original.Limit)
		}
	})
}

// TestUserListRoundTrip tests UserListMessage encoding
func TestUserListRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userCount := rapid.IntRange(0, 10).Draw(t, "user_count")
		users := make([]UserListEntry, userCount)

		for i := 0; i < userCount; i++ {
			nickname := rapid.StringN(1, 20, 256).Draw(t, "nickname")
			isRegistered := rapid.Bool().Draw(t, "is_registered")
			var userID *uint64
			if isRegistered {
				id := rapid.Uint64().Draw(t, "user_id")
				userID = &id
			}
			users[i] = UserListEntry{
				Nickname:     nickname,
				IsRegistered: isRegistered,
				UserID:       userID,
			}
		}

		original := &UserListMessage{
			Users: users,
		}

		// Encode
		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded := &UserListMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify round-trip
		if len(decoded.Users) != len(original.Users) {
			t.Fatalf("user count mismatch: got %d, want %d", len(decoded.Users), len(original.Users))
		}
		for i := range original.Users {
			if decoded.Users[i].Nickname != original.Users[i].Nickname {
				t.Fatalf("user[%d] nickname mismatch: got %s, want %s", i, decoded.Users[i].Nickname, original.Users[i].Nickname)
			}
			if decoded.Users[i].IsRegistered != original.Users[i].IsRegistered {
				t.Fatalf("user[%d] is_registered mismatch", i)
			}
			if (decoded.Users[i].UserID == nil) != (original.Users[i].UserID == nil) {
				t.Fatalf("user[%d] user_id presence mismatch", i)
			}
			if original.Users[i].UserID != nil && *decoded.Users[i].UserID != *original.Users[i].UserID {
				t.Fatalf("user[%d] user_id mismatch", i)
			}
		}
	})
}

// TestAuthRequestRoundTrip tests AuthRequestMessage encoding
func TestAuthRequestRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nickname := rapid.StringN(3, 20, 256).Draw(t, "nickname")
		password := rapid.StringN(1, 50, 256).Draw(t, "password")

		original := &AuthRequestMessage{
			Nickname: nickname,
			Password: password,
		}

		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &AuthRequestMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.Nickname != original.Nickname {
			t.Fatalf("nickname mismatch: got %s, want %s", decoded.Nickname, original.Nickname)
		}
		if decoded.Password != original.Password {
			t.Fatalf("password mismatch")
		}
	})
}

// TestAuthResponseRoundTrip tests AuthResponseMessage encoding
func TestAuthResponseRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		success := rapid.Bool().Draw(t, "success")
		var userID uint64
		if success {
			userID = rapid.Uint64().Draw(t, "user_id")
		}
		var nickname string
		if success {
			nickname = rapid.StringN(0, 32, 256).Draw(t, "nickname")
		}
		message := rapid.StringN(0, 100, 256).Draw(t, "message")
		var userFlags *UserFlags
		if success && rapid.Bool().Draw(t, "include_flags") {
			flags := UserFlags(rapid.Byte().Draw(t, "user_flags"))
			userFlags = &flags
		}

		original := &AuthResponseMessage{
			Success:   success,
			UserID:    userID,
			Nickname:  nickname,
			Message:   message,
			UserFlags: userFlags,
		}

		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &AuthResponseMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.Success != original.Success {
			t.Fatalf("success mismatch")
		}
		if decoded.Message != original.Message {
			t.Fatalf("message mismatch")
		}
		if success && decoded.UserID != original.UserID {
			t.Fatalf("user_id mismatch")
		}
		if success && decoded.Nickname != original.Nickname {
			t.Fatalf("nickname mismatch")
		}
		if success {
			if (decoded.UserFlags == nil) != (original.UserFlags == nil) {
				t.Fatalf("user_flags presence mismatch")
			}
			if decoded.UserFlags != nil && *decoded.UserFlags != *original.UserFlags {
				t.Fatalf("user_flags mismatch")
			}
		} else if decoded.UserFlags != nil {
			t.Fatalf("user_flags unexpectedly present on failure response")
		}
	})
}

// TestLeaveChannelRoundTrip tests LeaveChannelMessage encoding
func TestLeaveChannelRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		channelID := rapid.Uint64().Draw(t, "channel_id")
		var subchannelID *uint64
		if rapid.Bool().Draw(t, "has_subchannel") {
			id := rapid.Uint64().Draw(t, "subchannel_id")
			subchannelID = &id
		}

		original := &LeaveChannelMessage{
			ChannelID:    channelID,
			SubchannelID: subchannelID,
		}

		var buf bytes.Buffer
		if err := original.EncodeTo(&buf); err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &LeaveChannelMessage{}
		if err := decoded.Decode(buf.Bytes()); err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.ChannelID != original.ChannelID {
			t.Fatalf("channel_id mismatch")
		}
		if (decoded.SubchannelID == nil) != (original.SubchannelID == nil) {
			t.Fatalf("subchannel presence mismatch")
		}
		if decoded.SubchannelID != nil && *decoded.SubchannelID != *original.SubchannelID {
			t.Fatalf("subchannel mismatch")
		}
	})
}

// TestLeaveResponseRoundTrip tests LeaveResponseMessage encoding
func TestLeaveResponseRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		success := rapid.Bool().Draw(t, "success")
		channelID := rapid.Uint64().Draw(t, "channel_id")
		var subchannelID *uint64
		if rapid.Bool().Draw(t, "has_subchannel") {
			id := rapid.Uint64().Draw(t, "subchannel_id")
			subchannelID = &id
		}
		message := rapid.StringN(0, 64, 256).Draw(t, "message")

		original := &LeaveResponseMessage{
			Success:      success,
			ChannelID:    channelID,
			SubchannelID: subchannelID,
			Message:      message,
		}

		var buf bytes.Buffer
		if err := original.EncodeTo(&buf); err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &LeaveResponseMessage{}
		if err := decoded.Decode(buf.Bytes()); err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.Success != original.Success {
			t.Fatalf("success mismatch")
		}
		if decoded.ChannelID != original.ChannelID {
			t.Fatalf("channel mismatch")
		}
		if decoded.Message != original.Message {
			t.Fatalf("message mismatch")
		}
		if (decoded.SubchannelID == nil) != (original.SubchannelID == nil) {
			t.Fatalf("subchannel presence mismatch")
		}
		if decoded.SubchannelID != nil && *decoded.SubchannelID != *original.SubchannelID {
			t.Fatalf("subchannel mismatch")
		}
	})
}

// TestChannelUserListRoundTrip tests ChannelUserListMessage encoding
func TestChannelUserListRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		channelID := rapid.Uint64().Draw(t, "channel_id")
		var subchannelID *uint64
		if rapid.Bool().Draw(t, "has_subchannel") {
			id := rapid.Uint64().Draw(t, "subchannel_id")
			subchannelID = &id
		}
		userCount := rapid.IntRange(0, 5).Draw(t, "user_count")
		users := make([]ChannelUserEntry, userCount)
		for i := range users {
			users[i].SessionID = rapid.Uint64().Draw(t, fmt.Sprintf("session_id_%d", i))
			users[i].Nickname = rapid.StringN(1, 20, 256).Draw(t, fmt.Sprintf("nickname_%d", i))
			users[i].IsRegistered = rapid.Bool().Draw(t, fmt.Sprintf("is_registered_%d", i))
			if users[i].IsRegistered && rapid.Bool().Draw(t, fmt.Sprintf("has_user_id_%d", i)) {
				id := rapid.Uint64().Draw(t, fmt.Sprintf("user_id_%d", i))
				users[i].UserID = &id
			}
			users[i].UserFlags = UserFlags(rapid.Byte().Draw(t, fmt.Sprintf("user_flags_%d", i)))
		}

		original := &ChannelUserListMessage{
			ChannelID:    channelID,
			SubchannelID: subchannelID,
			Users:        users,
		}

		var buf bytes.Buffer
		if err := original.EncodeTo(&buf); err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &ChannelUserListMessage{}
		if err := decoded.Decode(buf.Bytes()); err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.ChannelID != original.ChannelID {
			t.Fatalf("channel mismatch")
		}
		if (decoded.SubchannelID == nil) != (original.SubchannelID == nil) {
			t.Fatalf("subchannel presence mismatch")
		}
		if decoded.SubchannelID != nil && *decoded.SubchannelID != *original.SubchannelID {
			t.Fatalf("subchannel mismatch")
		}
		if len(decoded.Users) != len(original.Users) {
			t.Fatalf("user len mismatch")
		}
		for i := range original.Users {
			ou := original.Users[i]
			du := decoded.Users[i]
			if du.SessionID != ou.SessionID {
				t.Fatalf("session mismatch")
			}
			if du.Nickname != ou.Nickname {
				t.Fatalf("nickname mismatch")
			}
			if du.IsRegistered != ou.IsRegistered {
				t.Fatalf("is_registered mismatch")
			}
			if (du.UserID == nil) != (ou.UserID == nil) {
				t.Fatalf("user_id presence mismatch")
			}
			if du.UserID != nil && *du.UserID != *ou.UserID {
				t.Fatalf("user_id mismatch")
			}
			if du.UserFlags != ou.UserFlags {
				t.Fatalf("user_flags mismatch")
			}
		}
	})
}

// TestChannelPresenceRoundTrip tests ChannelPresenceMessage encoding
func TestChannelPresenceRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		channelID := rapid.Uint64().Draw(t, "channel_id")
		var subchannelID *uint64
		if rapid.Bool().Draw(t, "has_subchannel") {
			id := rapid.Uint64().Draw(t, "subchannel_id")
			subchannelID = &id
		}
		sessionID := rapid.Uint64().Draw(t, "session_id")
		nickname := rapid.StringN(1, 20, 256).Draw(t, "nickname")
		isRegistered := rapid.Bool().Draw(t, "is_registered")
		var userID *uint64
		if isRegistered && rapid.Bool().Draw(t, "has_user_id") {
			id := rapid.Uint64().Draw(t, "user_id")
			userID = &id
		}
		userFlags := UserFlags(rapid.Byte().Draw(t, "user_flags"))
		joined := rapid.Bool().Draw(t, "joined")

		original := &ChannelPresenceMessage{
			ChannelID:    channelID,
			SubchannelID: subchannelID,
			SessionID:    sessionID,
			Nickname:     nickname,
			IsRegistered: isRegistered,
			UserID:       userID,
			UserFlags:    userFlags,
			Joined:       joined,
		}

		var buf bytes.Buffer
		if err := original.EncodeTo(&buf); err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &ChannelPresenceMessage{}
		if err := decoded.Decode(buf.Bytes()); err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.ChannelID != original.ChannelID {
			t.Fatalf("channel mismatch")
		}
		if (decoded.SubchannelID == nil) != (original.SubchannelID == nil) {
			t.Fatalf("subchannel presence mismatch")
		}
		if decoded.SubchannelID != nil && *decoded.SubchannelID != *original.SubchannelID {
			t.Fatalf("subchannel mismatch")
		}
		if decoded.SessionID != original.SessionID {
			t.Fatalf("session mismatch")
		}
		if decoded.Nickname != original.Nickname {
			t.Fatalf("nickname mismatch")
		}
		if decoded.IsRegistered != original.IsRegistered {
			t.Fatalf("is_registered mismatch")
		}
		if (decoded.UserID == nil) != (original.UserID == nil) {
			t.Fatalf("user_id presence mismatch")
		}
		if decoded.UserID != nil && *decoded.UserID != *original.UserID {
			t.Fatalf("user_id mismatch")
		}
		if decoded.UserFlags != original.UserFlags {
			t.Fatalf("user_flags mismatch")
		}
		if decoded.Joined != original.Joined {
			t.Fatalf("joined mismatch")
		}
	})
}

// TestServerPresenceRoundTrip tests ServerPresenceMessage encoding
func TestServerPresenceRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sessionID := rapid.Uint64().Draw(t, "session_id")
		nickname := rapid.StringN(1, 20, 256).Draw(t, "nickname")
		isRegistered := rapid.Bool().Draw(t, "is_registered")
		var userID *uint64
		if isRegistered && rapid.Bool().Draw(t, "has_user_id") {
			id := rapid.Uint64().Draw(t, "user_id")
			userID = &id
		}
		userFlags := UserFlags(rapid.Byte().Draw(t, "user_flags"))
		online := rapid.Bool().Draw(t, "online")

		original := &ServerPresenceMessage{
			SessionID:    sessionID,
			Nickname:     nickname,
			IsRegistered: isRegistered,
			UserID:       userID,
			UserFlags:    userFlags,
			Online:       online,
		}

		var buf bytes.Buffer
		if err := original.EncodeTo(&buf); err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &ServerPresenceMessage{}
		if err := decoded.Decode(buf.Bytes()); err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.SessionID != original.SessionID {
			t.Fatalf("session mismatch")
		}
		if decoded.Nickname != original.Nickname {
			t.Fatalf("nickname mismatch")
		}
		if decoded.IsRegistered != original.IsRegistered {
			t.Fatalf("is_registered mismatch")
		}
		if (decoded.UserID == nil) != (original.UserID == nil) {
			t.Fatalf("user_id presence mismatch")
		}
		if decoded.UserID != nil && *decoded.UserID != *original.UserID {
			t.Fatalf("user_id mismatch")
		}
		if decoded.UserFlags != original.UserFlags {
			t.Fatalf("user_flags mismatch")
		}
		if decoded.Online != original.Online {
			t.Fatalf("online mismatch")
		}
	})
}

// TestDMParticipantLeftRoundTrip tests DMParticipantLeftMessage encoding
func TestDMParticipantLeftRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dmChannelID := rapid.Uint64().Draw(t, "dm_channel_id")
		nickname := rapid.StringN(1, 20, 256).Draw(t, "nickname")
		var userID *uint64
		if rapid.Bool().Draw(t, "has_user_id") {
			id := rapid.Uint64().Draw(t, "user_id")
			userID = &id
		}

		original := &DMParticipantLeftMessage{
			DMChannelID: dmChannelID,
			UserID:      userID,
			Nickname:    nickname,
		}

		var buf bytes.Buffer
		if err := original.EncodeTo(&buf); err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &DMParticipantLeftMessage{}
		if err := decoded.Decode(buf.Bytes()); err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.DMChannelID != original.DMChannelID {
			t.Fatalf("dm_channel_id mismatch")
		}
		if (decoded.UserID == nil) != (original.UserID == nil) {
			t.Fatalf("user_id presence mismatch")
		}
		if decoded.UserID != nil && *decoded.UserID != *original.UserID {
			t.Fatalf("user_id mismatch")
		}
		if decoded.Nickname != original.Nickname {
			t.Fatalf("nickname mismatch")
		}
	})
}

// TestRegisterUserRoundTrip tests RegisterUserMessage encoding
func TestRegisterUserRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		password := rapid.StringN(1, 50, 256).Draw(t, "password")

		original := &RegisterUserMessage{
			Password: password,
		}

		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &RegisterUserMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.Password != original.Password {
			t.Fatalf("password mismatch")
		}
	})
}

// TestCreateChannelRoundTrip tests CreateChannelMessage encoding
func TestCreateChannelRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		name := rapid.StringN(1, 20, 256).Draw(t, "name")
		displayName := rapid.StringN(1, 30, 256).Draw(t, "display_name")
		var description *string
		if rapid.Bool().Draw(t, "has_description") {
			desc := rapid.StringN(0, 100, 256).Draw(t, "description")
			description = &desc
		}
		channelType := rapid.Uint8().Draw(t, "channel_type")
		retentionHours := rapid.Uint32().Draw(t, "retention_hours")

		original := &CreateChannelMessage{
			Name:           name,
			DisplayName:    displayName,
			Description:    description,
			ChannelType:    channelType,
			RetentionHours: retentionHours,
		}

		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &CreateChannelMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.Name != original.Name {
			t.Fatalf("name mismatch")
		}
		if decoded.DisplayName != original.DisplayName {
			t.Fatalf("display_name mismatch")
		}
		if (decoded.Description == nil) != (original.Description == nil) {
			t.Fatalf("description presence mismatch")
		}
		if original.Description != nil && *decoded.Description != *original.Description {
			t.Fatalf("description value mismatch")
		}
		if decoded.ChannelType != original.ChannelType {
			t.Fatalf("channel_type mismatch")
		}
		if decoded.RetentionHours != original.RetentionHours {
			t.Fatalf("retention_hours mismatch")
		}
	})
}

// TestChannelCreatedRoundTrip tests ChannelCreatedMessage encoding
func TestChannelCreatedRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		success := rapid.Bool().Draw(t, "success")
		var channelID uint64
		var name string
		var description string
		var channelType uint8
		var retentionHours uint32
		if success {
			channelID = rapid.Uint64().Draw(t, "channel_id")
			name = rapid.StringN(1, 20, 256).Draw(t, "name")
			description = rapid.StringN(0, 100, 256).Draw(t, "description")
			channelType = rapid.Uint8().Draw(t, "type")
			retentionHours = rapid.Uint32().Draw(t, "retention_hours")
		}
		message := rapid.StringN(0, 100, 256).Draw(t, "message")

		original := &ChannelCreatedMessage{
			Success:        success,
			ChannelID:      channelID,
			Name:           name,
			Description:    description,
			Type:           channelType,
			RetentionHours: retentionHours,
			Message:        message,
		}

		var buf bytes.Buffer
		err := original.EncodeTo(&buf)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		decoded := &ChannelCreatedMessage{}
		err = decoded.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if decoded.Success != original.Success {
			t.Fatalf("success mismatch")
		}
		if decoded.Message != original.Message {
			t.Fatalf("message mismatch")
		}
		if success {
			if decoded.ChannelID != original.ChannelID {
				t.Fatalf("channel_id mismatch")
			}
			if decoded.Name != original.Name {
				t.Fatalf("name mismatch")
			}
			if decoded.Description != original.Description {
				t.Fatalf("description mismatch")
			}
			if decoded.Type != original.Type {
				t.Fatalf("type mismatch")
			}
			if decoded.RetentionHours != original.RetentionHours {
				t.Fatalf("retention_hours mismatch")
			}
		}
	})
}
