package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/amisonnet8/san-db-ox/engine"
)

// newReadOnlyTestRepl mirrors newTestRepl (dotcmd_test.go) but with
// opts.readOnly set, for exercising the --read-only checks in
// doSnapshot/doLoad/doOverwrite/cmdHelp.
func newReadOnlyTestRepl(t *testing.T, db *engine.DB, out, errw *bytes.Buffer) *repl {
	t.Helper()
	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatalf("db.Session: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return &repl{db: db, sess: sess, self: "self", opts: &options{readOnly: true}, mode: modeList, out: out, errw: errw}
}

func TestReadOnlyNilOptsIsNotReadOnly(t *testing.T) {
	r := &repl{}
	if r.readOnly() {
		t.Fatal("a repl with nil opts should not be read-only")
	}
}

func TestDoSnapshotRejectedUnderReadOnly(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newReadOnlyTestRepl(t, db, &out, &errw)

	_, err := r.doSnapshot("whatever", false, false)
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("doSnapshot error = %v, want ErrReadOnly", err)
	}
}

// TestDoSnapshotSQLiteRejectedUnderReadOnly guards spec §2's explicit
// inclusion of ".snapshot --sqlite" among the rejected operations, even
// though it never touches the live DB's own content.
func TestDoSnapshotSQLiteRejectedUnderReadOnly(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newReadOnlyTestRepl(t, db, &out, &errw)

	_, err := r.doSnapshot("whatever", true, false)
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("doSnapshot(--sqlite) error = %v, want ErrReadOnly", err)
	}
}

func TestDoLoadRejectedUnderReadOnly(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newReadOnlyTestRepl(t, db, &out, &errw)

	_, err := r.doLoad("whatever")
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("doLoad error = %v, want ErrReadOnly", err)
	}
}

func TestDoOverwriteRejectedUnderReadOnly(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newReadOnlyTestRepl(t, db, &out, &errw)

	if err := r.doOverwrite(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("doOverwrite error = %v, want ErrReadOnly", err)
	}
}

// TestCmdHelpReadOnlyOmitsSaveOperations guards spec §2's ".helpの出力を
// 差し替え、拒否される操作を一覧に含めない".
func TestCmdHelpReadOnlyOmitsSaveOperations(t *testing.T) {
	var out bytes.Buffer
	cmdHelpReadOnly(&out)
	for _, unwanted := range []string{".snapshot", ".overwrite", ".load", ".import"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("read-only .help should not list %q, got:\n%s", unwanted, out.String())
		}
	}
	for _, wanted := range []string{".tables", ".schema", ".mode", ".headers", ".dump", ".exit", ".help"} {
		if !strings.Contains(out.String(), wanted) {
			t.Errorf("read-only .help is missing %q, got:\n%s", wanted, out.String())
		}
	}
}

// TestCmdHelpDispatchesToReadOnlyVariant confirms handleDotCommand's
// ".help" branch actually picks cmdHelpReadOnly when r is read-only,
// rather than only cmdHelpReadOnly being correct in isolation.
func TestCmdHelpDispatchesToReadOnlyVariant(t *testing.T) {
	db := newTestDB(t)
	var out, errw bytes.Buffer
	r := newReadOnlyTestRepl(t, db, &out, &errw)

	if _, _, err := r.handleDotCommand(".help"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), ".snapshot") {
		t.Fatalf(".help under --read-only should not list .snapshot, got:\n%s", out.String())
	}
}

// TestOpenSessionReadOnlyRejectsWrites confirms --read-only's write-SQL
// rejection (spec §2: implemented via SQLite's own query_only pragma on
// the Session openSession opens) actually takes effect, not just that
// the pragma statement itself succeeds.
func TestOpenSessionReadOnlyRejectsWrites(t *testing.T) {
	db := newTestDB(t)
	sess, err := openSession(context.Background(), db, &options{readOnly: true})
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	defer sess.Close()

	if _, err := sess.Exec("CREATE TABLE t(a)"); err == nil {
		t.Fatal("expected CREATE TABLE to fail under a query_only Session")
	}
}

// TestOpenSessionNotReadOnlyAllowsWrites is the control for the test
// above: without --read-only, the same statement must succeed.
func TestOpenSessionNotReadOnlyAllowsWrites(t *testing.T) {
	db := newTestDB(t)
	sess, err := openSession(context.Background(), db, &options{})
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	defer sess.Close()

	if _, err := sess.Exec("CREATE TABLE t(a)"); err != nil {
		t.Fatalf("CREATE TABLE should succeed without --read-only: %v", err)
	}
}
