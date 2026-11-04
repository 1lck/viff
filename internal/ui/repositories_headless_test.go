//go:build vivahead

package viffui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	"viff/internal/storage"
)

// TestRepositoriesOverview covers the workspace overview from the welcome
// screen: two real repositories are added by typing their paths, their branches
// and changed files are shown, a root-level file that exists in both
// repositories is opened from each one and must show that repository's
// content, the active row is removed while the remaining row stays reachable,
// and a bad path reports an error without adding a row.
func TestRepositoriesOverview(t *testing.T) {
	f := newUIFixture(t)
	root := filepath.Dir(f.dir)
	repoB := filepath.Join(root, "repo-b")
	if err := os.MkdirAll(repoB, 0o755); err != nil {
		t.Fatal(err)
	}
	f.gitIn(repoB, "init", "-q", "-b", "feature/b")
	f.gitIn(repoB, "config", "user.name", "Ada Lovelace")
	f.gitIn(repoB, "config", "user.email", "ada@example.com")
	f.gitIn(repoB, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repoB, "lib.go"), []byte("package lib\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.gitIn(repoB, "add", "-A")
	f.gitIn(repoB, "commit", "-q", "-m", "Add lib")

	// The same root-level file name in both repositories, with different
	// content: opening one must never read the other repository's copy.
	writeFile(t, filepath.Join(f.dir, "shared.txt"), "shared from viff-demo\n")
	writeFile(t, filepath.Join(repoB, "shared.txt"), "shared from repo-b\n")

	f.start(1280, 800)
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	f.snap("repositories-welcome")

	// The entry exists on the welcome screen, before any repository is open.
	f.click("Repositories…")
	waitOverview(t, f)
	if !f.s.HasText("No repositories yet") {
		t.Fatalf("empty overview did not explain itself: %s", strings.Join(f.s.Texts(), " | "))
	}

	addRepositoryThroughDialog(t, f, f.dir, 1)
	addRepositoryThroughDialog(t, f, repoB, 2)
	waitOverviewLoaded(t, f, 2)

	texts := strings.Join(f.s.Texts(), "\n")
	for _, want := range []string{"viff-demo", "repo-b", "main", "feature/b"} {
		if !strings.Contains(texts, want) {
			t.Fatalf("overview is missing %q:\n%s", want, texts)
		}
	}
	if !strings.Contains(texts, "shared.txt") {
		t.Fatalf("overview does not list the selected repository's files:\n%s", texts)
	}
	// The workspace file lives beside the temporary settings file, never in
	// the repository or its .git directory.
	workspacePath := storage.WorkspacePath(f.ws.Settings.Path())
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("workspace list was not stored beside the settings: %v", err)
	}
	if strings.HasPrefix(workspacePath, f.dir+string(filepath.Separator)) ||
		strings.HasPrefix(workspacePath, repoB+string(filepath.Separator)) {
		t.Fatalf("workspace list was stored inside a repository: %s", workspacePath)
	}
	f.snap("repositories-overview")

	// Pick the demo repository and open its copy of the shared file.
	f.click("viff-demo")
	f.click("shared.txt")
	waitRepositoryFile(t, f, f.dir, "shared.txt", "shared from viff-demo")
	if got := filepath.Base(f.ws.ActiveRoot); got != "viff-demo" {
		t.Fatalf("active repository = %q, want viff-demo", got)
	}
	f.snap("repositories-file-from-a")

	// Now the other repository's copy: the switch must happen first and the
	// diff must be loaded from that repository.
	f.win.openRepositories()
	f.win.Invalidate()
	waitOverview(t, f)
	waitOverviewLoaded(t, f, 2)
	f.click("repo-b")
	f.click("shared.txt")
	waitRepositoryFile(t, f, repoB, "shared.txt", "shared from repo-b")
	if got := filepath.Base(f.ws.ActiveRoot); got != "repo-b" {
		t.Fatalf("active repository = %q, want repo-b", got)
	}
	f.snap("repositories-file-from-b")

	// Removing the active repository returns to the welcome screen but keeps
	// the other row, and the overview stays open on top of it.
	f.win.openRepositories()
	f.win.Invalidate()
	waitOverview(t, f)
	waitOverviewLoaded(t, f, 2)
	f.click("repo-b")
	f.click("Remove from Workspace")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return len(f.ws.Repositories) == 1 && f.ws.Repo == nil
	}, 10*time.Second); err != nil {
		t.Fatalf("removed active repository did not return to the welcome screen: %v", err)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	if f.win.dialog == nil || f.win.dialog.title != "Repositories" {
		t.Fatalf("overview closed after removing the active repository: %+v", f.win.dialog)
	}
	if got := f.ws.Repositories[0].Root; got != canonicalRoot(t, f.dir) {
		t.Fatalf("remaining repository = %q, want the demo repository", got)
	}
	if !f.s.HasText("1 repository in the workspace") {
		t.Fatalf("overview did not show the remaining row count: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("repositories-after-remove")

	// The remaining row can still be made active from the overview over the
	// welcome screen.
	f.click("viff-demo")
	f.click("Switch to Repository")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && f.ws.Repo.StatusLoaded && f.win.dialog == nil
	}, 20*time.Second); err != nil {
		t.Fatalf("could not switch to the remaining repository: %v", err)
	}
	f.settle()
	if got := canonicalRoot(t, f.ws.ActiveRoot); got != canonicalRoot(t, f.dir) {
		t.Fatalf("active repository = %q, want the demo repository", got)
	}

	// A path Git cannot discover is reported in the dialog, not as a row.
	f.win.openRepositories()
	f.win.Invalidate()
	waitOverview(t, f)
	f.click("Repository path")
	if err := f.s.Type(filepath.Join(root, "missing-repository")); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	f.click("Add")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return strings.Contains(strings.Join(f.s.Texts(), "\n"), "Cannot add repository")
	}, 10*time.Second); err != nil {
		t.Fatalf("bad path was not reported: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	if len(f.ws.Repositories) != 1 {
		t.Fatalf("failed add changed the workspace list: %+v", f.ws.Repositories)
	}
	if f.win.dialog == nil || f.win.dialog.title != "Repositories" {
		t.Fatal("failed add closed the overview")
	}
	f.snap("repositories-error")
}

