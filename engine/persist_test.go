package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSamePath(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")

	if !samePath(a, a) {
		t.Errorf("samePath(a, a) = false, want true")
	}
	if !samePath(a, filepath.Join(dir, ".", "a")) {
		t.Errorf("samePath should ignore a redundant './' component")
	}
	if samePath(a, b) {
		t.Errorf("samePath(a, b) = true, want false (different files)")
	}

	rel := "a"
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if !samePath(rel, filepath.Join(dir, "a")) {
		t.Errorf("samePath should resolve a relative path against the current directory")
	}
	t.Chdir(wd)

	if runtime.GOOS == "windows" {
		if !samePath(filepath.Join(dir, "A"), a) {
			t.Errorf("samePath should be case-insensitive on Windows")
		}
	} else {
		if samePath(filepath.Join(dir, "A"), a) {
			t.Errorf("samePath should be case-sensitive outside Windows")
		}
	}
}

func TestLooksLikeGoRunTempBinary(t *testing.T) {
	tmp := os.TempDir()
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join(tmp, "go-build123456", "b001", "exe", "main"), true},
		{filepath.Join(tmp, "somethingelse", "main"), false},
		{filepath.Join(t.TempDir(), "main"), false}, // t.TempDir() is under os.TempDir() but has no "go-build" component
	}
	for _, c := range cases {
		if got := looksLikeGoRunTempBinary(c.path); got != c.want {
			t.Errorf("looksLikeGoRunTempBinary(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestReadEnginePrefixNoFooter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain-binary")
	content := []byte("pretend this is a whole ELF/PE binary with no footer")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := readEnginePrefix(path)
	if err != nil {
		t.Fatalf("readEnginePrefix: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("engine prefix = %q, want the whole file %q", got, content)
	}
}

func TestReadEnginePrefixWithFooter(t *testing.T) {
	engineBytes := []byte("pretend-engine-bytes")
	data := []byte("pretend-data-blob")
	blob := append(append([]byte{}, engineBytes...), data...)
	blob = append(blob, encodeFooter(int64(len(engineBytes)), int64(len(data)))...)

	path := filepath.Join(t.TempDir(), "image")
	if err := os.WriteFile(path, blob, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := readEnginePrefix(path)
	if err != nil {
		t.Fatalf("readEnginePrefix: %v", err)
	}
	if string(got) != string(engineBytes) {
		t.Fatalf("engine prefix = %q, want %q", got, engineBytes)
	}
}

// TestOverwriteSelfMechanics exercises overwriteSelf directly against a
// throwaway file rather than the real running test binary (testing.md:
// ".overwriteのテストは必ずコピーに対して行う" / "go testからの.overwriteの
// 実挙動は検証できない" -- unit tests cover the evacuate/write/rollback
// steps in isolation; make test's E2E suite, added in Phase 1 Step 5,
// covers the real self-overwrite path against a built binary).
func TestOverwriteSelfMechanics(t *testing.T) {
	dir := t.TempDir()
	selfPath := filepath.Join(dir, "fake-self")
	if err := os.WriteFile(selfPath, []byte("original-engine-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}

	newEngine := []byte("new-engine-bytes")
	newData := []byte("new-data-blob")
	if err := overwriteSelf(selfPath, newEngine, newData); err != nil {
		t.Fatalf("overwriteSelf: %v", err)
	}

	// The sidecar must be gone (best-effort removal succeeds on Linux).
	if _, err := os.Stat(selfPath + oldSuffix); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, stat err = %v", selfPath+oldSuffix, err)
	}

	got, err := os.ReadFile(selfPath)
	if err != nil {
		t.Fatalf("read overwritten file: %v", err)
	}
	want := append(append([]byte{}, newEngine...), newData...)
	want = append(want, encodeFooter(int64(len(newEngine)), int64(len(newData)))...)
	if string(got) != string(want) {
		t.Fatalf("overwritten content mismatch")
	}

	fi, err := os.Stat(selfPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("expected the overwritten file to be executable, mode = %v", fi.Mode())
	}
}

// TestOverwriteSelfRollsBackOnWriteFailure forces the write step to fail
// (a read-only directory) and confirms the original file is restored to
// selfPath rather than being left only at the sidecar path.
func TestOverwriteSelfRollsBackOnWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits don't block writes the same way on Windows")
	}

	dir := t.TempDir()
	selfPath := filepath.Join(dir, "fake-self")
	original := []byte("original-content")
	if err := os.WriteFile(selfPath, original, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0o500); err != nil { // r-x: rename ok, create/write blocked
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) }) // let t.TempDir() clean up afterward

	err := overwriteSelf(selfPath, []byte("engine"), []byte("data"))
	if err == nil {
		t.Fatalf("expected overwriteSelf to fail when the directory is not writable")
	}

	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got, readErr := os.ReadFile(selfPath)
	if readErr != nil {
		t.Fatalf("expected selfPath to be restored after a failed write, read err: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("restored content = %q, want original %q", got, original)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE t (v TEXT); INSERT INTO t VALUES ('snapshot-me')"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "snap")
	if err := db.Snapshot(path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	fi, size, err := readFooter(path)
	if err != nil {
		t.Fatalf("readFooter(snapshot): %v", err)
	}
	if !fi.hasData {
		t.Fatalf("expected the snapshot to carry a footer with data")
	}
	if fi.dataOffset+fi.dataLength+FooterSize != size {
		t.Fatalf("footer fields inconsistent with file size")
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	blob := make([]byte, fi.dataLength)
	_, err = f.ReadAt(blob, fi.dataOffset)
	f.Close()
	if err != nil {
		t.Fatalf("read data blob: %v", err)
	}

	// Deserialize the extracted blob into a fresh scratch DB the same way
	// loadBlobInto does, and confirm the row survived the round trip.
	scratch, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer scratch.Close()
	if err := loadBlobInto(blob, scratch.dsn); err != nil {
		t.Fatalf("loadBlobInto(snapshot data): %v", err)
	}
	var v string
	if err := scratch.QueryRow("SELECT v FROM t").Scan(&v); err != nil {
		t.Fatalf("query restored data: %v", err)
	}
	if v != "snapshot-me" {
		t.Fatalf("v = %q, want %q", v, "snapshot-me")
	}
}

func TestSnapshotIsAtomicNoPartialFileOnFailure(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// A directory that doesn't exist makes os.CreateTemp fail before any
	// output file is created at the destination path.
	path := filepath.Join(t.TempDir(), "no-such-dir", "snap")
	if err := db.Snapshot(path); err == nil {
		t.Fatalf("expected Snapshot to fail when the destination directory doesn't exist")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no file at %s after a failed Snapshot", path)
	}
}

func TestSerializeBarrierRejectsAfterClose(t *testing.T) {
	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Snapshot(filepath.Join(t.TempDir(), "snap")); err != ErrClosed {
		t.Fatalf("Snapshot after Close = %v, want ErrClosed", err)
	}
}

// TestOverwriteRejectsGoTestBinary documents the practical consequence of
// testing.md's guidance for this package: since `go test` compiles its
// binary under a `go-build*` temp directory, the real DB.Overwrite
// (unlike overwriteSelf tested above) always refuses to run here.
func TestOverwriteRejectsGoTestBinary(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !looksLikeGoRunTempBinary(self) {
		t.Skip("this Go toolchain does not build go test binaries under a go-build* temp dir; nothing to assert")
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Overwrite(); err != ErrNotOverwritable {
		t.Fatalf("Overwrite() from a go test binary = %v, want ErrNotOverwritable", err)
	}
}

// TestSnapshotTargetingSelfInGoTestIsRejected covers Snapshot's
// self-targeting branch (added after windows-latest CI found that a
// plain os.Rename onto the running executable's own path fails there --
// PLAN.md Phase 1 Step 5): when path resolves to the same file as
// os.Executable(), Snapshot switches to the same evacuate-then-write
// path Overwrite uses, so it must honor the same go-run-temp-binary
// guard. Exercising the successful evacuate-then-write path itself isn't
// possible from `go test` (same reason Overwrite's own success path
// isn't, see TestOverwriteSelfMechanics for that logic tested directly).
func TestSnapshotTargetingSelfInGoTestIsRejected(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !looksLikeGoRunTempBinary(self) {
		t.Skip("this Go toolchain does not build go test binaries under a go-build* temp dir; nothing to assert")
	}

	db, err := newDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Snapshot(self); err != ErrNotOverwritable {
		t.Fatalf("Snapshot(self) from a go test binary = %v, want ErrNotOverwritable", err)
	}
}
