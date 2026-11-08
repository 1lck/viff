package viffui

import (
	"path/filepath"
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// canWriteBranches reports whether the active backend can create, rename and
// delete local branches. A nil capability means the entries stay disabled
// rather than opening a dialog that cannot do anything.
func (w *Window) canWriteBranches() bool { return w.ws.Caps.Branches != nil }

// openBranchPopup opens IntelliJ's Git Branches popup under the widget.
func (w *Window) openBranchPopup(anchor ui.Rect) {
	rs := w.repo()
	readOnly := w.readOnly()
	var items []item
	op := rs.Operation
	if op.Active() && !readOnly {
		t := op.Kind.Title()
		if op.CanContinue() {
			items = append(items, item{title: w.t(i18n.ChromeBranchContinue, t), icon: icons.Success, run: func() { w.ws.OperationAction("continue") }})
		}
		if op.CanSkip() {
			items = append(items, item{title: w.t(i18n.ChromeBranchSkip), run: func() { w.ws.OperationAction("skip") }})
		}
		if op.CanAbort() {
			items = append(items, item{title: w.t(i18n.ChromeBranchAbort, t), icon: icons.Abort, run: func() { w.ws.OperationAction("abort") }})
		}
		items = append(items, sep())
	}
	hasRemote := len(rs.Remotes) > 0
	items = append(items,
		item{title: w.t(i18n.ChromeTbUpdate), icon: icons.Update, shortcut: "⌘T", disabled: readOnly || !hasRemote, run: w.openUpdate},
		item{title: w.t(i18n.ChromeTbCommit), icon: icons.Commit, shortcut: "⌘K", disabled: readOnly, run: w.focusCommit},
	)
	// A backend whose commit is the server operation has nothing to push.
	if w.ws.Caps.SeparatePush {
		items = append(items, item{title: w.t(i18n.ChromeTbPush), icon: icons.Push, shortcut: "⇧⌘K", disabled: readOnly || !rs.HasHead(), run: w.openPush})
	}
	items = append(items,
		sep(),
		item{title: w.t(i18n.ChromeMenuNewBranch), icon: icons.Add, shortcut: "⌥⌘N", disabled: readOnly || !rs.HasHead() || !w.canWriteBranches(), run: func() { w.openNewBranch("", rs.HeadLabel()) }},
		item{title: w.t(i18n.ChromeMenuCheckoutTagRev), disabled: readOnly, run: w.openCheckoutRevision},
	)
	locals := rs.RefsOf(domain.RefLocal)
	if len(locals) > 0 {
		items = append(items, sep(), header(w.t(i18n.ChromeRefsLocal)))
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
		items = append(items, sep(), header(w.t(i18n.ChromeRefsRemote)))
		for _, r := range remotes {
			items = append(items, w.branchItem(r))
		}
	}
	if w.ws.Caps.Worktrees != nil {
		// The panel is a read-only view; its New/Delete actions disable for a
		// remote repository, but opening the list is still useful.
		items = append(items, sep(), item{title: w.t(i18n.ChromeMenuWorktrees), icon: icons.Folder, run: w.openWorktrees})
	}
	p := &popup{x: anchor.X, y: bottom(anchor) + 4, items: items, sel: -1, minWidth: 340, title: w.t(i18n.ChromePopupGitBranch), search: text.NewEditor("")}
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
		it.hint = strings.TrimSpace(it.hint + w.t(i18n.ChromeRefsGone))
	}
	// A branch can be checked out in one worktree only; say where it lives so
	// the checkout action's behaviour is not a surprise.
	if r.Kind == domain.RefLocal && !r.Current {
		if wt, ok := w.repo().WorktreeElsewhere(r.Short); ok {
			where := w.t(i18n.ChromeBranchInWorktree, filepath.Base(wt.Path))
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
	if w.readOnly() {
		// Observation mode: compare and browse, never checkout, merge, push,
		// rename or delete. The current branch keeps the same read-only set so
		// the submenu cannot become a write back door.
		items := []item{}
		if cur := rs.CurrentBranch(); cur != "" && !r.Current {
			items = append(items, item{title: w.t(i18n.ChromeMenuCompareWith, "'"+cur+"'"), icon: icons.Diff, run: func() { w.compareBranches(cur, r.Short) }})
		}
		items = append(items,
			item{title: w.t(i18n.ChromeMenuShowChanged), icon: icons.Diff, disabled: r.Target == "", run: func() { w.compareBranchFiles(r) }},
			item{title: w.t(i18n.ChromeMenuShowDiffWorking), icon: icons.Diff, run: func() { w.compareWithWorking(r.Name) }},
		)
		return w.withBranchLinks(items, r)
	}
	cur := rs.CurrentBranch()
	curQ := "'" + cur + "'"
	name := r.Short
	q := "'" + name + "'"
	busy := rs.Operation.Active()
	var items []item
	if r.Kind == domain.RefTag {
		return []item{
			{title: w.t(i18n.ChromeActionCheckout), run: func() { w.ws.CheckoutRevision(r.Name) }},
			{title: w.t(i18n.ChromeMenuNewBranchFrom, q), icon: icons.Add, run: func() { w.openNewBranch(r.Name, name) }},
			sep(),
			{title: w.t(i18n.ChromeMenuMergeInto, q, curQ), icon: icons.Merge, disabled: cur == "" || busy, run: func() { w.ws.Merge(r.Name, false) }},
			sep(),
			{title: w.t(i18n.ChromeMenuPushTagTo, w.defaultRemote()), icon: icons.Push, disabled: len(rs.Remotes) == 0, run: func() { w.ws.PushTag(w.defaultRemote(), name) }},
			{title: w.t(i18n.ChromeMenuDeleteTag), icon: icons.Delete, run: func() {
				w.confirm(w.t(i18n.ChromeMenuDeleteTag), w.t(i18n.ChromeDialogDeleteTag, q), w.t(i18n.ChromeActionDelete), func() { w.ws.DeleteTag(name) })
			}},
			{title: w.t(i18n.ChromeMenuDeleteRemoteTag), icon: icons.Delete, disabled: len(rs.Remotes) == 0, run: func() { w.openDeleteRemoteTag(name) }},
		}
	}
	if r.Current {
		items = append(items,
			item{title: w.t(i18n.ChromeMenuNewBranchFrom, q), icon: icons.Add, run: func() { w.openNewBranch("", name) }},
			item{title: w.t(i18n.ChromeMenuUpdate), icon: icons.Update, disabled: r.Upstream == "" || busy, run: func() { w.ws.Pull(w.pullMode()) }},
			item{title: w.t(i18n.ChromeTbPush), icon: icons.Push, run: w.openPush},
			item{title: w.t(i18n.ChromeMenuPushAnother), icon: icons.Push, run: func() { w.openPushAnother(r) }},
			sep(),
			item{title: w.t(i18n.ChromeMenuRename), icon: icons.Edit, run: func() { w.openRename(r) }},
		)
		if r.Upstream == "" && len(rs.Remotes) > 0 {
			items = append(items, item{title: w.t(i18n.ChromeMenuSetUpstream), run: func() { w.openSetUpstream(r) }})
		}
		return w.withBranchLinks(items, r)
	}
	items = append(items,
		item{title: w.t(i18n.ChromeActionCheckout), disabled: busy, run: func() { w.checkoutBranch(r) }},
		item{title: w.t(i18n.ChromeMenuNewBranchFrom, q), icon: icons.Add, run: func() { w.openNewBranch(r.Name, name) }},
	)
	if cur != "" {
		items = append(items, item{title: w.t(i18n.ChromeMenuCheckoutRebase, curQ), disabled: busy || r.Kind != domain.RefLocal, run: func() {
			w.ws.Checkout(r)
			w.pendingRebase = cur
		}})
	}
	items = append(items, sep(),
		item{title: w.t(i18n.ChromeMenuCompareWith, curQ), icon: icons.Diff, disabled: cur == "", run: func() { w.compareBranches(cur, name) }},
		item{title: w.t(i18n.ChromeMenuShowChanged), icon: icons.Diff, disabled: cur == "" || r.Target == "", run: func() { w.compareBranchFiles(r) }},
		item{title: w.t(i18n.ChromeMenuShowDiffWorking), icon: icons.Diff, run: func() { w.compareWithWorking(r.Name) }},
	)
	// Rebase and merge rewrite history; a backend without that (Subversion)
	// does not offer them at all.
	if w.ws.Caps.HistoryWriter != nil {
		items = append(items,
			item{title: w.t(i18n.ChromeMenuRebaseOnto, curQ, q), disabled: cur == "" || busy, run: func() { w.ws.Rebase(r.Name) }},
			item{title: w.t(i18n.ChromeMenuMergeInto, q, curQ), icon: icons.Merge, disabled: cur == "" || busy, run: func() { w.ws.Merge(r.Name, false) }},
			sep(),
		)
	}
	if r.Kind == domain.RefLocal {
		if w.ws.Caps.SeparatePush {
			items = append(items, item{title: w.t(i18n.ChromeTbPush), icon: icons.Push, disabled: len(rs.Remotes) == 0, run: func() { w.openPushFor(r) }})
		}
		items = append(items,
			item{title: w.t(i18n.ChromeMenuRename), icon: icons.Edit, disabled: !w.canWriteBranches(), run: func() { w.openRename(r) }},
			item{title: w.t(i18n.ChromeActionDelete), icon: icons.Delete, disabled: !w.canWriteBranches(), run: func() { w.ws.DeleteBranch(r, false) }},
		)
		// A backend whose Remotes list is a fixed placeholder must not offer
		// this: the entry has to follow the capability, not the length of the
		// list.
		if w.ws.Caps.BranchUpdate && len(rs.Remotes) > 0 {
			items = append(items, item{title: w.t(i18n.ChromeMenuUpdateFromRemote), icon: icons.Update, disabled: busy, run: func() { w.updateBranchFromRemote(r) }})
		}
	} else {
		items = append(items, item{title: w.t(i18n.ChromeActionDelete), icon: icons.Delete, run: func() {
			w.confirm(w.t(i18n.ChromeDialogDeleteRemoteBranch), w.t(i18n.ChromeDialogDeleteRemoteBranchMsg, q, r.Remote()), w.t(i18n.ChromeActionDelete), func() {
				w.ws.DeleteBranch(r, false)
			})
		}})
	}
	return w.withBranchLinks(items, r)
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
	w.ws.Notify(vapp.NoticeInfo, w.t(i18n.ChromeNotifyComparingBranches), w.t(i18n.ChromeNotifyComparingBranchesMessage, other, base))
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
