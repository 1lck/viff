package svncli

import (
	"os"
	"path/filepath"
)

// Name is the short human name of this backend.
func (r *Runner) Name() string { return "Subversion" }

// Detect reports whether dir or one of its parents is a Subversion working
// copy.
//
// Since Subversion 1.7 every working copy keeps a single `.svn` directory at
// its root; the per-directory `.svn` of older checkouts is not supported by the
// client itself any more, so checking for one directory is enough.
func (r *Runner) Detect(dir string) bool {
	if dir == "" {
		return false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	for {
		if st, serr := os.Stat(filepath.Join(abs, ".svn")); serr == nil && st.IsDir() {
			return true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return false
		}
		abs = parent
	}
}

// CLIName is the executable name the console and the event log print for a
// command this adapter ran.
func (r *Runner) CLIName() string { return "svn" }
