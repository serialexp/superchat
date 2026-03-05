package server

import (
	"fmt"
	"log"

	"github.com/aeolun/superchat/pkg/archive"
	gen "github.com/aeolun/superchat/pkg/archive/generated"
	"github.com/aeolun/superchat/pkg/database"
	"github.com/aeolun/superchat/pkg/protocol"
)

// archiveProvider implements archive.BackfillProvider using the server's MemDB.
type archiveProvider struct {
	db     *database.MemDB
	server *Server
}

func (p *archiveProvider) ArchiveChannels() []archive.ChannelMeta {
	channels, err := p.db.ListChannels()
	if err != nil {
		log.Printf("[archive] failed to list channels: %v", err)
		return nil
	}

	var result []archive.ChannelMeta
	for _, ch := range channels {
		if !p.server.resolveArchiveEnabled(ch) {
			continue
		}
		result = append(result, archive.ChannelMeta{
			ID:             ch.ID,
			Name:           ch.Name,
			Description:    safeDeref(ch.Description, ""),
			ChannelType:    ch.ChannelType,
			RetentionHours: ch.MessageRetentionHours,
			ArchiveEnabled: true,
		})
	}
	return result
}

func (p *archiveProvider) ArchiveMessages(channelID int64, afterMessageID int64) []archive.MessageMeta {
	msgs, err := p.db.GetMessagesAfter(channelID, afterMessageID)
	if err != nil {
		log.Printf("[archive] failed to get messages for channel %d: %v", channelID, err)
		return nil
	}

	result := make([]archive.MessageMeta, len(msgs))
	for i, m := range msgs {
		result[i] = archive.MessageMeta{
			ID:             m.ID,
			ChannelID:      m.ChannelID,
			ParentID:       m.ParentID,
			ThreadRootID:   m.ThreadRootID,
			AuthorUserID:   m.AuthorUserID,
			AuthorNickname: p.server.archiveDisplayNickname(p.db, m),
			Content:        m.Content,
			CreatedAt:      m.CreatedAt,
			EditedAt:       m.EditedAt,
			DeletedAt:      m.DeletedAt,
		}
	}
	return result
}

// resolveArchiveEnabled resolves the effective archive status for a channel.
// Per-channel override takes precedence; otherwise falls back to server config.
func (s *Server) resolveArchiveEnabled(ch *database.Channel) bool {
	if ch.ArchiveEnabled != nil {
		return *ch.ArchiveEnabled
	}
	return s.config.ArchiveEnabled
}

// archiveNewMessage forwards a new message to the archive client if archiving is enabled.
func (s *Server) archiveNewMessage(dbMsg *database.Message) {
	if s.archiveClient == nil {
		return
	}

	s.archiveClient.EnqueueMessage(&gen.MessageSync{
		MessageId:      uint64(dbMsg.ID),
		ChannelId:      uint64(dbMsg.ChannelID),
		ParentId:       int64PtrToUint64Ptr(dbMsg.ParentID),
		ThreadRootId:   int64PtrToUint64Ptr(dbMsg.ThreadRootID),
		AuthorUserId:   int64PtrToUint64Ptr(dbMsg.AuthorUserID),
		AuthorNickname: s.archiveDisplayNickname(s.db, dbMsg),
		Content:        dbMsg.Content,
		CreatedAt:      dbMsg.CreatedAt,
		EditedAt:       dbMsg.EditedAt,
		DeletedAt:      dbMsg.DeletedAt,
	})
}

// archiveMessageEdited forwards a message edit to the archive client.
func (s *Server) archiveMessageEdited(dbMsg *database.Message) {
	if s.archiveClient == nil {
		return
	}

	editedAt := int64(0)
	if dbMsg.EditedAt != nil {
		editedAt = *dbMsg.EditedAt
	}

	s.archiveClient.EnqueueEdit(&gen.MessageEdited{
		MessageId:  uint64(dbMsg.ID),
		ChannelId:  uint64(dbMsg.ChannelID),
		NewContent: dbMsg.Content,
		EditedAt:   editedAt,
	})
}

// archiveMessageDeleted forwards a message deletion to the archive client.
func (s *Server) archiveMessageDeleted(dbMsg *database.Message) {
	if s.archiveClient == nil {
		return
	}

	deletedAt := int64(0)
	if dbMsg.DeletedAt != nil {
		deletedAt = *dbMsg.DeletedAt
	}

	s.archiveClient.EnqueueDelete(&gen.MessageDeleted{
		MessageId: uint64(dbMsg.ID),
		ChannelId: uint64(dbMsg.ChannelID),
		Content:   dbMsg.Content,
		DeletedAt: deletedAt,
	})
}

// archiveDisplayNickname applies the same prefix logic as the protocol layer:
// ~ for anonymous, $ for admin, @ for moderator, plain for registered users.
func (s *Server) archiveDisplayNickname(db *database.MemDB, dbMsg *database.Message) string {
	nickname := dbMsg.AuthorNickname
	if dbMsg.AuthorUserID != nil {
		user, err := db.GetUserByID(*dbMsg.AuthorUserID)
		if err == nil {
			prefix := protocol.UserFlags(user.UserFlags).DisplayPrefix()
			nickname = prefix + user.Nickname
		} else {
			nickname = "<user:" + fmt.Sprint(*dbMsg.AuthorUserID) + ">"
		}
	} else if dbMsg.AuthorNickname != "" {
		nickname = "~" + dbMsg.AuthorNickname
	}
	return nickname
}

func int64PtrToUint64Ptr(p *int64) *uint64 {
	if p == nil {
		return nil
	}
	v := uint64(*p)
	return &v
}
