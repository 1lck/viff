package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ChangeListsVersion is the change list format this build writes. A file with
// a higher version belongs to a newer Viff: it is loaded read-only and is
// never overwritten.
const ChangeListsVersion = 1

// The list every repository starts with. The default list cannot be renamed or
// removed; unassigned paths belong to it.
const (
	DefaultChangeListID   = "default"
	DefaultChangeListName = "Default"
)

// ErrChangeListsReadOnly reports a change list file written by a newer Viff.
var ErrChangeListsReadOnly = errors.New("change lists were written by a newer Viff")

// ChangeListRecord is one named change list.
type ChangeListRecord struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RepoChangeLists is the local change list state of one repository, keyed by
// its canonical root. Linked worktrees have different roots and therefore
// keep independent lists.
type RepoChangeLists struct {
	Lists []ChangeListRecord `json:"lists"`
	// Members maps a repository-relative path to its list ID. A path that is
	// absent belongs to the default list.
	Members map[string]string `json:"members,omitempty"`
}

// ChangeLists is the persisted organization document. It lives next to the
// user's settings file, never in a repository or in Git's own metadata, and it
// stores nothing but names and paths: membership is bookkeeping, not Git
// state, so no index or working-tree file is changed by it.
type ChangeLists struct {
	Version int                        `json:"version"`
	Repos   map[string]RepoChangeLists `json:"repositories,omitempty"`

	readOnly bool
}

// NewChangeLists returns an empty writable document.
func NewChangeLists() *ChangeLists {
	return &ChangeLists{Version: ChangeListsVersion, Repos: make(map[string]RepoChangeLists)}
}

// ChangeListsPath is where the change lists of a settings file live: the same
// directory, in their own file. An empty settings path disables persistence.
func ChangeListsPath(settingsPath string) string {
	if settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), "changelists.json")
}

// ReadOnly reports a document written by a newer Viff. Its lists can be shown,
// but every change is refused so the file is never overwritten.
func (c *ChangeLists) ReadOnly() bool { return c.readOnly }

// LoadChangeLists reads the document at path. A missing file gives an empty
// document. A corrupt file is an error and is left untouched; a file from a
// newer Viff is returned read-only.
func LoadChangeLists(path string) (*ChangeLists, error) {
	if path == "" {
		return nil, errors.New("change lists: no path")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewChangeLists(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("change lists: cannot read %s: %w", path, err)
	}
	var c ChangeLists
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("change lists: %s is unreadable: %w", path, err)
	}
	if c.Version < 0 {
		return nil, fmt.Errorf("change lists: %s has an invalid version %d", path, c.Version)
	}
	if c.Version > ChangeListsVersion {
		c.readOnly = true
	} else {
		c.Version = ChangeListsVersion
	}
	// A valid file with inconsistent entries (no default list, a member of a
	// list that does not exist, an empty name) is repaired in memory instead
	// of being rejected; nothing is written back until a change is requested.
	c.normalize()
	return &c, nil
}

// Save writes the document atomically: a temporary file in the same directory,
// flushed and renamed over the target. It refuses a read-only document, nil
// or the read-only document keeps the old state in memory on failure.
func (c *ChangeLists) Save(path string) error {
	if c.readOnly {
		return ErrChangeListsReadOnly
	}
	if path == "" {
		return errors.New("change lists: no path")
	}
	c.Version = ChangeListsVersion
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("change lists: cannot encode: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("change lists: cannot create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".changelists-*.tmp")
	if err != nil {
		return fmt.Errorf("change lists: cannot create a temporary file in %s: %w", dir, err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("change lists: cannot write %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("change lists: cannot sync %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("change lists: cannot close %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return fmt.Errorf("change lists: cannot replace %s: %w", path, err)
	}
	return nil
}

// Clone returns a deep copy. Callers edit the copy and save it so a failed
// save cannot leave a half-applied change in memory.
func (c *ChangeLists) Clone() *ChangeLists {
	next := &ChangeLists{
		Version:  c.Version,
		Repos:    make(map[string]RepoChangeLists, len(c.Repos)),
		readOnly: c.readOnly,
	}
	for root, repo := range c.Repos {
		next.Repos[root] = RepoChangeLists{
			Lists:   append([]ChangeListRecord(nil), repo.Lists...),
			Members: make(map[string]string, len(repo.Members)),
		}
		for path, id := range repo.Members {
			next.Repos[root].Members[path] = id
		}
	}
	return next
}

// normalize repairs a loaded document in memory: it guarantees the default
// list in front of every repository, drops entries that name no list and
// forgets membership of lists that no longer exist (those paths fall back to
// the default list).
func (c *ChangeLists) normalize() {
	if c.Repos == nil {
		c.Repos = make(map[string]RepoChangeLists)
	}
	for root, repo := range c.Repos {
		repo.normalize()
		c.Repos[root] = repo
	}
}

func (r *RepoChangeLists) normalize() {
	var others []ChangeListRecord
	seen := map[string]bool{DefaultChangeListID: true}
	for _, l := range r.Lists {
		id := strings.TrimSpace(l.ID)
		name := strings.TrimSpace(l.Name)
		if id == "" || name == "" || id == DefaultChangeListID || seen[id] {
			continue
		}
		seen[id] = true
		others = append(others, ChangeListRecord{ID: id, Name: name})
	}
	r.Lists = append([]ChangeListRecord{{ID: DefaultChangeListID, Name: DefaultChangeListName}}, others...)
	if r.Members == nil {
		r.Members = make(map[string]string)
	}
	known := make(map[string]bool, len(r.Lists))
	for _, l := range r.Lists {
		known[l.ID] = true
	}
	for path, id := range r.Members {
		if path == "" || !known[id] {
			delete(r.Members, path)
		}
	}
}
