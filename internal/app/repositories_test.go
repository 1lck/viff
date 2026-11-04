package app

import (
	"bytes"
	"encoding/json"
	"errors"
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

// errNotRun marks a callback that has not run yet.
var errNotRun = errors.New("callback has not run yet")

// repositoriesFixture is a multi-repository setup: a temporary settings path
// (so the workspace file is real) and helpers to create real Git repositories
// in an isolated environment.
type repositoriesFixture struct {
	t      *testing.T
	env    []string
	config string
	w      *Workspace
}

func newRepositoriesFixture(t *testing.T) *repositoriesFixture {
	t.Helper()
	home := t.TempDir()
	config := filepath.Join(home, "config")
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig"),
	}
	f := &repositoriesFixture{t: t, env: env, config: config}
	f.w = f.newWorkspace()
	return f
}

func (f *repositoriesFixture) settingsPath() string { return filepath.Join(f.config, "settings.json") }

func (f *repositoriesFixture) workspacePath() string { return storage.WorkspacePath(f.settingsPath()) }

// newWorkspace creates another workspace over the same settings path, as a
// restart would.
func (f *repositoriesFixture) newWorkspace() *Workspace {
	return NewWorkspace(&gitcli.Runner{Env: f.env}, storage.Load(f.settingsPath()), nil)
}

func (f *repositoriesFixture) git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

