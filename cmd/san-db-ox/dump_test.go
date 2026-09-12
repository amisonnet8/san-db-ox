package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/amisonnet8/san-db-ox/engine"
)

func newDumpTestRepl(t *testing.T, db *engine.DB) (*repl, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errw bytes.Buffer
	return newTestRepl(t, db, "self", &out, &errw), &out, &errw
}

// TestCmdDumpRoundTrip confirms piping .dump's own output back into a
// fresh database reproduces the same rows -- the primary contract of
// ".dump" (spec §3).
func TestCmdDumpRoundTrip(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`CREATE TABLE t(id INTEGER PRIMARY KEY, name TEXT, note TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t(name, note) VALUES ('alice', 'it''s a test'), ('bob', NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE INDEX idx_t_name ON t(name)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE VIEW v AS SELECT name FROM t`); err != nil {
		t.Fatal(err)
	}

	r, out, errw := newDumpTestRepl(t, db)
	if err := r.cmdDump(nil); err != nil {
		t.Fatalf("cmdDump: %v", err)
	}
	if errw.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errw.String())
	}

	fresh := newTestDB(t)
	for _, stmt := range strings.Split(out.String(), "\n") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "BEGIN") || strings.HasPrefix(stmt, "COMMIT") {
			continue
		}
		if _, err := fresh.Exec(stmt); err != nil {
			t.Fatalf("replaying dumped statement %q: %v", stmt, err)
		}
	}

	var cnt int
	if err := fresh.QueryRow("SELECT count(*) FROM t").Scan(&cnt); err != nil {
		t.Fatalf("t missing after replay: %v", err)
	}
	if cnt != 2 {
		t.Fatalf("row count = %d, want 2", cnt)
	}
	var note any
	if err := fresh.QueryRow("SELECT note FROM t WHERE name = 'alice'").Scan(&note); err != nil {
		t.Fatal(err)
	}
	if note != "it's a test" {
		t.Fatalf("note = %v, want %q (single-quote escaping via quote())", note, "it's a test")
	}
	if _, err := fresh.Query("SELECT * FROM v"); err != nil {
		t.Fatalf("view v missing after replay: %v", err)
	}
}

// TestCmdDumpPatternFiltersTables confirms PATTERN (a SQL LIKE pattern)
// restricts which tables (and their own indexes/views/triggers) are
// dumped (spec §3).
func TestCmdDumpPatternFiltersTables(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`CREATE TABLE users(id)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE orders(id)`); err != nil {
		t.Fatal(err)
	}

	r, out, _ := newDumpTestRepl(t, db)
	if err := r.cmdDump([]string{"user%"}); err != nil {
		t.Fatalf("cmdDump: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `CREATE TABLE users`) {
		t.Fatalf("expected users table in output, got %q", got)
	}
	if strings.Contains(got, `CREATE TABLE orders`) {
		t.Fatalf("orders table should have been filtered out, got %q", got)
	}
}

// TestCmdDumpBlobUsesHexLiteral confirms quote()'s BLOB rendering
// (X'..' hex) round-trips correctly, unlike a hand-rolled string escaper
// would for binary data.
func TestCmdDumpBlobUsesHexLiteral(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`CREATE TABLE t(b BLOB)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t VALUES (X'89504e47')`); err != nil {
		t.Fatal(err)
	}

	r, out, _ := newDumpTestRepl(t, db)
	if err := r.cmdDump(nil); err != nil {
		t.Fatalf("cmdDump: %v", err)
	}
	if !strings.Contains(out.String(), "X'89504E47'") && !strings.Contains(out.String(), "X'89504e47'") {
		t.Fatalf("expected a hex BLOB literal in output, got %q", out.String())
	}
}

// TestCmdDumpEmptyDatabase confirms dumping an empty database still
// produces a valid (replayable) BEGIN/COMMIT-wrapped, empty script.
func TestCmdDumpEmptyDatabase(t *testing.T) {
	db := newTestDB(t)
	r, out, errw := newDumpTestRepl(t, db)
	if err := r.cmdDump(nil); err != nil {
		t.Fatalf("cmdDump: %v", err)
	}
	if errw.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errw.String())
	}
	if !strings.Contains(out.String(), "BEGIN TRANSACTION;") || !strings.Contains(out.String(), "COMMIT;") {
		t.Fatalf("expected a BEGIN/COMMIT-wrapped script, got %q", out.String())
	}
}

func TestCmdDumpQuoteIdent(t *testing.T) {
	if got, want := quoteIdent("plain"), `"plain"`; got != want {
		t.Errorf("quoteIdent(%q) = %q, want %q", "plain", got, want)
	}
	if got, want := quoteIdent(`has"quote`), `"has""quote"`; got != want {
		t.Errorf("quoteIdent(%q) = %q, want %q", `has"quote`, got, want)
	}
}
