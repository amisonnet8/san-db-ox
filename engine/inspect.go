package engine

import (
	"fmt"
	"os"
)

// sqliteHeaderPrefix is the fixed 16-byte string every SQLite database
// file starts with (spec §6). It never appears at the start of a SanDBox
// executable (an OS-loadable binary header), so checking it is enough to
// tell the two formats apart without touching the trailing footer.
const sqliteHeaderPrefix = "SQLite format 3\x00"

// FileKind classifies what Inspect found at a path: one of the two
// formats engine reads, or neither (spec §6).
type FileKind int

const (
	// KindUnknown means path is neither a SQLite database file nor a
	// SanDBox executable -- not an error in itself (Inspect returns nil
	// error for it), just information Load and its callers act on.
	KindUnknown FileKind = iota
	// KindSQLite means path starts with the SQLite file header.
	KindSQLite
	// KindExecutable means path ends with a SanDBox footer (Magic,
	// footer.go).
	KindExecutable
)

func (k FileKind) String() string {
	switch k {
	case KindSQLite:
		return "sqlite"
	case KindExecutable:
		return "executable"
	default:
		return "unknown"
	}
}

// FileInfo is what Inspect learned about a file without opening it as a
// database (spec §6, §10). cmd/san-db-ox uses it to warn about a footer
// Version mismatch before calling Load (spec §4) -- engine itself never
// logs (§10's division of responsibility).
type FileInfo struct {
	Kind FileKind
	Size int64 // total file size, in bytes

	// HasData is DataLength > 0. For KindSQLite this is Size > 0 (an
	// empty file has no SQLite header to match in the first place, so
	// this only ever applies to KindExecutable in practice, but the
	// field is defined the same way for both kinds so callers do not
	// need a kind-specific check).
	HasData bool

	// Version is the footer's format version (spec §11). Meaningful
	// only for KindExecutable; zero for every other kind, since
	// FormatVersion itself starts at 1.
	Version uint32

	// DataOffset and DataLength describe where the data blob lives
	// within the file. For KindSQLite, DataOffset is always 0 and
	// DataLength is always Size -- the "blob" is the whole file -- so
	// that HasData's definition above holds uniformly across kinds.
	DataOffset int64
	DataLength int64
}

// Inspect reports what kind of file, if any, path is, and (for
// KindExecutable) what its trailing footer says, without opening it as a
// database (spec §6, §10). It is the basis Load (load.go) uses to decide
// how to read path, and the API cmd/san-db-ox is meant to call before
// Load to warn about a footer Version mismatch (spec §4) -- engine itself
// never logs.
//
// Detection order matches spec §6: path is read as KindSQLite if its
// first 16 bytes match the SQLite header, checked before any footer at
// all -- so a SQLite file that happens to carry trailing bytes matching
// the SanDBox footer format is still reported as KindSQLite. Otherwise, a
// trailing 32-byte footer whose Magic matches makes it KindExecutable.
// Anything else -- including a zero-byte file, or one too short to hold
// either signature -- is KindUnknown, with a nil error: telling "not
// readable by engine" from "an I/O error trying to find out" is Inspect's
// job, but deciding whether KindUnknown is fatal is the caller's (Load
// returns ErrUnsupportedFile for it; a tool that only wants to know a
// file's type has no reason to treat it as an error at all).
//
// A footer whose Magic matches but whose fields are inconsistent with
// the file's size is a corrupt footer and is reported as an error, same
// as readFooter (footer.go). A path that cannot be opened, cannot be
// stat'd, or does not name a regular file (missing file, permission
// error, a directory) surfaces that as an error too -- not
// ErrUnsupportedFile: that sentinel means "this byte content is not one
// of the two formats engine reads", which does not apply when there was
// no byte content to read in the first place. A directory needs its own
// check rather than falling out of the size-based logic below: os.Open
// and Stat both succeed on one, and its reported Size() can be small
// enough (0, at least on tmpfs) to skip every ReadAt that would
// otherwise surface "is a directory" on its own.
func Inspect(path string) (*FileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		// A directory in particular would otherwise slip through as
		// KindUnknown with no error: os.Open/Stat both succeed on a
		// directory, and on at least tmpfs its reported Size() is 0, so
		// neither of the length checks below would ever call ReadAt (the
		// call that would otherwise surface "is a directory" itself).
		return nil, fmt.Errorf("engine: %s: not a regular file", path)
	}
	size := stat.Size()

	if size >= int64(len(sqliteHeaderPrefix)) {
		head := make([]byte, len(sqliteHeaderPrefix))
		if _, err := f.ReadAt(head, 0); err != nil {
			return nil, err
		}
		if string(head) == sqliteHeaderPrefix {
			return &FileInfo{
				Kind:       KindSQLite,
				Size:       size,
				HasData:    size > 0,
				DataOffset: 0,
				DataLength: size,
			}, nil
		}
	}

	if size >= FooterSize {
		footer := make([]byte, FooterSize)
		if _, err := f.ReadAt(footer, size-FooterSize); err != nil {
			return nil, err
		}
		fi, err := decodeFooter(footer, size)
		if err != nil {
			return nil, fmt.Errorf("engine: %s: %w", path, err)
		}
		if fi.hasData {
			return &FileInfo{
				Kind:       KindExecutable,
				Size:       size,
				HasData:    fi.dataLength > 0,
				Version:    fi.version,
				DataOffset: fi.dataOffset,
				DataLength: fi.dataLength,
			}, nil
		}
	}

	return &FileInfo{Kind: KindUnknown, Size: size}, nil
}
