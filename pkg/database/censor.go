package database

import (
	"context"

	"github.com/aeolun/superchat/pkg/moderation"
)

// CensorMessages applies the word filter to message text that is already stored,
// rewriting only the rows it changes.
//
// Both the body and the denormalized author nickname are covered, in one pass.
// The nickname matters because anonymous authors are displayed straight from
// Message.author_nickname, so somebody who called themselves a slur before the
// filter existed still appears in every message header otherwise. New nicknames
// are refused outright rather than censored; this only cleans up what is already
// recorded. Registered users display from User.nickname instead, which is left
// alone here because it is UNIQUE and censoring it could collide.
//
// Call this before loading the MemDB so the in-memory copy is built from clean
// text. Reads go through the read pool and writes through the dedicated write
// connection, matching the rest of this package.
//
// MessageVersion is deliberately left alone. It is insert-only — nothing in the
// codebase ever selects from it and no protocol message exposes it — so it is not
// a way for uncensored text to reach a client, and it is the only place the
// original wording survives for moderation purposes.
func (db *DB) CensorMessages(ctx context.Context, f *moderation.Filter) (moderation.BackfillStats, error) {
	return f.Backfill(ctx, moderation.BackfillTarget{
		Read:           db.conn,
		Write:          db.writeConn,
		Table:          "Message",
		IDColumn:       "id",
		ContentColumns: []string{"content", "author_nickname"},
	})
}
