package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/amisonnet8/san-db-ox/engine"
)

func newTestDB(t *testing.T) *engine.DB {
	t.Helper()
	db, err := engine.Open(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("engine.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newTestRepl builds a repl backed by a fresh Session on db, writing to
// out/errw, for testing dot-command handlers directly.
func newTestRepl(t *testing.T, db *engine.DB, self string, out, errw *bytes.Buffer) *repl {
	t.Helper()
	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatalf("db.Session: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return &repl{db: db, sess: sess, self: self, opts: &options{}, mode: modeList, out: out, errw: errw}
}

func TestCmdTablesExcludesSqliteInternal(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE b (x TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE a (x TEXT)"); err != nil {
		t.Fatal(err)
	}

	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)
	if err := r.cmdTables(); err != nil {
		t.Fatalf("cmdTables: %v", err)
	}
	got := strings.Fields(out.String())
	want := []string{"a", "b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("cmdTables output = %q, want tables %v in order", out.String(), want)
	}
	if strings.Contains(out.String(), "sqlite_") {
		t.Fatalf("cmdTables leaked an internal sqlite_ table: %q", out.String())
	}
}

func TestCmdSchemaAllAndFiltered(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE t (v TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE INDEX idx_t_v ON t (v)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE other (v TEXT)"); err != nil {
		t.Fatal(err)
	}

	var all, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &all, &errw)
	if err := r.cmdSchema(nil); err != nil {
		t.Fatalf("cmdSchema (all): %v", err)
	}
	if !strings.Contains(all.String(), "CREATE TABLE t") || !strings.Contains(all.String(), "CREATE TABLE other") {
		t.Fatalf("cmdSchema (all) missing expected statements: %q", all.String())
	}

	var filtered bytes.Buffer
	r2 := newTestRepl(t, db, "self", &filtered, &errw)
	if err := r2.cmdSchema([]string{"t"}); err != nil {
		t.Fatalf("cmdSchema (t): %v", err)
	}
	out := filtered.String()
	if !strings.Contains(out, "CREATE TABLE t") {
		t.Fatalf("cmdSchema (t) missing CREATE TABLE t: %q", out)
	}
	if !strings.Contains(out, "CREATE INDEX idx_t_v") {
		t.Fatalf("cmdSchema (t) missing its index (tbl_name match): %q", out)
	}
	if strings.Contains(out, "CREATE TABLE other") {
		t.Fatalf("cmdSchema (t) leaked an unrelated table: %q", out)
	}
}

// TestCmdSnapshotDefaultAndExplicitName also pins down that a bare
// ".snapshot" (no FILENAME) writes to the CURRENT DIRECTORY using the
// running binary's own basename, not next to the binary itself
// (.claude/rules/testing.md's e2e pitfalls note this explicitly). self
// deliberately lives in a directory the test never os.Chdir()s into, so
// the assertions would fail if cmdSnapshot resolved the default name
// against filepath.Dir(self) instead of the process's actual CWD.
//
// self and the explicit name are both run through snapshotFilename to
// compute what cmdSnapshot should actually produce, rather than assuming
// the extension-less literal: on Windows, a real os.Executable() always
// ends in ".exe", and snapshotFilename appends ".exe" to any
// extension-less base -- a bare "fake-self"/"mydb" would silently become
// "fake-self.exe"/"mydb.exe" there, which is exactly the mismatch that
// broke this test on windows-latest CI before this fix.
func TestCmdSnapshotDefaultAndExplicitName(t *testing.T) {
	selfDir := t.TempDir()
	self := filepath.Join(selfDir, snapshotFilename("fake-self", false, false, time.Now(), runtime.GOOS))
	if err := os.WriteFile(self, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	cwd := t.TempDir()
	t.Chdir(cwd)
	db := newTestDB(t)

	var out, errw bytes.Buffer
	r := newTestRepl(t, db, self, &out, &errw)
	if err := r.cmdSnapshot(nil); err != nil {
		t.Fatalf("cmdSnapshot (default name): %v", err)
	}
	wantDefault := snapshotFilename(filepath.Base(self), false, false, time.Now(), runtime.GOOS)
	if _, err := os.Stat(filepath.Join(cwd, wantDefault)); err != nil {
		t.Fatalf("expected a snapshot at CWD/%s: %v", wantDefault, err)
	}

	out.Reset()
	if err := r.cmdSnapshot([]string{"mydb"}); err != nil {
		t.Fatalf("cmdSnapshot (explicit name): %v", err)
	}
	wantExplicit := snapshotFilename("mydb", false, false, time.Now(), runtime.GOOS)
	if _, err := os.Stat(filepath.Join(cwd, wantExplicit)); err != nil {
		t.Fatalf("expected a snapshot at CWD/%s: %v", wantExplicit, err)
	}
	if !strings.Contains(out.String(), wantExplicit) {
		t.Fatalf("cmdSnapshot output = %q, want it to mention %q", out.String(), wantExplicit)
	}
}

// TestCmdSnapshotSQLiteFlag confirms "--sqlite" routes through
// db.Export (a plain SQLite file, .sqlite extension completion) instead
// of db.Snapshot (spec §4, §6).
func TestCmdSnapshotSQLiteFlag(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	t.Chdir(dir)

	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)
	if err := r.cmdSnapshot([]string{"mydb", "--sqlite"}); err != nil {
		t.Fatalf("cmdSnapshot --sqlite: %v", err)
	}
	want := "mydb.sqlite"
	if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
		t.Fatalf("expected a SQLite file at %s: %v", want, err)
	}
	if !strings.Contains(out.String(), want) {
		t.Fatalf("cmdSnapshot output = %q, want it to mention %q", out.String(), want)
	}
}

// TestCmdSnapshotTimestampFlag confirms a per-call "--timestamp"
// produces a "_YYYYMMDDHHMMSS"-suffixed filename (naming.md), overriding
// the (unset, here) startup -t default.
func TestCmdSnapshotTimestampFlag(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	t.Chdir(dir)

	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)
	if err := r.cmdSnapshot([]string{"mydb", "--timestamp"}); err != nil {
		t.Fatalf("cmdSnapshot --timestamp: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one file in %s, got %v", dir, entries)
	}
	if !timestampSuffixPattern.MatchString(strings.TrimSuffix(entries[0].Name(), filepath.Ext(entries[0].Name()))) {
		t.Fatalf("filename %q does not carry a _YYYYMMDDHHMMSS suffix", entries[0].Name())
	}
}

