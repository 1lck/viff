package svncli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// Discover locates the Subversion working copy containing dir.
//
// Unlike Git there is no single "toplevel" query: svn reports the working copy
// root in wc-info/wcroot-abspath, so one `svn info` on the given directory
// answers it. The repository identity (URL, root, UUID) travels with the
// working copy because .svn holds only pristine base files and no history.
func (r *Runner) Discover(ctx context.Context, dir string) (domain.Repo, error) {
	out, err := r.Run(ctx, dir, "info", "--xml")
	if err != nil {
		return domain.Repo{}, err
	}
	info, err := ParseInfo(out)
	if err != nil {
		return domain.Repo{}, err
	}
	root := filepath.Clean(info.WCRootAbsPath)
	if root == "" || root == "." {
		root = filepath.Clean(dir)
	}
	repo := domain.Repo{
		Root:     root,
		Kind:     domain.KindSVN,
		MetaDir:  filepath.Join(root, ".svn"),
		URL:      info.URL,
		RelURL:   info.RelativeURL,
		RepoRoot: info.RepoRoot,
		UUID:     info.RepoUUID,
	}
	return repo, nil
}

// Init creates a working copy in dir from an existing repository URL.
//
// svn has no "init an empty repository here": a working copy is always a
// checkout of a server-side path. Init therefore requires the caller to supply
// the URL through the settings-backed default and reports a clear error when
// there is none, instead of inventing a local repository.
func (r *Runner) Init(ctx context.Context, dir string) error {
	return &vcs.Error{
		Kind:   vcs.ErrOther,
		Tool:   "svn",
		Stderr: "Subversion has no local repository: use Clone with a repository URL instead",
	}
}

// Clone checks out url into parent/dir.
func (r *Runner) Clone(ctx context.Context, parent, url, dir string, progress func(string)) error {
	target := dir
	if !filepath.IsAbs(target) {
		target = filepath.Join(parent, dir)
	}
	_, err := r.RunProgress(ctx, parent, progress, "checkout", url, target)
	return err
}

// helperSuffixes are the files svn leaves next to a conflicted path. svn
// reports them as unversioned, so without filtering every conflict would add
// three junk rows to the untracked list (finding S7).
var helperSuffixes = []string{".mine", ".r0", ".r1", ".r2", ".r3", ".r4", ".r5", ".r6", ".r7", ".r8", ".r9"}

