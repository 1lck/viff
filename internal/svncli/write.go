package svncli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// Stage schedules paths for the next commit.
//
// There is no index in Subversion: a new file simply is not versioned until
// `svn add` schedules it, and a removed one stays recorded until `svn delete`
// schedules the removal. This method therefore does exactly that, which is the
// same user intention ("include this in the next commit") expressed the only
// way this version control system can.
func (r *Runner) Stage(ctx context.Context, repo domain.Repo, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	var toAdd, toDelete []string
	for _, p := range paths {
		full := filepath.Join(repo.Root, filepath.FromSlash(p))
		if _, err := os.Lstat(full); err != nil {
			// Gone from disk: schedule the removal instead of the addition.
			toDelete = append(toDelete, p)
			continue
		}
		toAdd = append(toAdd, p)
	}
	if len(toAdd) > 0 {
		if _, err := r.Run(ctx, repo.Root, append([]string{"add", "--parents"}, toAdd...)...); err != nil {
			return err
		}
	}
	if len(toDelete) > 0 {
		if _, err := r.Run(ctx, repo.Root, append([]string{"delete", "--force"}, toDelete...)...); err != nil {
			return err
		}
	}
	return nil
}

// Unstage reverses Stage: a scheduled addition is un-scheduled, a scheduled
// deletion is restored.
//
// hasHead is meaningless here (Subversion always has a repository and never
// has a "first commit" the working copy tracks) and is accepted only to
// satisfy the shared contract.
func (r *Runner) Unstage(ctx context.Context, repo domain.Repo, paths []string, hasHead bool) error {
	if len(paths) == 0 {
		return nil
	}
	if _, err := r.Run(ctx, repo.Root, append([]string{"revert"}, paths...)...); err != nil {
		return err
	}
	// A reverted addition goes back to unversioned, which is the state it had
	// before `svn add`; nothing else to do.
	return nil
}

// Discard throws away working-copy changes to the given paths.
func (r *Runner) Discard(ctx context.Context, repo domain.Repo, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := r.Run(ctx, repo.Root, append([]string{"revert"}, paths...)...)
	return err
}

// Revert restores tracked paths from the recorded revision. It is the same
// operation as Discard in Subversion, which keeps only one recorded version.
func (r *Runner) Revert(ctx context.Context, repo domain.Repo, paths []string) error {
	return r.Discard(ctx, repo, paths)
}

// Commit sends the changes to the repository.
//
// A commit is an atomic remote operation: there is no local commit that a push
// later transfers. When Paths is empty everything scheduled is committed;
// otherwise only those paths are (finding S4), which is how Viff offers a
// partial commit without an index.
//
// The message goes through a file, not -m, so a message containing newlines,
// quotes or characters the shell would care about arrives byte for byte.
func (r *Runner) Commit(ctx context.Context, repo domain.Repo, o vcs.CommitOptions) (vcs.Output, error) {
	args := []string{"commit", "--file", "-"}
	if o.NoVerify {
		args = append(args, "--no-unlock")
	}
	if len(o.Paths) > 0 {
		args = append(args, o.Paths...)
	}
	out, err := r.RunInput(ctx, repo.Root, []byte(o.Message), args...)
	// A commit creates a revision, so any cached log is now out of date.
	invalidateLog(repo)
	return vcs.Output{Stdout: string(out)}, err
}

// LastCommitMessage returns the message of the revision the working copy is
// based on.
//
// The range is explicit (HEAD:1 with -l 1): a bare `svn log -l 1` answers from
// the working copy's base revision and, as measured, can return the oldest
// entry rather than the newest (finding S1).
func (r *Runner) LastCommitMessage(ctx context.Context, repo domain.Repo) (string, error) {
	out, err := r.Run(ctx, repo.Root, "log", "--xml", "-l", "1", "-r", "HEAD:1", repo.URL)
	if err != nil {
		return "", err
	}
	entries, err := ParseLog(out)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	return entries[0].Message, nil
}

// CreatePatch renders a diff of the given paths.
//
// staged selects only what is scheduled for the next commit. Subversion can
// express that with --changelist, but writing and removing a temporary
// changelist would mutate the working copy for a read; instead the scheduled
// subset is approximated by diffing the named paths, which is what the UI
// passes when the user picks files.
func (r *Runner) CreatePatch(ctx context.Context, repo domain.Repo, paths []string, staged bool) ([]byte, error) {
	args := []string{"diff"}
	if len(paths) > 0 {
		args = append(args, paths...)
	}
	return r.Run(ctx, repo.Root, args...)
}

