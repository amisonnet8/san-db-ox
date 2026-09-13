package main

import "errors"

// ErrReadOnly is returned by cmd/san-db-ox's own save operations
// (.snapshot/.overwrite/.load, including ".snapshot --sqlite") when the
// process was started with --read-only (spec §2). It lives here rather
// than in engine: --read-only is a concept cmd/san-db-ox owns, not
// engine (docs/spec/san-db-ox_spec_ja.md §2: "engineは概念を持たない").
// Write SQL statements are rejected separately, by SQLite's own
// query_only pragma applied to every Session opened under --read-only
// (openSession, repl.go) -- ErrReadOnly is only for the non-SQL save
// operations that pragma does not reach.
var ErrReadOnly = errors.New("read-only mode: write operations are disabled")

// readOnly reports whether r's run was started with --read-only. A nil
// r.opts (only possible in tests that construct a *repl by hand) behaves
// as not read-only, matching every other opts-derived default in this
// package (e.g. newRepl's mode/headers defaults, repl.go).
func (r *repl) readOnly() bool {
	return r.opts != nil && r.opts.readOnly
}
