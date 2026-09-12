package main

import (
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/amisonnet8/san-db-ox/engine"
)

// startSnapshotInterval starts a background goroutine that calls
// db.Snapshot every opts.snapshotInterval (spec §12's
// --snapshot-interval, REPL/stdio modes only -- batch execution disables
// it, Phase 4 scope). It returns a stop func that must be called once
// the REPL/stdio session ends; a zero (or negative) interval makes both
// start and stop no-ops.
//
// stop blocks until the goroutine has actually exited, not just signaled
// it to -- a plain "close a channel and return" stop would let a
// db.Snapshot call already in flight (or one last tick racing the signal)
// finish after the caller believes saving has stopped. That race is
// observable: a test built around a temporary CWD that gets restored
// once the test function returns raced exactly this way during
// development, leaving a stray relative-path snapshot file behind in the
// package directory instead of the temp dir. Blocking here closes that
// window for every caller, not just tests.
//
// A save failure (e.g. ErrBusy from a write transaction held open on a
// Session) is reported to errw as a warning and the loop continues
// (spec §12: "1回の失敗でREPL/stdioセッション自体を止める理由がない")
// -- this is a "don't forget to save" safety net, not a guarantee every
// interval actually persists. The filename follows the same
// defaultSnapshotBase/snapshotFilename rule ".snapshot" itself uses
// (filename.go), so -o/-t apply identically to both.
func startSnapshotInterval(db *engine.DB, self string, opts *options, errw io.Writer) (stop func()) {
	if opts == nil || opts.snapshotInterval <= 0 {
		return func() {}
	}

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(opts.snapshotInterval)
		defer ticker.Stop()
		base := defaultSnapshotBase(self, opts)
		for {
			select {
			case <-ticker.C:
				path := snapshotFilename(base, opts.timestamp, false, time.Now(), runtime.GOOS)
				if err := db.Snapshot(path); err != nil {
					fmt.Fprintln(errw, "Warning: periodic snapshot failed:", err)
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}
