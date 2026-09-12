package engine

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadWALDatabaseSeesUncheckpointedCommits is the regression test for
// the design decision behind Load's KindSQLite branch using restoreFrom
// (SQLite's Backup API run against the source file, backup.go) rather
// than reading the file's bytes and Deserializing them: a source file
// running in journal_mode=WAL can have committed rows that exist only in
// its "-wal" sidecar, not yet checkpointed into the main file. Confirmed
// in the Phase 2 Step 0 spike that reading just the main file's bytes
// would silently miss them.
func TestLoadWALDatabaseSeesUncheckpointedCommits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.sqlite")
	fdb, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fdb.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	if _, err := fdb.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}
	if _, err := fdb.Exec("INSERT INTO t VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	// Deliberately leave fdb open (not checkpointed/closed) so the
	// commit above sits only in the "-wal" sidecar when Load runs.
	defer fdb.Close()

	if _, err := os.Stat(path + "-wal"); err != nil {
		t.Fatalf("expected an uncheckpointed -wal sidecar to exist: %v", err)
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	var cnt int
	if err := db.QueryRow("SELECT count(*) FROM t").Scan(&cnt); err != nil {
		t.Fatalf("query after Load: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("count = %d, want 1 (the row committed only to the -wal sidecar)", cnt)
	}
}

// TestLoadKeepsExistingConnectionsAlive is the regression test for spec
// §4's "既存のセッションは接続を失わない": a connection obtained before
// Load runs must still work afterward, on the same connection, seeing
// the newly loaded data. This exercises the same guarantee
// TestLoadKeepsExistingSessionsAlive (added once Session exists in Step
// 5) verifies through the public Session API -- Load itself has no way
// to know the difference between a bare *sql.Conn and one wrapped in a
// Session, since Session is just a thin wrapper around one.
func TestLoadKeepsExistingConnectionsAlive(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE old(x)"); err != nil {
		t.Fatal(err)
	}

	sess, err := db.sdb.Conn(context.Background())
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer sess.Close()

	loadPath := filepath.Join(t.TempDir(), "new.sqlite")
	other, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Exec("CREATE TABLE fresh(v); INSERT INTO fresh VALUES ('hi')"); err != nil {
		t.Fatal(err)
	}
	conn, err := other.sdb.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	blob, err := serializeConn(conn)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loadPath, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := db.Load(loadPath); err != nil {
		t.Fatalf("Load: %v", err)
	}

	var val string
	if err := sess.QueryRowContext(context.Background(), "SELECT v FROM fresh").Scan(&val); err != nil {
		t.Fatalf("query on pre-Load connection after Load: %v", err)
	}
	if val != "hi" {
		t.Fatalf("val = %q, want %q", val, "hi")
	}
}

func TestLoadSQLiteFileReplacesState(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE old_table(x)"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "replacement.sqlite")
	fdb, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fdb.Exec("CREATE TABLE new_table(x)"); err != nil {
		t.Fatal(err)
	}
	fdb.Close()

	if err := db.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE name='old_table'").Scan(&name)
	if err != sql.ErrNoRows {
		t.Fatalf("old_table still present after Load (err=%v) -- Load should replace, not merge", err)
	}
	if err := db.QueryRow("SELECT name FROM sqlite_master WHERE name='new_table'").Scan(&name); err != nil {
		t.Fatalf("new_table missing after Load: %v", err)
	}
}

func TestLoadSanDBoxExecutable(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	src, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Exec("CREATE TABLE t(x); INSERT INTO t VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snap")
	if err := src.Snapshot(path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	if err := db.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	var x int
	if err := db.QueryRow("SELECT x FROM t").Scan(&x); err != nil {
		t.Fatalf("query after Load: %v", err)
	}
	if x != 42 {
		t.Fatalf("x = %d, want 42", x)
	}
}

func TestLoadUnsupportedFileLeavesStateUntouched(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE keep(x); INSERT INTO keep VALUES (7)"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "garbage")
	if err := os.WriteFile(path, []byte("not a recognizable format, long enough to exceed the footer size threshold"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = db.Load(path)
	if !errors.Is(err, ErrUnsupportedFile) {
		t.Fatalf("Load error = %v, want ErrUnsupportedFile", err)
	}

	var x int
	if err := db.QueryRow("SELECT x FROM keep").Scan(&x); err != nil {
		t.Fatalf("original data missing after failed Load: %v", err)
	}
	if x != 7 {
		t.Fatalf("x = %d, want 7 (original data must survive a failed Load)", x)
	}
}

func TestLoadZeroLengthDataIsError(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE keep(x)"); err != nil {
		t.Fatal(err)
	}

	// A footer claiming zero-length data over a nonzero "engine" prefix.
	path := filepath.Join(t.TempDir(), "empty-data")
	blob := append([]byte("fake-engine-bytes"), encodeFooter(17, 0)...)
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	err = db.Load(path)
	if !errors.Is(err, ErrNoData) {
		t.Fatalf("Load error = %v, want ErrNoData", err)
	}
	if cnt := tableCount(t, db); cnt != 1 {
		t.Fatalf("table count = %d, want 1 (original data must survive)", cnt)
	}
}

// TestLoadSQLiteFileFailsCleanlyWhenDestinationIsBusy exercises the final
// stage of loadFromSQLiteFile's two-step restore (backup.go's runBackup,
// via restoreFrom into the live database) failing because another
// connection holds a write transaction open on the live database for the
// live database's entire busy_timeout: Load must return an error
// wrapping ErrBusy, and the live database must be left exactly as it
// was (spec §4, backed by runBackup always calling Backup.Finish --
// backup.go). The lock is held for the whole call (rather than released
// partway through, which would make the outcome a race), so this
// necessarily waits out the live database's busy_timeout (5s,
// defaultBusyTimeoutMS in engine.go) -- there is currently no way to
// give a single Load call a shorter one.
func TestLoadSQLiteFileFailsCleanlyWhenDestinationIsBusy(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the live database's 5s busy_timeout")
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE keep(x); INSERT INTO keep VALUES (11)"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "replacement.sqlite")
	fdb, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fdb.Exec("CREATE TABLE new_table(x)"); err != nil {
		t.Fatal(err)
	}
	fdb.Close()

	blocker, err := db.sdb.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	if _, err := blocker.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}

	err = db.Load(path)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Load error = %v, want an error wrapping ErrBusy", err)
	}

	// Release the lock before asserting on the live database's state:
	// the assertions below run their own queries against it, which would
	// otherwise wait out busy_timeout too.
	if _, err := blocker.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}

	var x int
	if qerr := db.QueryRow("SELECT x FROM keep").Scan(&x); qerr != nil {
		t.Fatalf("original data missing after failed Load: %v", qerr)
	}
	if x != 11 {
		t.Fatalf("keep.x = %d, want 11 (original data must survive a failed Load)", x)
	}
}

