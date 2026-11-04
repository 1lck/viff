package app

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// RepoState is everything known about the open repository. Each field is
// replaced as a whole when a query returns.
type RepoState struct {
	Repo domain.Repo
	// Gen increases whenever the repository is reopened; results for an older
	// generation are dropped.
	Gen uint64

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
// thread.
func (w *Workspace) Open(dir string, done func(error)) {
	_, ctx, finish := w.startTask("Opening "+filepath.Base(dir), false)
	go func() {
		repo, err := w.Git.Discover(ctx, dir)
		w.post(func() {
			finish()
			if err != nil {
				if done != nil {
					done(err)
				}
				return
			}
			w.setRepo(repo)
			if done != nil {
				done(nil)
			}
		})
	}()
}

func (w *Workspace) setRepo(repo domain.Repo) {
	var gen uint64 = 1
	if w.Repo != nil {
		gen = w.Repo.Gen + 1
		w.Repo.Log.close()
	}
	if w.watcher != nil {
		w.watcher.Close()
		w.watcher = nil
	}
	w.Repo = &RepoState{Repo: repo, Gen: gen}
	if w.Settings != nil {
		w.Settings.AddRecent(repo.Root)
		w.Settings.Save()
	}
	w.Refresh(true)
	w.ReloadLog()
	if w.Watch != nil {
		state := w.Repo
		watcher, err := w.Watch(repo, func(meta bool) {
			w.post(func() {
				if w.Repo == state {
					w.Refresh(meta)
				}
			})
		})
		if err == nil {
			w.watcher = watcher
		}
	}
}

// Close closes the repository and returns to the welcome screen.
func (w *Workspace) Close() {
	if w.Repo != nil {
		w.Repo.Log.close()
	}
	if w.watcher != nil {
		w.watcher.Close()
		w.watcher = nil
	}
	w.Repo = nil
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
	rs.statusSeq++
	seq := rs.statusSeq
	repo := rs.Repo
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
		w.post(func() {
			if w.Repo != rs || rs.statusSeq != seq {
				return
			}
			rs.refreshing = false
			if err != nil {
				if gitcli.KindOf(err) == gitcli.ErrNotRepository {
					w.Notify(NoticeError, "Repository is gone", repo.Root)
					w.Close()
					return
				}
				w.NotifyError("Cannot read status", err)
			} else {
				rs.Status = st
				rs.StatusLoaded = true
				rs.StatusTime = w.Now()
				rs.Operation = op
				rs.Conflicts = conflicts
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
			if rs.refreshMore {
				rs.refreshMore = false
				w.Refresh(true)
			}
		})
	}()
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
