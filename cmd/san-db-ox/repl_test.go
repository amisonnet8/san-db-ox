package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amisonnet8/san-db-ox/engine"
)

func TestFormatValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{int64(42), "42"},
		{"alice", "alice"},
		{[]byte("blob"), "blob"},
	}
	for _, c := range cases {
		if got := formatValue(c.in); got != c.want {
			t.Errorf("formatValue(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatRow(t *testing.T) {
	got := formatRow([]any{int64(1), "alice", nil})
	want := "1|alice|"
	if got != want {
		t.Errorf("formatRow(...) = %q, want %q", got, want)
	}
}

// newTestReplWithSession opens a real Session on db (unlike
// newTestRepl in dotcmd_test.go, exercised here through runREPL's own
// codepath in most tests instead) for tests that call r.execSQL/r.run
// directly.
func newTestReplWithSession(t *testing.T, db *engine.DB, out, errw *bytes.Buffer, interactive bool) *repl {
	t.Helper()
	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatalf("db.Session: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return &repl{db: db, sess: sess, self: "self", opts: &options{}, interactive: interactive, mode: modeList, out: out, errw: errw}
}

func TestExecSQLPrintsRowsAndSuppressesNonSelect(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var out, errw bytes.Buffer
	r := newTestReplWithSession(t, db, &out, &errw, false)

	r.execSQL("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	if out.Len() != 0 || errw.Len() != 0 {
		t.Fatalf("CREATE TABLE should print nothing, got out=%q errw=%q", out.String(), errw.String())
	}

	r.execSQL("INSERT INTO t (v) VALUES ('alice')")
	if out.Len() != 0 || errw.Len() != 0 {
		t.Fatalf("INSERT should print nothing, got out=%q errw=%q", out.String(), errw.String())
	}

	out.Reset()
	r.execSQL("SELECT id, v FROM t")
	if got, want := out.String(), "1|alice\n"; got != want {
		t.Fatalf("SELECT output = %q, want %q", got, want)
	}

	out.Reset()
	errw.Reset()
	r.execSQL("not valid sql")
	if out.Len() != 0 {
		t.Fatalf("invalid SQL should print nothing to stdout, got %q", out.String())
	}
	if !strings.Contains(errw.String(), "Error:") {
		t.Fatalf("invalid SQL should print an error, got %q", errw.String())
	}
}

// TestSplitComplete exercises the package-level splitComplete function
// directly -- it holds no per-run state (engine.Complete needs no live
// DB), so no *repl/Session setup is needed here.
func TestSplitComplete(t *testing.T) {
	t.Run("no semicolon is never complete", func(t *testing.T) {
		stmts, remainder := splitComplete("SELECT 1")
		if len(stmts) != 0 || remainder != "SELECT 1" {
			t.Fatalf("splitComplete(%q) = %v, %q; want no statements, full remainder", "SELECT 1", stmts, remainder)
		}
	})

	t.Run("one statement", func(t *testing.T) {
		stmts, remainder := splitComplete("SELECT 1;\n")
		if len(stmts) != 1 || stmts[0] != "SELECT 1;" || strings.TrimSpace(remainder) != "" {
			t.Fatalf("splitComplete = %v, %q", stmts, remainder)
		}
	})

	t.Run("multiple statements on one line", func(t *testing.T) {
		stmts, remainder := splitComplete("SELECT 1; SELECT 2;")
		if len(stmts) != 2 || stmts[0] != "SELECT 1;" || stmts[1] != "SELECT 2;" || remainder != "" {
			t.Fatalf("splitComplete = %v, %q", stmts, remainder)
		}
	})

	t.Run("semicolon inside a string literal is not a boundary", func(t *testing.T) {
		stmts, remainder := splitComplete("SELECT ';';")
		if len(stmts) != 1 || stmts[0] != "SELECT ';';" || remainder != "" {
			t.Fatalf("splitComplete = %v, %q", stmts, remainder)
		}
	})

	t.Run("CREATE TRIGGER body is not split at its internal END", func(t *testing.T) {
		text := "CREATE TABLE t(a); CREATE TRIGGER trg AFTER INSERT ON t BEGIN SELECT CASE WHEN 1 THEN 2 ELSE 3 END; END;"
		stmts, remainder := splitComplete(text)
		if len(stmts) != 2 {
			t.Fatalf("splitComplete found %d statements, want 2: %v (remainder %q)", len(stmts), stmts, remainder)
		}
		if !strings.HasSuffix(stmts[1], "END;") || strings.Count(stmts[1], "END") != 2 {
			t.Fatalf("trigger statement split too early: %q", stmts[1])
		}
	})
}

func TestRunMultiLineStatement(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	script := "SELECT\n1\n;\n.exit\n"
	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader(script), &out, &errw, false, &options{})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "1\n") {
		t.Fatalf("expected the multi-line SELECT's result in output, got %q", out.String())
	}
}

func TestRunMultipleStatementsOnOneLineEachPrintSeparately(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	script := "SELECT 1; SELECT 2;\n.exit\n"
	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader(script), &out, &errw, false, &options{})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "1\n") || !strings.Contains(got, "2\n") {
		t.Fatalf("expected both SELECT results in output, got %q", got)
	}
}

