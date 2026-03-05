package generated

import (
	"fmt"
	"github.com/aeolun/superchat/pkg/archive/generated/runtime"
)

// String is a type alias for string (encoding config used inline at call sites)
type String = string

type ArchiveFrameHeader struct {
	Length uint32
	Type uint8
}

func (m *ArchiveFrameHeader) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *ArchiveFrameHeader) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"length": m.Length,
		"type": m.Type,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint32(m.Length, runtime.BigEndian)
	encoder.WriteUint8(m.Type)

	return encoder.Finish(), nil
}

func (m *ArchiveFrameHeader) CalculateSize() int {
	size := 0

	size += 4 // Length
	size += 1 // Type

	return size
}

func DecodeArchiveFrameHeader(bytes []byte) (*ArchiveFrameHeader, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeArchiveFrameHeaderWithDecoder(decoder)
}

func decodeArchiveFrameHeaderWithDecoder(decoder *runtime.BitStreamDecoder) (*ArchiveFrameHeader, error) {
	result := &ArchiveFrameHeader{}

	length, err := decoder.ReadUint32(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode length: %w", err)
	}
	result.Length = length

	type_, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode type: %w", err)
	}
	result.Type = type_


	return result, nil
}

type Handshake struct {
	ServerName string
	ProtocolVersion uint8
}

func (m *Handshake) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *Handshake) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"server_name": m.ServerName,
		"protocol_version": m.ProtocolVersion,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	m_ServerName_bytes := []byte(m.ServerName)
	encoder.WriteUint16(uint16(len(m_ServerName_bytes)), runtime.BigEndian)
	for _, b := range m_ServerName_bytes {
		encoder.WriteUint8(b)
	}
	encoder.WriteUint8(m.ProtocolVersion)

	return encoder.Finish(), nil
}

func (m *Handshake) CalculateSize() int {
	size := 0

	size += 2 + len(m.ServerName) // ServerName (length-prefixed string)
	size += 1 // ProtocolVersion

	return size
}

func DecodeHandshake(bytes []byte) (*Handshake, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeHandshakeWithDecoder(decoder)
}

func decodeHandshakeWithDecoder(decoder *runtime.BitStreamDecoder) (*Handshake, error) {
	result := &Handshake{}

	serverNameLength, err := decoder.ReadUint16(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode server_name length: %w", err)
	}
	serverNameBytes, err := decoder.ReadBytesSlice(int(serverNameLength))
	if err != nil {
		return nil, fmt.Errorf("failed to decode server_name: %w", err)
	}
	result.ServerName = string(serverNameBytes)

	protocolVersion, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode protocol_version: %w", err)
	}
	result.ProtocolVersion = protocolVersion


	return result, nil
}

type ChannelState struct {
	ChannelId uint64
	LastMessageId uint64
}

func (m *ChannelState) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *ChannelState) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"channel_id": m.ChannelId,
		"last_message_id": m.LastMessageId,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint64(m.ChannelId, runtime.BigEndian)
	encoder.WriteUint64(m.LastMessageId, runtime.BigEndian)

	return encoder.Finish(), nil
}

func (m *ChannelState) CalculateSize() int {
	size := 0

	size += 8 // ChannelId
	size += 8 // LastMessageId

	return size
}

func DecodeChannelState(bytes []byte) (*ChannelState, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeChannelStateWithDecoder(decoder)
}

func decodeChannelStateWithDecoder(decoder *runtime.BitStreamDecoder) (*ChannelState, error) {
	result := &ChannelState{}

	channelId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode channel_id: %w", err)
	}
	result.ChannelId = channelId

	lastMessageId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode last_message_id: %w", err)
	}
	result.LastMessageId = lastMessageId


	return result, nil
}

type HandshakeAck struct {
	Success uint8
	ChannelCount uint16
	Channels []ChannelState
}

func (m *HandshakeAck) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *HandshakeAck) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"success": m.Success,
		"channel_count": m.ChannelCount,
		"channels": m.Channels,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint8(m.Success)
	encoder.WriteUint16(m.ChannelCount, runtime.BigEndian)
	for _, Channels_item := range m.Channels {
		Channels_item_ctx := childCtx.WithByteOffset(ctx.ByteOffset + encoder.Position())
		Channels_item_bytes, err := Channels_item.EncodeWithContext(Channels_item_ctx)
		if err != nil {
			return nil, err
		}
		for _, b := range Channels_item_bytes {
			encoder.WriteUint8(b)
		}
	}

	return encoder.Finish(), nil
}

