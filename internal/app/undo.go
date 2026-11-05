package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/i18n"
)

// undoLimit bounds the session's undo history. It is a fixed window, not a
// cache: twenty operations is enough to walk back a mistaken cleanup, and each
// entry is a few strings.
const undoLimit = 20

// undoKind is the write operation a record stands for. The zero value is not a
// kind, so an entry without one can never be undone.
type undoKind uint8

const (
	undoCommit undoKind = iota + 1
	undoAmend
	undoMerge
	undoRebase
	undoCherryPick
	undoRevert
	undoReset
	undoCheckout
	undoCreateBranch
	undoDeleteBranch
	undoRenameBranch
)

// name is the user-facing operation name, the noun in "Undo Commit".
func (k undoKind) name() i18n.Key {
	switch k {
	case undoCommit:
		return i18n.AppUndoOpCommit
	case undoAmend:
		return i18n.AppUndoOpAmend
	case undoMerge:
		return i18n.AppUndoOpMerge
	case undoRebase:
		return i18n.AppUndoOpRebase
	case undoCherryPick:
		return i18n.AppUndoOpCherryPick
	case undoRevert:
		return i18n.AppUndoOpRevert
	case undoReset:
		return i18n.AppUndoOpReset
	case undoCheckout:
		return i18n.AppUndoOpCheckout
	case undoCreateBranch:
		return i18n.AppUndoOpCreateBranch
	case undoDeleteBranch:
		return i18n.AppUndoOpDeleteBranch
	case undoRenameBranch:
		return i18n.AppUndoOpRenameBranch
	}
	return i18n.AppUndoOpUnknown
}

// undoEntry is one successful write operation, recorded so the session can
// reverse it. before and after are read from Git on either side of the command
// while it holds the write lock, so a record never keeps a state somebody else
// changed in between.
type undoEntry struct {
	kind     undoKind
	repoRoot string
	before   gitcli.HeadState
	after    gitcli.HeadState
	// name is the branch the operation acted on, "" for operations that only
	// moved HEAD.
	name string
	// oldName is the branch's previous name, set for a rename only.
	oldName string
	// hash is the commit name pointed at when the operation finished; for a
	// deleted branch it is the commit that was deleted. It lets an undo refuse
	// a branch that moved after the record was taken.
	hash string
	// previous is the commit an overwritten branch pointed at before a create
	// replaced it, "" when the branch did not exist. Undo restores it instead
	// of deleting the branch.
	previous string
	when     time.Time
}

// changed reports that the operation left a state worth reversing. A merge or
// rebase that was already up to date changes nothing and is not recorded, but
// every branch operation is a real change even though HEAD stayed put.
func (e *undoEntry) changed() bool {
	switch e.kind {
	case undoCheckout:
		return e.before != e.after
	case undoCreateBranch, undoDeleteBranch, undoRenameBranch:
		return true
	}
	return e.before.Head != e.after.Head
}

// needsCleanWorktree reports the undo kinds that rewrite the working tree.
// A hard reset can overwrite an untracked file the target commit contains, so
// those undos refuse any change, untracked files included. A soft reset only
// moves HEAD and a branch edit touches no file, so those allow local changes.
func (e *undoEntry) needsCleanWorktree() bool {
	switch e.kind {
	case undoMerge, undoRebase, undoCherryPick, undoRevert, undoReset, undoCheckout:
		return true
	}
	return false
}

// needsRef reports the kinds whose entry is only usable with the ref hash the
// command captured; without it the undo could not tell a moved branch from the
// recorded one.
func (k undoKind) needsRef() bool {
	switch k {
	case undoCreateBranch, undoDeleteBranch, undoRenameBranch:
		return true
	}
	return false
}

// undoSpec is what a command hands run to have its successful run recorded.
// The command fills name, oldName and hash itself because only it knows which
// branch it acted on; run fills the HEAD states under the write lock.
type undoSpec struct {
	kind     undoKind
	name     string
	oldName  string
	hash     string
	previous string
}

// undoRefusal is a precondition that did not hold. It is not a Git failure, so
// the notification names the reason instead of Git's message.
type undoRefusal struct {
	key  i18n.Key
	args []any
}

func (r *undoRefusal) Error() string { return string(r.key) }

func refuse(key i18n.Key, args ...any) error { return &undoRefusal{key: key, args: args} }

