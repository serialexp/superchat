package protocol

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test decode errors for all message types

func TestSetNicknameDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &SetNicknameMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - partial string", func(t *testing.T) {
		msg := &SetNicknameMessage{}
		// String length says 10 bytes but only provide 2
		payload := []byte{0x00, 0x0A, 0x41, 0x42}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestNicknameResponseDecodeErrors(t *testing.T) {
	t.Run("invalid payload - missing bool", func(t *testing.T) {
		msg := &NicknameResponseMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing message", func(t *testing.T) {
		msg := &NicknameResponseMessage{}
		err := msg.Decode([]byte{0x01}) // Bool but no message
		assert.Error(t, err)
	})
}

func TestListChannelsDecodeErrors(t *testing.T) {
	t.Run("invalid payload - missing fields", func(t *testing.T) {
		msg := &ListChannelsMessage{}
		err := msg.Decode([]byte{0x00, 0x00, 0x00}) // Partial uint64
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing limit", func(t *testing.T) {
		msg := &ListChannelsMessage{}
		payload := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01} // uint64 but no limit
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestChannelListDecodeErrors(t *testing.T) {
	t.Run("invalid payload - missing count", func(t *testing.T) {
		msg := &ChannelListMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete channel", func(t *testing.T) {
		msg := &ChannelListMessage{}
		// Count says 1 channel but data is incomplete
		payload := []byte{0x00, 0x01, 0x00, 0x00}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestJoinChannelDecodeErrors(t *testing.T) {
	t.Run("invalid payload - missing channel ID", func(t *testing.T) {
		msg := &JoinChannelMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing optional field", func(t *testing.T) {
		msg := &JoinChannelMessage{}
		// uint64 channel ID but missing optional subchannel ID
		payload := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestJoinResponseDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &JoinResponseMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing fields", func(t *testing.T) {
		msg := &JoinResponseMessage{}
		payload := []byte{0x01, 0x00, 0x00} // Bool + partial uint64
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestLeaveChannelDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &LeaveChannelMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})
}

func TestLeaveResponseDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &LeaveResponseMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing channel", func(t *testing.T) {
		msg := &LeaveResponseMessage{}
		payload := []byte{0x01} // success flag only
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestListMessagesDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &ListMessagesMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete", func(t *testing.T) {
		msg := &ListMessagesMessage{}
		payload := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00} // Partial
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestMessageListDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &MessageListMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete message", func(t *testing.T) {
		msg := &MessageListMessage{}
		buf := new(bytes.Buffer)
		WriteUint64(buf, 1)           // channel_id
		WriteOptionalUint64(buf, nil) // subchannel_id
		WriteOptionalUint64(buf, nil) // parent_id
		WriteUint16(buf, 1)           // message count = 1
		WriteUint64(buf, 1)           // message id
		// Missing rest of message fields

		err := msg.Decode(buf.Bytes())
		assert.Error(t, err)
	})
}

func TestPostMessageDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &PostMessageMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("decode empty content", func(t *testing.T) {
		msg := &PostMessageMessage{}
		buf := new(bytes.Buffer)
		WriteUint64(buf, 1)           // channel_id
		WriteOptionalUint64(buf, nil) // subchannel_id
		WriteOptionalUint64(buf, nil) // parent_id
		WriteString(buf, "")          // empty content

		err := msg.Decode(buf.Bytes())
		assert.Error(t, err)
		assert.Equal(t, ErrEmptyContent, err)
	})

	t.Run("decode too long content", func(t *testing.T) {
		msg := &PostMessageMessage{}
		buf := new(bytes.Buffer)
		WriteUint64(buf, 1)                          // channel_id
		WriteOptionalUint64(buf, nil)                // subchannel_id
		WriteOptionalUint64(buf, nil)                // parent_id
		WriteString(buf, string(make([]byte, 4097))) // too long

		err := msg.Decode(buf.Bytes())
		assert.Error(t, err)
		assert.Equal(t, ErrMessageTooLong, err)
	})
}

func TestMessagePostedDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &MessagePostedMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete", func(t *testing.T) {
		msg := &MessagePostedMessage{}
		payload := []byte{0x01, 0x00, 0x00} // Bool + partial uint64
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestDeleteMessageDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &DeleteMessageMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})
}

func TestMessageDeletedDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &MessageDeletedMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - success but missing timestamp", func(t *testing.T) {
		msg := &MessageDeletedMessage{}
		buf := new(bytes.Buffer)
		WriteBool(buf, true)  // success = true
		WriteUint64(buf, 123) // message_id
		// Missing timestamp (required when success=true)

		err := msg.Decode(buf.Bytes())
		assert.Error(t, err)
	})
}

func TestPingDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &PingMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})
}

func TestPongDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &PongMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})
}

func TestErrorMessageDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &ErrorMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing message", func(t *testing.T) {
		msg := &ErrorMessage{}
		payload := []byte{0x03, 0xE8} // error code but no message
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestServerConfigDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &ServerConfigMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete", func(t *testing.T) {
		msg := &ServerConfigMessage{}
		payload := []byte{0x01, 0x00, 0x0A} // Partial fields
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestNewMessageDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &NewMessageMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete", func(t *testing.T) {
		msg := &NewMessageMessage{}
		buf := new(bytes.Buffer)
		WriteUint64(buf, 1) // ID
		WriteUint64(buf, 1) // ChannelID
		// Missing rest of fields

		err := msg.Decode(buf.Bytes())
		assert.Error(t, err)
	})
}

// Test error paths in Write functions

func TestWriteStringError(t *testing.T) {
	t.Run("empty string writes zero length", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := WriteString(buf, "")
		assert.NoError(t, err)

		// Verify it wrote length of 0
		assert.Equal(t, []byte{0x00, 0x00}, buf.Bytes())
	})
}

func TestWriteOptionalError(t *testing.T) {
	t.Run("nil optional uint64", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := WriteOptionalUint64(buf, nil)
		assert.NoError(t, err)

		// Verify it wrote false
		assert.Equal(t, []byte{0x00}, buf.Bytes())
	})

	t.Run("nil optional timestamp", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := WriteOptionalTimestamp(buf, nil)
		assert.NoError(t, err)

		// Verify it wrote false
		assert.Equal(t, []byte{0x00}, buf.Bytes())
	})
}

func TestEncodeFrameError(t *testing.T) {
	t.Run("nil payload", func(t *testing.T) {
		frame := &Frame{
			Version: 1,
			Type:    TypePing,
			Flags:   0,
			Payload: nil,
		}

		buf := new(bytes.Buffer)
		err := EncodeFrame(buf, frame)
		assert.NoError(t, err)
	})
}

func TestEncodeMessageError(t *testing.T) {
	t.Run("oversized message", func(t *testing.T) {
		// Mark as already compressed to skip compression attempt
		_, err := EncodeMessage(1, TypePostMessage, FlagCompressed, make([]byte, MaxFrameSize))
		assert.Error(t, err)
		assert.Equal(t, ErrFrameTooLarge, err)
	})
}

func TestAuthRequestDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &AuthRequestMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing password", func(t *testing.T) {
		msg := &AuthRequestMessage{}
		// Nickname but no password
		payload := []byte{0x00, 0x05, 'a', 'l', 'i', 'c', 'e'}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestAuthResponseDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &AuthResponseMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing user_id when success", func(t *testing.T) {
		msg := &AuthResponseMessage{}
		// Success=true but no user_id
		payload := []byte{0x01}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing message", func(t *testing.T) {
		msg := &AuthResponseMessage{}
		// Success=false but no message
		payload := []byte{0x00}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestRegisterUserDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &RegisterUserMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - partial string", func(t *testing.T) {
		msg := &RegisterUserMessage{}
		// String length says 10 bytes but only provide 2
		payload := []byte{0x00, 0x0A, 0x41, 0x42}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestRegisterResponseDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &RegisterResponseMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing user_id when success", func(t *testing.T) {
		msg := &RegisterResponseMessage{}
		// Success=true but no user_id
		payload := []byte{0x01}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestCreateChannelDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &CreateChannelMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing display_name", func(t *testing.T) {
		msg := &CreateChannelMessage{}
		// Name but no display_name
		payload := []byte{0x00, 0x07, 'g', 'e', 'n', 'e', 'r', 'a', 'l'}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing optional description", func(t *testing.T) {
		msg := &CreateChannelMessage{}
		// Name + display_name but missing optional description
		payload := []byte{0x00, 0x07, 'g', 'e', 'n', 'e', 'r', 'a', 'l', 0x00, 0x08, '#', 'g', 'e', 'n', 'e', 'r', 'a', 'l'}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestChannelCreatedDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &ChannelCreatedMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - success=true but missing channel_id", func(t *testing.T) {
		msg := &ChannelCreatedMessage{}
		// Success=true but no channel_id
		payload := []byte{0x01}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})

	t.Run("invalid payload - success=false but missing message", func(t *testing.T) {
		msg := &ChannelCreatedMessage{}
		// Success=false but no message
		payload := []byte{0x00}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestGetUserInfoDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &GetUserInfoMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - partial string", func(t *testing.T) {
		msg := &GetUserInfoMessage{}
		// String length says 10 bytes but only provide 2
		payload := []byte{0x00, 0x0A, 0x41, 0x42}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestUserInfoDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &UserInfoMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing is_registered", func(t *testing.T) {
		msg := &UserInfoMessage{}
		// Nickname but no is_registered
		payload := []byte{0x00, 0x05, 'a', 'l', 'i', 'c', 'e'}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing user_id optional", func(t *testing.T) {
		msg := &UserInfoMessage{}
		// Nickname + is_registered but missing optional user_id
		payload := []byte{0x00, 0x05, 'a', 'l', 'i', 'c', 'e', 0x01}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing online", func(t *testing.T) {
		msg := &UserInfoMessage{}
		// Nickname + is_registered + optional(false) but missing online
		payload := []byte{0x00, 0x05, 'a', 'l', 'i', 'c', 'e', 0x01, 0x00}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestListUsersDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &ListUsersMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - partial limit", func(t *testing.T) {
		msg := &ListUsersMessage{}
		payload := []byte{0x00} // Only 1 byte of uint16
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestUserListDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &UserListMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete user count", func(t *testing.T) {
		msg := &UserListMessage{}
		payload := []byte{0x00} // Only 1 byte of uint16
		err := msg.Decode(payload)
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete user data", func(t *testing.T) {
		msg := &UserListMessage{}
		// Count says 1 user but data is incomplete
		payload := []byte{0x00, 0x01, 0x00, 0x05, 'a', 'l', 'i'}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing is_registered", func(t *testing.T) {
		msg := &UserListMessage{}
		// Count says 1 user, has nickname but missing is_registered
		payload := []byte{0x00, 0x01, 0x00, 0x05, 'a', 'l', 'i', 'c', 'e'}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing user_id optional", func(t *testing.T) {
		msg := &UserListMessage{}
		// Count says 1 user, has nickname + is_registered but missing optional user_id
		payload := []byte{0x00, 0x01, 0x00, 0x05, 'a', 'l', 'i', 'c', 'e', 0x01}
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestListChannelUsersDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &ListChannelUsersMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})
}

func TestChannelUserListDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &ChannelUserListMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - missing users", func(t *testing.T) {
		msg := &ChannelUserListMessage{}
		buf := new(bytes.Buffer)
		WriteUint64(buf, 1)           // channel id
		WriteOptionalUint64(buf, nil) // subchannel
		// missing user count
		err := msg.Decode(buf.Bytes())
		assert.Error(t, err)
	})
}

func TestChannelPresenceDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &ChannelPresenceMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete", func(t *testing.T) {
		msg := &ChannelPresenceMessage{}
		payload := []byte{0x00, 0x00} // partial channel id
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestServerPresenceDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &ServerPresenceMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete", func(t *testing.T) {
		msg := &ServerPresenceMessage{}
		payload := []byte{0x00, 0x00} // partial session id
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}

func TestDMParticipantLeftDecodeErrors(t *testing.T) {
	t.Run("invalid payload - empty", func(t *testing.T) {
		msg := &DMParticipantLeftMessage{}
		err := msg.Decode([]byte{})
		assert.Error(t, err)
	})

	t.Run("invalid payload - incomplete", func(t *testing.T) {
		msg := &DMParticipantLeftMessage{}
		payload := []byte{0x00, 0x00} // partial dm_channel_id
		err := msg.Decode(payload)
		assert.Error(t, err)
	})
}
