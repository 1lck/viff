package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"viff/internal/domain"
)

// Editing one commit in the middle of history is a small interactive rebase:
// the todo lists the commits from the target on, the target's verb is changed,
// and the commits after it are replayed. The rules below keep that safe:
//
//   - a Git operation in progress owns the repository, so do nothing;
//   - a merge commit cannot be edited, because a plain interactive rebase
//     would flatten it and every other merge in the replayed range;
//   - a root commit has no parent to replay onto;
//   - any failure aborts the rebase, so the branch is left as it was.
var (
	// ErrRewriteOperation reports an operation that must finish first.
	ErrRewriteOperation = errors.New("a Git operation is in progress; finish or abort it before editing history")
	// ErrRewriteRoot reports the root commit, which has no parent to replay
	// onto; only "amend" applies to it.
	ErrRewriteRoot = errors.New("the first commit of the repository cannot be edited from the log; amend it instead")
	// ErrRewriteMerge reports a merge commit in the replayed range.
	ErrRewriteMerge = errors.New("merge commits cannot be edited this way: an interactive rebase would flatten them")
	// ErrRewriteUnrelated reports a commit that is not on the current branch.
	ErrRewriteUnrelated = errors.New("the commit is not on the current branch")
	// ErrRewriteMessage reports an empty commit message.
	ErrRewriteMessage = errors.New("the commit message cannot be empty")
	// ErrRewriteAbortLeft reports a failed edit whose rebase is still in
	// progress: the repository needs `git rebase --abort` by hand.
	ErrRewriteAbortLeft = errors.New("the rebase could not be aborted")
)

// RewordCommit replaces one commit's message and keeps its content. The commits
// after it are replayed; local changes are stashed and restored.
func (r *Runner) RewordCommit(ctx context.Context, repo domain.Repo, hash, message string) (Output, error) {
	return r.rewriteHistory(ctx, repo, hash, message, RebaseReword)
}

// SquashCommit folds one commit into its parent and gives the combined commit
// a new message. Both commits' content stays.
func (r *Runner) SquashCommit(ctx context.Context, repo domain.Repo, hash, message string) (Output, error) {
	return r.rewriteHistory(ctx, repo, hash, message, RebaseSquash)
}

// DropCommit removes one commit from the branch; the commits after it are
// replayed without it and its changes disappear from the working tree.
func (r *Runner) DropCommit(ctx context.Context, repo domain.Repo, hash string) (Output, error) {
	return r.rewriteHistory(ctx, repo, hash, "", RebaseDrop)
}