func (f *repositoriesFixture) write(dir, name, content string) {
	f.t.Helper()
	p := filepath.Join(dir, name)
	if dir := filepath.Dir(p); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// newRepo initializes a real Git repository with one commit.
func (f *repositoriesFixture) newRepo(branch string) string {
	f.t.Helper()
	dir, err := filepath.EvalSymlinks(f.t.TempDir())
	if err != nil {
		f.t.Fatal(err)
	}
	f.git(dir, "init", "-q", "-b", branch)
	f.git(dir, "config", "user.name", "Viff Test")
	f.git(dir, "config", "user.email", "test@example.com")
	f.git(dir, "config", "commit.gpgsign", "false")
	f.write(dir, "base.txt", "base\n")
	f.git(dir, "add", "-A")
	f.git(dir, "commit", "-q", "-m", "base")
	return dir
}

// settle drains background results on the fixture workspace until cond holds.
func (f *repositoriesFixture) settle(cond func() bool) {
	f.t.Helper()
	drainUntil(f.t, f.w, cond)
}

func drainUntil(t *testing.T, w *Workspace, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		w.Drain()
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for background work")
}

// idle reports that the workspace has no running query, task or queued result.
func idle(w *Workspace) bool {
	if w.Pending() || len(w.Tasks) != 0 {
		return false
	}
	rs := w.Repo
	return rs == nil || (!rs.refreshing && !rs.Log.Loading)
}

func (f *repositoriesFixture) open(dir string) {
	f.t.Helper()
	err := errNotRun
	f.w.Open(dir, func(e error) { err = e })
	f.settle(func() bool { return err != errNotRun })
	if err != nil {
		f.t.Fatalf("open %s: %v", dir, err)
	}
	f.settle(func() bool { return idle(f.w) })
}

func (f *repositoriesFixture) add(dir string) error {
	f.t.Helper()
	err := errNotRun
	f.w.AddRepository(dir, func(e error) { err = e })
	f.settle(func() bool { return err != errNotRun })
	return err
}

func (f *repositoriesFixture) activate(root string) error {
	f.t.Helper()
	err := errNotRun
	f.w.ActivateRepository(root, func(e error) { err = e })
	f.settle(func() bool { return err != errNotRun })
	if err == nil {
		f.settle(func() bool { return idle(f.w) })
	}
	return err
}

func (f *repositoriesFixture) refreshAll() {
	f.t.Helper()
	done := false
	f.w.RefreshRepositories(func() { done = true })
	f.settle(func() bool { return done })
}

func (f *repositoriesFixture) summary(root string) (RepositorySummary, bool) {
	return f.summaryFor(f.w, root)
}

func (f *repositoriesFixture) summaryFor(w *Workspace, root string) (RepositorySummary, bool) {
	want := canonicalPath(root)
	for _, s := range w.Repositories {
		if s.Root == want {
			return s, true
		}
	}
	return RepositorySummary{}, false
}

func (f *repositoriesFixture) mustSummary(root string) RepositorySummary {
	f.t.Helper()
	s, ok := f.summary(root)
	if !ok {
		f.t.Fatalf("repository %s is not listed: %+v", root, f.w.Repositories)
	}
	return s
}

func TestRepositoriesAddDeduplicatesAndDoesNotStealActive(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")

	// A fresh workspace has an empty list and an immediate refresh callback.
	called := false
	f.w.RefreshRepositories(func() { called = true })
	if !called {
		t.Fatal("RefreshRepositories with an empty list did not call done")
	}
	if len(f.w.Repositories) != 0 {
		t.Fatalf("fresh list = %+v", f.w.Repositories)
	}

	// Ordinary Open stays compatible and puts the current repository on the
	// list, loaded and active.
	f.open(repoA)
	if len(f.w.Repositories) != 1 {
		t.Fatalf("list after Open = %+v", f.w.Repositories)
	}
	a := f.mustSummary(repoA)
	if a.Root != canonicalPath(repoA) || !a.Loaded || a.Err != nil || a.Status.Branch.Branch != "main" {
		t.Fatalf("summary after Open = %+v", a)
	}
	if f.w.ActiveRoot != canonicalPath(repoA) || f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) {
		t.Fatalf("active = %q repo=%v", f.w.ActiveRoot, f.w.Repo)
	}

	// Adding the same repository, a subdirectory of it, and a spelled
	// differently path deduplicates to one row. A subdirectory is part of the
	// same working tree, so it must not add a second row.
	sub := filepath.Join(repoA, "sub", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{repoA, sub, canonicalPath(repoA) + string(os.PathSeparator)} {
		if err := f.add(dir); err != nil {
			t.Fatalf("add %q: %v", dir, err)
		}
	}
	// A symlink to the same repository also deduplicates: the root is resolved
	// to the real directory before it is stored.
	link := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(canonicalPath(repoA), link); err == nil {
		if err := f.add(link); err != nil {
			t.Fatalf("add symlink: %v", err)
		}
	}
	if len(f.w.Repositories) != 1 || f.w.Repositories[0].Root != canonicalPath(repoA) {
		t.Fatalf("deduplicated list = %+v", f.w.Repositories)
	}

	// Adding another repository only adds a row: the active repository and the
	// open state do not change.
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}
	if len(f.w.Repositories) != 2 {
		t.Fatalf("list = %+v", f.w.Repositories)
	}
	if f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) || f.w.ActiveRoot != canonicalPath(repoA) {
		t.Fatal("AddRepository stole the active repository")
	}

	// A directory that is not a repository is refused and changes nothing.
	notRepo := t.TempDir()
	if err := f.add(notRepo); err == nil {
		t.Fatal("adding a non-repository succeeded")
	}
	if len(f.w.Repositories) != 2 {
		t.Fatalf("failed add changed the list: %+v", f.w.Repositories)
	}

	// Refresh loads status for every row, including the active one.
	f.refreshAll()
	for _, root := range []string{repoA, repoB} {
		s := f.mustSummary(root)
		if !s.Loaded || s.Err != nil || s.Status.Branch.Branch != "main" {
			t.Fatalf("refreshed %s = %+v", root, s)
		}
	}
}