func (m *HandshakeAck) CalculateSize() int {
	size := 0

	size += 1 // Success
	size += 2 // ChannelCount
	for _, Channels_item := range m.Channels {
		size += Channels_item.CalculateSize()
	}

	return size
}

func DecodeHandshakeAck(bytes []byte) (*HandshakeAck, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeHandshakeAckWithDecoder(decoder)
}

func decodeHandshakeAckWithDecoder(decoder *runtime.BitStreamDecoder) (*HandshakeAck, error) {
	result := &HandshakeAck{}

	success, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode success: %w", err)
	}
	result.Success = success

	channelCount, err := decoder.ReadUint16(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode channel_count: %w", err)
	}
	result.ChannelCount = channelCount

	result.Channels = make([]ChannelState, result.ChannelCount)
	for i := range result.Channels {
		channelsItem, err := decodeChannelStateWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode nested struct: %w", err)
		}
		result.Channels[i] = *channelsItem
	}


	return result, nil
}

type ChannelInfo struct {
	ChannelId uint64
	Name string
	Description string
	ChannelType uint8
	RetentionHours uint32
	ArchiveEnabled uint8
}

func (m *ChannelInfo) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *ChannelInfo) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"channel_id": m.ChannelId,
		"name": m.Name,
		"description": m.Description,
		"channel_type": m.ChannelType,
		"retention_hours": m.RetentionHours,
		"archive_enabled": m.ArchiveEnabled,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint64(m.ChannelId, runtime.BigEndian)
	m_Name_bytes := []byte(m.Name)
	encoder.WriteUint16(uint16(len(m_Name_bytes)), runtime.BigEndian)
	for _, b := range m_Name_bytes {
		encoder.WriteUint8(b)
	}
	m_Description_bytes := []byte(m.Description)
	encoder.WriteUint16(uint16(len(m_Description_bytes)), runtime.BigEndian)
	for _, b := range m_Description_bytes {
		encoder.WriteUint8(b)
	}
	encoder.WriteUint8(m.ChannelType)
	encoder.WriteUint32(m.RetentionHours, runtime.BigEndian)
	encoder.WriteUint8(m.ArchiveEnabled)

	return encoder.Finish(), nil
}

func (m *ChannelInfo) CalculateSize() int {
	size := 0

	size += 8 // ChannelId
	size += 2 + len(m.Name) // Name (length-prefixed string)
	size += 2 + len(m.Description) // Description (length-prefixed string)
	size += 1 // ChannelType
	size += 4 // RetentionHours
	size += 1 // ArchiveEnabled

	return size
}

func DecodeChannelInfo(bytes []byte) (*ChannelInfo, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeChannelInfoWithDecoder(decoder)
}

func decodeChannelInfoWithDecoder(decoder *runtime.BitStreamDecoder) (*ChannelInfo, error) {
	result := &ChannelInfo{}

	channelId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode channel_id: %w", err)
	}
	result.ChannelId = channelId

	nameLength, err := decoder.ReadUint16(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode name length: %w", err)
	}
	nameBytes, err := decoder.ReadBytesSlice(int(nameLength))
	if err != nil {
		return nil, fmt.Errorf("failed to decode name: %w", err)
	}
	result.Name = string(nameBytes)

	descriptionLength, err := decoder.ReadUint16(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode description length: %w", err)
	}
	descriptionBytes, err := decoder.ReadBytesSlice(int(descriptionLength))
	if err != nil {
		return nil, fmt.Errorf("failed to decode description: %w", err)
	}
	result.Description = string(descriptionBytes)

	channelType, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode channel_type: %w", err)
	}
	result.ChannelType = channelType

	retentionHours, err := decoder.ReadUint32(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode retention_hours: %w", err)
	}
	result.RetentionHours = retentionHours

	archiveEnabled, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode archive_enabled: %w", err)
	}
	result.ArchiveEnabled = archiveEnabled


	return result, nil
}

