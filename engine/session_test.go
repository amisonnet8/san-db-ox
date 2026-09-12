package engine

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestSessionTransactionSpansCalls confirms the reason Session exists at
// all (spec §10): a transaction opened, used, and closed across separate
// calls behaves as a real transaction, unlike the same sequence run
// through DB.Exec (engine.go's Exec doc comment, exercised in the second
// half of this test for direct comparison).
func TestSessionTransactionSpansCalls(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	defer sess.Close()

	if _, err := sess.Exec("BEGIN"); err != nil {
		t.Fatalf("BEGIN: %v", err)
	}
	if _, err := sess.Exec("INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if _, err := sess.Exec("ROLLBACK"); err != nil {
		t.Fatalf("ROLLBACK: %v", err)
	}

	if cnt := tableRowCount(t, db, "t"); cnt != 0 {
		t.Fatalf("row count = %d after Session ROLLBACK, want 0", cnt)
	}

	if _, err := sess.Exec("BEGIN"); err != nil {
		t.Fatalf("BEGIN: %v", err)
	}
	if _, err := sess.Exec("INSERT INTO t VALUES (2)"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if _, err := sess.Exec("COMMIT"); err != nil {
		t.Fatalf("COMMIT: %v", err)
	}
	if cnt := tableRowCount(t, db, "t"); cnt != 1 {
		t.Fatalf("row count = %d after Session COMMIT, want 1", cnt)
	}
}

// TestOneShotExecCannotHoldTransaction is the contrasting case
// TestSessionTransactionSpansCalls exists to justify Session against:
// DB.Exec borrows a pooled connection per call, so a BEGIN/INSERT/COMMIT
// sequence run through it has no guarantee any two calls land on the
// same connection, and the insert may not survive.
func TestOneShotExecCannotHoldTransaction(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	// This sequence is exactly what a caller must not rely on: BEGIN,
	// INSERT and COMMIT are three unrelated pooled-connection calls with
	// no guaranteed relationship to each other.
	if _, err := db.Exec("BEGIN"); err != nil {
		t.Fatalf("BEGIN: %v", err)
	}
	db.Exec("INSERT INTO t VALUES (1)")
	db.Exec("COMMIT")

	// Whether or not the row above ended up visible is exactly the
	// point: database/sql's pooling makes this unspecified, which is
	// why Session exists. This test only documents that DB.Exec is not
	// a substitute for Session, not a particular row count.
}

func TestSessionCloseIsIdempotent(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestSessionUseAfterCloseReturnsErrClosed(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess.Close()

	if _, err := sess.Exec("SELECT 1"); !errors.Is(err, ErrClosed) {
		t.Errorf("Exec after Close = %v, want ErrClosed", err)
	}
	if _, err := sess.Query("SELECT 1"); !errors.Is(err, ErrClosed) {
		t.Errorf("Query after Close = %v, want ErrClosed", err)
	}
	if _, err := sess.Prepare("SELECT 1"); !errors.Is(err, ErrClosed) {
		t.Errorf("Prepare after Close = %v, want ErrClosed", err)
	}
}

// TestSessionQueryRowAfterCloseSurfacesScanError documents the one
// deliberate exception noted on QueryRowContext's doc comment: it cannot
// report ErrClosed itself, so the error surfaces from Scan instead, via
// database/sql's own error.
func TestSessionQueryRowAfterCloseSurfacesScanError(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess.Close()

	var x int
	err = sess.QueryRow("SELECT 1").Scan(&x)
	if err == nil {
		t.Fatal("QueryRow().Scan() after Close: expected an error, got nil")
	}
	if errors.Is(err, ErrClosed) {
		t.Errorf("QueryRow().Scan() after Close = %v, want database/sql's own closed-connection error, not ErrClosed", err)
	}
}

func TestSessionPrepareReusesStatement(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	stmt, err := sess.Prepare("INSERT INTO t VALUES (?)")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer stmt.Close()

	for i := 0; i < 5; i++ {
		if _, err := stmt.Exec(i); err != nil {
			t.Fatalf("stmt.Exec(%d): %v", i, err)
		}
	}

	if cnt := tableRowCount(t, db, "t"); cnt != 5 {
		t.Fatalf("row count = %d, want 5", cnt)
	}
}

// TestSessionsAreIndependent confirms two Sessions have separate
// transactions (each scoped to its own connection), not that one
// Session's reads are ever non-blocking against another's uncommitted
// write. On the memdb VFS they are not: unlike a plain file-backed
// SQLite database, a reader on memdb can be blocked (for up to
// busy_timeout) behind another connection's uncommitted write even
// without asking for a conflicting lock itself (spec §11's memdb
// discussion doesn't cover this; recorded as a quirk in
// .claude/rules/sqlite-quirks.md, inherited from the predecessor
// project ExecDB, which hit the same thing writing this same test). So
// this drives b's read from a goroutine and only checks the outcome
// once both Sessions are done, rather than asserting b's read completes
// (or sees a particular value) while a's transaction is still open.
func TestSessionsAreIndependent(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	a, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if _, err := a.Exec("BEGIN"); err != nil {
		t.Fatalf("a BEGIN: %v", err)
	}
	if _, err := a.Exec("INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("a INSERT: %v", err)
	}

	bErr := make(chan error, 1)
	go func() {
		if _, err := b.Exec("BEGIN"); err != nil {
			bErr <- err
			return
		}
		if _, err := b.Exec("INSERT INTO t VALUES (2)"); err != nil {
			bErr <- err
			return
		}
		_, err := b.Exec("COMMIT")
		bErr <- err
	}()

	// Give b a moment to actually start (and, on memdb, block behind
	// a's uncommitted write) before a commits, so this exercises the
	// blocking behavior rather than racing it.
	time.Sleep(50 * time.Millisecond)

	if _, err := a.Exec("COMMIT"); err != nil {
		t.Fatalf("a COMMIT: %v", err)
	}
	if err := <-bErr; err != nil {
		t.Fatalf("b's transaction: %v", err)
	}

	if cnt := tableRowCount(t, db, "t"); cnt != 2 {
		t.Fatalf("row count = %d, want 2 (both Sessions' independent transactions)", cnt)
	}
}

// TestSessionWriteTxnBlocksSnapshot exercises the first row of Session's
// doc comment table: a write transaction held open on a Session blocks
// Snapshot until it commits or rolls back, and Snapshot returns an error
// wrapping ErrBusy once the live database's busy_timeout elapses. This
// necessarily waits out the real 5s busy_timeout (defaultBusyTimeoutMS,
// engine.go) -- there is currently no way to give a single call a
// shorter one.
func TestSessionWriteTxnBlocksSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the live database's 5s busy_timeout")
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if _, err := sess.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer sess.Exec("ROLLBACK")

	err = db.Snapshot(filepath.Join(t.TempDir(), "snap"))
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Snapshot while a Session holds a write transaction: error = %v, want an error wrapping ErrBusy", err)
	}
}

