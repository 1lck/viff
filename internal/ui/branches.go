package viffui

import (
	"path/filepath"
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/ui/icons"
)

// openBranchPopup opens IntelliJ's Git Branches popup under the widget.
func (w *Window) openBranchPopup(anchor ui.Rect) {
	rs := w.repo()
	var items []item
	op := rs.Operation
	if op.Active() {
		t := op.Kind.Title()
		if op.CanContinue() {
			items = append(items, item{title: "Continue " + t, icon: icons.Success, run: func() { w.ws.OperationAction("continue") }})
		}
		if op.CanSkip() {
			items = append(items, item{title: "Skip Commit", run: func() { w.ws.OperationAction("skip") }})
		}
		if op.CanAbort() {
			items = append(items, item{title: "Abort " + t, icon: icons.Abort, run: func() { w.ws.OperationAction("abort") }})
		}
		items = append(items, sep())
	}
	hasRemote := len(rs.Remotes) > 0
	items = append(items,
		item{title: "Update Project…", icon: icons.Update, shortcut: "⌘T", disabled: !hasRemote, run: w.openUpdate},
		item{title: "Commit…", icon: icons.Commit, shortcut: "⌘K", run: w.focusCommit},
		item{title: "Push…", icon: icons.Push, shortcut: "⇧⌘K", disabled: !rs.HasHead(), run: w.openPush},
		sep(),
		item{title: "New Branch…", icon: icons.Add, shortcut: "⌥⌘N", disabled: !rs.HasHead(), run: func() { w.openNewBranch("", rs.HeadLabel()) }},
		item{title: "Checkout Tag or Revision…", run: w.openCheckoutRevision},
	)
	locals := rs.RefsOf(domain.RefLocal)
	if len(locals) > 0 {
		items = append(items, sep(), header("Local"))
		// Current first, then favourites, then the rest (IntelliJ order).
		ordered := make([]domain.Ref, 0, len(locals))
		for _, r := range locals {
			if r.Current {
				ordered = append(ordered, r)
			}
		}
		for _, r := range locals {
			if !r.Current && w.lg.favorites[r.Name] {
				ordered = append(ordered, r)
			}
		}
		for _, r := range locals {
			if !r.Current && !w.lg.favorites[r.Name] {
				ordered = append(ordered, r)
			}
		}
		for _, r := range ordered {
			items = append(items, w.branchItem(r))
		}
	}
	remotes := rs.RefsOf(domain.RefRemote)
	if len(remotes) > 0 {
		items = append(items, sep(), header("Remote"))
		for _, r := range remotes {
			items = append(items, w.branchItem(r))
		}
	}
	items = append(items, sep(), item{title: "Worktrees…", icon: icons.Folder, run: w.openWorktrees})
	p := &popup{x: anchor.X, y: bottom(anchor) + 4, items: items, sel: -1, minWidth: 340, title: "Git Branches", search: text.NewEditor("")}
	if anchor.Height == 0 {
		// Opened from the status bar: grow upwards.
		p.y = anchor.Y
	}
	w.popups = []*popup{p}
	w.tipID = ui.ID{}
}

func (w *Window) branchItem(r domain.Ref) item {
	fav := w.favorite(r.Name)
	it := item{title: r.Short, filter: r.Short, star: fav}
	switch {
	case r.Current:
		it.icon, it.tint = icons.CurrentBranchLabel, w.pal.RefHead
	case r.Kind == domain.RefRemote:
		it.icon, it.tint = icons.BranchLabel, w.pal.RefRemote
	default:
		it.icon, it.tint = icons.BranchLabel, w.pal.RefLocal
	}
	if r.Upstream != "" {
		it.hint = r.Upstream
		if r.Ahead > 0 {
			it.hint += " ↑" + itoa(r.Ahead)
		}
		if r.Behind > 0 {
			it.hint += " ↓" + itoa(r.Behind)
		}
	}
	if r.Gone {
		it.hint = strings.TrimSpace(it.hint + " (gone)")
	}
	// A branch can be checked out in one worktree only; say where it lives so
	// the checkout action's behaviour is not a surprise.
	if r.Kind == domain.RefLocal && !r.Current {
		if wt, ok := w.repo().WorktreeElsewhere(r.Short); ok {
			where := "in " + filepath.Base(wt.Path)
			it.hint = strings.TrimSpace(it.hint + " · " + where)
		}
	}
	ref := r
	it.subFn = func() []item { return w.refActions(ref) }
	return it
}

// favorite returns a stable pointer to a branch's favourite flag.
func (w *Window) favorite(name string) *bool {
	if w.favPtr == nil {
		w.favPtr = map[string]*bool{}
	}
	p := w.favPtr[name]
	if p == nil {
		v := w.lg.favorites[name]
		p = &v
		w.favPtr[name] = p
	}
	w.lg.favorites[name] = *p
	return p
}

