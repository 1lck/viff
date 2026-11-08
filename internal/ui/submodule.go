package viffui

import (
	"path/filepath"

	"github.com/vivaui/viva/ui"

	"viff/internal/sshpath"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// openSubmodule opens the submodule's working tree as the current project.
// Viff shows one repository per window, so this is the same as opening the
// folder from the welcome screen.
func (w *Window) openSubmodule(path string) {
	rs := w.repo()
	if rs == nil {
		return
	}
	var dir string
	if rs.Repo.IsSSH() {
		abs, err := sshpath.AbsPath(rs.Repo.Root, path)
		if err != nil {
			w.ws.NotifyError(w.t(i18n.SubmoduleCannotOpen), err)
			return
		}
		dir, err = sshpath.URLOf(rs.Repo.Root, abs)
		if err != nil {
			w.ws.NotifyError(w.t(i18n.SubmoduleCannotOpen), err)
			return
		}
	} else {
		dir = filepath.Join(rs.Repo.Root, filepath.FromSlash(path))
	}
	w.confirmDiscardEdits(func() {
		w.ws.Open(dir, func(err error) {
			if err != nil {
				w.ws.NotifyError(w.t(i18n.SubmoduleCannotOpen), err)
			}
		})
	})
}

// openSubmodules lists the submodules and offers add, update, sync, open,
// deinit and remove. The list reloads after a write and every dialog reopens
// it from Git, so what is shown is the repository's state and never a guess.
func (w *Window) openSubmodules() {
	rs := w.repo()
	if rs == nil {
		return
	}
	var (
		subs      []domain.Submodule
		direct    map[string]bool
		loadErr   error
		directErr error
		loading   = true
	)
	reload := func() {
		loading = true
		var pending = 2
		finish := func() {
			pending--
			loading = pending > 0
		}
		w.ws.Submodules(func(list []domain.Submodule, err error) {
			if w.repo() != rs {
				return
			}
			subs, loadErr = list, err
			finish()
		})
		// Nested submodules are listed for update and open, but only the
		// repository that records a submodule can deinit or remove it.
		w.ws.DirectSubmodules(func(paths map[string]bool, err error) {
			if w.repo() != rs {
				return
			}
			direct, directErr = paths, err
			finish()
		})
	}
	reload()
	w.openDialog(&dialog{
		title: w.t(i18n.SubmoduleTitle), ok: w.t(i18n.CommonClose), noCancel: true, width: 940,
		body: func(f *form) {
			f.custom(34, func(r ui.Rect) {
				bw := w.buttonWidth("Add Submodule…")
				if w.button(idSubmoduleAdd, rect(r.X, r.Y+3, bw, 28), "Add Submodule…", btnNormal, w.readOnly()) {
					w.after(func() {
						w.dialog = nil
						w.openSubmoduleAdd()
					})
				}
				empty := loading || len(subs) == 0
				x := r.X + bw + 12
				if w.button(idSubmoduleUpdateAll, rect(x, r.Y+3, 110, 28), "Update All", btnNormal, empty || w.readOnly()) {
					w.after(func() { w.ws.UpdateSubmodule("", true, func(error) { reload() }) })
				}
				x += 118
				if w.button(idSubmoduleSyncAll, rect(x, r.Y+3, 100, 28), "Sync All", btnNormal, empty || w.readOnly()) {
					w.after(func() { w.ws.SyncSubmodules("", func(error) { reload() }) })
				}
			})
			f.gap(6)
			switch {
			case loading && len(subs) == 0:
				f.muted(w.t(i18n.SubmoduleLoading))
			case loadErr != nil:
				f.message(vapp.ErrorText(loadErr), w.pal.Error)
			case len(subs) == 0:
				f.muted(w.t(i18n.SubmoduleNone))
				f.gap(4)
				f.muted(w.t(i18n.SubmoduleAddHint))
			default:
				const shown = 12
				nested := 0
				for i, sm := range subs {
					if direct != nil && !direct[sm.Path] {
						nested++
					}
					if i >= shown {
						continue
					}
					i, sm := i, sm
					f.custom(submoduleRowH, func(r ui.Rect) { w.paintSubmoduleRow(r, i, sm, direct, reload) })
				}
				if extra := len(subs) - shown; extra > 0 {
					f.muted(w.t(i18n.SubmoduleMore, itoa(extra)))
				}
				if nested > 0 || directErr != nil {
					f.gap(8)
					switch {
					case directErr != nil:
						f.muted(w.t(i18n.SubmoduleCannotRead, vapp.ErrorText(directErr)))
					default:
						f.muted(w.t(i18n.SubmoduleNestedHint))
					}
				}
			}
		},
		onOK: func() bool { return true },
	})
}

const submoduleRowH = 42

// submoduleRowLabels are the per-row actions, left to right. They are all
// about the submodule in that row.
var submoduleRowLabels = [...]string{"Update", "Sync", "Open", "Deinit", "Remove…"}

// paintSubmoduleRow draws one submodule: path, commit, state and its actions.
// Remove is offered even for an uninitialized submodule (there is a gitlink to
// remove), while Deinit, like Open, needs a working tree.
func (w *Window) paintSubmoduleRow(r ui.Rect, i int, sm domain.Submodule, direct map[string]bool, reload func()) {
	// Only a direct submodule can be deinitialized or removed here; a nested
	// one is listed for update and open only.
	manageable := direct != nil && direct[sm.Path]
	var (
		widths  [submoduleRowCtlCount]float32
		actions float32
	)
	for j, label := range submoduleRowLabels {
		widths[j] = w.buttonWidth(label)
		actions += widths[j]
	}
	actions += 6 * float32(submoduleRowCtlCount-1)

	textW := max(80, r.Width-actions-130)
	w.p.text(sm.Path, rect(r.X, r.Y+2, textW, 20), fontUI, semibold, w.pal.Text)
	detail := sm.ShortSHA()
	if sm.Describe != "" {
		detail += "  ·  " + sm.Describe
	}
	if direct != nil && !direct[sm.Path] {
		detail += "  ·  nested"
	}
	w.p.text(detail, rect(r.X, r.Y+22, textW, 16), fontSmall, regular, w.pal.TextMuted)

	state := sm.State.String()
	col := w.pal.TextMuted
	switch sm.State {
	case domain.SubmoduleModified:
		col = w.pal.Warning
	case domain.SubmoduleConflict:
		col = w.pal.Error
	}
	w.p.text(state, rect(right(r)-actions-124, r.Y+11, 120, 18), fontSmall, regular, col)

	x := right(r) - actions
	action := func(j int, enabled bool, run func()) {
		if w.button(submoduleRowCtl(i, j), rect(x, r.Y+7, widths[j], 26), submoduleRowLabels[j], btnBorderless, !enabled) {
			run()
		}
		x += widths[j] + 6
	}
	action(0, !w.readOnly(), func() {
		w.after(func() {
			w.ws.UpdateSubmodule(sm.Path, true, func(err error) {
				if err == nil {
					reload()
				}
			})
		})
	})
	action(1, !w.readOnly(), func() {
		w.after(func() {
			w.ws.SyncSubmodules(sm.Path, func(err error) {
				if err == nil {
					reload()
				}
			})
		})
	})
	action(2, sm.Initialized(), func() {
		w.after(func() {
			w.closeDialog()
			w.openSubmodule(sm.Path)
		})
	})
	action(3, !w.readOnly() && sm.Initialized() && manageable, func() {
		w.after(func() { w.startSubmoduleAction(sm, submoduleDeinit) })
	})
	action(4, !w.readOnly() && manageable, func() {
		w.after(func() { w.startSubmoduleAction(sm, submoduleRemove) })
	})
}

// gitMenu is the Git tool window's options menu.
func (w *Window) gitMenu() []item {
	rs := w.repo()
	if rs == nil {
		return nil
	}
	items := []item{}
	// Submodules are a Git concept; a backend without them must not open a tool
	// window that only reports "not available". The list itself is read-only,
	// so a remote repository still offers it; the panel disables its writes.
	if w.ws.Caps.Submodules != nil {
		items = append(items, item{title: w.t(i18n.SubmoduleMenu), icon: icons.Folder, run: w.openSubmodules})
	}
	if w.readOnly() {
		return items
	}
	if len(rs.Remotes) > 0 {
		items = append(items, sep(), item{title: w.t(i18n.MenuFetch), icon: icons.Fetch, run: func() { w.ws.Fetch(false) }})
	}
	return items
}
