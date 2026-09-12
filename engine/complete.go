package engine

import (
	"errors"
	"strings"

	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

// Complete reports whether sql is a syntactically complete SQL statement
// (or a semicolon-separated sequence of them), by calling SQLite's own
// sqlite3_complete() C routine -- the same one the real sqlite3 CLI shell
// uses to decide, while reading interactive input, whether to run what
// has been typed so far or keep waiting for more lines (spec §3, §10,
// §11; used by cmd/san-db-ox's REPL to drive multi-line input, naming.md).
//
// A hand-rolled scanner that just tracks BEGIN/END token nesting would
// be fooled by a CREATE TRIGGER body: a CASE...END expression inside it
// has an END that does not close the trigger, and such a scanner has no
// way to tell the two apart. sqlite3_complete's real tokenizer does not
// make that mistake, which is why Complete calls it directly rather than
// reimplementing the state machine.
//
// Complete does not touch any database -- sqlite3_complete is a pure
// text-scanning routine that takes no DB handle -- so this works without
// an open DB (Open/OpenSelf) by design, and does not validate sql as SQL:
// a syntactically invalid statement that ends in a semicolon is still
// "complete" (spec §11).
//
// Reaching sqlite3_complete requires calling into modernc.org/sqlite's
// generated internals (modernc.org/sqlite/lib, not the stable driver
// package) rather than a documented public API -- see
// .claude/rules/sqlite-quirks.md for what to recheck if modernc.org/sqlite
// is ever upgraded. modernc.org/libc and modernc.org/sqlite/lib are
// already transitive dependencies of modernc.org/sqlite itself
// (binary-size.md, Makefile's netcheck), so this adds no new third-party
// dependency and no binary-size cost.
//
// error is returned in exactly two cases: sql contains an embedded NUL
// byte (sqlite3_complete reads a C string and would silently stop at the
// first one, which could make it answer a question about only part of
// sql without any indication that happened), or the C string allocation
// itself fails (effectively out-of-memory).
func Complete(sql string) (bool, error) {
	if strings.IndexByte(sql, 0) >= 0 {
		return false, errors.New("engine: sql contains an embedded NUL byte")
	}

	tls := libc.NewTLS()
	defer tls.Close()

	csql, err := libc.CString(sql)
	if err != nil {
		return false, err
	}
	defer libc.Xfree(tls, csql)

	return sqlite3.Xsqlite3_complete(tls, csql) != 0, nil
}
