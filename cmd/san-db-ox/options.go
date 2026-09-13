package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// options holds the CLI startup options that configure REPL/batch/stdio
// behavior after start (spec §12).
type options struct {
	mode             outputMode
	snapshotAs       string
	quiet            bool
	timestamp        bool
	snapshotInterval time.Duration
	command          stringSliceFlag // -c/--command, repeatable, run in order (spec §5)
	serveStdio       bool            // --serve-stdio, no short form (naming.md)
	readOnly         bool            // -r/--read-only (spec §2)
	help             bool
	version          bool
}

// stringSliceFlag implements flag.Value so -c/--command can be given more
// than once, appending each value in the order given (spec §5:
// "複数回指定でき、指定順に実行"). flag.StringVar only binds a single
// value, so a plain string field cannot represent this.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// parseOptions parses args (os.Args[1:]) against the flags spec §12
// defines for this phase. Each flag is registered under both its short
// and long form bound to the same variable -- flag.FlagSet has no
// built-in alias mechanism, so this is the standard way to accept both
// spellings. flag.ContinueOnError (not the package-level flag.Parse's
// ExitOnError) reports a bad flag through the returned error instead of
// calling os.Exit itself, keeping run() (main.go) the sole place that
// decides process exit codes -- necessary for testability, and so a
// library embedder never sees a surprise process exit.
//
// -h/--help and -v/--version are registered as ordinary bool flags
// (opts.help/opts.version) rather than left to flag.FlagSet's own
// automatic "-h" handling: defining "h" ourselves overrides that default
// behavior, so both flags are reported back to the caller to act on
// (main.go prints san-db-ox's own usage/version text, not flag's).
func parseOptions(args []string, errw io.Writer) (*options, error) {
	fs := flag.NewFlagSet("san-db-ox", flag.ContinueOnError)
	fs.SetOutput(errw)
	fs.Usage = func() { printUsage(errw) }

	opts := &options{}

	var modeStr string
	fs.StringVar(&modeStr, "m", string(modeList), "output format: list|column|csv|json|line")
	fs.StringVar(&modeStr, "mode", string(modeList), "output format: list|column|csv|json|line")
	fs.StringVar(&opts.snapshotAs, "o", "", "default filename for .snapshot")
	fs.StringVar(&opts.snapshotAs, "snapshot-as", "", "default filename for .snapshot")
	fs.BoolVar(&opts.quiet, "q", false, "suppress the startup banner")
	fs.BoolVar(&opts.quiet, "quiet", false, "suppress the startup banner")
	fs.BoolVar(&opts.timestamp, "t", false, "append a timestamp to saved filenames")
	fs.BoolVar(&opts.timestamp, "timestamp", false, "append a timestamp to saved filenames")
	fs.DurationVar(&opts.snapshotInterval, "i", 0, "periodic snapshot interval, e.g. 5m")
	fs.DurationVar(&opts.snapshotInterval, "snapshot-interval", 0, "periodic snapshot interval, e.g. 5m")
	fs.Var(&opts.command, "c", "run this SQL/dot-command and exit (repeatable)")
	fs.Var(&opts.command, "command", "run this SQL/dot-command and exit (repeatable)")
	fs.BoolVar(&opts.serveStdio, "serve-stdio", false, "serve the stdio protocol (spec §7)")
	fs.BoolVar(&opts.readOnly, "r", false, "reject all write operations")
	fs.BoolVar(&opts.readOnly, "read-only", false, "reject all write operations")
	fs.BoolVar(&opts.help, "h", false, "show help and exit")
	fs.BoolVar(&opts.help, "help", false, "show help and exit")
	fs.BoolVar(&opts.version, "v", false, "show version and exit")
	fs.BoolVar(&opts.version, "version", false, "show version and exit")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unrecognized argument: %s", fs.Arg(0))
	}

	mode := outputMode(modeStr)
	if !validOutputModes[mode] {
		return nil, fmt.Errorf("unknown mode %q (try list, column, csv, json, line)", modeStr)
	}
	opts.mode = mode

	// Mode exclusivity and --read-only's incompatibility with periodic
	// saving are usage errors (exit code 2), not runtime failures --
	// spec §12's exclusivity table.
	if opts.serveStdio && len(opts.command) > 0 {
		return nil, fmt.Errorf("--serve-stdio and -c/--command cannot be used together")
	}
	if opts.readOnly && opts.snapshotInterval > 0 {
		return nil, fmt.Errorf("--read-only and -i/--snapshot-interval cannot be used together")
	}

	return opts, nil
}