type BackfillStart struct {
	ChannelId uint64
	TotalMessages uint32
}

func (m *BackfillStart) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *BackfillStart) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"channel_id": m.ChannelId,
		"total_messages": m.TotalMessages,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint64(m.ChannelId, runtime.BigEndian)
	encoder.WriteUint32(m.TotalMessages, runtime.BigEndian)

	return encoder.Finish(), nil
}

func (m *BackfillStart) CalculateSize() int {
	size := 0

	size += 8 // ChannelId
	size += 4 // TotalMessages

	return size
}

func DecodeBackfillStart(bytes []byte) (*BackfillStart, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeBackfillStartWithDecoder(decoder)
}

func decodeBackfillStartWithDecoder(decoder *runtime.BitStreamDecoder) (*BackfillStart, error) {
	result := &BackfillStart{}

	channelId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode channel_id: %w", err)
	}
	result.ChannelId = channelId

	totalMessages, err := decoder.ReadUint32(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode total_messages: %w", err)
	}
	result.TotalMessages = totalMessages


	return result, nil
}

type MessageSync struct {
	MessageId uint64
	ChannelId uint64
	ParentId *uint64
	ThreadRootId *uint64
	AuthorUserId *uint64
	AuthorNickname string
	Content string
	CreatedAt int64
	EditedAt *int64
	DeletedAt *int64
}

func (m *MessageSync) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *MessageSync) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"message_id": m.MessageId,
		"channel_id": m.ChannelId,
		"parent_id": m.ParentId,
		"thread_root_id": m.ThreadRootId,
		"author_user_id": m.AuthorUserId,
		"author_nickname": m.AuthorNickname,
		"content": m.Content,
		"created_at": m.CreatedAt,
		"edited_at": m.EditedAt,
		"deleted_at": m.DeletedAt,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint64(m.MessageId, runtime.BigEndian)
	encoder.WriteUint64(m.ChannelId, runtime.BigEndian)
	if m.ParentId != nil {
		encoder.WriteUint8(1)
		encoder.WriteUint64(*m.ParentId, runtime.BigEndian)
	} else {
		encoder.WriteUint8(0)
	}
	if m.ThreadRootId != nil {
		encoder.WriteUint8(1)
		encoder.WriteUint64(*m.ThreadRootId, runtime.BigEndian)
	} else {
		encoder.WriteUint8(0)
	}
	if m.AuthorUserId != nil {
		encoder.WriteUint8(1)
		encoder.WriteUint64(*m.AuthorUserId, runtime.BigEndian)
	} else {
		encoder.WriteUint8(0)
	}
	m_AuthorNickname_bytes := []byte(m.AuthorNickname)
	encoder.WriteUint16(uint16(len(m_AuthorNickname_bytes)), runtime.BigEndian)
	for _, b := range m_AuthorNickname_bytes {
		encoder.WriteUint8(b)
	}
	m_Content_bytes := []byte(m.Content)
	encoder.WriteUint16(uint16(len(m_Content_bytes)), runtime.BigEndian)
	for _, b := range m_Content_bytes {
		encoder.WriteUint8(b)
	}
	encoder.WriteInt64(m.CreatedAt, runtime.BigEndian)
	if m.EditedAt != nil {
		encoder.WriteUint8(1)
		encoder.WriteInt64(*m.EditedAt, runtime.BigEndian)
	} else {
		encoder.WriteUint8(0)
	}
	if m.DeletedAt != nil {
		encoder.WriteUint8(1)
		encoder.WriteInt64(*m.DeletedAt, runtime.BigEndian)
	} else {
		encoder.WriteUint8(0)
	}

	return encoder.Finish(), nil
}

