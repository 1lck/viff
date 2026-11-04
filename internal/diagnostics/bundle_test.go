package diagnostics

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"viff/internal/observability"
)

// testBase is the fixed UTC timestamp every test builds on.
func testBase() time.Time {
	return time.Date(2026, 11, 4, 13, 0, 0, 0, time.UTC)
}

// fakeSource is a controllable Source for bundle tests.
type fakeSource struct {
	env      Env
	settings SettingsSummary
	console  []ConsoleLine
	repos    []RepoInfo
}

func (f *fakeSource) Env() Env { return f.env }

func (f *fakeSource) SettingsSummary() SettingsSummary {
	if f.settings == nil {
		return SettingsSummary{}
	}
	return f.settings
}

func (f *fakeSource) ConsoleLines(limit int) []ConsoleLine {
	if limit <= 0 || len(f.console) <= limit {
		return f.console
	}
	return f.console[len(f.console)-limit:]
}

func (f *fakeSource) Repos() []RepoInfo { return f.repos }

// fakeRecorder is a controllable observability.Recorder for bundle tests.
type fakeRecorder struct {
	records        []observability.Record
	incidents      []observability.Incident
	incidentEvents map[string][]observability.Record
	snapshotSince  time.Time
	snapshotLimit  int
	snapshotCalls  int
	flushCalls     int
}

func (f *fakeRecorder) Event(observability.Event) {}

func (f *fakeRecorder) Snapshot(since time.Time, limit int) []observability.Record {
	f.snapshotCalls++
	f.snapshotSince = since
	f.snapshotLimit = limit
	out := make([]observability.Record, 0, len(f.records))
	for _, record := range f.records {
		if since.IsZero() || !record.Time.Before(since) {
			out = append(out, record)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func (f *fakeRecorder) Incidents() []observability.Incident { return f.incidents }

func (f *fakeRecorder) IncidentEvents(id string) []observability.Record {
	return f.incidentEvents[id]
}

func (f *fakeRecorder) CurrentIncident() string { return "" }
func (f *fakeRecorder) Dir() string             { return "" }
func (f *fakeRecorder) Flush()                  { f.flushCalls++ }
func (f *fakeRecorder) Close()                  {}

// testEnv returns a fully populated, already redacted environment snapshot.
func testEnv() Env {
	return Env{
		AppVersion:     "0.1.0-alpha.3",
		Commit:         "abc1234",
		BuildDate:      "2026-11-03",
		GoVersion:      "go1.27.3",
		VivaVersion:    "v0.2.3",
		GOOS:           "darwin",
		GOARCH:         "arm64",
		Locale:         "zh-CN",
		Theme:          "dark",
		UIScale:        1,
		EditorFont:     "SF Mono",
		GitVersion:     "git version 2.53.0",
		GitSource:      "settings",
		ConfigDir:      "<cfg>",
		SettingsSchema: 1,
		LogLevel:       "info",
	}
}

// testRecords returns a small, ordered event window.
func testRecords(base time.Time) []observability.Record {
	return []observability.Record{
		{Time: base.Add(1 * time.Minute), Seq: 1, Session: "7f3c1a2b", Level: "info", Event: "app.start"},
		{
			Time: base.Add(2 * time.Minute), Seq: 2, Session: "7f3c1a2b",
			Level: "error", Event: "git.fail", Op: "push", Code: "push.rejected",
			DurMS: 3000, Msg: "failed to push some refs",
			Data: map[string]any{"exit": 128, "kind": "rejected"},
		},
		{
			Time: base.Add(3 * time.Minute), Seq: 3, Session: "7f3c1a2b",
			Level: "info", Event: "git.end", Op: "commit", DurMS: 120,
			Data: map[string]any{"write": true},
		},
		{
			Time: base.Add(3*time.Minute + 30*time.Second), Seq: 4, Session: "7f3c1a2b",
			Level: "warn", Event: "repo.watch", Op: "watch", Msg: "watch overflow",
		},
	}
}

// writeBundle runs WriteBundle and returns the result plus the archive content.
func writeBundle(t *testing.T, dest string, src Source, rec observability.Recorder, crashDir string, opts Options) (Result, map[string][]byte) {
	t.Helper()
	result, err := WriteBundle(dest, src, rec, crashDir, opts)
	if err != nil {
		t.Fatalf("WriteBundle() error = %v", err)
	}
	_, files := readZip(t, dest)
	return result, files
}

// readZip returns the archive entry names in zip order and their contents.
func readZip(t *testing.T, path string) ([]string, map[string][]byte) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %q: %v", path, err)
	}
	defer reader.Close()
	names := make([]string, 0, len(reader.File))
	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		part, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %q: %v", file.Name, err)
		}
		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			t.Fatalf("read zip entry %q: %v", file.Name, err)
		}
		names = append(names, file.Name)
		files[file.Name] = data
	}
	return names, files
}

