package main

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amisonnet8/san-db-ox/engine"
)

// TestReplInterruptsFirstPressCancelsRunningStatement confirms a Ctrl+C
// while a statement is executing (begin's cancel registered) cancels it
// rather than exiting the process.
func TestReplInterruptsFirstPressCancelsRunningStatement(t *testing.T) {
	ri := &replInterrupts{idleSig: make(chan struct{}, 1)}
	exited := false
	ri.exitFunc = func(int) { exited = true }

	canceled := false
	end := ri.begin(func() { canceled = true })
	defer end()

	ri.onInterrupt()
	if !canceled {
		t.Fatal("first interrupt while a statement is running should cancel it")
	}
	if exited {
		t.Fatal("first interrupt should not exit the process")
	}
}

// TestReplInterruptsIdlePressSignalsIdleChannel confirms a Ctrl+C while
// idle (no cancel registered) notifies idleSig instead of exiting.
func TestReplInterruptsIdlePressSignalsIdleChannel(t *testing.T) {
	ri := &replInterrupts{idleSig: make(chan struct{}, 1)}
	ri.exitFunc = func(int) { t.Fatal("should not exit on a single idle interrupt") }

	ri.onInterrupt()
	select {
	case <-ri.idleSig:
	default:
		t.Fatal("expected an idle interrupt notification")
	}
}

// TestReplInterruptsSecondConsecutivePressExits confirms two Ctrl+C
// presses in a row, with no input line read in between, force-quit
// (spec §13).
func TestReplInterruptsSecondConsecutivePressExits(t *testing.T) {
	ri := &replInterrupts{idleSig: make(chan struct{}, 1)}
	var gotCode int
	exited := false
	ri.exitFunc = func(code int) { exited = true; gotCode = code }

	ri.onInterrupt() // 1st: idle signal only
	ri.onInterrupt() // 2nd consecutive: exit
	if !exited || gotCode != 1 {
		t.Fatalf("exited=%v code=%d, want exited=true code=1", exited, gotCode)
	}
}

// TestReplInterruptsResetOnNewLinePreventsForceQuit confirms reading a
// new input line between two Ctrl+C presses resets the consecutive-press
// counter, so the second press is treated as a fresh first press (spec
// §13: "新しい入力行を1行でも読むと連続回数はリセットされる").
func TestReplInterruptsResetOnNewLinePreventsForceQuit(t *testing.T) {
	ri := &replInterrupts{idleSig: make(chan struct{}, 1)}
	exited := false
	ri.exitFunc = func(int) { exited = true }

	ri.onInterrupt()
	ri.resetOnNewLine()
	<-ri.idleSig // drain, as the REPL loop would after acting on it
	ri.onInterrupt()
	if exited {
		t.Fatal("a new line read between two interrupts should reset the consecutive-press counter")
	}
}

// TestReplInterruptsIdleSignalNonBlockingWhenChannelFull confirms
// onInterrupt never blocks even if idleSig's one buffered slot is still
// full from an earlier, undrained notification.
func TestReplInterruptsIdleSignalNonBlockingWhenChannelFull(t *testing.T) {
	ri := &replInterrupts{idleSig: make(chan struct{}, 1)}
	ri.exitFunc = func(int) {}

	done := make(chan struct{})
	go func() {
		ri.onInterrupt()
		ri.resetOnNewLine()
		ri.onInterrupt() // idleSig's slot is still full (never drained) -- must not block
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("onInterrupt blocked when idleSig's buffered slot was already full")
	}
}

func TestStartLineReaderDeliversLinesThenNilErrorOnEOF(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("a\nb\n"))
	lr := startLineReader(scanner)

	var got []string
	for l := range lr.lines {
		got = append(got, l)
	}
	if err := <-lr.err; err != nil {
		t.Fatalf("unexpected scanner error: %v", err)
	}
	want := []string{"a", "b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("lines = %v, want %v", got, want)
	}
}

// TestExecSQLCancelsOnInterrupt bridges replInterrupts to repl.execSQL:
// an interrupt delivered while a statement is running cancels it via
// r.interrupts.begin's registered cancel func, and the Session remains
// usable for the next statement afterward (engine's
// TestSessionSurvivesCanceledQuery covers the Session half of this;
// this covers the REPL wiring on top of it).
func TestExecSQLCancelsOnInterrupt(t *testing.T) {
	db, err := engine.Open(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var out, errw bytes.Buffer
	r := newTestReplWithSession(t, db, &out, &errw, true)
	r.interrupts = &replInterrupts{idleSig: make(chan struct{}, 1), exitFunc: func(int) {}}

	go func() {
		time.Sleep(30 * time.Millisecond)
		r.interrupts.onInterrupt()
	}()

	const longQuery = `WITH RECURSIVE cnt(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM cnt WHERE x < 500000000) SELECT count(*) FROM cnt;`
	r.execSQL(longQuery)
	if !strings.Contains(errw.String(), "context canceled") {
		t.Fatalf("expected a cancellation error, got %q", errw.String())
	}

	errw.Reset()
	r.execSQL("SELECT 1;")
	if errw.Len() != 0 {
		t.Fatalf("Session should still work after a canceled query, got stderr %q", errw.String())
	}
	if got, want := out.String(), "1\n"; got != want {
		t.Fatalf("SELECT 1 after cancellation = %q, want %q", got, want)
	}
}
