//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
)

// openConflictResult opens the conflict tab and waits until its Result pane owns
// the keyboard. The pane is editable as soon as the tab loads, so tests must not
// type before that focus lands.
func openConflictResult(t *testing.T, f *uiFixture, path string) *diffTab {
	t.Helper()
	f.click(path)
	f.settleTab()
	tab := f.win.ed.cur()
	if tab == nil || tab.src.Path != path || tab.res == nil || tab.res.Conflict == nil {
		t.Fatalf("open conflict %s: %+v", path, tab)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.cedit != nil && f.win.focused(idConflictText)
	}, conflictWait); err != nil {
		t.Fatalf("Result pane did not become editable: %v\n%s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	return tab
}

// snapshotHit finds a hit even when it is disabled: headless Session.Hit only
// returns enabled targets, and the disabled state is exactly what some tests
// assert.
func snapshotHit(f *uiFixture, label string) (ui.Hit, bool) {
	for _, h := range f.s.Snapshot().Hits {
		if h.Label == label {
			return h, true
		}
	}
	return ui.Hit{}, false
}

func conflictBuffer(t *testing.T, tab *diffTab) string {
	t.Helper()
	if tab.cedit == nil {
		t.Fatal("the Result pane has no buffer")
	}
	return tab.cedit.ed.String()
}

func waitConflictBuffer(t *testing.T, f *uiFixture, tab *diffTab, want string) {
	t.Helper()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.cedit != nil && tab.cedit.ed.String() == want
	}, conflictWait); err != nil {
		t.Fatalf("buffer never became %q; it is %q", want, conflictBuffer(t, tab))
	}
}

// TestConflictResultTypeUndoRedoAndSave drives the whole editable Result path:
// type over the markers, undo and redo, save through app.SaveFile, confirm the
// view reloads from disk and the explicit Mark as Resolved still stages.
func TestConflictResultTypeUndoRedoAndSave(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	if !strings.Contains(conflictBuffer(t, tab), "<<<<<<<") {
		t.Fatalf("the Result pane did not start from the working-tree markers: %q", conflictBuffer(t, tab))
	}
	if !f.s.HasText("editable") || !f.s.HasText("Save Merge Result") {
		t.Fatalf("editable pane controls missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}

	if err := f.s.Keys("cmd+a"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("merged\n"); err != nil {
		t.Fatal(err)
	}
	waitConflictBuffer(t, f, tab, "merged\n")
	if !tab.cedit.dirty {
		t.Fatal("typing did not mark the Result buffer dirty")
	}
	if !strings.Contains(tab.title(), "*") {
		t.Fatalf("dirty tab title = %q", tab.title())
	}

	// The whole replacement is one undo unit; redo puts it back.
	if err := f.s.Keys("cmd+z"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.cedit != nil && strings.Contains(tab.cedit.ed.String(), "<<<<<<<")
	}, conflictWait); err != nil {
		t.Fatalf("undo did not restore the markers: %v", err)
	}
	if tab.cedit.dirty {
		t.Fatal("undo back to the loaded text is still dirty")
	}
	if err := f.s.Keys("cmd+shift+z"); err != nil {
		t.Fatal(err)
	}
	waitConflictBuffer(t, f, tab, "merged\n")

	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	uiWaitFile(t, f, "conflict.txt", "merged\n")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.cedit == nil && !tab.loading
	}, conflictWait); err != nil {
		t.Fatalf("the view did not reload after the save: %v", err)
	}
	f.settle()
	if !f.s.HasText("No conflict markers left in the file on disk") {
		t.Fatalf("resolved banner missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	if status := f.git("status", "--porcelain"); !strings.Contains(status, "UU conflict.txt") {
		t.Fatalf("save staged the file by itself:\n%s", status)
	}

	f.click("Mark as Resolved")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && len(f.ws.Repo.Entries(domain.AreaConflicts)) == 0
	}, conflictWait); err != nil {
		t.Fatalf("Marker as Resolved did not stage the file: %v", err)
	}
	f.settle()
	if subject := strings.TrimSpace(f.git("log", "-1", "--format=%s")); subject != "ours" {
		t.Fatalf("the resolution created a commit: %q", subject)
	}
	if !f.ws.Repo.Operation.Active() {
		t.Fatalf("operation ended by itself: %+v", f.ws.Repo.Operation)
	}
	f.assertIdle()
}

