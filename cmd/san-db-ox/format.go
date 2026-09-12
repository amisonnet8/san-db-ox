package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// outputMode is a REPL display mode (spec §3 ".mode"). The set is
// deliberately smaller than sqlite3's full list (which also has
// quote/insert/tabs/markdown/box/html): .claude/rules/cli-output.md
// rules out decorative modes in favor of plain, pipe-friendly output.
type outputMode string

const (
	modeList   outputMode = "list"
	modeColumn outputMode = "column"
	modeCSV    outputMode = "csv"
	modeJSON   outputMode = "json"
	modeLine   outputMode = "line"
)

var validOutputModes = map[outputMode]bool{
	modeList: true, modeColumn: true, modeCSV: true, modeJSON: true, modeLine: true,
}

// printRows renders rows (cols already read via rows.Columns()) to r.out
// in r.mode, matching sqlite3 CLI's own output where reasonable
// (.claude/rules/cli-output.md). Scan/iteration errors are printed to
// r.errw by forEachRow; this is not itself an error return, matching
// dispatchDotCommand's existing "print and move on" style.
func (r *repl) printRows(rows *sql.Rows, cols []string) {
	switch r.mode {
	case modeColumn:
		r.printRowsColumn(rows, cols)
	case modeCSV:
		r.printRowsCSV(rows, cols)
	case modeJSON:
		r.printRowsJSON(rows, cols)
	case modeLine:
		r.printRowsLine(rows, cols)
	default: // modeList, and the zero value
		r.printRowsList(rows, cols)
	}
}

// forEachRow scans every remaining row into a reused []any and calls fn
// with it. A Scan or rows.Err() error is printed to r.errw and stops
// iteration early, same as the rest of this package's dot-command error
// handling.
func (r *repl) forEachRow(rows *sql.Rows, cols []string, fn func(vals []any)) {
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Fprintln(r.errw, "Error:", err)
			return
		}
		fn(vals)
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintln(r.errw, "Error:", err)
	}
}

// printRowsList renders sqlite3's default "list" style: one row per
// line, columns separated by "|", NULL as an empty string.
func (r *repl) printRowsList(rows *sql.Rows, cols []string) {
	if r.headers {
		fmt.Fprintln(r.out, strings.Join(cols, "|"))
	}
	r.forEachRow(rows, cols, func(vals []any) {
		fmt.Fprintln(r.out, formatRow(vals))
	})
}

// formatRow joins vals with "|", "list" mode's separator (spec §3).
func formatRow(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = formatValue(v)
	}
	return strings.Join(parts, "|")
}

// printRowsCSV renders RFC 4180 CSV with CRLF line endings (spec §3;
// .claude/rules/cli-output.md: CSV is the one mode that uses CRLF, to
// match sqlite3's own ".mode csv" and common CSV-parser expectations).
// encoding/csv handles quoting fields that need it; values are rendered
// the same way "list" mode does (formatValue) before being handed to it.
func (r *repl) printRowsCSV(rows *sql.Rows, cols []string) {
	w := csv.NewWriter(r.out)
	w.UseCRLF = true
	if r.headers {
		w.Write(cols)
	}
	r.forEachRow(rows, cols, func(vals []any) {
		fields := make([]string, len(vals))
		for i, v := range vals {
			fields[i] = formatValue(v)
		}
		w.Write(fields)
	})
	w.Flush()
}

// printRowsColumn renders sqlite3's "column" style: values left-aligned
// and padded to each column's widest value (header included in the
// width calculation even when the header itself is not shown). All rows
// are buffered first since the column widths depend on every value, not
// just the header (spec §3, .claude/rules/cli-output.md).
//
// Unlike list/csv, whether the header row (and its "-" underline) is
// drawn at all follows r.headers, not a mode-specific rule: switching
// into column mode turns .headers on automatically (cmdMode, dotcmd.go),
// but the user can still turn it back off afterward (spec §3's
// .mode/.headers table) -- this function just honors whatever r.headers
// is at the time it runs.
func (r *repl) printRowsColumn(rows *sql.Rows, cols []string) {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	var allRows [][]string
	r.forEachRow(rows, cols, func(vals []any) {
		fields := make([]string, len(vals))
		for i, v := range vals {
			fields[i] = formatValue(v)
			if len(fields[i]) > widths[i] {
				widths[i] = len(fields[i])
			}
		}
		allRows = append(allRows, fields)
	})

	if r.headers {
		printPadded(r.out, cols, widths)
		underline := make([]string, len(cols))
		for i, w := range widths {
			underline[i] = strings.Repeat("-", w)
		}
		printPadded(r.out, underline, widths)
	}
	for _, fields := range allRows {
		printPadded(r.out, fields, widths)
	}
}

func printPadded(w io.Writer, fields []string, widths []int) {
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = f + strings.Repeat(" ", widths[i]-len(f))
	}
	fmt.Fprintln(w, strings.TrimRight(strings.Join(parts, "  "), " "))
}

// printRowsLine renders sqlite3's "line" style: one "  name = value" per
// column (names right-aligned so every "=" lines up), a blank line
// between rows. Always shows column names -- .headers has no effect on
// this mode (spec §3's .mode/.headers table), since the format is
// nothing but labeled values.
func (r *repl) printRowsLine(rows *sql.Rows, cols []string) {
	width := 0
	for _, c := range cols {
		if len(c) > width {
			width = len(c)
		}
	}
	first := true
	r.forEachRow(rows, cols, func(vals []any) {
		if !first {
			fmt.Fprintln(r.out)
		}
		first = false
		for i, v := range vals {
			fmt.Fprintf(r.out, "%*s = %s\n", width, cols[i], formatValue(v))
		}
	})
}

// jsonResult is the JSON object one .mode json result set renders as,
// shared shape with Phase 4's stdio protocol "query" response (spec §7).
type jsonResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// printRowsJSON renders one line of JSON per result set:
// {"columns":[...],"rows":[[...]]} (spec §3, §7). Always includes
// columns regardless of r.headers (spec §3's .mode/.headers table) --
// the format has no headerless variant. Values go through jsonValue
// (value.go), the same conversion Phase 4's stdio protocol will use.
func (r *repl) printRowsJSON(rows *sql.Rows, cols []string) {
	result := jsonResult{Columns: cols, Rows: [][]any{}}
	var scanErr error
	r.forEachRow(rows, cols, func(vals []any) {
		row := make([]any, len(vals))
		for i, v := range vals {
			jv, err := jsonValue(v)
			if err != nil && scanErr == nil {
				scanErr = err
			}
			row[i] = jv
		}
		result.Rows = append(result.Rows, row)
	})
	if scanErr != nil {
		fmt.Fprintln(r.errw, "Error:", scanErr)
		return
	}

	b, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintln(r.errw, "Error:", err)
		return
	}
	fmt.Fprintln(r.out, string(b))
}

// formatValue renders one column value in the "human-readable text"
// styles (list/csv/column/line): NULL as an empty string, BLOB ([]byte)
// as its raw bytes (matching sqlite3 CLI's own behavior in these modes
// -- .claude/rules/cli-output.md: "迷ったらsqlite3コマンドの挙動を確認
// し"), everything else via fmt's default verb (int64 prints as a bare
// integer, float64 as Go's %v float formatting -- sqlite3 itself is not
// exactly matched for REAL rendering here, only json mode's jsonReal
// commits to the exact "always has a decimal point" contract, spec §7).
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
