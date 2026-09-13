package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/amisonnet8/san-db-ox/engine"
)

// periodicSnapshotName mirrors what startSnapshotInterval actually writes
// for the extension-less base "periodic": snapshotFilename appends ".exe"
// on windows (naming.md), so a bare "periodic" never appears there.
func periodicSnapshotName() string {
	return snapshotFilename("periodic", false, false, time.Time{}, runtime.GOOS)
}

func TestStartSnapshotIntervalZeroIsNoop(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var errw bytes.Buffer
	stop := startSnapshotInterval(db, "self", &options{snapshotInterval: 0}, &errw)
	stop() // must not panic/hang
}

func TestStartSnapshotIntervalNilOptsIsNoop(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var errw bytes.Buffer
	stop := startSnapshotInterval(db, "self", nil, &errw)
	stop()
}

// TestStartSnapshotIntervalSavesPeriodically confirms the background
// goroutine actually calls db.Snapshot on the configured interval, using
// -o/--snapshot-as (opts.snapshotAs) as the fixed target filename (spec
// §12: no --timestamp means "same name every time").
func TestStartSnapshotIntervalSavesPeriodically(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "self")
	if err := os.WriteFile(self, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	db, err := engine.Open(filepath.Join(dir, "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	var errw bytes.Buffer
	opts := &options{snapshotAs: "periodic", snapshotInterval: 20 * time.Millisecond}
	stop := startSnapshotInterval(db, self, opts, &errw)
	defer stop()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, periodicSnapshotName())); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("periodic snapshot never appeared (stderr: %q)", errw.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestStartSnapshotIntervalStopBlocksUntilGoroutineExits guards the race
// startSnapshotInterval's doc comment describes: stop() must block until
// its goroutine has actually exited, not merely signal it to, or a
// db.Snapshot call already in flight (or one last tick racing the
// signal) can still write after the caller believes saving has stopped.
func TestStartSnapshotIntervalStopBlocksUntilGoroutineExits(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	self := filepath.Join(dir1, "self")
	if err := os.WriteFile(self, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir1)

	db, err := engine.Open(filepath.Join(dir1, "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	var errw bytes.Buffer
	opts := &options{snapshotAs: "periodic", snapshotInterval: 5 * time.Millisecond}
	stop := startSnapshotInterval(db, self, opts, &errw)

	time.Sleep(30 * time.Millisecond) // let at least one tick fire
	stop()                            // must block until the goroutine has fully exited

	// Once stop() has returned, changing the CWD elsewhere and waiting
	// past several more ticker intervals must not touch dir1/periodic
	// again -- a leftover in-flight write racing stop() would either
	// modify it (if it happened to land before the chdir completed) or,
	// in the original bug, land in whatever CWD is current by then.
	if err := os.Chdir(dir2); err != nil {
		t.Fatal(err)
	}
	info, statErr := os.Stat(filepath.Join(dir1, periodicSnapshotName()))
	if statErr != nil {
		t.Fatalf("expected at least one periodic snapshot to have been written before stop(): %v", statErr)
	}
	mtimeAtStop := info.ModTime()

	time.Sleep(50 * time.Millisecond)
	info2, err := os.Stat(filepath.Join(dir1, periodicSnapshotName()))
	if err != nil {
		t.Fatalf("periodic snapshot disappeared: %v", err)
	}
	if !info2.ModTime().Equal(mtimeAtStop) {
		t.Fatalf("periodic snapshot was modified after stop() returned (race)")
	}
}
