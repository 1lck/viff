//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// rulerFile is long enough that its changes are a small part of it, which is
// what the right edge has to convey: where the changes are in a big file.
func rulerFile(extra string) string {
	var b strings.Builder
	b.WriteString("package demo\n\n")
	for i := 0; i < 240; i++ {
		if i%7 == 0 {
			b.WriteString("\n")
			continue
		}
		b.WriteString("func f")
		b.WriteString(itoa(i))
		b.WriteString("() int { return 0 }\n")
	}
	b.WriteString(extra)
	return b.String()
}

// openEdgeTab opens a modified long file and waits for its right-edge caches.
func openEdgeTab(t *testing.T, f *uiFixture) *diffTab {
	t.Helper()
	f.write("demo/long.go", rulerFile(""))
	f.git("add", "demo/long.go")
	f.gitDated("2026-10-21T10:00:00", "commit", "-q", "-m", "Add a long file")
	f.write("demo/long.go", rulerFile("\n// a trailing edit\n"))
	f.win.showDiff(vapp.DiffSource{
		Path:    "demo/long.go",
		Area:    domain.AreaUnstaged,
		Working: true,
		Kind:    domain.ChangeModified,
	}, true)
	f.settleTab()
	f.s.Settle(60*time.Millisecond, time.Second)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		t := f.win.ed.cur()
		return t != nil && !t.loading && len(t.marksNew) > 0
	}, 10*time.Second); err != nil {
		t.Fatalf("the change marks were not built: %v", err)
	}
	return f.win.ed.cur()
}

// TestChangeMarksCoverOnlyChanges is the point of the ruler: one mark per
// changed stretch and nothing else. A file of hundreds of lines with one edit
// must produce a handful of marks, not one per line, and they must cover
// exactly the lines the diff reports as changed.
func TestChangeMarksCoverOnlyChanges(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEdgeTab(t, f)

	if len(tab.res.Blocks) == 0 {
		t.Fatal("the fixture produced no diff blocks")
	}
	if len(tab.marksNew) > len(tab.res.Blocks) {
		t.Fatalf("ruler has %d marks for %d diff blocks, so it is not grouping by change",
			len(tab.marksNew), len(tab.res.Blocks))
	}
	total, prev := 0, -1
	for _, m := range tab.marksNew {
		if m.to <= m.from {
			t.Fatalf("empty mark: %+v", m)
		}
		if m.from < prev {
			t.Fatalf("marks out of order: %+v after %d", m, prev)
		}
		prev = m.to
		total += m.to - m.from
	}
	changed := 0
	for _, b := range tab.newBlk {
		if b >= 0 {
			changed++
		}
	}
	if total != changed {
		t.Fatalf("marks cover %d lines, the diff marks %d", total, changed)
	}
	// The ruler must stay narrow: it is a strip of marks, not a picture.
	if marksWidth > 16 {
		t.Fatalf("the ruler is %d px wide, too wide for marks only", marksWidth)
	}
	f.assertIdle()
}

// TestChangeMarksTakeTheScrollBarColumn checks the layout fix: with the ruler
// on it owns the right edge and the overlay scroll bar is not drawn there, so
// the two no longer sit side by side on the same edge.
func TestChangeMarksTakeTheScrollBarColumn(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	openEdgeTab(t, f)

	marks, ok := f.win.areas[idDiffMarks]
	if !ok || marks.Width <= 0 || marks.Height <= 0 {
		t.Fatalf("the ruler was not painted: %+v (present=%v)", marks, ok)
	}
	if _, drawn := f.win.drags[idDiffMarks]; !drawn {
		t.Fatal("the ruler registered no pointer handler, so it cannot stand in for the bar")
	}
	// The ruler's drag target is what the reader uses to scroll, and the
	// overlay bar handler must not cover the same strip.
	if bar, has := f.win.areas[idDiffScrollR]; has && bar.Width > 0 {
		if bar.X+bar.Width > marks.X+1 && bar.X < right(marks)-1 {
			t.Fatalf("the scroll bar still overlaps the ruler: bar %+v ruler %+v", bar, marks)
		}
	}
	f.assertIdle()
}

