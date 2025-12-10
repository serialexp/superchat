package protocol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthRequestMessage(t *testing.T) {
	tests := []struct {
		name     string
		nickname string
		password string
	}{
		{
			name:     "valid auth request",
			nickname: "alice",
			password: "secret123",
		},
		{
			name:     "long password",
			nickname: "bob",
			password: "verylongpassword12345678901234567890",
		},
		{
			name:     "short credentials",
			nickname: "abc",
			password: "pwd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &AuthRequestMessage{
				Nickname: tt.nickname,
				Password: tt.password,
			}

			payload, err := msg.Encode()
			require.NoError(t, err)

			decoded := &AuthRequestMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.nickname, decoded.Nickname)
			assert.Equal(t, tt.password, decoded.Password)
		})
	}
}

func TestAuthResponseMessage(t *testing.T) {
	makeFlags := func(f UserFlags) *UserFlags {
		return &f
	}
	tests := []struct {
		name string
		msg  AuthResponseMessage
	}{
		{
			name: "success response",
			msg: AuthResponseMessage{
				Success:  true,
				UserID:   42,
				Nickname: "alice",
				Message:  "Welcome back!",
			},
		},
		{
			name: "failure response",
			msg: AuthResponseMessage{
				Success: false,
				Message: "Invalid credentials",
			},
		},
		{
			name: "success with user ID 0",
			msg: AuthResponseMessage{
				Success:  true,
				UserID:   0,
				Nickname: "bob",
				Message:  "Authenticated",
			},
		},
		{
			name: "success with admin flag",
			msg: AuthResponseMessage{
				Success:   true,
				UserID:    99,
				Nickname:  "carol",
				Message:   "Admin access granted",
				UserFlags: makeFlags(UserFlagAdmin),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &AuthResponseMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.Message, decoded.Message)
			if tt.msg.Success {
				assert.Equal(t, tt.msg.UserID, decoded.UserID)
				assert.Equal(t, tt.msg.Nickname, decoded.Nickname)
				if tt.msg.UserFlags != nil {
					require.NotNil(t, decoded.UserFlags)
					assert.Equal(t, *tt.msg.UserFlags, *decoded.UserFlags)
				} else {
					assert.Nil(t, decoded.UserFlags)
				}
			} else {
				assert.Nil(t, decoded.UserFlags)
			}
		})
	}
}

func TestRegisterUserMessage(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{
			name:     "valid password",
			password: "password123",
		},
		{
			name:     "long password",
			password: "verylongpassword12345678901234567890",
		},
		{
			name:     "short password",
			password: "pwd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &RegisterUserMessage{
				Password: tt.password,
			}

			payload, err := msg.Encode()
			require.NoError(t, err)

			decoded := &RegisterUserMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.password, decoded.Password)
		})
	}
}

func TestRegisterResponseMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  RegisterResponseMessage
	}{
		{
			name: "success response",
			msg: RegisterResponseMessage{
				Success: true,
				UserID:  123,
				Message: "Registration successful",
			},
		},
		{
			name: "failure response",
			msg: RegisterResponseMessage{
				Success: false,
				Message: "Nickname already registered",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &RegisterResponseMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.Message, decoded.Message)
			if tt.msg.Success {
				assert.Equal(t, tt.msg.UserID, decoded.UserID)
			}
		})
	}
}

func TestCreateChannelMessage(t *testing.T) {
	desc1 := "A test channel"

	tests := []struct {
		name string
		msg  CreateChannelMessage
	}{
		{
			name: "with description",
			msg: CreateChannelMessage{
				Name:           "general",
				DisplayName:    "#general",
				Description:    &desc1,
				ChannelType:    1,
				RetentionHours: 168,
			},
		},
		{
			name: "without description",
			msg: CreateChannelMessage{
				Name:           "random",
				DisplayName:    "#random",
				Description:    nil,
				ChannelType:    1,
				RetentionHours: 720,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &CreateChannelMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Name, decoded.Name)
			assert.Equal(t, tt.msg.DisplayName, decoded.DisplayName)
			assert.Equal(t, tt.msg.ChannelType, decoded.ChannelType)
			assert.Equal(t, tt.msg.RetentionHours, decoded.RetentionHours)

			if tt.msg.Description == nil {
				assert.Nil(t, decoded.Description)
			} else {
				require.NotNil(t, decoded.Description)
				assert.Equal(t, *tt.msg.Description, *decoded.Description)
			}
		})
	}
}

func TestChannelCreatedMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  ChannelCreatedMessage
	}{
		{
			name: "success response",
			msg: ChannelCreatedMessage{
				Success:        true,
				ChannelID:      42,
				Name:           "general",
				Description:    "General discussion",
				Type:           1,
				RetentionHours: 168,
				Message:        "Channel created successfully",
			},
		},
		{
			name: "failure response",
			msg: ChannelCreatedMessage{
				Success: false,
				Message: "Insufficient permissions",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &ChannelCreatedMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.Message, decoded.Message)

			if tt.msg.Success {
				assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
				assert.Equal(t, tt.msg.Name, decoded.Name)
				assert.Equal(t, tt.msg.Description, decoded.Description)
				assert.Equal(t, tt.msg.Type, decoded.Type)
				assert.Equal(t, tt.msg.RetentionHours, decoded.RetentionHours)
			}
		})
	}
}

func TestSetNicknameMessage(t *testing.T) {
	tests := []struct {
		name     string
		nickname string
		wantErr  bool
		errType  error
	}{
		{"valid nickname", "alice", false, nil},
		{"min length (3)", "bob", false, nil},
		{"max length (20)", "12345678901234567890", false, nil},
		{"too short (2)", "ab", true, ErrNicknameTooShort},
		{"too long (21)", "123456789012345678901", true, ErrNicknameTooLong},
		{"empty", "", true, ErrNicknameTooShort},
		{"with hyphen", "alice-bob", false, nil},
		{"with underscore", "alice_123", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &SetNicknameMessage{Nickname: tt.nickname}

			// Test encode
			payload, err := msg.Encode()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errType, err)
				return
			}
			require.NoError(t, err)

			// Test decode
			decoded := &SetNicknameMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.nickname, decoded.Nickname)
		})
	}
}

func TestNicknameResponseMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  NicknameResponseMessage
	}{
		{
			name: "success response",
			msg: NicknameResponseMessage{
				Success: true,
				Message: "Nickname set successfully",
			},
		},
		{
			name: "failure response",
			msg: NicknameResponseMessage{
				Success: false,
				Message: "Nickname already taken",
			},
		},
		{
			name: "empty message",
			msg: NicknameResponseMessage{
				Success: true,
				Message: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &NicknameResponseMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.Message, decoded.Message)
		})
	}
}

func TestListChannelsMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  ListChannelsMessage
	}{
		{
			name: "from beginning",
			msg: ListChannelsMessage{
				FromChannelID: 0,
				Limit:         50,
			},
		},
		{
			name: "from offset",
			msg: ListChannelsMessage{
				FromChannelID: 100,
				Limit:         100,
			},
		},
		{
			name: "max limit",
			msg: ListChannelsMessage{
				FromChannelID: 0,
				Limit:         1000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &ListChannelsMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.FromChannelID, decoded.FromChannelID)
			assert.Equal(t, tt.msg.Limit, decoded.Limit)
		})
	}
}

func TestChannelListMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  ChannelListMessage
	}{
		{
			name: "empty list",
			msg: ChannelListMessage{
				Channels: []Channel{},
			},
		},
		{
			name: "single channel",
			msg: ChannelListMessage{
				Channels: []Channel{
					{
						ID:             1,
						Name:           "general",
						Description:    "General discussion",
						UserCount:      42,
						IsOperator:     false,
						Type:           0,
						RetentionHours: 168,
					},
				},
			},
		},
		{
			name: "multiple channels",
			msg: ChannelListMessage{
				Channels: []Channel{
					{
						ID:             1,
						Name:           "general",
						Description:    "General discussion",
						UserCount:      42,
						IsOperator:     false,
						Type:           0,
						RetentionHours: 168,
					},
					{
						ID:             2,
						Name:           "tech",
						Description:    "Technical topics",
						UserCount:      15,
						IsOperator:     true,
						Type:           1,
						RetentionHours: 720,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &ChannelListMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, len(tt.msg.Channels), len(decoded.Channels))

			for i, ch := range tt.msg.Channels {
				assert.Equal(t, ch.ID, decoded.Channels[i].ID)
				assert.Equal(t, ch.Name, decoded.Channels[i].Name)
				assert.Equal(t, ch.Description, decoded.Channels[i].Description)
				assert.Equal(t, ch.UserCount, decoded.Channels[i].UserCount)
				assert.Equal(t, ch.IsOperator, decoded.Channels[i].IsOperator)
				assert.Equal(t, ch.Type, decoded.Channels[i].Type)
				assert.Equal(t, ch.RetentionHours, decoded.Channels[i].RetentionHours)
			}
		})
	}
}

