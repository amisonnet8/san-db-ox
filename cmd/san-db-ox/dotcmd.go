package main

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/amisonnet8/san-db-ox/engine"
)

// handleDotCommand parses and runs one dot command, returning whether the
// caller should now stop, with which exit code (spec §3 ".exit [CODE]"),
// and any error the command produced. It takes no REPL-specific state
// beyond r itself, so batch execution (batch.go) and the stdio protocol's
// op handlers (stdio.go) call it directly without duplicating the logic
// (naming.md, .claude/rules/directory-structure.md: "3つの実行モードは
// 同じドットコマンド実装を共有する"). Commands that just run SQL
// (.tables/.schema) go through r.sess, the REPL's own Session;
// .snapshot/.overwrite/.load are DB-level operations (they replace or
// persist the whole live database, not just run a statement on it) and
// go through r.db directly.
//
// The error is returned in addition to being printed to r.errw here (not
// instead of): the REPL's own line loop (repl.go) ignores it and keeps
// going, matching sqlite3's "show the error, stay in the shell"
// behavior, while batch execution (spec §5: "エラー発生時は即座に処理
// 全体を中断する") uses it to abort immediately. Printing unconditionally
// here, rather than leaving it to each caller, keeps that one line of
// user-facing text from being written twice or not at all depending on
// which mode is calling.
func (r *repl) handleDotCommand(line string) (exit bool, code int, err error) {
	fields := strings.Fields(line)
	name, args := fields[0], fields[1:]

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
	case ".load":
		err = r.cmdLoad(args)
	case ".dump":
		err = r.cmdDump(args)
	case ".import":
		err = r.cmdImport(args)
	case ".exit", ".quit":
		exit, code, err = cmdExit(args)
	case ".help":
		r.cmdHelp()
	default:
		err = fmt.Errorf("unknown command: %q (try .help)", name)
	}
	if err != nil {
		fmt.Fprintln(r.errw, "Error:", err)
	}
	return exit, code, err
}

// cmdTables implements ".tables": print listTables's result one name per
// line (spec §3).
func (r *repl) cmdTables() error {
	names, err := r.listTables()
	if err != nil {
		return err
	}
	for _, name := range names {
		fmt.Fprintln(r.out, name)
	}
	return nil
}

// listTables is ".tables"'s logic, split out from its text rendering
// (cmdTables above) so the stdio "tables" op (stdio.go) can reuse it and
// return the names as a JSON array instead of printing them
// (.claude/rules/directory-structure.md: "3つの実行モードは同じドット
// コマンド実装を共有する"). Excludes SQLite's own internal sqlite_*
// tables.
func (r *repl) listTables() ([]string, error) {
	rows, err := r.sess.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// cmdSchema implements ".schema [TABLE]": print schemaSQL's result, one
// CREATE statement per line (spec §3).
func (r *repl) cmdSchema(args []string) error {
	table := ""
	if len(args) > 0 {
		table = args[0]
	}
	stmts, err := r.schemaSQL(table)
	if err != nil {
		return err
	}
	for _, s := range stmts {
		fmt.Fprintf(r.out, "%s;\n", s)
	}
	return nil
}

// schemaSQL is ".schema [TABLE]"'s logic (split out for stdio.go's
// "schema" op, same reasoning as listTables above): CREATE statements,
// all of them by default or only those naming/belonging to table
// (matching an index's tbl_name too, like sqlite3's own ".schema") (spec
// §3). The returned statements do not include a trailing ";" -- callers
// add their own separator (cmdSchema's text rendering, or an op's JSON
// array where each element is one statement).
func (r *repl) schemaSQL(table string) ([]string, error) {
	query := `SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite\_%' ESCAPE '\'`
	var queryArgs []any
	if table != "" {
		query += ` AND (name = ? OR tbl_name = ?)`
		queryArgs = append(queryArgs, table, table)
	}
	query += ` ORDER BY rowid`

	rows, err := r.sess.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stmts []string
	for rows.Next() {
		var sql string
		if err := rows.Scan(&sql); err != nil {
			return nil, err
		}
		stmts = append(stmts, sql)
	}
	return stmts, rows.Err()
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

// cmdSnapshot implements ".snapshot [FILENAME] [--sqlite] [--timestamp]"
// (spec §4, §6): parses the arguments and prints doSnapshot's result.
// FILENAME and the two flags may appear in any order (matching the
// spec's own examples, ".snapshot bug_123 --timestamp" / ".snapshot
// bug_123 --sqlite --timestamp").
func (r *repl) cmdSnapshot(args []string) error {
	filename := ""
	asSQLite := false
	withTimestamp := r.opts != nil && r.opts.timestamp
	for _, a := range args {
		switch a {
		case "--sqlite":
			asSQLite = true
		case "--timestamp":
			withTimestamp = true
		default:
			if filename != "" {
				return fmt.Errorf("usage: .snapshot [FILENAME] [--sqlite] [--timestamp]")
			}
			filename = a
		}
	}

	path, err := r.doSnapshot(filename, asSQLite, withTimestamp)
	if err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Wrote %s\n", path)
	return nil
}

// doSnapshot is ".snapshot"'s logic (split out for stdio.go's "snapshot"
// op, same reasoning as listTables above): save a new executable
// carrying the current data, or (asSQLite) a plain SQLite file instead,
// and return the path written. With filename == "", the base name
// defaults to -o/--snapshot-as if the process was started with one,
// otherwise the running executable's own name (naming.md's table,
// "ファイル名省略、実行中バイナリ名がベース"; defaultSnapshotBase,
// filename.go). withTimestamp here overrides (never merely ORs with) the
// startup -t/--timestamp default, matching spec §12: "その場で上書き
// 指定できる".
func (r *repl) doSnapshot(filename string, asSQLite, withTimestamp bool) (path string, err error) {
	// Rejected here rather than left to db.Snapshot/db.Export: --read-only
	// blocks ".snapshot --sqlite" too (spec §2), since writing a plain
	// SQLite file to an arbitrary server-side path is exactly the kind of
	// filesystem write a read-only external deployment (§8) must not
	// allow, even though it never touches the live DB's own content.
	if r.readOnly() {
		return "", ErrReadOnly
	}

	base := filename
	if base == "" {
		base = defaultSnapshotBase(r.self, r.opts)
	}
	path = snapshotFilename(base, withTimestamp, asSQLite, time.Now(), runtime.GOOS)

	if asSQLite {
		err = r.db.Export(path)
	} else {
		err = r.db.Snapshot(path)
	}
	if err != nil {
		return "", err
	}
	return path, nil
}

// cmdLoad implements ".load FILE" (spec §4, §6): calls doLoad and prints
// its warning (if any) and a confirmation line.
func (r *repl) cmdLoad(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: .load FILENAME")
	}
	path := args[0]

	warning, err := r.doLoad(path)
	if err != nil {
		return err
	}
	if warning != "" {
		fmt.Fprintln(r.errw, warning)
	}
	fmt.Fprintf(r.out, "Loaded data from %s\n", path)
	return nil
}

// doLoad is ".load"'s logic (split out for stdio.go's "load" op, same
// reasoning as listTables above): replace the in-memory DB with the data
// in path (a SanDBox executable or a plain SQLite file, auto-detected).
// A footer format-Version mismatch is reported back as a warning string,
// not a rejection (spec §4: "警告を表示した上で処理を続行する"); engine
// itself never logs (§10's division of responsibility), so this calls
// Inspect itself to detect the mismatch before Load runs, and leaves
// deciding where the warning goes (stderr for the REPL/batch, dropped
// silently for stdio -- spec §7 defines no field for it) to the caller.
// The check only applies to KindExecutable: FileInfo.Version is defined
// as always zero for every other kind (inspect.go), so comparing it
// against engine.FormatVersion for a KindSQLite file would spuriously
// warn on every single plain SQLite file loaded. Inspect failing here (a
// missing file, a directory, ...) is not itself reported -- db.Load
// below fails on the same input and surfaces a clearer error for it.
func (r *repl) doLoad(path string) (warning string, err error) {
	if r.readOnly() {
		return "", ErrReadOnly
	}

	if info, ierr := engine.Inspect(path); ierr == nil &&
		info.Kind == engine.KindExecutable && info.HasData && info.Version != engine.FormatVersion {
		warning = fmt.Sprintf("Warning: %s has SanDBox format version %d; this build is version %d.", path, info.Version, engine.FormatVersion)
	}

	if err := r.db.Load(path); err != nil {
		return "", err
	}
	return warning, nil
}

// cmdOverwrite implements ".overwrite": calls doOverwrite and, only on
// success, prints the confirmation line and lets the caller know the
// REPL should exit (spec §4).
func (r *repl) cmdOverwrite() error {
	if err := r.doOverwrite(); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "Overwrite ok, exiting.")
	return nil
}

