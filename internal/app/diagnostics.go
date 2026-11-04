package app

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"time"

	"viff/internal/diagnostics"
	"viff/internal/observability"
	"viff/internal/storage"
	"viff/internal/version"
)

const (
	// diagnosticWindow is how far back a bundle reaches when there is no
	// diagnostic recording to scope it: half an hour of ordinary events.
	diagnosticWindow = 30 * time.Minute
	// maxDiagnosticRepos bounds the repository list in a bundle.
	maxDiagnosticRepos = 10
)

// DiagnosticResult describes a saved diagnostic bundle.
type DiagnosticResult struct {
	// Path is where the zip was written.
	Path string
	// Bytes is its size on disk.
	Bytes int64
	// Files is the number of entries inside the archive.
	Files int
	// Truncated reports that some section had to be cut to fit the size
	// budget; the bundle is still valid and says so in its manifest.
	Truncated bool
}

// SetRecorder installs the event recorder and the incident controller. A nil
// argument means "record nothing", so the workspace never has to check for
// nil. The configured log level is applied to the new recorder.
func (w *Workspace) SetRecorder(obs observability.Recorder, inc observability.IncidentController) {
	if obs == nil {
		obs = observability.Nop{}
	}
	if inc == nil {
		inc = observability.Nop{}
	}
	w.obs = obs
	w.inc = inc
	w.applyLogLevel()
}

// SetLogLevel applies the configured file log level: "", "info" or "debug".
// Anything else means the default level, so a typo in the settings file never
// silences logging. During a diagnostic recording the recorder itself forces
// debug detail; this call restores the configured level when it ends.
func (w *Workspace) SetLogLevel(level string) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		w.logLevel = "debug"
	default:
		w.logLevel = "info"
	}
	w.applyLogLevel()
}

// levelSetter is the optional capability a recorder may have; the Recorder
// interface is deliberately read/write-only, so the level stays a concrete
// logger concern.
type levelSetter interface {
	SetLevel(observability.Level)
}

// applyLogLevel pushes the configured level into the recorder when it supports
// it. A Nop recorder ignores it.
func (w *Workspace) applyLogLevel() {
	if setter, ok := w.recorder().(levelSetter); ok {
		setter.SetLevel(observability.ParseLevel(w.logLevel))
	}
}

// logLevelName is the configured level as written into a bundle's environment.
func (w *Workspace) logLevelName() string {
	if w.logLevel == "debug" {
		return "debug"
	}
	return "info"
}

// recorder returns the installed recorder, Nop when logging is off or the
// workspace was built as a literal without NewWorkspace.
func (w *Workspace) recorder() observability.Recorder {
	if w.obs == nil {
		return observability.Nop{}
	}
	return w.obs
}

// incidentController returns the installed controller, Nop by default.
func (w *Workspace) incidentController() observability.IncidentController {
	if w.inc == nil {
		return observability.Nop{}
	}
	return w.inc
}

// record sends one event to the recorder, if any.
func (w *Workspace) record(e observability.Event) {
	w.recorder().Event(e)
}

// repoRoot is the root of the active repository, "" on the welcome screen. It
// is the value events carry in Event.Repo; the logger hashes it there.
func (w *Workspace) repoRoot() string {
	if w.Repo == nil {
		return ""
	}
	return w.Repo.Repo.Root
}

// LogDirectory is where Viff writes its log files, "" when file logging is
// disabled. The Help menu reveals it, and a bundle's crash directory is its
// sibling.
func (w *Workspace) LogDirectory() string {
	return w.recorder().Dir()
}

// StartProblemRecording begins a diagnostic recording: the recorder raises the
// level to debug and tags every event with the incident ID until
// StopProblemRecording or ExportDiagnostics ends it.
func (w *Workspace) StartProblemRecording(description string) error {
	if w.ProblemRecording() {
		return errors.New("a problem recording is already running")
	}
	_, err := w.incidentController().StartIncident("problem", description)
	return err
}

