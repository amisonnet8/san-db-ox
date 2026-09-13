package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/amisonnet8/san-db-ox/engine"
)

// stdioTestSession drives runStdio over a pair of io.Pipes so requests
// and responses can be exchanged synchronously, one line at a time, the
// same way a real client would talk to it over a subprocess's stdin/
// stdout. Every read is bounded (readLine's t.Fatal on scanner.Scan
// returning false, and the caller's own responsibility to bound how long
// it waits on <-done) so a protocol bug here (a response never sent, a
// deadlock from a missed Flush) fails the test loudly instead of hanging
// the suite (.claude/rules/testing.md: stdio checks must always be able
// to detect a hang, not just success).
type stdioTestSession struct {
	t       *testing.T
	inW     *io.PipeWriter
	outR    *io.PipeReader
	scanner *bufio.Scanner
	errw    *bytes.Buffer
	done    chan int
}

func newStdioTestSession(t *testing.T, db *engine.DB, opts *options) *stdioTestSession {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	errw := &bytes.Buffer{}

	s := &stdioTestSession{t: t, inW: inW, outR: outR, scanner: bufio.NewScanner(outR), errw: errw, done: make(chan int, 1)}
	go func() {
		s.done <- runStdio(db, "self", opts, inR, outW, errw)
		outW.Close()
	}()
	t.Cleanup(func() {
		inW.Close()
		outR.Close()
	})
	return s
}

func (s *stdioTestSession) readLine() map[string]any {
	s.t.Helper()
	if !s.scanner.Scan() {
		s.t.Fatalf("no more stdio output (scanner error: %v)", s.scanner.Err())
	}
	var v map[string]any
	if err := json.Unmarshal(s.scanner.Bytes(), &v); err != nil {
		s.t.Fatalf("invalid JSON line %q: %v", s.scanner.Text(), err)
	}
	return v
}

func (s *stdioTestSession) send(req string) {
	s.t.Helper()
	if _, err := io.WriteString(s.inW, req+"\n"); err != nil {
		s.t.Fatalf("write request %q: %v", req, err)
	}
}

// waitDone waits for runStdio to return, failing the test rather than
// hanging forever if it does not (a stuck close/EOF path would otherwise
// block `go test` indefinitely).
func (s *stdioTestSession) waitDone() int {
	s.t.Helper()
	select {
	case code := <-s.done:
		return code
	case <-time.After(5 * time.Second):
		s.t.Fatal("runStdio did not return in time")
		return -1
	}
}

func TestRunStdioHelloLine(t *testing.T) {
	db := newTestDB(t)
	s := newStdioTestSession(t, db, &options{})

	hello := s.readLine()
	if hello["protocol"] != float64(1) {
		t.Fatalf("hello.protocol = %v, want 1", hello["protocol"])
	}
	if hello["product"] != "SanDBox" {
		t.Fatalf("hello.product = %v, want SanDBox", hello["product"])
	}
	if _, ok := hello["version"]; !ok {
		t.Fatal("hello line is missing version")
	}

	s.inW.Close()
	if code := s.waitDone(); code != 0 {
		t.Fatalf("runStdio exit code = %d, want 0 on stdin EOF", code)
	}
}

