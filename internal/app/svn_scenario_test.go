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
	"viff/internal/svncli"
)

// svnTools resolves the Subversion executables, skipping when the machine has
// none. VIFF_SVN / VIFF_SVNADMIN override the lookup so a Windows install
// outside PATH (the ordinary case) can still run these tests.
func svnTools(t *testing.T) (svn, admin string) {
	t.Helper()
	svn = os.Getenv("VIFF_SVN")
	if svn == "" {
		p, err := exec.LookPath("svn")
		if err != nil {
			t.Skip("svn is not installed; set VIFF_SVN to run this test")
		}
		svn = p
	}
	admin = os.Getenv("VIFF_SVNADMIN")
	if admin == "" {
		p, err := exec.LookPath("svnadmin")
		if err != nil {
			t.Skip("svnadmin is not installed; set VIFF_SVNADMIN to run this test")
		}
		admin = p
	}
	return svn, admin
}

// svnFixture builds a repository over file:// plus a checked-out working copy,
// and a workspace with both adapters registered, so the backend is chosen by
// probing the directory exactly as the application does.
type svnFixture struct {
	t   *testing.T
	wc  string
	env []string
	w   *Workspace
	svn *svncli.Runner
}

func newSVNFixture(t *testing.T) *svnFixture {
	t.Helper()
	svn, admin := svnTools(t)
	root := t.TempDir()
	home := t.TempDir()
	// APPDATA decides Subversion's own directory on Windows; HOME covers the
	// Unix layout. Both point into the temporary directory so the machine's
	// real auth cache and config are never touched.
	env := []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + home,
		"APPDATA=" + filepath.Join(home, "AppData"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig"),
	}

	repoPath := filepath.Join(root, "repo")
	create := exec.Command(admin, "create", repoPath)
	create.Env = env
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("svnadmin create: %v: %s", err, out)
	}
	// Windows needs the three-slash file URL.
	url := "file:///" + filepath.ToSlash(repoPath)

	f := &svnFixture{t: t, env: env}
	f.svn = &svncli.Runner{Path: svn, Env: env}

	for _, p := range []string{"/trunk", "/branches", "/tags"} {
		c := exec.Command(svn, "--non-interactive", "mkdir", "-m", "layout", url+p)
		c.Env = env
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("mkdir %s: %v: %s", p, err, out)
		}
	}
	parent := filepath.Join(root, "co")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	f.wc = filepath.Join(parent, "wc")
	co := exec.Command(svn, "--non-interactive", "checkout", url+"/trunk", f.wc)
	co.Env = env
	if out, err := co.CombinedOutput(); err != nil {
		t.Fatalf("checkout: %v: %s", err, out)
	}

	// Git is registered first on purpose: the probe must override it.
	git := &gitcli.Runner{Env: env}
	f.w = NewWorkspace(git, storage.Defaults(), nil)
	f.w.SetBackends(git, f.svn)
	return f
}