// StopProblemRecording ends the current diagnostic recording. It is a no-op
// when nothing is being recorded.
func (w *Workspace) StopProblemRecording() {
	if id := w.recorder().CurrentIncident(); id != "" {
		w.incidentController().StopIncident(id)
	}
}

// ProblemRecording reports whether a diagnostic recording is running.
func (w *Workspace) ProblemRecording() bool {
	return w.recorder().CurrentIncident() != ""
}

// LastIncident returns the most recently finished diagnostic recording. Export
// uses it to scope the bundle to the incidents the user just reproduced.
func (w *Workspace) LastIncident() (observability.Incident, bool) {
	incidents := w.recorder().Incidents()
	for i := len(incidents) - 1; i >= 0; i-- {
		if incidents[i].ID == "" || incidents[i].Active {
			continue
		}
		return incidents[i], true
	}
	return observability.Incident{}, false
}

// ExportDiagnostics writes a diagnostic bundle to dest and returns what was
// saved. When a recording is running it is stopped first and the bundle is
// scoped to that recording; without one, the bundle covers the last 30
// minutes. Logs are flushed by the bundle writer before it reads them.
func (w *Workspace) ExportDiagnostics(dest string) (DiagnosticResult, error) {
	if strings.TrimSpace(dest) == "" {
		return DiagnosticResult{}, errors.New("no diagnostics destination chosen")
	}
	start := time.Now()
	recording := w.ProblemRecording()
	if recording {
		w.StopProblemRecording()
	}
	now := w.now()
	since := now.Add(-diagnosticWindow)
	until := now
	incidentID, description := "", ""
	// Only a recording that was active for this export scopes the bundle;
	// "Save Diagnostics" must not silently reuse an old recording's window.
	if recording {
		if incident, ok := w.LastIncident(); ok {
			incidentID = incident.ID
			description = incident.Description
			if !incident.Start.IsZero() {
				since = incident.Start
			}
			if !incident.End.IsZero() {
				until = incident.End
			}
		}
	}
	rec := w.recorder()
	opts := diagnostics.Options{
		Since:          since,
		Until:          until,
		IncidentID:     incidentID,
		Description:    description,
		IncludeConsole: true,
	}
	res, err := diagnostics.WriteBundle(dest, w, rec, observability.CrashDir(rec.Dir()), opts)
	if err != nil {
		return DiagnosticResult{}, err
	}
	result := DiagnosticResult{
		Path:      res.Path,
		Bytes:     res.Bytes,
		Files:     len(res.Files),
		Truncated: res.Truncated,
	}
	fields := map[string]any{
		"files":     len(res.Files),
		"bytes":     res.Bytes,
		"truncated": res.Truncated,
		"profile":   "standard",
	}
	if incidentID != "" {
		fields["incident"] = incidentID
	}
	w.record(observability.Event{
		Event:  observability.EventDiagnosticsExport,
		Level:  observability.LevelInfo,
		Repo:   w.repoRoot(),
		Op:     "export",
		Dur:    time.Since(start),
		Fields: fields,
	})
	return result, nil
}

// Env implements diagnostics.Source: the environment facts a maintainer needs
// to reproduce a report. Paths are placeholders ("<cfg>"); the Git version is
// queried with a short timeout so a hung binary cannot stall an export.
func (w *Workspace) Env() diagnostics.Env {
	env := diagnostics.Env{
		AppVersion:     version.Version,
		Commit:         version.Commit,
		BuildDate:      version.BuildDate,
		GoVersion:      runtime.Version(),
		VivaVersion:    version.VivaVersion(),
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Locale:         locale(),
		ConfigDir:      "<cfg>",
		SettingsSchema: storage.SchemaVersion,
		LogLevel:       w.logLevelName(),
		Recording:      w.ProblemRecording(),
	}
	if w.Settings != nil {
		env.Theme = w.Settings.Theme
		env.UIScale = float64(w.Settings.UIScale)
		env.EditorFont = w.Settings.EditorFont
	}
	gitVersion := ""
	if w.Git != nil {
		// Version runs the Git binary: never pass a nil context, and never let
		// a broken installation hold the export open.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if v, err := w.Git.Version(ctx); err == nil {
			gitVersion = v
		}
		cancel()
	}
	env.GitVersion = gitVersion
	env.GitSource = "missing"
	switch {
	case w.Settings != nil && w.Settings.GitPath != "":
		env.GitSource = "settings"
	case gitVersion != "":
		env.GitSource = "path"
	}
	return env
}

