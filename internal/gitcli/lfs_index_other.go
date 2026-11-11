//go:build !windows

package gitcli

import (
	"io/fs"
	"os"
)

type lfsLocalIndexRevision struct{ info fs.FileInfo }

func (a lfsLocalIndexRevision) equal(b lfsLocalIndexRevision) bool {
	if a.info == nil || b.info == nil {
		return a.info == nil && b.info == nil
	}
	return a.info.Size() == b.info.Size() && a.info.ModTime().Equal(b.info.ModTime()) && os.SameFile(a.info, b.info)
}

func localLFSIndexRevision(index string) (lfsLocalIndexRevision, error) {
	info, err := os.Stat(index)
	return lfsLocalIndexRevision{info}, err
}
