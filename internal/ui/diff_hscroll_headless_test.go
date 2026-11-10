//go:build vivahead

package viffui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// These probes cover issue #80: a diff whose widest row overflows its
// narrowest pane gets one horizontal bar under the panes, and that bar scrolls
// both sides through the tab's shared offset. When the row fits, the strip is
// not claimed at all.

const diffHScrollPrefix = `func wideLine() string { return "`

func diffHScrollLine(tail string) string {
	return diffHScrollPrefix + strings.Repeat("abcdefghij", 40) + tail + `" }`
}

func openDiffHScrollTab(t *testing.T, f *uiFixture) *diffTab {
	t.Helper()
	f.write("demo/hscroll.txt", diffHScrollLine("ORIGINAL")+"\n")
	f.git("add", "demo/hscroll.txt")
	f.gitDated("2026-10-25T10:00:00", "commit", "-q", "-m", "Add a wide line")
	f.write("demo/hscroll.txt", diffHScrollLine("CHANGED")+"\n")
	f.win.showDiff(vapp.DiffSource{
		Path:    "demo/hscroll.txt",
		Area:    domain.AreaUnstaged,
		Working: true,
		Kind:    domain.ChangeModified,
	}, true)
	f.settleTab()
	f.s.Settle(60*time.Millisecond, time.Second)
	return f.win.ed.cur()
}

func diffHScrollPaint(t *testing.T, f *uiFixture) {
	t.Helper()
	f.win.Invalidate()
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
}

func diffHScrollHit(t *testing.T, f *uiFixture, id ui.ID) ui.Rect {
	t.Helper()
	for _, h := range f.s.Snapshot().Hits {
		if h.ID == id {
			return h.Bounds
		}
	}
	t.Fatalf("missing painted control %v", id)
	return ui.Rect{}
}

// diffHScrollBarHit returns the painted bottom bar, or its zero rect when it
// is hidden.
func diffHScrollBarHit(t *testing.T, f *uiFixture) ui.Rect {
	t.Helper()
	for _, h := range f.s.Snapshot().Hits {
		if h.ID == idDiffScrollX {
			return h.Bounds
		}
	}
	return ui.Rect{}
}

// diffHScrollLineXs is where the wide row is painted, in snapshot order.
func diffHScrollLineXs(t *testing.T, f *uiFixture) []float32 {
	t.Helper()
	var xs []float32
	for _, txt := range f.s.Snapshot().Texts {
		if strings.HasPrefix(txt.Value, diffHScrollPrefix) {
			xs = append(xs, txt.Bounds.X)
		}
	}
	if len(xs) == 0 {
		t.Fatal("the wide row was not painted")
	}
	slices.Sort(xs)
	return xs
}

