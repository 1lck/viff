package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/storage"
	"viff/internal/wslpath"
)

// RepositorySummary is one entry in the workspace-wide repository list. The
// list is what the user explicitly added, not the result of scanning the disk;
// every Status here is a cache that is rebuilt by querying Git again.
type RepositorySummary struct {
	// Root is the canonical working-tree root (symlinks resolved). Linked
	// worktrees keep their own Root and their own row: they share refs and
	// objects with the main checkout but they are separate working trees with
	// separate status.
	Root string
	// Status is the last known status. It is kept when a later query fails, so
	// a broken repository does not make the row look empty.
	Status domain.Status
	// Loaded reports that Status was read successfully at least once.
	Loaded bool
	// Err is the failure of the last status query; it is cleared by the next
	// success.
	Err error
}

var (
	// ErrRepositoryBusy refuses to switch away from, or remove, the active
	// repository while a write task is running: the task holds the repository
	// and its result would land in a different view.
	ErrRepositoryBusy = errors.New("a Git write task is running: wait for it to finish or cancel it before switching or removing the active repository")
	// ErrRepositoryUnknown reports a root that is not in the workspace list.
	ErrRepositoryUnknown = errors.New("repository is not in the workspace")
	// ErrRepositoryStale reports an activation that a newer one replaced, or
	// whose repository was removed before it finished.
	ErrRepositoryStale = errors.New("repository activation was replaced by a newer one")
)

// repoEntry is the mutable state behind one RepositorySummary. Only the UI
// thread touches it.
type repoEntry struct {
	root   string
	status domain.Status
	loaded bool
	err    error
	// seq identifies the latest status query for this entry. A result whose
	// entry is no longer in the list, or that is older than the latest query,
	// is dropped instead of resurrecting a deleted row or overwriting a newer
	// one.
	seq uint64
}

// repositoriesState is the private half of the repository list. Repositories
// and ActiveRoot are the exported, UI-readable view of it.
type repositoriesState struct {
	entries []*repoEntry
	byRoot  map[string]*repoEntry
	// activateSeq increments for each ActivateRepository call; an activation
	// that finishes after a newer one changed the active repository is stale.
	activateSeq uint64
	store       *storage.Repositories
}

