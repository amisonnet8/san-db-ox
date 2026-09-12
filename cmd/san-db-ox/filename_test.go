package main

import (
	"testing"
	"time"
)

func TestSnapshotFilename(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name                    string
		base                    string
		withTimestamp, asSQLite bool
		goos                    string
		want                    string
	}{
		{"no extension, linux", "mydb", false, false, "linux", "mydb"},
		{"no extension, windows gets .exe", "mydb", false, false, "windows", "mydb.exe"},
		{"existing .exe extension left as-is", "mydb.exe", false, false, "windows", "mydb.exe"},
		{"existing extension left as-is even on windows", "mydb.sqlite", false, false, "windows", "mydb.sqlite"},
		{"hyphenated base name (naming.md)", "san-db-ox", false, false, "windows", "san-db-ox.exe"},
		{"no extension, darwin, no .exe", "mydb", false, false, "darwin", "mydb"},
		{"no extension, --sqlite gets .sqlite on every OS", "mydb", false, true, "linux", "mydb.sqlite"},
		{"no extension, --sqlite gets .sqlite even on windows (no .exe)", "mydb", false, true, "windows", "mydb.sqlite"},
		{"timestamp appended before the extension", "mydb.exe", true, false, "windows", "mydb_20260901120000.exe"},
		{"timestamp with no extension, windows", "mydb", true, false, "windows", "mydb_20260901120000.exe"},
		{"timestamp with --sqlite", "mydb", true, true, "linux", "mydb_20260901120000.sqlite"},
		{"default base (bare binary name), no extension", "san-db-ox", true, false, "linux", "san-db-ox_20260901120000"},
		{"existing timestamp suffix is replaced, not duplicated", "mydb_20260101120000", true, false, "linux", "mydb_20260901120000"},
		{"existing timestamp suffix replaced, with extension", "mydb_20260101120000.exe", true, false, "windows", "mydb_20260901120000.exe"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := snapshotFilename(c.base, c.withTimestamp, c.asSQLite, now, c.goos)
			if got != c.want {
				t.Errorf("snapshotFilename(%q, %v, %v, _, %q) = %q, want %q", c.base, c.withTimestamp, c.asSQLite, c.goos, got, c.want)
			}
		})
	}
}

func TestDefaultSnapshotBase(t *testing.T) {
	if got := defaultSnapshotBase("/path/to/san-db-ox", nil); got != "san-db-ox" {
		t.Errorf("defaultSnapshotBase with nil opts = %q, want %q", got, "san-db-ox")
	}
	if got := defaultSnapshotBase("/path/to/san-db-ox", &options{}); got != "san-db-ox" {
		t.Errorf("defaultSnapshotBase with no -o = %q, want %q", got, "san-db-ox")
	}
	if got := defaultSnapshotBase("/path/to/san-db-ox", &options{snapshotAs: "mydb"}); got != "mydb" {
		t.Errorf("defaultSnapshotBase with -o mydb = %q, want %q", got, "mydb")
	}
}
