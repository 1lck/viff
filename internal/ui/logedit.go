package viffui

import (
	"strings"

	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/domain"
)

// Editing one commit of history from the log: reword, squash into the parent
// and drop. All three replay the commits after the target, so the dialogs only
// open for a single commit with a parent and no operation in flight (the menu
// decides that); the Git layer refuses the rest with an explanation.

// commitMessageEditor is the multi-line message field of the dialogs below.
type commitMessageEditor struct {
	ed     *text.Editor
	layout components.TextareaLayout
}

func (m *commitMessageEditor) paint(w *Window, id ui.ID, r ui.Rect) {
	value := m.ed.String()
	sel := m.ed.Selection()
	ss, se := runeToByte(value, min(sel.Start, sel.End)), runeToByte(value, max(sel.Start, sel.End))
	components.PaintTextarea(w.c, w.theme, r, components.TextareaSnapshot{
		ID: id, Label: "Commit message", Value: value, Placeholder: "Commit message",
		ShowCaret: true, HideEditorText: true, Layout: &m.layout,
		Caret: runeToByte(value, m.ed.Caret()), SelectionStart: ss, SelectionEnd: se,
	})
	w.editors[id] = m.ed
	w.onScrollIn(r, func(dx, dy float32) { m.layout.ScrollBy(dy) })
	w.onKeys(id, func(e ui.KeyEvent) bool {
		mv, extend := components.TextareaMovementForKey(e.Key, e.Modifiers)
		if mv == components.TextareaMoveNone || mv == components.TextareaMoveLineStart || mv == components.TextareaMoveLineEnd {
			return false
		}
		val := m.ed.String()
		nb := m.layout.Move(val, runeToByte(val, m.ed.Caret()), mv)
		m.ed.MoveTo(byteToRune(val, nb), extend)
		return true
	})
}

// openRewordCommit edits the message of c and keeps its content.
func (w *Window) openRewordCommit(c domain.Commit) {
	w.ws.CommitMessage(c.Hash, func(message string, err error) {
		if err != nil {
			w.ws.NotifyError("Cannot load the commit message", err)
			return
		}
		w.openCommitMessageDialog("Edit Commit Message", "Save Message", c, message,
			"The commit keeps its content. The commits after it are replayed on top.",
			func(msg string) { w.ws.RewordCommit(c.Hash, msg) })
	})
}

// openSquashCommit folds c into its parent and asks for the combined message,
// prefilled with the parent's message followed by the squashed one.
func (w *Window) openSquashCommit(c domain.Commit) {
	if len(c.Parents) != 1 {
		return
	}
	w.ws.CommitMessage(c.Parents[0], func(parent string, err error) {
		if err != nil {
			w.ws.NotifyError("Cannot load the parent's message", err)
			return
		}
		w.ws.CommitMessage(c.Hash, func(message string, err error) {
			if err != nil {
				w.ws.NotifyError("Cannot load the commit message", err)
				return
			}
			combined := strings.TrimRight(parent, "\n")
			if m := strings.TrimRight(message, "\n"); strings.TrimSpace(m) != "" {
				combined += "\n\n" + m
			}
			w.openCommitMessageDialog("Squash "+domain.ShortHash(c.Hash)+" into its Parent", "Squash", c, combined,
				"The parent keeps both changes and its author. The commits after it are replayed on top.",
				func(msg string) { w.ws.SquashCommit(c.Hash, msg) })
		})
	})
}

// openDropCommit removes c after a confirmation that names what is lost.
func (w *Window) openDropCommit(c domain.Commit) {
	short := domain.ShortHash(c.Hash)
	w.confirm("Drop Commit", "Drop "+short+" \""+c.Subject+"\"?\n\nIts changes disappear from the branch, and the commits after it are replayed without them. The old commits stay in the reflog until it expires.",
		"Drop Commit", func() { w.ws.DropCommit(c.Hash) })
}

// openCommitMessageDialog shows a commit's message in an editable text area.
func (w *Window) openCommitMessageDialog(title, ok string, c domain.Commit, message, note string, apply func(string)) {
	m := &commitMessageEditor{ed: text.NewEditor(message)}
	w.openDialog(&dialog{
		title: title, ok: ok, width: 600,
		textarea: idLogEditMessage,
		focus:    idLogEditMessage,
		body: func(f *form) {
			f.muted(domain.ShortHash(c.Hash) + "  " + firstLine(c.Subject))
			f.gap(8)
			f.custom(150, func(r ui.Rect) { m.paint(w, idLogEditMessage, r) })
			f.gap(6)
			f.muted(note)
		},
		okDisabled: func() bool { return strings.TrimSpace(m.ed.String()) == "" },
		onOK: func() bool {
			apply(m.ed.String())
			return true
		},
	})
}
