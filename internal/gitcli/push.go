package gitcli

import (
	"context"

	"viff/internal/domain"
)

// PushRef pushes src (a revision such as HEAD or a commit hash) to
// refs/heads/branch on remote. It is the shared primitive behind pushing an
// older commit and pushing HEAD under another branch name; src is never
// rewritten, so a non-fast-forward update is left for Git to reject.
func (r *Runner) PushRef(ctx context.Context, repo domain.Repo, remote, src, branch string, forceWithLease bool, progress func(string)) error {
	if src == "" {
		src = "HEAD"
	}
	args := []string{"push", "--progress"}
	if forceWithLease {
		// The lease compares against the remote-tracking ref, so a fetch that
		// has not happened yet makes the push fail instead of losing commits.
		args = append(args, "--force-with-lease")
	}
	args = append(args, remote, src+":refs/heads/"+branch)
	_, err := r.RunProgress(ctx, repo.Root, progress, args...)
	return err
}

// DeleteRemoteTag deletes tag from remote. The local tag is not touched;
// deleting it is a separate, local operation.
func (r *Runner) DeleteRemoteTag(ctx context.Context, repo domain.Repo, remote, tag string, progress func(string)) error {
	_, err := r.RunProgress(ctx, repo.Root, progress, "push", "--progress", remote, ":refs/tags/"+tag)
	return err
}
