package gitcli

import (
	"strings"
	"testing"
)

func TestBlameHistoryRenameAndWorktree(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\ntwo\nthree\n")
	tr.commitAll("First")
	first := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	tr.git("config", "user.name", "Grace Hopper")
	tr.write("a.txt", "one\n2\nthree\n")
	tr.commitAll("Second\n\nbody")
	second := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	tr.git("mv", "a.txt", "b.txt")
	tr.commitAll("Rename")

	b, err := tr.r.Blame(ctx, tr.repo, "HEAD", "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Lines) != 3 || len(b.Commits) != 2 {
		t.Fatalf("lines=%d commits=%+v", len(b.Lines), b.Commits)
	}
	c0, _ := b.CommitAt(0)
	c1, _ := b.CommitAt(1)
	c2, _ := b.CommitAt(2)
	if c0.Hash != first || c2.Hash != first || c1.Hash != second {
		t.Fatalf("owners = %s %s %s", c0.Hash, c1.Hash, c2.Hash)
	}
	// Rename detection follows the lines into the old file name.
	if c0.Path != "a.txt" || c1.Summary != "Second" || c1.Author != "Grace Hopper" || c0.Author != "Viff Test" {
		t.Fatalf("details: %+v / %+v", c0, c1)
	}
	if c1.Previous != first || c1.PreviousPath != "a.txt" || c0.Time.IsZero() || c1.AuthorEmail != "test@example.com" {
		t.Fatalf("previous/time: %+v", c1)
	}
	rank, n := b.AgeRank()
	if n != 2 && n != 1 || rank[b.Lines[1]] < rank[b.Lines[0]] {
		t.Fatalf("rank = %v of %d", rank, n)
	}

	// Blaming an older revision and the working tree.
	if old, err := tr.r.Blame(ctx, tr.repo, first, "a.txt"); err != nil || len(old.Commits) != 1 {
		t.Fatalf("old blame: %+v %v", old, err)
	}
	tr.write("b.txt", "one\n2\nthree\nfour\n")
	wt, err := tr.r.Blame(ctx, tr.repo, "", "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := wt.CommitAt(3); !c.Uncommitted() || len(wt.Lines) != 4 {
		t.Fatalf("uncommitted line: %+v", c)
	}
	if c, _ := wt.CommitAt(0); c.Uncommitted() {
		t.Fatal("committed line marked uncommitted")
	}
	if _, ok := wt.CommitAt(9); ok {
		t.Fatal("out of range line")
	}
}

func TestBlameContentLookingLikeHeaders(t *testing.T) {
	tr := newTestRepo(t)
	// Content lines are tab-prefixed, so text that looks like porcelain is safe.
	tr.write("x.txt", strings.Repeat("a", 40)+" 1 1 1\nauthor Mallory\n\n")
	tr.commitAll("Tricky")
	b, err := tr.r.Blame(ctx, tr.repo, "HEAD", "x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Lines) != 3 || len(b.Commits) != 1 || b.Commits[0].Author != "Viff Test" {
		t.Fatalf("blame = %+v", b)
	}
}

func TestBlameEmptyFileAndMissing(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("empty.txt", "")
	tr.commitAll("Empty")
	b, err := tr.r.Blame(ctx, tr.repo, "HEAD", "empty.txt")
	if err != nil || len(b.Lines) != 0 {
		t.Fatalf("empty: %+v %v", b, err)
	}
	if _, err := tr.r.Blame(ctx, tr.repo, "HEAD", "nope.txt"); err == nil {
		t.Fatal("missing file must fail")
	}
}
