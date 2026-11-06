package viffui

import (
	"path/filepath"

	vivaapp "github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
)

// discovery.go is the "Discover Repositories" dialog. It is opened when the
// user picks a folder that is not itself a repository, or from the Repositories
// overview. The scan runs in the app on a worker goroutine and the dialog only
// renders its result: nested repositories, submodules and linked worktrees are
// listed as separate rows, and importing only adds them to the workspace list.
//
// The scan is explicit and cancellable. Closing the dialog cancels the task, so
// a huge folder never keeps a background walk alive after the user lost
// interest.

const (
	discoveryRowH  = 28
	discoveryListH = 280
)

var (
	scDiscovery     = tree.Sub(fDialog, tree.Name("discovery"))
	scDiscoveryRows = tree.Sub(scDiscovery, tree.Name("rows"))
	scDiscoveryBox  = tree.Sub(scDiscovery, tree.Name("checks"))
	scDiscoveryBar  = tree.Sub(scDiscovery, tree.Name("scroll"))

	idDiscoveryList = tree.ID(scDiscovery, "list")
	idDiscoveryAll  = tree.ID(scDiscovery, "all")
	idDiscoveryNone = tree.ID(scDiscovery, "none")
)

// discoveryView is the dialog's state. It is only touched on the UI thread.
type discoveryView struct {
	root         string
	repos        []domain.Repo
	selected     map[string]bool
	sel          int
	scroll       scroller
	loading      bool
	closed       bool
	err          string
	truncated    bool
	scanned      int
	inaccessible int
	task         uint64
}

// openRepositoryDiscovery scans root and asks which repositories to add.
func (w *Window) openRepositoryDiscovery(root string) {
	root = canonicalRepositoryRoot(root)
	v := &discoveryView{root: root, selected: make(map[string]bool), loading: true}
	w.openDialog(&dialog{
		title: w.t(i18n.DiscoverTitle),
		width: 720,
		ok:    w.t(i18n.DiscoverImport),
		okDisabled: func() bool {
			return v.loading || len(v.selected) == 0
		},
		onOK: func() bool {
			roots := v.selectedRoots()
			if len(roots) == 0 {
				return false
			}
			v.closed = true
			w.after(func() { w.importDiscoveredRepositories(roots) })
			return true
		},
		onCancel: func() {
			v.closed = true
			if v.task != 0 {
				w.ws.CancelTask(v.task)
			}
		},
		body: func(f *form) { w.paintDiscovery(f, v) },
	})
	v.task = w.ws.DiscoverRepositories(root, vapp.DiscoveryOptions{}, func(res vapp.DiscoveryResult, err error) {
		w.after(func() {
			if v.closed {
				return
			}
			v.loading = false
			v.repos = res.Repositories
			v.scanned = res.Scanned
			v.inaccessible = res.Inaccessible
			v.truncated = res.Truncated
			for _, repo := range v.repos {
				v.selected[canonicalRepositoryRoot(repo.Root)] = true
			}
			if err != nil {
				v.err = repositoryOneLine(vapp.ErrorText(err))
			}
			w.Invalidate()
		})
	})
	w.Invalidate()
}

// importDiscoveredRepositories adds the chosen roots and then shows the
// overview, where the new rows can be inspected and activated. The scan result
// is a snapshot: AddRepositories discovers every root again, so a repository
// that vanished or changed meanwhile is reported instead of trusted.
func (w *Window) importDiscoveredRepositories(roots []string) {
	w.ws.AddRepositories(roots, func(res vapp.AddRepositoriesResult) {
		w.after(func() {
			if res.FirstErr != nil {
				w.ws.Notify(vapp.NoticeWarning, w.t(i18n.DiscoverTitle),
					w.t(i18n.DiscoverCannotAdd, vapp.ErrorText(res.FirstErr)))
			}
			w.openRepositories()
			w.Invalidate()
		})
	})
	w.Invalidate()
}

// browseDiscoveryRoot asks for a folder and scans it. Unlike opening a
// repository this works for any directory, including a repository root, so
// nested repositories inside an already known project are reachable too.
func (w *Window) browseDiscoveryRoot() {
	dir, err := vivaapp.OpenDirectory(vivaapp.OpenDirectoryOptions{Title: w.t(i18n.DiscoverAddTitle)})
	if err != nil || dir == "" {
		return
	}
	w.after(func() { w.openRepositoryDiscovery(dir) })
}

// paintDiscovery draws the scan status, the select controls and the candidate
// list.
func (w *Window) paintDiscovery(f *form, v *discoveryView) {
	f.custom(22, func(r ui.Rect) {
		msg, col := "", w.pal.TextSecond
		switch {
		case v.err != "":
			msg, col = w.t(i18n.DiscoverFailed, shortenHome(v.root), v.err), w.pal.Error
		case v.loading:
			msg = w.t(i18n.DiscoverScanning, shortenHome(v.root))
		case len(v.repos) == 0:
			msg = w.t(i18n.DiscoverNone, shortenHome(v.root))
		case len(v.repos) == 1:
			msg = w.t(i18n.DiscoverFoundOne, shortenHome(v.root))
		default:
			msg = w.t(i18n.DiscoverFoundMany, len(v.repos), shortenHome(v.root))
		}
		w.p.text(w.p.truncateMiddle(msg, fontUI, regular, r.Width), rect(r.X, r.Y, r.Width, 20), fontUI, regular, col)
	})
	f.custom(fieldH, func(r ui.Rect) {
		disabled := v.loading || len(v.repos) == 0
		aw := w.buttonWidth(w.t(i18n.DiscoverSelectAll))
		if w.button(idDiscoveryAll, rect(r.X, r.Y, aw, fieldH), w.t(i18n.DiscoverSelectAll), btnNormal, disabled) {
			w.after(func() {
				for _, repo := range v.repos {
					v.selected[canonicalRepositoryRoot(repo.Root)] = true
				}
			})
		}
		nw := w.buttonWidth(w.t(i18n.DiscoverSelectNone))
		if w.button(idDiscoveryNone, rect(r.X+aw+8, r.Y, nw, fieldH), w.t(i18n.DiscoverSelectNone), btnNormal, disabled) {
			w.after(func() {
				for _, repo := range v.repos {
					delete(v.selected, canonicalRepositoryRoot(repo.Root))
				}
			})
		}
	})
	f.gap(6)
	f.custom(discoveryListH, func(r ui.Rect) { w.paintDiscoveryList(v, r) })
	f.custom(20, func(r ui.Rect) {
		switch {
		case v.truncated:
			w.p.text(w.t(i18n.DiscoverTruncated), rect(r.X, r.Y, r.Width, 18), fontSmall, regular, w.pal.TextSecond)
		case v.inaccessible > 0:
			w.p.text(w.t(i18n.DiscoverInaccessible, v.inaccessible), rect(r.X, r.Y, r.Width, 18), fontSmall, regular, w.pal.TextSecond)
		}
	})
}