// TestCodemapIsOptional checks the second switch: the code map is off by
// default, its cache appears only when on, and it takes a column of its own
// inward of the ruler.
func TestCodemapIsOptional(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	if f.win.ws.Settings.DiffCodemap {
		t.Fatal("the code map is on by default; it is meant to be opt-in")
	}
	if !f.win.ws.Settings.DiffMarks {
		t.Fatal("the change marks are off by default")
	}
	tab := openEdgeTab(t, f)
	if len(tab.mapNew) != 0 {
		t.Fatalf("a code map was built while the setting is off: %d lines", len(tab.mapNew))
	}
	marks, ok := f.win.areas[idDiffMarks]
	if !ok {
		t.Fatal("the ruler was not painted")
	}
	if area, drawn := f.win.areas[idDiffCodemap]; drawn && area.Width > 0 {
		t.Fatalf("the code map is painted while off: %+v", area)
	}

	// Turn it on: the cache is built from data the tab already holds, and the
	// column appears inward of the ruler.
	seq := tab.seq
	f.win.ws.Settings.DiffCodemap = true
	f.win.rebuildDiffEdge()
	f.s.Settle(60*time.Millisecond, time.Second)
	if len(tab.mapNew) != len(tab.newDisp) {
		t.Fatalf("code map has %d lines for %d display lines", len(tab.mapNew), len(tab.newDisp))
	}
	if tab.seq != seq {
		t.Fatalf("enabling the code map reloaded the diff: seq %d -> %d", seq, tab.seq)
	}
	area, ok := f.win.areas[idDiffCodemap]
	if !ok || area.Width <= 0 {
		t.Fatalf("the code map is not painted after enabling it: %+v (present=%v)", area, ok)
	}
	if right(area) > marks.X+1 {
		t.Fatalf("the code map overlaps the ruler: map %+v ruler %+v", area, marks)
	}
	// Its shape carries syntax, which is what makes it a code map and not a
	// second ruler.
	kinds := map[tokKind]bool{}
	for _, ml := range tab.mapNew {
		for _, r := range ml.runs {
			kinds[r.kind] = true
		}
	}
	if !kinds[tkKeyword] && !kinds[tkPlain] {
		t.Fatalf("the code map carries no usable token kinds: %v", kinds)
	}
	f.assertIdle()
}

// TestChangeMarksDragScrolls checks the ruler's own interaction: a press on it
// scrolls the editor to that place and never past the end.
func TestChangeMarksDragScrolls(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEdgeTab(t, f)

	area, ok := f.win.areas[idDiffMarks]
	if !ok || area.Height <= 0 {
		t.Fatalf("the ruler was not painted: %+v (present=%v)", area, ok)
	}
	if err := f.s.Press(area.X+area.Width/2, bottom(area)-3); err != nil {
		t.Fatalf("press on the ruler: %v", err)
	}
	if err := f.s.Release(area.X+area.Width/2, bottom(area)-3); err != nil {
		t.Fatalf("release on the ruler: %v", err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	if tab.y <= 0 {
		t.Fatalf("pressing the ruler did not scroll the editor: y=%v", tab.y)
	}
	_, lineH, _ := f.win.metrics()
	content := float32(len(tab.newDisp)+3) * lineH
	if tab.y > content {
		t.Fatalf("the ruler scrolled past the end: %v > %v", tab.y, content)
	}
	f.assertIdle()
}

// TestChangeMarksToggle covers the first switch: off drops the marks and hands
// the edge back to the scroll bar, on rebuilds them without reloading the diff.
func TestChangeMarksToggle(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	tab := openEdgeTab(t, f)
	seq := tab.seq

	f.win.ws.Settings.DiffMarks = false
	f.win.rebuildDiffEdge()
	f.s.Settle(60*time.Millisecond, time.Second)
	if len(tab.marksNew) != 0 || len(tab.marksOld) != 0 {
		t.Fatalf("the marks survived the toggle: new=%d old=%d", len(tab.marksNew), len(tab.marksOld))
	}
	if area, drawn := f.win.areas[idDiffMarks]; drawn && area.Width > 0 {
		t.Fatalf("the ruler is still painted while off: %+v", area)
	}

	f.win.ws.Settings.DiffMarks = true
	f.win.rebuildDiffEdge()
	f.s.Settle(60*time.Millisecond, time.Second)
	if len(tab.marksNew) == 0 {
		t.Fatal("the marks were not rebuilt when the ruler came back")
	}
	if tab.seq != seq {
		t.Fatalf("toggling the ruler reloaded the diff: seq %d -> %d", seq, tab.seq)
	}
	if area, ok := f.win.areas[idDiffMarks]; !ok || area.Width <= 0 {
		t.Fatalf("the ruler is not painted after the toggle: %+v (present=%v)", area, ok)
	}
	f.assertIdle()
}
