package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// Output, CommitOptions, PullMode, PushOptions and ResetMode are shared with
// every backend and live in internal/vcs. They stay aliased here so existing
// call sites keep compiling.
type (
	// Output is what a write command printed.
	Output = vcs.Output
	// CommitOptions configures a commit.
	CommitOptions = vcs.CommitOptions
	// PullMode decides how pull integrates the upstream.
	PullMode = vcs.PullMode
	// PushOptions configures a push.
	PushOptions = vcs.PushOptions
	// ResetMode is the mode of a reset.
	ResetMode = vcs.ResetMode
)

const (
	PullMerge           = vcs.PullMerge
	PullRebase          = vcs.PullRebase
	PullFastForwardOnly = vcs.PullFastForwardOnly

	ResetSoft  = vcs.ResetSoft
	ResetMixed = vcs.ResetMixed
	ResetHard  = vcs.ResetHard
	ResetKeep  = vcs.ResetKeep
)

func (r *Runner) write(ctx context.Context, repo domain.Repo, args ...string) error {
	if err := readOnlyRemote(repo, "write"); err != nil {
		return err
	}
	_, err := r.Run(ctx, repo.Root, args...)
	return err
}

// Stage adds paths (including deletions) to the index.
func (r *Runner) Stage(ctx context.Context, repo domain.Repo, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return r.writePaths(ctx, repo, []string{"add", "-A"}, paths)
}

// Unstage resets paths in the index to HEAD (or removes them before the first
// commit).
func (r *Runner) Unstage(ctx context.Context, repo domain.Repo, paths []string, hasHead bool) error {
	if len(paths) == 0 {
		return nil
	}
	if !hasHead {
		return r.writePaths(ctx, repo, []string{"rm", "--cached", "-r", "-q"}, paths)
	}
	return r.writePaths(ctx, repo, []string{"restore", "--staged"}, paths)
}

// Discard restores tracked paths in the working tree from the index.
func (r *Runner) Discard(ctx context.Context, repo domain.Repo, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return r.writePaths(ctx, repo, []string{"restore", "--worktree"}, paths)
}

// Revert restores tracked paths in both index and working tree from HEAD.
func (r *Runner) Revert(ctx context.Context, repo domain.Repo, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return r.writePaths(ctx, repo, []string{"restore", "--staged", "--worktree", "--source=HEAD"}, paths)
}

// writePaths passes paths through a pathspec file on stdin, so any number of
// paths with any characters reach Git unchanged and literal.
func (r *Runner) writePaths(ctx context.Context, repo domain.Repo, args []string, paths []string) error {
	if err := readOnlyRemote(repo, "write"); err != nil {
		return err
	}
	input := strings.Join(paths, "\x00")
	full := append([]string{"--literal-pathspecs"}, args...)
	full = append(full, "--pathspec-from-file=-", "--pathspec-file-nul")
	_, err := r.RunInput(ctx, repo.Root, []byte(input), full...)
	return err
}

// Commit records the index, or only Paths' working-tree content.
//
// With Paths set, `git commit --only` builds the commit from HEAD's tree plus
// the working-tree version of exactly those paths: staged changes to other
// paths stay staged and out of the commit. Git rejects a pathspec it cannot
// resolve, so an untracked checked path is first recorded as intent-to-add.
// A failed or cancelled commit removes those entries again, leaving the file
// untracked; the index then looks exactly as it did before the attempt.
func (r *Runner) Commit(ctx context.Context, repo domain.Repo, o CommitOptions) (Output, error) {
	args := commitArgs(o)
	paths := uniquePaths(o.Paths)
	if len(paths) == 0 {
		out, err := r.RunInput(ctx, repo.Root, []byte(o.Message), args...)
		return Output{Stdout: string(out)}, err
	}
	return r.commitPaths(ctx, repo, o, paths, args)
}

// commitArgs is the option set both commit paths share.
func commitArgs(o CommitOptions) []string {
	args := []string{"commit", "--file=-", "--cleanup=strip"}
	if o.Amend {
		args = append(args, "--amend")
	}
	if o.SignOff {
		args = append(args, "--signoff")
	}
	if o.NoVerify {
		args = append(args, "--no-verify")
	}
	return args
}

