package main

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

func TestParseOptionsDefaults(t *testing.T) {
	var errw bytes.Buffer
	opts, err := parseOptions(nil, &errw)
	if err != nil {
		t.Fatalf("parseOptions(nil): %v", err)
	}
	if opts.mode != modeList {
		t.Errorf("default mode = %q, want %q", opts.mode, modeList)
	}
	if opts.snapshotAs != "" || opts.quiet || opts.timestamp || opts.snapshotInterval != 0 || opts.help || opts.version {
		t.Errorf("unexpected non-zero defaults: %+v", opts)
	}
	if len(opts.command) != 0 || opts.serveStdio || opts.readOnly {
		t.Errorf("unexpected non-zero Phase 4 defaults: %+v", opts)
	}
}

func TestParseOptionsShortAndLongFormsAgree(t *testing.T) {
	cases := []struct {
		short, long []string
	}{
		{[]string{"-m", "json"}, []string{"--mode", "json"}},
		{[]string{"-o", "mydb"}, []string{"--snapshot-as", "mydb"}},
		{[]string{"-q"}, []string{"--quiet"}},
		{[]string{"-t"}, []string{"--timestamp"}},
		{[]string{"-i", "5m"}, []string{"--snapshot-interval", "5m"}},
		{[]string{"-c", "SELECT 1"}, []string{"--command", "SELECT 1"}},
		{[]string{"-r"}, []string{"--read-only"}},
	}
	for _, c := range cases {
		var errw bytes.Buffer
		short, err := parseOptions(c.short, &errw)
		if err != nil {
			t.Fatalf("parseOptions(%v): %v", c.short, err)
		}
		long, err := parseOptions(c.long, &errw)
		if err != nil {
			t.Fatalf("parseOptions(%v): %v", c.long, err)
		}
		// options now holds a slice field (command), so it is no longer
		// comparable with != -- reflect.DeepEqual instead.
		if !reflect.DeepEqual(short, long) {
			t.Errorf("short form %v = %+v, long form %v = %+v; want equal", c.short, *short, c.long, *long)
		}
	}
}

// TestParseOptionsCommandRepeatable confirms -c/--command accumulates in
// the order given (spec §5: "複数回指定でき、指定順に実行"), including
// when the two spellings are mixed in one invocation.
func TestParseOptionsCommandRepeatable(t *testing.T) {
	var errw bytes.Buffer
	opts, err := parseOptions([]string{"-c", "CREATE TABLE t(x)", "--command", "INSERT INTO t VALUES(1)", "-c", ".tables"}, &errw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"CREATE TABLE t(x)", "INSERT INTO t VALUES(1)", ".tables"}
	if !reflect.DeepEqual([]string(opts.command), want) {
		t.Errorf("command = %v, want %v", opts.command, want)
	}
}

// TestParseOptionsServeStdioAndCommandConflict / ...ReadOnlyAndIntervalConflict
// confirm spec §12's exclusivity table is enforced as a usage error.
func TestParseOptionsServeStdioAndCommandConflict(t *testing.T) {
	var errw bytes.Buffer
	if _, err := parseOptions([]string{"--serve-stdio", "-c", "SELECT 1"}, &errw); err == nil {
		t.Fatal("expected an error combining --serve-stdio and -c")
	}
}

func TestParseOptionsReadOnlyAndIntervalConflict(t *testing.T) {
	var errw bytes.Buffer
	if _, err := parseOptions([]string{"-r", "-i", "5m"}, &errw); err == nil {
		t.Fatal("expected an error combining --read-only and --snapshot-interval")
	}
}

func TestParseOptionsSnapshotInterval(t *testing.T) {
	var errw bytes.Buffer
	opts, err := parseOptions([]string{"-i", "5m"}, &errw)
	if err != nil {
		t.Fatal(err)
	}
	if opts.snapshotInterval != 5*time.Minute {
		t.Errorf("snapshotInterval = %v, want 5m", opts.snapshotInterval)
	}
}

func TestParseOptionsRejectsUnknownMode(t *testing.T) {
	var errw bytes.Buffer
	if _, err := parseOptions([]string{"-m", "xml"}, &errw); err == nil {
		t.Fatal("expected an error for an unknown -m value")
	}
}

func TestParseOptionsRejectsUnknownFlag(t *testing.T) {
	var errw bytes.Buffer
	if _, err := parseOptions([]string{"--nope"}, &errw); err == nil {
		t.Fatal("expected an error for an unrecognized flag")
	}
}

func TestParseOptionsRejectsPositionalArgs(t *testing.T) {
	var errw bytes.Buffer
	if _, err := parseOptions([]string{"extra"}, &errw); err == nil {
		t.Fatal("expected an error for an unexpected positional argument")
	}
}

func TestParseOptionsHelpAndVersion(t *testing.T) {
	var errw bytes.Buffer
	opts, err := parseOptions([]string{"-h"}, &errw)
	if err != nil || !opts.help {
		t.Fatalf("parseOptions([-h]) = %+v, %v; want help=true, nil error", opts, err)
	}

	opts, err = parseOptions([]string{"--version"}, &errw)
	if err != nil || !opts.version {
		t.Fatalf("parseOptions([--version]) = %+v, %v; want version=true, nil error", opts, err)
	}
}
