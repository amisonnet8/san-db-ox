package main

import "testing"

func TestSnapshotFilename(t *testing.T) {
	cases := []struct {
		base, goos, want string
	}{
		{"mydb", "linux", "mydb"},
		{"mydb", "windows", "mydb.exe"},
		{"mydb.exe", "windows", "mydb.exe"},
		{"mydb.sqlite", "windows", "mydb.sqlite"}, // has an extension already; left as-is
		{"san-db-ox", "windows", "san-db-ox.exe"}, // hyphenated base name, naming.md
		{"mydb", "darwin", "mydb"},
	}
	for _, c := range cases {
		if got := snapshotFilename(c.base, c.goos); got != c.want {
			t.Errorf("snapshotFilename(%q, %q) = %q, want %q", c.base, c.goos, got, c.want)
		}
	}
}
