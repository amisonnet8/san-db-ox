package main

// options holds the CLI startup options that configure REPL/batch
// behavior after start (spec §12). It is deliberately introduced now as
// an (initially empty) struct so repl (repl.go) has a stable field to
// hold it; Phase 3 Step 3 fills in -m/-o/-t/-q/-i.
type options struct{}