// doOverwrite is ".overwrite"'s logic (split out for stdio.go's
// "overwrite" op, same reasoning as listTables above): save into this
// executable. Split out specifically so stdio.go never goes through
// cmdOverwrite's own r.out write above -- stdio's stdout carries protocol
// responses only (spec §0, §7), never that REPL-facing confirmation
// text.
func (r *repl) doOverwrite() error {
	if r.readOnly() {
		return ErrReadOnly
	}
	return r.db.Overwrite()
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
// supports, substituting the --read-only variant (which omits every
// rejected save operation, spec §2/§13) when r's run was started with
// --read-only -- don't advertise a command the user can't actually run
// (.claude/rules/cli-output.md's precedent, applied here too).
func (r *repl) cmdHelp() {
	if r.readOnly() {
		cmdHelpReadOnly(r.out)
		return
	}
	cmdHelp(r.out)
}

// cmdHelp lists every dot command this build actually supports. It
// intentionally does not list commands from later phases -- the same
// "don't advertise what can't be run" principle above applies to
// still-unimplemented features too.
func cmdHelp(out interface{ Write([]byte) (int, error) }) {
	fmt.Fprint(out, `.tables                 List tables
.schema [TABLE]         Show CREATE statements
.mode MODE              Set output mode: list|column|csv|json|line
.headers on|off         Show column names in output
.snapshot [FILE] [--sqlite] [--timestamp]
                        Save the current data as a new executable, or
                        (--sqlite) a plain SQLite file
.overwrite              Save into this executable and exit
.load FILE              Replace the in-memory database with FILE's data
.dump [PATTERN]         Render the schema and data as SQL text
.import FILE TABLE      Import CSV data into TABLE, creating it if needed
.exit [CODE]            Exit (alias: .quit)
.help                   Show this message
`)
}

// cmdHelpReadOnly is cmdHelp's --read-only variant (spec §2): omits
// .snapshot, .overwrite, and .load, the three save/replace operations
// --read-only rejects outright (doSnapshot/doLoad/cmdOverwrite,
// ErrReadOnly). .import is left out too -- it always writes (INSERT,
// possibly CREATE TABLE), so query_only's rejection of it is not worth
// listing as a usable command only to fail every time it's run.
func cmdHelpReadOnly(out interface{ Write([]byte) (int, error) }) {
	fmt.Fprint(out, `.tables                 List tables
.schema [TABLE]         Show CREATE statements
.mode MODE              Set output mode: list|column|csv|json|line
.headers on|off         Show column names in output
.dump [PATTERN]         Render the schema and data as SQL text
.exit [CODE]            Exit (alias: .quit)
.help                   Show this message
`)
}
