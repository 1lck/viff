//go:build vivahead

package viffui

import (
	"testing"
	"time"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// longDemoFile is long enough that the file viewer needs scrolling, so the map
// has something to show and a handle with travel.
func longDemoFile() string {
	var b []byte
	b = append(b, "package demo\n\n"...)
	for i := 0; i < 180; i++ {
		b = append(b, "func f"...)
		b = append(b, itoa(i)...)
		b = append(b, "() int { return 0 }\n"...)
	}
	return string(b)
}

// TestCodemapRealUserPath walks the path a reader actually takes: the map is
// switched on, then a file is opened for reading. Nothing is set up in advance,
// which is what the earlier tests did and why they passed while the feature
// looked broken.
func TestCodemapRealUserPath(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.write("demo/long.go", longDemoFile())
	f.git("add", "demo/long.go")
	f.gitDated("2026-10-23T10:00:00", "commit", "-q", "-m", "Add a long file")

	// The reader turns the map on first, the way the toolbar button does.
	f.win.ws.Settings.DiffCodemap = true
	f.win.rebuildDiffEdge()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, time.Second)

	// Then opens the file: this is the view the map belongs to.
	tab := openFileViewTab(t, f, "demo/long.go")
	if len(tab.codeMap) == 0 {
		t.Fatalf("the map was never built for an opened file: codeMap=%d lines=%d",
			len(tab.codeMap), len(tab.newDisp))
	}
	area, ok := f.win.areas[idDiffCodemap]
	if !ok || area.Width <= 0 || area.Height <= 0 {
		t.Fatalf("the map was not painted for an opened file: %+v (present=%v)", area, ok)
	}
	// It must be able to scroll: the wheel over it moves the viewer.
	before := tab.y
	if err := f.s.Scroll(ui.ScrollEvent{
		X: area.X + area.Width/2, Y: area.Y + area.Height/2, DeltaY: 120,
	}); err != nil {
		t.Fatalf("scroll over the map: %v", err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	if tab.y == before {
		t.Fatalf("the wheel over the map did not scroll the file: y=%v", tab.y)
	}
	f.assertIdle()
}

// TestCodemapOnlyInFileView is the rule this feature settled on: the map is a
// file viewer's minimap, so a diff must never give it a column, whichever diff
// mode is active and whether or not the setting is on.
func TestCodemapOnlyInFileView(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.write("demo/long.go", longDemoFile())
	f.git("add", "demo/long.go")
	f.gitDated("2026-10-24T10:00:00", "commit", "-q", "-m", "Add a long file")
	f.git("push", "-q", "origin", "main")
	f.write("demo/long.go", longDemoFile()+"\n// trailing edit\n")
	f.win.ws.Settings.DiffCodemap = true

	for _, unified := range []bool{false, true} {
		f.win.ws.Settings.DiffUnified = unified
		f.win.showDiff(vapp.DiffSource{
			Path:    "demo/long.go",
			Area:    domain.AreaUnstaged,
			Working: true,
			Kind:    domain.ChangeModified,
		}, true)
		f.settleTab()
		f.s.Settle(80*time.Millisecond, time.Second)
		if area, drawn := f.win.areas[idDiffCodemap]; drawn && area.Width > 0 {
			t.Fatalf("the map claimed a column in a diff (unified=%v): %+v", unified, area)
		}
	}

	// The same file opened for reading does show it.
	tab := openFileViewTab(t, f, "demo/long.go")
	if len(tab.codeMap) == 0 {
		t.Fatalf("the map was not built once the file was opened for reading")
	}
	if area, ok := f.win.areas[idDiffCodemap]; !ok || area.Width <= 0 {
		t.Fatalf("the map is not painted in the file viewer: %+v (present=%v)", area, ok)
	}
	f.assertIdle()
}

// TestCodemapHandleFollowsScroll checks the handle drawn over the map tracks the
// viewer: scrolling moves both together, because both read the same offset.
func TestCodemapHandleFollowsScroll(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.write("demo/long.go", longDemoFile())
	f.git("add", "demo/long.go")
	f.gitDated("2026-10-25T10:00:00", "commit", "-q", "-m", "Add a long file")
	f.win.ws.Settings.DiffCodemap = true
	tab := openFileViewTab(t, f, "demo/long.go")

	area, ok := f.win.areas[idDiffCodemap]
	if !ok || area.Height <= 0 {
		t.Fatalf("the map was not painted: %+v (present=%v)", area, ok)
	}
	_, lineH, _ := f.win.metrics()
	content := float32(len(tab.newDisp)+3) * lineH
	if content <= area.Height {
		t.Fatalf("the fixture file is too short to scroll: content=%.0f view=%.0f", content, area.Height)
	}
	// Scrolling moves the mask the map actually painted.
	top := f.win.codemapLast.y
	tab.y = (content - area.Height) / 2
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, time.Second)
	mid := f.win.codemapLast.y
	if mid <= top {
		t.Fatalf("the mask did not follow the scroll: %.1f -> %.1f", top, mid)
	}
	maxScroll := content - area.Height
	// Dragging the mask scrolls the file: the two share one offset. The press
	// lands inside the mask, so this exercises the proportional drag rather
	// than the click-to-jump branch.
	before := tab.y
	if err := f.s.Press(area.X+area.Width/2, mid+2); err != nil {
		t.Fatalf("press on the mask: %v", err)
	}
	if err := f.s.Move(area.X+area.Width/2, bottom(area)-4); err != nil {
		t.Fatalf("drag the mask: %v", err)
	}
	if err := f.s.Release(area.X+area.Width/2, bottom(area)-4); err != nil {
		t.Fatalf("release the mask: %v", err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	if tab.y <= before {
		t.Fatalf("dragging the mask did not scroll the file: %.0f -> %.0f", before, tab.y)
	}
	// A press at either end jumps to that end instead of dragging.
	if err := f.s.Press(area.X+area.Width/2, area.Y+2); err != nil {
		t.Fatalf("press at the top: %v", err)
	}
	if err := f.s.Move(area.X+area.Width/2, area.Y+2); err != nil {
		t.Fatalf("drag at the top: %v", err)
	}
	if err := f.s.Release(area.X+area.Width/2, area.Y+2); err != nil {
		t.Fatalf("release at the top: %v", err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	if tab.y != 0 {
		t.Fatalf("pressing the top of the map left y = %.0f, want 0", tab.y)
	}
	if err := f.s.Press(area.X+area.Width/2, bottom(area)-2); err != nil {
		t.Fatalf("press at the bottom: %v", err)
	}
	if err := f.s.Release(area.X+area.Width/2, bottom(area)-2); err != nil {
		t.Fatalf("release at the bottom: %v", err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	if tab.y < maxScroll*0.9 {
		t.Fatalf("pressing the bottom of the map left y = %.0f, want close to %.0f", tab.y, maxScroll)
	}
	f.assertIdle()
}

// TestCodemapToggleInFileView covers the switch while a file is open: off drops
// the map and its column, on brings them back without reloading the file.
func TestCodemapToggleInFileView(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.write("demo/long.go", longDemoFile())
	f.git("add", "demo/long.go")
	f.gitDated("2026-10-26T10:00:00", "commit", "-q", "-m", "Add a long file")
	f.win.ws.Settings.DiffCodemap = true
	tab := openFileViewTab(t, f, "demo/long.go")
	seq := tab.seq
	if len(tab.codeMap) == 0 {
		t.Fatal("the map was not built with the setting on")
	}

	f.win.ws.Settings.DiffCodemap = false
	f.win.rebuildDiffEdge()
	f.s.Settle(60*time.Millisecond, time.Second)
	if len(tab.codeMap) != 0 {
		t.Fatalf("the map survived the switch being turned off: %d rows", len(tab.codeMap))
	}
	if area, drawn := f.win.areas[idDiffCodemap]; drawn && area.Width > 0 {
		t.Fatalf("the map is still painted while off: %+v", area)
	}

	f.win.ws.Settings.DiffCodemap = true
	f.win.rebuildDiffEdge()
	f.s.Settle(60*time.Millisecond, time.Second)
	if len(tab.codeMap) == 0 {
		t.Fatal("the map was not rebuilt when the setting came back")
	}
	if tab.seq != seq {
		t.Fatalf("toggling the map reloaded the file: seq %d -> %d", seq, tab.seq)
	}
	if area, ok := f.win.areas[idDiffCodemap]; !ok || area.Width <= 0 {
		t.Fatalf("the map is not painted after the toggle: %+v (present=%v)", area, ok)
	}
	f.assertIdle()
}
