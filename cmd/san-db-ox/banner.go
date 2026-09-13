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
// stdio protocol never print it. readOnly appends "(read-only)" to the
// version line (spec §2/§13), the same information the stdio protocol's
// "inspect" op exposes via read_only (stdio.go).
func printBanner(out io.Writer, db *engine.DB, self string, readOnly bool) {
	if readOnly {
		fmt.Fprintf(out, "SanDBox %s (read-only)\n", resolvedVersion())
	} else {
		fmt.Fprintf(out, "SanDBox %s\n", resolvedVersion())
	}
	if db.HasData() {
		fmt.Fprintf(out, "Loaded snapshot: %s\n", filepath.Base(self))
	} else {
		fmt.Fprintln(out, "No embedded data. Starting with an empty in-memory database.")
	}
	fmt.Fprintln(out, `Enter ".help" for usage hints.`)
}