// commitPaths runs the partial commit. The commit message keeps stdin (it can
// be long and must not appear in argv), so the pathspec goes through a NUL
// file instead; that keeps any number of paths with spaces, quotes or leading
// dashes literal.
func (r *Runner) commitPaths(ctx context.Context, repo domain.Repo, o CommitOptions, paths, args []string) (Output, error) {
	wanted := make(map[string]bool, len(paths))
	for _, p := range paths {
		wanted[p] = true
	}
	status, err := r.Status(ctx, repo)
	if err != nil {
		return Output{}, err
	}
	var added []string
	for _, e := range status.Entries {
		if e.Untracked() && wanted[e.Path] {
			added = append(added, e.Path)
		}
	}
	if len(added) > 0 {
		if err := r.writePaths(ctx, repo, []string{"add", "--intent-to-add"}, added); err != nil {
			// A path can disappear between the status query and the add. Remove
			// whatever the add did record before the path list is committed.
			if cleanupErr := r.dropIntentToAdd(ctx, repo, added); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("restore intent-to-add paths: %w", cleanupErr))
			}
			return Output{}, err
		}
	}
	spec, err := os.CreateTemp("", "viff-commit-paths-")
	if err != nil {
		err = fmt.Errorf("create pathspec file: %w", err)
		if cleanupErr := r.dropIntentToAdd(ctx, repo, added); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("restore intent-to-add paths: %w", cleanupErr))
		}
		return Output{}, err
	}
	name := spec.Name()
	defer os.Remove(name)
	_, writeErr := spec.WriteString(strings.Join(paths, "\x00"))
	if closeErr := spec.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		writeErr = fmt.Errorf("write pathspec file: %w", writeErr)
		if cleanupErr := r.dropIntentToAdd(ctx, repo, added); cleanupErr != nil {
			writeErr = errors.Join(writeErr, fmt.Errorf("restore intent-to-add paths: %w", cleanupErr))
		}
		return Output{}, writeErr
	}
	full := append([]string{"--literal-pathspecs"}, args...)
	full = append(full, "--only", "--pathspec-from-file="+name, "--pathspec-file-nul")
	out, err := r.RunInput(ctx, repo.Root, []byte(o.Message), full...)
	if err != nil {
		// Replace the entries the failed commit needed with the untracked
		// state the user saw. A cancelled commit still runs the cleanup on a
		// detached context: cancelling a write is not a rollback.
		if cleanupErr := r.dropIntentToAdd(ctx, repo, added); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("restore intent-to-add paths: %w", cleanupErr))
		}
	}
	return Output{Stdout: string(out)}, err
}

// dropIntentToAdd removes the index entries commitPaths added for untracked
// paths, so they are untracked again. It uses a context detached from a
// cancelled commit; the timeout bounds a repository that stopped answering.
func (r *Runner) dropIntentToAdd(ctx context.Context, repo domain.Repo, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return r.writePaths(cleanup, repo, []string{"rm", "--cached", "-q", "--ignore-unmatch"}, paths)
}

// uniquePaths removes empty and repeated paths while keeping the caller's
// order, so one file checked through two rows is committed once.
func uniquePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// LastCommitMessage returns HEAD's full message, for amend.
func (r *Runner) LastCommitMessage(ctx context.Context, repo domain.Repo) (string, error) {
	out, err := r.Run(ctx, repo.Root, "log", "-1", "--format=%B", "HEAD", "--")
	return strings.TrimRight(string(out), "\n"), err
}

// Checkout switches to a branch, or detaches at a revision.
func (r *Runner) Checkout(ctx context.Context, repo domain.Repo, ref domain.Ref) error {
	switch ref.Kind {
	case domain.RefLocal:
		return r.write(ctx, repo, "switch", "--no-guess", ref.Short)
	case domain.RefRemote:
		// Checking out a remote branch creates a tracking local branch, as
		// IntelliJ does; an existing local branch of that name is switched to.
		return r.write(ctx, repo, "switch", "--track", "-c", ref.BranchName(), ref.Short)
	}
	return r.write(ctx, repo, "switch", "--detach", ref.Target)
}

