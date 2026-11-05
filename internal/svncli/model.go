package svncli

import "viff/internal/vcs"

// Model declares how a Subversion working copy differs from a Git repository
// in ways the UI must respect.
//
// Index is false and that is the important one: there is no staging area, so
// the changes list is a single column and a commit names its target paths
// instead of committing a staged snapshot. OfflineHistory is true because .svn
// holds only pristine base files: any history query is a network round trip, so
// a cached answer has to be labelled rather than presented as current.
func (r *Runner) Model() vcs.ModelFlags {
	return vcs.ModelFlags{
		Index:          false,
		Amend:          false,
		Properties:     true,
		Locks:          true,
		OfflineHistory: true,
	}
}
