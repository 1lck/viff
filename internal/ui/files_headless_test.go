//go:build vivahead

package viffui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	"viff/internal/i18n"
)

// TestProjectPanel opens the Project tool window, filters it, opens a file
// read-only and reaches the same viewer through Quick Open.
func TestProjectPanel(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	if err := f.s.Keys("cmd+1"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.projectOpen && f.win.fm.loaded }, 10*time.Second); err != nil {
		t.Fatalf("Project panel did not load the working tree: %v", err)
	}
	if f.win.commitOpen {
		t.Fatal("Commit panel stayed open beside the Project panel")
	}
	if !hasRow(f.win.fm.rows, projectRootKey) {
		t.Fatalf("repository root row missing: %+v", f.win.fm.rows)
	}
	if f.win.fm.sel != projectRootKey {
		t.Fatalf("root row not selected: %q", f.win.fm.sel)
	}
	// Folding the root hides the whole tree; unfolding brings it back.
	f.win.clickFileRow(0)
	if hasRow(f.win.fm.rows, "README.md") {
		t.Fatal("folded root still shows files")
	}
	f.win.clickFileRow(0)
	if !hasRow(f.win.fm.rows, "README.md") {
		t.Fatal("unfolded root does not show files")
	}
	for _, p := range []string{"README.md", "config.json", "internal/store/store.go", "notes/todo.txt"} {
		if !hasFilePath(f.win.fm.entries, p) {
			t.Fatalf("%s missing from the file list: %v", p, f.win.fm.entries)
		}
	}
	if !hasRow(f.win.fm.rows, "internal/") || !hasRow(f.win.fm.rows, "README.md") {
		t.Fatalf("tree rows missing directories or root files: %+v", f.win.fm.rows)
	}
	f.snap("50-project-panel")
	f.win.ws.Settings.Theme = "light"
	f.win.applyTheme()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, time.Second)
	f.snap("52-project-panel-light")
	f.win.ws.Settings.Theme = "dark"
	f.win.applyTheme()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, time.Second)

	// Folding a directory hides its subtree without dropping the paths.
	internal := f.win.fm.rowIndex("internal/")
	if internal < 0 {
		t.Fatal("internal/ row missing")
	}
	f.win.clickFileRow(internal)
	if f.win.fm.rowIndex("internal/store/store.go") >= 0 {
		t.Fatal("folded directory still shows its files")
	}
	f.win.clickFileRow(f.win.fm.rowIndex("internal/"))
	if f.win.fm.rowIndex("internal/store/store.go") < 0 {
		t.Fatal("expanded directory does not show its files")
	}

	// A filter narrows the tree; Enter opens the first match read-only.
	f.click("Filter files")
	if err := f.s.Type("config"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		for _, row := range f.win.fm.rows {
			if row.path == "config.json" {
				return true
			}
		}
		return false
	}, 5*time.Second); err != nil {
		t.Fatalf("filter did not narrow the tree: %+v", f.win.fm.rows)
	}
	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return tab != nil && !tab.loading && tab.src.View && tab.src.Path == "config.json" && tab.src.Commit == ""
	}, 10*time.Second); err != nil {
		t.Fatalf("Enter did not open config.json: %v", err)
	}
	f.settleTab()
	tab := f.win.ed.cur()
	if tab.res == nil || !tab.res.New.Text {
		t.Fatalf("config.json was not shown as text: %+v", tab.res)
	}
	if !strings.Contains(strings.Join(tab.res.New.Lines, "\n"), `"port": 9090`) {
		t.Fatalf("viewer shows the wrong content: %q", strings.Join(tab.res.New.Lines, "\n"))
	}

	// Quick Open uses the same list and viewer, and the shortcut reaches it
	// while a text field (the Project filter) still has focus.
	if err := f.s.Keys("cmd+shift+o"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 5*time.Second); err != nil {
		t.Fatal("Quick Open did not open")
	}
	f.s.Settle(120*time.Millisecond, time.Second)
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.fm.loaded }, 5*time.Second); err != nil {
		t.Fatalf("Quick Open list did not load: %v", err)
	}
	if err := f.s.Type("store.go"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(80*time.Millisecond, time.Second)
	f.snap("51-quick-open")
	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return tab != nil && !tab.loading && tab.src.View && strings.HasSuffix(tab.src.Path, "store.go")
	}, 10*time.Second); err != nil {
		t.Fatalf("Quick Open did not open store.go: %v", err)
	}
	f.settleTab()
	if tab := f.win.ed.cur(); !hasSyntaxSpans(tab) {
		t.Fatalf("the file view has no syntax spans: %+v", tab.newSyn)
	}
	f.snap("53-project-file-highlighted")

	// The left dock switches between Project and Commit, one at a time.
	if err := f.s.Keys("cmd+0"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.commitOpen && !f.win.projectOpen
	}, 5*time.Second); err != nil {
		t.Fatal("Command+0 did not switch back to the Commit panel")
	}
	if err := f.s.Keys("cmd+1"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.projectOpen && !f.win.commitOpen
	}, 5*time.Second); err != nil {
		t.Fatal("Command+1 did not switch back to the Project panel")
	}
}

