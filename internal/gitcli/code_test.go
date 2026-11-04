package gitcli

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// TestErrorKindCodes pins the codes: they are a public format shared by the
// UI, docs/troubleshooting.md and the diagnostic bundle, so a change here is a
// format change.
func TestErrorKindCodes(t *testing.T) {
	want := map[ErrorKind]string{
		ErrAuth:                  "auth.failed",
		ErrLocked:                "repo.locked",
		ErrNotRepository:         "repo.not_a_repository",
		ErrConflict:              "git.conflict",
		ErrNotFound:              "git.not_found",
		ErrRejected:              "push.rejected",
		ErrUntrackedOverwrite:    "worktree.untracked_overwrite",
		ErrLocalChangesOverwrite: "worktree.local_changes",
		ErrNotMerged:             "branch.not_merged",
		ErrNothingToCommit:       "commit.empty",
		ErrHookFailed:            "hook.failed",
		ErrNoGit:                 "git.missing",
		ErrCancelled:             "git.cancelled",
		ErrOther:                 "git.failed",
	}
	if len(want) != 14 {
		t.Fatalf("the mapping must cover every kind, got %d", len(want))
	}
	for kind, code := range want {
		if got := kind.Code(); got != code {
			t.Errorf("kind %d code = %q, want %q", kind, got, code)
		}
	}
	// A zero value is ErrOther and must never produce an empty code.
	if got := ErrorKind(200).Code(); got != "git.failed" {
		t.Errorf("unknown kind code = %q, want git.failed", got)
	}
}

func TestErrorKindString(t *testing.T) {
	want := map[ErrorKind]string{
		ErrAuth:      "auth",
		ErrRejected:  "rejected",
		ErrNoGit:     "no_git",
		ErrCancelled: "cancelled",
		ErrOther:     "other",
	}
	for kind, name := range want {
		if got := kind.String(); got != name {
			t.Errorf("kind %d string = %q, want %q", kind, got, name)
		}
	}
}

func TestCodeOf(t *testing.T) {
	if got := CodeOf(nil); got != "" {
		t.Errorf("CodeOf(nil) = %q, want empty", got)
	}
	gitErr := &Error{Kind: ErrAuth, Stderr: "fatal: Authentication failed"}
	if got := CodeOf(gitErr); got != "auth.failed" {
		t.Errorf("CodeOf(git error) = %q", got)
	}
	// CodeOf follows error wrapping, like the rest of the package.
	wrapped := fmt.Errorf("push failed: %w", gitErr)
	if got := CodeOf(wrapped); got != "auth.failed" {
		t.Errorf("CodeOf(wrapped) = %q", got)
	}
	if got := CodeOf(errors.New("something else")); got != "git.failed" {
		t.Errorf("CodeOf(foreign error) = %q", got)
	}
	if got := CodeOf(context.Canceled); got != "git.cancelled" {
		t.Errorf("CodeOf(context.Canceled) = %q", got)
	}
}

func TestErrorCode(t *testing.T) {
	e := &Error{Kind: ErrRejected}
	if got := e.Code(); got != "push.rejected" {
		t.Errorf("(*Error).Code = %q", got)
	}
	var nilError *Error
	if got := nilError.Code(); got != "git.failed" {
		t.Errorf("nil (*Error).Code = %q", got)
	}
}
