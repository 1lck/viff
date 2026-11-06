package viffui

import (
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
)

// openCompareCommits lists the files that differ between two commits. from is
// the old side, to the new one; the commits need not be parent and child.
// Double-clicking a file (or Open Diff) opens it in the editor as A → B.
func (w *Window) openCompareCommits(from, to string) {
	var changes []domain.Change
	loading, loadErr := true, error(nil)
	sel := -1
	scroll := &scroller{}
	w.ws.CompareCommits(from, to, func(list []domain.Change, err error) {
		changes, loading, loadErr = list, false, err
		if err == nil && sel < 0 && len(list) > 0 {
			sel = 0
		}
	})
	open := func() {
		if sel < 0 || sel >= len(changes) {
			return
		}
		ch := changes[sel]
		w.dialog = nil
		w.showDiff(vapp.DiffSource{Path: ch.Path, OldPath: ch.OldPath, Commit: to, Parent: from, Kind: ch.Kind}, true)
	}
	w.openDialog(&dialog{
		title: w.t(i18n.CompareTitle, domain.ShortHash(from), domain.ShortHash(to)),
		ok:    "Open Diff", width: 760,
		body: func(f *form) {
			f.custom(26, func(r ui.Rect) {
				x := r.X
				x += w.p.text(domain.ShortHash(from), rect(x, r.Y, 90, r.Height), fontUI, semibold, w.pal.Text)
				x += w.p.text(" → ", rect(x, r.Y, 28, r.Height), fontUI, regular, w.pal.TextMuted)
				x += w.p.text(domain.ShortHash(to), rect(x, r.Y, 90, r.Height), fontUI, semibold, w.pal.Text)
				if loading || loadErr != nil {
					return
				}
				info := plural(len(changes), "file changed", "files changed")
				iw := w.p.width(info, fontUI, regular)
				if x+16 < right(r)-iw {
					w.p.text(info, rect(right(r)-iw, r.Y, iw, r.Height), fontUI, regular, w.pal.TextSecond)
				}
			})
			f.gap(8)
			f.custom(300, func(lr ui.Rect) {
				w.p.outline(lr, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
				inner := inset(lr, 1, 1)
				switch {
				case loading:
					w.p.textCenter(w.t(i18n.CompareLoading), inner, fontUI, regular, w.pal.TextSecond)
					return
				case loadErr != nil:
					w.p.textCenter(w.ws.ErrorText(loadErr), inner, fontUI, regular, w.pal.Error)
					return
				case len(changes) == 0:
					w.p.textCenter(w.t(i18n.CompareNoDiff), inner, fontUI, regular, w.pal.TextSecond)
					return
				}
				content := float32(len(changes))*rowH + 8
				scroll.clamp(content, inner.Height)
				w.onScrollIn(inner, func(dx, dy float32) { scroll.Y += dy; scroll.clamp(content, inner.Height) })
				w.c.Clip(inner)
				for i, ch := range changes {
					y := inner.Y + 4 + float32(i)*rowH - scroll.Y
					if y+rowH < inner.Y || y > bottom(inner) {
						continue
					}
					row := rect(inner.X+4, y, inner.Width-8, rowH)
					id := scDialogRows.At(i)
					if sel == i {
						w.p.round(row, 4, w.pal.Selection)
					} else if w.hovered(id) {
						w.p.round(row, 4, w.pal.Hover)
					}
					x := row.X + 8
					x += w.p.text(ch.Kind.Letter(), rect(x, y, 12, rowH), fontUI, semibold, w.statusColor(ch.Kind)) + 8
					w.p.iconIn(fileIcon(ch.Path), rect(x, y, 16, rowH))
					x += 22
					tw := w.p.text(domain.FileName(ch.Path), rect(x, y, right(row)-x-10, rowH), fontUI, regular, w.pal.Text)
					hint := domain.DirName(ch.Path)
					if ch.OldPath != "" {
						hint = "← " + ch.OldPath
					}
					if hint != "" && x+tw+8 < right(row)-10 {
						hx := x + tw + 8
						w.p.text(w.p.truncateMiddle(hint, fontUI, regular, max(0, right(row)-hx-12)), rect(hx, y, right(row)-hx-10, rowH), fontUI, regular, w.pal.TextSecond)
					}
					w.c.HitWithOptions(id, row, ch.Path, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: sel == i})
					if w.c.Clicked(id) {
						i := i
						w.after(func() { sel = i })
					}
					ii := i
					w.onDouble(id, func() { sel = ii; open() })
				}
				w.c.Unclip()
				w.vbar(idCompareScroll, inner, scroll, content)
			})
			f.gap(8)
			f.muted(w.t(i18n.CompareHint))
		},
		okDisabled: func() bool { return sel < 0 || sel >= len(changes) },
		onOK:       func() bool { open(); return true },
	})
}

// compareBranchFiles compares the tips of two branches.
func (w *Window) compareBranchFiles(other domain.Ref) {
	rs := w.repo()
	if rs == nil || other.Target == "" || rs.Status.Branch.Head == "" {
		return
	}
	w.openCompareCommits(rs.Status.Branch.Head, other.Target)
}
