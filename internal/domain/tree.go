package domain

// TreeEntry is one entry of a revision's tree, as git ls-tree reports it.
type TreeEntry struct {
	// Path is relative to the repository root and uses forward slashes.
	Path string
	// Mode is Git's file mode: 100644, 100755, 120000 (symlink), 160000
	// (submodule) or 040000 (tree, which -r does not list).
	Mode string
	// Type is blob, tree or commit.
	Type string
	SHA  string
	// Size is the blob size in bytes, or -1 when Git reports no size (a
	// submodule points at a commit, not a blob).
	Size int64
}

// Executable reports whether the entry is stored with the executable bit.
func (e TreeEntry) Executable() bool { return e.Mode == "100755" }

// Symlink reports whether the entry is a symbolic link.
func (e TreeEntry) Symlink() bool { return e.Mode == "120000" }

// Submodule reports whether the entry is a submodule (a gitlink to another
// repository's commit).
func (e TreeEntry) Submodule() bool { return e.Mode == "160000" || e.Type == "commit" }
