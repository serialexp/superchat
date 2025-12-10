package protocol

import (
	"bytes"
	"errors"
	"io"
	"time"
)

// ProtocolMessage interface - all protocol messages must implement this
type ProtocolMessage interface {
	// Encode serializes the message to bytes (convenience wrapper)
	Encode() ([]byte, error)
	// EncodeTo serializes the message directly to a writer (efficient)
	EncodeTo(w io.Writer) error
	// Decode deserializes the message from bytes
	Decode(payload []byte) error
}

// Message type constants (Client → Server)
const (
	TypeAuthRequest        = 0x01
	TypeSetNickname        = 0x02
	TypeRegisterUser       = 0x03
	TypeListChannels       = 0x04
	TypeJoinChannel        = 0x05
	TypeLeaveChannel       = 0x06
	TypeCreateChannel      = 0x07
	TypeCreateSubchannel   = 0x08
	TypeListMessages       = 0x09
	TypePostMessage        = 0x0A
	TypeEditMessage        = 0x0B
	TypeDeleteMessage      = 0x0C
	TypeAddSSHKey          = 0x0D
	TypeChangePassword     = 0x0E
	TypeGetUserInfo        = 0x0F
	TypeUpdateSSHKeyLabel  = 0x12
	TypeDeleteSSHKey       = 0x13
	TypeListSSHKeys        = 0x14
	TypeGetSubchannels     = 0x15
	TypeLogout             = 0x1C
	TypePing               = 0x10
	TypeDisconnect         = 0x11
	TypeListUsers          = 0x16
	TypeListChannelUsers   = 0x17
	TypeGetUnreadCounts    = 0x18
	TypeStartDM            = 0x19 // V3: Initiate direct message
	TypeProvidePublicKey   = 0x1A // V3: Upload encryption key
	TypeAllowUnencrypted   = 0x1B // V3: Allow unencrypted DM
	TypeUpdateReadState    = 0x1D
	TypeSubscribeThread    = 0x51
	TypeUnsubscribeThread  = 0x52
	TypeSubscribeChannel   = 0x53
	TypeUnsubscribeChannel = 0x54
	TypeListServers        = 0x55
	TypeRegisterServer     = 0x56
	TypeHeartbeat          = 0x57
	TypeVerifyResponse     = 0x58

	// Admin commands (Client → Server)
	TypeBanUser       = 0x59
	TypeBanIP         = 0x5A
	TypeUnbanUser     = 0x5B
	TypeUnbanIP       = 0x5C
	TypeListBans      = 0x5D
	TypeDeleteUser    = 0x5E
	TypeDeleteChannel = 0x5F
)

// Message type constants (Server → Client)
const (
	TypeAuthResponse       = 0x81
	TypeNicknameResponse   = 0x82
	TypeRegisterResponse   = 0x83
	TypeChannelList        = 0x84
	TypeJoinResponse       = 0x85
	TypeLeaveResponse      = 0x86
	TypeChannelCreated     = 0x87
	TypeSubchannelCreated  = 0x88
	TypeMessageList        = 0x89
	TypeMessagePosted      = 0x8A
	TypeMessageEdited      = 0x8B
	TypeMessageDeleted     = 0x8C
	TypeNewMessage         = 0x8D
	TypePasswordChanged    = 0x8E
	TypeUserInfo           = 0x8F
	TypePong               = 0x90
	TypeError              = 0x91
	TypeSSHKeyLabelUpdated = 0x92
	TypeSSHKeyDeleted      = 0x93
	TypeSSHKeyList         = 0x94
	TypeSSHKeyAdded        = 0x95
	TypeSubchannelList     = 0x96
	TypeUnreadCounts       = 0x97
	TypeServerConfig       = 0x98
	TypeSubscribeOk        = 0x99
	TypeUserList           = 0x9A
	TypeServerList         = 0x9B
	TypeRegisterAck        = 0x9C
	TypeHeartbeatAck       = 0x9D
	TypeVerifyRegistration = 0x9E

	// V3 DM responses (Server → Client)
	TypeKeyRequired = 0xA1 // Server needs encryption key
	TypeDMReady     = 0xA2 // DM channel is ready
	TypeDMPending   = 0xA3 // Waiting for other party
	TypeDMRequest   = 0xA4 // Incoming DM request

	TypeChannelUserList = 0xAB
	TypeChannelPresence = 0xAC
	TypeServerPresence  = 0xAD

	// Admin responses (Server → Client)
	TypeUserBanned = 0x9F
	TypeIPBanned       = 0xA5
	TypeUserUnbanned   = 0xA6
	TypeIPUnbanned     = 0xA7
	TypeBanList        = 0xA8
	TypeUserDeleted    = 0xA9
	TypeChannelDeleted = 0xAA
)

// Error codes
const (
	// Protocol errors (1xxx)
	ErrCodeInvalidFormat      = 1000
	ErrCodeUnsupportedVersion = 1001
	ErrCodeInvalidFrame       = 1002

	// Authentication errors (2xxx)
	ErrCodeAuthRequired = 2000

	// Authorization errors (3xxx)
	ErrCodePermissionDenied = 3000

	// Resource errors (4xxx)
	ErrCodeNotFound           = 4000
	ErrCodeChannelNotFound    = 4001
	ErrCodeMessageNotFound    = 4002
	ErrCodeThreadNotFound     = 4003
	ErrCodeSubchannelNotFound = 4004

	// Rate limit errors (5xxx)
	ErrCodeRateLimitExceeded        = 5000
	ErrCodeMessageRateLimit         = 5001
	ErrCodeThreadSubscriptionLimit  = 5004
	ErrCodeChannelSubscriptionLimit = 5005

	// Validation errors (6xxx)
	ErrCodeInvalidInput     = 6000
	ErrCodeMessageTooLong   = 6001
	ErrCodeInvalidNickname  = 6003
	ErrCodeNicknameRequired = 6004

	// Server errors (9xxx)
	ErrCodeInternalError = 9000
	ErrCodeDatabaseError = 9001
)

var (
	ErrNicknameTooShort = errors.New("nickname must be at least 3 characters")
	ErrNicknameTooLong  = errors.New("nickname must be at most 20 characters")
	ErrMessageTooLong   = errors.New("message content exceeds maximum length (4096 bytes)")
	ErrEmptyContent     = errors.New("message content cannot be empty")
)

// AuthRequestMessage (0x01) - Authenticate with password
type AuthRequestMessage struct {
	Nickname string
	Password string
}

func (m *AuthRequestMessage) EncodeTo(w io.Writer) error {
	if err := WriteString(w, m.Nickname); err != nil {
		return err
	}
	return WriteString(w, m.Password)
}

func (m *AuthRequestMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *AuthRequestMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	nickname, err := ReadString(buf)
	if err != nil {
		return err
	}
	password, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Nickname = nickname
	m.Password = password
	return nil
}

// AuthResponseMessage (0x81) - Authentication result
type AuthResponseMessage struct {
	Success   bool
	UserID    uint64 // Only present if success=true
	Nickname  string // Only present if success=true
	Message   string
	UserFlags *UserFlags // Optional: present when Success=true and server includes flags
}

func (m *AuthResponseMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if m.Success {
		if err := WriteUint64(w, m.UserID); err != nil {
			return err
		}
		if err := WriteString(w, m.Nickname); err != nil {
			return err
		}
	}
	if err := WriteString(w, m.Message); err != nil {
		return err
	}
	if m.Success && m.UserFlags != nil {
		if err := WriteUint8(w, uint8(*m.UserFlags)); err != nil {
			return err
		}
	}
	return nil
}

func (m *AuthResponseMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *AuthResponseMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.UserFlags = nil

	if success {
		userID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		m.UserID = userID

		nickname, err := ReadString(buf)
		if err != nil {
			return err
		}
		m.Nickname = nickname
	}

	message, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Message = message

	if success && buf.Len() > 0 {
		flags, err := ReadUint8(buf)
		if err != nil {
			return err
		}
		f := UserFlags(flags)
		m.UserFlags = &f
	}

	return nil
}

// SetNicknameMessage (0x02) - Set/change nickname
type SetNicknameMessage struct {
	Nickname string
}

func (m *SetNicknameMessage) EncodeTo(w io.Writer) error {
	// Validate nickname
	if len(m.Nickname) < 3 {
		return ErrNicknameTooShort
	}
	if len(m.Nickname) > 20 {
		return ErrNicknameTooLong
	}

	return WriteString(w, m.Nickname)
}

func (m *SetNicknameMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SetNicknameMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	nickname, err := ReadString(buf)
	if err != nil {
		return err
	}

	// Validate nickname
	if len(nickname) < 3 {
		return ErrNicknameTooShort
	}
	if len(nickname) > 20 {
		return ErrNicknameTooLong
	}

	m.Nickname = nickname
	return nil
}

// NicknameResponseMessage (0x82) - Response to SET_NICKNAME
type NicknameResponseMessage struct {
	Success bool
	Message string
}

func (m *NicknameResponseMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	return WriteString(w, m.Message)
}

func (m *NicknameResponseMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *NicknameResponseMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	message, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.Message = message
	return nil
}

// RegisterUserMessage (0x03) - Register current nickname with password
type RegisterUserMessage struct {
	Password string
}

func (m *RegisterUserMessage) EncodeTo(w io.Writer) error {
	return WriteString(w, m.Password)
}

func (m *RegisterUserMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *RegisterUserMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	password, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Password = password
	return nil
}

// RegisterResponseMessage (0x83) - Registration result
type RegisterResponseMessage struct {
	Success bool
	UserID  uint64 // Only present if success=true
	Message string
}

func (m *RegisterResponseMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if m.Success {
		if err := WriteUint64(w, m.UserID); err != nil {
			return err
		}
	}
	return WriteString(w, m.Message)
}

func (m *RegisterResponseMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *RegisterResponseMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}

	m.Success = success

	if success {
		userID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		m.UserID = userID
	}

	message, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Message = message

	return nil
}

// LogoutMessage (0x15) - Clear authentication and become anonymous
type LogoutMessage struct{}

func (m *LogoutMessage) EncodeTo(w io.Writer) error {
	// Empty message
	return nil
}

func (m *LogoutMessage) Encode() ([]byte, error) {
	return []byte{}, nil
}

func (m *LogoutMessage) Decode(payload []byte) error {
	// Empty message - nothing to decode
	return nil
}

// ListChannelsMessage (0x04) - Request channel list
type ListChannelsMessage struct {
	FromChannelID uint64
	Limit         uint16
}

func (m *ListChannelsMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.FromChannelID); err != nil {
		return err
	}
	return WriteUint16(w, m.Limit)
}

func (m *ListChannelsMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ListChannelsMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	fromID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	limit, err := ReadUint16(buf)
	if err != nil {
		return err
	}

	m.FromChannelID = fromID
	m.Limit = limit
	return nil
}

// Channel represents a channel in CHANNEL_LIST
type Channel struct {
	ID              uint64
	Name            string
	Description     string
	UserCount       uint32
	IsOperator      bool
	Type            uint8
	RetentionHours  uint32
	HasSubchannels  bool   // V3: true if channel has subchannels
	SubchannelCount uint16 // V3: number of subchannels
}

