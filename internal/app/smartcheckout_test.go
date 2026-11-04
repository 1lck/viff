package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
)

// localRef builds the ref the UI passes for a local branch.
func localRef(name string) domain.Ref {
	return domain.Ref{Kind: domain.RefLocal, Name: "refs/heads/" + name, Short: name}
}

// currentBranch returns the checked-out branch, or "" when HEAD is detached.
func (f *fixture) currentBranch() string {
	out := strings.TrimSpace(f.git("rev-parse", "--abbrev-ref", "HEAD"))
	if out == "HEAD" {
		return ""
	}
	return out
}

func (f *fixture) branchIs(name string) bool { return f.currentBranch() == name }

func (f *fixture) read(name string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *fixture) stashList() string { return strings.TrimSpace(f.git("stash", "list")) }

// lastNotice returns the newest notification.
func (f *fixture) lastNotice() Notification {
	f.t.Helper()
	if len(f.w.Notifications) == 0 {
		f.t.Fatal("no notification")
	}
	return f.w.Notifications[len(f.w.Notifications)-1]
}

// TestSmartCheckoutRestoresChanges checks a blocked checkout that restores the
// stashed modifications cleanly: the branch changes at the top of the file,
// the local change at the bottom, so the three-way merge succeeds.
func TestSmartCheckoutRestoresChanges(t *testing.T) {
	f := newFixture(t)
	f.write("doc.txt", "top\none\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n")
	f.commitAll("Base")
	f.git("branch", "other")
	f.git("switch", "-q", "other")
	f.write("doc.txt", "TOP\none\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n")
	f.commitAll("Branch changes the top")
	f.git("switch", "-q", "main")
	f.write("doc.txt", "top\none\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nNINE\n")
	f.write("notes.txt", "untracked stays\n")
	f.open()

	var asked int
	f.w.ConfirmCheckout = func(b CheckoutBlocked) bool {
		asked++
		if b.Ref.Short != "other" || b.Rev != "" {
			t.Fatalf("blocked target = %+v", b)
		}
		f.w.SmartCheckout(b)
		return true
	}

	f.w.Checkout(localRef("other"))
	f.settle(func() bool { return f.idle() && f.branchIs("other") })

	if asked != 1 {
		t.Fatalf("confirmation asked %d times, want 1", asked)
	}
	if got, want := f.read("doc.txt"), "TOP\none\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nNINE\n"; got != want {
		t.Fatalf("doc.txt after checkout =\n%q\nwant\n%q", got, want)
	}
	if got := f.read("notes.txt"); got != "untracked stays\n" {
		t.Fatalf("untracked file was not restored: %q", got)
	}
	if list := f.stashList(); list != "" {
		t.Fatalf("stash should be empty after a clean restore:\n%s", list)
	}
	n := f.lastNotice()
	if n.Kind != NoticeSuccess || n.Title != "Checked out other" {
		t.Fatalf("notification = %+v", n)
	}
}

// TestSmartCheckoutConflictKeepsStash checks that a change that conflicts with
// the target branch stays in the stash, with HEAD moved to the target.
func TestSmartCheckoutConflictKeepsStash(t *testing.T) {
	f := newFixture(t)
	f.write("doc.txt", "alpha\nbeta\n")
	f.commitAll("Base")
	f.git("branch", "other")
	f.git("switch", "-q", "other")
	f.write("doc.txt", "alpha\nOTHER\n")
	f.commitAll("Branch changes the line")
	f.git("switch", "-q", "main")
	f.write("doc.txt", "alpha\nLOCAL\n")
	f.open()

	f.w.ConfirmCheckout = func(b CheckoutBlocked) bool { f.w.SmartCheckout(b); return true }
	f.w.Checkout(localRef("other"))
	f.settle(func() bool { return f.idle() && f.branchIs("other") })

	if list := f.stashList(); !strings.Contains(list, "viff: checkout other") {
		t.Fatalf("conflicting changes must stay in the stash, got:\n%s", list)
	}
	n := f.lastNotice()
	if n.Kind != NoticeWarning || !strings.Contains(n.Body, "still in the stash") {
		t.Fatalf("notification = %+v", n)
	}
}

// TestSmartCheckoutFailedCheckoutRestoresChanges checks the recovery path: the
// stash is popped back and the branch stays put when checkout itself fails.
// A branch checked out in another worktree fails for a different reason, which
// is a convenient way to reach that state.
func TestSmartCheckoutFailedCheckoutRestoresChanges(t *testing.T) {
	f := newFixture(t)
	f.write("doc.txt", "base\n")
	f.commitAll("Base")
	f.git("branch", "held")
	wt := filepath.Join(t.TempDir(), "held-wt")
	f.git("worktree", "add", "-q", wt, "held")
	f.write("doc.txt", "local\n")
	f.open()

	f.w.SmartCheckout(CheckoutBlocked{Ref: localRef("held")})
	f.settle(func() bool { return f.idle() })

	if !f.branchIs("main") {
		t.Fatalf("branch changed after a failed checkout: %q", f.currentBranch())
	}
	if got := f.read("doc.txt"); got != "local\n" {
		t.Fatalf("local change not restored: %q", got)
	}
	if list := f.stashList(); list != "" {
		t.Fatalf("stash must be emptied after the restore:\n%s", list)
	}
	n := f.lastNotice()
	if n.Kind != NoticeError || !strings.Contains(n.Body, "restored") {
		t.Fatalf("notification = %+v", n)
	}
}

