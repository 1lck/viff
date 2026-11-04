package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"viff/internal/domain"
)

// ErrSubmoduleNeedsForce marks a deinit or removal Git (or the pre-check here)
// refused because the submodule has local changes. The caller must confirm
// discarding them before trying again with force; no operation discards a
// submodule's local state on its own.
var ErrSubmoduleNeedsForce = errors.New("the submodule has local changes; discarding them needs explicit confirmation")

// CleanSubmodulePath validates a repository-relative submodule path and
// returns it with forward slashes. A submodule lives inside the checkout:
// absolute paths, "..", empty components and a .git component are refused, so
// a path typed by a user can never name something outside the repository.
func CleanSubmodulePath(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", errors.New("submodule path is empty")
	}
	if strings.ContainsAny(p, "\x00\n\r") {
		return "", fmt.Errorf("submodule path %q contains a control character", path)
	}
	if filepath.IsAbs(p) || filepath.VolumeName(p) != "" || strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("submodule path %q must be relative to the repository", path)
	}
	p = filepath.ToSlash(p)
	for _, part := range strings.Split(p, "/") {
		switch part {
		case "":
			return "", fmt.Errorf("submodule path %q has an empty component", path)
		case ".", "..":
			return "", fmt.Errorf("submodule path %q must not contain %q", path, part)
		case ".git":
			return "", fmt.Errorf("submodule path %q must not contain a .git component", path)
		}
	}
	if p == ".gitmodules" {
		return "", fmt.Errorf("submodule path %q is reserved for the submodule list", path)
	}
	return p, nil
}

// submoduleDir returns the absolute directory of a submodule inside repo. The
// path is validated first, then re-checked after joining so a path can never
// escape the checkout.
func submoduleDir(repo domain.Repo, path string) (string, error) {
	rel, err := CleanSubmodulePath(path)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(repo.Root)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if dir == root || !strings.HasPrefix(dir, root+string(filepath.Separator)) {
		return "", fmt.Errorf("submodule path %q is outside the repository", path)
	}
	return dir, nil
}

// AddSubmodule registers url at path and checks it out. Git writes
// .gitmodules and stages the submodule entry; committing stays with the user.
// The URL is passed after "--" so it can never be read as an option. The file
// transport is left to the user's Git configuration: Viff never enables it.
func (r *Runner) AddSubmodule(ctx context.Context, repo domain.Repo, url, path string, progress func(string)) error {
	u := strings.TrimSpace(url)
	switch {
	case u == "":
		return errors.New("submodule URL is empty")
	case strings.HasPrefix(u, "-"):
		return fmt.Errorf("submodule URL %q must not start with '-'", u)
	}
	rel, err := CleanSubmodulePath(path)
	if err != nil {
		return err
	}
	if _, err := submoduleDir(repo, rel); err != nil {
		return err
	}
	// --literal-pathspecs keeps a path such as "deps/lib[1]" literal instead
	// of letting Git treat the brackets as a glob.
	_, err = r.RunProgress(ctx, repo.Root, progress, "--literal-pathspecs", "submodule", "add", "--", u, rel)
	return err
}

