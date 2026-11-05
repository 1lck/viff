package gitcli

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"viff/internal/domain"
)

// LogFilter narrows the log. Empty fields do not filter.
type LogFilter struct {
	// Revisions are the starting points; empty means all branches, remote
	// branches, tags and HEAD.
	Revisions []string
	Text      string // matched against messages (and hashes by the caller)
	Author    string
	Since     time.Time
	Paths     []string
	// FirstParent follows only first parents.
	FirstParent bool
}

const logFormat = "%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%cn%x1f%ct%x1f%s%x1e"

// LogArgs builds the git log arguments for a filter.
func LogArgs(f LogFilter) []string {
	args := []string{"log", "--date-order", "-z", "--format=" + logFormat, "--no-show-signature"}
	if f.Text != "" {
		args = append(args, "--regexp-ignore-case", "--fixed-strings", "--grep="+f.Text)
	}
	if f.Author != "" {
		args = append(args, "--regexp-ignore-case", "--author="+f.Author)
	}
	if !f.Since.IsZero() {
		args = append(args, "--since="+strconv.FormatInt(f.Since.Unix(), 10))
	}
	if f.FirstParent {
		args = append(args, "--first-parent")
	}
	if len(f.Revisions) == 0 {
		args = append(args, "--branches", "--remotes", "--tags", "HEAD")
	} else {
		args = append(args, f.Revisions...)
	}
	args = append(args, "--")
	args = append(args, f.Paths...)
	return args
}

// LogReader streams commits from one git log process. The process blocks on
// its pipe between pages, so a long history is read only as far as the user
// scrolls, without re-traversing from the start for each page.
type LogReader struct {
	stream *Stream
	reader *bufio.Reader
	done   bool
	err    error
}

// OpenLog starts the log for a filter.
func (r *Runner) OpenLog(ctx context.Context, repo domain.Repo, f LogFilter) (*LogReader, error) {
	s, err := r.Stream(ctx, repo.Root, LogArgs(f)...)
	if err != nil {
		return nil, err
	}
	return &LogReader{stream: s, reader: bufio.NewReaderSize(s, 64<<10)}, nil
}

// Next reads up to n more commits. done is true at the end of history.
func (l *LogReader) Next(n int) (commits []domain.Commit, done bool, err error) {
	if l.done {
		return nil, true, l.err
	}
	for len(commits) < n {
		rec, rerr := l.reader.ReadBytes(0x1e)
		if len(rec) > 0 {
			rec = bytes.TrimSuffix(rec, []byte{0x1e})
			// With -z every record after the first starts with a NUL.
			rec = bytes.TrimLeft(rec, "\x00\n")
			if c, ok := parseCommitRecord(rec); ok {
				commits = append(commits, c)
			}
		}
		if rerr != nil {
			l.done = true
			l.err = l.stream.Wait()
			if l.err != nil && strings.Contains(l.err.Error(), "does not have any commits") {
				l.err = nil
			}
			if l.err != nil && KindOf(l.err) == ErrNotFound {
				// HEAD missing in an empty repository.
				l.err = nil
			}
			return commits, true, l.err
		}
	}
	return commits, false, nil
}

// Close stops the log process.
func (l *LogReader) Close() {
	if !l.done {
		l.done = true
		_ = l.stream.Close()
	}
}

func parseCommitRecord(rec []byte) (domain.Commit, bool) {
	f := strings.Split(string(rec), "\x1f")
	if len(f) < 8 {
		return domain.Commit{}, false
	}
	c := domain.Commit{Hash: f[0], Author: f[2], AuthorEmail: f[3], Committer: f[5], Subject: f[7]}
	if f[1] != "" {
		c.Parents = strings.Fields(f[1])
	}
	if t, err := strconv.ParseInt(f[4], 10, 64); err == nil {
		c.AuthorTime = time.Unix(t, 0)
	}
	if t, err := strconv.ParseInt(f[6], 10, 64); err == nil {
		c.CommitTime = time.Unix(t, 0)
	}
	return c, c.Hash != ""
}

// CommitDetail loads the full message and the changes against the first
// parent (or the empty tree for a root commit).
func (r *Runner) CommitDetail(ctx context.Context, repo domain.Repo, hash string) (domain.CommitDetail, error) {
	out, err := r.Run(ctx, repo.Root, "show", "-s", "--format="+strings.TrimSuffix(logFormat, "%x1e")+"%x1f%B", hash, "--")
	if err != nil {
		return domain.CommitDetail{}, err
	}
	parts := strings.SplitN(string(out), "\x1f", 9)
	var d domain.CommitDetail
	if c, ok := parseCommitRecord([]byte(strings.Join(parts[:min(8, len(parts))], "\x1f"))); ok {
		d.Commit = c
	}
	if len(parts) == 9 {
		d.Body = strings.TrimRight(parts[8], "\n")
	}
	args := []string{"diff-tree", "-r", "-z", "--no-commit-id", "--name-status", "-M", "--root"}
	if len(d.Parents) > 1 {
		args = append(args, d.Parents[0], hash)
	} else {
		args = append(args, hash)
	}
	out, err = r.Run(ctx, repo.Root, append(args, "--")...)
	if err != nil {
		return d, err
	}
	d.Changes = ParseNameStatus(out)
	return d, nil
}