// TestRunStdioQueryExecValueRoundTrip exercises NULL/INTEGER/REAL/TEXT/
// BLOB in both directions (spec §7's value table), including a BLOB sent
// back as a bind parameter (the 1-element base64 array).
func TestRunStdioQueryExecValueRoundTrip(t *testing.T) {
	db := newTestDB(t)
	s := newStdioTestSession(t, db, &options{})
	s.readLine() // hello

	s.send(`{"op":"exec","sql":"CREATE TABLE t(i INTEGER, r REAL, s TEXT, b BLOB, n TEXT)"}`)
	if resp := s.readLine(); resp["ok"] != true {
		t.Fatalf("CREATE failed: %v", resp)
	}

	// "aGVsbG8=" is base64("hello").
	s.send(`{"op":"exec","sql":"INSERT INTO t VALUES (?, ?, ?, ?, ?)","params":[42,1.5,"alice",["aGVsbG8="],null]}`)
	resp := s.readLine()
	if resp["ok"] != true {
		t.Fatalf("INSERT failed: %v", resp)
	}
	if resp["rows_affected"] != float64(1) {
		t.Fatalf("rows_affected = %v, want 1", resp["rows_affected"])
	}

	s.send(`{"op":"query","sql":"SELECT i, r, s, b, n FROM t"}`)
	resp = s.readLine()
	if resp["ok"] != true {
		t.Fatalf("query failed: %v", resp)
	}
	rows, _ := resp["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want 1 row", resp["rows"])
	}
	row, _ := rows[0].([]any)
	if len(row) != 5 {
		t.Fatalf("row = %v, want 5 values", row)
	}
	if row[0] != float64(42) {
		t.Errorf("i = %v, want 42", row[0])
	}
	if row[1] != float64(1.5) {
		t.Errorf("r = %v, want 1.5", row[1])
	}
	if row[2] != "alice" {
		t.Errorf("s = %v, want alice", row[2])
	}
	blob, ok := row[3].([]any)
	if !ok || len(blob) != 1 || blob[0] != "aGVsbG8=" {
		t.Errorf("b = %v, want a 1-element array [\"aGVsbG8=\"]", row[3])
	}
	if row[4] != nil {
		t.Errorf("n = %v, want null", row[4])
	}

	s.send(`{"op":"close"}`)
	if resp := s.readLine(); resp["ok"] != true {
		t.Fatalf("close failed: %v", resp)
	}
	if code := s.waitDone(); code != 0 {
		t.Fatalf("runStdio exit code after close = %d, want 0", code)
	}
}

// TestRunStdioIDIsEchoedBack guards spec §7's "id フィールドを含めた
// 場合、対応するレスポンスにそのまま echo back する".
func TestRunStdioIDIsEchoedBack(t *testing.T) {
	db := newTestDB(t)
	s := newStdioTestSession(t, db, &options{})
	s.readLine() // hello

	s.send(`{"id":"req-1","op":"query","sql":"SELECT 1"}`)
	resp := s.readLine()
	if resp["id"] != "req-1" {
		t.Fatalf("id = %v, want req-1", resp["id"])
	}

	s.inW.Close()
	s.waitDone()
}

