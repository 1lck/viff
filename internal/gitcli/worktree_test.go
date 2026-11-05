package gitcli

import (
	"context"
	"slices"
	"testing"
)

// TestWorktreeFiles lists the working tree through Git: files tracked in
// subdirectories, an untracked file, an untracked-but-ignored file, and paths
// with spaces and non-ASCII bytes. The list is sorted by path and keeps every
// path exactly as Git printed it (no unquoting or separator rewriting), and a
// second call sees a file added in between.
func TestWorktreeFiles(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.write("dir/nested/b.txt", "two\n")
	tr.write("空间/文件 名.txt", "three\n")
	tr.git("add", "-A")
	tr.git("commit", "-q", "-m", "tracked")
	tr.write(".gitignore", "ignored.txt\n")
	tr.git("add", ".gitignore")
	tr.git("commit", "-q", "-m", "ignore")
	tr.write("ignored.txt", "noise\n")
	tr.write("new file.txt", "new\n")

	got, err := tr.r.WorktreeFiles(context.Background(), tr.repo)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".gitignore", "a.txt", "dir/nested/b.txt", "new file.txt", "空间/文件 名.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("files = %q, want %q", got, want)
	}

	// Git is the source of truth: the working tree is asked again, not
	// remembered, so a file created after the first call shows up.
	tr.write("later file.txt", "later\n")
	got, err = tr.r.WorktreeFiles(context.Background(), tr.repo)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{".gitignore", "a.txt", "dir/nested/b.txt", "later file.txt", "new file.txt", "空间/文件 名.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("files after add = %q, want %q", got, want)
	}
}

// TestWorktreeFilesEmptyRepository covers an unborn repository and the first
// commit after it: an empty listing is a valid result, not an error.
func TestWorktreeFilesEmptyRepository(t *testing.T) {
	tr := newTestRepo(t)
	got, err := tr.r.WorktreeFiles(context.Background(), tr.repo)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("empty repository files = %#v", got)
	}
	tr.write("first.txt", "first\n")
	tr.git("add", "-A")
	tr.git("commit", "-q", "-m", "first")
	got, err = tr.r.WorktreeFiles(context.Background(), tr.repo)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"first.txt"}) {
		t.Fatalf("files after first commit = %q", got)
	}
}