func TestRepositoriesStatusIsolationAndExplicitRefresh(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.write(repoA, "base.txt", "modified\n") // unstaged change in A only
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}
	f.refreshAll()

	a := f.mustSummary(repoA)
	b := f.mustSummary(repoB)
	if n := len(a.Status.Entries); n != 1 {
		t.Fatalf("A entries = %d (%+v)", n, a.Status.Entries)
	}
	if n := len(b.Status.Entries); n != 0 || b.Err != nil {
		t.Fatalf("B status leaked from A: %+v", b)
	}

	// A change in the non-active repository does not appear until the workspace
	// is refreshed: only the active repository has a file watcher this round.
	f.write(repoB, "later.txt", "later\n")
	f.settle(func() bool { return idle(f.w) })
	if n := len(f.mustSummary(repoB).Status.Entries); n != 0 {
		t.Fatalf("non-active repository refreshed without a watcher: %d entries", n)
	}
	f.refreshAll()
	if n := len(f.mustSummary(repoB).Status.Entries); n != 1 {
		t.Fatalf("explicit refresh missed the non-active change: %d entries", n)
	}
	// The active row is kept in step by its own refresh path as well.
	if n := len(f.mustSummary(repoA).Status.Entries); n != 1 {
		t.Fatalf("active row changed by refreshing B: %d entries", n)
	}
}

func TestRepositoriesFailureKeepsKnownStatus(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}
	f.refreshAll()
	before := f.mustSummary(repoB)
	if !before.Loaded || before.Err != nil {
		t.Fatalf("B not loaded before failure: %+v", before)
	}

	// The repository directory disappears: the query fails, but the last known
	// status stays and the error is visible instead of blanking the row.
	if err := os.RemoveAll(repoB); err != nil {
		t.Fatal(err)
	}
	f.refreshAll()
	after := f.mustSummary(repoB)
	if after.Err == nil {
		t.Fatal("failed status kept no error")
	}
	if !after.Loaded || len(after.Status.Entries) != len(before.Status.Entries) || after.Status.Branch.Branch != before.Status.Branch.Branch {
		t.Fatalf("failure lost the known status: before=%+v after=%+v", before, after)
	}
	// The other repository is unaffected.
	if a := f.mustSummary(repoA); a.Err != nil || !a.Loaded {
		t.Fatalf("A affected by B's failure: %+v", a)
	}
}

func TestRepositoriesSwitchKeepsContentAndGenerationsGrow(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.write(repoA, "base.txt", "work in progress\n")
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}
	if f.w.Repo.rootKey != canonicalPath(repoA) {
		t.Fatal("A is not active")
	}
	genA := f.w.Repo.Gen

	if err := f.activate(repoB); err != nil {
		t.Fatal(err)
	}
	if f.w.Repo.rootKey != canonicalPath(repoB) || f.w.ActiveRoot != canonicalPath(repoB) {
		t.Fatalf("switch did not activate B: repo=%v active=%q", f.w.Repo, f.w.ActiveRoot)
	}
	if genB := f.w.Repo.Gen; genB <= genA {
		t.Fatalf("generation did not grow on switch: %d then %d", genA, genB)
	}
	// The uncommitted content of A is still on disk, untouched by the switch.
	data, err := os.ReadFile(filepath.Join(repoA, "base.txt"))
	if err != nil || string(data) != "work in progress\n" {
		t.Fatalf("switch touched A's working tree: %q %v", data, err)
	}

	// Switching back shows A's uncommitted change again.
	if err := f.activate(repoA); err != nil {
		t.Fatal(err)
	}
	if got := f.w.Repo.Status.Entries; len(got) != 1 {
		t.Fatalf("A's uncommitted change lost after switching: %+v", got)
	}
	genBack := f.w.Repo.Gen
	if genBack <= genA {
		t.Fatalf("generation reused after switching back: %d", genBack)
	}

	// Closing the project releases the repository but not the list, and
	// reopening never reuses a generation.
	f.w.Close()
	if f.w.Repo != nil {
		t.Fatal("Close kept the repository")
	}
	if len(f.w.Repositories) != 2 {
		t.Fatalf("Close cleared the list: %+v", f.w.Repositories)
	}
	if err := f.activate(repoA); err != nil {
		t.Fatal(err)
	}
	if f.w.Repo.Gen <= genBack {
		t.Fatalf("generation reset after Close: %d then %d", genBack, f.w.Repo.Gen)
	}
	// Re-activating the already active repository is a no-op.
	genActive := f.w.Repo.Gen
	if err := f.activate(repoA); err != nil || f.w.Repo.Gen != genActive {
		t.Fatalf("re-activate active repo: %v gen %d -> %d", err, genActive, f.w.Repo.Gen)
	}
}

