package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"viff/internal/domain"
	"viff/internal/sshpath"
)

// LFSIndexCache is the immutable index discovery result for one working tree.
// The index revision is independent of porcelain: flags such as
// assume-unchanged can hide an intent-to-add path from status.
type LFSIndexCache struct {
	path     string
	revision lfsIndexRevision
	paths    []string
}

type lfsIndexRevision struct {
	local  lfsLocalIndexRevision
	remote string
}

func (a lfsIndexRevision) equal(b lfsIndexRevision) bool {
	return a.remote == b.remote && a.local.equal(b.local)
}

// LFSTrackedAttributes reuses discovery when Git's actual index is unchanged.
// It only enumerates cached paths, and confirms the same index surrounded the
// command so a concurrent index replacement cannot poison the cache.
// The returned paths belong to the immutable cache; callers must not mutate them.
func (r *Runner) LFSTrackedAttributes(ctx context.Context, repo domain.Repo, previous *LFSIndexCache) (*LFSIndexCache, []string, error) {
	var index string
	if previous != nil {
		index = previous.path
	} else {
		out, err := r.Run(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-path", "index")
		if err != nil {
			return nil, nil, fmt.Errorf("locate Git index for attributes: %w", err)
		}
		path := strings.TrimSuffix(string(out), "\n")
		if path == "" {
			return nil, nil, errors.New("Git returned no index path")
		}
		index, err = toIdentityPath(repo.Root, path)
		if err != nil {
			return nil, nil, fmt.Errorf("map Git index path for attributes: %w", err)
		}
	}
	before, err := r.lfsIndexRevision(ctx, repo, index)
	if err != nil {
		return nil, nil, fmt.Errorf("check Git index for attributes: %w", err)
	}
	if previous != nil && previous.revision.equal(before) {
		return previous, previous.paths, nil
	}
	out, err := r.Run(ctx, repo.Root, "ls-files", "-z", "--cached", "--", ":(glob)**/.gitattributes")
	if err != nil {
		return nil, nil, fmt.Errorf("list tracked .gitattributes files: %w", err)
	}
	after, err := r.lfsIndexRevision(ctx, repo, index)
	if err != nil {
		return nil, nil, fmt.Errorf("recheck Git index for attributes: %w", err)
	}
	if !before.equal(after) {
		return nil, nil, errors.New("Git index changed while discovering LFS attributes")
	}
	if len(out) > 0 && out[len(out)-1] != 0 {
		return nil, nil, errors.New("incomplete tracked .gitattributes listing")
	}
	var paths []string
	for rec := range bytes.SplitSeq(out, []byte{0}) {
		if len(rec) > 0 {
			paths = append(paths, string(rec))
		}
	}
	cache := &LFSIndexCache{path: index, revision: after, paths: paths}
	return cache, paths, nil
}

func (r *Runner) lfsIndexRevision(ctx context.Context, repo domain.Repo, index string) (lfsIndexRevision, error) {
	if err := ctx.Err(); err != nil {
		return lfsIndexRevision{}, err
	}
	if repo.IsSSH() {
		path, err := sshpath.Parse(index)
		if err != nil {
			return lfsIndexRevision{}, err
		}
		rev, err := r.ssh().IndexRevision(ctx, repo, path.Path)
		return lfsIndexRevision{remote: rev}, err
	}
	stamp, err := localLFSIndexRevision(index)
	if errors.Is(err, fs.ErrNotExist) {
		return lfsIndexRevision{}, nil // unborn repository without an index
	}
	return lfsIndexRevision{local: stamp}, err
}
