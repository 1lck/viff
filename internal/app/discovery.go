package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// Repository discovery finds Git repositories below a directory the user
// chose. Git stays the source of truth: a directory is only reported after
// `Discover` confirmed it, so a directory *named* like a repository is never
// enough. Repositories nested inside other repositories, submodules and linked
// worktrees are reported as their own entries: they are separate working trees
// with separate status.
//
// The scan is always explicit. It never runs at startup and never changes the
// workspace by itself; ADR 0020's "do not scan automatically" still holds.

const (
	// DefaultDiscoveryDepth is how many directory levels below the chosen root
	// are scanned. Four covers the usual workspace/group/project layout while
	// keeping a large monorepo's tree from dominating the scan.
	DefaultDiscoveryDepth = 4
	// DefaultDiscoveryDirs caps the number of directories visited by one scan.
	DefaultDiscoveryDirs = 2000
	// DefaultDiscoveryRepos caps the number of repositories one scan reports.
	DefaultDiscoveryRepos = 200
)

// DiscoveryOptions limits one scan. Zero values use the defaults.
type DiscoveryOptions struct {
	MaxDepth int
	MaxDirs  int
	MaxRepos int
}

func (o DiscoveryOptions) normalized() DiscoveryOptions {
	if o.MaxDepth <= 0 {
		o.MaxDepth = DefaultDiscoveryDepth
	}
	if o.MaxDirs <= 0 {
		o.MaxDirs = DefaultDiscoveryDirs
	}
	if o.MaxRepos <= 0 {
		o.MaxRepos = DefaultDiscoveryRepos
	}
	return o
}

// DiscoveryResult is what one scan found. Truncated reports that a limit
// stopped the walk; the repositories found so far are still valid.
type DiscoveryResult struct {
	Repositories []domain.Repo
	Scanned      int
	Inaccessible int
	Truncated    bool
}

// errDiscoveryLimit stops the walk once a limit is reached; it is a control
// signal, not a failure.
var errDiscoveryLimit = errors.New("repository scan limit reached")

// discoverySkipDirs keeps dependency and build trees from consuming a scan's
// directory budget. They are skipped by name, which can hide a repository
// deliberately nested inside one of them; the scan limits and the ADR record
// that trade-off. `.git` is skipped separately: it holds metadata, not a
// working tree.
var discoverySkipDirs = map[string]struct{}{
	"node_modules": {},
	"vendor":       {},
	"target":       {},
	"dist":         {},
	"build":        {},
	"bin":          {},
	"obj":          {},
}

// IsNotRepository reports that Git was run in a directory that is not inside a
// repository. The UI uses it to offer repository discovery instead of showing
// Git's error.
func IsNotRepository(err error) bool {
	return gitcli.KindOf(err) == gitcli.ErrNotRepository
}

// DiscoverRepositories scans dir in the background and reports what it found on
// the UI thread. The returned task ID can be passed to CancelTask, which stops
// the WalkDir loop and any repository probe in progress.
func (w *Workspace) DiscoverRepositories(dir string, opts DiscoveryOptions, done func(DiscoveryResult, error)) uint64 {
	t, ctx, finish := w.startTask("Scanning "+filepath.Base(dir), false)
	go func() {
		res, err := w.scanRepositories(ctx, dir, opts)
		w.post(func() {
			finish()
			if done != nil {
				done(res, err)
			}
		})
	}()
	return t.ID
}