// requireEntries fails when a required archive entry is missing.
func requireEntries(t *testing.T, names []string, want ...string) {
	t.Helper()
	for _, name := range want {
		if !slices.Contains(names, name) {
			t.Errorf("zip is missing entry %q; entries = %v", name, names)
		}
	}
}

// parseManifest decodes manifest.json from the archive content.
func parseManifest(t *testing.T, files map[string][]byte) bundleManifest {
	t.Helper()
	raw, ok := files["manifest.json"]
	if !ok {
		t.Fatalf("manifest.json is missing; entries = %v", keysOf(files))
	}
	var manifest bundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("manifest.json is not valid JSON: %v", err)
	}
	return manifest
}

// keysOf returns map keys sorted for deterministic failure messages.
func keysOf(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// verifyManifestFiles checks every manifest entry against the zip content.
func verifyManifestFiles(t *testing.T, files map[string][]byte, manifest bundleManifest) {
	t.Helper()
	if len(manifest.Files) != len(files)-1 {
		t.Errorf("manifest lists %d files, archive has %d (excluding manifest.json)",
			len(manifest.Files), len(files)-1)
	}
	for _, entry := range manifest.Files {
		if entry.Name == "manifest.json" {
			t.Errorf("manifest.json must not list itself")
		}
		data, ok := files[entry.Name]
		if !ok {
			t.Errorf("manifest lists %q which is not in the archive", entry.Name)
			continue
		}
		if int64(len(data)) != entry.Bytes {
			t.Errorf("%s: manifest bytes = %d, actual = %d", entry.Name, entry.Bytes, len(data))
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != entry.SHA256 {
			t.Errorf("%s: manifest sha256 = %s, actual = %s", entry.Name, entry.SHA256, got)
		}
	}
}

// assertNoSensitive fails when any archive entry contains a forbidden string.
func assertNoSensitive(t *testing.T, files map[string][]byte, needles ...string) {
	t.Helper()
	for name, data := range files {
		for _, needle := range needles {
			if bytes.Contains(data, []byte(needle)) {
				t.Errorf("%s contains forbidden string %q", name, needle)
			}
		}
	}
}

// mustParseStamp parses an RFC 3339 stamp or fails the test.
func mustParseStamp(t *testing.T, label, stamp string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("%s %q is not RFC 3339: %v", label, stamp, err)
	}
	return parsed
}

// TestWriteBundleContents covers the happy path: every entry, manifest
// checksums, event window filtering and the Chinese summary.
func TestWriteBundleContents(t *testing.T) {
	base := testBase()
	dest := filepath.Join(t.TempDir(), "bundle.zip")
	src := &fakeSource{
		env: testEnv(),
		settings: SettingsSummary{
			"theme":    "dark",
			"fontSize": 13,
			"nested":   map[string]any{"k": "v"},
		},
		console: []ConsoleLine{{
			Time:    base.Add(2 * time.Minute),
			Command: "git push origin main",
			Output:  "rejected: non-fast-forward",
			Failed:  true,
			DurMS:   3000,
		}},
		repos: []RepoInfo{{
			Hash:        "9f3a1c2b",
			Name:        "viff",
			Branch:      "main",
			Operation:   "rebase",
			Dirty:       true,
			RefreshedAt: base.Add(30 * time.Minute),
		}},
	}
	records := append(testRecords(base), observability.Record{
		Time: base.Add(10 * time.Minute), Seq: 5, Session: "7f3c1a2b",
		Level: "error", Event: "git.fail", Op: "fetch", Msg: "after the window",
	})
	rec := &fakeRecorder{records: records}
	opts := Options{
		Since:          base,
		Until:          base.Add(5 * time.Minute),
		Description:    "推送一直失败",
		IncludeConsole: true,
	}

	result, files := writeBundle(t, dest, src, rec, "", opts)

	if rec.snapshotCalls != 1 {
		t.Errorf("Snapshot called %d times, want 1", rec.snapshotCalls)
	}
	if !rec.snapshotSince.Equal(opts.Since) {
		t.Errorf("Snapshot since = %v, want %v", rec.snapshotSince, opts.Since)
	}
	if rec.snapshotLimit != 0 {
		t.Errorf("Snapshot limit = %d, want 0 (no limit)", rec.snapshotLimit)
	}
	if rec.flushCalls != 1 {
		t.Errorf("Flush called %d times, want 1", rec.flushCalls)
	}
	if result.Truncated {
		t.Errorf("Result.Truncated = true, want false")
	}
	if result.Path != dest {
		t.Errorf("Result.Path = %q, want %q", result.Path, dest)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat bundle: %v", err)
	}
	if result.Bytes != info.Size() {
		t.Errorf("Result.Bytes = %d, file size = %d", result.Bytes, info.Size())
	}

	names := keysOf(files)
	requireEntries(t, names,
		"summary.md", "manifest.json", "env.txt", "settings-summary.json",
		"events.jsonl", "console.txt", "repo.txt")
	// The result reports the entries in archive order.
	ordered, _ := readZip(t, dest)
	if !slices.Equal(result.Files, ordered) {
		t.Errorf("Result.Files = %v, want %v", result.Files, ordered)
	}

	manifest := parseManifest(t, files)
	if manifest.Redaction != "standard" {
		t.Errorf("manifest redaction = %q, want standard", manifest.Redaction)
	}
	if !manifest.RedactionReviewed {
		t.Errorf("manifest redaction_reviewed = false, want true for clean input")
	}
	if manifest.Truncated {
		t.Errorf("manifest truncated = true, want false")
	}
	if manifest.AppVersion != "0.1.0-alpha.3" || manifest.Commit != "abc1234" {
		t.Errorf("manifest version fields = %q / %q", manifest.AppVersion, manifest.Commit)
	}
	if manifest.GOOS != "darwin" || manifest.GOARCH != "arm64" {
		t.Errorf("manifest platform = %s/%s", manifest.GOOS, manifest.GOARCH)
	}
	if manifest.IncidentID != "" {
		t.Errorf("manifest incident_id = %q, want empty", manifest.IncidentID)
	}
	if manifest.Description != "推送一直失败" {
		t.Errorf("manifest description = %q", manifest.Description)
	}
	if got := manifest.WindowStart; got != "2026-11-04T13:00:00Z" {
		t.Errorf("manifest window_start = %q", got)
	}
	if got := manifest.WindowEnd; got != "2026-11-04T13:05:00Z" {
		t.Errorf("manifest window_end = %q", got)
	}
	mustParseStamp(t, "generated_at", manifest.GeneratedAt)
	verifyManifestFiles(t, files, manifest)

	// Required manifest keys are present even when empty.
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(files["manifest.json"], &generic); err != nil {
		t.Fatalf("manifest.json: %v", err)
	}
	for _, key := range []string{
		"generated_at", "app_version", "commit", "build_date", "goos", "goarch",
		"incident_id", "description", "window_start", "window_end",
		"redaction", "truncated", "files",
	} {
		if _, ok := generic[key]; !ok {
			t.Errorf("manifest.json is missing required key %q", key)
		}
	}

	// events.jsonl holds the four in-window records in recording order.
	eventLines := strings.Split(strings.TrimSuffix(string(files["events.jsonl"]), "\n"), "\n")
	if len(eventLines) != 4 {
		t.Fatalf("events.jsonl has %d lines, want 4: %q", len(eventLines), eventLines)
	}
	var seqs []uint64
	for _, line := range eventLines {
		var record observability.Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("events.jsonl line is not valid JSON: %v", err)
		}
		seqs = append(seqs, record.Seq)
	}
	if !slices.Equal(seqs, []uint64{1, 2, 3, 4}) {
		t.Errorf("event sequence = %v, want [1 2 3 4]", seqs)
	}

	summary := string(files["summary.md"])
	for _, want := range []string{
		"# Viff 诊断摘要",
		"## 最近错误",
		"## 时间线（错误与写操作）",
		"## 环境与设置摘要",
		"## 脱敏说明",
		"0.1.0-alpha.3",
		"git.fail",
		"op=push",
		"code=push.rejected",
		"exit=128",
		"failed to push some refs",
		"git.end",
		"推送一直失败",
		"9f3a1c2b",
		"standard",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary.md does not contain %q", want)
		}
	}

	envText := string(files["env.txt"])
	for _, want := range []string{
		"app_version=0.1.0-alpha.3",
		"goos=darwin",
		"config_dir=<cfg>",
		"recording=false",
	} {
		if !strings.Contains(envText, want) {
			t.Errorf("env.txt does not contain %q", want)
		}
	}

	var settings map[string]any
	if err := json.Unmarshal(files["settings-summary.json"], &settings); err != nil {
		t.Fatalf("settings-summary.json: %v", err)
	}
	if settings["theme"] != "dark" {
		t.Errorf("settings theme = %v, want dark", settings["theme"])
	}
	if size, ok := settings["fontSize"].(float64); !ok || size != 13 {
		t.Errorf("settings fontSize = %v, want 13", settings["fontSize"])
	}

	repoText := string(files["repo.txt"])
	for _, want := range []string{"hash=9f3a1c2b", "name=viff", "branch=main", "dirty=true"} {
		if !strings.Contains(repoText, want) {
			t.Errorf("repo.txt does not contain %q", want)
		}
	}
	console := string(files["console.txt"])
	for _, want := range []string{"git push origin main", "FAILED", "non-fast-forward"} {
		if !strings.Contains(console, want) {
			t.Errorf("console.txt does not contain %q", want)
		}
	}

	assertNoSensitive(t, files, "/Users/", "/home/", "ghp_")
}

