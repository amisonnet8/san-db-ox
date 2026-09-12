package main

import "path/filepath"

// snapshotFilename applies the applicable part of the single file-naming
// rule in naming.md for ".snapshot": if base has no extension, append
// ".exe" on Windows (an extension-less name is left as-is on other
// OSes). The rule's timestamp-insertion half (--timestamp) is deferred
// until the matching -t/-o CLI flags land (naming.md; PLAN.md Phase 1
// Step 4 scope note).
func snapshotFilename(base, goos string) string {
	if filepath.Ext(base) == "" && goos == "windows" {
		return base + ".exe"
	}
	return base
}
