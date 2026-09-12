package main

import (
	"bytes"
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
		if *short != *long {
			t.Errorf("short form %v = %+v, long form %v = %+v; want equal", c.short, *short, c.long, *long)
		}
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