// DiffNames lists the changes between two revisions.
func (r *Runner) DiffNames(ctx context.Context, repo domain.Repo, from, to string) ([]domain.Change, error) {
	args := []string{"diff", "--name-status", "-z", "-M", from}
	if to != "" {
		args = append(args, to)
	}
	out, err := r.Run(ctx, repo.Root, append(args, "--")...)
	if err != nil {
		return nil, err
	}
	return ParseNameStatus(out), nil
}

// ParseNameStatus parses NUL-separated --name-status output.
func ParseNameStatus(out []byte) []domain.Change {
	fields := strings.Split(string(out), "\x00")
	var changes []domain.Change
	for i := 0; i < len(fields); i++ {
		code := fields[i]
		if code == "" {
			continue
		}
		c := domain.Change{}
		switch code[0] {
		case 'A':
			c.Kind = domain.ChangeAdded
		case 'D':
			c.Kind = domain.ChangeDeleted
		case 'R':
			c.Kind = domain.ChangeRenamed
		case 'C':
			c.Kind = domain.ChangeCopied
		case 'T':
			c.Kind = domain.ChangeTypeChanged
		case 'U':
			c.Kind = domain.ChangeConflicted
		case 'M':
			c.Kind = domain.ChangeModified
		default:
			continue
		}
		if c.Kind == domain.ChangeRenamed || c.Kind == domain.ChangeCopied {
			if i+2 >= len(fields) {
				break
			}
			c.OldPath, c.Path = fields[i+1], fields[i+2]
			i += 2
		} else {
			if i+1 >= len(fields) {
				break
			}
			c.Path = fields[i+1]
			i++
		}
		changes = append(changes, c)
	}
	return changes
}

// Refs lists local branches, remote branches and tags.
func (r *Runner) Refs(ctx context.Context, repo domain.Repo) ([]domain.Ref, error) {
	format := "%(refname)%00%(objectname)%00%(*objectname)%00%(upstream:short)%00%(upstream:track,nobracket)%00%(HEAD)%00%(committerdate:unix)%00%(contents:subject)"
	out, err := r.Run(ctx, repo.Root, "for-each-ref", "--format="+format, "refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, err
	}
	var refs []domain.Ref
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\x00")
		if len(f) < 8 {
			continue
		}
		ref := domain.Ref{Name: f[0], Target: f[1], Upstream: f[3], Current: f[5] == "*", Subject: f[7]}
		if f[2] != "" {
			ref.Target = f[2]
		}
		if t, err := strconv.ParseInt(f[6], 10, 64); err == nil {
			ref.Date = time.Unix(t, 0)
		}
		parseTrack(&ref, f[4])
		switch {
		case strings.HasPrefix(ref.Name, "refs/heads/"):
			ref.Kind, ref.Short = domain.RefLocal, strings.TrimPrefix(ref.Name, "refs/heads/")
		case strings.HasPrefix(ref.Name, "refs/remotes/"):
			ref.Kind, ref.Short = domain.RefRemote, strings.TrimPrefix(ref.Name, "refs/remotes/")
			if strings.HasSuffix(ref.Short, "/HEAD") {
				continue
			}
		case strings.HasPrefix(ref.Name, "refs/tags/"):
			ref.Kind, ref.Short = domain.RefTag, strings.TrimPrefix(ref.Name, "refs/tags/")
		default:
			continue
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// RemoteHeads maps each remote to the branch its HEAD points at, the default
// branch a comparison or a pull request targets. Git writes that symbolic ref
// on clone and on `git remote set-head`; a remote without one is absent from
// the map. The query stays local: it reads refs and never contacts a remote,
// because `git remote show` would go to the network.
func (r *Runner) RemoteHeads(ctx context.Context, repo domain.Repo) (map[string]string, error) {
	out, err := r.Run(ctx, repo.Root, "for-each-ref", "--format=%(refname)%00%(symref)", "refs/remotes")
	if err != nil {
		return nil, err
	}
	heads := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\x00")
		if len(f) < 2 {
			continue
		}
		// Only symbolic refs have a target; a plain ref prints an empty one.
		name, ok := strings.CutPrefix(f[0], "refs/remotes/")
		if !ok || !strings.HasSuffix(name, "/HEAD") || f[1] == "" {
			continue
		}
		remote := strings.TrimSuffix(name, "/HEAD")
		target, ok := strings.CutPrefix(f[1], "refs/remotes/")
		if !ok || !strings.HasPrefix(target, remote+"/") {
			continue
		}
		heads[remote] = strings.TrimPrefix(target, remote+"/")
	}
	return heads, nil
}

func parseTrack(ref *domain.Ref, track string) {
	if track == "gone" {
		ref.Gone = true
		return
	}
	for _, part := range strings.Split(track, ",") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, "ahead "); ok {
			ref.Ahead, _ = strconv.Atoi(v)
		}
		if v, ok := strings.CutPrefix(part, "behind "); ok {
			ref.Behind, _ = strconv.Atoi(v)
		}
	}
}