func TestJoinChannelMessage(t *testing.T) {
	subchannelID := uint64(5)

	tests := []struct {
		name string
		msg  JoinChannelMessage
	}{
		{
			name: "without subchannel",
			msg: JoinChannelMessage{
				ChannelID:    1,
				SubchannelID: nil,
			},
		},
		{
			name: "with subchannel",
			msg: JoinChannelMessage{
				ChannelID:    1,
				SubchannelID: &subchannelID,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &JoinChannelMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)

			if tt.msg.SubchannelID == nil {
				assert.Nil(t, decoded.SubchannelID)
			} else {
				require.NotNil(t, decoded.SubchannelID)
				assert.Equal(t, *tt.msg.SubchannelID, *decoded.SubchannelID)
			}
		})
	}
}

func TestJoinResponseMessage(t *testing.T) {
	subchannelID := uint64(5)

	tests := []struct {
		name string
		msg  JoinResponseMessage
	}{
		{
			name: "success without subchannel",
			msg: JoinResponseMessage{
				Success:      true,
				ChannelID:    1,
				SubchannelID: nil,
				Message:      "Joined successfully",
			},
		},
		{
			name: "success with subchannel",
			msg: JoinResponseMessage{
				Success:      true,
				ChannelID:    1,
				SubchannelID: &subchannelID,
				Message:      "",
			},
		},
		{
			name: "failure",
			msg: JoinResponseMessage{
				Success:      false,
				ChannelID:    999,
				SubchannelID: nil,
				Message:      "Channel not found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &JoinResponseMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			assert.Equal(t, tt.msg.Message, decoded.Message)

			if tt.msg.SubchannelID == nil {
				assert.Nil(t, decoded.SubchannelID)
			} else {
				require.NotNil(t, decoded.SubchannelID)
				assert.Equal(t, *tt.msg.SubchannelID, *decoded.SubchannelID)
			}
		})
	}
}

func TestLeaveChannelMessage(t *testing.T) {
	subchannelID := uint64(7)

	tests := []struct {
		name string
		msg  LeaveChannelMessage
	}{
		{
			name: "leave root channel",
			msg: LeaveChannelMessage{
				ChannelID:    2,
				SubchannelID: nil,
			},
		},
		{
			name: "leave subchannel",
			msg: LeaveChannelMessage{
				ChannelID:    3,
				SubchannelID: &subchannelID,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &LeaveChannelMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			if tt.msg.SubchannelID == nil {
				assert.Nil(t, decoded.SubchannelID)
			} else {
				require.NotNil(t, decoded.SubchannelID)
				assert.Equal(t, *tt.msg.SubchannelID, *decoded.SubchannelID)
			}
		})
	}
}

func TestLeaveResponseMessage(t *testing.T) {
	subchannelID := uint64(8)

	tests := []struct {
		name string
		msg  LeaveResponseMessage
	}{
		{
			name: "success leave root",
			msg: LeaveResponseMessage{
				Success:      true,
				ChannelID:    4,
				SubchannelID: nil,
				Message:      "Left channel",
			},
		},
		{
			name: "success leave subchannel",
			msg: LeaveResponseMessage{
				Success:      true,
				ChannelID:    5,
				SubchannelID: &subchannelID,
				Message:      "",
			},
		},
		{
			name: "failure to leave",
			msg: LeaveResponseMessage{
				Success:      false,
				ChannelID:    6,
				SubchannelID: nil,
				Message:      "Failed to leave",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &LeaveResponseMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			assert.Equal(t, tt.msg.Message, decoded.Message)
			if tt.msg.SubchannelID == nil {
				assert.Nil(t, decoded.SubchannelID)
			} else {
				require.NotNil(t, decoded.SubchannelID)
				assert.Equal(t, *tt.msg.SubchannelID, *decoded.SubchannelID)
			}
		})
	}
}

func TestListMessagesMessage(t *testing.T) {
	subchannelID := uint64(5)
	beforeID := uint64(100)
	afterID := uint64(75)
	parentID := uint64(50)

	tests := []struct {
		name string
		msg  ListMessagesMessage
	}{
		{
			name: "root messages, no filters",
			msg: ListMessagesMessage{
				ChannelID:    1,
				SubchannelID: nil,
				Limit:        50,
				BeforeID:     nil,
				ParentID:     nil,
				AfterID:      nil,
			},
		},
		{
			name: "with subchannel",
			msg: ListMessagesMessage{
				ChannelID:    1,
				SubchannelID: &subchannelID,
				Limit:        100,
				BeforeID:     nil,
				ParentID:     nil,
				AfterID:      nil,
			},
		},
		{
			name: "with pagination (before)",
			msg: ListMessagesMessage{
				ChannelID:    1,
				SubchannelID: nil,
				Limit:        50,
				BeforeID:     &beforeID,
				ParentID:     nil,
				AfterID:      nil,
			},
		},
		{
			name: "with pagination (after)",
			msg: ListMessagesMessage{
				ChannelID:    1,
				SubchannelID: nil,
				Limit:        50,
				BeforeID:     nil,
				ParentID:     nil,
				AfterID:      &afterID,
			},
		},
		{
			name: "thread view",
			msg: ListMessagesMessage{
				ChannelID:    1,
				SubchannelID: nil,
				Limit:        200,
				BeforeID:     nil,
				ParentID:     &parentID,
				AfterID:      nil,
			},
		},
		{
			name: "thread view with after_id (catching up)",
			msg: ListMessagesMessage{
				ChannelID:    1,
				SubchannelID: nil,
				Limit:        200,
				BeforeID:     nil,
				ParentID:     &parentID,
				AfterID:      &afterID,
			},
		},
		{
			name: "all filters",
			msg: ListMessagesMessage{
				ChannelID:    1,
				SubchannelID: &subchannelID,
				Limit:        50,
				BeforeID:     &beforeID,
				ParentID:     &parentID,
				AfterID:      nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &ListMessagesMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			assert.Equal(t, tt.msg.Limit, decoded.Limit)

			// Check optional fields
			if tt.msg.SubchannelID == nil {
				assert.Nil(t, decoded.SubchannelID)
			} else {
				require.NotNil(t, decoded.SubchannelID)
				assert.Equal(t, *tt.msg.SubchannelID, *decoded.SubchannelID)
			}

			if tt.msg.BeforeID == nil {
				assert.Nil(t, decoded.BeforeID)
			} else {
				require.NotNil(t, decoded.BeforeID)
				assert.Equal(t, *tt.msg.BeforeID, *decoded.BeforeID)
			}

			if tt.msg.ParentID == nil {
				assert.Nil(t, decoded.ParentID)
			} else {
				require.NotNil(t, decoded.ParentID)
				assert.Equal(t, *tt.msg.ParentID, *decoded.ParentID)
			}
		})
	}
}

