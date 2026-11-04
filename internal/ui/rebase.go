package viffui

import (
	"strings"

	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/ui/icons"
)

// rebaseRow is one commit of an interactive rebase plan.
type rebaseRow struct {
	c      gitcli.RebaseCommit
	action gitcli.RebaseAction
	msg    *text.Editor // the reworded message
}

// rebaseView is the state of the Interactive Rebase dialog.
type rebaseView struct {
	rows   []rebaseRow
	orig   []string // hashes in their original order
	base   string
	sel    int
	scroll scroller
	layout components.TextareaLayout
}

var rebaseActions = []struct {
	action gitcli.RebaseAction
	title  string
	key    string
}{
	{gitcli.RebasePick, "Pick", "p"},
	{gitcli.RebaseReword, "Reword", "r"},
	{gitcli.RebaseSquash, "Squash", "s"},
	{gitcli.RebaseFixup, "Fixup", "f"},
	{gitcli.RebaseDrop, "Drop", "d"},
}

// openInteractiveRebase loads the commits from c to HEAD and opens the plan
// editor, as IntelliJ's "Interactively Rebase from Here".
func (w *Window) openInteractiveRebase(c domain.Commit) {
	w.ws.RebaseCommits(c.Hash, func(list []gitcli.RebaseCommit, base string, err error) {
		if err != nil {
			w.ws.NotifyError("Cannot rebase from "+domain.ShortHash(c.Hash), err)
			return
		}
		for _, x := range list {
			if x.Merge {
				w.ws.NotifyError("Interactive rebase", gitcli.ErrRebaseMerges)
				return
			}
		}
		v := &rebaseView{base: base}
		for _, x := range list {
			v.rows = append(v.rows, rebaseRow{c: x, action: gitcli.RebasePick, msg: text.NewEditor(x.Message)})
			v.orig = append(v.orig, x.Hash)
		}
		w.openRebaseDialog(v)
	})
}

func (v *rebaseView) steps() []gitcli.RebaseStep {
	out := make([]gitcli.RebaseStep, len(v.rows))
	for i, r := range v.rows {
		out[i] = gitcli.RebaseStep{Action: r.action, Hash: r.c.Hash}
		if r.action == gitcli.RebaseReword {
			out[i].Message = r.msg.String()
		}
	}
	return out
}

// changed reports whether the plan differs from replaying everything as is.
func (v *rebaseView) changed() bool {
	for i, r := range v.rows {
		if r.c.Hash != v.orig[i] || r.action != gitcli.RebasePick {
			return true
		}
	}
	return false
}

// firstKept is the index of the first row that is not dropped.
func (v *rebaseView) firstKept() int {
	for i, r := range v.rows {
		if r.action != gitcli.RebaseDrop {
			return i
		}
	}
	return -1
}

func (v *rebaseView) set(a gitcli.RebaseAction) {
	if v.sel < 0 || v.sel >= len(v.rows) {
		return
	}
	if (a == gitcli.RebaseSquash || a == gitcli.RebaseFixup) && v.sel <= v.firstKept() {
		return // nothing before it to combine with
	}
	v.rows[v.sel].action = a
}

func (v *rebaseView) move(d int) {
	j := v.sel + d
	if v.sel < 0 || j < 0 || j >= len(v.rows) {
		return
	}
	v.rows[v.sel], v.rows[j] = v.rows[j], v.rows[v.sel]
	v.sel = j
}

func (v *rebaseView) reset() {
	byHash := map[string]rebaseRow{}
	for _, r := range v.rows {
		r.action = gitcli.RebasePick
		r.msg = text.NewEditor(r.c.Message)
		byHash[r.c.Hash] = r
	}
	for i, h := range v.orig {
		v.rows[i] = byHash[h]
	}
}

const rebaseRowH = float32(26)

func (w *Window) openRebaseDialog(v *rebaseView) {
	// +8 row padding, +2 for the outline the list sits inside.
	listH := float32(min(max(len(v.rows), 4), 9))*rebaseRowH + 10
	w.openDialog(&dialog{
		title:    "Rebasing " + plural(len(v.rows), "Commit", "Commits") + " onto " + rebaseBaseTitle(v.base),
		ok:       "Start Rebasing",
		width:    760,
		contentH: 34 + listH + 12 + 22 + 110 + 26,
		textarea: idRebaseMessage,
		shortcut: v.shortcut,
		body: func(f *form) {
			f.w.dialog.focus = idRebaseList
			f.custom(34, func(r ui.Rect) { w.paintRebaseToolbar(v, r) })
			f.custom(listH, func(r ui.Rect) { w.paintRebaseList(v, r) })
			f.gap(12)
			f.custom(22+110, func(r ui.Rect) { w.paintRebaseMessage(v, r) })
			err := gitcli.ValidateRebasePlan(v.steps())
			switch {
			case err != nil:
				f.message(err.Error(), w.pal.Error)
			case v.firstKept() < 0:
				f.message("Every commit is dropped: the branch will point at the base commit.", w.pal.Warning)
			default:
				f.muted("Local changes are stashed and restored automatically. Conflicts stop the rebase; continue or abort it from the Commit tool window.")
			}
		},
		okDisabled: func() bool { return !v.changed() || gitcli.ValidateRebasePlan(v.steps()) != nil },
		onOK: func() bool {
			w.ws.InteractiveRebase(v.base, v.steps())
			return true
		},
	})
}

