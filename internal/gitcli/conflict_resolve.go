package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// Errors of the in-app conflict resolution. When one is returned the file on
// disk was not changed: the rewrite happens in one atomic rename at the end.
var (
	// ErrResolvePath means the request names a path outside the repository or
	// a path that is not a plain relative Git path.
	ErrResolvePath = errors.New("the conflict path is not a file inside the repository")
	// ErrResolveNotConflicted means Git no longer lists the path as unmerged;
	// something else resolved or staged it after the view was loaded.
	ErrResolveNotConflicted = errors.New("the path is no longer an unmerged conflict")
	// ErrResolveStale means the file changed on disk after the conflict view
	// was built. The edit that was found is left in place; nothing is written.
	ErrResolveStale = errors.New("the file changed on disk after the conflict view was loaded")
	// ErrResolveMissing means the path has no working-tree file to rewrite.
	ErrResolveMissing = errors.New("the conflict file does not exist on disk")
	// ErrResolveBinary means the file is binary, so block sides cannot be
	// chosen as text.
	ErrResolveBinary = errors.New("the conflict file is binary; resolve it in an external editor")
	// ErrResolveTooLarge means the file is bigger than the text limit.
	ErrResolveTooLarge = errors.New("the conflict file is too large to resolve inside Viff")
	// ErrResolveUnsupported means the working-tree entry is a symlink, a
	// submodule or something else that does not have mergeable text.
	ErrResolveUnsupported = errors.New("this conflict cannot be resolved inside Viff; use an external editor")
)

// ConflictResolveRequest and ConflictResolveResult are shared with every
// backend and live in internal/vcs. They stay aliased here so existing call
// sites keep compiling.
type (
	// ConflictResolveRequest accepts one block of one unmerged file.
	ConflictResolveRequest = vcs.ConflictResolveRequest
	// ConflictResolveResult describes a completed rewrite.
	ConflictResolveResult = vcs.ConflictResolveResult
)

// ResolveConflictBlock rewrites the working-tree file of an unmerged path with
// one conflict block replaced by the chosen side(s). It refuses binary,
// oversized, symlink and submodule entries. Lines outside the block, the CRLF
// line endings and the final-newline state are preserved.
//
// The conflict viewer applies a block to its editor buffer and saves through
// the normal file-save path instead (docs/decisions/0051-editable-conflict-result.md),
// so this remains a backend capability without a caller in the UI.
//
// The file is read again immediately before the write. A save that landed
// before that last check is refused as ErrResolveStale and left in place; a
// save that races the final atomic rename cannot be detected, but the window
// is only that one check, and the write is a rename of a complete file.
//
// The index is never touched: Git still reports the path as unmerged, so the
// caller can keep using the ordinary "Mark as Resolved" step to stage it.
func (r *Runner) ResolveConflictBlock(ctx context.Context, repo domain.Repo, req ConflictResolveRequest) (ConflictResolveResult, error) {
	if err := readOnlyRemote(repo, "resolve"); err != nil {
		return ConflictResolveResult{}, err
	}
	var result ConflictResolveResult
	if !req.Choice.Valid() {
		return result, fmt.Errorf("unknown conflict choice %d", req.Choice)
	}
	abs, err := resolveWorktreePath(repo, req.Path)
	if err != nil {
		return result, err
	}
	if req.Block < 0 {
		return result, fmt.Errorf("%w: block %d", domain.ErrConflictBlock, req.Block)
	}
	unmerged, err := r.unmergedPath(ctx, repo, req.Path)
	if err != nil {
		return result, err
	}
	if !unmerged {
		return result, fmt.Errorf("%w: %s", ErrResolveNotConflicted, req.Path)
	}
	before, err := readWorktree(abs)
	if err != nil {
		return result, err
	}
	if err := checkResolvableFile(req.Path, before); err != nil {
		return result, err
	}
	if !bytes.Equal(before.Data, req.Expected) {
		return result, fmt.Errorf("%w: %s", ErrResolveStale, req.Path)
	}
	text := string(before.Data)
	blocksBefore := countConflictBlocks(text)
	out, err := domain.ResolveConflictText(text, req.Block, req.Choice)
	if err != nil {
		return result, fmt.Errorf("%s: %w", req.Path, err)
	}
	blocksAfter := countConflictBlocks(out)
	info, err := os.Lstat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result, fmt.Errorf("%w: %s", ErrResolveMissing, req.Path)
		}
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, fmt.Errorf("%w: %s", ErrResolveUnsupported, req.Path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), "."+filepath.Base(abs)+".viff-resolve-*")
	if err != nil {
		return result, err
	}
	cleanup := func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}
	if _, err := tmp.WriteString(out); err != nil {
		cleanup()
		return result, err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return result, err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		cleanup()
		return result, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return result, err
	}
	// Refresh the two facts the write depends on as late as possible. The Git
	// query goes first and the content comparison last, immediately before the
	// atomic rename, so the window in which an external save could land
	// unnoticed is as small as this design allows. It cannot be closed
	// completely: a save that races the final rename is still possible.
	unmerged, err = r.unmergedPath(ctx, repo, req.Path)
	if err != nil {
		os.Remove(tmp.Name())
		return result, err
	}
	if !unmerged {
		os.Remove(tmp.Name())
		return result, fmt.Errorf("%w: %s", ErrResolveNotConflicted, req.Path)
	}
	again, err := readWorktree(abs)
	if err != nil || !again.Exists || !bytes.Equal(again.Data, req.Expected) {
		os.Remove(tmp.Name())
		if err != nil {
			return result, err
		}
		return result, fmt.Errorf("%w: %s", ErrResolveStale, req.Path)
	}
	if err := os.Rename(tmp.Name(), abs); err != nil {
		os.Remove(tmp.Name())
		return result, err
	}
	return ConflictResolveResult{
		Path:         req.Path,
		Block:        req.Block,
		Choice:       req.Choice,
		BlocksBefore: blocksBefore,
		BlocksAfter:  blocksAfter,
		BytesWritten: len(out),
		LineEnding:   domain.DetectLineEnding(out),
		FinalNewline: strings.HasSuffix(out, "\n"),
	}, nil
}

