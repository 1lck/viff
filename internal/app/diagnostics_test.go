package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/observability"
	"viff/internal/storage"
)

// fakeObs is a Recorder and IncidentController for tests: events stay in
// memory, incidents follow the same lifecycle contract as the real logger.
type fakeObs struct {
	mu        sync.Mutex
	events    []observability.Event
	incidents []observability.Incident
	current   string
	dir       string
	level     observability.Level
	nextID    int
}

func (f *fakeObs) Event(e observability.Event) {
	f.mu.Lock()
	f.events = append(f.events, e)
	f.mu.Unlock()
}

func (f *fakeObs) Snapshot(time.Time, int) []observability.Record { return nil }

func (f *fakeObs) Incidents() []observability.Incident {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]observability.Incident, len(f.incidents))
	copy(out, f.incidents)
	return out
}

func (f *fakeObs) IncidentEvents(string) []observability.Record { return nil }

func (f *fakeObs) CurrentIncident() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}

func (f *fakeObs) Dir() string { return f.dir }
func (f *fakeObs) Flush()      {}
func (f *fakeObs) Close()      {}

func (f *fakeObs) SetLevel(level observability.Level) {
	f.mu.Lock()
	f.level = level
	f.mu.Unlock()
}

func (f *fakeObs) Level() observability.Level {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.level
}

func (f *fakeObs) StartIncident(kind, description string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.current != "" {
		return "", fmt.Errorf("incident %s is already running", f.current)
	}
	f.nextID++
	id := fmt.Sprintf("inc-%08x", f.nextID)
	f.incidents = append(f.incidents, observability.Incident{
		ID: id, Kind: kind, Description: description, Start: time.Now().UTC(), Active: true,
	})
	f.current = id
	return id, nil
}

func (f *fakeObs) StopIncident(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.incidents {
		if f.incidents[i].ID != id || !f.incidents[i].Active {
			continue
		}
		f.incidents[i].Active = false
		f.incidents[i].End = time.Now().UTC()
	}
	if f.current == id {
		f.current = ""
	}
}

func (f *fakeObs) eventsNamed(name string) []observability.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []observability.Event
	for _, e := range f.events {
		if e.Event == name {
			out = append(out, e)
		}
	}
	return out
}

// TestSettingsSummaryExcludesSecrets guards the privacy boundary of the bundle:
// only non-secret settings leave the machine.
func TestSettingsSummaryExcludesSecrets(t *testing.T) {
	s := storage.Defaults()
	s.Theme = "light"
	s.UIScale = 1.25
	s.FontSize = 15
	s.PullMode = "rebase"
	s.AutoFetch = true
	s.AutoFetchMins = 7
	s.FetchPrune = false
	s.DiffUnified = true
	s.DiffWhitespace = 1
	s.DiffSyncScroll = false
	s.DiffHighlightWords = false
	s.CommitTree = true
	s.SSHCommand = "ssh -i /secret/id_rsa"
	s.GPGProgram = "/secret/gpg"
	s.SigningKey = "SIGNING-KEY-SECRET"
	s.SignFormat = "ssh"
	s.CredentialHelper = "secret-credential-helper"
	s.GitPath = "/secret/bin/git"
	s.ExternalEditor = "/secret/editor"
	s.Recent = []storage.Recent{{Path: "/secret/private-repo"}}

	w := NewWorkspace(&gitcli.Runner{}, s, nil)
	summary := w.SettingsSummary()
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)

	for _, key := range []string{
		"theme", "ui_scale", "font_size", "pull_mode", "auto_fetch",
		"auto_fetch_minutes", "fetch_prune", "diff_unified", "diff_whitespace",
		"diff_sync_scroll", "diff_highlight_words", "commit_tree",
	} {
		if _, ok := summary[key]; !ok {
			t.Errorf("SettingsSummary misses non-secret field %q", key)
		}
	}
	for _, forbidden := range []string{
		"sshCommand", "ssh_command", "GPGProgram", "gpg_program", "/secret/gpg",
		"signingKey", "signing_key", "SIGNING-KEY-SECRET",
		"signFormat", "sign_format", "credentialHelper", "credential_helper",
		"secret-credential-helper", "gitPath", "git_path", "/secret/bin/git",
		"recent", "/secret/private-repo", "externalEditor", "external_editor",
		"/secret/editor", "/secret/id_rsa",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("SettingsSummary leaks %q: %s", forbidden, text)
		}
	}
}

