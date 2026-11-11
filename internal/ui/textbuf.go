package viffui

import (
	"bytes"
	"crypto/sha256"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// textBuffer is the part every in-app text surface shares: the native editor
// (typing, IME, selection, clipboard and undo), the disk baseline a save checks
// against, the encoding restored on save, the display rows and the geometry of
// the last paint. The file editor (fileedit.go) and the conflict Result pane
// (conflictedit.go) own this buffer and add their own layout; neither keeps a
// second copy of the text.
type textBuffer struct {
	ed       *text.Editor
	original string
	built    string
	expected [sha256.Size]byte
	lineEnd  string
	bom      bool
	synced   bool
	noFinal  bool
	lines    []string
	starts   []int
	maxCells int
	dirty    bool
	saving   bool

	// Geometry of the last paint, used by ui.TextPlacement.
	tr      ui.Rect
	top     float32
	scrollX float32
	cellW   float32
	lineH   float32
	painted bool
}

// editPos is a caret position in an editor's display grid.
type editPos struct {
	line int
	cell int
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func bytesWithoutBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, utf8BOM)
}

// newTextBuffer opens data as an LF-normalized editor buffer and remembers the
// bytes it came from. The caller decides whether the file is editable at all;
// this constructor rejects what a save could never write back: non-UTF-8 text
// and content above the editing limit.
func newTextBuffer(data []byte) (*textBuffer, error) {
	raw := bytesWithoutBOM(data)
	if !utf8.Valid(raw) {
		return nil, vapp.ErrEditBinary
	}
	if len(raw) > editContentLimit {
		return nil, vapp.ErrEditTooLarge
	}
	value := string(raw)
	normalized := normalizeNewlines(value)
	ed := text.NewEditor(normalized)
	ed.MoveTo(0, false)
	ed.ClearHistory()
	return &textBuffer{
		ed:       ed,
		original: normalized,
		built:    "\x00", // force the first sync even for an empty file
		expected: sha256.Sum256(data),
		lineEnd:  domain.DetectLineEnding(value),
		bom:      bytes.HasPrefix(data, utf8BOM),
	}, nil
}

