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
	"viff/internal/i18n"
	"viff/internal/platform"
	"viff/internal/storage"
	"viff/internal/svncli"
)

// svnUI resolves the Subversion executables, skipping when the machine has
// none. VIFF_SVN / VIFF_SVNADMIN override the lookup so a Windows install
// outside PATH (the ordinary case) can still run these tests.
func svnUI(t *testing.T) (svn, admin string) {
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

// svnFixture is a window on a Subversion working copy. It registers both
// adapters, so the backend is chosen by probing the directory exactly as the
// application does.
type svnFixture struct {
	t    *testing.T
	wc   string
	url  string
	env  []string
	ws   *vapp.Workspace
	win  *Window
	s    *headless.Session
	svn  *svncli.Runner
	home string
}

func newSVNFixture(t *testing.T) *svnFixture {
	t.Helper()
	svn, admin := svnUI(t)
	platform.RegisterFonts()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	// APPDATA decides Subversion's own directory on Windows; HOME covers the
	// Unix layout. Both point into the temporary directory so the machine's
	// real auth cache and configuration are never touched.
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
	url := "file:///" + filepath.ToSlash(repoPath)

	f := &svnFixture{t: t, url: url + "/trunk", env: env, home: home}
	f.svn = &svncli.Runner{Path: svn, Env: env}

	// r1: the conventional layout in one commit, so revision numbers stay
	// predictable.
	runSVN(t, env, "", svn, "mkdir", "-m", "layout", url+"/trunk", url+"/branches", url+"/tags")
	parent := filepath.Join(root, "co")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	f.wc = filepath.Join(parent, "wc")
	runSVN(t, env, parent, svn, "checkout", url+"/trunk", f.wc)

	// r2: content worth showing in a diff.
	f.write("README.md", "# Demo project\n\nA working copy used to exercise Viff.\n")
	f.write("main.go", "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n")
	f.write("internal/store/store.go", "package store\n\ntype Store struct{ m map[string]int }\n\nfunc New() *Store { return &Store{m: map[string]int{}} }\n")
	runSVN(t, env, f.wc, svn, "add", "README.md", "main.go", "internal")
	runSVN(t, env, f.wc, svn, "commit", "-m", "Initial project layout")

	// r3: a second commit so history has two entries.
	f.write("main.go", "package main\n\nfunc main() {\n\tprintln(\"hello from svn\")\n}\n")
	runSVN(t, env, f.wc, svn, "commit", "-m", "Print a friendlier greeting")

	settings := storage.Load(filepath.Join(home, "settings.json"))
	var win *Window
	git := &gitcli.Runner{Env: env}
	f.ws = vapp.NewWorkspace(git, settings, func() {
		if win != nil {
			win.Invalidate()
		}
	})
	f.ws.SetBackends(git, f.svn)
	win = New(f.ws)
	f.win = win
	return f
}

func runSVN(t *testing.T, env []string, dir, svn string, args ...string) string {
	t.Helper()
	cmd := exec.Command(svn, append([]string{"--non-interactive"}, args...)...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("svn %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (f *svnFixture) start(width, height int) {
	f.t.Helper()
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
	})
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

// settle waits for the workspace to finish background work.
func (f *svnFixture) settle() {
	f.t.Helper()
	err := f.s.WaitFor(func(app.Snapshot) bool {
		rs := f.ws.Repo
		return rs != nil && rs.StatusLoaded && rs.RefsLoaded && rs.Log.Loaded() &&
			!rs.Log.Loading && !f.ws.Busy() && !f.ws.Pending() && !rs.DetailLoading
	}, 25*time.Second)
	if err != nil {
		f.t.Fatal(err)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
}

func (f *svnFixture) open() {
	f.t.Helper()
	f.win.openRepo(f.wc)
	f.settle()
}

func (f *svnFixture) click(label string) {
	f.t.Helper()
	if err := f.s.ClickLabel(label); err != nil {
		f.t.Fatalf("click %q: %v\nhits: %s", label, err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
}

// groupTitles lists the area headings the changes tree is showing, which is
// what tells a staging area apart from a single-column list.
func (f *svnFixture) groupTitles() []string {
	f.t.Helper()
	var out []string
	for _, r := range f.win.cm.rows {
		if r.kind == rowGroup {
			out = append(out, f.win.areaTitle(r.area))
		}
	}
	return out
}

// TestSVNUIChangeListHasNoStagingArea is the visible half of the model
// difference: the changes tree must show one list, not a Staged group that
// would always be empty.
func TestSVNUIChangeListHasNoStagingArea(t *testing.T) {
	f := newSVNFixture(t)
	f.start(1440, 900)
	f.open()

	if f.ws.BackendName() != "Subversion" {
		t.Fatalf("backend = %q, want Subversion", f.ws.BackendName())
	}

	// A modification and an unversioned file, so both areas have content.
	f.write("README.md", "# Demo project\n\nEdited locally.\n")
	f.write("loose.txt", "not versioned\n")
	f.ws.Refresh(false)
	// Refresh is asynchronous; settle() alone returns before the new entries
	// arrive because the previous status was already loaded.
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		rs := f.ws.Repo
		return rs != nil && len(rs.Status.Entries) >= 2 && !f.ws.Busy() && !f.ws.Pending()
	}, 25*time.Second); err != nil {
		t.Fatalf("the local changes never reached the list: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	titles := f.groupTitles()
	for _, title := range titles {
		if title == "Staged" {
			t.Fatalf("a Staged group is shown for a backend without an index: %v", titles)
		}
	}
	var sawChanges, sawUnversioned bool
	for _, title := range titles {
		switch title {
		case "Changes":
			sawChanges = true
		case "Unversioned Files":
			sawUnversioned = true
		}
	}
	if !sawChanges {
		t.Fatalf("no Changes group in %v", titles)
	}
	if !sawUnversioned {
		t.Fatalf("no Unversioned Files group in %v", titles)
	}

	// The app state agrees: nothing is staged.
	if n := len(f.ws.Repo.Entries(domain.AreaStaged)); n != 0 {
		t.Fatalf("staged entries = %d, want 0", n)
	}
	if n := len(f.ws.Repo.Entries(domain.AreaUnstaged)); n != 1 {
		t.Fatalf("unstaged entries = %d, want 1", n)
	}
}

// TestSVNUIGitOnlyEntriesAreHidden checks that the entries a Subversion working
// copy cannot serve are absent from the window rather than present and failing.
func TestSVNUIGitOnlyEntriesAreHidden(t *testing.T) {
	f := newSVNFixture(t)
	f.start(1440, 900)
	f.open()

	texts := f.s.Texts()
	joined := strings.Join(texts, " | ")

	// The Stash tab belongs to the Commit tool window's header.
	if f.ws.Caps.Stash != nil {
		t.Fatal("Subversion reports a stash capability")
	}
	for _, absent := range []string{"Stash", "Reflog", "Worktrees"} {
		if strings.Contains(joined, absent) {
			t.Fatalf("%q is shown for a backend without that feature: %s", absent, joined)
		}
	}

	// The entries that do belong to a daily Subversion client are present.
	for _, present := range []string{"Commit", "Log", "Console"} {
		if !strings.Contains(joined, present) {
			t.Fatalf("%q is missing: %s", present, joined)
		}
	}

	// The branch menu must not offer rebase or merge.
	if err := f.s.ClickLabel("Branches"); err == nil {
		f.s.Settle(40*time.Millisecond, time.Second)
		menu := strings.Join(f.s.Texts(), " | ")
		for _, absent := range []string{"Rebase", "Merge"} {
			if strings.Contains(menu, absent) {
				t.Fatalf("%q is offered in the branch menu: %s", absent, menu)
			}
		}
	}
}

// TestSVNUIDiffViewerOpensAChangedFile drives the path the user takes after
// opening a working copy: pick a changed file and read its diff.
func TestSVNUIDiffViewerOpensAChangedFile(t *testing.T) {
	f := newSVNFixture(t)
	f.start(1440, 900)
	f.open()

	f.write("README.md", "# Demo project\n\nEdited locally.\n")
	f.ws.Refresh(false)
	// Refresh is asynchronous and the previous status was already loaded, so
	// settle() alone returns before the new entries arrive.
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		rs := f.ws.Repo
		return rs != nil && len(rs.Status.Entries) > 0 && !f.ws.Busy() && !f.ws.Pending()
	}, 25*time.Second); err != nil {
		t.Fatalf("the local edit never reached the changes list: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	f.click("README.md")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return tab != nil && !tab.loading
	}, 25*time.Second); err != nil {
		t.Fatalf("diff never loaded: %v", err)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	joined := strings.Join(f.s.Texts(), " | ")
	// Both sides of the comparison must be readable: the base text and the
	// edited text.
	if !strings.Contains(joined, "Edited locally") {
		t.Fatalf("the working-copy side is missing from the diff: %s", joined)
	}
	if !strings.Contains(joined, "A working copy used to exercise Viff") {
		t.Fatalf("the base side is missing from the diff: %s", joined)
	}
	// A broken read surfaces as one of these instead of a diff.
	for _, broken := range []string{"Cannot show diff", "File not found", "W160013", "E195002"} {
		if strings.Contains(joined, broken) {
			t.Fatalf("the diff failed with %q: %s", broken, joined)
		}
	}
}

// TestSVNUICommitThroughTheWindow commits from the tool window: type a
// message, press Commit, and confirm the working copy is clean and the log
// shows the new revision.
func TestSVNUICommitThroughTheWindow(t *testing.T) {
	f := newSVNFixture(t)
	f.start(1440, 900)
	f.open()

	f.write("notes.txt", "a new file\n")
	runSVN(t, f.env, f.wc, f.svn.Path, "add", "notes.txt")
	f.ws.Refresh(false)
	// The refresh is asynchronous and the previous status was already loaded,
	// so the list can still be empty here. Committing before the scheduled file
	// appears would find the button disabled and silently do nothing.
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		rs := f.ws.Repo
		return rs != nil && len(rs.Status.Entries) >= 1 && !f.ws.Busy() && !f.ws.Pending()
	}, 25*time.Second); err != nil {
		t.Fatalf("the scheduled file never reached the list: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	before := f.ws.Repo.Log.Commits[0].Hash

	// Focus the message editor and type into it.
	if err := f.s.ClickLabel("Commit message"); err != nil {
		t.Fatalf("commit message field: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	if err := f.s.Type("Add notes file"); err != nil {
		t.Fatalf("type: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)

	// The Commit button carries the same label as the Commit tab; the existing
	// tests click it by that label and it resolves to the button, so this uses
	// the same path rather than a shortcut that needs a focused editor.
	f.click("Commit")

	if err := f.s.WaitFor(func(app.Snapshot) bool {
		rs := f.ws.Repo
		return rs != nil && !f.ws.Busy() && !f.ws.Pending() && len(rs.Status.Entries) == 0
	}, 30*time.Second); err != nil {
		t.Fatalf("commit did not leave a clean working copy: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	rs := f.ws.Repo
	if len(rs.Status.Entries) != 0 {
		t.Fatalf("entries after commit: %+v", rs.Status.Entries)
	}
	if rs.Log.Commits[0].Hash == before {
		t.Fatalf("the log still shows revision %s after committing", before)
	}
	if got := rs.Log.Commits[0].Subject; got != "Add notes file" {
		t.Fatalf("newest subject = %q", got)
	}
}

// TestSVNUIUpdateThroughTheWindow brings another copy's revision in through the
// window's Update action.
func TestSVNUIUpdateThroughTheWindow(t *testing.T) {
	f := newSVNFixture(t)
	f.start(1440, 900)
	f.open()

	// A second checkout commits a file the open copy does not have.
	parent := filepath.Dir(f.wc)
	other := filepath.Join(parent, "other")
	runSVN(t, f.env, parent, f.svn.Path, "checkout", f.url, other)
	if err := os.WriteFile(filepath.Join(other, "remote.txt"), []byte("from the other copy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runSVN(t, f.env, other, f.svn.Path, "add", "remote.txt")
	runSVN(t, f.env, other, f.svn.Path, "commit", "-m", "Add remote file")

	// The open copy asks for it. "Update Project" opens a dialog that picks
	// the integration mode; the update itself runs when the dialog is accepted,
	// so both steps belong to the path a user takes.
	if err := f.s.ClickLabel("Update Project"); err != nil {
		t.Fatalf("Update Project: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil
	}, 15*time.Second); err != nil {
		t.Fatalf("the update dialog did not open: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	before := f.ws.Repo.Log.Commits[0].Hash
	f.click("OK")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		rs := f.ws.Repo
		return rs != nil && !f.ws.Busy() && !f.ws.Pending() &&
			rs.Log.Loaded() && !rs.Log.Loading && len(rs.Log.Commits) > 0 &&
			rs.Log.Commits[0].Hash != before
	}, 30*time.Second); err != nil {
		t.Fatalf("update did not refresh the log: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	if _, err := os.Stat(filepath.Join(f.wc, "remote.txt")); err != nil {
		t.Fatalf("the other copy's file did not arrive: %v", err)
	}
	// The log must reflect the new revision, which is the stale-cache failure
	// the adapter's invalidation guards against.
	if got := f.ws.Repo.Log.Commits[0].Subject; got != "Add remote file" {
		t.Fatalf("newest subject after update = %q", got)
	}
}

// TestSVNUISpeaksChinese checks the translated interface on a Subversion
// working copy: the tabs and headings must be Chinese, and the Subversion-only
// settings row must be translated too.
func TestSVNUISpeaksChinese(t *testing.T) {
	f := newSVNFixture(t)
	f.ws.Settings.Language = string(i18n.ZHCN)
	// The window caches its translator; changing the setting alone does not
	// reach it, which is what the language switch in the settings page calls.
	f.win.applyLanguage()
	f.start(1440, 900)
	f.open()

	// A local edit and an unversioned file, so both group headings are drawn.
	f.write("README.md", "# Demo project\n\nEdited locally.\n")
	f.write("loose.txt", "not versioned\n")
	f.ws.Refresh(false)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		rs := f.ws.Repo
		return rs != nil && len(rs.Status.Entries) >= 2 && !f.ws.Busy() && !f.ws.Pending()
	}, 25*time.Second); err != nil {
		t.Fatalf("the local changes never reached the list: %v", err)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	joined := strings.Join(f.s.Texts(), " | ")
	for _, want := range []string{"提交", "变更", "未跟踪文件"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("%q is missing from the Chinese interface: %s", want, joined)
		}
	}
	// Git-only words must not leak into a Subversion window.
	for _, absent := range []string{"Staged", "Unversioned Files"} {
		if strings.Contains(joined, absent) {
			t.Fatalf("English %q leaked into the Chinese interface: %s", absent, joined)
		}
	}

	// The settings page names the Subversion executable in Chinese.
	f.win.openSettings()
	f.win.Invalidate()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil
	}, 20*time.Second); err != nil {
		t.Fatalf("settings dialog did not open: %v", err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	settingsText := strings.Join(f.s.Texts(), " | ")
	if !strings.Contains(settingsText, "Subversion 可执行文件") {
		t.Fatalf("the Subversion executable group is not translated: %s", settingsText)
	}
	if !strings.Contains(settingsText, "svn 路径") {
		t.Fatalf("the svn path label is not translated: %s", settingsText)
	}
}