// ChannelListMessage (0x84) - List of channels
type ChannelListMessage struct {
	Channels []Channel
}

func (m *ChannelListMessage) EncodeTo(w io.Writer) error {
	// Write channel count
	if err := WriteUint16(w, uint16(len(m.Channels))); err != nil {
		return err
	}

	// Write each channel
	for _, ch := range m.Channels {
		if err := WriteUint64(w, ch.ID); err != nil {
			return err
		}
		if err := WriteString(w, ch.Name); err != nil {
			return err
		}
		if err := WriteString(w, ch.Description); err != nil {
			return err
		}
		if err := WriteUint32(w, ch.UserCount); err != nil {
			return err
		}
		if err := WriteBool(w, ch.IsOperator); err != nil {
			return err
		}
		if err := WriteUint8(w, ch.Type); err != nil {
			return err
		}
		if err := WriteUint32(w, ch.RetentionHours); err != nil {
			return err
		}
		// V3: subchannel info
		if err := WriteBool(w, ch.HasSubchannels); err != nil {
			return err
		}
		if err := WriteUint16(w, ch.SubchannelCount); err != nil {
			return err
		}
	}

	return nil
}

func (m *ChannelListMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ChannelListMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)

	// Read channel count
	count, err := ReadUint16(buf)
	if err != nil {
		return err
	}

	// Read each channel
	m.Channels = make([]Channel, count)
	for i := uint16(0); i < count; i++ {
		id, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		name, err := ReadString(buf)
		if err != nil {
			return err
		}
		desc, err := ReadString(buf)
		if err != nil {
			return err
		}
		userCount, err := ReadUint32(buf)
		if err != nil {
			return err
		}
		isOp, err := ReadBool(buf)
		if err != nil {
			return err
		}
		chType, err := ReadUint8(buf)
		if err != nil {
			return err
		}
		retention, err := ReadUint32(buf)
		if err != nil {
			return err
		}
		// V3: subchannel info
		hasSubchannels, err := ReadBool(buf)
		if err != nil {
			return err
		}
		subchannelCount, err := ReadUint16(buf)
		if err != nil {
			return err
		}

		m.Channels[i] = Channel{
			ID:              id,
			Name:            name,
			Description:     desc,
			UserCount:       userCount,
			IsOperator:      isOp,
			Type:            chType,
			RetentionHours:  retention,
			HasSubchannels:  hasSubchannels,
			SubchannelCount: subchannelCount,
		}
	}

	return nil
}

// JoinChannelMessage (0x05) - Join a channel
type JoinChannelMessage struct {
	ChannelID    uint64
	SubchannelID *uint64 // V1: always nil (no subchannels)
}

func (m *JoinChannelMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.SubchannelID)
}

func (m *JoinChannelMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *JoinChannelMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}

	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	return nil
}

// LeaveChannelMessage (0x06) - Leave a channel
type LeaveChannelMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
}

func (m *LeaveChannelMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.SubchannelID)
}

func (m *LeaveChannelMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *LeaveChannelMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}

	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	return nil
}

// JoinResponseMessage (0x85) - Response to JOIN_CHANNEL
type JoinResponseMessage struct {
	Success      bool
	ChannelID    uint64
	SubchannelID *uint64
	Message      string
}

func (m *JoinResponseMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.SubchannelID); err != nil {
		return err
	}
	return WriteString(w, m.Message)
}

func (m *JoinResponseMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *JoinResponseMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	message, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	m.Message = message
	return nil
}

// LeaveResponseMessage (0x86) - Response to LEAVE_CHANNEL
type LeaveResponseMessage struct {
	Success      bool
	ChannelID    uint64
	SubchannelID *uint64
	Message      string
}

func (m *LeaveResponseMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.SubchannelID); err != nil {
		return err
	}
	return WriteString(w, m.Message)
}

func (m *LeaveResponseMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *LeaveResponseMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	message, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	m.Message = message
	return nil
}

// CreateChannelMessage (0x07) - Create a new channel (V2+, requires registered user)
type CreateChannelMessage struct {
	Name           string // URL-friendly name (e.g., "general", "random")
	DisplayName    string // Human-readable name (e.g., "#general", "#random")
	Description    *string
	ChannelType    uint8  // 1=forum, 2=chat (V2+ only supports forum)
	RetentionHours uint32 // Message retention in hours
}

func (m *CreateChannelMessage) EncodeTo(w io.Writer) error {
	if err := WriteString(w, m.Name); err != nil {
		return err
	}
	if err := WriteString(w, m.DisplayName); err != nil {
		return err
	}
	if err := WriteOptionalString(w, m.Description); err != nil {
		return err
	}
	if err := WriteUint8(w, m.ChannelType); err != nil {
		return err
	}
	return WriteUint32(w, m.RetentionHours)
}

func (m *CreateChannelMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *CreateChannelMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	name, err := ReadString(buf)
	if err != nil {
		return err
	}
	displayName, err := ReadString(buf)
	if err != nil {
		return err
	}
	description, err := ReadOptionalString(buf)
	if err != nil {
		return err
	}
	channelType, err := ReadUint8(buf)
	if err != nil {
		return err
	}
	retentionHours, err := ReadUint32(buf)
	if err != nil {
		return err
	}

	m.Name = name
	m.DisplayName = displayName
	m.Description = description
	m.ChannelType = channelType
	m.RetentionHours = retentionHours
	return nil
}

// ChannelCreatedMessage (0x87) - Response to CREATE_CHANNEL + broadcast to all connected clients
// Hybrid message: sent to creator as confirmation, also broadcast to all others if success=true
type ChannelCreatedMessage struct {
	Success        bool
	ChannelID      uint64 // Only present if Success=true
	Name           string // Only present if Success=true
	Description    string // Only present if Success=true
	Type           uint8  // Only present if Success=true
	RetentionHours uint32 // Only present if Success=true
	Message        string // Error if failed, confirmation if success
}

func (m *ChannelCreatedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}

	// Only write channel data if success=true
	if m.Success {
		if err := WriteUint64(w, m.ChannelID); err != nil {
			return err
		}
		if err := WriteString(w, m.Name); err != nil {
			return err
		}
		if err := WriteString(w, m.Description); err != nil {
			return err
		}
		if err := WriteUint8(w, m.Type); err != nil {
			return err
		}
		if err := WriteUint32(w, m.RetentionHours); err != nil {
			return err
		}
	}

	return WriteString(w, m.Message)
}

func (m *ChannelCreatedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ChannelCreatedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}

	m.Success = success

	// Only read channel data if success=true
	if success {
		channelID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		name, err := ReadString(buf)
		if err != nil {
			return err
		}
		description, err := ReadString(buf)
		if err != nil {
			return err
		}
		channelType, err := ReadUint8(buf)
		if err != nil {
			return err
		}
		retentionHours, err := ReadUint32(buf)
		if err != nil {
			return err
		}

		m.ChannelID = channelID
		m.Name = name
		m.Description = description
		m.Type = channelType
		m.RetentionHours = retentionHours
	}

	message, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Message = message

	return nil
}

// CreateSubchannelMessage (0x08) - Create a new subchannel within a channel
type CreateSubchannelMessage struct {
	ChannelID      uint64 // Parent channel ID
	Name           string // URL-friendly name
	Description    string
	Type           uint8  // 0=chat, 1=forum
	RetentionHours uint32
}

func (m *CreateSubchannelMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteString(w, m.Name); err != nil {
		return err
	}
	if err := WriteString(w, m.Description); err != nil {
		return err
	}
	if err := WriteUint8(w, m.Type); err != nil {
		return err
	}
	return WriteUint32(w, m.RetentionHours)
}

func (m *CreateSubchannelMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *CreateSubchannelMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	name, err := ReadString(buf)
	if err != nil {
		return err
	}
	description, err := ReadString(buf)
	if err != nil {
		return err
	}
	channelType, err := ReadUint8(buf)
	if err != nil {
		return err
	}
	retentionHours, err := ReadUint32(buf)
	if err != nil {
		return err
	}

	m.ChannelID = channelID
	m.Name = name
	m.Description = description
	m.Type = channelType
	m.RetentionHours = retentionHours
	return nil
}

// SubchannelCreatedMessage (0x88) - Response to CREATE_SUBCHANNEL + broadcast
type SubchannelCreatedMessage struct {
	Success        bool
	ChannelID      uint64 // Parent channel ID (only if success)
	SubchannelID   uint64 // New subchannel ID (only if success)
	Name           string // Only if success
	Description    string // Only if success
	Type           uint8  // Only if success
	RetentionHours uint32 // Only if success
	Message        string // Error or confirmation message
}

func (m *SubchannelCreatedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if m.Success {
		if err := WriteUint64(w, m.ChannelID); err != nil {
			return err
		}
		if err := WriteUint64(w, m.SubchannelID); err != nil {
			return err
		}
		if err := WriteString(w, m.Name); err != nil {
			return err
		}
		if err := WriteString(w, m.Description); err != nil {
			return err
		}
		if err := WriteUint8(w, m.Type); err != nil {
			return err
		}
		if err := WriteUint32(w, m.RetentionHours); err != nil {
			return err
		}
	}
	return WriteString(w, m.Message)
}

func (m *SubchannelCreatedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SubchannelCreatedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.Success = success

	if success {
		channelID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		subchannelID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		name, err := ReadString(buf)
		if err != nil {
			return err
		}
		description, err := ReadString(buf)
		if err != nil {
			return err
		}
		channelType, err := ReadUint8(buf)
		if err != nil {
			return err
		}
		retentionHours, err := ReadUint32(buf)
		if err != nil {
			return err
		}

		m.ChannelID = channelID
		m.SubchannelID = subchannelID
		m.Name = name
		m.Description = description
		m.Type = channelType
		m.RetentionHours = retentionHours
	}

	message, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Message = message
	return nil
}

// GetSubchannelsMessage (0x15) - Request subchannels for a channel
type GetSubchannelsMessage struct {
	ChannelID uint64
}

func (m *GetSubchannelsMessage) EncodeTo(w io.Writer) error {
	return WriteUint64(w, m.ChannelID)
}

func (m *GetSubchannelsMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *GetSubchannelsMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	m.ChannelID = channelID
	return nil
}

// SubchannelInfo represents a single subchannel in a list
type SubchannelInfo struct {
	ID             uint64
	Name           string
	Description    string
	Type           uint8
	RetentionHours uint32
}

// SubchannelListMessage (0x96) - List of subchannels for a channel
type SubchannelListMessage struct {
	ChannelID   uint64
	Subchannels []SubchannelInfo
}

func (m *SubchannelListMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteUint16(w, uint16(len(m.Subchannels))); err != nil {
		return err
	}
	for _, sub := range m.Subchannels {
		if err := WriteUint64(w, sub.ID); err != nil {
			return err
		}
		if err := WriteString(w, sub.Name); err != nil {
			return err
		}
		if err := WriteString(w, sub.Description); err != nil {
			return err
		}
		if err := WriteUint8(w, sub.Type); err != nil {
			return err
		}
		if err := WriteUint32(w, sub.RetentionHours); err != nil {
			return err
		}
	}
	return nil
}