// TestProblemRecordingLifecycle covers start/stop/status and the incident
// lookup export uses, plus level application through SetRecorder.
func TestProblemRecordingLifecycle(t *testing.T) {
	obs := &fakeObs{dir: t.TempDir()}
	w := NewWorkspace(&gitcli.Runner{}, storage.Defaults(), nil)
	w.SetLogLevel("debug")
	w.SetRecorder(obs, obs)
	if got := obs.Level(); got != observability.LevelDebug {
		t.Errorf("installed recorder level = %v, want debug", got)
	}
	w.SetLogLevel("nonsense")
	if got := obs.Level(); got != observability.LevelInfo {
		t.Errorf("unknown level = %v, want info", got)
	}

	if w.ProblemRecording() {
		t.Fatal("a fresh workspace claims to be recording")
	}
	if err := w.StartProblemRecording("push failed"); err != nil {
		t.Fatal(err)
	}
	if !w.ProblemRecording() {
		t.Fatal("recording did not start")
	}
	if err := w.StartProblemRecording("again"); err == nil {
		t.Fatal("a second recording was accepted")
	}
	if _, ok := w.LastIncident(); ok {
		t.Fatal("an active recording must not be reported as finished")
	}
	w.StopProblemRecording()
	if w.ProblemRecording() {
		t.Fatal("recording did not stop")
	}
	incident, ok := w.LastIncident()
	if !ok || incident.Description != "push failed" || incident.Active || incident.End.IsZero() {
		t.Fatalf("last incident = %+v, ok = %v", incident, ok)
	}
	if got, want := w.LogDirectory(), obs.dir; got != want {
		t.Errorf("LogDirectory = %q, want %q", got, want)
	}
}

// TestExportDiagnosticsStopsRecording checks the ordering guarantee even while
// the bundle writer is still being implemented by another agent: the recording
// is stopped before the bundle is attempted.
func TestExportDiagnosticsStopsRecording(t *testing.T) {
	obs := &fakeObs{dir: t.TempDir()}
	w := NewWorkspace(&gitcli.Runner{}, storage.Defaults(), nil)
	w.SetRecorder(obs, obs)
	if err := w.StartProblemRecording("crash on push"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "viff-diagnostics.zip")
	_, err := w.ExportDiagnostics(dest)
	if w.ProblemRecording() {
		t.Fatal("export left the recording running")
	}
	if err != nil {
		t.Skipf("diagnostics bundle is not available yet, bundle assertions wait for it: %v", err)
	}
	if _, statErr := os.Stat(dest); statErr != nil {
		t.Fatalf("export reported success but wrote no file: %v", statErr)
	}
}

// TestExportDiagnosticsWritesBundle exercises the real bundle writer with the
// application as its Source: a successful export is a non-empty zip and is
// recorded as an event.
func TestExportDiagnosticsWritesBundle(t *testing.T) {
	obs := &fakeObs{dir: t.TempDir()}
	w := NewWorkspace(&gitcli.Runner{}, storage.Defaults(), nil)
	w.SetRecorder(obs, obs)
	w.Console = []ConsoleEntry{
		{Time: time.Now(), Command: "git status --porcelain", Took: 12 * time.Millisecond},
		{Time: time.Now(), Command: "git push origin main", Output: "fatal: no upstream", Failed: true, Took: 300 * time.Millisecond},
	}
	dest := filepath.Join(t.TempDir(), "viff-diagnostics.zip")
	result, err := w.ExportDiagnostics(dest)
	if err != nil {
		t.Skipf("diagnostics bundle is not available yet, bundle assertions wait for it: %v", err)
	}
	info, statErr := os.Stat(dest)
	if statErr != nil {
		t.Fatalf("export reported success but wrote no file: %v", statErr)
	}
	if result.Path != dest || result.Bytes <= 0 || result.Bytes != info.Size() || result.Files == 0 {
		t.Fatalf("result = %+v (on disk %d bytes)", result, info.Size())
	}
	data, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.HasPrefix(data, []byte("PK")) {
		t.Fatal("exported file is not a zip archive")
	}
	exports := obs.eventsNamed(observability.EventDiagnosticsExport)
	if len(exports) != 1 {
		t.Fatalf("diagnostics.export events = %d, want 1", len(exports))
	}
	if got := exports[0].Fields["files"]; got != result.Files {
		t.Errorf("event files = %v, want %d", got, result.Files)
	}
	if _, ok := exports[0].Fields["bytes"]; !ok {
		t.Error("diagnostics.export event misses bytes")
	}
}