func rebaseBaseTitle(base string) string {
	if base == "" {
		return "the Root"
	}
	return domain.ShortHash(base)
}

func (w *Window) paintRebaseToolbar(v *rebaseView, r ui.Rect) {
	x := r.X
	cur := gitcli.RebaseAction("")
	if v.sel >= 0 && v.sel < len(v.rows) {
		cur = v.rows[v.sel].action
	}
	for i, a := range rebaseActions {
		bw := w.p.width(a.title, fontUI, regular) + 20
		id := scRebaseActions.At(i)
		disabled := v.sel < 0 || ((a.action == gitcli.RebaseSquash || a.action == gitcli.RebaseFixup) && v.sel <= v.firstKept())
		br := rect(x, r.Y+3, bw, 26)
		if cur == a.action && !disabled {
			w.p.round(br, radiusCtl, w.pal.ToolPressed)
		}
		if w.button(id, br, a.title, btnBorderless, disabled) {
			act := a.action
			w.after(func() { v.set(act) })
		}
		w.tip(id, a.title+"\t⌘"+strings.ToUpper(a.key))
		x += bw + 2
	}
	x += 8
	w.p.vline(x, r.Y+8, 16, w.pal.Border)
	x += 10
	y := r.Y + (r.Height-actionSize)/2
	if w.action(scRebaseActions.At(10), x, y, icons.Up, "Move Up\t⌥↑", actOpts{disabled: v.sel <= 0}) {
		w.after(func() { v.move(-1) })
	}
	x += actionSize + 2
	if w.action(scRebaseActions.At(11), x, y, icons.Down, "Move Down\t⌥↓", actOpts{disabled: v.sel < 0 || v.sel >= len(v.rows)-1}) {
		w.after(func() { v.move(1) })
	}
	x += actionSize + 10
	if w.button(scRebaseActions.At(12), rect(x, r.Y+3, w.p.width("Reset", fontUI, regular)+20, 26), "Reset", btnBorderless, !v.changed()) {
		w.after(v.reset)
	}
}

func (w *Window) paintRebaseList(v *rebaseView, r ui.Rect) {
	w.p.outline(r, radiusCtl, 1, w.pal.ControlBorder, w.pal.ControlBg)
	inner := inset(r, 1, 1)
	w.list(idRebaseList, scRebaseRows, inner, "Rebase plan")
	focused := w.focused(idRebaseList)
	content := float32(len(v.rows))*rebaseRowH + 8
	v.scroll.clamp(content, inner.Height)
	w.onScrollIn(inner, func(dx, dy float32) { v.scroll.Y += dy; v.scroll.clamp(content, inner.Height) })
	w.onKeys(idRebaseList, func(e ui.KeyEvent) bool {
		alt := e.Modifiers&ui.ModifierAlt != 0
		switch {
		case e.Key == ui.KeyUp && alt:
			v.move(-1)
		case e.Key == ui.KeyDown && alt:
			v.move(1)
		case e.Key == ui.KeyUp:
			v.sel = max(0, v.sel-1)
		case e.Key == ui.KeyDown:
			v.sel = min(len(v.rows)-1, v.sel+1)
		case e.Key == ui.KeyDelete || e.Key == ui.KeyBackspace:
			v.set(gitcli.RebaseDrop)
		default:
			if e.Modifiers&(ui.ModifierMeta|ui.ModifierControl|ui.ModifierAlt) != 0 {
				return false
			}
			for _, a := range rebaseActions {
				if keyChar(e) == a.key {
					v.set(a.action)
					return true
				}
			}
			return false
		}
		v.scroll.reveal(4+float32(v.sel)*rebaseRowH, rebaseRowH, inner.Height)
		return true
	})
	w.c.Clip(inner)
	actW := float32(76)
	for i, row := range v.rows {
		y := inner.Y + 4 + float32(i)*rebaseRowH - v.scroll.Y
		if y+rebaseRowH < inner.Y || y > bottom(inner) {
			continue
		}
		rr := rect(inner.X+4, y, inner.Width-8, rebaseRowH)
		id := scRebaseRows.At(i)
		switch {
		case v.sel == i && focused:
			w.p.round(rr, 4, w.pal.Selection)
		case v.sel == i:
			w.p.round(rr, 4, w.pal.SelectionInac)
		case w.hovered(id):
			w.p.round(rr, 4, w.pal.Hover)
		}
		ac, fg := w.pal.TextSecond, w.pal.Text
		title := ""
		for _, a := range rebaseActions {
			if a.action == row.action {
				title = a.title
			}
		}
		switch row.action {
		case gitcli.RebaseReword:
			ac = w.pal.Link
		case gitcli.RebaseSquash, gitcli.RebaseFixup:
			ac = w.pal.RefRemote
		case gitcli.RebaseDrop:
			ac, fg = w.pal.TextDisable, w.pal.TextDisable
		}
		w.p.text(title, rect(rr.X+10, y, actW, rebaseRowH), fontUI, semibold, ac)
		hx := rr.X + 10 + actW
		w.p.monoText(domain.ShortHash(row.c.Hash), rect(hx, y, 80, rebaseRowH), fontSmall+0.5, w.pal.TextSecond)
		subject := row.c.Subject
		if row.action == gitcli.RebaseReword {
			subject, _, _ = strings.Cut(row.msg.String(), "\n")
		}
		sx := hx + 80
		aw := w.p.width(row.c.Author, fontSmall, regular)
		tw := w.p.text(subject, rect(sx, y, right(rr)-sx-aw-20, rebaseRowH), fontUI, regular, fg)
		if row.action == gitcli.RebaseDrop {
			w.p.hline(sx, y+rebaseRowH/2, tw, w.pal.TextDisable)
		}
		w.p.text(row.c.Author, rect(right(rr)-aw-10, y, aw+2, rebaseRowH), fontSmall, regular, w.pal.TextSecond)
		w.c.HitWithOptions(id, rr, subject, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: v.sel == i})
		if w.c.Clicked(id) {
			i := i
			w.after(func() { v.sel = i; w.focusNext = idRebaseList })
		}
		ii := i
		w.onContext(id, func(x, y float32) {
			v.sel = ii
			var items []item
			for _, a := range rebaseActions {
				act := a.action
				dis := (act == gitcli.RebaseSquash || act == gitcli.RebaseFixup) && ii <= v.firstKept()
				items = append(items, item{title: a.title, shortcut: strings.ToUpper(a.key), checked: v.rows[ii].action == act, disabled: dis, run: func() { v.set(act) }})
			}
			items = append(items, sep(),
				item{title: "Move Up", icon: icons.Up, shortcut: "⌥↑", disabled: ii == 0, run: func() { v.move(-1) }},
				item{title: "Move Down", icon: icons.Down, shortcut: "⌥↓", disabled: ii == len(v.rows)-1, run: func() { v.move(1) }},
			)
			w.openMenu(x, y, items)
		})
	}
	w.c.Unclip()
	w.vbar(scRebaseActions.At(20), inner, &v.scroll, content)
}