// TestConflictResultClickPlacesCaret pins the pointer mapping of the editable
// pane: a press picks the row and cell under it, and the caret keys and typing
// then act there.
func TestConflictResultClickPlacesCaret(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	e := tab.cedit
	line := -1
	for i, l := range e.lines {
		if l == "ours" {
			line = i
			break
		}
	}
	if line < 0 {
		t.Fatalf("no ours line in %q", strings.Join(e.lines, " | "))
	}
	x := e.tr.X + 6 + 2*e.cellW
	y := e.tr.Y + float32(line)*e.lineH - e.top + e.lineH/2
	if err := f.s.Click(x, y); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	want := utf8.RuneCountInString(e.built[:e.byteAt(line, 2)])
	if got := e.ed.Caret(); got != want {
		t.Fatalf("click placed the caret at rune %d, want %d", got, want)
	}
	if err := f.s.Type("X"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if got := e.lines[line]; got != "ouXrs" {
		t.Fatalf("typing at the click = %q, want %q", got, "ouXrs")
	}
	// A click at the line start plus a shift selection stay inside the pane.
	if err := f.s.Click(e.tr.X+6, y); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.Keys("shift+right"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("Y"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if got := e.lines[line]; got != "YuXrs" {
		t.Fatalf("selected typing = %q, want %q", got, "YuXrs")
	}
	if !e.dirty {
		t.Fatal("the edit is not dirty")
	}
	if f.win.canEdit(tab) || tab.edit != nil {
		t.Fatal("the conflict file also opened the file-editor mode")
	}
	if hit, ok := f.s.Hit(f.win.t(i18n.ConflictEditLabel)); !ok || hit.Role != ui.RoleTextField {
		t.Fatalf("Result pane hit = %+v, %t", hit, ok)
	}
	f.settle()
}

// TestConflictResultAcceptKeepsManualEditAndUndo proves the per-block choice and
// hand edits share one buffer: Accept edits the buffer (no disk write), keeps the
// manual line, and undo reverses it.
func TestConflictResultAcceptKeepsManualEditAndUndo(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	e := tab.cedit
	if err := f.s.Click(e.tr.X+6, e.tr.Y+e.lineH/2); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.Type("manual\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.cedit != nil && strings.HasPrefix(tab.cedit.ed.String(), "manual\n<<<<<<<")
	}, conflictWait); err != nil {
		t.Fatalf("manual edit was not inserted above the markers: %v (%q)", err, conflictBuffer(t, tab))
	}
	diskBefore := readFixtureFile(t, f, "conflict.txt")

	f.click("Accept Ours")
	waitConflictBuffer(t, f, tab, "manual\nours\n")
	if got := readFixtureFile(t, f, "conflict.txt"); got != diskBefore {
		t.Fatalf("Accept wrote the file before Save: %q", got)
	}
	if !tab.cedit.dirty {
		t.Fatal("the accepted result is not dirty")
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.focused(idConflictText)
	}, conflictWait); err != nil {
		t.Fatal("Accept did not return the keyboard to the Result pane")
	}

	if err := f.s.Keys("cmd+z"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.cedit != nil && tab.cedit.ed.String() == "manual\n"+diskBefore
	}, conflictWait); err != nil {
		t.Fatalf("undo did not restore the accepted block: %v", err)
	}
	if err := f.s.Keys("cmd+shift+z"); err != nil {
		t.Fatal(err)
	}
	waitConflictBuffer(t, f, tab, "manual\nours\n")

	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	uiWaitFile(t, f, "conflict.txt", "manual\nours\n")
	if status := f.git("status", "--porcelain"); !strings.Contains(status, "UU conflict.txt") {
		t.Fatalf("save staged the resolved content:\n%s", status)
	}
	f.settle()
}