// scanRepositories is the worker half of DiscoverRepositories. It walks the
// directory tree without following symlinks, probes every directory that has a
// `.git` entry (directory or file) and keeps walking inside a repository so
// nested repositories are found too.
func (w *Workspace) scanRepositories(ctx context.Context, dir string, opts DiscoveryOptions) (DiscoveryResult, error) {
	opts = opts.normalized()
	var res DiscoveryResult
	root := dir
	if abs, err := filepath.Abs(dir); err == nil {
		root = filepath.Clean(abs)
	}
	info, err := os.Stat(root)
	if err != nil {
		return res, fmt.Errorf("scan repositories: %w", err)
	}
	if !info.IsDir() {
		return res, fmt.Errorf("scan repositories: %s is not a directory", root)
	}
	seen := make(map[string]struct{})
	var probeErr error
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			// An unreadable subtree is skipped and counted; only an unreadable
			// root makes the whole scan fail.
			res.Inaccessible++
			if path == root {
				return err
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root {
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil && discoveryDepth(rel) > opts.MaxDepth {
				return fs.SkipDir
			}
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			if _, skip := discoverySkipDirs[strings.ToLower(d.Name())]; skip {
				return fs.SkipDir
			}
		}
		res.Scanned++
		if res.Scanned > opts.MaxDirs {
			res.Truncated = true
			return errDiscoveryLimit
		}
		if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr != nil {
			return nil
		}
		repo, discoverErr := w.Git.Discover(ctx, path)
		if discoverErr != nil {
			if gitcli.KindOf(discoverErr) != gitcli.ErrNotRepository && probeErr == nil {
				probeErr = discoverErr
			}
			return nil
		}
		// A candidate whose `.git` is broken can make Git answer with an
		// enclosing repository. Only the directory itself may be reported, so
		// the same repository is not listed twice under a nested name.
		key := canonicalPath(repo.Root)
		if key != canonicalPath(path) {
			return nil
		}
		if _, ok := seen[key]; ok {
			return nil
		}
		seen[key] = struct{}{}
		res.Repositories = append(res.Repositories, repo)
		if len(res.Repositories) >= opts.MaxRepos {
			res.Truncated = true
			return errDiscoveryLimit
		}
		return nil
	})
	switch {
	case errors.Is(walkErr, errDiscoveryLimit):
		walkErr = nil
	case walkErr != nil:
		return res, fmt.Errorf("scan repositories: %w", walkErr)
	}
	if len(res.Repositories) == 0 && probeErr != nil {
		return res, fmt.Errorf("scan repositories: %w", probeErr)
	}
	sort.Slice(res.Repositories, func(i, j int) bool {
		return strings.ToLower(res.Repositories[i].Root) < strings.ToLower(res.Repositories[j].Root)
	})
	return res, nil
}

// discoveryDepth is the number of directory levels in a relative path: "a" is
// one level below the scan root, "a/b" is two.
func discoveryDepth(rel string) int {
	if rel == "" || rel == "." {
		return 0
	}
	return 1 + strings.Count(rel, string(filepath.Separator))
}

// AddRepositoriesResult summarizes a batch add. Repositories already in the
// workspace are skipped instead of duplicated.
type AddRepositoriesResult struct {
	Added     int
	Skipped   int
	Failed    int
	FirstErr  error
	Cancelled bool
}

// AddRepositories adds several repositories to the workspace list in one task.
// It never changes the active repository and never runs a Git write. Each root
// is discovered again on the worker goroutine: the Git state may have changed
// since the scan, and Git stays the source of truth.
func (w *Workspace) AddRepositories(roots []string, done func(AddRepositoriesResult)) {
	_, ctx, finish := w.startTask(fmt.Sprintf("Adding %d repositories", len(roots)), false)
	go func() {
		var res AddRepositoriesResult
		repos := make([]domain.Repo, 0, len(roots))
		for _, root := range roots {
			if ctx.Err() != nil {
				res.Cancelled = true
				break
			}
			repo, err := w.Git.Discover(ctx, root)
			if err != nil {
				res.Failed++
				if res.FirstErr == nil {
					res.FirstErr = err
				}
				continue
			}
			repos = append(repos, repo)
		}
		w.post(func() {
			finish()
			if res.Cancelled {
				if done != nil {
					done(res)
				}
				return
			}
			for _, repo := range repos {
				key := canonicalPath(repo.Root)
				if key == "" {
					res.Failed++
					continue
				}
				if w.repos.byRoot[key] != nil {
					res.Skipped++
					continue
				}
				w.addEntry(key)
				res.Added++
			}
			if res.Added > 0 {
				w.saveRepositories()
			}
			if done != nil {
				done(res)
			}
		})
	}()
}