func (m *MessageSync) CalculateSize() int {
	size := 0

	size += 8 // MessageId
	size += 8 // ChannelId
	if m.ParentId != nil {
		size += 1 // ParentId (presence)
		size += 8 // m.ParentId
	} else {
		size += 1 // ParentId (absence)
	}
	if m.ThreadRootId != nil {
		size += 1 // ThreadRootId (presence)
		size += 8 // m.ThreadRootId
	} else {
		size += 1 // ThreadRootId (absence)
	}
	if m.AuthorUserId != nil {
		size += 1 // AuthorUserId (presence)
		size += 8 // m.AuthorUserId
	} else {
		size += 1 // AuthorUserId (absence)
	}
	size += 2 + len(m.AuthorNickname) // AuthorNickname (length-prefixed string)
	size += 2 + len(m.Content) // Content (length-prefixed string)
	size += 8 // CreatedAt
	if m.EditedAt != nil {
		size += 1 // EditedAt (presence)
		size += 8 // m.EditedAt
	} else {
		size += 1 // EditedAt (absence)
	}
	if m.DeletedAt != nil {
		size += 1 // DeletedAt (presence)
		size += 8 // m.DeletedAt
	} else {
		size += 1 // DeletedAt (absence)
	}

	return size
}

func DecodeMessageSync(bytes []byte) (*MessageSync, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeMessageSyncWithDecoder(decoder)
}

func decodeMessageSyncWithDecoder(decoder *runtime.BitStreamDecoder) (*MessageSync, error) {
	result := &MessageSync{}

	messageId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode message_id: %w", err)
	}
	result.MessageId = messageId

	channelId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode channel_id: %w", err)
	}
	result.ChannelId = channelId

	parentIdPresent, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode parent_id presence: %w", err)
	}
	if parentIdPresent == 1 {
		parentIdValue, err := decoder.ReadUint64(runtime.BigEndian)
		if err != nil {
			return nil, fmt.Errorf("failed to decode value: %w", err)
		}
		result.ParentId = &parentIdValue
	}

	threadRootIdPresent, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode thread_root_id presence: %w", err)
	}
	if threadRootIdPresent == 1 {
		threadRootIdValue, err := decoder.ReadUint64(runtime.BigEndian)
		if err != nil {
			return nil, fmt.Errorf("failed to decode value: %w", err)
		}
		result.ThreadRootId = &threadRootIdValue
	}

	authorUserIdPresent, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode author_user_id presence: %w", err)
	}
	if authorUserIdPresent == 1 {
		authorUserIdValue, err := decoder.ReadUint64(runtime.BigEndian)
		if err != nil {
			return nil, fmt.Errorf("failed to decode value: %w", err)
		}
		result.AuthorUserId = &authorUserIdValue
	}

	authorNicknameLength, err := decoder.ReadUint16(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode author_nickname length: %w", err)
	}
	authorNicknameBytes, err := decoder.ReadBytesSlice(int(authorNicknameLength))
	if err != nil {
		return nil, fmt.Errorf("failed to decode author_nickname: %w", err)
	}
	result.AuthorNickname = string(authorNicknameBytes)

	contentLength, err := decoder.ReadUint16(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode content length: %w", err)
	}
	contentBytes, err := decoder.ReadBytesSlice(int(contentLength))
	if err != nil {
		return nil, fmt.Errorf("failed to decode content: %w", err)
	}
	result.Content = string(contentBytes)

	createdAt, err := decoder.ReadInt64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode created_at: %w", err)
	}
	result.CreatedAt = createdAt

	editedAtPresent, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode edited_at presence: %w", err)
	}
	if editedAtPresent == 1 {
		editedAtValue, err := decoder.ReadInt64(runtime.BigEndian)
		if err != nil {
			return nil, fmt.Errorf("failed to decode value: %w", err)
		}
		result.EditedAt = &editedAtValue
	}

	deletedAtPresent, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode deleted_at presence: %w", err)
	}
	if deletedAtPresent == 1 {
		deletedAtValue, err := decoder.ReadInt64(runtime.BigEndian)
		if err != nil {
			return nil, fmt.Errorf("failed to decode value: %w", err)
		}
		result.DeletedAt = &deletedAtValue
	}


	return result, nil
}

type BackfillEnd struct {
	ChannelId uint64
	MessageCount uint32
}

func (m *BackfillEnd) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *BackfillEnd) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"channel_id": m.ChannelId,
		"message_count": m.MessageCount,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint64(m.ChannelId, runtime.BigEndian)
	encoder.WriteUint32(m.MessageCount, runtime.BigEndian)

	return encoder.Finish(), nil
}

