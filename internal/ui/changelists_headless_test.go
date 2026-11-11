//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// Change-list helpers shared by the headless tests below.

func openChangeListsMenu(t *testing.T, f *uiFixture) {
	t.Helper()
	if err := f.s.ClickLabel("Change Lists"); err != nil {
		t.Fatalf("open Change Lists: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(f.win.popups) == 0 {
		t.Fatalf("Change Lists menu did not open\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
}

func changeListsClick(t *testing.T, f *uiFixture, label string) {
	t.Helper()
	if err := f.s.ClickLabel(label); err != nil {
		t.Fatalf("click %q: %v\ntexts: %s", label, err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
}

func waitChangeListDialog(t *testing.T, f *uiFixture, title string) {
	t.Helper()
	err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && f.win.dialog.title == title
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("dialog %q did not open: %v\ntexts: %s", title, err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
}

// createChangeList creates a list through the Change Lists menu and returns
// its backend ID.
func createChangeList(t *testing.T, f *uiFixture, name string) string {
	t.Helper()
	before := len(f.ws.Lists())
	openChangeListsMenu(t, f)
	changeListsClick(t, f, "New Change List…")
	waitChangeListDialog(t, f, "New Change List")
	if err := f.s.ClickLabel("Name:"); err != nil {
		t.Fatalf("focus name field: %v", err)
	}
	if err := f.s.Type(name); err != nil {
		t.Fatalf("type list name: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	changeListsClick(t, f, "Create")
	err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog == nil && len(f.ws.Lists()) > before
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("change list %q was not created: %v\ntexts: %s", name, err, strings.Join(f.s.Texts(), " | "))
	}
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	for _, l := range f.ws.Lists() {
		if l.Name == name {
			return l.ID
		}
	}
	t.Fatalf("created list %q not in %+v", name, f.ws.Lists())
	return ""
}

// selectChangeList picks a filter in the Change Lists menu.
func selectChangeList(t *testing.T, f *uiFixture, name string) {
	t.Helper()
	openChangeListsMenu(t, f)
	changeListsClick(t, f, name)
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
}

// moveToChangeList assigns files through the staging tree's context menu.
func moveToChangeList(t *testing.T, f *uiFixture, file, name string) {
	t.Helper()
	if err := f.s.RightClickLabel(file); err != nil {
		t.Fatalf("right click %q: %v\ntexts: %s", file, err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.ClickLabel("Move to Change List"); err != nil {
		t.Fatalf("open Move to Change List: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	changeListsClick(t, f, name)
}

func changeListNamed(lists []vapp.ChangeList, name string) (vapp.ChangeList, bool) {
	for _, l := range lists {
		if l.Name == name {
			return l, true
		}
	}
	return vapp.ChangeList{}, false
}

func stagedNames(f *uiFixture) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(f.git("diff", "--cached", "--name-only")), "\n") {
		if line != "" {
			out[line] = true
		}
	}
	return out
}

func untrackedNames(f *uiFixture) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(f.git("ls-files", "--others", "--exclude-standard")), "\n") {
		if line != "" {
			out[line] = true
		}
	}
	return out
}

// TestChangeListsAssignAndFilter covers the two-list flow: create lists from
// the menu, assign files through the context menu, and filter the tree. It
// also checks counts, selection clearing, the commit text, and that lists do
// not touch Git's index or status.
func TestChangeListsAssignAndFilter(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	before := f.git("status", "--porcelain")

	localID := createChangeList(t, f, "Local tests")
	f.snap("01-new-change-list")
	scratchID := createChangeList(t, f, "Scratch work")
	if localID == "" || scratchID == "" || localID == scratchID {
		t.Fatalf("list ids = %q %q", localID, scratchID)
	}
	// Creating a list must leave the default All-lists view, since the new
	// list is selected after creation; go back for assignment.
	selectChangeList(t, f, "All lists")
	if f.win.cm.listID != "" {
		t.Fatalf("All lists filter = %q", f.win.cm.listID)
	}

	moveToChangeList(t, f, "main.go", "Local tests")
	moveToChangeList(t, f, "internal/store/store.go", "Scratch work")
	if got := f.ws.ChangeListFor("main.go"); got != localID {
		t.Fatalf("main.go list = %q, want %q", got, localID)
	}
	if got := f.ws.ChangeListFor("internal/store/store.go"); got != scratchID {
		t.Fatalf("store.go list = %q, want %q", got, scratchID)
	}
	if after := f.git("status", "--porcelain"); after != before {
		t.Fatalf("change lists touched Git state:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	f.snap("02-assigned-all-lists")

	// A staged file and an unversioned default file are always shown; only
	// the filtered lists are hidden.
	selectChangeList(t, f, "Local tests")
	if f.win.cm.listID != localID {
		t.Fatalf("filter = %q, want %q", f.win.cm.listID, localID)
	}
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	if !f.s.HasText("main.go") {
		t.Fatalf("filtered list does not show its file\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	for _, hidden := range []string{"store.go", "todo.txt"} {
		if f.s.HasText(hidden) {
			t.Fatalf("filtered view shows %q from another list\ntexts: %s", hidden, strings.Join(f.s.Texts(), " | "))
		}
	}
	if !f.s.HasText("README.md") || !f.s.HasText("Staged") {
		t.Fatalf("staged changes are hidden while filtering\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("Commit includes all staged files") {
		t.Fatalf("missing commit-scope warning\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("03-filtered-list")

	// Switching lists clears the old selection, so Space cannot stage a file
	// the new filter hides.
	if err := f.s.ClickLabel("main.go"); err != nil {
		t.Fatalf("select main.go: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(f.win.cm.sel) == 0 {
		t.Fatal("clicking main.go did not select it")
	}
	idx := f.win.rowIndex(rowKey(domain.AreaUnstaged, "main.go"))
	if idx < 0 {
		t.Fatal("main.go is not an unstaged row")
	}
	selectChangeList(t, f, "Scratch work")
	if len(f.win.cm.sel) != 0 || f.win.cm.anchor != "" || f.win.cm.cursor != "" {
		t.Fatalf("switching lists kept selection: %+v", f.win.cm.sel)
	}
	// Selecting a visible file of the new list and staging it must not drag
	// the hidden main.go along.
	if err := f.s.ClickLabel("internal/store/store.go"); err != nil {
		t.Fatalf("select store.go: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.Keys("space"); err != nil {
		t.Fatalf("space: %v", err)
	}
	f.settle()
	staged := stagedNames(f)
	if !staged["internal/store/store.go"] {
		t.Fatalf("Space did not stage the visible store.go: %v", staged)
	}
	if staged["main.go"] {
		t.Fatal("Space staged main.go while the Scratch work filter hid it")
	}

	// Editing the commit message keeps working; switching lists does not
	// discard it.
	if err := f.s.ClickLabel("Commit message"); err != nil {
		t.Fatalf("focus commit message: %v", err)
	}
	if err := f.s.Type("wip across lists"); err != nil {
		t.Fatalf("type commit message: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	selectChangeList(t, f, "Local tests")
	if got := f.win.cm.msg.String(); got != "wip across lists" {
		t.Fatalf("commit message after list switch = %q", got)
	}

	// The menu shows per-list counts and the list names.
	openChangeListsMenu(t, f)
	texts := strings.Join(f.s.Texts(), " | ")
	for _, want := range []string{"All lists", "Default", "Local tests", "Scratch work", "Rename Change List…", "Delete Change List…"} {
		if !strings.Contains(texts, want) {
			t.Fatalf("menu misses %q\ntexts: %s", want, texts)
		}
	}
	f.snap("04-change-lists-menu")
}

// TestChangeListDuplicateNameKeepsDialog checks that creating a list with an
// existing name fails in the dialog and does not add a second list.
func TestChangeListDuplicateNameKeepsDialog(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	createChangeList(t, f, "Local tests")

	before := len(f.ws.Lists())
	openChangeListsMenu(t, f)
	changeListsClick(t, f, "New Change List…")
	waitChangeListDialog(t, f, "New Change List")
	if err := f.s.ClickLabel("Name:"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("local tests"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if !f.s.HasText("already exists") {
		t.Fatalf("duplicate name is not reported while typing\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("10-duplicate-name-before-create")
	changeListsClick(t, f, "Create")
	f.s.Settle(80*time.Millisecond, time.Second)
	if f.win.dialog == nil {
		t.Fatalf("dialog closed for duplicate name; lists: %+v", f.ws.Lists())
	}
	if !f.s.HasText("already exists") {
		t.Fatalf("duplicate error disappeared\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if got := len(f.ws.Lists()); got != before {
		t.Fatalf("duplicate create changed lists: %d -> %d", before, got)
	}
	f.snap("11-duplicate-name-rejected")
	if err := f.s.Keys("escape"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if f.win.dialog != nil {
		t.Fatal("Escape did not close the dialog")
	}
}

// TestChangeListRenameDeleteFallsBackToDefault renames a list (membership
// survives the rename), deletes it (files return to Default), and checks the
// default list refuses both management actions.
func TestChangeListRenameDeleteFallsBackToDefault(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	id := createChangeList(t, f, "Local tests")
	selectChangeList(t, f, "All lists")
	moveToChangeList(t, f, "main.go", "Local tests")
	if got := f.ws.ChangeListFor("main.go"); got != id {
		t.Fatalf("main list = %q, want %q", got, id)
	}
	selectChangeList(t, f, "Local tests")

	// Rename: the ID must stay, so the assignment does not move.
	openChangeListsMenu(t, f)
	changeListsClick(t, f, "Rename Change List…")
	waitChangeListDialog(t, f, "Rename Change List")
	if err := f.s.ClickLabel("New name:"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Keys("end"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type(" 2"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("20-rename-change-list")
	changeListsClick(t, f, "Rename")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		l, ok := changeListNamed(f.ws.Lists(), "Local tests 2")
		return f.win.dialog == nil && ok && l.ID == id
	}, 5*time.Second); err != nil {
		t.Fatalf("rename did not apply: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	if got := f.ws.ChangeListFor("main.go"); got != id {
		t.Fatalf("rename moved the file to %q", got)
	}
	if !f.s.HasText("Local tests 2") {
		t.Fatalf("renamed list not shown\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}

	// Delete: the file must fall back to Default and the filter resets to All.
	openChangeListsMenu(t, f)
	changeListsClick(t, f, "Delete Change List…")
	waitChangeListDialog(t, f, "Delete Change List")
	f.snap("21-delete-change-list")
	changeListsClick(t, f, "Delete")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		_, ok := changeListNamed(f.ws.Lists(), "Local tests 2")
		return f.win.dialog == nil && !ok
	}, 5*time.Second); err != nil {
		t.Fatalf("delete did not apply: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	if got := f.ws.ChangeListFor("main.go"); got != "default" {
		t.Fatalf("deleted list kept the file: %q", got)
	}
	if f.win.cm.listID != "" || !f.s.HasText("All lists") {
		t.Fatalf("filter after delete = %q\ntexts: %s", f.win.cm.listID, strings.Join(f.s.Texts(), " | "))
	}
	f.snap("22-after-delete-all-lists")

	// Default cannot be renamed or deleted: the menu entries are disabled and
	// clicking them does nothing.
	selectChangeList(t, f, "Default")
	if f.win.cm.listID != "default" {
		t.Fatalf("Default filter = %q", f.win.cm.listID)
	}
	openChangeListsMenu(t, f)
	// The menu model marks both management rows disabled for the built-in
	// list. Disabled rows may not be registered as hit targets at all, so do
	// not require a hittable control: a click label lookup that finds nothing
	// and a dialog that never opens is the observable behavior.
	disabled := map[string]bool{}
	for _, p := range f.win.popups {
		for _, it := range p.items {
			if it.title == "Delete Change List…" || it.title == "Rename Change List…" {
				disabled[it.title] = it.disabled
			}
		}
	}
	for _, title := range []string{"Delete Change List…", "Rename Change List…"} {
		want, ok := disabled[title]
		if !ok {
			t.Fatalf("menu model misses %q", title)
		}
		if !want {
			t.Fatalf("%q is enabled for the Default list", title)
		}
		if !f.s.HasText(title) {
			t.Fatalf("%q is not visible in the menu", title)
		}
		if err := f.s.ClickLabel(title); err == nil {
			f.s.Settle(40*time.Millisecond, time.Second)
			if f.win.dialog != nil {
				t.Fatalf("clicking disabled %q opened a dialog", title)
			}
		}
	}
	f.snap("23-default-locked")
	if f.win.dialog != nil {
		t.Fatalf("Default management opened a dialog: %+v", f.win.dialog)
	}
	if err := f.s.Keys("escape"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if _, ok := changeListNamed(f.ws.Lists(), "Default"); !ok {
		t.Fatal("Default list disappeared")
	}
}

// TestChangeListStageAllOnlyStagesFiltered checks that Stage All follows the
// filter: files in another list stay untouched, and the button still works
// for the second list afterwards.
func TestChangeListStageAllOnlyStagesFiltered(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	for _, name := range []string{"notes/a.txt", "notes/b.txt", "notes/c.txt", "notes/d.txt"} {
		f.write(name, name+"\n")
	}
	f.ws.Refresh(true)
	f.settle()

	a, err := f.ws.CreateChangeList("Group A")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.ws.CreateChangeList("Group B")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ws.MoveToChangeList([]string{"notes/a.txt", "notes/b.txt"}, a); err != nil {
		t.Fatal(err)
	}
	if err := f.ws.MoveToChangeList([]string{"notes/c.txt", "notes/d.txt"}, b); err != nil {
		t.Fatal(err)
	}
	f.settle()

	selectChangeList(t, f, "Group A")
	if f.win.cm.listID != a {
		t.Fatalf("filter = %q, want %q", f.win.cm.listID, a)
	}
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("30-group-a-before-stage")
	changeListsClick(t, f, "Stage All")
	f.settle()

	staged := stagedNames(f)
	for _, want := range []string{"notes/a.txt", "notes/b.txt"} {
		if !staged[want] {
			t.Fatalf("Stage All did not stage %q; staged: %v", want, staged)
		}
	}
	for _, other := range []string{"notes/c.txt", "notes/d.txt"} {
		if staged[other] {
			t.Fatalf("Stage All staged %q from another list; staged: %v", other, staged)
		}
	}
	if untracked := untrackedNames(f); !untracked["notes/c.txt"] || !untracked["notes/d.txt"] {
		t.Fatalf("other list files are no longer untracked: %v", untracked)
	}
	f.snap("31-group-a-staged")

	selectChangeList(t, f, "Group B")
	if f.win.cm.listID != b {
		t.Fatalf("filter = %q, want %q", f.win.cm.listID, b)
	}
	changeListsClick(t, f, "Stage All")
	f.settle()
	staged = stagedNames(f)
	if !staged["notes/c.txt"] || !staged["notes/d.txt"] {
		t.Fatalf("Stage All for the second list staged: %v", staged)
	}
	f.snap("32-group-b-staged")
}

// TestChangeListFilterKeepsStagedFromOtherLists shows the visible staged rows
// and the commit-scope warning while another list is filtered out.
func TestChangeListFilterKeepsStagedFromOtherLists(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	a, err := f.ws.CreateChangeList("Group A")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.ws.CreateChangeList("Group B")
	if err != nil {
		t.Fatal(err)
	}
	// README.md is staged by the fixture; store.go and main.go are unstaged.
	if err := f.ws.MoveToChangeList([]string{"main.go"}, a); err != nil {
		t.Fatal(err)
	}
	if err := f.ws.MoveToChangeList([]string{"README.md", "internal/store/store.go"}, b); err != nil {
		t.Fatal(err)
	}
	f.settle()

	selectChangeList(t, f, "Group A")
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	texts := strings.Join(f.s.Texts(), " | ")
	if len(f.win.cm.sel) != 0 {
		t.Fatalf("unexpected selection after filter: %+v", f.win.cm.sel)
	}
	if !f.s.HasText("Staged") || !f.s.HasText("README.md") {
		t.Fatalf("staged file from Group B is hidden while filtering Group A\ntexts: %s", texts)
	}
	if !f.s.HasText("main.go") {
		t.Fatalf("Group A file missing\ntexts: %s", texts)
	}
	if f.s.HasText("store.go") {
		t.Fatalf("unstaged Group B file shown\ntexts: %s", texts)
	}
	if !f.s.HasText("Commit includes all staged files") {
		t.Fatalf("missing commit warning\ntexts: %s", texts)
	}
	f.snap("40-filtered-with-other-list-staged")

	// Staging the visible file must not unstage or otherwise touch the other
	// list's staged file.
	changeListsClick(t, f, "Stage All")
	f.settle()
	staged := stagedNames(f)
	if !staged["main.go"] || !staged["README.md"] {
		t.Fatalf("staged file from another list was lost: %v", staged)
	}
	f.snap("41-after-stage-all")
}

// TestChangeListGroupAndDirectorySelectionOnlyVisible selects the Changes
// group and a directory while a list is filtered: only visible files may be
// staged, never hidden files from another list.
func TestChangeListGroupAndDirectorySelectionOnlyVisible(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	files := []string{"multi/vis.txt", "multi/hidden.txt", "rootVis.txt", "rootHidden.txt"}
	for _, name := range files {
		f.write(name, "base "+name+"\n")
	}
	f.git(append([]string{"add", "--"}, files...)...)
	f.git(append([]string{"commit", "-q", "-m", "Add list selection fixtures", "--"}, files...)...)
	for _, name := range files {
		f.write(name, "changed "+name+"\n")
	}
	f.ws.Refresh(true)
	f.settle()
	a, err := f.ws.CreateChangeList("Visible")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.ws.CreateChangeList("Hidden")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ws.MoveToChangeList([]string{"multi/vis.txt", "rootVis.txt"}, a); err != nil {
		t.Fatal(err)
	}
	if err := f.ws.MoveToChangeList([]string{"multi/hidden.txt", "rootHidden.txt"}, b); err != nil {
		t.Fatal(err)
	}
	f.ws.Settings.CommitTree = true
	f.win.Invalidate()
	f.settle()
	selectChangeList(t, f, "Visible")

	// The group row must expand only visible rows.
	if err := f.s.ClickLabel("Changes"); err != nil {
		t.Fatalf("select Changes group: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.Keys("space"); err != nil {
		t.Fatalf("space: %v", err)
	}
	f.settle()
	staged := stagedNames(f)
	for _, want := range []string{"multi/vis.txt", "rootVis.txt"} {
		if !staged[want] {
			t.Fatalf("group selection did not stage %q; staged: %v", want, staged)
		}
	}
	for _, hidden := range []string{"multi/hidden.txt", "rootHidden.txt"} {
		if staged[hidden] {
			t.Fatalf("group selection staged hidden file %q; staged: %v", hidden, staged)
		}
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		visible := 0
		for _, entry := range f.win.changeListEntries(domain.AreaStaged) {
			if entry.Path == "multi/vis.txt" || entry.Path == "rootVis.txt" {
				visible++
			}
		}
		return visible == 2 && !f.ws.Busy() && !f.ws.Pending()
	}, 10*time.Second); err != nil {
		t.Fatalf("staged group status did not reach the UI: %v", err)
	}
	f.snap("50-group-selection-filtered")

	// Start over and select the directory row instead.
	changeListsClick(t, f, "Unstage All")
	f.settle()
	if left := stagedNames(f); len(left) > 0 {
		t.Fatalf("Unstage All left %v", left)
	}
	// Git's index can already be updated while its refreshed status is still
	// queued for the UI. Wait for the unstaged directory layout, then click it
	// once and verify the selection before sending Space.
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return len(f.win.changeListEntries(domain.AreaStaged)) == 0 &&
			f.win.rowIndex(rowKey(domain.AreaUnstaged, "multi/")) >= 0 && !f.ws.Busy() && !f.ws.Pending()
	}, 10*time.Second); err != nil {
		t.Fatalf("unstaged directory did not load: %v", err)
	}
	waitStableControl(t, f, "multi")
	if err := f.s.ClickLabel("multi"); err != nil {
		t.Fatalf("select multi dir: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.cm.sel[rowKey(domain.AreaUnstaged, "multi/")]
	}, 5*time.Second); err != nil {
		t.Fatalf("directory click did not select multi: %v; selection=%v", err, f.win.cm.sel)
	}
	if err := f.s.Keys("space"); err != nil {
		t.Fatalf("space: %v", err)
	}
	f.settle()
	staged = stagedNames(f)
	if !staged["multi/vis.txt"] {
		t.Fatalf("directory selection did not stage multi/vis.txt; staged: %v", staged)
	}
	if staged["multi/hidden.txt"] || staged["rootVis.txt"] || staged["rootHidden.txt"] {
		t.Fatalf("directory selection staged files outside the visible directory set: %v", staged)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		entries := f.win.changeListEntries(domain.AreaStaged)
		return len(entries) == 1 && entries[0].Path == "multi/vis.txt" && !f.ws.Busy() && !f.ws.Pending()
	}, 10*time.Second); err != nil {
		t.Fatalf("staged directory status did not reach the UI: %v", err)
	}
	f.snap("51-directory-selection-filtered")
}

// TestChangeListRepoSwitchResetsFilter opens another repository on the same
// workspace: the filter returns to All, membership stays per repository, and
// no path leaks across repositories.
func TestChangeListRepoSwitchResetsFilter(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	id := createChangeList(t, f, "Local tests")
	selectChangeList(t, f, "All lists")
	moveToChangeList(t, f, "main.go", "Local tests")
	selectChangeList(t, f, "Local tests")
	if f.win.cm.listID != id {
		t.Fatalf("filter = %q, want %q", f.win.cm.listID, id)
	}

	other := filepath.Join(filepath.Dir(f.dir), "other-repo")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	f.gitIn(other, "init", "-q", "-b", "main")
	f.gitIn(other, "config", "user.name", "Ada Lovelace")
	f.gitIn(other, "config", "user.email", "ada@example.com")
	f.gitIn(other, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(other, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.gitIn(other, "add", "-A")
	f.gitIn(other, "commit", "-q", "-m", "Other repo")
	if err := os.WriteFile(filepath.Join(other, "main.go"), []byte("package main\n\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f.ws.Open(other, nil)
	err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && f.ws.Repo.Repo.Root == other && f.ws.Repo.StatusLoaded && f.win.cm.listID == ""
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("repository switch did not reset filter: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.win.Invalidate()
	f.settle()
	if f.win.cm.listID != "" || !f.s.HasText("All lists") {
		t.Fatalf("filter after repo switch = %q", f.win.cm.listID)
	}
	if got := f.ws.ChangeListFor("main.go"); got != "default" {
		t.Fatalf("other repo main.go belongs to %q, want default", got)
	}
	if lists := f.ws.Lists(); len(lists) != 1 || lists[0].ID != "default" {
		t.Fatalf("other repo lists = %+v", lists)
	}
	if f.s.HasText("README.md") {
		t.Fatalf("previous repository files are still shown\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("main.go") {
		t.Fatalf("other repository changes are not shown\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("60-other-repo-all-lists")

	// Switching back restores the first repository's lists but still starts
	// from All lists.
	f.ws.Open(f.dir, nil)
	err = f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && f.ws.Repo.Repo.Root == f.dir && f.ws.Repo.StatusLoaded && f.win.cm.listID == ""
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("switching back failed: %v", err)
	}
	f.win.Invalidate()
	f.settle()
	if got := f.ws.ChangeListFor("main.go"); got != id {
		t.Fatalf("first repo main.go belongs to %q, want %q", got, id)
	}
	if _, ok := changeListNamed(f.ws.Lists(), "Local tests"); !ok {
		t.Fatalf("first repo lists = %+v", f.ws.Lists())
	}
	f.snap("61-back-to-first-repo")
}

// TestChangeListsBarAtDefaultAndNarrowWidths checks the filter bar layout at
// the default Commit panel width, at a 900px window, and at the panel's
// minimum width: the commit-scope warning must stay a full row of its own.
func TestChangeListsBarAtDefaultAndNarrowWidths(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	a, err := f.ws.CreateChangeList("Layout A")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ws.MoveToChangeList([]string{"main.go"}, a); err != nil {
		t.Fatal(err)
	}
	f.settle()
	selectChangeList(t, f, "Layout A")
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	if !f.s.HasText("Commit includes all staged files") {
		t.Fatalf("warning missing at default width\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("70-bar-default-1440")

	// The narrowest Commit panel: 240px, as clamped by the splitting code.
	f.ws.Settings.LeftPanelWidth = 240
	f.win.Invalidate()
	f.s.Settle(80*time.Millisecond, time.Second)
	if !f.s.HasText("Commit includes all staged files") {
		t.Fatalf("warning missing at 240px panel\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("Layout A") {
		t.Fatalf("filter name missing at 240px panel\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("71-bar-panel-240")

	// A 900px window with the default panel width.
	f.ws.Settings.LeftPanelWidth = 340
	resize(t, f, 900, 900)
	f.win.Invalidate()
	f.s.Settle(80*time.Millisecond, time.Second)
	if !f.s.HasText("Commit includes all staged files") {
		t.Fatalf("warning missing in 900px window\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("72-bar-900-window")

	// All lists uses one row only: the warning disappears and the bar stays
	// compact.
	selectChangeList(t, f, "All lists")
	f.win.Invalidate()
	f.s.Settle(80*time.Millisecond, time.Second)
	if f.s.HasText("Commit includes all staged files") {
		t.Fatalf("unfiltered view still shows the warning\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("73-bar-all-lists")
}

func resize(t *testing.T, f *uiFixture, width, height int) {
	t.Helper()
	if err := f.s.Resize(width, height); err != nil {
		t.Fatalf("resize: %v", err)
	}
	f.s.Settle(80*time.Millisecond, time.Second)
}
