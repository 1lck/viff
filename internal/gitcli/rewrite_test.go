package gitcli

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func headOf(tr *testRepo) string { return strings.TrimSpace(tr.git("rev-parse", "HEAD")) }

func subjectOf(tr *testRepo, rev string) string {
	return strings.TrimSpace(tr.git("log", "-1", "--format=%s", rev, "--"))
}

func messageOf(tr *testRepo, rev string) string {
	return strings.TrimRight(tr.git("log", "-1", "--format=%B", rev, "--"), "\n")
}

func authorOf(tr *testRepo, rev string) string {
	return strings.TrimSpace(tr.git("log", "-1", "--format=%an", rev, "--"))
}

func countOf(tr *testRepo) int {
	tr.t.Helper()
	n, err := strconv.Atoi(strings.TrimSpace(tr.git("rev-list", "--count", "HEAD")))
	if err != nil {
		tr.t.Fatal(err)
	}
	return n
}

func TestRewordCommitKeepsContentAndAuthor(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commitAll("First")
	first := headOf(tr)

	tr.git("config", "user.name", "Grace Hopper")
	tr.write("a.txt", "one\ntwo\n")
	tr.commitAll("Second\n\nBody of the second commit.")
	second := headOf(tr)
	tr.git("config", "user.name", "Viff Test")
	tr.write("b.txt", "three\n")
	tr.commitAll("Third")
	third := headOf(tr)
	tree := strings.TrimSpace(tr.git("rev-parse", "HEAD^{tree}"))

	if _, err := tr.r.RewordCommit(ctx, tr.repo, second, "Second, reworded\n\nNew body."); err != nil {
		t.Fatalf("reword: %v", err)
	}
	if got := headOf(tr); got == third {
		t.Fatal("HEAD did not move")
	}
	if countOf(tr) != 3 {
		t.Fatalf("commits = %d", countOf(tr))
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "HEAD^{tree}")); got != tree {
		t.Fatalf("tree changed: %s != %s", got, tree)
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "HEAD^^")); got != first {
		t.Fatalf("the first commit was rewritten: %s", got)
	}
	if got := messageOf(tr, "HEAD^"); got != "Second, reworded\n\nNew body." {
		t.Fatalf("message = %q", got)
	}
	if got := authorOf(tr, "HEAD^"); got != "Grace Hopper" {
		t.Fatalf("author = %q", got)
	}
	if got := strings.TrimSpace(tr.git("show", "HEAD^:a.txt")); got != "one\ntwo" {
		t.Fatalf("content = %q", got)
	}
	if out := tr.git("status", "--porcelain"); out != "" {
		t.Fatalf("dirty after reword: %q", out)
	}
	if tr.r.Operation(ctx, tr.repo).Active() {
		t.Fatal("rebase left in progress")
	}
}

func TestSquashCommitIntoParent(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commitAll("First")
	first := headOf(tr)

	tr.git("config", "user.name", "Grace Hopper")
	tr.write("b.txt", "two\n")
	tr.commitAll("Second")
	second := headOf(tr)

	tr.git("config", "user.name", "Viff Test")
	tr.write("c.txt", "three\n")
	tr.commitAll("Third")
	third := headOf(tr)

	if _, err := tr.r.SquashCommit(ctx, tr.repo, third, "Second and third"); err != nil {
		t.Fatalf("squash: %v", err)
	}
	if countOf(tr) != 2 {
		t.Fatalf("commits = %d, want 2", countOf(tr))
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "HEAD^")); got != first {
		t.Fatalf("parent = %s, want %s", got, first)
	}
	if got := subjectOf(tr, "HEAD"); got != "Second and third" {
		t.Fatalf("subject = %q", got)
	}
	// The squashed commit keeps the parent's author and both commits' content.
	if got := authorOf(tr, "HEAD"); got != "Grace Hopper" {
		t.Fatalf("author = %q", got)
	}
	for _, f := range []string{"a.txt", "b.txt", "c.txt"} {
		if _, err := tr.r.Run(ctx, tr.dir, "cat-file", "-e", "HEAD:"+f); err != nil {
			t.Fatalf("%s missing from the squashed commit", f)
		}
	}
	_ = second
	if out := tr.git("status", "--porcelain"); out != "" {
		t.Fatalf("dirty after squash: %q", out)
	}
}

