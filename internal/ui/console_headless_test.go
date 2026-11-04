//go:build vivahead

package viffui

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
)

// The Console tab does not wrap long command lines or output. These probes run
// the real window headlessly: they scroll it, then read the geometry the paint
// pass produced (painted text bounds, scrollbars and scroll offsets).

// consoleTestPaint repaints the window and settles the frame.
func consoleTestPaint(t *testing.T, f *uiFixture) {
	t.Helper()
	f.win.Invalidate()
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
}

// consoleTestRect returns the painted console area.
func consoleTestRect(t *testing.T, f *uiFixture) ui.Rect {
	t.Helper()
	for _, h := range f.s.Snapshot().Hits {
		if h.ID == idConsole {
			return h.Bounds
		}
	}
	t.Fatal("console area was not painted")
	return ui.Rect{}
}

// consoleTestWheel sends one wheel event to the middle of the console and
// settles the resulting paint.
func consoleTestWheel(t *testing.T, f *uiFixture, dx, dy float32) {
	t.Helper()
	r := consoleTestRect(t, f)
	if err := f.s.Scroll(ui.ScrollEvent{X: r.X + r.Width/2, Y: r.Y + r.Height/2, DeltaX: dx, DeltaY: dy}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
}

// consoleTestText returns the painted text that contains marker, before
// clipping, so a row can be measured even when part of it is off screen.
func consoleTestText(t *testing.T, f *uiFixture, marker string) ui.Text {
	t.Helper()
	for _, text := range f.s.Snapshot().Texts {
		if strings.Contains(text.Value, marker) {
			return text
		}
	}
	t.Fatalf("no painted console text contains %q", marker)
	return ui.Text{}
}

// consoleTestBar reports whether a scroll bar is painted in the given band of
// the console, and whether one of its thumbs is actually drawn there.
func consoleTestBar(t *testing.T, f *uiFixture, r ui.Rect, horizontal bool) (hit, thumb bool) {
	t.Helper()
	for _, h := range f.s.Snapshot().Hits {
		if h.Label != "Scroll bar" || h.Bounds.X < r.X-1 || h.Bounds.X+h.Bounds.Width > r.X+r.Width+1 {
			continue
		}
		if horizontal && h.Bounds.Width > r.Width/2 && h.Bounds.Y >= r.Y+r.Height-14 {
			hit = true
		}
		if !horizontal && h.Bounds.Width <= 12.5 && h.Bounds.X >= r.X+r.Width-14 {
			hit = true
		}
	}
	for _, p := range f.s.Snapshot().Primitives {
		if p.Radius <= 0 || p.Color != f.win.pal.ScrollThumb {
			continue
		}
		if p.Bounds.X < r.X-1 || p.Bounds.X+p.Bounds.Width > r.X+r.Width+1 {
			continue
		}
		if horizontal && p.Bounds.Y >= r.Y+r.Height-14 && p.Bounds.Width > 20 {
			thumb = true
		}
		if !horizontal && p.Bounds.Width <= 12.5 && p.Bounds.X >= r.X+r.Width-14 {
			thumb = true
		}
	}
	return hit, thumb
}

// consoleTestInside reports whether the row containing marker is fully inside
// the console area.
func consoleTestInside(t *testing.T, f *uiFixture, r ui.Rect, marker string) bool {
	t.Helper()
	for _, text := range f.s.Snapshot().Texts {
		if !strings.Contains(text.Value, marker) {
			continue
		}
		return text.Bounds.Y >= r.Y-0.5 && text.Bounds.Y+text.Bounds.Height <= r.Y+r.Height+0.5
	}
	return false
}

func TestConsoleHeadlessLongLineScrollsToTail(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	const tail = "TAIL_SENTINEL_CONSOLE_677"
	long := strings.Repeat("long-output-", 90) + tail
	row := "          " + long
	f.ws.Console = []vapp.ConsoleEntry{{Time: time.Now(), Command: "git audit", Output: long}}
	f.win.gitTab = 1
	consoleTestPaint(t, f)
	r := consoleTestRect(t, f)
	if r.Width <= 0 || r.Height <= 0 {
		t.Fatalf("console area has no size: %+v", r)
	}
	cellW := consoleTestCellWidth(t, f)

	// The row is not wrapped or ellipsized: only its left screenful is
	// painted, and the tail is off to the right.
	if f.win.con.scroll.X != 0 {
		t.Fatalf("console starts horizontally scrolled to %v", f.win.con.scroll.X)
	}
	first := consoleTestText(t, f, "long-output-")
	if want := r.X + consolePadX; first.Bounds.X != want {
		t.Errorf("unscrolled row starts at %v, want %v", first.Bounds.X, want)
	}
	if !strings.HasPrefix(row, first.Value) || len(first.Value) >= len(row) {
		t.Errorf("painted row is not the visible prefix of the long line: %q", first.Value)
	}
	if strings.Contains(first.Value, tail) {
		t.Error("tail is painted before scrolling to the right")
	}
	if hit, thumb := consoleTestBar(t, f, r, true); !hit || !thumb {
		t.Errorf("no visible horizontal scrollbar for an overflowing row (hit=%v thumb=%v)", hit, thumb)
	}

	// Scroll right: the tail is painted on the fixed-pitch grid and the scroll
	// stops exactly where the last cell ends, so the tail is truly visible.
	consoleTestWheel(t, f, 10000, 0)
	if f.win.con.scroll.X == 0 {
		t.Fatal("#677 UNMET: console has no horizontal scrolling for long rows")
	}
	last := consoleTestText(t, f, tail)
	if last.Bounds.X >= first.Bounds.X {
		t.Errorf("tail did not move left when scrolled: x %v -> %v", first.Bounds.X, last.Bounds.X)
	}
	if !strings.HasSuffix(row, last.Value) || !strings.HasSuffix(last.Value, tail) {
		t.Errorf("scrolled row is not the visible end of the long line: %q", last.Value)
	}
	end := last.Bounds.X + float32(displayCells(last.Value))*cellW
	if wantEnd := r.X + r.Width - consoleBarW - consolePadRight; end < wantEnd-cellW || end > wantEnd+cellW {
		t.Errorf("horizontal scroll ends the row at %v, want about %v; the tail would be cut off or blank", end, wantEnd)
	}
	if last.RunCount == 0 {
		t.Error("console row is not painted as a fixed-pitch grid line")
	}

	f.snap("console-tail-visible")

	// Scrolling back left returns to the start.
	consoleTestWheel(t, f, -10000, 0)
	if f.win.con.scroll.X != 0 {
		t.Errorf("horizontal scroll did not return to the start: %v", f.win.con.scroll.X)
	}
	back := consoleTestText(t, f, "long-output-")
	if back.Bounds.X != first.Bounds.X || back.Value != first.Value {
		t.Errorf("row did not return to its start: %q at %v, want %q at %v", back.Value, back.Bounds.X, first.Value, first.Bounds.X)
	}
}

// consoleTestGridRow returns the console's output row: the grid line just below
// the command header.
func consoleTestGridRow(t *testing.T, f *uiFixture) ui.Text {
	t.Helper()
	r := consoleTestRect(t, f)
	_, lineH, _ := f.win.metrics()
	y := r.Y + consolePadY + lineH - f.win.con.scroll.Y
	for _, text := range f.s.Snapshot().Texts {
		if text.RunCount == 0 || math.Abs(float64(text.Bounds.Y-y)) > 0.5 {
			continue
		}
		if text.Bounds.X+text.Bounds.Width < r.X || text.Bounds.X > r.X+r.Width {
			continue
		}
		return text
	}
	t.Fatal("console output row was not painted as a grid line")
	return ui.Text{}
}

// TestConsoleHeadlessTabUnicodeScrollsToTail covers the parts of a row that
// bytes do not measure: tabs are expanded before the row is built, and CJK
// characters and emoji take two cells, so a horizontally scrolled slice has to
// start and end on whole characters and stay on the cell grid.
func TestConsoleHeadlessTabUnicodeScrollsToTail(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	const tail = "TAIL_SENTINEL_TAB_877"
	line := strings.Repeat("列\t", 8) + strings.Repeat("🙂x\t", 8) + strings.Repeat("wide-\t", 20) + tail
	f.ws.Console = []vapp.ConsoleEntry{{Time: time.Now(), Command: "git log", Output: line}}
	f.win.gitTab = 1
	consoleTestPaint(t, f)
	r := consoleTestRect(t, f)
	cellW := consoleTestCellWidth(t, f)

	row := "          " + expandLine(line)
	if strings.ContainsRune(row, '\t') {
		t.Fatalf("tab was not expanded before the row was built: %q", row)
	}
	if got := displayCells(row); got <= 240 {
		t.Fatalf("row measured %d cells, want its wide characters counted as two", got)
	}
	if f.win.con.scroll.X != 0 {
		t.Fatalf("console starts horizontally scrolled to %v", f.win.con.scroll.X)
	}
	if strings.Contains(consoleTestText(t, f, "列").Value, tail) {
		t.Fatal("tail is painted before scrolling to the right")
	}

	// check reads the painted output row and holds it to the expanded row.
	check := func() (text ui.Text) {
		t.Helper()
		text = consoleTestGridRow(t, f)
		if !utf8.ValidString(text.Value) || strings.ContainsRune(text.Value, utf8.RuneError) {
			t.Fatalf("painted row is not whole characters: %q", text.Value)
		}
		if !strings.Contains(row, text.Value) {
			t.Fatalf("painted row is not a slice of the expanded row: %q", text.Value)
		}
		if strings.ContainsRune(text.Value, '\t') {
			t.Fatalf("painted row shows a raw tab: %q", text.Value)
		}
		cells := float32(displayCells(text.Value))
		if text.Bounds.Width < cells*cellW || text.Bounds.Width > (cells+2)*cellW {
			t.Errorf("painted %d cells wide %v, want a cell each plus slack", int(cells), text.Bounds.Width)
		}
		return text
	}

	check() // the start of the row

	// Scrolling can land inside a wide character: the slice must begin with
	// the whole character, one cell early, not with half of it.
	f.win.con.scroll.X = consolePadX + cellW*11.5
	consoleTestPaint(t, f)
	if text := check(); !strings.HasPrefix(text.Value, "列") {
		t.Errorf("slice starting inside a wide character dropped it: %q", text.Value)
	}

	consoleTestWheel(t, f, 10000, 0)
	if f.win.con.scroll.X == 0 {
		t.Fatal("console has no horizontal scrolling for a wide-character row")
	}
	last := check()
	if !strings.HasSuffix(last.Value, tail) {
		t.Fatalf("tail is not painted at the end of the scroll: %q", last.Value)
	}
	end := last.Bounds.X + float32(displayCells(last.Value))*cellW
	if want := r.X + r.Width - consoleBarW - consolePadRight; end < want-cellW || end > want+cellW {
		t.Errorf("wide-character row ends at %v, want about %v", end, want)
	}
}

func TestConsoleHeadlessVerticalFollow(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	var out strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&out, "row %02d\n", i)
	}
	f.ws.Console = []vapp.ConsoleEntry{{Time: time.Now(), Command: "git audit", Output: out.String()}}
	f.win.gitTab = 1
	consoleTestPaint(t, f)
	r := consoleTestRect(t, f)

	// New output starts at the bottom, like a terminal, and its last row is
	// on screen.
	if f.win.con.scroll.Y <= 0 {
		t.Fatalf("console did not follow its newest output: scroll %v", f.win.con.scroll.Y)
	}
	if !consoleTestInside(t, f, r, "row 59") {
		t.Error("newest output row is not visible after the console follows it")
	}
	if hit, thumb := consoleTestBar(t, f, r, false); !hit || !thumb {
		t.Errorf("no visible vertical scrollbar for overflowing output (hit=%v thumb=%v)", hit, thumb)
	}

	// Scrolling up takes the reader away from the bottom.
	top := f.win.con.scroll.Y
	consoleTestWheel(t, f, 0, -150)
	up := f.win.con.scroll.Y
	if up >= top {
		t.Fatalf("vertical scroll did not move up: %v -> %v", top, up)
	}
	if consoleTestInside(t, f, r, "row 59") {
		t.Error("scrolling up left the last row in view")
	}

	// Output that arrives while the reader is scrolled up must not drag the
	// view back to the bottom.
	f.ws.Console = append(f.ws.Console, vapp.ConsoleEntry{Time: time.Now(), Command: "git fetch", Output: "tail-after-scroll"})
	consoleTestPaint(t, f)
	if f.win.con.scroll.Y != up {
		t.Errorf("new output moved the reader's place: scroll %v, want %v", f.win.con.scroll.Y, up)
	}
	if consoleTestInside(t, f, r, "tail-after-scroll") {
		t.Error("new output was forced into a scrolled-up view")
	}

	// Returning to the bottom resumes following: the next output is shown.
	consoleTestWheel(t, f, 0, 10000)
	atEnd := f.win.con.scroll.Y
	if atEnd <= up {
		t.Fatalf("scrolling down did not reach the bottom: %v -> %v", up, atEnd)
	}
	f.ws.Console = append(f.ws.Console, vapp.ConsoleEntry{Time: time.Now(), Command: "git push", Output: "tail-after-follow"})
	consoleTestPaint(t, f)
	if f.win.con.scroll.Y <= atEnd {
		t.Errorf("console stopped following after returning to the bottom: %v -> %v", atEnd, f.win.con.scroll.Y)
	}
	if !consoleTestInside(t, f, r, "tail-after-follow") {
		t.Error("output after returning to the bottom is not visible")
	}
}