func TestRepositoriesRemoveKeepsDiskAndFallsBackToWelcome(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.write(repoA, "base.txt", "uncommitted\n")
	repoC := f.newRepo("main")
	f.open(repoA)
	for _, dir := range []string{repoB, repoC} {
		if err := f.add(dir); err != nil {
			t.Fatal(err)
		}
	}

	// Removing a non-active row leaves the active repository open and the disk
	// alone.
	if err := f.w.RemoveRepository(repoB); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoB, ".git")); err != nil {
		t.Fatalf("remove touched the disk: %v", err)
	}
	if len(f.w.Repositories) != 2 || f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) {
		t.Fatalf("remove non-active: list=%+v repo=%v", f.w.Repositories, f.w.Repo)
	}

	// Removing the active row closes it, keeps the remaining row listed and
	// returns to the welcome screen; the working tree, including uncommitted
	// changes, is untouched.
	if err := f.w.RemoveRepository(repoA); err != nil {
		t.Fatal(err)
	}
	if f.w.Repo != nil || f.w.ActiveRoot != "" {
		t.Fatalf("removed active repo is still active: repo=%v active=%q", f.w.Repo, f.w.ActiveRoot)
	}
	if len(f.w.Repositories) != 1 || f.w.Repositories[0].Root != canonicalPath(repoC) {
		t.Fatalf("remaining list = %+v", f.w.Repositories)
	}
	data, err := os.ReadFile(filepath.Join(repoA, "base.txt"))
	if err != nil || string(data) != "uncommitted\n" {
		t.Fatalf("remove touched A's working tree: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(repoA, ".git")); err != nil {
		t.Fatalf("remove deleted git data: %v", err)
	}

	// Unknown roots are refused with an error that names the problem.
	err = f.w.RemoveRepository(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, ErrRepositoryUnknown) {
		t.Fatalf("unknown remove error = %v", err)
	}
}

func TestRepositoriesPersistListAndActiveAcrossRestart(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}
	if err := f.activate(repoB); err != nil {
		t.Fatal(err)
	}
	// Closing (as quitting the app does) must not clear the persisted list or
	// the active row.
	f.w.Close()

	// The settings file keeps its old format: the workspace is a separate file.
	settingsData, err := os.ReadFile(f.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(settingsData, []byte("repositories")) || bytes.Contains(settingsData, []byte("workspace")) {
		t.Fatalf("workspace data leaked into settings: %s", settingsData)
	}
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(settingsData, &probe); err != nil || probe.Version != storage.SchemaVersion {
		t.Fatalf("settings format changed: %v %s", err, settingsData)
	}
	if _, err := os.Stat(f.workspacePath()); err != nil {
		t.Fatalf("workspace file missing: %v", err)
	}

	// A new workspace over the same settings loads the list and the active
	// row, but queries nothing by itself (no disk scan on startup).
	w2 := f.newWorkspace()
	if len(w2.Repositories) != 2 {
		t.Fatalf("reloaded list = %+v", w2.Repositories)
	}
	if w2.Repositories[0].Root != canonicalPath(repoA) || w2.Repositories[1].Root != canonicalPath(repoB) {
		t.Fatalf("reloaded order = %+v", w2.Repositories)
	}
	if w2.ActiveRoot != canonicalPath(repoB) {
		t.Fatalf("reloaded active = %q", w2.ActiveRoot)
	}
	for _, s := range w2.Repositories {
		if s.Loaded {
			t.Fatalf("startup queried status: %+v", s)
		}
	}

	// The explicit refresh fills the overview in the new workspace.
	done := false
	w2.RefreshRepositories(func() { done = true })
	drainUntil(t, w2, func() bool { return done })
	for _, s := range w2.Repositories {
		if !s.Loaded || s.Err != nil || s.Status.Branch.Branch != "main" {
			t.Fatalf("reloaded refresh = %+v", s)
		}
	}
}

