package gitcli

import (
	"os"
	"strings"
	"testing"
)

// changedAt lists the paths a commit changed, as `git show --name-only` sees
// them.
func changedAt(t *testing.T, tr *testRepo, rev string) map[string]bool {
	t.Helper()
	out := tr.git("show", "--name-only", "--format=", rev)
	names := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			names[line] = true
		}
	}
	return names
}

// stagedPaths lists what the index still holds for the next commit.
func stagedPaths(t *testing.T, tr *testRepo) map[string]bool {
	t.Helper()
	out := tr.git("diff", "--cached", "--name-only")
	names := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			names[line] = true
		}
	}
	return names
}

func trackedByIndex(t *testing.T, tr *testRepo) map[string]bool {
	t.Helper()
	out := tr.git("ls-files")
	names := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			names[line] = true
		}
	}
	return names
}

// TestCommitPathsCommitsOnlyCheckedWorkingTree is the issue #84 contract:
// checked files are committed from the working tree without staging, and a
// staged file that is not checked stays staged and out of the commit.
func TestCommitPathsCommitsOnlyCheckedWorkingTree(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("checked.txt", "one\n")
	tr.write("staged.txt", "one\n")
	tr.write("unstaged.txt", "one\n")
	tr.commitAll("base")

	tr.write("checked.txt", "two\n")
	tr.write("staged.txt", "two\n")
	tr.git("add", "staged.txt")
	// A second worktree change leaves staged.txt partially staged; neither
	// half may move into the scoped commit or be discarded.
	tr.write("staged.txt", "three\n")
	tr.write("unstaged.txt", "two\n")

	if _, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "checked only", Paths: []string{"checked.txt"}}); err != nil {
		t.Fatal(err)
	}

	committed := changedAt(t, tr, "HEAD")
	if !committed["checked.txt"] || committed["staged.txt"] || committed["unstaged.txt"] {
		t.Fatalf("commit changed %v, want only checked.txt", committed)
	}
	if got := tr.git("show", "HEAD:checked.txt"); got != "two\n" {
		t.Fatalf("HEAD:checked.txt = %q, want the worktree content", got)
	}
	st := tr.status()
	if e, ok := find(st, "checked.txt"); ok && (e.Staged() || e.Unstaged()) {
		t.Fatalf("checked.txt is still dirty: %+v", e)
	}
	if e, _ := find(st, "staged.txt"); !e.Staged() || !e.Unstaged() {
		t.Fatalf("unchecked staged.txt = %+v, want its staged and worktree changes kept", e)
	}
	if e, _ := find(st, "unstaged.txt"); e.Staged() || !e.Unstaged() {
		t.Fatalf("unchecked unstaged.txt = %+v", e)
	}
	if staged := stagedPaths(t, tr); !staged["staged.txt"] {
		t.Fatalf("staged progress lost after a scoped commit: %v", staged)
	}
}

// A checked untracked file is recorded without staging it first, and a checked
// deletion is committed. The path list is literal, so brackets and spaces stay
// part of the name.
func TestCommitPathsUntrackedAndDeleted(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("gone.txt", "bye\n")
	tr.write("kept.txt", "one\n")
	tr.write("other staged.txt", "one\n")
	tr.commitAll("base")

	if err := os.Remove(tr.dir + "/gone.txt"); err != nil {
		t.Fatal(err)
	}
	tr.write("new [1].txt", "fresh\n")
	tr.write("other staged.txt", "two\n")
	tr.git("add", "other staged.txt")
	tr.write("kept.txt", "two\n")

	if _, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "add and delete", Paths: []string{"gone.txt", "new [1].txt"}}); err != nil {
		t.Fatal(err)
	}

	committed := changedAt(t, tr, "HEAD")
	if !committed["gone.txt"] || !committed["new [1].txt"] || committed["kept.txt"] || committed["other staged.txt"] {
		t.Fatalf("commit changed %v", committed)
	}
	if _, err := tr.gitMayFail("cat-file", "-e", "HEAD:gone.txt"); err == nil {
		t.Fatal("gone.txt still exists in the commit")
	}
	if got := tr.git("show", "HEAD:new [1].txt"); got != "fresh\n" {
		t.Fatalf("HEAD:new [1].txt = %q", got)
	}
	if staged := stagedPaths(t, tr); !staged["other staged.txt"] {
		t.Fatalf("unchecked staged path not preserved: %v", staged)
	}
}

// A checked file with staged and unstaged changes commits its worktree
// content, which replaces the staged version in the index; nothing is left
// half-staged for that file.
func TestCommitPathsUsesWorktreeForPartiallyStagedFile(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "a1\n")
	tr.write("b.txt", "b1\n")
	tr.commitAll("base")
	tr.write("a.txt", "a2\n")
	tr.git("add", "a.txt")
	tr.write("a.txt", "a3\n")
	tr.write("b.txt", "b2\n")
	tr.git("add", "b.txt")

	if _, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "a3", Paths: []string{"a.txt"}}); err != nil {
		t.Fatal(err)
	}
	if got := tr.git("show", "HEAD:a.txt"); got != "a3\n" {
		t.Fatalf("HEAD:a.txt = %q, want the worktree version", got)
	}
	if e, ok := find(tr.status(), "a.txt"); ok && (e.Staged() || e.Unstaged()) {
		t.Fatalf("a.txt still dirty after commit: %+v", e)
	}
	if e, _ := find(tr.status(), "b.txt"); !e.Staged() {
		t.Fatalf("unchecked staged b.txt = %+v", e)
	}
}

