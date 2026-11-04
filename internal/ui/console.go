package viffui

import (
	"strings"
	"unicode/utf8"

	"github.com/vivaui/viva/ui"

	"viff/internal/ui/icons"
)

// Console geometry. The console does not wrap a command or an output line: a
// long row is reached by scrolling horizontally, and the bar along the bottom
// shows when there is more of it to the right.
const (
	consolePadX      = 12 // text inset from the left edge
	consolePadY      = 4  // space above the first row
	consolePadBottom = 6  // space below the last row
	consolePadRight  = 16 // room kept after the widest row, clear of the bar
	consoleBarW      = 12 // right gutter reserved for the vertical bar
)

// consoleView is the Console tab's scroll state; it lives here with the console
// rather than in gitwin.go so the tab owns it. maxTop is the largest vertical
// offset the previous paint used: while the reader sat on the last row, new
// output keeps the view there; once they scroll up, their place is left alone.
type consoleView struct {
	scroll scroller
	maxTop float32
}

// reset returns the view to its start, as clearing the console does.
func (v *consoleView) reset() { *v = consoleView{} }

// consoleRow is one painted console row, measured in fixed-pitch cells.
type consoleRow struct {
	text  string
	cells int
	color ui.Color
}

// consoleRows builds the rows of the console and measures the widest one.
func (w *Window) consoleRows() (rows []consoleRow, widest int) {
	row := func(text string, c ui.Color) consoleRow {
		return consoleRow{text: text, cells: displayCells(text), color: c}
	}
	for _, e := range w.ws.Console {
		head := e.Time.Format("15:04:05") + "  " + e.Command
		c := w.pal.EditorText
		if e.Failed {
			c = w.pal.Error
		}
		rows = append(rows, row(head, c))
		if out := strings.TrimRight(e.Output, "\n"); out != "" {
			for _, l := range strings.Split(out, "\n") {
				rows = append(rows, row("          "+expandLine(l), w.pal.TextMuted))
			}
		}
	}
	for _, r := range rows {
		widest = max(widest, r.cells)
	}
	return rows, widest
}

// consoleCellWidth is the width of one cell of the console's font. It is the
// same grid Canvas.LabelRuns draws on, so a row's measured width is exactly the
// width Viva rasterizes: a plain mono label would follow the font's own
// advances instead, and a long row whose cells are pixel-snapped a little
// narrower than those advances would be cut off before its tail.
func (w *Window) consoleCellWidth(fs float32) float32 {
	cellW, _ := w.c.MonoCellSize(fs-1, w.mono)
	return cellW
}

// consoleSlice returns the cells of text in [from, to), the cell the returned
// text starts at (a wide character that straddles from starts one cell early)
// and fixed-pitch runs covering it. ASCII and non-ASCII groups are separate
// runs, so ASCII glyphs land on the grid and wide or fallback glyphs keep their
// own shaping. Tabs and control characters were already expanded.
func consoleSlice(text string, from, to int, c ui.Color) (part string, start int, runs []ui.TextRun) {
	type piece struct {
		col, start, end, cells int
		ascii                  bool
	}
	var pieces []piece
	col := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		cw := cellWidthOf(r)
		if col+cw > from && col < to {
			ascii := r >= 0x20 && r < 0x7F
			if n := len(pieces); n > 0 && pieces[n-1].ascii == ascii && pieces[n-1].end == i {
				pieces[n-1].end = i + size
				pieces[n-1].cells += cw
			} else {
				pieces = append(pieces, piece{col: col, start: i, end: i + size, cells: cw, ascii: ascii})
			}
		}
		col += cw
		i += size
	}
	if len(pieces) == 0 {
		return "", 0, nil
	}
	start = pieces[0].col
	part = text[pieces[0].start:pieces[len(pieces)-1].end]
	runs = make([]ui.TextRun, 0, len(pieces))
	for _, p := range pieces {
		runs = append(runs, ui.TextRun{
			Column: int32(p.col - start),
			Bytes:  int32(p.end - p.start),
			Cells:  int32(p.cells),
			Color:  c,
		})
	}
	return part, start, runs
}

