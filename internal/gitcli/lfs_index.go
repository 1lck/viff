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

type lfsIndexRevision struct {
	local  lfsLocalIndexRevision
	remote string
}

func (a lfsIndexRevision) equal(b lfsIndexRevision) bool {
	return a.remote == b.remote && a.local.equal(b.local)
}

// lfsTrackedAttributes checks Git's actual index independently of porcelain:
// assume-unchanged can hide intent-to-add paths. Check both sides of discovery
// so a concurrent index replacement cannot poison the immutable cache.
func (r *Runner) lfsTrackedAttributes(ctx context.Context, repo domain.Repo, previous *LFSCache) (*LFSCache, error) {
	var index string
	if previous != nil {
		index = previous.indexPath
	} else {
		out, err := r.Run(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-path", "index")
		if err != nil {
			return nil, fmt.Errorf("locate Git index for attributes: %w", err)
		}
		path := strings.TrimSuffix(string(out), "\n")
		if path == "" {
			return nil, errors.New("Git returned no index path")
		}
		index, err = toIdentityPath(repo.Root, path)
		if err != nil {
			return nil, fmt.Errorf("map Git index path for attributes: %w", err)
		}
	}
	before, err := r.lfsIndexRevision(ctx, repo, index)
	if err != nil {
		return nil, fmt.Errorf("check Git index for attributes: %w", err)
	}
	if previous != nil && previous.revision.equal(before) {
		return previous, nil
	}
	out, err := r.Run(ctx, repo.Root, "ls-files", "-z", "--cached", "--", ":(glob)**/.gitattributes")
	if err != nil {
		return nil, fmt.Errorf("list tracked .gitattributes files: %w", err)
	}
	after, err := r.lfsIndexRevision(ctx, repo, index)
	if err != nil {
		return nil, fmt.Errorf("recheck Git index for attributes: %w", err)
	}
	if !before.equal(after) {
		return nil, errors.New("Git index changed while discovering LFS attributes")
	}
	if len(out) > 0 && out[len(out)-1] != 0 {
		return nil, errors.New("incomplete tracked .gitattributes listing")
	}
	var paths []string
	for rec := range bytes.SplitSeq(out, []byte{0}) {
		if len(rec) > 0 {
			paths = append(paths, string(rec))
		}
	}
	cache := &LFSCache{indexPath: index, revision: after, paths: paths}
	if previous != nil {
		cache.rules = previous.rules
	}
	return cache, nil
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
