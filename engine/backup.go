package engine

import (
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
)

// backupper mirrors modernc.org/sqlite's (unexported) driver connection
// type's exported NewBackup method: func (c *conn) NewBackup(dstUri
// string) (*Backup, error). Reached via sql.Conn.Raw() + a local
// interface assertion, the same technique serializer/deserializer in
// serialize.go use. Naming the return type requires a non-blank import of
// modernc.org/sqlite (engine.go's blank import stays for driver
// registration; this one exists purely to spell *sqlite.Backup).
type backupper interface {
	NewBackup(dstUri string) (*sqlite.Backup, error)
}

// restorer mirrors the driver connection type's exported NewRestore
// method: func (c *conn) NewRestore(srcUri string) (*Backup, error). This
// is NewBackup run in reverse -- the caller's own connection is the
// backup's destination, and srcUri names the source -- which is how Load
// pulls a plain SQLite file into engine's live database (spec §6, §11):
// SQLite opens srcUri through its normal VFS, so a WAL-mode source file's
// commits sitting only in its "-wal" sidecar are not silently dropped the
// way they would be by reading the main file's bytes and Deserializing
// them (confirmed in the Phase 2 Step 0 spike).
type restorer interface {
	NewRestore(srcUri string) (*sqlite.Backup, error)
}

// backupStepper is the subset of *sqlite.Backup's methods runBackup
// needs. Spelled out as an interface (rather than taking *sqlite.Backup
// directly) so backup_test.go can substitute a fake that records whether
// Finish actually ran after a failed Step -- the one thing this function
// exists to guarantee. *sqlite.Backup satisfies this implicitly.
type backupStepper interface {
	Step(n int32) (bool, error)
	Finish() error
}

// runBackup drives bk to completion and always calls Finish, even when
// Step fails or reports unfinished work. Skipping Finish is not just
// untidy: only sqlite3_backup_finish commits or rolls back whatever
// sqlite3_backup_step managed to do, and for a restorer's NewRestore,
// only Finish closes the source connection SQLite opened internally to
// read srcUri -- skip it and that connection (and its open file handle)
// leaks for as long as the *engine.DB stays open, which on Windows keeps
// the source file locked against deletion or a later rewrite
// (backup_test.go's TestBackupFinishesEvenOnStepFailure is a regression
// test for exactly this, caught in review before it ever shipped -- spec
// §11, "取り込み失敗時にメモリ上の状態が変わらないことの技術的根拠").
//
// A SQLITE_BUSY from either Step or Finish (another connection holding a
// conflicting lock past the live database's busy_timeout) is reported as
// ErrBusy, matching serializeBarrier's mapping in persist.go.
func runBackup(bk backupStepper) error {
	more, stepErr := bk.Step(-1)
	if stepErr == nil && more {
		stepErr = fmt.Errorf("engine: backup reported more pages remaining after Step(-1)")
	}
	finishErr := bk.Finish()
	if stepErr != nil {
		return mapBusy(stepErr)
	}
	return mapBusy(finishErr)
}

// sqliteBusyCode is SQLITE_BUSY (modernc.org/sqlite/lib.SQLITE_BUSY):
// "the database file is locked". Spelled out here rather than importing
// modernc.org/sqlite/lib just for this one constant -- Complete
// (complete.go) is the only place in engine that needs that package for
// other reasons.
const sqliteBusyCode = 5

// mapBusy translates a SQLITE_BUSY error from the backup/restore machinery
// into ErrBusy, the same sentinel serializeBarrier (persist.go) uses, so
// callers of Snapshot, Export and Load see one consistent error for "try
// again once the conflicting writer is done".
func mapBusy(err error) error {
	if err == nil {
		return nil
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqliteBusyCode {
		return fmt.Errorf("%w: %w", ErrBusy, err)
	}
	return err
}

// backupInto copies conn's entire current database content into dstDSN's
// live database via SQLite's online Backup API (spec §11). Unlike
// Deserialize, a backup propagates through SQLite's normal btree/pager
// machinery, so the result becomes visible to every connection on
// dstDSN, including ones opened before the backup ran (confirmed in the
// Phase 1 Step 2 spike).
//
// dstDSN's database must already have at least one connection open and
// held alive by the caller (DB.keeper does this for engine's own live
// database): Backup.Finish closes its own destination connection, and a
// memdb store with no connections left open can be freed as soon as that
// happens.
func backupInto(conn *sql.Conn, dstDSN string) error {
	return conn.Raw(func(driverConn any) error {
		b, ok := driverConn.(backupper)
		if !ok {
			return fmt.Errorf("engine: driver connection does not support NewBackup")
		}
		bk, err := b.NewBackup(dstDSN)
		if err != nil {
			return mapBusy(err)
		}
		return runBackup(bk)
	})
}

// restoreFrom copies srcDSN's entire database content into conn's own
// live database via SQLite's online Backup API run in reverse (spec §6,
// §11: this is how Load pulls in a plain SQLite file). Unlike backupInto,
// conn here is the destination: Backup.Finish closes only the *source*
// connection the driver opened for srcDSN, so conn -- and every other
// connection already open on the same live store, including a live
// Session -- stays open and immediately sees the new content (spec §4:
// "既存のセッションは接続を失わない").
func restoreFrom(conn *sql.Conn, srcDSN string) error {
	return conn.Raw(func(driverConn any) error {
		r, ok := driverConn.(restorer)
		if !ok {
			return fmt.Errorf("engine: driver connection does not support NewRestore")
		}
		bk, err := r.NewRestore(srcDSN)
		if err != nil {
			return mapBusy(err)
		}
		return runBackup(bk)
	})
}