func (m *BackfillEnd) CalculateSize() int {
	size := 0

	size += 8 // ChannelId
	size += 4 // MessageCount

	return size
}

func DecodeBackfillEnd(bytes []byte) (*BackfillEnd, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeBackfillEndWithDecoder(decoder)
}

func decodeBackfillEndWithDecoder(decoder *runtime.BitStreamDecoder) (*BackfillEnd, error) {
	result := &BackfillEnd{}

	channelId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode channel_id: %w", err)
	}
	result.ChannelId = channelId

	messageCount, err := decoder.ReadUint32(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode message_count: %w", err)
	}
	result.MessageCount = messageCount


	return result, nil
}

type MessageEdited struct {
	MessageId uint64
	ChannelId uint64
	NewContent string
	EditedAt int64
}

func (m *MessageEdited) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *MessageEdited) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"message_id": m.MessageId,
		"channel_id": m.ChannelId,
		"new_content": m.NewContent,
		"edited_at": m.EditedAt,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint64(m.MessageId, runtime.BigEndian)
	encoder.WriteUint64(m.ChannelId, runtime.BigEndian)
	m_NewContent_bytes := []byte(m.NewContent)
	encoder.WriteUint16(uint16(len(m_NewContent_bytes)), runtime.BigEndian)
	for _, b := range m_NewContent_bytes {
		encoder.WriteUint8(b)
	}
	encoder.WriteInt64(m.EditedAt, runtime.BigEndian)

	return encoder.Finish(), nil
}

func (m *MessageEdited) CalculateSize() int {
	size := 0

	size += 8 // MessageId
	size += 8 // ChannelId
	size += 2 + len(m.NewContent) // NewContent (length-prefixed string)
	size += 8 // EditedAt

	return size
}

func DecodeMessageEdited(bytes []byte) (*MessageEdited, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeMessageEditedWithDecoder(decoder)
}

func decodeMessageEditedWithDecoder(decoder *runtime.BitStreamDecoder) (*MessageEdited, error) {
	result := &MessageEdited{}

	messageId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode message_id: %w", err)
	}
	result.MessageId = messageId

	channelId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode channel_id: %w", err)
	}
	result.ChannelId = channelId

	newContentLength, err := decoder.ReadUint16(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode new_content length: %w", err)
	}
	newContentBytes, err := decoder.ReadBytesSlice(int(newContentLength))
	if err != nil {
		return nil, fmt.Errorf("failed to decode new_content: %w", err)
	}
	result.NewContent = string(newContentBytes)

	editedAt, err := decoder.ReadInt64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode edited_at: %w", err)
	}
	result.EditedAt = editedAt


	return result, nil
}

type MessageDeleted struct {
	MessageId uint64
	ChannelId uint64
	Content string
	DeletedAt int64
}

func (m *MessageDeleted) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *MessageDeleted) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"message_id": m.MessageId,
		"channel_id": m.ChannelId,
		"content": m.Content,
		"deleted_at": m.DeletedAt,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint64(m.MessageId, runtime.BigEndian)
	encoder.WriteUint64(m.ChannelId, runtime.BigEndian)
	m_Content_bytes := []byte(m.Content)
	encoder.WriteUint16(uint16(len(m_Content_bytes)), runtime.BigEndian)
	for _, b := range m_Content_bytes {
		encoder.WriteUint8(b)
	}
	encoder.WriteInt64(m.DeletedAt, runtime.BigEndian)

	return encoder.Finish(), nil
}

func (m *MessageDeleted) CalculateSize() int {
	size := 0

	size += 8 // MessageId
	size += 8 // ChannelId
	size += 2 + len(m.Content) // Content (length-prefixed string)
	size += 8 // DeletedAt

	return size
}

func DecodeMessageDeleted(bytes []byte) (*MessageDeleted, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeMessageDeletedWithDecoder(decoder)
}

