package vcs

import (
	"context"
	"time"

	"viff/internal/domain"
)

// Backend is what the use cases require from a version control system. It is
// the composition of the small interfaces below, not one flat list of every
// command: an adapter is checked against the capability it claims to provide.
//
// The required set is deliberately what a daily client needs to open a
// repository, show its state and history, change the working copy, commit and
// synchronise. Everything a version control system may not have at all
// (rebase, stash, worktrees, reflog, submodules) lives in Capabilities
// instead, so an adapter never implements a method just to return
// "unsupported" — see docs/decisions/0024-svn-support.md.
type Backend interface {
	Opener
	Reader
	WorkspaceWriter
	CommitWriter
	Syncer
	ReferenceBrowser
	Lifecycle
}

// Opener creates or finds a repository.
type Opener interface {
	// Discover locates the repository containing dir and reports its kind.
	Discover(ctx context.Context, dir string) (domain.Repo, error)
	// Init creates an empty repository in dir.
	Init(ctx context.Context, dir string) error
	// Clone copies url into parent/dir, or checks it out for backends that
	// keep history on a server.
	Clone(ctx context.Context, parent, url, dir string, progress func(string)) error
}

// Reader answers questions about the repository state and its history. Every
// method here is a query: it never writes, and it must be cancelable.
type Reader interface {
	Status(ctx context.Context, repo domain.Repo) (domain.Status, error)
	// Operation reports the multi-step operation in progress, rebuilt from
	// disk each time. A backend without such operations returns OpNone.
	Operation(ctx context.Context, repo domain.Repo) domain.Operation
	// Conflicts lists the unmerged paths, using status when the backend can
	// derive them from it.
	Conflicts(ctx context.Context, repo domain.Repo, status domain.Status) ([]domain.Conflict, error)
	// OpenLog starts a history query the caller reads page by page.
	OpenLog(ctx context.Context, repo domain.Repo, f LogFilter) (LogReader, error)
	CommitDetail(ctx context.Context, repo domain.Repo, hash string) (domain.CommitDetail, error)
	// Read loads one path at one revision. A path missing at that revision is
	// an absent side (Content.Exists false), not an error.
	Read(ctx context.Context, repo domain.Repo, rev Revision, path string) (Content, error)
	Blame(ctx context.Context, repo domain.Repo, rev, path string) (*domain.Blame, error)
	// DiffNames lists the files changed between two revisions.
	DiffNames(ctx context.Context, repo domain.Repo, from, to string) ([]domain.Change, error)
	// TreeAt lists the repository tree at a revision.
	TreeAt(ctx context.Context, repo domain.Repo, rev string) ([]domain.TreeEntry, error)
	// MultiCommitChanges lists the files each commit changed. The commits are
	// queried independently so one failure does not hide the others.
	MultiCommitChanges(ctx context.Context, repo domain.Repo, commits []domain.Commit) []MultiCommitChanges
	// Config reads one configuration value, "" when unset. A backend without
	// configuration returns "".
	Config(ctx context.Context, repo domain.Repo, key string) string
	// WorktreeFiles lists the paths a file browser may show: everything the
	// backend tracks plus the untracked files it does not ignore. Paths are
	// working-copy relative, use forward slashes, appear once and are sorted.
	WorktreeFiles(ctx context.Context, repo domain.Repo) ([]string, error)
}

// WorkspaceWriter changes the working copy without committing.
type WorkspaceWriter interface {
	// Stage records paths as part of the coming commit. A backend without an
	// index (Subversion) schedules additions and deletions instead: the
	// method keeps its name because the use case is the same, "include these
	// paths in the next commit".
	Stage(ctx context.Context, repo domain.Repo, paths []string) error
	// Unstage reverses Stage. hasHead reports whether HEAD exists, which a
	// backend needs to know before its first commit.
	Unstage(ctx context.Context, repo domain.Repo, paths []string, hasHead bool) error
	// Discard throws away working-copy changes to tracked paths.
	Discard(ctx context.Context, repo domain.Repo, paths []string) error
	// Revert restores tracked paths from the recorded revision.
	Revert(ctx context.Context, repo domain.Repo, paths []string) error
}