// TestCheckoutCleanTreeUnchanged checks the ordinary path: no prompt and no
// stash when the working tree is clean.
func TestCheckoutCleanTreeUnchanged(t *testing.T) {
	f := newFixture(t)
	f.write("doc.txt", "base\n")
	f.commitAll("Base")
	f.git("branch", "other")
	f.git("switch", "-q", "other")
	f.write("doc.txt", "other\n")
	f.commitAll("Other")
	f.git("switch", "-q", "main")
	f.open()

	var asked int
	f.w.ConfirmCheckout = func(CheckoutBlocked) bool { asked++; return true }

	f.w.Checkout(localRef("other"))
	f.settle(func() bool { return f.idle() && f.branchIs("other") })

	if asked != 0 {
		t.Fatalf("clean checkout asked for confirmation")
	}
	if list := f.stashList(); list != "" {
		t.Fatalf("clean checkout created a stash:\n%s", list)
	}
}

// TestCheckoutDeclineChangesKeepsTree checks that declining leaves everything
// alone and explains the failure.
func TestCheckoutDeclineChangesKeepsTree(t *testing.T) {
	f := newFixture(t)
	f.write("doc.txt", "base\n")
	f.commitAll("Base")
	f.git("branch", "other")
	f.git("switch", "-q", "other")
	f.write("doc.txt", "other\n")
	f.commitAll("Other")
	f.git("switch", "-q", "main")
	f.write("doc.txt", "local\n")
	f.open()

	var asked int
	f.w.ConfirmCheckout = func(CheckoutBlocked) bool { asked++; return false }

	f.w.Checkout(localRef("other"))
	f.settle(func() bool { return f.idle() && len(f.w.Notifications) > 0 })

	if asked != 1 {
		t.Fatalf("confirmation asked %d times, want 1", asked)
	}
	if !f.branchIs("main") {
		t.Fatalf("branch changed after declining: %q", f.currentBranch())
	}
	if got := f.read("doc.txt"); got != "local\n" {
		t.Fatalf("declining modified the working tree: %q", got)
	}
	if list := f.stashList(); list != "" {
		t.Fatalf("declining still stashed something:\n%s", list)
	}
	if n := f.lastNotice(); n.Kind != NoticeError || !strings.Contains(n.Body, "Commit, stash or roll back") {
		t.Fatalf("notification = %+v", n)
	}
}

// TestCheckoutBranchInOtherWorktreeUnchanged checks that a block Git reports
// for another reason keeps its own error and never offers to stash.
func TestCheckoutBranchInOtherWorktreeUnchanged(t *testing.T) {
	f := newFixture(t)
	f.write("doc.txt", "base\n")
	f.commitAll("Base")
	f.git("branch", "held")
	wt := filepath.Join(t.TempDir(), "held-wt")
	f.git("worktree", "add", "-q", wt, "held")
	f.write("doc.txt", "local\n")
	f.open()

	var asked int
	f.w.ConfirmCheckout = func(CheckoutBlocked) bool { asked++; return true }

	f.w.Checkout(localRef("held"))
	f.settle(func() bool { return f.idle() && len(f.w.Notifications) > 0 })

	if asked != 0 {
		t.Fatalf("another worktree's branch must not offer a stashing checkout")
	}
	if !f.branchIs("main") {
		t.Fatalf("branch changed: %q", f.currentBranch())
	}
	if list := f.stashList(); list != "" {
		t.Fatalf("stash created for a non-overwrite failure:\n%s", list)
	}
}

// TestSmartCheckoutRevision checks the detached-HEAD variant of the flow.
func TestSmartCheckoutRevision(t *testing.T) {
	f := newFixture(t)
	f.write("doc.txt", "alpha\nbeta\n")
	f.commitAll("Base")
	f.write("doc.txt", "alpha\nBETA\n")
	f.commitAll("Second")
	target := strings.TrimSpace(f.git("rev-parse", "HEAD~1"))
	f.write("doc.txt", "alpha\nLOCAL\n")
	f.open()

	var asked int
	f.w.ConfirmCheckout = func(b CheckoutBlocked) bool {
		asked++
		if b.Rev != target || b.Ref.Short != "" {
			t.Fatalf("blocked target = %+v", b)
		}
		f.w.SmartCheckout(b)
		return true
	}

	f.w.CheckoutRevision(target)
	f.settle(func() bool { return f.idle() && f.w.Repo.Status.Branch.Detached })

	if asked != 1 {
		t.Fatalf("confirmation asked %d times, want 1", asked)
	}
	if got := strings.TrimSpace(f.git("rev-parse", "HEAD")); got != target {
		t.Fatalf("HEAD = %s, want %s", got, target)
	}
	if list := f.stashList(); !strings.Contains(list, "viff: checkout "+domain.ShortHash(target)) {
		t.Fatalf("conflicting revision checkout must keep the stash:\n%s", list)
	}
}
