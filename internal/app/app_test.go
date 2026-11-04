package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/storage"
)

type fixture struct {
	t   *testing.T
	dir string
	env []string
	w   *Workspace
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig")}
	f := &fixture{t: t, dir: dir, env: env}
	f.git("init", "-q", "-b", "main")
	f.git("config", "user.name", "Viff Test")
	f.git("config", "user.email", "test@example.com")
	f.git("config", "commit.gpgsign", "false")
	f.w = NewWorkspace(&gitcli.Runner{Env: env}, storage.Defaults(), nil)
	return f
}

func (f *fixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (f *fixture) write(name, content string) {
	f.t.Helper()
	p := filepath.Join(f.dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// settle drains background results until cond holds.
func (f *fixture) settle(cond func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		f.w.Drain()
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.t.Fatal("timed out waiting for background work")
}

func (f *fixture) idle() bool {
	rs := f.w.Repo
	return rs != nil && !rs.refreshing && !rs.Log.Loading && len(f.w.Tasks) == 0 && !f.w.Pending()
}

func (f *fixture) open() {
	f.w.Open(f.dir, func(err error) {
		if err != nil {
			f.t.Fatal(err)
		}
	})
	f.settle(func() bool { return f.w.Repo != nil && f.w.Repo.StatusLoaded && f.idle() })
}

func TestOpenRefreshAndLog(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 5; i++ {
		f.write("a.txt", strings.Repeat("x\n", i+1))
		f.git("add", "-A")
		f.git("commit", "-q", "-m", "commit "+string(rune('A'+i)))
	}
	f.git("tag", "v1")
	f.open()
	rs := f.w.Repo
	if rs.Repo.Root != f.dir || rs.CurrentBranch() != "main" || len(rs.Log.Commits) != 5 {
		t.Fatalf("state = %+v commits=%d", rs.Repo, len(rs.Log.Commits))
	}
	head := rs.Log.Commits[0]
	refs := rs.RefsByHash[head.Hash]
	if len(refs) < 3 || refs[0].Kind != domain.RefHead {
		t.Fatalf("refs at head = %+v", refs)
	}
	// An external modification shows up after a refresh.
	f.write("b.txt", "new\n")
	f.w.Refresh(false)
	f.settle(func() bool { return len(f.w.Repo.Entries(domain.AreaUntracked)) == 1 && f.idle() })

	// A new commit made outside reloads the log through the refs change.
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "outside")
	f.w.Refresh(true)
	f.settle(func() bool { return f.idle() && len(f.w.Repo.Log.Commits) == 6 })
	if f.w.Repo.Log.Commits[0].Subject != "outside" {
		t.Fatal("log not reloaded")
	}
	// Filter by text.
	f.w.SetLogFilter(LogFilter{Text: "commit C"})
	f.settle(func() bool { return f.idle() && f.w.Repo.Log.Done })
	if n := len(f.w.Repo.Log.Commits); n != 1 {
		t.Fatalf("filtered = %d", n)
	}
	// Hash filter finds the commit itself.
	f.w.SetLogFilter(LogFilter{Text: head.Hash[:7]})
	f.settle(func() bool { return f.idle() && f.w.Repo.Log.Done })
	if n := len(f.w.Repo.Log.Commits); n != 1 || f.w.Repo.Log.Commits[0].Hash != head.Hash {
		t.Fatalf("hash filter = %d", n)
	}
	f.w.SelectCommit(head.Hash)
	f.settle(func() bool { return f.w.Repo.Detail != nil })
	if f.w.Repo.Detail.Subject != "commit E" || len(f.w.Repo.DetailBranches) == 0 {
		t.Fatalf("detail = %+v", f.w.Repo.Detail)
	}
}

func TestStageBlockAndStaleProtection(t *testing.T) {
	f := newFixture(t)
	f.write("f.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	f.write("f.txt", "ONE\n2\n3\n4\n5\n6\n7\n8\n9\nTEN\n")
	f.open()

	var diff *DiffResult
	load := func(area domain.Area) {
		diff = nil
		f.w.LoadDiff(DiffSource{Path: "f.txt", Working: true, Area: area}, domain.WhitespaceExact, func(d *DiffResult, err error) {
			if err != nil {
				t.Fatal(err)
			}
			diff = d
		})
		f.settle(func() bool { return diff != nil })
	}
	load(domain.AreaUnstaged)
	if len(diff.Blocks) != 2 || diff.Old.Title != "Staged" {
		t.Fatalf("blocks = %v", diff.Blocks)
	}
	// Stage the second block only.
	var applied error = os.ErrInvalid
	f.w.ApplyBlock(diff, 1, BlockStage, nil, nil, func(err error) { applied = err })
	f.settle(func() bool { return applied != os.ErrInvalid && f.idle() })
	if applied != nil {
		t.Fatal(applied)
	}
	if staged := f.w.Repo.Entries(domain.AreaStaged); len(staged) != 1 {
		t.Fatalf("staged = %+v", staged)
	}
	load(domain.AreaStaged)
	if len(diff.Blocks) != 1 || diff.New.Lines[9] != "TEN" || diff.New.Lines[0] != "1" {
		t.Fatalf("staged diff = %+v", diff.Blocks)
	}
	// Unstage it again.
	applied = os.ErrInvalid
	f.w.ApplyBlock(diff, 0, BlockUnstage, nil, nil, func(err error) { applied = err })
	f.settle(func() bool { return applied != os.ErrInvalid && f.idle() })
	if applied != nil || len(f.w.Repo.Entries(domain.AreaStaged)) != 0 {
		t.Fatalf("unstage: %v %+v", applied, f.w.Repo.Entries(domain.AreaStaged))
	}

	// An external tool edits the file after the diff was shown: the stale
	// block is refused and the index is untouched.
	load(domain.AreaUnstaged)
	f.write("f.txt", "ONE\n2\n3\n4\n5\n6\n7\n8\n9\nTEN\nELEVEN by AI\n")
	applied = os.ErrInvalid
	f.w.ApplyBlock(diff, 0, BlockStage, nil, nil, func(err error) { applied = err })
	f.settle(func() bool { return applied != os.ErrInvalid && f.idle() })
	if applied != ErrStale {
		t.Fatalf("expected stale, got %v", applied)
	}
	if staged := f.w.Repo.Entries(domain.AreaStaged); len(staged) != 0 {
		t.Fatalf("stale patch was applied: %+v", staged)
	}
	// Revert the first block in the working tree.
	load(domain.AreaUnstaged)
	applied = os.ErrInvalid
	f.w.ApplyBlock(diff, 0, BlockRevert, nil, nil, func(err error) { applied = err })
	f.settle(func() bool { return applied != os.ErrInvalid && f.idle() })
	data, _ := os.ReadFile(filepath.Join(f.dir, "f.txt"))
	if applied != nil || !strings.HasPrefix(string(data), "1\n2\n") || !strings.Contains(string(data), "ELEVEN") {
		t.Fatalf("revert: %v %q", applied, data)
	}
}

func TestCommitAndConflictFollowsExternalResolution(t *testing.T) {
	f := newFixture(t)
	f.write("c.txt", "base\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	f.git("checkout", "-q", "-b", "other")
	f.write("c.txt", "theirs\n")
	f.git("commit", "-q", "-am", "theirs")
	f.git("checkout", "-q", "main")
	f.open()

	f.write("c.txt", "ours\n")
	f.w.StagePaths([]string{"c.txt"})
	f.settle(func() bool { return f.idle() && len(f.w.Repo.Entries(domain.AreaStaged)) == 1 })
	committed := false
	f.w.Commit(CommitRequest{Message: "ours"}, func() { committed = true })
	f.settle(func() bool { return committed && f.idle() })
	if f.w.Repo.Log.Commits[0].Subject != "ours" {
		t.Fatal("commit not in log")
	}

	f.w.Merge("other", false)
	f.settle(func() bool { return f.idle() && f.w.Repo.Operation.Kind == domain.OpMerge })
	if len(f.w.Repo.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v", f.w.Repo.Conflicts)
	}
	var diff *DiffResult
	f.w.LoadDiff(DiffSource{Path: "c.txt", Working: true, Area: domain.AreaConflicts}, 0, func(d *DiffResult, err error) {
		if err != nil {
			t.Fatal(err)
		}
		diff = d
	})
	f.settle(func() bool { return diff != nil })
	cv := diff.Conflict
	if cv == nil || len(cv.Markers.Blocks) != 1 || !strings.Contains(cv.OursLabel, "main") || !strings.Contains(cv.TheirsLabel, "theirs") {
		t.Fatalf("conflict view = %+v", cv)
	}
	// The external tool edits the file: markers gone but not staged.
	f.write("c.txt", "merged by AI\n")
	diff = nil
	f.w.LoadDiff(DiffSource{Path: "c.txt", Working: true, Area: domain.AreaConflicts}, 0, func(d *DiffResult, err error) { diff = d })
	f.settle(func() bool { return diff != nil })
	if !diff.Conflict.Resolved {
		t.Fatal("expected resolved-but-unstaged")
	}
	// The tool stages and commits the merge itself: Viff follows.
	f.git("add", "c.txt")
	f.git("commit", "-q", "--no-edit")
	f.w.Refresh(true)
	f.settle(func() bool { return f.idle() && !f.w.Repo.Operation.Active() })
	if len(f.w.Repo.Conflicts) != 0 || len(f.w.Repo.Log.Rows[0].Edges) == 0 || !f.w.Repo.Log.Rows[0].IsMerge {
		t.Fatal("did not follow external merge commit")
	}
}

func TestDeleteUnmergedBranchOffersForce(t *testing.T) {
	f := newFixture(t)
	f.write("a", "a\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "a")
	f.git("checkout", "-q", "-b", "side")
	f.write("a", "b\n")
	f.git("commit", "-q", "-am", "b")
	f.git("checkout", "-q", "main")
	f.open()
	var side domain.Ref
	for _, r := range f.w.Repo.Refs {
		if r.Short == "side" {
			side = r
		}
	}
	f.w.DeleteBranch(side, false)
	f.settle(func() bool { return f.idle() && len(f.w.Notifications) > 0 })
	n := f.w.Notifications[len(f.w.Notifications)-1]
	if n.Kind != NoticeWarning || len(n.Actions) != 1 {
		t.Fatalf("notice = %+v", n)
	}
	n.Actions[0].Run()
	f.settle(func() bool { return f.idle() && len(f.w.Repo.RefsOf(domain.RefLocal)) == 1 })
	// The success notice offers to restore it.
	last := f.w.Notifications[len(f.w.Notifications)-1]
	if len(last.Actions) != 1 || last.Actions[0].Title != "Restore" {
		t.Fatalf("restore = %+v", last)
	}
	last.Actions[0].Run()
	f.settle(func() bool { return f.idle() && len(f.w.Repo.RefsOf(domain.RefLocal)) == 2 })
}

func TestSettingsRoundTripAndNewerVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := storage.Load(path)
	s.AddRecent("/tmp/x")
	s.Theme = "light"
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2 := storage.Load(path)
	if s2.Theme != "light" || len(s2.Recent) != 1 {
		t.Fatalf("reloaded = %+v", s2)
	}
	os.WriteFile(path, []byte(`{"version": 99, "theme": "dark"}`), 0o600)
	s3 := storage.Load(path)
	s3.Theme = "light"
	s3.Save()
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "99") || s3.Warning == "" {
		t.Fatal("newer settings were overwritten")
	}
	os.WriteFile(path, []byte(`{broken`), 0o600)
	s4 := storage.Load(path)
	if s4.Warning == "" || s4.Theme != "dark" {
		t.Fatal("corrupt settings not reset")
	}
}
