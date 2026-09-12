package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// handleDotCommand parses and runs one dot command, returning whether the
// REPL should now stop and, if so, with which exit code (spec §3
// ".exit [CODE]"). It takes no REPL-specific state beyond r itself, so
// later phases can call it directly from batch execution or a stdio op
// handler without duplicating the logic (naming.md,
// .claude/rules/directory-structure.md: "3つの実行モードは同じドット
// コマンド実装を共有する"). Commands that just run SQL (.tables/.schema)
// go through r.sess, the REPL's own Session; .snapshot/.overwrite/.load
// are DB-level operations (they replace or persist the whole live
// database, not just run a statement on it) and go through r.db
// directly.
func (r *repl) handleDotCommand(line string) (exit bool, code int) {
	fields := strings.Fields(line)
	name, args := fields[0], fields[1:]

	var err error
	switch name {
	case ".tables":
		err = r.cmdTables()
	case ".schema":
		err = r.cmdSchema(args)
	case ".mode":
		err = r.cmdMode(args)
	case ".headers":
		err = r.cmdHeaders(args)
	case ".snapshot":
		err = r.cmdSnapshot(args)
	case ".overwrite":
		if err = r.cmdOverwrite(); err == nil {
			exit = true
		}
	case ".exit", ".quit":
		exit, code, err = cmdExit(args)
	case ".help":
		cmdHelp(r.out)
	default:
		err = fmt.Errorf("unknown command: %q (try .help)", name)
	}
	if err != nil {
		fmt.Fprintln(r.errw, "Error:", err)
	}
	return exit, code
}

// cmdTables implements ".tables": list table names, excluding SQLite's
// own internal sqlite_* tables (spec §3).
func (r *repl) cmdTables() error {
	rows, err := r.sess.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		fmt.Fprintln(r.out, name)
	}
	return rows.Err()
}

// cmdSchema implements ".schema [TABLE]": print CREATE statements, all
// of them by default or only those naming/belonging to TABLE (matching
// an index's tbl_name too, like sqlite3's own ".schema") (spec §3).
func (r *repl) cmdSchema(args []string) error {
	query := `SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite\_%' ESCAPE '\'`
	var queryArgs []any
	if len(args) > 0 {
		query += ` AND (name = ? OR tbl_name = ?)`
		queryArgs = append(queryArgs, args[0], args[0])
	}
	query += ` ORDER BY rowid`

	rows, err := r.sess.Query(query, queryArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var sql string
		if err := rows.Scan(&sql); err != nil {
			return err
		}
		fmt.Fprintf(r.out, "%s;\n", sql)
	}
	return rows.Err()
}

// cmdMode implements ".mode MODE" (spec §3). Switching into column mode
// turns headers on automatically, matching sqlite3 -- a bare table of
// values with no header row is a lot less useful in column mode, where
// the whole point is readable alignment. headers can still be turned
// back off explicitly afterward (spec §3's .mode/.headers table).
func (r *repl) cmdMode(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: .mode MODE (list|column|csv|json|line)")
	}
	mode := outputMode(strings.ToLower(args[0]))
	if !validOutputModes[mode] {
		return fmt.Errorf("unknown mode %q. available modes: list, column, csv, json, line", args[0])
	}
	r.mode = mode
	if mode == modeColumn {
		r.headers = true
	}
	return nil
}

// cmdHeaders implements ".headers on|off" (spec §3).
func (r *repl) cmdHeaders(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf(`usage: .headers on|off`)
	}
	switch strings.ToLower(args[0]) {
	case "on":
		r.headers = true
	case "off":
		r.headers = false
	default:
		return fmt.Errorf("expected \"on\" or \"off\", got %q", args[0])
	}
	return nil
}

// cmdSnapshot implements ".snapshot [FILENAME]": save a new executable
// carrying the current data (spec §4). With no FILENAME, the base name
// defaults to the running executable's own name (naming.md's table,
// "ファイル名省略、実行中バイナリ名がベース"). --sqlite/--timestamp are
// Step 4 scope.
func (r *repl) cmdSnapshot(args []string) error {
	base := filepath.Base(r.self)
	if len(args) > 0 {
		base = args[0]
	}
	path := snapshotFilename(base, runtime.GOOS)

	if err := r.db.Snapshot(path); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Wrote %s\n", path)
	return nil
}

// cmdOverwrite implements ".overwrite": save into this executable and,
// only on success, let the caller know the REPL should exit (spec §4).
func (r *repl) cmdOverwrite() error {
	if err := r.db.Overwrite(); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "Overwrite ok, exiting.")
	return nil
}

// cmdExit implements ".exit [CODE]" / ".quit [CODE]" (spec §3): exit
// normally (0) or with the given code.
func cmdExit(args []string) (exit bool, code int, err error) {
	if len(args) == 0 {
		return true, 0, nil
	}
	code, err = strconv.Atoi(args[0])
	if err != nil {
		return false, 0, fmt.Errorf("invalid exit code %q", args[0])
	}
	return true, code, nil
}

// cmdHelp implements ".help": list the dot commands this build actually
// supports. It intentionally does not list commands from later phases --
// .claude/rules/cli-output.md's read-only-mode precedent applies the
// same principle: don't advertise what can't be run yet.
func cmdHelp(out interface{ Write([]byte) (int, error) }) {
	fmt.Fprint(out, `.tables                 List tables
.schema [TABLE]         Show CREATE statements
.mode MODE              Set output mode: list|column|csv|json|line
.headers on|off         Show column names in output
.snapshot [FILENAME]    Save a new executable with the current data
.overwrite              Save into this executable and exit
.exit [CODE]            Exit (alias: .quit)
.help                   Show this message
`)
}
