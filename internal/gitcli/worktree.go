package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"viff/internal/domain"
)

// WorktreeFiles lists the files of the repository's working tree: everything
// in the index plus untracked files, with paths excluded by .gitignore left
// out. It is the list a read-only Files browser shows.
//
// Git is the source of truth, so callers query it again instead of keeping a
// path list: an editor, a hook or another terminal may add or remove files
// between two calls.
//
// -z keeps paths with spaces, quotes, newlines or non-ASCII bytes intact:
// records are separated by NUL and each record becomes the path unchanged, with
// no unquoting, cleaning or separator conversion. A path can appear more than
// once in the combined listing (unmerged index entries, for example), so
// duplicates are dropped in Go; the result is sorted by path.
func (r *Runner) WorktreeFiles(ctx context.Context, repo domain.Repo) ([]string, error) {
	out, err := r.Run(ctx, repo.Root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("list worktree files: %w", err)
	}
	recs := bytes.Split(out, []byte{0})
	seen := make(map[string]struct{}, len(recs))
	list := make([]string, 0, len(recs))
	for _, rec := range recs {
		if len(rec) == 0 {
			continue
		}
		path := string(rec)
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}
		list = append(list, path)
	}
	sort.Strings(list)
	return list, nil
}
