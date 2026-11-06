package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"viff/internal/domain"
)

// LFSStatus is what Viff knows about Git LFS in a repository. Viff detects LFS
// and shows its facts; it never installs, configures or manages it (ADR 0031).
//
// Managed means the repository uses LFS: either an attribute rule selects
// filter=lfs, or git-lfs itself lists managed files. A repository with no
// rules and no listed files is simply not an LFS repository.
type LFSStatus struct {
	// Installed reports that `git lfs version` succeeded. A missing git-lfs is
	// an expected state, not an error.
	Installed bool
	// Version is the first line of `git lfs version`, e.g. "git-lfs/3.5.1
	// (GitHub; darwin arm64; go 1.22.3)".
	Version string
	// Patterns are the .gitattributes patterns whose filter attribute selects
	// lfs, in file order. Patterns from a nested .gitattributes are prefixed
	// with that file's directory so they read as repository-relative.
	Patterns []string
	// Files are the managed files `git lfs ls-files --name-only` reports,
	// unchanged and in Git's order. It stays empty when git-lfs is missing.
	Files []string
	// Managed reports that the repository uses LFS.
	Managed bool
	// Err records why this snapshot is incomplete: an unreadable
	// .gitattributes or a failing `git lfs ls-files`. It is diagnostic only;
	// Patterns, Files and Managed still hold what the successful parts found.
	// Callers log the classified code, never the message.
	Err error
}

// NeedsInstall reports that the repository uses LFS rules but this machine has
// no git-lfs. It is the condition for the UI's warning.
func (st LFSStatus) NeedsInstall() bool {
	return !st.Installed && len(st.Patterns) > 0
}

// Limits keep a pathological repository from turning detection into an
// unbounded read or an unbounded pattern list.
const (
	lfsAttributesMaxSize = 1 << 20 // 1 MiB per .gitattributes file
	lfsPatternLimit      = 512
)

// LFS inspects repo for Git LFS facts. It never returns an error: a missing
// git-lfs is a normal state, and a failed file listing leaves Files empty with
// Err set. Attribute parsing does not need git-lfs, which is what lets Viff
// warn about a repository it cannot yet fetch LFS objects for.
func (r *Runner) LFS(ctx context.Context, repo domain.Repo) LFSStatus {
	var st LFSStatus
	version, installed := r.lfsCapability(ctx, repo)
	st.Installed, st.Version = installed, version
	patterns, err := r.lfsPatterns(ctx, repo)
	st.Patterns = patterns
	st.Err = err
	if st.Installed {
		files, ferr := r.lfsFiles(ctx, repo)
		if ferr != nil {
			st.Err = errors.Join(st.Err, ferr)
		} else {
			st.Files = files
		}
	}
	st.Managed = len(st.Patterns) > 0 || len(st.Files) > 0
	return st
}

// lfsCapability probes `git lfs version` once per Git executable and process.
// The probe runs in the repository so a WSL repository probes the distro's
// git-lfs, not the Windows one; the cache key follows the executable that
// would serve the repo. A canceled probe is not cached: it says nothing about
// the installation.
func (r *Runner) lfsCapability(ctx context.Context, repo domain.Repo) (string, bool) {
	key := "custom"
	if r.LFSCommand == nil {
		if distro := wslDistroOf(repo.Root); distro != "" {
			key = "wsl:" + distro
		} else {
			git, err := r.git()
			if err != nil {
				// Without git no repository can be opened in the first place.
				return "", false
			}
			key = git
		}
	}
	r.lfsMu.Lock()
	if r.lfsProbed && r.lfsProbeKey == key {
		version, installed := r.lfsVersion, r.lfsInstalled
		r.lfsMu.Unlock()
		return version, installed
	}
	r.lfsMu.Unlock()

	out, err := r.lfsCommand(ctx, repo.Root, "version")
	if ctx.Err() != nil {
		return "", false
	}
	installed := err == nil
	version := ""
	if installed {
		version = lfsVersionLine(out)
	}
	r.lfsMu.Lock()
	r.lfsProbeKey, r.lfsProbed = key, true
	r.lfsInstalled, r.lfsVersion = installed, version
	r.lfsMu.Unlock()
	return version, installed
}

// lfsCommand runs a git-lfs subcommand. LFSCommand is the test seam: it lets a
// test simulate git-lfs being installed, missing or broken without the binary.
func (r *Runner) lfsCommand(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if r.LFSCommand != nil {
		return r.LFSCommand(ctx, dir, args...)
	}
	return r.Run(ctx, dir, append([]string{"lfs"}, args...)...)
}

func lfsVersionLine(out []byte) string {
	s := strings.TrimSpace(string(out))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	const limit = 160
	if len(s) > limit {
		s = s[:limit]
	}
	return s
}

func (r *Runner) lfsFiles(ctx context.Context, repo domain.Repo) ([]string, error) {
	// -z keeps paths with spaces, quotes, newlines or non-ASCII bytes intact,
	// the same rule the other list commands follow. An older git-lfs that
	// rejects the flag falls back to the documented newline form instead of
	// reporting the repository as having no managed files.
	out, err := r.lfsCommand(ctx, repo.Root, "ls-files", "--name-only", "-z")
	if err != nil && ctx.Err() == nil {
		out, err = r.lfsCommand(ctx, repo.Root, "ls-files", "--name-only")
	}
	if err != nil {
		return nil, fmt.Errorf("list Git LFS files: %w", err)
	}
	return parseLFSFiles(out), nil
}