// CheckoutRevision detaches HEAD at rev.
func (r *Runner) CheckoutRevision(ctx context.Context, repo domain.Repo, rev string) error {
	return r.write(ctx, repo, "switch", "--detach", rev)
}

// CreateBranch creates name at start, optionally switching to it.
func (r *Runner) CreateBranch(ctx context.Context, repo domain.Repo, name, start string, checkout, force bool) error {
	if checkout {
		flag := "-c"
		if force {
			flag = "-C"
		}
		args := []string{"switch", "--no-track", flag, name}
		if start != "" {
			args = append(args, start)
		}
		return r.write(ctx, repo, args...)
	}
	args := []string{"branch", "--no-track"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, name)
	if start != "" {
		args = append(args, start)
	}
	return r.write(ctx, repo, args...)
}

// RenameBranch renames a local branch.
func (r *Runner) RenameBranch(ctx context.Context, repo domain.Repo, old, name string) error {
	return r.write(ctx, repo, "branch", "-m", old, name)
}

// DeleteBranch deletes a local branch; force deletes it even if unmerged.
func (r *Runner) DeleteBranch(ctx context.Context, repo domain.Repo, name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	return r.write(ctx, repo, "branch", flag, name)
}

// DeleteRemoteBranch deletes a branch on its remote.
func (r *Runner) DeleteRemoteBranch(ctx context.Context, repo domain.Repo, remote, branch string, progress func(string)) error {
	_, err := r.RunProgress(ctx, repo.Root, progress, "push", "--porcelain", remote, "--delete", branch)
	return err
}

// SetUpstream sets the upstream of a local branch.
func (r *Runner) SetUpstream(ctx context.Context, repo domain.Repo, branch, upstream string) error {
	if upstream == "" {
		return r.write(ctx, repo, "branch", "--unset-upstream", branch)
	}
	return r.write(ctx, repo, "branch", "--set-upstream-to="+upstream, branch)
}

// CreateTag creates a lightweight tag, or an annotated one with a message.
func (r *Runner) CreateTag(ctx context.Context, repo domain.Repo, name, rev, message string) error {
	if message != "" {
		_, err := r.RunInput(ctx, repo.Root, []byte(message), "tag", "-a", "-F", "-", name, rev)
		return err
	}
	return r.write(ctx, repo, "tag", name, rev)
}

// DeleteTag deletes a local tag.
func (r *Runner) DeleteTag(ctx context.Context, repo domain.Repo, name string) error {
	return r.write(ctx, repo, "tag", "-d", name)
}

// Fetch fetches all remotes (or one), optionally pruning.
func (r *Runner) Fetch(ctx context.Context, repo domain.Repo, remote string, prune bool, progress func(string)) error {
	args := []string{"fetch", "--progress"}
	if prune {
		args = append(args, "--prune")
	}
	if remote == "" {
		args = append(args, "--all")
	} else {
		args = append(args, remote)
	}
	_, err := r.RunProgress(ctx, repo.Root, progress, args...)
	return err
}

// Pull fetches and integrates the upstream of the current branch.
func (r *Runner) Pull(ctx context.Context, repo domain.Repo, mode PullMode, progress func(string)) (Output, error) {
	args := []string{"pull", "--progress", "--no-edit"}
	switch mode {
	case PullRebase:
		args = append(args, "--rebase")
	case PullFastForwardOnly:
		args = append(args, "--ff-only")
	default:
		args = append(args, "--no-rebase")
	}
	out, err := r.RunProgress(ctx, repo.Root, progress, args...)
	return Output{Stdout: string(out)}, err
}

