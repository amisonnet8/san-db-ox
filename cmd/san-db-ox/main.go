// Command san-db-ox is SanDBox's single-binary RDBMS: an interactive SQL
// console (REPL) backed by the engine package, with the data area
// embedded in this very executable (docs/spec/san-db-ox_spec_ja.md §1).
//
// Phase 4 adds batch execution (-c/stdin), the stdio protocol
// (--serve-stdio), and --read-only on top of Phase 3's REPL.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/amisonnet8/san-db-ox/engine"
)

// version is set at build time via -ldflags -X main.version=<tag>
// (.claude/rules/distribution.md); local builds stay "dev". This must
// stay a plain string variable (not computed at init) for -ldflags -X to
// be able to reach it -- it only works on a string variable initialized
// with a constant (see `go help build`).
var version = "dev"

// resolvedVersion implements spec §12's version resolution order: a
// release build's -ldflags value wins if set; otherwise, whatever
// debug.ReadBuildInfo's Main.Version carries -- this covers both a
// binary fetched via `go install <module>@<version>` (which cannot
// receive -ldflags) and a local `go build` run inside a clean git
// checkout, since Go's toolchain stamps Main.Version with a pseudo-version
// derived from the commit in both cases (confirmed by hand: a plain `go
// build` in this repo's own working tree already reports something like
// "v0.0.0-20260913032324-<commit>", not "(devel)", because -buildvcs
// defaults to "auto" and finds the surrounding git checkout). A *dirty*
// checkout (uncommitted changes) gets the same treatment except for a
// "+dirty" suffix, which isDirtyBuild below checks for via the
// "vcs.modified" setting -- accepting that would let an unreproducible,
// possibly half-edited local build masquerade as a real version, so it
// is deliberately excluded and falls through to "dev" instead. The
// banner (banner.go) and the stdio protocol's hello line (stdio.go) both
// call this rather than reading the version variable directly, so all
// three report the same value (spec §12, §13).
func resolvedVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		// "(devel)" is what ReadBuildInfo reports when no version info
		// is available at all (e.g. -buildvcs=false, or building
		// outside any VCS checkout) -- not a real version.
		if v := bi.Main.Version; v != "" && v != "(devel)" && !isDirtyBuild(bi) {
			return v
		}
	}
	return version
}

// isDirtyBuild reports whether bi was built from a VCS checkout with
// uncommitted changes (the "vcs.modified" build setting Go's toolchain
// records alongside a VCS-derived Main.Version).
func isDirtyBuild(bi *debug.BuildInfo) bool {
	for _, s := range bi.Settings {
		if s.Key == "vcs.modified" {
			return s.Value == "true"
		}
	}
	return false
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		// flag.FlagSet already printed its own error and (via fs.Usage)
		// this package's printUsage to errw for a parse error; a value
		// validation error (e.g. an unknown -m MODE) has not, so print
		// both here. Printing usage twice for the former is harmless
		// (both go to stderr) and keeps this branch a single codepath.
		fmt.Fprintln(stderr, "san-db-ox:", err)
		printUsage(stderr)
		return 2
	}
	if opts.help {
		printUsage(stdout)
		return 0
	}
	if opts.version {
		fmt.Fprintf(stdout, "SanDBox %s\n", resolvedVersion())
		return 0
	}

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "san-db-ox:", err)
		return 1
	}

	db, err := engine.OpenSelf()
	if err != nil {
		fmt.Fprintln(stderr, "san-db-ox:", err)
		return 1
	}
	defer db.Close()

	// Mode selection (spec §2, §12): --serve-stdio and -c are mutually
	// exclusive (parseOptions already rejected combining them), and both
	// take priority over stdin's own interactiveness. Absent either, a
	// non-interactive stdin (piped/redirected) is read as a batch script
	// (spec §5); only a live terminal starts the REPL. --snapshot-interval
	// is REPL/stdio-only (spec §5, §12): batch mode warns and ignores it
	// rather than starting the background goroutine at all.
	switch {
	case opts.serveStdio:
		if opts.snapshotInterval > 0 {
			stop := startSnapshotInterval(db, self, opts, stderr)
			defer stop()
		}
		return runStdio(db, self, opts, stdin, stdout, stderr)

	case len(opts.command) > 0:
		warnIntervalIgnored(opts, stderr)
		return runBatch(db, self, opts, opts.command, stdout, stderr)

	case !isInteractive(stdin):
		warnIntervalIgnored(opts, stderr)
		return runBatchFromReader(db, self, opts, stdin, stdout, stderr)

	default:
		if !opts.quiet {
			printBanner(stdout, db, self, opts.readOnly)
		}
		stop := startSnapshotInterval(db, self, opts, stderr)
		defer stop()
		return runREPL(db, self, stdin, stdout, stderr, true, opts)
	}
}

// warnIntervalIgnored implements spec §5's "指定された場合は警告を
// stderrへ出して無視する": --snapshot-interval has no meaning for a
// short-lived batch run, so batch.go never starts the background
// goroutine at all -- this only tells the user why.
func warnIntervalIgnored(opts *options, errw io.Writer) {
	if opts.snapshotInterval > 0 {
		fmt.Fprintln(errw, "Warning: --snapshot-interval has no effect in batch mode; ignoring it.")
	}
}

// isInteractive reports whether in looks like a terminal (spec §13): a
// non-interactive run (piped/redirected stdin) prints no prompt, no
// continuation prompt, and no banner, and gets no SIGINT handler
// (.claude/rules/cli-output.md). Only *os.File carries an OS-level mode
// bit to check; anything else (a test's strings.Reader, for instance) is
// treated as non-interactive, since no test wants a live terminal's
// tty-only stimuli (SIGINT among them, Step 6).
func isInteractive(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: san-db-ox [options]

SanDBox: a portable, single-binary RDBMS with no setup required. Running
with no options starts an interactive SQL console (REPL).

Options:
  -c, --command SQL             Run this SQL/dot-command and exit (repeatable)
  -m, --mode MODE               Output format: list|column|csv|json|line (default list)
  -o, --snapshot-as FILENAME    Default filename for .snapshot
  -q, --quiet                   Suppress the startup banner
  -t, --timestamp               Append a timestamp to saved filenames
  -i, --snapshot-interval DUR   Periodically save a snapshot, e.g. 5m
      --serve-stdio             Serve the stdio protocol (JSON Lines)
  -r, --read-only               Reject all write operations
  -v, --version                 Show version and exit
  -h, --help                    Show this help and exit
`)
}