// Remotes lists configured remotes.
func (r *Runner) Remotes(ctx context.Context, repo domain.Repo) ([]domain.Remote, error) {
	out, err := r.Run(ctx, repo.Root, "remote", "-v")
	if err != nil {
		return nil, err
	}
	var remotes []domain.Remote
	index := map[string]int{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		i, ok := index[f[0]]
		if !ok {
			i = len(remotes)
			index[f[0]] = i
			remotes = append(remotes, domain.Remote{Name: f[0]})
		}
		if f[2] == "(push)" {
			remotes[i].PushURL = f[1]
		} else {
			remotes[i].FetchURL = f[1]
		}
	}
	return remotes, nil
}

// Stashes lists the stash.
func (r *Runner) Stashes(ctx context.Context, repo domain.Repo) ([]domain.Stash, error) {
	out, err := r.Run(ctx, repo.Root, "stash", "list", "-z", "--format=%gd%x1f%H%x1f%gs%x1f%ct")
	if err != nil {
		return nil, err
	}
	var list []domain.Stash
	for i, rec := range strings.Split(string(out), "\x00") {
		f := strings.Split(strings.TrimLeft(rec, "\n"), "\x1f")
		if len(f) < 4 {
			continue
		}
		s := domain.Stash{Index: i, Ref: f[0], Hash: f[1], Message: f[2]}
		if t, err := strconv.ParseInt(f[3], 10, 64); err == nil {
			s.Time = time.Unix(t, 0)
		}
		list = append(list, s)
	}
	return list, nil
}

// Reflog lists HEAD's reflog, newest first.
func (r *Runner) Reflog(ctx context.Context, repo domain.Repo, limit int) ([]domain.ReflogEntry, error) {
	out, err := r.Run(ctx, repo.Root, "reflog", "show", "-z", "-n", strconv.Itoa(limit), "--format=%H%x1f%gd%x1f%gs%x1f%ct", "HEAD", "--")
	if err != nil {
		if KindOf(err) == ErrNotFound || strings.Contains(err.Error(), "does not have any commits") {
			return nil, nil
		}
		return nil, err
	}
	var list []domain.ReflogEntry
	for _, rec := range strings.Split(string(out), "\x00") {
		f := strings.Split(strings.TrimLeft(rec, "\n"), "\x1f")
		if len(f) < 4 {
			continue
		}
		e := domain.ReflogEntry{Hash: f[0], Selector: f[1], Message: f[2]}
		if t, err := strconv.ParseInt(f[3], 10, 64); err == nil {
			e.Time = time.Unix(t, 0)
		}
		list = append(list, e)
	}
	return list, nil
}

// Worktrees lists the repository's worktrees.
func (r *Runner) Worktrees(ctx context.Context, repo domain.Repo) ([]domain.Worktree, error) {
	out, err := r.Run(ctx, repo.Root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var list []domain.Worktree
	var cur *domain.Worktree
	for _, f := range strings.Split(string(out), "\x00") {
		switch {
		case f == "":
			if cur != nil {
				list = append(list, *cur)
				cur = nil
			}
		case strings.HasPrefix(f, "worktree "):
			cur = &domain.Worktree{Path: strings.TrimPrefix(f, "worktree ")}
		case cur == nil:
		case strings.HasPrefix(f, "HEAD "):
			cur.Head = strings.TrimPrefix(f, "HEAD ")
		case strings.HasPrefix(f, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(f, "branch "), "refs/heads/")
		case f == "bare":
			cur.Bare = true
		case f == "detached":
			cur.Detached = true
		case strings.HasPrefix(f, "locked"):
			cur.Locked = true
		case strings.HasPrefix(f, "prunable"):
			cur.Prunable = true
		}
	}
	if cur != nil {
		list = append(list, *cur)
	}
	return list, nil
}

// Config reads one configuration value ("" when unset).
func (r *Runner) Config(ctx context.Context, repo domain.Repo, key string) string {
	out, err := r.Run(ctx, repo.Root, "config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ContainingBranches lists branches that contain hash.
func (r *Runner) ContainingBranches(ctx context.Context, repo domain.Repo, hash string, limit int) ([]string, error) {
	out, err := r.Run(ctx, repo.Root, "for-each-ref", "--contains="+hash, "--count="+strconv.Itoa(limit), "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" && !strings.HasSuffix(l, "/HEAD") {
			names = append(names, l)
		}
	}
	return names, nil
}