// Push pushes a branch.
func (r *Runner) Push(ctx context.Context, repo domain.Repo, o PushOptions, progress func(string)) (Output, error) {
	args := []string{"push", "--progress", "--porcelain"}
	if o.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if o.ForceWithLease {
		args = append(args, "--force-with-lease")
	}
	if o.Tags {
		args = append(args, "--follow-tags")
	}
	if o.NoVerify {
		args = append(args, "--no-verify")
	}
	if o.Remote != "" {
		args = append(args, o.Remote)
		if o.Branch != "" {
			dst := o.RemoteName
			if dst == "" {
				dst = o.Branch
			}
			args = append(args, "refs/heads/"+o.Branch+":refs/heads/"+dst)
		}
	}
	out, err := r.RunProgress(ctx, repo.Root, progress, args...)
	return Output{Stdout: string(out)}, err
}

// PushTag pushes one tag to a remote.
func (r *Runner) PushTag(ctx context.Context, repo domain.Repo, remote, tag string, progress func(string)) error {
	_, err := r.RunProgress(ctx, repo.Root, progress, "push", "--progress", remote, "refs/tags/"+tag)
	return err
}

// AddRemote adds a remote.
func (r *Runner) AddRemote(ctx context.Context, repo domain.Repo, name, url string) error {
	return r.write(ctx, repo, "remote", "add", name, url)
}

// RemoveRemote removes a remote.
func (r *Runner) RemoveRemote(ctx context.Context, repo domain.Repo, name string) error {
	return r.write(ctx, repo, "remote", "remove", name)
}

// SetRemoteURL changes a remote's URL.
func (r *Runner) SetRemoteURL(ctx context.Context, repo domain.Repo, name, url string) error {
	return r.write(ctx, repo, "remote", "set-url", name, url)
}

// Merge merges rev into the current branch.
func (r *Runner) Merge(ctx context.Context, repo domain.Repo, rev string, noFF, squash bool) (Output, error) {
	args := []string{"merge", "--no-edit"}
	if noFF {
		args = append(args, "--no-ff")
	}
	if squash {
		args = append(args, "--squash")
	}
	out, err := r.Run(ctx, repo.Root, append(args, rev)...)
	return Output{Stdout: string(out)}, err
}

// Rebase rebases the current branch onto rev.
func (r *Runner) Rebase(ctx context.Context, repo domain.Repo, onto string) (Output, error) {
	out, err := r.Run(ctx, repo.Root, "-c", "core.editor=true", "rebase", onto)
	return Output{Stdout: string(out)}, err
}

// CherryPick applies commits onto HEAD.
func (r *Runner) CherryPick(ctx context.Context, repo domain.Repo, hashes []string) (Output, error) {
	out, err := r.Run(ctx, repo.Root, append([]string{"cherry-pick", "--allow-empty"}, hashes...)...)
	return Output{Stdout: string(out)}, err
}

// RevertCommits creates commits reverting hashes.
func (r *Runner) RevertCommits(ctx context.Context, repo domain.Repo, hashes []string) (Output, error) {
	out, err := r.Run(ctx, repo.Root, append([]string{"revert", "--no-edit"}, hashes...)...)
	return Output{Stdout: string(out)}, err
}

// Reset moves the current branch to rev.
func (r *Runner) Reset(ctx context.Context, repo domain.Repo, mode ResetMode, rev string) error {
	return r.write(ctx, repo, "reset", "-q", string(mode), rev, "--")
}

// Continue, Skip and Abort drive an in-progress operation. The editor is
// replaced by "true" so Git keeps its prepared message instead of waiting for
// a terminal editor the GUI cannot show.
func (r *Runner) OperationCommand(ctx context.Context, repo domain.Repo, op domain.OperationKind, action string) (Output, error) {
	var args []string
	switch op {
	case domain.OpMerge:
		args = []string{"merge", "--" + action}
	case domain.OpRebase:
		args = []string{"rebase", "--" + action}
	case domain.OpCherryPick:
		args = []string{"cherry-pick", "--" + action}
	case domain.OpRevert:
		args = []string{"revert", "--" + action}
	case domain.OpApplyMailbox:
		args = []string{"am", "--" + action}
	case domain.OpBisect:
		args = []string{"bisect", "reset"}
	default:
		return Output{}, nil
	}
	if action == "continue" {
		args = append([]string{"-c", "core.editor=true"}, args...)
	}
	out, err := r.Run(ctx, repo.Root, args...)
	return Output{Stdout: string(out)}, err
}

