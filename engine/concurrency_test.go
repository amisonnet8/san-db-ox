package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// This file exists to be run under `make race` (CGO_ENABLED=1 go test
// -race -count=1 ./...), not `make check` (testing.md's "-race の運用方針"
// section). Every test here aims to give the Go race detector concurrent
// access to engine's own mutable state (DB.mu, Session.closed) alongside
// the SQL-level concurrency modernc.org/sqlite's busy_timeout already
// handles on its own -- a passing run here means no data race, not that
// every goroutine's SQL call necessarily succeeded (a busy_timeout
// failure under heavy concurrent load is an acceptable outcome; a data
// race is not).

// TestRaceConcurrentQueriesDuringLoad runs many goroutines issuing plain
// queries against db.Query while another goroutine calls Load, so the
// race detector can see db.mu (guarding db.closed/db.hasData) and
// db.sdb's own connection pool used concurrently from both paths.
func TestRaceConcurrentQueriesDuringLoad(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	src, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Exec("CREATE TABLE t(x); INSERT INTO t VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	loadPath := filepath.Join(t.TempDir(), "snap")
	if err := src.Snapshot(loadPath); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				rows, err := db.Query("SELECT * FROM sqlite_master")
				if err != nil {
					// A busy/locked error under concurrent Load is
					// acceptable; only a panic or data race is not.
					continue
				}
				for rows.Next() {
				}
				rows.Close()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Load(loadPath) // error ignored: outcome doesn't matter here, absence of a race does
	}()

	wg.Wait()
}

// TestRaceConcurrentSessions opens, uses and closes many Sessions
// concurrently, exercising DB.Session/DB.pooled alongside Session's own
// atomic.Bool-guarded closed flag.
func TestRaceConcurrentSessions(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sess, err := db.Session(context.Background())
			if err != nil {
				t.Errorf("Session #%d: %v", i, err)
				return
			}
			defer sess.Close()
			if _, err := sess.Exec("INSERT INTO t VALUES (?)", i); err != nil {
				t.Errorf("Session #%d Exec: %v", i, err)
			}
			var x int
			sess.QueryRow("SELECT ?", i).Scan(&x)
		}(i)
	}
	wg.Wait()
}

// TestRaceSessionCloseFromTwoGoroutines calls Close on the same Session
// from two goroutines simultaneously, so the race detector can confirm
// closed's atomic.Bool CompareAndSwap (session.go) actually serializes
// them rather than merely happening to avoid a race in practice.
func TestRaceSessionCloseFromTwoGoroutines(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sess, err := db.Session(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sess.Close()
		}()
	}
	wg.Wait()
}

// TestRaceCompleteFromManyGoroutines confirms Complete's per-call
// libc.TLS (complete.go) is actually safe under concurrent use -- the
// design deliberately avoids a single process-wide TLS shared across
// goroutines (libc.TLS itself is documented as not safe for that) by
// creating and closing one per call instead.
func TestRaceCompleteFromManyGoroutines(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sql := "SELECT 1"
			if i%2 == 0 {
				sql += ";"
			}
			if _, err := Complete(sql); err != nil {
				t.Errorf("Complete: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

// TestRaceExportWhileWriting runs Export concurrently with ordinary
// writes on the live database, exercising backupInto's connection
// alongside db.sdb's pool.
func TestRaceExportWhileWriting(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range 10 {
				db.Exec("INSERT INTO t VALUES (?)", i*100+j)
			}
		}(i)
	}

	dir := t.TempDir()
	for i := range 3 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := filepath.Join(dir, fmt.Sprintf("out%d.sqlite", i))
			db.Export(path) // error (e.g. ErrBusy) is an acceptable outcome
		}(i)
	}

	wg.Wait()
}

// TestRaceHasDataWhileLoad reads DB.HasData concurrently with Load
// (which never changes it, spec §7/§10 -- TestLoadDoesNotChangeHasData
// pins the value down; this test only cares that reading db.mu while
// Load runs is race-free).
func TestRaceHasDataWhileLoad(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	src, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}
	loadPath := filepath.Join(t.TempDir(), "snap")
	if err := src.Snapshot(loadPath); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				db.HasData()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		db.Load(loadPath)
	}()
	wg.Wait()
}
