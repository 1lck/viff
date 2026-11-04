package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// WorkspaceSchemaVersion is the workspace file format this build writes. It is
// deliberately separate from SchemaVersion: the settings file keeps its
// format, so a Viff without multi-repository support never sees or rewrites
// the repository list. A workspace written by a newer Viff is read-only.
const WorkspaceSchemaVersion = 1

// ErrWorkspaceReadOnly reports a workspace file this build must not
// overwrite, because a newer Viff wrote it or it could not be read. The file
// and its content are preserved.
var ErrWorkspaceReadOnly = errors.New("workspace list is read-only")

// Repositories is the persisted multi-repository workspace: the explicit list
// of repository roots and which one was active. It lives in its own file next
// to the settings file. The list is a user decision, not a disk scan: only
// repositories the user added are stored here.
type Repositories struct {
	Version      int      `json:"version"`
	Repositories []string `json:"repositories"`
	Active       string   `json:"active,omitempty"`

	path     string
	readOnly bool
	mu       sync.Mutex
	// Warning is set when the file could not be used as is.
	Warning string `json:"-"`
}

// WorkspacePath is the workspace file for a settings file. The list is stored
// beside the settings instead of inside them so the settings format does not
// change; settings without a path (tests, first start before a config
// directory exists) keep the list in memory only.
func WorkspacePath(settingsPath string) string {
	if settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), "workspace.json")
}

// LoadRepositories reads the workspace list for settingsPath. A missing file
// gives an empty list. Any unreadable or malformed file is used read-only and
// left exactly as it is: overwriting it could destroy entries this build does
// not understand (a newer format, or data a future version could repair).
// Only a successfully parsed current or older format may be written back.
func LoadRepositories(settingsPath string) *Repositories {
	r := &Repositories{Version: WorkspaceSchemaVersion, path: WorkspacePath(settingsPath)}
	if r.path == "" {
		return r
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return r
	}
	if err != nil {
		r.Warning = "Cannot read the workspace list: " + err.Error()
		r.readOnly = true
		return r
	}
	if err := json.Unmarshal(data, r); err != nil {
		r.Warning = "The workspace list could not be read and is kept read-only: " + err.Error()
		r.readOnly = true
		r.Repositories, r.Active = nil, ""
		return r
	}
	if r.Version < 0 {
		r.Warning = fmt.Sprintf("The workspace list has an invalid format version %d and is kept read-only.", r.Version)
		r.readOnly = true
		r.Repositories, r.Active = nil, ""
		return r
	}
	if r.Version > WorkspaceSchemaVersion {
		r.Warning = fmt.Sprintf("The workspace list was written by a newer Viff (format %d). It is used read-only and will not be overwritten.", r.Version)
		r.readOnly = true
	}
	if !r.readOnly {
		r.Version = WorkspaceSchemaVersion
	}
	r.sanitize()
	return r
}

// sanitize drops entries that cannot name a repository. Roots are kept as
// written (the application canonicalizes them); empty strings would only make
// the list inconsistent.
func (r *Repositories) sanitize() {
	if len(r.Repositories) == 0 {
		r.Repositories = nil
		return
	}
	roots := r.Repositories[:0]
	for _, root := range r.Repositories {
		if root != "" {
			roots = append(roots, root)
		}
	}
	r.Repositories = roots
}

// Path returns where the workspace list is stored.
func (r *Repositories) Path() string { return r.path }

// ReadOnly reports that the file belongs to a newer Viff and is left as is.
func (r *Repositories) ReadOnly() bool { return r.readOnly }

// Save writes the workspace list atomically. A list without a path (tests)
// has nothing to write; a read-only file (newer Viff, unreadable) reports
// ErrWorkspaceReadOnly and is left untouched instead of being silently
// ignored.
func (r *Repositories) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path == "" {
		return nil
	}
	if r.readOnly {
		return ErrWorkspaceReadOnly
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".workspace-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), r.path)
}
