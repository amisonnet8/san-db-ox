package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amisonnet8/san-db-ox/engine"
)

// runQuery is a small helper: run stmt (a SELECT) on db and render it
// through r.execSQL, returning stdout/stderr.
func runQuery(t *testing.T, r *repl, db *engine.DB, stmt string) (out, errw string) {
	t.Helper()
	ob, eb := r.out.(*bytes.Buffer), r.errw.(*bytes.Buffer)
	ob.Reset()
	eb.Reset()
	r.execSQL(stmt)
	return ob.String(), eb.String()
}

func newFormatTestRepl(t *testing.T) (*repl, *engine.DB) {
	t.Helper()
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := newTestReplWithSession(t, db, &bytes.Buffer{}, &bytes.Buffer{}, false)
	return r, db
}

func TestPrintRowsListMode(t *testing.T) {
	r, db := newFormatTestRepl(t)
	if _, err := db.Exec("CREATE TABLE t(a INTEGER, b TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES (1, 'alice'), (2, NULL)"); err != nil {
		t.Fatal(err)
	}

	out, errw := runQuery(t, r, db, "SELECT a, b FROM t ORDER BY a")
	if errw != "" {
		t.Fatalf("unexpected stderr: %q", errw)
	}
	want := "1|alice\n2|\n"
	if out != want {
		t.Fatalf("list mode output = %q, want %q", out, want)
	}

	r.headers = true
	out, errw = runQuery(t, r, db, "SELECT a, b FROM t ORDER BY a")
	if errw != "" {
		t.Fatalf("unexpected stderr: %q", errw)
	}
	want = "a|b\n1|alice\n2|\n"
	if out != want {
		t.Fatalf("list mode with headers = %q, want %q", out, want)
	}
}

func TestPrintRowsColumnMode(t *testing.T) {
	r, db := newFormatTestRepl(t)
	if _, err := db.Exec("CREATE TABLE t(id INTEGER, name TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES (1, 'alice'), (2, 'bob')"); err != nil {
		t.Fatal(err)
	}

	r.mode = modeColumn
	r.headers = true
	out, errw := runQuery(t, r, db, "SELECT id, name FROM t ORDER BY id")
	if errw != "" {
		t.Fatalf("unexpected stderr: %q", errw)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("column mode with headers should have 4 lines (header, underline, 2 rows), got %d: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "id") || !strings.Contains(lines[0], "name") {
		t.Fatalf("column mode header line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "--") {
		t.Fatalf("column mode underline = %q", lines[1])
	}

	r.headers = false
	out, errw = runQuery(t, r, db, "SELECT id, name FROM t ORDER BY id")
	if errw != "" {
		t.Fatalf("unexpected stderr: %q", errw)
	}
	lines = strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("column mode without headers should have 2 lines, got %d: %q", len(lines), out)
	}
}

func TestModeColumnAutoEnablesHeaders(t *testing.T) {
	r, _ := newFormatTestRepl(t)
	if r.headers {
		t.Fatal("headers should start off")
	}
	if err := r.cmdMode([]string{"column"}); err != nil {
		t.Fatal(err)
	}
	if !r.headers {
		t.Fatal(".mode column should turn .headers on automatically")
	}
	if err := r.cmdHeaders([]string{"off"}); err != nil {
		t.Fatal(err)
	}
	if r.headers {
		t.Fatal(".headers off should still work after the automatic on")
	}
}

func TestPrintRowsCSVMode(t *testing.T) {
	r, db := newFormatTestRepl(t)
	if _, err := db.Exec("CREATE TABLE t(a TEXT, b TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES ('has,comma', 'plain')"); err != nil {
		t.Fatal(err)
	}

	r.mode = modeCSV
	r.headers = true
	out, errw := runQuery(t, r, db, "SELECT a, b FROM t")
	if errw != "" {
		t.Fatalf("unexpected stderr: %q", errw)
	}
	want := "a,b\r\n\"has,comma\",plain\r\n"
	if out != want {
		t.Fatalf("csv mode output = %q, want %q", out, want)
	}
}

func TestPrintRowsLineMode(t *testing.T) {
	r, db := newFormatTestRepl(t)
	if _, err := db.Exec("CREATE TABLE t(id INTEGER, name TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES (1, 'alice'), (2, 'bob')"); err != nil {
		t.Fatal(err)
	}

	r.mode = modeLine
	out, errw := runQuery(t, r, db, "SELECT id, name FROM t ORDER BY id")
	if errw != "" {
		t.Fatalf("unexpected stderr: %q", errw)
	}
	want := "  id = 1\nname = alice\n\n  id = 2\nname = bob\n"
	if out != want {
		t.Fatalf("line mode output = %q, want %q", out, want)
	}
}

func TestPrintRowsJSONMode(t *testing.T) {
	r, db := newFormatTestRepl(t)
	if _, err := db.Exec("CREATE TABLE t(i INTEGER, r REAL, s TEXT, b BLOB, n TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES (42, 88.0, 'alice', X'89504e47', NULL)"); err != nil {
		t.Fatal(err)
	}

	r.mode = modeJSON
	out, errw := runQuery(t, r, db, "SELECT i, r, s, b, n FROM t")
	if errw != "" {
		t.Fatalf("unexpected stderr: %q", errw)
	}
	want := `{"columns":["i","r","s","b","n"],"rows":[[42,88.0,"alice",["iVBORw=="],null]]}` + "\n"
	if out != want {
		t.Fatalf("json mode output = %q, want %q", out, want)
	}
}

func TestPrintRowsJSONModeEmptyResultSet(t *testing.T) {
	r, db := newFormatTestRepl(t)
	if _, err := db.Exec("CREATE TABLE t(a)"); err != nil {
		t.Fatal(err)
	}

	r.mode = modeJSON
	out, errw := runQuery(t, r, db, "SELECT a FROM t")
	if errw != "" {
		t.Fatalf("unexpected stderr: %q", errw)
	}
	want := `{"columns":["a"],"rows":[]}` + "\n"
	if out != want {
		t.Fatalf("json mode (empty) output = %q, want %q", out, want)
	}
}

func TestPrintRowsJSONModeHeadersHasNoEffect(t *testing.T) {
	r, db := newFormatTestRepl(t)
	if _, err := db.Exec("CREATE TABLE t(a)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES (1)"); err != nil {
		t.Fatal(err)
	}

	r.mode = modeJSON
	r.headers = false
	outOff, _ := runQuery(t, r, db, "SELECT a FROM t")
	r.headers = true
	outOn, _ := runQuery(t, r, db, "SELECT a FROM t")
	if outOff != outOn {
		t.Fatalf(".headers should not affect json mode: off=%q on=%q", outOff, outOn)
	}
}
