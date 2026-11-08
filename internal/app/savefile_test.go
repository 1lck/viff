package app

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func hashFile(t *testing.T, dir, name string) [sha256.Size]byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func saveFile(t *testing.T, f *fixture, req SaveFileRequest) (SaveFileResult, error) {
	t.Helper()
	var (
		result SaveFileResult
		err    error
		done   bool
	)
	f.w.SaveFile(req, func(r SaveFileResult, e error) {
		result, err, done = r, e, true
	})
	f.settle(func() bool { return done && f.idle() })
	return result, err
}

func TestSaveFileWritesWorkingTreeAndRefreshesStatus(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "old\n")
	f.commitAll("base")
	f.open()

	result, err := saveFile(t, f, SaveFileRequest{
		Path: "a.txt", Text: "new\n", LineEnding: "LF",
		Expected: hashFile(t, f.dir, "a.txt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.read("a.txt"); got != "new\n" {
		t.Fatalf("file = %q", got)
	}
	if result.Hash != hashFile(t, f.dir, "a.txt") || result.Size != 4 {
		t.Fatalf("result = %+v", result)
	}
	entry, ok := f.entry("a.txt")
	if !ok || entry.Worktree != 'M' {
		t.Fatalf("status = %+v, %t", entry, ok)
	}
}

func TestSaveFileRefusesAnExternalChange(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "old\n")
	f.commitAll("base")
	f.open()
	expected := hashFile(t, f.dir, "a.txt")
	f.write("a.txt", "someone else\n")

	_, err := saveFile(t, f, SaveFileRequest{
		Path: "a.txt", Text: "mine\n", LineEnding: "LF", Expected: expected,
	})
	if !errors.Is(err, ErrEditChanged) {
		t.Fatalf("err = %v, want ErrEditChanged", err)
	}
	if got := f.read("a.txt"); got != "someone else\n" {
		t.Fatalf("the save overwrote the external change: %q", got)
	}
}

