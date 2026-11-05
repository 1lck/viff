package app

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/i18n"
	"viff/internal/observability"
)

// RepoState is everything known about the open repository. Each field is
// replaced as a whole when a query returns.
type RepoState struct {
	Repo domain.Repo
	// Gen increases whenever the repository is reopened; results for an older
	// generation are dropped. Workspace.nextGen issues it and never reuses a
	// value, so closing and reopening also yields a new generation.
	Gen uint64
	// rootKey is the canonical root used as the workspace list key.
	rootKey string

	Status       domain.Status
	StatusLoaded bool
	StatusTime   time.Time
	Operation    domain.Operation
	Conflicts    []domain.Conflict
	Refs         []domain.Ref
	Remotes      []domain.Remote
	Stashes      []domain.Stash
	RefsLoaded   bool
	// RefsByHash maps a commit to the refs pointing at it, for the log.
	RefsByHash map[string][]domain.Ref

	// Worktrees are the linked checkouts of this repository. A branch can be
	// checked out in only one worktree, so the branch popup needs the list to
	// say where a branch lives and where to open it.
	Worktrees       []domain.Worktree
	WorktreesLoaded bool

	// Shallow marks a clone whose history is truncated. The Git window offers
	// to fetch the missing history while it is set.
	Shallow bool

	// LFS is the Git LFS snapshot of this repository. Viff detects LFS but
	// does not manage it (ADR 0031); the UI shows the marker and warns when
	// git-lfs is missing.
	LFS gitcli.LFSStatus
	// lfsKey and lfsAt throttle LFS detection: the refresh reuses the snapshot
	// while the root .gitattributes is unchanged and the last check is recent,
	// because probing git-lfs and listing managed files on every status
	// refresh would cost a process per file-system event.
	lfsKey string
	lfsAt  time.Time

	Log LogState

	// Detail is the selected commit's detail in the log.
	Detail         *domain.CommitDetail
	DetailHash     string
	DetailLoading  bool
	DetailBranches []string

	statusSeq   uint64 // increments for each status query
	refreshing  bool
	refreshMore bool
	refsSeq     uint64
	refsKey     string
}

// writeLocks serializes writes per common Git directory: worktrees of one
// repository share refs and objects.
var writeLocks sync.Map