// canonicalPath resolves a path to the real directory it names, following
// symlinks. It is the deduplication key: /tmp and /private/tmp (on macOS) name
// one repository and must not become two rows.
//
// A WSL path dedups by its normalized \\wsl.localhost spelling instead: the
// legacy \\wsl$ alias and distro-name casing are the same file system, and
// EvalSymlinks is both slow over 9P and wrong there — the Linux side can hold
// symlinks the Windows spelling must not resolve (wslpath; ADR 0036).
func canonicalPath(path string) string {
	if path == "" {
		return ""
	}
	if n, ok := wslpath.Normalize(path); ok {
		return n
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return filepath.Clean(abs)
}

// loadRepositories reads the persisted list on startup. It only loads: a
// repository that disappeared from disk is not dropped and not queried here.
func (w *Workspace) loadRepositories() {
	// The map must exist even without settings: a workspace that is created
	// directly (tests, a composition root without a settings file) still has to
	// accept repositories.
	w.repos.byRoot = make(map[string]*repoEntry)
	if w.Settings == nil {
		return
	}
	store := storage.LoadRepositories(w.Settings.Path())
	w.repos.store = store
	if store.Warning != "" {
		w.WorkspaceWarning = store.Warning
	}
	for _, saved := range store.Repositories {
		root := canonicalPath(saved)
		if root == "" || w.repos.byRoot[root] != nil {
			continue
		}
		w.addEntry(root)
	}
	// The persisted active repository is only a hint until it is opened; the
	// UI can show it as the active row or open it. Open and Activate keep it
	// up to date.
	if active := canonicalPath(store.Active); active != "" && w.repos.byRoot[active] != nil {
		w.ActiveRoot = active
	}
	w.syncRepositoryViews()
}

// addEntry appends a repository row; the caller has checked for duplicates.
func (w *Workspace) addEntry(root string) *repoEntry {
	if e := w.repos.byRoot[root]; e != nil {
		return e
	}
	e := &repoEntry{root: root}
	w.repos.entries = append(w.repos.entries, e)
	w.repos.byRoot[root] = e
	w.syncRepositoryViews()
	return e
}

// removeEntry drops a row and invalidates its in-flight status queries.
func (w *Workspace) removeEntry(e *repoEntry) {
	delete(w.repos.byRoot, e.root)
	for i, x := range w.repos.entries {
		if x == e {
			w.repos.entries = append(w.repos.entries[:i:i], w.repos.entries[i+1:]...)
			break
		}
	}
	e.seq++
	w.syncRepositoryViews()
}

// syncRepositoryViews rebuilds the exported list from the entries, so the UI
// always reads one consistent snapshot.
func (w *Workspace) syncRepositoryViews() {
	if len(w.repos.entries) == 0 {
		w.Repositories = nil
		return
	}
	views := make([]RepositorySummary, len(w.repos.entries))
	for i, e := range w.repos.entries {
		views[i] = RepositorySummary{Root: e.root, Status: e.status, Loaded: e.loaded, Err: e.err}
	}
	w.Repositories = views
}

// saveRepositories writes the list and the active root. It is called on the UI
// thread after every change; the file is small and Save is atomic. The
// in-memory list stays usable when the write fails (a read-only file written
// by a newer Viff keeps its contents), but the failure is never swallowed: it
// sets WorkspaceWarning and notifies once per failure streak.
func (w *Workspace) saveRepositories() error {
	store := w.repos.store
	if store == nil {
		return nil
	}
	roots := make([]string, len(w.repos.entries))
	for i, e := range w.repos.entries {
		roots[i] = e.root
	}
	store.Repositories = roots
	store.Active = w.ActiveRoot
	if err := store.Save(); err != nil {
		w.WorkspaceWarning = w.tr(i18n.AppNoticeWorkspaceSaveFailed, err.Error())
		if !w.workspaceSaveWarned {
			w.workspaceSaveWarned = true
			w.Notify(NoticeWarning, w.tr(i18n.AppNoticeWorkspace), w.WorkspaceWarning)
		}
		return err
	}
	w.workspaceSaveWarned = false
	return nil
}

// noteActiveRepository makes repo part of the list and the active repository,
// and persists both. It returns the canonical root.
func (w *Workspace) noteActiveRepository(repo domain.Repo) string {
	root := canonicalPath(repo.Root)
	entry := w.repos.byRoot[root]
	if entry == nil {
		entry = w.addEntry(root)
	}
	entry.err = nil
	w.ActiveRoot = root
	w.saveRepositories()
	return root
}

// syncActiveSummary copies the active repository's fresh state into its list
// row. statusErr is the failure of the query that produced the state: known
// status is kept and the error stays visible.
func (w *Workspace) syncActiveSummary(statusErr error) {
	rs := w.Repo
	if rs == nil {
		return
	}
	entry := w.repos.byRoot[rs.rootKey]
	if entry == nil {
		return
	}
	if statusErr != nil {
		entry.err = statusErr
	} else if rs.StatusLoaded {
		// The repository view is authoritative for the active row, and this
		// state is newer than any overview query that may still be running.
		// Bumping the sequence drops those older results instead of letting
		// them overwrite the status a finished write just produced.
		entry.seq++
		entry.status = rs.Status
		entry.loaded = true
		entry.err = nil
	}
	w.syncRepositoryViews()
}

// markRepositoryError records a status failure for a row that is not
// necessarily open. The known status is left untouched.
func (w *Workspace) markRepositoryError(root string, err error) {
	if root == "" || err == nil {
		return
	}
	if entry := w.repos.byRoot[root]; entry != nil {
		entry.err = err
		w.syncRepositoryViews()
	}
}

// AddRepository adds the repository containing dir to the workspace list. It
// never changes the active repository: switching is explicit through
// ActivateRepository. The error is reported on the UI thread.
func (w *Workspace) AddRepository(dir string, done func(error)) {
	_, ctx, finish := w.startTask("Adding "+filepath.Base(dir), false)
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
			w.addDiscoveredRepository(repo)
			if done != nil {
				done(nil)
			}
		})
	}()
}

// addDiscoveredRepository records a discovered repository. A repository that
// is already listed is a no-op, however it was spelled (symlinks, trailing
// slashes) and whether or not it is the active one.
func (w *Workspace) addDiscoveredRepository(repo domain.Repo) {
	root := canonicalPath(repo.Root)
	if root == "" || w.repos.byRoot[root] != nil {
		return
	}
	w.addEntry(root)
	w.saveRepositories()
}