// recordUndo adds a successful operation to the session history. It runs on
// the UI thread and drops an entry for a repository that is no longer open, so
// a late result from a switch cannot be undone against the wrong worktree.
func (w *Workspace) recordUndo(e *undoEntry) {
	if w.Repo == nil || w.Repo.Repo.Root != e.repoRoot || e.kind == 0 || !e.changed() {
		return
	}
	w.undos = append(w.undos, e)
	if len(w.undos) > undoLimit {
		w.undos = append([]*undoEntry(nil), w.undos[len(w.undos)-undoLimit:]...)
	}
}

// clearUndo forgets the session history: a switched, closed or removed
// repository has a different HEAD and its records no longer describe anything.
func (w *Workspace) clearUndo() { w.undos = nil }

// lastUndo is the newest record, nil when the session has none.
func (w *Workspace) lastUndo() *undoEntry {
	if len(w.undos) == 0 {
		return nil
	}
	return w.undos[len(w.undos)-1]
}

// popUndo removes e when it is still in the history.
func (w *Workspace) popUndo(e *undoEntry) {
	for i := len(w.undos) - 1; i >= 0; i-- {
		if w.undos[i] == e {
			w.undos = append(w.undos[:i:i], w.undos[i+1:]...)
			return
		}
	}
}

// UndoName is the name of the operation Undo would reverse ("Commit",
// "Merge"), "" when the session has nothing to undo. The interface builds the
// menu label from it.
func (w *Workspace) UndoName() string {
	e := w.lastUndo()
	if e == nil {
		return ""
	}
	return w.tr(e.kind.name())
}

// Undo reverses the newest recorded operation of the open repository. It is a
// write task like any other: it takes the write lock, re-reads the repository
// state under that lock, and refreshes afterwards, so an undo never acts on the
// snapshot the user saw. A failed or cancelled undo leaves the record in place,
// so the user can fix the condition and try again.
//
// Undo is not itself recorded: nothing the undo does goes through the undo
// spec, so pressing Command-Z twice walks further back instead of stacking
// "undo the undo".
func (w *Workspace) Undo() {
	rs := w.Repo
	if rs == nil || w.undoing {
		return
	}
	e := w.lastUndo()
	if e == nil || e.repoRoot != rs.Repo.Root {
		if e != nil {
			w.clearUndo()
		}
		w.Notify(NoticeInfo, w.tr(i18n.AppUndoNothing), w.tr(i18n.AppUndoNothingBody))
		return
	}
	if w.Busy() {
		// The stack is about to change when that command is recorded; undoing
		// now would reverse the wrong operation.
		w.Notify(NoticeInfo, w.tr(i18n.AppUndoBusy), w.tr(i18n.AppUndoBusyBody))
		return
	}
	w.undoing = true
	w.run("Undo "+w.tr(e.kind.name()), nil, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		if err := w.checkUndo(ctx, repo, e); err != nil {
			return err
		}
		return w.applyUndo(ctx, repo, e)
	}, RunOptions{
		OnError: func(err error) bool {
			var refusal *undoRefusal
			if errors.As(err, &refusal) {
				w.Notify(NoticeWarning, w.tr(i18n.AppUndoFailed, w.tr(e.kind.name())), w.tr(refusal.key, refusal.args...))
				return true
			}
			return false
		},
		After: func() {
			w.popUndo(e)
			w.Notify(NoticeSuccess, w.tr(i18n.AppUndoDone, w.tr(e.kind.name())), w.undoBody(e))
		},
		Always: func() {
			w.undoing = false
			// A switch during the undo invalidates what is left of the history.
			if w.Repo == nil || w.Repo.Repo.Root != e.repoRoot {
				w.clearUndo()
			}
		},
	})
}

