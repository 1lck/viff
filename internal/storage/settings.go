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
	"strings"
	"sync"
	"time"
)

// SchemaVersion is the settings format this build writes.
const SchemaVersion = 1

// Clock formats the Time format setting understands. TimeFormat24Hour is the
// default and the value an older settings file implies, so upgrading never
// changes how times read.
const (
	TimeFormat24Hour = "24h"
	TimeFormat12Hour = "12h"
)

// NormalizeTimeFormat maps anything this build does not understand — an empty
// field from an older settings file, or a value a newer build wrote — to the
// 24-hour default, so no caller has to guess what to render.
func NormalizeTimeFormat(value string) string {
	if value == TimeFormat12Hour {
		return TimeFormat12Hour
	}
	return TimeFormat24Hour
}

// The left dock's width. The default is the old Commit panel width, the wider
// of the two widths older files stored, so upgrading never narrows that panel.
// legacyProjectPanelWidth is the old Project default; migration compares it
// with the stored keys to tell which page the user resized.
const (
	DefaultLeftPanelWidth   float32 = 340
	MinLeftPanelWidth       float32 = 200
	legacyProjectPanelWidth float32 = 300
)

// MaxRecentProjects caps the Recent projects list. It is a convenience list,
// not the workspace: repositories the user added live in workspace.json and are
// not truncated. The cap used to be 20, which with a hundred repositories cut
// off every project the user had not opened lately; searching still reaches the
// workspace, so the list only has to stay short enough to scan.
const MaxRecentProjects = 50

