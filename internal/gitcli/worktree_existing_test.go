package gitcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// existingWorktreeTemp returns a temporary directory with symlinks resolved,
// so paths compare equal to the ones Git prints in worktree list.
func existingWorktreeTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// checkoutSnapshot captures the parts of a checkout a worktree command must
// not disturb: HEAD, the index and the working tree.
func (tr *testRepo) checkoutSnapshot() string {
	tr.t.Helper()
	return strings.Join([]string{
		tr.git("rev-parse", "HEAD"),
		tr.git("ls-files", "-s"),
		tr.git("status", "--porcelain=v2"),
		tr.git("diff", "--binary"),
		tr.git("diff", "--cached", "--binary"),
	}, "\x00")
}

func (tr *testRepo) branchSnapshot() string {
	tr.t.Helper()
	return tr.git("for-each-ref", "--format=%(refname) %(objectname)", "refs/heads")
}

func (tr *testRepo) worktreeSnapshot() string {
	tr.t.Helper()
	return tr.git("worktree", "list", "--porcelain")
}

func sameTestPath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// TestAddWorktreeExistingChecksOutLocalBranch covers the success path: an
// existing branch is checked out in a new worktree while the current
// checkout, its index and the branch tip stay exactly as they were.
func TestAddWorktreeExistingChecksOutLocalBranch(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "base\n")
	tr.commitAll("base")
	tr.git("branch", "side")

	// Dirty the current checkout: adding a worktree must not touch it.
	tr.write("a.txt", "dirty\n")
	tr.write("notes.txt", "untracked\n")
	tr.write("staged.txt", "staged\n")
	tr.git("add", "staged.txt")

	before := tr.checkoutSnapshot()
	branchesBefore := tr.branchSnapshot()
	sideBefore := strings.TrimSpace(tr.git("rev-parse", "side"))
	mainBefore := strings.TrimSpace(tr.git("rev-parse", "main"))

	wt := filepath.Join(existingWorktreeTemp(t), "wt-side")
	if err := tr.r.AddWorktreeExisting(ctx, tr.repo, wt, "side"); err != nil {
		t.Fatalf("add worktree for existing branch: %v", err)
	}

	// The new worktree is attached to the branch and holds its committed
	// files only, not the dirty state of the main checkout.
	if got := strings.TrimSpace(tr.git("-C", wt, "symbolic-ref", "--short", "HEAD")); got != "side" {
		t.Fatalf("worktree HEAD = %q, want side", got)
	}
	if got := tr.git("-C", wt, "status", "--porcelain=v2"); strings.TrimSpace(got) != "" {
		t.Fatalf("new worktree is not clean:\n%s", got)
	}
	if body, err := os.ReadFile(filepath.Join(wt, "a.txt")); err != nil || string(body) != "base\n" {
		t.Fatalf("worktree file = %q, %v", body, err)
	}
	for _, name := range []string{"notes.txt", "staged.txt"} {
		if _, err := os.Stat(filepath.Join(wt, name)); !os.IsNotExist(err) {
			t.Fatalf("uncommitted file %s leaked into the new worktree: %v", name, err)
		}
	}

	list, err := tr.r.Worktrees(ctx, tr.repo)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range list {
		if w.Branch == "side" && sameTestPath(w.Path, wt) {
			found = true
		}
	}
	if !found {
		t.Fatalf("worktrees = %+v, want side at %s", list, wt)
	}

	// Neither tip moved, and no branch was created or removed.
	if got := strings.TrimSpace(tr.git("rev-parse", "side")); got != sideBefore {
		t.Fatalf("side tip moved: %s -> %s", sideBefore, got)
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "HEAD")); got != mainBefore {
		t.Fatalf("HEAD moved: %s -> %s", mainBefore, got)
	}
	if got := tr.branchSnapshot(); got != branchesBefore {
		t.Fatalf("branches changed:\n%s\nwant:\n%s", got, branchesBefore)
	}
	if got := tr.checkoutSnapshot(); got != before {
		t.Fatalf("current checkout changed:\n%s\nwant:\n%s", got, before)
	}
}