func TestMessageListMessage(t *testing.T) {
	now := time.Now()
	editedTime := now.Add(5 * time.Minute)

	subchannelID := uint64(5)
	parentID := uint64(10)
	authorUserID := uint64(42)

	tests := []struct {
		name string
		msg  MessageListMessage
	}{
		{
			name: "empty list",
			msg: MessageListMessage{
				ChannelID:    1,
				SubchannelID: nil,
				ParentID:     nil,
				Messages:     []Message{},
			},
		},
		{
			name: "single message",
			msg: MessageListMessage{
				ChannelID:    1,
				SubchannelID: nil,
				ParentID:     nil,
				Messages: []Message{
					{
						ID:             1,
						ChannelID:      1,
						SubchannelID:   nil,
						ParentID:       nil,
						AuthorUserID:   &authorUserID,
						AuthorNickname: "alice",
						Content:        "Hello, world!",
						CreatedAt:      now,
						EditedAt:       nil,
						ReplyCount:     5,
					},
				},
			},
		},
		{
			name: "multiple messages with all fields",
			msg: MessageListMessage{
				ChannelID:    1,
				SubchannelID: &subchannelID,
				ParentID:     &parentID,
				Messages: []Message{
					{
						ID:             1,
						ChannelID:      1,
						SubchannelID:   &subchannelID,
						ParentID:       nil,
						AuthorUserID:   &authorUserID,
						AuthorNickname: "alice",
						Content:        "Root message",
						CreatedAt:      now,
						EditedAt:       nil,
						ReplyCount:     2,
					},
					{
						ID:             2,
						ChannelID:      1,
						SubchannelID:   &subchannelID,
						ParentID:       &parentID,
						AuthorUserID:   nil, // Anonymous user
						AuthorNickname: "bob",
						Content:        "Reply message",
						CreatedAt:      now.Add(time.Minute),
						EditedAt:       &editedTime,
						ReplyCount:     0,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &MessageListMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			assert.Equal(t, len(tt.msg.Messages), len(decoded.Messages))

			for i, msg := range tt.msg.Messages {
				dec := decoded.Messages[i]
				assert.Equal(t, msg.ID, dec.ID)
				assert.Equal(t, msg.ChannelID, dec.ChannelID)
				assert.Equal(t, msg.AuthorNickname, dec.AuthorNickname)
				assert.Equal(t, msg.Content, dec.Content)
				assert.Equal(t, msg.ReplyCount, dec.ReplyCount)

				assert.InDelta(t, msg.CreatedAt.UnixMilli(), dec.CreatedAt.UnixMilli(), 1)

				// Check optional fields
				if msg.SubchannelID == nil {
					assert.Nil(t, dec.SubchannelID)
				} else {
					require.NotNil(t, dec.SubchannelID)
					assert.Equal(t, *msg.SubchannelID, *dec.SubchannelID)
				}

				if msg.ParentID == nil {
					assert.Nil(t, dec.ParentID)
				} else {
					require.NotNil(t, dec.ParentID)
					assert.Equal(t, *msg.ParentID, *dec.ParentID)
				}

				if msg.AuthorUserID == nil {
					assert.Nil(t, dec.AuthorUserID)
				} else {
					require.NotNil(t, dec.AuthorUserID)
					assert.Equal(t, *msg.AuthorUserID, *dec.AuthorUserID)
				}

				if msg.EditedAt == nil {
					assert.Nil(t, dec.EditedAt)
				} else {
					require.NotNil(t, dec.EditedAt)
					assert.InDelta(t, msg.EditedAt.UnixMilli(), dec.EditedAt.UnixMilli(), 1)
				}
			}
		})
	}
}

func TestPostMessageMessage(t *testing.T) {
	subchannelID := uint64(5)
	parentID := uint64(10)

	tests := []struct {
		name    string
		msg     PostMessageMessage
		wantErr bool
		errType error
	}{
		{
			name: "root message",
			msg: PostMessageMessage{
				ChannelID:    1,
				SubchannelID: nil,
				ParentID:     nil,
				Content:      "Hello, world!",
			},
			wantErr: false,
		},
		{
			name: "reply message",
			msg: PostMessageMessage{
				ChannelID:    1,
				SubchannelID: nil,
				ParentID:     &parentID,
				Content:      "This is a reply",
			},
			wantErr: false,
		},
		{
			name: "with subchannel",
			msg: PostMessageMessage{
				ChannelID:    1,
				SubchannelID: &subchannelID,
				ParentID:     nil,
				Content:      "In subchannel",
			},
			wantErr: false,
		},
		{
			name: "max length (4096)",
			msg: PostMessageMessage{
				ChannelID:    1,
				SubchannelID: nil,
				ParentID:     nil,
				Content:      string(make([]byte, 4096)),
			},
			wantErr: false,
		},
		{
			name: "empty content",
			msg: PostMessageMessage{
				ChannelID: 1,
				Content:   "",
			},
			wantErr: true,
			errType: ErrEmptyContent,
		},
		{
			name: "too long content",
			msg: PostMessageMessage{
				ChannelID: 1,
				Content:   string(make([]byte, 4097)),
			},
			wantErr: true,
			errType: ErrMessageTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errType, err)
				return
			}
			require.NoError(t, err)

			decoded := &PostMessageMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			assert.Equal(t, tt.msg.Content, decoded.Content)

			if tt.msg.SubchannelID == nil {
				assert.Nil(t, decoded.SubchannelID)
			} else {
				require.NotNil(t, decoded.SubchannelID)
				assert.Equal(t, *tt.msg.SubchannelID, *decoded.SubchannelID)
			}

			if tt.msg.ParentID == nil {
				assert.Nil(t, decoded.ParentID)
			} else {
				require.NotNil(t, decoded.ParentID)
				assert.Equal(t, *tt.msg.ParentID, *decoded.ParentID)
			}
		})
	}
}

func TestMessagePostedMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  MessagePostedMessage
	}{
		{
			name: "success",
			msg: MessagePostedMessage{
				Success:   true,
				MessageID: 123,
				Message:   "",
			},
		},
		{
			name: "failure",
			msg: MessagePostedMessage{
				Success:   false,
				MessageID: 0,
				Message:   "Rate limit exceeded",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &MessagePostedMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.MessageID, decoded.MessageID)
			assert.Equal(t, tt.msg.Message, decoded.Message)
		})
	}
}

func TestEditMessageMessage(t *testing.T) {
	tests := []struct {
		name       string
		messageID  uint64
		newContent string
		wantErr    bool
		errType    error
	}{
		{"valid edit", 123, "Updated message content", false, nil},
		{"min length content", 1, "Hi", false, nil},
		{"max length content", 999, string(make([]byte, 4096)), false, nil},
		{"empty content", 42, "", true, ErrEmptyContent},
		{"content too long", 55, string(make([]byte, 4097)), true, ErrMessageTooLong},
		{"zero message id", 0, "Valid content", false, nil},
		{"large message id", 18446744073709551615, "Content", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &EditMessageMessage{
				MessageID:  tt.messageID,
				NewContent: tt.newContent,
			}

			// Test encode
			payload, err := msg.Encode()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errType, err)
				return
			}
			require.NoError(t, err)

			// Test decode
			decoded := &EditMessageMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.messageID, decoded.MessageID)
			assert.Equal(t, tt.newContent, decoded.NewContent)
		})
	}
}

func TestMessageEditedMessage(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		msg  MessageEditedMessage
	}{
		{
			name: "successful edit",
			msg: MessageEditedMessage{
				Success:    true,
				MessageID:  123,
				EditedAt:   now,
				NewContent: "Updated content",
				Message:    "",
			},
		},
		{
			name: "failed edit - not author",
			msg: MessageEditedMessage{
				Success:   false,
				MessageID: 456,
				Message:   "Not message author",
			},
		},
		{
			name: "failed edit - message not found",
			msg: MessageEditedMessage{
				Success:   false,
				MessageID: 789,
				Message:   "Message not found",
			},
		},
		{
			name: "failed edit - anonymous user",
			msg: MessageEditedMessage{
				Success:   false,
				MessageID: 999,
				Message:   "Authentication required",
			},
		},
		{
			name: "successful edit with long content",
			msg: MessageEditedMessage{
				Success:    true,
				MessageID:  100,
				EditedAt:   now,
				NewContent: string(make([]byte, 4096)),
				Message:    "Success",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &MessageEditedMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.MessageID, decoded.MessageID)
			if tt.msg.Success {
				assert.Equal(t, tt.msg.EditedAt.Unix(), decoded.EditedAt.Unix())
				assert.Equal(t, tt.msg.NewContent, decoded.NewContent)
			}
			assert.Equal(t, tt.msg.Message, decoded.Message)
		})
	}
}

func TestDeleteMessageMessage(t *testing.T) {
	msg := &DeleteMessageMessage{MessageID: 123}

	payload, err := msg.Encode()
	require.NoError(t, err)

	decoded := &DeleteMessageMessage{}
	err = decoded.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, msg.MessageID, decoded.MessageID)
}

