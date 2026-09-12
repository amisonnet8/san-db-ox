package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFooterNoFooterOnShortFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "short")
	if err := os.WriteFile(path, []byte("too short"), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, size, err := readFooter(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fi.hasData {
		t.Fatalf("expected hasData=false for a file shorter than FooterSize")
	}
	if size != int64(len("too short")) {
		t.Fatalf("size = %d, want %d", size, len("too short"))
	}
}

func TestReadFooterNoMagic(t *testing.T) {
	// Long enough to hold a footer, but the trailing bytes don't spell
	// Magic.
	body := make([]byte, FooterSize*2)
	path := filepath.Join(t.TempDir(), "nomagic")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}

	fi, size, err := readFooter(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fi.hasData {
		t.Fatalf("expected hasData=false when the trailing bytes don't start with Magic")
	}
	if size != int64(len(body)) {
		t.Fatalf("size = %d, want %d", size, len(body))
	}
}

func TestFooterRoundTrip(t *testing.T) {
	engineBytes := []byte("pretend-engine-bytes")
	data := []byte("pretend-serialized-sqlite-bytes")

	path := filepath.Join(t.TempDir(), "image")
	blob := append(append([]byte{}, engineBytes...), data...)
	blob = append(blob, encodeFooter(int64(len(engineBytes)), int64(len(data)))...)
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	fi, size, err := readFooter(path)
	if err != nil {
		t.Fatalf("readFooter: %v", err)
	}
	if !fi.hasData {
		t.Fatalf("expected hasData=true")
	}
	if fi.version != FormatVersion {
		t.Fatalf("version = %d, want %d", fi.version, FormatVersion)
	}
	if fi.dataOffset != int64(len(engineBytes)) {
		t.Fatalf("dataOffset = %d, want %d", fi.dataOffset, len(engineBytes))
	}
	if fi.dataLength != int64(len(data)) {
		t.Fatalf("dataLength = %d, want %d", fi.dataLength, len(data))
	}
	if size != int64(len(blob)) {
		t.Fatalf("size = %d, want %d", size, len(blob))
	}
}

func TestReadFooterCorrupt(t *testing.T) {
	// A footer whose magic matches but whose offset/length don't agree
	// with the actual file size is corrupt, not "no footer".
	footer := encodeFooter(1000, 2000) // claims far more data than present
	path := filepath.Join(t.TempDir(), "corrupt")
	if err := os.WriteFile(path, footer, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := readFooter(path)
	if err == nil {
		t.Fatalf("expected an error for an inconsistent footer")
	}
}

func TestEncodeFooterMagicAndBigEndian(t *testing.T) {
	footer := encodeFooter(0x0102030405060708, 0x1112131415161718)
	if string(footer[0:8]) != Magic {
		t.Fatalf("magic = %q, want %q", footer[0:8], Magic)
	}
	// Spot-check big-endian encoding of DataOffset's high byte.
	if footer[12] != 0x01 {
		t.Fatalf("DataOffset high byte = %#x, want 0x01 (big-endian)", footer[12])
	}
	if footer[20] != 0x11 {
		t.Fatalf("DataLength high byte = %#x, want 0x11 (big-endian)", footer[20])
	}
}