// TestWriteBundleIncidentFilter checks that an incident export uses only the
// incident's records and its window.
func TestWriteBundleIncidentFilter(t *testing.T) {
	base := testBase()
	dest := filepath.Join(t.TempDir(), "incident.zip")
	incidentEvents := []observability.Record{
		{
			Time: base.Add(1 * time.Minute), Seq: 10, Level: "error",
			Event: "git.fail", Op: "push", Code: "push.rejected",
			Incident: "inc-8f21", Msg: "incident failure",
		},
		{
			Time: base.Add(2 * time.Minute), Seq: 11, Level: "debug",
			Event: "git.start", Op: "push", Incident: "inc-8f21",
		},
	}
	rec := &fakeRecorder{
		records: []observability.Record{
			{Time: base.Add(3 * time.Minute), Seq: 12, Level: "error", Event: "git.fail", Msg: "unrelated"},
		},
		incidents: []observability.Incident{{
			ID: "inc-8f21", Kind: "problem", Description: "推送失败",
			Start: base.Add(30 * time.Second), End: base.Add(2*time.Minute + 30*time.Second),
		}},
		incidentEvents: map[string][]observability.Record{"inc-8f21": incidentEvents},
	}
	src := &fakeSource{env: testEnv()}
	opts := Options{IncidentID: "inc-8f21", IncludeConsole: true}

	result, files := writeBundle(t, dest, src, rec, "", opts)

	if rec.snapshotCalls != 0 {
		t.Errorf("Snapshot called %d times for an incident export, want 0", rec.snapshotCalls)
	}
	if result.Truncated {
		t.Errorf("Result.Truncated = true, want false")
	}
	manifest := parseManifest(t, files)
	if manifest.IncidentID != "inc-8f21" {
		t.Errorf("manifest incident_id = %q", manifest.IncidentID)
	}
	// The empty Options description falls back to the incident description.
	if manifest.Description != "推送失败" {
		t.Errorf("manifest description = %q, want the incident description", manifest.Description)
	}
	if manifest.WindowStart != "2026-11-04T13:00:30Z" || manifest.WindowEnd != "2026-11-04T13:02:30Z" {
		t.Errorf("manifest window = %s – %s, want the incident window",
			manifest.WindowStart, manifest.WindowEnd)
	}
	lines := strings.Split(strings.TrimSuffix(string(files["events.jsonl"]), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("events.jsonl has %d lines, want 2: %q", len(lines), lines)
	}
	if !strings.Contains(string(files["events.jsonl"]), `"incident":"inc-8f21"`) {
		t.Errorf("events.jsonl does not keep the incident tag: %s", files["events.jsonl"])
	}
	if strings.Contains(string(files["events.jsonl"]), "unrelated") {
		t.Errorf("events.jsonl contains an event outside the incident")
	}
	if !strings.Contains(string(files["summary.md"]), "inc-8f21") {
		t.Errorf("summary.md does not mention the incident id")
	}
}

// TestWriteBundleTruncatesEvents covers both the 5000-record and the 2 MiB
// limits: the newest records are kept and the manifest marks the truncation.
func TestWriteBundleTruncatesEvents(t *testing.T) {
	base := testBase()
	t.Run("by record count", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "count.zip")
		records := make([]observability.Record, 5100)
		for i := range records {
			records[i] = observability.Record{
				Time:  base.Add(time.Duration(i) * time.Second),
				Seq:   uint64(i + 1),
				Level: "info",
				Event: "repo.refresh",
				Op:    "refresh",
			}
		}
		rec := &fakeRecorder{records: records}
		result, files := writeBundle(t, dest, &fakeSource{env: testEnv()}, rec, "",
			Options{IncludeConsole: true})

		if !result.Truncated {
			t.Errorf("Result.Truncated = false, want true")
		}
		manifest := parseManifest(t, files)
		if !manifest.Truncated {
			t.Errorf("manifest truncated = false, want true")
		}
		if _, ok := manifest.Truncations["events.jsonl"]; !ok {
			t.Errorf("manifest truncations = %v, want an events.jsonl note", manifest.Truncations)
		}
		lines := strings.Split(strings.TrimSuffix(string(files["events.jsonl"]), "\n"), "\n")
		if len(lines) != maxEventsRecords {
			t.Fatalf("events.jsonl has %d lines, want %d", len(lines), maxEventsRecords)
		}
		var first, last observability.Record
		if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
			t.Fatalf("first event: %v", err)
		}
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
			t.Fatalf("last event: %v", err)
		}
		if first.Seq != 101 || last.Seq != 5100 {
			t.Errorf("kept seq range = %d..%d, want 101..5100", first.Seq, last.Seq)
		}
		if info, err := os.Stat(dest); err != nil || info.Size() > maxBundleBytes {
			t.Errorf("bundle size = %v (err %v), want <= %d", info, err, maxBundleBytes)
		}
	})

	t.Run("by byte budget", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "bytes.zip")
		payload := strings.Repeat("x", 9000)
		records := make([]observability.Record, 300)
		for i := range records {
			records[i] = observability.Record{
				Time:  base.Add(time.Duration(i) * time.Second),
				Seq:   uint64(i + 1),
				Level: "info",
				Event: "repo.refresh",
				Op:    "refresh",
				Data:  map[string]any{"stderr": payload},
			}
		}
		rec := &fakeRecorder{records: records}
		result, files := writeBundle(t, dest, &fakeSource{env: testEnv()}, rec, "",
			Options{IncludeConsole: true})

		if !result.Truncated {
			t.Errorf("Result.Truncated = false, want true")
		}
		data := files["events.jsonl"]
		if len(data) > maxEventsBytes {
			t.Errorf("events.jsonl = %d bytes, want <= %d", len(data), maxEventsBytes)
		}
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		if len(lines) >= len(records) {
			t.Errorf("events.jsonl was not truncated: %d lines", len(lines))
		}
		var last observability.Record
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
			t.Fatalf("last event: %v", err)
		}
		if last.Seq != 300 {
			t.Errorf("last kept seq = %d, want 300", last.Seq)
		}
		manifest := parseManifest(t, files)
		if !manifest.Truncated {
			t.Errorf("manifest truncated = false, want true")
		}
	})
}

