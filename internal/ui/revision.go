package viffui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/ui/icons"
)

// The revision browser lists the files of one commit, like IntelliJ's "Show
// Repository at Revision". Double-clicking a file opens it read-only at that
// revision through the existing file view.
var (
	idRevisionFilter = tree.ID(fDialog, "revision-filter")
	idRevisionScroll = tree.ID(fDialog, "revision-scroll")
	scRevisionRows   = tree.Sub(fDialog, tree.Name("revision-rows"))
)

func (w *Window) openRevisionBrowser(c domain.Commit) {
	rev := c.Hash
	var entries []domain.TreeEntry
	// shown indexes the entries that pass the filter, so the full tree stays
	// in memory while the list only paints what the filter keeps.
	var shown []int
	loading, loadErr := true, error(nil)
	sel := -1
	filter := text.NewEditor("")
	query := "\x00" // the filter text the current shown list was built from
	scroll := &scroller{}
	viewH := float32(0)

	apply := func() {
		q := strings.ToLower(strings.TrimSpace(filter.String()))
		query = q
		shown = shown[:0]
		for i, e := range entries {
			if q == "" || strings.Contains(strings.ToLower(e.Path), q) {
				shown = append(shown, i)
			}
		}
		if sel >= len(shown) {
			sel = len(shown) - 1
		}
		if sel < 0 && len(shown) > 0 {
			sel = 0
		}
		scroll.Y = 0
	}
	open := func() {
		if sel < 0 || sel >= len(shown) {
			return
		}
		e := entries[shown[sel]]
		w.dialog = nil
		w.openFileView(e.Path, rev, false)
	}

	w.ws.RevisionTree(rev, func(list []domain.TreeEntry, err error) {
		entries, loading, loadErr = list, false, err
		if err == nil {
			apply()
		}
	})

	w.openDialog(&dialog{
		title: "Repository at " + domain.ShortHash(rev),
		ok:    "Open File", width: 760,
		body: func(f *form) {
			f.custom(26, func(r ui.Rect) {
				x := r.X
				x += w.p.text(domain.ShortHash(rev), rect(x, r.Y, 90, r.Height), fontUI, semibold, w.pal.Text)
				x += 8
				aw := float32(0)
				if c.Author != "" {
					aw = w.p.width(c.Author, fontUI, regular)
				}
				room := max(0, right(r)-x-aw-16)
				subject := w.p.truncateMiddle(c.Subject, fontUI, regular, room)
				w.p.text(subject, rect(x, r.Y, room, r.Height), fontUI, regular, w.pal.Text)
				if aw > 0 && x+room+8 < right(r) {
					w.p.text(c.Author, rect(right(r)-aw, r.Y, aw, r.Height), fontUI, regular, w.pal.TextSecond)
				}
			})
			f.gap(10)
			f.custom(30, func(r ui.Rect) {
				if w.field(idRevisionFilter, filter, r, fieldOpts{icon: icons.Search, placeholder: "Filter files"}) {
					w.after(w.submitDialog)
				}
				w.onKeys(idRevisionFilter, func(e ui.KeyEvent) bool {
					switch e.Key {
					case ui.KeyDown:
						if len(shown) > 0 {
							sel = min(sel+1, len(shown)-1)
							scroll.reveal(float32(sel)*rowH, rowH, viewH)
							w.c.RequestFrame()
						}
						return true
					case ui.KeyUp:
						if len(shown) > 0 {
							sel = max(sel-1, 0)
							scroll.reveal(float32(sel)*rowH, rowH, viewH)
							w.c.RequestFrame()
						}
						return true
					}
					return false
				})
			})
			f.gap(10)
			f.custom(360, func(lr ui.Rect) {
				w.p.outline(lr, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
				inner := inset(lr, 1, 1)
				viewH = inner.Height
				// Rebuild the filtered list as soon as the text changes; the
				// tree itself is untouched.
				if q := strings.ToLower(strings.TrimSpace(filter.String())); !loading && loadErr == nil && q != query {
					apply()
					w.c.RequestFrame()
				}
				switch {
				case loading:
					w.p.textCenter("Loading the files of this revision…", inner, fontUI, regular, w.pal.TextSecond)
					return
				case loadErr != nil:
					w.p.textCenter(vapp.ErrorText(loadErr), inner, fontUI, regular, w.pal.Error)
					return
				case len(entries) == 0:
					w.p.textCenter("The revision has no files", inner, fontUI, regular, w.pal.TextSecond)
					return
				case len(shown) == 0:
					w.p.textCenter("No files match the filter", inner, fontUI, regular, w.pal.TextSecond)
					return
				}
				n := len(shown)
				content := float32(n)*rowH + 8
				scroll.clamp(content, inner.Height)
				w.onScrollIn(inner, func(dx, dy float32) { scroll.Y += dy; scroll.clamp(content, inner.Height) })
				w.c.Clip(inner)
				// Paint only what is visible: a monorepo tree can hold
				// hundreds of thousands of files.
				first := max(0, int(scroll.Y/rowH)-1)
				last := min(n, first+int(inner.Height/rowH)+3)
				for k := first; k < last; k++ {
					e := entries[shown[k]]
					y := inner.Y + 4 + float32(k)*rowH - scroll.Y
					row := rect(inner.X+4, y, inner.Width-8, rowH)
					id := scRevisionRows.At(k)
					if sel == k {
						w.p.round(row, 4, w.pal.Selection)
					} else if w.hovered(id) {
						w.p.round(row, 4, w.pal.Hover)
					}
					x := row.X + 8
					w.p.iconIn(fileIcon(e.Path), rect(x, y, 16, rowH))
					x += 22
					size := ""
					if e.Submodule() {
						size = "submodule"
					} else if e.Symlink() {
						size = "symlink"
					} else if e.Size >= 0 {
						size = humanSize(e.Size)
					}
					sw := float32(0)
					if size != "" {
						sw = w.p.width(size, fontSmall, regular) + 16
						w.p.text(size, rect(right(row)-sw, y, sw-10, rowH), fontSmall, regular, w.pal.TextSecond)
					}
					tw := w.p.text(domain.FileName(e.Path), rect(x, y, max(0, right(row)-x-sw-10), rowH), fontUI, regular, w.pal.Text)
					hint := domain.DirName(e.Path)
					if e.Executable() {
						if hint == "" {
							hint = "executable"
						} else {
							hint += " · executable"
						}
					}
					if hint != "" && x+tw+8 < right(row)-sw-10 {
						hx := x + tw + 8
						w.p.text(w.p.truncateMiddle(hint, fontUI, regular, max(0, right(row)-hx-sw-12)), rect(hx, y, right(row)-hx-sw-10, rowH), fontUI, regular, w.pal.TextSecond)
					}
					w.c.HitWithOptions(id, row, e.Path, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: sel == k})
					if w.c.Clicked(id) {
						k := k
						w.after(func() { sel = k })
					}
					w.onDouble(id, func() { sel = k; open() })
				}
				w.c.Unclip()
				w.vbar(idRevisionScroll, inner, scroll, content)
			})
			f.gap(6)
			f.muted("Double-click a file to open it at this revision")
		},
		okDisabled: func() bool { return sel < 0 || sel >= len(shown) },
		onOK:       func() bool { open(); return true },
	})
}

// humanSize formats a blob size for the file list.
func humanSize(n int64) string {
	switch {
	case n < 1024:
		return strconv.FormatInt(n, 10) + " B"
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}
