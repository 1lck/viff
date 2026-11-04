package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"viff/internal/domain"
)

// TreeAt lists every file that a revision's tree contains, recursively. The
// result is sorted by path. rev may be any commit-ish; "" means HEAD, and an
// unknown revision is reported with Git's own message.
//
// -l asks for blob sizes so the browser can show them without one stat per
// file, and -z keeps paths with spaces, tabs, newlines or non-ASCII bytes
// intact (Git quotes them otherwise).
func (r *Runner) TreeAt(ctx context.Context, repo domain.Repo, rev string) ([]domain.TreeEntry, error) {
	rev = strings.TrimSpace(rev)
	if rev == "" {
		rev = "HEAD"
	}
	out, err := r.Run(ctx, repo.Root, "ls-tree", "-r", "-z", "--full-tree", "-l", rev)
	if err != nil {
		return nil, fmt.Errorf("list tree of %s: %w", rev, err)
	}
	return ParseTree(out)
}

// ParseTree reads the NUL-separated output of git ls-tree -r -z -l: each
// record is "<mode> <type> <sha> <size>\t<path>", and only NUL separates
// records, so a path may contain any byte except NUL.
func ParseTree(out []byte) ([]domain.TreeEntry, error) {
	list := make([]domain.TreeEntry, 0, bytes.Count(out, []byte{0}))
	for _, rec := range bytes.Split(out, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		e, err := parseTreeEntry(rec)
		if err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
	return list, nil
}

func parseTreeEntry(rec []byte) (domain.TreeEntry, error) {
	tab := bytes.IndexByte(rec, '\t')
	if tab < 0 {
		return domain.TreeEntry{}, fmt.Errorf("ls-tree: missing tab in %q", rec)
	}
	meta, path := string(rec[:tab]), string(rec[tab+1:])
	if path == "" {
		return domain.TreeEntry{}, fmt.Errorf("ls-tree: empty path in %q", rec)
	}
	f := strings.Fields(meta)
	if len(f) != 4 {
		return domain.TreeEntry{}, fmt.Errorf("ls-tree: malformed entry %q", rec)
	}
	size := int64(-1)
	if f[3] != "-" {
		n, err := strconv.ParseInt(f[3], 10, 64)
		if err != nil {
			return domain.TreeEntry{}, fmt.Errorf("ls-tree: bad size %q in %q", f[3], rec)
		}
		size = n
	}
	return domain.TreeEntry{Mode: f[0], Type: f[1], SHA: f[2], Size: size, Path: path}, nil
}
