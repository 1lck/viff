// Package storage keeps Viff's own settings. It never stores Git state:
// repositories are always read from Git. Writes are atomic (temporary file
// and rename); a settings file written by a newer version is left untouched
// and reported instead of silently overwritten.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SchemaVersion is the settings format this build writes.
const SchemaVersion = 1

// Settings is the persisted configuration.
type Settings struct {
	Version int `json:"version"`

	Theme      string  `json:"theme"`    // "dark", "light" or "system"
	UIScale    float32 `json:"uiScale"`  // 1 = 100%
	FontSize   float32 `json:"fontSize"` // editor font size
	EditorFont string  `json:"editorFont"`

	GitPath       string `json:"gitPath"`
	AutoFetch     bool   `json:"autoFetch"`
	AutoFetchMins int    `json:"autoFetchMinutes"`
	FetchPrune    bool   `json:"fetchPrune"`
	PullMode      string `json:"pullMode"` // "merge", "rebase", "ff-only"

	// Git settings passed to every invocation. Empty values leave Git's own
	// configuration and defaults in charge, so an old settings file (which
	// has none of these fields) keeps behaving exactly as before.
	SSHCommand       string `json:"sshCommand,omitempty"`
	GPGProgram       string `json:"gpgProgram,omitempty"`
	SigningKey       string `json:"signingKey,omitempty"`
	SignFormat       string `json:"signFormat,omitempty"` // "", "openpgp" or "ssh"
	CommitSigning    bool   `json:"commitSigning,omitempty"`
	CredentialHelper string `json:"credentialHelper,omitempty"`

	ExternalEditor string `json:"externalEditor"`

	Recent []Recent `json:"recent"`

	Window Window `json:"window"`

	// Layout keeps panel proportions.
	CommitPanelWidth   float32 `json:"commitPanelWidth"`
	GitPanelHeight     float32 `json:"gitPanelHeight"`
	BranchesWidth      float32 `json:"branchesWidth"`
	DetailsWidth       float32 `json:"detailsWidth"`
	DiffUnified        bool    `json:"diffUnified"`
	DiffWhitespace     int     `json:"diffWhitespace"`
	DiffSyncScroll     bool    `json:"diffSyncScroll"`
	DiffHighlightWords bool    `json:"diffHighlightWords"`
	CommitTree         bool    `json:"commitTree"`

	path     string
	readOnly bool
	mu       sync.Mutex
	// Warning is set when the file could not be used as is.
	Warning string `json:"-"`
}

// Recent is a recently opened repository.
type Recent struct {
	Path   string    `json:"path"`
	Opened time.Time `json:"opened"`
}

// Window is the last window geometry.
type Window struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Defaults returns the settings of a first start.
func Defaults() *Settings {
	return &Settings{
		Version: SchemaVersion, Theme: "dark", UIScale: 1, FontSize: 13,
		AutoFetch: false, AutoFetchMins: 10, FetchPrune: true, PullMode: "merge",
		Window:           Window{Width: 1440, Height: 900},
		CommitPanelWidth: 340, GitPanelHeight: 330, BranchesWidth: 220, DetailsWidth: 360,
		DiffSyncScroll: true, DiffHighlightWords: true, CommitTree: false,
	}
}

// DefaultPath is the settings file in the user's configuration directory.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Viff", "settings.json")
}

// Load reads settings from path, migrating older versions. A missing file
// gives defaults. A corrupt file is moved aside and defaults are used.
func Load(path string) *Settings {
	s := Defaults()
	s.path = path
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err != nil {
		s.Warning = "Cannot read settings: " + err.Error()
		s.readOnly = true
		return s
	}
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		backup := path + ".corrupt-" + time.Now().Format("20060102-150405")
		_ = os.Rename(path, backup)
		s.Warning = "Settings were unreadable and were reset. The old file is " + backup
		return s
	}
	if probe.Version > SchemaVersion {
		s.Warning = fmt.Sprintf("Settings were written by a newer Viff (format %d). They are used read-only and will not be overwritten.", probe.Version)
		s.readOnly = true
	}
	if probe.Version < SchemaVersion {
		_ = os.WriteFile(path+fmt.Sprintf(".v%d.bak", probe.Version), data, 0o600)
	}
	if err := json.Unmarshal(data, s); err != nil {
		s.Warning = "Some settings could not be read: " + err.Error()
	}
	migrate(s, probe.Version)
	s.path = path
	s.sanitize()
	return s
}

// migrate upgrades older formats step by step.
func migrate(s *Settings, from int) {
	if from < 1 {
		// Version 0 had no version field; nothing to convert yet.
	}
	if !s.readOnly {
		s.Version = SchemaVersion
	}
}

func (s *Settings) sanitize() {
	d := Defaults()
	if s.UIScale < 0.75 || s.UIScale > 2 {
		s.UIScale = 1
	}
	if s.FontSize < 9 || s.FontSize > 28 {
		s.FontSize = d.FontSize
	}
	if s.Theme != "dark" && s.Theme != "light" && s.Theme != "system" {
		s.Theme = d.Theme
	}
	if s.AutoFetchMins < 1 {
		s.AutoFetchMins = d.AutoFetchMins
	}
	if s.Window.Width < 800 || s.Window.Height < 500 {
		s.Window = d.Window
	}
	if s.CommitPanelWidth < 200 {
		s.CommitPanelWidth = d.CommitPanelWidth
	}
	if s.GitPanelHeight < 120 {
		s.GitPanelHeight = d.GitPanelHeight
	}
	if s.BranchesWidth < 120 {
		s.BranchesWidth = d.BranchesWidth
	}
	if s.DetailsWidth < 160 {
		s.DetailsWidth = d.DetailsWidth
	}
	if s.DiffWhitespace < 0 || s.DiffWhitespace > 2 {
		s.DiffWhitespace = 0
	}
	switch s.PullMode {
	case "merge", "rebase", "ff-only":
	default:
		s.PullMode = d.PullMode
	}
	switch s.SignFormat {
	case "", "openpgp", "ssh":
	default:
		s.SignFormat = ""
	}
	if len(s.Recent) > 20 {
		s.Recent = s.Recent[:20]
	}
}

// AddRecent moves path to the top of the recent list.
func (s *Settings) AddRecent(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := []Recent{{Path: path, Opened: time.Now()}}
	for _, r := range s.Recent {
		if r.Path != path {
			list = append(list, r)
		}
	}
	if len(list) > 20 {
		list = list[:20]
	}
	s.Recent = list
}

// RemoveRecent forgets a repository.
func (s *Settings) RemoveRecent(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.Recent {
		if r.Path == path {
			s.Recent = append(s.Recent[:i:i], s.Recent[i+1:]...)
			return
		}
	}
}

// Save writes the settings atomically. It does nothing for read-only
// settings or settings without a path (tests).
func (s *Settings) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" || s.readOnly {
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*.tmp")
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
	return os.Rename(tmp.Name(), s.path)
}

// Path returns where the settings are stored.
func (s *Settings) Path() string { return s.path }