func normalizeNewlines(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// changed reports whether the editor holds text that was not saved or loaded.
// It reads the editor rather than the dirty flag so a guard can ask before the
// next paint has synced the flag.
func (b *textBuffer) changed() bool { return b.ed.String() != b.original }

// refresh rebuilds the display rows and the dirty flag when the editor text
// changed. It reports whether anything was rebuilt.
func (b *textBuffer) refresh() bool {
	value := b.ed.String()
	if b.synced && value == b.built {
		return false
	}
	b.synced = true
	b.built = value
	b.lines, b.noFinal = domain.SplitLines(value)
	b.starts = b.starts[:0]
	pos := 0
	for i, line := range b.lines {
		b.starts = append(b.starts, pos)
		pos += len(line)
		if i < len(b.lines)-1 || !b.noFinal {
			pos++ // the newline that ended this line
		}
	}
	b.starts = append(b.starts, pos)
	b.maxCells = 0
	for _, line := range b.lines {
		b.maxCells = max(b.maxCells, displayCells(expandLine(line)))
	}
	b.dirty = value != b.original
	return true
}

// replaceAll sets the whole editor text as one undoable edit, keeping the
// history a per-block accept or a revert builds on.
func (b *textBuffer) replaceAll(value string) {
	if b.ed.String() == value {
		return
	}
	b.ed.SetSelection(text.Range{Start: 0, End: b.ed.RuneCount()})
	b.ed.Insert(value)
	b.ed.BreakUndoGroup()
	b.synced = false
}

// applyText mirrors one native text event into the editor. It is the hook that
// normalizes a pasted CRLF to LF before the display rows are rebuilt.
func (b *textBuffer) applyText(ev app.TextEvent) (app.TextState, error) {
	if ev.Kind == app.TextInsert || ev.Kind == app.TextPaste {
		ev.Text = normalizeNewlines(ev.Text)
	}
	ev.Apply(b.ed)
	return app.TextStateOf(b.ed), nil
}

// posOfRune maps a rune offset of the editor to a display row and cell.
func (b *textBuffer) posOfRune(index int) editPos {
	return b.posOfByte(runeToByte(b.built, index))
}

// posOfByte maps a UTF-8 byte offset to a display row and cell. A caret at the
// end of a final newline belongs to the last visible row: SplitLines has no
// empty row for it, and the read-only view never showed one.
func (b *textBuffer) posOfByte(offset int) editPos {
	if len(b.lines) == 0 {
		return editPos{}
	}
	offset = max(0, min(offset, len(b.built)))
	line := sort.Search(len(b.starts), func(i int) bool { return b.starts[i] > offset }) - 1
	if line < 0 {
		line = 0
	}
	if line >= len(b.lines) {
		line = len(b.lines) - 1
	}
	col := max(0, min(offset-b.starts[line], len(b.lines[line])))
	return editPos{line: line, cell: columnOf(b.lines[line], col, 4)}
}

// byteAt maps a row and display cell to a byte offset in the editor text.
func (b *textBuffer) byteAt(line, cell int) int {
	if len(b.lines) == 0 {
		return 0
	}
	line = max(0, min(line, len(b.lines)-1))
	return b.starts[line] + byteOffsetAtCell(b.lines[line], cell)
}

// byteOffsetAtCell walks a raw line to the byte boundary nearest a display
// cell. A tab takes the cells to the next four-column stop; a wide rune keeps
// its two cells together.
func byteOffsetAtCell(raw string, cell int) int {
	if cell <= 0 {
		return 0
	}
	col := 0
	for i, r := range raw {
		w := cellWidthOf(r)
		if r == '\t' {
			w = 4 - col%4
		}
		if col+w > cell {
			return i
		}
		col += w
		if col == cell {
			return i + utf8.RuneLen(r)
		}
	}
	return len(raw)
}

func (b *textBuffer) caretPos() editPos { return b.posOfRune(b.ed.Caret()) }

func (b *textBuffer) selectionPoints() (start, end editPos, ok bool) {
	sel := b.ed.Selection()
	if sel.Start == sel.End {
		return editPos{}, editPos{}, false
	}
	start, end = b.posOfRune(sel.Start), b.posOfRune(sel.End)
	if start.line > end.line || start.line == end.line && start.cell > end.cell {
		start, end = end, start
	}
	return start, end, true
}

func (b *textBuffer) compositionPoints() (start, end editPos, ok bool) {
	r, composing := b.ed.Composition()
	if !composing || r.Start == r.End {
		return editPos{}, editPos{}, false
	}
	start, end = b.posOfRune(r.Start), b.posOfRune(r.End)
	if start.line > end.line || start.line == end.line && start.cell > end.cell {
		start, end = end, start
	}
	return start, end, true
}

// ---- ui.TextPlacement ------------------------------------------------------

// OffsetAt is where the runtime puts the caret for a press or a drag. The
// offset is a byte offset into the field's Value, as the interface requires.
func (b *textBuffer) OffsetAt(point ui.Point) (int, bool) {
	if !b.painted || !contains(b.tr, point.X, point.Y) {
		return 0, false
	}
	if len(b.lines) == 0 {
		return 0, true
	}
	line := int((point.Y - b.tr.Y + b.top) / b.lineH)
	line = max(0, min(line, len(b.lines)-1))
	cell := int((point.X - (b.tr.X + 6 - b.scrollX)) / b.cellW)
	return b.byteAt(line, cell), true
}

// CaretBounds is where the platform puts an input-method window. It reports
// false while the caret is outside the painted rows.
func (b *textBuffer) CaretBounds() (ui.Rect, bool) {
	if !b.painted {
		return ui.Rect{}, false
	}
	p := b.caretPos()
	y := b.tr.Y + float32(p.line)*b.lineH - b.top
	if y+b.lineH < b.tr.Y || y > bottom(b.tr) {
		return ui.Rect{}, false
	}
	x := b.tr.X + 6 - b.scrollX + float32(p.cell)*b.cellW
	return ui.Rect{X: x, Y: y, Width: 1, Height: b.lineH}, true
}

// ---- painting --------------------------------------------------------------

// followBufferCaret scrolls the smallest amount that keeps the caret visible.
// It returns the new top and horizontal scroll of the surface the buffer is
// painted in.
func (w *Window) followBufferCaret(b *textBuffer, top, scrollX, viewW, viewH, cellW, lineH float32) (float32, float32) {
	p := b.caretPos()
	if float32(p.line+1)*lineH > top+viewH {
		top = float32(p.line+1)*lineH - viewH
	}
	if float32(p.line)*lineH < top {
		top = float32(p.line) * lineH
	}
	width := max(viewW, cellW)
	x := float32(p.cell) * cellW
	if x < scrollX {
		scrollX = x
	}
	if x+cellW > scrollX+width {
		scrollX = x + cellW - width
	}
	return top, scrollX
}

func (w *Window) paintBufferSelection(b *textBuffer, id ui.ID, line, cells int, tr ui.Rect, y, scrollX, cellW, lineH float32) {
	start, end, ok := b.selectionPoints()
	if !ok || line < start.line || line > end.line {
		return
	}
	from, to := 0, cells
	if line == start.line {
		from = start.cell
	}
	if line == end.line {
		to = end.cell
	}
	from = max(0, min(from, cells))
	to = max(from, min(to, cells))
	if to <= from {
		return
	}
	c := w.pal.Selection
	if !w.focused(id) {
		c = w.pal.SelectionInac
	}
	x := tr.X + 6 - scrollX + float32(from)*cellW
	w.c.Clip(tr)
	w.p.fill(rect(x, y, float32(to-from)*cellW, lineH), c)
	w.c.Unclip()
}

// paintBufferComposition underlines the range an input method is composing.
func (w *Window) paintBufferComposition(b *textBuffer, line, cells int, tr ui.Rect, y, scrollX, cellW, lineH float32) {
	start, end, ok := b.compositionPoints()
	if !ok || line < start.line || line > end.line {
		return
	}
	from, to := 0, cells
	if line == start.line {
		from = start.cell
	}
	if line == end.line {
		to = end.cell
	}
	from = max(0, min(from, cells))
	to = max(from, min(to, cells))
	if to <= from {
		return
	}
	x := tr.X + 6 - scrollX + float32(from)*cellW
	w.c.Clip(tr)
	w.p.fill(rect(x, y+lineH-2, float32(to-from)*cellW, 2), w.pal.Link)
	w.c.Unclip()
}

func (w *Window) paintBufferCaret(b *textBuffer, id ui.ID, line int, tr ui.Rect, y, scrollX, cellW, lineH float32) {
	if !w.focused(id) {
		return
	}
	p := b.caretPos()
	if p.line != line {
		return
	}
	x := tr.X + 6 - scrollX + float32(p.cell)*cellW
	w.c.Clip(tr)
	w.p.vline(x, y+1, lineH-2, w.pal.Text)
	w.c.Unclip()
}
