//go:build vivahead

package viffui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/platform"
	"viff/internal/storage"
)

// uiFixture is a realistic repository with history, branches, a remote and
// local changes, opened in a headless window.
type uiFixture struct {
	t    *testing.T
	dir  string
	env  []string
	ws   *vapp.Workspace
	win  *Window
	s    *headless.Session
	shot string
}

func newUIFixture(t *testing.T) *uiFixture {
	t.Helper()
	platform.RegisterFonts()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	home := filepath.Join(root, "home")
	dir := filepath.Join(root, "viff-demo")
	remote := filepath.Join(root, "remote.git")
	os.MkdirAll(home, 0o755)
	os.MkdirAll(dir, 0o755)
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig")}
	f := &uiFixture{t: t, dir: dir, env: env, shot: os.Getenv("VIFF_SHOTS")}
	if f.shot != "" && os.Getenv("VIFF_SHOTS_BY_TEST") == "1" {
		f.shot = filepath.Join(f.shot, t.Name())
	}
	f.gitIn(root, "init", "-q", "--bare", "-b", "main", remote)
	f.git("init", "-q", "-b", "main")
	f.git("config", "user.name", "Ada Lovelace")
	f.git("config", "user.email", "ada@example.com")
	f.git("config", "commit.gpgsign", "false")
	f.git("remote", "add", "origin", remote)

	f.write("README.md", "# Viff demo\n\nA small project used to exercise the client.\n")
	f.write("main.go", mainV1)
	f.write("internal/store/store.go", storeV1)
	f.write("config.json", "{\n  \"name\": \"demo\",\n  \"port\": 8080,\n  \"debug\": false\n}\n")
	f.commit("Initial commit", "2026-10-01T09:00:00")
	f.write("internal/store/store_test.go", "package store\n\nimport \"testing\"\n\nfunc TestPut(t *testing.T) {\n\ts := New()\n\ts.Put(\"a\", 1)\n}\n")
	f.commit("Add store tests", "2026-10-03T10:12:00")
	f.git("push", "-q", "-u", "origin", "main")
	f.git("tag", "v0.1.0")
	f.git("checkout", "-q", "-b", "feature/search")
	f.write("internal/search/search.go", "package search\n\n// Find returns the keys that contain q.\nfunc Find(keys []string, q string) []string {\n\tvar out []string\n\tfor _, k := range keys {\n\t\tif contains(k, q) {\n\t\t\tout = append(out, k)\n\t\t}\n\t}\n\treturn out\n}\n")
	f.commit("Add search package", "2026-10-05T14:30:00")
	f.write("README.md", "# Viff demo\n\nA small project used to exercise the client.\n\n## Search\n\nUse `search.Find`.\n")
	f.commit("Document search", "2026-10-06T08:45:00")
	f.git("checkout", "-q", "main")
	f.write("config.json", "{\n  \"name\": \"demo\",\n  \"port\": 9090,\n  \"debug\": false,\n  \"timeout\": 30\n}\n")
	f.commit("Raise default port and add timeout", "2026-10-07T11:00:00")
	f.gitDated("2026-10-08T12:00:00", "merge", "-q", "--no-ff", "-m", "Merge branch 'feature/search'", "feature/search")
	f.git("push", "-q", "origin", "main", "feature/search")
	f.git("checkout", "-q", "-b", "fix/store-locking")
	f.write("internal/store/store.go", storeV2)
	f.commit("Guard the store with a mutex", "2026-10-09T16:20:00")
	f.git("checkout", "-q", "main")
	f.write("main.go", mainV2)
	f.commit("Log the listen address", "2026-10-12T09:30:00")

	// Local changes: one staged file, unstaged edits, an untracked file.
	f.write("README.md", "# Viff demo\n\nA small project used to exercise Viff, an IntelliJ-style Git client.\n\n## Search\n\nUse `search.Find`.\n")
	f.git("add", "README.md")
	f.write("main.go", mainV3)
	f.write("internal/store/store.go", storeV1+"\n// Len reports the number of keys.\nfunc (s *Store) Len() int { return len(s.m) }\n")
	f.write("notes/todo.txt", "- staging per line\n- conflict viewer\n")

	settings := storage.Load(filepath.Join(home, "settings.json"))
	var win *Window
	f.ws = vapp.NewWorkspace(&gitcli.Runner{Env: env}, settings, func() {
		if win != nil {
			win.Invalidate()
		}
	})
	win = New(f.ws)
	f.win = win
	return f
}