// TestWriteBundleCapsConsole checks the 1 MiB console budget.
func TestWriteBundleCapsConsole(t *testing.T) {
	base := testBase()
	out := strings.Repeat("y", 5000)
	lines := make([]ConsoleLine, 300)
	for i := range lines {
		lines[i] = ConsoleLine{
			Time:    base.Add(time.Duration(i) * time.Second),
			Command: "git fetch origin",
			Output:  out,
		}
	}
	rec := &fakeRecorder{}
	dest := filepath.Join(t.TempDir(), "console.zip")
	result, files := writeBundle(t, dest, &fakeSource{env: testEnv(), console: lines}, rec, "",
		Options{IncludeConsole: true})

	if !result.Truncated {
		t.Errorf("Result.Truncated = false, want true")
	}
	data := files["console.txt"]
	if len(data) > maxConsoleBytes {
		t.Errorf("console.txt = %d bytes, want <= %d", len(data), maxConsoleBytes)
	}
	manifest := parseManifest(t, files)
	if !manifest.Truncated {
		t.Errorf("manifest truncated = false, want true")
	}
	if _, ok := manifest.Truncations["console.txt"]; !ok {
		t.Errorf("manifest truncations = %v, want a console.txt note", manifest.Truncations)
	}
	if info, err := os.Stat(dest); err != nil || info.Size() > maxBundleBytes {
		t.Errorf("bundle size = %v (err %v), want <= %d", info, err, maxBundleBytes)
	}
}

