package gitcli

import (
	"os"
	"path/filepath"
)

// Name is the short human name of this backend.
func (r *Runner) Name() string { return "Git" }

// Detect reports whether dir or one of its parents is a Git working tree.
//
// The check is local and needs no process: a `.git` directory is the ordinary
// case, but a `.git` *file* is how a linked worktree and a submodule record
// where their real git directory lives, so both count.
func (r *Runner) Detect(dir string) bool {
	return findMeta(dir, ".git", func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && (st.IsDir() || st.Mode().IsRegular())
	}) != ""
}

// findMeta walks up from dir looking for a metadata entry accepted by ok, and
// returns the directory holding it.
func findMeta(dir, name string, ok func(string) bool) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if ok(filepath.Join(abs, name)) {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
}

// CLIName is the executable name the console and the event log print for a
// command this adapter ran.
func (r *Runner) CLIName() string { return "git" }
