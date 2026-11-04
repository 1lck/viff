package viffui

import (
	"time"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/ui/icons"
)

// stashView is the Stash tab of the Commit tool window.
type stashView struct {
	sel     int
	files   []domain.Change
	forHash string
	loading bool
	scroll  scroller
	fileSel int
}

func (w *Window) paintStash(r ui.Rect) {
	v := &w.st
	rs := w.repo()
	stashes := rs.Stashes
	if v.sel >= len(stashes) {
		v.sel = len(stashes) - 1
	}
	tb := splitTop(&r, 34)
	x, y := tb.X+8, tb.Y+(tb.Height-actionSize)/2
	if w.action(idCmStash, x, y, icons.Shelve, "Stash Changes…", actOpts{disabled: rs.Status.Clean()}) {
		w.after(w.openStashDialog)
	}
	x += actionSize + 2
	var cur *domain.Stash
	if v.sel >= 0 && v.sel < len(stashes) {
		cur = &stashes[v.sel]
	}
	if w.action(scStashActions.At(0), x, y, icons.Unshelve, "Apply Stash", actOpts{disabled: cur == nil}) {
		s := *cur
		w.after(func() { w.ws.StashApply(s, false, false) })
	}
	x += actionSize + 2
	if w.action(scStashActions.At(1), x, y, icons.Download, "Pop Stash (apply and drop)", actOpts{disabled: cur == nil}) {
		s := *cur
		w.after(func() { w.ws.StashApply(s, true, false) })
	}
	x += actionSize + 2
	if w.action(scStashActions.At(2), x, y, icons.Delete, "Drop Stash…", actOpts{disabled: cur == nil}) {
		s := *cur
		w.after(func() { w.confirmDropStash(s) })
	}
	x += actionSize + 2
	if w.action(scStashActions.At(3), x, y, icons.Vcs, "Unstash as Branch…", actOpts{disabled: cur == nil}) {
		s := *cur
		w.after(func() { w.openStashBranch(s) })
	}
	w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
	r.Y++
	r.Height--
	if len(stashes) == 0 {
		w.p.textCenter("No stashes", rect(r.X, r.Y+r.Height*0.3, r.Width, 22), fontUI, regular, w.pal.TextSecond)
		w.p.textCenter("Stash changes to set them aside and get back to them later", rect(r.X, r.Y+r.Height*0.3+22, r.Width, 20), fontSmall, regular, w.pal.TextDisable)
		return
	}
	listH := min(float32(len(stashes))*rowH+8, r.Height*0.5)
	lr := splitTop(&r, listH)
	w.list(idStashList, scStashRows, lr, "Stashes")
	focused := w.focused(idStashList)
	content := float32(len(stashes))*rowH + 8
	v.scroll.clamp(content, lr.Height)
	w.onScrollIn(lr, func(dx, dy float32) { v.scroll.Y += dy; v.scroll.clamp(content, lr.Height) })
	w.onKeys(idStashList, func(e ui.KeyEvent) bool {
		switch e.Key {
		case ui.KeyDown:
			v.sel = min(len(stashes)-1, v.sel+1)
		case ui.KeyUp:
			v.sel = max(0, v.sel-1)
		case ui.KeyDelete, ui.KeyBackspace:
			if cur != nil {
				w.confirmDropStash(*cur)
			}
		default:
			return false
		}
		return true
	})
	now := time.Now()
	w.c.Clip(lr)
	for i, s := range stashes {
		yy := lr.Y + 4 + float32(i)*rowH - v.scroll.Y
		rr := rect(lr.X, yy, lr.Width, rowH)
		hl := rect(rr.X+6, rr.Y, rr.Width-12, rr.Height)
		id := scStashRows.At(i)
		if v.sel == i {
			c := w.pal.SelectionInac
			if focused {
				c = w.pal.Selection
			}
			w.p.round(hl, 4, c)
		} else if w.hovered(id) {
			w.p.round(hl, 4, w.pal.Hover)
		}
		w.p.iconIn(icons.Shelve, rect(hl.X+6, yy, 16, rowH))
		msg := s.Message
		when := ago(s.Time, now)
		ww := w.p.width(when, fontSmall, regular)
		w.p.text(msg, rect(hl.X+28, yy, hl.Width-28-ww-16, rowH), fontUI, regular, w.pal.Text)
		w.p.text(when, rect(right(hl)-ww-8, yy, ww+2, rowH), fontSmall, regular, w.pal.TextSecond)
		w.c.HitWithOptions(id, hl, msg, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: v.sel == i})
		if w.c.Clicked(id) {
			i := i
			w.after(func() { v.sel = i })
		}
		st := s
		w.onDouble(id, func() { w.ws.StashApply(st, false, false) })
		ii := i
		w.onContext(id, func(x, y float32) {
			v.sel = ii
			w.openMenu(x, y, []item{
				{title: "Apply Stash", icon: icons.Unshelve, run: func() { w.ws.StashApply(st, false, false) }},
				{title: "Apply with Index", run: func() { w.ws.StashApply(st, false, true) }},
				{title: "Pop Stash", icon: icons.Download, run: func() { w.ws.StashApply(st, true, false) }},
				{title: "Unstash as Branch…", icon: icons.Vcs, run: func() { w.openStashBranch(st) }},
				sep(),
				{title: "Copy Revision Number", icon: icons.Copy, run: func() { w.copyText(st.Hash) }},
				{title: "Drop…", icon: icons.Delete, run: func() { w.confirmDropStash(st) }},
			})
		})
	}
	w.c.Unclip()
	w.vbar(scLogScrollbars.At(7), lr, &v.scroll, content)

	// Files of the selected stash.
	w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
	if cur == nil {
		return
	}
	if v.forHash != cur.Hash && !v.loading {
		hash := cur.Hash
		v.loading = true
		v.fileSel = -1
		w.after(func() {
			w.ws.CompareWith(hash+"^1", hash, func(ch []domain.Change, err error) {
				v.loading = false
				v.forHash = hash
				v.files = ch
				if err != nil {
					v.files = nil
				}
			})
		})
	}
	fr := rect(r.X, r.Y+1, r.Width, r.Height-1)
	if v.forHash != cur.Hash {
		w.p.textCenter("Loading…", rect(fr.X, fr.Y+20, fr.Width, 22), fontUI, regular, w.pal.TextSecond)
		return
	}
	w.c.Clip(fr)
	y = fr.Y + 4
	w.p.text(plural(len(v.files), "file", "files")+" in "+cur.Ref, rect(fr.X+12, y, fr.Width-24, 22), fontSmall, regular, w.pal.TextSecond)
	y += 24
	for i, ch := range v.files {
		rr := rect(fr.X+6, y, fr.Width-12, rowH)
		y += rowH
		if rr.Y > bottom(fr) {
			break
		}
		id := scDetailRows.At(500 + i)
		if v.fileSel == i {
			w.p.round(rr, 4, w.pal.Selection)
		} else if w.hovered(id) {
			w.p.round(rr, 4, w.pal.Hover)
		}
		w.p.iconIn(fileIcon(ch.Path), rect(rr.X+8, rr.Y, 16, rowH))
		tw := w.p.text(domain.FileName(ch.Path), rect(rr.X+30, rr.Y, rr.Width-40, rowH), fontUI, regular, w.statusColor(ch.Kind))
		if d := domain.DirName(ch.Path); d != "" {
			w.p.text(d, rect(rr.X+30+tw+8, rr.Y, rr.Width-tw-48, rowH), fontUI, regular, w.pal.TextSecond)
		}
		w.c.HitWithOptions(id, rr, ch.Path, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true})
		if w.c.Clicked(id) {
			i, ch, hash := i, ch, cur.Hash
			w.after(func() {
				v.fileSel = i
				w.showDiff(vapp.DiffSource{Path: ch.Path, OldPath: ch.OldPath, Commit: hash, Parent: hash + "^1", Kind: ch.Kind}, false)
			})
		}
	}
	w.c.Unclip()
}

func (w *Window) confirmDropStash(s domain.Stash) {
	w.confirm("Drop Stash", "Drop "+s.Ref+" \""+s.Message+"\"? You can still find its commit in the reflog for a while, but not in Viff.", "Drop", func() {
		w.ws.StashDrop(s)
	})
}

func (w *Window) openStashBranch(s domain.Stash) {
	w.openNewBranchFor("Unstash as Branch", func(name string) { w.ws.StashBranch(s, name) })
}

// openNewBranchFor asks for a branch name and calls fn with it.
func (w *Window) openNewBranchFor(title string, fn func(string)) {
	name := newEditor()
	w.openDialog(&dialog{
		title: title, ok: "Create", width: 440,
		body: func(f *form) {
			n := name.String()
			p := validRefName(n)
			if p == "" && w.branchExists(n) {
				p = "Branch '" + n + "' already exists"
			}
			id := f.field("Branch name:", name, "", n != "" && p != "")
			f.w.dialog.focus = id
			if n != "" && p != "" {
				f.message(p, w.pal.Error)
			}
		},
		okDisabled: func() bool { n := name.String(); return validRefName(n) != "" || w.branchExists(n) },
		onOK:       func() bool { fn(name.String()); return true },
	})
}
