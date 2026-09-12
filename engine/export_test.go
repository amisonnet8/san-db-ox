package engine

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExportRoundTrip(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x); INSERT INTO t VALUES (1), (2)"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.sqlite")
	if err := db.Export(path); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Open with a completely independent connection -- no memdb, no
	// fileDSN -- to confirm the result is a plain SQLite file.
	fdb, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fdb.Close()
	var cnt int
	if err := fdb.QueryRow("SELECT count(*) FROM t").Scan(&cnt); err != nil {
		t.Fatalf("query exported file: %v", err)
	}
	if cnt != 2 {
		t.Fatalf("count = %d, want 2", cnt)
	}
}

func TestExportProducesSQLiteHeader(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.sqlite")
	if err := db.Export(path); err != nil {
		t.Fatalf("Export: %v", err)
	}

	fi, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if fi.Kind != KindSQLite {
		t.Errorf("Kind = %v, want KindSQLite", fi.Kind)
	}
}

func TestExportOverwritesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.sqlite")
	if err := os.WriteFile(path, []byte("pre-existing content, not a database"), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	if err := db.Export(path); err != nil {
		t.Fatalf("Export: %v", err)
	}

	fi, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if fi.Kind != KindSQLite {
		t.Errorf("Kind = %v, want KindSQLite after overwriting a pre-existing file", fi.Kind)
	}
}

func TestExportIsAtomicNoPartialFileOnFailure(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	// A directory component that does not exist: tempFileFor fails
	// before anything is written to the target path.
	path := filepath.Join(t.TempDir(), "nonexistent-dir", "out.sqlite")
	if err := db.Export(path); err == nil {
		t.Fatal("Export into a nonexistent directory: expected an error, got nil")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Export left something at path after failing: stat err = %v", err)
	}
}

func TestExportLeavesNoTempArtifacts(t *testing.T) {
	check := func(t *testing.T, dir string) {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name() != "out.sqlite" {
				t.Errorf("leftover file in export directory: %s", e.Name())
			}
		}
	}

	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		db, err := newDB()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
			t.Fatal(err)
		}
		if err := db.Export(filepath.Join(dir, "out.sqlite")); err != nil {
			t.Fatalf("Export: %v", err)
		}
		check(t, dir)
	})

	t.Run("failure", func(t *testing.T) {
		dir := t.TempDir()
		db, err := newDB()
		if err != nil {
			t.Fatal(err)
		}
		db.Close() // Export will fail with ErrClosed before writing anything
		_ = db.Export(filepath.Join(dir, "out.sqlite"))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("leftover files after a failed Export: %v", entries)
		}
	})
}

func TestExportPermissionsAre0644(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits to assert on (testing.md)")
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.sqlite")
	if err := db.Export(path); err != nil {
		t.Fatalf("Export: %v", err)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != exportPerm {
		t.Errorf("permissions = %o, want %o", perm, exportPerm)
	}
}

func TestExportEmptyDatabase(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	path := filepath.Join(t.TempDir(), "empty.sqlite")
	if err := db.Export(path); err != nil {
		t.Fatalf("Export: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open exported empty database: %v", err)
	}
	defer reopened.Close()
	if cnt := tableCount(t, reopened); cnt != 0 {
		t.Fatalf("table count = %d, want 0", cnt)
	}
}

func TestExportAfterCloseReturnsErrClosed(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	err = db.Export(filepath.Join(t.TempDir(), "out.sqlite"))
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Export after Close error = %v, want ErrClosed", err)
	}
}

func TestExportThenLoadRoundTrip(t *testing.T) {
	src, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Exec("CREATE TABLE t(x); INSERT INTO t VALUES (99)"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.sqlite")
	if err := src.Export(path); err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if err := dst.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	var x int
	if err := dst.QueryRow("SELECT x FROM t").Scan(&x); err != nil {
		t.Fatal(err)
	}
	if x != 99 {
		t.Fatalf("x = %d, want 99", x)
	}
}