func (m *SubchannelListMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SubchannelListMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	count, err := ReadUint16(buf)
	if err != nil {
		return err
	}

	m.ChannelID = channelID
	m.Subchannels = make([]SubchannelInfo, count)

	for i := uint16(0); i < count; i++ {
		id, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		name, err := ReadString(buf)
		if err != nil {
			return err
		}
		description, err := ReadString(buf)
		if err != nil {
			return err
		}
		subType, err := ReadUint8(buf)
		if err != nil {
			return err
		}
		retentionHours, err := ReadUint32(buf)
		if err != nil {
			return err
		}

		m.Subchannels[i] = SubchannelInfo{
			ID:             id,
			Name:           name,
			Description:    description,
			Type:           subType,
			RetentionHours: retentionHours,
		}
	}
	return nil
}

// ListMessagesMessage (0x09) - Request messages
type ListMessagesMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
	Limit        uint16
	BeforeID     *uint64
	ParentID     *uint64
	AfterID      *uint64
}

func (m *ListMessagesMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.SubchannelID); err != nil {
		return err
	}
	if err := WriteUint16(w, m.Limit); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.BeforeID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.ParentID); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.AfterID)
}

func (m *ListMessagesMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ListMessagesMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	limit, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	beforeID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	parentID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	afterID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}

	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	m.Limit = limit
	m.BeforeID = beforeID
	m.ParentID = parentID
	m.AfterID = afterID
	return nil
}

// Message represents a single message
type Message struct {
	ID             uint64
	ChannelID      uint64
	SubchannelID   *uint64
	ParentID       *uint64
	AuthorUserID   *uint64
	AuthorNickname string // Only populated for anonymous users (when AuthorUserID IS NULL)
	Content        string
	CreatedAt      time.Time
	EditedAt       *time.Time
	ReplyCount     uint32
}

// MessageListMessage (0x89) - List of messages
type MessageListMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
	ParentID     *uint64
	Messages     []Message
}

func (m *MessageListMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.SubchannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.ParentID); err != nil {
		return err
	}
	if err := WriteUint16(w, uint16(len(m.Messages))); err != nil {
		return err
	}

	for _, msg := range m.Messages {
		if err := WriteUint64(w, msg.ID); err != nil {
			return err
		}
		if err := WriteUint64(w, msg.ChannelID); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, msg.SubchannelID); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, msg.ParentID); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, msg.AuthorUserID); err != nil {
			return err
		}
		if err := WriteString(w, msg.AuthorNickname); err != nil {
			return err
		}
		if err := WriteString(w, msg.Content); err != nil {
			return err
		}
		if err := WriteTimestamp(w, msg.CreatedAt); err != nil {
			return err
		}
		if err := WriteOptionalTimestamp(w, msg.EditedAt); err != nil {
			return err
		}
		if err := WriteUint32(w, msg.ReplyCount); err != nil {
			return err
		}
	}

	return nil
}

func (m *MessageListMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *MessageListMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)

	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	parentID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	count, err := ReadUint16(buf)
	if err != nil {
		return err
	}

	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	m.ParentID = parentID
	m.Messages = make([]Message, count)

	for i := uint16(0); i < count; i++ {
		id, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		chID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		subID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		parID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		authorID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		authorNick, err := ReadString(buf)
		if err != nil {
			return err
		}
		content, err := ReadString(buf)
		if err != nil {
			return err
		}
		createdAt, err := ReadTimestamp(buf)
		if err != nil {
			return err
		}
		editedAt, err := ReadOptionalTimestamp(buf)
		if err != nil {
			return err
		}
		replyCount, err := ReadUint32(buf)
		if err != nil {
			return err
		}

		m.Messages[i] = Message{
			ID:             id,
			ChannelID:      chID,
			SubchannelID:   subID,
			ParentID:       parID,
			AuthorUserID:   authorID,
			AuthorNickname: authorNick,
			Content:        content,
			CreatedAt:      createdAt,
			EditedAt:       editedAt,
			ReplyCount:     replyCount,
		}
	}

	return nil
}

// PostMessageMessage (0x0A) - Post a new message
type PostMessageMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
	ParentID     *uint64
	Content      string
}

func (m *PostMessageMessage) EncodeTo(w io.Writer) error {
	// Validate content
	if len(m.Content) == 0 {
		return ErrEmptyContent
	}
	if len(m.Content) > 4096 {
		return ErrMessageTooLong
	}

	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.SubchannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.ParentID); err != nil {
		return err
	}
	return WriteString(w, m.Content)
}

func (m *PostMessageMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *PostMessageMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	parentID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	content, err := ReadString(buf)
	if err != nil {
		return err
	}

	// Validate content
	if len(content) == 0 {
		return ErrEmptyContent
	}
	if len(content) > 4096 {
		return ErrMessageTooLong
	}

	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	m.ParentID = parentID
	m.Content = content
	return nil
}

// MessagePostedMessage (0x8A) - Confirmation of message post
type MessagePostedMessage struct {
	Success   bool
	MessageID uint64
	Message   string
}

func (m *MessagePostedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if err := WriteUint64(w, m.MessageID); err != nil {
		return err
	}
	return WriteString(w, m.Message)
}

func (m *MessagePostedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *MessagePostedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	messageID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	message, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.MessageID = messageID
	m.Message = message
	return nil
}

// EditMessageMessage (0x0B) - Edit an existing message
type EditMessageMessage struct {
	MessageID  uint64
	NewContent string
}

func (m *EditMessageMessage) EncodeTo(w io.Writer) error {
	// Validate content
	if len(m.NewContent) == 0 {
		return ErrEmptyContent
	}
	if len(m.NewContent) > 4096 {
		return ErrMessageTooLong
	}

	if err := WriteUint64(w, m.MessageID); err != nil {
		return err
	}
	return WriteString(w, m.NewContent)
}

func (m *EditMessageMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *EditMessageMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	messageID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	content, err := ReadString(buf)
	if err != nil {
		return err
	}

	// Validate content
	if len(content) == 0 {
		return ErrEmptyContent
	}
	if len(content) > 4096 {
		return ErrMessageTooLong
	}

	m.MessageID = messageID
	m.NewContent = content
	return nil
}

// MessageEditedMessage (0x8B) - Edit confirmation + real-time broadcast
type MessageEditedMessage struct {
	Success    bool
	MessageID  uint64
	EditedAt   time.Time
	NewContent string
	Message    string // Error message if failed
}

func (m *MessageEditedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if err := WriteUint64(w, m.MessageID); err != nil {
		return err
	}
	if m.Success {
		if err := WriteTimestamp(w, m.EditedAt); err != nil {
			return err
		}
		if err := WriteString(w, m.NewContent); err != nil {
			return err
		}
	}
	return WriteString(w, m.Message)
}

func (m *MessageEditedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *MessageEditedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	messageID, err := ReadUint64(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.MessageID = messageID

	if success {
		editedAt, err := ReadTimestamp(buf)
		if err != nil {
			return err
		}
		newContent, err := ReadString(buf)
		if err != nil {
			return err
		}
		m.EditedAt = editedAt
		m.NewContent = newContent
	}

	message, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Message = message

	return nil
}

// DeleteMessageMessage (0x0C) - Delete a message
type DeleteMessageMessage struct {
	MessageID uint64
}

func (m *DeleteMessageMessage) EncodeTo(w io.Writer) error {
	return WriteUint64(w, m.MessageID)
}

func (m *DeleteMessageMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *DeleteMessageMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	messageID, err := ReadUint64(buf)
	if err != nil {
		return err
	}

	m.MessageID = messageID
	return nil
}

// MessageDeletedMessage (0x8C) - Confirmation of deletion + broadcast
type MessageDeletedMessage struct {
	Success   bool
	MessageID uint64
	DeletedAt time.Time
	Message   string
}

func (m *MessageDeletedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if err := WriteUint64(w, m.MessageID); err != nil {
		return err
	}
	if m.Success {
		if err := WriteTimestamp(w, m.DeletedAt); err != nil {
			return err
		}
	}
	return WriteString(w, m.Message)
}

func (m *MessageDeletedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *MessageDeletedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	messageID, err := ReadUint64(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.MessageID = messageID

	if success {
		deletedAt, err := ReadTimestamp(buf)
		if err != nil {
			return err
		}
		m.DeletedAt = deletedAt
	}

	message, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Message = message

	return nil
}

// PingMessage (0x10) - Keepalive ping
type PingMessage struct {
	Timestamp int64
}

func (m *PingMessage) EncodeTo(w io.Writer) error {
	return WriteInt64(w, m.Timestamp)
}

func (m *PingMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *PingMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	timestamp, err := ReadInt64(buf)
	if err != nil {
		return err
	}

	m.Timestamp = timestamp
	return nil
}

// PongMessage (0x90) - Ping response
type PongMessage struct {
	ClientTimestamp int64
}

func (m *PongMessage) EncodeTo(w io.Writer) error {
	return WriteInt64(w, m.ClientTimestamp)
}

func (m *PongMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *PongMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	timestamp, err := ReadInt64(buf)
	if err != nil {
		return err
	}

	m.ClientTimestamp = timestamp
	return nil
}

// ErrorMessage (0x91) - Generic error response
type ErrorMessage struct {
	ErrorCode uint16
	Message   string
}

func (m *ErrorMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint16(w, m.ErrorCode); err != nil {
		return err
	}
	return WriteString(w, m.Message)
}

func (m *ErrorMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ErrorMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	errorCode, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	message, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.ErrorCode = errorCode
	m.Message = message
	return nil
}

// ServerConfigMessage (0x98) - Server configuration and limits
type ServerConfigMessage struct {
	ProtocolVersion         uint8
	MaxMessageRate          uint16
	MaxChannelCreates       uint16
	InactiveCleanupDays     uint16
	MaxConnectionsPerIP     uint8
	MaxMessageLength        uint32
	MaxThreadSubscriptions  uint16
	MaxChannelSubscriptions uint16
	DirectoryEnabled        bool
}

func (m *ServerConfigMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint8(w, m.ProtocolVersion); err != nil {
		return err
	}
	if err := WriteUint16(w, m.MaxMessageRate); err != nil {
		return err
	}
	if err := WriteUint16(w, m.MaxChannelCreates); err != nil {
		return err
	}
	if err := WriteUint16(w, m.InactiveCleanupDays); err != nil {
		return err
	}
	if err := WriteUint8(w, m.MaxConnectionsPerIP); err != nil {
		return err
	}
	if err := WriteUint32(w, m.MaxMessageLength); err != nil {
		return err
	}
	if err := WriteUint16(w, m.MaxThreadSubscriptions); err != nil {
		return err
	}
	if err := WriteUint16(w, m.MaxChannelSubscriptions); err != nil {
		return err
	}
	return WriteBool(w, m.DirectoryEnabled)
}

func (m *ServerConfigMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ServerConfigMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	protocolVersion, err := ReadUint8(buf)
	if err != nil {
		return err
	}
	maxMessageRate, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	maxChannelCreates, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	inactiveCleanup, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	maxConnsPerIP, err := ReadUint8(buf)
	if err != nil {
		return err
	}
	maxMsgLen, err := ReadUint32(buf)
	if err != nil {
		return err
	}
	maxThreadSubs, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	maxChannelSubs, err := ReadUint16(buf)
	if err != nil {
		return err
	}

	m.ProtocolVersion = protocolVersion
	m.MaxMessageRate = maxMessageRate
	m.MaxChannelCreates = maxChannelCreates
	m.InactiveCleanupDays = inactiveCleanup
	m.MaxConnectionsPerIP = maxConnsPerIP
	m.MaxMessageLength = maxMsgLen
	m.MaxThreadSubscriptions = maxThreadSubs
	m.MaxChannelSubscriptions = maxChannelSubs

	directoryEnabled, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.DirectoryEnabled = directoryEnabled

	return nil
}

// NewMessageMessage (0x8D) - Real-time new message broadcast
// Uses the same format as Message in MESSAGE_LIST
type NewMessageMessage Message

func (m *NewMessageMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ID); err != nil {
		return err
	}
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.SubchannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.ParentID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.AuthorUserID); err != nil {
		return err
	}
	if err := WriteString(w, m.AuthorNickname); err != nil {
		return err
	}
	if err := WriteString(w, m.Content); err != nil {
		return err
	}
	if err := WriteTimestamp(w, m.CreatedAt); err != nil {
		return err
	}
	if err := WriteOptionalTimestamp(w, m.EditedAt); err != nil {
		return err
	}
	return WriteUint32(w, m.ReplyCount)
}

