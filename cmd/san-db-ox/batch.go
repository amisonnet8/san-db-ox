package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/amisonnet8/san-db-ox/engine"
)

// runBatch opens this run's Session (openSession, repl.go) and executes
// chunks -- either the -c/--command values in the order given, or (via
// runBatchFromReader below) the whole of stdin as a single chunk -- as
// one non-interactive script (spec §5). Unlike the REPL, an error aborts
// the run immediately with exit code 1, matching "エラー発生時は即座に
// 処理全体を中断する" -- CI relying on this to fail loudly on a broken
// migration script is the whole point of batch mode.
func runBatch(db *engine.DB, self string, opts *options, chunks []string, out, errw io.Writer) int {
	sess, err := openSession(context.Background(), db, opts)
	if err != nil {
		fmt.Fprintln(errw, "Error:", err)
		return 1
	}
	defer sess.Close()

	r := newRepl(db, sess, self, opts, false, out, errw)
	return r.runBatch(chunks)
}

// runBatchFromReader implements the "stdin as a SQL script" form of spec
// §5 (a non-interactive stdin with no -c given): it is read in full up
// front, then handed to runBatch as a single chunk -- the same treatment
// a single -c value gets.
func runBatchFromReader(db *engine.DB, self string, opts *options, in io.Reader, out, errw io.Writer) int {
	data, err := io.ReadAll(in)
	if err != nil {
		fmt.Fprintln(errw, "Error:", err)
		return 1
	}
	return runBatch(db, self, opts, []string{string(data)}, out, errw)
}

// runBatch runs each chunk to completion, in order, stopping at the
// first one that aborts (an error, or a dot command requesting exit).
// Each chunk is independent: spec §5 deliberately does not carry
// unterminated SQL text over from one -c argument into the next (see
// runBatchChunk's own doc comment for why).
func (r *repl) runBatch(chunks []string) (code int) {
	for _, chunk := range chunks {
		if code, stop := r.runBatchChunk(chunk); stop {
			return code
		}
	}
	return 0
}

// runBatchChunk processes one chunk -- one -c value, or the whole of
// stdin (runBatchFromReader above) -- to completion. It has the same
// per-line shape as the REPL's run() (repl.go): accumulate SQL text in
// buf until splitComplete recognizes complete statements, and recognize
// a dot command only on a line where buf is empty (sqlite3's own rule, a
// "." mid-statement is just text) -- but with no prompts, no Ctrl+C
// state machine, and an error aborting the whole run rather than just
// being printed.
//
// A chunk is treated as one complete, independent unit: at its own end,
// a trailing statement missing its ";" is still implicitly terminated
// and run (spec §5's "sqlite3 CLI's -cmd doesn't require a final ';'"
// rule) -- but that implicit termination never reaches into the next
// chunk. Two -c arguments are two separate commands, not one statement
// split across them (spec §5; this project's own multi -c documentation
// example, docs/usage/cli-options_ja.md, relies on exactly this: none of
// its several -c values end in ";").
func (r *repl) runBatchChunk(chunk string) (code int, stop bool) {
	lines := strings.Split(chunk, "\n")

	var buf strings.Builder
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if buf.Len() == 0 && strings.HasPrefix(trimmed, ".") {
			exit, code, err := r.handleDotCommand(trimmed) // error already printed to r.errw by handleDotCommand
			if err != nil {
				return 1, true
			}
			if exit {
				return code, true
			}
			continue
		}
		if buf.Len() == 0 && trimmed == "" {
			continue
		}

		buf.WriteString(line)
		buf.WriteString("\n")

		stmts, remainder := splitComplete(buf.String())
		for _, stmt := range stmts {
			if err := r.execSQL(stmt); err != nil { // error already printed to r.errw by execSQL
				return 1, true
			}
		}
		// A whitespace-only remainder (e.g. just the "\n" splitComplete
		// leaves behind right after a statement's ";") must reset buf to
		// truly empty, not to that whitespace -- otherwise buf.Len() == 0
		// never holds again for the rest of this chunk, and the next
		// line's dot command (if any) is fed to execSQL as SQL text
		// instead of recognized as a dot command (mirrors repl.go's run()
		// doing the same reset).
		buf.Reset()
		if strings.TrimSpace(remainder) != "" {
			buf.WriteString(remainder)
		}

		if i != len(lines)-1 {
			continue
		}
		// Last line of this chunk: implicitly terminate a trailing
		// statement missing its ";" (spec §5), scoped to this chunk only.
		trailing := strings.TrimSpace(remainder)
		if trailing == "" {
			continue
		}
		candidate := trailing + ";"
		if ok, cerr := engine.Complete(candidate); cerr != nil || !ok {
			fmt.Fprintf(r.errw, "Error: incomplete SQL statement: %s\n", trailing)
			return 1, true
		}
		if err := r.execSQL(candidate); err != nil {
			return 1, true
		}
	}
	return 0, false
}
