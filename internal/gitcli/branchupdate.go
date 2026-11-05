package gitcli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// ErrBranchCheckedOut reports an update of a branch that a worktree has
// checked out. Moving the ref would leave that checkout's HEAD behind its
// index, so the caller has to update the branch in that worktree instead.
var ErrBranchCheckedOut = errors.New("branch is checked out in a worktree")

// ErrBranchDiverged reports an update that is not a fast-forward. The local
// branch keeps its commits, so nothing was written.
var ErrBranchDiverged = errors.New("branch has diverged from the remote")

// BranchUpdate is shared with every backend and lives in internal/vcs.
type BranchUpdate = vcs.BranchUpdate

// UpdateBranch fast-forwards the local branch from remote's same-named
// branch. The remote branch is fetched into its remote-tracking ref first;
// only when that ref is a descendant of the local branch does the local ref
// move, and update-ref checks the old value so a concurrent change aborts
// instead of being overwritten. A checked-out branch and a diverged branch
// are refused without writing anything.
func (r *Runner) UpdateBranch(ctx context.Context, repo domain.Repo, remote, branch string, progress func(string)) (BranchUpdate, error) {
	res := BranchUpdate{Branch: branch}
	if remote == "" {
		return res, errors.New("no remote to update from")
	}
	if strings.TrimSpace(branch) == "" {
		return res, errors.New("no branch to update")
	}
	old, err := r.revParse(ctx, repo.Root, "refs/heads/"+branch)
	if err != nil {
		return res, fmt.Errorf("no local branch %q", branch)
	}
	res.Old = old

	worktrees, err := r.Worktrees(ctx, repo)
	if err != nil {
		return res, err
	}
	for _, wt := range worktrees {
		if wt.Bare || wt.Prunable || wt.Branch != branch {
			continue
		}
		return res, fmt.Errorf("%w: %s", ErrBranchCheckedOut, wt.Path)
	}

	tracking := "refs/remotes/" + remote + "/" + branch
	// The tracking ref is a mirror of the remote, so it may be rewritten; the
	// local branch is only moved after the ancestor check below.
	if _, err := r.RunProgress(ctx, repo.Root, progress, "fetch", "--progress", remote, "+refs/heads/"+branch+":"+tracking); err != nil {
		return res, err
	}
	tip, err := r.revParse(ctx, repo.Root, tracking)
	if err != nil {
		return res, err
	}
	if tip == old {
		res.New = old
		res.UpToDate = true
		return res, nil
	}
	// Moving only a descendant keeps the branch's own commits. When the local
	// branch already contains the remote tip there is nothing to fetch.
	behind, err := r.isAncestor(ctx, repo.Root, old, tip)
	if err != nil {
		return res, err
	}
	if !behind {
		ahead, err := r.isAncestor(ctx, repo.Root, tip, old)
		if err != nil {
			return res, err
		}
		if ahead {
			res.New = old
			res.UpToDate = true
			return res, nil
		}
		return res, fmt.Errorf("%w: %s is not a descendant of %s", ErrBranchDiverged, tracking, branch)
	}
	if _, err := r.Run(ctx, repo.Root, "update-ref", "-m", "viff: update from "+remote, "refs/heads/"+branch, tip, old); err != nil {
		return res, err
	}
	gained := 0
	if out, err := r.Run(ctx, repo.Root, "rev-list", "--count", old+".."+tip); err == nil {
		gained, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	}
	res.New = tip
	res.Commits = gained
	return res, nil
}

// IsShallow reports whether the repository has truncated history.
func (r *Runner) IsShallow(ctx context.Context, repo domain.Repo) (bool, error) {
	out, err := r.Run(ctx, repo.Root, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// Unshallow fetches the history a shallow clone left out. Git needs a remote
// for it; the error is passed through so the user sees why it failed.
func (r *Runner) Unshallow(ctx context.Context, repo domain.Repo, progress func(string)) error {
	_, err := r.RunProgress(ctx, repo.Root, progress, "fetch", "--unshallow", "--progress")
	return err
}

func (r *Runner) revParse(ctx context.Context, dir, rev string) (string, error) {
	out, err := r.Run(ctx, dir, "rev-parse", "--verify", "--quiet", rev)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// isAncestor reports whether a is reachable from b. Git exits 1 when it is
// not; every other failure is a real error and must not be read as "no".
func (r *Runner) isAncestor(ctx context.Context, dir, a, b string) (bool, error) {
	_, err := r.Run(ctx, dir, "merge-base", "--is-ancestor", a, b)
	if err == nil {
		return true, nil
	}
	var ge *Error
	if errors.As(err, &ge) && ge.ExitCode == 1 {
		return false, nil
	}
	return false, err
}