// checkUndo re-reads the repository inside the write lock and refuses unless
// the recorded state is still the current one. This is the guard that keeps an
// external commit, checkout or rebase from being overwritten by a record the
// user made earlier.
func (w *Workspace) checkUndo(ctx context.Context, repo domain.Repo, e *undoEntry) error {
	if repo.Root != e.repoRoot {
		return refuse(i18n.AppUndoBlockedRepository)
	}
	if op := w.Git.Operation(ctx, repo); op.Active() {
		return refuse(i18n.AppUndoBlockedOperation, op.Kind.Title())
	}
	head, err := w.Git.Head(ctx, repo)
	if err != nil {
		return err
	}
	if head != e.after {
		return refuse(i18n.AppUndoBlockedMoved)
	}
	if e.needsCleanWorktree() {
		st, err := w.Git.Status(ctx, repo)
		if err != nil {
			return err
		}
		if !st.Clean() {
			return refuse(i18n.AppUndoBlockedDirty)
		}
	}
	switch e.kind {
	case undoCreateBranch:
		sha, err := w.Git.ResolveRef(ctx, repo, "refs/heads/"+e.name)
		if err != nil || sha != e.hash {
			return refuse(i18n.AppUndoBlockedBranch, e.name)
		}
	case undoDeleteBranch:
		if _, err := w.Git.ResolveRef(ctx, repo, "refs/heads/"+e.name); err == nil {
			return refuse(i18n.AppUndoBlockedBranchExists, e.name)
		}
	case undoRenameBranch:
		sha, err := w.Git.ResolveRef(ctx, repo, "refs/heads/"+e.name)
		if err != nil || sha != e.hash {
			return refuse(i18n.AppUndoBlockedBranch, e.name)
		}
		if _, err := w.Git.ResolveRef(ctx, repo, "refs/heads/"+e.oldName); err == nil {
			return refuse(i18n.AppUndoBlockedBranchExists, e.oldName)
		}
	}
	return nil
}

// applyUndo reverses the operation after checkUndo passed.
func (w *Workspace) applyUndo(ctx context.Context, repo domain.Repo, e *undoEntry) error {
	switch e.kind {
	case undoCommit, undoAmend:
		// A soft reset puts HEAD back but keeps the index, so the commit's
		// changes stay staged and can be committed again. It never touches the
		// working tree, which is why a dirty tree is allowed here.
		return w.Git.Reset(ctx, repo, gitcli.ResetSoft, e.before.Head)
	case undoMerge, undoRebase, undoCherryPick, undoRevert, undoReset:
		// These moved the branch to a new commit and left a clean tree, so a
		// hard reset restores exactly the recorded state.
		return w.Git.Reset(ctx, repo, gitcli.ResetHard, e.before.Head)
	case undoCheckout:
		return w.switchTo(ctx, repo, e.before)
	case undoCreateBranch:
		// Git refuses to delete the branch a worktree has checked out, so a
		// create-with-checkout first returns to the recorded state. The hash
		// check above proved the branch holds nothing new; force only skips
		// Git's merge check, not that hash guard.
		if e.after.Branch == e.name {
			if err := w.switchTo(ctx, repo, e.before); err != nil {
				return err
			}
		}
		if e.previous != "" {
			// The create overwrote an existing branch: put its old commit back
			// rather than deleting it.
			return w.Git.CreateBranch(ctx, repo, e.name, e.previous, false, true)
		}
		return w.Git.DeleteBranch(ctx, repo, e.name, true)
	case undoDeleteBranch:
		return w.Git.CreateBranch(ctx, repo, e.name, e.hash, false, false)
	case undoRenameBranch:
		return w.Git.RenameBranch(ctx, repo, e.name, e.oldName)
	}
	return fmt.Errorf("undo: unknown operation %d", e.kind)
}

// switchTo puts the worktree back on a recorded head state: its branch, or a
// detached HEAD at the recorded commit.
func (w *Workspace) switchTo(ctx context.Context, repo domain.Repo, hs gitcli.HeadState) error {
	if hs.Branch != "" {
		return w.Git.Checkout(ctx, repo, domain.Ref{Kind: domain.RefLocal, Short: hs.Branch})
	}
	return w.Git.CheckoutRevision(ctx, repo, hs.Head)
}

// undoBody describes where the undo left the repository, for the notification.
func (w *Workspace) undoBody(e *undoEntry) string {
	short := domain.ShortHash
	switch e.kind {
	case undoCommit:
		return w.tr(i18n.AppUndoCommitBody)
	case undoAmend:
		return w.tr(i18n.AppUndoAmendBody)
	case undoCheckout:
		if e.before.Branch != "" {
			return w.tr(i18n.AppUndoCheckoutBranchBody, e.before.Branch)
		}
		return w.tr(i18n.AppUndoCheckoutRevisionBody, short(e.before.Head))
	case undoCreateBranch:
		return w.tr(i18n.AppUndoCreateBranchBody, e.name)
	case undoDeleteBranch:
		return w.tr(i18n.AppUndoDeleteBranchBody, e.name, short(e.hash))
	case undoRenameBranch:
		return w.tr(i18n.AppUndoRenameBranchBody, e.name, e.oldName)
	}
	return w.tr(i18n.AppUndoResetBody, short(e.before.Head))
}