// StashPush stashes local changes.
func (r *Runner) StashPush(ctx context.Context, repo domain.Repo, message string, untracked, keepIndex bool) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	if untracked {
		args = append(args, "--include-untracked")
	}
	if keepIndex {
		args = append(args, "--keep-index")
	}
	return r.write(ctx, repo, args...)
}

// StashApply applies (or pops) a stash entry.
func (r *Runner) StashApply(ctx context.Context, repo domain.Repo, ref string, pop, index bool) (Output, error) {
	verb := "apply"
	if pop {
		verb = "pop"
	}
	args := []string{"stash", verb}
	if index {
		args = append(args, "--index")
	}
	out, err := r.Run(ctx, repo.Root, append(args, ref)...)
	return Output{Stdout: string(out)}, err
}

// StashDrop removes a stash entry.
func (r *Runner) StashDrop(ctx context.Context, repo domain.Repo, ref string) error {
	return r.write(ctx, repo, "stash", "drop", ref)
}

// StashBranch creates a branch from a stash entry.
func (r *Runner) StashBranch(ctx context.Context, repo domain.Repo, branch, ref string) error {
	return r.write(ctx, repo, "stash", "branch", branch, ref)
}

// Init creates a repository in dir.
func (r *Runner) Init(ctx context.Context, dir string) error {
	_, err := r.Run(ctx, dir, "init")
	return err
}

// Clone clones url into dir with progress. dir is the target location in the
// Windows identity domain; for a parent inside a WSL distro it maps to the
// Linux spelling Git runs with (toGitPath; wsl.go).
func (r *Runner) Clone(ctx context.Context, parent, url, dir string, progress func(string)) error {
	target, err := toGitPath(parent, dir)
	if err != nil {
		return err
	}
	_, err = r.RunProgress(ctx, parent, progress, "clone", "--progress", "--", url, target)
	return err
}

// CreatePatch returns a patch for paths (every change when empty): the working
// tree against the index, or the index against HEAD when staged. Paths are
// relative to the repository root.
//
// The output is binary-safe and keeps the a/ and b/ prefixes and the exact
// paths whatever diff.* the user configured, so another checkout can apply it.
func (r *Runner) CreatePatch(ctx context.Context, repo domain.Repo, paths []string, staged bool) ([]byte, error) {
	args := []string{
		"diff", "--no-color", "--binary",
		"--no-ext-diff", "--no-textconv", "--no-relative",
		"--src-prefix=a/", "--dst-prefix=b/",
	}
	if staged {
		args = append(args, "--cached")
	}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	return r.Run(ctx, repo.Root, args...)
}

// ApplyPatch applies a patch to the index (cached) or the working tree,
// optionally in reverse. It checks first so a stale patch never half-applies.
func (r *Runner) ApplyPatch(ctx context.Context, repo domain.Repo, patch []byte, cached, reverse bool) error {
	args := []string{"apply", "--whitespace=nowarn", "--recount"}
	if cached {
		args = append(args, "--cached")
	}
	if reverse {
		args = append(args, "--reverse")
	}
	if _, err := r.RunInput(ctx, repo.Root, patch, append(args, "--check", "-")...); err != nil {
		return err
	}
	_, err := r.RunInput(ctx, repo.Root, patch, append(args, "-")...)
	return err
}

// AddWorktree, RemoveWorktree manage linked worktrees. The path arrives in
// the Windows identity domain and is handed to Git in the spelling its own
// file system understands (toGitPath; wsl.go).
func (r *Runner) AddWorktree(ctx context.Context, repo domain.Repo, path, branch, start string) error {
	target, err := toGitPath(repo.Root, path)
	if err != nil {
		return err
	}
	args := []string{"worktree", "add"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, target)
	if start != "" {
		args = append(args, start)
	}
	return r.write(ctx, repo, args...)
}

