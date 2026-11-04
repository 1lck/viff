package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"viff/internal/domain"
)

// RebaseAction is the verb of one interactive-rebase step.
type RebaseAction string

const (
	RebasePick   RebaseAction = "pick"
	RebaseReword RebaseAction = "reword"
	RebaseSquash RebaseAction = "squash"
	RebaseFixup  RebaseAction = "fixup"
	RebaseDrop   RebaseAction = "drop"
)

// RebaseStep is one line of the rebase plan.
type RebaseStep struct {
	Action RebaseAction
	Hash   string
	// Message replaces the commit message for RebaseReword.
	Message string
}

// RebaseCommit is a commit an interactive rebase would replay.
type RebaseCommit struct {
	Hash    string
	Subject string
	Message string
	Author  string
	Merge   bool
}

// ErrRebaseMerges is returned when the range holds merge commits, which a
// plain interactive rebase would silently flatten.
var ErrRebaseMerges = errors.New("the range contains merge commits; interactive rebase would flatten them")

// RebaseCommits lists the commits from from (inclusive) to HEAD, oldest first,
// and the base to rebase onto ("" when from is a root commit). from must be an
// ancestor of HEAD.
func (r *Runner) RebaseCommits(ctx context.Context, repo domain.Repo, from string) ([]RebaseCommit, string, error) {
	if !r.IsAncestor(ctx, repo, from, "HEAD") {
		return nil, "", fmt.Errorf("%s is not on the current branch", domain.ShortHash(from))
	}
	base := ""
	if out, err := r.Run(ctx, repo.Root, "rev-parse", "--verify", "-q", from+"^"); err == nil {
		base = strings.TrimSpace(string(out))
	}
	rangeArg := "HEAD"
	if base != "" {
		rangeArg = base + "..HEAD"
	}
	out, err := r.Run(ctx, repo.Root, "log", "--reverse", "--format=%H%x00%P%x00%an%x00%B%x1e", rangeArg, "--")
	if err != nil {
		return nil, "", err
	}
	var list []RebaseCommit
	for _, rec := range strings.Split(string(out), "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		f := strings.SplitN(rec, "\x00", 4)
		if len(f) < 4 {
			continue
		}
		msg := strings.TrimRight(f[3], "\n")
		subject, _, _ := strings.Cut(msg, "\n")
		list = append(list, RebaseCommit{
			Hash: f[0], Merge: len(strings.Fields(f[1])) > 1, Author: f[2],
			Subject: subject, Message: msg,
		})
	}
	return list, base, nil
}

// ValidateRebasePlan checks a plan before it is handed to Git.
func ValidateRebasePlan(steps []RebaseStep) error {
	kept := false
	for _, s := range steps {
		switch s.Action {
		case RebaseDrop:
			continue
		case RebaseSquash, RebaseFixup:
			if !kept {
				return errors.New("the first kept commit cannot be squashed or fixed up: there is nothing before it to combine with")
			}
		case RebaseReword:
			if strings.TrimSpace(s.Message) == "" {
				return errors.New("a reworded commit needs a message")
			}
		case RebasePick:
		default:
			return fmt.Errorf("unknown rebase action %q", s.Action)
		}
		kept = true
	}
	return nil
}

// InteractiveRebase replays the commits after base following steps. base ""
// rebases from the root. The plan is written to a todo file that Git's
// sequence editor copies in place, so no editor ever opens; a reword is a pick
// followed by an amend with the new message, which keeps the author and runs
// the user's hooks. Local changes are stashed and restored by --autostash.
func (r *Runner) InteractiveRebase(ctx context.Context, repo domain.Repo, base string, steps []RebaseStep) (Output, error) {
	if err := ValidateRebasePlan(steps); err != nil {
		return Output{}, err
	}
	dir := filepath.Join(repo.GitDir, "viff-rebase")
	os.RemoveAll(dir) // files of an earlier, finished or aborted rebase
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Output{}, err
	}
	var todo strings.Builder
	for i, s := range steps {
		switch s.Action {
		case RebaseReword:
			msg := filepath.Join(dir, fmt.Sprintf("message-%d.txt", i))
			if err := os.WriteFile(msg, []byte(strings.TrimRight(s.Message, "\n")+"\n"), 0o644); err != nil {
				return Output{}, err
			}
			fmt.Fprintf(&todo, "pick %s\nexec git commit --amend --allow-empty --quiet -F %s\n", s.Hash, shellQuote(filepath.ToSlash(msg)))
		default:
			fmt.Fprintf(&todo, "%s %s\n", s.Action, s.Hash)
		}
	}
	todoPath := filepath.Join(dir, "todo")
	if err := os.WriteFile(todoPath, []byte(todo.String()), 0o644); err != nil {
		return Output{}, err
	}
	args := []string{
		"-c", "sequence.editor=cp " + shellQuote(filepath.ToSlash(todoPath)),
		"-c", "core.editor=true",
		"-c", "rebase.missingCommitsCheck=ignore",
		"rebase", "-i", "--autostash", "--no-autosquash",
	}
	if base == "" {
		args = append(args, "--root")
	} else {
		args = append(args, base)
	}
	out, err := r.Run(ctx, repo.Root, args...)
	if err == nil {
		os.RemoveAll(dir)
	}
	return Output{Stdout: string(out)}, err
}

// shellQuote quotes s for the POSIX shell Git runs editors through (Git for
// Windows ships one too).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