// Settings is the persisted configuration.
type Settings struct {
	Version int `json:"version"`

	Theme      string  `json:"theme"`    // "dark", "light" or "system"
	UIScale    float32 `json:"uiScale"`  // 1 = 100%
	FontSize   float32 `json:"fontSize"` // editor font size
	EditorFont string  `json:"editorFont"`
	// DiffFont is the fixed-pitch family for diffs and the console. Empty keeps
	// the bundled face on macOS and the platform's monospace default elsewhere.
	DiffFont string `json:"diffFont,omitempty"`
	// Language names the interface language: "en" or "zh-CN". Empty means the
	// default, English, so an older settings file keeps working.
	Language string `json:"language,omitempty"`

	GitPath string `json:"gitPath"`
	// SVNPath is the svn executable; empty means PATH. It exists so a user
	// whose Subversion lives outside PATH (a common case on Windows) can point
	// Viff at it, the same way GitPath does for Git.
	SVNPath       string `json:"svnPath,omitempty"`
	AutoFetch     bool   `json:"autoFetch"`
	AutoFetchMins int    `json:"autoFetchMinutes"`
	FetchPrune    bool   `json:"fetchPrune"`
	PullMode      string `json:"pullMode"` // "merge", "rebase", "ff-only"

	// SkipUpdateDialog runs the Update Project shortcut with PullMode instead of
	// opening its dialog. The dialog's "Don't ask again" and the Git settings
	// page write the same field, so the choice stays reversible.
	SkipUpdateDialog bool `json:"skipUpdateDialog,omitempty"`

	// AutoUpdateCheck asks GitHub Releases for a newer version at startup. It
	// defaults to true: Decode keeps the default for an old settings file that
	// has no field, while an explicit false is stored and respected.
	AutoUpdateCheck bool `json:"autoUpdateCheck"`

	// CheckoutOnDoubleClick makes a double-click on a branch row check that
	// branch out. It defaults to false: checking out rewrites the working tree,
	// which is a lot to do on a second click, and the row's context menu always
	// offers the action explicitly. An older settings file has no field, and
	// Load unmarshals into Defaults, so upgrading never turns the old behavior
	// back on.
	CheckoutOnDoubleClick bool `json:"checkoutOnDoubleClick,omitempty"`

	// LogLevel is the file log level: "debug" keeps verbose diagnostics,
	// empty or "info" keeps the default. It has no effect on behavior, so an
	// old settings file without the field keeps working unchanged.
	LogLevel string `json:"logLevel,omitempty"`

	// Git settings passed to every invocation. Empty values leave Git's own
	// configuration and defaults in charge, so an old settings file (which
	// has none of these fields) keeps behaving exactly as before.
	SSHCommand       string `json:"sshCommand,omitempty"`
	GPGProgram       string `json:"gpgProgram,omitempty"`
	SigningKey       string `json:"signingKey,omitempty"`
	SignFormat       string `json:"signFormat,omitempty"` // "", "openpgp" or "ssh"
	CommitSigning    bool   `json:"commitSigning,omitempty"`
	CredentialHelper string `json:"credentialHelper,omitempty"`

	// AI endpoint for the commit message generator: an OpenAI-compatible base
	// URL such as https://api.openai.com/v1, the model name, and the key. The
	// key is stored in plain text on purpose: this is a local single-user
	// application, and the file already holds SSH and signing settings.
	// AIProvider is the chosen provider: a built-in id ("openai", "anthropic",
	// "deepseek", "openrouter", "ollama"), a name typed for a custom endpoint, or
	// empty for an older settings file that only has endpoint and protocol.
	AIProvider string `json:"aiProvider,omitempty"`
	AIEndpoint string `json:"aiEndpoint,omitempty"`
	AIModel    string `json:"aiModel,omitempty"`
	AIKey      string `json:"aiKey,omitempty"`
	// AIProtocol names the wire format of the endpoint: "openai-completions"
	// (the default when empty, which keeps an older settings file working) or
	// "anthropic-messages".
	AIProtocol string `json:"aiProtocol,omitempty"`
	// AIConfirmGenerate asks before the AI button starts a request. It defaults
	// to true: Decode keeps the default for an older settings file that has no
	// field, while an explicit false is stored and respected. The switch lives
	// in the AI button's own context menu, next to the rule picker.
	AIConfirmGenerate bool `json:"aiConfirmGenerate"`

	ExternalEditor string `json:"externalEditor"`

	// SearchTool selects the file-content search backend: "auto" (ripgrep
	// when it is installed, Git grep otherwise), "git", or "rg". Empty keeps
	// "auto", so an older settings file behaves as before.
	SearchTool string `json:"searchTool,omitempty"`
	// RipgrepPath is the ripgrep executable. Empty searches PATH; ripgrep is
	// not bundled, so the setting exists for installs outside PATH.
	RipgrepPath string `json:"ripgrepPath,omitempty"`

	// TimeFormat picks the clock used for commit dates, notifications and the
	// command console: TimeFormat24Hour (also the empty default, so an older
	// settings file keeps rendering 24-hour times) or TimeFormat12Hour.
	TimeFormat string `json:"timeFormat,omitempty"`

	Recent []Recent `json:"recent"`

	Window Window `json:"window"`

	// Layout keeps panel proportions.
	//
	// LeftPanelWidth is the width of the left dock, shared by the Project tree
	// and the Commit panel: both pages show one tool window at a time, so two
	// widths would move the splitter whenever the user switches pages. Older
	// settings files have commitPanelWidth and projectPanelWidth instead; Load
	// folds them into this field (see adoptLeftPanelWidth) and Save writes only
	// this key.
	LeftPanelWidth     float32 `json:"leftPanelWidth"`
	GitPanelHeight     float32 `json:"gitPanelHeight"`
	TerminalHeight     float32 `json:"terminalHeight,omitempty"`
	BranchesWidth      float32 `json:"branchesWidth"`
	DetailsWidth       float32 `json:"detailsWidth"`
	DiffUnified        bool    `json:"diffUnified"`
	DiffWhitespace     int     `json:"diffWhitespace"`
	DiffSyncScroll     bool    `json:"diffSyncScroll"`
	DiffHighlightWords bool    `json:"diffHighlightWords"`
	// DiffMarks keeps the change ruler on the right edge of the diff, where it
	// also stands in for the overlay scroll bar. DiffCodemap adds the optional
	// code minimap next to it. DiffCollapse folds long unchanged stretches of a
	// diff down to a few context lines. Older files omit these fields, and Load
	// unmarshals into Defaults, so an absent value keeps its default: marks on,
	// codemap off, unchanged stretches folded.
	DiffMarks    bool `json:"diffMarks"`
	DiffCodemap  bool `json:"diffCodemap"`
	DiffCollapse bool `json:"diffCollapse"`
	CommitTree   bool `json:"commitTree"`

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
	// X and Y are the top-left corner of the content area in screen
	// coordinates. A display placed left of or above the primary one makes
	// them negative, and (0,0) is a real position, so Positioned records
	// whether a frame was ever saved. Viva keeps a restored position on a
	// screen and falls back to the main one when the saved frame matches no
	// connected display, so no monitor list is kept here.
	X          int  `json:"x,omitempty"`
	Y          int  `json:"y,omitempty"`
	Positioned bool `json:"positioned,omitempty"`
}

