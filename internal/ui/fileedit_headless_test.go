//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/i18n"
)

func openEditableFile(t *testing.T, f *uiFixture, path string) *diffTab {
	t.Helper()
	f.win.openRepoFile(path, true)
	f.settleTab()
	tab := f.win.ed.cur()
	if tab == nil || tab.src.Path != path || tab.res == nil {
		t.Fatalf("open %s: %+v", path, tab)
	}
	return tab
}

func waitEditing(t *testing.T, f *uiFixture, tab *diffTab) {
	t.Helper()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.edit != nil && f.win.focused(idDiffText)
	}, 10*time.Second); err != nil {
		t.Fatalf("editor did not take focus: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
}

func hasExactHit(f *uiFixture, label string) bool {
	for _, hit := range f.s.Snapshot().Hits {
		if hit.Label == label {
			return true
		}
	}
	return false
}

func readFixtureFile(t *testing.T, f *uiFixture, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestFileEditTypeSaveAndDirtyState drives the whole in-app editor: enter the
// mode from the toolbar, replace the text, save with Command-S, and check both
// the bytes on disk and Git's status.
func TestFileEditTypeSaveAndDirtyState(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEditableFile(t, f, "config.json")
	if !f.win.canEdit(tab) {
		t.Fatal("a working-tree text file is not editable")
	}

	f.click("Edit\t⌘E")
	waitEditing(t, f, tab)
	if !f.win.focused(idDiffText) {
		t.Fatal("the editor does not hold the keyboard")
	}
	if tab.edit.dirty {
		t.Fatal("a fresh editor is dirty")
	}

	if err := f.s.Keys("cmd+a"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("{\n  \"name\": \"edited\"\n}\n"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if !tab.edit.dirty {
		t.Fatal("typing did not mark the buffer dirty")
	}
	if !strings.Contains(tab.title(), "*") {
		t.Fatalf("dirty tab title = %q", tab.title())
	}
	if !f.s.HasText("Modified") {
		t.Fatalf("toolbar does not show the dirty state: %s", strings.Join(f.s.Texts(), " | "))
	}

	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return !tab.edit.dirty && !tab.edit.saving }, 10*time.Second); err != nil {
		t.Fatalf("save did not finish: %v", err)
	}
	f.settle()
	want := "{\n  \"name\": \"edited\"\n}\n"
	if got := readFixtureFile(t, f, "config.json"); got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
	if out := f.git("status", "--porcelain", "--", "config.json"); !strings.Contains(out, "config.json") {
		t.Fatalf("git status does not see the save: %q", out)
	}

	// A second edit and a close with unsaved text must ask first.
	if err := f.s.Type("// trailing\n"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if !tab.edit.dirty {
		t.Fatal("the second edit is not dirty")
	}
	if err := f.s.Keys("cmd+w"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 5*time.Second); err != nil {
		t.Fatalf("closing a dirty editor did not ask: %v", err)
	}
	if f.win.dialog.title != f.win.t(i18n.FileviewDiscardTitle) {
		t.Fatalf("dialog = %q", f.win.dialog.title)
	}
	f.click("Cancel")
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(f.win.ed.tabs) == 0 || f.win.ed.cur() != tab || tab.edit == nil {
		t.Fatal("Cancel closed the editor")
	}
	if err := f.s.Keys("cmd+w"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	f.click(f.win.t(i18n.FileviewDiscardButton))
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(f.win.ed.tabs) != 0 {
		t.Fatalf("Discard left %d tabs open", len(f.win.ed.tabs))
	}
	if got := readFixtureFile(t, f, "config.json"); got != want {
		t.Fatalf("Discard wrote the buffer: %q", got)
	}
	f.assertIdle()
}

// TestFileEditRefusesAnExternalChange checks that Save never overwrites a file
// someone else changed, and that the Reload notice takes the new disk text.
func TestFileEditRefusesAnExternalChange(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEditableFile(t, f, "config.json")
	f.click("Edit\t⌘E")
	waitEditing(t, f, tab)

	if err := f.s.Keys("cmd+a"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("mine\n"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.write("config.json", "theirs\n")

	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return !tab.edit.saving && tab.edit.dirty }, 10*time.Second); err != nil {
		t.Fatalf("the failed save did not return: %v", err)
	}
	if got := readFixtureFile(t, f, "config.json"); got != "theirs\n" {
		t.Fatalf("Save overwrote the external change: %q", got)
	}
	foundWarning := false
	for _, n := range f.win.ws.Notifications {
		if n.Kind == vapp.NoticeWarning && strings.Contains(n.Body, f.win.t(i18n.FileviewChangedBody)) {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("no changed-on-disk warning: %+v", f.win.ws.Notifications)
	}

	// The warning's Reload action is itself the confirmation: it discards the
	// buffer without a second dialog and opens the disk text.
	f.win.reloadEditNow(tab)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return tab.edit != nil && tab.edit.original == "theirs\n"
	}, 10*time.Second); err != nil {
		t.Fatalf("Reload did not take the disk text: %v", err)
	}
	if f.win.dialog != nil {
		t.Fatalf("Reload asked a second time: %q", f.win.dialog.title)
	}
}

// TestEditPlacementMapsTabsAndWideRunes pins the display-cell mapping the
// caret, pointer placement and IME window share. Tabs expand to four-column
// stops and a wide rune takes two cells, so a raw-byte count is wrong.
func TestEditPlacementMapsTabsAndWideRunes(t *testing.T) {
	ed := text.NewEditor("\tfoo\n你\tX")
	e := &fileEdit{ed: ed}
	tab := &diffTab{src: vapp.DiffSource{Path: "x.txt"}}
	e.sync(tab)
	e.painted = true
	e.tr = ui.Rect{X: 10, Y: 20, Width: 300, Height: 100}
	e.top, e.scrollX, e.cellW, e.lineH = 0, 0, 10, 20

	// End of the tab-indented first line: cell 7, byte 4.
	ed.MoveTo(4, false)
	caret, ok := e.CaretBounds()
	if !ok || caret.X != 10+6+7*10 || caret.Y != 20 {
		t.Fatalf("end-of-line caret = %+v, %t", caret, ok)
	}
	offset, ok := e.OffsetAt(ui.Point{X: 10 + 6 + 7*10, Y: 25})
	if !ok || offset != 4 {
		t.Fatalf("click on the tabbed line = %d, %t", offset, ok)
	}

	// End of the wide-rune line: "你" is two cells, the tab reaches cell 4,
	// and X ends at cell 5.
	ed.MoveTo(8, false)
	caret, ok = e.CaretBounds()
	if !ok || caret.X != 10+6+5*10 || caret.Y != 40 {
		t.Fatalf("wide-rune caret = %+v, %t", caret, ok)
	}
	offset, ok = e.OffsetAt(ui.Point{X: 10 + 6 + 5*10, Y: 45})
	if !ok || offset != 10 {
		t.Fatalf("click on the wide-rune line = %d, %t", offset, ok)
	}
}

// TestEditPasteNormalizesCRLF keeps the reason the editor owns the text event:
// a pasted Windows line ending must not become a visible control character.
func TestEditPasteNormalizesCRLF(t *testing.T) {
	e := &fileEdit{ed: text.NewEditor("")}
	state, err := e.applyText(app.TextEvent{Kind: app.TextPaste, Text: "a\r\nb\rc"})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.ed.String(); got != "a\nb\nc" {
		t.Fatalf("editor text = %q", got)
	}
	if state.Value != "a\nb\nc" {
		t.Fatalf("native state = %q", state.Value)
	}
}

// TestFileEditDoneAndWindowClosePrompt covers the two exits that do not save:
// Done returns to the read-only viewer, and closing the window asks before a
// dirty buffer is dropped.
func TestFileEditDoneAndWindowClosePrompt(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEditableFile(t, f, "config.json")
	f.click("Edit\t⌘E")
	waitEditing(t, f, tab)
	if err := f.s.Type("// draft\n"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)

	f.click("Done")
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 5*time.Second); err != nil {
		t.Fatal("Done did not ask about the dirty buffer")
	}
	f.click(f.win.t(i18n.FileviewDiscardButton))
	if err := f.s.WaitFor(func(app.Snapshot) bool { return tab.edit == nil && !tab.loading }, 10*time.Second); err != nil {
		t.Fatalf("Done did not restore the viewer: %v", err)
	}
	if !hasExactHit(f, "Edit\t⌘E") {
		t.Fatalf("the read-only toolbar did not come back: %s", strings.Join(f.s.Texts(), " | "))
	}

	if err := f.s.Keys("cmd+e"); err != nil {
		t.Fatal(err)
	}
	waitEditing(t, f, tab)
	if err := f.s.Type("// second draft\n"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	allowed := false
	f.win.onCloseRequest(func() { allowed = true })
	f.win.Invalidate()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && hasExactHit(f, f.win.t(i18n.FileviewDiscardButton))
	}, 5*time.Second); err != nil {
		t.Fatal("closing the window did not ask about the dirty buffer")
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	if f.win.dialog.title != f.win.t(i18n.FileviewDiscardTitle) {
		t.Fatalf("close dialog = %q", f.win.dialog.title)
	}
	f.click(f.win.t(i18n.FileviewDiscardButton))
	if !allowed {
		t.Fatal("Discard did not allow the window close")
	}
	// The test does not really close the window; drop the buffer directly.
	f.win.closeTab(0)
}

// TestSwitchToActiveRepositoryKeepsTheBuffer covers the overview's OK button
// on the row that is already active: it is not a switch and must not ask to
// discard an editor buffer.
func TestSwitchToActiveRepositoryKeepsTheBuffer(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEditableFile(t, f, "config.json")
	f.click("Edit\t⌘E")
	waitEditing(t, f, tab)
	if err := f.s.Type("// keep me\n"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	buffer := tab.edit.ed.String()
	f.win.switchRepository(f.ws.ActiveRoot)
	f.settle()
	if f.win.dialog != nil {
		t.Fatalf("re-selecting the active repository asked %q", f.win.dialog.title)
	}
	if tab.edit == nil || tab.edit.ed.String() != buffer {
		t.Fatal("re-selecting the active repository dropped the buffer")
	}
	f.win.closeTab(0)
}

// TestFileEditNotOfferedForReadOnlySources covers the boundary: a historical
// revision, a binary file and a conflicted path keep the read-only viewer.
func TestFileEditNotOfferedForReadOnlySources(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	head := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	f.win.openFileView("README.md", head, false)
	f.settleTab()
	if f.win.canEdit(f.win.ed.cur()) {
		t.Fatal("a historical revision is editable")
	}
	if hasExactHit(f, "Edit\t⌘E") {
		t.Fatal("a historical revision painted the Edit button")
	}

	f.write("logo.bin", "\x00\x01\x02")
	f.win.openRepoFile("logo.bin", true)
	f.settleTab()
	if f.win.canEdit(f.win.ed.cur()) {
		t.Fatal("a binary file is editable")
	}
	if hasExactHit(f, "Edit\t⌘E") {
		t.Fatal("a binary file painted the Edit button")
	}

	f.write("latin1.txt", "caf\xe9\n")
	f.win.openRepoFile("latin1.txt", true)
	f.settleTab()
	if f.win.canEdit(f.win.ed.cur()) {
		t.Fatal("a non-UTF-8 file is editable")
	}

	f.write("big.txt", strings.Repeat("x\n", editContentLimit/2+1))
	f.win.openRepoFile("big.txt", true)
	f.settleTab()
	if f.win.canEdit(f.win.ed.cur()) {
		t.Fatal("a file above the edit limit is editable")
	}
}

// TestFileEditPreservesCRLFAndBOM pins the encoding contract: the editor works
// in LF and strips the BOM, while Save restores both.
func TestFileEditPreservesCRLFAndBOM(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	original := "\xef\xbb\xbfone\r\ntwo\r\n"
	f.write("crlf.txt", original)
	f.ws.Refresh(true)
	f.settle()
	tab := openEditableFile(t, f, "crlf.txt")
	f.click("Edit\t⌘E")
	waitEditing(t, f, tab)
	if tab.edit.lineEnd != "CRLF" || !tab.edit.bom {
		t.Fatalf("encoding = %q, BOM %t", tab.edit.lineEnd, tab.edit.bom)
	}
	if err := f.s.Keys("cmd+a"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("one\ntwo\nthree\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return !tab.edit.dirty && !tab.edit.saving }, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	want := "\xef\xbb\xbfone\r\ntwo\r\nthree\r\n"
	if got := readFixtureFile(t, f, "crlf.txt"); got != want {
		t.Fatalf("saved %q, want %q", got, want)
	}
}

// TestFileEditRevertAndEmptyFile covers the clean-buffer action and the row
// mapping of a file with no lines at all.
func TestFileEditRevertAndEmptyFile(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	f.write("empty.txt", "")
	f.ws.Refresh(true)
	f.settle()
	tab := openEditableFile(t, f, "empty.txt")
	f.click("Edit\t⌘E")
	waitEditing(t, f, tab)

	if err := f.s.Type("first line\nsecond\n"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if !tab.edit.dirty {
		t.Fatal("typing into an empty file did not dirty it")
	}
	f.click("Revert")
	f.s.Settle(40*time.Millisecond, time.Second)
	if tab.edit.dirty || tab.edit.ed.String() != "" {
		t.Fatalf("Revert left %q, dirty %t", tab.edit.ed.String(), tab.edit.dirty)
	}

	// Typing again and saving from the empty state exercises the placement
	// path with no display rows before the first edit.
	if err := f.s.Keys("cmd+a"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("first line\nsecond\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Keys("cmd+s"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return !tab.edit.dirty && !tab.edit.saving }, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := readFixtureFile(t, f, "empty.txt"); got != "first line\nsecond\n" {
		t.Fatalf("file = %q", got)
	}
}