// TestRepositoriesOverviewMenuAndScrolling opens the overview from the project
// menu, then proves that neither the repository list nor the changed-file list
// is cut at a dozen rows: the thirteenth repository is selected after
// scrolling, and a file far down its list is opened.
func TestRepositoriesOverviewMenuAndScrolling(t *testing.T) {
	f := newUIFixture(t)
	root := filepath.Dir(f.dir)
	var extra []string
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("repo-%02d", i)
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f.gitIn(dir, "init", "-q", "-b", "main")
		f.gitIn(dir, "config", "user.name", "Ada Lovelace")
		f.gitIn(dir, "config", "user.email", "ada@example.com")
		f.gitIn(dir, "config", "commit.gpgsign", "false")
		writeFile(t, filepath.Join(dir, "main.go"), "package main\n")
		f.gitIn(dir, "add", "-A")
		f.gitIn(dir, "commit", "-q", "-m", "Initial commit")
		// Twenty changed files: the file list needs its own scrolling too.
		for j := 0; j < 20; j++ {
			writeFile(t, filepath.Join(dir, fmt.Sprintf("long-%02d.txt", j)),
				fmt.Sprintf("long %d in %s\n", j, name))
		}
		extra = append(extra, dir)
	}

	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	for _, dir := range extra {
		dir := dir
		f.ws.AddRepository(dir, nil)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return len(f.ws.Repositories) == 13 && !f.ws.Pending()
	}, 30*time.Second); err != nil {
		t.Fatalf("repositories were not added: %v (have %d)", err, len(f.ws.Repositories))
	}

	// The project menu is the normal way in.
	f.click("Project: viff-demo")
	f.click("Repositories…")
	waitOverview(t, f)
	waitOverviewLoaded(t, f, 13)
	if !f.s.HasText("13 repositories in the workspace") {
		t.Fatalf("overview does not count all rows: %s", strings.Join(f.s.Texts(), " | "))
	}
	if _, ok := f.s.Hit("Refresh"); !ok {
		t.Fatalf("overview has no refresh action: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("repositories-menu-overview")

	// The thirteenth row starts below the list viewport; scroll to it.
	last := f.ws.Repositories[len(f.ws.Repositories)-1]
	lastName := filepath.Base(last.Root)
	if _, ok := f.s.Hit(lastName); ok {
		t.Fatalf("row %q is already visible: the list did not need scrolling", lastName)
	}
	scrollWithin(t, f, "viff-demo", 600)
	if _, ok := f.s.Hit(lastName); !ok {
		t.Fatalf("scrolling never revealed the last repository %q", lastName)
	}
	f.click(lastName)
	if got := f.win.dialog; got == nil || got.title != "Repositories" {
		t.Fatal("selecting a repository closed the overview")
	}
	f.snap("repositories-scrolled")

	// Its twenty changed files exceed the details viewport as well.
	if _, ok := f.s.Hit("long-19.txt"); ok {
		t.Fatalf("last file of %s is already visible: the file list did not need scrolling", lastName)
	}
	scrollWithin(t, f, "long-00.txt", 500)
	if _, ok := f.s.Hit("long-19.txt"); !ok {
		t.Fatalf("scrolling never revealed the last changed file of %s", lastName)
	}
	f.snap("repositories-long-files")
	f.click("long-19.txt")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		if f.ws.Repo == nil || !f.ws.Repo.StatusLoaded || f.ws.ActiveRoot == "" {
			return false
		}
		if filepath.Base(f.ws.ActiveRoot) != lastName {
			return false
		}
		tab := f.win.ed.cur()
		return tab != nil && !tab.loading && tab.res != nil && tab.src.Path == "long-19.txt"
	}, 20*time.Second); err != nil {
		t.Fatalf("file from the last repository did not open: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	tab := f.win.ed.cur()
	body := strings.Join(tab.res.New.Lines, "\n")
	if !strings.Contains(body, "long 19 in "+lastName) {
		t.Fatalf("diff shows the wrong content for %s:\n%s", lastName, body)
	}
	f.snap("repositories-file-open")
}

// ---- helpers ---------------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func canonicalRoot(t *testing.T, path string) string {
	t.Helper()
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(abs)
}