func writeLock(repo domain.Repo) *sync.Mutex {
	key := repo.CommonDir
	if key == "" {
		key = repo.GitDir
	}
	m, _ := writeLocks.LoadOrStore(key, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// Open opens the repository containing dir. done reports failure on the UI
// thread. Opening a repository while another one is active is a switch, so a
// running write task refuses it just like ActivateRepository does.
//
// Open shares the activation sequence with ActivateRepository and Close: a slow
// discovery that finishes after a newer open, switch or close is stale and
// must not replace that newer state.
func (w *Workspace) Open(dir string, done func(error)) {
	if w.Repo != nil && w.Busy() {
		if done != nil {
			done(ErrRepositoryBusy)
		}
		return
	}
	w.repos.activateSeq++
	seq := w.repos.activateSeq
	start := time.Now()
	_, ctx, finish := w.startTask("Opening "+filepath.Base(dir), false)
	go func() {
		repo, err := w.Git.Discover(ctx, dir)
		took := time.Since(start)
		w.post(func() {
			finish()
			if seq != w.repos.activateSeq {
				// A newer open, switch or close replaced this request.
				w.record(observability.Event{
					Event: observability.EventRepoOpen, Level: observability.LevelWarn,
					Repo: dir, Op: "open", Dur: took,
					Fields: map[string]any{"status": "stale"},
				})
				if done != nil {
					done(ErrRepositoryStale)
				}
				return
			}
			if err != nil {
				w.record(observability.Event{
					Event: observability.EventRepoOpen, Level: observability.LevelError,
					Repo: dir, Op: "open", Code: gitcli.CodeOf(err), Dur: took,
					Fields: map[string]any{"status": "failed"},
				})
				if done != nil {
					done(err)
				}
				return
			}
			// A write task may have started while discovery ran; opening a
			// repository is a switch from the existing one, so refuse it the
			// same way ActivateRepository does.
			if w.Busy() {
				w.record(observability.Event{
					Event: observability.EventRepoOpen, Level: observability.LevelWarn,
					Repo: dir, Op: "open", Dur: took,
					Fields: map[string]any{"status": "busy"},
				})
				if done != nil {
					done(ErrRepositoryBusy)
				}
				return
			}
			w.setRepo(repo)
			w.record(observability.Event{
				Event: observability.EventRepoOpen, Level: observability.LevelInfo,
				Repo: repo.Root, Op: "open", Dur: took,
				Fields: map[string]any{"status": "ok", "worktree": repo.IsLinkedWorktree()},
			})
			if done != nil {
				done(nil)
			}
		})
	}()
}

func (w *Workspace) setRepo(repo domain.Repo) {
	// The repository becomes part of the explicit workspace list and the
	// active one; this also persists the list before any command runs.
	root := w.noteActiveRepository(repo)
	oldRoot := ""
	if w.Repo != nil {
		oldRoot = w.Repo.Repo.Root
		w.Repo.Log.close()
	}
	w.closeWatcher(oldRoot)
	w.nextGen++
	w.Repo = &RepoState{Repo: repo, Gen: w.nextGen, rootKey: root}
	if w.Settings != nil {
		w.Settings.AddRecent(repo.Root)
		w.Settings.Save()
	}
	w.Refresh(true)
	w.ReloadLog()
	w.startWatcher(repo)
}

// startWatcher starts the file-system watch of repo. The backend is chosen by
// the Watch function the composition root installs, so the event leaves
// backend empty rather than guessing between native and polling.
func (w *Workspace) startWatcher(repo domain.Repo) {
	if w.Watch == nil {
		return
	}
	state := w.Repo
	watcher, err := w.Watch(repo, func(meta bool) {
		w.post(func() {
			if w.Repo == state {
				w.Refresh(meta)
			}
		})
	})
	if err != nil {
		w.record(observability.Event{
			Event: observability.EventRepoWatch, Level: observability.LevelWarn,
			Repo: repo.Root, Op: "watch",
			Fields: map[string]any{"status": "error", "backend": "", "error": truncateBytes(err.Error(), 512)},
		})
		return
	}
	w.watcher = watcher
	w.record(observability.Event{
		Event: observability.EventRepoWatch, Level: observability.LevelInfo,
		Repo: repo.Root, Op: "watch",
		Fields: map[string]any{"status": "started", "backend": ""},
	})
}

// closeWatcher stops the current watcher, if any, and records it. root is the
// repository the watcher watched, "" when unknown.
func (w *Workspace) closeWatcher(root string) {
	if w.watcher == nil {
		return
	}
	w.watcher.Close()
	w.watcher = nil
	w.record(observability.Event{
		Event: observability.EventRepoWatch, Level: observability.LevelInfo,
		Repo: root, Op: "watch",
		Fields: map[string]any{"status": "stopped", "backend": ""},
	})
}

// Close closes the repository and returns to the welcome screen. It releases
// the repository-specific log and file watcher; the persisted workspace list
// and the persisted active repository are left alone, so closing (or quitting)
// never forgets the repositories the user added. RemoveRepository is the only
// operation that clears the active root, and it does so explicitly.
func (w *Workspace) Close() {
	// Invalidate opens and switches that are still discovering: a repository
	// that is being opened must not reopen the window after it was closed.
	w.repos.activateSeq++
	root := ""
	if w.Repo != nil {
		root = w.Repo.Repo.Root
		w.record(observability.Event{
			Event: observability.EventRepoClose, Level: observability.LevelInfo,
			Repo: root, Op: "close",
			Fields: map[string]any{
				"op_state":      operationStateName(w.Repo.Operation),
				"tasks_running": len(w.Tasks),
				"watcher":       w.watcher != nil,
			},
		})
		w.Repo.Log.close()
	}
	w.closeWatcher(root)
	w.Repo = nil
}

// operationStateName is the stable name of the in-progress operation used in
// event fields, "none" when the working tree is not in the middle of one.
func operationStateName(op domain.Operation) string {
	switch op.Kind {
	case domain.OpMerge:
		return "merge"
	case domain.OpRebase:
		return "rebase"
	case domain.OpCherryPick:
		return "cherry-pick"
	case domain.OpRevert:
		return "revert"
	case domain.OpApplyMailbox:
		return "apply-mailbox"
	case domain.OpBisect:
		return "bisect"
	}
	return "none"
}

// Refresh queries status and operation state again, and refs when meta is
// set. Overlapping refreshes are coalesced: one runs, and one more follows if
// requests arrived meanwhile, so an event storm costs two queries.
func (w *Workspace) Refresh(meta bool) {
	rs := w.Repo
	if rs == nil {
		return
	}
	if rs.refreshing {
		rs.refreshMore = true
		if meta {
			rs.refsKey = "" // force refs reload on the follow-up
		}
		return
	}
	rs.refreshing = true
	start := time.Now()
	rs.statusSeq++
	seq := rs.statusSeq
	repo := rs.Repo
	lfsKey, lfsAt, cachedLFS := rs.lfsKey, rs.lfsAt, rs.LFS
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		st, err := w.Git.Status(ctx, repo)
		op := w.Git.Operation(ctx, repo)
		var conflicts []domain.Conflict
		if err == nil {
			conflicts, _ = w.Git.Conflicts(ctx, repo, st)
		}
		var refs []domain.Ref
		var remotes []domain.Remote
		var stashes []domain.Stash
		var refsErr error
		refs, refsErr = w.Git.Refs(ctx, repo)
		if refsErr == nil {
			remotes, _ = w.Git.Remotes(ctx, repo)
			stashes, _ = w.Git.Stashes(ctx, repo)
		}
		worktrees, worktreesErr := w.Git.Worktrees(ctx, repo)
		shallow, shallowErr := w.Git.IsShallow(ctx, repo)
		// LFS detection runs a probe and a file listing, so it is neither part
		// of every status refresh nor on the UI thread. A metadata refresh is
		// the user's refresh button or the end of a write task, which is
		// exactly when the managed file list can change.
		key := lfsAttributesKey(repo.Root)
		lfs, lfsRefreshed := cachedLFS, false
		if meta || lfsAt.IsZero() || key != lfsKey || time.Since(lfsAt) >= lfsRecheckInterval {
			lfs = w.Git.LFS(ctx, repo)
			lfsRefreshed = true
		}
		w.post(func() {
			if w.Repo != rs || rs.statusSeq != seq {
				return
			}
			rs.refreshing = false
			if lfsRefreshed {
				rs.LFS = lfs
				rs.lfsKey, rs.lfsAt = key, w.now()
			}
			staged, unstaged, untracked, conflicted := 0, 0, 0, 0
			if err != nil {
				w.markRepositoryError(rs.rootKey, err)
				if gitcli.KindOf(err) == gitcli.ErrNotRepository {
					w.Notify(NoticeError, w.tr(i18n.AppNoticeRepositoryGone), repo.Root)
					// Keep the row (with its error) but there is nothing to be
					// active any more.
					w.Close()
					if w.ActiveRoot == rs.rootKey {
						w.ActiveRoot = ""
						w.saveRepositories()
					}
					return
				}
				w.NotifyError(w.tr(i18n.AppNoticeCannotReadStatus), err)
			} else {
				rs.Status = st
				rs.StatusLoaded = true
				rs.StatusTime = w.Now()
				rs.Operation = op
				rs.Conflicts = conflicts
				staged, unstaged, untracked, conflicted = st.Counts()
				// Git reports a rename as one entry whose old path is gone;
				// keep change-list membership attached to the new path. A
				// failure is reported once per failure streak, not on every
				// refresh.
				w.warnChangeListSync(w.SyncChangeListRenames())
			}
			if refsErr == nil {
				key := refsFingerprint(refs, st.Branch.Head)
				changed := key != rs.refsKey
				rs.Refs, rs.Remotes, rs.Stashes, rs.RefsLoaded = refs, remotes, stashes, true
				rs.RefsByHash = indexRefs(refs, st.Branch)
				if changed {
					first := rs.refsKey == ""
					rs.refsKey = key
					if !first || rs.Log.Loaded() {
						w.ReloadLog()
					}
				}
			}
			if worktreesErr == nil {
				rs.Worktrees, rs.WorktreesLoaded = worktrees, true
			}
			if shallowErr == nil {
				rs.Shallow = shallow
			}
			// Keep the workspace overview row of the active repository in
			// step with the view; a failure keeps the known status.
			w.syncActiveSummary(err)
			if err == nil {
				w.record(observability.Event{
					Event: observability.EventRepoRefresh, Level: observability.LevelInfo,
					Repo: repo.Root, Op: "refresh", Dur: time.Since(start),
					Fields: map[string]any{
						"gen":        rs.Gen,
						"meta":       meta,
						"staged":     staged,
						"unstaged":   unstaged,
						"untracked":  untracked,
						"conflicted": conflicted,
						"op_state":   operationStateName(op),
						"lfs":        lfsEventFields(rs.LFS),
					},
				})
			}
			if rs.refreshMore {
				rs.refreshMore = false
				w.Refresh(true)
			}
		})
	}()
}

// lfsRecheckInterval bounds how often a plain working-tree refresh re-runs LFS
// detection. Attribute changes, metadata events and the refresh button still
// re-run it immediately; the interval only collapses file-system event storms
// into one probe.
const lfsRecheckInterval = 30 * time.Second

// lfsAttributesKey is the cheap change detector for LFS rules: the size and
// modification time of the repository-root .gitattributes, the file `git lfs
// track` writes. Nested attribute files are not part of the key; a metadata
// refresh or the interval picks their changes up.
func lfsAttributesKey(root string) string {
	st, err := os.Stat(filepath.Join(root, ".gitattributes"))
	if err != nil {
		return "missing"
	}
	return strconv.FormatInt(st.Size(), 10) + ":" + strconv.FormatInt(st.ModTime().UnixNano(), 10)
}

// lfsEventFields is the log-safe part of an LFS snapshot: flags and counts
// only, never a pattern, path or file name. The error is the classified code,
// matching how git.fail reports failures.
func lfsEventFields(st gitcli.LFSStatus) map[string]any {
	fields := map[string]any{
		"installed": st.Installed,
		"managed":   st.Managed,
		"patterns":  len(st.Patterns),
		"files":     len(st.Files),
	}
	if st.Err != nil {
		fields["error"] = gitcli.CodeOf(st.Err)
	}
	return fields
}

func refsFingerprint(refs []domain.Ref, head string) string {
	var b strings.Builder
	b.WriteString(head)
	for _, r := range refs {
		b.WriteString(r.Name)
		b.WriteByte(0)
		b.WriteString(r.Target)
		b.WriteByte(0)
	}
	return b.String()
}

func indexRefs(refs []domain.Ref, branch domain.BranchStatus) map[string][]domain.Ref {
	m := make(map[string][]domain.Ref, len(refs))
	if branch.Head != "" {
		m[branch.Head] = append(m[branch.Head], domain.Ref{Kind: domain.RefHead, Short: "HEAD", Name: "HEAD", Target: branch.Head})
	}
	for _, r := range refs {
		m[r.Target] = append(m[r.Target], r)
	}
	for h, list := range m {
		sort.SliceStable(list, func(i, j int) bool { return refOrder(list[i]) < refOrder(list[j]) })
		m[h] = list
	}
	return m
}

func refOrder(r domain.Ref) int {
	switch {
	case r.Kind == domain.RefHead:
		return 0
	case r.Kind == domain.RefLocal && r.Current:
		return 1
	case r.Kind == domain.RefLocal:
		return 2
	case r.Kind == domain.RefRemote:
		return 3
	}
	return 4
}

// CurrentBranch is the checked-out branch name, "" when detached.
func (rs *RepoState) CurrentBranch() string { return rs.Status.Branch.Branch }

// CurrentRef returns the local ref of the current branch.
func (rs *RepoState) CurrentRef() (domain.Ref, bool) {
	for _, r := range rs.Refs {
		if r.Kind == domain.RefLocal && r.Current {
			return r, true
		}
	}
	return domain.Ref{}, false
}

// WorktreeOf returns the worktree that has branch checked out.
func (rs *RepoState) WorktreeOf(branch string) (domain.Worktree, bool) {
	if branch == "" {
		return domain.Worktree{}, false
	}
	for _, wt := range rs.Worktrees {
		// A prunable worktree's directory is gone, so it cannot be opened and
		// no longer holds the branch in practice.
		if wt.Prunable || wt.Bare {
			continue
		}
		if wt.Branch == branch {
			return wt, true
		}
	}
	return domain.Worktree{}, false
}

// WorktreeElsewhere returns the other worktree that has branch checked out,
// that is, not the checkout this state belongs to. A branch can be checked
// out once, so the popup opens that worktree instead of failing a checkout.
func (rs *RepoState) WorktreeElsewhere(branch string) (domain.Worktree, bool) {
	wt, ok := rs.WorktreeOf(branch)
	if !ok || samePath(wt.Path, rs.Repo.Root) {
		return domain.Worktree{}, false
	}
	return wt, true
}

// IsCurrentWorktree reports whether wt is the checkout this state belongs to.
func (rs *RepoState) IsCurrentWorktree(wt domain.Worktree) bool {
	return samePath(wt.Path, rs.Repo.Root)
}

// samePath reports whether two paths name one directory, following symlinks
// (on macOS /tmp is a link to /private/tmp).
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// HeadLabel is the branch widget text: the branch, or the short hash when
// detached, with the operation in progress.
func (rs *RepoState) HeadLabel() string {
	b := rs.Status.Branch
	label := b.Branch
	if b.Detached || label == "" {
		label = domain.ShortHash(b.Head)
		if label == "" {
			label = "No commits"
		}
	}
	if rs.Operation.Kind == domain.OpRebase && rs.Operation.HeadName != "" {
		label = "Rebasing " + rs.Operation.HeadName
	}
	return label
}

// HasHead reports a repository with at least one commit.
func (rs *RepoState) HasHead() bool { return rs.Status.Branch.Head != "" }

// Locals, RemoteRefs and Tags filter refs by kind.
func (rs *RepoState) RefsOf(kind domain.RefKind) []domain.Ref {
	var out []domain.Ref
	for _, r := range rs.Refs {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

// Entries returns the status entries shown in an area.
func (rs *RepoState) Entries(area domain.Area) []domain.StatusEntry {
	var out []domain.StatusEntry
	for _, e := range rs.Status.Entries {
		switch area {
		case domain.AreaStaged:
			if e.Staged() {
				out = append(out, e)
			}
		case domain.AreaUnstaged:
			if e.Unstaged() {
				out = append(out, e)
			}
		case domain.AreaUntracked:
			if e.Untracked() {
				out = append(out, e)
			}
		case domain.AreaConflicts:
			if e.Conflicted() {
				out = append(out, e)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Conflict returns the conflict record for path.
func (rs *RepoState) Conflict(path string) (domain.Conflict, bool) {
	for _, c := range rs.Conflicts {
		if c.Path == path {
			return c, true
		}
	}
	return domain.Conflict{}, false
}

// exists reports whether a path exists in the working tree.
func exists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}
