package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/amisonnet8/san-db-ox/engine"
)

// prompt is the REPL's line prompt: the display name (spec §0's naming
// slot table), matching the startup banner's "SanDBox v..." line rather
// than any of the operational lowercase tiers (naming.md) -- settled in
// PLAN.md "未確認事項" #2.
const prompt = "SanDBox> "

// continuationPrompt is shown while a multi-line SQL statement is still
// being typed, i.e. no ";" recognized as a statement boundary has been
// seen yet (spec §0, §3; sqlite3 CLI's own "   ...> ", same width as
// prompt).
const continuationPrompt = "   ...> "

// repl holds everything one REPL run needs across the lifetime of the
// process: the DB-level handle (for .snapshot/.overwrite/.load, which
// replace or persist the whole live database), the one Session this REPL
// holds for its entire run (see runREPL's doc comment), the running
// executable's own path (.snapshot's default name, naming.md), the
// parsed startup options (Step 3: -m/-o/-t/-q/-i defaults), and whether
// stdin looks like a terminal (spec §13: a non-interactive REPL run
// prints no prompt or banner and installs no SIGINT handler).
type repl struct {
	db          *engine.DB
	sess        *engine.Session
	self        string
	opts        *options
	interactive bool
	mode        outputMode // .mode (format.go, Step 2); zero value behaves as modeList
	headers     bool       // .headers (Step 2)

	// interrupts is non-nil only for an interactive session
	// (interrupt.go): SIGINT keeps its default (process-terminating)
	// behavior for non-interactive/piped input, so it stays nil there.
	interrupts *replInterrupts

	out  io.Writer
	errw io.Writer
}

// openSession opens one dedicated engine.Session on db -- the single
// connection every mode (REPL, batch execution, stdio) runs its SQL
// through for the run's entire lifetime, spec §2 -- and, if opts
// requests --read-only, immediately switches it into SQLite's
// query_only mode (spec §2's implementation note) so every write SQL
// statement subsequently run on it fails with a query_only error.
// .snapshot/.overwrite/.load are not SQL statements, so query_only does
// not reach them; doSnapshot/doLoad/cmdOverwrite (dotcmd.go) each check
// opts.readOnly themselves.
//
// All SQL runs through this one Session rather than db.Query/db.Exec's
// one-shot pooled connections: database/sql's ResetSession does not roll
// back a transaction left open on a returned connection
// (.claude/rules/sqlite-quirks.md), so running BEGIN through pooled
// one-shot calls would let COMMIT/ROLLBACK silently land on a different
// connection than BEGIN did. .snapshot/.overwrite/.load go through db
// directly instead: they replace or persist the whole live database, not
// run a statement on a Session's transaction.
func openSession(ctx context.Context, db *engine.DB, opts *options) (*engine.Session, error) {
	sess, err := db.Session(ctx)
	if err != nil {
		return nil, err
	}
	if opts != nil && opts.readOnly {
		if _, err := sess.Exec("PRAGMA query_only = ON"); err != nil {
			sess.Close()
			return nil, err
		}
	}
	return sess, nil
}

// newRepl builds a *repl from an already-opened Session, filling in the
// -m/-o/-t startup defaults (options.go) that every mode (REPL,
// batch.go, stdio.go) shares. interactive/interrupts are the only fields
// that vary by mode and so are left to the caller (a batch or stdio run
// is never interactive and never installs interrupts, repl.go's own
// zero-value default for both).
func newRepl(db *engine.DB, sess *engine.Session, self string, opts *options, interactive bool, out, errw io.Writer) *repl {
	mode := modeList
	if opts != nil && opts.mode != "" {
		mode = opts.mode
	}
	headers := mode == modeColumn // .mode column auto-enables .headers, matching cmdMode (dotcmd.go)

	return &repl{
		db: db, sess: sess, self: self, opts: opts,
		interactive: interactive, mode: mode, headers: headers,
		out: out, errw: errw,
	}
}

// runREPL opens this run's Session (openSession above) and reads SQL
// statements and dot commands from in until EOF (Ctrl+D), a command that
// requests exit (spec §3, §13), or a successful ".overwrite".
func runREPL(db *engine.DB, self string, in io.Reader, out, errw io.Writer, interactive bool, opts *options) int {
	sess, err := openSession(context.Background(), db, opts)
	if err != nil {
		fmt.Fprintln(errw, "Error:", err)
		return 1
	}
	defer sess.Close()

	r := newRepl(db, sess, self, opts, interactive, out, errw)
	return r.run(in)
}