func TestRepositoriesBusyWriteRefusesSwitchAndActiveRemove(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}

	// Stand in for a running Git write task.
	f.w.Tasks = append(f.w.Tasks, &Task{ID: 999, Title: "Commit", Write: true, cancel: func() {}})
	if !f.w.Busy() {
		t.Fatal("fake write task is not busy")
	}
	if err := f.activate(repoB); !errors.Is(err, ErrRepositoryBusy) {
		t.Fatalf("switch during write = %v", err)
	}
	if f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) {
		t.Fatal("busy switch changed the active repository")
	}
	if err := f.w.RemoveRepository(repoA); !errors.Is(err, ErrRepositoryBusy) {
		t.Fatalf("remove active during write = %v", err)
	}
	if f.w.Repo == nil || len(f.w.Repositories) != 2 {
		t.Fatalf("busy remove changed state: repo=%v list=%+v", f.w.Repo, f.w.Repositories)
	}
	var openErr error = errNotRun
	f.w.Open(repoB, func(e error) { openErr = e })
	if !errors.Is(openErr, ErrRepositoryBusy) {
		t.Fatalf("Open during write = %v", openErr)
	}

	// Removing a non-active row is safe while a write runs: no command uses it.
	if err := f.w.RemoveRepository(repoB); err != nil {
		t.Fatalf("remove non-active during write = %v", err)
	}
	f.w.Tasks = nil
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}
	if err := f.activate(repoB); err != nil {
		t.Fatalf("switch after write finished = %v", err)
	}
	if f.w.Repo.rootKey != canonicalPath(repoB) {
		t.Fatal("switch after write did not activate B")
	}
}

func TestRepositoriesStaleResultsDoNotResurrectRows(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}
	f.refreshAll()

	// A status query that was started before the row was removed must be
	// dropped instead of bringing the row back.
	entry := f.w.repos.byRoot[canonicalPath(repoB)]
	if entry == nil {
		t.Fatal("B has no entry")
	}
	seq := entry.seq
	if err := f.w.RemoveRepository(repoB); err != nil {
		t.Fatal(err)
	}
	f.w.applyRepositoryStatus(entry, seq, domain.Status{Branch: domain.BranchStatus{Branch: "main"}}, nil)
	if len(f.w.Repositories) != 1 || f.w.Repositories[0].Root != canonicalPath(repoA) {
		t.Fatalf("stale result resurrected a removed row: %+v", f.w.Repositories)
	}

	// The same holds for a re-added row: the old entry's result must not
	// overwrite the new row.
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}
	fresh := f.w.repos.byRoot[canonicalPath(repoB)]
	if fresh == entry {
		t.Fatal("re-added row reused the removed entry")
	}
	f.w.applyRepositoryStatus(entry, seq, domain.Status{Branch: domain.BranchStatus{Branch: "old"}}, nil)
	if got := f.mustSummary(repoB); got.Loaded {
		t.Fatalf("stale entry filled the new row: %+v", got)
	}
	// And an older query for the current entry is dropped by its sequence.
	fresh.seq++
	stale := fresh.seq - 1
	f.w.applyRepositoryStatus(fresh, stale, domain.Status{Branch: domain.BranchStatus{Branch: "stale"}}, nil)
	if got := f.mustSummary(repoB); got.Loaded {
		t.Fatalf("outdated sequence filled the row: %+v", got)
	}

	// An activation that finishes after the row was removed must not open it.
	if err := f.activate(repoA); err != nil {
		t.Fatal(err)
	}
	var activateErr error = errNotRun
	f.w.ActivateRepository(repoB, func(e error) { activateErr = e })
	if err := f.w.RemoveRepository(repoB); err != nil {
		t.Fatal(err)
	}
	f.settle(func() bool { return activateErr != errNotRun })
	if !errors.Is(activateErr, ErrRepositoryStale) {
		t.Fatalf("stale activation error = %v", activateErr)
	}
	if f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) {
		t.Fatal("stale activation changed the active repository")
	}
	if len(f.w.Repositories) != 1 {
		t.Fatalf("stale activation resurrected the row: %+v", f.w.Repositories)
	}
}

