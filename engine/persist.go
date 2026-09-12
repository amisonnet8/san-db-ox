package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// oldSuffix is the suffix a running executable is renamed to during
// Overwrite, freeing up the original path for a fresh write (spec §11).
const oldSuffix = ".san-db-ox.old"

// Snapshot writes the current DB state to path: the running process's
// own executable bytes, followed by a fresh data blob and footer (spec
// §4, §10, §12; REPL ".snapshot", CLI "--snapshot-as"). This is
// independent of whether the DB was opened via Open or OpenSelf (spec
// §10: "この区別はOpenで開いたかOpenSelfで開いたかに依存しない") --
// Snapshot always replicates whatever binary is currently running,
// because "ここでの「エンジンバイト」はホストアプリのバイナリ全体" (§10).
// The write is atomic: a temporary file in path's directory, then
// renamed into place (spec §11).
//
// Snapshot does not generate a file name (timestamps, ".exe" completion,
// §12): that is cmd/san-db-ox's responsibility (naming.md); path is used
// as-is.
func (db *DB) Snapshot(path string) error {
	engineBytes, data, err := db.image()
	if err != nil {
		return err
	}
	return writeImageAtomic(path, engineBytes, data)
}

// image re-reads the running executable's engine prefix and takes a
// consistent snapshot of the live database. Both Snapshot and Overwrite
// build their output this way.
func (db *DB) image() (engineBytes, data []byte, err error) {
	self, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("engine: os.Executable: %w", err)
	}

	data, err = db.serializeBarrier()
	if err != nil {
		return nil, nil, err
	}

	engineBytes, err = readEnginePrefix(self)
	if err != nil {
		return nil, nil, err
	}
	return engineBytes, data, nil
}

// serializeBarrier takes a consistent snapshot of the live database.
// Serialize() itself is not transaction-aware -- called bare, it can
// return a torn snapshot that includes another connection's uncommitted
// write -- so this wraps it in a BEGIN IMMEDIATE barrier on a dedicated
// connection. Starting that transaction blocks (up to the live
// database's busy_timeout) until no other connection holds a conflicting
// write lock, so once it succeeds, Serialize() cannot observe a write in
// progress. The barrier transaction itself never writes anything; it
// only exists to hold the lock, so it always ends in ROLLBACK.
func (db *DB) serializeBarrier() ([]byte, error) {
	sdb, err := db.pooled()
	if err != nil {
		return nil, err
	}

	conn, err := sdb.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBusy, err)
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")

	blob, err := serializeConn(conn)
	if err != nil {
		return nil, err
	}
	if len(blob) == 0 {
		// modernc.org/sqlite's Serialize can return (nil, nil) rather
		// than an error when malloc fails for a very large database --
		// treat an empty result as a failure rather than silently
		// writing a zero-length data blob.
		return nil, ErrTooLarge
	}
	return blob, nil
}

// readEnginePrefix re-reads path (the running process's own executable)
// just before a write, rather than keeping engine bytes resident for the
// DB's whole lifetime: a modernc.org/sqlite binary runs 10-15MB, and a
// DB stays open far longer than a single Snapshot/Overwrite call (spec
// §11). The engine prefix is path's whole content if it has no footer
// yet, or everything before the existing footer's data blob if it does.
func readEnginePrefix(path string) ([]byte, error) {
	fi, size, err := readFooter(path)
	if err != nil {
		return nil, err
	}
	engineLen := size
	if fi.hasData {
		engineLen = fi.dataOffset
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, engineLen)
	if _, err := f.ReadAt(buf, 0); err != nil {
		return nil, err
	}
	return buf, nil
}

// writeImageAtomic writes engineBytes+data+footer to path via a
// temporary file in the same directory followed by a rename, so a reader
// never observes a half-written file (spec §11). The output is always
// executable (0755): Snapshot's output is meant to be run directly
// (spec §1, §9).
func writeImageAtomic(path string, engineBytes, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".san-db-ox_tmp_*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if err := writeImage(tmp, engineBytes, data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func writeImage(w interface{ Write([]byte) (int, error) }, engineBytes, data []byte) error {
	if _, err := w.Write(engineBytes); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err := w.Write(encodeFooter(int64(len(engineBytes)), int64(len(data))))
	return err
}

// Overwrite replaces the running process's own executable
// (os.Executable()) in place with its own engine bytes followed by the
// current DB state (spec §4, §11; REPL ".overwrite"). The process itself
// is not terminated (that decision belongs to the caller, e.g.
// cmd/san-db-ox's REPL).
func (db *DB) Overwrite() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("engine: os.Executable: %w", err)
	}
	if looksLikeGoRunTempBinary(self) {
		return ErrNotOverwritable
	}

	data, err := db.serializeBarrier()
	if err != nil {
		return err
	}
	engineBytes, err := readEnginePrefix(self)
	if err != nil {
		return err
	}

	return overwriteSelf(self, engineBytes, data)
}

// looksLikeGoRunTempBinary reports whether path sits under a
// `go-build*` directory inside the OS temp dir, the pattern `go run`
// uses for the binary it builds and deletes on exit (spec §11).
func looksLikeGoRunTempBinary(path string) bool {
	tmp := os.TempDir()
	if tmp == "" {
		return false
	}
	rel, err := filepath.Rel(tmp, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && strings.Contains(rel, "go-build")
}

// overwriteSelf replaces the file at selfPath with
// engineBytes+data+footer even though selfPath is the currently running
// executable. A direct overwrite is rejected by the OS (Linux ETXTBSY,
// Windows ERROR_SHARING_VIOLATION), so instead (spec §11,
// PoC-verified on Linux/Windows in the predecessor project):
//  1. rename selfPath to selfPath+oldSuffix (renaming a running
//     executable is allowed on both Linux and Windows)
//  2. write the new content to the now-vacated selfPath (a fresh file,
//     not an overwrite, so it succeeds on both OSes)
//  3. best-effort remove the sidecar (succeeds immediately on Linux;
//     Windows keeps it locked while this process runs, so it is cleaned
//     up on the next OpenSelf instead)
func overwriteSelf(selfPath string, engineBytes, data []byte) error {
	oldPath := selfPath + oldSuffix
	os.Remove(oldPath) // clear a leftover from an earlier run, if any

	if err := os.Rename(selfPath, oldPath); err != nil {
		return fmt.Errorf("engine: could not move aside the running executable: %w", err)
	}

	blob := make([]byte, 0, len(engineBytes)+len(data)+FooterSize)
	blob = append(blob, engineBytes...)
	blob = append(blob, data...)
	blob = append(blob, encodeFooter(int64(len(engineBytes)), int64(len(data)))...)

	if err := os.WriteFile(selfPath, blob, 0o755); err != nil {
		os.Rename(oldPath, selfPath) // best-effort restore
		return fmt.Errorf("engine: could not write the new executable: %w", err)
	}

	os.Remove(oldPath)
	return nil
}
