package gitcli

import (
	"context"
	"strings"
	"testing"

	"viff/internal/domain"
)

// commitMeta reads one commit's metadata the way the log does, for tests that
// need parents and a subject rather than only a hash.
func commitMeta(t *testing.T, tr *testRepo, rev string) domain.Commit {
	t.Helper()
	hash := strings.TrimSpace(tr.git("rev-parse", rev))
	d, err := tr.r.CommitDetail(ctx, tr.repo, hash)
	if err != nil {
		t.Fatalf("CommitDetail(%s): %v", rev, err)
	}
	return d.Commit
}

func findChange(changes []domain.Change, path string) (domain.Change, bool) {
	for _, c := range changes {
		if c.Path == path {
			return c, true
		}
	}
	return domain.Change{}, false
}

func changePaths(changes []domain.Change) []string {
	out := make([]string, len(changes))
	for i, c := range changes {
		out[i] = c.Path
	}
	return out
}

// TestMultiCommitChangesFirstParentAndOrder checks that every commit is read
// against its own first parent and that the caller's order is kept: the result
// is a list of per-commit change sets, not a net diff between the selection's
// ends and not a date-sorted rewrite.
func TestMultiCommitChangesFirstParentAndOrder(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "a1\n")
	tr.write("base.txt", "base\n")
	tr.write("old/name.txt", strings.Repeat("rename me\n", 20))
	tr.commitAll("root")
	root := commitMeta(t, tr, "HEAD")

	tr.write("a.txt", "a2\n")
	tr.git("mv", "old/name.txt", "new-name.txt")
	tr.commitAll("rename and modify")
	rename := commitMeta(t, tr, "HEAD")

	tr.write("a.txt", "a3\n")
	tr.commitAll("modify again")
	modify := commitMeta(t, tr, "HEAD")

	tr.write("temp.txt", "temporary\n")
	tr.commitAll("add temp")
	add := commitMeta(t, tr, "HEAD")

	tr.git("rm", "-q", "temp.txt")
	tr.commitAll("delete temp")
	del := commitMeta(t, tr, "HEAD")

	tr.git("commit", "-q", "--allow-empty", "-m", "empty")
	empty := commitMeta(t, tr, "HEAD")

	// Deliberately not chronological: the result must follow this order.
	sel := []domain.Commit{modify, root, add, del}
	got := tr.r.MultiCommitChanges(ctx, tr.repo, sel)
	if len(got) != len(sel) {
		t.Fatalf("got %d entries, want %d", len(got), len(sel))
	}
	for i := range sel {
		if got[i].Commit.Hash != sel[i].Hash {
			t.Fatalf("entry %d is %s, want %s: order must follow the selection", i, got[i].Commit.Hash, sel[i].Hash)
		}
		if got[i].Err != nil {
			t.Fatalf("entry %d (%s): %v", i, domain.ShortHash(sel[i].Hash), got[i].Err)
		}
	}
	// The root commit is compared with the empty tree: everything is added.
	if paths := changePaths(got[1].Changes); len(paths) != 3 {
		t.Fatalf("root commit changes = %v, want 3 additions", paths)
	}
	for _, ch := range got[1].Changes {
		if ch.Kind != domain.ChangeAdded {
			t.Errorf("root change %s kind = %v, want added", ch.Path, ch.Kind)
		}
	}
	// A later commit contributes only its own first-parent change.
	if paths := changePaths(got[0].Changes); len(paths) != 1 || paths[0] != "a.txt" {
		t.Fatalf("modify commit changes = %v, want only a.txt", paths)
	}
	if ch, ok := findChange(got[0].Changes, "a.txt"); !ok || ch.Kind != domain.ChangeModified {
		t.Fatalf("a.txt in modify commit = %+v", ch)
	}
	if ch, ok := findChange(got[2].Changes, "temp.txt"); !ok || ch.Kind != domain.ChangeAdded {
		t.Fatalf("temp.txt in add commit = %+v", ch)
	}
	if ch, ok := findChange(got[3].Changes, "temp.txt"); !ok || ch.Kind != domain.ChangeDeleted {
		t.Fatalf("temp.txt in delete commit = %+v", ch)
	}

	// A rename keeps its old path, so the UI can explain it.
	renamed := tr.r.MultiCommitChanges(ctx, tr.repo, []domain.Commit{rename})
	if len(renamed) != 1 || renamed[0].Err != nil {
		t.Fatalf("rename query = %+v", renamed)
	}
	ch, ok := findChange(renamed[0].Changes, "new-name.txt")
	if !ok || ch.Kind != domain.ChangeRenamed || ch.OldPath != "old/name.txt" {
		t.Fatalf("rename change = %+v", ch)
	}

	// An empty commit contributes nothing and is not an error.
	em := tr.r.MultiCommitChanges(ctx, tr.repo, []domain.Commit{empty})
	if len(em) != 1 || em[0].Err != nil || len(em[0].Changes) != 0 {
		t.Fatalf("empty commit = %+v", em)
	}

	// A cancelled request stops before starting another Git process.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if out := tr.r.MultiCommitChanges(cancelled, tr.repo, sel); len(out) != 0 {
		t.Fatalf("cancelled query returned %d entries, want none", len(out))
	}
}

// TestMultiCommitChangesMergeUsesFirstParent checks a merge commit: it shows
// what it introduced relative to its first parent, not a combined diff.
func TestMultiCommitChangesMergeUsesFirstParent(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("main.txt", "m1\n")
	tr.commitAll("base")
	tr.git("checkout", "-q", "-b", "feature")
	tr.write("feature.txt", "f\n")
	tr.commitAll("feature work")
	tr.git("checkout", "-q", "main")
	tr.write("main.txt", "m2\n")
	tr.commitAll("main work")
	tr.git("merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	merge := commitMeta(t, tr, "HEAD")
	if len(merge.Parents) < 2 {
		t.Fatalf("merge parents = %v, want two", merge.Parents)
	}
	got := tr.r.MultiCommitChanges(context.Background(), tr.repo, []domain.Commit{merge})
	if len(got) != 1 || got[0].Err != nil {
		t.Fatalf("merge query = %+v", got)
	}
	if ch, ok := findChange(got[0].Changes, "feature.txt"); !ok || ch.Kind != domain.ChangeAdded {
		t.Fatalf("merge changes = %+v, want feature.txt added against the first parent", got[0].Changes)
	}
	if _, ok := findChange(got[0].Changes, "main.txt"); ok {
		t.Fatalf("merge changes include the first parent's own change: %+v", got[0].Changes)
	}
}