func TestSaveFilePreservesLineEndingAndBOM(t *testing.T) {
	f := newFixture(t)
	original := "\xef\xbb\xbfone\r\ntwo\r\n"
	f.write("a.txt", original)
	f.commitAll("base")
	f.open()

	_, err := saveFile(t, f, SaveFileRequest{
		Path: "a.txt", Text: "one\ntwo\nthree\n", LineEnding: "CRLF", BOM: true,
		Expected: hashFile(t, f.dir, "a.txt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "\xef\xbb\xbfone\r\ntwo\r\nthree\r\n"
	if got := f.read("a.txt"); got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

func TestSaveFileKeepsAMissingFinalNewline(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\ntwo")
	f.commitAll("base")
	f.open()

	_, err := saveFile(t, f, SaveFileRequest{
		Path: "a.txt", Text: "one\ntwo\nthree", LineEnding: "LF",
		Expected: hashFile(t, f.dir, "a.txt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.read("a.txt"); got != "one\ntwo\nthree" {
		t.Fatalf("file = %q", got)
	}
}

func TestSaveFileRejectsUnsafePathsAndFiles(t *testing.T) {
	f := newFixture(t)
	f.write("dir/a.txt", "a\n")
	f.write("bin", "\x00\x01")
	f.commitAll("base")
	f.open()

	hash := hashFile(t, f.dir, "dir/a.txt")
	for _, tc := range []struct {
		name string
		req  SaveFileRequest
		want error
	}{
		{"parent traversal", SaveFileRequest{Path: "../outside.txt", Text: "x", Expected: hash}, ErrEditPath},
		{"absolute path", SaveFileRequest{Path: filepath.Join(f.dir, "dir/a.txt"), Text: "x", Expected: hash}, ErrEditPath},
		{"dot git", SaveFileRequest{Path: ".git/config", Text: "x", Expected: hash}, ErrEditPath},
		{"missing", SaveFileRequest{Path: "dir/missing.txt", Text: "x", Expected: hash}, ErrEditMissing},
		{"binary", SaveFileRequest{Path: "bin", Text: "x", Expected: hashFile(t, f.dir, "bin")}, ErrEditBinary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := saveFile(t, f, tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSaveFileRejectsReadOnlyFile(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "old\n")
	f.commitAll("base")
	f.open()
	path := filepath.Join(f.dir, "a.txt")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, err := saveFile(t, f, SaveFileRequest{
		Path: "a.txt", Text: "new\n", LineEnding: "LF", Expected: hashFile(t, f.dir, "a.txt"),
	})
	if !errors.Is(err, ErrEditReadOnly) {
		t.Fatalf("err = %v, want ErrEditReadOnly", err)
	}
}

func TestSaveFileRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink needs a privilege Windows CI does not grant")
	}
	f := newFixture(t)
	f.write("target.txt", "old\n")
	if err := os.Symlink("target.txt", filepath.Join(f.dir, "link.txt")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	f.open()

	_, err := saveFile(t, f, SaveFileRequest{
		Path: "link.txt", Text: "new\n", LineEnding: "LF",
		Expected: [sha256.Size]byte{},
	})
	if !errors.Is(err, ErrEditSymlink) {
		t.Fatalf("err = %v, want ErrEditSymlink", err)
	}
}

func TestSaveFileRejectsEditedTextAboveLimit(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "old\n")
	f.commitAll("base")
	f.open()

	_, err := saveFile(t, f, SaveFileRequest{
		Path: "a.txt", Text: strings.Repeat("x", EditLimit+1), LineEnding: "LF",
		Expected: hashFile(t, f.dir, "a.txt"),
	})
	if !errors.Is(err, ErrEditTooLarge) {
		t.Fatalf("err = %v, want ErrEditTooLarge", err)
	}
	if got := f.read("a.txt"); got != "old\n" {
		t.Fatalf("the rejected save changed the file: %q", got)
	}
}

func TestSaveFilePreservesModeAndWritesEmptyFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not represented on Windows")
	}
	f := newFixture(t)
	f.write("run.sh", "old\n")
	if err := os.Chmod(filepath.Join(f.dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.write("empty.txt", "")
	f.commitAll("base")
	f.open()

	if _, err := saveFile(t, f, SaveFileRequest{
		Path: "run.sh", Text: "new\n", LineEnding: "LF", Expected: hashFile(t, f.dir, "run.sh"),
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(f.dir, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 755", info.Mode().Perm())
	}
	if _, err := saveFile(t, f, SaveFileRequest{
		Path: "empty.txt", Text: "x\n", LineEnding: "LF", Expected: hashFile(t, f.dir, "empty.txt"),
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.read("empty.txt"); got != "x\n" {
		t.Fatalf("empty file = %q", got)
	}
}

func TestSaveFileKeepsABOMOnlyFile(t *testing.T) {
	f := newFixture(t)
	f.write("bom.txt", "\xef\xbb\xbf")
	f.commitAll("base")
	f.open()

	if _, err := saveFile(t, f, SaveFileRequest{
		Path: "bom.txt", Text: "", LineEnding: "", BOM: true, Expected: hashFile(t, f.dir, "bom.txt"),
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.read("bom.txt"); got != "\xef\xbb\xbf" {
		t.Fatalf("file = %q", got)
	}
}

func TestSaveFileRejectsLargeAndNonUTF8Text(t *testing.T) {
	f := newFixture(t)
	f.write("big.txt", strings.Repeat("x\n", EditLimit/2+1))
	f.write("latin1.txt", "caf\xe9\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	f.open()

	for _, tc := range []struct {
		name string
		path string
		want error
	}{
		{"large", "big.txt", ErrEditTooLarge},
		{"latin1", "latin1.txt", ErrEditBinary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := saveFile(t, f, SaveFileRequest{
				Path: tc.path, Text: "x", LineEnding: "LF",
				Expected: hashFile(t, f.dir, tc.path),
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