func decodeMessageDeletedWithDecoder(decoder *runtime.BitStreamDecoder) (*MessageDeleted, error) {
	result := &MessageDeleted{}

	messageId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode message_id: %w", err)
	}
	result.MessageId = messageId

	channelId, err := decoder.ReadUint64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode channel_id: %w", err)
	}
	result.ChannelId = channelId

	contentLength, err := decoder.ReadUint16(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode content length: %w", err)
	}
	contentBytes, err := decoder.ReadBytesSlice(int(contentLength))
	if err != nil {
		return nil, fmt.Errorf("failed to decode content: %w", err)
	}
	result.Content = string(contentBytes)

	deletedAt, err := decoder.ReadInt64(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode deleted_at: %w", err)
	}
	result.DeletedAt = deletedAt


	return result, nil
}

// MessageCode is an enum type
type MessageCode uint8

const (
	MessageCodeHANDSHAKE MessageCode = 1
	MessageCodeHANDSHAKEACK MessageCode = 129
	MessageCodeCHANNELINFO MessageCode = 2
	MessageCodeBACKFILLSTART MessageCode = 3
	MessageCodeMESSAGESYNC MessageCode = 4
	MessageCodeBACKFILLEND MessageCode = 5
	MessageCodeMESSAGEEDITED MessageCode = 6
	MessageCodeMESSAGEDELETED MessageCode = 7
)

func (e MessageCode) String() string {
	switch e {
	case 1:
		return "HANDSHAKE"
	case 129:
		return "HANDSHAKE_ACK"
	case 2:
		return "CHANNEL_INFO"
	case 3:
		return "BACKFILL_START"
	case 4:
		return "MESSAGE_SYNC"
	case 5:
		return "BACKFILL_END"
	case 6:
		return "MESSAGE_EDITED"
	case 7:
		return "MESSAGE_DELETED"
	default:
		return fmt.Sprintf("MessageCode(%d)", e)
	}
}

func (m MessageCode) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m MessageCode) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)
	encoder.WriteUint8(uint8(m))
	return encoder.Finish(), nil
}

func (m MessageCode) CalculateSize() int {
	return 1
}

func DecodeMessageCode(data []byte) (*MessageCode, error) {
	decoder := runtime.NewBitStreamDecoder(data, runtime.MSBFirst)
	return decodeMessageCodeWithDecoder(decoder)
}

func decodeMessageCodeWithDecoder(decoder *runtime.BitStreamDecoder) (*MessageCode, error) {
	val, err := decoder.ReadUint8()
	if err != nil {
		return nil, fmt.Errorf("failed to decode MessageCode: %w", err)
	}
	switch val {
	case 1, 129, 2, 3, 4, 5, 6, 7:
		result := MessageCode(val)
		return &result, nil
	default:
		return nil, fmt.Errorf("invalid MessageCode value: %d", val)
	}
}

type Frame struct {
	Length uint32
	Type MessageCode
	Payload interface{}
}

func (m *Frame) Encode() ([]byte, error) {
	return m.EncodeWithContext(runtime.NewEncodingContext())
}

