package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

func testCommit(hash, subject string, parents ...string) domain.Commit {
	return domain.Commit{Hash: hash, Subject: subject, Parents: parents}
}

// TestAggregateMultiCommitsUnion checks the aggregation rule: one row per path,
// sources kept in selection order, deletions and renames explained, and the
// commit order never re-sorted.
func TestAggregateMultiCommitsUnion(t *testing.T) {
	newer := testCommit("aaaaaaaa1111", "newer", "parent-a")
	older := testCommit("bbbbbbbb2222", "older", "parent-b")
	per := []gitcli.MultiCommitChanges{
		{Commit: newer, Changes: []domain.Change{
			{Path: "a.txt", Kind: domain.ChangeModified},
			{Path: "b.txt", Kind: domain.ChangeDeleted},
			{Path: "new/name.txt", OldPath: "old/name.txt", Kind: domain.ChangeRenamed},
		}},
		{Commit: older, Changes: []domain.Change{
			{Path: "a.txt", Kind: domain.ChangeModified},
			{Path: "c.txt", Kind: domain.ChangeAdded},
		}},
	}
	sum := aggregateMultiCommits(per)
	if sum.Err != nil {
		t.Fatalf("Err = %v", sum.Err)
	}
	if len(sum.Commits) != 2 || sum.Commits[0].Hash != newer.Hash || sum.Commits[1].Hash != older.Hash {
		t.Fatalf("Commits = %+v, want the selection order kept", sum.Commits)
	}
	wantPaths := []string{"a.txt", "b.txt", "c.txt", "new/name.txt"}
	if len(sum.Files) != len(wantPaths) {
		t.Fatalf("files = %v, want %v", multiCommitPaths(sum), wantPaths)
	}
	for i, path := range wantPaths {
		if sum.Files[i].Path != path {
			t.Fatalf("files = %v, want %v", multiCommitPaths(sum), wantPaths)
		}
	}
	a := sum.Files[0]
	if len(a.Sources) != 2 {
		t.Fatalf("a.txt sources = %d, want the two selected commits deduplicated", len(a.Sources))
	}
	if a.Sources[0].Commit.Hash != newer.Hash || a.Sources[1].Commit.Hash != older.Hash {
		t.Fatalf("a.txt source order = %s, %s", a.Sources[0].Commit.Hash, a.Sources[1].Commit.Hash)
	}
	if a.Sources[0].Parent != "parent-a" || a.Sources[1].Parent != "parent-b" {
		t.Fatalf("a.txt source parents = %q, %q", a.Sources[0].Parent, a.Sources[1].Parent)
	}
	if a.Kind != domain.ChangeModified {
		t.Fatalf("a.txt kind = %v, want modified", a.Kind)
	}
	if sum.Files[1].Kind != domain.ChangeDeleted {
		t.Fatalf("b.txt kind = %v, want deleted", sum.Files[1].Kind)
	}
	if sum.Files[2].Kind != domain.ChangeAdded {
		t.Fatalf("c.txt kind = %v, want added", sum.Files[2].Kind)
	}
	renamed := sum.Files[3]
	if renamed.Kind != domain.ChangeRenamed || renamed.RenamedFrom != "old/name.txt" {
		t.Fatalf("renamed row = %+v", renamed)
	}
	if renamed.Sources[0].Change.OldPath != "old/name.txt" {
		t.Fatalf("rename source lost its old path: %+v", renamed.Sources[0].Change)
	}
}