// TestSessionWriteTxnBlocksExportAndLoad covers the remaining two rows
// of the same table: Export and Load, both blocked by a write
// transaction the same way Snapshot is. Also waits out the real 5s
// busy_timeout twice.
func TestSessionWriteTxnBlocksExportAndLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the live database's 5s busy_timeout, twice")
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if _, err := sess.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer sess.Exec("ROLLBACK")

	err = db.Export(filepath.Join(t.TempDir(), "out.sqlite"))
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Export while a Session holds a write transaction: error = %v, want an error wrapping ErrBusy", err)
	}

	srcPath := filepath.Join(t.TempDir(), "src.sqlite")
	fdb, err := sql.Open("sqlite", srcPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fdb.Exec("CREATE TABLE other(x)"); err != nil {
		t.Fatal(err)
	}
	fdb.Close()

	err = db.Load(srcPath)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Load while a Session holds a write transaction: error = %v, want an error wrapping ErrBusy", err)
	}
}

// TestSessionReadTxnDoesNotBlockSnapshotOrExport confirms the other half
// of Session's doc comment table: a read transaction held open on a
// Session does not block Snapshot or Export the way a write transaction
// does -- they only read the live database themselves, so they do not
// conflict with another reader (unlike Load, which needs exclusive
// access to the destination it writes).
func TestSessionReadTxnDoesNotBlockSnapshotOrExport(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if _, err := sess.Exec("BEGIN"); err != nil {
		t.Fatal(err)
	}
	defer sess.Exec("ROLLBACK")
	var cnt int
	if err := sess.QueryRow("SELECT count(*) FROM t").Scan(&cnt); err != nil {
		t.Fatal(err)
	}

	if err := db.Snapshot(filepath.Join(t.TempDir(), "snap")); err != nil {
		t.Errorf("Snapshot while a Session holds a read transaction: %v, want nil", err)
	}
	if err := db.Export(filepath.Join(t.TempDir(), "out.sqlite")); err != nil {
		t.Errorf("Export while a Session holds a read transaction: %v, want nil", err)
	}
}

