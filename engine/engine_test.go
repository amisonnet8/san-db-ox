package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func tableCount(t *testing.T, db *DB) int {
	t.Helper()
	var cnt int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&cnt); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	return cnt
}

func TestOpenMissingFileIsEmpty(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "does-not-exist.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if cnt := tableCount(t, db); cnt != 0 {
		t.Fatalf("table count = %d, want 0 for a missing-file Open", cnt)
	}
	if _, err := db.Exec("CREATE TABLE t (v TEXT)"); err != nil {
		t.Fatalf("expected the empty DB to be writable: %v", err)
	}
}

func TestOpenEmptyFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.sqlite")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if cnt := tableCount(t, db); cnt != 0 {
		t.Fatalf("table count = %d, want 0 for a zero-byte file", cnt)
	}
}

func TestOpenRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garbage.sqlite")
	if err := os.WriteFile(path, []byte("not a sqlite database"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Open(path)
	if err == nil {
		t.Fatalf("expected an error opening a file that is not a valid SQLite database")
	}
}

// TestOpenFromSerializedFile confirms spec §6's "独自のデータファイル形式は
// 存在しない" claim end to end: bytes written straight from Serialize()
// are, unmodified, a file Open can read back (this is exactly what
// ".snapshot --sqlite" plus ".load" will do once implemented in a later
// phase; Phase 1 only needs Open itself to round-trip).
func TestOpenFromSerializedFile(t *testing.T) {
	src, err := newDB()
	if err != nil {
		t.Fatalf("newDB: %v", err)
	}
	defer src.Close()

	if _, err := src.Exec("CREATE TABLE t (v TEXT); INSERT INTO t VALUES ('hello')"); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	conn, err := src.sdb.Conn(context.Background())
	if err != nil {
		t.Fatalf("src conn: %v", err)
	}
	blob, err := serializeConn(conn)
	conn.Close()
	if err != nil {
		t.Fatalf("serializeConn: %v", err)
	}

	path := filepath.Join(t.TempDir(), "exported.sqlite")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(exported): %v", err)
	}
	defer db.Close()

	var v string
	if err := db.QueryRow("SELECT v FROM t").Scan(&v); err != nil {
		t.Fatalf("query after Open: %v", err)
	}
	if v != "hello" {
		t.Fatalf("v = %q, want %q", v, "hello")
	}

	// The restored DB must stay writable (SQLITE_DESERIALIZE_RESIZEABLE),
	// not just readable.
	if _, err := db.Exec("INSERT INTO t VALUES ('world')"); err != nil {
		t.Fatalf("expected the opened DB to remain writable: %v", err)
	}
}

// TestOpenSelfNoEmbeddedData relies on the go test binary itself having
// no SanDBox footer: OpenSelf must fall back to an empty database rather
// than erroring.
func TestOpenSelfNoEmbeddedData(t *testing.T) {
	db, err := OpenSelf()
	if err != nil {
		t.Fatalf("OpenSelf: %v", err)
	}
	defer db.Close()
	if cnt := tableCount(t, db); cnt != 0 {
		t.Fatalf("table count = %d, want 0 (go test binary has no embedded data)", cnt)
	}
}

func TestCloseIsIdempotentAndBlocksFurtherUse(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("second Close should be a no-op, got: %v", err)
	}
	if _, err := db.Exec("SELECT 1"); err != ErrClosed {
		t.Fatalf("Exec after Close = %v, want ErrClosed", err)
	}
	if _, err := db.Query("SELECT 1"); err != ErrClosed {
		t.Fatalf("Query after Close = %v, want ErrClosed", err)
	}
}

func TestMultipleDBsDoNotCollide(t *testing.T) {
	a, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if _, err := a.Exec("CREATE TABLE t (v TEXT); INSERT INTO t VALUES ('a')"); err != nil {
		t.Fatal(err)
	}
	if cnt := tableCount(t, b); cnt != 0 {
		t.Fatalf("DB b sees %d tables from DB a; store names must collide", cnt)
	}
}
