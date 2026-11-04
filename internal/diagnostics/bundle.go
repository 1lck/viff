package diagnostics

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"viff/internal/observability"
)

const (
	// maxBundleBytes is the hard zip size budget from docs/observability.md §8.3.
	maxBundleBytes = 5 << 20
	// maxEventsRecords bounds events.jsonl by count.
	maxEventsRecords = 5000
	// maxEventsBytes bounds events.jsonl by size.
	maxEventsBytes = 2 << 20
	// maxConsoleBytes bounds console.txt.
	maxConsoleBytes = 1 << 20
	// maxCrashBytes bounds the crash/ directory inside the bundle.
	maxCrashBytes = 1 << 20
	// maxCrashFiles is how many crash files are copied.
	maxCrashFiles = 3
	// maxConsoleLines is the hint passed to Source.ConsoleLines.
	maxConsoleLines = 400
	// redactionStandard is the only redaction level v1 exports.
	redactionStandard = "standard"
)

// bundleEntry is one uncompressed file inside the zip.
type bundleEntry struct {
	name string
	data []byte
}

// manifestFile describes one file inside the bundle.
type manifestFile struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// bundleManifest is the machine-readable index written to manifest.json.
type bundleManifest struct {
	GeneratedAt       string            `json:"generated_at"`
	AppVersion        string            `json:"app_version"`
	Commit            string            `json:"commit"`
	BuildDate         string            `json:"build_date"`
	GOOS              string            `json:"goos"`
	GOARCH            string            `json:"goarch"`
	GoVersion         string            `json:"go_version,omitempty"`
	VivaVersion       string            `json:"viva_version,omitempty"`
	IncidentID        string            `json:"incident_id"`
	Description       string            `json:"description"`
	WindowStart       string            `json:"window_start"`
	WindowEnd         string            `json:"window_end"`
	Redaction         string            `json:"redaction"`
	RedactionReviewed bool              `json:"redaction_reviewed"`
	Truncated         bool              `json:"truncated"`
	Files             []manifestFile    `json:"files"`
	Truncations       map[string]string `json:"truncations,omitempty"`
}

// bundleState carries the sanitized bundle content while it is assembled and
// possibly trimmed to fit the size budget.
type bundleState struct {
	env               Env
	settings          SettingsSummary
	opts              Options
	generatedAt       time.Time
	windowStart       time.Time
	windowEnd         time.Time
	redactionReviewed bool
	truncated         bool
	truncations       map[string]string

	summary       []byte
	envText       []byte
	settingsJSON  []byte
	eventsChunks  [][]byte
	eventsJSONL   []byte
	consoleChunks [][]byte
	consoleText   []byte
	repoText      []byte
	crash         []bundleEntry
}

