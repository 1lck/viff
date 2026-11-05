package viffui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// worktrees.go is IntelliJ's Git Worktrees dialog: the linked checkouts of one
// repository with the branch each of them has, and the actions on them.

const worktreeRowH = 48

// openWorktrees shows the worktree list, loading it first when the repository
// state has not been read yet.
func (w *Window) openWorktrees() {
	rs := w.repo()
	if rs == nil {
		return
	}
	if !rs.WorktreesLoaded {
		w.ws.Worktrees(func(_ []domain.Worktree, err error) {
			if err != nil {
				w.ws.NotifyError(w.t(i18n.WorktreeCannotList), err)
				return
			}
			w.after(w.openWorktrees)
		})
		return
	}
	w.showWorktrees()
}

func (w *Window) showWorktrees() {
	if w.repo() == nil {
		return
	}
	sel := 0 // the current worktree, which worktreeList puts first
	scroll := &scroller{}
	w.openDialog(&dialog{
		title: w.t(i18n.WorktreeTitle), ok: w.t(i18n.CommonClose), width: 760, noCancel: true,
		body: func(f *form) {
			f.custom(30, func(r ui.Rect) {
				bw := w.buttonWidth("New Worktree…")
				if w.button(idWorktreeNew, rect(r.X, r.Y, bw, 28), "New Worktree…", btnNormal, false) {
					w.after(w.openNewWorktree)
				}
				if rs := w.repo(); rs != nil {
					info := plural(len(rs.Worktrees), "worktree", "worktrees")
					iw := w.p.width(info, fontUI, regular)
					w.p.text(info, rect(right(r)-iw, r.Y, iw, 28), fontUI, regular, w.pal.TextSecond)
				}
			})
			f.gap(8)
			f.custom(252, func(r ui.Rect) {
				w.p.outline(r, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
				inner := inset(r, 1, 1)
				list := w.worktreeList()
				switch {
				case sel >= len(list):
					sel = len(list) - 1
				case sel < 0 && len(list) > 0:
					sel = 0
				}
				content := float32(len(list)) * worktreeRowH
				scroll.clamp(content, inner.Height)
				w.onScrollIn(inner, func(dx, dy float32) {
					scroll.Y += dy
					scroll.clamp(content, inner.Height)
				})
				w.c.Clip(inner)
				for i, wt := range list {
					y := inner.Y + float32(i)*worktreeRowH - scroll.Y
					if y+worktreeRowH <= inner.Y || y >= bottom(inner) {
						continue
					}
					if w.paintWorktreeRow(wt, i, sel, rect(inner.X, y, inner.Width, worktreeRowH)) {
						sel = i
					}
				}
				w.c.Unclip()
				w.vbar(scWorktreeBar.At(0), inner, scroll, content)
			})
			f.gap(8)
			f.custom(30, func(r ui.Rect) {
				list := w.worktreeList()
				var pick domain.Worktree
				picked := sel >= 0 && sel < len(list)
				if picked {
					pick = list[sel]
				}
				chosen := pick
				canOpen := picked && w.openableWorktree(chosen)
				canDelete := picked && !chosen.Bare && !w.repo().IsCurrentWorktree(chosen)
				x := right(r)
				for _, b := range []struct {
					id      ui.ID
					label   string
					enabled bool
					run     func()
				}{
					{idWorktreeOpen, "Open", canOpen, func() {
						w.after(func() {
							w.closeDialog()
							w.openRepo(chosen.Path)
						})
					}},
					{idWorktreeDelete, "Delete…", canDelete, func() { w.confirmDeleteWorktree(chosen) }},
					{idWorktreeCopy, "Copy Path", picked, func() { w.copyText(chosen.Path) }},
				} {
					bw := w.buttonWidth(b.label)
					x -= bw
					if w.button(b.id, rect(x, r.Y+1, bw, 28), b.label, btnNormal, !b.enabled) && b.enabled {
						b.run()
					}
					x -= 8
				}
				w.p.text(w.t(i18n.WorktreeDoubleClickHint), rect(r.X, r.Y, x-r.X-8, 30), fontSmall, regular, w.pal.TextSecond)
			})
		},
	})
}

// worktreeList sorts the current checkout first, then by path.
func (w *Window) worktreeList() []domain.Worktree {
	rs := w.repo()
	if rs == nil {
		return nil
	}
	list := append([]domain.Worktree(nil), rs.Worktrees...)
	sort.SliceStable(list, func(i, j int) bool {
		ci, cj := rs.IsCurrentWorktree(list[i]), rs.IsCurrentWorktree(list[j])
		if ci != cj {
			return ci
		}
		return list[i].Path < list[j].Path
	})
	return list
}

func worktreeName(wt domain.Worktree) string {
	switch {
	case wt.Bare:
		return "Bare repository"
	case wt.Branch != "":
		return wt.Branch
	}
	return "Detached HEAD"
}

func worktreeLabel(wt domain.Worktree) string {
	return worktreeName(wt) + "  " + domain.ShortHash(wt.Head) + "  " + wt.Path
}

// paintWorktreeRow draws one row and reports whether it was clicked.
func (w *Window) paintWorktreeRow(wt domain.Worktree, i, sel int, row ui.Rect) bool {
	rs := w.repo()
	current := rs != nil && rs.IsCurrentWorktree(wt)
	id := scWorktreeRows.At(i)
	if sel == i {
		w.p.round(row, 4, w.pal.Selection)
	} else if w.hovered(id) {
		w.p.round(row, 4, w.pal.Hover)
	}

	x := row.X + 12
	icon, tint := icons.BranchLabel, w.pal.RefLocal
	if current {
		icon, tint = icons.CurrentBranchLabel, w.pal.RefHead
	}
	w.p.iconTinted(icon, rect(x, row.Y+8, 16, 16), tint)

	tx := x + 24
	name := worktreeName(wt)
	nameW := min(w.p.width(name, fontUI, semibold), 300)
	w.p.text(name, rect(tx, row.Y+6, nameW, 20), fontUI, semibold, w.pal.Text)
	if wt.Head != "" {
		w.p.text(domain.ShortHash(wt.Head), rect(tx+nameW+10, row.Y+6, 90, 20), fontUI, regular, w.pal.TextMuted)
	}

	pathW := right(row) - tx - 100
	path := w.fitPathLeft(shortenHome(wt.Path), fontSmall, regular, pathW)
	pw := w.p.text(path, rect(tx, row.Y+27, pathW, 16), fontSmall, regular, w.pal.TextSecond)
	if note := worktreeNote(wt); note != "" {
		w.p.text("  ·  "+note, rect(tx+pw, row.Y+27, max(0, right(row)-tx-pw-100), 16), fontSmall, regular, w.pal.Warning)
	}

	if current {
		label := "current"
		bw := w.p.width(label, fontSmall, regular) + 14
		br := rect(right(row)-bw-10, row.Y+(worktreeRowH-18)/2, bw, 18)
		w.p.round(br, 9, w.pal.Brand.WithAlpha(0.18))
		w.p.textCenter(label, br, fontSmall, regular, w.pal.Brand)
	}

	w.c.HitWithOptions(id, row, worktreeLabel(wt), ui.HitOptions{Role: ui.RoleOption, SkipFocus: true})
	clicked := w.c.Clicked(id)
	if w.openableWorktree(wt) {
		wt := wt
		w.onDouble(id, func() {
			w.after(func() {
				w.closeDialog()
				w.openRepo(wt.Path)
			})
		})
	}
	return clicked
}

// fitPathLeft shortens a path from the left, so the directory name, the part
// that tells two worktrees apart, stays visible in a narrow column.
func (w *Window) fitPathLeft(s string, size, weight, width float32) string {
	if width <= 0 || w.p.width(s, size, weight) <= width {
		return s
	}
	parts := strings.Split(filepath.ToSlash(s), "/")
	for i := 1; i < len(parts); i++ {
		cut := "…/" + strings.Join(parts[i:], "/")
		if w.p.width(cut, size, weight) <= width {
			return cut
		}
	}
	return parts[len(parts)-1]
}

func worktreeNote(wt domain.Worktree) string {
	switch {
	case wt.Locked:
		return "locked"
	case wt.Prunable:
		return "prunable"
	}
	return ""
}

func (w *Window) openableWorktree(wt domain.Worktree) bool {
	rs := w.repo()
	return rs != nil && !rs.IsCurrentWorktree(wt) && !wt.Prunable && !wt.Bare
}

// defaultWorktreePath suggests a sibling directory named after the branch.
func defaultWorktreePath(root, branch string) string {
	name := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(filepath.Dir(root), filepath.Base(root)+"-"+name)
}

// worktreeModes are the two ways New Worktree gets the branch it checks out.
var worktreeModes = []string{"New branch", "Existing branch"}

const (
	worktreeModeNew = iota
	worktreeModeExisting
)

// worktreeChoice is one local branch in New Worktree's existing-branch mode.
// heldIn is the worktree that already has it checked out; "" means a new
// worktree can take it.
type worktreeChoice struct {
	name   string
	heldIn string
}

func (c worktreeChoice) free() bool { return c.heldIn == "" }

// heldLabel names the worktree that holds the branch, in the few words a menu
// row has room for. current is the repository's current branch.
func (c worktreeChoice) heldLabel(current string) string {
	switch {
	case c.heldIn == "":
		return ""
	case c.name == current:
		return "in this checkout"
	}
	return "in " + filepath.Base(c.heldIn)
}

// The existing-branch list shows at most six rows and scrolls the rest.
const (
	worktreeBranchRowH    = 28
	worktreeBranchRowsMax = 6
)

// paintWorktreeBranches draws the existing mode's branch list and selects the
// clicked free branch. A branch a worktree already has is disabled and says
// where, so the reason is visible in the dialog itself.
func (w *Window) paintWorktreeBranches(area ui.Rect, choices []worktreeChoice, chosen *string, scroll *scroller) {
	current := w.repo().CurrentBranch()
	border := w.pal.ControlBorder
	if w.focused(idDialogBranchList) {
		border = w.pal.Focus
	}
	w.p.outline(area, 6, 1, border, w.pal.ControlBg)
	inner := inset(area, 1, 1)
	// One focusable list around the rows, so the keyboard can reach it and
	// move the choice with the arrow keys; the rows themselves skip focus.
	w.list(idDialogBranchList, scDialogBranchRows, inner, "Local branches")
	w.onKeys(idDialogBranchList, w.worktreeBranchKeys(choices, chosen, scroll, inner.Height))
	content := float32(len(choices)) * worktreeBranchRowH
	scroll.clamp(content, inner.Height)
	w.onScrollIn(inner, func(dx, dy float32) {
		scroll.Y += dy
		scroll.clamp(content, inner.Height)
	})
	w.c.Clip(inner)
	for i, c := range choices {
		y := inner.Y + float32(i)*worktreeBranchRowH - scroll.Y
		if y+worktreeBranchRowH <= inner.Y || y >= bottom(inner) {
			continue
		}
		row := rect(inner.X, y, inner.Width, worktreeBranchRowH)
		id := scDialogBranchRows.At(i)
		selected := c.name == *chosen
		switch {
		case selected:
			w.p.round(row, 4, w.pal.Selection)
		case c.free() && w.hovered(id):
			w.p.round(row, 4, w.pal.Hover)
		}
		ix := row.X + 8
		if selected {
			w.p.iconIn(icons.Checked, rect(ix, row.Y, 16, worktreeBranchRowH))
		} else {
			w.p.iconTinted(icons.BranchLabel, rect(ix, row.Y, 16, worktreeBranchRowH), w.pal.RefLocal)
		}
		fg := w.pal.Text
		if !c.free() {
			fg = w.pal.TextDisable
		}
		nameW := min(w.p.width(c.name, fontUI, regular), max(0, row.Width-140))
		w.p.text(c.name, rect(ix+22, row.Y, nameW, worktreeBranchRowH), fontUI, regular, fg)
		if where := c.heldLabel(current); where != "" {
			hintW := max(0, right(row)-ix-22-nameW-18)
			w.p.text("checked out "+where, rect(ix+22+nameW+10, row.Y, hintW, worktreeBranchRowH), fontSmall, regular, w.pal.TextSecond)
		}
		w.c.HitWithOptions(id, row, c.name, ui.HitOptions{Role: ui.RoleOption, Checked: selected, Disabled: !c.free(), SkipFocus: true})
		if c.free() && w.c.Clicked(id) {
			c := c
			w.after(func() { *chosen = c.name })
		}
	}
	w.c.Unclip()
	w.vbar(scDialogBranchBar.At(0), inner, scroll, content)
}

// worktreeBranchKeys moves the existing mode's choice with the arrow keys,
// steps over the branches a worktree already has, and keeps the chosen row in
// view. It returns false for keys it does not own, so Enter still submits.
func (w *Window) worktreeBranchKeys(choices []worktreeChoice, chosen *string, scroll *scroller, view float32) func(ui.KeyEvent) bool {
	return func(e ui.KeyEvent) bool {
		next := worktreeBranchStep(choices, *chosen, e.Key)
		if next < 0 {
			switch e.Key {
			case ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd:
				return true // ours, even when no free branch is left
			}
			return false
		}
		*chosen = choices[next].name
		scroll.reveal(float32(next)*worktreeBranchRowH, worktreeBranchRowH, view)
		return true
	}
}

// worktreeBranchStep is where a key moves the choice: the next free branch in
// that direction, or -1 when the key is not a navigation key or nothing is
// left. Branches a worktree has are skipped, never selected.
func worktreeBranchStep(choices []worktreeChoice, chosen string, key ui.Key) int {
	if key != ui.KeyHome && key != ui.KeyEnd && key != ui.KeyUp && key != ui.KeyDown {
		return -1
	}
	cur := -1
	for i, c := range choices {
		if c.name == chosen {
			cur = i
			break
		}
	}
	free := func(i int) bool { return i >= 0 && i < len(choices) && choices[i].free() }
	switch key {
	case ui.KeyHome:
		for i := range choices {
			if free(i) {
				return i
			}
		}
	case ui.KeyEnd:
		for i := len(choices) - 1; i >= 0; i-- {
			if free(i) {
				return i
			}
		}
	case ui.KeyUp:
		for i := cur - 1; i >= 0; i-- {
			if free(i) {
				return i
			}
		}
		if cur < 0 {
			for i := len(choices) - 1; i >= 0; i-- {
				if free(i) {
					return i
				}
			}
		}
	case ui.KeyDown:
		for i := max(cur+1, 0); i < len(choices); i++ {
			if free(i) {
				return i
			}
		}
	}
	return -1
}

// worktreeChoices lists the local branches in Git's order, each with the
// worktree that has it checked out. A branch can be checked out in one
// worktree at a time: Git refuses the second even when the first worktree is
// prunable, so every listed worktree counts, not only the ones on disk.
func worktreeChoices(rs *vapp.RepoState) []worktreeChoice {
	held := make(map[string]string, len(rs.Worktrees))
	for _, wt := range rs.Worktrees {
		if wt.Bare || wt.Branch == "" {
			continue
		}
		held[wt.Branch] = wt.Path
	}
	var out []worktreeChoice
	for _, r := range rs.RefsOf(domain.RefLocal) {
		out = append(out, worktreeChoice{name: r.Short, heldIn: held[r.Short]})
	}
	return out
}

// worktreeBranchProblem says why branch cannot be checked out in a new
// worktree; "" when nothing stands in the way.
func (w *Window) worktreeBranchProblem(rs *vapp.RepoState, branch string) string {
	if branch == "" {
		return w.t(i18n.WorktreeSelectBranch)
	}
	for _, c := range worktreeChoices(rs) {
		if c.name != branch {
			continue
		}
		if c.heldIn == "" {
			return ""
		}
		where := shortenHome(c.heldIn)
		if c.name == rs.CurrentBranch() {
			where = w.t(i18n.WorktreeThisCheckout)
		}
		return w.t(i18n.WorktreeAlreadyCheckedOut, branch, where)
	}
	return w.t(i18n.WorktreeNotLocal, branch)
}

// worktreePathProblem says why the directory cannot take a worktree; "" when
// it can.
func (w *Window) worktreePathProblem(p string) string {
	switch {
	case p == "":
		return w.t(i18n.WorktreePathRequired)
	case !filepath.IsAbs(p):
		return w.t(i18n.WorktreePathAbsolute)
	}
	if _, err := os.Stat(p); err == nil {
		return w.t(i18n.DialogExists, p)
	}
	return ""
}

// newWorktreeBranchProblem says why name cannot be a new branch; "" when it can.
func (w *Window) newWorktreeBranchProblem(name string) string {
	if p := validRefName(name); p != "" {
		return p
	}
	if w.branchExists(name) {
		return "Branch '" + name + "' already exists. Pick another name."
	}
	return ""
}

// openNewWorktree is IntelliJ's New Worktree dialog: a new branch checked out
// in a directory next to the repository, or an existing local branch that no
// worktree has yet. New branch is the default mode and keeps the old behaviour.
func (w *Window) openNewWorktree() {
	rs := w.repo()
	if rs == nil {
		return
	}
	root := rs.Repo.Root
	mode := worktreeModeNew
	branch := text.NewEditor("")
	start := text.NewEditor("HEAD")
	if b := rs.CurrentBranch(); b != "" {
		start = text.NewEditor(b)
	}
	// The first branch no worktree has is the default of the existing mode.
	existing := ""
	for _, c := range worktreeChoices(rs) {
		if c.free() {
			existing = c.name
			break
		}
	}
	pathEd := text.NewEditor("")
	lastAuto := ""
	branchScroll := &scroller{}

	absPath := func() string {
		p := expandHome(strings.TrimSpace(pathEd.String()))
		if p == "" {
			return ""
		}
		if a, err := filepath.Abs(p); err == nil {
			return a
		}
		return p
	}
	// branchName is the name the path suggestion follows in either mode.
	branchName := func() string {
		if mode == worktreeModeExisting {
			return existing
		}
		return strings.TrimSpace(branch.String())
	}
	problem := func() string {
		if mode == worktreeModeExisting {
			if p := w.worktreeBranchProblem(w.repo(), existing); p != "" {
				return p
			}
		} else if p := w.newWorktreeBranchProblem(strings.TrimSpace(branch.String())); p != "" {
			return p
		}
		return w.worktreePathProblem(absPath())
	}

	w.openDialog(&dialog{
		title: w.t(i18n.WorktreeNewTitle), ok: w.t(i18n.DialogCreate), width: 580,
		onCancel: func() { w.after(w.openWorktrees) },
		body: func(f *form) {
			f.radio(worktreeModes, &mode)
			f.gap(2)
			if mode == worktreeModeExisting {
				f.muted(w.t(i18n.WorktreeNewCheckoutHint))
			} else {
				f.muted(w.t(i18n.WorktreeNewBranchHint))
			}
			f.gap(4)
			f.labelW = 96

			choices := worktreeChoices(w.repo())
			if mode == worktreeModeNew {
				id := f.field(w.t(i18n.WorktreeBranchName), branch, "feature-login", false)
				if !w.dialog.focused {
					w.dialog.focus = id
				}
			} else if visible := min(len(choices), worktreeBranchRowsMax); visible > 0 {
				// A list inside the dialog, not a combo: a popup opened from a
				// dialog is painted but cannot be clicked through its focus
				// trap (0012).
				f.custom(float32(visible)*worktreeBranchRowH+2, func(r ui.Rect) {
					w.p.text(w.t(i18n.WorktreeBranch), rect(r.X, r.Y+4, f.labelW, 20), fontUI, regular, w.pal.Text)
					w.paintWorktreeBranches(rect(r.X+f.labelW, r.Y, r.Width-f.labelW, float32(visible)*worktreeBranchRowH+2), choices, &existing, branchScroll)
				})
				// Switching to this mode hands the keyboard to the list, so a
				// keyboard user can pick a branch without the pointer. The
				// request is applied once and never pulls focus back.
				w.c.RequestFocus(idDialogBranchList)
				f.gap(6)
			}

			// Suggest a sibling directory until the user edits the path.
			auto := ""
			if b := branchName(); b != "" {
				auto = defaultWorktreePath(root, b)
			}
			if pathEd.String() == lastAuto && auto != lastAuto {
				lastAuto = auto
				w.after(func() { setText(pathEd, auto) })
			}
			f.field(w.t(i18n.WorktreePath), pathEd, "~/Projects/repo-feature-login", false)
			if mode == worktreeModeNew {
				f.field(w.t(i18n.WorktreeStartPoint), start, "HEAD", false)
			}

			switch {
			case mode == worktreeModeExisting && existing == "":
				if len(choices) == 0 {
					f.message(w.t(i18n.WorktreeNoBranches), w.pal.Error)
				} else {
					f.message(w.t(i18n.WorktreeEveryBranch), w.pal.Error)
				}
			case mode == worktreeModeExisting || strings.TrimSpace(branch.String()) != "":
				if p := problem(); p != "" {
					f.message(p, w.pal.Error)
				}
			}
		},
		okDisabled: func() bool { return problem() != "" },
		onOK: func() bool {
			path := absPath()
			if mode == worktreeModeExisting {
				w.ws.AddWorktreeExisting(path, existing, func(err error) {
					if err == nil {
						w.after(w.openWorktrees)
					}
				})
				return true
			}
			w.ws.AddWorktree(path, strings.TrimSpace(branch.String()), strings.TrimSpace(start.String()), func(err error) {
				if err == nil {
					w.after(w.openWorktrees)
				}
			})
			return true
		},
	})
}

// confirmDeleteWorktree removes a worktree, explaining that its branch stays.
func (w *Window) confirmDeleteWorktree(wt domain.Worktree) {
	force := false
	name := worktreeName(wt)
	w.openDialog(&dialog{
		title: w.t(i18n.WorktreeDeleteTitle), ok: w.t(i18n.DialogDelete), width: 560,
		onCancel: func() { w.after(w.openWorktrees) },
		body: func(f *form) {
			f.text(w.t(i18n.WorktreeDeleteQuestion, shortenHome(wt.Path)))
			f.gap(4)
			f.muted(w.t(i18n.WorktreeDeleteHint, name))
			f.gap(8)
			f.check(w.t(i18n.WorktreeForceDelete), &force)
		},
		onOK: func() bool {
			w.ws.RemoveWorktree(wt.Path, force, func(err error) {
				if err == nil {
					w.after(w.openWorktrees)
				}
			})
			return true
		},
	})
}
