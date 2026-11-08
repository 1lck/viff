package viffui

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
)

// In-app editing of one working-tree text file. It is an explicit mode of the
// read-only file view, not a second editor: the same tab, tab bar and toolbar
// switch to an editable surface, and leaving the mode reloads the file from
// disk. Diffs, historical revisions, conflicts, images and submodules stay
// read-only.
//
// The native text service owns typing, IME, selection, clipboard and undo. The
// view paints the rows itself, so it also tells the runtime where a pointer
// lands (OffsetAt) and where the IME window belongs (CaretBounds). Saving goes
// through app.SaveFile, which re-reads the file and refuses an external change.

// editContentLimit mirrors app.EditLimit; the UI checks it before entering the
// mode, so the button is absent instead of failing later.
const editContentLimit = vapp.EditLimit

// editPos is a caret position in the editor's display grid.
type editPos struct {
	line int
	cell int
}

// fileEdit is the edit-mode state of one tab. The editor owns the text,
// selection and undo history; this type owns the mapping to display rows and
// the geometry of the last paint.
type fileEdit struct {
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

// canEdit reports whether t can switch to the editor. Historical revisions,
// images, binaries, submodules, symlinks, oversized files and conflicted paths
// stay read-only.
func (w *Window) canEdit(t *diffTab) bool {
	if t == nil || t.res == nil || !t.editOK {
		return false
	}
	if rs := w.repo(); rs != nil {
		for _, c := range rs.Conflicts {
			if c.Path == t.src.Path {
				return false
			}
		}
	}
	return true
}

// editableContent is the content half of canEdit, evaluated once per load.
func editableContent(t *diffTab) bool {
	if t == nil || t.res == nil || !t.src.View || t.src.Commit != "" || t.res.Notice != "" {
		return false
	}
	c := t.res.New.Content
	if !c.Exists || c.Binary || c.TooLarge || c.Symlink || c.Submodule || !t.res.New.Text {
		return false
	}
	return len(c.Data) <= editContentLimit && utf8.Valid(bytesWithoutBOM(c.Data))
}

func bytesWithoutBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
}

// enterEdit switches t to the editor. It does not write anything: the buffer
// belongs to the tab until Save.
func (w *Window) enterEdit(t *diffTab) {
	if t == nil || t.edit != nil || !w.canEdit(t) {
		return
	}
	c := t.res.New.Content
	bom := bytes.HasPrefix(c.Data, []byte{0xEF, 0xBB, 0xBF})
	raw := bytesWithoutBOM(c.Data)
	if !utf8.Valid(raw) {
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewEditing), w.t(i18n.FileviewEditNotUTF8))
		return
	}
	if len(raw) > editContentLimit {
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewEditing), w.t(i18n.FileviewEditTooLarge))
		return
	}
	value := string(raw)
	lineEnd := domain.DetectLineEnding(value)
	normalized := normalizeNewlines(value)
	ed := text.NewEditor(normalized)
	ed.MoveTo(0, false)
	ed.ClearHistory()
	t.edit = &fileEdit{
		ed:       ed,
		original: normalized,
		built:    "\x00", // force the first sync even for an empty file
		expected: sha256.Sum256(c.Data),
		lineEnd:  lineEnd,
		bom:      bom,
	}
	t.annotate = false
	t.searchOpen = false
	t.clearTextSel()
	w.focusNext = idDiffText
	w.Invalidate()
}

// requestExitEdit leaves the editor, asking before dropping a dirty buffer.
func (w *Window) requestExitEdit(t *diffTab) {
	if t == nil || t.edit == nil {
		return
	}
	if t.edit.dirty {
		w.confirm(w.t(i18n.FileviewDiscardTitle), w.t(i18n.FileviewDiscardBody, t.src.Path), w.t(i18n.FileviewDiscardButton), func() {
			w.exitEdit(t)
		})
		return
	}
	w.exitEdit(t)
}

func (w *Window) exitEdit(t *diffTab) {
	if t.edit == nil {
		return
	}
	t.edit = nil
	t.reenterEdit = false
	w.clearEditInput()
	t.clearTextSel()
	// The editor may have changed t.newDisp without touching t.res. Restore
	// the read-only display from the loaded result before asking for a reload;
	// the paint between this event and the reload must not mix the edited rows
	// with the old block map.
	if t.res != nil {
		w.prepareTab(t)
	}
	w.loadTab(t)
	w.Invalidate()
}