func TestRepositoriesLinkedWorktreesStaySeparate(t *testing.T) {
	f := newRepositoriesFixture(t)
	main := f.newRepo("main")
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(base, "wt")
	f.git(main, "worktree", "add", "-q", "-b", "feature", worktree)

	f.open(main)
	// The linked worktree is a separate working tree: same common Git
	// directory, different root and different status, so it gets its own row
	// instead of being merged into the main checkout.
	if err := f.add(worktree); err != nil {
		t.Fatal(err)
	}
	// Adding either checkout again deduplicates by root, not by CommonDir.
	if err := f.add(main); err != nil {
		t.Fatal(err)
	}
	if err := f.add(worktree); err != nil {
		t.Fatal(err)
	}
	if len(f.w.Repositories) != 2 {
		t.Fatalf("worktree merged with its main checkout: %+v", f.w.Repositories)
	}
	if canonicalPath(main) == canonicalPath(worktree) {
		t.Fatal("test setup names one directory twice")
	}

	f.write(worktree, "only-in-worktree.txt", "dirty\n")
	f.refreshAll()
	m := f.mustSummary(main)
	wt := f.mustSummary(worktree)
	if !m.Loaded || !wt.Loaded {
		t.Fatalf("worktree statuses not loaded: main=%+v wt=%+v", m, wt)
	}
	if m.Status.Branch.Branch != "main" || wt.Status.Branch.Branch != "feature" {
		t.Fatalf("branches mixed up: main=%q wt=%q", m.Status.Branch.Branch, wt.Status.Branch.Branch)
	}
	if len(m.Status.Entries) != 0 {
		t.Fatalf("worktree change leaked into the main checkout: %+v", m.Status.Entries)
	}
	if len(wt.Status.Entries) != 1 {
		t.Fatalf("worktree status = %+v", wt.Status.Entries)
	}

	// Switching to the worktree uses its own root and its own state.
	if err := f.activate(worktree); err != nil {
		t.Fatal(err)
	}
	if f.w.Repo.rootKey != canonicalPath(worktree) || f.w.Repo.Status.Branch.Branch != "feature" {
		t.Fatalf("active worktree = %+v", f.w.Repo)
	}
	if f.w.Repo.Repo.CommonDir == "" || !strings.Contains(f.w.Repo.Repo.CommonDir, ".git") {
		t.Fatalf("discovered worktree lost its common dir: %+v", f.w.Repo.Repo)
	}
}

func TestRepositoriesBusyStartingDuringDiscoveryRefusesSwitch(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}

	// Activation starts discovery, then a write task starts before the result
	// is applied: the switch must be refused rather than moved under the writer.
	var activateErr error = errNotRun
	f.w.ActivateRepository(repoB, func(e error) { activateErr = e })
	f.w.Tasks = append(f.w.Tasks, &Task{ID: 1000, Title: "Commit", Write: true, cancel: func() {}})
	f.settle(func() bool { return activateErr != errNotRun })
	if !errors.Is(activateErr, ErrRepositoryBusy) {
		t.Fatalf("switch with a write started during discovery = %v", activateErr)
	}
	if f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) {
		t.Fatal("busy switch changed the active repository")
	}
	f.w.Tasks = nil

	// The same applies to ordinary Open, which is a switch from the open
	// repository.
	var openErr error = errNotRun
	f.w.Open(repoB, func(e error) { openErr = e })
	f.w.Tasks = append(f.w.Tasks, &Task{ID: 1001, Title: "Commit", Write: true, cancel: func() {}})
	f.settle(func() bool { return openErr != errNotRun })
	if !errors.Is(openErr, ErrRepositoryBusy) {
		t.Fatalf("Open with a write started during discovery = %v", openErr)
	}
	if f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) {
		t.Fatal("busy Open changed the active repository")
	}
	f.w.Tasks = nil
}