// IsConflictHelper reports a path svn created as a conflict side file.
func IsConflictHelper(p string) bool {
	for _, suffix := range helperSuffixes {
		if strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return strings.HasSuffix(p, ".working") || strings.HasSuffix(p, ".merge-left") ||
		strings.HasSuffix(p, ".merge-right") || strings.HasSuffix(p, ".merge")
}

// Status reports the working copy state.
//
// Subversion has one change state per path, not the index/worktree pair Git
// has, so the change letter goes into Worktree and Index stays '.'. The UI
// then shows a single list instead of the two-column staging area.
func (r *Runner) Status(ctx context.Context, repo domain.Repo) (domain.Status, error) {
	// No --no-ignore: the default already leaves out everything svn:ignore
	// covers, which is what the changes list wants. svn has no --ignore option
	// (measured on 1.15.0: "invalid option: --ignore").
	out, err := r.Run(ctx, repo.Root, "status", "--xml")
	if err != nil {
		return domain.Status{}, err
	}
	entries, err := ParseStatus(out)
	if err != nil {
		return domain.Status{}, err
	}
	status := domain.Status{Entries: make([]domain.StatusEntry, 0, len(entries))}
	for _, e := range entries {
		if IsConflictHelper(e.Path) {
			continue
		}
		if e.Item == "ignored" {
			continue
		}
		letter := statusLetter(e.Item)
		if letter == ' ' && e.Props == "none" {
			continue
		}
		entry := domain.StatusEntry{
			Path:     filepath.ToSlash(e.Path),
			Index:    '.',
			Worktree: letter,
		}
		if e.Props == "modified" {
			entry.Props = 'M'
		} else if e.Props == "conflicted" {
			entry.Props = 'C'
		}
		status.Entries = append(status.Entries, entry)
	}
	r.fillBranchStatus(ctx, repo, &status)
	return status, nil
}

// statusLetter maps svn's item attribute onto the letter the rest of Viff
// already understands. 'missing' becomes '!' because the file is gone from
// disk but still recorded, which is what '!' means in the status model.
func statusLetter(item string) byte {
	switch item {
	case "modified":
		return 'M'
	case "added":
		return 'A'
	case "deleted":
		return 'D'
	case "missing":
		return '!'
	case "unversioned":
		return '?'
	case "conflicted":
		return 'C'
	case "replaced":
		return 'R'
	case "external":
		return 'X'
	case "incomplete":
		return '!'
	}
	return ' '
}

// fillBranchStatus reads the working copy's revision and URL for the status
// bar. It never fails the status query: a missing revision only means the
// numbers stay zero.
func (r *Runner) fillBranchStatus(ctx context.Context, repo domain.Repo, status *domain.Status) {
	out, err := r.Run(ctx, repo.Root, "info", "--xml")
	if err == nil {
		if info, ierr := ParseInfo(out); ierr == nil {
			status.Branch.Head = info.Revision
			status.Branch.Branch = branchFromRelURL(info.RelativeURL)
		}
	}
	head, err := r.headRevision(ctx, repo)
	if err != nil {
		return
	}
	status.Branch.Upstream = head
	// Behind is the count of revisions the working copy lacks. Subversion
	// cannot be ahead of the repository, so Ahead stays zero.
	if base, cerr := strconv.ParseInt(status.Branch.Head, 10, 64); cerr == nil {
		if h, herr := strconv.ParseInt(head, 10, 64); herr == nil && h > base {
			status.Branch.Behind = int(h - base)
			status.Branch.HasAB = true
		}
	}
}

// branchFromRelURL turns "^/trunk" or "^/branches/feature" into a display
// name. Subversion has no branches, only directories, so the convention is
// read as a hint and never presented as a first-class ref.
func branchFromRelURL(rel string) string {
	rel = strings.TrimPrefix(rel, "^/")
	if rel == "" {
		return "trunk"
	}
	if rest, ok := strings.CutPrefix(rel, "branches/"); ok {
		return rest
	}
	if rest, ok := strings.CutPrefix(rel, "tags/"); ok {
		return rest
	}
	if rel == "trunk" {
		return "trunk"
	}
	return rel
}

// headRevision asks the repository for HEAD. This is a network call, so it is
// only used where the answer matters (the "update available" indicator).
func (r *Runner) headRevision(ctx context.Context, repo domain.Repo) (string, error) {
	if repo.RepoRoot == "" {
		return "", errors.New("repository root unknown")
	}
	out, err := r.Run(ctx, "", "info", "-r", "HEAD", "--show-item", "revision", repo.RepoRoot)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Operation reports no multi-step operation: Subversion has no rebase, merge
// or cherry-pick state machine that a working copy records. The zero value is
// OpNone, which is what the UI then shows.
func (r *Runner) Operation(ctx context.Context, repo domain.Repo) domain.Operation {
	return domain.Operation{}
}

// Conflicts lists the conflicted paths. svn marks them with item="conflicted"
// and leaves .mine/.rN side files; the sides become the conflict stages so the
// existing read-only conflict viewer can show them.
func (r *Runner) Conflicts(ctx context.Context, repo domain.Repo, status domain.Status) ([]domain.Conflict, error) {
	var out []domain.Conflict
	for _, e := range status.Entries {
		if !e.Conflicted() {
			continue
		}
		// Subversion leaves the three sides next to the file rather than in an
		// index, so the stages name those files. Base is .r1 for the common
		// case; the viewer falls back to the file itself when one is missing.
		out = append(out, domain.Conflict{
			Path: e.Path,
			Kind: domain.ConflictBothModified,
			Stages: []domain.Stage{
				{Number: 1, Mode: "100644", Hash: e.Path + ".r1"},
				{Number: 2, Mode: "100644", Hash: e.Path + ".mine"},
				{Number: 3, Mode: "100644", Hash: e.Path + ".r2"},
			},
		})
	}
	return out, nil
}

// TreeAt lists the files recorded at rev.
//
// The listing is recursive: the tree view expects every path, not one level.
// svn prints a recursive listing's nested entries as their own relative path,
// which is exactly the working-copy relative name the caller wants.
func (r *Runner) TreeAt(ctx context.Context, repo domain.Repo, rev string) ([]domain.TreeEntry, error) {
	if rev == "" {
		rev = "HEAD"
	}
	out, err := r.Run(ctx, repo.Root, "ls", "-r", rev, "--xml", "-R", repo.URL)
	if err != nil {
		return nil, err
	}
	list, err := ParseList(out)
	if err != nil {
		return nil, err
	}
	entries := make([]domain.TreeEntry, 0, len(list))
	for _, e := range list {
		name := strings.TrimSuffix(e.Name, "/")
		if name == "" {
			continue
		}
		kind, mode, size := "blob", "100644", e.Size
		if e.Kind == "dir" {
			kind, mode = "tree", "040000"
			size = -1
		}
		entries = append(entries, domain.TreeEntry{Path: name, Type: kind, Mode: mode, Size: size})
	}
	return entries, nil
}

// Read loads path at rev.
//
// Two address forms have to be kept apart, because svn rejects the
// working-copy keywords when the target is a URL (E195002):
//
//   - a working-copy revision (the base/"index" side, and the working tree
//     itself) is addressed by its relative path inside the working copy;
//   - a committed revision is addressed by URL.
//
// The base form also works when the file is gone from disk, which is exactly
// the case the diff view needs: a deleted file still has a base side.
func (r *Runner) Read(ctx context.Context, repo domain.Repo, rev vcs.Revision, p string) (vcs.Content, error) {
	rel := strings.TrimPrefix(p, "/")
	if rev.Kind == "worktree" {
		return readWorktree(filepath.Join(repo.Root, filepath.FromSlash(rel)))
	}

	var target string
	var address string
	switch rev.Kind {
	case "index", "stage":
		// The pristine base the working copy is compared against. Subversion
		// has no index or stages, so both mean "as recorded".
		target = "BASE"
		address = filepath.FromSlash(rel)
	default:
		target = rev.Rev
		if target == "" {
			target = "HEAD"
		}
		address = strings.TrimSuffix(repo.URL, "/") + "/" + rel
	}

	out, err := r.Run(ctx, repo.Root, "cat", "-r", target, address)
	if err != nil {
		// A path that does not exist at that revision is an absent side, not a
		// failure: it is how an added or deleted file's missing half is shown.
		switch vcs.KindOf(err) {
		case vcs.ErrNotFound, vcs.ErrNoRevision:
			return vcs.Content{}, nil
		}
		return vcs.Content{}, err
	}
	c := vcs.Content{Exists: true, Data: out, Size: int64(len(out)), Mode: "100644"}
	c.Binary = isBinary(out)
	return c, nil
}

// readWorktree reads the file on disk, bounded like the Git adapter does.
func readWorktree(full string) (vcs.Content, error) {
	data, err := readFileLimited(full, vcs.ContentLimit)
	if err != nil {
		if errors.Is(err, errNotExist) {
			return vcs.Content{}, nil
		}
		return vcs.Content{}, err
	}
	c := vcs.Content{Exists: true, Size: int64(len(data)), Mode: "100644"}
	if len(data) > vcs.ContentLimit {
		c.TooLarge = true
		c.Data = nil
		return c, nil
	}
	c.Data = data
	c.Binary = isBinary(data)
	return c, nil
}

// Blame annotates a file with the revision that last changed each line.
func (r *Runner) Blame(ctx context.Context, repo domain.Repo, rev, p string) (*domain.Blame, error) {
	args := []string{"blame", "--xml"}
	if rev != "" {
		args = append(args, "-r", rev)
	}
	args = append(args, repo.URL+"/"+strings.TrimPrefix(p, "/"))
	out, err := r.Run(ctx, repo.Root, args...)
	if err != nil {
		return nil, err
	}
	files, err := ParseBlame(out)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return &domain.Blame{}, nil
	}
	file := files[0]
	blame := &domain.Blame{}
	index := make(map[string]int)
	for _, line := range file.Lines {
		key := line.Revision
		if key == "" {
			key = "0"
		}
		i, ok := index[key]
		if !ok {
			i = len(blame.Commits)
			index[key] = i
			blame.Commits = append(blame.Commits, domain.BlameCommit{
				Hash:   line.Revision,
				Author: line.Author,
				Time:   line.Date,
			})
		}
		blame.Lines = append(blame.Lines, int32(i))
		blame.Orig = append(blame.Orig, int32(line.LineNumber))
	}
	return blame, nil
}

// DiffNames lists the files that differ between two revisions.
//
// Two cases have to be kept apart, because svn rejects the working-copy
// keywords (BASE, PREV, COMMITTED) when the target is a URL:
//
//   - no revisions given: diff the working copy, where those keywords apply;
//   - explicit numeric revisions: diff them on the repository URL, and strip
//     the URL prefix to get working-copy-relative names.
func (r *Runner) DiffNames(ctx context.Context, repo domain.Repo, from, to string) ([]domain.Change, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" && to == "" {
		// Working copy against its base. No -r: that is what this form means,
		// and adding the keywords would make svn reject the invocation.
		out, err := r.Run(ctx, repo.Root, "diff", "--summarize")
		if err != nil {
			return nil, err
		}
		return parseSummary(out, ""), nil
	}
	if from == "" {
		from = "HEAD"
	}
	if to == "" {
		to = "HEAD"
	}
	// A keyword here would fail on a URL, so it is reported instead of being
	// passed through and turning into a confusing svn error.
	for _, rev := range []string{from, to} {
		if !isNumericRevision(rev) {
			return nil, &vcs.Error{
				Kind: vcs.ErrOther, Tool: "svn",
				Stderr: "diff between revisions needs numeric revisions, got " + rev,
			}
		}
	}
	out, err := r.Run(ctx, repo.Root, "diff", "--summarize", "-r", from+":"+to, repo.URL)
	if err != nil {
		return nil, err
	}
	return parseSummary(out, repo.URL), nil
}