func TestConsoleHeadlessClearResetsScrolling(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	var out strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&out, "long-output-row-%02d-%s\n", i, strings.Repeat("x", 240))
	}
	entry := vapp.ConsoleEntry{Time: time.Now(), Command: "git audit", Output: out.String()}
	f.ws.Console = []vapp.ConsoleEntry{entry}
	f.win.gitTab = 1
	consoleTestPaint(t, f)
	r := consoleTestRect(t, f)
	consoleTestWheel(t, f, 1000, -150)
	if f.win.con.scroll.X == 0 || f.win.con.scroll.Y == 0 {
		t.Fatalf("setup did not scroll both ways: %+v", f.win.con.scroll)
	}

	// A short output leaves nothing to scroll: offsets return to the start so
	// no empty band is left above or beside the text.
	f.ws.Console = []vapp.ConsoleEntry{{Time: time.Now(), Command: "git status", Output: "short output"}}
	consoleTestPaint(t, f)
	if f.win.con.scroll.X != 0 || f.win.con.scroll.Y != 0 {
		t.Errorf("short output kept the old offsets: %+v", f.win.con.scroll)
	}
	if !f.s.HasText("short output") {
		t.Error("short output was not painted")
	}
	if hit, _ := consoleTestBar(t, f, r, true); hit {
		t.Error("horizontal scrollbar is still offered when nothing overflows")
	}

	// Scrolling again and clearing the console resets both offsets and shows
	// the empty state.
	f.ws.Console = []vapp.ConsoleEntry{entry}
	consoleTestPaint(t, f)
	consoleTestWheel(t, f, 1000, -150)
	if f.win.con.scroll.X == 0 {
		t.Fatal("setup did not scroll horizontally")
	}
	f.click("Clear All")
	f.click("Clear") // Clear All asks for confirmation first
	consoleTestPaint(t, f)
	if len(f.ws.Console) != 0 {
		t.Fatalf("Clear All did not empty the console: %d entries", len(f.ws.Console))
	}
	if f.win.con.scroll.X != 0 || f.win.con.scroll.Y != 0 {
		t.Errorf("clearing the console did not reset its scroll: %+v", f.win.con.scroll)
	}
	if !f.s.HasText("No commands yet") {
		t.Error("cleared console does not show its empty state")
	}
	if hit, thumb := consoleTestBar(t, f, r, true); hit || thumb {
		t.Error("cleared console still paints a scrollbar")
	}

	// Output after a clear starts at the top left again.
	f.ws.Console = []vapp.ConsoleEntry{{Time: time.Now(), Command: "git status", Output: "fresh output"}}
	consoleTestPaint(t, f)
	if f.win.con.scroll.X != 0 {
		t.Errorf("output after a clear starts scrolled horizontally: %v", f.win.con.scroll.X)
	}
	fresh := consoleTestText(t, f, "fresh output")
	if want := r.X + consolePadX; fresh.Bounds.X != want {
		t.Errorf("output after a clear starts at %v, want %v", fresh.Bounds.X, want)
	}
}

