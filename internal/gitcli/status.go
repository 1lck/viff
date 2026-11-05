package gitcli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"viff/internal/domain"
)

// Discover finds the repository containing dir.
func (r *Runner) Discover(ctx context.Context, dir string) (domain.Repo, error) {
	out, err := r.Run(ctx, dir, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	if err != nil {
		return domain.Repo{}, err
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) < 3 {
		return domain.Repo{}, &Error{Kind: ErrNotRepository, Args: []string{"rev-parse"}, Stderr: "not a git repository (bare repositories are not supported)"}
	}
	repo := domain.Repo{Root: filepath.Clean(lines[0]), GitDir: filepath.Clean(lines[1]), CommonDir: filepath.Clean(lines[2])}
	return repo, nil
}

// Status runs git status --porcelain=v2 and parses it. Ignored files are not
// listed.
func (r *Runner) Status(ctx context.Context, repo domain.Repo) (domain.Status, error) {
	out, err := r.Run(ctx, repo.Root, "status", "--porcelain=v2", "-z", "--branch", "--show-stash", "--untracked-files=all", "--ignore-submodules=dirty")
	if err != nil {
		return domain.Status{}, err
	}
	return ParseStatus(out), nil
}

// HeadState is what HEAD names right now: the commit and the branch checked
// out in this worktree. Both are empty before the first commit; Branch is also
// empty while HEAD is detached.
type HeadState struct {
	Head   string
	Branch string
}

// Head reads HEAD's commit and branch without scanning the working tree. It is
// the identity query behind undo records; Status stays the source for changes.
// An unborn branch is a state, not a failure: it returns an empty Head.
func (r *Runner) Head(ctx context.Context, repo domain.Repo) (HeadState, error) {
	var hs HeadState
	out, err := r.Run(ctx, repo.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	switch {
	case err == nil:
		hs.Branch = strings.TrimSpace(string(out))
	case exitCodeOf(err) != 1:
		// symbolic-ref exits 1 for a detached HEAD; anything else is real.
		return HeadState{}, err
	}
	sha, err := r.Run(ctx, repo.Root, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		if hs.Branch != "" && exitCodeOf(err) == 1 {
			return hs, nil
		}
		return HeadState{}, err
	}
	hs.Head = strings.TrimSpace(string(sha))
	return hs, nil
}

// exitCodeOf is the process exit code of a Git failure, -1 when the error did
// not come from a exited process. Callers use it for the "no" answers Git
// signals with exit status 1 (detached HEAD, unborn branch).
func exitCodeOf(err error) int {
	var ge *Error
	if errors.As(err, &ge) {
		return ge.ExitCode
	}
	return -1
}

// ParseStatus parses NUL-separated porcelain v2 output. Paths are kept as
// raw bytes converted to string, never unquoted or re-escaped, so they can be
// passed back to Git exactly.
func ParseStatus(out []byte) domain.Status {
	var st domain.Status
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		f := string(fields[i])
		if f == "" {
			continue
		}
		switch f[0] {
		case '#':
			parseBranchHeader(&st.Branch, f)
		case '1':
			// 1 XY sub mH mI mW hH hI path
			p := strings.SplitN(f, " ", 9)
			if len(p) < 9 {
				continue
			}
			st.Entries = append(st.Entries, domain.StatusEntry{
				Path: p[8], Index: p[1][0], Worktree: p[1][1], Submodule: p[2],
				ModeHead: p[3], ModeIndex: p[4], ModeWork: p[5],
			})
		case '2':
			// 2 XY sub mH mI mW hH hI Xscore path NUL origPath
			p := strings.SplitN(f, " ", 10)
			if len(p) < 10 {
				continue
			}
			e := domain.StatusEntry{
				Path: p[9], Index: p[1][0], Worktree: p[1][1], Submodule: p[2],
				ModeHead: p[3], ModeIndex: p[4], ModeWork: p[5],
			}
			if i+1 < len(fields) {
				i++
				e.OrigPath = string(fields[i])
			}
			st.Entries = append(st.Entries, e)
		case 'u':
			// u XY sub m1 m2 m3 mW h1 h2 h3 path
			p := strings.SplitN(f, " ", 11)
			if len(p) < 11 {
				continue
			}
			st.Entries = append(st.Entries, domain.StatusEntry{
				Path: p[10], Index: p[1][0], Worktree: p[1][1], Submodule: p[2], ModeWork: p[6],
			})
		case '?':
			if len(f) > 2 {
				st.Entries = append(st.Entries, domain.StatusEntry{Path: f[2:], Index: '?', Worktree: '?'})
			}
		case '!':
			if len(f) > 2 {
				st.Entries = append(st.Entries, domain.StatusEntry{Path: f[2:], Index: '!', Worktree: '!'})
			}
		}
	}
	return st
}

func parseBranchHeader(b *domain.BranchStatus, line string) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return
	}
	switch parts[1] {
	case "branch.oid":
		if parts[2] != "(initial)" {
			b.Head = parts[2]
		}
	case "branch.head":
		if parts[2] == "(detached)" {
			b.Detached = true
		} else {
			b.Branch = parts[2]
		}
	case "branch.upstream":
		b.Upstream = parts[2]
	case "branch.ab":
		ab := strings.Fields(parts[2])
		if len(ab) == 2 {
			b.Ahead, _ = strconv.Atoi(strings.TrimPrefix(ab[0], "+"))
			b.Behind, _ = strconv.Atoi(strings.TrimPrefix(ab[1], "-"))
			b.HasAB = true
		}
	case "stash":
		b.Stashes, _ = strconv.Atoi(parts[2])
	}
}

