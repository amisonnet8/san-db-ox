package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const timestampLayout = "20060102150405"

var timestampSuffixPattern = regexp.MustCompile(`_[0-9]{14}$`)

// snapshotFilename applies the single file-naming rule in naming.md for
// ".snapshot" / "--snapshot-as" / "--timestamp":
//
//  1. if withTimestamp, strip any existing "_YYYYMMDDHHMMSS" suffix from
//     base's extension-less part (deduping a re-run) and insert a fresh
//     one right before the extension;
//  2. if the extension was omitted, append ".sqlite" on every OS when
//     asSQLite (spec §6: SQLite output never gets ".exe"), else ".exe"
//     on goos "windows" only (executable output, spec §12).
//
// Ties both halves of the naming.md rule to a single implementation so
// CLI startup (-o/-t), ".snapshot [FILE] [--sqlite] [--timestamp]", and
// (Step 3) --snapshot-interval's periodic saves can never drift apart.
func snapshotFilename(base string, withTimestamp, asSQLite bool, now time.Time, goos string) string {
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	if withTimestamp {
		name = timestampSuffixPattern.ReplaceAllString(name, "")
		name = name + "_" + now.Format(timestampLayout)
	}

	if ext == "" {
		if asSQLite {
			ext = ".sqlite"
		} else if goos == "windows" {
			ext = ".exe"
		}
	}
	return name + ext
}

// defaultSnapshotBase is the base filename ".snapshot" (with no
// FILENAME argument) and --snapshot-interval's periodic saves use:
// --snapshot-as (-o) if the caller set one, otherwise the running
// executable's own basename (naming.md's "ファイル名省略、実行中バイナリ
// 名がベース").
func defaultSnapshotBase(self string, opts *options) string {
	if opts != nil && opts.snapshotAs != "" {
		return opts.snapshotAs
	}
	return filepath.Base(self)
}