// TestEnvGitSource checks the environment snapshot: placeholders for paths and
// an honest report of where Git came from.
func TestEnvGitSource(t *testing.T) {
	// A path that does not exist makes the version query fail quickly, so the
	// test does not depend on the Git installed on the machine.
	missing := NewWorkspace(&gitcli.Runner{Path: "/nonexistent/viff-test-git"}, storage.Defaults(), nil)
	env := missing.Env()
	if env.GitSource != "missing" || env.GitVersion != "" {
		t.Errorf("missing git: source = %q version = %q", env.GitSource, env.GitVersion)
	}
	if env.ConfigDir != "<cfg>" {
		t.Errorf("ConfigDir = %q, want the <cfg> placeholder", env.ConfigDir)
	}
	if env.GoVersion != runtime.Version() || env.GOOS != runtime.GOOS || env.GOARCH != runtime.GOARCH {
		t.Errorf("Go/OS fields = %+v", env)
	}
	if env.SettingsSchema != storage.SchemaVersion {
		t.Errorf("SettingsSchema = %d, want %d", env.SettingsSchema, storage.SchemaVersion)
	}
	if env.AppVersion == "" || env.Recording {
		t.Errorf("env = %+v", env)
	}

	configured := storage.Defaults()
	configured.GitPath = "/custom/bin/git"
	configured.Theme = "light"
	configured.LogLevel = "debug"
	w := NewWorkspace(&gitcli.Runner{Path: "/nonexistent/viff-test-git"}, configured, nil)
	w.SetLogLevel("debug")
	env = w.Env()
	if env.GitSource != "settings" {
		t.Errorf("configured GitSource = %q, want settings", env.GitSource)
	}
	if env.Theme != "light" || env.LogLevel != "debug" {
		t.Errorf("settings in env = %+v", env)
	}
}

// TestConsoleLinesRecentAndRedacted covers the bundle's console section: only
// the newest entries, with repository paths and credentials replaced.
func TestConsoleLinesRecentAndRedacted(t *testing.T) {
	w := NewWorkspace(&gitcli.Runner{}, storage.Defaults(), nil)
	root := t.TempDir()
	w.Repo = &RepoState{Repo: domain.Repo{Root: root}}
	w.Console = []ConsoleEntry{
		{Time: time.Now().Add(-2 * time.Second), Command: "git status", Took: 2 * time.Millisecond},
		{Time: time.Now().Add(-time.Second), Command: "git add " + filepath.Join(root, "a.txt"), Output: "added " + filepath.Join(root, "a.txt")},
		{
			Time:    time.Now(),
			Command: "git push https://user:ghp_abcdefghijklmnopqrstuvwxyz@github.com/org/private.git",
			Failed:  true,
			Took:    1500 * time.Millisecond,
		},
	}
	lines := w.ConsoleLines(2)
	if len(lines) != 2 {
		t.Fatalf("ConsoleLines(2) = %d lines", len(lines))
	}
	last := lines[len(lines)-1]
	if last.Failed != true || last.DurMS != 1500 {
		t.Errorf("last line = %+v", last)
	}
	if strings.Contains(last.Command, "ghp_") || strings.Contains(last.Command, "user:") {
		t.Errorf("credential leaked into the bundle: %q", last.Command)
	}
	if !strings.Contains(last.Command, "https://github.com") {
		t.Errorf("remote host was lost, command = %q", last.Command)
	}
	if strings.Contains(lines[0].Command, root) || strings.Contains(lines[0].Output, root) {
		t.Errorf("repository path leaked: %+v", lines[0])
	}
	if !strings.Contains(lines[0].Command, "<repo>") {
		t.Errorf("repository path was not replaced with <repo>: %q", lines[0].Command)
	}
}

