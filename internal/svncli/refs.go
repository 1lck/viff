package svncli

import (
	"context"
	"strconv"
	"strings"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// A Subversion branch is a server-side directory copy, not a reference, so the
// three reference writes below are `svn copy`, `svn delete` and `svn mv`
// against URLs. Each one is its own commit, which is why every call carries a
// message: there is no later "push" that could name the change.
//
// Implementing the interface (rather than leaving it absent) is what makes the
// New Branch dialog work for a working copy instead of opening and doing
// nothing, and it is the capability the UI already gates the entries on.
//
// Only the branch slice is implemented: Subversion has branches, but no tags as
// Viff models them, no remotes and no refspecs, so vcs.ReferenceWriter stays
// out of reach on purpose.
var _ vcs.BranchWriter = (*Runner)(nil)

// CreateBranch copies the start revision of a branch to branches/<name>.
//
// start is what the caller had in mind as the base: empty for the current
// branch, a revision number, or a branch name. Subversion has no commit ids to
// resolve, so a number is passed to copy -r and a name selects that directory.
func (r *Runner) CreateBranch(ctx context.Context, repo domain.Repo, name, start string, checkout, force bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return notSupported("an empty branch name")
	}
	dst, err := r.branchURL(repo, name)
	if err != nil {
		return err
	}
	src, rev, err := r.branchSource(repo, start)
	if err != nil {
		return err
	}
	if force {
		// Overwrite means "replace whatever is there": svn spells that as a
		// delete followed by a copy. A missing target is not a failure here.
		_, _ = r.Run(ctx, "", "delete", "--force", "-m", "Overwrite branch "+name, dst)
	}
	args := []string{"copy", "-m", "Create branch " + name}
	if rev != "" {
		args = append(args, "-r", rev)
	}
	args = append(args, src, dst)
	if _, err := r.Run(ctx, "", args...); err != nil {
		return err
	}
	invalidateLog(repo)
	if checkout {
		// Checking out the new branch is `svn switch`: the working copy moves
		// to the new URL. Local modifications are carried over by svn itself.
		if _, err := r.Run(ctx, repo.Root, "switch", dst); err != nil {
			return err
		}
	}
	return nil
}

// DeleteBranch removes branches/<name> from the repository. trash is not used:
// Subversion deletes in place, and the revision history keeps the content.
func (r *Runner) DeleteBranch(ctx context.Context, repo domain.Repo, name string, force bool) error {
	name = strings.TrimSpace(name)
	if name == "" || name == "trunk" {
		return notSupported("deleting " + name)
	}
	target, err := r.branchURL(repo, name)
	if err != nil {
		return err
	}
	args := []string{"delete", "-m", "Delete branch " + name}
	if force {
		args = append(args, "--force")
	}
	args = append(args, target)
	if _, err := r.Run(ctx, "", args...); err != nil {
		return err
	}
	invalidateLog(repo)
	return nil
}

// RenameBranch moves branches/<old> to branches/<name> server-side.
func (r *Runner) RenameBranch(ctx context.Context, repo domain.Repo, old, name string) error {
	old, name = strings.TrimSpace(old), strings.TrimSpace(name)
	if old == "" || name == "" || old == "trunk" || name == "trunk" {
		return notSupported("renaming " + old + " to " + name)
	}
	src, err := r.branchURL(repo, old)
	if err != nil {
		return err
	}
	dst, err := r.branchURL(repo, name)
	if err != nil {
		return err
	}
	if _, err := r.Run(ctx, "", "mv", "-m", "Rename branch "+old+" to "+name, src, dst); err != nil {
		return err
	}
	invalidateLog(repo)
	return nil
}

// branchSource resolves the base URL and revision of a new branch.
func (r *Runner) branchSource(repo domain.Repo, start string) (string, string, error) {
	current, err := r.branchURL(repo, branchFromRelURL(repo.RelURL))
	if err != nil {
		return "", "", err
	}
	start = strings.TrimSpace(start)
	// No base named, or a base that is a commit id (Git-shaped callers pass a
	// hash, which means nothing here): branch off the working copy's branch.
	if start == "" {
		return current, "", nil
	}
	if _, convErr := strconv.Atoi(start); convErr == nil {
		return current, start, nil
	}
	if name, ok := strings.CutPrefix(start, "branches/"); ok {
		url, err := r.branchURL(repo, name)
		return url, "", err
	}
	if name, ok := strings.CutPrefix(start, "tags/"); ok {
		return r.urlFor(repo, "tags", name)
	}
	if start == "trunk" {
		url, err := r.branchURL(repo, "trunk")
		return url, "", err
	}
	if isBranchName(start) {
		url, err := r.branchURL(repo, start)
		return url, "", err
	}
	return current, "", nil
}

// urlFor builds the URL of a directory under a conventional layout folder.
func (r *Runner) urlFor(repo domain.Repo, dir, name string) (string, string, error) {
	if repo.RepoRoot == "" {
		return "", "", notSupported("a repository without a URL")
	}
	return strings.TrimSuffix(repo.RepoRoot, "/") + "/" + dir + "/" + name, "", nil
}

// isBranchName reports whether start looks like a branch directory name rather
// than a revision expression or a commit id.
func isBranchName(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t^@:") {
		return false
	}
	// A hex string of the length a Git hash has is a commit id, not a branch.
	if len(s) >= 7 && len(s) <= 40 && isHex(s) {
		return false
	}
	return true
}

func isHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
