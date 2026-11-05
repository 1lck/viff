package viffui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vivaui/viva/ui"
)

// Text selection in the read-only file view.
//
// The viewer paints expanded display lines in fixed cells: tabs become spaces
// (four-column stops) and control characters become '·' when the side is
// prepared (expandLine). A selection is therefore kept in those display cells
// instead of bytes: selPoint.line indexes t.newDisp and selPoint.cell counts
// cells before the caret. Copying extracts the same expanded text, so the
// clipboard matches what is highlighted; a tab selected in the viewer arrives
// as the spaces it occupies, not as a tab byte. Text that was already drawn
// capped at the visible width is still copied in full.
//
// The state lives on the tab and never outlives the content it indexes: it is
// cleared on a reload, on a tab switch or close and, because a repository
// change drops the tabs, on a repository switch.

// selPoint is a caret position in the file view: a line of t.newDisp and a
// cell of that display line (0 is before its first cell).
type selPoint struct {
	line int
	cell int
}

// textSel is an anchor/focus pair with its focus flag. A click without a drag
// leaves anchor == focus: an empty range with a caret, as editors do.
type textSel struct {
	anchor, focus selPoint
	set           bool
}

// normalize orders the two ends as they are read.
func (s textSel) normalize() (start, end selPoint) {
	if s.anchor.line > s.focus.line ||
		(s.anchor.line == s.focus.line && s.anchor.cell > s.focus.cell) {
		return s.focus, s.anchor
	}
	return s.anchor, s.focus
}

// empty reports whether there is nothing to copy: no caret, or a collapsed
// caret without a range.
func (s textSel) empty() bool { return !s.set || s.anchor == s.focus }

// span returns the selected cell range [from, to) of one display line, given
// the line's width in cells. It is false when the line is outside the selection
// or when the range is empty: a selection ending at the start of a line covers
// the newline before it, which has no cells to paint.
func (s textSel) span(line, cells int) (from, to int, ok bool) {
	if s.empty() {
		return 0, 0, false
	}
	start, end := s.normalize()
	if line < start.line || line > end.line {
		return 0, 0, false
	}
	from, to = 0, cells
	if line == start.line {
		from = start.cell
	}
	if line == end.line {
		to = end.cell
	}
	from = max(0, min(from, cells))
	to = max(from, min(to, cells))
	if to <= from {
		return 0, 0, false
	}
	return from, to, true
}

// text returns the selected text. Lines are joined with '\n' as the viewer
// shows them, and a selection that ends at the start of a line keeps the
// newline before it.
func (s textSel) text(lines []string) string {
	if s.empty() || len(lines) == 0 {
		return ""
	}
	start, end := s.normalize()
	first := max(0, min(start.line, len(lines)-1))
	last := max(first, min(end.line, len(lines)-1))
	var b strings.Builder
	for line := first; line <= last; line++ {
		text := lines[line]
		cells := displayCells(text)
		from, to := 0, cells
		if line == start.line {
			from = start.cell
		}
		if line == end.line {
			to = end.cell
		}
		from = max(0, min(from, cells))
		to = max(from, min(to, cells))
		if line > first {
			b.WriteByte('\n')
		}
		b.WriteString(text[cellOffset(text, from):cellOffset(text, to)])
	}
	return b.String()
}

// cellOffset maps a display-cell boundary to a byte offset in line, so UTF-8
// runes and wide (two-cell) characters take the bytes they need. A boundary
// that falls inside a wide rune snaps back to the rune's first byte, so half a
// character is neither copied nor highlighted.
func cellOffset(line string, cell int) int {
	if cell <= 0 {
		return 0
	}
	col := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		w := cellWidthOf(r)
		if col+w > cell {
			return i
		}
		col += w
		i += size
		if col == cell {
			return i
		}
	}
	return len(line)
}

// nextCell returns the next caret boundary right (dir > 0) or left (dir < 0) of
// cell, stepping over wide runes whole. It reports false at the line's ends.
func nextCell(line string, cell, dir int) (int, bool) {
	if dir < 0 {
		prev, col := -1, 0
		for i := 0; i < len(line); {
			if col >= cell {
				break
			}
			prev = col
			r, size := utf8.DecodeRuneInString(line[i:])
			col += cellWidthOf(r)
			i += size
		}
		if prev < 0 {
			return 0, false
		}
		return prev, true
	}
	col := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		col += cellWidthOf(r)
		i += size
		if col > cell {
			return col, true
		}
	}
	return 0, false
}