// CommitWriter records changes and moves patches around.
type CommitWriter interface {
	Commit(ctx context.Context, repo domain.Repo, o CommitOptions) (Output, error)
	// LastCommitMessage returns the current commit's message, for amend.
	LastCommitMessage(ctx context.Context, repo domain.Repo) (string, error)
	// CreatePatch renders a diff of paths. staged selects only what is
	// recorded for the next commit; a backend without an index treats staged
	// as "the changes that would be committed".
	CreatePatch(ctx context.Context, repo domain.Repo, paths []string, staged bool) ([]byte, error)
	ApplyPatch(ctx context.Context, repo domain.Repo, patch []byte, cached, reverse bool) error
}

// Syncer talks to the remote.
type Syncer interface {
	// Fetch updates what the local repository knows about the remote without
	// touching the working copy. A backend whose update is a single network
	// operation performs nothing here; Pull does the work.
	Fetch(ctx context.Context, repo domain.Repo, remote string, prune bool, progress func(string)) error
	// Pull brings the remote changes in. mode is a Git integration choice a
	// backend without rebase ignores.
	Pull(ctx context.Context, repo domain.Repo, mode PullMode, progress func(string)) (Output, error)
	Push(ctx context.Context, repo domain.Repo, o PushOptions, progress func(string)) (Output, error)
	// UpdateBranch fast-forwards a local branch from the remote without
	// checking it out. A backend without cheap ref updates returns
	// ErrUnsupported from Capabilities-supplied wrappers instead.
	UpdateBranch(ctx context.Context, repo domain.Repo, remote, branch string, progress func(string)) (BranchUpdate, error)
	IsShallow(ctx context.Context, repo domain.Repo) (bool, error)
	// Unshallow fetches the history a shallow clone left out.
	Unshallow(ctx context.Context, repo domain.Repo, progress func(string)) error
}

// ReferenceBrowser reads branches, tags and remotes and moves between them.
// It is separate from ReferenceWriter because reading is required and
// rewriting references is not universally available.
type ReferenceBrowser interface {
	Refs(ctx context.Context, repo domain.Repo) ([]domain.Ref, error)
	Remotes(ctx context.Context, repo domain.Repo) ([]domain.Remote, error)
	// Checkout switches the working copy to ref.
	Checkout(ctx context.Context, repo domain.Repo, ref domain.Ref) error
	// CheckoutRevision switches to a detached revision. A backend without a
	// detached state checks out the containing branch instead.
	CheckoutRevision(ctx context.Context, repo domain.Repo, rev string) error
	// ResolveRef turns a revision expression into the full identifier.
	ResolveRef(ctx context.Context, repo domain.Repo, rev string) (string, error)
	// IsAncestor reports whether a is an ancestor of b. A linear-history
	// backend compares revision numbers.
	IsAncestor(ctx context.Context, repo domain.Repo, a, b string) bool
	// ContainingBranches lists the branches whose history holds hash.
	ContainingBranches(ctx context.Context, repo domain.Repo, hash string, limit int) ([]string, error)
}

// Prober lets the composition root choose a backend before running anything.
//
// Detection has to be a cheap local check, not a command: picking the backend
// must not start a process, because a directory that belongs to neither system
// would otherwise pay two failed invocations before the user sees the welcome
// screen.
type Prober interface {
	// Detect reports whether dir looks like a repository of this kind.
	Detect(dir string) bool
	// Name is the short human name shown when reporting which backend is in
	// use ("Git", "Subversion").
	Name() string
}

// CommandLog receives one line per finished command. It is a function type
// rather than an interface because each call site passes a closure and there
// is no other state to model.
type CommandLog func(dir string, args []string, took time.Duration, err error)

