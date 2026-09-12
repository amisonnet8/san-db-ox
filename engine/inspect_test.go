package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeSQLiteFile serializes db's current content and writes it straight
// to path, the same "Serialize() output is a plain SQLite file" fact
// TestOpenFromSerializedFile (engine_test.go) exercises -- used here as a
// stand-in for Export (export.go, not yet implemented as of Step 2)
// wherever a test just needs *some* valid SQLite file on disk.
func writeSQLiteFile(t *testing.T, db *DB, path string) {
	t.Helper()
	conn, err := db.sdb.Conn(context.Background())
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer conn.Close()
	blob, err := serializeConn(conn)
	if err != nil {
		t.Fatalf("serializeConn: %v", err)
	}
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInspectSQLiteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.sqlite")

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}
	writeSQLiteFile(t, db, path)

	fi, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if fi.Kind != KindSQLite {
		t.Errorf("Kind = %v, want KindSQLite", fi.Kind)
	}
	if !fi.HasData {
		t.Errorf("HasData = false, want true")
	}
	if fi.DataOffset != 0 {
		t.Errorf("DataOffset = %d, want 0", fi.DataOffset)
	}
	if fi.DataLength != fi.Size {
		t.Errorf("DataLength = %d, want Size (%d)", fi.DataLength, fi.Size)
	}
	if fi.Version != 0 {
		t.Errorf("Version = %d, want 0 for KindSQLite", fi.Version)
	}
}

func TestInspectSanDBoxExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap")

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Snapshot(path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	fi, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if fi.Kind != KindExecutable {
		t.Errorf("Kind = %v, want KindExecutable", fi.Kind)
	}
	if !fi.HasData {
		t.Errorf("HasData = false, want true")
	}
	if fi.Version != FormatVersion {
		t.Errorf("Version = %d, want %d", fi.Version, FormatVersion)
	}
	if fi.DataOffset+fi.DataLength+FooterSize != fi.Size {
		t.Errorf("DataOffset(%d)+DataLength(%d)+FooterSize != Size(%d)", fi.DataOffset, fi.DataLength, fi.Size)
	}
}

func TestInspectUnknownFile(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
	}{
		{"empty", nil},
		{"short", []byte("hi")},
		{"garbage-under-footer-size", make([]byte, 10)},
		{"garbage-large", []byte("this is definitely not a recognizable file format, just plain text padding to exceed the footer size threshold comfortably")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f")
			if err := os.WriteFile(path, c.content, 0o644); err != nil {
				t.Fatal(err)
			}
			fi, err := Inspect(path)
			if err != nil {
				t.Fatalf("Inspect: unexpected error: %v", err)
			}
			if fi.Kind != KindUnknown {
				t.Errorf("Kind = %v, want KindUnknown", fi.Kind)
			}
		})
	}
}

func TestInspectCorruptFooterIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt")

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Snapshot(path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Corrupt the DataLength field (bytes 20:28 of the trailing footer)
	// so the footer's own internal consistency check fails.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte{0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f}, stat.Size()-FooterSize+20); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if _, err := Inspect(path); err == nil {
		t.Fatal("Inspect: expected an error for a corrupt footer, got nil")
	}
}

func TestInspectPrefersSQLiteHeaderOverFooter(t *testing.T) {
	// A file that starts with the SQLite header AND happens to carry a
	// valid-looking SanDBox footer at the end must still be reported as
	// KindSQLite (spec §6's detection order: header first).
	dir := t.TempDir()
	path := filepath.Join(dir, "hybrid")

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}
	writeSQLiteFile(t, db, path)

	// Append a syntactically valid footer whose engine-prefix length
	// covers the whole SQLite file, so the footer itself is internally
	// consistent -- Inspect must not even get that far, since the header
	// check wins first.
	blob := []byte("fake-data-blob")
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(blob); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(encodeFooter(stat.Size(), int64(len(blob)))); err != nil {
		t.Fatal(err)
	}
	f.Close()

	fi, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if fi.Kind != KindSQLite {
		t.Errorf("Kind = %v, want KindSQLite (header must win over a trailing footer)", fi.Kind)
	}
}

func TestInspectMissingFileReturnsOSError(t *testing.T) {
	_, err := Inspect(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("Inspect: expected an OS error for a missing file, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Inspect error = %v, want an os.IsNotExist error", err)
	}
}

func TestInspectDirectoryReturnsOSError(t *testing.T) {
	_, err := Inspect(t.TempDir())
	if err == nil {
		t.Fatal("Inspect: expected an OS error for a directory, got nil")
	}
}

func TestFileKindString(t *testing.T) {
	cases := []struct {
		k    FileKind
		want string
	}{
		{KindUnknown, "unknown"},
		{KindSQLite, "sqlite"},
		{KindExecutable, "executable"},
		{FileKind(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("FileKind(%d).String() = %q, want %q", c.k, got, c.want)
		}
	}
}
