//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/ui"
)

// fileViewSelectionContent has a tab (painted as spaces), a wide rune and
// partial lines, so the test covers the display-cell mapping end to end.
const fileViewSelectionContent = "alpha\tbeta\nwide 世界 line\nlast line\n"

// TestFileViewTextSelection opens a read-only file view, drags a selection
// through the same pointer path a mouse takes, copies it with Command+C and
// checks that reload and a tab switch drop the selection again.
func TestFileViewTextSelection(t *testing.T) {
	f := newUIFixture(t)
	f.write("selection.txt", fileViewSelectionContent)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	f.win.openFileView("selection.txt", "", false)
	f.settleTab()
	tab := f.win.ed.cur()
	if tab == nil || !tab.src.View {
		t.Fatal("file view did not open")
	}
	if len(tab.newDisp) != 3 {
		t.Fatalf("file view lines = %d, want 3: %q", len(tab.newDisp), tab.newDisp)
	}
	if !f.s.HasText("alpha   beta") {
		t.Fatalf("file view does not paint the expanded tab: %s", strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("wide 世界 line") {
		t.Fatalf("file view does not paint the wide line: %s", strings.Join(f.s.Texts(), " | "))
	}

	hit, ok := f.s.Hit("File contents")
	if !ok {
		t.Fatalf("the text area is not a reachable control: %s", strings.Join(f.s.Texts(), " | "))
	}
	cellW, lineH, _ := f.win.metrics()
	at := func(cell, line int) (float32, float32) {
		return hit.Bounds.X + 6 + float32(cell)*cellW, hit.Bounds.Y + float32(line)*lineH + lineH/2
	}
	drag := func(fromCell, fromLine, toCell, toLine int) {
		t.Helper()
		sx, sy := at(fromCell, fromLine)
		ex, ey := at(toCell, toLine)
		if err := f.s.Press(sx, sy); err != nil {
			t.Fatalf("press: %v", err)
		}
		if err := f.s.Move(ex, ey); err != nil {
			t.Fatalf("drag: %v", err)
		}
		if err := f.s.Release(ex, ey); err != nil {
			t.Fatalf("release: %v", err)
		}
		if err := f.s.Settle(60*time.Millisecond, 2*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	copySelection := func() string {
		t.Helper()
		if err := f.s.Keys("cmd+c"); err != nil {
			t.Fatalf("cmd+c: %v", err)
		}
		if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
			t.Fatal(err)
		}
		return f.s.Driver().Clipboard()
	}

	// A drag from line 0 cell 2 to line 2 cell 4 keeps the partial first and
	// last lines, the expanded tab and the wide rune.
	drag(2, 0, 4, 2)
	if tab.textSel.anchor != (selPoint{line: 0, cell: 2}) || tab.textSel.focus != (selPoint{line: 2, cell: 4}) {
		t.Fatalf("selection = %+v, want line 0 cell 2 to line 2 cell 4", tab.textSel)
	}
	want := "pha   beta\nwide 世界 line\nlast"
	if got := tab.selectionText(); got != want {
		t.Errorf("selectionText() = %q, want %q", got, want)
	}
	if got := copySelection(); got != want {
		t.Errorf("Command+C clipboard = %q, want %q", got, want)
	}
	f.snap("file-view-text-selection")

	// Escape clears it; Shift+Down then extends from the top of the viewport.
	if err := f.s.Keys("escape"); err != nil {
		t.Fatal(err)
	}
	if tab.textSel.set {
		t.Fatal("Escape kept the selection")
	}
	if err := f.s.Keys("shift+down"); err != nil {
		t.Fatal(err)
	}
	if tab.textSel.anchor != (selPoint{line: 0, cell: 0}) || tab.textSel.focus != (selPoint{line: 1, cell: 0}) {
		t.Fatalf("Shift+Down selection = %+v, want line 0 cell 0 to line 1 cell 0", tab.textSel)
	}
	if got := copySelection(); got != "alpha   beta\n" {
		t.Errorf("Shift+Down copy = %q, want %q", got, "alpha   beta\n")
	}

	// A reload re-reads the file and drops the selection.
	f.win.loadTab(tab)
	f.settleTab()
	if tab.textSel.set {
		t.Fatal("reload kept the selection")
	}

	// Select All covers the whole file without a trailing newline.
	if err := f.s.Keys("cmd+a"); err != nil {
		t.Fatalf("cmd+a: %v", err)
	}
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	last := len(tab.newDisp) - 1
	if tab.textSel.anchor != (selPoint{line: 0, cell: 0}) ||
		tab.textSel.focus != (selPoint{line: last, cell: displayCells(tab.newDisp[last])}) {
		t.Fatalf("Select All = %+v", tab.textSel)
	}
	if got, want := copySelection(), strings.Join(tab.newDisp, "\n"); got != want {
		t.Errorf("Select All copy = %q, want %q", got, want)
	}

	// Opening a diff tab switches away: the file view's selection resets.
	f.click("main.go")
	f.settleTab()
	diff := f.win.ed.cur()
	if diff == nil || diff.src.View {
		t.Fatalf("expected a diff tab, got %+v", diff)
	}
	if tab.textSel.set {
		t.Error("switching tabs kept the file view's selection")
	}

	// A press-drag inside the diff paints no selection: diffs and the line
	// selection used for staging are untouched.
	diffHit, ok := f.s.Hit("Diff")
	if !ok {
		t.Fatalf("the diff pane is not reachable: %s", strings.Join(f.s.Texts(), " | "))
	}
	cx, cy := diffHit.Bounds.X+diffHit.Bounds.Width/2, diffHit.Bounds.Y+diffHit.Bounds.Height/2
	if err := f.s.Press(cx, cy); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Move(cx, cy+3*lineH); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Release(cx, cy+3*lineH); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if diff.textSel.set {
		t.Error("a diff tab took a text selection")
	}

	// Back on the view tab, a double click selects the word under the pointer
	// and a plain click clears it again. Headless Keys lower "]" to "}", so the
	// Command+Shift+] tab chord goes through the shortcut handler directly.
	if !f.win.shortcut(ui.KeyEvent{Text: "]", Modifiers: ui.ModifierMeta | ui.ModifierShift}) {
		t.Fatal("Command+Shift+] did not switch tabs")
	}
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if f.win.ed.cur() != tab {
		t.Fatalf("tab switch did not return to the file view")
	}
	x, y := at(9, 0)
	if err := f.s.DoubleClick(x, y); err != nil {
		t.Fatalf("double click: %v", err)
	}
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if got := tab.selectionText(); got != "beta" {
		t.Errorf("double click selected %q, want %q", got, "beta")
	}
	if err := f.s.Click(x, y); err != nil {
		t.Fatalf("click: %v", err)
	}
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if !tab.textSel.empty() {
		t.Errorf("a click without a drag left a range: %+v", tab.textSel)
	}
	f.assertIdle()
}
