package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// exportPerm is the permission Export writes its output file with.
// Unlike writeImageAtomic's 0755 (Snapshot's output is meant to be run
// directly, spec §1/§9), a plain SQLite file is not executable, so this
// stays at the more conservative 0644 (spec §6). os.CreateTemp creates
// its file at 0600, so Export must Chmod explicitly -- otherwise the
// exported file would be unreadable by anyone but its owner, unlike
// every other file engine writes.
const exportPerm = 0o644

// Export writes db's current state to path as a plain SQLite database
// file (spec §6, §10; REPL ".snapshot --sqlite"). Unlike Snapshot, the
// result is data only -- no engine bytes, no footer -- and is readable by
// any SQLite client (DBeaver, sqlite3, pandas, etc, spec §6).
//
// The copy runs through SQLite's online Backup API (backupInto,
// backup.go) rather than Serialize(), because a backup is consistent
// with respect to a concurrent write transaction on another connection
// (spec §6, §11) -- Export can therefore return an error wrapping
// ErrBusy if such a transaction (e.g. one held open on a Session) is
// still open once the live database's busy_timeout elapses.
//
// The write is atomic: Export writes through a temporary file in path's
// same directory (so the final os.Rename is a same-filesystem move) and
// only replaces path once the copy has fully succeeded, so a reader
// never observes a half-written file (spec §6, §11, matching Snapshot's
// own write discipline in persist.go). Any sidecar files SQLite created
// alongside the temporary file while writing to it (-journal, -wal,
// -shm) are cleaned up whether Export succeeds or fails.
//
// An existing file at path is overwritten (spec §6). Export does not
// generate a file name (".sqlite" completion, timestamps): that is
// cmd/san-db-ox's responsibility (naming.md); path is used as-is. Unlike
// Load (load.go), path itself never becomes part of a DSN here -- only a
// freshly generated temporary path does (fileDSN's "no '?'" restriction,
// persist.go, therefore does not apply to path).
func (db *DB) Export(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}

	sdb, err := db.pooled()
	if err != nil {
		return err
	}
	conn, err := sdb.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()

	tmp, err := tempFileFor(abs)
	if err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	// Close the handle immediately rather than holding it open for the
	// backup below: SQLite opens tmpPath itself through its own VFS, and
	// on Windows a second, exclusive-minded open of a file this process
	// already has open can fail. A zero-byte file is what SQLite expects
	// to find and open as a fresh, empty database.
	if err := tmp.Close(); err != nil {
		removeTempArtifacts(tmpPath)
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	defer removeTempArtifacts(tmpPath) // no-op once the rename below succeeds

	tmpDSN, err := fileDSN(tmpPath)
	if err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	if err := backupInto(conn, tmpDSN); err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}

	if err := os.Chmod(tmpPath, exportPerm); err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, abs); err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	return nil
}
