package domain

// SubmoduleState is the single-character status Git prints for a submodule.
// The character is part of `git submodule status` output, so it is kept
// verbatim and only translated to text in the UI.
type SubmoduleState rune

const (
	// SubmoduleClean means the checked-out commit matches the index.
	SubmoduleClean SubmoduleState = ' '
	// SubmoduleUninitialized means the submodule is not checked out yet.
	SubmoduleUninitialized SubmoduleState = '-'
	// SubmoduleModified means the checked-out commit differs from the index.
	SubmoduleModified SubmoduleState = '+'
	// SubmoduleConflict means the submodule has merge conflicts.
	SubmoduleConflict SubmoduleState = 'U'
)

// String names the state for display.
func (s SubmoduleState) String() string {
	switch s {
	case SubmoduleClean:
		return "Up to date"
	case SubmoduleUninitialized:
		return "Not initialized"
	case SubmoduleModified:
		return "Modified"
	case SubmoduleConflict:
		return "Conflict"
	}
	return "Unknown"
}

// Submodule is one entry of `git submodule status --recursive`.
type Submodule struct {
	// Path is relative to the repository root, with forward slashes. Nested
	// submodules carry the full path from the root.
	Path string
	// SHA is the commit the submodule has checked out; for an uninitialized
	// submodule it is the commit recorded in the superproject's index.
	SHA string
	// State is the status prefix; it says nothing about the superproject's
	// working tree.
	State SubmoduleState
	// Describe is Git's `describe` of SHA when the submodule is initialized,
	// for example "v1.2.0-3-gabc1234"; it is empty when not initialized.
	Describe string
}

// ShortSHA is the hash Git would print for menus and tooltips.
func (s Submodule) ShortSHA() string { return ShortHash(s.SHA) }

// Initialized reports whether the submodule has a working tree.
func (s Submodule) Initialized() bool { return s.State != SubmoduleUninitialized }

// Change reports whether the submodule differs from the superproject.
func (s Submodule) Changed() bool {
	return s.State == SubmoduleModified || s.State == SubmoduleConflict
}