// saveEdit writes the buffer through the app layer. The tab stays in edit mode
// after a successful save so the user can keep typing.
func (w *Window) saveEdit(t *diffTab) {
	e := t.edit
	if e == nil || e.saving {
		return
	}
	e.sync(t)
	if !e.dirty {
		return
	}
	e.saving = true
	value := e.ed.String()
	w.ws.SaveFile(vapp.SaveFileRequest{
		Path: t.src.Path, Text: value, LineEnding: e.lineEnd, BOM: e.bom, Expected: e.expected,
	}, func(res vapp.SaveFileResult, err error) {
		if t.edit != e {
			// The tab was closed or left the editor while the save ran; its
			// buffer no longer owns any state this window shows.
			return
		}
		e.saving = false
		if err != nil {
			w.notifySaveError(t, err)
			// A toolbar click focused the button; put the keyboard back where
			// the next keystroke belongs.
			w.focusNext = idDiffText
			w.Invalidate()
			return
		}
		e.original = value
		e.expected = res.Hash
		// The editor stayed live during the save: anything typed after the
		// snapshot is in the buffer but not on disk and must stay dirty.
		e.dirty = e.ed.String() != value
		w.focusNext = idDiffText
		w.Invalidate()
	})
}

func (w *Window) notifySaveError(t *diffTab, err error) {
	switch {
	case errors.Is(err, vapp.ErrEditChanged):
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewChangedTitle), w.t(i18n.FileviewChangedBody),
			vapp.NoticeAction{Title: w.t(i18n.FileviewReload), Run: func() { w.reloadEditNow(t) }})
	case errors.Is(err, vapp.ErrEditMissing):
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewSaveFailed, t.src.Path), w.t(i18n.FileviewFileMissing),
			vapp.NoticeAction{Title: w.t(i18n.FileviewReload), Run: func() { w.reloadEditNow(t) }})
	case errors.Is(err, vapp.ErrEditReadOnly):
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewSaveFailed, t.src.Path), w.t(i18n.FileviewEditReadOnly))
	case errors.Is(err, vapp.ErrEditTooLarge):
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewSaveFailed, t.src.Path), w.t(i18n.FileviewSaveTooLarge))
	case errors.Is(err, vapp.ErrEditBinary):
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewSaveFailed, t.src.Path), w.t(i18n.FileviewEditNotUTF8))
	case errors.Is(err, vapp.ErrEditSymlink), errors.Is(err, vapp.ErrEditSubmodule), errors.Is(err, vapp.ErrEditPath):
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewSaveFailed, t.src.Path), w.t(i18n.FileviewEditUnavailable))
	default:
		w.ws.NotifyError(w.t(i18n.FileviewSaveFailed, t.src.Path), err)
	}
}

func (w *Window) reloadEditNow(t *diffTab) {
	// The warning's Reload action was the confirmation; do not ask again.
	// Re-enter the editor only when the tab was editing when Reload ran.
	editing := t.edit != nil
	t.edit = nil
	t.reenterEdit = editing
	w.clearEditInput()
	if t.res != nil {
		w.prepareTab(t)
	}
	w.loadTab(t)
	w.Invalidate()
}

// clearEditInput drops the cached native input of the editor so a closed or
// exited buffer is not kept alive by its State and event closure.
func (w *Window) clearEditInput() {
	if w.inputs != nil {
		delete(w.inputs, idDiffText)
	}
}

// revertEdit restores the buffer to the last saved or loaded text. It is not a
// write: Save still has to be pressed.
func (w *Window) revertEdit(t *diffTab) {
	e := t.edit
	if e == nil || !e.dirty {
		return
	}
	e.ed = text.NewEditor(e.original)
	e.ed.MoveTo(0, false)
	e.ed.ClearHistory()
	e.synced = false
	e.saving = false
	w.focusNext = idDiffText
	w.Invalidate()
}