func TestMessageDeletedMessage(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		msg  MessageDeletedMessage
	}{
		{
			name: "success",
			msg: MessageDeletedMessage{
				Success:   true,
				MessageID: 123,
				DeletedAt: now,
				Message:   "",
			},
		},
		{
			name: "failure",
			msg: MessageDeletedMessage{
				Success:   false,
				MessageID: 123,
				DeletedAt: time.Time{}, // Not used when Success=false
				Message:   "Not your message",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &MessageDeletedMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.MessageID, decoded.MessageID)
			assert.Equal(t, tt.msg.Message, decoded.Message)

			if tt.msg.Success {
				assert.InDelta(t, tt.msg.DeletedAt.UnixMilli(), decoded.DeletedAt.UnixMilli(), 1)
			}
		})
	}
}

func TestPingMessage(t *testing.T) {
	now := time.Now().UnixMilli()
	msg := &PingMessage{Timestamp: now}

	payload, err := msg.Encode()
	require.NoError(t, err)

	decoded := &PingMessage{}
	err = decoded.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, msg.Timestamp, decoded.Timestamp)
}

func TestPongMessage(t *testing.T) {
	now := time.Now().UnixMilli()
	msg := &PongMessage{ClientTimestamp: now}

	payload, err := msg.Encode()
	require.NoError(t, err)

	decoded := &PongMessage{}
	err = decoded.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, msg.ClientTimestamp, decoded.ClientTimestamp)
}

func TestErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  ErrorMessage
	}{
		{
			name: "protocol error",
			msg: ErrorMessage{
				ErrorCode: ErrCodeInvalidFormat,
				Message:   "Invalid message format",
			},
		},
		{
			name: "auth error",
			msg: ErrorMessage{
				ErrorCode: ErrCodeAuthRequired,
				Message:   "Authentication required",
			},
		},
		{
			name: "not found",
			msg: ErrorMessage{
				ErrorCode: ErrCodeChannelNotFound,
				Message:   "Channel not found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &ErrorMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.ErrorCode, decoded.ErrorCode)
			assert.Equal(t, tt.msg.Message, decoded.Message)
		})
	}
}

func TestServerConfigMessage(t *testing.T) {
	msg := &ServerConfigMessage{
		ProtocolVersion:     1,
		MaxMessageRate:      10,
		MaxChannelCreates:   5,
		InactiveCleanupDays: 90,
		MaxConnectionsPerIP: 10,
		MaxMessageLength:    4096,
	}

	payload, err := msg.Encode()
	require.NoError(t, err)

	decoded := &ServerConfigMessage{}
	err = decoded.Decode(payload)
	require.NoError(t, err)
	assert.Equal(t, msg.ProtocolVersion, decoded.ProtocolVersion)
	assert.Equal(t, msg.MaxMessageRate, decoded.MaxMessageRate)
	assert.Equal(t, msg.MaxChannelCreates, decoded.MaxChannelCreates)
	assert.Equal(t, msg.InactiveCleanupDays, decoded.InactiveCleanupDays)
	assert.Equal(t, msg.MaxConnectionsPerIP, decoded.MaxConnectionsPerIP)
	assert.Equal(t, msg.MaxMessageLength, decoded.MaxMessageLength)
}

func TestNewMessageMessage(t *testing.T) {
	now := time.Now()
	editedTime := now.Add(5 * time.Minute)

	subchannelID := uint64(5)
	parentID := uint64(10)
	authorUserID := uint64(42)

	tests := []struct {
		name string
		msg  NewMessageMessage
	}{
		{
			name: "root message",
			msg: NewMessageMessage{
				ID:             1,
				ChannelID:      1,
				SubchannelID:   nil,
				ParentID:       nil,
				AuthorUserID:   &authorUserID,
				AuthorNickname: "alice",
				Content:        "Hello, world!",
				CreatedAt:      now,
				EditedAt:       nil,
				ReplyCount:     0,
			},
		},
		{
			name: "reply with all fields",
			msg: NewMessageMessage{
				ID:             2,
				ChannelID:      1,
				SubchannelID:   &subchannelID,
				ParentID:       &parentID,
				AuthorUserID:   nil, // Anonymous
				AuthorNickname: "bob",
				Content:        "This is a reply",
				CreatedAt:      now,
				EditedAt:       &editedTime,
				ReplyCount:     0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &NewMessageMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.ID, decoded.ID)
			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			assert.Equal(t, tt.msg.AuthorNickname, decoded.AuthorNickname)
			assert.Equal(t, tt.msg.Content, decoded.Content)
			assert.Equal(t, tt.msg.ReplyCount, decoded.ReplyCount)
			assert.InDelta(t, tt.msg.CreatedAt.UnixMilli(), decoded.CreatedAt.UnixMilli(), 1)

			// Check optional fields
			if tt.msg.SubchannelID == nil {
				assert.Nil(t, decoded.SubchannelID)
			} else {
				require.NotNil(t, decoded.SubchannelID)
				assert.Equal(t, *tt.msg.SubchannelID, *decoded.SubchannelID)
			}

			if tt.msg.ParentID == nil {
				assert.Nil(t, decoded.ParentID)
			} else {
				require.NotNil(t, decoded.ParentID)
				assert.Equal(t, *tt.msg.ParentID, *decoded.ParentID)
			}

			if tt.msg.AuthorUserID == nil {
				assert.Nil(t, decoded.AuthorUserID)
			} else {
				require.NotNil(t, decoded.AuthorUserID)
				assert.Equal(t, *tt.msg.AuthorUserID, *decoded.AuthorUserID)
			}

			if tt.msg.EditedAt == nil {
				assert.Nil(t, decoded.EditedAt)
			} else {
				require.NotNil(t, decoded.EditedAt)
				assert.InDelta(t, tt.msg.EditedAt.UnixMilli(), decoded.EditedAt.UnixMilli(), 1)
			}
		})
	}
}

func TestGetUserInfoMessage(t *testing.T) {
	tests := []struct {
		name     string
		nickname string
	}{
		{
			name:     "valid nickname",
			nickname: "alice",
		},
		{
			name:     "long nickname",
			nickname: "12345678901234567890",
		},
		{
			name:     "nickname with underscore",
			nickname: "alice_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &GetUserInfoMessage{
				Nickname: tt.nickname,
			}

			payload, err := msg.Encode()
			require.NoError(t, err)

			decoded := &GetUserInfoMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.nickname, decoded.Nickname)
		})
	}
}

func TestUserInfoMessage(t *testing.T) {
	userID := uint64(42)

	tests := []struct {
		name string
		msg  UserInfoMessage
	}{
		{
			name: "registered user online",
			msg: UserInfoMessage{
				Nickname:     "alice",
				IsRegistered: true,
				UserID:       &userID,
				Online:       true,
			},
		},
		{
			name: "registered user offline",
			msg: UserInfoMessage{
				Nickname:     "bob",
				IsRegistered: true,
				UserID:       &userID,
				Online:       false,
			},
		},
		{
			name: "anonymous user online",
			msg: UserInfoMessage{
				Nickname:     "charlie",
				IsRegistered: false,
				UserID:       nil,
				Online:       true,
			},
		},
		{
			name: "user not found",
			msg: UserInfoMessage{
				Nickname:     "unknown",
				IsRegistered: false,
				UserID:       nil,
				Online:       false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &UserInfoMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.msg.Nickname, decoded.Nickname)
			assert.Equal(t, tt.msg.IsRegistered, decoded.IsRegistered)
			assert.Equal(t, tt.msg.Online, decoded.Online)

			if tt.msg.UserID == nil {
				assert.Nil(t, decoded.UserID)
			} else {
				require.NotNil(t, decoded.UserID)
				assert.Equal(t, *tt.msg.UserID, *decoded.UserID)
			}
		})
	}
}