// Lifecycle is how the settings page and the diagnostics view talk to the
// active backend.
type Lifecycle interface {
	Version(ctx context.Context) (string, error)
	// SetConfig pushes the per-tool configuration. It changes how later
	// commands run, never a file on disk.
	SetConfig(c Config)
	// SetPath overrides the executable location; "" means PATH.
	SetPath(path string)
	// SetCommandLog installs the hook that records finished commands. Passing
	// nil disables it. The application layer installs one implementation for
	// whichever backend is active, so the console and the event log behave the
	// same way.
	SetCommandLog(fn CommandLog)
}

// ModelFlags are the capability differences that come from the version control
// model rather than from a missing method. A backend declares them once instead
// of the application inferring them from the adapter's type.
type ModelFlags struct {
	// Index reports a staging area distinct from the working copy.
	Index bool
	// Amend reports that the last commit can be rewritten locally.
	Amend bool
	// Properties reports versioned file properties.
	Properties bool
	// Locks reports repository locks.
	Locks bool
	// OfflineHistory reports that history needs the repository, so cached
	// results must be labelled as possibly stale.
	OfflineHistory bool
}

// ModelReporter is implemented by a backend that wants to declare its model
// differences. Without it the application assumes the Git-shaped defaults.
type ModelReporter interface {
	Model() ModelFlags
}

// Capabilities names the optional features of a backend. A nil field means the
// backend does not have it at all, and the UI hides the corresponding entry
// instead of showing it and failing later.
//
// Keep these fields coarse: each one is a screen or a menu, not a command.
type Capabilities struct {
	// Index reports a two-stage commit model: a staging area distinct from
	// the working copy. Git has one, Subversion does not.
	Index bool
	// Amend reports that the last commit can be rewritten locally.
	Amend bool
	// Properties reports versioned file properties (svn:ignore,
	// svn:eol-style, ...). Git has none, Subversion does.
	Properties bool
	// Locks reports repository locks (svn lock/unlock).
	Locks bool
	// OfflineHistory reports that history needs the repository, not just the
	// local copy. The UI must then label cached results as possibly stale
	// instead of presenting them as current.
	OfflineHistory bool

	// HistoryWriter merges, rebases and otherwise rewrites history.
	HistoryWriter HistoryWriter
	// Stash shelves and restores uncommitted work.
	Stash StashWriter
	// Worktrees manages additional checkouts of one repository.
	Worktrees WorktreeWriter
	// Reflog reads the reference log.
	Reflog ReflogReader
	// Submodules manages nested repositories.
	Submodules SubmoduleWriter
	// References creates and deletes branches and tags.
	References ReferenceWriter
	// Conflicts resolves a single conflict block in place.
	Conflicts ConflictResolver
}

// HistoryWriter is the set of operations that rewrite commits. Subversion has
// no equivalent — a committed revision is immutable — so it leaves this nil.
type HistoryWriter interface {
	Merge(ctx context.Context, repo domain.Repo, rev string, noFF, squash bool) (Output, error)
	Rebase(ctx context.Context, repo domain.Repo, onto string) (Output, error)
	CherryPick(ctx context.Context, repo domain.Repo, hashes []string) (Output, error)
	RevertCommits(ctx context.Context, repo domain.Repo, hashes []string) (Output, error)
	Reset(ctx context.Context, repo domain.Repo, mode ResetMode, rev string) error
	// OperationCommand drives an in-progress operation ("continue", "skip",
	// "abort").
	OperationCommand(ctx context.Context, repo domain.Repo, op domain.OperationKind, action string) (Output, error)
	RebaseCommits(ctx context.Context, repo domain.Repo, from string) ([]RebaseCommit, string, error)
	InteractiveRebase(ctx context.Context, repo domain.Repo, base string, steps []RebaseStep) (Output, error)
	RewordCommit(ctx context.Context, repo domain.Repo, hash, message string) (Output, error)
	SquashCommit(ctx context.Context, repo domain.Repo, hash, message string) (Output, error)
	DropCommit(ctx context.Context, repo domain.Repo, hash string) (Output, error)
}