// isNumericRevision reports a plain revision number.
func isNumericRevision(rev string) bool {
	if rev == "" {
		return false
	}
	for i := 0; i < len(rev); i++ {
		if rev[i] < '0' || rev[i] > '9' {
			return false
		}
	}
	return true
}

// parseSummary turns `svn diff --summarize` output into changes, stripping the
// repository URL prefix so paths are working-copy relative.
func parseSummary(out []byte, prefix string) []domain.Change {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	var changes []domain.Change
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 2 {
			continue
		}
		action := line[0]
		rest := strings.TrimSpace(line[1:])
		if rest == "" {
			continue
		}
		// The working-copy form prefixes a column of spaces for property
		// changes; the action letter is still the first non-space byte.
		if action == ' ' {
			trimmed := strings.TrimLeft(line, " ")
			if trimmed == "" {
				continue
			}
			action = trimmed[0]
			rest = strings.TrimSpace(trimmed[1:])
		}
		p := strings.TrimPrefix(strings.TrimPrefix(rest, prefix), "/")
		changes = append(changes, domain.Change{Kind: actionKind(action), Path: p})
	}
	return changes
}

// actionKind maps svn's action letters onto the shared change kinds.
func actionKind(action byte) domain.ChangeKind {
	switch action {
	case 'A':
		return domain.ChangeAdded
	case 'D':
		return domain.ChangeDeleted
	case 'R':
		return domain.ChangeRenamed
	}
	return domain.ChangeModified
}