// TestCmdSnapshotFlagsAndFilenameInAnyOrder confirms both spec §12
// examples work: ".snapshot NAME --timestamp" and
// ".snapshot NAME --sqlite --timestamp" -- the flags and the filename
// may appear in any order.
func TestCmdSnapshotFlagsAndFilenameInAnyOrder(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	t.Chdir(dir)

	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)
	if err := r.cmdSnapshot([]string{"--sqlite", "--timestamp", "bug_123"}); err != nil {
		t.Fatalf("cmdSnapshot: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "bug_123_") || !strings.HasSuffix(entries[0].Name(), ".sqlite") {
		t.Fatalf("expected one bug_123_<timestamp>.sqlite file, got %v", entries)
	}
}

// TestCmdLoadReplacesState confirms ".load" fully replaces (not merges)
// the live DB's state (spec §4).
// TestCmdLoadFromSQLiteFileDoesNotWarnAboutVersion is a regression test:
// FileInfo.Version is always zero for KindSQLite (inspect.go), so
// comparing it against engine.FormatVersion without also checking Kind
// would spuriously warn on every plain SQLite file .load'd -- caught by
// tests/e2e.sh's .snapshot --sqlite / .load round trip during Step 7.
func TestCmdLoadFromSQLiteFileDoesNotWarnAboutVersion(t *testing.T) {
	src := newTestDB(t)
	if _, err := src.Exec("CREATE TABLE t(x); INSERT INTO t VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "src.sqlite")
	if err := src.Export(path); err != nil {
		t.Fatal(err)
	}

	dst := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, dst, "self", &out, &errw)
	if err := r.cmdLoad([]string{path}); err != nil {
		t.Fatalf("cmdLoad: %v", err)
	}
	if errw.Len() != 0 {
		t.Fatalf("loading a plain SQLite file should never warn about format version, got stderr %q", errw.String())
	}
}

