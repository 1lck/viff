package viffui

// Remote links in the window: the entries that open a commit, a branch, a
// comparison and the pull request form on the hosting service, and the notices
// that explain a link Viff could not build or open. The addresses themselves
// come from app's RemoteLink; the window only decides which entry to show.

import (
	"errors"
	"strings"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
)

// openLink hands an address to the system browser. A failure is notified: an
// entry that does nothing leaves the user unable to tell a missing browser from
// a wrong address.
func (w *Window) openLink(url string) {
	if err := w.openURL(url); err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
	}
}

// openCommitOnHost opens the page of one commit on the service of the
// repository's remote. The current branch picks the remote, so a fork compares
// on the fork while a detached HEAD still finds the default remote.
func (w *Window) openCommitOnHost(hash string) {
	link, err := w.repo().LinkFor(w.repo().CurrentBranch())
	if err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
		return
	}
	url, err := link.CommitURL(hash)
	if err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
		return
	}
	w.openLink(url)
}

// openBranchPage opens the file listing of a branch.
func (w *Window) openBranchPage(branch string) {
	link, err := w.repo().LinkFor(branch)
	if err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
		return
	}
	url, err := link.BranchURL(branch)
	if err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
		return
	}
	w.openLink(url)
}

// openCompare opens the commits a branch has that base does not.
func (w *Window) openCompare(branch, base string) {
	link, err := w.repo().LinkFor(branch)
	if err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
		return
	}
	url, err := link.CompareURL(base, branch)
	if errors.Is(err, vapp.ErrLinkUnsupported) {
		w.openBranchFallback(link, branch)
		return
	}
	if err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
		return
	}
	w.openLink(url)
}

// openPullRequest opens the service form that creates a pull request (a merge
// request on GitLab) from branch into base; the user reviews it there, Viff
// never posts anything.
func (w *Window) openPullRequest(branch, base string) {
	link, err := w.repo().LinkFor(branch)
	if err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
		return
	}
	url, err := link.NewPullRequestURL(base, branch)
	if errors.Is(err, vapp.ErrLinkUnsupported) {
		w.openBranchFallback(link, branch)
		return
	}
	if err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
		return
	}
	w.openLink(url)
}

// openBranchFallback opens the branch page on a service whose comparison and
// pull request addresses Viff does not build, and says so: opening a page the
// user did not ask for without a word would be worse than the missing entry.
func (w *Window) openBranchFallback(link vapp.RemoteLink, branch string) {
	url, err := link.BranchURL(branch)
	if err != nil {
		w.ws.NotifyError(w.t(i18n.LinksCannotOpen), err)
		return
	}
	w.ws.Notify(vapp.NoticeInfo, w.t(i18n.LinksFallback, link.Label()), branch)
	w.openLink(url)
}

// branchLinkActions is the branch submenu's group of web links. It is empty
// when the remote has no web page (a local path, a remote Viff cannot read), so
// an entry never opens a guess.
func (w *Window) branchLinkActions(r domain.Ref) []item {
	rs := w.repo()
	if rs == nil || r.Kind != domain.RefLocal {
		return nil
	}
	link, err := rs.LinkFor(r.Short)
	if err != nil {
		return nil
	}
	name, label := r.Short, link.Label()
	items := []item{
		{title: w.t(i18n.LinksOpenBranch, name, label), run: func() { w.openBranchPage(name) }},
	}
	compare := w.t(i18n.LinksCompareChoose)
	if base, ok := rs.BaseBranch(name); ok {
		compare = w.t(i18n.LinksCompare, base)
	}
	items = append(items, item{title: compare, run: func() {
		w.withBaseBranch(name, func(base string) { w.openCompare(name, base) })
	}})
	// A pull request needs a branch the service can see: the current branch, or
	// one that already tracks a remote branch. Anything else would open a form
	// for a branch that does not exist there yet.
	if r.Current || r.Upstream != "" {
		items = append(items, item{title: w.t(i18n.LinksCreatePR), run: func() {
			w.withBaseBranch(name, func(base string) { w.openPullRequest(name, base) })
		}})
	}
	return items
}

// withBranchLinks appends the web-link group to a branch submenu.
func (w *Window) withBranchLinks(items []item, r domain.Ref) []item {
	if links := w.branchLinkActions(r); len(links) > 0 {
		items = append(items, sep())
		items = append(items, links...)
	}
	return items
}

// withBaseBranch runs action with the branch a comparison or pull request from
// branch targets. When Viff cannot tell which branch that is, it asks instead
// of opening a page for a guess.
func (w *Window) withBaseBranch(branch string, action func(base string)) {
	if base, ok := w.repo().BaseBranch(branch); ok {
		action(base)
		return
	}
	w.pickBaseBranch(branch, action)
}

// pickBaseBranch lists the branches the user can compare with. The candidates
// come from the state Viff already holds, so opening the list costs no Git call;
// local branches come first because that is what a user usually picks.
func (w *Window) pickBaseBranch(branch string, action func(base string)) {
	rs := w.repo()
	items := []item{}
	seen := map[string]bool{}
	add := func(name, hint string) {
		if name == "" || name == branch || seen[name] {
			return
		}
		seen[name] = true
		items = append(items, item{title: name, hint: hint, filter: name, run: func() { action(name) }})
	}
	for _, r := range rs.RefsOf(domain.RefLocal) {
		add(r.Short, "")
	}
	for _, r := range rs.RefsOf(domain.RefRemote) {
		remote := r.Remote()
		add(strings.TrimPrefix(r.Short, remote+"/"), remote)
	}
	if len(items) == 0 {
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.LinksChooseBase), w.t(i18n.LinksNoBase))
		return
	}
	w.popups = []*popup{{x: w.mouse.X, y: w.mouse.Y, items: items, sel: -1, minWidth: 280, title: w.t(i18n.LinksChooseBase)}}
	w.tipID = ui.ID{}
}