// MultiCommitChanges lists what each revision changed. Subversion answers one
// revision per call, so they are queried independently as the contract asks.
func (r *Runner) MultiCommitChanges(ctx context.Context, repo domain.Repo, commits []domain.Commit) []vcs.MultiCommitChanges {
	out := make([]vcs.MultiCommitChanges, 0, len(commits))
	for _, c := range commits {
		if err := ctx.Err(); err != nil {
			return out
		}
		detail, err := r.CommitDetail(ctx, repo, c.Hash)
		out = append(out, vcs.MultiCommitChanges{Commit: c, Changes: detail.Changes, Err: err})
	}
	return out
}

// Config returns "" for every key: Subversion keeps its configuration in its
// own files and Viff does not read them.
func (r *Runner) Config(ctx context.Context, repo domain.Repo, key string) string { return "" }

// WorktreeFiles lists what a file browser may show: every path the working copy
// records plus the files it does not ignore.
//
// One call does both: `svn status -v --no-ignore --xml` reports scheduled paths
// with their revision and unversioned ones with item="unversioned", so there is
// no need for a second round trip. The output carries no kind, so a directory
// is recognised on disk; a path whose entry is gone from disk is still a file
// candidate and is kept, because the browser must show removed files too.
func (r *Runner) WorktreeFiles(ctx context.Context, repo domain.Repo) ([]string, error) {
	out, err := r.Run(ctx, repo.Root, "status", "-v", "--no-ignore", "--xml")
	if err != nil {
		return nil, err
	}
	entries, err := ParseStatus(out)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(entries))
	list := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Path == "." || e.Path == "" {
			continue
		}
		// An ignored file is not part of what the browser lists; svn reports
		// those only because --no-ignore was asked for.
		if e.Item == "ignored" || e.Item == "external" {
			continue
		}
		p := filepath.ToSlash(e.Path)
		if _, dup := seen[p]; dup {
			continue
		}
		full := filepath.Join(repo.Root, filepath.FromSlash(p))
		if st, serr := os.Stat(full); serr == nil && st.IsDir() {
			continue
		}
		seen[p] = struct{}{}
		list = append(list, p)
	}
	sort.Strings(list)
	return list, nil
}

