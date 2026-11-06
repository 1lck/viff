package gitcli

import "viff/internal/vcs"

// compile-time proof that the Git adapter satisfies the contract the
// application layer depends on. If a method drifts, this file fails to build
// instead of the mismatch surfacing as a runtime type assertion in cmd/viff.
var (
	_ vcs.Backend          = (*Runner)(nil)
	_ vcs.Opener           = (*Runner)(nil)
	_ vcs.Reader           = (*Runner)(nil)
	_ vcs.WorkspaceWriter  = (*Runner)(nil)
	_ vcs.CommitWriter     = (*Runner)(nil)
	_ vcs.Syncer           = (*Runner)(nil)
	_ vcs.ReferenceBrowser = (*Runner)(nil)
	_ vcs.Lifecycle        = (*Runner)(nil)
	_ vcs.HistoryWriter    = (*Runner)(nil)
	_ vcs.StashWriter      = (*Runner)(nil)
	_ vcs.WorktreeWriter   = (*Runner)(nil)
	_ vcs.ReflogReader     = (*Runner)(nil)
	_ vcs.SubmoduleWriter  = (*Runner)(nil)
	_ vcs.ReferenceWriter  = (*Runner)(nil)
	_ vcs.ConflictResolver = (*Runner)(nil)
	_ vcs.LogReader        = (*LogReader)(nil)
)
