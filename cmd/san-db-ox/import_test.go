package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCSV(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCmdImportCreatesTableFromHeader confirms a missing TABLE is
// created with all-TEXT columns named from the CSV's own header row
// (spec §3).
func TestCmdImportCreatesTableFromHeader(t *testing.T) {
	db := newTestDB(t)
	r, out, errw := newDumpTestRepl(t, db)

	path := writeCSV(t, t.TempDir(), "data.csv", "id,name\n1,alice\n2,bob\n")
	if err := r.cmdImport([]string{path, "people"}); err != nil {
		t.Fatalf("cmdImport: %v", err)
	}
	if errw.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errw.String())
	}
	if !strings.Contains(out.String(), "Inserted 2 rows") {
		t.Fatalf("cmdImport output = %q, want it to mention 2 inserted rows", out.String())
	}

	rows, err := db.Query("SELECT id, name FROM people ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		got = append(got, id+"|"+name)
	}
	want := []string{"1|alice", "2|bob"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("imported rows = %v, want %v", got, want)
	}

	// All-TEXT columns: declared type should be TEXT for every column
	// (spec §3), not inferred from the CSV's own content.
	var decltype string
	if err := db.QueryRow(`SELECT type FROM pragma_table_info('people') WHERE name = 'id'`).Scan(&decltype); err != nil {
		t.Fatal(err)
	}
	if decltype != "TEXT" {
		t.Fatalf("column 'id' decltype = %q, want TEXT", decltype)
	}
}

// TestCmdImportIntoExistingTable confirms an existing table's own
// column order is used and its first CSV row is treated as data, not a
// header (spec §3).
func TestCmdImportIntoExistingTable(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE people(id INTEGER, name TEXT)"); err != nil {
		t.Fatal(err)
	}
	r, out, errw := newDumpTestRepl(t, db)

	path := writeCSV(t, t.TempDir(), "data.csv", "1,alice\n2,bob\n")
	if err := r.cmdImport([]string{path, "people"}); err != nil {
		t.Fatalf("cmdImport: %v", err)
	}
	if errw.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errw.String())
	}
	if !strings.Contains(out.String(), "Inserted 2 rows") {
		t.Fatalf("cmdImport output = %q", out.String())
	}

	var cnt int
	if err := db.QueryRow("SELECT count(*) FROM people").Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 2 {
		t.Fatalf("row count = %d, want 2", cnt)
	}
}

// TestCmdImportFieldCountMismatchInsertsNothing confirms spec §3's
// intentional deviation from sqlite3: a field-count mismatch aborts the
// whole import (naming the offending row) rather than silently
// padding/truncating, and NO rows are inserted -- not even the ones
// before the bad row.
func TestCmdImportFieldCountMismatchInsertsNothing(t *testing.T) {
	db := newTestDB(t)
	r, _, _ := newDumpTestRepl(t, db)

	path := writeCSV(t, t.TempDir(), "data.csv", "id,name\n1,alice\n2\n3,carol\n")
	err := r.cmdImport([]string{path, "people"})
	if err == nil {
		t.Fatal("expected an error for a field-count mismatch")
	}
	if !strings.Contains(err.Error(), "row 3") {
		t.Fatalf("error should name the offending row (3, counting the header): %v", err)
	}

	var cnt int
	if err := db.QueryRow("SELECT count(*) FROM people").Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("row count = %d, want 0 (no partial import)", cnt)
	}
}

// TestCmdImportDoesNotErrorInsideAnExistingTransaction confirms .import's
// own SAVEPOINT (not a bare BEGIN/COMMIT) can run while the Session
// already has a user-started transaction open -- a bare BEGIN would fail
// with "cannot start a transaction within a transaction". A SAVEPOINT
// released inside an already-open outer transaction merges into it
// rather than committing independently (ordinary SQLite semantics, not
// something .import changes), so this only needs the outer transaction
// committed at the end to see everything land together.
func TestCmdImportDoesNotErrorInsideAnExistingTransaction(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE marker(x)"); err != nil {
		t.Fatal(err)
	}
	r, _, errw := newDumpTestRepl(t, db)

	if _, err := r.sess.Exec("BEGIN"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.sess.Exec("INSERT INTO marker VALUES (1)"); err != nil {
		t.Fatal(err)
	}

	path := writeCSV(t, t.TempDir(), "data.csv", "id,name\n1,alice\n")
	if err := r.cmdImport([]string{path, "people"}); err != nil {
		t.Fatalf("cmdImport: %v", err)
	}
	if errw.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errw.String())
	}
	if _, err := r.sess.Exec("COMMIT"); err != nil {
		t.Fatalf("user's own BEGIN should still be open (and commit cleanly) after .import: %v", err)
	}

	var cnt int
	if err := db.QueryRow("SELECT count(*) FROM marker").Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("marker row count = %d, want 1", cnt)
	}
	if err := db.QueryRow("SELECT count(*) FROM people").Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("people row count = %d, want 1", cnt)
	}
}

// TestCmdImportFailurePreservesEarlierWorkInTheSameTransaction confirms
// a failed .import rolls back only its own SAVEPOINT, not work the user
// already did earlier in the same still-open transaction.
func TestCmdImportFailurePreservesEarlierWorkInTheSameTransaction(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE marker(x)"); err != nil {
		t.Fatal(err)
	}
	r, _, _ := newDumpTestRepl(t, db)

	if _, err := r.sess.Exec("BEGIN"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.sess.Exec("INSERT INTO marker VALUES (1)"); err != nil {
		t.Fatal(err)
	}

	path := writeCSV(t, t.TempDir(), "data.csv", "id,name\n1,alice\n2\n")
	if err := r.cmdImport([]string{path, "people"}); err == nil {
		t.Fatal("expected a field-count-mismatch error")
	}
	if _, err := r.sess.Exec("COMMIT"); err != nil {
		t.Fatalf("user's own transaction should still be open (and commit cleanly) after a failed .import: %v", err)
	}

	var cnt int
	if err := db.QueryRow("SELECT count(*) FROM marker").Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("marker row count = %d, want 1 (earlier work in the same transaction survives a failed .import)", cnt)
	}
}

func TestCmdImportUsage(t *testing.T) {
	db := newTestDB(t)
	r, _, _ := newDumpTestRepl(t, db)
	if err := r.cmdImport([]string{"onlyonearg"}); err == nil {
		t.Fatal("expected a usage error for a missing TABLE argument")
	}
}

func TestCmdImportEmptyFileWithNoExistingTable(t *testing.T) {
	db := newTestDB(t)
	r, _, _ := newDumpTestRepl(t, db)
	path := writeCSV(t, t.TempDir(), "empty.csv", "")
	if err := r.cmdImport([]string{path, "people"}); err == nil {
		t.Fatal("expected an error: no header row to create the table from")
	}
}