// writeCrashFile writes one crash fixture with an explicit modification time.
func writeCrashFile(t *testing.T, dir, name, content string, mod time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write crash file: %v", err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("chtimes crash file: %v", err)
	}
}

// TestWriteBundleCrashFiles checks that only the newest three crash files are
// copied, that they are scrubbed, and that the 1 MiB total cap is enforced.
func TestWriteBundleCrashFiles(t *testing.T) {
	base := testBase()
	t.Run("newest three", func(t *testing.T) {
		crashDir := t.TempDir()
		for i := 0; i < 5; i++ {
			name := "last-crash-" + string(rune('a'+i)) + ".txt"
			writeCrashFile(t, crashDir, name, "crash "+name, base.Add(time.Duration(i)*time.Minute))
		}
		dest := filepath.Join(t.TempDir(), "crash.zip")
		_, files := writeBundle(t, dest, &fakeSource{env: testEnv()}, &fakeRecorder{}, crashDir,
			Options{IncludeConsole: true})

		manifest := parseManifest(t, files)
		if !manifest.Truncated {
			t.Errorf("manifest truncated = false, want true when older crash files are omitted")
		}
		if _, ok := manifest.Truncations["crash/"]; !ok {
			t.Errorf("manifest truncations = %v, want a crash/ note", manifest.Truncations)
		}
		var crashNames []string
		for name := range files {
			if strings.HasPrefix(name, "crash/") {
				crashNames = append(crashNames, name)
			}
		}
		slices.Sort(crashNames)
		want := []string{"crash/last-crash-c.txt", "crash/last-crash-d.txt", "crash/last-crash-e.txt"}
		if !slices.Equal(crashNames, want) {
			t.Errorf("crash entries = %v, want %v", crashNames, want)
		}
	})

	t.Run("scrubbed stack", func(t *testing.T) {
		crashDir := t.TempDir()
		content := "panic: boom\n" +
			"goroutine 1 [running]:\n" +
			"viff/internal/app.(*Workspace).Refresh(0x1)\n" +
			"\t/Users/lick/code/viff/internal/app/repo.go:42 +0x1f\n" +
			"main.run()\n" +
			"\t/Users/lick/code/viff/cmd/viff/main.go:10 +0x2f\n"
		writeCrashFile(t, crashDir, "last-crash-1.txt", content, base)
		dest := filepath.Join(t.TempDir(), "crash-scrub.zip")
		_, files := writeBundle(t, dest, &fakeSource{env: testEnv()}, &fakeRecorder{}, crashDir,
			Options{IncludeConsole: true})

		crash, ok := files["crash/last-crash-1.txt"]
		if !ok {
			t.Fatalf("crash entry missing; entries = %v", keysOf(files))
		}
		if !strings.Contains(string(crash), "panic: boom") {
			t.Errorf("crash entry lost the panic message: %q", crash)
		}
		if !strings.Contains(string(crash), "Workspace") {
			t.Errorf("crash entry lost the stack function: %q", crash)
		}
		if strings.Contains(string(crash), "/Users/") {
			t.Errorf("crash entry leaks an absolute path: %q", crash)
		}
		manifest := parseManifest(t, files)
		if manifest.RedactionReviewed {
			t.Errorf("manifest redaction_reviewed = true, want false after skipping paths")
		}
	})

	t.Run("size cap", func(t *testing.T) {
		crashDir := t.TempDir()
		big := strings.Repeat("z", 1500*1024)
		writeCrashFile(t, crashDir, "last-crash-big.txt", big, base.Add(2*time.Minute))
		writeCrashFile(t, crashDir, "last-crash-small.txt", "small", base.Add(time.Minute))
		dest := filepath.Join(t.TempDir(), "crash-size.zip")
		result, files := writeBundle(t, dest, &fakeSource{env: testEnv()}, &fakeRecorder{}, crashDir,
			Options{IncludeConsole: true})

		if !result.Truncated {
			t.Errorf("Result.Truncated = false, want true")
		}
		var total int
		for name, data := range files {
			if strings.HasPrefix(name, "crash/") {
				total += len(data)
			}
		}
		if total > maxCrashBytes {
			t.Errorf("crash/ total = %d bytes, want <= %d", total, maxCrashBytes)
		}
		manifest := parseManifest(t, files)
		if !manifest.Truncated {
			t.Errorf("manifest truncated = false, want true")
		}
		if len(manifest.Truncations) == 0 {
			t.Errorf("manifest truncations is empty, want crash notes")
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "no-crash.zip")
		result, files := writeBundle(t, dest, &fakeSource{env: testEnv()}, &fakeRecorder{},
			filepath.Join(t.TempDir(), "does-not-exist"), Options{IncludeConsole: true})
		if result.Truncated {
			t.Errorf("Result.Truncated = true, want false")
		}
		for name := range files {
			if strings.HasPrefix(name, "crash/") {
				t.Errorf("unexpected crash entry %q for a missing crash directory", name)
			}
		}
	})
}