// TestConflictResultSavePreservesEncodingAndMode pins the bytes a save writes:
// the original CRLF line ending, the UTF-8 BOM and the executable bit survive,
// and the final newline is kept.
func TestConflictResultSavePreservesEncodingAndMode(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	original := "\xef\xbb\xbf<<<<<<< HEAD\r\nours\r\n=======\r\ntheirs\r\n>>>>>>> other\r\n"
	f.write("conflict.txt", original)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(f.dir, "conflict.txt"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	if tab.cedit.lineEnd != "CRLF" || !tab.cedit.bom {
		t.Fatalf("encoding = %q, BOM %t", tab.cedit.lineEnd, tab.cedit.bom)
	}
	f.click("Accept Ours")
	waitConflictBuffer(t, f, tab, "ours\n")
	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	want := "\xef\xbb\xbfours\r\n"
	uiWaitFile(t, f, "conflict.txt", want)
	if got := readFixtureFile(t, f, "conflict.txt"); got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(f.dir, "conflict.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("executable bit lost: %v", info.Mode().Perm())
		}
	}
	f.settle()
}

// TestConflictResultSaveRefusesExternalChange edits the file outside Viff after
// the buffer was loaded: Save must refuse, keep the user's text and offer the
// explicit Reload that drops it.
func TestConflictResultSaveRefusesExternalChange(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	if err := f.s.Keys("cmd+a"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("mine\n"); err != nil {
		t.Fatal(err)
	}
	waitConflictBuffer(t, f, tab, "mine\n")

	external := "<<<<<<< HEAD\nexternal\n=======\ntheirs\n>>>>>>> other\n"
	f.write("conflict.txt", external)
	notifications := len(f.ws.Notifications)
	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return len(f.ws.Notifications) > notifications &&
			tab.cedit != nil && !tab.cedit.saving && tab.cedit.dirty
	}, conflictWait); err != nil {
		t.Fatalf("the failed save did not report and keep the buffer: %v", err)
	}
	if got := readFixtureFile(t, f, "conflict.txt"); got != external {
		t.Fatalf("Save overwrote the external change: %q", got)
	}
	if got := conflictBuffer(t, tab); got != "mine\n" {
		t.Fatalf("failed save changed the buffer: %q", got)
	}
	last := f.ws.Notifications[len(f.ws.Notifications)-1]
	if last.Kind != vapp.NoticeWarning || last.Title != f.win.t(i18n.FileviewChangedTitle) {
		t.Fatalf("notification = %+v", last)
	}

	// The notice's Reload is the explicit confirmation; it drops the buffer and
	// reopens the pane on what is on disk, without another dialog.
	f.win.reloadConflict(tab)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.cedit != nil && tab.cedit.original == external
	}, conflictWait); err != nil {
		t.Fatalf("Reload did not take the disk text: %v (%q)", err, conflictBuffer(t, tab))
	}
	if f.win.dialog != nil {
		t.Fatalf("Reload asked a second time: %q", f.win.dialog.title)
	}
	f.settle()
}

// TestConflictMarkResolvedBlockedWhileDirty pins the explicit resolution rule:
// Mark as Resolved never stages unsaved buffer text, and becomes available only
// after the edited result was saved and holds no markers.
func TestConflictMarkResolvedBlockedWhileDirty(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	e := tab.cedit
	line := -1
	for i, l := range e.lines {
		if l == "ours" {
			line = i
			break
		}
	}
	if line < 0 {
		t.Fatalf("no ours line in %q", strings.Join(e.lines, " | "))
	}
	if err := f.s.Click(e.tr.X+6, e.tr.Y+float32(line)*e.lineH-e.top+e.lineH/2); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.Type("X"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if hit, ok := snapshotHit(f, "Mark as Resolved"); !ok || !hit.Disabled {
		t.Fatalf("Mark as Resolved hit = %+v, %t; want disabled while dirty", hit, ok)
	}
	if status := f.git("status", "--porcelain"); !strings.Contains(status, "UU conflict.txt") {
		t.Fatalf("a dirty buffer staged the file:\n%s", status)
	}

	// Saving with the markers still present keeps the file unmerged and the
	// control disabled: only a result without markers may be marked resolved.
	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.cedit != nil && !tab.cedit.dirty && !tab.loading
	}, conflictWait); err != nil {
		t.Fatalf("the save did not finish: %v", err)
	}
	if hit, ok := snapshotHit(f, "Mark as Resolved"); !ok || !hit.Disabled {
		t.Fatalf("Mark as Resolved hit = %+v, %t; want disabled while markers remain", hit, ok)
	}

	if err := f.s.Keys("cmd+a"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("resolved\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	uiWaitFile(t, f, "conflict.txt", "resolved\n")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.cedit == nil && !tab.loading
	}, conflictWait); err != nil {
		t.Fatal(err)
	}
	f.settle()
	if hit, ok := snapshotHit(f, "Mark as Resolved"); !ok || hit.Disabled {
		t.Fatalf("Mark as Resolved hit = %+v, %t; want enabled after the clean save", hit, ok)
	}
	f.click(f.win.t(i18n.ConflictActionMarkAsResolved))
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && len(f.ws.Repo.Entries(domain.AreaConflicts)) == 0
	}, conflictWait); err != nil {
		t.Fatalf("Mark as Resolved did not run: %v", err)
	}
	f.settle()
}

