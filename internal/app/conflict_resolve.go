package app

import (
	"context"
	"errors"
	"fmt"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/i18n"
)

// ResolveConflict accepts one side of one conflict block in the working-tree
// file of an unmerged path. It is the in-app counterpart of editing the file
// by hand: only the bytes of that block change; the file keeps its line
// endings, its final newline and every other block.
//
// The conflict viewer no longer calls this: its Result pane applies a block to
// the editor buffer and writes through SaveFile, so unsaved manual edits and
// undo survive (see docs/decisions/0047-editable-conflict-result.md). The
// method stays as the ConflictResolver backend capability; removing it is a
// separate decision.
//
// The operation reads the file from disk before writing and refuses a file
// that no longer matches the view d was built from (ErrStale), so an external
// editor's save that landed before the last check is not overwritten. The
// check runs immediately before the atomic rename; a save racing that rename
// cannot be detected. It also refuses binary, oversized and non-textual
// conflicts, and paths that Git no longer lists as unmerged.
//
// Nothing is staged. Once no markers are left the user keeps the ordinary
// "Mark as Resolved" step, which stages the path and lets the merge,
// cherry-pick or rebase continue. done runs on the UI thread with nil on
// success, or with the reason on failure; failures are also notified.
func (w *Workspace) ResolveConflict(d *DiffResult, block int, choice domain.ResolveChoice, done func(error)) {
	if done == nil {
		done = func(error) {}
	}
	fail := func(err error) {
		w.NotifyError(w.tr(i18n.AppNoticeResolveConflictFailed), err)
		w.post(func() { done(err) })
	}
	if w.Repo == nil {
		fail(errors.New(w.tr(i18n.AppErrorOpenRepositoryFirst)))
		return
	}
	if !choice.Valid() {
		fail(fmt.Errorf("unknown conflict choice %d", choice))
		return
	}
	if d == nil || d.Conflict == nil || !d.Source.Working {
		fail(errors.New(w.tr(i18n.AppErrorNoConflictView)))
		return
	}
	cv := d.Conflict
	if !cv.Conflict.Kind.Textual() {
		fail(fmt.Errorf("%w: %s", gitcli.ErrResolveUnsupported, w.tr(i18n.AppErrorNoTextToMerge)))
		return
	}
	switch {
	case !cv.Current.Content.Exists:
		fail(fmt.Errorf("%w: %s", gitcli.ErrResolveMissing, cv.Conflict.Path))
		return
	case cv.Current.Content.Binary:
		fail(fmt.Errorf("%w: %s", gitcli.ErrResolveBinary, cv.Conflict.Path))
		return
	case cv.Current.Content.TooLarge:
		fail(fmt.Errorf("%w: %s", gitcli.ErrResolveTooLarge, cv.Conflict.Path))
		return
	case cv.Current.Content.Submodule, cv.Current.Content.Symlink:
		fail(fmt.Errorf("%w: %s", gitcli.ErrResolveUnsupported, cv.Conflict.Path))
		return
	case !cv.Current.Text:
		fail(fmt.Errorf("%w: %s", gitcli.ErrResolveUnsupported, cv.Conflict.Path))
		return
	}
	if !cv.Markers.Reliable {
		fail(fmt.Errorf("%w: %s", domain.ErrConflictMarkers, cv.Conflict.Path))
		return
	}
	if block < 0 || block >= len(cv.Markers.Blocks) {
		fail(fmt.Errorf("%w: block %d of %d", domain.ErrConflictBlock, block, len(cv.Markers.Blocks)))
		return
	}
	path := cv.Conflict.Path
	expected := append([]byte(nil), cv.Current.Content.Data...)
	title := "Accept " + choice.String()
	w.Run(title, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		resolver, rerr := w.conflictResolver()
		if rerr != nil {
			return rerr
		}
		_, err := resolver.ResolveConflictBlock(ctx, repo, gitcli.ConflictResolveRequest{
			Path:     path,
			Block:    block,
			Choice:   choice,
			Expected: expected,
		})
		if errors.Is(err, gitcli.ErrResolveStale) {
			// The diff layer's stale message is what the UI and the other
			// line actions already show for an externally changed file.
			return ErrStale
		}
		return err
	}, RunOptions{
		OnError: func(err error) bool {
			done(err)
			if errors.Is(err, ErrStale) {
				w.Notify(NoticeWarning, w.tr(i18n.AppNoticeFileChanged), w.tr(i18n.AppNoticeFileChangedBody))
				return true
			}
			return false
		},
		After: func() { done(nil) },
	})
}
