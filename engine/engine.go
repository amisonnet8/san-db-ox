package engine

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	_ "modernc.org/sqlite"
)

// defaultBusyTimeoutMS bounds how long any connection to the live
// database waits behind another connection's lock before giving up with
// SQLITE_BUSY, rather than relying on engine to reimplement locking
// itself (spec §2). See the Phase 1 Step 2 spike notes (PLAN.md) for why
// this matters specifically for the memdb VFS.
const defaultBusyTimeoutMS = 5000

// dbSeq gives each in-memory database a unique name in the memdb VFS's
// shared-store namespace (a name starting with "/" is shared globally by
// name within the process), so multiple DBs in the same process (e.g. a
// host app calling engine.Open more than once) never collide.
var dbSeq int64

// DB is an in-memory SQL database backed by modernc.org/sqlite, whose
// state can be persisted as a new executable (Snapshot) or by
// overwriting the host process's own executable in place (Overwrite).
// See docs/spec/san-db-ox_spec_ja.md §10-11 for the full design. The
// zero value is not usable; construct a DB with Open or OpenSelf.
type DB struct {
	mu     sync.RWMutex
	sdb    *sql.DB
	keeper *sql.Conn // keeps the live memdb store alive; never used to run SQL
	dsn    string    // this DB's live memdb DSN -- the backup destination in loadBlobInto
	closed bool
}

// newLiveDB opens a fresh memdb-backed live database and returns a
// keeper connection that must stay open for the database's entire
// lifetime (a store with no connections left open can be freed), plus
// the DSN callers need to reach the same store as a Backup destination
// (backup.go) or from DB.sdb's own connection pool.
func newLiveDB() (sdb *sql.DB, keeper *sql.Conn, dsn string, err error) {
	name := fmt.Sprintf("san-db-ox%d", atomic.AddInt64(&dbSeq, 1))
	dsn = fmt.Sprintf("file:/%s?vfs=memdb&_busy_timeout=%d", name, defaultBusyTimeoutMS)
	sdb, err = sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, "", err
	}
	keeper, err = sdb.Conn(context.Background())
	if err != nil {
		sdb.Close()
		return nil, nil, "", err
	}
	return sdb, keeper, dsn, nil
}

func newDB() (*DB, error) {
	sdb, keeper, dsn, err := newLiveDB()
	if err != nil {
		return nil, err
	}
	return &DB{sdb: sdb, keeper: keeper, dsn: dsn}, nil
}

// Open loads path as a plain SQLite database file into a new in-memory
// database (spec §6, §10: "独自のデータファイル形式は存在しない"). A
// path that does not exist, or one that is empty, yields an empty
// database rather than an error.
func Open(path string) (*DB, error) {
	db, err := newDB()
	if err != nil {
		return nil, err
	}

	blob, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return db, nil
		}
		db.Close()
		return nil, err
	}
	if len(blob) == 0 {
		return db, nil
	}
	if err := loadBlobInto(blob, db.dsn); err != nil {
		db.Close()
		return nil, fmt.Errorf("engine: %s: %w", path, err)
	}
	return db, nil
}

// OpenSelf loads the data embedded in the running process's own
// executable (os.Executable()), if any (spec §4, §11, §13). It also
// removes a leftover "<self>.san-db-ox.old" sidecar left behind by a
// previous Overwrite, on a best-effort basis.
func OpenSelf() (*DB, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("engine: os.Executable: %w", err)
	}
	os.Remove(self + oldSuffix)

	db, err := newDB()
	if err != nil {
		return nil, err
	}

	fi, _, err := readFooter(self)
	if err != nil {
		db.Close()
		return nil, err
	}
	if !fi.hasData {
		return db, nil
	}

	f, err := os.Open(self)
	if err != nil {
		db.Close()
		return nil, err
	}
	blob := make([]byte, fi.dataLength)
	_, err = f.ReadAt(blob, fi.dataOffset)
	f.Close()
	if err != nil {
		db.Close()
		return nil, err
	}

	if err := loadBlobInto(blob, db.dsn); err != nil {
		db.Close()
		return nil, fmt.Errorf("engine: %s: %w", self, err)
	}
	return db, nil
}

// Exec, Query and QueryRow (and their *Context variants) each run on a
// connection borrowed from the pool (db.sdb), used once and returned --
// they are one-shot operations, not a session. A statement like BEGIN
// executed this way appears to succeed, but the transaction it opens is
// invisible to every later call, since database/sql may hand the next
// caller a different pooled connection. Session-scoped access
// (BEGIN/COMMIT/ROLLBACK across multiple calls) is out of scope for
// Phase 1 (PLAN.md).
func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	return db.ExecContext(context.Background(), query, args...)
}

func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	sdb, err := db.pooled()
	if err != nil {
		return nil, err
	}
	return sdb.ExecContext(ctx, query, args...)
}

func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(context.Background(), query, args...)
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	sdb, err := db.pooled()
	if err != nil {
		return nil, err
	}
	return sdb.QueryContext(ctx, query, args...)
}

func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.QueryRowContext(context.Background(), query, args...)
}

// QueryRowContext behaves like ExecContext/QueryContext, with one
// difference after Close: it cannot return ErrClosed directly, because
// *sql.Row has no way to carry a caller-supplied error before Scan is
// called. A QueryRowContext call made after Close instead surfaces
// database/sql's own "sql: database is closed" once Scan is called on
// the result.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	db.mu.RLock()
	sdb := db.sdb
	db.mu.RUnlock()
	return sdb.QueryRowContext(ctx, query, args...)
}

// pooled returns db.sdb for a one-shot call, or ErrClosed if db has
// already been closed.
func (db *DB) pooled() (*sql.DB, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil, ErrClosed
	}
	return db.sdb, nil
}

// Close releases the database's resources. It does not persist
// anything; callers that want to keep the data must call Snapshot or
// Overwrite first (spec §4: persistence is explicit only). Close is
// idempotent: a second call is a no-op that returns nil.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return nil
	}
	db.closed = true
	if db.keeper != nil {
		db.keeper.Close()
	}
	return db.sdb.Close()
}