// wordSpan returns the cell range of the word around cell on line, or an empty
// range when the cell sits on whitespace.
func wordSpan(line string, cell int) (from, to int) {
	type glyph struct {
		r          rune
		start, end int
	}
	glyphs := make([]glyph, 0, len(line))
	col := 0
	for _, r := range line {
		w := cellWidthOf(r)
		glyphs = append(glyphs, glyph{r: r, start: col, end: col + w})
		col += w
	}
	index := -1
	for i, g := range glyphs {
		if cell < g.end {
			index = i
			break
		}
	}
	if index < 0 || !isWordRune(glyphs[index].r) {
		return cell, cell
	}
	first, last := index, index
	for first > 0 && isWordRune(glyphs[first-1].r) {
		first--
	}
	for last+1 < len(glyphs) && isWordRune(glyphs[last+1].r) {
		last++
	}
	return glyphs[first].start, glyphs[last].end
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// moveCaret returns the caret one key step from p: arrows walk rune boundaries,
// Up and Down keep the cell when the neighbouring line is long enough.
func moveCaret(lines []string, p selPoint, key ui.Key) selPoint {
	if len(lines) == 0 {
		return p
	}
	line := max(0, min(p.line, len(lines)-1))
	cells := displayCells(lines[line])
	cell := max(0, min(p.cell, cells))
	switch key {
	case ui.KeyHome:
		return selPoint{line: line}
	case ui.KeyEnd:
		return selPoint{line: line, cell: cells}
	case ui.KeyUp:
		if line == 0 {
			return selPoint{line: 0, cell: cell}
		}
		line--
		return selPoint{line: line, cell: min(cell, displayCells(lines[line]))}
	case ui.KeyDown:
		if line == len(lines)-1 {
			return selPoint{line: line, cell: cell}
		}
		line++
		return selPoint{line: line, cell: min(cell, displayCells(lines[line]))}
	case ui.KeyLeft:
		if next, ok := nextCell(lines[line], cell, -1); ok {
			return selPoint{line: line, cell: next}
		}
		if line == 0 {
			return selPoint{line: 0}
		}
		line--
		return selPoint{line: line, cell: displayCells(lines[line])}
	case ui.KeyRight:
		if next, ok := nextCell(lines[line], cell, 1); ok {
			return selPoint{line: line, cell: next}
		}
		if line == len(lines)-1 {
			return selPoint{line: line, cell: cells}
		}
		return selPoint{line: line + 1}
	}
	return selPoint{line: line, cell: cell}
}

// ---- tab state -------------------------------------------------------------

// clearTextSel drops the view's selection and caret.
func (t *diffTab) clearTextSel() { t.textSel = textSel{} }

// hasTextSelection reports whether the tab is a file view with a copyable
// range.
func (t *diffTab) hasTextSelection() bool { return t.src.View && !t.textSel.empty() }

// selectionText returns the selected file-view text; "" when nothing is
// selected.
func (t *diffTab) selectionText() string {
	if !t.hasTextSelection() {
		return ""
	}
	return t.textSel.text(t.newDisp)
}

// selectAllText selects the whole file view and reports whether that left a
// range to copy.
func (t *diffTab) selectAllText() bool {
	if !t.src.View || t.res == nil || len(t.newDisp) == 0 {
		return false
	}
	last := len(t.newDisp) - 1
	t.textSel = textSel{
		anchor: selPoint{line: 0, cell: 0},
		focus:  selPoint{line: last, cell: displayCells(t.newDisp[last])},
		set:    true,
	}
	return !t.textSel.empty()
}

// ---- painting --------------------------------------------------------------

// paintTextSelection paints the selection of one visible line behind its text.
// The caller has the pane clipped; this clips to the text area as well so the
// highlight never covers the line numbers or the neighbouring pane. Boundaries
// snap to whole characters, so the highlight always covers exactly what a copy
// would put on the clipboard.
func (w *Window) paintTextSelection(t *diffTab, line int, text string, tr ui.Rect, y, cellW, lineH float32) {
	from, to, ok := t.textSel.span(line, displayCells(text))
	if !ok {
		return
	}
	from = displayCells(text[:cellOffset(text, from)])
	to = displayCells(text[:cellOffset(text, to)])
	if to <= from {
		return
	}
	c := w.pal.Selection
	if !w.focused(idDiffText) {
		c = w.pal.SelectionInac
	}
	x := tr.X + 6 - t.x + float32(from)*cellW
	w.c.Clip(tr)
	w.p.fill(rect(x, y, float32(to-from)*cellW, lineH), c)
	w.c.Unclip()
}

// paintTextCaret shows where the caret is while the text area holds the
// keyboard.
func (w *Window) paintTextCaret(t *diffTab, line int, text string, tr ui.Rect, y, cellW, lineH float32) {
	if !t.textSel.set || t.textSel.focus.line != line || !w.focused(idDiffText) {
		return
	}
	cell := max(0, min(t.textSel.focus.cell, displayCells(text)))
	cell = displayCells(text[:cellOffset(text, cell)])
	x := tr.X + 6 - t.x + float32(cell)*cellW
	w.c.Clip(tr)
	w.p.vline(x, y+1, lineH-2, w.pal.Text)
	w.c.Unclip()
}

// ---- input -----------------------------------------------------------------

// textPointer wires the file view's text area: a press anchors the selection, a
// drag extends it and a double click selects the word under the pointer. A
// press without a drag clears the selection by collapsing it to the clicked
// caret. The handler captures the geometry of the paint that registered it, so
// a drag keeps the coordinates it started with while the view repaints.
func (w *Window) textPointer(t *diffTab, tr ui.Rect, top, cellW, lineH float32) {
	disp := t.newDisp
	point := func(x, y float32) selPoint {
		if len(disp) == 0 {
			return selPoint{}
		}
		line := max(0, min(int((y-tr.Y+top)/lineH), len(disp)-1))
		cell := int((x - (tr.X + 6 - t.x)) / cellW)
		return selPoint{line: line, cell: max(0, min(cell, displayCells(disp[line])))}
	}
	w.c.HitWithOptions(idDiffText, tr, "File contents", ui.HitOptions{Role: ui.RoleGroup, Cursor: ui.PointerCursorText})
	w.onDrag(idDiffText, func(e dragEvent) {
		p := point(e.X, e.Y)
		switch e.Phase {
		case dragBegin:
			t.textSel = textSel{anchor: p, focus: p, set: true}
		case dragMove:
			if t.textSel.set {
				t.textSel.focus = p
			}
		}
	})
	w.onDouble(idDiffText, func() {
		p := point(w.mouse.X, w.mouse.Y)
		if p.line < 0 || p.line >= len(disp) {
			return
		}
		from, to := wordSpan(disp[p.line], p.cell)
		if to > from {
			t.textSel = textSel{
				anchor: selPoint{line: p.line, cell: from},
				focus:  selPoint{line: p.line, cell: to},
				set:    true,
			}
		}
	})
	w.onKeys(idDiffText, w.viewKeys(t, lineH, tr.Height))
}

// viewKeys is the file view's key handler: the caret keys move the selection,
// everything else keeps the diff panes' scrolling.
func (w *Window) viewKeys(t *diffTab, lineH, view float32) func(ui.KeyEvent) bool {
	return func(e ui.KeyEvent) bool {
		if w.textKey(t, e, lineH) {
			return true
		}
		return w.scrollKey(t, e, lineH, view)
	}
}

// textKey handles the caret keys of the file view. Shift extends the selection
// from its anchor and a bare arrow collapses it to a caret; before the first
// caret the bare arrows keep scrolling as they always did.
func (w *Window) textKey(t *diffTab, e ui.KeyEvent, lineH float32) bool {
	if t.res == nil || len(t.newDisp) == 0 {
		return false
	}
	switch e.Key {
	case ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd:
	default:
		return false
	}
	extend := e.Modifiers&ui.ModifierShift != 0
	if !t.textSel.set {
		if !extend {
			return false
		}
		// Start at the top of the viewport, so the extension begins where the
		// reader is looking.
		line := max(0, min(int(t.y/lineH), len(t.newDisp)-1))
		t.textSel = textSel{anchor: selPoint{line: line}, focus: selPoint{line: line}, set: true}
	}
	next := moveCaret(t.newDisp, t.textSel.focus, e.Key)
	if extend {
		t.textSel.focus = next
	} else {
		t.textSel.anchor, t.textSel.focus = next, next
	}
	return true
}

// copyViewSelection puts the file view's selection on the clipboard and reports
// whether it did. A focused text field owns Copy, and so does any tab without a
// selection, so the window's other Command+C chords keep their meaning.
func (w *Window) copyViewSelection() bool {
	if w.c == nil {
		return false
	}
	if _, editing := w.editors[w.c.Focused()]; editing {
		return false
	}
	t := w.ed.cur()
	if t == nil {
		return false
	}
	text := t.selectionText()
	if text == "" {
		return false
	}
	w.copyText(text)
	return true
}
