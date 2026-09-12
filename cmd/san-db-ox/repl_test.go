package main

import (
	"bytes"
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

func TestRunSQLPrintsRowsAndSuppressesNonSelect(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var out, errw bytes.Buffer
	runSQL(db, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)", &out, &errw)
	if out.Len() != 0 || errw.Len() != 0 {
		t.Fatalf("CREATE TABLE should print nothing, got out=%q errw=%q", out.String(), errw.String())
	}

	runSQL(db, "INSERT INTO t (v) VALUES ('alice')", &out, &errw)
	if out.Len() != 0 || errw.Len() != 0 {
		t.Fatalf("INSERT should print nothing, got out=%q errw=%q", out.String(), errw.String())
	}

	out.Reset()
	runSQL(db, "SELECT id, v FROM t", &out, &errw)
	if got, want := out.String(), "1|alice\n"; got != want {
		t.Fatalf("SELECT output = %q, want %q", got, want)
	}

	out.Reset()
	errw.Reset()
	runSQL(db, "not valid sql", &out, &errw)
	if out.Len() != 0 {
		t.Fatalf("invalid SQL should print nothing to stdout, got %q", out.String())
	}
	if !strings.Contains(errw.String(), "Error:") {
		t.Fatalf("invalid SQL should print an error, got %q", errw.String())
	}
}

func TestRunREPLFullSession(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	script := strings.Join([]string{
		"CREATE TABLE t (v TEXT)",
		"INSERT INTO t VALUES ('hello')",
		"SELECT v FROM t",
		".tables",
		"", // blank line should be ignored, not error
		".exit 5",
	}, "\n") + "\n"

	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader(script), &out, &errw)

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
}

func TestRunREPLExitsOnEOFWithoutExitCommand(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var out, errw bytes.Buffer
	code := runREPL(db, "self", strings.NewReader("SELECT 1\n"), &out, &errw)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 on plain EOF", code)
	}
}
