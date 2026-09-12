// Package engine provides SanDBox's in-memory SQL engine as a Go library.
//
// It wraps modernc.org/sqlite to offer an in-memory database that can be
// opened from a plain SQLite file or a SanDBox executable (Open,
// OpenSelf), queried with one-shot calls (Exec, Query, QueryRow) or a
// dedicated connection spanning multiple calls (Session), replaced in
// place from another file or stream (Load, LoadFrom), and persisted
// either as a new executable (Snapshot), by overwriting the host
// process's own executable (Overwrite), or as a plain SQLite file
// (Export). Inspect reports a file's format and footer information
// without opening it as a database. Complete answers whether a string of
// SQL is a syntactically complete statement, for a caller reading SQL
// interactively (cmd/san-db-ox's REPL).
//
// The package does not itself import net or net/http: it has no network
// I/O of any kind; external access is the responsibility of the caller
// (cmd/san-db-ox). It also never logs -- every failure is returned as an
// error, never written to stderr (spec §10's division of responsibility).
//
// See docs/spec/san-db-ox_spec_ja.md §4, §6, §10-11 for the full design.
package engine
