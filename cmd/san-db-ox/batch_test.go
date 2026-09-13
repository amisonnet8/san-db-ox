package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunBatchMultipleChunksInOrder(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)

	code := r.runBatch([]string{"CREATE TABLE t(x)", "INSERT INTO t VALUES(1)", "SELECT * FROM t"})
	if code != 0 {
		t.Fatalf("runBatch code = %d, want 0 (errw: %q)", code, errw.String())
	}
	if strings.TrimSpace(out.String()) != "1" {
		t.Fatalf("runBatch output = %q, want \"1\"", out.String())
	}
}

// TestRunBatchAbortsOnError guards spec §5's "エラー発生時は即座に処理
// 全体を中断する": a failing chunk must stop the whole run, exit code 1,
// and no later chunk may run.
func TestRunBatchAbortsOnError(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)

	code := r.runBatch([]string{"CREATE TABLE t(x)", "SELECT * FROM nope", "INSERT INTO t VALUES(999)"})
	if code != 1 {
		t.Fatalf("runBatch code = %d, want 1", code)
	}
	if errw.Len() == 0 {
		t.Fatal("expected an error message on errw")
	}

	var count int
	if err := db.QueryRow("SELECT count(*) FROM t").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("the chunk after the failing one should not have run, but t has %d row(s)", count)
	}
}

// TestRunBatchExitCommandStopsImmediately guards ".exit CODE" behaving
// the same way mid-batch as it does anywhere else a dot command runs
// (spec §3, §5): the given code is returned, and no later chunk runs.
func TestRunBatchExitCommandStopsImmediately(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)

	code := r.runBatch([]string{"SELECT 1", ".exit 5", "SELECT 2"})
	if code != 5 {
		t.Fatalf("runBatch code = %d, want 5", code)
	}
	if strings.Contains(out.String(), "2") {
		t.Fatalf("chunk after .exit should not have run, got output %q", out.String())
	}
}

// TestRunBatchChunkImplicitTrailingSemicolon guards spec §5's "each -c
// argument is its own independent unit": a chunk missing its trailing
// ';' still runs, implicitly terminated at the chunk's own end.
func TestRunBatchChunkImplicitTrailingSemicolon(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)

	// stop == false: a successfully completed chunk tells runBatch's loop
	// to move on to the next chunk, not to abort (runBatchChunk's doc
	// comment; runBatch's own "if code, stop := ...; stop { return code }").
	code, stop := r.runBatchChunk("SELECT 42")
	if stop || code != 0 {
		t.Fatalf("runBatchChunk(%q) = (%d, %v), want (0, false)", "SELECT 42", code, stop)
	}
	if strings.TrimSpace(out.String()) != "42" {
		t.Fatalf("output = %q, want \"42\"", out.String())
	}
	if errw.Len() != 0 {
		t.Fatalf("unexpected errw: %q", errw.String())
	}
}

// TestRunBatchChunkIncompleteStatementIsSyntaxError guards that an
// unterminated statement (not just a missing ';', but genuinely
// incomplete SQL) is reported as a syntax error rather than silently
// dropped or run as-is.
func TestRunBatchChunkIncompleteStatementIsSyntaxError(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)

	code, stop := r.runBatchChunk("SELECT 'unterminated")
	if !stop || code != 1 {
		t.Fatalf("runBatchChunk(unterminated) = (%d, %v), want (1, true)", code, stop)
	}
	if !strings.Contains(errw.String(), "incomplete") {
		t.Fatalf("errw = %q, want a message mentioning the statement is incomplete", errw.String())
	}
}

// TestRunBatchChunkDotCommandAfterCompleteStatement is a regression test
// for a real bug found during development: splitComplete can leave a
// whitespace-only remainder (just the "\n" right after a statement's
// ";") that must reset buf to truly empty, not to that whitespace --
// otherwise buf.Len() == 0 never holds again for the rest of the chunk,
// and a dot command on the very next line is fed to execSQL as SQL text
// instead of being recognized (it fails as "near '.': syntax error").
// Confirmed by reverting the fix and watching this fail the same way.
func TestRunBatchChunkDotCommandAfterCompleteStatement(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)

	code, stop := r.runBatchChunk("CREATE TABLE t(a INTEGER);\n.tables")
	if stop || code != 0 {
		t.Fatalf("runBatchChunk = (%d, %v), want (0, false); errw=%q", code, stop, errw.String())
	}
	if strings.TrimSpace(out.String()) != "t" {
		t.Fatalf(".tables output = %q, want \"t\" (errw: %q)", out.String(), errw.String())
	}
}

// TestRunBatchMultiStatementChunk confirms a single chunk containing
// several ';'-terminated statements still runs each of them (splitComplete's
// own job, exercised here through the batch path rather than the REPL's).
func TestRunBatchMultiStatementChunk(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newTestRepl(t, db, "self", &out, &errw)

	code := r.runBatch([]string{"SELECT 1; SELECT 2;"})
	if code != 0 {
		t.Fatalf("runBatch code = %d, want 0 (errw: %q)", code, errw.String())
	}
	lines := strings.Fields(out.String())
	if len(lines) != 2 || lines[0] != "1" || lines[1] != "2" {
		t.Fatalf("output = %q, want two lines \"1\" and \"2\"", out.String())
	}
}

func TestRunBatchFromReader(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	code := runBatchFromReader(db, "self", &options{}, strings.NewReader("CREATE TABLE t(x);\nINSERT INTO t VALUES(7);\nSELECT * FROM t;\n"), &out, &errw)
	if code != 0 {
		t.Fatalf("runBatchFromReader code = %d, want 0 (errw: %q)", code, errw.String())
	}
	if strings.TrimSpace(out.String()) != "7" {
		t.Fatalf("output = %q, want \"7\"", out.String())
	}
}

func TestRunBatchTopLevel(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	code := runBatch(db, "self", &options{}, []string{"CREATE TABLE t(x)", "INSERT INTO t VALUES(3)", "SELECT * FROM t"}, &out, &errw)
	if code != 0 {
		t.Fatalf("runBatch code = %d, want 0 (errw: %q)", code, errw.String())
	}
	if strings.TrimSpace(out.String()) != "3" {
		t.Fatalf("output = %q, want \"3\"", out.String())
	}
}