// WriteBundle creates dest as a diagnostic zip archive. It never mutates the
// recorder or the source: events are copied, sanitized, truncated to the size
// budget, and rendered in memory before the archive is written.
//
// The application is responsible for the redaction options; this package only
// enforces the "standard" level as a last line of defence, dropping text lines
// that still look like an absolute home path or a credential.
func WriteBundle(dest string, src Source, rec observability.Recorder, crashDir string, opts Options) (Result, error) {
	if strings.TrimSpace(dest) == "" {
		return Result{}, errors.New("diagnostics: empty destination path")
	}
	if src == nil {
		return Result{}, errors.New("diagnostics: nil source")
	}
	if rec == nil {
		rec = observability.Nop{}
	}
	generatedAt := time.Now().UTC()
	// Flush first so events still queued in the async writer are visible to
	// Snapshot; export is user initiated, so blocking briefly is fine.
	rec.Flush()

	incidents := rec.Incidents()
	incident := findIncident(incidents, opts.IncidentID)
	var records []observability.Record
	if opts.IncidentID != "" {
		records = rec.IncidentEvents(opts.IncidentID)
	} else {
		records = filterUntil(rec.Snapshot(opts.Since, 0), opts.Until)
	}
	windowStart, windowEnd := resolveWindow(opts, incident, records, generatedAt)

	san := &sanitizer{}
	env := san.env(src.Env())
	settings := san.settings(src.SettingsSummary())
	repos := san.repos(src.Repos())
	records = san.records(records)
	description := san.description(opts.Description)
	if description == "" && incident != nil {
		description = san.description(incident.Description)
	}
	opts.Description = description

	var consoleLines []ConsoleLine
	if opts.IncludeConsole {
		consoleLines = san.consoleLines(src.ConsoleLines(maxConsoleLines))
	}
	consoleChunks, consoleData, consoleTrunc, consoleTruncated := encodeConsole(consoleLines)
	eventsChunks, eventsData, eventsTrunc, eventsTruncated := encodeEvents(records)
	crashEntries, crashTrunc, crashTruncated := readCrashEntries(crashDir, san)

	state := &bundleState{
		env:           env,
		settings:      settings,
		opts:          opts,
		generatedAt:   generatedAt,
		windowStart:   windowStart,
		windowEnd:     windowEnd,
		truncated:     eventsTruncated || consoleTruncated || crashTruncated,
		truncations:   map[string]string{},
		eventsChunks:  eventsChunks,
		eventsJSONL:   eventsData,
		consoleChunks: consoleChunks,
		consoleText:   consoleData,
		crash:         crashEntries,
	}
	for name, note := range eventsTrunc {
		state.noteTruncation(name, note)
	}
	for name, note := range consoleTrunc {
		state.noteTruncation(name, note)
	}
	for name, note := range crashTrunc {
		state.noteTruncation(name, note)
	}
	state.envText = san.scrubText(envText(env))
	state.repoText = san.scrubText(repoText(repos))
	settingsRaw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("diagnostics: marshal settings summary: %w", err)
	}
	state.settingsJSON = san.scrubJSON(append(settingsRaw, '\n'))

	summaryIn := summaryInput{
		env:               env,
		settings:          settings,
		repos:             repos,
		events:            records,
		opts:              opts,
		incident:          incident,
		generatedAt:       generatedAt,
		windowStart:       windowStart,
		windowEnd:         windowEnd,
		redactionReviewed: !san.flagged,
	}
	state.summary = san.scrubText(buildSummary(summaryIn))
	if summaryIn.redactionReviewed && san.flagged {
		// The summary itself contained something suspicious: rebuild it with
		// the warning line so the note agrees with the manifest flag.
		summaryIn.redactionReviewed = false
		state.summary = san.scrubText(buildSummary(summaryIn))
	}
	state.redactionReviewed = !san.flagged

	entries := state.dataEntries()
	var zipEntries []bundleEntry
	for attempt := 0; ; attempt++ {
		manifestJSON, err := state.manifestJSON(entries)
		if err != nil {
			return Result{}, fmt.Errorf("diagnostics: marshal manifest: %w", err)
		}
		zipEntries = insertManifest(entries, manifestJSON)
		size, err := zipSize(zipEntries, state.generatedAt)
		if err != nil {
			return Result{}, fmt.Errorf("diagnostics: build archive: %w", err)
		}
		if size <= maxBundleBytes {
			break
		}
		if attempt >= 64 {
			return Result{}, fmt.Errorf("diagnostics: bundle exceeds %d bytes", maxBundleBytes)
		}
		trimmed := state.trimConsole() || state.trimCrash() || state.trimEvents() || state.trimSettings()
		if !trimmed {
			return Result{}, fmt.Errorf("diagnostics: bundle exceeds %d bytes", maxBundleBytes)
		}
		entries = state.dataEntries()
	}

	size, err := writeZipFile(dest, zipEntries, state.generatedAt)
	if err != nil {
		return Result{}, err
	}
	files := make([]string, 0, len(zipEntries))
	for _, entry := range zipEntries {
		files = append(files, entry.name)
	}
	return Result{Path: dest, Bytes: size, Files: files, Truncated: state.truncated}, nil
}