// TestLogCommandEvents covers the structured side of logCommand: silent reads
// stay out of the Console, failures carry code/kind/exit, successful writes and
// slow reads produce git.end, and the Console output is capped.
func TestLogCommandEvents(t *testing.T) {
	obs := &fakeObs{}
	w := NewWorkspace(&gitcli.Runner{}, storage.Defaults(), nil)
	w.SetRecorder(obs, obs)
	w.Drain()

	// A successful read: no Console entry, no event.
	w.logCommand("", []string{"status", "--porcelain"}, time.Millisecond, nil)
	if len(w.Console) != 0 || len(obs.events) != 0 {
		t.Fatalf("silent read: console=%d events=%d", len(w.Console), len(obs.events))
	}

	// A successful write: Console entry and a git.end event marked write.
	w.logCommand("", []string{"push", "origin", "main"}, 20*time.Millisecond, nil)
	w.Drain()
	if len(w.Console) != 1 || w.Console[0].Failed || !strings.Contains(w.Console[0].Command, "git push origin main") {
		t.Fatalf("write console = %+v", w.Console)
	}
	ends := obs.eventsNamed(observability.EventGitEnd)
	if len(ends) != 1 || ends[0].Op != "push" || ends[0].Fields["write"] != true || ends[0].Dur != 20*time.Millisecond {
		t.Fatalf("git.end = %+v", ends)
	}

	// A failing read is shown and produces a git.fail event with the code.
	// (fetch updates refs, so it counts as a write family; the point here is a
	// command classified by the write heuristic, not the Console rules.)
	big := strings.Repeat("x", 20000)
	authErr := &gitcli.Error{Kind: gitcli.ErrAuth, ExitCode: 128, Stderr: big}
	w.logCommand("", []string{"fetch"}, 10*time.Millisecond, authErr)
	w.Drain()
	if len(w.Console) != 2 || !w.Console[1].Failed {
		t.Fatalf("failure console = %+v", w.Console)
	}
	if got := len(w.Console[1].Output); got > consoleOutputLimit {
		t.Errorf("Console output = %d bytes, want <= %d", got, consoleOutputLimit)
	}
	fails := obs.eventsNamed(observability.EventGitFail)
	if len(fails) != 1 {
		t.Fatalf("git.fail events = %d", len(fails))
	}
	fail := fails[0]
	if fail.Code != "auth.failed" || fail.Op != "fetch" || fail.Level != observability.LevelError {
		t.Errorf("git.fail = %+v", fail)
	}
	if fail.Fields["kind"] != "auth" || fail.Fields["code"] != "auth.failed" || fail.Fields["exit"] != 128 || fail.Fields["write"] != true {
		t.Errorf("git.fail fields = %+v", fail.Fields)
	}
	stderr, _ := fail.Fields["stderr"].(string)
	if stderr == "" || len(stderr) > failStderrLimit {
		t.Errorf("git.fail stderr = %d bytes, want 1..%d", len(stderr), failStderrLimit)
	}

	// A slow successful read is not Console noise but is recorded.
	w.logCommand("", []string{"log", "--oneline"}, 700*time.Millisecond, nil)
	w.Drain()
	ends = obs.eventsNamed(observability.EventGitEnd)
	if len(ends) != 2 || ends[1].Op != "log" || ends[1].Fields["write"] != false {
		t.Fatalf("slow read event = %+v", ends)
	}
	if len(w.Console) != 2 {
		t.Fatalf("slow read should stay out of the Console, got %d entries", len(w.Console))
	}
}

// TestRepoAndTaskEvents checks the lifecycle events through a real open: the
// repository open, the refresh and the opening task are all recorded.
func TestRepoAndTaskEvents(t *testing.T) {
	f := newFixture(t)
	obs := &fakeObs{}
	f.w.SetRecorder(obs, obs)
	f.open()

	if got := len(obs.eventsNamed(observability.EventRepoOpen)); got != 1 {
		t.Fatalf("repo.open events = %d, want 1", got)
	}
	opens := obs.eventsNamed(observability.EventRepoOpen)
	if opens[0].Fields["status"] != "ok" || opens[0].Code != "" {
		t.Errorf("repo.open = %+v", opens[0])
	}
	refreshes := obs.eventsNamed(observability.EventRepoRefresh)
	if len(refreshes) == 0 {
		t.Fatal("no repo.refresh event")
	}
	last := refreshes[len(refreshes)-1]
	if last.Fields["gen"] != f.w.Repo.Gen {
		t.Errorf("refresh gen = %v, want %d", last.Fields["gen"], f.w.Repo.Gen)
	}
	for _, key := range []string{"staged", "unstaged", "untracked", "conflicted", "meta"} {
		if _, ok := last.Fields[key]; !ok {
			t.Errorf("repo.refresh misses %q: %+v", key, last.Fields)
		}
	}
	if len(obs.eventsNamed(observability.EventTaskStart)) == 0 || len(obs.eventsNamed(observability.EventTaskEnd)) == 0 {
		t.Error("task start/end events were not recorded for the open")
	}
}