// SubmoduleDirty reports whether deinitializing or removing the submodule
// would discard local work: its checkout differs from the index, the
// superproject sees a change to it, or its working tree has modified or
// untracked files. An uninitialized submodule has no working tree and is never
// dirty.
func (r *Runner) SubmoduleDirty(ctx context.Context, repo domain.Repo, path string) (bool, error) {
	sm, err := r.submodule(ctx, repo, path)
	if err != nil {
		return false, err
	}
	if !sm.Initialized() {
		return false, nil
	}
	if sm.Changed() {
		return true, nil
	}
	// The superproject reports a changed gitlink (a staged commit, a different
	// checkout, modified content). --ignore-submodules=none makes Git report
	// it even when the configuration would hide it. The whole status is read
	// rather than one pathspec because a nested submodule has no pathspec of
	// its own in this repository.
	out, err := r.Run(ctx, repo.Root, "status", "--porcelain=v2", "-z", "--ignore-submodules=none", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	for _, e := range ParseStatus(out).Entries {
		if e.Path == sm.Path {
			return true, nil
		}
	}
	return r.submoduleWorktreeDirty(ctx, repo, sm)
}

// submoduleWorktreeDirty inspects the submodule's own working tree.
func (r *Runner) submoduleWorktreeDirty(ctx context.Context, repo domain.Repo, sm domain.Submodule) (bool, error) {
	dir, err := submoduleDir(repo, sm.Path)
	if err != nil {
		return false, err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return false, nil
	}
	out, err := r.Run(ctx, dir, "status", "--porcelain=v1", "--untracked-files=normal", "--ignore-submodules=none")
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// DeinitSubmodule empties the submodule's working tree while keeping its entry
// in the index and .gitmodules; Update checks it out again. Local changes are
// refused unless force is set, and ErrSubmoduleNeedsForce says the caller must
// confirm them first. Only a submodule this repository records itself can be
// deinitialized: a nested one belongs to its parent repository.
func (r *Runner) DeinitSubmodule(ctx context.Context, repo domain.Repo, path string, force bool) error {
	sm, err := r.directSubmodule(ctx, repo, path)
	if err != nil {
		return err
	}
	if !sm.Initialized() {
		return fmt.Errorf("submodule %q is not initialized; there is nothing to deinit", sm.Path)
	}
	if err := r.refuseUnforcedDiscard(ctx, repo, sm, force, "deinit"); err != nil {
		return err
	}
	return r.deinit(ctx, repo, sm.Path, force)
}

// RemoveSubmodule removes the submodule with one git rm: the working tree,
// the gitlink in the index and the entry in .gitmodules go together, and the
// change is staged for the user to commit. Git validates the removal before
// touching anything, so a refusal (for example a locally modified .gitmodules)
// leaves the checkout as it was. Local changes are refused unless force is
// set, and ErrSubmoduleNeedsForce says the caller must confirm them first.
// Only a submodule this repository records itself can be removed.
func (r *Runner) RemoveSubmodule(ctx context.Context, repo domain.Repo, path string, force bool) error {
	sm, err := r.directSubmodule(ctx, repo, path)
	if err != nil {
		return err
	}
	if err := r.refuseUnforcedDiscard(ctx, repo, sm, force, "remove"); err != nil {
		return err
	}
	// Git's own .gitmodules cleanup matches the path as a pathspec; for a path
	// such as deps/x[1] it finds no entry and leaves the section behind even
	// though the gitlink and the working tree are removed. Read the section
	// first so the leftover can be removed deliberately below.
	name, err := r.gitmodulesSectionName(ctx, repo, sm.Path)
	if err != nil {
		return err
	}
	args := []string{"--literal-pathspecs", "rm"}
	if force {
		args = append(args, "-f")
	}
	err = r.write(ctx, repo, append(args, "--", sm.Path)...)
	if err != nil {
		if !force && gitRefusedDiscard(err) {
			return fmt.Errorf("%w: %s", ErrSubmoduleNeedsForce, errorMessage(err))
		}
		return err
	}
	leftover, err := r.gitmodulesSectionName(ctx, repo, sm.Path)
	switch {
	case err != nil:
		return fmt.Errorf("%q was removed, but .gitmodules could not be read again: %w", sm.Path, err)
	case leftover == "":
		return nil
	}
	if name == "" {
		name = leftover
	}
	if err := r.dropGitmodulesSection(ctx, repo, name, sm.Path); err != nil {
		return fmt.Errorf("%q was removed, but .gitmodules still records it: %w", sm.Path, err)
	}
	return nil
}

// gitmodulesSectionName returns the .gitmodules section name that records
// path, "" when no section does. Values are read with -z so a path with spaces
// stays one value.
func (r *Runner) gitmodulesSectionName(ctx context.Context, repo domain.Repo, path string) (string, error) {
	if _, err := os.Stat(filepath.Join(repo.Root, ".gitmodules")); err != nil {
		return "", nil
	}
	out, err := r.Run(ctx, repo.Root, "config", "-z", "-f", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	if err != nil {
		var ge *Error
		// Exit 1 means no matching key: a repository without submodules.
		if errors.As(err, &ge) && ge.ExitCode == 1 {
			return "", nil
		}
		return "", err
	}
	for _, record := range strings.Split(string(out), "\x00") {
		key, value, ok := strings.Cut(record, "\n")
		if !ok {
			continue
		}
		name, ok := strings.CutPrefix(key, "submodule.")
		if !ok {
			continue
		}
		name, ok = strings.CutSuffix(name, ".path")
		if !ok || value != path {
			continue
		}
		return name, nil
	}
	return "", nil
}

// dropGitmodulesSection removes the section from .gitmodules and stages the
// file, the way git rm does for paths Git handles itself. It is only used for
// the sections Git's path matching misses.
func (r *Runner) dropGitmodulesSection(ctx context.Context, repo domain.Repo, name, path string) error {
	file := filepath.Join(repo.Root, ".gitmodules")
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	out, removed := dropSubmoduleSection(data, name)
	if !removed {
		return fmt.Errorf("no section %q", name)
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(file); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.WriteFile(file, out, mode); err != nil {
		return err
	}
	return r.write(ctx, repo, "--literal-pathspecs", "add", "--", ".gitmodules")
}

// dropSubmoduleSection removes the [submodule "<name>"] block from a
// .gitmodules file. Section names are written with C-style quoting, so the
// header is decoded rather than compared textually.
func dropSubmoduleSection(data []byte, name string) ([]byte, bool) {
	lines := strings.SplitAfter(string(data), "\n")
	var out strings.Builder
	removed, dropping := false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			dropping = false
			if section, ok := submoduleHeaderName(trimmed); ok && section == name {
				dropping, removed = true, true
				continue
			}
		}
		if dropping {
			continue
		}
		out.WriteString(line)
	}
	return []byte(out.String()), removed
}

// submoduleHeaderName reads the subsection of a "[submodule \"...\"]" line.
func submoduleHeaderName(line string) (string, bool) {
	const prefix = "[submodule "
	rest, ok := strings.CutPrefix(line, prefix)
	if !ok {
		return "", false
	}
	rest, ok = strings.CutSuffix(strings.TrimSpace(rest), "]")
	if !ok {
		return "", false
	}
	return unquoteConfigString(strings.TrimSpace(rest))
}

// unquoteConfigString decodes the quoting git writes around a subsection
// name: a bare name, or a C-style quoted string with the usual escapes.
func unquoteConfigString(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	if s[0] != '"' {
		// Git leaves a subsection without special characters unquoted.
		return s, true
	}
	if len(s) < 2 || s[len(s)-1] != '"' {
		return "", false
	}
	body := s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(body) {
			return "", false
		}
		switch body[i] {
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '\\', '"':
			b.WriteByte(body[i])
		default:
			if body[i] < '0' || body[i] > '7' || i+2 >= len(body) {
				return "", false
			}
			value := int(body[i]-'0')*64 + int(body[i+1]-'0')*8 + int(body[i+2]-'0')
			b.WriteByte(byte(value))
			i += 2
		}
	}
	return b.String(), true
}

// refuseUnforcedDiscard stops an operation that would discard local work. The
// pre-check gives a clear error before Git is asked; Git is asked anyway so a
// change that appeared since the check cannot slip through, and its refusal is
// translated to ErrSubmoduleNeedsForce too.
func (r *Runner) refuseUnforcedDiscard(ctx context.Context, repo domain.Repo, sm domain.Submodule, force bool, verb string) error {
	if force || !sm.Initialized() {
		return nil
	}
	dirty, err := r.SubmoduleDirty(ctx, repo, sm.Path)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("%w: %q has uncommitted changes or a different checkout; confirm discarding them to %s it", ErrSubmoduleNeedsForce, sm.Path, verb)
	}
	return nil
}