// consoleTestCellWidth is the fixed-pitch cell width the console paints with.
func consoleTestCellWidth(t *testing.T, f *uiFixture) float32 {
	t.Helper()
	_, _, fs := f.win.metrics()
	if f.win.c == nil {
		t.Fatal("console was not painted")
	}
	return f.win.consoleCellWidth(fs)
}

// consoleTestPixels returns the image the window currently renders, and its
// device pixels per logical pixel.
func consoleTestPixels(t *testing.T, f *uiFixture) (*image.RGBA, float32) {
	t.Helper()
	img, err := f.s.Image()
	if err != nil {
		t.Fatal(err)
	}
	scale := float32(img.Bounds().Dx()) / float32(f.win.size.Width)
	if scale <= 0 {
		t.Fatalf("bad screenshot scale from %v px for a %v px window", img.Bounds().Dx(), f.win.size.Width)
	}
	return img, scale
}

// consoleTestBandDiff counts the device pixels that differ between two frames
// inside one console row band, and reports the leftmost and rightmost of them.
func consoleTestBandDiff(a, b *image.RGBA, x0, y0, x1, y1 int) (count, first, last int) {
	first, last = -1, -1
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			ia, ib := a.PixOffset(x, y), b.PixOffset(x, y)
			if a.Pix[ia] == b.Pix[ib] && a.Pix[ia+1] == b.Pix[ib+1] && a.Pix[ia+2] == b.Pix[ib+2] && a.Pix[ia+3] == b.Pix[ib+3] {
				continue
			}
			count++
			if first < 0 || x < first {
				first = x
			}
			if x > last {
				last = x
			}
		}
	}
	return count, first, last
}

