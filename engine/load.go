package engine

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// maxLoadImageSize bounds how many bytes LoadFrom will read before giving
// up with ErrTooLarge, so a caller cannot be made to buffer an unbounded
// stream in memory. It is generous enough for any image engine can
// actually produce: MaxDataSize (the memdb VFS's own ceiling, spec §11)
// plus headroom for a SanDBox executable's engine prefix, which runs
// 10-15MB in practice (binary-size.md) even with room to spare.
const maxLoadImageSize = MaxDataSize + 256<<20

// sqliteWALFormatOffset and sqliteWALFormatVersion locate and identify
// the byte in a SQLite file header that says whether the database is in
// WAL mode: offsets 18 and 19 are the file-format write and read version
// (1 = legacy rollback journal, 2 = WAL), and a database always carries
// the same value in both (confirmed in the Phase 2 Step 0 spike).
const (
	sqliteWALFormatOffset  = 18
	sqliteWALFormatVersion = 2
)

// Load replaces db's in-memory state with the data in path, discarding
// whatever db previously held; it does not merge (spec §4, REPL
// ".load"). Load writes no file, and for a SanDBox executable it never
// references path's engine portion -- only its data (spec §4: "常に今
// 動いているプロセス自身のエンジンで、データのみを読み込む").
//
// path is classified the same way Inspect (inspect.go) classifies it,
// before db is touched at all:
//   - KindSQLite is restored via loadFromSQLiteFile below, which reads
//     path through SQLite's own VFS rather than as raw bytes, so a
//     source file running in journal_mode=WAL is read correctly even if
//     its most recent commits sit only in an unmerged "-wal" sidecar
//     (spec §6, §11; confirmed in the Phase 2 Step 0 spike -- reading
//     just the main file's bytes would silently miss them). This does
//     open path for read-write, so Load fails against a read-only file
//     or filesystem, and the source file's WAL may be checkpointed as a
//     side effect of the connection SQLite opens internally being closed
//     (spec §4).
//   - KindExecutable's data blob (the footer's [DataOffset, DataLength)
//     range) goes through loadBlobInto (serialize.go), the same
//     Deserialize-then-Backup path Open/OpenSelf use. It is always a
//     Serialize() output (a rollback-mode memdb image), so the WAL
//     concern above cannot arise here.
//   - A zero-length KindExecutable data blob is ErrNoData (spec §4:
//     "SanDBoxファイルだがデータ長が0の場合はエラー").
//   - KindUnknown is ErrUnsupportedFile.
//
// In every error case above, db's live database is left untouched: Load
// only reaches the code that actually replaces content once path's
// classification and (for KindExecutable) the footer's own internal
// consistency are already known-good, and that replacement itself runs
// as SQLite's online Backup API's single write transaction (backup.go's
// runBackup), which rolls back cleanly if it fails partway through
// (spec §4, §11).
//
// Load does not warn about a footer format-version mismatch; call
// Inspect first if the caller wants that (spec §4) -- engine itself
// never logs (§10).
func (db *DB) Load(path string) error {
	fi, err := Inspect(path)
	if err != nil {
		return err
	}

	switch fi.Kind {
	case KindSQLite:
		return db.loadFromSQLiteFile(path)
	case KindExecutable:
		if !fi.HasData {
			return fmt.Errorf("engine: %s: %w", path, ErrNoData)
		}
		return db.loadEmbeddedBlob(path, fi.DataOffset, fi.DataLength)
	default:
		return fmt.Errorf("engine: %s: %w", path, ErrUnsupportedFile)
	}
}

// LoadFrom behaves like Load, but reads the image from r instead of a
// named file (spec §10: the io.Reader counterpart of Load, for a caller
// with no file path -- see "Load と LoadFrom の違い" in the spec).
//
// r is read to completion (bounded by maxLoadImageSize) and classified
// from the resulting bytes the same way Inspect classifies a file: the
// SQLite header first, then a trailing footer. Unlike Load, there is no
// second path to fall back on for a SQLite image: a byte stream cannot
// carry a "-wal" sidecar, so LoadFrom rejects a WAL-mode SQLite image
// outright (detected from the file header's format-version byte,
// confirmed in the Phase 2 Step 0 spike) rather than silently importing
// a database that may be missing its most recent commits. Call
// Load(path) for a database that lives on disk; LoadFrom exists for
// input that has no path in the first place.
func (db *DB) LoadFrom(r io.Reader) error {
	buf, err := io.ReadAll(io.LimitReader(r, maxLoadImageSize+1))
	if err != nil {
		return err
	}
	if int64(len(buf)) > maxLoadImageSize {
		return ErrTooLarge
	}
	size := int64(len(buf))

	if size >= int64(len(sqliteHeaderPrefix)) && string(buf[:len(sqliteHeaderPrefix)]) == sqliteHeaderPrefix {
		if size > sqliteWALFormatOffset && buf[sqliteWALFormatOffset] == sqliteWALFormatVersion {
			return fmt.Errorf("engine: WAL-mode SQLite image is not supported by LoadFrom (use Load with a file path instead): %w", ErrUnsupportedFile)
		}
		return db.applyBlob(buf)
	}

	var footer []byte
	if size >= FooterSize {
		footer = buf[size-FooterSize:]
	}
	fi, err := decodeFooter(footer, size)
	if err != nil {
		return fmt.Errorf("engine: %w", err)
	}
	if !fi.hasData {
		return fmt.Errorf("engine: %w", ErrUnsupportedFile)
	}
	if fi.dataLength == 0 {
		return fmt.Errorf("engine: %w", ErrNoData)
	}
	return db.applyBlob(buf[fi.dataOffset : fi.dataOffset+fi.dataLength])
}