// parseLFSFiles splits `git lfs ls-files --name-only` output. Newer git-lfs
// writes NUL-separated paths for -z; an older or shimmed one may write lines,
// so both are accepted without trimming anything from a path.
func parseLFSFiles(out []byte) []string {
	var files []string
	if bytes.IndexByte(out, 0) >= 0 {
		for _, rec := range bytes.Split(out, []byte{0}) {
			if len(rec) > 0 {
				files = append(files, string(rec))
			}
		}
		return files
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// lfsPatterns finds filter=lfs rules without asking git-lfs. The root
// .gitattributes is always read directly, so detection still works when the
// listing fails; other .gitattributes files join through `git ls-files` so
// nested `git lfs track` rules and not-yet-committed attribute files count.
func (r *Runner) lfsPatterns(ctx context.Context, repo domain.Repo) ([]string, error) {
	paths := []string{".gitattributes"}
	var err error
	out, listErr := r.Run(ctx, repo.Root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ":(glob)**/.gitattributes")
	if listErr != nil {
		err = fmt.Errorf("list .gitattributes files: %w", listErr)
		if ctx.Err() != nil {
			return nil, err
		}
	} else {
		for _, rec := range bytes.Split(out, []byte{0}) {
			if p := string(rec); p != "" && p != ".gitattributes" {
				paths = append(paths, p)
			}
		}
	}
	sort.Strings(paths)
	var patterns []string
	seen := map[string]bool{}
	for _, rel := range paths {
		data, readErr := readLFSAttributes(repo.Root, rel)
		if readErr != nil {
			err = errors.Join(err, readErr)
			continue
		}
		for _, p := range parseLFSAttributes(data, lfsAttributeDir(rel)) {
			if seen[p] {
				continue
			}
			seen[p] = true
			patterns = append(patterns, p)
			if len(patterns) >= lfsPatternLimit {
				return patterns, err
			}
		}
	}
	return patterns, err
}

// readLFSAttributes reads one .gitattributes relative to the repository root.
// A missing file is not an error: most repositories have none.
func readLFSAttributes(root, rel string) ([]byte, error) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read .gitattributes: %w", err)
	}
	if info.Size() > lfsAttributesMaxSize {
		return nil, fmt.Errorf("read .gitattributes: file is larger than %d bytes", lfsAttributesMaxSize)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read .gitattributes: %w", err)
	}
	return data, nil
}

// lfsAttributeDir returns the repository-relative directory a .gitattributes
// file lives in, "" for the root file. Patterns in a nested file apply there.
func lfsAttributeDir(rel string) string {
	dir := path.Dir(rel)
	if dir == "." {
		return ""
	}
	return dir
}

// parseLFSAttributes returns the patterns whose final filter attribute is lfs.
// Within one file the last assignment wins, matching how Git reads a repeated
// pattern; patterns from different files are scoped by dir and merged by the
// caller.
func parseLFSAttributes(data []byte, dir string) []string {
	var order []string
	state := map[string]bool{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := splitAttributeFields(trimmed)
		if len(fields) < 2 {
			continue
		}
		pattern := lfsAttributePattern(dir, fields[0])
		if pattern == "" {
			continue
		}
		mark := func() {
			if _, ok := state[pattern]; !ok {
				order = append(order, pattern)
			}
		}
		for _, attr := range fields[1:] {
			switch {
			case strings.HasPrefix(attr, "filter="):
				mark()
				state[pattern] = strings.TrimPrefix(attr, "filter=") == "lfs"
			case attr == "filter" || attr == "-filter" || attr == "!filter":
				// `filter` sets the attribute to true (not the lfs driver) and
				// the other two unset it; none of them selects Git LFS.
				mark()
				state[pattern] = false
			}
		}
	}
	var out []string
	for _, p := range order {
		if state[p] {
			out = append(out, p)
		}
	}
	return out
}

func lfsAttributePattern(dir, pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	if dir == "" {
		return pattern
	}
	return dir + "/" + strings.TrimPrefix(pattern, "/")
}

// splitAttributeFields splits one attributes line. A pattern with spaces is
// C-quoted (`"model final.bin"`); backslash escapes the next byte, and tabs
// separate fields as well as spaces. Attribute values are ordinary fields.
func splitAttributeFields(line string) []string {
	var fields []string
	var cur strings.Builder
	started, quoted, escaped := false, false, false
	flush := func() {
		if started {
			fields = append(fields, cur.String())
			cur.Reset()
			started = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escaped {
			switch c {
			case 'n':
				cur.WriteByte('\n')
			case 't':
				cur.WriteByte('\t')
			case 'r':
				cur.WriteByte('\r')
			default:
				cur.WriteByte(c)
			}
			escaped = false
			started = true
			continue
		}
		switch {
		case c == '\\':
			escaped = true
			started = true
		case c == '"':
			quoted = !quoted
			started = true
		case (c == ' ' || c == '\t') && !quoted:
			flush()
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	flush()
	return fields
}