// Operation reads the in-progress operation from the Git directory.
func (r *Runner) Operation(ctx context.Context, repo domain.Repo) domain.Operation {
	gd := repo.GitDir
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(gd, name))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(gd, name))
		return err == nil
	}
	var op domain.Operation
	switch {
	case exists("rebase-merge"):
		op.Kind = domain.OpRebase
		op.HeadName = strings.TrimPrefix(read("rebase-merge/head-name"), "refs/heads/")
		op.Onto = read("rebase-merge/onto")
		op.Step, _ = strconv.Atoi(read("rebase-merge/msgnum"))
		op.Total, _ = strconv.Atoi(read("rebase-merge/end"))
		op.Interactive = exists("rebase-merge/interactive")
		op.Incoming = read("rebase-merge/stopped-sha")
		if h := read("REBASE_HEAD"); h != "" {
			op.Incoming = h
		}
	case exists("rebase-apply"):
		if exists("rebase-apply/applying") {
			op.Kind = domain.OpApplyMailbox
		} else {
			op.Kind = domain.OpRebase
			op.HeadName = strings.TrimPrefix(read("rebase-apply/head-name"), "refs/heads/")
			op.Onto = read("rebase-apply/onto")
		}
		op.Step, _ = strconv.Atoi(read("rebase-apply/next"))
		op.Total, _ = strconv.Atoi(read("rebase-apply/last"))
		op.Incoming = read("REBASE_HEAD")
	case exists("MERGE_HEAD"):
		op.Kind = domain.OpMerge
		heads := strings.Fields(read("MERGE_HEAD"))
		if len(heads) > 0 {
			op.Incoming = heads[0]
		}
		op.MergeMessage = read("MERGE_MSG")
	case exists("CHERRY_PICK_HEAD"):
		op.Kind = domain.OpCherryPick
		op.Incoming = read("CHERRY_PICK_HEAD")
		op.MergeMessage = read("MERGE_MSG")
	case exists("REVERT_HEAD"):
		op.Kind = domain.OpRevert
		op.Incoming = read("REVERT_HEAD")
		op.MergeMessage = read("MERGE_MSG")
	case exists("BISECT_LOG"):
		op.Kind = domain.OpBisect
	}
	if op.Incoming != "" && op.Kind != domain.OpBisect {
		if out, err := r.Run(ctx, repo.Root, "log", "-1", "--format=%s", op.Incoming, "--"); err == nil {
			op.IncomingSubject = strings.TrimSpace(string(out))
		}
	}
	return op
}

// Conflicts lists unmerged paths with their index stages.
func (r *Runner) Conflicts(ctx context.Context, repo domain.Repo, status domain.Status) ([]domain.Conflict, error) {
	var paths []domain.Conflict
	idx := map[string]int{}
	for _, e := range status.Entries {
		if e.Conflicted() {
			idx[e.Path] = len(paths)
			paths = append(paths, domain.Conflict{Path: e.Path, Kind: domain.ConflictKindOf(e.Index, e.Worktree)})
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}
	out, err := r.Run(ctx, repo.Root, "ls-files", "-u", "-z")
	if err != nil {
		return paths, err
	}
	for _, rec := range bytes.Split(out, []byte{0}) {
		// mode SP hash SP stage TAB path
		s := string(rec)
		tab := strings.IndexByte(s, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(s[:tab])
		if len(meta) != 3 {
			continue
		}
		i, ok := idx[s[tab+1:]]
		if !ok {
			continue
		}
		n, _ := strconv.Atoi(meta[2])
		paths[i].Stages = append(paths[i].Stages, domain.Stage{Number: n, Mode: meta[0], Hash: meta[1]})
	}
	return paths, nil
}
