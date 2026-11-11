package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
)

// commitFiles lists the paths the newest commit changed.
func commitFiles(f *fixture) map[string]bool {
	f.t.Helper()
	out := f.git("show", "--name-only", "--format=", "HEAD")
	names := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			names[line] = true
		}
	}
	return names
}

// lastNotice returns the newest notification, for asserting what the commit
// told the user.
func lastNotice(f *fixture) Notification {
	f.t.Helper()
	if len(f.w.Notifications) == 0 {
		f.t.Fatal("no notifications")
	}
	return f.w.Notifications[len(f.w.Notifications)-1]
}

func seedAndCommit(f *fixture, files map[string]string, message string) {
	f.t.Helper()
	for name, content := range files {
		f.write(name, content)
	}
	f.git("add", "-A")
	f.git("commit", "-q", "-m", message)
}

// TestCommitCheckedPathsScope verifies the app commits exactly the checked
// paths from the working tree and leaves other staged work staged.
func TestCommitCheckedPathsScope(t *testing.T) {
	f := newFixture(t)
	seedAndCommit(f, map[string]string{"checked.txt": "one\n", "staged.txt": "one\n"}, "base")
	f.open()

	f.write("checked.txt", "two\n")
	f.write("staged.txt", "two\n")
	f.git("add", "staged.txt")
	f.write("new.txt", "new\n")
	f.w.Refresh(false)
	f.settle(func() bool { return f.idle() && len(f.w.Repo.Status.Entries) == 3 })

	done := false
	f.w.Commit(CommitRequest{Message: "checked scope", Paths: []string{"checked.txt", "new.txt"}}, func() { done = true })
	f.settle(func() bool { return done && f.idle() })

	committed := commitFiles(f)
	if !committed["checked.txt"] || !committed["new.txt"] || committed["staged.txt"] {
		t.Fatalf("commit changed %v, want the two checked paths", committed)
	}
	if staged := f.git("diff", "--cached", "--name-only"); strings.TrimSpace(staged) != "staged.txt" {
		t.Fatalf("staged index = %q, want only staged.txt", staged)
	}
	n := lastNotice(f)
	if n.Title != "Committed" || !strings.Contains(n.Body, "2 files committed") {
		t.Fatalf("notice = %+v", n)
	}
}

// A checked path that is no longer changed when the write starts makes the
// selection stale; nothing is committed and the view is asked to refresh.
func TestCommitCheckedPathsStaleSelection(t *testing.T) {
	f := newFixture(t)
	seedAndCommit(f, map[string]string{"a.txt": "one\n"}, "base")
	f.open()
	f.write("a.txt", "two\n")
	f.w.Refresh(false)
	f.settle(func() bool { return f.idle() && len(f.w.Repo.Entries(domain.AreaUnstaged)) == 1 })
	// The file is committed or reverted elsewhere between the view and Commit.
	f.git("checkout", "--", "a.txt")
	before := len(f.w.Notifications)
	done := false
	f.w.Commit(CommitRequest{Message: "stale", Paths: []string{"a.txt"}}, func() { done = true })
	f.settle(func() bool { return f.idle() && len(f.w.Notifications) > before })

	if done {
		t.Fatal("a stale selection still ran the after callback")
	}
	if n := lastNotice(f); n.Title != "Commit selection changed" {
		t.Fatalf("notice = %+v", n)
	}
	if subject := strings.TrimSpace(f.git("log", "-1", "--format=%s")); subject != "base" {
		t.Fatalf("HEAD moved to %q", subject)
	}
}

// A merge or cherry-pick in progress records the whole index, so a scoped
// commit is refused with an explanation instead of silently committing more.
func TestCommitCheckedPathsBlockedDuringMerge(t *testing.T) {
	f := newFixture(t)
	seedAndCommit(f, map[string]string{"a.txt": "a1\n", "b.txt": "b1\n"}, "base")
	f.git("switch", "-q", "-c", "side")
	f.write("b.txt", "b2\n")
	f.git("commit", "-q", "-am", "side")
	f.git("switch", "-q", "main")
	f.write("a.txt", "a2\n")
	f.git("commit", "-q", "-am", "main")
	f.git("merge", "--no-commit", "--no-ff", "side")
	f.open()
	if f.w.Repo.Operation.Kind != domain.OpMerge {
		t.Fatalf("operation = %+v, want a merge in progress", f.w.Repo.Operation)
	}

	before := len(f.w.Notifications)
	done := false
	f.w.Commit(CommitRequest{Message: "checked", Paths: []string{"a.txt"}}, func() { done = true })
	f.settle(func() bool { return f.idle() && len(f.w.Notifications) > before })

	if done {
		t.Fatal("a scoped commit ran during a merge")
	}
	if n := lastNotice(f); n.Title != "Cannot commit the checked files" {
		t.Fatalf("notice = %+v", n)
	}
	if subject := strings.TrimSpace(f.git("log", "-1", "--format=%s")); subject != "main" {
		t.Fatalf("HEAD moved to %q", subject)
	}
}

// A failing commit does not run the success callback the UI uses to open Push,
// and it restores an untracked checked file to the index state the user saw.
func TestCommitCheckedPathsFailureDoesNotContinue(t *testing.T) {
	f := newFixture(t)
	seedAndCommit(f, map[string]string{"a.txt": "one\n"}, "base")
	f.open()
	f.write("new.txt", "fresh\n")
	hook := filepath.Join(f.dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho hook failed >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	before := len(f.w.Notifications)
	done := false
	f.w.Commit(CommitRequest{Message: "rejected", Paths: []string{"new.txt"}}, func() { done = true })
	f.settle(func() bool { return f.idle() && len(f.w.Notifications) > before })

	if done {
		t.Fatal("the success callback ran for a failed commit")
	}
	if n := lastNotice(f); n.Title != "Commit failed" {
		t.Fatalf("notice = %+v", n)
	}
	if subject := strings.TrimSpace(f.git("log", "-1", "--format=%s")); subject != "base" {
		t.Fatalf("HEAD moved to %q", subject)
	}
	if tracked := strings.TrimSpace(f.git("ls-files", "new.txt")); tracked != "" {
		t.Fatalf("new.txt left in the index: %q", tracked)
	}
}

// A staged rename checked as one file is committed with both sides and counted
// once in the notification.
func TestCommitCheckedPathsRenameCountsOnce(t *testing.T) {
	f := newFixture(t)
	seedAndCommit(f, map[string]string{"old.txt": "content\n"}, "base")
	f.git("mv", "old.txt", "new.txt")
	f.open()
	done := false
	f.w.Commit(CommitRequest{Message: "rename", Paths: []string{"old.txt", "new.txt"}}, func() { done = true })
	f.settle(func() bool { return done && f.idle() })

	tree := strings.TrimSpace(f.git("ls-tree", "-r", "--name-only", "HEAD"))
	if strings.Contains(tree, "old.txt") || !strings.Contains(tree, "new.txt") {
		t.Fatalf("HEAD tree = %q", tree)
	}
	if n := lastNotice(f); !strings.Contains(n.Body, "1 file committed") {
		t.Fatalf("notice = %+v", n)
	}
}