// AddWorktreeExisting checks out an existing local branch in a new linked
// worktree. Unlike AddWorktree it never creates or moves a branch, and it
// never forces: the branch must already exist locally, must not be checked
// out in any worktree, and the path must not be occupied. The checks here
// produce a clear error before anything is written; Git repeats them when the
// command runs, so two callers racing still cannot check out one branch twice
// or overwrite an existing directory.
func (r *Runner) AddWorktreeExisting(ctx context.Context, repo domain.Repo, path, branch string) error {
	if strings.TrimSpace(branch) == "" {
		return errors.New("branch name is empty")
	}
	// The branch is passed to Git by name (not as refs/heads/...) so Git
	// attaches the new worktree to the branch; a leading dash would make Git
	// read the name as an option instead.
	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("invalid branch name %q", branch)
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("worktree path is empty")
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Errorf("invalid worktree path %q", path)
	}
	if err := r.checkBranchName(ctx, repo, branch); err != nil {
		return err
	}
	exists, err := r.localBranchExists(ctx, repo, branch)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%q is not an existing local branch", branch)
	}
	worktrees, err := r.Worktrees(ctx, repo)
	if err != nil {
		return err
	}
	for _, wt := range worktrees {
		if wt.Branch == branch {
			return fmt.Errorf("branch %q is already checked out at %s", branch, wt.Path)
		}
	}
	// -- keeps a path with a leading dash literal; Git's own path check then
	// refuses a non-empty directory without --force.
	target, err := toGitPath(repo.Root, path)
	if err != nil {
		return err
	}
	return r.write(ctx, repo, "worktree", "add", "--", target, branch)
}

// checkBranchName rejects names that Git would not read back as the literal
// branch refs/heads/branch. HEAD is the case that matters: refs/heads/HEAD can
// exist, but git resolves the name HEAD to the current commit, so worktree add
// would silently detach at HEAD instead of checking the branch out. Git also
// rejects other reserved and revision-like names here.
func (r *Runner) checkBranchName(ctx context.Context, repo domain.Repo, branch string) error {
	out, err := r.Run(ctx, repo.Root, "check-ref-format", "--branch", branch)
	if err != nil {
		if KindOf(err) == ErrCancelled {
			return err
		}
		return fmt.Errorf("invalid branch name %q", branch)
	}
	// --branch expands @{-n}; anything but the name itself would make Git
	// check out a different ref than the one validated below.
	if got := strings.TrimSpace(string(out)); got != branch {
		return fmt.Errorf("invalid branch name %q", branch)
	}
	return nil
}

// localBranchExists reports whether refs/heads/branch exists. Git exits 1 when
// it does not; every other failure is a real error and must not be read as
// "no".
func (r *Runner) localBranchExists(ctx context.Context, repo domain.Repo, branch string) (bool, error) {
	_, err := r.Run(ctx, repo.Root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	var ge *Error
	if errors.As(err, &ge) && ge.ExitCode == 1 {
		return false, nil
	}
	return false, err
}

func (r *Runner) RemoveWorktree(ctx context.Context, repo domain.Repo, path string, force bool) error {
	target, err := toGitPath(repo.Root, path)
	if err != nil {
		return err
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	return r.write(ctx, repo, append(args, target)...)
}

// IsAncestor reports whether a is an ancestor of b.
func (r *Runner) IsAncestor(ctx context.Context, repo domain.Repo, a, b string) bool {
	_, err := r.Run(ctx, repo.Root, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

// ResolveRef returns the commit a revision points at.
func (r *Runner) ResolveRef(ctx context.Context, repo domain.Repo, rev string) (string, error) {
	out, err := r.Run(ctx, repo.Root, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	return strings.TrimSpace(string(out)), err
}

// IndexBlob returns the blob id recorded in the index for path ("" if none),
// used to detect that the index changed since a patch was built.
func (r *Runner) IndexBlob(ctx context.Context, repo domain.Repo, path string) string {
	out, err := r.Run(ctx, repo.Root, "--literal-pathspecs", "ls-files", "-s", "-z", "--", path)
	if err != nil {
		return ""
	}
	f := strings.Fields(strings.SplitN(string(out), "\t", 2)[0])
	if len(f) >= 2 {
		return f[1]
	}
	return ""
}
