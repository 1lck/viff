package app

import (
	"context"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// The accessors below return the optional service of the active backend, or
// ErrUnsupported when it has none. Call sites in the command layer use them
// instead of calling the backend directly, so one backend can offer rebase and
// another can refuse it without the adapter stubbing methods it cannot
// implement.
//
// The UI hides the related entry when the capability is nil, so ErrUnsupported
// should be unreachable through normal use; it exists so a stale window or a
// keyboard shortcut cannot panic or silently do nothing.

func (w *Workspace) history() (vcs.HistoryWriter, error) {
	if w.Caps.HistoryWriter == nil {
		return nil, ErrUnsupported
	}
	return w.Caps.HistoryWriter, nil
}

func (w *Workspace) stash() (vcs.StashWriter, error) {
	if w.Caps.Stash == nil {
		return nil, ErrUnsupported
	}
	return w.Caps.Stash, nil
}

func (w *Workspace) worktrees() (vcs.WorktreeWriter, error) {
	if w.Caps.Worktrees == nil {
		return nil, ErrUnsupported
	}
	return w.Caps.Worktrees, nil
}

func (w *Workspace) reflog() (vcs.ReflogReader, error) {
	if w.Caps.Reflog == nil {
		return nil, ErrUnsupported
	}
	return w.Caps.Reflog, nil
}

func (w *Workspace) submodules() (vcs.SubmoduleWriter, error) {
	if w.Caps.Submodules == nil {
		return nil, ErrUnsupported
	}
	return w.Caps.Submodules, nil
}

func (w *Workspace) refs() (vcs.ReferenceWriter, error) {
	if w.Caps.References == nil {
		return nil, ErrUnsupported
	}
	return w.Caps.References, nil
}

func (w *Workspace) conflictResolver() (vcs.ConflictResolver, error) {
	if w.Caps.Conflicts == nil {
		return nil, ErrUnsupported
	}
	return w.Caps.Conflicts, nil
}

// The worktree writes are wrapped so the command layer keeps one line per call
// site while still going through the capability check.
func (w *Workspace) addWorktree(ctx context.Context, repo domain.Repo, path, branch, start string) error {
	wt, err := w.worktrees()
	if err != nil {
		return err
	}
	return wt.AddWorktree(ctx, repo, path, branch, start)
}

func (w *Workspace) addWorktreeExisting(ctx context.Context, repo domain.Repo, path, branch string) error {
	wt, err := w.worktrees()
	if err != nil {
		return err
	}
	return wt.AddWorktreeExisting(ctx, repo, path, branch)
}

func (w *Workspace) removeWorktree(ctx context.Context, repo domain.Repo, path string, force bool) error {
	wt, err := w.worktrees()
	if err != nil {
		return err
	}
	return wt.RemoveWorktree(ctx, repo, path, force)
}

// Has reports whether an optional capability is present, for the UI to decide
// whether to draw an entry.
func (w *Workspace) Has(feature string) bool {
	switch feature {
	case "history":
		return w.Caps.HistoryWriter != nil
	case "stash":
		return w.Caps.Stash != nil
	case "worktrees":
		return w.Caps.Worktrees != nil
	case "reflog":
		return w.Caps.Reflog != nil
	case "submodules":
		return w.Caps.Submodules != nil
	case "refs":
		return w.Caps.References != nil
	case "conflicts":
		return w.Caps.Conflicts != nil
	case "index":
		return w.Caps.Index
	case "amend":
		return w.Caps.Amend
	case "properties":
		return w.Caps.Properties
	case "locks":
		return w.Caps.Locks
	}
	return false
}