// TestProjectPanelGroupsWorkspaceRepositories covers Phase A of the
// multi-repository project view: the Project panel groups the explicit
// workspace list under one row, keeps the active repository's file tree, and
// makes a non-active repository active when its row is clicked.
func TestProjectPanelGroupsWorkspaceRepositories(t *testing.T) {
	f := newUIFixture(t)
	repoB := filepath.Join(filepath.Dir(f.dir), "repo-b")
	makeDiscoverableRepo(t, f, repoB)
	// A distinct branch makes the cached summary of the non-active row
	// observable without opening the Repositories dialog.
	f.gitIn(repoB, "checkout", "-q", "-b", "feature/b")

	f.start(1440, 900)
	repoARoot := canonicalRootNoFatal(f.dir)
	repoBRoot := canonicalRootNoFatal(repoB)
	waitActive := func(root string) {
		t.Helper()
		if err := f.s.WaitFor(func(app.Snapshot) bool {
			return f.ws.Repo != nil && f.ws.Repo.StatusLoaded && !f.ws.Pending() &&
				canonicalRootNoFatal(f.ws.ActiveRoot) == root
		}, 20*time.Second); err != nil {
			t.Fatalf("repository %s did not become active: %v\nactive: %q", root, err, f.ws.ActiveRoot)
		}
		f.settle()
	}
	// Open the second repository first, then the demo repository: both
	// statuses are loaded and the demo repository is active.
	f.win.openRepo(repoB)
	waitActive(repoBRoot)
	f.win.openRepo(f.dir)
	waitActive(repoARoot)

	if err := f.s.Keys("cmd+1"); err != nil {
		t.Fatal(err)
	}
	v := &f.win.fm
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.projectOpen && v.loaded && v.grouped() && len(v.repoRoots) == 2 &&
			v.activeRoot == repoARoot
	}, 10*time.Second); err != nil {
		t.Fatalf("Project panel did not group the workspace: %v\nrows: %+v", err, v.rows)
	}
	if !hasRow(v.rows, projectWorkspaceKey) {
		t.Fatalf("workspace row missing: %+v", v.rows)
	}
	if !hasRow(v.rows, projectRootKey) {
		t.Fatalf("active repository row missing: %+v", v.rows)
	}
	nonActiveKey := repoRowKey(repoBRoot)
	if !hasRow(v.rows, nonActiveKey) {
		t.Fatalf("non-active repository row missing: %+v", v.rows)
	}
	if !hasRow(v.rows, "internal/store/store.go") {
		t.Fatalf("active repository tree missing after grouping: %+v", v.rows)
	}

	// A non-active row is a leaf: its summary is shown, but no files are
	// loaded for it.
	i := v.rowIndex(nonActiveKey)
	if i < 0 || v.rows[i].root {
		t.Fatalf("non-active row is not a leaf: %+v", v.rows[i])
	}
	if hint, _ := f.win.projectRepositoryHint(v.rows[i].repoRoot, false); !strings.Contains(hint, "feature/b") {
		t.Fatalf("cached summary of %s = %q, want its branch", repoBRoot, hint)
	}
	for k := i + 1; k < len(v.rows) && v.rows[k].depth > v.rows[i].depth; k++ {
		t.Fatalf("non-active repository has file rows: %+v", v.rows[k])
	}
	if hint, _ := f.win.projectRepositoryHint(v.rows[v.rowIndex(projectRootKey)].repoRoot, true); !strings.Contains(hint, "main") {
		t.Fatalf("active repository summary = %q, want its branch", hint)
	}
	f.snap("54-project-workspace-repositories")

	// Clicking the non-active row goes through the normal switch path. The
	// active repository changes, the panel stays open, and the file tree now
	// belongs to the repository that was clicked.
	if err := f.s.ClickLabel("repo-b"); err != nil {
		t.Fatalf("click repo-b: %v\nrows: %+v", err, v.rows)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		if f.ws.Repo == nil || !f.ws.Repo.StatusLoaded || f.ws.Pending() {
			return false
		}
		return canonicalRootNoFatal(f.ws.ActiveRoot) == repoBRoot
	}, 20*time.Second); err != nil {
		t.Fatalf("clicking the non-active row did not switch: %v\nactive: %q", err, f.ws.ActiveRoot)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return v.loaded && v.activeRoot == repoBRoot
	}, 10*time.Second); err != nil {
		t.Fatalf("Project panel did not reload the clicked repository: %v\nactive: %q", err, v.activeRoot)
	}
	f.settle()
	if !f.win.projectOpen {
		t.Fatal("clicking a repository row closed the Project panel")
	}
	if !hasRow(v.rows, projectWorkspaceKey) || !hasRow(v.rows, repoRowKey(repoARoot)) {
		t.Fatalf("both repositories are not still listed: %+v", v.rows)
	}
	if got := v.rows[v.rowIndex(projectRootKey)].repoRoot; got != repoBRoot {
		t.Fatalf("active row root = %q, want %q", got, repoBRoot)
	}
	if !hasRow(v.rows, "README.md") {
		t.Fatalf("clicked repository's file tree missing: %+v", v.rows)
	}
	if hasRow(v.rows, "internal/store/store.go") {
		t.Fatalf("previous repository's file tree is still shown: %+v", v.rows)
	}
	if hint, _ := f.win.projectRepositoryHint(repoBRoot, true); !strings.Contains(hint, "feature/b") {
		t.Fatalf("active repository summary = %q, want its branch", hint)
	}
	if hint, _ := f.win.projectRepositoryHint(repoARoot, false); !strings.Contains(hint, "main") {
		t.Fatalf("previous repository lost its summary: %q", hint)
	}
	f.snap("55-project-workspace-switched")

	// Folding the active repository hides its tree but not the workspace or
	// the other repository row.
	active := v.rowIndex(projectRootKey)
	f.win.clickFileRow(active)
	if hasRow(v.rows, "README.md") {
		t.Fatalf("folding the active repository kept its files: %+v", v.rows)
	}
	if !hasRow(v.rows, repoRowKey(repoARoot)) {
		t.Fatalf("folding the active repository hid another repository: %+v", v.rows)
	}
	f.win.clickFileRow(v.rowIndex(projectRootKey))
	if !hasRow(v.rows, "README.md") {
		t.Fatalf("unfolding the active repository did not restore its files: %+v", v.rows)
	}
}

