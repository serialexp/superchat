package moderation

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"strings"
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

	Table    string
	IDColumn string

	// ContentColumns are all the text columns to censor. They are handled in a
	// single pass rather than one pass each, because a message's author nickname
	// lives in the same row as its content and scanning the table twice to reach
	// them would double the startup cost for nothing.
	ContentColumns []string
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
	id int64
	// values holds one entry per ContentColumns, in order. Columns that did not
	// change carry their original text, so a single prepared statement can write
	// every row regardless of which columns actually matched.
	values []string
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
	if len(t.ContentColumns) == 0 {
		return stats, fmt.Errorf("moderation: no content columns given")
	}
	for _, name := range append([]string{t.Table, t.IDColumn}, t.ContentColumns...) {
		if !identifierPattern.MatchString(name) {
			return stats, fmt.Errorf("moderation: %q is not a usable SQL identifier", name)
		}
	}

	assignments := make([]string, len(t.ContentColumns))
	for i, col := range t.ContentColumns {
		assignments[i] = col + " = ?"
	}

	selectSQL := fmt.Sprintf(
		"SELECT %s, %s FROM %s WHERE %s > ? ORDER BY %s LIMIT %d",
		t.IDColumn, strings.Join(t.ContentColumns, ", "), t.Table, t.IDColumn, t.IDColumn, backfillBatch,
	)
	updateSQL := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s = ?",
		t.Table, strings.Join(assignments, ", "), t.IDColumn,
	)

	updates := make([]pendingUpdate, 0, backfillBatch)
	lastID := int64(math.MinInt64)

	for {
		updates = updates[:0]

		batchRows, maxID, err := f.scanBatch(ctx, t.Read, selectSQL, lastID, len(t.ContentColumns), &updates)
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
func (f *Filter) scanBatch(ctx context.Context, db *sql.DB, query string, afterID int64, numCols int, updates *[]pendingUpdate) (rowCount int, maxID int64, err error) {
	rows, err := db.QueryContext(ctx, query, afterID)
	if err != nil {
		return 0, afterID, err
	}
	defer rows.Close()

	// Scan targets are reused across rows; only the rows that change get a copy.
	var id int64
	values := make([]string, numCols)
	dest := make([]any, 0, numCols+1)
	dest = append(dest, &id)
	for i := range values {
		dest = append(dest, &values[i])
	}

	// Censored text lands in scratch first. Allocating a per-row slice up front
	// would allocate once per row scanned; the overwhelming majority of rows
	// match nothing, so the copy only happens for the few that do.
	scratch := make([]string, numCols)

	maxID = afterID
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			return rowCount, maxID, err
		}
		rowCount++
		maxID = id

		var rowChanged bool
		for i, v := range values {
			censored, changed := f.Censor(v)
			scratch[i] = censored
			rowChanged = rowChanged || changed
		}
		if rowChanged {
			row := make([]string, numCols)
			copy(row, scratch)
			*updates = append(*updates, pendingUpdate{id: id, values: row})
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

	args := make([]any, 0, len(updates[0].values)+1)
	for _, u := range updates {
		args = args[:0]
		for _, v := range u.values {
			args = append(args, v)
		}
		args = append(args, u.id)

		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("row %d: %w", u.id, err)
		}
	}
	return tx.Commit()
}
