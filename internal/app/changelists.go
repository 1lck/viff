package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"viff/internal/storage"
)

// ChangeList is one local, purely organizational group of changed files. It
// is bookkeeping for the UI: it never changes Git state, so staging,
// committing, stashing and the index all keep working as before. A path
// belongs to exactly one list; unassigned paths belong to the default list.
type ChangeList struct {
	ID   string
	Name string
}

// ChangeListManager keeps the per-repository change lists and their
// membership. One repository is identified by its canonical root, so two
// repositories that both contain "same.txt" keep independent lists, and a
// linked worktree (a different root) has its own lists too.
//
// The manager is created lazily by Workspace.changeLists and its document is
// only ever replaced as a whole after a successful save: a failed write leaves
// both memory and the old file untouched.
//
// Reads are cheap: the published document is immutable, so Lists and
// ChangeListFor look at it directly instead of copying it, and the canonical
// root of a repository is computed once. Only a change copies the document it
// is about to replace.
type ChangeListManager struct {
	mu sync.Mutex

	// path is the JSON file next to the settings; "" disables persistence
	// (settings without a path, mostly tests).
	path string

	doc     *storage.ChangeLists
	loaded  bool
	loadErr error
	warned  bool

	// roots caches the canonical key of a Repo.Root after the first use, so a
	// read never resolves symlinks again.
	roots map[string]string
	// renames caches the rename map of the current status snapshot (new path
	// -> old path). A status snapshot is replaced as a whole, so the map is
	// rebuilt only when that snapshot changes.
	renameRepo *RepoState
	renameTime time.Time
	renames    map[string]string
}

var errChangeListNoRepo = errors.New("no repository is open")

func newChangeListManager(settingsPath string) *ChangeListManager {
	return &ChangeListManager{path: storage.ChangeListsPath(settingsPath)}
}

// changeLists returns the manager, creating it on first use. Workspace owns the
// field; this file only fills it in so a workspace that never shows change
// lists pays nothing.
func (w *Workspace) changeLists() *ChangeListManager {
	if m := w.ChangeLists; m != nil {
		return m
	}
	settingsPath := ""
	if w.Settings != nil {
		settingsPath = w.Settings.Path()
	}
	m := newChangeListManager(settingsPath)
	w.ChangeLists = m
	return m
}

// changeListRepoRoot is the root Git reported for the active repository. Every
// read and write is keyed by its canonical form, so switching repositories
// cannot carry membership across repositories and a stale repository state
// cannot answer for the new one.
func (w *Workspace) changeListRepoRoot() (string, bool) {
	if w.Repo == nil || w.Repo.Repo.Root == "" {
		return "", false
	}
	return w.Repo.Repo.Root, true
}

// canonicalRootLocked maps root to the key used in the metadata file, resolving
// symlinks once per distinct root. The caller holds m.mu.
func (m *ChangeListManager) canonicalRootLocked(root string) string {
	if m.roots == nil {
		m.roots = make(map[string]string)
	}
	if canonical, ok := m.roots[root]; ok {
		return canonical
	}
	canonical := filepath.Clean(root)
	if abs, err := filepath.Abs(canonical); err == nil {
		canonical = abs
	}
	if resolved, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = resolved
	}
	m.roots[root] = canonical
	return canonical
}

// docLocked loads the document once and caches the result. A corrupt file
// never makes the repository unusable: reads degrade to the default list and
// every change reports the error instead of overwriting the file.
func (m *ChangeListManager) docLocked() (*storage.ChangeLists, error) {
	if m.loaded {
		return m.doc, m.loadErr
	}
	m.loaded = true
	if m.path == "" {
		m.doc = storage.NewChangeLists()
		return m.doc, nil
	}
	doc, err := storage.LoadChangeLists(m.path)
	if err != nil {
		m.doc = storage.NewChangeLists()
		m.loadErr = err
		return m.doc, err
	}
	m.doc = doc
	return doc, nil
}

