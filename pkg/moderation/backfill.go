package moderation

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
)

// BackfillStats reports what a Backfill pass did.
type BackfillStats struct {
	Scanned int // rows examined
	Changed int // rows rewritten
}

// BackfillTarget identifies the table and columns Backfill should rewrite.
type BackfillTarget struct {
	// Read is the handle used for the streaming SELECT, Write for the UPDATEs.
	// They may be the same handle — the archiver passes one for both. The split
	// exists because the superchat server keeps a dedicated single-connection
	// writer alongside a read pool, and writes must go through the former.
	Read  *sql.DB
	Write *sql.DB

	Table         string
	IDColumn      string
	ContentColumn string
}

// backfillBatch is how many rows are read, and then written, per round trip.
// Message content is capped at a few kilobytes by the protocol, so one batch
// holds a couple of megabytes at worst regardless of how large the table is.
const backfillBatch = 500

// identifierPattern guards the table and column names that get interpolated into
// SQL. Every caller passes a compile-time constant today; this is here so that
// stays true.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type pendingUpdate struct {
	id      int64
	content string
}

// Backfill applies the filter to content that is already stored, rewriting only
// the rows it actually changes.
//
// Censor at write time cannot reach content that predates it, so this is what
// cleans up messages posted before the filter was switched on — or before a term
// was added to the list. It is safe and cheap to run on every startup: a pass
// over already-censored content changes nothing and writes nothing.
//
// Rows are paged by id rather than read from one long-lived cursor, because
// SQLite does not appreciate writes issued against a table while a read cursor
// is open on it. Memory stays bounded to a single batch however large the table
// grows, and the write transaction is held for one batch at a time rather than
// for the whole pass.
//
// A nil *Filter is a no-op, matching Censor.
func (f *Filter) Backfill(ctx context.Context, t BackfillTarget) (BackfillStats, error) {
	var stats BackfillStats
	if f == nil {
		return stats, nil
	}
	for _, name := range []string{t.Table, t.IDColumn, t.ContentColumn} {
		if !identifierPattern.MatchString(name) {
			return stats, fmt.Errorf("moderation: %q is not a usable SQL identifier", name)
		}
	}

	selectSQL := fmt.Sprintf(
		"SELECT %s, %s FROM %s WHERE %s > ? ORDER BY %s LIMIT %d",
		t.IDColumn, t.ContentColumn, t.Table, t.IDColumn, t.IDColumn, backfillBatch,
	)
	updateSQL := fmt.Sprintf(
		"UPDATE %s SET %s = ? WHERE %s = ?",
		t.Table, t.ContentColumn, t.IDColumn,
	)

	updates := make([]pendingUpdate, 0, backfillBatch)
	lastID := int64(math.MinInt64)

	for {
		updates = updates[:0]

		batchRows, maxID, err := f.scanBatch(ctx, t.Read, selectSQL, lastID, &updates)
		if err != nil {
			return stats, fmt.Errorf("moderation: scanning %s: %w", t.Table, err)
		}
		stats.Scanned += batchRows

		if len(updates) > 0 {
			if err := applyUpdates(ctx, t.Write, updateSQL, updates); err != nil {
				return stats, fmt.Errorf("moderation: rewriting %s: %w", t.Table, err)
			}
			stats.Changed += len(updates)
		}

		// A short batch means the table is exhausted.
		if batchRows < backfillBatch {
			return stats, nil
		}
		lastID = maxID
	}
}

// scanBatch reads one page of rows, appending the ones the filter changes to
// updates. The read cursor is closed before it returns, so the caller is free to
// write.
func (f *Filter) scanBatch(ctx context.Context, db *sql.DB, query string, afterID int64, updates *[]pendingUpdate) (rowCount int, maxID int64, err error) {
	rows, err := db.QueryContext(ctx, query, afterID)
	if err != nil {
		return 0, afterID, err
	}
	defer rows.Close()

	maxID = afterID
	for rows.Next() {
		var id int64
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			return rowCount, maxID, err
		}
		rowCount++
		maxID = id

		if censored, changed := f.Censor(content); changed {
			*updates = append(*updates, pendingUpdate{id: id, content: censored})
		}
	}
	return rowCount, maxID, rows.Err()
}

// applyUpdates writes one batch in a single transaction, so a failure part way
// through a large table leaves whole batches applied rather than half a row.
func applyUpdates(ctx context.Context, db *sql.DB, updateSQL string, updates []pendingUpdate) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op once Commit succeeds

	stmt, err := tx.PrepareContext(ctx, updateSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, u := range updates {
		if _, err := stmt.ExecContext(ctx, u.content, u.id); err != nil {
			return fmt.Errorf("row %d: %w", u.id, err)
		}
	}
	return tx.Commit()
}