// findIncident returns the incident with the given id, or nil.
func findIncident(incidents []observability.Incident, id string) *observability.Incident {
	if id == "" {
		return nil
	}
	for i := range incidents {
		if incidents[i].ID == id {
			return &incidents[i]
		}
	}
	return nil
}

// filterUntil drops records newer than until. A zero until keeps everything.
func filterUntil(records []observability.Record, until time.Time) []observability.Record {
	if until.IsZero() || len(records) == 0 {
		return records
	}
	out := make([]observability.Record, 0, len(records))
	for _, r := range records {
		if !r.Time.After(until) {
			out = append(out, r)
		}
	}
	return out
}

// resolveWindow picks the exported window: explicit options first, then the
// incident bounds, then the first/last exported record, then "now".
func resolveWindow(opts Options, incident *observability.Incident, records []observability.Record, now time.Time) (time.Time, time.Time) {
	start, end := opts.Since, opts.Until
	if incident != nil {
		if start.IsZero() {
			start = incident.Start
		}
		if end.IsZero() && !incident.End.IsZero() {
			end = incident.End
		}
	}
	if start.IsZero() && len(records) > 0 {
		start = records[0].Time
	}
	if end.IsZero() && len(records) > 0 {
		end = records[len(records)-1].Time
	}
	if start.IsZero() {
		start = now
	}
	if end.IsZero() {
		end = now
	}
	return start.UTC(), end.UTC()
}

// encodeEvents serializes records as JSON lines. Truncation keeps the newest
// records: first by count (5000), then by bytes (2 MiB).
func encodeEvents(records []observability.Record) ([][]byte, []byte, map[string]string, bool) {
	truncations := map[string]string{}
	truncated := false
	if len(records) > maxEventsRecords {
		dropped := len(records) - maxEventsRecords
		records = records[dropped:]
		truncations["events.jsonl"] = fmt.Sprintf(
			"超过 %d 条上限，保留最新 %d 条（丢弃最旧 %d 条）",
			maxEventsRecords, maxEventsRecords, dropped)
		truncated = true
	}
	chunks := make([][]byte, 0, len(records))
	failed := 0
	for _, record := range records {
		raw, err := json.Marshal(record)
		if err != nil {
			failed++
			continue
		}
		chunks = append(chunks, append(raw, '\n'))
	}
	if failed > 0 {
		truncations["events.jsonl"] = appendNote(truncations["events.jsonl"],
			fmt.Sprintf("%d 条事件无法序列化，已跳过", failed))
		truncated = true
	}
	kept, data, dropped := capChunks(chunks, maxEventsBytes)
	if dropped > 0 {
		truncations["events.jsonl"] = appendNote(truncations["events.jsonl"],
			fmt.Sprintf("超过 %d MiB 上限，保留最新 %d 条（丢弃最旧 %d 条）",
				maxEventsBytes/(1<<20), len(kept), dropped))
		truncated = true
	}
	if len(data) > maxEventsBytes {
		// Only reachable when a single record exceeds the byte budget.
		data = truncateUTF8(data[:maxEventsBytes])
		truncations["events.jsonl"] = appendNote(truncations["events.jsonl"],
			"单条事件超过 2 MiB，文件尾部已截断")
		truncated = true
	}
	return kept, data, truncations, truncated
}