func TestSquashCommitIntoRootCommit(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commitAll("First")
	tr.write("b.txt", "two\n")
	tr.commitAll("Second")
	second := headOf(tr)

	if _, err := tr.r.SquashCommit(ctx, tr.repo, second, "First and second"); err != nil {
		t.Fatalf("squash into root: %v", err)
	}
	if countOf(tr) != 1 {
		t.Fatalf("commits = %d, want 1", countOf(tr))
	}
	for _, f := range []string{"a.txt", "b.txt"} {
		if _, err := tr.r.Run(ctx, tr.dir, "cat-file", "-e", "HEAD:"+f); err != nil {
			t.Fatalf("%s missing from the squashed root commit", f)
		}
	}
	if got := subjectOf(tr, "HEAD"); got != "First and second" {
		t.Fatalf("subject = %q", got)
	}
}

func TestDropCommitRemovesOnlyThatCommit(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commitAll("First")
	first := headOf(tr)
	tr.write("b.txt", "two\n")
	tr.commitAll("Second")
	second := headOf(tr)
	tr.write("c.txt", "three\n")
	tr.commitAll("Third")

	if _, err := tr.r.DropCommit(ctx, tr.repo, second); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if countOf(tr) != 2 {
		t.Fatalf("commits = %d, want 2", countOf(tr))
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "HEAD^")); got != first {
		t.Fatalf("parent = %s, want %s", got, first)
	}
	if got := subjectOf(tr, "HEAD"); got != "Third" {
		t.Fatalf("subject = %q", got)
	}
	if _, err := tr.r.Run(ctx, tr.dir, "cat-file", "-e", "HEAD:b.txt"); err == nil {
		t.Fatal("dropped commit's file is still there")
	}
	if _, err := tr.r.Run(ctx, tr.dir, "cat-file", "-e", "HEAD:c.txt"); err != nil {
		t.Fatal("the following commit's file is missing")
	}
	if out := tr.git("status", "--porcelain"); out != "" {
		t.Fatalf("dirty after drop: %q", out)
	}
}

func TestRewriteRejectsRootMergeAndUnrelatedCommits(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commitAll("First")
	first := headOf(tr)
	tr.write("a.txt", "one\ntwo\n")
	tr.commitAll("Second")
	second := headOf(tr)

	if _, err := tr.r.RewordCommit(ctx, tr.repo, first, "Root"); !errors.Is(err, ErrRewriteRoot) {
		t.Fatalf("reword root: %v", err)
	}
	if _, err := tr.r.DropCommit(ctx, tr.repo, first); !errors.Is(err, ErrRewriteRoot) {
		t.Fatalf("drop root: %v", err)
	}
	if _, err := tr.r.SquashCommit(ctx, tr.repo, first, "Root"); !errors.Is(err, ErrRewriteRoot) {
		t.Fatalf("squash root: %v", err)
	}
	if _, err := tr.r.RewordCommit(ctx, tr.repo, second, "  "); !errors.Is(err, ErrRewriteMessage) {
		t.Fatalf("empty message: %v", err)
	}

	// A side branch whose commits are not on main.
	tr.git("checkout", "-q", "-b", "side", first)
	tr.write("side.txt", "s\n")
	tr.commitAll("Side")
	side := headOf(tr)
	tr.git("checkout", "-q", "main")
	if _, err := tr.r.RewordCommit(ctx, tr.repo, side, "Unrelated"); !errors.Is(err, ErrRewriteUnrelated) {
		t.Fatalf("unrelated: %v", err)
	}

	// A merge commit cannot be edited.
	tr.git("merge", "-q", "--no-ff", "-m", "Merge side", "side")
	merge := headOf(tr)
	if _, err := tr.r.RewordCommit(ctx, tr.repo, merge, "Merge, reworded"); !errors.Is(err, ErrRewriteMerge) {
		t.Fatalf("reword merge: %v", err)
	}
	if got := headOf(tr); got != merge {
		t.Fatalf("HEAD moved after a refused edit: %s", got)
	}
	if tr.r.Operation(ctx, tr.repo).Active() {
		t.Fatal("refusal left a rebase in progress")
	}
}