// paintRebaseMessage shows the selected commit's message, editable when the
// commit is reworded.
func (w *Window) paintRebaseMessage(v *rebaseView, r ui.Rect) {
	if v.sel < 0 || v.sel >= len(v.rows) {
		return
	}
	row := v.rows[v.sel]
	label := "Message of " + domain.ShortHash(row.c.Hash)
	if row.action != gitcli.RebaseReword {
		label += " (choose Reword to edit)"
	}
	w.p.text(label, rect(r.X, r.Y, r.Width, 18), fontSmall, regular, w.pal.TextSecond)
	mr := rect(r.X, r.Y+22, r.Width, r.Height-22)
	if row.action != gitcli.RebaseReword {
		w.p.outline(mr, radiusCtl, 1, w.pal.ControlBorder, w.pal.IslandAlt)
		y := mr.Y + 6
		for _, l := range strings.Split(row.c.Message, "\n") {
			if y+18 > bottom(mr) {
				break
			}
			w.p.text(l, rect(mr.X+8, y, mr.Width-16, 18), fontUI, regular, w.pal.TextMuted)
			y += 18
		}
		return
	}
	ed := row.msg
	value := ed.String()
	sel := ed.Selection()
	ss, se := runeToByte(value, min(sel.Start, sel.End)), runeToByte(value, max(sel.Start, sel.End))
	components.PaintTextarea(w.c, w.theme, mr, components.TextareaSnapshot{
		ID: idRebaseMessage, Label: "Commit message", Value: value, Placeholder: "Commit message",
		ShowCaret: true, HideEditorText: true, Layout: &v.layout,
		Caret: runeToByte(value, ed.Caret()), SelectionStart: ss, SelectionEnd: se,
	})
	w.editors[idRebaseMessage] = ed
	w.onScrollIn(mr, func(dx, dy float32) { v.layout.ScrollBy(dy) })
	w.onKeys(idRebaseMessage, func(e ui.KeyEvent) bool {
		mv, extend := components.TextareaMovementForKey(e.Key, e.Modifiers)
		if mv == components.TextareaMoveNone || mv == components.TextareaMoveLineStart || mv == components.TextareaMoveLineEnd {
			return false
		}
		val := ed.String()
		nb := v.layout.Move(val, runeToByte(val, ed.Caret()), mv)
		ed.MoveTo(byteToRune(val, nb), extend)
		return true
	})
}

// shortcut applies the action whose ⌘ chord was pressed to the selected row.
func (v *rebaseView) shortcut(e ui.KeyEvent) bool {
	if e.Modifiers&(ui.ModifierMeta|ui.ModifierControl) == 0 || e.Modifiers&(ui.ModifierShift|ui.ModifierAlt) != 0 {
		return false
	}
	for _, a := range rebaseActions {
		if keyChar(e) == a.key {
			v.set(a.action)
			return true
		}
	}
	return false
}