func (m *NewMessageMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *NewMessageMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)

	id, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	parentID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	authorUserID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	authorNickname, err := ReadString(buf)
	if err != nil {
		return err
	}
	content, err := ReadString(buf)
	if err != nil {
		return err
	}
	createdAt, err := ReadTimestamp(buf)
	if err != nil {
		return err
	}
	editedAt, err := ReadOptionalTimestamp(buf)
	if err != nil {
		return err
	}
	replyCount, err := ReadUint32(buf)
	if err != nil {
		return err
	}

	m.ID = id
	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	m.ParentID = parentID
	m.AuthorUserID = authorUserID
	m.AuthorNickname = authorNickname
	m.Content = content
	m.CreatedAt = createdAt
	m.EditedAt = editedAt
	m.ReplyCount = replyCount

	return nil
}

// DisconnectMessage (0x11) - Graceful disconnect notification
// Can be sent by either client or server to signal intentional disconnect
type DisconnectMessage struct {
	Reason *string // Optional reason for disconnect
}

func (m *DisconnectMessage) EncodeTo(w io.Writer) error {
	return WriteOptionalString(w, m.Reason)
}

func (m *DisconnectMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *DisconnectMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	reason, err := ReadOptionalString(buf)
	if err != nil {
		return err
	}
	m.Reason = reason
	return nil
}

// SubscribeThreadMessage (0x51) - Subscribe to a thread
type SubscribeThreadMessage struct {
	ThreadID uint64
}

func (m *SubscribeThreadMessage) EncodeTo(w io.Writer) error {
	return WriteUint64(w, m.ThreadID)
}

func (m *SubscribeThreadMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SubscribeThreadMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	threadID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	m.ThreadID = threadID
	return nil
}

// UnsubscribeThreadMessage (0x52) - Unsubscribe from a thread
type UnsubscribeThreadMessage struct {
	ThreadID uint64
}

func (m *UnsubscribeThreadMessage) EncodeTo(w io.Writer) error {
	return WriteUint64(w, m.ThreadID)
}

func (m *UnsubscribeThreadMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UnsubscribeThreadMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	threadID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	m.ThreadID = threadID
	return nil
}

// SubscribeChannelMessage (0x53) - Subscribe to a channel/subchannel
type SubscribeChannelMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
}

func (m *SubscribeChannelMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.SubchannelID)
}

func (m *SubscribeChannelMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SubscribeChannelMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	return nil
}

// UnsubscribeChannelMessage (0x54) - Unsubscribe from a channel/subchannel
type UnsubscribeChannelMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
}

func (m *UnsubscribeChannelMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.SubchannelID)
}

func (m *UnsubscribeChannelMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UnsubscribeChannelMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	return nil
}

// SubscribeOkMessage (0x99) - Subscription confirmed
type SubscribeOkMessage struct {
	Type         uint8   // Type of subscription: 1=thread, 2=channel
	ID           uint64  // thread_id or channel_id depending on Type
	SubchannelID *uint64 // Present only for channel subscriptions (Type=2)
}

func (m *SubscribeOkMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint8(w, m.Type); err != nil {
		return err
	}
	if err := WriteUint64(w, m.ID); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.SubchannelID)
}

func (m *SubscribeOkMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SubscribeOkMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	subType, err := ReadUint8(buf)
	if err != nil {
		return err
	}
	id, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	m.Type = subType
	m.ID = id
	m.SubchannelID = subchannelID
	return nil
}

// GetUserInfoMessage (0x0F) - Request user information by nickname
type GetUserInfoMessage struct {
	Nickname string
}

func (m *GetUserInfoMessage) EncodeTo(w io.Writer) error {
	return WriteString(w, m.Nickname)
}

func (m *GetUserInfoMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *GetUserInfoMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	nickname, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Nickname = nickname
	return nil
}

// UserInfoMessage (0x8F) - User information response
type UserInfoMessage struct {
	Nickname     string
	IsRegistered bool
	UserID       *uint64 // Only present if IsRegistered = true
	Online       bool
}

func (m *UserInfoMessage) EncodeTo(w io.Writer) error {
	if err := WriteString(w, m.Nickname); err != nil {
		return err
	}
	if err := WriteBool(w, m.IsRegistered); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.UserID); err != nil {
		return err
	}
	return WriteBool(w, m.Online)
}

func (m *UserInfoMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UserInfoMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	nickname, err := ReadString(buf)
	if err != nil {
		return err
	}
	isRegistered, err := ReadBool(buf)
	if err != nil {
		return err
	}
	userID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	online, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.Nickname = nickname
	m.IsRegistered = isRegistered
	m.UserID = userID
	m.Online = online
	return nil
}

// ListUsersMessage (0x16) - Request list of online or all users
type ListUsersMessage struct {
	Limit          uint16
	IncludeOffline bool // Optional field - admin only
}

func (m *ListUsersMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint16(w, m.Limit); err != nil {
		return err
	}
	// Write include_offline flag (optional, defaults to false)
	return WriteBool(w, m.IncludeOffline)
}

func (m *ListUsersMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ListUsersMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	limit, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	m.Limit = limit

	// IncludeOffline is optional - if not present, defaults to false
	includeOffline, err := ReadBool(buf)
	if err != nil && err != io.EOF {
		return err
	}
	if err == io.EOF {
		// Field not present (backwards compatible with older clients)
		m.IncludeOffline = false
	} else {
		m.IncludeOffline = includeOffline
	}

	return nil
}

// UserListEntry represents a single user in the user list
type UserListEntry struct {
	Nickname     string
	IsRegistered bool
	UserID       *uint64 // Only present if IsRegistered = true
	Online       bool    // True if user has an active session
}

// UserListMessage (0x9A) - List of online users response
type UserListMessage struct {
	Users []UserListEntry
}

func (m *UserListMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint16(w, uint16(len(m.Users))); err != nil {
		return err
	}
	for _, user := range m.Users {
		if err := WriteString(w, user.Nickname); err != nil {
			return err
		}
		if err := WriteBool(w, user.IsRegistered); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, user.UserID); err != nil {
			return err
		}
		if err := WriteBool(w, user.Online); err != nil {
			return err
		}
	}
	return nil
}

func (m *UserListMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UserListMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	userCount, err := ReadUint16(buf)
	if err != nil {
		return err
	}

	users := make([]UserListEntry, userCount)
	for i := uint16(0); i < userCount; i++ {
		nickname, err := ReadString(buf)
		if err != nil {
			return err
		}
		isRegistered, err := ReadBool(buf)
		if err != nil {
			return err
		}
		userID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		online, err := ReadBool(buf)
		if err != nil {
			return err
		}
		users[i] = UserListEntry{
			Nickname:     nickname,
			IsRegistered: isRegistered,
			UserID:       userID,
			Online:       online,
		}
	}

	m.Users = users
	return nil
}

// ListChannelUsersMessage (0x17) - Request list of users in a channel
type ListChannelUsersMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
}

func (m *ListChannelUsersMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.SubchannelID)
}

func (m *ListChannelUsersMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ListChannelUsersMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	return nil
}

// ChannelUserEntry represents a single user currently present in a channel
type ChannelUserEntry struct {
	SessionID    uint64
	Nickname     string
	IsRegistered bool
	UserID       *uint64
	UserFlags    UserFlags
}

// ChannelUserListMessage (0xAB) - Snapshot of users in a channel
type ChannelUserListMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
	Users        []ChannelUserEntry
}

func (m *ChannelUserListMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.SubchannelID); err != nil {
		return err
	}
	if err := WriteUint16(w, uint16(len(m.Users))); err != nil {
		return err
	}
	for _, user := range m.Users {
		if err := WriteUint64(w, user.SessionID); err != nil {
			return err
		}
		if err := WriteString(w, user.Nickname); err != nil {
			return err
		}
		if err := WriteBool(w, user.IsRegistered); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, user.UserID); err != nil {
			return err
		}
		if err := WriteUint8(w, uint8(user.UserFlags)); err != nil {
			return err
		}
	}
	return nil
}

func (m *ChannelUserListMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ChannelUserListMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	userCount, err := ReadUint16(buf)
	if err != nil {
		return err
	}

	users := make([]ChannelUserEntry, userCount)
	for i := uint16(0); i < userCount; i++ {
		sessionID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		nickname, err := ReadString(buf)
		if err != nil {
			return err
		}
		isRegistered, err := ReadBool(buf)
		if err != nil {
			return err
		}
		userID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		flags, err := ReadUint8(buf)
		if err != nil {
			return err
		}

		users[i] = ChannelUserEntry{
			SessionID:    sessionID,
			Nickname:     nickname,
			IsRegistered: isRegistered,
			UserID:       userID,
			UserFlags:    UserFlags(flags),
		}
	}

	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	m.Users = users
	return nil
}

// ChannelPresenceMessage (0xAC) - User joined or left a channel
type ChannelPresenceMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
	SessionID    uint64
	Nickname     string
	IsRegistered bool
	UserID       *uint64
	UserFlags    UserFlags
	Joined       bool
}

func (m *ChannelPresenceMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.SubchannelID); err != nil {
		return err
	}
	if err := WriteUint64(w, m.SessionID); err != nil {
		return err
	}
	if err := WriteString(w, m.Nickname); err != nil {
		return err
	}
	if err := WriteBool(w, m.IsRegistered); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.UserID); err != nil {
		return err
	}
	if err := WriteUint8(w, uint8(m.UserFlags)); err != nil {
		return err
	}
	return WriteBool(w, m.Joined)
}

func (m *ChannelPresenceMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ChannelPresenceMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	sessionID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	nickname, err := ReadString(buf)
	if err != nil {
		return err
	}
	isRegistered, err := ReadBool(buf)
	if err != nil {
		return err
	}
	userID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	flags, err := ReadUint8(buf)
	if err != nil {
		return err
	}
	joined, err := ReadBool(buf)
	if err != nil {
		return err
	}

	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	m.SessionID = sessionID
	m.Nickname = nickname
	m.IsRegistered = isRegistered
	m.UserID = userID
	m.UserFlags = UserFlags(flags)
	m.Joined = joined
	return nil
}