// warnLocked reports a broken file once instead of on every frame. The caller
// holds m.mu; w.Notify must not touch the manager.
func (w *Workspace) warnChangeLists(m *ChangeListManager, err error) {
	if err == nil || m.warned {
		return
	}
	m.warned = true
	w.Notify(NoticeWarning, "Change lists are unavailable", err.Error())
}

// renamesLocked maps a renamed path's new name to its old name for the current
// status snapshot. Only "R" entries count: a "C" (copied) entry also carries
// OrigPath, but there the source path still exists and keeps its list.
func (m *ChangeListManager) renamesLocked(rs *RepoState) map[string]string {
	if m.renameRepo == rs && m.renameTime.Equal(rs.StatusTime) && m.renames != nil {
		return m.renames
	}
	renames := make(map[string]string)
	for _, e := range rs.Status.Entries {
		if e.OrigPath == "" || e.OrigPath == e.Path {
			continue
		}
		if e.Index == 'R' || e.Worktree == 'R' {
			renames[e.Path] = e.OrigPath
		}
	}
	m.renameRepo, m.renameTime, m.renames = rs, rs.StatusTime, renames
	return renames
}

// changeListRepoCopy returns a detached copy of root's record with the default
// list guaranteed in front and a non-nil member map. Only mutation paths use
// it; reads look at the immutable document directly.
func changeListRepoCopy(doc *storage.ChangeLists, root string) storage.RepoChangeLists {
	repo, ok := doc.Repos[root]
	out := storage.RepoChangeLists{
		Lists:   append([]storage.ChangeListRecord(nil), repo.Lists...),
		Members: make(map[string]string, len(repo.Members)),
	}
	for p, id := range repo.Members {
		out.Members[p] = id
	}
	if !ok || len(out.Lists) == 0 || out.Lists[0].ID != storage.DefaultChangeListID {
		out.Lists = append([]storage.ChangeListRecord{{ID: storage.DefaultChangeListID, Name: storage.DefaultChangeListName}}, out.Lists...)
	}
	return out
}

func changeListIndex(repo storage.RepoChangeLists, id string) int {
	for i, l := range repo.Lists {
		if l.ID == id {
			return i
		}
	}
	return -1
}

func changeListExists(repo storage.RepoChangeLists, id string) bool {
	return changeListIndex(repo, id) >= 0
}

// changeListNamed reports whether name is already used by another list.
// Comparison ignores case so "Default" and "default" cannot both exist.
func changeListNamed(repo storage.RepoChangeLists, name, exceptID string) bool {
	for _, l := range repo.Lists {
		if l.ID != exceptID && strings.EqualFold(l.Name, name) {
			return true
		}
	}
	return false
}

// newChangeListID returns an ID that is stable across renames and reloads and
// does not depend on a list's position or name.
func newChangeListID(repo storage.RepoChangeLists) string {
	for i := 0; i < 100; i++ {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			break
		}
		id := "list-" + hex.EncodeToString(b[:])
		if !changeListExists(repo, id) {
			return id
		}
	}
	for n := 1; ; n++ {
		id := fmt.Sprintf("list-%d", n)
		if !changeListExists(repo, id) {
			return id
		}
	}
}

// changeListUpdate applies fn to a copy of the active repository's record and
// writes the whole document. fn reports whether anything changed; when it did
// not, nothing is written. On any error the cached document, and therefore
// every read, keeps the last successfully saved state.
func (w *Workspace) changeListUpdate(root string, fn func(repo *storage.RepoChangeLists) (bool, error)) error {
	m := w.changeLists()
	m.mu.Lock()
	defer m.mu.Unlock()

	var doc *storage.ChangeLists
	if m.path == "" {
		loaded, err := m.docLocked()
		if err != nil {
			return err
		}
		doc = loaded
	} else {
		// Read the file again: another window (or a manual edit) may have
		// changed it since this manager loaded it. A broken file is reported
		// instead of being replaced by this manager's stale copy.
		loaded, err := storage.LoadChangeLists(m.path)
		if err != nil {
			m.loadErr = err
			return err
		}
		doc = loaded
	}
	if doc.ReadOnly() {
		return storage.ErrChangeListsReadOnly
	}
	next := doc.Clone()
	key := m.canonicalRootLocked(root)
	repo := changeListRepoCopy(next, key)
	changed, err := fn(&repo)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	next.Repos[key] = repo
	if m.path != "" {
		if err := next.Save(m.path); err != nil {
			return err
		}
	}
	m.doc = next
	m.loaded = true
	m.loadErr = nil
	m.warned = false
	return nil
}

