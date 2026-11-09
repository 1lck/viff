//go:build vivahead

package viffui

import (
	"testing"
	"time"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// TestDiffPaneResize covers both ways to move the side-by-side splitter: drag
// it with the pointer and nudge it with the arrow keys once focused. The ratio
// belongs to the tab, so it is remembered until the tab reloads.
func TestDiffPaneResize(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEdgeTab(t, f)

	hit, ok := f.s.Hit("Resize diff panes (Left / Right)")
	if !ok {
		t.Fatal("diff resize handle is missing")
	}
	// The divider spans the content area; its top is above every block button,
	// so this point cannot be swallowed by a staging action.
	x := hit.Bounds.X + hit.Bounds.Width/2
	y := hit.Bounds.Y + 10
	if err := f.s.Press(x, y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Move(x+120, y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Release(x+120, y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if tab.split <= 0.5 || tab.split >= 1 {
		t.Fatalf("dragged pane fraction = %v, want a wider left pane", tab.split)
	}
	before := tab.split
	if err := f.s.Keys("left"); err != nil {
		t.Fatal(err)
	}
	if tab.split >= before {
		t.Fatalf("left key did not shrink the left pane: %v -> %v", before, tab.split)
	}
	if err := f.s.Keys("right"); err != nil {
		t.Fatal(err)
	}
	if tab.split <= before-0.001 {
		t.Fatalf("right key did not restore the left pane: %v -> %v", before, tab.split)
	}
	if len(tab.res.Blocks) == 0 || !f.win.canApply(tab) {
		t.Fatal("fixture must expose a staging action")
	}
	if _, ok := f.win.drags[idDiffResize]; !ok {
		t.Fatal("resize handle lost its drag handler")
	}
	f.assertIdle()
}

// TestDiffPaneResizeIsPerTab pins the ratio to the tab: opening another diff
// must not inherit the previous split.
func TestDiffPaneResizeIsPerTab(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	first := openEdgeTab(t, f)
	first.split = 0.7
	f.s.Settle(40*time.Millisecond, time.Second)

	f.win.showDiff(vapp.DiffSource{
		Path: "main.go", Area: domain.AreaUnstaged, Working: true, Kind: domain.ChangeModified,
	}, true)
	f.settleTab()
	second := f.win.ed.cur()
	if second == first {
		t.Fatal("the same tab was reused, so the test cannot observe a fresh tab")
	}
	if second.split != 0 {
		t.Fatalf("new tab inherited split %v, want the default", second.split)
	}
	f.assertIdle()
}