// TestWriteBundleEmptyData makes sure an empty recording still produces a
// usable bundle with explicit "无" markers instead of blank sections.
func TestWriteBundleEmptyData(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "empty.zip")
	result, files := writeBundle(t, dest, &fakeSource{}, &fakeRecorder{}, "",
		Options{IncludeConsole: true})

	if result.Truncated {
		t.Errorf("Result.Truncated = true, want false")
	}
	names := keysOf(files)
	requireEntries(t, names,
		"summary.md", "manifest.json", "env.txt", "settings-summary.json",
		"events.jsonl", "console.txt", "repo.txt")
	if len(files["events.jsonl"]) != 0 {
		t.Errorf("events.jsonl = %q, want empty", files["events.jsonl"])
	}
	manifest := parseManifest(t, files)
	if manifest.Truncated {
		t.Errorf("manifest truncated = true, want false")
	}
	verifyManifestFiles(t, files, manifest)
	summary := string(files["summary.md"])
	for _, want := range []string{
		"# Viff 诊断摘要",
		"## 最近错误\n\n（无）",
		"## 时间线（错误与写操作）\n\n（无）",
		"### 设置摘要\n\n（无）",
		"## 脱敏说明",
		"无",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary.md does not contain %q:\n%s", want, summary)
		}
	}
	if !strings.Contains(string(files["repo.txt"]), "无已知仓库") {
		t.Errorf("repo.txt = %q, want the no-repository marker", files["repo.txt"])
	}
}