func (r *Runner) rewriteHistory(ctx context.Context, repo domain.Repo, hash, message string, action RebaseAction) (Output, error) {
	if r.Operation(ctx, repo).Active() {
		return Output{}, ErrRewriteOperation
	}
	if action != RebaseDrop && strings.TrimSpace(message) == "" {
		return Output{}, ErrRewriteMessage
	}
	full, err := r.ResolveRef(ctx, repo, hash)
	if err != nil || full == "" {
		return Output{}, fmt.Errorf("%s is not a commit", domain.ShortHash(hash))
	}
	if !r.IsAncestor(ctx, repo, full, "HEAD") {
		return Output{}, ErrRewriteUnrelated
	}
	parents, err := r.commitParents(ctx, repo, full)
	if err != nil {
		return Output{}, err
	}
	switch {
	case len(parents) == 0:
		return Output{}, ErrRewriteRoot
	case len(parents) > 1:
		return Output{}, ErrRewriteMerge
	}

	// A reword or drop replays the target and what follows. A squash needs the
	// parent in the same range, so it starts one commit earlier.
	from := full
	if action == RebaseSquash {
		grandparents, err := r.commitParents(ctx, repo, parents[0])
		if err != nil {
			return Output{}, err
		}
		switch {
		case len(grandparents) == 0:
			// The parent is the root commit: --root brings it into the range.
			from = parents[0]
		case len(grandparents) > 1:
			return Output{}, ErrRewriteMerge
		default:
			from = grandparents[0]
		}
	}

	list, base, err := r.RebaseCommits(ctx, repo, from)
	if err != nil {
		return Output{}, err
	}
	found := false
	for _, c := range list {
		if c.Merge {
			return Output{}, ErrRewriteMerge
		}
		if c.Hash == full {
			found = true
		}
	}
	if !found {
		return Output{}, ErrRewriteUnrelated
	}

	var todo strings.Builder
	for _, c := range list {
		verb := RebasePick
		if c.Hash == full {
			verb = action
		}
		fmt.Fprintf(&todo, "%s %s\n", verb, c.Hash)
	}

	dir := filepath.Join(repo.MetaDir, "viff-rewrite")
	if err := os.RemoveAll(dir); err != nil { // files of an earlier edit
		return Output{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Output{}, err
	}
	defer os.RemoveAll(dir)
	todoPath := filepath.Join(dir, "todo")
	if err := os.WriteFile(todoPath, []byte(todo.String()), 0o644); err != nil {
		return Output{}, err
	}
	todoArg, err := toGitPath(repo.Root, todoPath)
	if err != nil {
		return Output{}, err
	}
	args := []string{
		// The todo and message files are written through the Windows identity
		// domain; the editors run in Git's file system, so the paths they copy
		// from have to be spelled for that side (toGitPath; wsl.go).
		"-c", "sequence.editor=cp " + shellQuote(filepath.ToSlash(todoArg)),
		"-c", "rebase.missingCommitsCheck=ignore",
		"-c", "commit.cleanup=strip",
	}
	switch action {
	case RebaseReword, RebaseSquash:
		// The message editor is a copy of our file: Git reads it as the new
		// message of the reworded (or squashed) commit.
		msgPath := filepath.Join(dir, "message")
		if err := os.WriteFile(msgPath, []byte(strings.TrimRight(message, "\n")+"\n"), 0o644); err != nil {
			return Output{}, err
		}
		msgArg, err := toGitPath(repo.Root, msgPath)
		if err != nil {
			return Output{}, err
		}
		args = append(args, "-c", "core.editor=cp "+shellQuote(filepath.ToSlash(msgArg)))
	default:
		args = append(args, "-c", "core.editor=true")
	}
	// --empty=keep preserves a later commit whose change becomes empty once the
	// target is gone; dropping one commit must not silently drop others.
	args = append(args, "rebase", "-i", "--autostash", "--no-autosquash", "--empty=keep")
	if base == "" {
		args = append(args, "--root")
	} else {
		args = append(args, base)
	}

	out, err := r.Run(ctx, repo.Root, args...)
	if err != nil {
		if abortErr := r.abortRebase(repo); abortErr != nil {
			// The branch is not back where it started: say so instead of
			// letting the original conflict look like a clean abort.
			return Output{Stdout: string(out)}, fmt.Errorf("%w (%v); the edit failed with: %v", ErrRewriteAbortLeft, abortErr, err)
		}
		return Output{Stdout: string(out)}, err
	}
	return Output{Stdout: string(out)}, nil
}

// abortRebase undoes a stopped rebase so a failed edit leaves the branch as it
// was. It reports an error only when the rebase is still in progress after the
// abort, which the caller must surface: the repository needs attention then.
// The context is fresh: a cancelled task must still clean up after itself.
func (r *Runner) abortRebase(repo domain.Repo) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if !r.Operation(ctx, repo).Active() {
		return nil
	}
	if _, err := r.Run(ctx, repo.Root, "-c", "core.editor=true", "rebase", "--abort"); err != nil {
		return err
	}
	if op := r.Operation(ctx, repo); op.Active() {
		return fmt.Errorf("%s is still in progress", op.Kind.Title())
	}
	return nil
}

// commitParents lists a commit's parents, oldest line of rev-list.
func (r *Runner) commitParents(ctx context.Context, repo domain.Repo, hash string) ([]string, error) {
	out, err := r.Run(ctx, repo.Root, "rev-list", "--parents", "-n", "1", hash, "--")
	if err != nil {
		return nil, err
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return nil, fmt.Errorf("%s is not a commit", domain.ShortHash(hash))
	}
	return f[1:], nil
}