func TestLoadFromReaderSanDBoxImage(t *testing.T) {
	src, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Exec("CREATE TABLE t(x); INSERT INTO t VALUES (9)"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snap")
	if err := src.Snapshot(path); err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.LoadFrom(bytes.NewReader(image)); err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	var x int
	if err := db.QueryRow("SELECT x FROM t").Scan(&x); err != nil {
		t.Fatal(err)
	}
	if x != 9 {
		t.Fatalf("x = %d, want 9", x)
	}
}

func TestLoadFromReaderSQLiteImage(t *testing.T) {
	src, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Exec("CREATE TABLE t(x); INSERT INTO t VALUES (3)"); err != nil {
		t.Fatal(err)
	}
	conn, err := src.sdb.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	blob, err := serializeConn(conn)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.LoadFrom(bytes.NewReader(blob)); err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	var x int
	if err := db.QueryRow("SELECT x FROM t").Scan(&x); err != nil {
		t.Fatal(err)
	}
	if x != 3 {
		t.Fatalf("x = %d, want 3", x)
	}
}

func TestLoadFromReaderUnsupported(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.LoadFrom(bytes.NewReader([]byte("not a recognizable format at all, long enough to matter")))
	if !errors.Is(err, ErrUnsupportedFile) {
		t.Fatalf("LoadFrom error = %v, want ErrUnsupportedFile", err)
	}
}

func TestLoadFromRejectsOversizedStream(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r := &infiniteReader{}
	err = db.LoadFrom(r)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("LoadFrom error = %v, want ErrTooLarge", err)
	}
}

type infiniteReader struct{}

func (r *infiniteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// TestLoadFromRejectsWALModeImage is the regression test for the
// asymmetry documented on LoadFrom and in the spec: unlike Load(path),
// LoadFrom cannot fall back to a "-wal" sidecar, so it must reject a
// WAL-mode SQLite image outright rather than silently restoring a
// possibly-stale database.
func TestLoadFromRejectsWALModeImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.sqlite")
	fdb, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fdb.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	if _, err := fdb.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}
	fdb.Close()

	image, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if image[sqliteWALFormatOffset] != sqliteWALFormatVersion {
		t.Fatalf("test setup: expected a WAL-mode header byte, got %d", image[sqliteWALFormatOffset])
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.LoadFrom(bytes.NewReader(image))
	if !errors.Is(err, ErrUnsupportedFile) {
		t.Fatalf("LoadFrom(WAL-mode image) error = %v, want an error wrapping ErrUnsupportedFile", err)
	}
}

func TestLoadAfterCloseReturnsErrClosed(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	path := filepath.Join(t.TempDir(), "whatever")
	if err := os.WriteFile(path, []byte("SQLite format 3\x00padding-to-be-long-enough"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = db.Load(path)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Load after Close error = %v, want ErrClosed", err)
	}
}

// TestLoadDoesNotChangeHasData is the regression test for the design
// decision that HasData/has_data (spec §7) describes how the process was
// launched, not its current content, so Load leaves it alone (spec §10).
func TestLoadDoesNotChangeHasData(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.HasData() {
		t.Fatal("a freshly created DB should report HasData() = false")
	}

	src, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snap")
	if err := src.Snapshot(path); err != nil {
		t.Fatal(err)
	}

	if err := db.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if db.HasData() {
		t.Fatal("HasData() = true after Load, want false (Load must not change it)")
	}
}

// TestOpenAcceptsSanDBoxExecutable confirms Open's Step 3 switch to the
// same auto-detection Load uses: a path holding a SanDBox executable
// (not just a plain SQLite file) is now a valid argument to Open.
func TestOpenAcceptsSanDBoxExecutable(t *testing.T) {
	src, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Exec("CREATE TABLE t(x); INSERT INTO t VALUES (5)"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snap")
	if err := src.Snapshot(path); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if !db.HasData() {
		t.Fatal("HasData() = false after opening a SanDBox executable with data")
	}
	var x int
	if err := db.QueryRow("SELECT x FROM t").Scan(&x); err != nil {
		t.Fatal(err)
	}
	if x != 5 {
		t.Fatalf("x = %d, want 5", x)
	}
}
