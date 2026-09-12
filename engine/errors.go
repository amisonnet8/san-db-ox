package engine

import "errors"

var (
	// ErrClosed is returned by DB methods called after Close. It is not
	// returned by QueryRow: *sql.Row has no way to carry a
	// caller-supplied error before Scan is called, so a post-Close
	// QueryRow instead surfaces database/sql's own "sql: database is
	// closed" through Scan.
	ErrClosed = errors.New("engine: database is closed")

	// ErrNotOverwritable is returned by Overwrite when the running
	// process's own executable path looks like a `go run` temporary
	// binary: go run deletes it as soon as the process exits, which
	// would make Overwrite a no-op that looks like it succeeded (spec
	// §11).
	ErrNotOverwritable = errors.New("engine: running executable is not overwritable (looks like a `go run` temporary binary)")

	// ErrBusy is returned when Snapshot/Overwrite could not acquire the
	// serialization barrier because another connection held a
	// conflicting write transaction open past the live database's
	// busy_timeout.
	ErrBusy = errors.New("engine: database is busy (another connection is writing)")

	// ErrTooLarge is returned when the in-memory database cannot be
	// serialized as a single contiguous byte slice -- either because it
	// exceeds what Serialize can represent, or because
	// modernc.org/sqlite returned an empty buffer for a large database
	// rather than an error.
	ErrTooLarge = errors.New("engine: database is too large to serialize")

	// ErrUnsupportedFile is returned by Load, LoadFrom and Open when the
	// bytes they were given are neither a plain SQLite database file nor
	// a SanDBox executable with a trailing footer (spec §4, §6). The
	// live database is left untouched.
	ErrUnsupportedFile = errors.New("engine: not a SQLite database file or a SanDBox executable")

	// ErrNoData is returned by Load and LoadFrom for a SanDBox executable
	// whose footer is present but claims a zero-length data blob (spec
	// §4: "SanDBoxファイルだがデータ長が0の場合はエラー"). The live
	// database is left untouched.
	ErrNoData = errors.New("engine: file contains no data")
)