// locale reports the user's locale from the environment, "" when unset.
func locale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// SettingsSummary implements diagnostics.Source: the non-secret settings that
// help reproduce a problem. Credentials, signing keys, the Git path, recent
// repository paths and the external editor are deliberately absent.
func (w *Workspace) SettingsSummary() diagnostics.SettingsSummary {
	summary := diagnostics.SettingsSummary{}
	if w.Settings == nil {
		return summary
	}
	s := w.Settings
	summary["theme"] = s.Theme
	summary["ui_scale"] = s.UIScale
	summary["font_size"] = s.FontSize
	summary["pull_mode"] = s.PullMode
	summary["auto_fetch"] = s.AutoFetch
	summary["auto_fetch_minutes"] = s.AutoFetchMins
	summary["fetch_prune"] = s.FetchPrune
	summary["diff_unified"] = s.DiffUnified
	summary["diff_whitespace"] = s.DiffWhitespace
	summary["diff_sync_scroll"] = s.DiffSyncScroll
	summary["diff_highlight_words"] = s.DiffHighlightWords
	summary["commit_tree"] = s.CommitTree
	return summary
}

// ConsoleLines implements diagnostics.Source: the most recent Console entries.
// The text is redacted here because the bundle writer only scans for leftovers;
// the application is responsible for handing it clean lines.
func (w *Workspace) ConsoleLines(limit int) []diagnostics.ConsoleLine {
	entries := w.Console
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	root := w.repoRoot()
	out := make([]diagnostics.ConsoleLine, 0, len(entries))
	for _, entry := range entries {
		out = append(out, diagnostics.ConsoleLine{
			Time:    entry.Time,
			Command: redactForBundle(entry.Command, root),
			Output:  redactForBundle(entry.Output, root),
			Failed:  entry.Failed,
			DurMS:   entry.Took.Milliseconds(),
		})
	}
	return out
}

// redactForBundle applies the standard redaction profile to one string, with
// the active repository root becoming "<repo>".
func redactForBundle(s, root string) string {
	if s == "" {
		return s
	}
	if root != "" {
		return observability.RedactPath(root, s)
	}
	return observability.Redact(s, observability.DefaultRedaction())
}

// Repos implements diagnostics.Source: the active repository first, then the
// other repositories known to the workspace. Only the redacted hash identifies
// a repository; Name stays empty unless the user opts into sharing it.
func (w *Workspace) Repos() []diagnostics.RepoInfo {
	out := make([]diagnostics.RepoInfo, 0, 1+len(w.Repositories))
	seen := make(map[string]bool, 1+len(w.Repositories))
	if rs := w.Repo; rs != nil {
		hash := observability.RepoHash(rs.Repo.Root)
		if hash != "" {
			seen[hash] = true
		}
		out = append(out, diagnostics.RepoInfo{
			Hash:        hash,
			Branch:      rs.Status.Branch.Branch,
			Operation:   operationStateName(rs.Operation),
			Dirty:       rs.StatusLoaded && !rs.Status.Clean(),
			RefreshedAt: rs.StatusTime,
		})
	}
	for _, repo := range w.Repositories {
		hash := observability.RepoHash(repo.Root)
		if hash == "" || seen[hash] {
			continue
		}
		seen[hash] = true
		out = append(out, diagnostics.RepoInfo{
			Hash:   hash,
			Branch: repo.Status.Branch.Branch,
			Dirty:  repo.Loaded && !repo.Status.Clean(),
		})
		if len(out) >= maxDiagnosticRepos {
			break
		}
	}
	return out
}