func TestListUsersMessage(t *testing.T) {
	tests := []struct {
		name           string
		limit          uint16
		includeOffline bool
	}{
		{
			name:           "default limit, online only",
			limit:          100,
			includeOffline: false,
		},
		{
			name:           "default limit, include offline",
			limit:          100,
			includeOffline: true,
		},
		{
			name:           "small limit, online only",
			limit:          10,
			includeOffline: false,
		},
		{
			name:           "small limit, include offline",
			limit:          10,
			includeOffline: true,
		},
		{
			name:           "max limit, online only",
			limit:          500,
			includeOffline: false,
		},
		{
			name:           "max limit, include offline",
			limit:          500,
			includeOffline: true,
		},
		{
			name:           "zero limit, online only",
			limit:          0,
			includeOffline: false,
		},
		{
			name:           "zero limit, include offline",
			limit:          0,
			includeOffline: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &ListUsersMessage{
				Limit:          tt.limit,
				IncludeOffline: tt.includeOffline,
			}

			payload, err := msg.Encode()
			require.NoError(t, err)

			decoded := &ListUsersMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			assert.Equal(t, tt.limit, decoded.Limit)
			assert.Equal(t, tt.includeOffline, decoded.IncludeOffline)
		})
	}
}

func TestListUsersMessageBackwardsCompatibility(t *testing.T) {
	// Test that messages without the include_offline flag (older clients)
	// default to false when decoded
	tests := []struct {
		name    string
		payload []byte // Manually constructed payload without include_offline field
		limit   uint16
	}{
		{
			name:    "limit 100, no include_offline field",
			payload: []byte{0x00, 0x64}, // uint16(100) in big endian
			limit:   100,
		},
		{
			name:    "limit 10, no include_offline field",
			payload: []byte{0x00, 0x0A}, // uint16(10) in big endian
			limit:   10,
		},
		{
			name:    "limit 500, no include_offline field",
			payload: []byte{0x01, 0xF4}, // uint16(500) in big endian
			limit:   500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded := &ListUsersMessage{}
			err := decoded.Decode(tt.payload)
			require.NoError(t, err)
			assert.Equal(t, tt.limit, decoded.Limit)
			assert.False(t, decoded.IncludeOffline, "include_offline should default to false for backwards compatibility")
		})
	}
}

