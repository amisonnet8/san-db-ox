package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func TestCmdTablesExcludesSqliteInternal(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE b (x TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE a (x TEXT)"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := cmdTables(db, &out); err != nil {
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

	var all bytes.Buffer
	if err := cmdSchema(db, nil, &all); err != nil {
		t.Fatalf("cmdSchema (all): %v", err)
	}
	if !strings.Contains(all.String(), "CREATE TABLE t") || !strings.Contains(all.String(), "CREATE TABLE other") {
		t.Fatalf("cmdSchema (all) missing expected statements: %q", all.String())
	}

	var filtered bytes.Buffer
	if err := cmdSchema(db, []string{"t"}, &filtered); err != nil {
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
	self := filepath.Join(selfDir, snapshotFilename("fake-self", runtime.GOOS))
	if err := os.WriteFile(self, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	cwd := t.TempDir()
	t.Chdir(cwd)
	db := newTestDB(t)

	var out bytes.Buffer
	if err := cmdSnapshot(db, self, nil, &out); err != nil {
		t.Fatalf("cmdSnapshot (default name): %v", err)
	}
	wantDefault := snapshotFilename(filepath.Base(self), runtime.GOOS)
	if _, err := os.Stat(filepath.Join(cwd, wantDefault)); err != nil {
		t.Fatalf("expected a snapshot at CWD/%s: %v", wantDefault, err)
	}

	out.Reset()
	if err := cmdSnapshot(db, self, []string{"mydb"}, &out); err != nil {
		t.Fatalf("cmdSnapshot (explicit name): %v", err)
	}
	wantExplicit := snapshotFilename("mydb", runtime.GOOS)
	if _, err := os.Stat(filepath.Join(cwd, wantExplicit)); err != nil {
		t.Fatalf("expected a snapshot at CWD/%s: %v", wantExplicit, err)
	}
	if !strings.Contains(out.String(), wantExplicit) {
		t.Fatalf("cmdSnapshot output = %q, want it to mention %q", out.String(), wantExplicit)
	}
}

func TestCmdExit(t *testing.T) {
	res, err := cmdExit(nil)
	if err != nil || !res.exit || res.code != 0 {
		t.Fatalf("cmdExit(nil) = %+v, %v; want exit=true code=0", res, err)
	}

	res, err = cmdExit([]string{"7"})
	if err != nil || !res.exit || res.code != 7 {
		t.Fatalf("cmdExit([7]) = %+v, %v; want exit=true code=7", res, err)
	}

	if _, err := cmdExit([]string{"not-a-number"}); err == nil {
		t.Fatalf("expected an error for a non-numeric exit code")
	}
}

func TestDispatchDotCommandUnknown(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	res := dispatchDotCommand(db, "self", ".nope", &out, &errw)
	if res.exit {
		t.Fatalf("unknown command should not request exit")
	}
	if !strings.Contains(errw.String(), ".nope") {
		t.Fatalf("expected the error to name the unknown command, got %q", errw.String())
	}
}

func TestDispatchDotCommandExit(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	res := dispatchDotCommand(db, "self", ".exit 3", &out, &errw)
	if !res.exit || res.code != 3 {
		t.Fatalf("dispatchDotCommand(.exit 3) = %+v, want exit=true code=3", res)
	}

	out.Reset()
	errw.Reset()
	res = dispatchDotCommand(db, "self", ".quit", &out, &errw)
	if !res.exit || res.code != 0 {
		t.Fatalf("dispatchDotCommand(.quit) = %+v, want exit=true code=0", res)
	}
}

// TestDispatchDotCommandOverwriteInGoTest documents that .overwrite
// cannot succeed from `go test` (its binary lives under a go-build* temp
// dir; see engine's TestOverwriteRejectsGoTestBinary) and confirms the
// dispatcher does NOT set exit=true when the underlying Overwrite call
// fails.
func TestDispatchDotCommandOverwriteInGoTest(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	res := dispatchDotCommand(db, "self", ".overwrite", &out, &errw)
	if res.exit {
		t.Fatalf("expected .overwrite to fail (and not request exit) under go test")
	}
	if !strings.Contains(errw.String(), "Error:") {
		t.Fatalf("expected an error message, got out=%q errw=%q", out.String(), errw.String())
	}
}

func TestCmdHelpListsOnlyPhase1Commands(t *testing.T) {
	var out bytes.Buffer
	cmdHelp(&out)
	for _, want := range []string{".tables", ".schema", ".snapshot", ".overwrite", ".exit", ".help"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("cmdHelp output missing %q", want)
		}
	}
	for _, notYet := range []string{".load", ".import", ".dump", ".mode", ".headers"} {
		if strings.Contains(out.String(), notYet) {
			t.Errorf("cmdHelp output should not advertise unimplemented %q yet", notYet)
		}
	}
}