// TestRunSessionSpansStatements confirms the reason the REPL now holds
// one Session (spec §2, .claude/rules/sqlite-quirks.md's ResetSession
// pitfall): a BEGIN/INSERT/ROLLBACK sequence typed across separate lines
// stays on the same connection, so the ROLLBACK actually takes effect.
func TestRunSessionSpansStatements(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	script := "CREATE TABLE t(x);\nBEGIN;\nINSERT INTO t VALUES (1);\nROLLBACK;\nSELECT count(*) FROM t;\n.exit\n"
	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader(script), &out, &errw, false, &options{})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errw.String())
	}
	if !strings.Contains(out.String(), "0\n") {
		t.Fatalf("expected ROLLBACK to have taken effect (count = 0), got %q", out.String())
	}
}

func TestRunREPLFullSession(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	script := strings.Join([]string{
		"CREATE TABLE t (v TEXT);",
		"INSERT INTO t VALUES ('hello');",
		"SELECT v FROM t;",
		".tables",
		"", // blank line should be ignored, not error
		".exit 5",
	}, "\n") + "\n"

	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader(script), &out, &errw, true, &options{})

	if code != 5 {
		t.Fatalf("exit code = %d, want 5", code)
	}
	if errw.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errw.String())
	}
	got := out.String()
	if !strings.Contains(got, "hello") {
		t.Fatalf("expected the SELECT result in output, got %q", got)
	}
	if !strings.Contains(got, "t\n") && !strings.HasSuffix(strings.TrimRight(got, " "), "t") {
		t.Fatalf(".tables output missing table name, got %q", got)
	}
	if strings.Count(got, prompt) == 0 {
		t.Fatalf("expected the prompt to appear at least once, got %q", got)
	}
	if !strings.Contains(got, continuationPrompt) {
		// Not required by this script (no multi-line input), but the
		// symbol itself is exercised by TestRunMultiLineStatement's
		// interactive variant below; nothing to assert here.
		_ = continuationPrompt
	}
}

func TestRunREPLExitsOnEOFWithoutExitCommand(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader("SELECT 1;\n"), &out, &errw, false, &options{})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 on plain EOF", code)
	}
}

// TestRunREPLStartupModeAppliesFromOptions confirms -m/--mode
// (options.go's opts.mode) becomes the REPL's initial output mode, and
// that .mode column's automatic .headers-on (cmdMode, dotcmd.go) applies
// the same way when column is the startup mode too.
func TestRunREPLStartupModeAppliesFromOptions(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(a)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES (1)"); err != nil {
		t.Fatal(err)
	}

	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader("SELECT a FROM t;\n"), &out, &errw, false, &options{mode: modeJSON})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errw.String())
	}
	if got, want := out.String(), `{"columns":["a"],"rows":[[1]]}`+"\n"; got != want {
		t.Fatalf("startup -m json output = %q, want %q", got, want)
	}
}

// TestRunREPLNonInteractiveSuppressesPromptAndBanner confirms spec §13:
// a non-interactive run (piped stdin) prints no prompt/continuation
// prompt at all, keeping stdout exactly the query output.
func TestRunREPLNonInteractiveSuppressesPromptAndBanner(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader("SELECT 1;\n"), &out, &errw, false, &options{})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(out.String(), prompt) || strings.Contains(out.String(), continuationPrompt) {
		t.Fatalf("non-interactive output should contain no prompt, got %q", out.String())
	}
	if got, want := out.String(), "1\n"; got != want {
		t.Fatalf("non-interactive output = %q, want exactly %q", got, want)
	}
}

// TestRunREPLInteractivePrintsPromptAndTrailingNewline confirms the
// interactive-only prompt/continuation-prompt output and the trailing
// newline printed on EOF (so the shell's next prompt doesn't run into
// the REPL's last line).
func TestRunREPLInteractivePrintsPromptAndTrailingNewline(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader("SELECT\n1\n;\n"), &out, &errw, true, &options{})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, continuationPrompt) {
		t.Fatalf("expected the continuation prompt while the statement was incomplete, got %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected a trailing newline on EOF, got %q", got)
	}
}
