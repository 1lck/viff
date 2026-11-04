package app

import (
	"strings"
	"testing"
)

// TestScenarioEditCommitHistory drives reword, squash and drop through the
// workspace and checks the repository Git reports afterwards.
func TestScenarioEditCommitHistory(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("First")
	f.write("b.txt", "two\n")
	f.commitAll("Second")
	second := f.head()
	f.write("c.txt", "three\n")
	f.commitAll("Third")
	third := f.head()
	f.write("d.txt", "four\n")
	f.commitAll("Fourth")
	f.open()

	// Reword keeps the tree and every commit after it.
	tree := strings.TrimSpace(f.git("rev-parse", "HEAD^{tree}"))
	f.w.RewordCommit(second, "Second, reworded")
	f.settle(f.idle)
	if got := strings.TrimSpace(f.git("rev-parse", "HEAD^{tree}")); got != tree {
		t.Fatalf("tree changed after reword: %s", got)
	}
	if got := subjectAt(f, "HEAD^^"); got != "Second, reworded" {
		t.Fatalf("reworded subject = %q", got)
	}
	if got := strings.TrimSpace(f.git("rev-list", "--count", "HEAD")); got != "4" {
		t.Fatalf("commits after reword = %s", got)
	}

	// Squash the third commit into the reworded second one.
	third = strings.TrimSpace(f.git("rev-parse", "HEAD^"))
	f.w.SquashCommit(third, "Second and third")
	f.settle(f.idle)
	if got := strings.TrimSpace(f.git("rev-list", "--count", "HEAD")); got != "3" {
		t.Fatalf("commits after squash = %s", got)
	}
	if got := subjectAt(f, "HEAD^"); got != "Second and third" {
		t.Fatalf("squashed subject = %q", got)
	}
	for _, p := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		f.git("cat-file", "-e", "HEAD:"+p)
	}

	// Drop the combined commit: only its files disappear, the fourth stays.
	combined := strings.TrimSpace(f.git("rev-parse", "HEAD^"))
	f.w.DropCommit(combined)
	f.settle(f.idle)
	if got := strings.TrimSpace(f.git("rev-list", "--count", "HEAD")); got != "2" {
		t.Fatalf("commits after drop = %s", got)
	}
	if got := strings.TrimSpace(f.git("log", "-1", "--format=%s")); got != "Fourth" {
		t.Fatalf("HEAD subject = %q", got)
	}
	if out := f.git("status", "--porcelain"); out != "" {
		t.Fatalf("dirty after edits: %q", out)
	}
	if errs := f.errorsSince(0); len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(f.w.Notifications) == 0 {
		t.Fatal("no notification for the edits")
	}
	last := f.w.Notifications[len(f.w.Notifications)-1]
	if last.Kind != NoticeSuccess || !strings.Contains(last.Title, "Dropped") {
		t.Fatalf("last notification = %+v", last)
	}
}

// TestScenarioRefusedHistoryEdit checks that a refused edit is a warning, not
// an error, and leaves the repository untouched.
func TestScenarioRefusedHistoryEdit(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("First")
	root := f.head()
	f.write("a.txt", "one\ntwo\n")
	f.commitAll("Second")
	f.open()

	before := f.head()
	notices := len(f.w.Notifications)
	f.w.RewordCommit(root, "Root, reworded")
	f.settle(f.idle)
	if got := f.head(); got != before {
		t.Fatalf("refused edit moved HEAD to %s", got)
	}
	if len(f.w.Notifications) != notices+1 {
		t.Fatalf("notifications = %d, want one more", len(f.w.Notifications)-notices)
	}
	n := f.w.Notifications[len(f.w.Notifications)-1]
	if n.Kind != NoticeWarning || !strings.Contains(n.Body, "first commit") {
		t.Fatalf("notification = %+v", n)
	}
	if errs := f.errorsSince(0); len(errs) != 0 {
		t.Fatalf("refusal reported as an error: %+v", errs)
	}
}

func subjectAt(f *fixture, rev string) string {
	return strings.TrimSpace(f.git("log", "-1", "--format=%s", rev, "--"))
}