// TestAggregateMultiCommitsDeleteExplained checks a path added and later
// deleted: it stays one row whose sources explain both commits and whose
// summary reads as a deletion.
func TestAggregateMultiCommitsDeleteExplained(t *testing.T) {
	newest := testCommit("cccccccc3333", "delete it", "parent-c")
	oldest := testCommit("dddddddd4444", "add it", "parent-d")
	sum := aggregateMultiCommits([]gitcli.MultiCommitChanges{
		{Commit: newest, Changes: []domain.Change{{Path: "temp.txt", Kind: domain.ChangeDeleted}}},
		{Commit: oldest, Changes: []domain.Change{{Path: "temp.txt", Kind: domain.ChangeAdded}}},
	})
	if len(sum.Files) != 1 {
		t.Fatalf("files = %v, want one deduplicated row", multiCommitPaths(sum))
	}
	f := sum.Files[0]
	if f.Kind != domain.ChangeDeleted {
		t.Fatalf("kind = %v, want deleted from the newest change", f.Kind)
	}
	if len(f.Sources) != 2 || f.Sources[0].Change.Kind != domain.ChangeDeleted || f.Sources[1].Change.Kind != domain.ChangeAdded {
		t.Fatalf("sources = %+v, want deleted then added", f.Sources)
	}
}

// TestAggregateMultiCommitsKeepsSelectionOrder proves the summary keeps the
// order it is given and does not sort commits by date: the caller passes the
// log's selection order.
func TestAggregateMultiCommitsKeepsSelectionOrder(t *testing.T) {
	a := testCommit("1111aaaa5555", "first passed")
	b := testCommit("2222bbbb6666", "second passed")
	sum := aggregateMultiCommits([]gitcli.MultiCommitChanges{
		{Commit: a, Changes: []domain.Change{{Path: "z.txt", Kind: domain.ChangeAdded}}},
		{Commit: b, Changes: []domain.Change{{Path: "y.txt", Kind: domain.ChangeAdded}}},
	})
	if len(sum.Commits) != 2 || sum.Commits[0].Hash != a.Hash || sum.Commits[1].Hash != b.Hash {
		t.Fatalf("Commits = %+v, want passed order", sum.Commits)
	}
	zf, ok := multiCommitFile(sum, "z.txt")
	if !ok || len(zf.Sources) != 1 || zf.Sources[0].Commit.Hash != a.Hash {
		t.Fatalf("z.txt source order = %+v", zf.Sources)
	}
}

// TestAggregateMultiCommitsPartialFailure keeps the readable commits when one
// commit cannot be read.
func TestAggregateMultiCommitsPartialFailure(t *testing.T) {
	bad := testCommit("edededed7777", "unreadable")
	good := testCommit("fafafafa8888", "readable")
	sum := aggregateMultiCommits([]gitcli.MultiCommitChanges{
		{Commit: bad, Err: errors.New("boom")},
		{Commit: good, Changes: []domain.Change{{Path: "ok.txt", Kind: domain.ChangeAdded}}},
	})
	if sum.Err == nil {
		t.Fatal("Err = nil, want the failed commit reported")
	}
	if len(sum.Commits) != 2 {
		t.Fatalf("Commits = %d, want both selected commits listed", len(sum.Commits))
	}
	if len(sum.Files) != 1 || sum.Files[0].Path != "ok.txt" {
		t.Fatalf("files = %v, want the readable commit's file", multiCommitPaths(sum))
	}
}

func multiCommitFile(sum MultiCommitSummary, path string) (MultiCommitFile, bool) {
	for _, f := range sum.Files {
		if f.Path == path {
			return f, true
		}
	}
	return MultiCommitFile{}, false
}

func multiCommitPaths(sum MultiCommitSummary) []string {
	out := make([]string, len(sum.Files))
	for i, f := range sum.Files {
		out[i] = f.Path
	}
	return out
}

// TestMultiCommitResultDropRules checks that only the UI's own cancellation is
// silent. A deadline must still be delivered, or the panel would stay on its
// loading state after the query gave up.
func TestMultiCommitResultDropRules(t *testing.T) {
	if !multiCommitResultDropped(context.Canceled) {
		t.Error("cancellation must be dropped: a newer selection owns the view")
	}
	if !multiCommitResultDropped(fmt.Errorf("superceded: %w", context.Canceled)) {
		t.Error("wrapped cancellation must be dropped")
	}
	if multiCommitResultDropped(context.DeadlineExceeded) {
		t.Error("deadline must be delivered so the view stops loading")
	}
	if multiCommitResultDropped(nil) {
		t.Error("a successful result must be delivered")
	}
}

