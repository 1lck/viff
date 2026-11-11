package viffui

import (
	"errors"
	"unicode/utf8"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
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

// fileEdit is the edit-mode state of one tab. The shared textBuffer owns the
// editor, the disk baseline, the display rows and the paint geometry; this type
// only adds the file viewer's layout.
type fileEdit struct {
	textBuffer
}

// canEdit reports whether t can switch to the editor. Historical revisions,
// images, binaries, submodules, symlinks, oversized files and conflicted paths
// stay read-only.
func (w *Window) canEdit(t *diffTab) bool {
	if t == nil || t.res == nil || !t.editOK || w.readOnly() {
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

// enterEdit switches t to the editor. It does not write anything: the buffer
// belongs to the tab until Save.
func (w *Window) enterEdit(t *diffTab) {
	if t == nil || t.edit != nil || !w.canEdit(t) {
		return
	}
	buf, err := newTextBuffer(t.res.New.Content.Data)
	if err != nil {
		body := w.t(i18n.FileviewEditNotUTF8)
		if errors.Is(err, vapp.ErrEditTooLarge) {
			body = w.t(i18n.FileviewEditTooLarge)
		}
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewEditing), body)
		return
	}
	t.edit = &fileEdit{textBuffer: *buf}
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
	w.notifySaveFailure(t, err, func() { w.reloadEditNow(t) })
}

// notifySaveFailure maps the save errors the app layer returns to the one
// warning each surface shows. reload is the action offered for a file that
// changed or disappeared; the caller decides what discarding means there.
func (w *Window) notifySaveFailure(t *diffTab, err error, reload func()) {
	switch {
	case errors.Is(err, vapp.ErrEditChanged):
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewChangedTitle), w.t(i18n.FileviewChangedBody),
			vapp.NoticeAction{Title: w.t(i18n.FileviewReload), Run: reload})
	case errors.Is(err, vapp.ErrEditMissing):
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewSaveFailed, t.src.Path), w.t(i18n.FileviewFileMissing),
			vapp.NoticeAction{Title: w.t(i18n.FileviewReload), Run: reload})
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

// sync rebuilds the display rows and syntax when the editor text changed.
func (e *fileEdit) sync(t *diffTab) {
	if !e.refresh() {
		return
	}
	t.newDisp = expandAll(e.lines)
	t.newSyn = highlight(t.src.Path, t.newDisp)
	t.maxCells = e.maxCells
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
		t.y, t.x = w.followBufferCaret(&e.textBuffer, t.y, t.x, r.Width-gutter, r.Height, cellW, lineH)
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
		w.paintBufferSelection(&e.textBuffer, idDiffText, i, displayCells(t.newDisp[i]), tr, y, t.x, cellW, lineH)
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
		w.paintBufferComposition(&e.textBuffer, i, displayCells(t.newDisp[i]), tr, y, t.x, cellW, lineH)
		w.paintBufferCaret(&e.textBuffer, idDiffText, i, tr, y, t.x, cellW, lineH)
	}
	if len(t.newDisp) == 0 {
		w.paintBufferCaret(&e.textBuffer, idDiffText, 0, tr, r.Y, t.x, cellW, lineH)
	}
	w.c.Unclip()
	w.p.vline(gr.X+gr.Width-1, r.Y, r.Height, w.pal.Border)
	// The text hit is registered before the scrollbar hit, so a press on the
	// bar reaches the bar and every other press reaches the editor.
	w.c.HitWithOptions(idDiffText, tr, w.t(i18n.FileviewEditLabel), ui.HitOptions{
		Role: ui.RoleTextField, Value: e.built, Multiline: true,
		TextBounds: rect(tr.X+6, tr.Y, tr.Width-6, tr.Height),
		TextSize:   fs, TextColor: w.pal.EditorText,
		HideNativeOverlays: true, Placement: &e.textBuffer,
	})
	w.editors[idDiffText] = e.ed
	w.vbar(idDiffScrollR, r, &scroller{Y: t.y}, content)
	w.wrapBarDrag(idDiffScrollR, func(y float32) { t.y = y }, func() float32 { return t.y }, content, r.Height)
}

// ---- lifecycle -------------------------------------------------------------

// dirtyBuffer reports unsaved text in either in-app editor: the file editor or
// the conflict Result pane. Guards ask it instead of a concrete mode so a new
// editable surface cannot silently opt out of them.
func (t *diffTab) dirtyBuffer() bool {
	if t.edit != nil && t.edit.dirty {
		return true
	}
	if t.cedit != nil {
		t.cedit.refresh()
		return t.cedit.dirty
	}
	return false
}

// dirtyEdit returns the first tab with unsaved editor changes, or nil.
func (w *Window) dirtyEdit() *diffTab {
	for _, t := range w.ed.tabs {
		if t.dirtyBuffer() {
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
		if t == keep || !t.dirtyBuffer() {
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
	if t.dirtyBuffer() {
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
