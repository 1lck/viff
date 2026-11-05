package app

import (
	"slices"
	"testing"
	"time"
)

// TestWorktreeFiles lists the working tree through the workspace: a tracked and
// an untracked file are present, an ignored file is left out, and the result is
// sorted. A result posted after the repository was closed is dropped instead of
// being delivered for a repository that is no longer open.
func TestWorktreeFiles(t *testing.T) {
	f := newFixture(t)
	f.write("tracked.txt", "one\n")
	f.write("dir with space/中文.txt", "two\n")
	f.commitAll("base")
	f.write(".gitignore", "*.log\n")
	f.git("add", ".gitignore")
	f.git("commit", "-q", "-m", "ignore")
	f.write("untracked.txt", "three\n")
	f.write("ignored.log", "noise\n")
	f.open()

	var list []string
	var err error
	done := false
	f.w.WorktreeFiles(func(l []string, e error) {
		list, err, done = l, e, true
	})
	f.settle(func() bool { return done })
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(list, "tracked.txt") || !slices.Contains(list, "untracked.txt") {
		t.Fatalf("files = %q", list)
	}
	if slices.Contains(list, "ignored.log") {
		t.Fatalf("ignored file listed: %q", list)
	}
	if !slices.IsSorted(list) {
		t.Fatalf("files not sorted: %q", list)
	}

	// A late answer belongs to the repository that asked for it. Close
	// invalidates the generation, so the posted closure must drop the result.
	for f.w.Pending() {
		f.w.Drain()
	}
	f.write("late.txt", "late\n")
	called := false
	f.w.WorktreeFiles(func([]string, error) { called = true })
	f.w.Close()
	// Wait for the query to post its result without draining it. The existing
	// settle helper drains, which would hide whether the guard dropped it, so
	// this polls the same 5 ms interval with a bounded deadline.
	deadline := time.Now().Add(15 * time.Second)
	for !f.w.Pending() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the query result")
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.w.Drain()
	if called {
		t.Fatal("delivered a result after Close")
	}
}