func (f *uiFixture) start(width, height int) {
	cfg := f.win.Config()
	cfg.Width, cfg.Height = width, height
	s, err := headless.Open(cfg, nil, headless.Options{Scale: 2})
	if err != nil {
		f.t.Fatal(err)
	}
	f.s = s
	f.t.Cleanup(func() {
		defer f.ws.Close()
		defer s.Close()
		if os.Getenv("VIFF_SHOTS_BY_TEST") == "1" {
			f.snap("99-final-state")
		}
	})
}

func (f *uiFixture) git(args ...string) string { f.t.Helper(); return f.gitIn(f.dir, args...) }

func (f *uiFixture) gitIn(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (f *uiFixture) gitDated(date string, args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true", "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (f *uiFixture) write(name, content string) {
	p := filepath.Join(f.dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *uiFixture) commit(msg, date string) {
	f.git("add", "-A")
	cmd := exec.Command("git", "commit", "-q", "-m", msg)
	cmd.Dir = f.dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("commit: %v\n%s", err, out)
	}
}

// settle waits for the workspace to finish background work.
func (f *uiFixture) settle() {
	f.t.Helper()
	err := f.s.WaitFor(func(app.Snapshot) bool {
		rs := f.ws.Repo
		return rs != nil && rs.StatusLoaded && rs.RefsLoaded && rs.Log.Loaded() && !rs.Log.Loading && !f.ws.Busy() && !f.ws.Pending() && !rs.DetailLoading
	}, 20*time.Second)
	if err != nil {
		f.t.Fatal(err)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
}

// settleTab also waits for the active editor tab to load.
func (f *uiFixture) settleTab() {
	f.t.Helper()
	f.settle()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		t := f.win.ed.cur()
		return t != nil && !t.loading
	}, 20*time.Second); err != nil {
		f.t.Fatal(err)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
}

// assertIdle fails if the window keeps repainting with nothing going on
// (a deferred-work loop would show up here).
func (f *uiFixture) assertIdle() {
	f.t.Helper()
	f.s.Settle(100*time.Millisecond, 2*time.Second)
	before := f.win.paints
	time.Sleep(700 * time.Millisecond)
	f.s.Settle(50*time.Millisecond, time.Second)
	if n := f.win.paints - before; n > 3 {
		f.t.Fatalf("window repainted %d times while idle", n)
	}
}

func (f *uiFixture) snap(name string) {
	f.t.Helper()
	if f.shot == "" {
		return
	}
	if err := os.MkdirAll(f.shot, 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := f.s.Screenshot(filepath.Join(f.shot, name+".png")); err != nil {
		f.t.Fatal(err)
	}
}

func (f *uiFixture) click(label string) {
	f.t.Helper()
	if err := f.s.ClickLabel(label); err != nil {
		f.t.Fatalf("click %q: %v\nhits: %s", label, err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
}

func TestScreenshots(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	f.snap("01-main")
	f.assertIdle()

	f.click("main.go")
	f.settleTab()
	f.snap("02-diff")

	f.click("Branch: main")
	f.snap("03-branches")
	f.s.Keys("escape")

	f.click("Merge branch 'feature/search'")
	f.settle()
	f.snap("04-log-selected")
	f.assertIdle()

	f.ws.Settings.DiffUnified = true
	f.click("main.go")
	f.settle()
	f.snap("05-unified")
	f.ws.Settings.DiffUnified = false

	f.win.openPush()
	f.settle()
	f.snap("06-push")
	f.s.Keys("escape")

	f.win.openNewBranch("", "main")
	f.win.Invalidate()
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("07-new-branch")
	f.s.Keys("escape")

	f.ws.Settings.Theme = "light"
	f.win.applyTheme()
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("08-light")
}

// TestMoreScreenshots covers the welcome screen, stashes, settings and the
// conflict viewer.
func TestMoreScreenshots(t *testing.T) {
	f := newUIFixture(t)
	f.git("stash", "push", "-q", "-m", "WIP on store length", "--", "internal/store/store.go")
	f.ws.Settings.AddRecent(f.dir)
	f.ws.Settings.AddRecent(filepath.Join(filepath.Dir(f.dir), "missing-project"))
	f.start(1280, 800)
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	f.snap("20-welcome")

	f.win.openRepo(f.dir)
	f.settle()
	f.win.commitTab = 1
	f.win.Invalidate()
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.st.forHash != "" && !f.win.st.loading }, 10*time.Second); err != nil {
		t.Fatalf("stash files never loaded: %v (st=%+v stashes=%+v)", err, f.win.st, f.ws.Repo.Stashes)
	}
	f.settle()
	found := false
	for _, c := range f.win.st.files {
		found = found || c.Path == "internal/store/store.go"
	}
	if !found {
		t.Fatalf("stash files = %v, want store.go among them", f.win.st.files)
	}
	f.snap("21-stash")
	f.assertIdle()
	f.win.commitTab = 0

	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("22-settings")
	f.s.Keys("escape")

	f.win.openStashDialog()
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("23-stash-dialog")
	f.s.Keys("escape")

	// A merge conflict in store.go.
	f.git("checkout", "-q", "--", ".")
	f.git("reset", "-q", "--hard")
	f.write("internal/store/store.go", strings.Replace(storeV1, "in-memory key-value store", "tiny in-memory key-value store", 1)+"\n// Len reports the number of keys.\nfunc (s *Store) Len() int { return len(s.m) }\n")
	f.commit("Add Store.Len", "2026-10-13T10:00:00")
	cmd := exec.Command("git", "merge", "fix/store-locking")
	cmd.Dir = f.dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("merge unexpectedly succeeded: %s", out)
	}
	f.ws.Refresh(true)
	f.win.Invalidate()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && len(f.ws.Repo.Entries(domain.AreaConflicts)) > 0
	}, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	f.settle()
	f.snap("24-merge-state")
	f.click("store.go")
	f.settleTab()
	f.snap("25-conflict")
	f.assertIdle()
}

// TestLogSearch types into the log's search field and checks the debounced
// filter runs without any click.
func TestLogSearch(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	f.click("Text or hash")
	if err := f.s.Type("mutex"); err != nil {
		t.Fatal(err)
	}
	f.s.Wait(500 * time.Millisecond)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		l := &f.ws.Repo.Log
		return l.Filter.Text == "mutex" && l.Loaded() && !l.Loading
	}, 10*time.Second); err != nil {
		t.Fatalf("search never applied: filter=%q", f.ws.Repo.Log.Filter.Text)
	}
	f.settle()
	if got := f.ws.Repo.Log.Commits; len(got) != 1 || got[0].Subject != "Guard the store with a mutex" {
		t.Fatalf("filtered log = %v", got)
	}
	f.snap("26-log-search")
}