// historyCache holds log pages per repository so scrolling through a large
// history does not re-query the server for every page. It is bounded by entry
// count, and an entry older than historyTTL is rebuilt on the next query — the
// log lives on the server, so a cached page can be stale in a way a Git log
// never is.
type historyCache struct {
	mu      sync.Mutex
	entries map[string]*historyEntry
}

type historyEntry struct {
	commits []domain.Commit
	fetched time.Time
}

const (
	historyTTL  = 2 * time.Minute
	historyKeep = 8
)

var logs = &historyCache{entries: make(map[string]*historyEntry)}

// LogReader serves history from one svn log query, page by page without
// asking the server again for the same range.
type LogReader struct {
	commits []domain.Commit
	pos     int
	closed  bool
}

// Next returns up to n more commits.
func (l *LogReader) Next(n int) ([]domain.Commit, bool, error) {
	if l.closed || l.pos >= len(l.commits) {
		return nil, true, nil
	}
	end := l.pos + n
	if end > len(l.commits) {
		end = len(l.commits)
	}
	page := l.commits[l.pos:end]
	l.pos = end
	return page, l.pos >= len(l.commits), nil
}

// Close releases the reader.
func (l *LogReader) Close() { l.closed = true }

// OpenLog starts a history query.
//
// The range is always explicit: `svn log` without -r stops at the working
// copy's base revision and silently omits everything after it (finding S1), so
// a plain call would show a history that is quietly out of date.
func (r *Runner) OpenLog(ctx context.Context, repo domain.Repo, f vcs.LogFilter) (vcs.LogReader, error) {
	commits, err := r.loadLog(ctx, repo, f)
	if err != nil {
		return nil, err
	}
	return &LogReader{commits: commits}, nil
}

// invalidateLog drops the cached history of a working copy.
//
// The cache exists because a Subversion log is a network query, but a commit or
// an update changes the history, so the next read has to ask the repository
// again. Without this, committing and then refreshing shows the revision
// before the one just made — the stale-cache failure the decision record warns
// about.
func invalidateLog(repo domain.Repo) {
	logs.mu.Lock()
	for k := range logs.entries {
		if strings.HasPrefix(k, repo.Root+"|") {
			delete(logs.entries, k)
		}
	}
	logs.mu.Unlock()
}

func (r *Runner) loadLog(ctx context.Context, repo domain.Repo, f vcs.LogFilter) ([]domain.Commit, error) {
	key := repo.Root + "|" + strings.Join(f.Revisions, ",") + "|" + f.Text + "|" + f.Author
	logs.mu.Lock()
	if e, ok := logs.entries[key]; ok && time.Since(e.fetched) < historyTTL {
		commits := e.commits
		logs.mu.Unlock()
		return commits, nil
	}
	logs.mu.Unlock()

	args := []string{"log", "--xml", "-v", "-r", "1:HEAD"}
	if f.Text != "" {
		args = append(args, "--search", f.Text)
	}
	target := repo.URL
	args = append(args, target)
	out, err := r.Run(ctx, repo.Root, args...)
	if err != nil {
		return nil, err
	}
	entries, err := ParseLog(out)
	if err != nil {
		return nil, err
	}
	commits := make([]domain.Commit, 0, len(entries))
	for _, e := range entries {
		commits = append(commits, logEntryToCommit(e))
	}
	// Newest first, matching what the log view expects.
	sort.SliceStable(commits, func(i, j int) bool {
		return revNumber(commits[i].Hash) > revNumber(commits[j].Hash)
	})
	if f.Author != "" {
		commits = filterAuthor(commits, f.Author)
	}
	logs.mu.Lock()
	if len(logs.entries) >= historyKeep {
		for k := range logs.entries {
			delete(logs.entries, k)
			break
		}
	}
	logs.entries[key] = &historyEntry{commits: commits, fetched: time.Now()}
	logs.mu.Unlock()
	return commits, nil
}

