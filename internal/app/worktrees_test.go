package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorktreeCommands drives the Add/Remove worktree commands and checks the
// result with git itself.
func TestWorktreeCommands(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.commitAll("Base")
	f.open()

	wt := filepath.Join(t.TempDir(), "wt-side")
	var err error
	done := false
	f.w.AddWorktree(wt, "side", "HEAD", func(e error) { err, done = e, true })
	f.settle(func() bool { return done })
	if err != nil {
		t.Fatalf("add worktree: %v", err)
	}
	if out := f.git("worktree", "list", "--porcelain"); !strings.Contains(out, wt) {
		t.Fatalf("git worktree list does not contain %s:\n%s", wt, out)
	}
	if _, err := os.Stat(filepath.Join(wt, "a.txt")); err != nil {
		t.Fatalf("worktree checkout missing: %v", err)
	}
	// The state follows: the new branch is known and reported as living in
	// the other worktree.
	f.settle(func() bool { return f.w.Repo.WorktreesLoaded && len(f.w.Repo.Worktrees) == 2 })
	if _, ok := f.w.Repo.WorktreeOf("side"); !ok {
		t.Fatalf("worktree of 'side' not found in %+v", f.w.Repo.Worktrees)
	}
	other, ok := f.w.Repo.WorktreeElsewhere("side")
	if !ok || !samePath(other.Path, wt) {
		t.Fatalf("elsewhere = %+v / %v", other, ok)
	}
	if _, ok := f.w.Repo.WorktreeElsewhere("main"); ok {
		t.Fatal("the current branch is not elsewhere")
	}

	// Removing a dirty worktree without force fails and keeps it.
	if err := os.WriteFile(filepath.Join(wt, "a.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	done = false
	f.w.RemoveWorktree(wt, false, func(e error) { err, done = e, true })
	f.settle(func() bool { return done })
	if err == nil {
		t.Fatal("removing a dirty worktree without force must fail")
	}
	if out := f.git("worktree", "list", "--porcelain"); !strings.Contains(out, wt) {
		t.Fatalf("worktree was removed despite the failure:\n%s", out)
	}

	// Force removes it and the state shrinks back.
	done = false
	f.w.RemoveWorktree(wt, true, func(e error) { err, done = e, true })
	f.settle(func() bool { return done })
	if err != nil {
		t.Fatalf("force remove: %v", err)
	}
	if out := f.git("worktree", "list", "--porcelain"); strings.Contains(out, wt) {
		t.Fatalf("worktree still listed:\n%s", out)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree directory still there: %v", err)
	}
	f.settle(func() bool { return len(f.w.Repo.Worktrees) == 1 })
}

// TestWorktreeStateFromLinkedCheckout opens a linked worktree and checks that
// the current worktree and the branch kept by the main checkout are told apart.
func TestWorktreeStateFromLinkedCheckout(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.commitAll("Base")
	main := f.dir
	wt := filepath.Join(t.TempDir(), "wt-feature")
	f.git("worktree", "add", "-q", "-b", "feature", wt)

	f.dir = wt
	f.open()
	rs := f.w.Repo
	if !rs.WorktreesLoaded || len(rs.Worktrees) != 2 {
		t.Fatalf("worktrees = %+v", rs.Worktrees)
	}
	cur, ok := rs.WorktreeOf("feature")
	if !ok || !rs.IsCurrentWorktree(cur) {
		t.Fatalf("current worktree = %+v / %v", cur, ok)
	}
	other, ok := rs.WorktreeElsewhere("main")
	if !ok || !samePath(other.Path, main) {
		t.Fatalf("main worktree = %+v / %v", other, ok)
	}
	if rs.IsCurrentWorktree(other) {
		t.Fatal("the main worktree reported as current")
	}
}
