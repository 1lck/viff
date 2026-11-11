package viffui

import (
	"crypto/sha256"
	"unicode/utf8"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
)

// The conflict viewer's Result pane edits the merged text in place. It is not a
// second editor: it owns the same textBuffer as the file editor (textbuf.go),
// so typing, selection, IME, clipboard and undo behave identically, and Save
// goes through app.SaveFile with the same stale check. Left and right stay
// read-only snapshots of the two sides.
//
// The buffer, not the loaded view, is the truth while editing: block
// highlighting, navigation, per-block accept and Copy for AI all read the
// markers parsed from it. Accepting a side replaces the whole buffer with one
// undoable edit, so unsaved manual changes are never dropped.

// conflictEdit is the editable Result pane of one conflict tab.
type conflictEdit struct {
	textBuffer
	markers domain.MarkerParse
}

func newConflictEdit(data []byte) (*conflictEdit, error) {
	buf, err := newTextBuffer(data)
	if err != nil {
		return nil, err
	}
	e := &conflictEdit{textBuffer: *buf}
	e.refresh()
	return e, nil
}

// refresh rebuilds the display rows and re-parses the conflict markers the
// pane highlights, navigates and accepts.
func (e *conflictEdit) refresh() bool {
	if !e.textBuffer.refresh() {
		return false
	}
	e.markers = domain.ParseConflictMarkers(e.lines)
	return true
}

// conflictEditableContent is the file-level half of the editable Result pane:
// only a UTF-8 working-tree text whose markers form complete blocks, within the
// editor's size limit, can be edited and saved in place. Binary, oversized,
// symlink and submodule entries and an unreliable parse stay read-only.
func conflictEditableContent(cv *vapp.ConflictView) bool {
	if cv == nil {
		return false
	}
	c := cv.Current.Content
	if !cv.Conflict.Kind.Textual() || !c.Exists || !cv.Current.Text ||
		c.Binary || c.TooLarge || c.Submodule || c.Symlink {
		return false
	}
	if !cv.Markers.Reliable || len(cv.Markers.Blocks) == 0 {
		return false
	}
	raw := bytesWithoutBOM(c.Data)
	return len(raw) <= editContentLimit && utf8.Valid(raw)
}

// conflictEditable reports whether t shows a conflict Viff may edit: a backend
// that resolves conflicts (not Subversion), a writable repository (not a
// read-only or remote one) and content the editor can write back.
func (w *Window) conflictEditable(t *diffTab) bool {
	if t == nil || t.res == nil || t.res.Conflict == nil {
		return false
	}
	return w.ws.Caps.Conflicts != nil && !w.readOnly() && conflictEditableContent(t.res.Conflict)
}

// syncConflictEdit creates or refreshes the Result buffer after a load. A dirty
// buffer is never replaced: an external change is reported by the save's stale
// check, which keeps the user's text.
func (w *Window) syncConflictEdit(t *diffTab) bool {
	cv := t.res.Conflict
	if t.cedit != nil {
		if t.cedit.changed() || t.cedit.saving {
			return false
		}
		if !w.conflictEditable(t) {
			t.cedit = nil
			w.clearConflictInput()
			return false
		}
		if sha256.Sum256(cv.Current.Content.Data) == t.cedit.expected {
			return false
		}
		e, err := newConflictEdit(cv.Current.Content.Data)
		if err != nil {
			t.cedit = nil
			w.clearConflictInput()
			return false
		}
		t.cedit = e
		return false
	}
	if !w.conflictEditable(t) {
		return false
	}
	e, err := newConflictEdit(cv.Current.Content.Data)
	if err != nil {
		return false
	}
	t.cedit = e
	return true
}

// conflictBlocks is the block list the pane currently shows: the buffer's when
// it is editable, the loaded view's otherwise.
func (w *Window) conflictBlocks(t *diffTab) []domain.ConflictBlock {
	if t.cedit != nil {
		t.cedit.refresh()
		return t.cedit.markers.Blocks
	}
	if t.res != nil && t.res.Conflict != nil {
		return t.res.Conflict.Markers.Blocks
	}
	return nil
}