// consoleChunk renders one console command with its output.
func consoleChunk(line ConsoleLine) []byte {
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(consoleTime(line.Time))
	b.WriteString("] ")
	if line.Failed {
		b.WriteString("FAILED")
	} else {
		b.WriteString("OK")
	}
	if dur := fmtDurationMS(line.DurMS); dur != "" {
		b.WriteString(" (")
		b.WriteString(dur)
		b.WriteString(")")
	}
	if line.Command != "" {
		b.WriteString(" ")
		b.WriteString(line.Command)
	}
	b.WriteString("\n")
	if line.Output != "" {
		for _, outLine := range strings.Split(strings.TrimRight(line.Output, "\r\n"), "\n") {
			b.WriteString("    ")
			b.WriteString(strings.TrimRight(outLine, "\r"))
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}

// encodeConsole renders console.txt, keeping the newest entries when the
// 1 MiB budget is exceeded.
func encodeConsole(lines []ConsoleLine) ([][]byte, []byte, map[string]string, bool) {
	truncations := map[string]string{}
	truncated := false
	chunks := make([][]byte, 0, len(lines))
	for _, line := range lines {
		chunks = append(chunks, consoleChunk(line))
	}
	kept, data, dropped := capChunks(chunks, maxConsoleBytes)
	if dropped > 0 {
		truncations["console.txt"] = fmt.Sprintf(
			"超过 %d MiB 上限，保留最新 %d 条（丢弃最旧 %d 条）",
			maxConsoleBytes/(1<<20), len(kept), dropped)
		truncated = true
	}
	return kept, data, truncations, truncated
}

// consoleTime renders a console line timestamp in UTC.
func consoleTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format(time.RFC3339)
}

// capChunks keeps the tail of chunks whose total size fits maxBytes.
func capChunks(chunks [][]byte, maxBytes int) ([][]byte, []byte, int) {
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	dropped := 0
	for total > maxBytes && dropped < len(chunks) {
		total -= len(chunks[dropped])
		dropped++
	}
	kept := chunks[dropped:]
	var buf bytes.Buffer
	buf.Grow(total)
	for _, chunk := range kept {
		buf.Write(chunk)
	}
	return kept, buf.Bytes(), dropped
}

// joinChunks concatenates chunks into one byte slice.
func joinChunks(chunks [][]byte) []byte {
	var buf bytes.Buffer
	for _, chunk := range chunks {
		buf.Write(chunk)
	}
	return buf.Bytes()
}

// readCrashEntries copies the newest crash files (at most 3, at most 1 MiB in
// total). A missing or unreadable crash directory is not an error: the bundle
// simply has no crash/ entries.
func readCrashEntries(crashDir string, san *sanitizer) ([]bundleEntry, map[string]string, bool) {
	truncations := map[string]string{}
	truncated := false
	if crashDir == "" {
		return nil, truncations, false
	}
	dirEntries, err := os.ReadDir(crashDir)
	if err != nil {
		return nil, truncations, false
	}
	type candidate struct {
		name string
		mod  time.Time
	}
	files := make([]candidate, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}
		info, err := dirEntry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, candidate{name: dirEntry.Name(), mod: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].mod.Equal(files[j].mod) {
			return files[i].name > files[j].name
		}
		return files[i].mod.After(files[j].mod)
	})
	if len(files) > maxCrashFiles {
		files = files[:maxCrashFiles]
		truncations["crash/"] = fmt.Sprintf("只包含最新 %d 份崩溃文件", maxCrashFiles)
		truncated = true
	}
	entries := make([]bundleEntry, 0, len(files))
	var used int64
	for _, file := range files {
		remaining := int64(maxCrashBytes) - used
		if remaining <= 0 {
			truncations["crash/"] = appendNote(truncations["crash/"],
				fmt.Sprintf("崩溃文件总量超过 %d MiB，其余文件已省略", maxCrashBytes/(1<<20)))
			truncated = true
			break
		}
		data, cut, err := readLimitedFile(filepath.Join(crashDir, file.name), remaining)
		if err != nil {
			continue
		}
		data = san.scrubText(data)
		name := "crash/" + filepath.Base(file.name)
		if cut {
			truncations[name] = fmt.Sprintf("超过 %d MiB 总量上限，文件尾部已截断", maxCrashBytes/(1<<20))
			truncated = true
		}
		used += int64(len(data))
		entries = append(entries, bundleEntry{name: name, data: data})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return entries, truncations, truncated
}

