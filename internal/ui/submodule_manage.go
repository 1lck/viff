package viffui

import (
	"errors"
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/i18n"
)

// submodule_manage.go holds the Add Submodule, Deinit and Remove dialogs of
// the Submodules window. Every action is about one submodule the user picked
// in the list: nothing here selects paths or guesses which submodule to touch.
// Git's answer decides the outcome; the dialogs keep Git's message in front of
// the user instead of turning a failure into a success note.

// The dialogs name their controls in a scope of their own: ids.go holds the
// shared ones, and these are painted only by the submodule window.
var (
	scSubmoduleManage    = tree.Sub(fDialog, tree.Name("submodule-manage"))
	scSubmoduleRowCtl    = tree.Sub(scSubmoduleManage, tree.Name("row-actions"))
	idSubmoduleAdd       = scSubmoduleManage.ID(tree.Name("add"))
	idSubmoduleUpdateAll = scSubmoduleManage.ID(tree.Name("update-all"))
	idSubmoduleSyncAll   = scSubmoduleManage.ID(tree.Name("sync-all"))
)

// submoduleRowCtlCount is the number of buttons on one submodule row; they
// share the row's scope with consecutive items.
const submoduleRowCtlCount = 5

func submoduleRowCtl(i, button int) ui.ID {
	return scSubmoduleRowCtl.At(i*submoduleRowCtlCount + button)
}

// submoduleAction is what the user asked to do with a listed submodule.
type submoduleAction int

const (
	submoduleDeinit submoduleAction = iota
	submoduleRemove
)

// startSubmoduleAction asks Git whether the chosen submodule has local work,
// then opens the confirmation with that answer. Reading Git first means the
// warning and the Force button appear only when they are true, and the write
// itself checks again, so a change made in between still cannot be discarded
// silently.
func (w *Window) startSubmoduleAction(sm domain.Submodule, action submoduleAction) {
	if w.guardReadOnly() {
		return
	}
	w.ws.SubmoduleDirty(sm.Path, func(dirty bool, err error) {
		w.after(func() {
			if err != nil {
				w.ws.NotifyError(w.t(i18n.SubmoduleCannotReadOne, sm.Path), err)
				return
			}
			w.confirmSubmoduleAction(sm, action, dirty, "")
		})
	})
}

// confirmSubmoduleAction asks before a submodule's checkout is removed or
// emptied. With force the dialog names exactly what is lost and the OK button
// says Force; Git's refusal (from a change made after the check) reopens this
// dialog as the force variant instead of passing force on its own.
func (w *Window) confirmSubmoduleAction(sm domain.Submodule, action submoduleAction, force bool, reason string) {
	title, ok := "Deinit Submodule", "Deinit"
	if action == submoduleRemove {
		title, ok = "Remove Submodule", "Remove"
	}
	if force {
		ok = "Force " + ok
	}
	var d *dialog
	pending := false
	d = &dialog{
		title: title, ok: ok, width: 600,
		onCancel: func() { w.after(w.openSubmodules) },
		body: func(f *form) {
			if action == submoduleRemove {
				f.text(w.t(i18n.SubmoduleRemoveQuestion, sm.Path))
				f.gap(4)
				f.muted(w.t(i18n.SubmoduleRemoveHint))
			} else {
				f.text(w.t(i18n.SubmoduleDeinitQuestion, sm.Path))
				f.gap(4)
				f.muted(w.t(i18n.SubmoduleDeinitHint))
			}
			if force {
				f.gap(8)
				f.message(w.t(i18n.SubmoduleForceHint), w.pal.Warning)
				if reason != "" {
					f.gap(4)
					f.message(reason, w.pal.Warning)
				}
			}
		},
		okDisabled: func() bool { return pending },
		onOK: func() bool {
			pending = true
			done := func(err error) {
				switch {
				case err == nil:
					w.after(func() {
						w.dialog = nil
						w.openSubmodules()
					})
				case !force && errors.Is(err, gitcli.ErrSubmoduleNeedsForce):
					// Git found local work the earlier check did not see:
					// ask again, now naming what will be discarded.
					pending = false
					msg := vapp.ErrorText(err)
					w.after(func() {
						w.dialog = nil
						w.confirmSubmoduleAction(sm, action, true, msg)
					})
				default:
					pending = false
					d.err = vapp.ErrorText(err)
					w.Invalidate()
				}
			}
			if action == submoduleRemove {
				w.ws.RemoveSubmodule(sm.Path, force, done)
			} else {
				w.ws.DeinitSubmodule(sm.Path, force, done)
			}
			return false
		},
	}
	w.openDialog(d)
}

// openSubmoduleAdd is the Add Submodule dialog: a URL or local path, the
// directory inside this repository, and Git's error on failure. The form
// validates the path before Git starts because a bad path is a typo, while a
// clone failure is Git's to report.
func (w *Window) openSubmoduleAdd() {
	if w.guardReadOnly() {
		return
	}
	url := text.NewEditor("")
	path := text.NewEditor("")
	lastAuto := ""
	var d *dialog
	pending := false

	// pathOrSuggestion is what will be passed to Git: the typed path, or the
	// directory Git would derive from the URL while the field is untouched.
	pathOrSuggestion := func() string {
		if p := strings.TrimSpace(path.String()); p != "" {
			return p
		}
		return vapp.SubmodulePathFromURL(strings.TrimSpace(url.String()))
	}
	problem := func() string {
		if p := vapp.SubmoduleURLProblem(url.String()); p != "" {
			return p
		}
		return vapp.SubmodulePathProblem(pathOrSuggestion())
	}

	d = &dialog{
		title: w.t(i18n.SubmoduleAddTitle), ok: w.t(i18n.DialogAdd), width: 640,
		onCancel: func() { w.after(w.openSubmodules) },
		body: func(f *form) {
			f.labelW = 70
			// Suggest the path Git would pick until the user edits it.
			auto := vapp.SubmodulePathFromURL(strings.TrimSpace(url.String()))
			if path.String() == lastAuto && auto != lastAuto {
				lastAuto = auto
				w.after(func() { setText(path, auto) })
			}
			id := f.field(w.t(i18n.DialogURL), url, "https://github.com/user/repo.git", vapp.SubmoduleURLProblem(url.String()) != "")
			if !d.focused {
				d.focus = id
			}
			f.field(w.t(i18n.WorktreePath), path, "deps/lib", vapp.SubmodulePathProblem(strings.TrimSpace(path.String())) != "" && strings.TrimSpace(path.String()) != "")
			f.gap(2)
			f.muted(w.t(i18n.SubmoduleAddBody))
			f.gap(4)
			f.muted(w.t(i18n.SubmoduleLocalPathHint))
			if p := problem(); p != "" && (strings.TrimSpace(url.String()) != "" || strings.TrimSpace(path.String()) != "") {
				f.gap(4)
				f.message(p, w.pal.Error)
			}
		},
		okDisabled: func() bool { return pending || problem() != "" },
		onOK: func() bool {
			pending = true
			w.ws.AddSubmodule(strings.TrimSpace(url.String()), pathOrSuggestion(), func(err error) {
				if err == nil {
					w.after(func() {
						w.dialog = nil
						w.openSubmodules()
					})
					return
				}
				pending = false
				d.err = vapp.ErrorText(err)
				w.Invalidate()
			})
			return false
		},
	}
	w.openDialog(d)
}