// TestConsoleHeadlessLongLineTailPixels checks the pixels, not just the label
// geometry: two console lines that differ only in their tail must differ on
// screen in that tail. A row drawn through a plain mono label had its tail
// rasterized outside the label texture, so both frames looked the same.
func TestConsoleHeadlessLongLineTailPixels(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	prefix := strings.Repeat("long-output-", 90)
	tailA := "TAIL_SENTINEL_CONSOLE_677"
	tailB := "ZZZZ_SENTINEL_CONSOLE_678" // same cells as tailA, different glyphs
	if displayCells(tailA) != displayCells(tailB) {
		t.Fatalf("test tails differ in width: %d vs %d cells", displayCells(tailA), displayCells(tailB))
	}
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	entry := func(tail string) vapp.ConsoleEntry {
		return vapp.ConsoleEntry{Time: at, Command: "git audit", Output: prefix + tail}
	}

	f.ws.Console = []vapp.ConsoleEntry{entry(tailA)}
	f.win.gitTab = 1
	consoleTestPaint(t, f)
	f.win.con.scroll.X = 1e9 // ask for the end without waiting on wheel steps
	consoleTestPaint(t, f)
	r := consoleTestRect(t, f)
	cellW := consoleTestCellWidth(t, f)
	if f.win.con.scroll.X == 0 {
		t.Fatal("console did not scroll horizontally")
	}
	rowA := consoleTestText(t, f, tailA)
	imgA, scale := consoleTestPixels(t, f)
	f.snap("console-tail-sentinel")

	// The same row, same length, with a different tail.
	f.ws.Console = []vapp.ConsoleEntry{entry(tailB)}
	consoleTestPaint(t, f)
	if f.win.con.scroll.X != 1e9 && f.win.con.scroll.X <= 0 {
		t.Fatalf("console lost its horizontal position: %v", f.win.con.scroll.X)
	}
	if rowB := consoleTestText(t, f, tailB); rowB.Bounds.Y != rowA.Bounds.Y {
		t.Fatalf("frames are not comparable: row y %v vs %v", rowA.Bounds.Y, rowB.Bounds.Y)
	}
	imgB, _ := consoleTestPixels(t, f)
	f.snap("console-tail-zzzz")

	_, lineH, _ := f.win.metrics()
	x0 := int(math.Floor(float64(r.X) * float64(scale)))
	x1 := int(math.Ceil(float64(r.X+r.Width) * float64(scale)))
	y0 := int(math.Floor(float64(rowA.Bounds.Y) * float64(scale)))
	y1 := int(math.Ceil(float64(rowA.Bounds.Y+lineH) * float64(scale)))
	diffs, firstX, lastX := consoleTestBandDiff(imgA, imgB, x0, y0, x1, y1)
	if diffs == 0 {
		t.Fatal("the two tails render identically: the tail is outside the rasterized row (it is cut off)")
	}
	// The differing pixels must sit where the tail is, not somewhere else.
	tailCell := 10 + displayCells(prefix) // the row's "          " indent, then the long line
	tailX := float64(r.X+consolePadX+float32(tailCell)*cellW-f.win.con.scroll.X) * float64(scale)
	if firstX < int(math.Floor(tailX))-2*int(math.Ceil(float64(scale))) {
		t.Errorf("pixels differ %d px before the tail starts at %v: the row is shifted", firstX, int(tailX))
	}
	if endX := float64(r.X+r.Width-consoleBarW-consolePadRight) * float64(scale); lastX > int(math.Ceil(endX))+1 {
		t.Errorf("pixels differ %d px past the row's end at %v", lastX, int(endX))
	}
	if tailX < float64(r.X)*float64(scale) || tailX > float64(r.X+r.Width)*float64(scale) {
		t.Errorf("tail is outside the console viewport: device x %v", tailX)
	}
}
