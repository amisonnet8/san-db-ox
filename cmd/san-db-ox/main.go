// Command san-db-ox is SanDBox's single-binary RDBMS: an interactive SQL
// console (REPL) backed by the engine package, with the data area
// embedded in this very executable (docs/spec/san-db-ox_spec_ja.md §1).
//
// Phase 1 Step 4 keeps this deliberately minimal: no -c/--serve-stdio
// (batch execution, stdio protocol -- Phase ④), no -i/-r
// (--snapshot-interval, --read-only), no -m/-o/-t/-q (output mode,
// snapshot defaults, quiet -- introduced alongside the REPL/CLI features
// they configure). Running with no arguments is the only supported
// invocation besides -h/--help and -v/--version.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/amisonnet8/san-db-ox/engine"
)

// version is set at build time via -ldflags -X main.version=<tag>
// (.claude/rules/distribution.md); local builds stay "dev".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	for _, a := range args {
		switch a {
		case "-h", "--help":
			printUsage(stdout)
			return 0
		case "-v", "--version":
			fmt.Fprintf(stdout, "SanDBox %s\n", version)
			return 0
		default:
			fmt.Fprintf(stderr, "san-db-ox: unrecognized argument: %s\n", a)
			printUsage(stderr)
			return 2
		}
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

	interactive := isInteractive(stdin)
	if interactive {
		printBanner(stdout, db, self)
	}
	return runREPL(db, self, stdin, stdout, stderr, interactive, &options{})
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
  -h, --help     Show this help and exit
  -v, --version  Show version and exit
`)
}
