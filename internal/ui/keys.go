package viffui

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"
)

func newEditor() *text.Editor { return text.NewEditor("") }

// keyChar is the character of a chord, lower-cased.
func keyChar(e ui.KeyEvent) string {
	s := e.Text
	if s == "" {
		s = e.Typed
	}
	return strings.ToLower(s)
}

// globalKey handles named keys nobody else took.
func (w *Window) globalKey(e ui.KeyEvent) bool {
	if w.ws.Repo == nil {
		return false
	}
	t := w.ed.cur()
	switch e.Key {
	case ui.KeyF7:
		if t != nil && t.res != nil {
			w.jumpBlock(t, e.Modifiers&ui.ModifierShift == 0)
			return true
		}
	case ui.KeyF4:
		if t != nil && (t.src.Working || t.src.View && t.src.Commit == "") {
			w.openExternal(t.src.Path, w.currentLine(t))
			return true
		}
	case ui.KeyEscape:
		if t != nil && t.searchOpen {
			t.searchOpen = false
			return true
		}
		if t != nil && t.src.View && t.textSel.set {
			t.clearTextSel()
			return true
		}
		if w.notifyOpen {
			w.notifyOpen = false
			return true
		}
	}
	if e.Modifiers&ui.ModifierAlt != 0 && e.Modifiers&ui.ModifierShift != 0 && keyChar(e) == "`" {
		w.openBranchPopup(rect(w.size.Width/2-170, toolbarH+40, 340, 0))
		return true
	}
	return false
}

// shortcut handles Cmd/Ctrl chords.
func (w *Window) shortcut(e ui.KeyEvent) bool {
	mod := e.Modifiers&ui.ModifierMeta != 0 || e.Modifiers&ui.ModifierControl != 0
	if !mod {
		return false
	}
	shift := e.Modifiers&ui.ModifierShift != 0
	alt := e.Modifiers&ui.ModifierAlt != 0
	ch := keyChar(e)
	if w.dialog != nil && w.dialog.shortcut != nil && len(w.popups) == 0 {
		return w.dialog.shortcut(e)
	}
	if w.dialog != nil || len(w.popups) > 0 {
		return false
	}
	switch ch {
	case ",":
		w.openSettings()
		return true
	case "o":
		if !shift {
			w.chooseRepoNow()
			return true
		}
		w.openQuickOpen()
		return true
	}
	if w.ws.Repo == nil {
		return false
	}
	rs := w.repo()
	switch {
	case ch == "k" && shift:
		w.openPush()
	case ch == "k":
		w.focusCommit()
	case ch == "t":
		if len(rs.Remotes) > 0 {
			w.openUpdate()
		}
	case ch == "d":
		w.showDiffForFocus()
	case ch == "f" && shift:
		w.openFindInFiles()
	case ch == "f":
		if t := w.ed.cur(); t != nil && t.res != nil && !w.focused(idLogTable) && !w.focused(idLogSearch) {
			w.openDiffSearch()
		} else {
			w.focusLogSearch()
		}
	case ch == "s":
		t := w.ed.cur()
		if t == nil || t.edit == nil {
			return false
		}
		w.after(func() { w.saveEdit(t) })
	case ch == "e":
		t := w.ed.cur()
		if t == nil {
			return false
		}
		switch {
		case t.edit != nil:
			w.after(func() { w.requestExitEdit(t) })
		case w.canEdit(t):
			w.after(func() { w.enterEdit(t) })
		default:
			return false
		}
	case ch == "w":
		if len(w.ed.tabs) > 0 {
			w.requestCloseTab(w.ed.active)
		}
	case ch == "0":
		w.toggleCommit()
	case ch == "1":
		w.toggleProject()
	case ch == "9":
		w.toggleGitWindow()
	case ch == "z" && alt:
		if len(w.rollbackTargets()) > 0 {
			w.openRollback()
		}
	case ch == "z" && !shift:
		// A focused text field keeps Command-Z for its own undo; only the
		// window outside text editing reverses a repository operation.
		if _, editing := w.editors[w.c.Focused()]; editing {
			return false
		}
		w.ws.Undo()
	case ch == "n" && alt:
		if rs.HasHead() {
			w.openNewBranch("", rs.HeadLabel())
		}
	case ch == "a" && w.focused(idChangesList):
		for _, r := range w.cm.rows {
			w.cm.sel[r.key] = true
		}
	case ch == "a" && !shift && (w.focused(idDiffText) || w.focused(idDiffTextOld) || w.focused(idDiffRight)):
		// Select All in the read-only file view; a focused text field keeps
		// Command+A for its own text (Viva consumes the chord before this).
		t := w.ed.cur()
		if t == nil || t.edit != nil || !t.selectAllText(w.ws.Settings.DiffUnified) {
			return false
		}
	case ch == "c" && !shift && !alt:
		// Copy the file view's selection; Shift+Command+C keeps copying paths
		// in the lists below.
		if !w.copyViewSelection() {
			return false
		}
	case ch == "c" && shift && w.focused(idChangesList):
		if sel := w.selectedEntries(); len(sel) > 0 {
			w.copyText(w.repoPath(sel[0].entry.Path))
		}
	case ch == "c" && alt && shift && w.focused(idLogTable):
		var hs []string
		for _, c := range w.selectedCommits() {
			hs = append(hs, c.Hash)
		}
		w.copyText(strings.Join(hs, " "))
	case (ch == "=" || ch == "+") && w.focused(idChangesList):
		clear(w.cm.collapsed)
	case ch == "-" && w.focused(idChangesList):
		for _, a := range areaOrder {
			w.cm.collapsed[rowKey(a, "")] = true
		}
	case ch == "]" && shift:
		if n := len(w.ed.tabs); n > 0 {
			w.ed.activate((w.ed.active + 1) % n)
		}
	case ch == "[" && shift:
		if n := len(w.ed.tabs); n > 0 {
			w.ed.activate((w.ed.active + n - 1) % n)
		}
	case ch == "r" && !shift:
		w.ws.Refresh(true)
	default:
		return false
	}
	return true
}

// showDiffForFocus opens the diff of whatever list has focus.
func (w *Window) showDiffForFocus() {
	switch {
	case w.focused(idDetailsFiles):
		if d := w.repo().Detail; d != nil && w.lg.dtSel >= 0 {
			w.showCommitFile(d, w.lg.dtSel, true)
		}
	default:
		w.openSelectedDiff(true)
	}
}

// ---- background fetch -------------------------------------------------------

var fetchDue atomic.Bool

// startAutoFetch (re)starts the background fetch timer from settings.
func (w *Window) startAutoFetch() {
	w.stopAutoFetch()
	s := w.ws.Settings
	if !s.AutoFetch || s.AutoFetchMins < 1 {
		return
	}
	stop := make(chan struct{})
	w.fetchStop = stop
	every := time.Duration(s.AutoFetchMins) * time.Minute
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				fetchDue.Store(true)
				w.Invalidate()
			}
		}
	}()
}

// stopAutoFetch ends the timer goroutine, if any.
func (w *Window) stopAutoFetch() {
	if w.fetchStop != nil {
		close(w.fetchStop)
		w.fetchStop = nil
	}
}

// runDue performs timer work on the UI thread.
func (w *Window) runDue() {
	if p := w.pendingCrash.Swap(nil); p != nil {
		w.ShowCrashNotice(*p)
	}
	if fetchDue.Swap(false) && w.ws.Repo != nil && !w.ws.Busy() && !w.ws.ReadOnly() && len(w.ws.Repo.Remotes) > 0 {
		w.ws.Fetch(true)
	}
}
