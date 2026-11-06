package gitcli

import (
	"context"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// MultiCommitChanges is shared with every backend and lives in internal/vcs.
type MultiCommitChanges = vcs.MultiCommitChanges

// MultiCommitChanges lists the files each commit changed relative to its first
// parent, the same comparison CommitDetail uses. A merge commit therefore shows
// what it introduced on its first-parent side, and no combined or net diff
// between the selected commits is invented. A root commit is compared with the
// empty tree.
//
// Commits are asked of Git one by one. The result keeps the order of commits,
// so callers can tell which commit each change belongs to; it is not re-sorted.
// On cancellation the loop stops at the next commit instead of running the rest
// of a selection the caller no longer wants; such a partial result must be
// discarded by the caller, as the workspace does.
func (r *Runner) MultiCommitChanges(ctx context.Context, repo domain.Repo, commits []domain.Commit) []MultiCommitChanges {
	out := make([]MultiCommitChanges, 0, len(commits))
	for _, c := range commits {
		if err := ctx.Err(); err != nil {
			return out
		}
		changes, err := r.commitChanges(ctx, repo, c)
		out = append(out, MultiCommitChanges{Commit: c, Changes: changes, Err: err})
	}
	return out
}

// commitChanges reads one commit's first-parent changes with diff-tree. The
// arguments mirror CommitDetail, including the explicit parent pair for a merge
// commit, because diff-tree prints nothing for a merge by default.
func (r *Runner) commitChanges(ctx context.Context, repo domain.Repo, c domain.Commit) ([]domain.Change, error) {
	args := []string{"diff-tree", "-r", "-z", "--no-commit-id", "--name-status", "-M", "--root"}
	if len(c.Parents) > 1 {
		args = append(args, c.Parents[0], c.Hash)
	} else {
		args = append(args, c.Hash)
	}
	out, err := r.Run(ctx, repo.Root, append(args, "--")...)
	if err != nil {
		return nil, err
	}
	return ParseNameStatus(out), nil
}
