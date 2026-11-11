package gitcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLFSWindowsIndexRevisionDetectsSameSizeReplacement(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "Alternate Index")
	stamp := time.Unix(1_700_000_000, 0)
	write := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	write(index, "old")
	before, err := localLFSIndexRevision(index)
	if err != nil {
		t.Fatal(err)
	}
	// Create the replacement while the original still exists, so the
	// filesystem cannot give it the original file ID.
	lock := index + ".lock"
	write(lock, "new")
	if err := os.Rename(lock, index); err != nil {
		t.Fatal(err)
	}
	after, err := localLFSIndexRevision(index)
	if err != nil || before.equal(after) {
		t.Fatalf("same-size and same-mtime replacement: before %v, after %v, err %v", before, after, err)
	}
}