func (f *svnFixture) run(args ...string) string {
	f.t.Helper()
	c := exec.Command(f.svn.Path, append([]string{"--non-interactive"}, args...)...)
	c.Dir = f.wc
	c.Env = f.env
	out, err := c.CombinedOutput()
	if err != nil {
		f.t.Fatalf("svn %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (f *svnFixture) write(name, content string) {
	f.t.Helper()
	p := filepath.Join(f.wc, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// settle drains background results until cond holds.
func (f *svnFixture) settle(cond func() bool) {
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

func (f *svnFixture) idle() bool {
	rs := f.w.Repo
	return rs != nil && !rs.refreshing && !rs.Log.Loading && len(f.w.Tasks) == 0 && !f.w.Pending()
}

func (f *svnFixture) open() {
	f.t.Helper()
	f.w.Open(f.wc, func(err error) {
		if err != nil {
			f.t.Fatalf("open: %v", err)
		}
	})
	f.settle(func() bool { return f.w.Repo != nil && f.w.Repo.StatusLoaded && f.idle() })
}

// seed commits a first revision through the command line so the workspace
// opens onto a repository with history.
func (f *svnFixture) seed() {
	f.t.Helper()
	f.write("main.txt", "alpha\nbeta\n")
	f.run("add", "main.txt")
	f.run("commit", "-m", "initial content")
}

// TestSVNWorkspaceOpensByProbe covers the whole user path: a directory holding
// a Subversion working copy is served by the Subversion adapter even though the
// Git adapter is registered first.
func TestSVNWorkspaceOpensByProbe(t *testing.T) {
	f := newSVNFixture(t)
	f.seed()
	f.open()

	rs := f.w.Repo
	if rs.Repo.Kind != domain.KindSVN {
		t.Fatalf("repository kind = %v, want KindSVN", rs.Repo.Kind)
	}
	if f.w.BackendName() != "Subversion" {
		t.Fatalf("backend = %q, want Subversion", f.w.BackendName())
	}
	if rs.Repo.URL == "" || rs.Repo.RepoRoot == "" || rs.Repo.UUID == "" {
		t.Fatalf("repository identity incomplete: %+v", rs.Repo)
	}
	if !rs.RefsLoaded {
		t.Fatal("refs were not loaded")
	}
	if len(rs.Log.Commits) == 0 {
		t.Fatal("history is empty after opening a repository with one commit")
	}
	if rs.Log.Commits[0].Subject != "initial content" {
		t.Fatalf("newest subject = %q, want the seeded commit", rs.Log.Commits[0].Subject)
	}
}

// TestSVNWorkspaceHasNoStagedGroup checks the model difference the UI depends
// on: with no index, a modification lands in the unstaged column and the
// staging column stays empty.
func TestSVNWorkspaceHasNoStagedGroup(t *testing.T) {
	f := newSVNFixture(t)
	f.seed()
	f.open()

	if f.w.Caps.Index {
		t.Fatal("Subversion reports a staging area")
	}
	if f.w.Caps.Stash != nil || f.w.Caps.HistoryWriter != nil || f.w.Caps.Worktrees != nil ||
		f.w.Caps.Reflog != nil || f.w.Caps.Submodules != nil {
		t.Fatalf("Subversion must not advertise Git-only services: %+v", f.w.Caps)
	}
	if !f.w.Caps.OfflineHistory {
		t.Fatal("Subversion history needs the repository, so OfflineHistory must be true")
	}

	f.write("main.txt", "alpha\nBETA\n")
	f.w.Refresh(false)
	f.settle(func() bool { return len(f.w.Repo.Status.Entries) == 1 && f.idle() })

	rs := f.w.Repo
	if got := len(rs.Entries(domain.AreaStaged)); got != 0 {
		t.Fatalf("staged entries = %d, want 0 (no index in Subversion)", got)
	}
	if got := len(rs.Entries(domain.AreaUnstaged)); got != 1 {
		t.Fatalf("unstaged entries = %d, want 1", got)
	}
}

// TestSVNWorkspaceCommit drives the write path through the use cases: schedule
// an addition, commit, then confirm the working copy is clean and the new
// revision reached the log.
func TestSVNWorkspaceCommit(t *testing.T) {
	f := newSVNFixture(t)
	f.seed()
	f.open()

	f.write("new.txt", "brand new\n")
	f.w.Refresh(false)
	f.settle(func() bool { return len(f.w.Repo.Entries(domain.AreaUntracked)) == 1 && f.idle() })

	// Schedule the addition the way the UI does, then commit through the use
	// case so the refresh and notification path is exercised too.
	if err := f.svn.Stage(t.Context(), f.w.Repo.Repo, []string{"new.txt"}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	var committed bool
	f.w.Commit(CommitRequest{Message: "add new file"}, func() { committed = true })
	f.settle(func() bool { return committed && f.idle() })

	f.settle(func() bool { return len(f.w.Repo.Status.Entries) == 0 && f.idle() })

	f.w.Refresh(true)
	f.settle(func() bool { return f.idle() && len(f.w.Repo.Log.Commits) > 0 })
	if subj := f.w.Repo.Log.Commits[0].Subject; subj != "add new file" {
		t.Fatalf("newest subject = %q, want the commit just made", subj)
	}
}

// TestSVNThenGitBackendSwitch checks that opening a Git repository in the same
// workspace moves the active adapter back, which the multi-repository list
// depends on.
func TestSVNThenGitBackendSwitch(t *testing.T) {
	f := newSVNFixture(t)
	f.seed()
	f.open()
	if f.w.BackendName() != "Subversion" {
		t.Fatalf("backend = %q, want Subversion", f.w.BackendName())
	}

	gitDir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "Viff Test"},
		{"config", "user.email", "test@example.com"},
		{"config", "commit.gpgsign", "false"},
	} {
		c := exec.Command("git", args...)
		c.Dir = gitDir
		c.Env = f.env
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "git commit"}} {
		c := exec.Command("git", args...)
		c.Dir = gitDir
		c.Env = f.env
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	f.w.Open(gitDir, func(err error) {
		if err != nil {
			t.Fatalf("open git: %v", err)
		}
	})
	f.settle(func() bool { return f.w.Repo != nil && f.w.Repo.Repo.Root == gitDir && f.idle() })

	if f.w.BackendName() != "Git" {
		t.Fatalf("backend after switching = %q, want Git", f.w.BackendName())
	}
	if !f.w.Caps.Index {
		t.Fatal("the Git backend must report a staging area")
	}
	if f.w.Caps.Stash == nil {
		t.Fatal("the Git backend must offer stash")
	}
	if len(f.w.Repo.Log.Commits) == 0 {
		t.Fatal("git history is empty")
	}
	if got := strings.TrimSpace(f.w.Repo.Log.Commits[0].Subject); got != "git commit" {
		t.Fatalf("git history subject = %q", got)
	}
}
