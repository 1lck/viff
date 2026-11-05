package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// These tests drive the real commands through the workspace and assert the Git
// state afterwards, because the undo promises are about HEAD, the index, the
// working tree and refs, not about the notifications.

// waitIdle drains background results until the workspace has nothing left.
func (f *fixture) waitIdle() {
	f.t.Helper()
	f.settle(func() bool { return f.idle() })
}

// runCommand starts an app command and waits for its task and refresh.
func (f *fixture) runCommand(fn func()) {
	f.t.Helper()
	fn()
	f.waitIdle()
}

// undo presses Undo and waits for it.
func (f *fixture) undo() {
	f.t.Helper()
	f.runCommand(f.w.Undo)
}

func (f *fixture) revOf(rev string) string {
	f.t.Helper()
	return strings.TrimSpace(f.git("rev-parse", rev))
}

func (f *fixture) porcelain() string {
	f.t.Helper()
	return strings.TrimSpace(f.git("status", "--porcelain"))
}

// commitBase creates one commit directly so the fixture has a state to undo to.
func (f *fixture) commitBase() {
	f.t.Helper()
	f.write("a.txt", "one\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
}

// gitFail runs Git that is expected to fail and returns the output.
func (f *fixture) gitMayFail(args ...string) (string, error) {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestUndoCommitAndAmend(t *testing.T) {
	f := newFixture(t)
	f.commitBase()
	f.open()
	base := f.revOf("HEAD")

	f.write("b.txt", "two\n")
	f.git("add", "-A")
	f.w.Refresh(true)
	f.waitIdle()
	f.runCommand(func() { f.w.Commit(CommitRequest{Message: "Add b"}, nil) })
	committed := f.revOf("HEAD")
	if committed == base {
		t.Fatal("the commit did not move HEAD")
	}
	if got := f.w.UndoName(); got != "Commit" {
		t.Fatalf("UndoName = %q, want Commit", got)
	}

	f.undo()
	if got := f.revOf("HEAD"); got != base {
		t.Fatalf("HEAD after undo = %s, want %s", got, base)
	}
	// reset --soft keeps the index, so the commit's content is staged again and
	// the working tree is untouched.
	if got := strings.TrimSpace(f.git("diff", "--cached", "--name-only")); got != "b.txt" {
		t.Fatalf("staged after undo = %q, want b.txt", got)
	}
	if got := f.porcelain(); got != "A  b.txt" {
		t.Fatalf("status after undo = %q, want the addition staged only", got)
	}
	if n := len(f.w.undos); n != 0 {
		t.Fatalf("undo left %d entries, want the record consumed", n)
	}
	if got := f.w.UndoName(); got != "" {
		t.Fatalf("UndoName after undo = %q, want empty", got)
	}
	if n := f.lastNotice(); n.Kind != NoticeSuccess || n.Title != "Undid Commit" {
		t.Fatalf("notification = %+v, want a success named Undid Commit", n)
	}

	// Amend keeps the earlier commit; undo restores it and stages the amended
	// content so it can be committed again.
	f.runCommand(func() { f.w.Commit(CommitRequest{Message: "Add b again"}, nil) })
	committed = f.revOf("HEAD")
	f.write("b.txt", "two\nthree\n")
	f.git("add", "-A")
	f.w.Refresh(true)
	f.waitIdle()
	f.runCommand(func() { f.w.Commit(CommitRequest{Message: "Add b amended", Amend: true}, nil) })
	if f.revOf("HEAD") == committed {
		t.Fatal("amend did not move HEAD")
	}
	if got := f.w.UndoName(); got != "Amend" {
		t.Fatalf("UndoName = %q, want Amend", got)
	}
	f.undo()
	if got := f.revOf("HEAD"); got != committed {
		t.Fatalf("HEAD after undoing amend = %s, want %s", got, committed)
	}
	if got := strings.TrimSpace(f.git("diff", "--cached", "--name-only")); got != "b.txt" {
		t.Fatalf("staged after undoing amend = %q, want b.txt", got)
	}
}

func TestUndoMergeRebaseAndReset(t *testing.T) {
	f := newFixture(t)
	f.commitBase()
	f.open()
	base := f.revOf("HEAD")

	// A merge that creates a merge commit.
	f.git("checkout", "-q", "-b", "feature")
	f.write("f.txt", "feature\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "feature")
	f.git("checkout", "-q", "main")
	f.w.Refresh(true)
	f.waitIdle()
	f.runCommand(func() { f.w.Merge("feature", true) })
	merged := f.revOf("HEAD")
	if merged == base {
		t.Fatal("merge did not move HEAD")
	}
	if got := f.w.UndoName(); got != "Merge" {
		t.Fatalf("UndoName = %q, want Merge", got)
	}
	f.undo()
	if got := f.revOf("HEAD"); got != base {
		t.Fatalf("HEAD after undoing merge = %s, want %s", got, base)
	}
	if got := f.porcelain(); got != "" {
		t.Fatalf("worktree after undoing merge = %q, want clean", got)
	}
	if got := f.revOf("feature"); got == base {
		t.Fatal("undoing the merge must not move the merged branch")
	}

	// A rebase that replays main's history onto a moved main.
	f.git("checkout", "-q", "-b", "topic")
	f.write("t.txt", "topic\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "topic")
	topicTip := f.revOf("HEAD")
	f.git("checkout", "-q", "main")
	f.write("m.txt", "main\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "main moved")
	f.git("checkout", "-q", "topic")
	f.w.Refresh(true)
	f.waitIdle()
	f.runCommand(func() { f.w.Rebase("main") })
	if f.revOf("HEAD") == topicTip {
		t.Fatal("rebase did not rewrite the topic branch")
	}
	if got := f.w.UndoName(); got != "Rebase" {
		t.Fatalf("UndoName = %q, want Rebase", got)
	}
	f.undo()
	if got := f.revOf("HEAD"); got != topicTip {
		t.Fatalf("HEAD after undoing rebase = %s, want %s", got, topicTip)
	}
	if got := f.revOf("topic"); got != topicTip {
		t.Fatalf("topic after undoing rebase = %s, want %s", got, topicTip)
	}
	if got := f.porcelain(); got != "" {
		t.Fatalf("worktree after undoing rebase = %q, want clean", got)
	}

	// A hard reset.
	f.git("checkout", "-q", "main")
	f.write("later.txt", "later\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "later")
	later := f.revOf("HEAD")
	f.w.Refresh(true)
	f.waitIdle()
	f.runCommand(func() { f.w.Reset(gitcli.ResetHard, "HEAD~1") })
	if f.revOf("HEAD") == later {
		t.Fatal("reset did not move HEAD")
	}
	if got := f.w.UndoName(); got != "Reset" {
		t.Fatalf("UndoName = %q, want Reset", got)
	}
	f.undo()
	if got := f.revOf("HEAD"); got != later {
		t.Fatalf("HEAD after undoing reset = %s, want %s", got, later)
	}
	if got := f.porcelain(); got != "" {
		t.Fatalf("worktree after undoing reset = %q, want clean", got)
	}
}

func TestUndoCherryPickAndRevert(t *testing.T) {
	f := newFixture(t)
	f.commitBase()
	f.open()
	base := f.revOf("HEAD")

	f.git("checkout", "-q", "-b", "side")
	f.write("side.txt", "side\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "side work")
	pick := f.revOf("HEAD")
	f.git("checkout", "-q", "main")
	f.w.Refresh(true)
	f.waitIdle()

	f.runCommand(func() { f.w.CherryPick([]string{pick}) })
	if f.revOf("HEAD") == base {
		t.Fatal("cherry-pick did not move HEAD")
	}
	if got := f.w.UndoName(); got != "Cherry-pick" {
		t.Fatalf("UndoName = %q, want Cherry-pick", got)
	}
	f.undo()
	if got := f.revOf("HEAD"); got != base {
		t.Fatalf("HEAD after undoing cherry-pick = %s, want %s", got, base)
	}
	if got := f.porcelain(); got != "" {
		t.Fatalf("worktree after undoing cherry-pick = %q, want clean", got)
	}

	// Revert creates a commit; undoing it removes the revert commit only.
	f.runCommand(func() { f.w.CherryPick([]string{pick}) })
	picked := f.revOf("HEAD")
	f.w.Refresh(true)
	f.waitIdle()
	f.runCommand(func() { f.w.RevertCommits([]string{picked}) })
	if f.revOf("HEAD") == picked {
		t.Fatal("revert did not create a commit")
	}
	if got := f.w.UndoName(); got != "Revert" {
		t.Fatalf("UndoName = %q, want Revert", got)
	}
	f.undo()
	if got := f.revOf("HEAD"); got != picked {
		t.Fatalf("HEAD after undoing revert = %s, want %s", got, picked)
	}
	if got := f.porcelain(); got != "" {
		t.Fatalf("worktree after undoing revert = %q, want clean", got)
	}
}

func TestUndoCheckout(t *testing.T) {
	f := newFixture(t)
	f.commitBase()
	f.open()
	base := f.revOf("HEAD")
	f.git("branch", "feature")
	f.git("checkout", "-q", "-b", "detached-source")
	f.write("d.txt", "d\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "detached source")
	detached := f.revOf("HEAD")
	f.git("checkout", "-q", "main")
	f.w.Refresh(true)
	f.waitIdle()

	f.runCommand(func() { f.w.Checkout(domain.Ref{Kind: domain.RefLocal, Short: "feature"}) })
	if got := f.w.Repo.CurrentBranch(); got != "feature" {
		t.Fatalf("branch after checkout = %q, want feature", got)
	}
	if got := f.w.UndoName(); got != "Checkout" {
		t.Fatalf("UndoName = %q, want Checkout", got)
	}
	f.undo()
	if got := f.w.Repo.CurrentBranch(); got != "main" {
		t.Fatalf("branch after undoing checkout = %q, want main", got)
	}
	if got := f.revOf("HEAD"); got != base {
		t.Fatalf("HEAD after undoing checkout = %s, want %s", got, base)
	}

	// A detached checkout is undone by detaching at the recorded commit, or by
	// returning to the branch that was checked out before.
	f.runCommand(func() { f.w.CheckoutRevision(detached) })
	if !f.w.Repo.Status.Branch.Detached {
		t.Fatal("checkout revision did not detach HEAD")
	}
	f.undo()
	if got := f.w.Repo.CurrentBranch(); got != "main" {
		t.Fatalf("branch after undoing detached checkout = %q, want main", got)
	}
	if got := f.revOf("HEAD"); got != base {
		t.Fatalf("HEAD after undoing detached checkout = %s, want %s", got, base)
	}
}

func TestUndoBranchOperations(t *testing.T) {
	f := newFixture(t)
	f.commitBase()
	f.open()
	base := f.revOf("HEAD")

	// Create without checkout: undo deletes the branch and leaves HEAD alone.
	f.runCommand(func() { f.w.NewBranch("topic", "", false, false) })
	if got := f.revOf("refs/heads/topic"); got != base {
		t.Fatalf("topic = %s, want %s", got, base)
	}
	if got := f.w.UndoName(); got != "Create branch" {
		t.Fatalf("UndoName = %q, want Create branch", got)
	}
	f.undo()
	if _, err := f.gitMayFail("rev-parse", "--verify", "refs/heads/topic"); err == nil {
		t.Fatal("undo left the created branch behind")
	}
	if got := f.revOf("HEAD"); got != base {
		t.Fatalf("HEAD changed while undoing branch creation: %s", got)
	}

	// Create with checkout: undo returns to the previous branch and deletes the
	// new one.
	f.runCommand(func() { f.w.NewBranch("work", "", true, false) })
	if got := f.w.Repo.CurrentBranch(); got != "work" {
		t.Fatalf("branch after create-with-checkout = %q, want work", got)
	}
	f.undo()
	if got := f.w.Repo.CurrentBranch(); got != "main" {
		t.Fatalf("branch after undoing create-with-checkout = %q, want main", got)
	}
	if _, err := f.gitMayFail("rev-parse", "--verify", "refs/heads/work"); err == nil {
		t.Fatal("undo left the checked-out branch behind")
	}

	// Delete: undo recreates the branch at the commit it had.
	f.git("branch", "feature")
	feature := f.revOf("feature")
	f.w.Refresh(true)
	f.waitIdle()
	f.runCommand(func() { f.w.DeleteBranch(domain.Ref{Kind: domain.RefLocal, Short: "feature"}, false) })
	if _, err := f.gitMayFail("rev-parse", "--verify", "refs/heads/feature"); err == nil {
		t.Fatal("delete left the branch behind")
	}
	if got := f.w.UndoName(); got != "Delete branch" {
		t.Fatalf("UndoName = %q, want Delete branch", got)
	}
	f.undo()
	if got := f.revOf("refs/heads/feature"); got != feature {
		t.Fatalf("restored branch = %s, want %s", got, feature)
	}

	// Rename: undo renames back and refuses once the branch has moved.
	f.runCommand(func() { f.w.RenameBranch("feature", "renamed") })
	if _, err := f.gitMayFail("rev-parse", "--verify", "refs/heads/feature"); err == nil {
		t.Fatal("rename left the old name behind")
	}
	if got := f.w.UndoName(); got != "Rename branch" {
		t.Fatalf("UndoName = %q, want Rename branch", got)
	}
	f.undo()
	if got := f.revOf("refs/heads/feature"); got != feature {
		t.Fatalf("branch after undoing rename = %s, want %s", got, feature)
	}
	if _, err := f.gitMayFail("rev-parse", "--verify", "refs/heads/renamed"); err == nil {
		t.Fatal("undo left the renamed branch behind")
	}

	// Overwrite: creating a branch over an existing name restores the old
	// commit instead of deleting the branch.
	f.write("later.txt", "later\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "later")
	later := f.revOf("HEAD")
	f.git("branch", "over", base)
	f.w.Refresh(true)
	f.waitIdle()
	f.runCommand(func() { f.w.NewBranch("over", "HEAD", false, true) })
	if got := f.revOf("over"); got != later {
		t.Fatalf("overwritten branch = %s, want %s", got, later)
	}
	f.undo()
	if got := f.revOf("over"); got != base {
		t.Fatalf("branch after undoing overwrite = %s, want %s", got, base)
	}
}

func TestUndoRefusesMovedBranch(t *testing.T) {
	f := newFixture(t)
	f.commitBase()
	f.open()

	f.runCommand(func() { f.w.NewBranch("moved", "", false, false) })
	// Somebody commits on the branch after it was created. Undo must not
	// delete the branch and lose that work.
	f.git("checkout", "-q", "moved")
	f.write("m.txt", "m\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "work on moved")
	moved := f.revOf("HEAD")
	f.git("checkout", "-q", "main")
	f.w.Refresh(true)
	f.waitIdle()

	f.undo()
	if n := f.lastNotice(); n.Kind != NoticeWarning || n.Title != "Cannot undo Create branch" {
		t.Fatalf("notification = %+v, want a refusal", n)
	}
	if got := f.revOf("refs/heads/moved"); got != moved {
		t.Fatalf("branch = %s, want it kept at %s", got, moved)
	}
	if n := len(f.w.undos); n != 1 {
		t.Fatalf("refused undo consumed the record: %d entries", n)
	}
}

func TestUndoRefusals(t *testing.T) {
	t.Run("nothing to undo", func(t *testing.T) {
		f := newFixture(t)
		f.commitBase()
		f.open()
		// A failing command leaves no record: there is nothing to undo yet.
		f.runCommand(func() { f.w.Commit(CommitRequest{Message: "nothing staged"}, nil) })
		if n := len(f.w.undos); n != 0 {
			t.Fatalf("failed commit recorded %d entries", n)
		}
		f.undo()
		if n := f.lastNotice(); n.Kind != NoticeInfo || n.Title != "Nothing to undo" {
			t.Fatalf("notification = %+v, want Nothing to undo", n)
		}
	})

	t.Run("dirty worktree", func(t *testing.T) {
		f := newFixture(t)
		f.commitBase()
		f.open()
		base := f.revOf("HEAD")
		f.git("checkout", "-q", "-b", "feature")
		f.write("f.txt", "f\n")
		f.git("add", "-A")
		f.git("commit", "-q", "-m", "feature")
		f.git("checkout", "-q", "main")
		f.w.Refresh(true)
		f.waitIdle()
		f.runCommand(func() { f.w.Merge("feature", true) })
		merged := f.revOf("HEAD")
		f.write("local.txt", "uncommitted\n")

		f.undo()
		n := f.lastNotice()
		if n.Kind != NoticeWarning || n.Title != "Cannot undo Merge" {
			t.Fatalf("notification = %+v, want a refusal", n)
		}
		if !strings.Contains(n.Body, "Commit, stash or discard") {
			t.Fatalf("refusal body = %q, want the dirty-worktree reason", n.Body)
		}
		if got := f.revOf("HEAD"); got != merged {
			t.Fatalf("refused undo moved HEAD to %s, want %s", got, merged)
		}
		if got := f.revOf("HEAD~1"); got != base {
			t.Fatalf("refused undo changed history: HEAD~1 = %s, want %s", got, base)
		}
	})

	t.Run("head moved outside", func(t *testing.T) {
		f := newFixture(t)
		f.commitBase()
		f.open()
		f.write("b.txt", "two\n")
		f.git("add", "-A")
		f.w.Refresh(true)
		f.waitIdle()
		f.runCommand(func() { f.w.Commit(CommitRequest{Message: "Add b"}, nil) })
		committed := f.revOf("HEAD")

		// Another tool commits after the record was taken.
		f.write("c.txt", "three\n")
		f.git("add", "-A")
		f.git("commit", "-q", "-m", "outside commit")
		outside := f.revOf("HEAD")

		f.undo()
		n := f.lastNotice()
		if n.Kind != NoticeWarning || n.Title != "Cannot undo Commit" {
			t.Fatalf("notification = %+v, want a refusal", n)
		}
		if !strings.Contains(n.Body, "HEAD or the current branch changed") {
			t.Fatalf("refusal body = %q, want the moved-HEAD reason", n.Body)
		}
		if got := f.revOf("HEAD"); got != outside {
			t.Fatalf("refused undo moved HEAD to %s, want %s", got, outside)
		}
		if got := f.revOf("HEAD~1"); got != committed {
			t.Fatalf("refused undo rewrote history: HEAD~1 = %s, want %s", got, committed)
		}
	})

	t.Run("operation in progress", func(t *testing.T) {
		f := newFixture(t)
		f.commitBase()
		f.open()
		f.write("a.txt", "app\n")
		f.git("add", "-A")
		f.w.Refresh(true)
		f.waitIdle()
		f.runCommand(func() { f.w.Commit(CommitRequest{Message: "app change"}, nil) })
		committed := f.revOf("HEAD")

		f.git("checkout", "-q", "-b", "conflict", "HEAD~1")
		f.write("a.txt", "conflict\n")
		f.git("add", "-A")
		f.git("commit", "-q", "-m", "conflict change")
		f.git("checkout", "-q", "main")
		f.gitFails("merge", "conflict")
		f.w.Refresh(false)
		f.waitIdle()

		f.undo()
		n := f.lastNotice()
		if n.Kind != NoticeWarning || n.Title != "Cannot undo Commit" {
			t.Fatalf("notification = %+v, want a refusal", n)
		}
		if !strings.Contains(n.Body, "Finish or abort") {
			t.Fatalf("refusal body = %q, want the in-progress reason", n.Body)
		}
		if got := f.revOf("HEAD"); got != committed {
			t.Fatalf("refused undo moved HEAD to %s, want %s", got, committed)
		}
	})
}

func TestUndoClearedOnRepositorySwitch(t *testing.T) {
	f := newFixture(t)
	f.commitBase()
	f.open()
	f.write("b.txt", "two\n")
	f.git("add", "-A")
	f.w.Refresh(true)
	f.waitIdle()
	f.runCommand(func() { f.w.Commit(CommitRequest{Message: "Add b"}, nil) })
	if len(f.w.undos) == 0 {
		t.Fatal("the commit was not recorded")
	}

	other, _ := filepath.EvalSymlinks(t.TempDir())
	f.gitIn(other, "init", "-q", "-b", "main")
	f.gitIn(other, "config", "user.name", "Viff Test")
	f.gitIn(other, "config", "user.email", "test@example.com")
	f.gitIn(other, "config", "commit.gpgsign", "false")
	f.w.Open(other, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle(func() bool { return f.w.Repo != nil && f.w.Repo.Repo.Root == other && f.idle() })
	if n := len(f.w.undos); n != 0 {
		t.Fatalf("switch kept %d undo entries", n)
	}
	if got := f.w.UndoName(); got != "" {
		t.Fatalf("UndoName after switch = %q, want empty", got)
	}
}

func TestUndoHistoryLimit(t *testing.T) {
	f := newFixture(t)
	f.commitBase()
	f.open()
	for i := 0; i < undoLimit+5; i++ {
		f.w.recordUndo(&undoEntry{
			kind:     undoCommit,
			repoRoot: f.w.Repo.Repo.Root,
			before:   gitcli.HeadState{Head: "before"},
			after:    gitcli.HeadState{Head: fmt.Sprintf("after-%d", i)},
		})
	}
	if got := len(f.w.undos); got != undoLimit {
		t.Fatalf("history = %d entries, want %d", got, undoLimit)
	}
	if got := f.w.undos[0].after.Head; got != "after-5" {
		t.Fatalf("oldest kept entry = %q, want after-5", got)
	}
}