// StashWriter shelves uncommitted work.
type StashWriter interface {
	Stashes(ctx context.Context, repo domain.Repo) ([]domain.Stash, error)
	StashPush(ctx context.Context, repo domain.Repo, message string, untracked, keepIndex bool) error
	StashApply(ctx context.Context, repo domain.Repo, ref string, pop, index bool) (Output, error)
	StashDrop(ctx context.Context, repo domain.Repo, ref string) error
	StashBranch(ctx context.Context, repo domain.Repo, branch, ref string) error
}

// WorktreeWriter manages additional checkouts sharing one repository.
type WorktreeWriter interface {
	Worktrees(ctx context.Context, repo domain.Repo) ([]domain.Worktree, error)
	AddWorktree(ctx context.Context, repo domain.Repo, path, branch, start string) error
	AddWorktreeExisting(ctx context.Context, repo domain.Repo, path, branch string) error
	RemoveWorktree(ctx context.Context, repo domain.Repo, path string, force bool) error
}

// ReflogReader reads the reference log.
type ReflogReader interface {
	Reflog(ctx context.Context, repo domain.Repo, limit int) ([]domain.ReflogEntry, error)
}

// SubmoduleWriter manages nested repositories.
type SubmoduleWriter interface {
	Submodules(ctx context.Context, repo domain.Repo) ([]domain.Submodule, error)
	SubmoduleDirty(ctx context.Context, repo domain.Repo, path string) (bool, error)
	DirectSubmodulePaths(ctx context.Context, repo domain.Repo) (map[string]bool, error)
	AddSubmodule(ctx context.Context, repo domain.Repo, url, path string, progress func(string)) error
	DeinitSubmodule(ctx context.Context, repo domain.Repo, path string, force bool) error
	RemoveSubmodule(ctx context.Context, repo domain.Repo, path string, force bool) error
	UpdateSubmodule(ctx context.Context, repo domain.Repo, path string, init bool, progress func(string)) error
	SyncSubmodules(ctx context.Context, repo domain.Repo, path string) error
}

// ReferenceWriter creates, moves and deletes branches and tags. Reading them
// is in ReferenceBrowser and always available; writing is not.
type ReferenceWriter interface {
	CreateBranch(ctx context.Context, repo domain.Repo, name, start string, checkout, force bool) error
	RenameBranch(ctx context.Context, repo domain.Repo, old, name string) error
	DeleteBranch(ctx context.Context, repo domain.Repo, name string, force bool) error
	DeleteRemoteBranch(ctx context.Context, repo domain.Repo, remote, branch string, progress func(string)) error
	SetUpstream(ctx context.Context, repo domain.Repo, branch, upstream string) error
	CreateTag(ctx context.Context, repo domain.Repo, name, rev, message string) error
	DeleteTag(ctx context.Context, repo domain.Repo, name string) error
	PushTag(ctx context.Context, repo domain.Repo, remote, tag string, progress func(string)) error
	DeleteRemoteTag(ctx context.Context, repo domain.Repo, remote, tag string, progress func(string)) error
	PushRef(ctx context.Context, repo domain.Repo, remote, src, branch string, forceWithLease bool, progress func(string)) error
	AddRemote(ctx context.Context, repo domain.Repo, name, url string) error
	RemoveRemote(ctx context.Context, repo domain.Repo, name string) error
	SetRemoteURL(ctx context.Context, repo domain.Repo, name, url string) error
}

// ConflictResolver rewrites one conflict block in place.
type ConflictResolver interface {
	ResolveConflictBlock(ctx context.Context, repo domain.Repo, req ConflictResolveRequest) (ConflictResolveResult, error)
}