// TestProjectFilterKeepsFoldsWorking covers the Project panel after a filter
// is entered and submitted: its matches start expanded, and folding still
// works from there — a directory folds and unfolds, the repository root folds
// the whole tree, and a new filter reopens the tree so its matches stay
// visible instead of disappearing into a folded directory.
func TestProjectFilterKeepsFoldsWorking(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	if err := f.s.Keys("cmd+1"); err != nil {
		t.Fatal(err)
	}
	v := &f.win.fm
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.projectOpen && v.loaded
	}, 10*time.Second); err != nil {
		t.Fatalf("Project panel did not load the working tree: %v", err)
	}

	f.click("Filter files")
	if err := f.s.Type("internal"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return hasRow(v.rows, "internal/store/store.go")
	}, 5*time.Second); err != nil {
		t.Fatalf("filter did not expand its matches: %+v", v.rows)
	}
	// Enter submits the filter and opens the first match; the rest stay shown.
	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return hasRow(v.rows, "internal/store/store.go")
	}, 5*time.Second); err != nil {
		t.Fatalf("submitted filter dropped its matches: %+v", v.rows)
	}
	f.settleTab()

	// A directory folds and unfolds while the filter is applied.
	if dir := v.rowIndex("internal/"); dir < 0 {
		t.Fatalf("internal/ row missing: %+v", v.rows)
	} else {
		f.win.clickFileRow(dir)
	}
	if v.rowIndex("internal/store/store.go") >= 0 {
		t.Fatalf("clicking a filtered directory did not fold it: %+v", v.rows)
	}
	f.win.clickFileRow(v.rowIndex("internal/"))
	if v.rowIndex("internal/store/store.go") < 0 {
		t.Fatalf("clicking a filtered directory again did not unfold it: %+v", v.rows)
	}

	// The repository root still folds the tree it filtered.
	f.win.clickFileRow(0)
	if hasRow(v.rows, "internal/") {
		t.Fatalf("clicking the root did not fold the filtered tree: %+v", v.rows)
	}
	f.win.clickFileRow(0)
	if !hasRow(v.rows, "internal/") {
		t.Fatalf("clicking the root again did not unfold the filtered tree: %+v", v.rows)
	}

	// A different filter reopens the tree; a folded directory must not hide
	// the matches the new filter is looking for.
	f.win.clickFileRow(v.rowIndex("internal/"))
	for v.filter.RuneCount() > 0 {
		v.filter.DeleteBackward()
	}
	v.filter.Paste("store")
	f.win.Invalidate()
	if err := f.s.WaitFor(func(app.Snapshot) bool { return v.query == "store" }, 5*time.Second); err != nil {
		t.Fatalf("the new filter was not applied: %q", v.query)
	}
	if v.rowIndex("internal/store/store.go") < 0 {
		t.Fatalf("the new filter left its match folded away: %+v", v.rows)
	}
}