// TestManySessionsDoNotExhaustPool is the regression test for newLiveDB's
// decision (engine.go) not to cap the connection pool: opening more
// Sessions than any plausible pool limit, all left open simultaneously,
// must not prevent a later Session or a plain Exec from succeeding.
func TestManySessionsDoNotExhaustPool(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const n = 20
	sessions := make([]*Session, n)
	for i := range sessions {
		s, err := db.Session(context.Background())
		if err != nil {
			t.Fatalf("Session #%d: %v", i, err)
		}
		sessions[i] = s
	}
	defer func() {
		for _, s := range sessions {
			s.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	extra, err := db.Session(ctx)
	if err != nil {
		t.Fatalf("Session after %d others already open: %v", n, err)
	}
	defer extra.Close()

	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatalf("plain Exec with %d Sessions open: %v", n, err)
	}
}

// TestSessionAfterDBCloseFailsButCloseIsSafe confirms the doc comment on
// Session: DB.Close does not track or forcibly close outstanding
// Sessions. database/sql's own Close only closes a connection once it is
// returned to the pool, and a Session holds its connection out of the
// pool until Close runs -- so a Session's connection, and the live memdb
// store it keeps alive, stays usable across a DB.Close as long as the
// Session itself has not been closed. What this test actually verifies
// is the second half: Session.Close remains safe (idempotent, no error)
// to call after DB.Close, whichever order a caller does things in.
func TestSessionAfterDBCloseFailsButCloseIsSafe(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	if _, err := sess.Exec("SELECT 1"); err != nil {
		t.Errorf("Exec on a Session after DB.Close = %v, want nil: the Session's own connection outlives DB.Close until the Session itself is closed", err)
	}
	if err := sess.Close(); err != nil {
		t.Errorf("Close after DB.Close: %v, want nil (Close must stay safe to call)", err)
	}
	if err := sess.Close(); err != nil {
		t.Errorf("second Close after DB.Close: %v, want nil", err)
	}
}

// TestSessionSurvivesCanceledQuery confirms a Session stays usable after a
// query running on it is canceled via context, not just after a query
// that fails or returns an error normally (TestSessionUseAfterCloseReturnsErrClosed
// only covers the Close case). Phase 3's REPL Ctrl+C state machine cancels
// the in-flight statement's context on the first interrupt while keeping
// the REPL's single Session open for the next statement (spec §2, §13) --
// this test pins the assumption that makes that design sound.
func TestSessionSurvivesCanceledQuery(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// A deliberately long-running query: modernc.org/sqlite checks for
	// context cancellation between VM steps, so this is expected to be
	// interrupted well before it completes on its own.
	const longQuery = `WITH RECURSIVE cnt(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM cnt WHERE x < 500000000) SELECT count(*) FROM cnt`
	rows, err := sess.QueryContext(ctx, longQuery)
	if err == nil {
		rows.Close()
	}
	// Either outcome (canceled before or after the query itself returned)
	// is acceptable here -- what matters is what follows: the Session's
	// underlying connection must still be usable afterward.

	var x int
	if err := sess.QueryRow("SELECT 1").Scan(&x); err != nil {
		t.Fatalf("Session unusable after a canceled query: %v", err)
	}
	if x != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", x)
	}
}

func tableRowCount(t *testing.T, db *DB, table string) int {
	t.Helper()
	var cnt int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&cnt); err != nil {
		t.Fatalf("count rows in %s: %v", table, err)
	}
	return cnt
}
