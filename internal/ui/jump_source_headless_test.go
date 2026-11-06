//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"
)

// TestUnifiedDiffTextSelection opens a diff in the unified mode, drags a
// selection through the same pointer path a mouse takes and copies it with
// Command+C. The selection copies the rows' raw content, so a mixed range of
// context and changed lines reads like the view shows it.
func TestUnifiedDiffTextSelection(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	// fixture repo has unstaged main.go changes (mainV3); unified mode
	f.ws.Settings.DiffUnified = true
	defer func() { f.ws.Settings.DiffUnified = false }()
	f.click("main.go")
	f.settleTab()
	tab := f.win.ed.cur()
	if tab == nil || tab.src.View {
		t.Fatal("no diff tab opened")
	}
	if n := len(tab.unifDisp); n != len(tab.unified) {
		t.Fatalf("unifDisp rows = %d, unified rows = %d", n, len(tab.unified))
	}
	if len(tab.unifDisp) < 3 {
		t.Fatalf("diff has too few rows to select: %q", tab.unifDisp)
	}
	// Scroll to the top so row indexes match the visible rows, then drag over
	// the first wide row: the cell→byte mapping must survive tab indentation
	// and wide runes, and the copy must carry exactly the row's cells.
	tab.y = 0
	f.settle()
	row, rowIndex := "", 0
	for i, l := range tab.unifDisp {
		if displayCells(l) >= 5 {
			row, rowIndex = l, i
			break
		}
	}
	if row == "" {
		t.Fatalf("no unified row wide enough to select: %q", tab.unifDisp)
	}
	want := row[cellOffset(row, 1):cellOffset(row, 4)]

	hit, ok := f.s.Hit("File contents")
	if !ok {
		t.Fatalf("the unified text area is not a reachable control: %s", strings.Join(f.s.Texts(), " | "))
	}
	cellW, lineH, _ := f.win.metrics()
	sx := hit.Bounds.X + 6 + 1*cellW
	ex := hit.Bounds.X + 6 + 4*cellW
	sy := hit.Bounds.Y + float32(rowIndex)*lineH + lineH/2
	if err := f.s.Press(sx, sy); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Move(ex, sy); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Release(ex, sy); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(60*time.Millisecond, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if !tab.textSel.set || tab.textSel.kind != selUnified {
		t.Fatalf("selection is not a unified one: %+v", tab.textSel)
	}
	if got := tab.selectionText(); got != want {
		t.Errorf("selectionText() = %q, want %q", got, want)
	}
	if err := f.s.Keys("cmd+c"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if got := f.s.Driver().Clipboard(); got != want {
		t.Errorf("Command+C clipboard = %q, want %q", got, want)
	}
	f.snap("10-unified-selection")
	f.assertIdle()
}

// TestSideBySideDiffTextSelection opens a diff in the side-by-side mode and
// drags on the new pane: both panes take selectable text, each over its own
// line list. Hit("File contents") returns the new pane because it registers
// after the old one.
func TestSideBySideDiffTextSelection(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	f.click("main.go")
	f.settleTab()
	tab := f.win.ed.cur()
	if tab == nil || tab.src.View {
		t.Fatal("no diff tab opened")
	}
	f.ws.Settings.DiffUnified = false
	if f.ws.Settings.DiffUnified {
		t.Fatal("side-by-side mode is not active")
	}
	tab.y = 0
	f.settle()
	if len(tab.newDisp) < 5 {
		t.Fatalf("new pane has too few lines: %q", tab.newDisp)
	}
	row, rowIndex := "", 0
	for i, l := range tab.newDisp {
		if displayCells(l) >= 5 {
			row, rowIndex = l, i
			break
		}
	}
	if row == "" {
		t.Fatalf("no new-pane line wide enough to select: %q", tab.newDisp)
	}
	want := row[cellOffset(row, 1):cellOffset(row, 4)]

	hit, ok := f.s.Hit("File contents")
	if !ok {
		t.Fatalf("the diff panes are not reachable text areas: %s", strings.Join(f.s.Texts(), " | "))
	}
	cellW, lineH, _ := f.win.metrics()
	sx := hit.Bounds.X + 6 + 1*cellW
	ex := hit.Bounds.X + 6 + 4*cellW
	sy := hit.Bounds.Y + float32(rowIndex)*lineH + lineH/2
	if err := f.s.Press(sx, sy); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Move(ex, sy); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Release(ex, sy); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(60*time.Millisecond, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if !tab.textSel.set || tab.textSel.kind != selNewPane {
		t.Fatalf("selection is not a new-pane one: %+v", tab.textSel)
	}
	if got := tab.selectionText(); got != want {
		t.Errorf("selectionText() = %q, want %q", got, want)
	}
	f.snap("12-side-by-side-selection")
	f.assertIdle()
}