// TestStageBlockAndNewBranch stages one diff block from the divider and
// creates a branch through the New Branch dialog.
func TestStageBlockAndNewBranch(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	f.click("main.go")
	f.settleTab()

	h, ok := f.s.Hit("Stage Change")
	if !ok {
		t.Fatalf("no Stage Change button; hits: %s", strings.Join(f.s.Texts(), " | "))
	}
	if err := f.s.Click(h.Bounds.X+h.Bounds.Width/2, h.Bounds.Y+h.Bounds.Height/2); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.git("diff", "--cached", "--", "main.go") != ""
	}, 10*time.Second); err != nil {
		t.Fatal("no block was staged")
	}
	f.settle()
	staged := f.git("diff", "--cached", "--", "main.go")
	if n := strings.Count(staged, "\n@@ "); n != 1 {
		t.Fatalf("staged %d hunks, want exactly one block:\n%s", n, staged)
	}
	if !strings.Contains(f.git("diff", "--", "main.go"), "addr returns") {
		t.Fatal("the other blocks should stay unstaged")
	}
	f.snap("30-block-staged")

	f.s.Keys("cmd+alt+n")
	f.s.Settle(40*time.Millisecond, time.Second)
	if f.win.dialog == nil {
		t.Fatal("⌥⌘N did not open the New Branch dialog")
	}
	if err := f.s.Type("feature/ui-test"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("Create")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo.CurrentBranch() == "feature/ui-test"
	}, 10*time.Second); err != nil {
		t.Fatalf("branch not created/checked out; current %q", f.ws.Repo.CurrentBranch())
	}
	f.settle()
	if got := strings.TrimSpace(f.git("rev-parse", "--abbrev-ref", "HEAD")); got != "feature/ui-test" {
		t.Fatalf("HEAD = %q", got)
	}
	f.snap("31-new-branch-done")
}