// Lists returns the change lists of the active repository, default first.
// Without a repository it returns nil; a corrupt metadata file degrades to the
// default list and reports the problem once as a notification.
func (w *Workspace) Lists() []ChangeList {
	root, ok := w.changeListRepoRoot()
	if !ok {
		return nil
	}
	m := w.changeLists()
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.docLocked()
	if err != nil {
		w.warnChangeLists(m, err)
		return []ChangeList{{ID: storage.DefaultChangeListID, Name: storage.DefaultChangeListName}}
	}
	repo := doc.Repos[m.canonicalRootLocked(root)]
	out := make([]ChangeList, 0, len(repo.Lists)+1)
	out = append(out, ChangeList{ID: storage.DefaultChangeListID, Name: storage.DefaultChangeListName})
	for _, l := range repo.Lists {
		if l.ID == storage.DefaultChangeListID {
			continue
		}
		out = append(out, ChangeList{ID: l.ID, Name: l.Name})
	}
	return out
}

// CreateChangeList adds a list and returns its stable ID. Empty names and
// duplicate names are refused; nothing is written when validation fails.
func (w *Workspace) CreateChangeList(name string) (string, error) {
	root, ok := w.changeListRepoRoot()
	if !ok {
		return "", errChangeListNoRepo
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("change list name is empty")
	}
	var id string
	err := w.changeListUpdate(root, func(repo *storage.RepoChangeLists) (bool, error) {
		if changeListNamed(*repo, name, "") {
			return false, fmt.Errorf("a change list named %q already exists", name)
		}
		id = newChangeListID(*repo)
		repo.Lists = append(repo.Lists, storage.ChangeListRecord{ID: id, Name: name})
		return true, nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// RenameChangeList renames a list. The default list cannot be renamed and the
// new name must be non-empty and unused.
func (w *Workspace) RenameChangeList(id, name string) error {
	root, ok := w.changeListRepoRoot()
	if !ok {
		return errChangeListNoRepo
	}
	if id == storage.DefaultChangeListID {
		return errors.New("the default change list cannot be renamed")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("change list name is empty")
	}
	return w.changeListUpdate(root, func(repo *storage.RepoChangeLists) (bool, error) {
		index := changeListIndex(*repo, id)
		if index < 0 {
			return false, fmt.Errorf("unknown change list %q", id)
		}
		if changeListNamed(*repo, name, id) {
			return false, fmt.Errorf("a change list named %q already exists", name)
		}
		repo.Lists[index].Name = name
		return true, nil
	})
}

// DeleteChangeList removes a list and returns its paths to the default list.
// Files, the index and the working tree are not touched. The default list
// cannot be deleted.
func (w *Workspace) DeleteChangeList(id string) error {
	root, ok := w.changeListRepoRoot()
	if !ok {
		return errChangeListNoRepo
	}
	if id == storage.DefaultChangeListID {
		return errors.New("the default change list cannot be deleted")
	}
	return w.changeListUpdate(root, func(repo *storage.RepoChangeLists) (bool, error) {
		index := changeListIndex(*repo, id)
		if index < 0 {
			return false, fmt.Errorf("unknown change list %q", id)
		}
		repo.Lists = append(repo.Lists[:index:index], repo.Lists[index+1:]...)
		for p, listID := range repo.Members {
			if listID == id {
				delete(repo.Members, p)
			}
		}
		return true, nil
	})
}

// MoveToChangeList assigns paths to a list. The default list is the implicit
// home of unassigned paths, so moving a path there just forgets its membership.
// Paths must be clean repository-relative names; they do not have to appear in
// the current status, because a path keeps its list while it is committed and
// clean, or temporarily gone from status.
func (w *Workspace) MoveToChangeList(paths []string, id string) error {
	root, ok := w.changeListRepoRoot()
	if !ok {
		return errChangeListNoRepo
	}
	if id == "" {
		return errors.New("change list is empty")
	}
	clean := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if err := validateChangeListPath(p); err != nil {
			return err
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return nil
	}
	return w.changeListUpdate(root, func(repo *storage.RepoChangeLists) (bool, error) {
		if id != storage.DefaultChangeListID && !changeListExists(*repo, id) {
			return false, fmt.Errorf("unknown change list %q", id)
		}
		for _, p := range clean {
			if id == storage.DefaultChangeListID {
				delete(repo.Members, p)
			} else {
				repo.Members[p] = id
			}
		}
		return true, nil
	})
}

// validateChangeListPath accepts only clean, repository-relative paths. Status
// paths are already in that form; this keeps a wrong caller from writing
// absolute paths or traversal into the metadata file.
func validateChangeListPath(p string) error {
	if p == "" {
		return errors.New("path is empty")
	}
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("path %q is invalid", p)
	}
	if strings.HasPrefix(p, "/") || filepath.IsAbs(p) || path.IsAbs(p) {
		return fmt.Errorf("path %q must be relative to the repository", p)
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return fmt.Errorf("path %q must not leave the repository", p)
		}
	}
	if path.Clean(p) != p || p == "." {
		return fmt.Errorf("path %q is not a clean repository-relative path", p)
	}
	return nil
}

// ChangeListFor returns the list a path belongs to. Unknown and unassigned
// paths belong to the default list. A rename that Git still reports as
// "old -> new" (StatusEntry.OrigPath) inherits the old path's list even before
// SyncChangeListRenames persists the move, so the UI never flashes a renamed
// file back to the default list. The rename map is cached per status snapshot,
// so per-row calls stay O(1).
func (w *Workspace) ChangeListFor(p string) string {
	root, ok := w.changeListRepoRoot()
	if !ok {
		return storage.DefaultChangeListID
	}
	m := w.changeLists()
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.docLocked()
	if err != nil {
		w.warnChangeLists(m, err)
		return storage.DefaultChangeListID
	}
	repo := doc.Repos[m.canonicalRootLocked(root)]
	if id, ok := repo.Members[p]; ok && changeListExists(repo, id) {
		return id
	}
	if w.Repo != nil {
		if old, ok := m.renamesLocked(w.Repo)[p]; ok {
			if id, ok := repo.Members[old]; ok && changeListExists(repo, id) {
				return id
			}
		}
	}
	return storage.DefaultChangeListID
}

// SyncChangeListRenames persists the membership move of renamed paths: Git
// reports a rename as one status entry whose OrigPath is the old name, and the
// old name disappears from status, so without this the list would stay
// attached to a path nothing shows any more. A copied path ("C") is left
// alone: its source still exists and must keep its list.
//
// Workspace.Refresh calls this on the UI thread after it applies a successful
// status snapshot; it does nothing when no rename touches a list.
func (w *Workspace) SyncChangeListRenames() error {
	root, ok := w.changeListRepoRoot()
	if !ok || w.Repo == nil {
		return nil
	}
	renames := make(map[string]string) // old path -> new path, renames only
	for _, e := range w.Repo.Status.Entries {
		if e.OrigPath == "" || e.OrigPath == e.Path {
			continue
		}
		if e.Index == 'R' || e.Worktree == 'R' {
			renames[e.OrigPath] = e.Path
		}
	}
	if len(renames) == 0 {
		return nil
	}
	return w.changeListUpdate(root, func(repo *storage.RepoChangeLists) (bool, error) {
		changed := false
		for old, newPath := range renames {
			id, ok := repo.Members[old]
			if !ok {
				continue
			}
			delete(repo.Members, old)
			// An explicit assignment of the new path wins over the inherited
			// one; only an unassigned new path inherits the old list.
			if _, assigned := repo.Members[newPath]; !assigned {
				repo.Members[newPath] = id
			}
			changed = true
		}
		return changed, nil
	})
}
