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

// openSubmodules lists the submodules and offers update, sync and open. The
// list reloads after a write so the state is always Git's, not a guess.
func (w *Window) openSubmodules() {
	rs := w.repo()
	if rs == nil {
		return
	}
	var (
		subs    []domain.Submodule
		loadErr error
		loading = true
	)
	reload := func() {
		loading = true
		w.ws.Submodules(func(list []domain.Submodule, err error) {
			if w.repo() != rs {
				return
			}
			subs, loadErr, loading = list, err, false
		})
	}
	reload()
	w.openDialog(&dialog{
		title: "Submodules", ok: "Close", noCancel: true, width: 780,
		body: func(f *form) {
			switch {
			case loading:
				f.muted("Loading submodules…")
			case loadErr != nil:
				f.message(vapp.ErrorText(loadErr), w.pal.Error)
			case len(subs) == 0:
				f.muted("This repository has no submodules.")
				f.gap(4)
				f.muted("Add one with git submodule add in a terminal, then reopen this dialog.")
			default:
				const shown = 12
				for i, sm := range subs {
					if i >= shown {
						break
					}
					i, sm := i, sm
					f.custom(submoduleRowH, func(r ui.Rect) { w.paintSubmoduleRow(r, i, sm, reload) })
				}
				if extra := len(subs) - shown; extra > 0 {
					f.muted(itoa(extra) + " more submodules are not shown. Update All still applies to them.")
				}
				f.gap(8)
				f.custom(34, func(r ui.Rect) {
					if w.button(scDialogLinks.At(900), rect(r.X, r.Y+3, 110, 28), "Update All", btnNormal, false) {
						w.after(func() { w.ws.UpdateSubmodule("", true, func(error) { reload() }) })
					}
					if w.button(scDialogLinks.At(901), rect(r.X+118, r.Y+3, 100, 28), "Sync All", btnNormal, false) {
						w.after(func() { w.ws.SyncSubmodules("", func(error) { reload() }) })
					}
				})
			}
		},
		onOK: func() bool { return true },
	})
}

const submoduleRowH = 42

// paintSubmoduleRow draws one submodule: path, commit, state and its actions.
func (w *Window) paintSubmoduleRow(r ui.Rect, i int, sm domain.Submodule, reload func()) {
	const updateW, smallW, gapW float32 = 76, 58, 6
	actions := updateW + gapW + smallW + gapW + smallW
	textW := max(80, r.Width-actions-130)
	w.p.text(sm.Path, rect(r.X, r.Y+2, textW, 20), fontUI, semibold, w.pal.Text)
	detail := sm.ShortSHA()
	if sm.Describe != "" {
		detail += "  ·  " + sm.Describe
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
	if w.button(scDialogLinks.At(i*3), rect(x, r.Y+7, updateW, 26), "Update", btnBorderless, false) {
		w.after(func() {
			w.ws.UpdateSubmodule(sm.Path, true, func(err error) {
				if err == nil {
					reload()
				}
			})
		})
	}
	x += updateW + gapW
	if w.button(scDialogLinks.At(i*3+1), rect(x, r.Y+7, smallW, 26), "Sync", btnBorderless, false) {
		w.after(func() {
			w.ws.SyncSubmodules(sm.Path, func(err error) {
				if err == nil {
					reload()
				}
			})
		})
	}
	x += smallW + gapW
	if w.button(scDialogLinks.At(i*3+2), rect(x, r.Y+7, smallW, 26), "Open", btnBorderless, !sm.Initialized()) {
		w.after(func() {
			w.closeDialog()
			w.openSubmodule(sm.Path)
		})
	}
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