// TestRunStdioMalformedJSONDoesNotCloseConnection guards spec §7:
// "リクエストが不正なJSONである場合も、エラーレスポンスを1行返して
// 処理を継続する（接続は切らない）".
func TestRunStdioMalformedJSONDoesNotCloseConnection(t *testing.T) {
	db := newTestDB(t)
	s := newStdioTestSession(t, db, &options{})
	s.readLine() // hello

	s.send(`not json`)
	resp := s.readLine()
	if resp["ok"] != false {
		t.Fatalf("malformed JSON response = %v, want ok:false", resp)
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != "bad_request" {
		t.Fatalf("error.code = %v, want bad_request", errObj["code"])
	}

	// The connection must still work afterward.
	s.send(`{"op":"query","sql":"SELECT 1"}`)
	resp = s.readLine()
	if resp["ok"] != true {
		t.Fatalf("query after malformed JSON failed: %v", resp)
	}

	s.inW.Close()
	s.waitDone()
}

func TestRunStdioUnknownOp(t *testing.T) {
	db := newTestDB(t)
	s := newStdioTestSession(t, db, &options{})
	s.readLine() // hello

	s.send(`{"op":"frobnicate"}`)
	resp := s.readLine()
	errObj, _ := resp["error"].(map[string]any)
	if resp["ok"] != false || errObj["code"] != "unsupported_op" {
		t.Fatalf("unknown op response = %v, want ok:false code:unsupported_op", resp)
	}

	s.inW.Close()
	s.waitDone()
}

// TestRunStdioTablesSchemaDump exercises the three read-only dot-command
// ops that reuse dotcmd.go/dump.go's logic (listTables/schemaSQL/
// dumpSQL). "dump" is Phase ④'s own addition to the op table (spec §7).
func TestRunStdioTablesSchemaDump(t *testing.T) {
	db := newTestDB(t)
	s := newStdioTestSession(t, db, &options{})
	s.readLine() // hello

	s.send(`{"op":"exec","sql":"CREATE TABLE t(a INTEGER)"}`)
	s.readLine()
	s.send(`{"op":"exec","sql":"INSERT INTO t VALUES (1)"}`)
	s.readLine()

	s.send(`{"op":"tables"}`)
	resp := s.readLine()
	tables, _ := resp["tables"].([]any)
	if len(tables) != 1 || tables[0] != "t" {
		t.Fatalf("tables = %v, want [\"t\"]", resp["tables"])
	}

	s.send(`{"op":"schema"}`)
	resp = s.readLine()
	schema, _ := resp["schema"].([]any)
	if len(schema) != 1 || !strings.Contains(schema[0].(string), "CREATE TABLE t") {
		t.Fatalf("schema = %v, want one CREATE TABLE t statement", resp["schema"])
	}

	s.send(`{"op":"dump"}`)
	resp = s.readLine()
	sqlText, _ := resp["sql"].(string)
	if !strings.Contains(sqlText, "INSERT INTO \"t\" VALUES(1)") {
		t.Fatalf("dump sql = %q, missing the expected INSERT", sqlText)
	}

	s.inW.Close()
	s.waitDone()
}

func TestRunStdioInspectNoData(t *testing.T) {
	db := newTestDB(t)
	s := newStdioTestSession(t, db, &options{})
	s.readLine() // hello

	s.send(`{"op":"inspect"}`)
	resp := s.readLine()
	if resp["has_data"] != false {
		t.Fatalf("has_data = %v, want false", resp["has_data"])
	}
	if resp["version"] != nil {
		t.Fatalf("version = %v, want null when has_data is false", resp["version"])
	}
	if resp["data_length"] != nil {
		t.Fatalf("data_length = %v, want null when has_data is false", resp["data_length"])
	}
	if resp["read_only"] != false {
		t.Fatalf("read_only = %v, want false", resp["read_only"])
	}

	s.inW.Close()
	s.waitDone()
}

// TestRunStdioReadOnlyRejectsWritesAndSaves guards --read-only's stdio
// behavior (spec §2, §7): a write exec fails (via the query_only pragma
// openSession applies), and snapshot/load/overwrite return read_only
// without touching the filesystem or the live DB.
func TestRunStdioReadOnlyRejectsWritesAndSaves(t *testing.T) {
	db := newTestDB(t)
	s := newStdioTestSession(t, db, &options{readOnly: true})
	s.readLine() // hello

	s.send(`{"op":"exec","sql":"CREATE TABLE t(a)"}`)
	resp := s.readLine()
	if resp["ok"] != false {
		t.Fatalf("write exec under --read-only = %v, want ok:false", resp)
	}

	s.send(`{"op":"snapshot"}`)
	resp = s.readLine()
	errObj, _ := resp["error"].(map[string]any)
	if resp["ok"] != false || errObj["code"] != "read_only" {
		t.Fatalf("snapshot under --read-only = %v, want ok:false code:read_only", resp)
	}

	s.send(`{"op":"load","path":"whatever"}`)
	resp = s.readLine()
	errObj, _ = resp["error"].(map[string]any)
	if resp["ok"] != false || errObj["code"] != "read_only" {
		t.Fatalf("load under --read-only = %v, want ok:false code:read_only", resp)
	}

	s.send(`{"op":"overwrite"}`)
	resp = s.readLine()
	errObj, _ = resp["error"].(map[string]any)
	if resp["ok"] != false || errObj["code"] != "read_only" {
		t.Fatalf("overwrite under --read-only = %v, want ok:false code:read_only", resp)
	}

	s.send(`{"op":"inspect"}`)
	resp = s.readLine()
	if resp["read_only"] != true {
		t.Fatalf("inspect.read_only = %v, want true", resp["read_only"])
	}

	s.inW.Close()
	// runStdio must still return promptly (i.e. the rejected ops above
	// did not leave the Session or process in a stuck state).
	if code := s.waitDone(); code != 0 {
		t.Fatalf("runStdio exit code = %d, want 0", code)
	}
}

func TestRunStdioLoadMissingPathIsBadRequest(t *testing.T) {
	db := newTestDB(t)
	s := newStdioTestSession(t, db, &options{})
	s.readLine() // hello

	s.send(`{"op":"load"}`)
	resp := s.readLine()
	errObj, _ := resp["error"].(map[string]any)
	if resp["ok"] != false || errObj["code"] != "bad_request" {
		t.Fatalf("load with no path = %v, want ok:false code:bad_request", resp)
	}

	s.inW.Close()
	s.waitDone()
}