// resolveWorktreePath maps a Git path to an absolute worktree path. It
// refuses absolute paths, NUL bytes, empty, "." and ".." components, and any
// path whose real parent directory is outside the real repository root, so a
// directory symlink cannot redirect the write elsewhere.
//
// The parent is resolved with EvalSymlinks and every file operation afterwards
// uses that real directory. A directory swapped for a symlink between this
// check and the rename is a local race this function cannot rule out; the
// rename then replaces a link in the real parent rather than following it, and
// ErrResolvePath keeps the ordinary case out.
func resolveWorktreePath(repo domain.Repo, p string) (string, error) {
	if p == "" || strings.ContainsRune(p, 0) || filepath.IsAbs(p) || filepath.VolumeName(p) != "" {
		return "", fmt.Errorf("%w: %q", ErrResolvePath, p)
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%w: %q", ErrResolvePath, p)
		}
	}
	root, err := filepath.EvalSymlinks(repo.Root)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrResolvePath, p)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrResolvePath, p)
	}
	root = filepath.Clean(root)
	abs := filepath.Join(root, filepath.FromSlash(p))
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrResolveMissing, p)
		}
		return "", fmt.Errorf("%w: %q", ErrResolvePath, p)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrResolvePath, p)
	}
	parent = filepath.Clean(parent)
	if parent != root && !strings.HasPrefix(parent, root+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrResolvePath, p)
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

// unmergedPath reports whether Git still lists path in a conflict stage.
func (r *Runner) unmergedPath(ctx context.Context, repo domain.Repo, path string) (bool, error) {
	out, err := r.Run(ctx, repo.Root, "ls-files", "-u", "-z", "--", ":(literal)"+path)
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// checkResolvableFile refuses working-tree entries that have no text blocks
// the resolver could safely rewrite.
func checkResolvableFile(path string, c Content) error {
	switch {
	case !c.Exists:
		return fmt.Errorf("%w: %s", ErrResolveMissing, path)
	case c.Symlink, c.Submodule:
		return fmt.Errorf("%w: %s", ErrResolveUnsupported, path)
	case c.TooLarge:
		return fmt.Errorf("%w: %s", ErrResolveTooLarge, path)
	case c.Binary:
		return fmt.Errorf("%w: %s", ErrResolveBinary, path)
	}
	return nil
}

// countConflictBlocks counts the blocks domain.ParseConflictMarkers sees,
// without caring whether they are reliable.
func countConflictBlocks(text string) int {
	lines, _ := domain.SplitLines(text)
	return len(domain.ParseConflictMarkers(lines).Blocks)
}
