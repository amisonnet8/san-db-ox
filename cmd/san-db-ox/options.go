package main

import (
	"flag"
	"fmt"
	"io"
	"time"
)

// options holds the CLI startup options that configure REPL/batch
// behavior after start (spec §12). -c/--command, --serve-stdio, and
// -r/--read-only are Phase 4 scope: parseOptions treats them, like any
// other unrecognized flag, as a usage error (exit code 2, spec §5).
type options struct {
	mode             outputMode
	snapshotAs       string
	quiet            bool
	timestamp        bool
	snapshotInterval time.Duration
	help             bool
	version          bool
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

	return opts, nil
}
