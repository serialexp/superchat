package database

import (
	"context"

	"github.com/aeolun/superchat/pkg/moderation"
)

// CensorMessages applies the word filter to message content that is already
// stored, rewriting only the rows it changes.
//
// Call this before loading the MemDB so the in-memory copy is built from clean
// content. Reads go through the read pool and writes through the dedicated write
// connection, matching the rest of this package.
//
// MessageVersion is deliberately left alone. It is insert-only — nothing in the
// codebase ever selects from it and no protocol message exposes it — so it is not
// a way for uncensored text to reach a client, and it is the only place the
// original wording survives for moderation purposes.
func (db *DB) CensorMessages(ctx context.Context, f *moderation.Filter) (moderation.BackfillStats, error) {
	return f.Backfill(ctx, moderation.BackfillTarget{
		Read:          db.conn,
		Write:         db.writeConn,
		Table:         "Message",
		IDColumn:      "id",
		ContentColumn: "content",
	})
}