// ServerPresenceMessage (0xAD) - User connected or disconnected from the server
type ServerPresenceMessage struct {
	SessionID    uint64
	Nickname     string
	IsRegistered bool
	UserID       *uint64
	UserFlags    UserFlags
	Online       bool
}

func (m *ServerPresenceMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.SessionID); err != nil {
		return err
	}
	if err := WriteString(w, m.Nickname); err != nil {
		return err
	}
	if err := WriteBool(w, m.IsRegistered); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.UserID); err != nil {
		return err
	}
	if err := WriteUint8(w, uint8(m.UserFlags)); err != nil {
		return err
	}
	return WriteBool(w, m.Online)
}

func (m *ServerPresenceMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ServerPresenceMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	sessionID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	nickname, err := ReadString(buf)
	if err != nil {
		return err
	}
	isRegistered, err := ReadBool(buf)
	if err != nil {
		return err
	}
	userID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	flags, err := ReadUint8(buf)
	if err != nil {
		return err
	}
	online, err := ReadBool(buf)
	if err != nil {
		return err
	}

	m.SessionID = sessionID
	m.Nickname = nickname
	m.IsRegistered = isRegistered
	m.UserID = userID
	m.UserFlags = UserFlags(flags)
	m.Online = online
	return nil
}

// ===== Server Discovery Messages =====

// ListServersMessage (0x55) - Request server list from directory
type ListServersMessage struct {
	Limit uint16
}

func (m *ListServersMessage) EncodeTo(w io.Writer) error {
	return WriteUint16(w, m.Limit)
}

func (m *ListServersMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ListServersMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	limit, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	m.Limit = limit
	return nil
}

// ServerInfo represents a single server in the server list
type ServerInfo struct {
	Hostname      string
	Port          uint16
	Name          string
	Description   string
	UserCount     uint32
	MaxUsers      uint32
	UptimeSeconds uint64
	IsPublic      bool
	ChannelCount  uint32
}

// ServerListMessage (0x9B) - List of discoverable servers
type ServerListMessage struct {
	Servers []ServerInfo
}

func (m *ServerListMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint16(w, uint16(len(m.Servers))); err != nil {
		return err
	}
	for _, server := range m.Servers {
		if err := WriteString(w, server.Hostname); err != nil {
			return err
		}
		if err := WriteUint16(w, server.Port); err != nil {
			return err
		}
		if err := WriteString(w, server.Name); err != nil {
			return err
		}
		if err := WriteString(w, server.Description); err != nil {
			return err
		}
		if err := WriteUint32(w, server.UserCount); err != nil {
			return err
		}
		if err := WriteUint32(w, server.MaxUsers); err != nil {
			return err
		}
		if err := WriteUint64(w, server.UptimeSeconds); err != nil {
			return err
		}
		if err := WriteBool(w, server.IsPublic); err != nil {
			return err
		}
		if err := WriteUint32(w, server.ChannelCount); err != nil {
			return err
		}
	}
	return nil
}

func (m *ServerListMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ServerListMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	serverCount, err := ReadUint16(buf)
	if err != nil {
		return err
	}

	servers := make([]ServerInfo, serverCount)
	for i := uint16(0); i < serverCount; i++ {
		hostname, err := ReadString(buf)
		if err != nil {
			return err
		}
		port, err := ReadUint16(buf)
		if err != nil {
			return err
		}
		name, err := ReadString(buf)
		if err != nil {
			return err
		}
		description, err := ReadString(buf)
		if err != nil {
			return err
		}
		userCount, err := ReadUint32(buf)
		if err != nil {
			return err
		}
		maxUsers, err := ReadUint32(buf)
		if err != nil {
			return err
		}
		uptimeSeconds, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		isPublic, err := ReadBool(buf)
		if err != nil {
			return err
		}
		channelCount, err := ReadUint32(buf)
		if err != nil {
			return err
		}

		servers[i] = ServerInfo{
			Hostname:      hostname,
			Port:          port,
			Name:          name,
			Description:   description,
			UserCount:     userCount,
			MaxUsers:      maxUsers,
			UptimeSeconds: uptimeSeconds,
			IsPublic:      isPublic,
			ChannelCount:  channelCount,
		}
	}

	m.Servers = servers
	return nil
}

// RegisterServerMessage (0x56) - Register server with directory
type RegisterServerMessage struct {
	Hostname     string
	Port         uint16
	Name         string
	Description  string
	MaxUsers     uint32
	IsPublic     bool
	ChannelCount uint32
}

func (m *RegisterServerMessage) EncodeTo(w io.Writer) error {
	if err := WriteString(w, m.Hostname); err != nil {
		return err
	}
	if err := WriteUint16(w, m.Port); err != nil {
		return err
	}
	if err := WriteString(w, m.Name); err != nil {
		return err
	}
	if err := WriteString(w, m.Description); err != nil {
		return err
	}
	if err := WriteUint32(w, m.MaxUsers); err != nil {
		return err
	}
	if err := WriteBool(w, m.IsPublic); err != nil {
		return err
	}
	return WriteUint32(w, m.ChannelCount)
}

func (m *RegisterServerMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *RegisterServerMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	hostname, err := ReadString(buf)
	if err != nil {
		return err
	}
	port, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	name, err := ReadString(buf)
	if err != nil {
		return err
	}
	description, err := ReadString(buf)
	if err != nil {
		return err
	}
	maxUsers, err := ReadUint32(buf)
	if err != nil {
		return err
	}
	isPublic, err := ReadBool(buf)
	if err != nil {
		return err
	}
	channelCount, err := ReadUint32(buf)
	if err != nil {
		return err
	}

	m.Hostname = hostname
	m.Port = port
	m.Name = name
	m.Description = description
	m.MaxUsers = maxUsers
	m.IsPublic = isPublic
	m.ChannelCount = channelCount
	return nil
}

// RegisterAckMessage (0x9C) - Server registration acknowledgment
type RegisterAckMessage struct {
	Success           bool
	HeartbeatInterval uint32 // Only present if success = true
	Message           string
}

func (m *RegisterAckMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if m.Success {
		if err := WriteUint32(w, m.HeartbeatInterval); err != nil {
			return err
		}
	}
	return WriteString(w, m.Message)
}

func (m *RegisterAckMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *RegisterAckMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.Success = success

	if success {
		heartbeatInterval, err := ReadUint32(buf)
		if err != nil {
			return err
		}
		m.HeartbeatInterval = heartbeatInterval
	}

	message, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Message = message
	return nil
}

// VerifyRegistrationMessage (0x9E) - Verification challenge
type VerifyRegistrationMessage struct {
	Challenge uint64
}

func (m *VerifyRegistrationMessage) EncodeTo(w io.Writer) error {
	return WriteUint64(w, m.Challenge)
}

func (m *VerifyRegistrationMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *VerifyRegistrationMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	challenge, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	m.Challenge = challenge
	return nil
}

// VerifyResponseMessage (0x58) - Response to verification challenge
type VerifyResponseMessage struct {
	Challenge uint64
}

func (m *VerifyResponseMessage) EncodeTo(w io.Writer) error {
	return WriteUint64(w, m.Challenge)
}

func (m *VerifyResponseMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *VerifyResponseMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	challenge, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	m.Challenge = challenge
	return nil
}

// HeartbeatMessage (0x57) - Periodic heartbeat to directory
type HeartbeatMessage struct {
	Hostname      string
	Port          uint16
	UserCount     uint32
	UptimeSeconds uint64
	ChannelCount  uint32
}

func (m *HeartbeatMessage) EncodeTo(w io.Writer) error {
	if err := WriteString(w, m.Hostname); err != nil {
		return err
	}
	if err := WriteUint16(w, m.Port); err != nil {
		return err
	}
	if err := WriteUint32(w, m.UserCount); err != nil {
		return err
	}
	if err := WriteUint64(w, m.UptimeSeconds); err != nil {
		return err
	}
	return WriteUint32(w, m.ChannelCount)
}

func (m *HeartbeatMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *HeartbeatMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	hostname, err := ReadString(buf)
	if err != nil {
		return err
	}
	port, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	userCount, err := ReadUint32(buf)
	if err != nil {
		return err
	}
	uptimeSeconds, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	channelCount, err := ReadUint32(buf)
	if err != nil {
		return err
	}

	m.Hostname = hostname
	m.Port = port
	m.UserCount = userCount
	m.UptimeSeconds = uptimeSeconds
	m.ChannelCount = channelCount
	return nil
}

// HeartbeatAckMessage (0x9D) - Heartbeat acknowledgment with interval
type HeartbeatAckMessage struct {
	HeartbeatInterval uint32
}

func (m *HeartbeatAckMessage) EncodeTo(w io.Writer) error {
	return WriteUint32(w, m.HeartbeatInterval)
}

func (m *HeartbeatAckMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *HeartbeatAckMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	heartbeatInterval, err := ReadUint32(buf)
	if err != nil {
		return err
	}
	m.HeartbeatInterval = heartbeatInterval
	return nil
}

// ===== CHANGE_PASSWORD (0x0E) - Client → Server =====

// ChangePasswordRequest is sent by clients to change their password
type ChangePasswordRequest struct {
	OldPassword string // Empty for SSH-registered users changing password for first time
	NewPassword string
}

func (m *ChangePasswordRequest) EncodeTo(w io.Writer) error {
	if err := WriteString(w, m.OldPassword); err != nil {
		return err
	}
	return WriteString(w, m.NewPassword)
}

func (m *ChangePasswordRequest) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ChangePasswordRequest) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	oldPassword, err := ReadString(buf)
	if err != nil {
		return err
	}
	newPassword, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.OldPassword = oldPassword
	m.NewPassword = newPassword
	return nil
}

// ===== PASSWORD_CHANGED (0x8E) - Server → Client =====

// PasswordChangedResponse is sent by server after password change attempt
type PasswordChangedResponse struct {
	Success      bool
	ErrorMessage string // Empty if success=true
}

func (m *PasswordChangedResponse) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	return WriteString(w, m.ErrorMessage)
}

func (m *PasswordChangedResponse) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *PasswordChangedResponse) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	errorMessage, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Success = success
	m.ErrorMessage = errorMessage
	return nil
}

// ===== ADD_SSH_KEY (0x0D) - Client → Server =====

// AddSSHKeyRequest is sent by clients to add a new SSH public key to their account
type AddSSHKeyRequest struct {
	PublicKey string // Full SSH public key (e.g., "ssh-rsa AAAA... user@host")
	Label     string // Optional user-friendly label (e.g., "Work Laptop")
}

func (m *AddSSHKeyRequest) EncodeTo(w io.Writer) error {
	if err := WriteString(w, m.PublicKey); err != nil {
		return err
	}
	return WriteString(w, m.Label)
}

func (m *AddSSHKeyRequest) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *AddSSHKeyRequest) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	publicKey, err := ReadString(buf)
	if err != nil {
		return err
	}
	label, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.PublicKey = publicKey
	m.Label = label
	return nil
}

// ===== SSH_KEY_ADDED (0x95) - Server → Client =====

// SSHKeyAddedResponse is sent by server after adding an SSH key
type SSHKeyAddedResponse struct {
	Success      bool
	KeyID        int64  // Database ID of the added key
	Fingerprint  string // SHA256 fingerprint
	ErrorMessage string // Empty if success=true
}

