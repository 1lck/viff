package viffui

import (
	"strings"

	"viff/internal/domain"
)

// branchRemote picks the remote a local branch is updated from: the remote of
// its configured upstream when it has one, the default remote otherwise.
func (w *Window) branchRemote(r domain.Ref) string {
	if r.Upstream != "" {
		if i := strings.IndexByte(r.Upstream, '/'); i > 0 {
			return r.Upstream[:i]
		}
	}
	return w.defaultRemote()
}

// updateBranchFromRemote fast-forwards a branch that this window has not
// checked out. The branch popup only offers it for local branches that are
// not current, because a checked-out branch is updated by pulling.
func (w *Window) updateBranchFromRemote(r domain.Ref) {
	if w.guardReadOnly() {
		return
	}
	if remote := w.branchRemote(r); remote != "" {
		w.ws.UpdateBranch(remote, r.Short)
	}
}

// fetchFullHistory offers to complete a shallow clone's history.
func (w *Window) fetchFullHistory() {
	if w.guardReadOnly() {
		return
	}
	w.ws.UnshallowRepository()
}