// ActivateRepository makes root the active repository: subsequent Git
// commands, the log and the file watcher belong to it. The switch waits for
// discovery here (it is cheap) and then reuses the normal setRepo path, which
// bumps the generation and reloads status, refs and the log. A write task in
// progress refuses the switch so its result cannot land in another view.
func (w *Workspace) ActivateRepository(root string, done func(error)) {
	key := canonicalPath(root)
	entry := w.repos.byRoot[key]
	if entry == nil {
		if done != nil {
			done(fmt.Errorf("%w: %s", ErrRepositoryUnknown, root))
		}
		return
	}
	if w.Repo != nil && w.Repo.rootKey == entry.root {
		// Selecting the active repository again cancels a switch to another one
		// that is still discovering: the user's latest click must win instead of
		// being stolen by the older activation.
		w.repos.activateSeq++
		if done != nil {
			done(nil)
		}
		return
	}
	if w.Busy() {
		if done != nil {
			done(ErrRepositoryBusy)
		}
		return
	}
	w.repos.activateSeq++
	seq := w.repos.activateSeq
	_, ctx, finish := w.startTask("Opening "+filepath.Base(entry.root), false)
	go func() {
		repo, err := w.Git.Discover(ctx, entry.root)
		w.post(func() {
			finish()
			// The row may have been removed while discovery ran; a newer
			// activation may have won. Neither may revive the old selection.
			if w.repos.byRoot[entry.root] != entry || seq != w.repos.activateSeq {
				if done != nil {
					done(ErrRepositoryStale)
				}
				return
			}
			if err != nil {
				// Keep the last known status; only the error is new.
				entry.err = err
				w.syncRepositoryViews()
				if done != nil {
					done(err)
				}
				return
			}
			// Discovery takes a moment; a write task may have started in the
			// meantime. Switching now would move that write's result into a
			// different view, so refuse like the synchronous check does.
			if w.Busy() {
				if done != nil {
					done(ErrRepositoryBusy)
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

// RemoveRepository removes root from the workspace list. It never touches the
// working tree: the directory, the Git data and uncommitted changes stay where
// they are. Removing the active repository releases its log and watcher, then
// shows the welcome screen (the remaining rows stay listed, so the UI can
// activate one of them).
func (w *Workspace) RemoveRepository(root string) error {
	key := canonicalPath(root)
	entry := w.repos.byRoot[key]
	if entry == nil {
		return fmt.Errorf("%w: %s", ErrRepositoryUnknown, root)
	}
	active := w.Repo != nil && w.Repo.rootKey == entry.root
	if active && w.Busy() {
		return ErrRepositoryBusy
	}
	w.removeEntry(entry)
	switch {
	case active:
		// Close releases the repository-specific watcher and log state. The
		// list itself is persisted by the save below and by Close, so closing
		// never empties the workspace.
		w.Close()
		w.ActiveRoot = ""
		w.saveRepositories()
	case w.ActiveRoot == entry.root:
		// The persisted active row was removed without being open.
		w.ActiveRoot = ""
		w.saveRepositories()
	default:
		w.saveRepositories()
	}
	return nil
}

// RefreshRepositories queries the status of every listed repository, including
// the active one. Non-active repositories have no watcher in this round, so
// this is the explicit refresh path (opening the overview and its Refresh
// action); without it a non-active row only shows its last known state. done
// runs on the UI thread after every row has been updated.
func (w *Workspace) RefreshRepositories(done func()) {
	entries := make([]*repoEntry, len(w.repos.entries))
	copy(entries, w.repos.entries)
	if len(entries) == 0 {
		if done != nil {
			done()
		}
		return
	}
	var pending sync.WaitGroup
	for _, entry := range entries {
		entry.seq++
		seq := entry.seq
		root := entry.root
		pending.Add(1)
		go func() {
			defer pending.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			status, err := w.Git.Status(ctx, domain.Repo{Root: root})
			w.post(func() { w.applyRepositoryStatus(entry, seq, status, err) })
		}()
	}
	go func() {
		pending.Wait()
		w.post(func() {
			if done != nil {
				done()
			}
		})
	}()
}

// applyRepositoryStatus stores one refresh result. Results for a removed row
// or an outdated query are dropped: they must not resurrect a deleted row or
// replace a newer result.
func (w *Workspace) applyRepositoryStatus(entry *repoEntry, seq uint64, status domain.Status, err error) {
	if w.repos.byRoot[entry.root] != entry || entry.seq != seq {
		return
	}
	if err != nil {
		// Keep the last known status: a failure must stay visible instead of
		// blanking the row.
		entry.err = err
	} else {
		entry.status = status
		entry.loaded = true
		entry.err = nil
	}
	w.syncRepositoryViews()
}