func (w *Window) paintConsole(r ui.Rect) {
	v := &w.con
	entries := w.ws.Console
	tb := splitTop(&r, 34)
	if w.action(idConsoleClear, right(tb)-8-actionSize, tb.Y+4, icons.Delete, "Clear All", actOpts{disabled: len(entries) == 0}) {
		w.after(func() { w.ws.Console = nil })
	}
	w.p.text("Git commands run by Viff", rect(tb.X+12, tb.Y, tb.Width-60, tb.Height), fontSmall, regular, w.pal.TextSecond)
	_, lineH, fs := w.metrics()
	w.c.HitWithOptions(idConsole, r, "Console", ui.HitOptions{Role: ui.RoleGroup})
	rows, widest := w.consoleRows()
	if len(rows) == 0 {
		v.reset()
		w.p.textCenter("No commands yet", rect(r.X, r.Y+r.Height*0.35, r.Width, 22), fontUI, regular, w.pal.TextSecond)
		return
	}
	viewW := max(0, r.Width-consoleBarW)
	if viewW <= 0 {
		return
	}
	cellW := w.consoleCellWidth(fs)
	contentH := float32(len(rows))*lineH + consolePadY + consolePadBottom
	contentW := consolePadX + float32(widest)*cellW + consolePadRight
	maxTop := max(0, contentH-r.Height)
	// Follow the last row whenever the previous paint sat on it; a reader who
	// scrolled up keeps their place, including while output keeps arriving.
	if v.scroll.Y >= v.maxTop-0.5 {
		v.scroll.Y = maxTop
	}
	v.scroll.clamp(contentH, r.Height)
	v.scroll.clampX(contentW, viewW)
	v.maxTop = maxTop
	w.onScrollIn(r, func(dx, dy float32) {
		v.scroll.Y = clampF(v.scroll.Y+dy, 0, maxTop)
		v.scroll.X = clampF(v.scroll.X+dx, 0, max(0, contentW-viewW))
	})
	w.c.Clip(rect(r.X, r.Y, viewW, r.Height))
	first := max(0, int(v.scroll.Y/lineH))
	last := min(len(rows), int((v.scroll.Y+r.Height)/lineH)+1)
	// Only the cells inside the viewport are drawn: a very long row costs a
	// screenful of glyphs, not its whole length, and its label texture stays
	// small however long the line is. The extra cells cover a wide character
	// that straddles the edge and the clip.
	fromCell := max(0, int((v.scroll.X-consolePadX)/cellW))
	for i := first; i < last; i++ {
		row := rows[i]
		if row.text == "" {
			continue
		}
		toCell := min(row.cells, int((v.scroll.X+viewW-consolePadX)/cellW)+2)
		if fromCell >= toCell {
			continue
		}
		part, startCell, runs := consoleSlice(row.text, fromCell, toCell, row.color)
		if len(runs) == 0 {
			continue
		}
		x := r.X + consolePadX + float32(startCell)*cellW - v.scroll.X
		y := r.Y + consolePadY + float32(i)*lineH - v.scroll.Y
		// One spare cell of width, as the diff panes give their runs, so the
		// last glyph's ink is not clipped by the label's edge.
		width := float32(toCell-startCell+1) * cellW
		style := ui.TextStyle{Size: fs - 1, Color: row.color, Mono: true, MonoFace: w.mono}
		w.c.LabelRuns(part, rect(x, y, width, lineH), style, cellW, runs)
	}
	w.c.Unclip()
	w.vbar(scLogScrollbars.At(5), r, &v.scroll, contentH)
	w.hbar(scLogScrollbars.At(8), rect(r.X, r.Y, viewW, r.Height), &v.scroll, contentW)
}
