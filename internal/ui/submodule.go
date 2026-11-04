package viffui

import (
	"path/filepath"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
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
	dir := filepath.Join(rs.Repo.Root, filepath.FromSlash(path))
	w.ws.Open(dir, func(err error) {
		if err != nil {
			w.ws.NotifyError("Cannot open submodule", err)
		}
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
		title: "Submodules", ok: "Close", noCancel: true, width: 940,
		body: func(f *form) {
			f.custom(34, func(r ui.Rect) {
				bw := w.buttonWidth("Add Submodule…")
				if w.button(idSubmoduleAdd, rect(r.X, r.Y+3, bw, 28), "Add Submodule…", btnNormal, false) {
					w.after(func() {
						w.dialog = nil
						w.openSubmoduleAdd()
					})
				}
				empty := loading || len(subs) == 0
				x := r.X + bw + 12
				if w.button(idSubmoduleUpdateAll, rect(x, r.Y+3, 110, 28), "Update All", btnNormal, empty) {
					w.after(func() { w.ws.UpdateSubmodule("", true, func(error) { reload() }) })
				}
				x += 118
				if w.button(idSubmoduleSyncAll, rect(x, r.Y+3, 100, 28), "Sync All", btnNormal, empty) {
					w.after(func() { w.ws.SyncSubmodules("", func(error) { reload() }) })
				}
			})
			f.gap(6)
			switch {
			case loading && len(subs) == 0:
				f.muted("Loading submodules…")
			case loadErr != nil:
				f.message(vapp.ErrorText(loadErr), w.pal.Error)
			case len(subs) == 0:
				f.muted("This repository has no submodules.")
				f.gap(4)
				f.muted("Add one with Add Submodule…; Git records it in .gitmodules and stages the change.")
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
					f.muted(itoa(extra) + " more submodules are not shown. Update All still applies to them.")
				}
				if nested > 0 || directErr != nil {
					f.gap(8)
					switch {
					case directErr != nil:
						f.muted("Cannot read .gitmodules (" + vapp.ErrorText(directErr) + "), so Deinit and Remove are unavailable.")
					default:
						f.muted("A nested submodule belongs to the repository that records it; deinit and remove it there, in that repository's own Submodules dialog.")
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
	action(0, true, func() {
		w.after(func() {
			w.ws.UpdateSubmodule(sm.Path, true, func(err error) {
				if err == nil {
					reload()
				}
			})
		})
	})
	action(1, true, func() {
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
	action(3, sm.Initialized() && manageable, func() {
		w.after(func() { w.startSubmoduleAction(sm, submoduleDeinit) })
	})
	action(4, manageable, func() {
		w.after(func() { w.startSubmoduleAction(sm, submoduleRemove) })
	})
}

// gitMenu is the Git tool window's options menu.
func (w *Window) gitMenu() []item {
	rs := w.repo()
	if rs == nil {
		return nil
	}
	items := []item{
		{title: "Submodules…", icon: icons.Folder, run: w.openSubmodules},
	}
	if len(rs.Remotes) > 0 {
		items = append(items, sep(), item{title: "Fetch", icon: icons.Fetch, run: func() { w.ws.Fetch(false) }})
	}
	return items
}
