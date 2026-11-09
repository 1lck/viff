//go:build vivahead

package viffui

import (
	"testing"
	"time"
)

// TestDiffCollapseUnchangedFragments covers issue #76: a long file with a
// small edit starts folded, the fold row expands on click, and the toolbar
// toggle turns the whole behaviour off and on again.
func TestDiffCollapseUnchangedFragments(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEdgeTab(t, f)

	if !f.win.ws.Settings.DiffCollapse {
		t.Fatal("unchanged stretches are not folded by default")
	}
	if !tab.collapse {
		t.Fatal("the loaded tab did not pick up the collapse setting")
	}
	if tab.foldNew.total >= len(tab.newDisp) || tab.foldOld.total >= len(tab.oldDisp) {
		t.Fatalf("no row was folded: new %d/%d, old %d/%d",
			tab.foldNew.total, len(tab.newDisp), tab.foldOld.total, len(tab.oldDisp))
	}
	if !f.s.HasText("unchanged lines") {
		t.Fatalf("the fold row was not painted: %v", f.s.Texts())
	}
	before := tab.foldNew.total
	if err := f.s.ClickLabel("Show"); err != nil {
		t.Fatalf("click the fold row: %v", err)
	}
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if tab.foldNew.total <= before {
		t.Fatalf("the fold did not expand: %d -> %d rows", before, tab.foldNew.total)
	}
	if len(tab.expanded) == 0 {
		t.Fatal("the expanded fold was not remembered")
	}

	// The toolbar toggle disables folding for every open diff and starts fully
	// collapsed again when it comes back.
	if err := f.s.ClickLabel("Collapse Unchanged Fragments"); err != nil {
		t.Fatalf("toggle collapse off: %v", err)
	}
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if f.win.ws.Settings.DiffCollapse {
		t.Fatal("the setting stayed on after toggling it off")
	}
	if tab.foldNew.total != len(tab.newDisp) {
		t.Fatalf("folding stayed active while off: %d of %d rows", tab.foldNew.total, len(tab.newDisp))
	}
	if err := f.s.ClickLabel("Collapse Unchanged Fragments"); err != nil {
		t.Fatalf("toggle collapse on: %v", err)
	}
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if !f.win.ws.Settings.DiffCollapse {
		t.Fatal("the setting stayed off after toggling it on")
	}
	if tab.foldNew.total >= len(tab.newDisp) {
		t.Fatalf("turning folding back on did not collapse: %d of %d rows", tab.foldNew.total, len(tab.newDisp))
	}
	f.assertIdle()
}

// TestDiffCollapseInUnifiedView is the same projection in the single-column
// viewer: its context rows fold too, and one click expands the shared fold.
func TestDiffCollapseInUnifiedView(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEdgeTab(t, f)
	f.win.ws.Settings.DiffUnified = true
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if tab.foldUni.total >= len(tab.unified) {
		t.Fatalf("unified rows were not folded: %d of %d", tab.foldUni.total, len(tab.unified))
	}
	if !f.s.HasText("unchanged lines") {
		t.Fatalf("the unified fold row was not painted: %v", f.s.Texts())
	}
	before := tab.foldUni.total
	if err := f.s.ClickLabel("Show"); err != nil {
		t.Fatalf("click the unified fold row: %v", err)
	}
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if tab.foldUni.total <= before {
		t.Fatalf("the unified fold did not expand: %d -> %d rows", before, tab.foldUni.total)
	}
	f.assertIdle()
}
