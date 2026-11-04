package observability

import (
	"strings"
	"time"
)

// Level is the severity of an event. The zero value is LevelError so a
// forgotten level never hides a failure.
type Level uint8

const (
	// LevelError marks something the user should act on or report.
	LevelError Level = iota
	// LevelWarn marks a recoverable problem worth keeping.
	LevelWarn
	// LevelInfo is the default: lifecycle, writes, slow operations.
	LevelInfo
	// LevelDebug is verbose and only recorded during a diagnostic session
	// or when explicitly enabled.
	LevelDebug
)

// String is the stable name written to logs.
func (l Level) String() string {
	switch l {
	case LevelError:
		return "error"
	case LevelWarn:
		return "warn"
	case LevelInfo:
		return "info"
	case LevelDebug:
		return "debug"
	}
	return "info"
}

// ParseLevel reads a configured level. Empty or unknown values mean
// LevelInfo so a typo in settings never silences logging.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return LevelError
	case "warn", "warning":
		return LevelWarn
	case "debug":
		return LevelDebug
	}
	return LevelInfo
}

// Event names. These are a public format: tools and tests match on them, so
// rename one only with a migration plan.
const (
	EventAppStart          = "app.start"
	EventAppReady          = "app.ready"
	EventAppStop           = "app.stop"
	EventAppFatal          = "app.fatal"
	EventAppRecover        = "app.recover"
	EventSettingsLoad      = "settings.load"
	EventSettingsSave      = "settings.save"
	EventRepoOpen          = "repo.open"
	EventRepoClose         = "repo.close"
	EventRepoRefresh       = "repo.refresh"
	EventRepoWatch         = "repo.watch"
	EventTaskStart         = "task.start"
	EventTaskEnd           = "task.end"
	EventTaskCancel        = "task.cancel"
	EventGitStart          = "git.start"
	EventGitEnd            = "git.end"
	EventGitFail           = "git.fail"
	EventUINotice          = "ui.notice"
	EventConflictDetected  = "conflict.detected"
	EventPerfSlow          = "perf.slow"
	EventPanic             = "panic"
	EventFault             = "fault"
	EventDiagnosticsExport = "diagnostics.export"
)

// Event is one fact the application records. Callers fill the structured
// fields; the logger owns timestamps, sequence numbers, redaction, and
// serialization, so business code never formats log lines itself.
type Event struct {
	// Event is one of the Event* constants.
	Event string
	// Level defaults to LevelInfo when zero? No: zero is LevelError. Callers
	// must choose; helpers below make the common cases short.
	Level Level
	// Repo is the absolute repository root. The logger hashes it; it never
	// reaches a log file or an exported bundle.
	Repo string
	// Op is the Git subcommand or the application action (open, refresh,
	// export).
	Op string
	// Code is the stable error code (gitcli.ErrorKind.Code) for failures.
	Code string
	// Dur is the measured duration for operations that finish.
	Dur time.Duration
	// Fields carries small, redacted facts. Keys are snake_case. Never put
	// file contents, diffs, commit messages, environment variables, full
	// command lines, absolute paths, or full remote URLs in here.
	Fields map[string]any
}

// Record is the stored, redacted, serialized form of an Event. Its JSON tags
// define the log and events.jsonl format.
type Record struct {
	Time     time.Time      `json:"ts"`
	Seq      uint64         `json:"seq"`
	Session  string         `json:"session"`
	Level    string         `json:"level"`
	Event    string         `json:"event"`
	Incident string         `json:"incident,omitempty"`
	Repo     string         `json:"repo,omitempty"`
	Op       string         `json:"op,omitempty"`
	Code     string         `json:"code,omitempty"`
	DurMS    int64          `json:"dur_ms,omitempty"`
	Msg      string         `json:"msg,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// Incident is one diagnostic recording session: the user reproduced a
// problem while the logger kept debug detail and tagged every event.
type Incident struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Description string    `json:"description,omitempty"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end,omitempty"`
	Active      bool      `json:"active"`
	Events      int       `json:"events"`
}

// Recorder is everything the application layers may do with telemetry. It is
// deliberately small so business code depends on an interface, not on the
// logging implementation.
type Recorder interface {
	// Event records one fact without blocking the caller.
	Event(Event)
	// Snapshot returns remembered records at or after since, newest last.
	// A zero since means "whatever is remembered". limit <= 0 means no limit.
	Snapshot(since time.Time, limit int) []Record
	// Incidents returns the most recent diagnostic sessions, newest last.
	Incidents() []Incident
	// IncidentEvents returns the records tagged with one incident.
	IncidentEvents(id string) []Record
	// CurrentIncident returns the active incident ID, or "".
	CurrentIncident() string
	// Dir returns the directory logs are written to, "" when disabled.
	Dir() string
	// Flush writes queued records and syncs the files.
	Flush()
	// Close flushes and releases the files. It is idempotent.
	Close()
}

// IncidentController starts and stops diagnostic recordings. It is separate
// from Recorder so the common Event/Snapshot path stays read-mostly.
type IncidentController interface {
	// StartIncident begins a recording and raises the level to debug. It
	// returns the incident ID.
	StartIncident(kind, description string) (string, error)
	// StopIncident ends a recording and restores the previous level. An
	// unknown or already stopped ID is ignored.
	StopIncident(id string)
}

// Nop is a Recorder and IncidentController that records nothing. Workspaces
// and tests use it as the default so call sites never check for nil.
type Nop struct{}

// Event implements Recorder.
func (Nop) Event(Event) {}

// Snapshot implements Recorder.
func (Nop) Snapshot(time.Time, int) []Record { return nil }

// Incidents implements Recorder.
func (Nop) Incidents() []Incident { return nil }

// IncidentEvents implements Recorder.
func (Nop) IncidentEvents(string) []Record { return nil }

// CurrentIncident implements Recorder.
func (Nop) CurrentIncident() string { return "" }

// Dir implements Recorder.
func (Nop) Dir() string { return "" }

// Flush implements Recorder.
func (Nop) Flush() {}

// Close implements Recorder.
func (Nop) Close() {}

// StartIncident implements IncidentController.
func (Nop) StartIncident(string, string) (string, error) { return "", nil }

// StopIncident implements IncidentController.
func (Nop) StopIncident(string) {}