func (m *Frame) EncodeWithContext(ctx *runtime.EncodingContext) ([]byte, error) {
	encoder := runtime.NewBitStreamEncoder(runtime.MSBFirst)

	// Build parent context for nested struct encoding
	parentFields := map[string]interface{}{
		"length": m.Length,
		"type": m.Type,
		"payload": m.Payload,
	}
	childCtx := ctx.ExtendWithParent(parentFields)
	_ = childCtx // Used by nested struct encoding

	encoder.WriteUint32(m.Length, runtime.BigEndian)
	m_Type_ctx := childCtx.WithByteOffset(ctx.ByteOffset + encoder.Position())
	m_Type_bytes, err := m.Type.EncodeWithContext(m_Type_ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range m_Type_bytes {
		encoder.WriteUint8(b)
	}
	// Encode discriminated union variant
	switch v := m.Payload.(type) {
	case *Handshake:
		variantBytes, err := v.EncodeWithContext(childCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to encode Handshake variant: %w", err)
		}
		for _, b := range variantBytes {
			encoder.WriteUint8(b)
		}
	case *HandshakeAck:
		variantBytes, err := v.EncodeWithContext(childCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to encode HandshakeAck variant: %w", err)
		}
		for _, b := range variantBytes {
			encoder.WriteUint8(b)
		}
	case *ChannelInfo:
		variantBytes, err := v.EncodeWithContext(childCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to encode ChannelInfo variant: %w", err)
		}
		for _, b := range variantBytes {
			encoder.WriteUint8(b)
		}
	case *BackfillStart:
		variantBytes, err := v.EncodeWithContext(childCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to encode BackfillStart variant: %w", err)
		}
		for _, b := range variantBytes {
			encoder.WriteUint8(b)
		}
	case *MessageSync:
		variantBytes, err := v.EncodeWithContext(childCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to encode MessageSync variant: %w", err)
		}
		for _, b := range variantBytes {
			encoder.WriteUint8(b)
		}
	case *BackfillEnd:
		variantBytes, err := v.EncodeWithContext(childCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to encode BackfillEnd variant: %w", err)
		}
		for _, b := range variantBytes {
			encoder.WriteUint8(b)
		}
	case *MessageEdited:
		variantBytes, err := v.EncodeWithContext(childCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to encode MessageEdited variant: %w", err)
		}
		for _, b := range variantBytes {
			encoder.WriteUint8(b)
		}
	case *MessageDeleted:
		variantBytes, err := v.EncodeWithContext(childCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to encode MessageDeleted variant: %w", err)
		}
		for _, b := range variantBytes {
			encoder.WriteUint8(b)
		}
	default:
		return nil, fmt.Errorf("unknown discriminated union variant type: %T", m.Payload)
	}

	return encoder.Finish(), nil
}

func (m *Frame) CalculateSize() int {
	size := 0

	size += 4 // Length
	size += 1 // Type (enum)
	// Payload: inline discriminated union
	if sizable, ok := m.Payload.(interface{ CalculateSize() int }); ok {
		size += sizable.CalculateSize()
	} else if m.Payload != nil {
		Payload_bytes, _ := m.Payload.(interface{ Encode() ([]byte, error) }).Encode()
		size += len(Payload_bytes)
	}

	return size
}

func DecodeFrame(bytes []byte) (*Frame, error) {
	decoder := runtime.NewBitStreamDecoder(bytes, runtime.MSBFirst)
	return decodeFrameWithDecoder(decoder)
}

func decodeFrameWithDecoder(decoder *runtime.BitStreamDecoder) (*Frame, error) {
	result := &Frame{}

	length, err := decoder.ReadUint32(runtime.BigEndian)
	if err != nil {
		return nil, fmt.Errorf("failed to decode length: %w", err)
	}
	result.Length = length

	type_, err := decodeMessageCodeWithDecoder(decoder)
	if err != nil {
		return nil, fmt.Errorf("failed to decode type: %w", err)
	}
	result.Type = *type_

	if result.Type == MessageCodeHANDSHAKE {
		variantValue, err := decodeHandshakeWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode Handshake variant: %w", err)
		}
		result.Payload = variantValue
	} else if result.Type == MessageCodeHANDSHAKEACK {
		variantValue, err := decodeHandshakeAckWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode HandshakeAck variant: %w", err)
		}
		result.Payload = variantValue
	} else if result.Type == MessageCodeCHANNELINFO {
		variantValue, err := decodeChannelInfoWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode ChannelInfo variant: %w", err)
		}
		result.Payload = variantValue
	} else if result.Type == MessageCodeBACKFILLSTART {
		variantValue, err := decodeBackfillStartWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode BackfillStart variant: %w", err)
		}
		result.Payload = variantValue
	} else if result.Type == MessageCodeMESSAGESYNC {
		variantValue, err := decodeMessageSyncWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode MessageSync variant: %w", err)
		}
		result.Payload = variantValue
	} else if result.Type == MessageCodeBACKFILLEND {
		variantValue, err := decodeBackfillEndWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode BackfillEnd variant: %w", err)
		}
		result.Payload = variantValue
	} else if result.Type == MessageCodeMESSAGEEDITED {
		variantValue, err := decodeMessageEditedWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode MessageEdited variant: %w", err)
		}
		result.Payload = variantValue
	} else if result.Type == MessageCodeMESSAGEDELETED {
		variantValue, err := decodeMessageDeletedWithDecoder(decoder)
		if err != nil {
			return nil, fmt.Errorf("failed to decode MessageDeleted variant: %w", err)
		}
		result.Payload = variantValue
	} else {
		return nil, fmt.Errorf("unknown discriminator value for payload: %v", result.Type)
	}


	return result, nil
}
