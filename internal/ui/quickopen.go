package viffui

import (
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/ui/icons"
)

// Quick Open is the keyboard-first way into the working tree: filter every
// path and open the match read-only. It reuses the same app query and viewer
// as the Project tool window, so it adds no second source of truth.
func (w *Window) openQuickOpen() {
	if w.repo() == nil {
		return
	}
	w.ensureFiles()
	v := &w.fm
	filter := text.NewEditor("")
	query := "\x00"
	built := false
	builtSeq := uint64(0)
	var shown []int
	sel := -1
	scroll := &scroller{}
	viewH := float32(0)

	apply := func() {
		q := strings.ToLower(strings.TrimSpace(filter.String()))
		query = q
		shown = shown[:0]
		for i, p := range v.entries {
			if q == "" || strings.Contains(strings.ToLower(p), q) {
				shown = append(shown, i)
			}
		}
		if sel >= len(shown) {
			sel = len(shown) - 1
		}
		if sel >= len(shown) {
			sel = -1
		}
		if sel < 0 && len(shown) > 0 {
			sel = 0
		}
		scroll.Y = 0
	}
	open := func() {
		if sel < 0 || sel >= len(shown) || shown[sel] >= len(v.entries) {
			return
		}
		p := v.entries[shown[sel]]
		w.dialog = nil
		w.openRepoFile(p, true)
	}
	reveal := func() {
		if sel >= 0 {
			scroll.reveal(float32(sel)*rowH, rowH, viewH)
		}
	}

	w.openDialog(&dialog{
		title: "Open File",
		ok:    "Open",
		width: 680,
		focus: idQuickInput,
		body: func(f *form) {
			f.custom(30, func(r ui.Rect) {
				if w.field(idQuickInput, filter, r, fieldOpts{icon: icons.Search, placeholder: "Search files by path", nav: true}) {
					w.submitDialog()
				}
				w.onKeys(idQuickInput, func(e ui.KeyEvent) bool {
					switch e.Key {
					case ui.KeyDown:
						if len(shown) > 0 {
							sel = min(sel+1, len(shown)-1)
							reveal()
							w.c.RequestFrame()
						}
						return true
					case ui.KeyUp:
						if len(shown) > 0 {
							sel = max(sel-1, 0)
							reveal()
							w.c.RequestFrame()
						}
						return true
					case ui.KeyHome:
						if len(shown) > 0 {
							sel = 0
							reveal()
							w.c.RequestFrame()
						}
						return true
					case ui.KeyEnd:
						if len(shown) > 0 {
							sel = len(shown) - 1
							reveal()
							w.c.RequestFrame()
						}
						return true
					}
					return false
				})
			})
			f.gap(10)
			f.custom(340, func(lr ui.Rect) {
				w.p.outline(lr, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
				inner := inset(lr, 1, 1)
				viewH = inner.Height
				q := strings.ToLower(strings.TrimSpace(filter.String()))
				if v.loaded && (!built || q != query || builtSeq != v.seq) {
					apply()
					built = true
					builtSeq = v.seq
				}
				switch {
				case !v.loaded:
					w.p.textCenter("Loading files…", inner, fontUI, regular, w.pal.TextSecond)
					return
				case v.err != nil:
					w.p.textCenter(w.ws.ErrorText(v.err), inner, fontUI, regular, w.pal.Error)
					return
				case len(v.entries) == 0:
					w.p.textCenter("The working tree has no files", inner, fontUI, regular, w.pal.TextSecond)
					return
				case len(shown) == 0:
					w.p.textCenter("No files match the search", inner, fontUI, regular, w.pal.TextSecond)
					return
				}
				n := len(shown)
				content := float32(n)*rowH + 8
				scroll.clamp(content, inner.Height)
				w.onScrollIn(inner, func(dx, dy float32) {
					scroll.Y += dy
					scroll.clamp(content, inner.Height)
				})
				w.c.Clip(inner)
				first := max(0, int(scroll.Y/rowH)-1)
				last := min(n, first+int(inner.Height/rowH)+3)
				for k := first; k < last; k++ {
					p := v.entries[shown[k]]
					y := inner.Y + 4 + float32(k)*rowH - scroll.Y
					row := rect(inner.X+4, y, inner.Width-8, rowH)
					id := scQuickRows.At(k)
					if k == sel {
						w.p.round(row, 4, w.pal.Selection)
					} else if w.hovered(id) {
						w.p.round(row, 4, w.pal.Hover)
					}
					x := row.X + 8
					w.p.iconIn(fileIcon(p), rect(x, y, 16, rowH))
					x += 22
					tw := max(0, right(row)-x-8)
					w.p.text(w.p.truncateMiddle(p, fontUI, regular, tw), rect(x, y, tw, rowH), fontUI, regular, w.pal.Text)
					w.c.HitWithOptions(id, row, p, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: k == sel})
					if w.c.Clicked(id) {
						k := k
						w.after(func() {
							sel = k
							reveal()
						})
					}
					w.onDouble(id, func() {
						sel = k
						open()
					})
				}
				w.c.Unclip()
				w.vbar(idQuickBar, inner, scroll, content)
			})
			f.gap(6)
			f.muted("Enter opens the file read-only; Esc closes")
		},
		okDisabled: func() bool { return sel < 0 || sel >= len(shown) },
		onOK:       func() bool { open(); return true },
	})
	// The trap opens on this paint; request the field once more so typing works
	// immediately even when another field had focus. The extra frame is what
	// moves the native text input to the new field: focus is applied while the
	// hit regions are rebuilt at the end of a paint, after that paint already
	// synced the text input.
	w.focusNext = idQuickInput
	w.after(func() {
		if w.dialog == nil {
			return
		}
		w.c.RequestFocus(idQuickInput)
		w.Invalidate()
	})
}
