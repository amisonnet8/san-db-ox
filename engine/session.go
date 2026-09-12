package engine

import (
	"context"
	"database/sql"
	"sync/atomic"
)

// Session is a single connection checked out of db's pool and held for
// the caller's exclusive use, so BEGIN/COMMIT/ROLLBACK and other
// connection-scoped state span multiple calls (spec §10). db.Exec and
// friends cannot do this: they borrow a connection per call and may hand
// the next call a different one, so a BEGIN executed through them opens
// a transaction no later call is guaranteed to see (engine.go's Exec doc
// comment).
//
// Session deliberately has no Begin/BeginTx: BEGIN/COMMIT/ROLLBACK are
// plain SQL statements to it, executed like any other, which is enough
// to give them real transactional meaning on a connection nothing else
// shares. Wrapping them in *sql.Tx would create a second, conflicting
// notion of "the current transaction" that database/sql itself does not
// know about.
//
// A Session is meant to be driven by one logical client at a time (spec
// §10) -- concurrent calls from multiple goroutines do not race (the
// underlying *sql.Conn serializes them), but two goroutines sharing one
// Session for unrelated work will end up inside each other's
// transactions, since the transaction lives on the connection, not
// per-caller state.
//
// Sessions must be closed explicitly. DB.Close (engine.go) does not wait
// for or forcibly close outstanding Sessions -- engine reports errors
// rather than policing a caller's object lifetimes (spec §10's division
// of responsibility) -- so a Session left open after DB.Close keeps its
// one connection (and, through it, the live memdb store) alive for as
// long as the Session itself is not closed.
//
// A write transaction held open on a Session blocks Snapshot, Overwrite,
// Export and Load until it commits or rolls back, all returning an error
// wrapping ErrBusy once the live database's busy_timeout elapses -- they
// each need either a read or write lock the transaction is holding (spec
// §11's serializeBarrier for Snapshot/Overwrite, Export and Load's own
// Backup API calls). A read transaction only blocks Load, which needs
// exclusive access to write the copy destination; Snapshot and Export
// only read the live database and do not conflict with another reader.
type Session struct {
	conn   *sql.Conn
	closed atomic.Bool
}

// Session checks out a dedicated connection from db's pool (spec §10).
// The pool itself has no connection limit (see newLiveDB's comment,
// engine.go): a Session holds its connection until Close, so capping the
// pool would let enough outstanding Sessions deadlock every later
// Session/Exec/Query call against it.
func (db *DB) Session(ctx context.Context) (*Session, error) {
	sdb, err := db.pooled()
	if err != nil {
		return nil, err
	}
	conn, err := sdb.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return &Session{conn: conn}, nil
}

func (s *Session) Exec(query string, args ...any) (sql.Result, error) {
	return s.ExecContext(context.Background(), query, args...)
}

func (s *Session) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if s.closed.Load() {
		return nil, ErrClosed
	}
	return s.conn.ExecContext(ctx, query, args...)
}

func (s *Session) Query(query string, args ...any) (*sql.Rows, error) {
	return s.QueryContext(context.Background(), query, args...)
}

func (s *Session) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if s.closed.Load() {
		return nil, ErrClosed
	}
	return s.conn.QueryContext(ctx, query, args...)
}

func (s *Session) QueryRow(query string, args ...any) *sql.Row {
	return s.QueryRowContext(context.Background(), query, args...)
}

// QueryRowContext behaves like DB.QueryRowContext (engine.go): it cannot
// return ErrClosed directly after Close, since *sql.Row has no way to
// carry a caller-supplied error before Scan is called. A post-Close call
// instead surfaces database/sql's own "sql: connection is already
// closed" once Scan runs.
func (s *Session) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.conn.QueryRowContext(ctx, query, args...)
}

// Prepare and PrepareContext return a *sql.Stmt bound to this Session's
// own connection, for a caller that runs the same statement many times
// (e.g. bulk-loading a CSV file's rows, cmd/san-db-ox's future ".import")
// and wants to avoid re-preparing it on every call.
func (s *Session) Prepare(query string) (*sql.Stmt, error) {
	return s.PrepareContext(context.Background(), query)
}

func (s *Session) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	if s.closed.Load() {
		return nil, ErrClosed
	}
	return s.conn.PrepareContext(ctx, query)
}

// Close releases the Session's connection back to db's pool, rolling
// back any transaction left open on it (closing a *sql.Conn does this
// automatically). Close is idempotent: a second call is a no-op that
// returns nil.
func (s *Session) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	return s.conn.Close()
}
