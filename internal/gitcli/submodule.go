package gitcli

import (
	"context"
	"strings"

	"viff/internal/domain"
)

// Submodules lists the repository's submodules, including nested ones, with
// the commit each has checked out and its state. An uninitialized submodule
// reports the commit recorded in the superproject instead.
func (r *Runner) Submodules(ctx context.Context, repo domain.Repo) ([]domain.Submodule, error) {
	// core.quotePath=false keeps non-ASCII paths readable; the parser needs
	// no C-style unquoting then.
	out, err := r.Run(ctx, repo.Root, "-c", "core.quotePath=false", "submodule", "status", "--recursive")
	if err != nil {
		return nil, err
	}
	return ParseSubmodules(out), nil
}

// UpdateSubmodule checks out the commit recorded in the index. With init it
// initializes the submodule first; an empty path updates every submodule.
// Git refuses to overwrite local changes, which the caller reports as is.
func (r *Runner) UpdateSubmodule(ctx context.Context, repo domain.Repo, path string, init bool, progress func(string)) error {
	args := []string{"submodule", "update", "--recursive"}
	if init {
		args = append(args, "--init")
	}
	if path != "" {
		args = append(args, "--", path)
	}
	_, err := r.RunProgress(ctx, repo.Root, progress, args...)
	return err
}

// SyncSubmodules copies the URLs from .gitmodules into .git/config.
func (r *Runner) SyncSubmodules(ctx context.Context, repo domain.Repo, path string) error {
	args := []string{"submodule", "sync", "--recursive"}
	if path != "" {
		args = append(args, "--", path)
	}
	return r.write(ctx, repo, args...)
}

// ParseSubmodules reads `git submodule status --recursive` output. Each line
// is "<state><sha> <path>" optionally followed by " (<describe>)". The path
// may contain spaces; the description is the last parenthesized suffix.
func ParseSubmodules(out []byte) []domain.Submodule {
	var list []domain.Submodule
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if sm, ok := parseSubmoduleLine(line); ok {
			list = append(list, sm)
		}
	}
	return list
}

func parseSubmoduleLine(line string) (domain.Submodule, bool) {
	if line == "" {
		return domain.Submodule{}, false
	}
	state := domain.SubmoduleState(line[0])
	switch state {
	case domain.SubmoduleClean, domain.SubmoduleUninitialized, domain.SubmoduleModified, domain.SubmoduleConflict:
	default:
		return domain.Submodule{}, false
	}
	rest := strings.TrimPrefix(line[1:], " ")
	sha, tail, ok := strings.Cut(rest, " ")
	if !ok || !isObjectName(sha) {
		return domain.Submodule{}, false
	}
	sm := domain.Submodule{SHA: sha, State: state}
	if strings.HasSuffix(tail, ")") {
		if i := strings.LastIndex(tail, " ("); i >= 0 {
			sm.Path = tail[:i]
			sm.Describe = tail[i+2 : len(tail)-1]
			return sm, sm.Path != ""
		}
	}
	sm.Path = tail
	return sm, sm.Path != ""
}

// isObjectName accepts an abbreviated or full hash; the length is not fixed
// because the repository may use SHA-256.
func isObjectName(s string) bool {
	if len(s) < 7 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
