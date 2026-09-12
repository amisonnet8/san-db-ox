package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/amisonnet8/san-db-ox/engine"
)

// printBanner shows the REPL startup banner: version, embedded-data
// status, and a hint toward .help (spec §13). It is REPL-only
// (.claude/rules/cli-output.md's mode table) -- batch execution and the
// stdio protocol, neither implemented yet in Phase 1, never print it.
func printBanner(out io.Writer, db *engine.DB, self string) {
	fmt.Fprintf(out, "SanDBox %s\n", version)
	if db.HasData() {
		fmt.Fprintf(out, "Loaded snapshot: %s\n", filepath.Base(self))
	} else {
		fmt.Fprintln(out, "No embedded data. Starting with an empty in-memory database.")
	}
	fmt.Fprintln(out, `Enter ".help" for usage hints.`)
}
