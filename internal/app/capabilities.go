package app

import "viff/internal/vcs"

// Capabilities are the optional features of the active backend. It is the
// contract's type, re-exported under an app-level name so call sites read
// "app.Capabilities" rather than reaching into vcs for a domain concept the
// application layer owns.
type Capabilities = vcs.Capabilities

// Git is the required backend contract the use cases depend on: open, read,
// change the working copy, commit and synchronise. Optional services (history
// rewriting, stash, worktrees, reflog, submodules, ref writing, conflict
// resolution) are deliberately NOT part of it — they are reached through
// Workspace.Caps, so a backend that lacks a feature reports it as nil and the
// call site says "not supported" instead of the adapter having to stub a
// method it cannot implement.
//
// That split is what lets one binary host both backends:
// docs/decisions/0024-svn-support.md records that Subversion has no rebase,
// stash, worktree, reflog or submodule, so none of those may be compulsory.
type Git = vcs.Backend

// ErrUnsupported reports that the active backend does not have a feature the
// caller asked for. It carries a stable code like every other failure, and it
// is only reachable if the UI offered an entry it should have hidden.
var ErrUnsupported = &vcs.Error{Kind: vcs.ErrOther, Stderr: "this operation is not available for the open repository"}

// CommittableCount is how many changes the next commit would record.
//
// A backend with an index (Git) records only what is staged. One without it
// (Subversion) records every tracked change, because "svn commit" takes the
// scheduled and modified paths directly: there is no intermediate snapshot to
// stage into. Untracked files are excluded either way — they need an explicit
// add first — and a conflicted working copy can commit nothing.
//
// The UI uses this for the default commit scope. A checked selection is a
// second, explicit scope (CommitRequest.Paths): checked files are committed
// from the working tree without staging, so a zero count does not by itself
// disable Commit while files are checked. Asking for the staged count directly
// made the button permanently disabled on Subversion, where that count is
// always zero.
func (w *Workspace) CommittableCount() int {
	if w.Repo == nil {
		return 0
	}
	staged, unstaged, _, conflicts := w.Repo.Status.Counts()
	if conflicts > 0 {
		return 0
	}
	if w.Caps.Index {
		return staged
	}
	return staged + unstaged
}

// CapabilitiesOf derives what the UI may offer from the backend instance. A
// backend that does not implement an optional interface reports it as absent,
// which is how the same binary hides rebase for Subversion and shows it for
// Git; the model differences come from the backend's own declaration.
func CapabilitiesOf(g Git) Capabilities {
	caps := vcs.Capabilities{
		// Git-shaped defaults: an index, local amend, a push step after the
		// commit, no properties or locks, and a fully local object store.
		Index:          true,
		Amend:          true,
		SeparatePush:   true,
		BranchUpdate:   true,
		Locks:          false,
		OfflineHistory: false,
		Properties:     false,
	}
	if m, ok := g.(vcs.ModelReporter); ok {
		flags := m.Model()
		caps.Index = flags.Index
		caps.Amend = flags.Amend
		caps.SeparatePush = flags.SeparatePush
		caps.BranchUpdate = flags.BranchUpdate
		caps.Properties = flags.Properties
		caps.Locks = flags.Locks
		caps.OfflineHistory = flags.OfflineHistory
	}
	if h, ok := g.(vcs.HistoryWriter); ok {
		caps.HistoryWriter = h
	}
	if s, ok := g.(vcs.StashWriter); ok {
		caps.Stash = s
	}
	if w, ok := g.(vcs.WorktreeWriter); ok {
		caps.Worktrees = w
	}
	if r, ok := g.(vcs.ReflogReader); ok {
		caps.Reflog = r
	}
	if s, ok := g.(vcs.SubmoduleWriter); ok {
		caps.Submodules = s
	}
	if r, ok := g.(vcs.ReferenceWriter); ok {
		caps.References = r
	}
	if b, ok := g.(vcs.BranchWriter); ok {
		caps.Branches = b
	}
	if c, ok := g.(vcs.ConflictResolver); ok {
		caps.Conflicts = c
	}
	return caps
}