func dragDiffHScrollBar(t *testing.T, f *uiFixture, bar ui.Rect, dx float32) {
	t.Helper()
	if _, ok := f.win.drags[idDiffScrollX]; !ok {
		t.Fatal("the bottom horizontal bar has no drag handler")
	}
	x, y := bar.X+bar.Width/2, bar.Y+bar.Height/2
	if err := f.s.Press(x, y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Move(x+dx, y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Release(x+dx, y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestDiffHorizontalScrollBarSideBySide(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openDiffHScrollTab(t, f)

	bar := diffHScrollBarHit(t, f)
	if bar.Width <= 0 || bar.Height <= 0 {
		t.Fatal("no bottom horizontal scroll bar for a diff wider than its panes")
	}
	if bar.Height > diffBarH+0.5 {
		t.Errorf("bar is %v high, want at most the reserved strip %v", bar.Height, diffBarH)
	}

	left := diffHScrollHit(t, f, idDiffLeft)
	rightP := diffHScrollHit(t, f, idDiffRight)
	// The bar owns a strip below both panes, so they end above it and the last
	// painted row cannot sit underneath the bar.
	if left.Y+left.Height > bar.Y+1 || rightP.Y+rightP.Height > bar.Y+1 {
		t.Fatalf("panes run into the bar: left %+v right %+v bar %+v", left, rightP, bar)
	}
	if bar.X < left.X-0.5 {
		t.Fatalf("bar starts left of the panes: bar %+v left %+v", bar, left)
	}
	// The right edge belongs to the change ruler, which doubles as the vertical
	// scroll bar, so the horizontal bar stops before that column.
	if marks, ok := f.win.areas[idDiffMarks]; ok && bar.X+bar.Width > marks.X+0.5 {
		t.Fatalf("bar runs under the change ruler: bar %+v ruler %+v", bar, marks)
	}

	beforeX := diffHScrollLineXs(t, f)
	if len(beforeX) != 2 {
		t.Fatalf("wide row painted %d times, want once per side: %v", len(beforeX), beforeX)
	}
	before := tab.x
	dragDiffHScrollBar(t, f, bar, 80)
	if tab.x <= before {
		t.Fatalf("dragging the bar did not move the shared offset: %v -> %v", before, tab.x)
	}
	afterX := diffHScrollLineXs(t, f)
	if len(afterX) != 2 {
		t.Fatalf("wide row painted %d times after the drag, want once per side: %v", len(afterX), afterX)
	}
	shift := afterX[0] - beforeX[0]
	if shift >= 0 {
		t.Fatalf("the left pane did not scroll left: %v -> %v", beforeX[0], afterX[0])
	}
	if got := afterX[1] - beforeX[1]; got < shift-0.51 || got > shift+0.51 {
		t.Errorf("panes scrolled by different amounts: %v and %v", shift, got)
	}
	f.snap("diff-horizontal-bar-side-by-side")
	f.assertIdle()
}

func TestDiffHorizontalScrollBarUnified(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	f.win.ws.Settings.DiffUnified = true
	tab := openDiffHScrollTab(t, f)

	bar := diffHScrollBarHit(t, f)
	if bar.Width <= 0 || bar.Height <= 0 {
		t.Fatal("no bottom horizontal scroll bar in the unified view")
	}
	pane := diffHScrollHit(t, f, idDiffRight)
	if pane.Y+pane.Height > bar.Y+1 {
		t.Fatalf("the unified pane runs into the bar: pane %+v bar %+v", pane, bar)
	}
	if bar.X < pane.X-0.5 {
		t.Fatalf("bar starts left of the unified pane: bar %+v pane %+v", bar, pane)
	}
	if marks, ok := f.win.areas[idDiffMarks]; ok && bar.X+bar.Width > marks.X+0.5 {
		t.Fatalf("bar runs under the change ruler: bar %+v ruler %+v", bar, marks)
	}

	beforeX := diffHScrollLineXs(t, f)
	before := tab.x
	dragDiffHScrollBar(t, f, bar, 80)
	if tab.x <= before {
		t.Fatalf("dragging the bar did not move the unified offset: %v -> %v", before, tab.x)
	}
	afterX := diffHScrollLineXs(t, f)
	if len(afterX) != len(beforeX) {
		t.Fatalf("wide rows painted %d times after the drag, want %d", len(afterX), len(beforeX))
	}
	for i := range afterX {
		if afterX[i] >= beforeX[i] {
			t.Errorf("unified row %d did not scroll left: %v -> %v", i, beforeX[i], afterX[i])
		}
	}
	f.snap("diff-horizontal-bar-unified")
	f.assertIdle()
}

func TestDiffHorizontalScrollBarClampsAfterPaneResize(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openDiffHScrollTab(t, f)

	// Start with a lopsided split, so widening the narrow side later makes the
	// overflow shrink rather than grow.
	tab.split = 0.82
	diffHScrollPaint(t, f)
	bar := diffHScrollBarHit(t, f)
	if bar.Width <= 0 {
		t.Fatal("no bottom horizontal scroll bar for the lopsided split")
	}
	dragDiffHScrollBar(t, f, bar, bar.Width*2)
	before := tab.x
	if before <= 0 {
		t.Fatal("the drag did not reach the end of the horizontal range")
	}

	// Back to the default split: both panes get wider, so the shared offset no
	// longer fits and must be clamped in the same paint that lays the panes out.
	tab.split = 0.5
	diffHScrollPaint(t, f)
	cellW, _, _ := f.win.metrics()
	left := diffHScrollHit(t, f, idDiffLeft)
	rightP := diffHScrollHit(t, f, idDiffRight)
	gutter := f.win.gutterWidth(max(len(tab.oldDisp), len(tab.newDisp)), cellW)
	contentW := float32(tab.maxCells)*cellW + 40
	maxX := max(0, contentW-(min(left.Width, rightP.Width)-gutter))
	if tab.x > maxX+0.5 {
		t.Fatalf("shared offset %v exceeds the new pane range %v", tab.x, maxX)
	}
	if tab.x >= before {
		t.Fatalf("the offset was not pulled back after the panes widened: %v -> %v", before, tab.x)
	}
	f.assertIdle()
}

func TestDiffHorizontalScrollBarHiddenWhenFits(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	f.write("demo/short.txt", "a\nb\n")
	f.git("add", "demo/short.txt")
	f.gitDated("2026-10-25T11:00:00", "commit", "-q", "-m", "Add a short file")
	f.write("demo/short.txt", "a\nc\n")

	for _, unified := range []bool{false, true} {
		f.win.ws.Settings.DiffUnified = unified
		f.win.showDiff(vapp.DiffSource{
			Path:    "demo/short.txt",
			Area:    domain.AreaUnstaged,
			Working: true,
			Kind:    domain.ChangeModified,
		}, true)
		f.settleTab()
		diffHScrollPaint(t, f)
		if bar := diffHScrollBarHit(t, f); bar.Width > 0 || bar.Height > 0 {
			t.Fatalf("bar shown for a diff that fits (unified=%v): %+v", unified, bar)
		}
		if _, ok := f.win.drags[idDiffScrollX]; ok {
			t.Fatalf("bar accepts drags although the widest row fits (unified=%v)", unified)
		}
	}
	f.assertIdle()
}
