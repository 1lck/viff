package vcs

import (
	"time"

	"viff/internal/domain"
)

// Content is one side of a diff: the bytes of a file at some revision, or a
// description of why the bytes are not shown.
type Content struct {
	Exists    bool
	Data      []byte
	Size      int64
	Binary    bool
	TooLarge  bool
	Mode      string
	Symlink   bool
	Submodule bool
}

// ContentLimit is the largest file loaded for a text diff. Bigger files get a
// restricted preview that never reads them whole.
const ContentLimit = 8 << 20

// Revision selects which version of a file to read.
type Revision struct {
	// Kind: "worktree", "index", "stage" (with Stage), or "commit" (with Rev).
	Kind  string
	Rev   string
	Stage int
}

// The revisions every backend must be able to name. A backend without an index
// (Subversion) maps RevIndex and RevStage onto its own base revision.
var (
	RevWorktree = Revision{Kind: "worktree"}
	RevIndex    = Revision{Kind: "index"}
	RevHead     = Revision{Kind: "commit", Rev: "HEAD"}
)

// RevCommit names a file at a commit.
func RevCommit(rev string) Revision { return Revision{Kind: "commit", Rev: rev} }

// RevStage names one index stage of an unmerged file.
func RevStage(n int) Revision { return Revision{Kind: "stage", Stage: n} }

// LogFilter narrows the log. Empty fields do not filter.
type LogFilter struct {
	// Revisions are the starting points; empty means all branches, remote
	// branches, tags and HEAD.
	Revisions []string
	Text      string // matched against messages (and hashes by the caller)
	Author    string
	Since     time.Time
	Paths     []string
	// FirstParent follows only first parents.
	FirstParent bool
}

// LogReader streams commits from one backend query. It exists so a long
// history is read only as far as the user scrolls instead of being built
// whole; Close releases whatever the backend keeps open.
type LogReader interface {
	// Next reads up to n more commits. done is true at the end of history.
	Next(n int) (commits []domain.Commit, done bool, err error)
	// Close releases the reader. It is safe to call more than once.
	Close()
}

// Output is what a write command printed, for the console and notifications.
type Output struct {
	Stdout string
	Stderr string
}

// CommitOptions configures a commit.
type CommitOptions struct {
	Message  string
	Amend    bool
	SignOff  bool
	NoVerify bool
	// Paths, when set, is the commit's scope: exactly these paths, using
	// their current working-copy content. For Git this is
	// `git commit --only`, which leaves staged changes to paths outside the
	// set staged and out of the commit; an untracked path is recorded as
	// intent-to-add for the duration of the commit. For Subversion it is the
	// natural way to commit because there is no index, so the same field
	// serves both. Empty means the backend's default scope: the index for
	// Git, every scheduled change for Subversion.
	Paths []string
}

// PullMode decides how pull integrates the upstream. A backend without a
// merge/rebase distinction ignores the value and performs its update.
type PullMode uint8

const (
	PullMerge PullMode = iota
	PullRebase
	PullFastForwardOnly
)

// PushOptions configures a push.
type PushOptions struct {
	Remote      string
	Branch      string // local branch
	RemoteName  string // remote branch name; defaults to Branch
	SetUpstream bool
	// ForceWithLease protects against overwriting work pushed by others.
	ForceWithLease bool
	Tags           bool
	NoVerify       bool
}

// ResetMode is the mode of a reset.
type ResetMode string

const (
	ResetSoft  ResetMode = "--soft"
	ResetMixed ResetMode = "--mixed"
	ResetHard  ResetMode = "--hard"
	ResetKeep  ResetMode = "--keep"
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

// BranchUpdate describes a fast-forward update of a local branch.
type BranchUpdate struct {
	Branch   string
	Old      string
	New      string
	Commits  int // commits gained; 0 when up to date
	UpToDate bool
}

// MultiCommitChanges is the change list of one commit in a multi-commit
// selection. The commits are queried independently, so one failure does not
// hide the changes of the others.
type MultiCommitChanges struct {
	Commit  domain.Commit
	Changes []domain.Change
	Err     error
}

// ConflictResolveRequest accepts one block of one unmerged file.
type ConflictResolveRequest struct {
	// Path is the repository-relative path, exactly as the backend reports it.
	Path string
	// Block is the index of the conflict block in the file, counted the way
	// domain.ParseConflictMarkers counts blocks.
	Block int
	// Choice is the side (or both) to keep.
	Choice domain.ResolveChoice
	// Expected is the working-tree file as the conflict view read it. The
	// bytes on disk must still be exactly these at the last content check,
	// immediately before the atomic rename; otherwise the request is refused
	// as stale and the edit found on disk is left in place.
	Expected []byte
}

// ConflictResolveResult describes a completed rewrite.
type ConflictResolveResult struct {
	Path         string
	Block        int
	Choice       domain.ResolveChoice
	BlocksBefore int
	BlocksAfter  int
	BytesWritten int
	LineEnding   string
	FinalNewline bool
}

// Config is the per-tool configuration the settings page pushes into whatever
// backend is active. It never changes files on disk. A backend ignores the
// fields it has no use for: Subversion has no signing and no credential
// helper, so only the fields it understands are read there.
type Config struct {
	SSHCommand       string // GIT_SSH_COMMAND
	GPGProgram       string // gpg.program
	SigningKey       string // user.signingkey
	SignFormat       string // gpg.format: "openpgp" or "ssh"
	CommitSigning    bool   // commit.gpgsign=true
	CredentialHelper string // credential.helper
}