func (m *SSHKeyAddedResponse) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if err := WriteInt64(w, m.KeyID); err != nil {
		return err
	}
	if err := WriteString(w, m.Fingerprint); err != nil {
		return err
	}
	return WriteString(w, m.ErrorMessage)
}

func (m *SSHKeyAddedResponse) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SSHKeyAddedResponse) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	keyID, err := ReadInt64(buf)
	if err != nil {
		return err
	}
	fingerprint, err := ReadString(buf)
	if err != nil {
		return err
	}
	errorMessage, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Success = success
	m.KeyID = keyID
	m.Fingerprint = fingerprint
	m.ErrorMessage = errorMessage
	return nil
}

// ===== LIST_SSH_KEYS (0x14) - Client → Server =====

// ListSSHKeysRequest is sent by clients to retrieve their SSH keys
type ListSSHKeysRequest struct {
	// No fields - user is identified from session
}

func (m *ListSSHKeysRequest) EncodeTo(w io.Writer) error {
	// No data to encode
	return nil
}

func (m *ListSSHKeysRequest) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ListSSHKeysRequest) Decode(payload []byte) error {
	// No data to decode
	return nil
}

// ===== SSH_KEY_LIST (0x94) - Server → Client =====

// SSHKeyInfo represents a single SSH key in the list
type SSHKeyInfo struct {
	ID          int64
	Fingerprint string
	KeyType     string // ssh-rsa, ssh-ed25519, etc.
	Label       string // May be empty
	AddedAt     int64  // Unix milliseconds
	LastUsedAt  int64  // Unix milliseconds (0 if never used)
}

// SSHKeyListResponse is sent by server with list of user's SSH keys
type SSHKeyListResponse struct {
	Keys []SSHKeyInfo
}

func (m *SSHKeyListResponse) EncodeTo(w io.Writer) error {
	// Write number of keys
	if err := WriteUint32(w, uint32(len(m.Keys))); err != nil {
		return err
	}

	// Write each key
	for _, key := range m.Keys {
		if err := WriteInt64(w, key.ID); err != nil {
			return err
		}
		if err := WriteString(w, key.Fingerprint); err != nil {
			return err
		}
		if err := WriteString(w, key.KeyType); err != nil {
			return err
		}
		if err := WriteString(w, key.Label); err != nil {
			return err
		}
		if err := WriteInt64(w, key.AddedAt); err != nil {
			return err
		}
		if err := WriteInt64(w, key.LastUsedAt); err != nil {
			return err
		}
	}
	return nil
}

func (m *SSHKeyListResponse) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SSHKeyListResponse) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)

	// Read number of keys
	count, err := ReadUint32(buf)
	if err != nil {
		return err
	}

	m.Keys = make([]SSHKeyInfo, count)

	// Read each key
	for i := uint32(0); i < count; i++ {
		id, err := ReadInt64(buf)
		if err != nil {
			return err
		}
		fingerprint, err := ReadString(buf)
		if err != nil {
			return err
		}
		keyType, err := ReadString(buf)
		if err != nil {
			return err
		}
		label, err := ReadString(buf)
		if err != nil {
			return err
		}
		addedAt, err := ReadInt64(buf)
		if err != nil {
			return err
		}
		lastUsedAt, err := ReadInt64(buf)
		if err != nil {
			return err
		}

		m.Keys[i] = SSHKeyInfo{
			ID:          id,
			Fingerprint: fingerprint,
			KeyType:     keyType,
			Label:       label,
			AddedAt:     addedAt,
			LastUsedAt:  lastUsedAt,
		}
	}
	return nil
}

// ===== UPDATE_SSH_KEY_LABEL (0x12) - Client → Server =====

// UpdateSSHKeyLabelRequest is sent by clients to update an SSH key's label
type UpdateSSHKeyLabelRequest struct {
	KeyID    int64
	NewLabel string
}

func (m *UpdateSSHKeyLabelRequest) EncodeTo(w io.Writer) error {
	if err := WriteInt64(w, m.KeyID); err != nil {
		return err
	}
	return WriteString(w, m.NewLabel)
}

func (m *UpdateSSHKeyLabelRequest) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UpdateSSHKeyLabelRequest) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	keyID, err := ReadInt64(buf)
	if err != nil {
		return err
	}
	newLabel, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.KeyID = keyID
	m.NewLabel = newLabel
	return nil
}

// ===== SSH_KEY_LABEL_UPDATED (0x92) - Server → Client =====

// SSHKeyLabelUpdatedResponse is sent by server after updating an SSH key label
type SSHKeyLabelUpdatedResponse struct {
	Success      bool
	ErrorMessage string // Empty if success=true
}

func (m *SSHKeyLabelUpdatedResponse) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	return WriteString(w, m.ErrorMessage)
}

func (m *SSHKeyLabelUpdatedResponse) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SSHKeyLabelUpdatedResponse) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	errorMessage, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Success = success
	m.ErrorMessage = errorMessage
	return nil
}

// ===== DELETE_SSH_KEY (0x13) - Client → Server =====

// DeleteSSHKeyRequest is sent by clients to delete an SSH key
type DeleteSSHKeyRequest struct {
	KeyID int64
}

func (m *DeleteSSHKeyRequest) EncodeTo(w io.Writer) error {
	return WriteInt64(w, m.KeyID)
}

func (m *DeleteSSHKeyRequest) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *DeleteSSHKeyRequest) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	keyID, err := ReadInt64(buf)
	if err != nil {
		return err
	}
	m.KeyID = keyID
	return nil
}

// ===== SSH_KEY_DELETED (0x93) - Server → Client =====

// SSHKeyDeletedResponse is sent by server after deleting an SSH key
type SSHKeyDeletedResponse struct {
	Success      bool
	ErrorMessage string // Empty if success=true
}

func (m *SSHKeyDeletedResponse) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	return WriteString(w, m.ErrorMessage)
}

func (m *SSHKeyDeletedResponse) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *SSHKeyDeletedResponse) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	errorMessage, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Success = success
	m.ErrorMessage = errorMessage
	return nil
}

// ===== ADMIN COMMANDS =====

// BanUserMessage (0x59) - Ban a user by user ID or nickname
type BanUserMessage struct {
	UserID          *uint64 // Optional: user ID to ban (takes precedence if provided)
	Nickname        *string // Optional: nickname to ban (if user_id not provided)
	Reason          string
	Shadowban       bool
	DurationSeconds *uint64 // NULL = permanent ban
}

func (m *BanUserMessage) EncodeTo(w io.Writer) error {
	if err := WriteOptionalUint64(w, m.UserID); err != nil {
		return err
	}
	if err := WriteOptionalString(w, m.Nickname); err != nil {
		return err
	}
	if err := WriteString(w, m.Reason); err != nil {
		return err
	}
	if err := WriteBool(w, m.Shadowban); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.DurationSeconds)
}

func (m *BanUserMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *BanUserMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	userID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	nickname, err := ReadOptionalString(buf)
	if err != nil {
		return err
	}
	reason, err := ReadString(buf)
	if err != nil {
		return err
	}
	shadowban, err := ReadBool(buf)
	if err != nil {
		return err
	}
	durationSeconds, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}

	m.UserID = userID
	m.Nickname = nickname
	m.Reason = reason
	m.Shadowban = shadowban
	m.DurationSeconds = durationSeconds
	return nil
}

// UserBannedMessage (0x9F) - Response to BAN_USER
type UserBannedMessage struct {
	Success bool
	BanID   uint64 // Only present if Success=true
	Message string
}

func (m *UserBannedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if m.Success {
		if err := WriteUint64(w, m.BanID); err != nil {
			return err
		}
	}
	return WriteString(w, m.Message)
}

func (m *UserBannedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UserBannedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.Success = success

	if success {
		banID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		m.BanID = banID
	}

	message, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Message = message
	return nil
}

// BanIPMessage (0x5A) - Ban an IP address or CIDR range
type BanIPMessage struct {
	IPCIDR          string // IP address or CIDR range (e.g., "10.0.0.5/32", "192.168.1.0/24")
	Reason          string
	DurationSeconds *uint64 // NULL = permanent ban
}

func (m *BanIPMessage) EncodeTo(w io.Writer) error {
	if err := WriteString(w, m.IPCIDR); err != nil {
		return err
	}
	if err := WriteString(w, m.Reason); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.DurationSeconds)
}

func (m *BanIPMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *BanIPMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	ipCIDR, err := ReadString(buf)
	if err != nil {
		return err
	}
	reason, err := ReadString(buf)
	if err != nil {
		return err
	}
	durationSeconds, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}

	m.IPCIDR = ipCIDR
	m.Reason = reason
	m.DurationSeconds = durationSeconds
	return nil
}

// IPBannedMessage (0xA5) - Response to BAN_IP
type IPBannedMessage struct {
	Success bool
	BanID   uint64 // Only present if Success=true
	Message string
}

func (m *IPBannedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if m.Success {
		if err := WriteUint64(w, m.BanID); err != nil {
			return err
		}
	}
	return WriteString(w, m.Message)
}

func (m *IPBannedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *IPBannedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.Success = success

	if success {
		banID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		m.BanID = banID
	}

	message, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Message = message
	return nil
}

// UnbanUserMessage (0x5B) - Remove user ban
type UnbanUserMessage struct {
	UserID   *uint64 // Optional: user ID to unban
	Nickname *string // Optional: nickname to unban
}

func (m *UnbanUserMessage) EncodeTo(w io.Writer) error {
	if err := WriteOptionalUint64(w, m.UserID); err != nil {
		return err
	}
	return WriteOptionalString(w, m.Nickname)
}

func (m *UnbanUserMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UnbanUserMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	userID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	nickname, err := ReadOptionalString(buf)
	if err != nil {
		return err
	}

	m.UserID = userID
	m.Nickname = nickname
	return nil
}

// UserUnbannedMessage (0xA6) - Response to UNBAN_USER
type UserUnbannedMessage struct {
	Success bool
	Message string
}

func (m *UserUnbannedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	return WriteString(w, m.Message)
}

func (m *UserUnbannedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UserUnbannedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	message, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.Message = message
	return nil
}

// UnbanIPMessage (0x5C) - Remove IP ban
type UnbanIPMessage struct {
	IPCIDR string // IP address or CIDR range to unban
}

func (m *UnbanIPMessage) EncodeTo(w io.Writer) error {
	return WriteString(w, m.IPCIDR)
}

func (m *UnbanIPMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UnbanIPMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	ipCIDR, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.IPCIDR = ipCIDR
	return nil
}

// IPUnbannedMessage (0xA7) - Response to UNBAN_IP
type IPUnbannedMessage struct {
	Success bool
	Message string
}

func (m *IPUnbannedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	return WriteString(w, m.Message)
}

func (m *IPUnbannedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *IPUnbannedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	message, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.Message = message
	return nil
}

// ListBansMessage (0x5D) - Request list of all active bans
type ListBansMessage struct {
	IncludeExpired bool
}

func (m *ListBansMessage) EncodeTo(w io.Writer) error {
	return WriteBool(w, m.IncludeExpired)
}

