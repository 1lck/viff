package app

import (
	"context"
	"fmt"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// CheckoutBlocked describes a checkout Git refused because local changes or
// untracked files would be overwritten. The UI decides whether to stash them
// and retry; see Workspace.ConfirmCheckout and Workspace.SmartCheckout.
type CheckoutBlocked struct {
	// Ref is the branch to switch to. It is zero when Rev is set.
	Ref domain.Ref
	// Rev detaches HEAD at that revision; set for revision checkouts.
	Rev string
	// Err is Git's original error.
	Err error
}

// label names the blocked target in messages.
func (b CheckoutBlocked) label() string {
	if b.Rev != "" {
		return domain.ShortHash(b.Rev)
	}
	return b.Ref.Short
}

// stashCheckoutResult tells how a stashing checkout ended.
type stashCheckoutResult int

const (
	// stashRestored means the changes are back in the working tree, either
	// after a successful checkout or after a failed one.
	stashRestored stashCheckoutResult = iota
	// stashKept means restoring the changes failed, so they stay in the stash.
	// For a failed checkout the branch did not change either.
	stashKept
	// stashNotCreated means the stash itself failed; nothing was attempted.
	stashNotCreated
)

// checkoutBlocked handles a checkout error that local changes caused. It
// returns false for every other error so the caller keeps Git's message; for
// a block it either asks the UI to stash and retry, or reports the failure.
func (w *Workspace) checkoutBlocked(b CheckoutBlocked) bool {
	switch gitcli.KindOf(b.Err) {
	case gitcli.ErrLocalChangesOverwrite, gitcli.ErrUntrackedOverwrite:
	default:
		return false
	}
	if w.ConfirmCheckout != nil && w.ConfirmCheckout(b) {
		return true
	}
	w.Notify(NoticeError, "Checkout failed", ErrorText(b.Err)+"\n\nCommit, stash or roll back your changes first.")
	return true
}

// SmartCheckout checks out the blocked target by stashing local changes
// (including untracked files) first and restoring them afterwards. It is
// called only after the user confirmed; nothing is stashed silently.
func (w *Workspace) SmartCheckout(b CheckoutBlocked) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	label := b.label()
	_, ctx, finish := w.startTask("Checkout "+label, true)
	go func() {
		lock := writeLock(repo)
		lock.Lock()
		res, err := w.checkoutStashing(ctx, repo, b)
		lock.Unlock()
		w.post(func() {
			finish()
			if w.Repo == rs {
				w.Refresh(true)
			}
			w.reportStashCheckout(label, res, err)
		})
	}()
}

// checkoutStashing stashes, checks out and restores. The changes are restored
// in both outcomes: after a successful checkout they follow the user, and
// after a failed one they belong to the branch that is still checked out.
func (w *Workspace) checkoutStashing(ctx context.Context, repo domain.Repo, b CheckoutBlocked) (stashCheckoutResult, error) {
	if err := w.Git.StashPush(ctx, repo, "viff: checkout "+b.label(), true, false); err != nil {
		return stashNotCreated, fmt.Errorf("stash local changes: %w", err)
	}
	var checkoutErr error
	if b.Rev != "" {
		checkoutErr = w.Git.CheckoutRevision(ctx, repo, b.Rev)
	} else {
		checkoutErr = w.Git.Checkout(ctx, repo, b.Ref)
	}
	// git stash pop removes the entry only when it applies cleanly, so a
	// conflict leaves the changes in the stash and the user can recover them.
	if _, popErr := w.Git.StashApply(ctx, repo, "stash@{0}", true, false); popErr != nil {
		return stashKept, checkoutErr
	}
	return stashRestored, checkoutErr
}

// reportStashCheckout tells how the stashing checkout ended. A kept stash is
// always spelled out: the user must know where the changes are.
func (w *Workspace) reportStashCheckout(label string, res stashCheckoutResult, err error) {
	switch {
	case res == stashNotCreated:
		w.Notify(NoticeError, "Checkout failed", ErrorText(err)+"\n\nNothing was stashed.")
	case res == stashKept && err != nil:
		w.Notify(NoticeError, "Checkout failed", ErrorText(err)+"\n\nRestoring the changes failed too; they are still in the stash and the branch did not change.")
	case res == stashKept:
		w.Notify(NoticeWarning, "Checked out "+label, "Your local changes conflict with the target. They are still in the stash; apply them when ready.")
	case err != nil:
		w.Notify(NoticeError, "Checkout failed", ErrorText(err)+"\n\nYour local changes were restored and the branch did not change.")
	default:
		w.Notify(NoticeSuccess, "Checked out "+label, "Local changes were stashed and restored.")
	}
}