// readLimitedFile reads at most remaining bytes from path. It reports whether
// the file had to be cut.
func readLimitedFile(path string, remaining int64) ([]byte, bool, error) {
	if remaining <= 0 {
		return nil, true, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, remaining+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > remaining {
		return truncateUTF8(data[:int(remaining)]), true, nil
	}
	return data, false, nil
}

// truncateUTF8 removes an incomplete trailing UTF-8 sequence.
func truncateUTF8(data []byte) []byte {
	for i := 0; i < 3 && len(data) > 0 && !utf8.Valid(data); i++ {
		data = data[:len(data)-1]
	}
	return data
}

// envSchema renders the settings schema version for env.txt, empty when
// unknown.
func envSchema(schema int) string {
	if schema <= 0 {
		return ""
	}
	return strconv.Itoa(schema)
}

// envText renders env.txt as key=value lines.
func envText(env Env) []byte {
	scale := ""
	if env.UIScale > 0 {
		scale = fmt.Sprintf("%.2f", env.UIScale)
	}
	fields := []struct {
		key   string
		value string
	}{
		{"app_version", env.AppVersion},
		{"commit", env.Commit},
		{"build_date", env.BuildDate},
		{"go_version", env.GoVersion},
		{"viva_version", env.VivaVersion},
		{"goos", env.GOOS},
		{"goarch", env.GOARCH},
		{"locale", env.Locale},
		{"theme", env.Theme},
		{"ui_scale", scale},
		{"editor_font", env.EditorFont},
		{"git_version", env.GitVersion},
		{"git_source", env.GitSource},
		{"config_dir", env.ConfigDir},
		{"settings_schema", envSchema(env.SettingsSchema)},
		{"log_level", env.LogLevel},
		{"recording", strconv.FormatBool(env.Recording)},
	}
	var b strings.Builder
	b.WriteString("# Viff environment snapshot\n")
	for _, field := range fields {
		value := strings.NewReplacer("\r", " ", "\n", " ").Replace(field.value)
		b.WriteString(field.key)
		b.WriteString("=")
		b.WriteString(value)
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// repoText renders repo.txt: redacted repository identities only.
func repoText(repos []RepoInfo) []byte {
	var b strings.Builder
	b.WriteString("# Viff repositories (hash is the redacted identity)\n")
	if len(repos) == 0 {
		b.WriteString("# 无已知仓库\n")
		return []byte(b.String())
	}
	for _, repo := range repos {
		refreshed := ""
		if !repo.RefreshedAt.IsZero() {
			refreshed = formatStamp(repo.RefreshedAt)
		}
		fmt.Fprintf(&b, "repo hash=%s name=%s branch=%s operation=%s dirty=%t refreshed_at=%s\n",
			repo.Hash, repo.Name, repo.Branch, repo.Operation, repo.Dirty, refreshed)
	}
	return []byte(b.String())
}

// dataEntries returns the files that are not the manifest, in zip order.
func (s *bundleState) dataEntries() []bundleEntry {
	entries := make([]bundleEntry, 0, 6+len(s.crash))
	entries = append(entries,
		bundleEntry{name: "summary.md", data: s.summary},
		bundleEntry{name: "env.txt", data: s.envText},
		bundleEntry{name: "settings-summary.json", data: s.settingsJSON},
		bundleEntry{name: "events.jsonl", data: s.eventsJSONL},
	)
	if s.opts.IncludeConsole {
		entries = append(entries, bundleEntry{name: "console.txt", data: s.consoleText})
	}
	entries = append(entries, bundleEntry{name: "repo.txt", data: s.repoText})
	entries = append(entries, s.crash...)
	return entries
}

// manifestJSON renders manifest.json for the given data entries.
func (s *bundleState) manifestJSON(entries []bundleEntry) ([]byte, error) {
	manifest := bundleManifest{
		GeneratedAt:       formatStamp(s.generatedAt),
		AppVersion:        s.env.AppVersion,
		Commit:            s.env.Commit,
		BuildDate:         s.env.BuildDate,
		GOOS:              s.env.GOOS,
		GOARCH:            s.env.GOARCH,
		GoVersion:         s.env.GoVersion,
		VivaVersion:       s.env.VivaVersion,
		IncidentID:        s.opts.IncidentID,
		Description:       s.opts.Description,
		WindowStart:       formatStamp(s.windowStart),
		WindowEnd:         formatStamp(s.windowEnd),
		Redaction:         redactionStandard,
		RedactionReviewed: s.redactionReviewed,
		Truncated:         s.truncated,
		Files:             make([]manifestFile, 0, len(entries)),
	}
	if len(s.truncations) > 0 {
		manifest.Truncations = s.truncations
	}
	for _, entry := range entries {
		digest := sha256.Sum256(entry.data)
		manifest.Files = append(manifest.Files, manifestFile{
			Name:   entry.name,
			Bytes:  int64(len(entry.data)),
			SHA256: hex.EncodeToString(digest[:]),
		})
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// noteTruncation records why and where content was dropped.
func (s *bundleState) noteTruncation(name, note string) {
	if note == "" {
		return
	}
	s.truncated = true
	if s.truncations == nil {
		s.truncations = map[string]string{}
	}
	s.truncations[name] = appendNote(s.truncations[name], note)
}

// trimConsole drops the oldest half of console.txt. It reports whether it
// changed anything.
func (s *bundleState) trimConsole() bool {
	if len(s.consoleChunks) == 0 {
		return false
	}
	keep := len(s.consoleChunks) / 2
	dropped := len(s.consoleChunks) - keep
	s.consoleChunks = s.consoleChunks[keep:]
	s.consoleText = joinChunks(s.consoleChunks)
	s.noteTruncation("console.txt",
		fmt.Sprintf("诊断包超过 %d MiB，丢弃最旧 %d 条 Console 记录", maxBundleBytes/(1<<20), dropped))
	return true
}

// trimCrash drops the oldest crash entry. It reports whether it changed
// anything.
func (s *bundleState) trimCrash() bool {
	if len(s.crash) == 0 {
		return false
	}
	removed := s.crash[0].name
	s.crash = s.crash[1:]
	s.noteTruncation("crash/",
		fmt.Sprintf("诊断包超过 %d MiB，跳过 %s", maxBundleBytes/(1<<20), removed))
	return true
}

// trimEvents drops the oldest half of events.jsonl. It reports whether it
// changed anything.
func (s *bundleState) trimEvents() bool {
	if len(s.eventsChunks) == 0 {
		return false
	}
	keep := len(s.eventsChunks) / 2
	dropped := len(s.eventsChunks) - keep
	s.eventsChunks = s.eventsChunks[keep:]
	s.eventsJSONL = joinChunks(s.eventsChunks)
	s.noteTruncation("events.jsonl",
		fmt.Sprintf("诊断包超过 %d MiB，进一步丢弃最旧 %d 条事件", maxBundleBytes/(1<<20), dropped))
	return true
}

// trimSettings empties settings-summary.json as the last size resort. It
// reports whether it changed anything.
func (s *bundleState) trimSettings() bool {
	if len(s.settings) == 0 && string(s.settingsJSON) == "{}\n" {
		return false
	}
	s.settings = SettingsSummary{}
	s.settingsJSON = []byte("{}\n")
	s.noteTruncation("settings-summary.json",
		fmt.Sprintf("诊断包超过 %d MiB，设置摘要已清空", maxBundleBytes/(1<<20)))
	return true
}

// appendNote joins two truncation notes.
func appendNote(previous, note string) string {
	if previous == "" {
		return note
	}
	return previous + "；" + note
}

// insertManifest places manifest.json right after summary.md.
func insertManifest(entries []bundleEntry, manifest []byte) []bundleEntry {
	out := make([]bundleEntry, 0, len(entries)+1)
	if len(entries) > 0 {
		out = append(out, entries[0])
	}
	out = append(out, bundleEntry{name: "manifest.json", data: manifest})
	out = append(out, entries[1:]...)
	return out
}

// zipSize builds the archive in memory and returns its size, mirroring exactly
// what writeZipFile will produce.
func zipSize(entries []bundleEntry, modified time.Time) (int64, error) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	if err := writeZipEntries(writer, entries, modified); err != nil {
		return 0, err
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}
	return int64(buf.Len()), nil
}

// writeZipEntries writes every entry through an existing zip writer. Every
// entry carries the export time so an unpacked bundle has a meaningful date.
func writeZipEntries(writer *zip.Writer, entries []bundleEntry, modified time.Time) error {
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate, Modified: modified}
		part, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := part.Write(entry.data); err != nil {
			return err
		}
	}
	return nil
}

// writeZipFile writes entries to dest, removing the partial file on failure.
func writeZipFile(dest string, entries []bundleEntry, modified time.Time) (int64, error) {
	file, err := os.Create(dest)
	if err != nil {
		return 0, fmt.Errorf("diagnostics: create %q: %w", dest, err)
	}
	remove := true
	defer func() {
		if remove {
			_ = file.Close()
			_ = os.Remove(dest)
		}
	}()
	writer := zip.NewWriter(file)
	if err := writeZipEntries(writer, entries, modified); err != nil {
		_ = writer.Close()
		return 0, fmt.Errorf("diagnostics: write %q: %w", dest, err)
	}
	if err := writer.Close(); err != nil {
		return 0, fmt.Errorf("diagnostics: finish %q: %w", dest, err)
	}
	if err := file.Sync(); err != nil {
		return 0, fmt.Errorf("diagnostics: sync %q: %w", dest, err)
	}
	if err := file.Close(); err != nil {
		return 0, fmt.Errorf("diagnostics: close %q: %w", dest, err)
	}
	remove = false
	info, err := os.Stat(dest)
	if err != nil {
		return 0, fmt.Errorf("diagnostics: stat %q: %w", dest, err)
	}
	return info.Size(), nil
}

// tokenPatterns are the credential shapes observability redacts upstream. This
// package treats them as a hard stop: a line still matching one is dropped and
// the manifest records redaction_reviewed=false.
var tokenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`glpat-[A-Za-z0-9_\-]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)(token|password|secret|apikey|authorization)\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)("(?:token|password|secret|apikey|authorization)"\s*:\s*)"(?:[^"\\]|\\.)*"`),
}