func TestRewriteRejectsOperationInProgress(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "base\n")
	tr.commitAll("Base")

	tr.git("checkout", "-q", "-b", "side")
	tr.write("a.txt", "side\n")
	tr.commitAll("Side")
	tr.git("checkout", "-q", "main")
	tr.write("a.txt", "main\n")
	tr.commitAll("Main")

	if _, err := tr.gitMayFail("merge", "side"); err == nil {
		t.Fatal("merge should conflict")
	}
	if !tr.r.Operation(ctx, tr.repo).Active() {
		t.Fatal("no merge in progress")
	}
	main := headOf(tr)
	if _, err := tr.r.RewordCommit(ctx, tr.repo, main, "Reword"); !errors.Is(err, ErrRewriteOperation) {
		t.Fatalf("during merge: %v", err)
	}
	tr.git("merge", "--abort")
	if tr.r.Operation(ctx, tr.repo).Active() {
		t.Fatal("merge abort left an operation")
	}
	if _, err := tr.r.RewordCommit(ctx, tr.repo, main, "Main, reworded"); err != nil {
		t.Fatalf("after abort: %v", err)
	}
}

func TestRewriteAbortsOnConflict(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("f.txt", "1\n2\n3\n")
	tr.commitAll("First")
	tr.write("f.txt", "B\n2\n3\n")
	tr.commitAll("Second")
	second := headOf(tr)
	tr.write("f.txt", "C\n2\n3\n")
	tr.commitAll("Third")
	head := headOf(tr)

	// Dropping Second replays Third onto First: its patch no longer applies.
	if _, err := tr.r.DropCommit(ctx, tr.repo, second); err == nil {
		t.Fatal("drop should have conflicted")
	}
	if tr.r.Operation(ctx, tr.repo).Active() {
		t.Fatal("rebase left in progress after a failed edit")
	}
	if got := headOf(tr); got != head {
		t.Fatalf("HEAD = %s, want %s", got, head)
	}
	if countOf(tr) != 3 {
		t.Fatalf("commits = %d, want 3", countOf(tr))
	}
	if got := strings.TrimSpace(tr.git("show", "HEAD:f.txt")); got != "C\n2\n3" {
		t.Fatalf("content = %q", got)
	}
	if out := tr.git("status", "--porcelain"); out != "" {
		t.Fatalf("dirty after abort: %q", out)
	}
	// The repository still accepts edits.
	if _, err := tr.r.RewordCommit(ctx, tr.repo, second, "Second, reworded"); err != nil {
		t.Fatalf("reword after abort: %v", err)
	}
}

func TestRewriteKeepsLocalChanges(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commitAll("First")
	tr.write("a.txt", "one\ntwo\n")
	tr.commitAll("Second")
	second := headOf(tr)

	tr.write("a.txt", "one\ntwo\nlocal\n")
	if _, err := tr.r.RewordCommit(ctx, tr.repo, second, "Second, reworded"); err != nil {
		t.Fatalf("reword with local changes: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(tr.dir, "a.txt")); err != nil || string(b) != "one\ntwo\nlocal\n" {
		t.Fatalf("stashed change lost: %q %v", b, err)
	}
	if out := tr.git("status", "--porcelain"); !strings.Contains(out, "a.txt") {
		t.Fatalf("working tree state = %q", out)
	}
	if tr.r.Operation(ctx, tr.repo).Active() {
		t.Fatal("rebase left in progress")
	}
}

func TestDropCommitKeepsFollowingEmptyCommit(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "x\n")
	tr.commitAll("First")
	tr.write("b.txt", "y\n")
	tr.commitAll("Second")
	second := headOf(tr)
	tr.git("rm", "-q", "b.txt")
	tr.git("commit", "-q", "-m", "Remove b")

	if _, err := tr.r.DropCommit(ctx, tr.repo, second); err != nil {
		t.Fatalf("drop: %v", err)
	}
	// Removing a file that the dropped commit added leaves the later commit
	// empty; it stays instead of disappearing with the target.
	if countOf(tr) != 2 {
		t.Fatalf("commits = %d, want 2", countOf(tr))
	}
	if got := subjectOf(tr, "HEAD"); got != "Remove b" {
		t.Fatalf("subject = %q", got)
	}
	if _, err := tr.r.Run(ctx, tr.dir, "cat-file", "-e", "HEAD:b.txt"); err == nil {
		t.Fatal("b.txt should be gone")
	}
}