func TestRepositoriesReselectActiveCancelsPendingSwitch(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}

	// The user clicks B and then A again before B's discovery finishes. The
	// last click must win: A is already active, and the pending switch to B is
	// invalidated even though it finishes later.
	var switchErr error = errNotRun
	f.w.ActivateRepository(repoB, func(e error) { switchErr = e })
	var keepErr error = errNotRun
	f.w.ActivateRepository(repoA, func(e error) { keepErr = e })
	if keepErr != nil {
		t.Fatalf("re-selecting the active repository = %v", keepErr)
	}
	f.settle(func() bool { return switchErr != errNotRun })
	if !errors.Is(switchErr, ErrRepositoryStale) {
		t.Fatalf("superseded switch error = %v", switchErr)
	}
	if f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) || f.w.ActiveRoot != canonicalPath(repoA) {
		t.Fatal("an older switch stole the selection back")
	}
}

func TestRepositoriesReadOnlyWorkspaceWarnsAndKeepsFile(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	future := []byte(`{"version": 99, "repositories": ["/some/newer/entry"], "active": "/some/newer/entry"}`)
	if err := os.WriteFile(f.workspacePath(), future, 0o600); err != nil {
		t.Fatal(err)
	}
	w := f.newWorkspace()
	if w.WorkspaceWarning == "" {
		t.Fatal("a newer workspace file was not reported")
	}

	// The list stays usable in memory, but the failed write is visible instead
	// of silent: one warning for the failure streak, not one per change.
	err := errNotRun
	w.AddRepository(repoA, func(e error) { err = e })
	drainUntil(t, w, func() bool { return err != errNotRun })
	if err != nil {
		t.Fatalf("add with a read-only workspace file = %v", err)
	}
	// The read-only file's rows are still readable (a newer Viff may have more
	// fields); the added repository joins them in memory only.
	if len(w.Repositories) != 2 {
		t.Fatalf("in-memory list = %+v", w.Repositories)
	}
	if _, ok := f.summaryFor(w, repoA); !ok {
		t.Fatalf("added repository missing: %+v", w.Repositories)
	}
	warnCount := func() int {
		n := 0
		for _, notice := range w.Notifications {
			if notice.Kind == NoticeWarning && notice.Title == "Workspace" {
				n++
			}
		}
		return n
	}
	if got := warnCount(); got != 1 {
		t.Fatalf("workspace save warnings = %d, want 1", got)
	}
	err = errNotRun
	w.AddRepository(repoB, func(e error) { err = e })
	drainUntil(t, w, func() bool { return err != errNotRun })
	if err != nil {
		t.Fatal(err)
	}
	if got := warnCount(); got != 1 {
		t.Fatalf("warning repeated on every change: %d", got)
	}
	data, err := os.ReadFile(f.workspacePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, future) {
		t.Fatalf("read-only workspace file was overwritten: %s", data)
	}
}

func TestRepositoriesOlderOverviewCannotOverwriteActiveAfterWrite(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}
	f.refreshAll()

	// Simulate an overview refresh that captured A's row before a write
	// finished. The write's refresh then publishes the authoritative status.
	entry := f.w.repos.byRoot[canonicalPath(repoA)]
	if entry == nil {
		t.Fatal("A has no entry")
	}
	staleSeq := entry.seq + 1
	entry.seq = staleSeq
	f.w.Repo.Status = domain.Status{Branch: domain.BranchStatus{Branch: "main", Head: "after-write"}}
	f.w.Repo.StatusLoaded = true
	f.w.syncActiveSummary(nil)
	if got := f.mustSummary(repoA); got.Status.Branch.Head != "after-write" || got.Err != nil {
		t.Fatalf("write refresh not applied: %+v", got)
	}

	// The older overview result lands afterwards and must be dropped.
	f.w.applyRepositoryStatus(entry, staleSeq, domain.Status{Branch: domain.BranchStatus{Branch: "main", Head: "before-write"}}, nil)
	if got := f.mustSummary(repoA); got.Status.Branch.Head != "after-write" {
		t.Fatalf("older overview result overwrote the active row: %+v", got)
	}
}