// acceptConflictBlock applies one side to the edited buffer. Nothing is written
// until Save: the replacement is one undoable edit, and manual changes elsewhere
// in the buffer stay where they are.
func (w *Window) acceptConflictBlock(t *diffTab, choice domain.ResolveChoice) {
	e := t.cedit
	if e == nil || t.loading || t.busy {
		return
	}
	e.refresh()
	block := t.conflictBlock
	if !e.markers.Reliable || block < 0 || block >= len(e.markers.Blocks) {
		return
	}
	out, err := domain.ResolveConflictText(e.ed.String(), block, choice)
	if err != nil {
		w.ws.NotifyError(w.t(i18n.AppNoticeResolveConflictFailed), err)
		return
	}
	e.replaceAll(out)
	e.refresh()
	if len(e.lines) > 0 {
		// Keep the caret where the accepted block was instead of at the end of
		// the buffer the whole-buffer replacement would leave it at.
		line := max(0, min(block, len(e.lines)-1))
		e.ed.MoveTo(utf8.RuneCountInString(e.built[:e.starts[line]]), false)
	}
	if t.conflictBlock >= len(e.markers.Blocks) {
		t.conflictBlock = max(0, len(e.markers.Blocks)-1)
	}
	w.focusNext = idConflictText
	w.Invalidate()
}

// revertConflict restores the buffer to the text it was loaded or last saved
// with. It is one undoable edit and writes nothing.
func (w *Window) revertConflict(t *diffTab) {
	e := t.cedit
	if e == nil || e.saving || !e.dirty {
		return
	}
	e.replaceAll(e.original)
	e.refresh()
	w.focusNext = idConflictText
	w.Invalidate()
}

// saveConflict writes the edited result through app.SaveFile: the file is read
// again before the atomic rename and an external change is refused, keeping the
// buffer. A successful save reloads the tab from disk, so the panes, the marker
// count and the Mark as Resolved control reflect what was written.
func (w *Window) saveConflict(t *diffTab) {
	e := t.cedit
	if e == nil || e.saving {
		return
	}
	e.refresh()
	if !e.dirty {
		return
	}
	if !e.markers.Reliable {
		// Saving an incomplete marker set would leave a file that Git still
		// calls unmerged but that the conflict viewer can no longer edit safely.
		// Keep the buffer so the user can finish or remove the markers.
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.FileviewSaveFailed, t.src.Path), w.t(i18n.ConflictSubUnreliable))
		w.focusNext = idConflictText
		w.Invalidate()
		return
	}
	e.saving = true
	value := e.ed.String()
	w.ws.SaveFile(vapp.SaveFileRequest{
		Path: t.src.Path, Text: value, LineEnding: e.lineEnd, BOM: e.bom, Expected: e.expected,
	}, func(res vapp.SaveFileResult, err error) {
		if t.cedit != e {
			// The tab was closed or reloaded while the save ran.
			return
		}
		e.saving = false
		if err != nil {
			w.notifyConflictSaveError(t, err)
			w.focusNext = idConflictText
			w.Invalidate()
			return
		}
		if e.ed.String() != value {
			// The editor stayed live during the save: keep the text typed
			// after the snapshot dirty against the bytes now on disk.
			e.original = value
			e.expected = res.Hash
			e.refresh()
			w.focusNext = idConflictText
			w.Invalidate()
			return
		}
		t.cedit = nil
		t.x = 0 // the result may be narrower than the line that was being edited
		w.clearConflictInput()
		w.ws.Notify(vapp.NoticeInfo, w.t(i18n.ConflictNoticeSaved), w.t(i18n.ConflictNoticeSavedBody))
		w.loadTab(t)
		w.Invalidate()
	})
}

func (w *Window) notifyConflictSaveError(t *diffTab, err error) {
	w.notifySaveFailure(t, err, func() { w.reloadConflict(t) })
}

// reloadConflict drops the buffer without asking: its callers are the explicit
// Reload action of a failed save, which is itself the confirmation.
func (w *Window) reloadConflict(t *diffTab) {
	if t.cedit == nil {
		return
	}
	t.cedit = nil
	t.x = 0
	w.clearConflictInput()
	w.loadTab(t)
	w.Invalidate()
}

// clearConflictInput drops the cached native input of the Result editor so a
// closed or reloaded buffer is not kept alive by its State and event closure.
func (w *Window) clearConflictInput() {
	if w.inputs != nil {
		delete(w.inputs, idConflictText)
	}
}

// conflictViewForContext is the view Copy for AI renders: the edited buffer when
// one is open, so a copied context matches what the user sees, and the loaded
// view otherwise.
func (w *Window) conflictViewForContext(t *diffTab) *vapp.ConflictView {
	if t == nil || t.res == nil || t.res.Conflict == nil {
		return nil
	}
	cv := t.res.Conflict
	e := t.cedit
	if e == nil {
		return cv
	}
	e.refresh()
	out := *cv
	cur := cv.Current
	cur.Lines, cur.NoEOL = e.lines, e.noFinal
	out.Current = cur
	out.Markers = e.markers
	return &out
}