// The first commit can be scoped too: an untracked checked file is recorded as
// a root commit while the other untracked files stay untracked.
func TestCommitPathsWithoutHead(t *testing.T) {
	tr := newTestRepo(t)
	if st := tr.status(); st.Branch.Head != "" {
		t.Fatalf("repo already has HEAD %q", st.Branch.Head)
	}
	tr.write("first.txt", "first\n")
	tr.write("second.txt", "second\n")

	if _, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "root", Paths: []string{"first.txt"}}); err != nil {
		t.Fatal(err)
	}
	if committed := changedAt(t, tr, "HEAD"); !committed["first.txt"] || committed["second.txt"] {
		t.Fatalf("root commit changed %v, want only first.txt", committed)
	}
	if e, _ := find(tr.status(), "second.txt"); !e.Untracked() {
		t.Fatalf("second.txt = %+v, want untracked", e)
	}
}

// Amend with a checked scope records the checked paths' worktree content in
// place of their content in HEAD and leaves staged files outside the scope
// staged.
func TestCommitPathsAmendOnlyChecked(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "a1\n")
	tr.write("b.txt", "b1\n")
	tr.commitAll("base")
	tr.write("a.txt", "a2\n")
	tr.commitAll("second")

	tr.write("a.txt", "a3\n")
	tr.write("b.txt", "b2\n")
	tr.git("add", "b.txt")
	if _, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "amended", Amend: true, Paths: []string{"a.txt"}}); err != nil {
		t.Fatal(err)
	}
	if got := tr.git("show", "HEAD:a.txt"); got != "a3\n" {
		t.Fatalf("HEAD:a.txt = %q, want the worktree version", got)
	}
	if got := tr.git("show", "HEAD:b.txt"); got != "b1\n" {
		t.Fatalf("HEAD:b.txt = %q, amend expanded to an unchecked file", got)
	}
	if subject := strings.TrimSpace(tr.git("log", "-1", "--format=%s")); subject != "amended" {
		t.Fatalf("subject = %q", subject)
	}
	if e, _ := find(tr.status(), "b.txt"); !e.Staged() {
		t.Fatalf("unchecked staged b.txt = %+v after amend", e)
	}
}

// A staged rename checked through its row commits both sides. The unchecked
// staged file stays staged.
func TestCommitPathsKeepsStagedRenameTogether(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("old.txt", "content\n")
	tr.write("other.txt", "one\n")
	tr.commitAll("base")
	tr.git("mv", "old.txt", "new.txt")
	tr.write("other.txt", "two\n")
	tr.git("add", "other.txt")

	if _, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "rename", Paths: []string{"old.txt", "new.txt"}}); err != nil {
		t.Fatal(err)
	}
	tree := strings.Split(strings.TrimSpace(tr.git("ls-tree", "-r", "--name-only", "HEAD")), "\n")
	has := func(p string) bool {
		for _, name := range tree {
			if name == p {
				return true
			}
		}
		return false
	}
	if has("old.txt") || !has("new.txt") {
		t.Fatalf("tree = %v, want the rename", tree)
	}
	if e, _ := find(tr.status(), "other.txt"); !e.Staged() {
		t.Fatalf("unchecked staged other.txt = %+v", e)
	}
}

// A failed commit leaves the index as it was: the intent-to-add entry it
// needed for the untracked file is removed again, and earlier staged work is
// untouched.
func TestCommitPathsFailureRestoresUntracked(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("staged.txt", "one\n")
	tr.write("a.txt", "one\n")
	tr.commitAll("base")
	tr.write("staged.txt", "two\n")
	tr.git("add", "staged.txt")
	tr.write("new.txt", "fresh\n")

	hook := tr.dir + "/.git/hooks/pre-commit"
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho hook failed >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "rejected", Paths: []string{"new.txt"}})
	if err == nil {
		t.Fatal("commit with a failing hook succeeded")
	}
	if KindOf(err) != ErrHookFailed {
		t.Fatalf("kind = %v, want ErrHookFailed", KindOf(err))
	}
	if e, _ := find(tr.status(), "new.txt"); !e.Untracked() {
		t.Fatalf("new.txt = %+v, want it untracked again", e)
	}
	if tracked := trackedByIndex(t, tr); tracked["new.txt"] {
		t.Fatalf("new.txt left in the index: %v", tracked)
	}
	if e, _ := find(tr.status(), "staged.txt"); !e.Staged() {
		t.Fatalf("unchecked staged work lost: %+v", e)
	}
	if e, ok := find(tr.status(), "staged.txt"); !ok || e.Index != 'M' {
		t.Fatalf("staged.txt index state = %+v", e)
	}
}