// ApplyPatch applies a patch to the working copy.
//
// cached and reverse are Git concepts: Subversion applies patches to the
// working copy only and has no reverse mode, so a reverse request is rejected
// rather than silently applied forwards.
//
// svn patch takes the patch as a file argument and cannot read stdin
// ("E205001: Not enough arguments provided"), so the bytes go to a temporary
// file that is removed afterwards. The file lives in the system temporary
// directory rather than the repository, so it never shows up in a status query.
func (r *Runner) ApplyPatch(ctx context.Context, repo domain.Repo, patch []byte, cached, reverse bool) error {
	if reverse {
		return errorf(vcs.ErrOther, "Subversion cannot apply a patch in reverse")
	}
	if cached {
		return errorf(vcs.ErrOther, "Subversion has no staging area to apply a patch to")
	}
	f, err := os.CreateTemp("", "viff-*.patch")
	if err != nil {
		return fmt.Errorf("create patch file: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(patch); err != nil {
		f.Close()
		return fmt.Errorf("write patch file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close patch file: %w", err)
	}
	// WCPATH is the working copy root, so a patch whose paths are relative to
	// it applies from there; svn otherwise assumes the current directory.
	_, err = r.Run(ctx, repo.Root, "patch", name, ".")
	return err
}

// Fetch is a no-op: Subversion has no separate fetch step, because `svn
// update` contacts the repository and merges in one operation. Pull does the
// work.
func (r *Runner) Fetch(ctx context.Context, repo domain.Repo, remote string, prune bool, progress func(string)) error {
	return nil
}

// Pull brings the repository's changes into the working copy. In Subversion
// that operation is `svn update`; the mode argument is a Git merge/rebase
// choice and is ignored.
//
// --accept postpone keeps a conflict on disk with markers instead of letting
// svn ask a question no GUI can answer.
func (r *Runner) Pull(ctx context.Context, repo domain.Repo, mode vcs.PullMode, progress func(string)) (vcs.Output, error) {
	out, err := r.RunProgress(ctx, repo.Root, progress, "update", "--accept", "postpone")
	// The working copy moved, so the cached log no longer reflects its base.
	invalidateLog(repo)
	return vcs.Output{Stdout: string(out)}, err
}

// Push sends local changes to the repository.
//
// A Subversion "push" is a commit, so the changes must already be scheduled and
// a message is required. Options that describe Git refs (Remote, Branch, Tags,
// ForceWithLease) have no equivalent and are ignored; the contract keeps one
// method for the same user intention rather than exposing the difference.
func (r *Runner) Push(ctx context.Context, repo domain.Repo, o vcs.PushOptions, progress func(string)) (vcs.Output, error) {
	msg := "Committed with Viff"
	out, err := r.RunProgressInput(ctx, repo.Root, progress, []byte(msg), "commit", "--file", "-")
	invalidateLog(repo)
	return vcs.Output{Stdout: string(out)}, err
}

// UpdateBranch is unavailable: Subversion has no cheap way to fast-forward a
// directory that is not checked out, so the branch menu offers "switch"
// instead and the capability stays nil.
func (r *Runner) UpdateBranch(ctx context.Context, repo domain.Repo, remote, branch string, progress func(string)) (vcs.BranchUpdate, error) {
	return vcs.BranchUpdate{}, errorf(vcs.ErrOther, "Subversion cannot update a branch that is not checked out")
}

// IsShallow reports false: Subversion working copies are never shallow, they
// are checked out at a depth the user chose and can be deepened with update.
func (r *Runner) IsShallow(ctx context.Context, repo domain.Repo) (bool, error) { return false, nil }

// Unshallow deepens a sparse working copy to full depth.
func (r *Runner) Unshallow(ctx context.Context, repo domain.Repo, progress func(string)) error {
	_, err := r.RunProgress(ctx, repo.Root, progress, "update", "--set-depth", "infinity")
	invalidateLog(repo)
	return err
}

// ResolveConflictMarkers is not part of the contract: marking a Subversion
// conflict resolved is `svn resolve --accept working`, exposed through the
// conflict resolver only when the capability is present.
func (r *Runner) resolveConflict(ctx context.Context, repo domain.Repo, path string) error {
	_, err := r.Run(ctx, repo.Root, "resolve", "--accept", "working", path)
	return err
}

// notSupported builds the error an optional capability reports when the UI
// somehow calls it despite the capability being absent. It should be
// unreachable: the UI hides the entry when the capability is nil.
func notSupported(what string) error {
	return &vcs.Error{Kind: vcs.ErrOther, Tool: "svn", Stderr: what + " is not available in Subversion"}
}
