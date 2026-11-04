package gitcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
)

// TestParseTree covers the -z record format: NUL-separated records whose path
// may contain spaces, tabs, newlines and non-ASCII bytes, and whose sizes are
// "-" for entries that are not blobs.
func TestParseTree(t *testing.T) {
	in := []byte(
		"100644 blob aaa 3\tone.txt\x00" +
			"100755 blob bbb 0\trun.sh\x00" +
			"120000 blob ccc 9\tlink.txt\x00" +
			"160000 commit ddd -\tvendor/lib\x00" +
			"100644 blob eee 12\tdir/空 格\n换行.txt\x00")

	got, err := ParseTree(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.TreeEntry{
		{Path: "dir/空 格\n换行.txt", Mode: "100644", Type: "blob", SHA: "eee", Size: 12},
		{Path: "link.txt", Mode: "120000", Type: "blob", SHA: "ccc", Size: 9},
		{Path: "one.txt", Mode: "100644", Type: "blob", SHA: "aaa", Size: 3},
		{Path: "run.sh", Mode: "100755", Type: "blob", SHA: "bbb", Size: 0},
		{Path: "vendor/lib", Mode: "160000", Type: "commit", SHA: "ddd", Size: -1},
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if !got[3].Executable() || got[3].Symlink() || got[3].Submodule() {
		t.Fatalf("run.sh mode helpers = %+v", got[3])
	}
	if !got[1].Symlink() || got[1].Executable() {
		t.Fatalf("link.txt mode helpers = %+v", got[1])
	}
	if !got[4].Submodule() || got[4].Size != -1 {
		t.Fatalf("gitlink = %+v", got[4])
	}

	// Empty output and a trailing NUL are fine.
	for _, body := range []string{"", "\x00", "\x00\x00"} {
		list, err := ParseTree([]byte(body))
		if err != nil || len(list) != 0 {
			t.Fatalf("%q = %+v, %v", body, list, err)
		}
	}
	// Malformed records are reported, not skipped silently.
	for _, body := range []string{"100644 blob aaa 3 one.txt\x00", "100644 blob aaa\x00", "x y z w\tfile\x00"} {
		if _, err := ParseTree([]byte(body)); err == nil {
			t.Fatalf("%q parsed without error", body)
		}
	}
}

// TestTreeAt reads the tree of a real commit: nested directories, modes,
// symlinks and a submodule gitlink, plus the error for an unknown revision.
func TestTreeAt(t *testing.T) {
	tr := newTestRepo(t)
	// On Windows Git ignores the filesystem modes (core.filemode=false) and
	// records a link as a regular file unless core.symlinks is on, so ask for
	// the tree modes explicitly below instead of relying on the platform.
	tr.git("config", "core.symlinks", "true")
	tr.write("README.md", "hello\n")
	tr.write("src/main.go", "package main\n")
	tr.write("docs/空 格.txt", "text\n")
	tr.commitAll("base")

	// A second commit adds the executable bit, a symlink and a submodule.
	run := filepath.Join(tr.dir, "run.sh")
	if err := os.WriteFile(run, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(run, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("README.md", filepath.Join(tr.dir, "link.md")); err != nil {
		t.Fatal(err)
	}
	tr.git("add", "run.sh", "link.md")
	// Git does not read the executable bit from an NTFS file, and the index
	// mode is what the tree records: set it directly so this test covers real
	// 100755 tree metadata on every platform.
	tr.git("update-index", "--chmod=+x", "run.sh")
	head := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	// A gitlink is how a submodule appears in the tree; the target
	// repository does not need to exist to list it.
	tr.git("update-index", "--add", "--cacheinfo", "160000,"+head+",vendor/lib")
	tr.git("commit", "-q", "-m", "modes")

	got, err := tr.r.TreeAt(ctx, tr.repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]domain.TreeEntry, len(got))
	var paths []string
	for _, e := range got {
		byPath[e.Path] = e
		paths = append(paths, e.Path)
	}
	wantPaths := []string{"README.md", "docs/空 格.txt", "link.md", "run.sh", "src/main.go", "vendor/lib"}
	if strings.Join(paths, "|") != strings.Join(wantPaths, "|") {
		t.Fatalf("paths = %v, want %v", paths, wantPaths)
	}
	if e := byPath["README.md"]; e.Mode != "100644" || e.Size != 6 || e.Type != "blob" {
		t.Fatalf("README.md = %+v", e)
	}
	if e := byPath["docs/空 格.txt"]; e.Path != "docs/空 格.txt" || e.Size != 5 {
		t.Fatalf("non-ASCII path = %+v", e)
	}
	if e := byPath["run.sh"]; !e.Executable() || e.Mode != "100755" || e.Size != 10 {
		t.Fatalf("run.sh = %+v", e)
	}
	if e := byPath["link.md"]; !e.Symlink() || e.Mode != "120000" || e.Size != int64(len("README.md")) {
		t.Fatalf("link.md = %+v", e)
	}
	if e := byPath["vendor/lib"]; !e.Submodule() || e.Type != "commit" || e.Size != -1 || e.Mode != "160000" {
		t.Fatalf("vendor/lib = %+v", e)
	}

	// An empty revision means HEAD.
	again, err := tr.r.TreeAt(ctx, tr.repo, "")
	if err != nil || len(again) != len(got) {
		t.Fatalf("HEAD default = %d entries, %v", len(again), err)
	}

	// An empty commit has an empty tree.
	tr.git("rm", "-r", "-q", "--cached", ".")
	tr.git("commit", "-q", "-m", "empty")
	empty, err := tr.r.TreeAt(ctx, tr.repo, "HEAD")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty tree = %+v, %v", empty, err)
	}

	// Unknown revisions keep Git's own explanation.
	if _, err := tr.r.TreeAt(ctx, tr.repo, "no-such-revision"); err == nil {
		t.Fatal("unknown revision listed without error")
	} else if !strings.Contains(err.Error(), "no-such-revision") {
		t.Fatalf("error = %v", err)
	}
}