// TestAddWorktreeExistingRefusals covers the requests that must fail without
// touching HEAD, the index, the working tree, the branches or other worktrees.
func TestAddWorktreeExistingRefusals(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "base\n")
	tr.commitAll("base")
	tr.git("branch", "taken")
	tr.git("branch", "free")

	takenWT := filepath.Join(existingWorktreeTemp(t), "taken")
	tr.git("worktree", "add", "-q", takenWT, "taken")

	// A prunable worktree still holds its branch for Git, so it must be
	// refused too until it is pruned.
	tr.git("branch", "pruned")
	prunedWT := filepath.Join(existingWorktreeTemp(t), "pruned")
	tr.git("worktree", "add", "-q", prunedWT, "pruned")
	if err := os.RemoveAll(prunedWT); err != nil {
		t.Fatal(err)
	}

	// A remote-only branch must never be checked out by this command.
	tr.git("update-ref", "refs/remotes/origin/remote-only", "HEAD")

	tr.write("a.txt", "dirty\n")
	tr.write("notes.txt", "untracked\n")

	dir := existingWorktreeTemp(t)
	conflict := filepath.Join(dir, "conflict")
	if err := os.MkdirAll(conflict, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(conflict, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		path   string
		branch string
		want   string
	}{
		{"empty branch", filepath.Join(dir, "empty"), "", "branch name is empty"},
		{"option injection", filepath.Join(dir, "option"), "--force", "invalid branch name"},
		{"missing branch", filepath.Join(dir, "missing"), "nope", "not an existing local branch"},
		{"remote-only branch", filepath.Join(dir, "remote"), "origin/remote-only", "not an existing local branch"},
		{"current branch", filepath.Join(dir, "current"), "main", "already checked out"},
		{"checked out elsewhere", filepath.Join(dir, "busy"), "taken", "already checked out"},
		{"checked out in prunable worktree", filepath.Join(dir, "pruned"), "pruned", "already checked out"},
		{"option injection in path", "-viff-worktree", "free", "invalid worktree path"},
		{"occupied path", conflict, "free", "already exists"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := tr.checkoutSnapshot()
			branches := tr.branchSnapshot()
			worktrees := tr.worktreeSnapshot()

			err := tr.r.AddWorktreeExisting(ctx, tr.repo, tc.path, tc.branch)
			if err == nil {
				t.Fatalf("add worktree %q for %q unexpectedly succeeded", tc.path, tc.branch)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.want)
			}

			if got := tr.checkoutSnapshot(); got != before {
				t.Fatalf("current checkout changed:\n%s\nwant:\n%s", got, before)
			}
			if got := tr.branchSnapshot(); got != branches {
				t.Fatalf("branches changed:\n%s\nwant:\n%s", got, branches)
			}
			if got := tr.worktreeSnapshot(); got != worktrees {
				t.Fatalf("worktrees changed:\n%s\nwant:\n%s", got, worktrees)
			}
			if tc.name == "occupied path" {
				if body, err := os.ReadFile(keep); err != nil || string(body) != "keep\n" {
					t.Fatalf("occupied path was modified: %q, %v", body, err)
				}
				return
			}
			if tc.name == "option injection in path" {
				if _, err := os.Stat(filepath.Join(tr.dir, tc.path)); !os.IsNotExist(err) {
					t.Fatalf("dash path was created: %v", err)
				}
				return
			}
			if _, err := os.Stat(tc.path); !os.IsNotExist(err) {
				t.Fatalf("refused request created %s: %v", tc.path, err)
			}
		})
	}
}

// TestAddWorktreeExistingRefusesBranchNamedHEAD covers a local ref that exists
// but whose name Git resolves specially: refs/heads/HEAD pointing at a commit
// other than HEAD. Without the check-ref-format pre-check, worktree add would
// silently create a detached checkout at HEAD instead of checking out the ref
// or failing.
func TestAddWorktreeExistingRefusesBranchNamedHEAD(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "base\n")
	tr.commitAll("base")
	baseTip := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	tr.write("a.txt", "second\n")
	tr.commitAll("second")
	currentTip := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	if currentTip == baseTip {
		t.Fatal("setup: HEAD must differ from refs/heads/HEAD")
	}
	// Git refuses to create this branch itself, but update-ref can.
	tr.git("update-ref", "refs/heads/HEAD", baseTip)

	before := tr.checkoutSnapshot()
	branches := tr.branchSnapshot()
	worktrees := tr.worktreeSnapshot()
	wt := filepath.Join(existingWorktreeTemp(t), "wt-head")

	err := tr.r.AddWorktreeExisting(ctx, tr.repo, wt, "HEAD")
	if err == nil {
		t.Fatal("checking out the special refs/heads/HEAD unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "invalid branch name") {
		t.Fatalf("error = %v, want an invalid branch name error", err)
	}

	// Nothing was written: no detached checkout, no ref or index change.
	if _, statErr := os.Stat(wt); !os.IsNotExist(statErr) {
		t.Fatalf("refused request created %s: %v", wt, statErr)
	}
	if got := tr.checkoutSnapshot(); got != before {
		t.Fatalf("current checkout changed:\n%s\nwant:\n%s", got, before)
	}
	if got := tr.branchSnapshot(); got != branches {
		t.Fatalf("branches changed:\n%s\nwant:\n%s", got, branches)
	}
	if got := tr.worktreeSnapshot(); got != worktrees {
		t.Fatalf("worktrees changed:\n%s\nwant:\n%s", got, worktrees)
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "refs/heads/main")); got != currentTip {
		t.Fatalf("main moved: %s -> %s", currentTip, got)
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "refs/heads/HEAD")); got != baseTip {
		t.Fatalf("refs/heads/HEAD moved: %s -> %s", baseTip, got)
	}
}

// TestAddWorktreeNewBranchStillWorks guards the original AddWorktree behavior:
// it creates the branch at the start point and checks it out in the worktree.
func TestAddWorktreeNewBranchStillWorks(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "base\n")
	tr.commitAll("base")
	head := strings.TrimSpace(tr.git("rev-parse", "HEAD"))
	before := tr.checkoutSnapshot()

	wt := filepath.Join(existingWorktreeTemp(t), "wt-new")
	if err := tr.r.AddWorktree(ctx, tr.repo, wt, "brand-new", "HEAD"); err != nil {
		t.Fatalf("add worktree with new branch: %v", err)
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "refs/heads/brand-new")); got != head {
		t.Fatalf("new branch tip = %s, want %s", got, head)
	}
	if got := strings.TrimSpace(tr.git("-C", wt, "symbolic-ref", "--short", "HEAD")); got != "brand-new" {
		t.Fatalf("worktree HEAD = %q, want brand-new", got)
	}
	if _, err := os.Stat(filepath.Join(wt, "a.txt")); err != nil {
		t.Fatalf("worktree checkout missing: %v", err)
	}
	if got := tr.checkoutSnapshot(); got != before {
		t.Fatalf("current checkout changed:\n%s\nwant:\n%s", got, before)
	}
}