// TestWriteBundleRejectsBadDestination covers the error path.
func TestWriteBundleRejectsBadDestination(t *testing.T) {
	src := &fakeSource{env: testEnv()}
	rec := &fakeRecorder{}
	missingDir := filepath.Join(t.TempDir(), "missing", "bundle.zip")
	if _, err := WriteBundle(missingDir, src, rec, "", Options{}); err == nil {
		t.Errorf("WriteBundle() with a missing parent directory returned no error")
	}
	existingDir := t.TempDir()
	if _, err := WriteBundle(existingDir, src, rec, "", Options{}); err == nil {
		t.Errorf("WriteBundle() with a directory destination returned no error")
	}
	if _, err := WriteBundle("   ", src, rec, "", Options{}); err == nil {
		t.Errorf("WriteBundle() with an empty destination returned no error")
	}
}

// TestWriteBundleSkipsSensitiveContent checks the last-line-of-defence scan:
// tokens and absolute paths never reach the archive.
func TestWriteBundleSkipsSensitiveContent(t *testing.T) {
	base := testBase()
	dest := filepath.Join(t.TempDir(), "sensitive.zip")
	src := &fakeSource{
		env: func() Env {
			env := testEnv()
			env.ConfigDir = "/Users/lick/.config/viff"
			return env
		}(),
		settings: SettingsSummary{
			"theme":      "dark",
			"cache_dir":  "/Users/lick/cache",
			"projectUrl": "https://user:ghp_abcdefghijklmnopqrstuvwxyz123456@github.com/viff/viff.git",
		},
		console: []ConsoleLine{
			{
				Time:    base,
				Command: "git remote add origin https://user:ghp_abcdefghijklmnopqrstuvwxyz123456@github.com/viff/viff.git",
			},
			{Time: base.Add(time.Minute), Command: "git status --short"},
		},
	}
	records := []observability.Record{
		{
			Time: base.Add(time.Minute), Seq: 1, Level: "error", Event: "git.fail",
			Op: "fetch", Msg: "fatal: password=hunter2 rejected",
		},
		{
			Time: base.Add(2 * time.Minute), Seq: 2, Level: "error", Event: "git.fail",
			Op: "push", Code: "push.rejected", Msg: "failed to push some refs",
		},
	}
	opts := Options{
		IncludeConsole: true,
		Description:    "复现步骤：见 https://user:ghp_abcdefghijklmnopqrstuvwxyz123456@github.com/x",
	}
	result, files := writeBundle(t, dest, src, &fakeRecorder{records: records}, "", opts)

	if result.Truncated {
		t.Errorf("Result.Truncated = true, want false (redaction skips are not size truncation)")
	}
	assertNoSensitive(t, files,
		"/Users/", "ghp_", "hunter2", "abcdefghijklmnopqrstuvwxyz123456")
	manifest := parseManifest(t, files)
	if manifest.RedactionReviewed {
		t.Errorf("manifest redaction_reviewed = true, want false")
	}
	if !strings.Contains(string(files["summary.md"]), "疑似敏感内容") {
		t.Errorf("summary.md does not warn about skipped sensitive content")
	}
	if !strings.Contains(string(files["console.txt"]), "git status --short") {
		t.Errorf("console.txt lost the clean console line")
	}
	if strings.Contains(string(files["console.txt"]), "git remote add") {
		t.Errorf("console.txt kept the token-bearing command")
	}
	if !strings.Contains(string(files["env.txt"]), "config_dir=<redacted>") {
		t.Errorf("env.txt did not redact the absolute config path: %q", files["env.txt"])
	}
}