// paintDiscoveryList draws the scrollable candidate rows. A row click moves the
// cursor; the checkbox (or Space) toggles the selection.
func (w *Window) paintDiscoveryList(v *discoveryView, r ui.Rect) {
	w.p.outline(r, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
	inner := inset(r, 1, 1)
	content := float32(len(v.repos)) * discoveryRowH
	v.scroll.clamp(content, inner.Height)
	w.onScrollIn(inner, func(dx, dy float32) {
		v.scroll.Y += dy
		v.scroll.clamp(content, inner.Height)
	})
	w.list(idDiscoveryList, scDiscoveryRows, inner, w.t(i18n.DiscoverList))
	w.onKeys(idDiscoveryList, func(e ui.KeyEvent) bool {
		switch e.Key {
		case ui.KeyDown:
			w.after(func() {
				if v.sel < len(v.repos)-1 {
					v.sel++
				}
				v.scroll.reveal(float32(v.sel)*discoveryRowH, discoveryRowH, inner.Height)
			})
		case ui.KeyUp:
			w.after(func() {
				if v.sel > 0 {
					v.sel--
				}
				v.scroll.reveal(float32(v.sel)*discoveryRowH, discoveryRowH, inner.Height)
			})
		case ui.KeySpace:
			if v.sel >= 0 && v.sel < len(v.repos) {
				i := v.sel
				w.after(func() { v.toggle(i) })
			}
		default:
			return false
		}
		return true
	})
	if v.loading {
		w.p.textCenter(w.t(i18n.DiscoverScanningShort), inner, fontUI, regular, w.pal.TextSecond)
		return
	}
	if len(v.repos) == 0 {
		w.p.textCenter(w.t(i18n.DiscoverEmptyList), inner, fontUI, regular, w.pal.TextSecond)
		return
	}
	w.c.Clip(inner)
	for i, repo := range v.repos {
		row := rect(inner.X, inner.Y+float32(i)*discoveryRowH-v.scroll.Y, inner.Width, discoveryRowH)
		if bottom(row) <= inner.Y || row.Y >= bottom(inner) {
			continue
		}
		key := canonicalRepositoryRoot(repo.Root)
		checked := v.selected[key]
		id := scDiscoveryRows.At(i)
		if v.sel == i {
			w.p.round(row, 4, w.pal.Selection)
		} else if w.hovered(id) {
			w.p.round(row, 4, w.pal.Hover)
		}
		name := filepath.Base(repo.Root)
		hint := ""
		if rel, err := filepath.Rel(v.root, repo.Root); err == nil && rel != "." && rel != "" {
			hint = rel
		}
		label := name
		if hint != "" {
			label += "  " + hint
		}
		w.c.HitWithOptions(id, row, label, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: v.sel == i})
		w.tip(id, repo.Root)
		// The checkbox is painted after the row hit area so a click on it
		// toggles the check instead of only moving the cursor.
		if w.checkbox(scDiscoveryBox.At(i), row.X+10, row.Y, row.Height, "", checked, false) {
			i := i
			w.after(func() { v.toggle(i) })
		}
		tx := row.X + 34
		nw := w.p.text(name, rect(tx, row.Y, row.Width-42, row.Height), fontUI, regular, w.pal.Text)
		if hint != "" {
			hx := tx + nw + 10
			if hx < right(row)-8 {
				w.p.text(w.p.truncateMiddle(hint, fontSmall, regular, right(row)-hx-8),
					rect(hx, row.Y, right(row)-hx-8, row.Height), fontSmall, regular, w.pal.TextSecond)
			}
		}
		if w.c.Clicked(id) {
			i := i
			w.after(func() { v.sel = i })
		}
		w.onDouble(id, func() {
			i := i
			w.after(func() { v.toggle(i) })
		})
	}
	w.c.Unclip()
	w.vbar(scDiscoveryBar.At(0), inner, &v.scroll, content)
}

func (v *discoveryView) toggle(i int) {
	if i < 0 || i >= len(v.repos) {
		return
	}
	key := canonicalRepositoryRoot(v.repos[i].Root)
	if v.selected[key] {
		delete(v.selected, key)
	} else {
		v.selected[key] = true
	}
}

func (v *discoveryView) selectedRoots() []string {
	out := make([]string, 0, len(v.selected))
	for _, repo := range v.repos {
		if v.selected[canonicalRepositoryRoot(repo.Root)] {
			out = append(out, repo.Root)
		}
	}
	return out
}