// TestConflictUnsavedGuards covers the exits that could drop the Result buffer:
// closing the tab asks, Cancel keeps it, another preview tab may not replace it,
// and the shared repository/quit guard sees it as dirty too.
func TestConflictUnsavedGuards(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	if err := f.s.Type("draft\n"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if f.win.dirtyEdit() != tab {
		t.Fatal("the shared unsaved guard does not see the Result buffer")
	}

	if err := f.s.Keys("cmd+w"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, conflictWait); err != nil {
		t.Fatalf("closing a dirty Result pane did not ask: %v", err)
	}
	if f.win.dialog.title != f.win.t(i18n.FileviewDiscardTitle) {
		t.Fatalf("dialog = %q", f.win.dialog.title)
	}
	f.click("Cancel")
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog == nil }, conflictWait); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if tab.cedit == nil || !strings.HasPrefix(conflictBuffer(t, tab), "draft\n") {
		t.Fatal("Cancel dropped the Result buffer")
	}

	// A preview tab cannot replace one that holds unsaved text: the conflict tab
	// is pinned and the buffer stays.
	f.win.showDiff(vapp.DiffSource{Path: "main.go", Working: true, Area: domain.AreaUnstaged, Kind: domain.ChangeModified}, false)
	f.settleTab()
	found := false
	for _, o := range f.win.ed.tabs {
		if o == tab {
			found = true
		}
	}
	if !found || tab.preview || tab.cedit == nil {
		t.Fatal("opening another file dropped the dirty conflict tab")
	}
	f.click("conflict.txt")
	f.settleTab()

	// The window-close guard shares the same dirty check.
	allowed := false
	f.win.onCloseRequest(func() { allowed = true })
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && hasExactHit(f, "Cancel")
	}, conflictWait); err != nil {
		t.Fatalf("closing the window did not ask about the Result buffer: %v", err)
	}
	f.click("Cancel")
	f.s.Settle(40*time.Millisecond, time.Second)
	if allowed {
		t.Fatal("Cancel allowed the window to close")
	}
	if tab.cedit == nil || !strings.HasPrefix(conflictBuffer(t, tab), "draft\n") {
		t.Fatal("the window-close guard dropped the Result buffer")
	}

	if err := f.s.Keys("cmd+w"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, conflictWait); err != nil {
		t.Fatal(err)
	}
	f.click(f.win.t(i18n.FileviewDiscardButton))
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		for _, o := range f.win.ed.tabs {
			if o == tab {
				return false
			}
		}
		return true
	}, conflictWait); err != nil {
		t.Fatalf("Discard did not close the tab: %v", err)
	}
	if tab.cedit != nil {
		t.Fatal("Discard kept the Result buffer")
	}
	f.settle()
}