// absPathPattern matches absolute home paths on macOS, Linux and Windows.
var absPathPattern = regexp.MustCompile(`(?:/Users/|/home/|/root/|[A-Za-z]:\\\\Users\\\\|/private/var/folders/)`)

// sanitizer records whether it had to drop or replace content, and implements
// the last-line-of-defence scan described in docs/observability.md §4.
type sanitizer struct {
	flagged bool
}

// mark notes that suspicious content was found.
func (s *sanitizer) mark() { s.flagged = true }

// suspiciousText reports whether one line contains an absolute home path or a
// credential shape.
func suspiciousText(line string) bool {
	if absPathPattern.MatchString(line) {
		return true
	}
	for _, pattern := range tokenPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

// scrubText drops suspicious lines from a text file.
func (s *sanitizer) scrubText(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	lines := strings.SplitAfter(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		if suspiciousText(line) {
			s.mark()
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, ""))
}

// scrubJSON replaces suspicious strings inside a JSON document, keeping the
// document valid.
func (s *sanitizer) scrubJSON(data []byte) []byte {
	text := string(data)
	for _, pattern := range tokenPatterns {
		if !pattern.MatchString(text) {
			continue
		}
		s.mark()
		if pattern.NumSubexp() > 0 {
			text = pattern.ReplaceAllString(text, `${1}"<redacted>"`)
		} else {
			text = pattern.ReplaceAllString(text, "<redacted>")
		}
	}
	if absPathPattern.MatchString(text) {
		s.mark()
		text = absPathPattern.ReplaceAllString(text, "<redacted>")
	}
	return []byte(text)
}

// env returns a copy of in with suspicious string fields replaced.
func (s *sanitizer) env(in Env) Env {
	clean := func(value string) string {
		if suspiciousText(value) {
			s.mark()
			return "<redacted>"
		}
		return value
	}
	in.AppVersion = clean(in.AppVersion)
	in.Commit = clean(in.Commit)
	in.BuildDate = clean(in.BuildDate)
	in.GoVersion = clean(in.GoVersion)
	in.VivaVersion = clean(in.VivaVersion)
	in.GOOS = clean(in.GOOS)
	in.GOARCH = clean(in.GOARCH)
	in.Locale = clean(in.Locale)
	in.Theme = clean(in.Theme)
	in.EditorFont = clean(in.EditorFont)
	in.GitVersion = clean(in.GitVersion)
	in.GitSource = clean(in.GitSource)
	in.ConfigDir = clean(in.ConfigDir)
	in.LogLevel = clean(in.LogLevel)
	return in
}

// settings returns a sanitized copy of the settings summary. A JSON round trip
// is used so nested maps and slices are covered too.
func (s *sanitizer) settings(in SettingsSummary) SettingsSummary {
	if len(in) == 0 {
		return SettingsSummary{}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		out := make(SettingsSummary, len(in))
		for key, value := range in {
			if suspiciousText(key) {
				s.mark()
				continue
			}
			out[key] = s.sanitizeValue(value)
		}
		return out
	}
	raw = s.scrubJSON(raw)
	var out SettingsSummary
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return SettingsSummary{}
	}
	return out
}