// run is the REPL's main loop. SQL text accumulates across lines in buf
// until splitComplete (below) reports no unterminated trailing text left
// -- dot commands are always a single line and only recognized when buf
// is empty (sqlite3's own rule: a "." mid-statement is just SQL text,
// e.g. inside a string literal).
//
// The line reader runs in its own goroutine (interrupt.go's
// startLineReader) so this loop is never blocked inside a stdin read: it
// can instead select between "a line arrived" and "an idle Ctrl+C
// arrived". Only an interactive session installs a SIGINT handler at
// all (spec §13); idleSig stays nil otherwise, which disables that case
// of the select below (a nil channel blocks forever) and leaves SIGINT's
// default (process-terminating) behavior in place for piped input.
func (r *repl) run(in io.Reader) int {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	lr := startLineReader(scanner)

	var idleSig chan struct{}
	if r.interactive {
		var stop func()
		r.interrupts, stop = newReplInterrupts()
		defer stop()
		idleSig = r.interrupts.idleSig
	}

	var buf strings.Builder
	r.printPrompt(false)
	for {
		var line string
		select {
		case l, ok := <-lr.lines:
			if !ok {
				if err := <-lr.err; err != nil {
					fmt.Fprintln(r.errw, "Error:", err)
				}
				if r.interactive {
					fmt.Fprintln(r.out)
				}
				return 0
			}
			line = l
			if r.interrupts != nil {
				r.interrupts.resetOnNewLine()
			}
		case <-idleSig:
			// First Ctrl+C while idle (interrupt.go): discard whatever
			// multi-line statement was being typed and redraw the
			// prompt, matching sqlite3's own behavior. A second,
			// consecutive press instead exits the process directly
			// from onInterrupt, without going through this loop at all.
			buf.Reset()
			r.printPrompt(false)
			continue
		}

		trimmed := strings.TrimSpace(line)

		if buf.Len() == 0 && strings.HasPrefix(trimmed, ".") {
			exit, code, _ := r.handleDotCommand(trimmed) // REPL shows the error (handleDotCommand already did) and keeps going; only batch.go acts on it
			if exit {
				return code
			}
			r.printPrompt(false)
			continue
		}
		if buf.Len() == 0 && trimmed == "" {
			r.printPrompt(false)
			continue
		}

		buf.WriteString(line)
		buf.WriteString("\n")

		stmts, remainder := splitComplete(buf.String())
		for _, stmt := range stmts {
			r.execSQL(stmt) // error already printed to r.errw; REPL just moves on to the next statement
		}
		if strings.TrimSpace(remainder) == "" {
			buf.Reset()
			r.printPrompt(false)
		} else {
			buf.Reset()
			buf.WriteString(remainder)
			r.printPrompt(true)
		}
	}
}

// printPrompt writes prompt or continuationPrompt, but only for an
// interactive session (spec §13): a non-interactive run (piped stdin)
// prints neither, so scripted input's stdout stays exactly the query
// output.
func (r *repl) printPrompt(continuing bool) {
	if !r.interactive {
		return
	}
	if continuing {
		fmt.Fprint(r.out, continuationPrompt)
	} else {
		fmt.Fprint(r.out, prompt)
	}
}

// splitComplete splits text into as many complete SQL statements as
// engine.Complete recognizes, using it as the sole authority on
// statement boundaries -- a ";" inside a string/identifier literal, a
// comment, or a CREATE TRIGGER body's BEGIN...END is not a boundary
// (spec §3, §11's Complete implementation notes: a hand-rolled BEGIN/END
// counter cannot tell such a CASE...END's END from the one that actually
// closes the trigger, .claude/rules/sqlite-quirks.md).
//
// For each ";" in text, in order, splitComplete asks whether the text
// from the last recognized boundary (or the start) up to and including
// that ";" is itself a complete statement. The first prefix engine.Complete
// accepts becomes one statement; scanning then resumes right after it.
// Whatever text is left after the last recognized boundary -- the whole
// input, if none was found -- is returned as remainder for the caller to
// re-submit, with more input appended, on a later call.
//
// A package-level function rather than a *repl method: it holds no
// per-run state (engine.Complete needs no live DB), so both the REPL
// loop (above) and batch execution (batch.go) call it the same way.
func splitComplete(text string) (stmts []string, remainder string) {
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] != ';' {
			continue
		}
		candidate := text[start : i+1]
		ok, err := engine.Complete(candidate)
		if err != nil || !ok {
			continue
		}
		stmts = append(stmts, strings.TrimSpace(candidate))
		start = i + 1
	}
	return stmts, text[start:]
}

// execSQL runs one already-complete statement on r.sess, choosing Query
// over Exec so a non-row-returning statement (DDL/DML) simply yields
// zero columns and prints nothing. Statements that do return columns are
// rendered in r.mode (format.go's printRows) -- list by default,
// matching sqlite3's own ".mode list".
//
// The statement's context is registered with r.interrupts (when
// interactive) for the duration of the call, so a Ctrl+C while it is
// running cancels it instead of terminating the process (interrupt.go).
// engine's TestSessionSurvivesCanceledQuery already established that a
// Session stays usable after a canceled query.
//
// The error is returned in addition to being printed to r.errw here, for
// the same reason handleDotCommand's is (dotcmd.go): the REPL loop
// (run(), above) ignores it and keeps going, while batch.go's runBatch
// uses it to abort immediately (spec §5).
func (r *repl) execSQL(stmt string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if r.interrupts != nil {
		defer r.interrupts.begin(cancel)()
	}

	rows, err := r.sess.QueryContext(ctx, stmt)
	if err != nil {
		fmt.Fprintln(r.errw, "Error:", err)
		return err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		fmt.Fprintln(r.errw, "Error:", err)
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	r.printRows(rows, cols)
	return nil
}