func TestCmdLoadReplacesState(t *testing.T) {
	src := newTestDB(t)
	if _, err := src.Exec("CREATE TABLE t(x); INSERT INTO t VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "src")
	if err := src.Snapshot(path); err != nil {
		t.Fatal(err)
	}

	dst := newTestDB(t)
	if _, err := dst.Exec("CREATE TABLE other(y)"); err != nil {
		t.Fatal(err)
	}

	var out, errw bytes.Buffer
	r := newTestRepl(t, dst, "self", &out, &errw)
	if err := r.cmdLoad([]string{path}); err != nil {
		t.Fatalf("cmdLoad: %v", err)
	}
	if errw.Len() != 0 {
		t.Fatalf("unexpected warning for a matching format version: %q", errw.String())
	}
	if !strings.Contains(out.String(), path) {
		t.Fatalf("cmdLoad output = %q, want it to mention %q", out.String(), path)
	}

	var cnt int
	if err := dst.QueryRow("SELECT count(*) FROM t").Scan(&cnt); err != nil {
		t.Fatalf("t should exist after Load: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("row count = %d, want 1", cnt)
	}
	if _, err := dst.Query("SELECT * FROM other"); err == nil {
		t.Fatal("expected table 'other' to be gone after Load (full replace, not merge)")
	}
}

// TestCmdLoadWarnsOnVersionMismatch confirms the spec §4 footer
// Version-mismatch warning: a real snapshot's footer Version field is
// patched to a different value (the format footer.go documents: trailing
// 32 bytes, Magic(8)+Version(4 big-endian)+..., so the Version field
// starts at size-24) rather than requiring an actual different build.
func TestCmdLoadWarnsOnVersionMismatch(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "old")
	if err := db.Snapshot(path); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], engine.FormatVersion+99)
	if _, err := f.WriteAt(buf[:], info.Size()-24); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)
	if err := r.cmdLoad([]string{path}); err != nil {
		t.Fatalf("cmdLoad: %v", err)
	}
	if !strings.Contains(errw.String(), "Warning:") || !strings.Contains(errw.String(), "format version") {
		t.Fatalf("expected a version-mismatch warning, got %q", errw.String())
	}
}

func TestCmdExit(t *testing.T) {
	exit, code, err := cmdExit(nil)
	if err != nil || !exit || code != 0 {
		t.Fatalf("cmdExit(nil) = %v, %v, %v; want exit=true code=0", exit, code, err)
	}

	exit, code, err = cmdExit([]string{"7"})
	if err != nil || !exit || code != 7 {
		t.Fatalf("cmdExit([7]) = %v, %v, %v; want exit=true code=7", exit, code, err)
	}

	if _, _, err := cmdExit([]string{"not-a-number"}); err == nil {
		t.Fatalf("expected an error for a non-numeric exit code")
	}
}

func TestHandleDotCommandUnknown(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)
	exit, _ := r.handleDotCommand(".nope")
	if exit {
		t.Fatalf("unknown command should not request exit")
	}
	if !strings.Contains(errw.String(), ".nope") {
		t.Fatalf("expected the error to name the unknown command, got %q", errw.String())
	}
}

func TestHandleDotCommandExit(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)
	exit, code := r.handleDotCommand(".exit 3")
	if !exit || code != 3 {
		t.Fatalf("handleDotCommand(.exit 3) = %v, %v, want exit=true code=3", exit, code)
	}

	out.Reset()
	errw.Reset()
	exit, code = r.handleDotCommand(".quit")
	if !exit || code != 0 {
		t.Fatalf("handleDotCommand(.quit) = %v, %v, want exit=true code=0", exit, code)
	}
}

// TestHandleDotCommandOverwriteInGoTest documents that .overwrite cannot
// succeed from `go test` (its binary lives under a go-build* temp dir;
// see engine's TestOverwriteRejectsGoTestBinary) and confirms the
// dispatcher does NOT report exit=true when the underlying Overwrite
// call fails.
func TestHandleDotCommandOverwriteInGoTest(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)
	exit, _ := r.handleDotCommand(".overwrite")
	if exit {
		t.Fatalf("expected .overwrite to fail (and not request exit) under go test")
	}
	if !strings.Contains(errw.String(), "Error:") {
		t.Fatalf("expected an error message, got out=%q errw=%q", out.String(), errw.String())
	}
}

// TestCmdHelpListsAllImplementedCommands confirms every dot command this
// build supports is advertised in ".help" (spec §3's full list is now
// implemented as of Phase 3 Step 5).
func TestCmdHelpListsAllImplementedCommands(t *testing.T) {
	var out bytes.Buffer
	cmdHelp(&out)
	for _, want := range []string{".tables", ".schema", ".mode", ".headers", ".snapshot", ".overwrite", ".load", ".dump", ".import", ".exit", ".help"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("cmdHelp output missing %q", want)
		}
	}
}