// sanitizeValue recursively replaces suspicious strings when the JSON round
// trip is not available.
func (s *sanitizer) sanitizeValue(value any) any {
	switch typed := value.(type) {
	case string:
		if suspiciousText(typed) {
			s.mark()
			return "<redacted>"
		}
		return typed
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if suspiciousText(key) {
				s.mark()
				continue
			}
			out[key] = s.sanitizeValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = s.sanitizeValue(item)
		}
		return out
	default:
		return value
	}
}

// repos returns a copy of in with suspicious string fields replaced.
func (s *sanitizer) repos(in []RepoInfo) []RepoInfo {
	out := make([]RepoInfo, len(in))
	copy(out, in)
	for i := range out {
		out[i].Hash = s.cleanString(out[i].Hash)
		out[i].Name = s.cleanString(out[i].Name)
		out[i].Branch = s.cleanString(out[i].Branch)
		out[i].Operation = s.cleanString(out[i].Operation)
	}
	return out
}

// records drops records whose serialized form still contains sensitive text.
func (s *sanitizer) records(in []observability.Record) []observability.Record {
	out := make([]observability.Record, 0, len(in))
	for _, record := range in {
		raw, err := json.Marshal(record)
		if err != nil {
			s.mark()
			continue
		}
		if suspiciousText(string(raw)) {
			s.mark()
			continue
		}
		out = append(out, record)
	}
	return out
}

// consoleLines drops console lines whose command or output is suspicious.
func (s *sanitizer) consoleLines(in []ConsoleLine) []ConsoleLine {
	out := make([]ConsoleLine, 0, len(in))
	for _, line := range in {
		if suspiciousText(line.Command) || suspiciousText(line.Output) {
			s.mark()
			continue
		}
		out = append(out, line)
	}
	return out
}

// description drops suspicious lines from the user's description.
func (s *sanitizer) description(in string) string {
	if in == "" {
		return ""
	}
	lines := strings.Split(in, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if suspiciousText(line) {
			s.mark()
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// cleanString replaces one suspicious string.
func (s *sanitizer) cleanString(value string) string {
	if suspiciousText(value) {
		s.mark()
		return "<redacted>"
	}
	return value
}