// TestWriteBundleWithoutConsole checks the IncludeConsole option.
func TestWriteBundleWithoutConsole(t *testing.T) {
	base := testBase()
	dest := filepath.Join(t.TempDir(), "no-console.zip")
	src := &fakeSource{
		env:     testEnv(),
		console: []ConsoleLine{{Time: base, Command: "git status"}},
	}
	result, files := writeBundle(t, dest, src, &fakeRecorder{}, "",
		Options{IncludeConsole: false})
	if slices.Contains(result.Files, "console.txt") {
		t.Errorf("Result.Files = %v, want no console.txt", result.Files)
	}
	if _, ok := files["console.txt"]; ok {
		t.Errorf("console.txt is present even though IncludeConsole is false")
	}
	manifest := parseManifest(t, files)
	verifyManifestFiles(t, files, manifest)
}

// TestWriteBundleNilRecorderAndSource covers the nil guards.
func TestWriteBundleNilRecorderAndSource(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "nil.zip")
	if _, err := WriteBundle(dest, nil, nil, "", Options{}); err == nil {
		t.Errorf("WriteBundle() with a nil Source returned no error")
	}
	result, err := WriteBundle(dest, &fakeSource{env: testEnv()}, nil, "", Options{IncludeConsole: true})
	if err != nil {
		t.Fatalf("WriteBundle() with a nil Recorder: %v", err)
	}
	if result.Truncated {
		t.Errorf("Result.Truncated = true, want false")
	}
}
