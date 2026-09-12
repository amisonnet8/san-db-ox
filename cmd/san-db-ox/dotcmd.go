package main

import (
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/amisonnet8/san-db-ox/engine"
)

// dotCmdResult tells the REPL loop (and, once they exist, batch
// execution and the stdio protocol) whether to stop after a dot command,
// and with which exit code (spec §3 ".exit [CODE]").
type dotCmdResult struct {
	exit bool
	code int
}

// dispatchDotCommand parses and runs one dot command. It takes no
// REPL-specific state, so later phases can call it directly from batch
// execution or a stdio op handler without duplicating the logic
// (naming.md, .claude/rules/directory-structure.md: "3つの実行モードは
// 同じドットコマンド実装を共有する"). Phase 1 Step 4 implements only the
// six commands PLAN.md scopes for this step; .load/.import/.dump/.mode/
// .headers and the rest of docs/usage/repl-commands_ja.md's list are
// later phases.
func dispatchDotCommand(db *engine.DB, self, line string, out, errw io.Writer) dotCmdResult {
	fields := strings.Fields(line)
	name, args := fields[0], fields[1:]

	var res dotCmdResult
	var err error
	switch name {
	case ".tables":
		err = cmdTables(db, out)
	case ".schema":
		err = cmdSchema(db, args, out)
	case ".snapshot":
		err = cmdSnapshot(db, self, args, out)
	case ".overwrite":
		if err = cmdOverwrite(db, out); err == nil {
			res.exit = true
		}
	case ".exit", ".quit":
		res, err = cmdExit(args)
	case ".help":
		cmdHelp(out)
	default:
		err = fmt.Errorf("unknown command: %q (try .help)", name)
	}
	if err != nil {
		fmt.Fprintln(errw, "Error:", err)
	}
	return res
}

// cmdTables implements ".tables": list table names, excluding SQLite's
// own internal sqlite_* tables (spec §3).
func cmdTables(db *engine.DB, out io.Writer) error {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		fmt.Fprintln(out, name)
	}
	return rows.Err()
}

// cmdSchema implements ".schema [TABLE]": print CREATE statements, all
// of them by default or only those naming/belonging to TABLE (matching
// an index's tbl_name too, like sqlite3's own ".schema") (spec §3).
func cmdSchema(db *engine.DB, args []string, out io.Writer) error {
	query := `SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite\_%' ESCAPE '\'`
	var queryArgs []any
	if len(args) > 0 {
		query += ` AND (name = ? OR tbl_name = ?)`
		queryArgs = append(queryArgs, args[0], args[0])
	}
	query += ` ORDER BY rowid`

	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var sql string
		if err := rows.Scan(&sql); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s;\n", sql)
	}
	return rows.Err()
}

// cmdSnapshot implements ".snapshot [FILENAME]": save a new executable
// carrying the current data (spec §4). With no FILENAME, the base name
// defaults to the running executable's own name (naming.md's table,
// "ファイル名省略、実行中バイナリ名がベース"). --sqlite (needs
// engine.Export) and --timestamp (needs the matching -t/-o CLI flags)
// are deferred (PLAN.md Phase 1 Step 4 scope note).
func cmdSnapshot(db *engine.DB, self string, args []string, out io.Writer) error {
	base := filepath.Base(self)
	if len(args) > 0 {
		base = args[0]
	}
	path := snapshotFilename(base, runtime.GOOS)

	if err := db.Snapshot(path); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s\n", path)
	return nil
}

// cmdOverwrite implements ".overwrite": save into this executable and,
// only on success, let the caller know the REPL should exit (spec §4).
func cmdOverwrite(db *engine.DB, out io.Writer) error {
	if err := db.Overwrite(); err != nil {
		return err
	}
	fmt.Fprintln(out, "Overwrite ok, exiting.")
	return nil
}

// cmdExit implements ".exit [CODE]" / ".quit [CODE]" (spec §3): exit
// normally (0) or with the given code.
func cmdExit(args []string) (dotCmdResult, error) {
	if len(args) == 0 {
		return dotCmdResult{exit: true}, nil
	}
	code, err := strconv.Atoi(args[0])
	if err != nil {
		return dotCmdResult{}, fmt.Errorf("invalid exit code %q", args[0])
	}
	return dotCmdResult{exit: true, code: code}, nil
}

// cmdHelp implements ".help": list the dot commands this Phase 1 build
// actually supports. It intentionally does not list commands from later
// phases -- .claude/rules/cli-output.md's read-only-mode precedent
// applies the same principle: don't advertise what can't be run yet.
func cmdHelp(out io.Writer) {
	fmt.Fprint(out, `.tables                 List tables
.schema [TABLE]         Show CREATE statements
.snapshot [FILENAME]    Save a new executable with the current data
.overwrite              Save into this executable and exit
.exit [CODE]            Exit (alias: .quit)
.help                   Show this message
`)
}