// deinit runs git submodule deinit. A refusal caused by local changes is
// reported as ErrSubmoduleNeedsForce so the caller can ask the user.
func (r *Runner) deinit(ctx context.Context, repo domain.Repo, path string, force bool) error {
	command := "submodule"
	if runtime.GOOS == "windows" {
		// Git for Windows's submodule wrapper discards literal-pathspec
		// mode. Call its own helper directly so both selection and the
		// helper's nested git-rm safety check preserve literal paths.
		// Never work around a failed safety check by adding --force.
		command = "submodule--helper"
	}
	args := []string{"--literal-pathspecs", command, "deinit"}
	if force {
		args = append(args, "--force")
	}
	err := r.write(ctx, repo, append(args, "--", path)...)
	if err != nil && !force && gitRefusedDiscard(err) {
		return fmt.Errorf("%w: %s", ErrSubmoduleNeedsForce, errorMessage(err))
	}
	return err
}

// directSubmodule resolves path to a submodule this repository records in its
// own .gitmodules. A nested submodule is refused with the repository that owns
// it: deinitializing or removing it through this repository is not the same
// operation, and Git would only fail with a pathspec error.
func (r *Runner) directSubmodule(ctx context.Context, repo domain.Repo, path string) (domain.Submodule, error) {
	rel, err := CleanSubmodulePath(path)
	if err != nil {
		return domain.Submodule{}, err
	}
	direct, err := r.directSubmodules(ctx, repo)
	if err != nil {
		return domain.Submodule{}, err
	}
	for _, sm := range direct {
		if sm.Path == rel {
			return sm, nil
		}
	}
	all, err := r.Submodules(ctx, repo)
	if err != nil {
		return domain.Submodule{}, err
	}
	owner := ""
	for _, sm := range all {
		if sm.Path != rel && strings.HasPrefix(rel, sm.Path+"/") && len(sm.Path) > len(owner) {
			owner = sm.Path
		}
	}
	if owner != "" {
		return domain.Submodule{}, fmt.Errorf("%q is a nested submodule of %q; deinit or remove it from that repository instead", rel, owner)
	}
	return domain.Submodule{}, fmt.Errorf("%q is not a submodule of this repository", rel)
}

