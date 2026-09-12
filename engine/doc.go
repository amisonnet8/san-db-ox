// Package engine provides SanDBox's in-memory SQL engine as a Go library.
//
// It wraps modernc.org/sqlite to offer an in-memory database that can be
// persisted either as a new executable (Snapshot) or by overwriting the
// host process's own executable in place (Overwrite). The package does
// not itself import net or net/http: it has no network I/O of any kind;
// external access is the responsibility of the caller (cmd/san-db-ox).
//
// See docs/spec/san-db-ox_spec_ja.md §6, §10-11 for the full design.
package engine