func waitOverview(t *testing.T, f *uiFixture) {
	t.Helper()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && f.win.dialog.title == "Repositories"
	}, 10*time.Second); err != nil {
		t.Fatalf("Repositories dialog never opened: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
}

func waitOverviewLoaded(t *testing.T, f *uiFixture, want int) {
	t.Helper()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		if len(f.ws.Repositories) < want || f.ws.Pending() {
			return false
		}
		for _, s := range f.ws.Repositories {
			if !s.Loaded && s.Err == nil {
				return false
			}
		}
		return true
	}, 30*time.Second); err != nil {
		t.Fatalf("repositories never reached a loaded status (%d wanted): %v\nrows: %+v", want, err, f.ws.Repositories)
	}
	f.s.Settle(80*time.Millisecond, 3*time.Second)
}

func addRepositoryThroughDialog(t *testing.T, f *uiFixture, dir string, want int) {
	t.Helper()
	f.click("Repository path")
	if err := f.s.Type(dir); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(50*time.Millisecond, time.Second)
	f.click("Add")
	waitOverviewLoaded(t, f, want)
}

func waitRepositoryFile(t *testing.T, f *uiFixture, root, path, content string) {
	t.Helper()
	want := canonicalRoot(t, root)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		if f.ws.Repo == nil || !f.ws.Repo.StatusLoaded {
			return false
		}
		if canonicalRootNoFatal(f.ws.ActiveRoot) != want {
			return false
		}
		tab := f.win.ed.cur()
		if tab == nil || tab.loading || tab.res == nil || tab.src.Path != path {
			return false
		}
		return strings.Contains(strings.Join(tab.res.New.Lines, "\n"), content)
	}, 20*time.Second); err != nil {
		tab := f.win.ed.cur()
		got := ""
		if tab != nil && tab.res != nil {
			got = strings.Join(tab.res.New.Lines, "\n")
		}
		t.Fatalf("%s did not open from %s (%v)\nactive: %q content: %q\ntexts: %s",
			path, filepath.Base(root), err, f.ws.ActiveRoot, got, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
}

func canonicalRootNoFatal(path string) string {
	if path == "" {
		return ""
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

// scrollWithin scrolls the pane that contains anchor. Container hits are not
// reachable at their centre (a row covers it), so a reachable row is used as
// the anchor point instead.
func scrollWithin(t *testing.T, f *uiFixture, anchor string, dy float32) {
	t.Helper()
	hit, ok := f.s.Hit(anchor)
	if !ok {
		t.Fatalf("no reachable anchor %q to scroll from\ntexts: %s", anchor, strings.Join(f.s.Texts(), " | "))
	}
	if err := f.s.Scroll(ui.ScrollEvent{
		X:      hit.Bounds.X + hit.Bounds.Width/2,
		Y:      hit.Bounds.Y + hit.Bounds.Height/2,
		DeltaY: dy, Precise: true,
	}); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
}

// TestRepositoriesOverviewFitsSmallWindow checks the default 900x600 window:
// the responsive body must leave the dialog's button row reachable instead of
// pushing it below the window.
func TestRepositoriesOverviewFitsSmallWindow(t *testing.T) {
	f := newUIFixture(t)
	f.start(900, 600)
	f.win.openRepo(f.dir)
	f.settle()
	f.win.openRepositories()
	f.win.Invalidate()
	waitOverview(t, f)
	waitOverviewLoaded(t, f, 1)
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	for _, label := range []string{"Switch to Repository", "Add", "Remove from Workspace", "Changed files"} {
		hit, ok := f.s.Hit(label)
		if !ok {
			t.Fatalf("%q is not reachable in a 900x600 window: %s", label, strings.Join(f.s.Texts(), " | "))
		}
		b := hit.Bounds
		if b.X < 0 || b.Y < 0 || b.X+b.Width > 900.5 || b.Y+b.Height > 600.5 {
			t.Fatalf("%q at %+v does not fit the 900x600 window", label, b)
		}
	}
	f.snap("repositories-900x600")
}