func (m *ListBansMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ListBansMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	includeExpired, err := ReadBool(buf)
	if err != nil {
		return err
	}

	m.IncludeExpired = includeExpired
	return nil
}

// BanEntry represents a single ban in the ban list
type BanEntry struct {
	ID          uint64
	Type        string  // "user" or "ip"
	UserID      *uint64 // NULL for IP bans
	Nickname    *string // NULL for IP bans
	IPCIDR      *string // NULL for user bans
	Reason      string
	Shadowban   bool
	BannedAt    int64  // Unix milliseconds
	BannedUntil *int64 // NULL = permanent, Unix milliseconds for timed bans
	BannedBy    string // Admin nickname
}

// BanListMessage (0xA8) - List of active bans
type BanListMessage struct {
	Bans []BanEntry
}

func (m *BanListMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint16(w, uint16(len(m.Bans))); err != nil {
		return err
	}

	for _, ban := range m.Bans {
		if err := WriteUint64(w, ban.ID); err != nil {
			return err
		}
		if err := WriteString(w, ban.Type); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, ban.UserID); err != nil {
			return err
		}
		if err := WriteOptionalString(w, ban.Nickname); err != nil {
			return err
		}
		if err := WriteOptionalString(w, ban.IPCIDR); err != nil {
			return err
		}
		if err := WriteString(w, ban.Reason); err != nil {
			return err
		}
		if err := WriteBool(w, ban.Shadowban); err != nil {
			return err
		}
		if err := WriteInt64(w, ban.BannedAt); err != nil {
			return err
		}
		if err := WriteOptionalInt64(w, ban.BannedUntil); err != nil {
			return err
		}
		if err := WriteString(w, ban.BannedBy); err != nil {
			return err
		}
	}

	return nil
}

func (m *BanListMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *BanListMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)

	count, err := ReadUint16(buf)
	if err != nil {
		return err
	}

	m.Bans = make([]BanEntry, count)

	for i := uint16(0); i < count; i++ {
		id, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		banType, err := ReadString(buf)
		if err != nil {
			return err
		}
		userID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		nickname, err := ReadOptionalString(buf)
		if err != nil {
			return err
		}
		ipCIDR, err := ReadOptionalString(buf)
		if err != nil {
			return err
		}
		reason, err := ReadString(buf)
		if err != nil {
			return err
		}
		shadowban, err := ReadBool(buf)
		if err != nil {
			return err
		}
		bannedAt, err := ReadInt64(buf)
		if err != nil {
			return err
		}
		bannedUntil, err := ReadOptionalInt64(buf)
		if err != nil {
			return err
		}
		bannedBy, err := ReadString(buf)
		if err != nil {
			return err
		}

		m.Bans[i] = BanEntry{
			ID:          id,
			Type:        banType,
			UserID:      userID,
			Nickname:    nickname,
			IPCIDR:      ipCIDR,
			Reason:      reason,
			Shadowban:   shadowban,
			BannedAt:    bannedAt,
			BannedUntil: bannedUntil,
			BannedBy:    bannedBy,
		}
	}

	return nil
}

// DeleteUserMessage (0x5E) - Permanently delete a user account
type DeleteUserMessage struct {
	UserID uint64
}

func (m *DeleteUserMessage) EncodeTo(w io.Writer) error {
	return WriteUint64(w, m.UserID)
}

func (m *DeleteUserMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *DeleteUserMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	userID, err := ReadUint64(buf)
	if err != nil {
		return err
	}

	m.UserID = userID
	return nil
}

// UserDeletedMessage (0xA9) - Response to DELETE_USER
type UserDeletedMessage struct {
	Success bool
	Message string
}

func (m *UserDeletedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	return WriteString(w, m.Message)
}

func (m *UserDeletedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UserDeletedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	message, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.Message = message
	return nil
}

// DeleteChannelMessage (0x5F) - Delete a channel (admin only)
type DeleteChannelMessage struct {
	ChannelID uint64
	Reason    string
}

func (m *DeleteChannelMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	return WriteString(w, m.Reason)
}

func (m *DeleteChannelMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *DeleteChannelMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	reason, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.ChannelID = channelID
	m.Reason = reason
	return nil
}

// ChannelDeletedMessage (0xAA) - Response to DELETE_CHANNEL + broadcast
type ChannelDeletedMessage struct {
	Success   bool
	ChannelID uint64
	Message   string
}

func (m *ChannelDeletedMessage) EncodeTo(w io.Writer) error {
	if err := WriteBool(w, m.Success); err != nil {
		return err
	}
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	return WriteString(w, m.Message)
}

func (m *ChannelDeletedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ChannelDeletedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	success, err := ReadBool(buf)
	if err != nil {
		return err
	}
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	message, err := ReadString(buf)
	if err != nil {
		return err
	}

	m.Success = success
	m.ChannelID = channelID
	m.Message = message
	return nil
}

// UnreadTarget represents a channel/subchannel/thread for unread count requests
type UnreadTarget struct {
	ChannelID    uint64
	SubchannelID *uint64
	ThreadID     *uint64 // Optional: if present, count only messages in this thread
}

// GetUnreadCountsMessage (0x18) - Request unread counts for channels
type GetUnreadCountsMessage struct {
	SinceTimestamp *int64         // Optional: if null, uses server's stored last_read_at (registered users only)
	Targets        []UnreadTarget // Channels/subchannels to get counts for
}

func (m *GetUnreadCountsMessage) EncodeTo(w io.Writer) error {
	if err := WriteOptionalInt64(w, m.SinceTimestamp); err != nil {
		return err
	}
	if err := WriteUint16(w, uint16(len(m.Targets))); err != nil {
		return err
	}
	for _, target := range m.Targets {
		if err := WriteUint64(w, target.ChannelID); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, target.SubchannelID); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, target.ThreadID); err != nil {
			return err
		}
	}
	return nil
}

func (m *GetUnreadCountsMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *GetUnreadCountsMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	sinceTimestamp, err := ReadOptionalInt64(buf)
	if err != nil {
		return err
	}
	targetCount, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	targets := make([]UnreadTarget, targetCount)
	for i := uint16(0); i < targetCount; i++ {
		channelID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		subchannelID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		threadID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		targets[i] = UnreadTarget{
			ChannelID:    channelID,
			SubchannelID: subchannelID,
			ThreadID:     threadID,
		}
	}
	m.SinceTimestamp = sinceTimestamp
	m.Targets = targets
	return nil
}

// UnreadCount represents unread count for a single channel/subchannel/thread
type UnreadCount struct {
	ChannelID    uint64
	SubchannelID *uint64
	ThreadID     *uint64 // Optional: if present, this count is for a specific thread
	UnreadCount  uint32
}

// UnreadCountsMessage (0x97) - Response with unread counts
type UnreadCountsMessage struct {
	Counts []UnreadCount
}

func (m *UnreadCountsMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint16(w, uint16(len(m.Counts))); err != nil {
		return err
	}
	for _, count := range m.Counts {
		if err := WriteUint64(w, count.ChannelID); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, count.SubchannelID); err != nil {
			return err
		}
		if err := WriteOptionalUint64(w, count.ThreadID); err != nil {
			return err
		}
		if err := WriteUint32(w, count.UnreadCount); err != nil {
			return err
		}
	}
	return nil
}

func (m *UnreadCountsMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UnreadCountsMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	countCount, err := ReadUint16(buf)
	if err != nil {
		return err
	}
	counts := make([]UnreadCount, countCount)
	for i := uint16(0); i < countCount; i++ {
		channelID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		subchannelID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		threadID, err := ReadOptionalUint64(buf)
		if err != nil {
			return err
		}
		unreadCount, err := ReadUint32(buf)
		if err != nil {
			return err
		}
		counts[i] = UnreadCount{
			ChannelID:    channelID,
			SubchannelID: subchannelID,
			ThreadID:     threadID,
			UnreadCount:  unreadCount,
		}
	}
	m.Counts = counts
	return nil
}

// UpdateReadStateMessage (0x1D) - Update last read timestamp for a channel
type UpdateReadStateMessage struct {
	ChannelID    uint64
	SubchannelID *uint64
	Timestamp    int64 // Unix timestamp (seconds)
}

func (m *UpdateReadStateMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.SubchannelID); err != nil {
		return err
	}
	return WriteInt64(w, m.Timestamp)
}

func (m *UpdateReadStateMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *UpdateReadStateMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	subchannelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	timestamp, err := ReadInt64(buf)
	if err != nil {
		return err
	}
	m.ChannelID = channelID
	m.SubchannelID = subchannelID
	m.Timestamp = timestamp
	return nil
}

// ============================================================================
// V3 Direct Message (DM) Messages
// ============================================================================

// Target type constants for START_DM
const (
	DMTargetByUserID    = 0x00 // Target by registered user ID
	DMTargetByNickname  = 0x01 // Target by nickname (registered or anonymous)
	DMTargetBySessionID = 0x02 // Target by session ID (anonymous users)
)

// Key type constants for PROVIDE_PUBLIC_KEY
const (
	KeyTypeDerivedFromSSH = 0x00 // Ed25519 -> X25519 conversion
	KeyTypeGenerated      = 0x01 // Generated X25519 key
	KeyTypeEphemeral      = 0x02 // Session-only ephemeral key
)

// StartDMMessage (0x19) - Initiate a DM with another user
type StartDMMessage struct {
	TargetType       uint8  // 0=user_id, 1=nickname, 2=session_id
	TargetUserID     uint64 // Used when TargetType=0 or TargetType=2
	TargetNickname   string // Used when TargetType=1
	AllowUnencrypted bool   // If true, allow unencrypted DM
}

func (m *StartDMMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint8(w, m.TargetType); err != nil {
		return err
	}
	switch m.TargetType {
	case DMTargetByUserID, DMTargetBySessionID:
		if err := WriteUint64(w, m.TargetUserID); err != nil {
			return err
		}
	case DMTargetByNickname:
		if err := WriteString(w, m.TargetNickname); err != nil {
			return err
		}
	}
	return WriteBool(w, m.AllowUnencrypted)
}

func (m *StartDMMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *StartDMMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	targetType, err := ReadUint8(buf)
	if err != nil {
		return err
	}
	m.TargetType = targetType

	switch targetType {
	case DMTargetByUserID, DMTargetBySessionID:
		targetID, err := ReadUint64(buf)
		if err != nil {
			return err
		}
		m.TargetUserID = targetID
	case DMTargetByNickname:
		nickname, err := ReadString(buf)
		if err != nil {
			return err
		}
		m.TargetNickname = nickname
	}

	allowUnencrypted, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.AllowUnencrypted = allowUnencrypted
	return nil
}

// ProvidePublicKeyMessage (0x1A) - Upload X25519 public key for encryption
type ProvidePublicKeyMessage struct {
	KeyType   uint8    // 0=derived, 1=generated, 2=ephemeral
	PublicKey [32]byte // X25519 public key (32 bytes)
	Label     string   // Optional label (e.g., "laptop", "phone")
}

func (m *ProvidePublicKeyMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint8(w, m.KeyType); err != nil {
		return err
	}
	if _, err := w.Write(m.PublicKey[:]); err != nil {
		return err
	}
	return WriteString(w, m.Label)
}

