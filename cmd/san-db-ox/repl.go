package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/amisonnet8/san-db-ox/engine"
)

// prompt is the REPL's line prompt. "sandbox" is the product name's
// third-tier, length-constrained spelling (naming.md); an interactive
// prompt typed on every line is exactly that kind of context, the same
// reasoning sqlite3's own CLI applies to its "sqlite> " prompt despite
// the product being named "sqlite3". docs/usage/repl-commands_ja.md's
// example already used this spelling as a placeholder; this is now the
// settled choice (PLAN.md "未確認事項" #2).
const prompt = "sandbox> "

// runREPL reads SQL statements and dot commands from in, one line at a
// time, until EOF (Ctrl+D) or a command that requests exit (spec §3,
// §13). Phase 1 keeps this deliberately simple: no multi-line SQL
// continuation (that needs engine.Complete, Phase 2), no Ctrl+C state
// machine (out of Phase 1's scope per PLAN.md), and no output-mode
// formatting beyond the default "list" style (.mode/.headers are Phase 3
// scope). Each line is either a dot command or a single SQL statement,
// executed and printed immediately.
func runREPL(db *engine.DB, self string, in io.Reader, out, errw io.Writer) int {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	fmt.Fprint(out, prompt)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			if strings.HasPrefix(line, ".") {
				res := dispatchDotCommand(db, self, line, out, errw)
				if res.exit {
					return res.code
				}
			} else {
				runSQL(db, line, out, errw)
			}
		}
		fmt.Fprint(out, prompt)
	}
	fmt.Fprintln(out)
	return 0
}

// runSQL executes one SQL statement via db.Query, which works uniformly
// across SELECT/DDL/DML (a non-row-returning statement simply yields
// zero columns), and prints any result rows in the default "list" style:
// "|"-separated, no headers, NULL as an empty string (spec §3's .mode
// list). Full output-mode support (.mode, .headers, other formats) is
// Phase 3 scope.
func runSQL(db *engine.DB, stmt string, out, errw io.Writer) {
	rows, err := db.Query(stmt)
	if err != nil {
		fmt.Fprintln(errw, "Error:", err)
		return
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		fmt.Fprintln(errw, "Error:", err)
		return
	}
	if len(cols) == 0 {
		return
	}

	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Fprintln(errw, "Error:", err)
			return
		}
		fmt.Fprintln(out, formatRow(vals))
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintln(errw, "Error:", err)
	}
}

func formatRow(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = formatValue(v)
	}
	return strings.Join(parts, "|")
}

// formatValue renders one column value in "list" mode: NULL as an empty
// string, BLOB ([]byte) as its raw bytes (matching sqlite3 CLI's own
// list-mode behavior -- .claude/rules/cli-output.md: "迷ったらsqlite3
// コマンドの挙動を確認し"), everything else via fmt's default verb. The
// distinct BLOB/REAL/etc. representations .mode json and the stdio
// protocol require are Phase 3/4 scope (cli-output.md).
func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}
