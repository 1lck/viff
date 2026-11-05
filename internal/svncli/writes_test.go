package svncli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/vcs"
)

// TestAllWrites covers WorkspaceWriter and CommitWriter against a real
// repository. Each subtest leaves the working copy usable for the next one.
func TestAllWrites(t *testing.T) {
	f := newFullFixture(t)
	r, repo := f.r, f.repo
	ctx := f.ctx()

	t.Run("Stage_new_file", func(t *testing.T) {
		f.write("added.txt", "hello\n")
		st, err := r.Status(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		var untracked bool
		for _, e := range st.Entries {
			if e.Path == "added.txt" && e.Untracked() {
				untracked = true
			}
		}
		if !untracked {
			t.Fatalf("added.txt not untracked before Stage: %+v", st.Entries)
		}

		if err := r.Stage(ctx, repo, []string{"added.txt"}); err != nil {
			t.Fatalf("stage: %v", err)
		}
		st, err = r.Status(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		var scheduled bool
		for _, e := range st.Entries {
			if e.Path == "added.txt" && e.Worktree == 'A' {
				scheduled = true
			}
		}
		if !scheduled {
			t.Fatalf("added.txt not scheduled after Stage: %+v", st.Entries)
		}
	})

	t.Run("Unstage_reverts_the_schedule", func(t *testing.T) {
		// Unstage is "undo the svn add": the file goes back to unversioned.
		if err := r.Unstage(ctx, repo, []string{"added.txt"}, true); err != nil {
			t.Fatalf("unstage: %v", err)
		}
		st, err := r.Status(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		var untracked bool
		for _, e := range st.Entries {
			if e.Path == "added.txt" && e.Untracked() {
				untracked = true
			}
			if e.Path == "added.txt" && e.Worktree == 'A' {
				t.Fatalf("still scheduled after Unstage: %+v", e)
			}
		}
		if !untracked {
			t.Fatalf("added.txt should be unversioned again: %+v", st.Entries)
		}
		os.Remove(filepath.Join(f.wc, "added.txt"))
	})

	t.Run("Stage_deleted_file_schedules_removal", func(t *testing.T) {
		// A path gone from disk is scheduled as a delete, which is what "include
		// this in the next commit" means for a removed file.
		target := filepath.Join(f.wc, "README.md")
		orig, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
		if err := r.Stage(ctx, repo, []string{"README.md"}); err != nil {
			t.Fatalf("stage delete: %v", err)
		}
		st, err := r.Status(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		var scheduledDelete bool
		for _, e := range st.Entries {
			if e.Path == "README.md" && e.Worktree == 'D' {
				scheduledDelete = true
			}
		}
		if !scheduledDelete {
			t.Fatalf("removal not scheduled: %+v", st.Entries)
		}
		// Restore without committing the delete.
		if err := r.Discard(ctx, repo, []string{"README.md"}); err != nil {
			t.Fatalf("discard: %v", err)
		}
		if err := os.WriteFile(target, orig, 0o644); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Discard_restores_from_base", func(t *testing.T) {
		f.write("README.md", "locally changed\n")
		if err := r.Discard(ctx, repo, []string{"README.md"}); err != nil {
			t.Fatalf("discard: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(f.wc, "README.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "readme line\n" {
			t.Fatalf("content after Discard = %q", data)
		}
		st, err := r.Status(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range st.Entries {
			if e.Path == "README.md" {
				t.Fatalf("README.md still dirty after Discard: %+v", e)
			}
		}
	})

	t.Run("Revert_matches_Discard", func(t *testing.T) {
		f.write("README.md", "changed again\n")
		if err := r.Revert(ctx, repo, []string{"README.md"}); err != nil {
			t.Fatalf("revert: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(f.wc, "README.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "readme line\n" {
			t.Fatalf("content after Revert = %q", data)
		}
	})

	t.Run("LastCommitMessage", func(t *testing.T) {
		msg, err := r.LastCommitMessage(ctx, repo)
		if err != nil {
			t.Fatalf("last message: %v", err)
		}
		if msg != "greet" {
			t.Fatalf("LastCommitMessage = %q, want greet (the newest revision)", msg)
		}
	})

	t.Run("CreatePatch", func(t *testing.T) {
		f.write("README.md", "readme line\nan added line\n")
		defer func() {
			if err := r.Discard(ctx, repo, []string{"README.md"}); err != nil {
				t.Fatalf("cleanup: %v", err)
			}
		}()
		patch, err := r.CreatePatch(ctx, repo, []string{"README.md"}, false)
		if err != nil {
			t.Fatalf("patch: %v", err)
		}
		if len(patch) == 0 {
			t.Fatal("empty patch")
		}
		if !strings.Contains(string(patch), "an added line") {
			t.Fatalf("patch does not contain the change:\n%s", patch)
		}
	})

	t.Run("ApplyPatch", func(t *testing.T) {
		f.write("README.md", "readme line\npatched in\n")
		patch, err := r.CreatePatch(ctx, repo, []string{"README.md"}, false)
		if err != nil {
			t.Fatalf("make patch: %v", err)
		}
		// Revert, so applying the patch is a real change.
		if err := r.Discard(ctx, repo, []string{"README.md"}); err != nil {
			t.Fatalf("revert first: %v", err)
		}
		if err := r.ApplyPatch(ctx, repo, patch, false, false); err != nil {
			t.Fatalf("apply patch: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(f.wc, "README.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "patched in") {
			t.Fatalf("content after ApplyPatch = %q", data)
		}
		if err := r.Discard(ctx, repo, []string{"README.md"}); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	})

	t.Run("ApplyPatch_rejects_reverse_and_cached", func(t *testing.T) {
		// Both are Git concepts. Silently applying forwards would be wrong.
		small := []byte("--- a\n+++ b\n")
		if err := r.ApplyPatch(ctx, repo, small, false, true); err == nil {
			t.Fatal("reverse apply should be refused")
		}
		if err := r.ApplyPatch(ctx, repo, small, true, false); err == nil {
			t.Fatal("staged apply should be refused")
		}
	})

	t.Run("Commit_partial_target_set", func(t *testing.T) {
		// Two scheduled changes, only one committed: the other stays pending.
		f.write("one.txt", "one\n")
		f.write("two.txt", "two\n")
		if err := r.Stage(ctx, repo, []string{"one.txt", "two.txt"}); err != nil {
			t.Fatalf("stage: %v", err)
		}
		out, err := r.Commit(ctx, repo, vcs.CommitOptions{
			Message: "add only one",
			Paths:   []string{"one.txt"},
		})
		if err != nil {
			t.Fatalf("partial commit: %v: %s", err, out.Stdout)
		}
		st, err := r.Status(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		var twoPending, onePending bool
		for _, e := range st.Entries {
			switch e.Path {
			case "two.txt":
				twoPending = true
			case "one.txt":
				onePending = true
			}
		}
		if onePending {
			t.Fatalf("one.txt still pending after being committed: %+v", st.Entries)
		}
		if !twoPending {
			t.Fatalf("two.txt should still be pending: %+v", st.Entries)
		}
		// Commit the rest so later subtests start clean.
		if out, err := r.Commit(ctx, repo, vcs.CommitOptions{Message: "add two"}); err != nil {
			t.Fatalf("commit rest: %v: %s", err, out.Stdout)
		}
	})

	t.Run("Commit_multiline_message", func(t *testing.T) {
		f.write("msg.txt", "x\n")
		if err := r.Stage(ctx, repo, []string{"msg.txt"}); err != nil {
			t.Fatal(err)
		}
		want := "subject line\n\nbody paragraph\nwith \"quotes\" and a $dollar"
		if out, err := r.Commit(ctx, repo, vcs.CommitOptions{Message: want}); err != nil {
			t.Fatalf("commit: %v: %s", err, out.Stdout)
		}
		got, err := r.LastCommitMessage(ctx, repo)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if strings.TrimSpace(got) != want {
			t.Fatalf("message round trip:\n got %q\nwant %q", got, want)
		}
	})

	t.Run("Commit_nothing_reports_clean", func(t *testing.T) {
		// A commit with no scheduled change must not fabricate a revision; svn
		// either does nothing or fails, and Viff must not hide the outcome.
		before, err := r.LastCommitMessage(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = r.Commit(ctx, repo, vcs.CommitOptions{Message: "empty"})
		after, err := r.LastCommitMessage(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		if after != before {
			t.Fatalf("an empty commit created a revision: %q -> %q", before, after)
		}
	})
}

// TestAllSync covers Syncer. Fetch is a no-op by design; Pull is the update;
// Push is a commit; the branch-update and shallow helpers report what a
// Subversion working copy can and cannot do.
func TestAllSync(t *testing.T) {
	f := newFullFixture(t)
	r, repo := f.r, f.repo
	ctx := f.ctx()

	t.Run("Fetch_is_noop", func(t *testing.T) {
		// Subversion has no separate fetch: update contacts the repository and
		// merges in one step. Fetch must succeed without touching the copy.
		if err := r.Fetch(ctx, repo, "", true, nil); err != nil {
			t.Fatalf("fetch: %v", err)
		}
	})

	t.Run("Pull_is_update", func(t *testing.T) {
		// A second working copy commits; the first must see it after Pull.
		parent := filepath.Join(f.root, "sync")
		if err := os.MkdirAll(parent, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := r.Clone(ctx, parent, f.url, "wc2", nil); err != nil {
			t.Fatalf("clone: %v", err)
		}
		other := filepath.Join(parent, "wc2")
		otherRepo, err := r.Discover(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(other, "from_other.txt"), []byte("remote\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := r.Stage(ctx, otherRepo, []string{"from_other.txt"}); err != nil {
			t.Fatalf("stage other: %v", err)
		}
		if out, err := r.Commit(ctx, otherRepo, vcs.CommitOptions{Message: "from the other copy"}); err != nil {
			t.Fatalf("commit other: %v: %s", err, out.Stdout)
		}

		var progress []string
		if _, err := r.Pull(ctx, repo, vcs.PullMerge, func(s string) { progress = append(progress, s) }); err != nil {
			t.Fatalf("pull: %v", err)
		}
		if _, err := os.Stat(filepath.Join(f.wc, "from_other.txt")); err != nil {
			t.Fatalf("the other copy's file did not arrive: %v", err)
		}
		// The log must see the new revision without a stale cache.
		reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{})
		if err != nil {
			t.Fatal(err)
		}
		commits, _, _ := reader.Next(50)
		reader.Close()
		if commits[0].Subject != "from the other copy" {
			t.Fatalf("newest after Pull = %q", commits[0].Subject)
		}
	})

	t.Run("Push_is_commit", func(t *testing.T) {
		f.write("pushed.txt", "x\n")
		if err := r.Stage(ctx, repo, []string{"pushed.txt"}); err != nil {
			t.Fatal(err)
		}
		out, err := r.Push(ctx, repo, vcs.PushOptions{}, nil)
		if err != nil {
			t.Fatalf("push: %v: %s", err, out.Stdout)
		}
		if _, err := os.Stat(filepath.Join(f.wc, "pushed.txt")); err != nil {
			t.Fatalf("file missing after Push: %v", err)
		}
		st, err := r.Status(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range st.Entries {
			if e.Path == "pushed.txt" {
				t.Fatalf("still pending after Push: %+v", e)
			}
		}
	})

	t.Run("UpdateBranch_is_refused", func(t *testing.T) {
		// There is no cheap way to fast-forward a directory that is not checked
		// out, so the capability is absent rather than faked.
		if _, err := r.UpdateBranch(ctx, repo, "", "trunk", nil); err == nil {
			t.Fatal("UpdateBranch should be refused for Subversion")
		}
	})

	t.Run("IsShallow_false", func(t *testing.T) {
		shallow, err := r.IsShallow(ctx, repo)
		if err != nil {
			t.Fatalf("is shallow: %v", err)
		}
		if shallow {
			t.Fatal("a full checkout is not shallow")
		}
	})

	t.Run("Unshallow_deepens", func(t *testing.T) {
		// On a full working copy this is a plain update to infinity; it must
		// succeed rather than report an error.
		if err := r.Unshallow(ctx, repo, nil); err != nil {
			t.Fatalf("unshallow: %v", err)
		}
	})
}

// TestSparseCheckout covers the depth case Unshallow is for.
func TestSparseCheckout(t *testing.T) {
	f := newFullFixture(t)
	ctx := context.Background()

	// A depth-empty checkout has no files at all.
	parent := filepath.Join(f.root, "sparse")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := f.r.Run(ctx, parent, "checkout", "--depth", "empty", f.url, "shallow"); err != nil {
		t.Fatalf("depth-empty checkout: %v", err)
	}
	shallowWC := filepath.Join(parent, "shallow")
	repo, err := f.r.Discover(ctx, shallowWC)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shallowWC, "README.md")); err == nil {
		t.Fatal("a depth-empty checkout should have no files")
	}
	if err := f.r.Unshallow(ctx, repo, nil); err != nil {
		t.Fatalf("unshallow: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shallowWC, "README.md")); err != nil {
		t.Fatalf("Unshallow did not bring the files: %v", err)
	}
}
