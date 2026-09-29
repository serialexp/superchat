package moderation

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func newBackfillDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE Message (id INTEGER PRIMARY KEY, content TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

// insertRows seeds the fixture in one transaction. Row-at-a-time inserts commit
// (and fsync) individually, which made the batch-boundary tests take seconds.
func insertRows(t *testing.T, db *sql.DB, contents ...string) []int64 {
	t.Helper()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO Message (content) VALUES (?)`)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer stmt.Close()

	ids := make([]int64, 0, len(contents))
	for _, c := range contents {
		res, err := stmt.Exec(c)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("last insert id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return ids
}

func readContent(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	var content string
	if err := db.QueryRow(`SELECT content FROM Message WHERE id = ?`, id).Scan(&content); err != nil {
		t.Fatalf("select id %d: %v", id, err)
	}
	return content
}

func messageTarget(db *sql.DB, columns ...string) BackfillTarget {
	if len(columns) == 0 {
		columns = []string{"content"}
	}
	return BackfillTarget{
		Read:           db,
		Write:          db,
		Table:          "Message",
		IDColumn:       "id",
		ContentColumns: columns,
	}
}

func TestBackfillCensorsExistingRows(t *testing.T) {
	db := newBackfillDB(t)
	f := mustNew(t, Options{})

	ids := insertRows(t, db,
		"hey nigger, what's up",
		"perfectly ordinary message",
		"you kike!",
		"the niggardly landlord was niggling again",
		"n1gg3r",
	)

	stats, err := f.Backfill(context.Background(), messageTarget(db))
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if stats.Scanned != 5 {
		t.Errorf("Scanned = %d, want 5", stats.Scanned)
	}
	if stats.Changed != 3 {
		t.Errorf("Changed = %d, want 3", stats.Changed)
	}

	want := []string{
		"hey ******, what's up",
		"perfectly ordinary message",
		"you ****!",
		"the niggardly landlord was niggling again",
		"******",
	}
	for i, id := range ids {
		if got := readContent(t, db, id); got != want[i] {
			t.Errorf("row %d = %q, want %q", id, got, want[i])
		}
	}
}

// Running the pass again must be a no-op. The server does this on every startup.
func TestBackfillIsIdempotent(t *testing.T) {
	db := newBackfillDB(t)
	f := mustNew(t, Options{})
	insertRows(t, db, "hey nigger", "clean message", "you kike!")

	first, err := f.Backfill(context.Background(), messageTarget(db))
	if err != nil {
		t.Fatalf("first Backfill: %v", err)
	}
	if first.Changed != 2 {
		t.Fatalf("first pass Changed = %d, want 2", first.Changed)
	}

	second, err := f.Backfill(context.Background(), messageTarget(db))
	if err != nil {
		t.Fatalf("second Backfill: %v", err)
	}
	if second.Scanned != 3 {
		t.Errorf("second pass Scanned = %d, want 3", second.Scanned)
	}
	if second.Changed != 0 {
		t.Errorf("second pass Changed = %d, want 0; already-censored content was rewritten", second.Changed)
	}
}

// The pass pages by id, so it has to cross batch boundaries correctly. This is
// the bug that a small fixture would never catch: an off-by-one in the paging
// would silently skip or re-read rows.
func TestBackfillCrossesBatchBoundaries(t *testing.T) {
	db := newBackfillDB(t)
	f := mustNew(t, Options{})

	// Deliberately more than two full batches, with a known slur every third row
	// so matches land on both sides of every boundary.
	const total = backfillBatch*2 + 37
	contents := make([]string, 0, total)
	wantChanged := 0
	for i := 0; i < total; i++ {
		if i%3 == 0 {
			contents = append(contents, fmt.Sprintf("row %d says nigger", i))
			wantChanged++
		} else {
			contents = append(contents, fmt.Sprintf("row %d is fine", i))
		}
	}
	ids := insertRows(t, db, contents...)

	stats, err := f.Backfill(context.Background(), messageTarget(db))
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if stats.Scanned != total {
		t.Errorf("Scanned = %d, want %d; paging skipped or repeated rows", stats.Scanned, total)
	}
	if stats.Changed != wantChanged {
		t.Errorf("Changed = %d, want %d", stats.Changed, wantChanged)
	}

	// Verify every row individually rather than trusting the counts.
	for i, id := range ids {
		got := readContent(t, db, id)
		var want string
		if i%3 == 0 {
			want = fmt.Sprintf("row %d says ******", i)
		} else {
			want = fmt.Sprintf("row %d is fine", i)
		}
		if got != want {
			t.Fatalf("row %d (id %d) = %q, want %q", i, id, got, want)
		}
	}
}

// A batch with no matches at all must still advance the cursor, or the pass
// loops forever on the same page.
func TestBackfillTerminatesWithNoMatches(t *testing.T) {
	db := newBackfillDB(t)
	f := mustNew(t, Options{})

	contents := make([]string, backfillBatch*2)
	for i := range contents {
		contents[i] = fmt.Sprintf("clean row %d", i)
	}
	insertRows(t, db, contents...)

	stats, err := f.Backfill(context.Background(), messageTarget(db))
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if stats.Scanned != len(contents) {
		t.Errorf("Scanned = %d, want %d", stats.Scanned, len(contents))
	}
	if stats.Changed != 0 {
		t.Errorf("Changed = %d, want 0", stats.Changed)
	}
}

func TestBackfillEmptyTable(t *testing.T) {
	db := newBackfillDB(t)
	f := mustNew(t, Options{})

	stats, err := f.Backfill(context.Background(), messageTarget(db))
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if stats.Scanned != 0 || stats.Changed != 0 {
		t.Errorf("stats = %+v, want zero", stats)
	}
}

func TestBackfillNilFilterIsNoOp(t *testing.T) {
	db := newBackfillDB(t)
	ids := insertRows(t, db, "hey nigger")

	var f *Filter
	stats, err := f.Backfill(context.Background(), messageTarget(db))
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if stats.Scanned != 0 || stats.Changed != 0 {
		t.Errorf("stats = %+v, want zero for a nil filter", stats)
	}
	if got := readContent(t, db, ids[0]); got != "hey nigger" {
		t.Errorf("content = %q, want it untouched by a nil filter", got)
	}
}

// Nicknames live in the same row as the message body, so both are censored in a
// single pass. A match in either column has to rewrite the row, and the column
// that did not match must be written back unchanged rather than blanked.
func TestBackfillMultipleColumns(t *testing.T) {
	db := newBackfillDB(t)
	if _, err := db.Exec(`ALTER TABLE Message ADD COLUMN author_nickname TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatalf("add column: %v", err)
	}
	f := mustNew(t, Options{})

	type row struct{ nickname, content string }
	seed := []row{
		{"nigger", "a perfectly ordinary message"}, // nickname only
		{"alice", "hey nigger"},                    // content only
		{"kike", "you gook"},                       // both
		{"bob", "nothing to see"},                  // neither
	}
	for i, r := range seed {
		if _, err := db.Exec(
			`INSERT INTO Message (id, content, author_nickname) VALUES (?, ?, ?)`,
			i+1, r.content, r.nickname); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	stats, err := f.Backfill(context.Background(), messageTarget(db, "content", "author_nickname"))
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if stats.Scanned != 4 {
		t.Errorf("Scanned = %d, want 4", stats.Scanned)
	}
	if stats.Changed != 3 {
		t.Errorf("Changed = %d, want 3", stats.Changed)
	}

	want := []row{
		{"******", "a perfectly ordinary message"},
		{"alice", "hey ******"},
		{"****", "you ****"},
		{"bob", "nothing to see"},
	}
	for i, w := range want {
		var got row
		if err := db.QueryRow(
			`SELECT author_nickname, content FROM Message WHERE id = ?`, i+1,
		).Scan(&got.nickname, &got.content); err != nil {
			t.Fatalf("select %d: %v", i+1, err)
		}
		if got != w {
			t.Errorf("row %d = %+v, want %+v", i+1, got, w)
		}
	}
}

func TestBackfillRejectsBadIdentifiers(t *testing.T) {
	db := newBackfillDB(t)
	f := mustNew(t, Options{})

	bad := []BackfillTarget{
		{Read: db, Write: db, Table: "Message; DROP TABLE Message", IDColumn: "id", ContentColumns: []string{"content"}},
		{Read: db, Write: db, Table: "Message", IDColumn: "id = 1 OR 1", ContentColumns: []string{"content"}},
		{Read: db, Write: db, Table: "Message", IDColumn: "id", ContentColumns: []string{""}},
		{Read: db, Write: db, Table: "Message", IDColumn: "id", ContentColumns: []string{"content", "x; DROP TABLE Message"}},
		{Read: db, Write: db, Table: "Message", IDColumn: "id"}, // no columns at all
	}
	for _, target := range bad {
		if _, err := f.Backfill(context.Background(), target); err == nil {
			t.Errorf("Backfill accepted target %+v", target)
		}
	}
}

func TestBackfillHonoursCancelledContext(t *testing.T) {
	db := newBackfillDB(t)
	f := mustNew(t, Options{})
	insertRows(t, db, "hey nigger")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := f.Backfill(ctx, messageTarget(db)); err == nil {
		t.Error("Backfill succeeded with a cancelled context")
	}
}