func TestUserListMessage(t *testing.T) {
	userID1 := uint64(1)
	userID2 := uint64(2)

	tests := []struct {
		name string
		msg  UserListMessage
	}{
		{
			name: "empty list",
			msg: UserListMessage{
				Users: []UserListEntry{},
			},
		},
		{
			name: "single registered user online",
			msg: UserListMessage{
				Users: []UserListEntry{
					{
						Nickname:     "alice",
						IsRegistered: true,
						UserID:       &userID1,
						Online:       true,
					},
				},
			},
		},
		{
			name: "single registered user offline",
			msg: UserListMessage{
				Users: []UserListEntry{
					{
						Nickname:     "alice",
						IsRegistered: true,
						UserID:       &userID1,
						Online:       false,
					},
				},
			},
		},
		{
			name: "single anonymous user (always online)",
			msg: UserListMessage{
				Users: []UserListEntry{
					{
						Nickname:     "bob",
						IsRegistered: false,
						UserID:       nil,
						Online:       true,
					},
				},
			},
		},
		{
			name: "mixed users with varied online status",
			msg: UserListMessage{
				Users: []UserListEntry{
					{
						Nickname:     "alice",
						IsRegistered: true,
						UserID:       &userID1,
						Online:       true,
					},
					{
						Nickname:     "bob",
						IsRegistered: false,
						UserID:       nil,
						Online:       true,
					},
					{
						Nickname:     "charlie",
						IsRegistered: true,
						UserID:       &userID2,
						Online:       false,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &UserListMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)
			require.Equal(t, len(tt.msg.Users), len(decoded.Users))

			for i, user := range tt.msg.Users {
				assert.Equal(t, user.Nickname, decoded.Users[i].Nickname)
				assert.Equal(t, user.IsRegistered, decoded.Users[i].IsRegistered)
				assert.Equal(t, user.Online, decoded.Users[i].Online)

				if user.UserID == nil {
					assert.Nil(t, decoded.Users[i].UserID)
				} else {
					require.NotNil(t, decoded.Users[i].UserID)
					assert.Equal(t, *user.UserID, *decoded.Users[i].UserID)
				}
			}
		})
	}
}

func TestChannelUserListMessage(t *testing.T) {
	userID := uint64(10)
	subchannelID := uint64(5)
	msg := ChannelUserListMessage{
		ChannelID:    3,
		SubchannelID: &subchannelID,
		Users: []ChannelUserEntry{
			{
				SessionID:    101,
				Nickname:     "alice",
				IsRegistered: true,
				UserID:       &userID,
				UserFlags:    UserFlagAdmin,
			},
			{
				SessionID:    102,
				Nickname:     "guest",
				IsRegistered: false,
				UserFlags:    0,
			},
		},
	}

	payload, err := msg.Encode()
	require.NoError(t, err)

	decoded := &ChannelUserListMessage{}
	err = decoded.Decode(payload)
	require.NoError(t, err)

	assert.Equal(t, msg.ChannelID, decoded.ChannelID)
	require.NotNil(t, decoded.SubchannelID)
	assert.Equal(t, *msg.SubchannelID, *decoded.SubchannelID)
	require.Equal(t, len(msg.Users), len(decoded.Users))

	for i := range msg.Users {
		assert.Equal(t, msg.Users[i].SessionID, decoded.Users[i].SessionID)
		assert.Equal(t, msg.Users[i].Nickname, decoded.Users[i].Nickname)
		assert.Equal(t, msg.Users[i].IsRegistered, decoded.Users[i].IsRegistered)
		assert.Equal(t, msg.Users[i].UserFlags, decoded.Users[i].UserFlags)
		if msg.Users[i].UserID == nil {
			assert.Nil(t, decoded.Users[i].UserID)
		} else {
			require.NotNil(t, decoded.Users[i].UserID)
			assert.Equal(t, *msg.Users[i].UserID, *decoded.Users[i].UserID)
		}
	}
}

func TestChannelPresenceMessage(t *testing.T) {
	userID := uint64(42)
	subchannelID := uint64(8)
	msg := ChannelPresenceMessage{
		ChannelID:    7,
		SubchannelID: &subchannelID,
		SessionID:    555,
		Nickname:     "carol",
		IsRegistered: true,
		UserID:       &userID,
		UserFlags:    UserFlagModerator,
		Joined:       true,
	}

	payload, err := msg.Encode()
	require.NoError(t, err)

	decoded := &ChannelPresenceMessage{}
	err = decoded.Decode(payload)
	require.NoError(t, err)

	assert.Equal(t, msg.ChannelID, decoded.ChannelID)
	require.NotNil(t, decoded.SubchannelID)
	assert.Equal(t, *msg.SubchannelID, *decoded.SubchannelID)
	assert.Equal(t, msg.SessionID, decoded.SessionID)
	assert.Equal(t, msg.Nickname, decoded.Nickname)
	assert.Equal(t, msg.IsRegistered, decoded.IsRegistered)
	require.NotNil(t, decoded.UserID)
	assert.Equal(t, *msg.UserID, *decoded.UserID)
	assert.Equal(t, msg.UserFlags, decoded.UserFlags)
	assert.Equal(t, msg.Joined, decoded.Joined)
}

func TestServerPresenceMessage(t *testing.T) {
	userID := uint64(12)
	msg := ServerPresenceMessage{
		SessionID:    777,
		Nickname:     "dave",
		IsRegistered: true,
		UserID:       &userID,
		UserFlags:    UserFlagAdmin,
		Online:       false,
	}

	payload, err := msg.Encode()
	require.NoError(t, err)

	decoded := &ServerPresenceMessage{}
	err = decoded.Decode(payload)
	require.NoError(t, err)

	assert.Equal(t, msg.SessionID, decoded.SessionID)
	assert.Equal(t, msg.Nickname, decoded.Nickname)
	assert.Equal(t, msg.IsRegistered, decoded.IsRegistered)
	require.NotNil(t, decoded.UserID)
	assert.Equal(t, *msg.UserID, *decoded.UserID)
	assert.Equal(t, msg.UserFlags, decoded.UserFlags)
	assert.Equal(t, msg.Online, decoded.Online)
}

func TestMessageTypeConstants(t *testing.T) {
	// Test that message type constants have expected values
	assert.Equal(t, 0x02, TypeSetNickname)
	assert.Equal(t, 0x04, TypeListChannels)
	assert.Equal(t, 0x05, TypeJoinChannel)
	assert.Equal(t, 0x06, TypeLeaveChannel)
	assert.Equal(t, 0x09, TypeListMessages)
	assert.Equal(t, 0x0A, TypePostMessage)
	assert.Equal(t, 0x0C, TypeDeleteMessage)
	assert.Equal(t, 0x0F, TypeGetUserInfo)
	assert.Equal(t, 0x10, TypePing)
	assert.Equal(t, 0x16, TypeListUsers)
	assert.Equal(t, 0x17, TypeListChannelUsers)

	assert.Equal(t, 0x82, TypeNicknameResponse)
	assert.Equal(t, 0x84, TypeChannelList)
	assert.Equal(t, 0x85, TypeJoinResponse)
	assert.Equal(t, 0x86, TypeLeaveResponse)
	assert.Equal(t, 0x89, TypeMessageList)
	assert.Equal(t, 0x8A, TypeMessagePosted)
	assert.Equal(t, 0x8B, TypeMessageEdited)
	assert.Equal(t, 0x8C, TypeMessageDeleted)
	assert.Equal(t, 0x8D, TypeNewMessage)
	assert.Equal(t, 0x8F, TypeUserInfo)
	assert.Equal(t, 0x90, TypePong)
	assert.Equal(t, 0x91, TypeError)
	assert.Equal(t, 0x98, TypeServerConfig)
	assert.Equal(t, 0x9A, TypeUserList)
	assert.Equal(t, 0xAB, TypeChannelUserList)
	assert.Equal(t, 0xAC, TypeChannelPresence)
	assert.Equal(t, 0xAD, TypeServerPresence)
	assert.Equal(t, 0x18, TypeGetUnreadCounts)
	assert.Equal(t, 0x1D, TypeUpdateReadState)
	assert.Equal(t, 0x97, TypeUnreadCounts)
}

func TestErrorCodeConstants(t *testing.T) {
	// Test that error code constants are in correct ranges
	assert.Equal(t, 1000, ErrCodeInvalidFormat)
	assert.Equal(t, 1001, ErrCodeUnsupportedVersion)
	assert.Equal(t, 2000, ErrCodeAuthRequired)
	assert.Equal(t, 3000, ErrCodePermissionDenied)
	assert.Equal(t, 4000, ErrCodeNotFound)
	assert.Equal(t, 5000, ErrCodeRateLimitExceeded)
	assert.Equal(t, 6000, ErrCodeInvalidInput)
	assert.Equal(t, 9000, ErrCodeInternalError)
}

func TestGetUnreadCountsMessage(t *testing.T) {
	uint64Ptr := func(v uint64) *uint64 { return &v }
	int64Ptr := func(v int64) *int64 { return &v }

	tests := []struct {
		name string
		msg  GetUnreadCountsMessage
	}{
		{
			name: "with timestamp and single channel",
			msg: GetUnreadCountsMessage{
				SinceTimestamp: int64Ptr(1234567890),
				Targets: []UnreadTarget{
					{ChannelID: 1, SubchannelID: nil},
				},
			},
		},
		{
			name: "with timestamp and multiple channels",
			msg: GetUnreadCountsMessage{
				SinceTimestamp: int64Ptr(9876543210),
				Targets: []UnreadTarget{
					{ChannelID: 1, SubchannelID: nil},
					{ChannelID: 2, SubchannelID: uint64Ptr(5)},
					{ChannelID: 3, SubchannelID: nil},
				},
			},
		},
		{
			name: "without timestamp (use server state)",
			msg: GetUnreadCountsMessage{
				SinceTimestamp: nil,
				Targets: []UnreadTarget{
					{ChannelID: 42, SubchannelID: nil},
				},
			},
		},
		{
			name: "empty targets list",
			msg: GetUnreadCountsMessage{
				SinceTimestamp: int64Ptr(1000),
				Targets:        []UnreadTarget{},
			},
		},
		{
			name: "with subchannels",
			msg: GetUnreadCountsMessage{
				SinceTimestamp: int64Ptr(5000),
				Targets: []UnreadTarget{
					{ChannelID: 10, SubchannelID: uint64Ptr(1), ThreadID: nil},
					{ChannelID: 10, SubchannelID: uint64Ptr(2), ThreadID: nil},
					{ChannelID: 20, SubchannelID: nil, ThreadID: nil},
				},
			},
		},
		{
			name: "with thread IDs",
			msg: GetUnreadCountsMessage{
				SinceTimestamp: int64Ptr(2000),
				Targets: []UnreadTarget{
					{ChannelID: 1, SubchannelID: nil, ThreadID: uint64Ptr(42)},
					{ChannelID: 1, SubchannelID: nil, ThreadID: uint64Ptr(99)},
					{ChannelID: 2, SubchannelID: uint64Ptr(5), ThreadID: uint64Ptr(100)},
				},
			},
		},
		{
			name: "mixed - channels and threads",
			msg: GetUnreadCountsMessage{
				SinceTimestamp: int64Ptr(3000),
				Targets: []UnreadTarget{
					{ChannelID: 1, SubchannelID: nil, ThreadID: nil},          // whole channel
					{ChannelID: 1, SubchannelID: nil, ThreadID: uint64Ptr(5)}, // specific thread
					{ChannelID: 2, SubchannelID: nil, ThreadID: nil},          // another channel
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &GetUnreadCountsMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			if tt.msg.SinceTimestamp == nil {
				assert.Nil(t, decoded.SinceTimestamp)
			} else {
				require.NotNil(t, decoded.SinceTimestamp)
				assert.Equal(t, *tt.msg.SinceTimestamp, *decoded.SinceTimestamp)
			}

			assert.Equal(t, len(tt.msg.Targets), len(decoded.Targets))
			for i := range tt.msg.Targets {
				assert.Equal(t, tt.msg.Targets[i].ChannelID, decoded.Targets[i].ChannelID)
				if tt.msg.Targets[i].SubchannelID == nil {
					assert.Nil(t, decoded.Targets[i].SubchannelID)
				} else {
					require.NotNil(t, decoded.Targets[i].SubchannelID)
					assert.Equal(t, *tt.msg.Targets[i].SubchannelID, *decoded.Targets[i].SubchannelID)
				}
				if tt.msg.Targets[i].ThreadID == nil {
					assert.Nil(t, decoded.Targets[i].ThreadID)
				} else {
					require.NotNil(t, decoded.Targets[i].ThreadID)
					assert.Equal(t, *tt.msg.Targets[i].ThreadID, *decoded.Targets[i].ThreadID)
				}
			}
		})
	}
}

func TestUnreadCountsMessage(t *testing.T) {
	uint64Ptr := func(v uint64) *uint64 { return &v }

	tests := []struct {
		name string
		msg  UnreadCountsMessage
	}{
		{
			name: "single channel with count",
			msg: UnreadCountsMessage{
				Counts: []UnreadCount{
					{ChannelID: 1, SubchannelID: nil, UnreadCount: 42},
				},
			},
		},
		{
			name: "multiple channels",
			msg: UnreadCountsMessage{
				Counts: []UnreadCount{
					{ChannelID: 1, SubchannelID: nil, UnreadCount: 10},
					{ChannelID: 2, SubchannelID: uint64Ptr(3), UnreadCount: 25},
					{ChannelID: 3, SubchannelID: nil, UnreadCount: 0},
				},
			},
		},
		{
			name: "zero counts",
			msg: UnreadCountsMessage{
				Counts: []UnreadCount{
					{ChannelID: 1, SubchannelID: nil, UnreadCount: 0},
					{ChannelID: 2, SubchannelID: nil, UnreadCount: 0},
				},
			},
		},
		{
			name: "empty counts list",
			msg: UnreadCountsMessage{
				Counts: []UnreadCount{},
			},
		},
		{
			name: "large count values",
			msg: UnreadCountsMessage{
				Counts: []UnreadCount{
					{ChannelID: 999, SubchannelID: uint64Ptr(888), UnreadCount: 4294967295}, // max uint32
				},
			},
		},
		{
			name: "mixed subchannels",
			msg: UnreadCountsMessage{
				Counts: []UnreadCount{
					{ChannelID: 10, SubchannelID: nil, ThreadID: nil, UnreadCount: 5},
					{ChannelID: 10, SubchannelID: uint64Ptr(1), ThreadID: nil, UnreadCount: 3},
					{ChannelID: 10, SubchannelID: uint64Ptr(2), ThreadID: nil, UnreadCount: 7},
				},
			},
		},
		{
			name: "with thread counts",
			msg: UnreadCountsMessage{
				Counts: []UnreadCount{
					{ChannelID: 1, SubchannelID: nil, ThreadID: uint64Ptr(42), UnreadCount: 3},
					{ChannelID: 1, SubchannelID: nil, ThreadID: uint64Ptr(99), UnreadCount: 7},
				},
			},
		},
		{
			name: "mixed channels and threads",
			msg: UnreadCountsMessage{
				Counts: []UnreadCount{
					{ChannelID: 1, SubchannelID: nil, ThreadID: nil, UnreadCount: 50},          // whole channel
					{ChannelID: 1, SubchannelID: nil, ThreadID: uint64Ptr(10), UnreadCount: 5}, // specific thread
					{ChannelID: 2, SubchannelID: nil, ThreadID: nil, UnreadCount: 0},           // another channel, no unreads
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &UnreadCountsMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, len(tt.msg.Counts), len(decoded.Counts))
			for i := range tt.msg.Counts {
				assert.Equal(t, tt.msg.Counts[i].ChannelID, decoded.Counts[i].ChannelID)
				assert.Equal(t, tt.msg.Counts[i].UnreadCount, decoded.Counts[i].UnreadCount)
				if tt.msg.Counts[i].SubchannelID == nil {
					assert.Nil(t, decoded.Counts[i].SubchannelID)
				} else {
					require.NotNil(t, decoded.Counts[i].SubchannelID)
					assert.Equal(t, *tt.msg.Counts[i].SubchannelID, *decoded.Counts[i].SubchannelID)
				}
				if tt.msg.Counts[i].ThreadID == nil {
					assert.Nil(t, decoded.Counts[i].ThreadID)
				} else {
					require.NotNil(t, decoded.Counts[i].ThreadID)
					assert.Equal(t, *tt.msg.Counts[i].ThreadID, *decoded.Counts[i].ThreadID)
				}
			}
		})
	}
}

func TestUpdateReadStateMessage(t *testing.T) {
	uint64Ptr := func(v uint64) *uint64 { return &v }

	tests := []struct {
		name string
		msg  UpdateReadStateMessage
	}{
		{
			name: "channel without subchannel",
			msg: UpdateReadStateMessage{
				ChannelID:    1,
				SubchannelID: nil,
				Timestamp:    1234567890,
			},
		},
		{
			name: "channel with subchannel",
			msg: UpdateReadStateMessage{
				ChannelID:    42,
				SubchannelID: uint64Ptr(7),
				Timestamp:    9876543210,
			},
		},
		{
			name: "zero timestamp",
			msg: UpdateReadStateMessage{
				ChannelID:    100,
				SubchannelID: nil,
				Timestamp:    0,
			},
		},
		{
			name: "negative timestamp",
			msg: UpdateReadStateMessage{
				ChannelID:    5,
				SubchannelID: uint64Ptr(2),
				Timestamp:    -1000,
			},
		},
		{
			name: "large timestamp",
			msg: UpdateReadStateMessage{
				ChannelID:    999,
				SubchannelID: nil,
				Timestamp:    9223372036854775807, // max int64
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &UpdateReadStateMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			assert.Equal(t, tt.msg.Timestamp, decoded.Timestamp)
			if tt.msg.SubchannelID == nil {
				assert.Nil(t, decoded.SubchannelID)
			} else {
				require.NotNil(t, decoded.SubchannelID)
				assert.Equal(t, *tt.msg.SubchannelID, *decoded.SubchannelID)
			}
		})
	}
}

func TestCreateSubchannelMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  CreateSubchannelMessage
	}{
		{
			name: "forum subchannel",
			msg: CreateSubchannelMessage{
				ChannelID:      123,
				Name:           "announcements",
				Description:    "Important announcements",
				Type:           1, // forum
				RetentionHours: 720,
			},
		},
		{
			name: "chat subchannel",
			msg: CreateSubchannelMessage{
				ChannelID:      456,
				Name:           "general-chat",
				Description:    "",
				Type:           0, // chat
				RetentionHours: 168,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &CreateSubchannelMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			assert.Equal(t, tt.msg.Name, decoded.Name)
			assert.Equal(t, tt.msg.Description, decoded.Description)
			assert.Equal(t, tt.msg.Type, decoded.Type)
			assert.Equal(t, tt.msg.RetentionHours, decoded.RetentionHours)
		})
	}
}

func TestSubchannelCreatedMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  SubchannelCreatedMessage
	}{
		{
			name: "success",
			msg: SubchannelCreatedMessage{
				Success:        true,
				ChannelID:      123,
				SubchannelID:   456,
				Name:           "announcements",
				Description:    "Important stuff",
				Type:           1,
				RetentionHours: 720,
				Message:        "Subchannel created successfully",
			},
		},
		{
			name: "failure",
			msg: SubchannelCreatedMessage{
				Success: false,
				Message: "Permission denied",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &SubchannelCreatedMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.Success, decoded.Success)
			assert.Equal(t, tt.msg.Message, decoded.Message)
			if tt.msg.Success {
				assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
				assert.Equal(t, tt.msg.SubchannelID, decoded.SubchannelID)
				assert.Equal(t, tt.msg.Name, decoded.Name)
				assert.Equal(t, tt.msg.Description, decoded.Description)
				assert.Equal(t, tt.msg.Type, decoded.Type)
				assert.Equal(t, tt.msg.RetentionHours, decoded.RetentionHours)
			}
		})
	}
}

func TestGetSubchannelsMessage(t *testing.T) {
	msg := &GetSubchannelsMessage{
		ChannelID: 12345,
	}

	payload, err := msg.Encode()
	require.NoError(t, err)

	decoded := &GetSubchannelsMessage{}
	err = decoded.Decode(payload)
	require.NoError(t, err)

	assert.Equal(t, msg.ChannelID, decoded.ChannelID)
}

func TestSubchannelListMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  SubchannelListMessage
	}{
		{
			name: "empty list",
			msg: SubchannelListMessage{
				ChannelID:   123,
				Subchannels: []SubchannelInfo{},
			},
		},
		{
			name: "multiple subchannels",
			msg: SubchannelListMessage{
				ChannelID: 123,
				Subchannels: []SubchannelInfo{
					{
						ID:             1,
						Name:           "announcements",
						Description:    "Important stuff",
						Type:           1,
						RetentionHours: 720,
					},
					{
						ID:             2,
						Name:           "general-chat",
						Description:    "",
						Type:           0,
						RetentionHours: 168,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &SubchannelListMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			require.Equal(t, len(tt.msg.Subchannels), len(decoded.Subchannels))

			for i, sub := range tt.msg.Subchannels {
				assert.Equal(t, sub.ID, decoded.Subchannels[i].ID)
				assert.Equal(t, sub.Name, decoded.Subchannels[i].Name)
				assert.Equal(t, sub.Description, decoded.Subchannels[i].Description)
				assert.Equal(t, sub.Type, decoded.Subchannels[i].Type)
				assert.Equal(t, sub.RetentionHours, decoded.Subchannels[i].RetentionHours)
			}
		})
	}
}

// ============================================================================
// V3 Direct Message (DM) Message Tests
// ============================================================================

func TestStartDMMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  StartDMMessage
	}{
		{
			name: "target by user ID",
			msg: StartDMMessage{
				TargetType:       DMTargetByUserID,
				TargetUserID:     12345,
				AllowUnencrypted: false,
			},
		},
		{
			name: "target by nickname",
			msg: StartDMMessage{
				TargetType:       DMTargetByNickname,
				TargetNickname:   "alice",
				AllowUnencrypted: true,
			},
		},
		{
			name: "target by session ID",
			msg: StartDMMessage{
				TargetType:       DMTargetBySessionID,
				TargetUserID:     99999,
				AllowUnencrypted: false,
			},
		},
		{
			name: "target by nickname with long name",
			msg: StartDMMessage{
				TargetType:       DMTargetByNickname,
				TargetNickname:   "a_very_long_nickname_here",
				AllowUnencrypted: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &StartDMMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.TargetType, decoded.TargetType)
			assert.Equal(t, tt.msg.AllowUnencrypted, decoded.AllowUnencrypted)

			switch tt.msg.TargetType {
			case DMTargetByUserID, DMTargetBySessionID:
				assert.Equal(t, tt.msg.TargetUserID, decoded.TargetUserID)
			case DMTargetByNickname:
				assert.Equal(t, tt.msg.TargetNickname, decoded.TargetNickname)
			}
		})
	}
}

func TestProvidePublicKeyMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  ProvidePublicKeyMessage
	}{
		{
			name: "derived from SSH key",
			msg: ProvidePublicKeyMessage{
				KeyType:   KeyTypeDerivedFromSSH,
				PublicKey: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
				Label:     "laptop",
			},
		},
		{
			name: "generated key with empty label",
			msg: ProvidePublicKeyMessage{
				KeyType:   KeyTypeGenerated,
				PublicKey: [32]byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8, 0xF7, 0xF6, 0xF5, 0xF4, 0xF3, 0xF2, 0xF1, 0xF0, 0xEF, 0xEE, 0xED, 0xEC, 0xEB, 0xEA, 0xE9, 0xE8, 0xE7, 0xE6, 0xE5, 0xE4, 0xE3, 0xE2, 0xE1, 0xE0},
				Label:     "",
			},
		},
		{
			name: "ephemeral key",
			msg: ProvidePublicKeyMessage{
				KeyType:   KeyTypeEphemeral,
				PublicKey: [32]byte{},
				Label:     "session-only",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &ProvidePublicKeyMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.KeyType, decoded.KeyType)
			assert.Equal(t, tt.msg.PublicKey, decoded.PublicKey)
			assert.Equal(t, tt.msg.Label, decoded.Label)
		})
	}
}

func TestAllowUnencryptedMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  AllowUnencryptedMessage
	}{
		{
			name: "one-time allowance",
			msg: AllowUnencryptedMessage{
				DMChannelID: 12345,
				Permanent:   false,
			},
		},
		{
			name: "permanent allowance",
			msg: AllowUnencryptedMessage{
				DMChannelID: 99999,
				Permanent:   true,
			},
		},
		{
			name: "zero channel ID",
			msg: AllowUnencryptedMessage{
				DMChannelID: 0,
				Permanent:   false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &AllowUnencryptedMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.DMChannelID, decoded.DMChannelID)
			assert.Equal(t, tt.msg.Permanent, decoded.Permanent)
		})
	}
}

func TestKeyRequiredMessage(t *testing.T) {
	channelID := uint64(12345)
	tests := []struct {
		name string
		msg  KeyRequiredMessage
	}{
		{
			name: "with channel ID",
			msg: KeyRequiredMessage{
				Reason:      "DM encryption requires a key",
				DMChannelID: &channelID,
			},
		},
		{
			name: "without channel ID",
			msg: KeyRequiredMessage{
				Reason:      "Please set up encryption before starting DMs",
				DMChannelID: nil,
			},
		},
		{
			name: "long reason",
			msg: KeyRequiredMessage{
				Reason:      "This is a very long reason message that explains why a key is required for this particular DM conversation to proceed securely",
				DMChannelID: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &KeyRequiredMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.Reason, decoded.Reason)
			if tt.msg.DMChannelID != nil {
				require.NotNil(t, decoded.DMChannelID)
				assert.Equal(t, *tt.msg.DMChannelID, *decoded.DMChannelID)
			} else {
				assert.Nil(t, decoded.DMChannelID)
			}
		})
	}
}

func TestDMReadyMessage(t *testing.T) {
	userID := uint64(42)
	tests := []struct {
		name string
		msg  DMReadyMessage
	}{
		{
			name: "encrypted DM with registered user",
			msg: DMReadyMessage{
				ChannelID:      12345,
				OtherUserID:    &userID,
				OtherNickname:  "alice",
				IsEncrypted:    true,
				OtherPublicKey: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
			},
		},
		{
			name: "encrypted DM with anonymous user",
			msg: DMReadyMessage{
				ChannelID:      99999,
				OtherUserID:    nil,
				OtherNickname:  "anon_guest",
				IsEncrypted:    true,
				OtherPublicKey: [32]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99},
			},
		},
		{
			name: "unencrypted DM",
			msg: DMReadyMessage{
				ChannelID:     54321,
				OtherUserID:   &userID,
				OtherNickname: "bob",
				IsEncrypted:   false,
				// OtherPublicKey is not sent for unencrypted DMs
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &DMReadyMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.ChannelID, decoded.ChannelID)
			assert.Equal(t, tt.msg.OtherNickname, decoded.OtherNickname)
			assert.Equal(t, tt.msg.IsEncrypted, decoded.IsEncrypted)

			if tt.msg.OtherUserID != nil {
				require.NotNil(t, decoded.OtherUserID)
				assert.Equal(t, *tt.msg.OtherUserID, *decoded.OtherUserID)
			} else {
				assert.Nil(t, decoded.OtherUserID)
			}

			if tt.msg.IsEncrypted {
				assert.Equal(t, tt.msg.OtherPublicKey, decoded.OtherPublicKey)
			}
		})
	}
}

func TestDMPendingMessage(t *testing.T) {
	userID := uint64(42)
	tests := []struct {
		name string
		msg  DMPendingMessage
	}{
		{
			name: "waiting for registered user",
			msg: DMPendingMessage{
				DMChannelID:        12345,
				WaitingForUserID:   &userID,
				WaitingForNickname: "alice",
				Reason:             "Waiting for alice to accept DM request",
			},
		},
		{
			name: "waiting for anonymous user",
			msg: DMPendingMessage{
				DMChannelID:        99999,
				WaitingForUserID:   nil,
				WaitingForNickname: "guest123",
				Reason:             "Waiting for guest123 to set up encryption",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &DMPendingMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.DMChannelID, decoded.DMChannelID)
			assert.Equal(t, tt.msg.WaitingForNickname, decoded.WaitingForNickname)
			assert.Equal(t, tt.msg.Reason, decoded.Reason)

			if tt.msg.WaitingForUserID != nil {
				require.NotNil(t, decoded.WaitingForUserID)
				assert.Equal(t, *tt.msg.WaitingForUserID, *decoded.WaitingForUserID)
			} else {
				assert.Nil(t, decoded.WaitingForUserID)
			}
		})
	}
}

func TestDMRequestMessage(t *testing.T) {
	userID := uint64(42)
	tests := []struct {
		name string
		msg  DMRequestMessage
	}{
		{
			name: "encryption required from registered user",
			msg: DMRequestMessage{
				DMChannelID:      12345,
				FromUserID:       &userID,
				FromNickname:     "alice",
				EncryptionStatus: DMEncryptionRequired,
			},
		},
		{
			name: "encryption optional",
			msg: DMRequestMessage{
				DMChannelID:      99999,
				FromUserID:       &userID,
				FromNickname:     "bob",
				EncryptionStatus: DMEncryptionOptional,
			},
		},
		{
			name: "encryption not possible from anonymous user",
			msg: DMRequestMessage{
				DMChannelID:      54321,
				FromUserID:       nil,
				FromNickname:     "anon_guest",
				EncryptionStatus: DMEncryptionNotPossible,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := tt.msg.Encode()
			require.NoError(t, err)

			decoded := &DMRequestMessage{}
			err = decoded.Decode(payload)
			require.NoError(t, err)

			assert.Equal(t, tt.msg.DMChannelID, decoded.DMChannelID)
			assert.Equal(t, tt.msg.FromNickname, decoded.FromNickname)
			assert.Equal(t, tt.msg.EncryptionStatus, decoded.EncryptionStatus)

			if tt.msg.FromUserID != nil {
				require.NotNil(t, decoded.FromUserID)
				assert.Equal(t, *tt.msg.FromUserID, *decoded.FromUserID)
			} else {
				assert.Nil(t, decoded.FromUserID)
			}
		})
	}
}