func (m *ProvidePublicKeyMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ProvidePublicKeyMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	keyType, err := ReadUint8(buf)
	if err != nil {
		return err
	}
	m.KeyType = keyType

	if _, err := io.ReadFull(buf, m.PublicKey[:]); err != nil {
		return err
	}

	label, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Label = label
	return nil
}

// AllowUnencryptedMessage (0x1B) - Accept unencrypted DM
type AllowUnencryptedMessage struct {
	DMChannelID uint64 // The DM channel ID from the invite
	Permanent   bool   // If true, allow all future unencrypted DMs
}

func (m *AllowUnencryptedMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.DMChannelID); err != nil {
		return err
	}
	return WriteBool(w, m.Permanent)
}

func (m *AllowUnencryptedMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *AllowUnencryptedMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	m.DMChannelID = channelID

	permanent, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.Permanent = permanent
	return nil
}

// KeyRequiredMessage (0xA1) - Server needs encryption key
type KeyRequiredMessage struct {
	Reason      string  // Human-readable explanation
	DMChannelID *uint64 // Optional: specific DM channel this is for
}

func (m *KeyRequiredMessage) EncodeTo(w io.Writer) error {
	if err := WriteString(w, m.Reason); err != nil {
		return err
	}
	return WriteOptionalUint64(w, m.DMChannelID)
}

func (m *KeyRequiredMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *KeyRequiredMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	reason, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Reason = reason

	channelID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	m.DMChannelID = channelID
	return nil
}

// DMReadyMessage (0xA2) - DM channel is ready to use
type DMReadyMessage struct {
	ChannelID      uint64   // The DM channel ID
	OtherUserID    *uint64  // Other user's ID (nil if anonymous)
	OtherNickname  string   // Other user's nickname
	IsEncrypted    bool     // Whether this DM uses encryption
	OtherPublicKey [32]byte // Other party's X25519 public key (only if encrypted)
}

func (m *DMReadyMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.ChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.OtherUserID); err != nil {
		return err
	}
	if err := WriteString(w, m.OtherNickname); err != nil {
		return err
	}
	if err := WriteBool(w, m.IsEncrypted); err != nil {
		return err
	}
	if m.IsEncrypted {
		if _, err := w.Write(m.OtherPublicKey[:]); err != nil {
			return err
		}
	}
	return nil
}

func (m *DMReadyMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *DMReadyMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	m.ChannelID = channelID

	otherUserID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	m.OtherUserID = otherUserID

	otherNickname, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.OtherNickname = otherNickname

	isEncrypted, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.IsEncrypted = isEncrypted

	if isEncrypted {
		if _, err := io.ReadFull(buf, m.OtherPublicKey[:]); err != nil {
			return err
		}
	}
	return nil
}

// DMPendingMessage (0xA3) - Waiting for other party
type DMPendingMessage struct {
	DMChannelID        uint64  // The pending DM channel ID
	WaitingForUserID   *uint64 // Other user's ID (nil if anonymous)
	WaitingForNickname string  // Other user's nickname
	Reason             string  // Human-readable status
}

func (m *DMPendingMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.DMChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.WaitingForUserID); err != nil {
		return err
	}
	if err := WriteString(w, m.WaitingForNickname); err != nil {
		return err
	}
	return WriteString(w, m.Reason)
}

func (m *DMPendingMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *DMPendingMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	m.DMChannelID = channelID

	waitingForUserID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	m.WaitingForUserID = waitingForUserID

	waitingForNickname, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.WaitingForNickname = waitingForNickname

	reason, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.Reason = reason
	return nil
}

// DMRequestMessage (0xA4) - Incoming DM request
type DMRequestMessage struct {
	DMChannelID               uint64  // The pending DM channel ID
	FromUserID                *uint64 // Initiator's user ID (nil if anonymous)
	FromNickname              string  // Initiator's nickname
	RequiresKey               bool    // True if recipient needs to set up a key
	InitiatorAllowsUnencrypted bool    // True if initiator allows unencrypted
}

func (m *DMRequestMessage) EncodeTo(w io.Writer) error {
	if err := WriteUint64(w, m.DMChannelID); err != nil {
		return err
	}
	if err := WriteOptionalUint64(w, m.FromUserID); err != nil {
		return err
	}
	if err := WriteString(w, m.FromNickname); err != nil {
		return err
	}
	if err := WriteBool(w, m.RequiresKey); err != nil {
		return err
	}
	return WriteBool(w, m.InitiatorAllowsUnencrypted)
}

func (m *DMRequestMessage) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := m.EncodeTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *DMRequestMessage) Decode(payload []byte) error {
	buf := bytes.NewReader(payload)
	channelID, err := ReadUint64(buf)
	if err != nil {
		return err
	}
	m.DMChannelID = channelID

	fromUserID, err := ReadOptionalUint64(buf)
	if err != nil {
		return err
	}
	m.FromUserID = fromUserID

	fromNickname, err := ReadString(buf)
	if err != nil {
		return err
	}
	m.FromNickname = fromNickname

	requiresKey, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.RequiresKey = requiresKey

	allowsUnencrypted, err := ReadBool(buf)
	if err != nil {
		return err
	}
	m.InitiatorAllowsUnencrypted = allowsUnencrypted
	return nil
}

// Compile-time checks to ensure all message types implement the ProtocolMessage interface
// This will cause a compile error if any message type is missing Encode(), EncodeTo(), or Decode()
var (
	// Client → Server messages
	_ ProtocolMessage = (*AuthRequestMessage)(nil)
	_ ProtocolMessage = (*SetNicknameMessage)(nil)
	_ ProtocolMessage = (*RegisterUserMessage)(nil)
	_ ProtocolMessage = (*LogoutMessage)(nil)
	_ ProtocolMessage = (*ListChannelsMessage)(nil)
	_ ProtocolMessage = (*JoinChannelMessage)(nil)
	_ ProtocolMessage = (*LeaveChannelMessage)(nil)
	_ ProtocolMessage = (*CreateChannelMessage)(nil)
	_ ProtocolMessage = (*ListMessagesMessage)(nil)
	_ ProtocolMessage = (*PostMessageMessage)(nil)
	_ ProtocolMessage = (*EditMessageMessage)(nil)
	_ ProtocolMessage = (*DeleteMessageMessage)(nil)
	_ ProtocolMessage = (*PingMessage)(nil)
	_ ProtocolMessage = (*DisconnectMessage)(nil)
	_ ProtocolMessage = (*SubscribeThreadMessage)(nil)
	_ ProtocolMessage = (*UnsubscribeThreadMessage)(nil)
	_ ProtocolMessage = (*SubscribeChannelMessage)(nil)
	_ ProtocolMessage = (*UnsubscribeChannelMessage)(nil)
	_ ProtocolMessage = (*GetUserInfoMessage)(nil)
	_ ProtocolMessage = (*ListUsersMessage)(nil)
	_ ProtocolMessage = (*ListChannelUsersMessage)(nil)
	_ ProtocolMessage = (*ListServersMessage)(nil)
	_ ProtocolMessage = (*RegisterServerMessage)(nil)
	_ ProtocolMessage = (*VerifyRegistrationMessage)(nil)
	_ ProtocolMessage = (*HeartbeatMessage)(nil)
	_ ProtocolMessage = (*ChangePasswordRequest)(nil)
	_ ProtocolMessage = (*AddSSHKeyRequest)(nil)
	_ ProtocolMessage = (*ListSSHKeysRequest)(nil)
	_ ProtocolMessage = (*UpdateSSHKeyLabelRequest)(nil)
	_ ProtocolMessage = (*DeleteSSHKeyRequest)(nil)
	_ ProtocolMessage = (*BanUserMessage)(nil)
	_ ProtocolMessage = (*BanIPMessage)(nil)
	_ ProtocolMessage = (*UnbanUserMessage)(nil)
	_ ProtocolMessage = (*UnbanIPMessage)(nil)
	_ ProtocolMessage = (*ListBansMessage)(nil)
	_ ProtocolMessage = (*DeleteUserMessage)(nil)

	// Server → Client messages
	_ ProtocolMessage = (*AuthResponseMessage)(nil)
	_ ProtocolMessage = (*NicknameResponseMessage)(nil)
	_ ProtocolMessage = (*RegisterResponseMessage)(nil)
	_ ProtocolMessage = (*ChannelListMessage)(nil)
	_ ProtocolMessage = (*JoinResponseMessage)(nil)
	_ ProtocolMessage = (*LeaveResponseMessage)(nil)
	_ ProtocolMessage = (*ChannelCreatedMessage)(nil)
	_ ProtocolMessage = (*MessageListMessage)(nil)
	_ ProtocolMessage = (*MessagePostedMessage)(nil)
	_ ProtocolMessage = (*MessageEditedMessage)(nil)
	_ ProtocolMessage = (*MessageDeletedMessage)(nil)
	_ ProtocolMessage = (*PongMessage)(nil)
	_ ProtocolMessage = (*ErrorMessage)(nil)
	_ ProtocolMessage = (*ServerConfigMessage)(nil)
	_ ProtocolMessage = (*SubscribeOkMessage)(nil)
	_ ProtocolMessage = (*UserInfoMessage)(nil)
	_ ProtocolMessage = (*UserListMessage)(nil)
	_ ProtocolMessage = (*ChannelUserListMessage)(nil)
	_ ProtocolMessage = (*ChannelPresenceMessage)(nil)
	_ ProtocolMessage = (*ServerPresenceMessage)(nil)
	_ ProtocolMessage = (*ServerListMessage)(nil)
	_ ProtocolMessage = (*RegisterAckMessage)(nil)
	_ ProtocolMessage = (*VerifyResponseMessage)(nil)
	_ ProtocolMessage = (*HeartbeatAckMessage)(nil)
	_ ProtocolMessage = (*PasswordChangedResponse)(nil)
	_ ProtocolMessage = (*SSHKeyAddedResponse)(nil)
	_ ProtocolMessage = (*SSHKeyListResponse)(nil)
	_ ProtocolMessage = (*SSHKeyLabelUpdatedResponse)(nil)
	_ ProtocolMessage = (*SSHKeyDeletedResponse)(nil)
	_ ProtocolMessage = (*UserBannedMessage)(nil)
	_ ProtocolMessage = (*IPBannedMessage)(nil)
	_ ProtocolMessage = (*UserUnbannedMessage)(nil)
	_ ProtocolMessage = (*IPUnbannedMessage)(nil)
	_ ProtocolMessage = (*BanListMessage)(nil)
	_ ProtocolMessage = (*UserDeletedMessage)(nil)
	_ ProtocolMessage = (*GetUnreadCountsMessage)(nil)
	_ ProtocolMessage = (*UnreadCountsMessage)(nil)
	_ ProtocolMessage = (*UpdateReadStateMessage)(nil)

	// V3 DM messages
	_ ProtocolMessage = (*StartDMMessage)(nil)
	_ ProtocolMessage = (*ProvidePublicKeyMessage)(nil)
	_ ProtocolMessage = (*AllowUnencryptedMessage)(nil)
	_ ProtocolMessage = (*KeyRequiredMessage)(nil)
	_ ProtocolMessage = (*DMReadyMessage)(nil)
	_ ProtocolMessage = (*DMPendingMessage)(nil)
	_ ProtocolMessage = (*DMRequestMessage)(nil)
)
