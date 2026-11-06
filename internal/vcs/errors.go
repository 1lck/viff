package vcs

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrorKind classifies a failure so the UI can offer the right action. It is
// shared by every backend: the application layer branches on the kind, not on
// which tool produced the error.
type ErrorKind uint8

const (
	ErrOther ErrorKind = iota
	ErrCancelled
	ErrAuth
	ErrLocked
	ErrNotRepository
	ErrConflict
	ErrNotFound
	ErrRejected // push rejected (non-fast-forward)
	ErrUntrackedOverwrite
	ErrLocalChangesOverwrite
	ErrNotMerged
	ErrNothingToCommit
	ErrHookFailed
	// ErrNoGit means the backend's executable was not found. The name is kept
	// from the Git-only era because the user-facing code "git.missing" and the
	// log field "no_git" are part of the diagnostics format; a second backend
	// reports the same kind for its own missing tool.
	ErrNoGit

	// Subversion-specific kinds (ADR 0024). They are part of the shared enum
	// because the application layer reacts to them the same way regardless of
	// backend: refresh, ask the user, or retry after an update.
	//
	// ErrOutOfDate is a commit refused because the working copy is older than
	// the repository (svn E155011). Nothing was written.
	ErrOutOfDate
	// ErrNetwork is an unreachable or unreadable repository over the wire
	// (svn E170013/E180001). Unlike Git this is a normal state for a working
	// copy whose history lives on a server.
	ErrNetwork
	// ErrNoRevision names a revision that does not exist (svn E160006).
	ErrNoRevision

	// WSL kinds (ADR 0038). A Git backend running through wsl.exe can fail
	// before Git itself starts, and the user has three different things to fix,
	// so the three are separate kinds rather than one "git.failed".
	//
	// ErrNoWSL means wsl.exe is missing or unusable on this machine.
	ErrNoWSL
	// ErrNoDistro means the distribution named by the path does not exist.
	ErrNoDistro
	// ErrNoWSLGit means the distribution has no git executable.
	ErrNoWSLGit
)

// Error is a failed backend invocation with the context needed to diagnose it.
type Error struct {
	// Tool names the executable that failed ("git", "svn"). It appears in the
	// message so the console and the log stay readable with two backends; an
	// empty value means "git", which keeps the Git-era messages unchanged.
	Tool     string
	Kind     ErrorKind
	Args     []string
	ExitCode int
	Stderr   string
	Stdout   string
	Err      error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s %s: %s", e.tool(), firstArg(e.Args), e.Message())
}

// Message is the output meant for the user, without the command prefix.
func (e *Error) Message() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(e.Stdout)
	}
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) tool() string {
	if e.Tool == "" {
		return "git"
	}
	return e.Tool
}

// FirstArg is the subcommand shown in an error message, skipping the option
// arguments every backend puts in front of it. It is exported for the Git
// adapter's compatibility layer, which predates the shared error model.
func FirstArg(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" || args[i] == "-C" {
			i++
			continue
		}
		return args[i]
	}
	return ""
}

func firstArg(args []string) string { return FirstArg(args) }

// KindOf returns the error kind of err, ErrOther for foreign errors.
func KindOf(err error) ErrorKind {
	var be *Error
	if errors.As(err, &be) {
		return be.Kind
	}
	if errors.Is(err, context.Canceled) {
		return ErrCancelled
	}
	return ErrOther
}

// String is the stable name of the kind written to logs next to the code. It
// is a log format: rename one only together with the log format.
func (k ErrorKind) String() string {
	switch k {
	case ErrCancelled:
		return "cancelled"
	case ErrAuth:
		return "auth"
	case ErrLocked:
		return "locked"
	case ErrNotRepository:
		return "not_repository"
	case ErrConflict:
		return "conflict"
	case ErrNotFound:
		return "not_found"
	case ErrRejected:
		return "rejected"
	case ErrUntrackedOverwrite:
		return "untracked_overwrite"
	case ErrLocalChangesOverwrite:
		return "local_changes"
	case ErrNotMerged:
		return "not_merged"
	case ErrNothingToCommit:
		return "nothing_to_commit"
	case ErrHookFailed:
		return "hook_failed"
	case ErrNoGit:
		return "no_git"
	case ErrOutOfDate:
		return "out_of_date"
	case ErrNetwork:
		return "network"
	case ErrNoRevision:
		return "no_revision"
	case ErrNoWSL:
		return "no_wsl"
	case ErrNoDistro:
		return "no_distro"
	case ErrNoWSLGit:
		return "no_wsl_git"
	}
	return "other"
}

// Code is the stable, user-facing error code for the kind. It is what the UI,
// the log and the diagnostics bundle show, so users can paste one token into
// an issue and be understood. It never changes without a migration plan.
//
// The "git." prefix on the generic codes predates the second backend and is
// kept deliberately: it is documented in docs/observability.md and users quote
// it in issues. Backend-neutral codes stay unprefixed ("auth.failed",
// "repo.locked", "repo.not_a_repository"). Checks that need to distinguish the
// tool use Error.Tool instead of prefix-matching these strings.
func (k ErrorKind) Code() string {
	switch k {
	case ErrAuth:
		return "auth.failed"
	case ErrLocked:
		return "repo.locked"
	case ErrNotRepository:
		return "repo.not_a_repository"
	case ErrConflict:
		return "git.conflict"
	case ErrNotFound:
		return "git.not_found"
	case ErrRejected:
		return "push.rejected"
	case ErrUntrackedOverwrite:
		return "worktree.untracked_overwrite"
	case ErrLocalChangesOverwrite:
		return "worktree.local_changes"
	case ErrNotMerged:
		return "branch.not_merged"
	case ErrNothingToCommit:
		return "commit.empty"
	case ErrHookFailed:
		return "hook.failed"
	case ErrNoGit:
		return "git.missing"
	case ErrCancelled:
		return "git.cancelled"
	case ErrOutOfDate:
		return "repo.out_of_date"
	case ErrNetwork:
		return "repo.unreachable"
	case ErrNoRevision:
		return "revision.not_found"
	case ErrNoWSL:
		return "wsl.unavailable"
	case ErrNoDistro:
		return "wsl.distro_missing"
	case ErrNoWSLGit:
		return "wsl.git_missing"
	}
	return "git.failed"
}

// Code returns the error code of e.
func (e *Error) Code() string {
	if e == nil {
		return ErrOther.Code()
	}
	return e.Kind.Code()
}

// CodeOf returns the stable error code for err: the code of its kind, with the
// fallback code for errors Viff did not classify, and "" for nil. Callers pass
// the result to the UI and to log events; a failure event always has a code.
func CodeOf(err error) string {
	if err == nil {
		return ""
	}
	return KindOf(err).Code()
}

// Invocation errors that are not tied to one backend: they describe what the
// operation would have done wrong, so every backend returns the same value.
var (
	// ErrNotInstalled means the backend's executable is missing.
	ErrNotInstalled = errors.New("the version control executable was not found")
)
