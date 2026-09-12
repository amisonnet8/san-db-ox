package engine

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	// Magic identifies a SanDBox footer at the end of a file (spec §11).
	Magic = "SANDBOX1"
	// FooterSize is the fixed size, in bytes, of the trailing footer:
	// Magic(8) + Version(4) + DataOffset(8) + DataLength(8) + Reserved(4).
	// All integer fields are big-endian.
	FooterSize = 32
	// FormatVersion is the current data-blob format version written into
	// a footer's Version field.
	FormatVersion = 1
	// MaxDataSize is the largest data blob engine can hold. It matches
	// modernc.org/sqlite's memdb VFS default ceiling
	// (SQLITE_MEMDB_DEFAULT_MAXSIZE = 1GiB, confirmed in the Phase 1
	// Step 2 spike). A footer claiming a DataLength larger than this is
	// necessarily corrupt.
	MaxDataSize = 1 << 30
)

// footerInfo is what a trailing footer says about a file, if anything.
// It stays unexported: it is a purely internal fact ("is there a footer,
// and where does the data start") that OpenSelf/Snapshot/Overwrite/
// readEnginePrefix need, none of which care whether path is even a
// SQLite file. The public Inspect(path) API (inspect.go) wraps this into
// the richer FileInfo/FileKind pair callers actually want, rather than
// folding SQLite-header detection into footerInfo itself.
type footerInfo struct {
	hasData    bool
	version    uint32
	dataOffset int64
	dataLength int64
}

// readFooter inspects the trailing FooterSize bytes of path, if it is
// large enough to hold one, and reports the file's total size alongside
// whatever footer it found. A file too short to hold a footer, or one
// whose trailing bytes don't start with Magic, means "no footer"
// (hasData=false), not an error.
func readFooter(path string) (footerInfo, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return footerInfo{}, 0, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return footerInfo{}, 0, err
	}
	size := stat.Size()
	if size < FooterSize {
		return footerInfo{}, size, nil
	}

	buf := make([]byte, FooterSize)
	if _, err := f.ReadAt(buf, size-FooterSize); err != nil {
		return footerInfo{}, 0, err
	}

	fi, err := decodeFooter(buf, size)
	if err != nil {
		return footerInfo{}, 0, fmt.Errorf("engine: %s: %w", path, err)
	}
	return fi, size, nil
}

// decodeFooter parses a FooterSize-byte trailing footer against the total
// size, in bytes, of whatever it came from, and reports what it says.
// This is the byte-slice-only half of readFooter's logic, split out so
// LoadFrom (load.go) can run the same decoding against bytes already read
// from an io.Reader instead of re-implementing it against a file. footer
// may be shorter than FooterSize (or nil) if size < FooterSize; a footer
// whose magic doesn't match, or a size too small to hold one, means "no
// footer" (hasData=false in the result), not an error -- only a footer
// whose magic matches but whose fields are inconsistent with size is an
// error.
func decodeFooter(footer []byte, size int64) (footerInfo, error) {
	if size < FooterSize || string(footer[0:8]) != Magic {
		return footerInfo{}, nil
	}

	dataOffset := int64(binary.BigEndian.Uint64(footer[12:20]))
	dataLength := int64(binary.BigEndian.Uint64(footer[20:28]))
	if dataOffset < 0 || dataLength < 0 || dataLength > MaxDataSize || dataOffset+dataLength+FooterSize != size {
		return footerInfo{}, fmt.Errorf("corrupt footer (offset=%d length=%d size=%d)", dataOffset, dataLength, size)
	}

	return footerInfo{
		hasData:    true,
		version:    binary.BigEndian.Uint32(footer[8:12]),
		dataOffset: dataOffset,
		dataLength: dataLength,
	}, nil
}

// encodeFooter builds the trailing footer for an image whose engine
// prefix is dataOffset bytes long and whose data blob is dataLength
// bytes.
func encodeFooter(dataOffset, dataLength int64) []byte {
	buf := make([]byte, FooterSize)
	copy(buf[0:8], Magic)
	binary.BigEndian.PutUint32(buf[8:12], FormatVersion)
	binary.BigEndian.PutUint64(buf[12:20], uint64(dataOffset))
	binary.BigEndian.PutUint64(buf[20:28], uint64(dataLength))
	return buf
}
