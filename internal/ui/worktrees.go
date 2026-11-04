package viffui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/domain"
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
				w.ws.NotifyError("Cannot list worktrees", err)
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
		title: "Worktrees", ok: "Close", width: 760, noCancel: true,
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
				w.p.text("Double-click a worktree to open it.", rect(r.X, r.Y, x-r.X-8, 30), fontSmall, regular, w.pal.TextSecond)
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

// openNewWorktree is IntelliJ's New Worktree dialog: a new branch checked out
// in a directory next to the repository.
func (w *Window) openNewWorktree() {
	rs := w.repo()
	if rs == nil {
		return
	}
	root := rs.Repo.Root
	branch := text.NewEditor("")
	pathEd := text.NewEditor("")
	start := text.NewEditor("HEAD")
	if b := rs.CurrentBranch(); b != "" {
		start = text.NewEditor(b)
	}
	lastAuto := ""

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
	problem := func() string {
		b := strings.TrimSpace(branch.String())
		if p := validRefName(b); p != "" {
			return p
		}
		if w.branchExists(b) {
			return "Branch '" + b + "' already exists. Pick another name."
		}
		p := absPath()
		switch {
		case p == "":
			return "Enter a path for the worktree."
		case !filepath.IsAbs(p):
			return "Use an absolute path, for example ~/Projects/repo-branch."
		}
		if _, err := os.Stat(p); err == nil {
			return p + " already exists."
		}
		return ""
	}

	w.openDialog(&dialog{
		title: "New Worktree", ok: "Create", width: 580,
		onCancel: func() { w.after(w.openWorktrees) },
		body: func(f *form) {
			f.labelW = 96
			id := f.field("Branch name:", branch, "feature-login", false)
			if !w.dialog.focused {
				w.dialog.focus = id
			}
			// Suggest a sibling directory until the user edits the path.
			b := strings.TrimSpace(branch.String())
			auto := ""
			if b != "" {
				auto = defaultWorktreePath(root, b)
			}
			if pathEd.String() == lastAuto && auto != lastAuto {
				lastAuto = auto
				w.after(func() { setText(pathEd, auto) })
			}
			f.field("Path:", pathEd, "~/Projects/repo-feature-login", false)
			f.field("Start point:", start, "HEAD", false)
			if b != "" {
				if p := problem(); p != "" {
					f.message(p, w.pal.Error)
				}
			}
			f.muted("Creates the branch and checks it out in a new worktree.")
		},
		okDisabled: func() bool {
			return strings.TrimSpace(branch.String()) == "" || problem() != ""
		},
		onOK: func() bool {
			w.ws.AddWorktree(absPath(), strings.TrimSpace(branch.String()), strings.TrimSpace(start.String()), func(err error) {
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
		title: "Delete Worktree", ok: "Delete", width: 560,
		onCancel: func() { w.after(w.openWorktrees) },
		body: func(f *form) {
			f.text("Delete the worktree at " + shortenHome(wt.Path) + "?")
			f.gap(4)
			f.muted("The branch '" + name + "' and its commits stay in the repository. Uncommitted changes in that worktree are lost.")
			f.gap(8)
			f.check("Force delete (discard modified and untracked files)", &force)
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