// TestConflictContextCopyUsesEditedResult keeps Copy for AI honest while the
// buffer has unsaved edits: the copied block is the one on screen, not the
// markers as they were loaded.
func TestConflictContextCopyUsesEditedResult(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	copied := copySeam(f)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	e := tab.cedit
	line := -1
	for i, l := range e.lines {
		if l == "ours" {
			line = i
			break
		}
	}
	if line < 0 {
		t.Fatalf("no ours line in %q", strings.Join(e.lines, " | "))
	}
	if err := f.s.Click(e.tr.X+6, e.tr.Y+float32(line)*e.lineH-e.top+e.lineH/2); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.Type("hand-"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("Copy This Block")
	if len(*copied) != 1 {
		t.Fatalf("copied %d texts, want 1", len(*copied))
	}
	if !strings.Contains((*copied)[0], "hand-ours") {
		t.Fatalf("copied context is not the edited block:\n%s", (*copied)[0])
	}
	f.settle()
}

// TestConflictUnreliableMarkersStayReadOnly covers the fallback: a file whose
// markers do not form complete blocks gets no Result editor and no Save.
func TestConflictUnreliableMarkersStayReadOnly(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.write("conflict.txt", "<<<<<<< HEAD\nours\n=======\ntheirs\n") // no closing marker
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	f.click("conflict.txt")
	f.settleTab()
	tab := f.win.ed.cur()
	if tab == nil || tab.res == nil || tab.res.Conflict == nil {
		t.Fatalf("conflict tab = %+v", tab)
	}
	if tab.cedit != nil {
		t.Fatal("an unreliable marker parse got an editable Result pane")
	}
	if f.win.conflictEditable(tab) {
		t.Fatal("conflictEditable accepted an unreliable parse")
	}
	for _, label := range []string{"Save Merge Result", "Accept Ours", "Accept Theirs", "Accept Both"} {
		if hasExactHit(f, label) {
			t.Fatalf("%s shown for a read-only conflict", label)
		}
	}
	if f.s.HasText("editable") {
		t.Fatalf("read-only Result pane is labelled editable: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.settle()
}

// TestConflictResultRefusesIncompleteMarkers keeps a save from writing a marker
// set the viewer could no longer parse: the buffer stays editable and the file
// on disk is unchanged until the user removes or completes the markers.
func TestConflictResultRefusesIncompleteMarkers(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	e := tab.cedit
	disk := readFixtureFile(t, f, "conflict.txt")
	if err := f.s.Click(e.tr.X+6, e.tr.Y+e.lineH/2); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.Type("X"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if tab.cedit.markers.Reliable {
		t.Fatal("the corrupted markers still parse reliably")
	}
	notifications := len(f.ws.Notifications)
	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return len(f.ws.Notifications) > notifications
	}, conflictWait); err != nil {
		t.Fatal("the refused save did not report")
	}
	last := f.ws.Notifications[len(f.ws.Notifications)-1]
	if last.Kind != vapp.NoticeWarning || last.Title != f.win.t(i18n.FileviewSaveFailed, "conflict.txt") {
		t.Fatalf("notification = %+v", last)
	}
	if got := readFixtureFile(t, f, "conflict.txt"); got != disk {
		t.Fatalf("the refused save wrote the file: %q", got)
	}
	if tab.cedit == nil || !tab.cedit.dirty || !strings.HasPrefix(conflictBuffer(t, tab), "X") {
		t.Fatal("the refused save dropped or cleaned the buffer")
	}
	f.settle()
}

// TestConflictEditableRequiresResolverCapability covers the backend half of the
// fallback: Subversion has no per-block resolver capability, so its conflict tab
// must stay read-only even though the file itself is text.
func TestConflictEditableRequiresResolverCapability(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	tab := openConflictResult(t, f, "conflict.txt")
	if !f.win.conflictEditable(tab) {
		t.Fatal("a Git text conflict is not editable")
	}
	caps := f.ws.Caps.Conflicts
	f.ws.Caps.Conflicts = nil
	defer func() { f.ws.Caps.Conflicts = caps }()
	if f.win.conflictEditable(tab) {
		t.Fatal("a backend without a conflict resolver is editable")
	}
	if !conflictEditableContent(tab.res.Conflict) {
		t.Fatal("the content check should not depend on the backend capability")
	}
	f.settle()
}
