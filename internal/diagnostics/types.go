// Package diagnostics assembles the diagnostic bundle a user saves and shares
// when something goes wrong. It depends only on internal/observability and the
// standard library: the application supplies its own facts through Source, so
// this package stays testable with fakes and never imports UI or Git code.
package diagnostics

import "time"

// Env is the environment snapshot included in every bundle. Paths must be
// placeholders such as "<cfg>"; this package does not redact them again.
type Env struct {
	AppVersion     string  `json:"app_version"`
	Commit         string  `json:"commit,omitempty"`
	BuildDate      string  `json:"build_date,omitempty"`
	GoVersion      string  `json:"go_version"`
	VivaVersion    string  `json:"viva_version"`
	GOOS           string  `json:"goos"`
	GOARCH         string  `json:"goarch"`
	Locale         string  `json:"locale,omitempty"`
	Theme          string  `json:"theme,omitempty"`
	UIScale        float64 `json:"ui_scale,omitempty"`
	EditorFont     string  `json:"editor_font,omitempty"`
	GitVersion     string  `json:"git_version,omitempty"`
	GitSource      string  `json:"git_source,omitempty"` // "settings", "path" or "missing"
	ConfigDir      string  `json:"config_dir,omitempty"` // placeholder, e.g. "<cfg>"
	SettingsSchema int     `json:"settings_schema,omitempty"`
	LogLevel       string  `json:"log_level,omitempty"`
	Recording      bool    `json:"recording"`
}

// ConsoleLine is one Git command from the Console tab, already redacted by the
// application layer.
type ConsoleLine struct {
	Time    time.Time `json:"time"`
	Command string    `json:"command"`
	Output  string    `json:"output,omitempty"`
	Failed  bool      `json:"failed,omitempty"`
	DurMS   int64     `json:"dur_ms,omitempty"`
}

// RepoInfo is one repository known to the workspace. Name is empty unless the
// user opted into sharing it; Hash is always the redacted identity.
type RepoInfo struct {
	Hash        string    `json:"hash"`
	Name        string    `json:"name,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	Operation   string    `json:"operation,omitempty"`
	Dirty       bool      `json:"dirty,omitempty"`
	RefreshedAt time.Time `json:"refreshed_at,omitempty"`
}

// SettingsSummary is the non-secret part of the settings file: field values
// that help reproduce a problem, never keys, tokens or paths.
type SettingsSummary map[string]any

// Source is what the application contributes to a bundle.
type Source interface {
	Env() Env
	SettingsSummary() SettingsSummary
	ConsoleLines(limit int) []ConsoleLine
	Repos() []RepoInfo
}

// Options selects what a bundle contains.
type Options struct {
	// Since and Until bound the event window. A zero Since means "whatever
	// is remembered".
	Since time.Time
	Until time.Time
	// IncidentID, when set, limits events to that recording.
	IncidentID string
	// Description is the user's own account of the problem.
	Description string
	// IncludeConsole adds console.txt. Default true at the call site.
	IncludeConsole bool
}

// Result describes the saved bundle.
type Result struct {
	Path      string   `json:"path"`
	Bytes     int64    `json:"bytes"`
	Files     []string `json:"files"`
	Truncated bool     `json:"truncated"`
}