// TestInteractiveRebaseDialog fixes up and rewords commits through the
// dialog opened from the log, then checks the history with git.
func TestInteractiveRebaseDialog(t *testing.T) {
	f := newUIFixture(t)
	for _, c := range []struct{ file, msg, date string }{{"docs/a.md", "Docs A", "2026-10-12T10:00:00"}, {"docs/b.md", "Docs B", "2026-10-12T11:00:00"}} {
		f.write(c.file, c.msg+"\n")
		f.git("add", c.file)
		f.gitDated(c.date, "commit", "-q", "-m", c.msg, "--", c.file)
	}
	before := strings.TrimSpace(f.git("rev-list", "--count", "HEAD"))
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	if err := f.s.RightClickLabel("Docs A"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("Interactively Rebase from Here…")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && strings.HasPrefix(f.win.dialog.title, "Rebasing 2 Commits")
	}, 10*time.Second); err != nil {
		t.Fatalf("rebase dialog did not open: %v", f.win.dialog)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("Docs B")
	f.s.Keys("Meta+f")
	f.click("Docs A")
	f.s.Keys("Meta+r")
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("Commit message") // the dialog's field is on top of the Commit window's
	if err := f.s.Type(" (reworded)"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("32-rebase-dialog")
	f.click("Start Rebasing")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog == nil && !f.ws.Busy()
	}, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	f.settle()
	after := strings.TrimSpace(f.git("rev-list", "--count", "HEAD"))
	subject := strings.TrimSpace(f.git("log", "-1", "--format=%s"))
	files := f.git("show", "--name-only", "--format=", "HEAD")
	if after == before || !strings.Contains(subject, "reworded") || !strings.Contains(files, "docs/a.md") || !strings.Contains(files, "docs/b.md") {
		t.Fatalf("rebase result: %s→%s commits, subject %q, files %q", before, after, subject, files)
	}
	// Local changes survived through --autostash.
	if data, _ := os.ReadFile(filepath.Join(f.dir, "main.go")); string(data) != mainV3 {
		t.Fatal("local change to main.go lost")
	}
	f.snap("33-rebased")
	f.assertIdle()
}

const mainV1 = `package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello")
	})
	http.ListenAndServe(":8080", nil)
}
`

const mainV2 = `package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello")
	})
	log.Println("listening on :8080")
	http.ListenAndServe(":8080", nil)
}
`

const mainV3 = `package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

// addr returns the listen address from $PORT.
func addr() string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":9090"
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello, world")
	})
	log.Println("listening on", addr())
	log.Fatal(http.ListenAndServe(addr(), nil))
}
`

const storeV1 = `package store

// Store is an in-memory key-value store.
type Store struct {
	m map[string]int
}

// New creates an empty store.
func New() *Store { return &Store{m: map[string]int{}} }

// Put sets a key.
func (s *Store) Put(k string, v int) { s.m[k] = v }

// Get reads a key.
func (s *Store) Get(k string) (int, bool) {
	v, ok := s.m[k]
	return v, ok
}
`

const storeV2 = `package store

import "sync"

// Store is an in-memory key-value store safe for concurrent use.
type Store struct {
	mu sync.Mutex
	m  map[string]int
}

// New creates an empty store.
func New() *Store { return &Store{m: map[string]int{}} }

// Put sets a key.
func (s *Store) Put(k string, v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
}
`