// TestFilesMenuOffersReadOnlyActions checks the context menu stays inside the
// read-only contract: open, annotate, history and external tools.
func TestFilesMenuOffersReadOnlyActions(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	if err := f.s.Keys("cmd+1"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.projectOpen && f.win.fm.loaded }, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	menu := f.win.filesMenu(fileRow{path: "README.md", name: "README.md"})
	var titles []string
	for _, it := range menu {
		if it.title != "" {
			titles = append(titles, it.title)
		}
	}
	for _, want := range []string{"Open", "Open in External Editor", "Annotate with Git Blame", "Show History", "Copy Path"} {
		found := false
		for _, got := range titles {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q missing from %v", want, titles)
		}
	}
}

// The English default cannot show whether a label goes through the message
// table: a hardcoded string reads exactly the same. Chinese is the case that
// fails, so this is the regression test for the panel's own menus.
func TestProjectPanelSpeaksTheChosenLanguage(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	if err := f.s.Keys("cmd+1"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.projectOpen && f.win.fm.loaded }, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	f.ws.Settings.Language = "zh-CN"
	f.win.applyLanguage()

	assertMenuTitles(t, "file menu", f.win.filesMenu(fileRow{path: "README.md", name: "README.md"}), []string{
		"打开", "用外部编辑器打开", "用 Git blame 注解", "显示历史",
		"复制路径", "复制相对路径",
		// The reveal action names the platform's file manager, so the expectation
		// comes from the table rather than a literal that only fits macOS.
		f.win.t(i18n.WorkspaceMenuRevealInFinder),
	})
	assertMenuTitles(t, "tool window menu", f.win.projectWindowMenu(), []string{
		"打开文件…", "在文件中查找…", "全部展开", "全部折叠", "隐藏项目",
	})
}

// assertMenuTitles fails when one of want is missing from the menu's titles, so
// a label that kept its English text shows up as the missing one.
func assertMenuTitles(t *testing.T, what string, menu []item, want []string) {
	t.Helper()
	var titles []string
	for _, it := range menu {
		if it.title != "" {
			titles = append(titles, it.title)
		}
	}
	for _, w := range want {
		found := false
		for _, got := range titles {
			if got == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s: %q missing from %v", what, w, titles)
		}
	}
}

func hasFilePath(list []string, p string) bool {
	for _, x := range list {
		if x == p {
			return true
		}
	}
	return false
}

// hasSyntaxSpans reports whether the highlighted file view found at least one
// keyword or string, which is what makes "store.go opened from Project" show
// code colours rather than plain text.
func hasSyntaxSpans(t *diffTab) bool {
	for _, spans := range t.newSyn {
		for _, sp := range spans {
			if sp.kind == tkKeyword || sp.kind == tkString {
				return true
			}
		}
	}
	return false
}

func hasRow(rows []fileRow, key string) bool {
	for _, r := range rows {
		if r.key == key {
			return true
		}
	}
	return false
}

// TestProjectPanelRefreshesAfterExternalChange checks that a file created outside
// Viff shows up after the status snapshot moves, without reopening the tab.
func TestProjectPanelRefreshesAfterExternalChange(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	if err := f.s.Keys("cmd+1"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.projectOpen && f.win.fm.loaded }, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	f.write("notes/from-ai.txt", "changed outside Viff\n")
	f.win.ws.Refresh(true)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return hasFilePath(f.win.fm.entries, "notes/from-ai.txt")
	}, 10*time.Second); err != nil {
		t.Fatalf("new file missing after refresh: %v", f.win.fm.entries)
	}
}
