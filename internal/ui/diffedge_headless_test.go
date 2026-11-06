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

// openFileViewTab opens a working-tree file in the read-only viewer and waits
// for it to load. This is the path that shows the code map: the map belongs to
// the file viewer, not to a diff.
func openFileViewTab(t *testing.T, f *uiFixture, path string) *diffTab {
	t.Helper()
	f.win.openFileView(path, "", false)
	f.settleTab()
	f.s.Settle(80*time.Millisecond, time.Second)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return tab != nil && !tab.loading && tab.src.View && tab.src.Path == path
	}, 10*time.Second); err != nil {
		t.Fatalf("the file viewer did not open %s: %v", path, err)
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
// default, its cache appears only when on, and it draws only in the file viewer
// — a diff already shows its changes through the ruler, so it claims no width
// there.
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
	// A diff first: with the map off nothing is built, and nothing is painted.
	tab := openEdgeTab(t, f)
	if len(tab.codeMap) != 0 {
		t.Fatalf("a code map was built while the setting is off: %d rows", len(tab.codeMap))
	}
	if _, ok := f.win.areas[idDiffMarks]; !ok {
		t.Fatal("the ruler was not painted")
	}
	if area, drawn := f.win.areas[idDiffCodemap]; drawn && area.Width > 0 {
		t.Fatalf("the code map is painted while off: %+v", area)
	}

	// On, but still a diff: the map belongs to the file viewer, so a diff must
	// not give it a column however the setting stands.
	seq := tab.seq
	f.win.ws.Settings.DiffCodemap = true
	f.win.rebuildDiffEdge()
	f.s.Settle(60*time.Millisecond, time.Second)
	if tab.seq != seq {
		t.Fatalf("enabling the code map reloaded the diff: seq %d -> %d", seq, tab.seq)
	}
	if area, drawn := f.win.areas[idDiffCodemap]; drawn && area.Width > 0 {
		t.Fatalf("the code map drew beside a diff: %+v", area)
	}

	// Open the file itself: now the map appears, and its rows follow that file's
	// lines. The file viewer has no change ruler, so the map owns the trailing
	// edge outright.
	ft := openFileViewTab(t, f, "demo/long.go")
	if len(ft.codeMap) != len(ft.newDisp) {
		t.Fatalf("code map has %d rows for %d file lines", len(ft.codeMap), len(ft.newDisp))
	}
	area, ok := f.win.areas[idDiffCodemap]
	if !ok || area.Width <= 0 {
		t.Fatalf("the code map is not painted in the file viewer: %+v (present=%v)", area, ok)
	}
	if ruler, drawn := f.win.areas[idDiffMarks]; drawn && ruler.Width > 0 {
		t.Fatalf("the file viewer drew a change ruler at %+v; the map owns that edge", ruler)
	}
	// Its shape carries syntax, which is what makes it a code map and not a
	// second ruler.
	kinds := map[tokKind]bool{}
	for _, ml := range ft.codeMap {
		for _, r := range ml.runs {
			kinds[r.kind] = true
		}
	}
	if !kinds[tkKeyword] && !kinds[tkPlain] {
		t.Fatalf("the code map carries no usable token kinds: %v", kinds)
	}
	f.assertIdle()
}

// TestCodemapDrawsRealTextShape is the point of the rewrite: the map is built
// from character columns the way VS Code's minimap is, so indentation is blank
// and a line ends where its text ends instead of stretching across the column.
func TestCodemapDrawsRealTextShape(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.write("demo/shape.go", "package demo\n\nfunc a() {\n\treturn\n}\n")
	f.git("add", "demo/shape.go")
	f.gitDated("2026-10-22T10:00:00", "commit", "-q", "-m", "Add a short file")
	f.win.ws.Settings.DiffCodemap = true
	tab := openFileViewTab(t, f, "demo/shape.go")

	if len(tab.codeMap) != len(tab.newDisp) {
		t.Fatalf("code map has %d rows for %d file lines", len(tab.codeMap), len(tab.newDisp))
	}

	// An indented line must not start at column zero: the indent occupies cells
	// that paint nothing, so its first run begins after them.
	indented := -1
	for i, line := range tab.newDisp {
		if strings.HasPrefix(line, "    return") {
			indented = i
			break
		}
	}
	if indented < 0 {
		t.Fatalf("the fixture lost its indented line: %q", tab.newDisp)
	}
	ml := tab.codeMap[indented]
	if len(ml.runs) == 0 {
		t.Fatalf("the indented line painted nothing: %+v", ml)
	}
	if ml.runs[0].from == 0 {
		t.Fatalf("the indent was painted as code: first run starts at column %d", ml.runs[0].from)
	}
	// And no run may reach past the column limit instead of stretching.
	for i, ml := range tab.codeMap {
		for _, run := range ml.runs {
			if run.to > codemapMaxCols || run.from < 0 || run.to <= run.from {
				t.Fatalf("row %d has a run outside the map: %+v", i, run)
			}
		}
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