// refActions is the submenu of a branch or tag.
func (w *Window) refActions(r domain.Ref) []item {
	rs := w.repo()
	cur := rs.CurrentBranch()
	curQ := "'" + cur + "'"
	name := r.Short
	q := "'" + name + "'"
	busy := rs.Operation.Active()
	var items []item
	if r.Kind == domain.RefTag {
		return []item{
			{title: "Checkout", run: func() { w.ws.CheckoutRevision(r.Name) }},
			{title: "New Branch from " + q + "…", icon: icons.Add, run: func() { w.openNewBranch(r.Name, name) }},
			sep(),
			{title: "Merge " + q + " into " + curQ, icon: icons.Merge, disabled: cur == "" || busy, run: func() { w.ws.Merge(r.Name, false) }},
			sep(),
			{title: "Push Tag to " + w.defaultRemote(), icon: icons.Push, disabled: len(rs.Remotes) == 0, run: func() { w.ws.PushTag(w.defaultRemote(), name) }},
			{title: "Delete Tag", icon: icons.Delete, run: func() {
				w.confirm("Delete Tag", "Delete tag "+q+" locally?", "Delete", func() { w.ws.DeleteTag(name) })
			}},
		}
	}
	if r.Current {
		items = append(items,
			item{title: "New Branch from " + q + "…", icon: icons.Add, run: func() { w.openNewBranch("", name) }},
			item{title: "Update", icon: icons.Update, disabled: r.Upstream == "" || busy, run: func() { w.ws.Pull(w.pullMode()) }},
			item{title: "Push…", icon: icons.Push, run: w.openPush},
			sep(),
			item{title: "Rename…", icon: icons.Edit, run: func() { w.openRename(r) }},
		)
		if r.Upstream == "" && len(rs.Remotes) > 0 {
			items = append(items, item{title: "Set Upstream…", run: func() { w.openSetUpstream(r) }})
		}
		return items
	}
	items = append(items,
		item{title: "Checkout", disabled: busy, run: func() { w.checkoutBranch(r) }},
		item{title: "New Branch from " + q + "…", icon: icons.Add, run: func() { w.openNewBranch(r.Name, name) }},
	)
	if cur != "" {
		items = append(items, item{title: "Checkout and Rebase onto " + curQ, disabled: busy || r.Kind != domain.RefLocal, run: func() {
			w.ws.Checkout(r)
			w.pendingRebase = cur
		}})
	}
	items = append(items, sep(),
		item{title: "Compare with " + curQ, icon: icons.Diff, disabled: cur == "", run: func() { w.compareBranches(cur, name) }},
		item{title: "Show Changed Files", icon: icons.Diff, disabled: cur == "" || r.Target == "", run: func() { w.compareBranchFiles(r) }},
		item{title: "Show Diff with Working Tree", icon: icons.Diff, run: func() { w.compareWithWorking(r.Name) }},
		sep(),
		item{title: "Rebase " + curQ + " onto " + q, disabled: cur == "" || busy, run: func() { w.ws.Rebase(r.Name) }},
		item{title: "Merge " + q + " into " + curQ, icon: icons.Merge, disabled: cur == "" || busy, run: func() { w.ws.Merge(r.Name, false) }},
		sep(),
	)
	if r.Kind == domain.RefLocal {
		items = append(items,
			item{title: "Push…", icon: icons.Push, disabled: len(rs.Remotes) == 0, run: func() { w.openPushFor(r) }},
			item{title: "Rename…", icon: icons.Edit, run: func() { w.openRename(r) }},
			item{title: "Delete", icon: icons.Delete, run: func() { w.ws.DeleteBranch(r, false) }},
		)
	} else {
		items = append(items, item{title: "Delete", icon: icons.Delete, run: func() {
			w.confirm("Delete Remote Branch", "Delete "+q+" on "+r.Remote()+"? Everyone who fetches will lose this branch.", "Delete", func() {
				w.ws.DeleteBranch(r, false)
			})
		}})
	}
	return items
}

// checkoutBranch checks a branch out here. A branch can be checked out in one
// worktree only, so when another worktree already has it, IntelliJ opens that
// worktree instead of failing the checkout.
func (w *Window) checkoutBranch(r domain.Ref) {
	if wt, ok := w.repo().WorktreeElsewhere(r.Short); ok {
		w.openRepo(wt.Path)
		return
	}
	w.ws.Checkout(r)
}

// compareBranches shows the commits of one branch missing from the other
// in the log.
func (w *Window) compareBranches(base, other string) {
	w.gitOpen = true
	w.gitTab = 0
	w.setFilter(func(f *vapp.LogFilter) { f.Branch = base + ".." + other })
	w.ws.Notify(vapp.NoticeInfo, "Comparing branches", "The log shows commits in '"+other+"' that are not in '"+base+"'. Clear the Branch filter to return.")
}

func (w *Window) defaultRemote() string {
	rs := w.repo()
	if up := rs.Status.Branch.Upstream; up != "" {
		if i := strings.IndexByte(up, '/'); i > 0 {
			return up[:i]
		}
	}
	for _, r := range rs.Remotes {
		if r.Name == "origin" {
			return r.Name
		}
	}
	if len(rs.Remotes) > 0 {
		return rs.Remotes[0].Name
	}
	return ""
}