// TestCommitFlow drives the UI: stage a file from the keyboard, type a
// message, commit, and checks the repository.
func TestCommitFlow(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.click("main.go")
	f.settle()
	if err := f.s.Keys("space"); err != nil {
		t.Fatal(err)
	}
	f.settle()
	if out := f.git("diff", "--cached", "--name-only"); !strings.Contains(out, "main.go") {
		t.Fatalf("main.go not staged after Space: %q", out)
	}

	f.click("Commit message")
	if err := f.s.Type("Read the port from the environment"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("10-message")
	f.click("Commit")
	f.settle()
	if subj := strings.TrimSpace(f.git("log", "-1", "--format=%s")); subj != "Read the port from the environment" {
		t.Fatalf("last commit = %q", subj)
	}
	if got := f.win.cm.msg.String(); got != "" {
		t.Fatalf("message not cleared: %q", got)
	}
	f.snap("11-committed")
}

// TestWorktreesDialog covers the Worktrees dialog, the marker the branch
// popup puts on a branch that another worktree has checked out, and creating
// a worktree through the form.
func TestWorktreesDialog(t *testing.T) {
	f := newUIFixture(t)
	wt := filepath.Join(t.TempDir(), "viff-demo-wt")
	f.git("worktree", "add", "-q", "-b", "wt-feature", wt)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	// The branch popup says where a branch checked out elsewhere lives.
	f.win.openBranchPopup(rect(240, 60, 160, 24))
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	if !f.s.HasText("in viff-demo-wt") {
		t.Fatalf("popup does not mark the other worktree: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("40-worktree-branch")

	f.click("Worktrees…")
	if f.win.dialog == nil || f.win.dialog.title != "Worktrees" {
		t.Fatalf("worktrees dialog = %+v", f.win.dialog)
	}
	if !f.s.HasText("2 worktrees") || !f.s.HasText("wt-feature") || !f.s.HasText("viff-demo-wt") || !f.s.HasText("current") {
		t.Fatalf("worktrees rows: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("41-worktrees")

	// The form suggests a sibling path from the branch name.
	f.click("New Worktree…")
	if f.win.dialog == nil || f.win.dialog.title != "New Worktree" {
		t.Fatalf("new worktree dialog = %+v", f.win.dialog)
	}
	f.click("Branch name:")
	if err := f.s.Type("ui-wt"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("42-new-worktree")
	f.click("Create")

	want := filepath.Join(filepath.Dir(f.dir), filepath.Base(f.dir)+"-ui-wt")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo.WorktreesLoaded && len(f.ws.Repo.Worktrees) == 3 && !f.ws.Busy()
	}, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && f.win.dialog.title == "Worktrees"
	}, 10*time.Second); err != nil {
		t.Fatalf("the worktrees dialog did not come back: %v", f.win.dialog)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	if out := f.git("worktree", "list", "--porcelain"); !porcelainNamesPath(out, want) {
		t.Fatalf("worktree %s not created:\n%s", want, out)
	}
	if !f.s.HasText("3 worktrees") || !f.s.HasText("ui-wt") || !f.s.HasText("viff-demo-ui-wt") {
		t.Fatalf("the new worktree is not listed: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("43-worktree-created")

	rowOf := func(branch string) string {
		for _, wt := range f.ws.Repo.Worktrees {
			if wt.Branch == branch {
				return worktreeLabel(wt)
			}
		}
		return ""
	}

	// Select the other worktree and delete it with the toolbar action.
	f.click(rowOf("wt-feature"))
	if !f.s.HasText("Copy Path") || !f.s.HasText("Delete…") {
		t.Fatalf("actions missing: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.click("Delete…")
	if f.win.dialog == nil || f.win.dialog.title != "Delete Worktree" {
		t.Fatalf("delete dialog = %+v", f.win.dialog)
	}
	f.snap("44-delete-worktree")
	f.click("Delete")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return len(f.ws.Repo.Worktrees) == 2 && !f.ws.Busy()
	}, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if out := f.git("worktree", "list", "--porcelain"); porcelainNamesPath(out, wt) {
		t.Fatalf("worktree still listed:\n%s", out)
	}
	if !f.s.HasText("2 worktrees") {
		t.Fatalf("the deleted worktree is still listed: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("45-worktree-deleted")

	// Open the new worktree: the window switches to it.
	f.click(rowOf("ui-wt"))
	f.click("Open")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && f.ws.Repo.Repo.Root == want && !f.ws.Busy()
	}, 20*time.Second); err != nil {
		t.Fatalf("did not open %s: %v", want, err)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	if f.win.dialog != nil {
		t.Fatalf("the worktrees dialog stayed open: %+v", f.win.dialog)
	}
	if !f.s.HasText("ui-wt") {
		t.Fatalf("the branch widget does not show ui-wt: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("46-worktree-opened")
	f.assertIdle()
}

// TestCheckoutOpensOtherWorktree checks that checking out a branch another
// worktree holds opens that worktree instead of failing.
func TestCheckoutOpensOtherWorktree(t *testing.T) {
	f := newUIFixture(t)
	wt := filepath.Join(t.TempDir(), "viff-demo-wt")
	f.git("worktree", "add", "-q", "-b", "wt-feature", wt)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.win.openBranchPopup(rect(240, 60, 160, 24))
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("wt-feature")
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("Checkout")
	real, err := filepath.EvalSymlinks(wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && f.ws.Repo.Repo.Root == real && !f.ws.Busy()
	}, 20*time.Second); err != nil {
		t.Fatalf("checkout did not open %s: %v", real, err)
	}
	// Opening a worktree loads its state in the background: the worktree list
	// can still be the empty one from before the switch when the root is
	// already the new checkout, so wait for that load too.
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		rs := f.ws.Repo
		return rs != nil && rs.WorktreesLoaded && len(rs.Worktrees) == 2 && !f.ws.Busy()
	}, 20*time.Second); err != nil {
		t.Fatalf("worktrees did not load for %s: %v\nworktrees: %+v", real, err, f.ws.Repo.Worktrees)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	if len(f.ws.Repo.Worktrees) != 2 {
		t.Fatalf("worktrees = %+v", f.ws.Repo.Worktrees)
	}
	if !f.s.HasText("feature") && !f.s.HasText("wt-feature") {
		t.Fatalf("branch widget: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("47-checkout-other-worktree")
	f.assertIdle()
}