// applyText mirrors one native text event into the editor. It is the hook that
// normalizes a pasted CRLF to LF before the display rows are rebuilt.
func (e *fileEdit) applyText(ev app.TextEvent) (app.TextState, error) {
	if ev.Kind == app.TextInsert || ev.Kind == app.TextPaste {
		ev.Text = normalizeNewlines(ev.Text)
	}
	ev.Apply(e.ed)
	return app.TextStateOf(e.ed), nil
}

func normalizeNewlines(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// sync rebuilds the display rows and syntax when the editor text changed.
func (e *fileEdit) sync(t *diffTab) {
	value := e.ed.String()
	if e.synced && value == e.built {
		return
	}
	e.synced = true
	e.built = value
	e.lines, e.noFinal = domain.SplitLines(value)
	e.starts = e.starts[:0]
	pos := 0
	for i, line := range e.lines {
		e.starts = append(e.starts, pos)
		pos += len(line)
		if i < len(e.lines)-1 || !e.noFinal {
			pos++ // the newline that ended this line
		}
	}
	e.starts = append(e.starts, pos)
	t.newDisp = expandAll(e.lines)
	t.newSyn = highlight(t.src.Path, t.newDisp)
	t.maxCells = 0
	for _, line := range t.newDisp {
		t.maxCells = max(t.maxCells, displayCells(line))
	}
	e.dirty = value != e.original
}

// posOfRune maps a rune offset of the editor to a display row and cell.
func (e *fileEdit) posOfRune(index int) editPos {
	return e.posOfByte(runeToByte(e.built, index))
}

// posOfByte maps a UTF-8 byte offset to a display row and cell. A caret at the
// end of a final newline belongs to the last visible row: SplitLines has no
// empty row for it, and the read-only view never showed one.
func (e *fileEdit) posOfByte(offset int) editPos {
	if len(e.lines) == 0 {
		return editPos{}
	}
	offset = max(0, min(offset, len(e.built)))
	line := sort.Search(len(e.starts), func(i int) bool { return e.starts[i] > offset }) - 1
	if line < 0 {
		line = 0
	}
	if line >= len(e.lines) {
		line = len(e.lines) - 1
	}
	col := max(0, min(offset-e.starts[line], len(e.lines[line])))
	return editPos{line: line, cell: columnOf(e.lines[line], col, 4)}
}

// byteAt maps a row and display cell to a byte offset in the editor text.
func (e *fileEdit) byteAt(line, cell int) int {
	if len(e.lines) == 0 {
		return 0
	}
	line = max(0, min(line, len(e.lines)-1))
	return e.starts[line] + byteOffsetAtCell(e.lines[line], cell)
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

func (e *fileEdit) caretPos() editPos { return e.posOfRune(e.ed.Caret()) }

func (e *fileEdit) selectionPoints() (start, end editPos, ok bool) {
	sel := e.ed.Selection()
	if sel.Start == sel.End {
		return editPos{}, editPos{}, false
	}
	start, end = e.posOfRune(sel.Start), e.posOfRune(sel.End)
	if start.line > end.line || start.line == end.line && start.cell > end.cell {
		start, end = end, start
	}
	return start, end, true
}

func (e *fileEdit) compositionPoints() (start, end editPos, ok bool) {
	r, composing := e.ed.Composition()
	if !composing || r.Start == r.End {
		return editPos{}, editPos{}, false
	}
	start, end = e.posOfRune(r.Start), e.posOfRune(r.End)
	if start.line > end.line || start.line == end.line && start.cell > end.cell {
		start, end = end, start
	}
	return start, end, true
}

// ---- ui.TextPlacement ------------------------------------------------------

// OffsetAt is where the runtime puts the caret for a press or a drag. The
// offset is a byte offset into the field's Value, as the interface requires.
func (e *fileEdit) OffsetAt(point ui.Point) (int, bool) {
	if !e.painted || !contains(e.tr, point.X, point.Y) {
		return 0, false
	}
	if len(e.lines) == 0 {
		return 0, true
	}
	line := int((point.Y - e.tr.Y + e.top) / e.lineH)
	line = max(0, min(line, len(e.lines)-1))
	cell := int((point.X - (e.tr.X + 6 - e.scrollX)) / e.cellW)
	return e.byteAt(line, cell), true
}

// CaretBounds is where the platform puts an input-method window. It reports
// false while the caret is outside the painted rows.
func (e *fileEdit) CaretBounds() (ui.Rect, bool) {
	if !e.painted {
		return ui.Rect{}, false
	}
	p := e.caretPos()
	y := e.tr.Y + float32(p.line)*e.lineH - e.top
	if y+e.lineH < e.tr.Y || y > bottom(e.tr) {
		return ui.Rect{}, false
	}
	x := e.tr.X + 6 - e.scrollX + float32(p.cell)*e.cellW
	return ui.Rect{X: x, Y: y, Width: 1, Height: e.lineH}, true
}

// ---- painting --------------------------------------------------------------

func (w *Window) paintEditBody(t *diffTab, r ui.Rect) {
	e := t.edit
	e.sync(t)
	cellW, lineH, fs := w.metrics()
	content := float32(len(t.newDisp)+3) * lineH
	t.y = clampF(t.y, 0, max(0, content-r.Height))
	gutter := w.gutterWidth(len(t.newDisp), cellW)
	maxX := max(0, float32(t.maxCells)*cellW+40-(r.Width-gutter))
	t.x = clampF(t.x, 0, maxX)
	w.onScrollIn(r, func(dx, dy float32) {
		t.x = clampF(t.x+dx, 0, maxX)
		t.y += dy
	})
	if w.focused(idDiffText) {
		w.followEditCaret(t, e, r, gutter, cellW, lineH)
		t.y = clampF(t.y, 0, max(0, content-r.Height))
		t.x = clampF(t.x, 0, maxX)
	}
	gr := rect(r.X, r.Y, gutter, r.Height)
	tr := rect(r.X+gutter, r.Y, r.Width-gutter, r.Height)
	e.tr, e.top, e.scrollX = tr, t.y, t.x
	e.cellW, e.lineH, e.painted = cellW, lineH, true
	w.c.Clip(r)
	first := max(0, int(t.y/lineH))
	last := min(len(t.newDisp), int((t.y+r.Height)/lineH)+1)
	style := ui.TextStyle{Size: fs, Color: w.pal.EditorText, Mono: true, MonoFace: w.mono}
	numStyle := ui.TextStyle{Size: fs - 1, Color: w.pal.LineNumber, Mono: true, MonoFace: w.mono}
	var runs []ui.TextRun
	for i := first; i < last; i++ {
		y := r.Y + float32(i)*lineH - t.y
		w.paintEditSelection(t, e, i, displayCells(t.newDisp[i]), tr, y, cellW, lineH)
		w.p.c.LabelAligned(itoa(i+1), rect(gr.X, y, gr.Width-14, lineH), numStyle, ui.AlignRight)
		line := t.newDisp[i]
		if line != "" {
			var spans []span
			if i < len(t.newSyn) {
				spans = t.newSyn[i]
			}
			runs = w.runsFor(line, spans, runs, w.pal.EditorText)
			w.c.Clip(tr)
			w.c.LabelRuns(line, rect(tr.X+6-t.x, y, float32(displayCells(line)+1)*cellW, lineH), style, cellW, runs)
			w.c.Unclip()
		}
		w.paintEditComposition(t, e, i, displayCells(t.newDisp[i]), tr, y, cellW, lineH)
		w.paintEditCaret(t, e, i, tr, y, lineH)
	}
	if len(t.newDisp) == 0 {
		w.paintEditCaret(t, e, 0, tr, r.Y, lineH)
	}
	w.c.Unclip()
	w.p.vline(gr.X+gr.Width-1, r.Y, r.Height, w.pal.Border)
	// The text hit is registered before the scrollbar hit, so a press on the
	// bar reaches the bar and every other press reaches the editor.
	w.c.HitWithOptions(idDiffText, tr, w.t(i18n.FileviewEditLabel), ui.HitOptions{
		Role: ui.RoleTextField, Value: e.built, Multiline: true,
		TextBounds: rect(tr.X+6, tr.Y, tr.Width-6, tr.Height),
		TextSize:   fs, TextColor: w.pal.EditorText,
		HideNativeOverlays: true, Placement: e,
	})
	w.editors[idDiffText] = e.ed
	w.vbar(idDiffScrollR, r, &scroller{Y: t.y}, content)
	w.wrapBarDrag(idDiffScrollR, func(y float32) { t.y = y }, func() float32 { return t.y }, content, r.Height)
}

// followEditCaret scrolls the smallest amount that keeps the caret visible.
func (w *Window) followEditCaret(t *diffTab, e *fileEdit, r ui.Rect, gutter, cellW, lineH float32) {
	p := e.caretPos()
	if float32(p.line+1)*lineH > t.y+r.Height {
		t.y = float32(p.line+1)*lineH - r.Height
	}
	if float32(p.line)*lineH < t.y {
		t.y = float32(p.line) * lineH
	}
	width := max(r.Width-gutter, cellW)
	x := float32(p.cell) * cellW
	if x < t.x {
		t.x = x
	}
	if x+cellW > t.x+width {
		t.x = x + cellW - width
	}
}

func (w *Window) paintEditSelection(t *diffTab, e *fileEdit, line, cells int, tr ui.Rect, y, cellW, lineH float32) {
	start, end, ok := e.selectionPoints()
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
	if !w.focused(idDiffText) {
		c = w.pal.SelectionInac
	}
	x := tr.X + 6 - t.x + float32(from)*cellW
	w.c.Clip(tr)
	w.p.fill(rect(x, y, float32(to-from)*cellW, lineH), c)
	w.c.Unclip()
}

// paintEditComposition underlines the range an input method is composing.
func (w *Window) paintEditComposition(t *diffTab, e *fileEdit, line, cells int, tr ui.Rect, y, cellW, lineH float32) {
	start, end, ok := e.compositionPoints()
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
	x := tr.X + 6 - t.x + float32(from)*cellW
	w.c.Clip(tr)
	w.p.fill(rect(x, y+lineH-2, float32(to-from)*cellW, 2), w.pal.Link)
	w.c.Unclip()
}

func (w *Window) paintEditCaret(t *diffTab, e *fileEdit, line int, tr ui.Rect, y, lineH float32) {
	if !w.focused(idDiffText) {
		return
	}
	p := e.caretPos()
	if p.line != line {
		return
	}
	x := tr.X + 6 - t.x + float32(p.cell)*e.cellW
	w.c.Clip(tr)
	w.p.vline(x, y+1, lineH-2, w.pal.Text)
	w.c.Unclip()
}

// ---- lifecycle -------------------------------------------------------------

// dirtyEdit returns the first tab with unsaved editor changes, or nil.
func (w *Window) dirtyEdit() *diffTab {
	for _, t := range w.ed.tabs {
		if t.edit != nil && t.edit.dirty {
			return t
		}
	}
	return nil
}

// confirmDiscardEdits runs then, asking first when any editor buffer is dirty.
// Repository switches and "close all" use it so a mode change never drops work
// silently.
func (w *Window) confirmDiscardEdits(then func()) {
	t := w.dirtyEdit()
	if t == nil {
		then()
		return
	}
	w.confirm(w.t(i18n.FileviewDiscardTitle), w.t(i18n.FileviewDiscardBody, t.src.Path), w.t(i18n.FileviewDiscardButton), then)
}

// confirmDiscardOtherEdits is confirmDiscardEdits for an action that keeps one
// tab: asking about the kept tab's unsaved text would describe work that is not
// dropped.
func (w *Window) confirmDiscardOtherEdits(keep *diffTab, then func()) {
	for _, t := range w.ed.tabs {
		if t == keep || t.edit == nil || !t.edit.dirty {
			continue
		}
		w.confirm(w.t(i18n.FileviewDiscardTitle), w.t(i18n.FileviewDiscardBody, t.src.Path), w.t(i18n.FileviewDiscardButton), then)
		return
	}
	then()
}

// requestCloseTab closes a tab after asking about unsaved editor changes.
func (w *Window) requestCloseTab(index int) {
	if index < 0 || index >= len(w.ed.tabs) {
		return
	}
	t := w.ed.tabs[index]
	if t.edit != nil && t.edit.dirty {
		w.confirm(w.t(i18n.FileviewDiscardTitle), w.t(i18n.FileviewDiscardBody, t.src.Path), w.t(i18n.FileviewDiscardButton), func() {
			for i, o := range w.ed.tabs {
				if o == t {
					w.closeTab(i)
					return
				}
			}
		})
		return
	}
	w.closeTab(index)
}