// readRange reads exactly length bytes from path starting at offset.
// Shared by OpenSelf (engine.go, reading its own footer's data range)
// and loadEmbeddedBlob below (reading another SanDBox executable's).
func readRange(path string, offset, length int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, length)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return nil, err
	}
	return buf, nil
}

// loadFromSQLiteFile is Load's KindSQLite branch. It cannot restore path
// straight into db's live database with a single restoreFrom call the
// way it first looks like it should: SQLite's Backup API copies a
// source's header bytes verbatim into an empty destination, including
// the journal-mode flag, and while a plain file VFS tolerates a WAL-mode
// header with no matching "-wal" file sitting next to it just fine, the
// memdb VFS engine's live database uses does not -- it fails to open the
// result at all (confirmed in the Phase 2 Step 3 spike; spec §11).
//
// So this routes through a rollback-mode intermediate file instead:
//  1. restore path into a fresh temp file (a plain file VFS, so a
//     WAL-mode header is fine here)
//  2. force that temp file to rollback mode with journal_mode=DELETE,
//     which both checkpoints it and rewrites its header
//  3. restore the now rollback-mode temp file into db's live database
//
// This runs the same two-step path even when path is not in WAL mode to
// begin with, rather than branching on Inspect-time information that
// could be stale by the time this actually reads the file.
func (db *DB) loadFromSQLiteFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	srcDSN, err := fileDSN(abs)
	if err != nil {
		return err
	}

	sdb, err := db.pooled()
	if err != nil {
		return err
	}

	stageDSN, cleanup, err := stageRollbackModeCopy(srcDSN)
	if err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	defer cleanup()

	conn, err := sdb.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := restoreFrom(conn, stageDSN); err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	return nil
}

// stageRollbackModeCopy restores srcDSN into a fresh temporary file
// (created in the OS temp directory, not srcDSN's own -- srcDSN's
// directory may not be writable, and Load only needs to read from it)
// and forces that copy to rollback mode, so it is safe to restore from
// again into a memdb-backed destination (loadFromSQLiteFile above). The
// returned cleanup func removes the temp file (and any sidecars) and
// must be called once the caller is done with the DSN it returns.
func stageRollbackModeCopy(srcDSN string) (stageDSN string, cleanup func(), err error) {
	tmp, err := os.CreateTemp("", ".san-db-ox_load_tmp_*")
	if err != nil {
		return "", nil, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	cleanup = func() { removeTempArtifacts(tmpPath) }

	stageDSN, err = fileDSN(tmpPath)
	if err != nil {
		cleanup()
		return "", nil, err
	}

	sdb, err := sql.Open("sqlite", stageDSN)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	defer sdb.Close()
	conn, err := sdb.Conn(context.Background())
	if err != nil {
		cleanup()
		return "", nil, err
	}
	defer conn.Close()

	if err := restoreFrom(conn, srcDSN); err != nil {
		cleanup()
		return "", nil, err
	}
	if _, err := conn.ExecContext(context.Background(), "PRAGMA journal_mode=DELETE"); err != nil {
		cleanup()
		return "", nil, err
	}
	return stageDSN, cleanup, nil
}

// loadEmbeddedBlob is Load's KindExecutable branch: it reads exactly the
// footer's data range out of path and applies it the same way
// OpenSelf does.
func (db *DB) loadEmbeddedBlob(path string, dataOffset, dataLength int64) error {
	blob, err := readRange(path, dataOffset, dataLength)
	if err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	if err := db.applyBlob(blob); err != nil {
		return fmt.Errorf("engine: %s: %w", path, err)
	}
	return nil
}

// applyBlob is the shared tail end of Load's KindExecutable branch and
// LoadFrom: both end up with a Serialize()-shaped byte slice that must
// go through the Deserialize-then-Backup path (loadBlobInto,
// serialize.go) rather than being handed to the live database directly
// (spec §11: Deserialize only ever affects the exact connection it runs
// on). Reads db.dsn directly (rather than going through pooled(), which
// hands back a *sql.DB this has no use for) purely to check db.closed
// before doing any work.
func (db *DB) applyBlob(blob []byte) error {
	db.mu.RLock()
	dsn := db.dsn
	closed := db.closed
	db.mu.RUnlock()
	if closed {
		return ErrClosed
	}
	return loadBlobInto(blob, dsn)
}