// DirectSubmodulePaths reports the submodule paths this repository records in
// its own .gitmodules. The list dialog offers deinit and remove only for
// these; a nested submodule must be managed in the repository that records it.
func (r *Runner) DirectSubmodulePaths(ctx context.Context, repo domain.Repo) (map[string]bool, error) {
	list, err := r.directSubmodules(ctx, repo)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]bool, len(list))
	for _, sm := range list {
		paths[sm.Path] = true
	}
	return paths, nil
}

// directSubmodules lists only the submodules this repository records itself;
// `git submodule status` without --recursive omits nested ones.
func (r *Runner) directSubmodules(ctx context.Context, repo domain.Repo) ([]domain.Submodule, error) {
	out, err := r.Run(ctx, repo.Root, "--literal-pathspecs", "-c", "core.quotePath=false", "submodule", "status")
	if err != nil {
		return nil, err
	}
	return ParseSubmodules(out), nil
}

// submodule resolves path to the submodule Git reports for it. The path must
// name a submodule of this repository, so a selected row can never act on a
// plain file or on something outside the checkout.
func (r *Runner) submodule(ctx context.Context, repo domain.Repo, path string) (domain.Submodule, error) {
	rel, err := CleanSubmodulePath(path)
	if err != nil {
		return domain.Submodule{}, err
	}
	list, err := r.Submodules(ctx, repo)
	if err != nil {
		return domain.Submodule{}, err
	}
	for _, sm := range list {
		if sm.Path == rel {
			return sm, nil
		}
	}
	return domain.Submodule{}, fmt.Errorf("%q is not a submodule of this repository", rel)
}

// gitRefusedDiscard reports whether Git's failure message says the operation
// would discard local changes and that -f/--force is needed.
func gitRefusedDiscard(err error) bool {
	var ge *Error
	if !errors.As(err, &ge) {
		return false
	}
	s := strings.ToLower(ge.Stderr + "\n" + ge.Stdout)
	return strings.Contains(s, "local modifications") ||
		strings.Contains(s, "use '-f'") || strings.Contains(s, "use --force") ||
		strings.Contains(s, "changes staged in the index") ||
		strings.Contains(s, "changes not staged for commit")
}

// errorMessage is the user-facing text of a Git failure.
func errorMessage(err error) string {
	var ge *Error
	if errors.As(err, &ge) {
		if msg := strings.TrimSpace(ge.Message()); msg != "" {
			return msg
		}
	}
	return err.Error()
}

// Submodules lists the repository's submodules, including nested ones, with
// the commit each has checked out and its state. An uninitialized submodule
// reports the commit recorded in the superproject instead.
func (r *Runner) Submodules(ctx context.Context, repo domain.Repo) ([]domain.Submodule, error) {
	// core.quotePath=false keeps non-ASCII paths readable; the parser needs
	// no C-style unquoting then. --literal-pathspecs matters even without a
	// path argument: Git matches submodule paths as pathspecs internally, so
	// with a sibling such as deps/x1 a path deps/x[1] would otherwise be
	// compared against the wrong entry and reported modified.
	out, err := r.Run(ctx, repo.Root, "--literal-pathspecs", "-c", "core.quotePath=false", "submodule", "status", "--recursive")
	if err != nil {
		return nil, err
	}
	return ParseSubmodules(out), nil
}

// UpdateSubmodule checks out the commit recorded in the index. With init it
// initializes the submodule first; an empty path updates every submodule.
// Git refuses to overwrite local changes, which the caller reports as is.
func (r *Runner) UpdateSubmodule(ctx context.Context, repo domain.Repo, path string, init bool, progress func(string)) error {
	// Pass literal magic through to the submodule helper. Git for Windows's
	// shell wrapper does not preserve the global --literal-pathspecs option.
	args := []string{"submodule", "update", "--recursive"}
	if init {
		args = append(args, "--init")
	}
	if path != "" {
		args = append(args, "--", ":(literal)"+path)
	}
	_, err := r.RunProgress(ctx, repo.Root, progress, args...)
	return err
}

// SyncSubmodules copies the URLs from .gitmodules into .git/config.
func (r *Runner) SyncSubmodules(ctx context.Context, repo domain.Repo, path string) error {
	args := []string{"submodule", "sync", "--recursive"}
	if path != "" {
		args = append(args, "--", ":(literal)"+path)
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
