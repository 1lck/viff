package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// existingWorktreeAppTemp returns a temporary directory with symlinks
// resolved, so worktree paths compare equal to the ones Git prints.
func existingWorktreeAppTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestWorkspaceAddWorktreeExisting checks the command as the UI drives it: a
// background write task, the refreshed worktree state, and a success notice,
// with the current checkout and the branch tip left unchanged.
func TestWorkspaceAddWorktreeExisting(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.commitAll("Base")
	f.git("branch", "feature")
	featureTip := strings.TrimSpace(f.git("rev-parse", "feature"))
	f.open()

	head := f.head()
	status := f.git("status", "--porcelain=v2")
	index := f.git("ls-files", "-s")

	wt := filepath.Join(existingWorktreeAppTemp(t), "wt-feature")
	var err error
	done := 0
	f.w.AddWorktreeExisting(wt, "feature", func(e error) {
		err, done = e, done+1
	})
	f.settle(func() bool {
		return done > 0 && f.idle() && f.w.Repo.WorktreesLoaded && len(f.w.Repo.Worktrees) == 2
	})
	if done != 1 || err != nil {
		t.Fatalf("add worktree: done=%d err=%v", done, err)
	}

	if body, e := os.ReadFile(filepath.Join(wt, "a.txt")); e != nil || string(body) != "base\n" {
		t.Fatalf("worktree file = %q, %v", body, e)
	}
	if got := strings.TrimSpace(f.git("rev-parse", "feature")); got != featureTip {
		t.Fatalf("feature tip moved: %s -> %s", featureTip, got)
	}
	if got := f.head(); got != head {
		t.Fatalf("HEAD moved: %s -> %s", head, got)
	}
	if got := f.git("status", "--porcelain=v2"); got != status {
		t.Fatalf("status changed:\n%s\nwant:\n%s", got, status)
	}
	if got := f.git("ls-files", "-s"); got != index {
		t.Fatalf("index changed:\n%s\nwant:\n%s", got, index)
	}

	// The refreshed state knows the new worktree and where the branch lives.
	other, ok := f.w.Repo.WorktreeElsewhere("feature")
	if !ok || !samePath(other.Path, wt) {
		t.Fatalf("worktree elsewhere = %+v / %v, want %s", other, ok, wt)
	}
	found := false
	for _, n := range f.w.Notifications {
		if n.Kind == NoticeSuccess && n.Title == "Worktree created" && n.Body == wt {
			found = true
		}
	}
	if !found {
		t.Fatalf("no success notification for %s: %+v", wt, f.w.Notifications)
	}
	if len(f.w.Tasks) != 0 {
		t.Fatalf("task still running: %+v", f.w.Tasks)
	}
}

// TestWorkspaceAddWorktreeExistingRefusals checks that failures surface as
// error notices, still call done, and leave the repository untouched.
func TestWorkspaceAddWorktreeExistingRefusals(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.commitAll("Base")
	f.git("branch", "taken")
	f.git("branch", "free")
	busy := filepath.Join(existingWorktreeAppTemp(t), "busy")
	f.git("worktree", "add", "-q", busy, "taken")
	f.write("a.txt", "dirty\n")
	f.write("notes.txt", "untracked\n")
	f.open()

	head := f.head()
	status := f.git("status", "--porcelain=v2")
	branches := f.git("for-each-ref", "--format=%(refname) %(objectname)", "refs/heads")
	worktrees := f.git("worktree", "list", "--porcelain")

	dir := existingWorktreeAppTemp(t)
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
	}{
		{"missing branch", filepath.Join(dir, "missing"), "nope"},
		{"busy branch", filepath.Join(dir, "busy2"), "taken"},
		{"occupied path", conflict, "free"},
		{"empty branch", filepath.Join(dir, "empty"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			firstNotice := len(f.w.Notifications)
			var err error
			done := 0
			f.w.AddWorktreeExisting(tc.path, tc.branch, func(e error) {
				err, done = e, done+1
			})
			f.settle(func() bool { return done > 0 && f.idle() })
			if done != 1 || err == nil {
				t.Fatalf("done=%d err=%v, want one error", done, err)
			}
			notified := false
			for _, n := range f.w.Notifications[firstNotice:] {
				if n.Kind == NoticeError && n.Title == "Cannot create worktree" {
					notified = true
				}
			}
			if !notified {
				t.Fatalf("no error notification: %+v", f.w.Notifications[firstNotice:])
			}
			if len(f.w.Tasks) != 0 {
				t.Fatalf("task still running: %+v", f.w.Tasks)
			}
			if got := f.head(); got != head {
				t.Fatalf("HEAD moved: %s -> %s", head, got)
			}
			if got := f.git("status", "--porcelain=v2"); got != status {
				t.Fatalf("status changed:\n%s\nwant:\n%s", got, status)
			}
			if got := f.git("for-each-ref", "--format=%(refname) %(objectname)", "refs/heads"); got != branches {
				t.Fatalf("branches changed:\n%s\nwant:\n%s", got, branches)
			}
			if got := f.git("worktree", "list", "--porcelain"); got != worktrees {
				t.Fatalf("worktrees changed:\n%s\nwant:\n%s", got, worktrees)
			}
			if tc.name == "occupied path" {
				if body, e := os.ReadFile(keep); e != nil || string(body) != "keep\n" {
					t.Fatalf("occupied path was modified: %q, %v", body, e)
				}
				return
			}
			if _, e := os.Stat(tc.path); !os.IsNotExist(e) {
				t.Fatalf("refused request created %s: %v", tc.path, e)
			}
		})
	}
}
