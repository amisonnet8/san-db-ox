package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
//
// If path happens to name the running executable itself -- the default
// no-FILENAME case naturally lands here, since it names path after
// self's own basename in the current directory (naming.md) -- Snapshot
// switches to Overwrite's evacuate-then-write technique instead of a
// plain rename-into-place: confirmed by Phase 1 Step 5's Windows CI run,
// a straight `os.Rename` onto the running executable's own path fails
// there ("Access is denied") even though it succeeds on Linux/macOS.
// Unlike Overwrite, the caller's process keeps running from its original
// (now-unlinked-or-renamed-away) image either way; only the file on disk
// changes.
//
// Otherwise the write is atomic: a temporary file in path's directory,
// then renamed into place (spec §11).
//
// Snapshot does not generate a file name (timestamps, ".exe" completion,
// §12): that is cmd/san-db-ox's responsibility (naming.md); path is used
// as-is.
func (db *DB) Snapshot(path string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("engine: os.Executable: %w", err)
	}
	targetsSelf := samePath(path, self)
	if targetsSelf && looksLikeGoRunTempBinary(self) {
		// Same reasoning as Overwrite: `go run` deletes this file on
		// exit, so evacuating and rewriting it would be pointless.
		return ErrNotOverwritable
	}

	engineBytes, data, err := db.currentImage(self)
	if err != nil {
		return err
	}

	if targetsSelf {
		return overwriteSelf(self, engineBytes, data)
	}
	return writeImageAtomic(path, engineBytes, data)
}

// samePath reports whether a and b name the same file. When both exist,
// it defers to os.SameFile, which compares OS-level file identity
// (volume + file index on Windows, device + inode on Unix) rather than
// the path spelling -- necessary in practice, not just in theory: a
// pure string comparison of filepath.Abs(a)/filepath.Abs(b) (even
// case-folded) was confirmed insufficient on windows-latest CI, where
// os.Executable() and the equivalent path built from os.Getwd() can
// legitimately spell the same directory differently (e.g. a short
// 8.3-style path component such as "RUNNER~1" appearing in one but not
// the other). Snapshot's target commonly does NOT exist yet, though (a
// brand-new snapshot name), so when either os.Stat fails, this falls
// back to comparing cleaned absolute paths -- case-insensitively on
// Windows (NTFS/Windows semantics), case-sensitively elsewhere.
func samePath(a, b string) bool {
	if fa, errA := os.Stat(a); errA == nil {
		if fb, errB := os.Stat(b); errB == nil {
			return os.SameFile(fa, fb)
		}
	}

	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	absA, absB = filepath.Clean(absA), filepath.Clean(absB)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(absA, absB)
	}
	return absA == absB
}

// currentImage takes a consistent snapshot of the live database and
// re-reads self's engine prefix. Both Snapshot and Overwrite build their
// output this way; self is the caller's already-resolved
// os.Executable() so Overwrite can check looksLikeGoRunTempBinary before
// paying for this (serializeBarrier can block on another connection's
// write lock).
func (db *DB) currentImage(self string) (engineBytes, data []byte, err error) {
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

// tempFileFor creates a temporary file in the same directory as path, so
// a later os.Rename onto path is a same-filesystem (and therefore atomic)
// move rather than a cross-filesystem copy (spec §11). Shared by
// writeImageAtomic (below) and Export (export.go), which differ in what
// they put in the file and what permissions it ends up with, but agree on
// this much.
func tempFileFor(path string) (*os.File, error) {
	return os.CreateTemp(filepath.Dir(path), ".san-db-ox_tmp_*")
}

// removeTempArtifacts removes tmpPath and the SQLite sidecar files
// ("-journal", "-wal", "-shm") a connection writing to tmpPath may have
// created alongside it. writeImageAtomic never produces sidecars (it
// writes engineBytes+data+footer directly, never through a SQLite
// connection), but calls this anyway to share one cleanup path with
// Export (export.go), which does write tmpPath through SQLite's Backup
// API and can leave sidecars behind on failure. Every removal is
// best-effort and silently ignores a missing file: this exists to be
// called from a deferred cleanup path, where the original error already
// matters more than a cleanup failure would.
func removeTempArtifacts(tmpPath string) {
	os.Remove(tmpPath)
	os.Remove(tmpPath + "-journal")
	os.Remove(tmpPath + "-wal")
	os.Remove(tmpPath + "-shm")
}

// writeImageAtomic writes engineBytes+data+footer to path via a
// temporary file in the same directory followed by a rename, so a reader
// never observes a half-written file (spec §11). The output is always
// executable (0755): Snapshot's output is meant to be run directly
// (spec §1, §9).
func writeImageAtomic(path string, engineBytes, data []byte) error {
	tmp, err := tempFileFor(path)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer removeTempArtifacts(tmpPath) // no-op once the rename below succeeds

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

	engineBytes, data, err := db.currentImage(self)
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

// fileDSN turns an absolute OS path into a DSN modernc.org/sqlite's
// driver will open as a plain file (not the memdb VFS), for Export
// (export.go) and Load's SQLite-file restore path (load.go via
// restoreFrom, backup.go).
//
// The driver's newConn treats a DSN that does not start with "file:" as
// a bare path to open verbatim, except that it still splits off
// everything from the first "?" onward and reads it as driver parameters
// (modernc.org/sqlite's conn.go). This has two consequences here:
//   - path must not itself contain "?": on Unix a path legally could,
//     and if it did, everything from that "?" onward would silently be
//     dropped from the path the driver actually opens rather than
//     erroring, so fileDSN rejects it upfront instead.
//   - path must be absolute: a relative path that happened to start
//     with "file:" would otherwise be reparsed as a URI instead of
//     opened verbatim. Every caller runs path through filepath.Abs
//     first, which structurally rules this out (a Windows absolute path
//     starts with a drive letter, a Unix one with "/", neither of which
//     spells "file:").
//
// Backslashes need no translation: since the result never starts with
// "file:", it is never URI-parsed, and reaches the OS's own path-opening
// call (e.g. Windows CreateFile) exactly as given.
//
// fileDSN takes advantage of the "?"-stripping behavior above to append
// _busy_timeout without it ever reaching the OS as part of the path,
// matching the live database's own busy_timeout (engine.go) so a
// conflicting writer on the file blocks briefly rather than failing
// immediately.
func fileDSN(path string) (string, error) {
	if strings.ContainsRune(path, '?') {
		return "", fmt.Errorf("engine: %s: path must not contain '?'", path)
	}
	return fmt.Sprintf("%s?_busy_timeout=%d", path, defaultBusyTimeoutMS), nil
}
