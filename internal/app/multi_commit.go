package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// MultiCommitSource is the change one selected commit made to an aggregated
// file, relative to that commit's first parent. Commit and Parent are what the
// diff viewer needs to show the real change of that commit; Change keeps the
// exact kind and old path, so a deletion or a rename stays explained by the
// commit that made it.
type MultiCommitSource struct {
	Commit domain.Commit
	// Parent is the first parent of Commit, "" for a root commit.
	Parent string
	Change domain.Change
}

// MultiCommitFile is one file in the deduplicated union of several commits'
// changed files. Sources keeps every selected commit that touched the path, so
// the UI can offer each commit's real diff instead of a merged one.
type MultiCommitFile struct {
	Path string
	// Kind summarizes the row for display. Sources keeps the exact kinds.
	Kind domain.ChangeKind
	// RenamedFrom is the old path of the newest rename source, if any.
	RenamedFrom string
	// Sources are the selected commits that changed Path, newest first.
	Sources []MultiCommitSource
}

// MultiCommitSummary is the union of the changed files of the selected commits.
// Each commit contributed its changes against its first parent; the union is
// not the diff between the oldest and the newest selected commit.
type MultiCommitSummary struct {
	// Commits are the selected commits in the order they were passed, which is
	// the log's selection order (newest first). They are never re-sorted by
	// date, so the panel lists them as the selection does.
	Commits []domain.Commit
	Files   []MultiCommitFile
	// Err reports commits that could not be read. Files still covers the other
	// commits, so a partial summary is shown together with the failure.
	Err error
}

// MultiCommitChanges loads the changed files of each commit against its first
// parent and returns their deduplicated union. The query runs in the
// background; done runs on the UI thread with the result, and is dropped when
// the repository was closed or reopened while Git was working.
//
// The returned cancel function stops the remaining per-commit Git processes.
// The UI calls it when the selection changes, so a burst of Shift or Meta
// clicks does not leave one Git process per commit running for a selection the
// user has already left. It is nil when no query was started.
func (w *Workspace) MultiCommitChanges(commits []domain.Commit, done func(MultiCommitSummary)) context.CancelFunc {
	rs := w.Repo
	if rs == nil || len(commits) == 0 || done == nil {
		return nil
	}
	repo := rs.Repo
	list := append([]domain.Commit(nil), commits...)
	ctx, cancel := context.WithTimeout(context.Background(), multiCommitTimeout(len(list)))
	go func() {
		defer cancel()
		sum := aggregateMultiCommits(w.Git.MultiCommitChanges(ctx, repo, list))
		ctxErr := ctx.Err()
		if multiCommitResultDropped(ctxErr) {
			// A newer selection cancelled this request; its result belongs to
			// no view.
			return
		}
		if ctxErr != nil {
			// A deadline is not a superseded selection: deliver what was read
			// with the failure, so the panel ends its loading state instead of
			// waiting forever for a result that will not come.
			sum.Err = errors.Join(sum.Err, ctxErr)
		}
		w.post(func() {
			if w.Repo != rs {
				return
			}
			done(sum)
		})
	}()
	return cancel
}

// multiCommitResultDropped reports a result that must not reach the UI: only an
// explicit cancellation means the selection moved on. A deadline expires on its
// own and still has to end the view with its error.
func multiCommitResultDropped(err error) bool {
	return errors.Is(err, context.Canceled)
}

// multiCommitTimeout allows a larger selection more time without letting a
// stuck Git process keep the query alive forever.
func multiCommitTimeout(commits int) time.Duration {
	d := time.Minute + time.Duration(commits)*2*time.Second
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

// aggregateMultiCommits deduplicates per-commit change lists by destination
// path and sorts the result by path. A commit that failed is recorded in Err;
// its files are unknown, but the other commits still contribute.
func aggregateMultiCommits(perCommit []gitcli.MultiCommitChanges) MultiCommitSummary {
	sum := MultiCommitSummary{}
	index := make(map[string]int)
	var errs []error
	for _, pc := range perCommit {
		sum.Commits = append(sum.Commits, pc.Commit)
		if pc.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", domain.ShortHash(pc.Commit.Hash), pc.Err))
			continue
		}
		parent := ""
		if len(pc.Commit.Parents) > 0 {
			parent = pc.Commit.Parents[0]
		}
		for _, ch := range pc.Changes {
			i, ok := index[ch.Path]
			if !ok {
				i = len(sum.Files)
				index[ch.Path] = i
				sum.Files = append(sum.Files, MultiCommitFile{Path: ch.Path})
			}
			sum.Files[i].Sources = append(sum.Files[i].Sources, MultiCommitSource{Commit: pc.Commit, Parent: parent, Change: ch})
		}
	}
	for i := range sum.Files {
		f := &sum.Files[i]
		f.Kind = summarizeMultiCommitKind(f.Sources)
		for _, s := range f.Sources {
			if s.Change.Kind == domain.ChangeRenamed && s.Change.OldPath != "" {
				f.RenamedFrom = s.Change.OldPath
				break
			}
		}
	}
	sort.SliceStable(sum.Files, func(i, j int) bool {
		return strings.ToLower(sum.Files[i].Path) < strings.ToLower(sum.Files[j].Path)
	})
	sum.Err = errors.Join(errs...)
	return sum
}

// summarizeMultiCommitKind picks the letter of an aggregated row. A single
// source keeps its own kind; otherwise the newest change decides a file that no
// longer exists, a rename is called out even when the file was modified later,
// and everything else reads as a modification.
func summarizeMultiCommitKind(sources []MultiCommitSource) domain.ChangeKind {
	if len(sources) == 0 {
		return domain.ChangeModified
	}
	if len(sources) == 1 {
		return sources[0].Change.Kind
	}
	newest := sources[0].Change.Kind
	if newest == domain.ChangeDeleted {
		return domain.ChangeDeleted
	}
	for _, s := range sources {
		if s.Change.Kind == domain.ChangeRenamed {
			return domain.ChangeRenamed
		}
	}
	if newest == domain.ChangeAdded {
		return domain.ChangeAdded
	}
	return domain.ChangeModified
}