func TestMultiCommitTimeout(t *testing.T) {
	if d := multiCommitTimeout(0); d != time.Minute {
		t.Fatalf("small selection timeout = %v, want a minute", d)
	}
	if d := multiCommitTimeout(10); d != time.Minute+20*time.Second {
		t.Fatalf("timeout = %v", d)
	}
	if d := multiCommitTimeout(10000); d != 5*time.Minute {
		t.Fatalf("huge selection timeout = %v, want the cap", d)
	}
}

// TestWorkspaceMultiCommitChanges checks the asynchronous query end to end on a
// real repository: non-contiguous commits contribute their own first-parent
// changes and the result is delivered on the UI thread.
func TestWorkspaceMultiCommitChanges(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.write("keep.txt", "keep\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	f.write("a.txt", "two\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "modify a")
	f.write("b.txt", "b\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "add b")
	f.git("rm", "-q", "b.txt")
	f.git("commit", "-q", "-m", "delete b")
	f.open()

	commits := append([]domain.Commit(nil), f.w.Repo.Log.Commits...)
	if len(commits) < 4 {
		t.Fatalf("log has %d commits", len(commits))
	}
	// Deleting b and modifying a are not adjacent in the history.
	sel := []domain.Commit{commits[0], commits[2]}

	var got *MultiCommitSummary
	if cancel := f.w.MultiCommitChanges(sel, func(sum MultiCommitSummary) { got = &sum }); cancel == nil {
		t.Fatal("MultiCommitChanges returned no cancel function")
	}
	f.settle(func() bool { return got != nil })
	sum := *got
	if sum.Err != nil {
		t.Fatalf("Err = %v", sum.Err)
	}
	if len(sum.Commits) != 2 || sum.Commits[0].Hash != sel[0].Hash || sum.Commits[1].Hash != sel[1].Hash {
		t.Fatalf("Commits = %+v, want the selection order", sum.Commits)
	}
	paths := multiCommitPaths(sum)
	if len(paths) != 2 || paths[0] != "a.txt" || paths[1] != "b.txt" {
		t.Fatalf("files = %v, want a.txt and b.txt", paths)
	}
	if sum.Files[0].Kind != domain.ChangeModified || len(sum.Files[0].Sources) != 1 {
		t.Fatalf("a.txt = %+v", sum.Files[0])
	}
	if sum.Files[1].Kind != domain.ChangeDeleted || len(sum.Files[1].Sources) != 1 {
		t.Fatalf("b.txt = %+v", sum.Files[1])
	}
}

// TestWorkspaceMultiCommitChangesCancel checks that cancelling a selection stops
// its background work and does not deliver a stale summary.
func TestWorkspaceMultiCommitChangesCancel(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 60; i++ {
		f.git("commit", "-q", "--allow-empty", "-m", fmt.Sprintf("empty %02d", i))
	}
	f.open()
	commits := append([]domain.Commit(nil), f.w.Repo.Log.Commits...)
	called := false
	cancel := f.w.MultiCommitChanges(commits, func(MultiCommitSummary) { called = true })
	if cancel == nil {
		t.Fatal("MultiCommitChanges returned no cancel function")
	}
	cancel()
	// The cancelled query must finish without delivering. Give the goroutine
	// time to observe the cancellation and post anything it still had.
	for i := 0; i < 40; i++ {
		f.w.Drain()
		if called {
			t.Fatal("cancelled selection delivered a summary")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWorkspaceMultiCommitChangesNeedsRepository(t *testing.T) {
	f := newFixture(t)
	if cancel := f.w.MultiCommitChanges(nil, func(MultiCommitSummary) {}); cancel != nil {
		t.Fatal("cancel = non-nil without a repository")
	}
}
