package archive

import (
	"net"
	"testing"

	gen "github.com/aeolun/superchat/pkg/archive/generated"
)

func TestFramingRoundTrip(t *testing.T) {
	// Create a pipe to simulate a connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sConn := NewConn(server)
	cConn := NewConn(client)

	tests := []struct {
		name    string
		msgType uint8
		msg     interface{ Encode() ([]byte, error) }
		decode  func([]byte) (interface{}, error)
		verify  func(t *testing.T, decoded interface{})
	}{
		{
			name:    "Handshake",
			msgType: TypeHandshake,
			msg:     &gen.Handshake{ServerName: "test-server", ProtocolVersion: 1},
			decode:  func(b []byte) (interface{}, error) { return gen.DecodeHandshake(b) },
			verify: func(t *testing.T, decoded interface{}) {
				hs := decoded.(*gen.Handshake)
				if hs.ServerName != "test-server" {
					t.Errorf("ServerName = %q, want %q", hs.ServerName, "test-server")
				}
				if hs.ProtocolVersion != 1 {
					t.Errorf("ProtocolVersion = %d, want 1", hs.ProtocolVersion)
				}
			},
		},
		{
			name:    "ChannelInfo",
			msgType: TypeChannelInfo,
			msg: &gen.ChannelInfo{
				ChannelId: 42, Name: "general", Description: "General chat",
				ChannelType: 0, RetentionHours: 168, ArchiveEnabled: 1,
			},
			decode: func(b []byte) (interface{}, error) { return gen.DecodeChannelInfo(b) },
			verify: func(t *testing.T, decoded interface{}) {
				ci := decoded.(*gen.ChannelInfo)
				if ci.ChannelId != 42 {
					t.Errorf("ChannelId = %d, want 42", ci.ChannelId)
				}
				if ci.Name != "general" {
					t.Errorf("Name = %q, want %q", ci.Name, "general")
				}
				if ci.ArchiveEnabled != 1 {
					t.Errorf("ArchiveEnabled = %d, want 1", ci.ArchiveEnabled)
				}
			},
		},
		{
			name:    "MessageSync",
			msgType: TypeMessageSync,
			msg: &gen.MessageSync{
				MessageId: 100, ChannelId: 42,
				AuthorNickname: "alice", Content: "hello world",
				CreatedAt: 1700000000000,
			},
			decode: func(b []byte) (interface{}, error) { return gen.DecodeMessageSync(b) },
			verify: func(t *testing.T, decoded interface{}) {
				ms := decoded.(*gen.MessageSync)
				if ms.MessageId != 100 {
					t.Errorf("MessageId = %d, want 100", ms.MessageId)
				}
				if ms.Content != "hello world" {
					t.Errorf("Content = %q, want %q", ms.Content, "hello world")
				}
				if ms.ParentId != nil {
					t.Errorf("ParentId = %v, want nil", ms.ParentId)
				}
			},
		},
		{
			name:    "MessageSync with optional fields",
			msgType: TypeMessageSync,
			msg: func() *gen.MessageSync {
				parentID := uint64(50)
				threadRoot := uint64(50)
				editedAt := int64(1700000001000)
				return &gen.MessageSync{
					MessageId: 101, ChannelId: 42,
					ParentId: &parentID, ThreadRootId: &threadRoot,
					AuthorNickname: "bob", Content: "reply",
					CreatedAt: 1700000001000, EditedAt: &editedAt,
				}
			}(),
			decode: func(b []byte) (interface{}, error) { return gen.DecodeMessageSync(b) },
			verify: func(t *testing.T, decoded interface{}) {
				ms := decoded.(*gen.MessageSync)
				if ms.ParentId == nil || *ms.ParentId != 50 {
					t.Errorf("ParentId = %v, want 50", ms.ParentId)
				}
				if ms.EditedAt == nil || *ms.EditedAt != 1700000001000 {
					t.Errorf("EditedAt = %v, want 1700000001000", ms.EditedAt)
				}
				if ms.DeletedAt != nil {
					t.Errorf("DeletedAt = %v, want nil", ms.DeletedAt)
				}
			},
		},
		{
			name:    "MessageEdited",
			msgType: TypeMessageEdited,
			msg:     &gen.MessageEdited{MessageId: 100, ChannelId: 42, NewContent: "edited content", EditedAt: 1700000002000},
			decode:  func(b []byte) (interface{}, error) { return gen.DecodeMessageEdited(b) },
			verify: func(t *testing.T, decoded interface{}) {
				me := decoded.(*gen.MessageEdited)
				if me.NewContent != "edited content" {
					t.Errorf("NewContent = %q, want %q", me.NewContent, "edited content")
				}
			},
		},
		{
			name:    "MessageDeleted",
			msgType: TypeMessageDeleted,
			msg:     &gen.MessageDeleted{MessageId: 100, ChannelId: 42, Content: "[deleted by ~alice]", DeletedAt: 1700000003000},
			decode:  func(b []byte) (interface{}, error) { return gen.DecodeMessageDeleted(b) },
			verify: func(t *testing.T, decoded interface{}) {
				md := decoded.(*gen.MessageDeleted)
				if md.Content != "[deleted by ~alice]" {
					t.Errorf("Content = %q, want %q", md.Content, "[deleted by ~alice]")
				}
				if md.DeletedAt != 1700000003000 {
					t.Errorf("DeletedAt = %d, want 1700000003000", md.DeletedAt)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write in a goroutine since net.Pipe is synchronous
			errCh := make(chan error, 1)
			go func() {
				errCh <- sConn.WriteMessage(tt.msgType, tt.msg)
			}()

			msgType, payload, err := cConn.ReadMessage()
			if err != nil {
				t.Fatalf("ReadMessage failed: %v", err)
			}

			if err := <-errCh; err != nil {
				t.Fatalf("WriteMessage failed: %v", err)
			}

			if msgType != tt.msgType {
				t.Fatalf("msgType = 0x%02x, want 0x%02x", msgType, tt.msgType)
			}

			decoded, err := tt.decode(payload)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			tt.verify(t, decoded)
		})
	}
}

func TestDecodePayloadAllTypes(t *testing.T) {
	// Verify DecodePayload dispatches correctly for all types
	types := []struct {
		msgType uint8
		msg     interface{ Encode() ([]byte, error) }
	}{
		{TypeHandshake, &gen.Handshake{ServerName: "s", ProtocolVersion: 1}},
		{TypeHandshakeAck, &gen.HandshakeAck{Success: 1, ChannelCount: 0, Channels: []gen.ChannelState{}}},
		{TypeChannelInfo, &gen.ChannelInfo{ChannelId: 1, Name: "a", Description: "b"}},
		{TypeBackfillStart, &gen.BackfillStart{ChannelId: 1, TotalMessages: 10}},
		{TypeMessageSync, &gen.MessageSync{MessageId: 1, ChannelId: 1, AuthorNickname: "a", Content: "b", CreatedAt: 1}},
		{TypeBackfillEnd, &gen.BackfillEnd{ChannelId: 1, MessageCount: 10}},
		{TypeMessageEdited, &gen.MessageEdited{MessageId: 1, ChannelId: 1, NewContent: "c", EditedAt: 1}},
		{TypeMessageDeleted, &gen.MessageDeleted{MessageId: 1, ChannelId: 1, Content: "[deleted by ~x]", DeletedAt: 1}},
	}

	for _, tt := range types {
		payload, err := tt.msg.Encode()
		if err != nil {
			t.Fatalf("encode type 0x%02x: %v", tt.msgType, err)
		}

		decoded, err := DecodePayload(tt.msgType, payload)
		if err != nil {
			t.Fatalf("DecodePayload type 0x%02x: %v", tt.msgType, err)
		}

		if decoded == nil {
			t.Fatalf("DecodePayload type 0x%02x returned nil", tt.msgType)
		}
	}
}

func TestDecodePayloadUnknownType(t *testing.T) {
	_, err := DecodePayload(0xFF, []byte{})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestHandshakeAckWithChannels(t *testing.T) {
	ack := &gen.HandshakeAck{
		Success:      1,
		ChannelCount: 2,
		Channels: []gen.ChannelState{
			{ChannelId: 1, LastMessageId: 100},
			{ChannelId: 2, LastMessageId: 200},
		},
	}

	payload, err := ack.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := gen.DecodeHandshakeAck(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Success != 1 {
		t.Errorf("Success = %d, want 1", decoded.Success)
	}
	if decoded.ChannelCount != 2 {
		t.Errorf("ChannelCount = %d, want 2", decoded.ChannelCount)
	}
	if len(decoded.Channels) != 2 {
		t.Fatalf("len(Channels) = %d, want 2", len(decoded.Channels))
	}
	if decoded.Channels[0].ChannelId != 1 || decoded.Channels[0].LastMessageId != 100 {
		t.Errorf("Channel[0] = %+v, want {1, 100}", decoded.Channels[0])
	}
	if decoded.Channels[1].ChannelId != 2 || decoded.Channels[1].LastMessageId != 200 {
		t.Errorf("Channel[1] = %+v, want {2, 200}", decoded.Channels[1])
	}
}