func filterAuthor(commits []domain.Commit, author string) []domain.Commit {
	out := commits[:0]
	for _, c := range commits {
		if strings.Contains(strings.ToLower(c.Author), strings.ToLower(author)) {
			out = append(out, c)
		}
	}
	return out
}

// logEntryToCommit converts one log entry into the shared commit shape.
//
// Parents holds the previous revision *in this path's log*, not revision-1:
// Subversion numbers revisions globally, so a path's history legitimately
// skips numbers (r3, r7, r9) and inventing n-1 would draw edges to commits
// that never touched the path.
func logEntryToCommit(e LogEntry) domain.Commit {
	return domain.Commit{
		Hash:       strconv.FormatInt(e.Revision, 10),
		Author:     e.Author,
		Committer:  e.Author,
		AuthorTime: e.Date,
		CommitTime: e.Date,
		Subject:    firstLine(e.Message),
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// revNumber parses a revision string, 0 when it is not a number.
func revNumber(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// wcPrefix is the repository-relative path of the working copy root: "^/trunk"
// becomes "trunk", "^/" becomes "".
//
// The distinction matters because svn reports two different path bases: `svn
// log -v` and `svn diff` on a URL print paths relative to the *repository*
// root ("/trunk/src/main.go"), while status and a working-copy diff print paths
// relative to the *working copy* ("src/main.go"). Mixing them builds a URL with
// the branch segment twice, which svn rejects with W160013.
func wcPrefix(relURL string) string {
	return strings.Trim(strings.TrimSuffix(relURL, "/"), "^/")
}

// wcRelative converts a repository-root-relative path into a working-copy
// relative one. ok is false when the path lies outside the working copy (the
// revision touched another branch), so the caller can decide what to show.
func wcRelative(relURL, repoPath string) (string, bool) {
	p := strings.TrimPrefix(repoPath, "/")
	prefix := wcPrefix(relURL)
	if prefix == "" {
		return p, true
	}
	if p == prefix {
		// The working copy root itself.
		return "", true
	}
	if rest, ok := strings.CutPrefix(p, prefix+"/"); ok {
		return rest, true
	}
	return p, false
}

// CommitDetail loads one revision's message and changed paths.
func (r *Runner) CommitDetail(ctx context.Context, repo domain.Repo, hash string) (domain.CommitDetail, error) {
	out, err := r.Run(ctx, repo.Root, "log", "--xml", "-v", "-r", hash, repo.URL)
	if err != nil {
		return domain.CommitDetail{}, err
	}
	entries, err := ParseLog(out)
	if err != nil {
		return domain.CommitDetail{}, err
	}
	if len(entries) == 0 {
		return domain.CommitDetail{}, &vcs.Error{
			Kind: vcs.ErrNoRevision, Tool: "svn",
			Stderr: "no such revision " + hash,
		}
	}
	e := entries[0]
	detail := domain.CommitDetail{
		Commit: logEntryToCommit(e),
		Body:   e.Message,
	}
	detail.Parents = nil
	for _, pc := range e.Paths {
		// svn lists directories as well as files when a tree is added
		// ("/trunk/src" with kind="dir"). The change list is a file list — the
		// diff view opens every entry — and reading a directory fails with
		// W195007, so directory entries are dropped here.
		if pc.Kind == "dir" {
			continue
		}
		rel, inside := wcRelative(repo.RelURL, pc.Path)
		if rel == "" && inside {
			// The working copy root directory itself; it has no file to open.
			continue
		}
		detail.Changes = append(detail.Changes, domain.Change{
			Kind: actionKind([]byte(pc.Action)[0]),
			Path: rel,
		})
	}
	return detail, nil
}

// Refs lists the conventional branch and tag directories under the repository
// root.
//
// Subversion has no refs: branches and tags are directories by convention
// (branches/x, tags/x). They are reported as refs so the existing branch
// switcher keeps working, but they are read-only hints — Viff never claims
// they behave like Git refs.
func (r *Runner) Refs(ctx context.Context, repo domain.Repo) ([]domain.Ref, error) {
	var refs []domain.Ref
	current := branchFromRelURL(repo.RelURL)

	// branches/ and tags/ live directly under the repository root, whatever
	// branch the working copy is on. Deriving them from RelURL produced a URL
	// with the "^" marker in it, which svn cannot open.
	for _, kind := range []struct {
		dir     string
		refKind domain.RefKind
	}{
		{"branches", domain.RefLocal},
		{"tags", domain.RefTag},
	} {
		target := strings.TrimSuffix(repo.RepoRoot, "/") + "/" + kind.dir
		out, err := r.Run(ctx, "", "ls", target)
		if err != nil {
			// A repository without a branches/ or tags/ directory is normal.
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			name := strings.TrimSuffix(strings.TrimSpace(line), "/")
			if name == "" {
				continue
			}
			refs = append(refs, domain.Ref{
				Name:    kind.dir + "/" + name,
				Short:   name,
				Kind:    kind.refKind,
				Current: kind.refKind == domain.RefLocal && name == current,
			})
		}
	}
	refs = append(refs, domain.Ref{Name: "trunk", Short: "trunk", Kind: domain.RefLocal, Current: current == "trunk"})
	return refs, nil
}

// Remotes reports the working copy's repository as a single remote.
func (r *Runner) Remotes(ctx context.Context, repo domain.Repo) ([]domain.Remote, error) {
	if repo.RepoRoot == "" {
		return nil, nil
	}
	return []domain.Remote{{Name: "origin", FetchURL: repo.RepoRoot, PushURL: repo.RepoRoot}}, nil
}

// Checkout switches the working copy to another branch directory.
//
// This is `svn switch`, which rewrites the working copy to the new URL. It is
// the closest thing Subversion has to changing branches; it is a network
// operation and keeps local modifications where possible.
func (r *Runner) Checkout(ctx context.Context, repo domain.Repo, ref domain.Ref) error {
	target, err := r.branchURL(repo, ref.Short)
	if err != nil {
		return err
	}
	_, err = r.Run(ctx, repo.Root, "switch", target)
	return err
}

// branchURL builds the repository URL of a branch or tag directory.
func (r *Runner) branchURL(repo domain.Repo, name string) (string, error) {
	if repo.RepoRoot == "" {
		return "", errors.New("repository root unknown")
	}
	// The conventional layout puts trunk and branches/ directly under the
	// repository root; they do not move with the working copy's own path.
	root := strings.TrimSuffix(repo.RepoRoot, "/")
	if name == "trunk" || name == "" {
		return root + "/trunk", nil
	}
	return root + "/branches/" + name, nil
}

// CheckoutRevision switches to a revision. Subversion has no detached HEAD, so
// the working copy moves to that revision on its current path.
func (r *Runner) CheckoutRevision(ctx context.Context, repo domain.Repo, rev string) error {
	_, err := r.Run(ctx, repo.Root, "update", "-r", rev)
	return err
}

// ResolveRef validates and returns a revision expression.
func (r *Runner) ResolveRef(ctx context.Context, repo domain.Repo, rev string) (string, error) {
	rev = strings.TrimSpace(rev)
	if rev == "" {
		return "", nil
	}
	out, err := r.Run(ctx, "", "info", "-r", rev, "--show-item", "revision", repo.URL)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsAncestor compares revision numbers: Subversion history is linear, so a
// lower revision is always an ancestor of a higher one.
func (r *Runner) IsAncestor(ctx context.Context, repo domain.Repo, a, b string) bool {
	return revNumber(a) <= revNumber(b)
}

// ContainingBranches reports the conventional branch directories that contain
// a revision. svn cannot answer this cheaply, so it returns nothing and the UI
// simply shows no branch markers for the revision.
func (r *Runner) ContainingBranches(ctx context.Context, repo domain.Repo, hash string, limit int) ([]string, error) {
	return nil, nil
}