// Defaults returns the settings of a first start.
func Defaults() *Settings {
	return &Settings{
		Version: SchemaVersion, Theme: "dark", UIScale: 1, FontSize: 13,
		AutoFetch: false, AutoFetchMins: 10, FetchPrune: true, PullMode: "merge",
		AutoUpdateCheck:   true,
		AIConfirmGenerate: true,
		SearchTool:        "auto",
		Window:            Window{Width: 1440, Height: 900},
		LeftPanelWidth:    DefaultLeftPanelWidth, GitPanelHeight: 330, TerminalHeight: 300, BranchesWidth: 220, DetailsWidth: 360,
		DiffSyncScroll: true, DiffHighlightWords: true, DiffMarks: true, DiffCollapse: true, CommitTree: false,
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
	s.adoptLeftPanelWidth(data)
	migrate(s, probe.Version)
	s.path = path
	s.sanitize()
	return s
}

// leftPanelWidthKeys are the layout keys that decide the left dock's width in a
// settings file. leftPanelWidth is the current key; the other two are the
// per-page keys older versions wrote.
type leftPanelWidthKeys struct {
	LeftPanelWidth    *float32 `json:"leftPanelWidth"`
	CommitPanelWidth  *float32 `json:"commitPanelWidth"`
	ProjectPanelWidth *float32 `json:"projectPanelWidth"`
}

// adoptLeftPanelWidth folds the left-dock widths into the one shared width.
// The keys are read from the raw file rather than kept as Settings fields so a
// code path cannot read or write a per-page width again.
func (s *Settings) adoptLeftPanelWidth(data []byte) {
	var keys leftPanelWidthKeys
	// The file already unmarshaled as a whole; an undecodable layout key just
	// leaves its pointer nil and falls through to the next rule.
	_ = json.Unmarshal(data, &keys)
	s.LeftPanelWidth = pickLeftPanelWidth(keys)
}

// pickLeftPanelWidth chooses the shared width from the keys a settings file
// has. The rule, in order:
//
//  1. leftPanelWidth when it is valid. A file this build wrote, or one from a
//     newer build, is used exactly as written.
//  2. When only one per-page key was resized, that width. Older versions wrote
//     both keys on every save, so the key that differs from its old default is
//     the page the user adjusted; taking it keeps the width the user chose
//     instead of replacing it with the other page's default.
//  3. commitPanelWidth when both keys are usable. It was the wider page (340 by
//     default), so an ambiguous file never narrows the Commit panel.
//  4. projectPanelWidth when it is the only usable key.
//  5. the default.
//
// A missing or unusable key never replaces a usable one, so a corrupt value in
// the current key still falls back to a width the user actually chose.
func pickLeftPanelWidth(keys leftPanelWidthKeys) float32 {
	if validPanelWidth(keys.LeftPanelWidth) {
		return *keys.LeftPanelWidth
	}
	commitOK := validPanelWidth(keys.CommitPanelWidth)
	projectOK := validPanelWidth(keys.ProjectPanelWidth)
	switch {
	case commitOK && projectOK:
		commitTuned := *keys.CommitPanelWidth != DefaultLeftPanelWidth
		projectTuned := *keys.ProjectPanelWidth != legacyProjectPanelWidth
		if projectTuned && !commitTuned {
			return *keys.ProjectPanelWidth
		}
		return *keys.CommitPanelWidth
	case commitOK:
		return *keys.CommitPanelWidth
	case projectOK:
		return *keys.ProjectPanelWidth
	default:
		return DefaultLeftPanelWidth
	}
}

func validPanelWidth(p *float32) bool {
	return p != nil && *p >= MinLeftPanelWidth
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
	// Family names come from the settings page's two lists: every installed
	// family for the interface, the fixed-pitch ones for diffs. Trim them, and
	// refuse an implausible name instead of handing the renderer something it
	// cannot resolve.
	s.EditorFont = sanitizeFamily(s.EditorFont)
	s.DiffFont = sanitizeFamily(s.DiffFont)
	// An unknown or missing language keeps the default, English.
	switch s.Language = strings.TrimSpace(s.Language); s.Language {
	case "en", "zh-CN":
	default:
		s.Language = ""
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
	if s.LeftPanelWidth < MinLeftPanelWidth {
		s.LeftPanelWidth = d.LeftPanelWidth
	}
	if s.GitPanelHeight < 120 {
		s.GitPanelHeight = d.GitPanelHeight
	}
	if s.TerminalHeight < 120 {
		s.TerminalHeight = d.TerminalHeight
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
	switch s.LogLevel {
	case "", "info", "debug":
	default:
		s.LogLevel = ""
	}
	// An unknown or missing search backend keeps "auto": ripgrep when it is
	// installed, Git grep otherwise.
	s.RipgrepPath = strings.TrimSpace(s.RipgrepPath)
	switch strings.TrimSpace(s.SearchTool) {
	case "", "auto":
		s.SearchTool = "auto"
	case "git", "rg":
		s.SearchTool = strings.TrimSpace(s.SearchTool)
	default:
		s.SearchTool = "auto"
	}
	if len(s.Recent) > MaxRecentProjects {
		s.Recent = s.Recent[:MaxRecentProjects]
	}
}

// sanitizeFamily keeps a family name the renderer can use, or "" for the
// default. Real names are a few words long; anything longer cannot name one.
func sanitizeFamily(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 96 {
		return ""
	}
	return name
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
	if len(list) > MaxRecentProjects {
		list = list[:MaxRecentProjects]
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