func TestRepositoriesWithoutSettingsStillAcceptAdds(t *testing.T) {
	// A composition root without settings (tests) must not panic when a list
	// operation runs: the internal maps have to exist.
	w := &Workspace{Git: &gitcli.Runner{}}
	w.loadRepositories()
	entry := w.addEntry("/tmp/viff-example-repo")
	if entry == nil || len(w.Repositories) != 1 || w.Repositories[0].Root != "/tmp/viff-example-repo" {
		t.Fatalf("workspace without settings = %+v", w.Repositories)
	}
	if err := w.saveRepositories(); err != nil {
		t.Fatalf("pathless save = %v", err)
	}
}

func TestRepositoriesRefreshMovesChangeListMembershipOnRename(t *testing.T) {
	f := newRepositoriesFixture(t)
	repo := f.newRepo("main")
	f.open(repo)

	// Integration point with the change-list backend: a status refresh must
	// persist the membership move for a rename Git reports, so a renamed file
	// does not fall back to the default list.
	id, err := f.w.CreateChangeList("Work")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.MoveToChangeList([]string{"base.txt"}, id); err != nil {
		t.Fatal(err)
	}
	if got := f.w.ChangeListFor("base.txt"); got != id {
		t.Fatalf("membership before rename = %q, want %q", got, id)
	}

	f.git(repo, "mv", "base.txt", "renamed.txt")
	f.w.Refresh(true)
	f.settle(func() bool { return idle(f.w) })
	if got := f.w.ChangeListFor("renamed.txt"); got != id {
		t.Fatalf("membership after rename = %q, want %q", got, id)
	}
}

func TestRepositoriesInterleavedOpenActivateCloseRace(t *testing.T) {
	f := newRepositoriesFixture(t)
	repoA := f.newRepo("main")
	repoB := f.newRepo("main")
	f.open(repoA)
	if err := f.add(repoB); err != nil {
		t.Fatal(err)
	}

	// Drain only runs queued results, so each request below is deliberately
	// left pending while the next one starts: the ordering is deterministic.

	// A slow Open of B is superseded by a later Activate of A (already active).
	// The last click wins; the open result must not replace it.
	var openErr error = errNotRun
	f.w.Open(repoB, func(e error) { openErr = e })
	var activateErr error = errNotRun
	f.w.ActivateRepository(repoA, func(e error) { activateErr = e })
	if activateErr != nil {
		t.Fatalf("re-selecting the active repository = %v", activateErr)
	}
	f.settle(func() bool { return openErr != errNotRun })
	if !errors.Is(openErr, ErrRepositoryStale) {
		t.Fatalf("superseded Open error = %v", openErr)
	}
	if f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) || f.w.ActiveRoot != canonicalPath(repoA) {
		t.Fatal("a slow Open replaced a newer activation")
	}

	// A slow Activate of B is superseded by Close: it must not reopen anything.
	var switchErr error = errNotRun
	f.w.ActivateRepository(repoB, func(e error) { switchErr = e })
	f.w.Close()
	f.settle(func() bool { return switchErr != errNotRun })
	if !errors.Is(switchErr, ErrRepositoryStale) {
		t.Fatalf("activation after Close did not go stale: %v", switchErr)
	}
	if f.w.Repo != nil {
		t.Fatal("a slow activation reopened the closed workspace")
	}
	if len(f.w.Repositories) != 2 {
		t.Fatalf("Close changed the list: %+v", f.w.Repositories)
	}

	// A slow Open after Close must not reopen the window either.
	var reopenErr error = errNotRun
	f.w.Open(repoB, func(e error) { reopenErr = e })
	f.w.Close()
	f.settle(func() bool { return reopenErr != errNotRun })
	if !errors.Is(reopenErr, ErrRepositoryStale) {
		t.Fatalf("Open after Close did not go stale: %v", reopenErr)
	}
	if f.w.Repo != nil {
		t.Fatal("a slow Open reopened the closed workspace")
	}

	// Invalidation only affects older requests: a new deliberate open works.
	f.open(repoA)
	if f.w.Repo == nil || f.w.Repo.rootKey != canonicalPath(repoA) {
		t.Fatal("a fresh Open was refused after the stale ones")
	}
	if f.w.Repo.Gen == 0 {
		t.Fatal("fresh Open reused generation 0")
	}
}
