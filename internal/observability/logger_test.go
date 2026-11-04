package observability

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// newTestLog builds a Log and closes it when the test ends.
func newTestLog(t *testing.T, opts Options) *Log {
	t.Helper()
	log, err := New(opts)
	if err != nil {
		t.Fatalf("New(%+v): %v", opts, err)
	}
	t.Cleanup(log.Close)
	return log
}

var sessionPattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

func TestNewDefaults(t *testing.T) {
	log, err := New(Options{})
	if err != nil {
		t.Fatalf("New(Options{}): %v", err)
	}
	defer log.Close()
	if got := log.Dir(); got != "" {
		t.Fatalf("Dir() = %q, want empty (memory only)", got)
	}
	if got := log.Level(); got != LevelInfo {
		t.Fatalf("Level() = %v, want LevelInfo", got)
	}
	if got := log.Dropped(); got != 0 {
		t.Fatalf("Dropped() = %d, want 0", got)
	}
	opts := log.opts
	if opts.MaxBytes != defaultMaxBytes || opts.MaxFileBytes != defaultMaxFileBytes {
		t.Fatalf("size defaults = %d/%d, want %d/%d", opts.MaxBytes, opts.MaxFileBytes, defaultMaxBytes, defaultMaxFileBytes)
	}
	if opts.MaxFiles != defaultMaxFiles || opts.RingSize != ringSizeDefault {
		t.Fatalf("count defaults = %d/%d, want %d/%d", opts.MaxFiles, opts.RingSize, defaultMaxFiles, ringSizeDefault)
	}
	if got := log.Snapshot(time.Time{}, 0); got != nil {
		t.Fatalf("empty Snapshot = %+v, want nil", got)
	}
}

func TestNewExplicitOptions(t *testing.T) {
	dir := t.TempDir()
	log := newTestLog(t, Options{Dir: dir, Level: LevelWarn, MaxBytes: 1 << 20, MaxFileBytes: 4096, MaxFiles: 2, RingSize: 7})
	if got := log.Dir(); got != dir {
		t.Fatalf("Dir() = %q, want %q", got, dir)
	}
	if got := log.Level(); got != LevelWarn {
		t.Fatalf("Level() = %v, want LevelWarn", got)
	}
	if log.opts.RingSize != 7 || log.opts.MaxFiles != 2 || log.opts.MaxBytes != 1<<20 || log.opts.MaxFileBytes != 4096 {
		t.Fatalf("options were not preserved: %+v", log.opts)
	}
}

func TestNewClampsFileBytesToTotal(t *testing.T) {
	log := newTestLog(t, Options{Dir: t.TempDir(), MaxBytes: 1024, MaxFileBytes: 1 << 20})
	if log.opts.MaxFileBytes != 1024 {
		t.Fatalf("MaxFileBytes = %d, want clamped to MaxBytes 1024", log.opts.MaxFileBytes)
	}
}

func TestNewInvalidDir(t *testing.T) {
	parent := t.TempDir()
	file := filepath.Join(parent, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	log, err := New(Options{Dir: filepath.Join(file, "logs")})
	if err == nil {
		log.Close()
		t.Fatal("New should fail when the log dir cannot be created")
	}
}

func TestNewDoesNotWriteStartEvent(t *testing.T) {
	dir := t.TempDir()
	log := newTestLog(t, Options{Dir: dir})
	log.Flush()
	if records := readLogRecords(t, dir); len(records) != 0 {
		t.Fatalf("New wrote %d record(s); app.start belongs to main", len(records))
	}
}

func TestLevelFiltering(t *testing.T) {
	log := newTestLog(t, Options{Dir: t.TempDir(), Level: LevelInfo})
	log.Event(Event{Event: "level.debug", Level: LevelDebug})
	log.Event(Event{Event: "level.info", Level: LevelInfo})
	log.Event(Event{Event: "level.warn", Level: LevelWarn})
	log.Event(Event{Event: "level.error", Level: LevelError})
	records := log.Snapshot(time.Time{}, 0)
	if len(records) != 3 {
		t.Fatalf("got %d records at info level, want 3", len(records))
	}
	for _, rec := range records {
		if rec.Event == "level.debug" {
			t.Fatalf("debug event was recorded at info level: %+v", rec)
		}
	}

	log.SetLevel(LevelDebug)
	log.Event(Event{Event: "level.debug", Level: LevelDebug})
	if got := len(log.Snapshot(time.Time{}, 0)); got != 4 {
		t.Fatalf("got %d records after SetLevel(debug), want 4", got)
	}

	log.SetLevel(LevelError)
	log.Event(Event{Event: "level.info2", Level: LevelInfo})
	log.Event(Event{Event: "level.error2", Level: LevelError})
	if got := len(log.Snapshot(time.Time{}, 0)); got != 5 {
		t.Fatalf("got %d records at error level, want 5", got)
	}
}

func TestRecordFieldsAndRedaction(t *testing.T) {
	dir := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("home directory unavailable")
	}
	repo := filepath.Join(dir, "repo")
	log := newTestLog(t, Options{Dir: dir, Level: LevelDebug})
	log.Event(Event{
		Event: EventGitFail,
		Level: LevelWarn,
		Repo:  repo,
		Op:    "push",
		Code:  "push.rejected",
		Dur:   1500 * time.Millisecond,
		Fields: map[string]any{
			"msg":   "failed to push " + filepath.Join(home, "work", "repo"),
			"count": 3,
			"write": true,
			"paths": []string{filepath.Join(home, "a.go"), "src/b.go"},
			"meta":  map[string]string{"where": "/etc/hosts"},
			"token": "ghp_0123456789abcdefghij",
		},
	})
	log.Flush()

	records := log.Snapshot(time.Time{}, 0)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	rec := records[0]
	if rec.Seq != 1 {
		t.Fatalf("Seq = %d, want 1", rec.Seq)
	}
	if !sessionPattern.MatchString(rec.Session) {
		t.Fatalf("Session = %q, want 8 lowercase hex characters", rec.Session)
	}
	if rec.Level != "warn" || rec.Event != EventGitFail {
		t.Fatalf("level/event = %q/%q", rec.Level, rec.Event)
	}
	if rec.Repo != RepoHash(repo) || rec.Repo == repo {
		t.Fatalf("Repo = %q, want hash %q and never the path", rec.Repo, RepoHash(repo))
	}
	if rec.Op != "push" || rec.Code != "push.rejected" {
		t.Fatalf("op/code = %q/%q", rec.Op, rec.Code)
	}
	if rec.DurMS != 1500 {
		t.Fatalf("DurMS = %d, want 1500", rec.DurMS)
	}
	if rec.Time.Location() != time.UTC || rec.Time.Nanosecond()%int(time.Millisecond) != 0 {
		t.Fatalf("Time = %v, want UTC with millisecond precision", rec.Time)
	}
	mustNotContain(t, rec.Msg, home, "ghp_")
	if !strings.Contains(rec.Msg, "~") {
		t.Fatalf("home directory not replaced in msg: %q", rec.Msg)
	}
	if got := rec.Data["token"]; got != redactedValue {
		t.Fatalf("data.token = %v, want %q", got, redactedValue)
	}
	if got := rec.Data["count"]; got != 3 {
		t.Fatalf("data.count = %v, want 3", got)
	}
	paths, ok := rec.Data["paths"].([]string)
	if !ok || len(paths) != 2 {
		t.Fatalf("data.paths = %#v", rec.Data["paths"])
	}
	if !strings.HasPrefix(paths[0], "~") || paths[1] != "src/b.go" {
		t.Fatalf("data.paths = %#v, want redacted home and untouched relative", paths)
	}
	meta, ok := rec.Data["meta"].(map[string]string)
	if !ok || meta["where"] != placeholderPath {
		t.Fatalf("data.meta = %#v", rec.Data["meta"])
	}
	if _, exists := rec.Data["msg"]; exists {
		t.Fatalf("msg must be extracted out of data: %#v", rec.Data)
	}

	// Nothing sensitive may reach the file either.
	content, err := os.ReadFile(filepath.Join(dir, logFiles(t, dir)[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	mustNotContain(t, text, home, "ghp_", repo, "/etc/hosts")
	var disk Record
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &disk); err != nil {
		t.Fatalf("log line does not decode as Record: %v", err)
	}
	if disk.Event != EventGitFail || disk.Repo != RepoHash(repo) {
		t.Fatalf("disk record = %+v", disk)
	}
}

func TestMessageTruncation(t *testing.T) {
	log := newTestLog(t, Options{})
	log.Event(Event{Event: "msg.long", Level: LevelInfo, Fields: map[string]any{"msg": strings.Repeat("x", 1000)}})
	records := log.Snapshot(time.Time{}, 0)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if got := len(records[0].Msg); got != maxMsgBytes {
		t.Fatalf("msg length = %d, want %d", got, maxMsgBytes)
	}

	// Truncation must not split a rune.
	log.Event(Event{Event: "msg.utf8", Level: LevelInfo, Fields: map[string]any{"msg": strings.Repeat("é", 400)}})
	records = log.Snapshot(time.Time{}, 0)
	msg := records[len(records)-1].Msg
	if !utf8.ValidString(msg) {
		t.Fatalf("truncated message is not valid UTF-8: %q", msg)
	}
	if len(msg) > maxMsgBytes {
		t.Fatalf("truncated message is %d bytes, cap is %d", len(msg), maxMsgBytes)
	}
}

func TestRecordTruncation(t *testing.T) {
	log := newTestLog(t, Options{})
	log.Event(Event{
		Event: "huge",
		Level: LevelInfo,
		Fields: map[string]any{
			"blob": strings.Repeat("a", 8000),
			"n":    1,
			"msg":  "hello",
		},
	})
	records := log.Snapshot(time.Time{}, 0)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	rec := records[0]
	if rec.Msg != "hello" {
		t.Fatalf("Msg = %q, want hello", rec.Msg)
	}
	if got := rec.Data["truncated"]; got != true {
		t.Fatalf("data.truncated = %v, want true", got)
	}
	if _, ok := rec.Data["bytes"]; !ok {
		t.Fatalf("data.bytes missing: %#v", rec.Data)
	}
	encoded, err := marshalRecord(rec)
	if err != nil {
		t.Fatalf("marshalRecord: %v", err)
	}
	if len(encoded) > maxRecordBytes {
		t.Fatalf("encoded record is %d bytes, cap is %d", len(encoded), maxRecordBytes)
	}

	// Many medium-sized fields exceed the cap through sheer count as well.
	many := make(map[string]any, 100)
	for i := 0; i < 100; i++ {
		many[fmt.Sprintf("field_%02d", i)] = strings.Repeat("v", 80)
	}
	log.Event(Event{Event: "huge.many", Level: LevelInfo, Fields: many})
	records = log.Snapshot(time.Time{}, 0)
	rec = records[len(records)-1]
	if rec.Data["truncated"] != true {
		t.Fatalf("data.truncated = %v, want true", rec.Data["truncated"])
	}
	encoded, err = marshalRecord(rec)
	if err != nil {
		t.Fatalf("marshalRecord: %v", err)
	}
	if len(encoded) > maxRecordBytes {
		t.Fatalf("many-field record is %d bytes, cap is %d", len(encoded), maxRecordBytes)
	}
}

func TestRingEviction(t *testing.T) {
	log := newTestLog(t, Options{Dir: "", RingSize: 3})
	for i := 0; i < 5; i++ {
		log.Event(Event{Event: "ring", Level: LevelInfo})
	}
	records := log.Snapshot(time.Time{}, 0)
	if len(records) != 3 {
		t.Fatalf("ring kept %d records, want 3", len(records))
	}
	want := []uint64{3, 4, 5}
	for i, rec := range records {
		if rec.Seq != want[i] {
			t.Fatalf("record %d seq = %d, want %d (oldest evicted first)", i, rec.Seq, want[i])
		}
	}
}

func TestSnapshotSinceAndLimit(t *testing.T) {
	log := newTestLog(t, Options{Dir: ""})
	for i := 0; i < 5; i++ {
		log.Event(Event{Event: "snap", Level: LevelInfo})
	}
	if got := log.Snapshot(time.Now().Add(time.Hour), 0); got != nil {
		t.Fatalf("Snapshot with a future since = %+v, want nil", got)
	}
	if got := log.Snapshot(time.Now().Add(-time.Hour), 0); len(got) != 5 {
		t.Fatalf("Snapshot with a past since = %d records, want 5", len(got))
	}
	limited := log.Snapshot(time.Time{}, 2)
	if len(limited) != 2 || limited[0].Seq != 4 || limited[1].Seq != 5 {
		t.Fatalf("Snapshot(limit 2) = %+v, want the newest two (4, 5)", limited)
	}
	if got := log.Snapshot(time.Time{}, -1); len(got) != 5 {
		t.Fatalf("Snapshot(limit -1) = %d records, want 5", len(got))
	}
}

func TestSessionAndSequence(t *testing.T) {
	first := newTestLog(t, Options{Dir: ""})
	second := newTestLog(t, Options{Dir: ""})
	first.Event(Event{Event: "seq", Level: LevelInfo})
	first.Event(Event{Event: "seq", Level: LevelInfo})
	second.Event(Event{Event: "seq", Level: LevelInfo})

	firstRecs := first.Snapshot(time.Time{}, 0)
	if len(firstRecs) != 2 || firstRecs[0].Seq != 1 || firstRecs[1].Seq != 2 {
		t.Fatalf("first log sequences = %+v, want 1 then 2", firstRecs)
	}
	secondRecs := second.Snapshot(time.Time{}, 0)
	if len(secondRecs) != 1 || secondRecs[0].Seq != 1 {
		t.Fatalf("second log should start at seq 1: %+v", secondRecs)
	}
	if firstRecs[0].Session == secondRecs[0].Session {
		t.Fatalf("two starts shared the session ID %q", firstRecs[0].Session)
	}
	if !sessionPattern.MatchString(secondRecs[0].Session) {
		t.Fatalf("session %q is not 8 hex characters", secondRecs[0].Session)
	}
}

func TestIncidentLifecycle(t *testing.T) {
	log := newTestLog(t, Options{Dir: "", Level: LevelInfo})
	id, err := log.StartIncident("bug", "cannot push /etc/hosts token=abcd1234")
	if err != nil {
		t.Fatalf("StartIncident: %v", err)
	}
	if !strings.HasPrefix(id, "inc-") || len(id) != len("inc-")+8 {
		t.Fatalf("incident ID = %q, want inc-<8 hex>", id)
	}
	if got := log.CurrentIncident(); got != id {
		t.Fatalf("CurrentIncident() = %q, want %q", got, id)
	}
	if got := log.Level(); got != LevelDebug {
		t.Fatalf("Level() during incident = %v, want LevelDebug", got)
	}
	log.Event(Event{Event: "incident.debug", Level: LevelDebug})

	// The level is forced to debug even if someone changes it mid-recording.
	log.SetLevel(LevelError)
	log.Event(Event{Event: "incident.forced", Level: LevelDebug})
	log.SetLevel(LevelInfo)

	log.StopIncident(id)
	if got := log.CurrentIncident(); got != "" {
		t.Fatalf("CurrentIncident() after stop = %q, want empty", got)
	}
	if got := log.Level(); got != LevelInfo {
		t.Fatalf("Level() after stop = %v, want the saved LevelInfo", got)
	}
	log.Event(Event{Event: "incident.after", Level: LevelDebug})

	events := log.IncidentEvents(id)
	if len(events) != 2 {
		t.Fatalf("IncidentEvents = %d records, want 2", len(events))
	}
	for _, rec := range events {
		if rec.Incident != id {
			t.Fatalf("record not tagged with the incident: %+v", rec)
		}
	}
	if got := len(log.Snapshot(time.Time{}, 0)); got != 2 {
		t.Fatalf("incident debug events missing from the snapshot: %d", got)
	}

	incidents := log.Incidents()
	if len(incidents) != 1 {
		t.Fatalf("Incidents() = %d, want 1", len(incidents))
	}
	incident := incidents[0]
	if incident.ID != id || incident.Kind != "bug" || incident.Active || incident.End.IsZero() || incident.Events != 2 {
		t.Fatalf("unexpected incident metadata: %+v", incident)
	}
	mustNotContain(t, incident.Description, "/etc/hosts", "abcd1234")
	if !strings.Contains(incident.Description, placeholderPath) {
		t.Fatalf("incident description was not redacted: %q", incident.Description)
	}

	// Unknown and repeated stops are ignored.
	log.StopIncident("inc-00000000")
	log.StopIncident(id)
	if got := log.IncidentEvents("inc-00000000"); got != nil {
		t.Fatalf("IncidentEvents(unknown) = %+v, want nil", got)
	}
}

func TestIncidentLevelRestored(t *testing.T) {
	log := newTestLog(t, Options{Dir: "", Level: LevelWarn})
	id, err := log.StartIncident("bug", "")
	if err != nil {
		t.Fatalf("StartIncident: %v", err)
	}
	log.Event(Event{Event: "warn.during", Level: LevelWarn})
	log.StopIncident(id)
	if got := log.Level(); got != LevelWarn {
		t.Fatalf("Level() after stop = %v, want LevelWarn", got)
	}
	log.Event(Event{Event: "info.after", Level: LevelInfo})
	if got := len(log.Snapshot(time.Time{}, 0)); got != 1 {
		t.Fatalf("got %d records, want 1 (warn during incident, info filtered)", got)
	}
}

func TestIncidentsMaxFive(t *testing.T) {
	log := newTestLog(t, Options{Dir: ""})
	ids := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		id, err := log.StartIncident("bug", fmt.Sprintf("description %d", i))
		if err != nil {
			t.Fatalf("StartIncident %d: %v", i, err)
		}
		log.Event(Event{Event: "incident.event", Level: LevelDebug})
		log.StopIncident(id)
		ids = append(ids, id)
	}
	incidents := log.Incidents()
	if len(incidents) != maxIncidents {
		t.Fatalf("Incidents() = %d, want %d", len(incidents), maxIncidents)
	}
	if incidents[len(incidents)-1].ID != ids[len(ids)-1] {
		t.Fatalf("newest incident = %q, want %q", incidents[len(incidents)-1].ID, ids[len(ids)-1])
	}
	if got := log.IncidentEvents(ids[0]); got != nil {
		t.Fatalf("evicted incident still has %d records", len(got))
	}
	if got := log.IncidentEvents(ids[len(ids)-1]); len(got) != 1 {
		t.Fatalf("newest incident has %d records, want 1", len(got))
	}
	if got := log.CurrentIncident(); got != "" {
		t.Fatalf("CurrentIncident() = %q, want empty", got)
	}
}

func TestIncidentEventLimit(t *testing.T) {
	log := newTestLog(t, Options{Dir: ""})
	id, err := log.StartIncident("slow", "")
	if err != nil {
		t.Fatalf("StartIncident: %v", err)
	}
	const extra = 5
	for i := 0; i < incidentEventLimit+extra; i++ {
		log.Event(Event{Event: "tick", Level: LevelDebug})
	}
	events := log.IncidentEvents(id)
	if len(events) != incidentEventLimit {
		t.Fatalf("IncidentEvents = %d records, want %d", len(events), incidentEventLimit)
	}
	if events[0].Seq != uint64(extra)+1 {
		t.Fatalf("oldest kept seq = %d, want %d (oldest evicted first)", events[0].Seq, extra+1)
	}
	if last := events[len(events)-1].Seq; last != uint64(incidentEventLimit+extra) {
		t.Fatalf("newest kept seq = %d, want %d", last, incidentEventLimit+extra)
	}
	if incidents := log.Incidents(); len(incidents) != 1 || incidents[0].Events != incidentEventLimit {
		t.Fatalf("incident event count = %+v", incidents)
	}
}

func TestDroppedWhenQueueFull(t *testing.T) {
	log, err := newLog(Options{Dir: t.TempDir()}, 4, false)
	if err != nil {
		t.Fatalf("newLog: %v", err)
	}
	defer log.Close()
	const total = 10
	for i := 0; i < total; i++ {
		log.Event(Event{Event: "drop", Level: LevelInfo})
	}
	if got := log.Dropped(); got != total-4 {
		t.Fatalf("Dropped() = %d, want %d", got, total-4)
	}
	// The in-memory ring still saw every record even though the writer queue
	// dropped some.
	if got := len(log.Snapshot(time.Time{}, 0)); got != total {
		t.Fatalf("Snapshot = %d records, want %d", got, total)
	}
}

func TestEventAfterClose(t *testing.T) {
	dir := t.TempDir()
	log, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Event(Event{Event: "before.close", Level: LevelInfo})
	log.Close()
	before := len(log.Snapshot(time.Time{}, 0))
	log.Event(Event{Event: "after.close", Level: LevelInfo})
	if got := len(log.Snapshot(time.Time{}, 0)); got != before {
		t.Fatalf("Event after Close changed the snapshot: %d -> %d", before, got)
	}
	start := time.Now()
	log.Flush()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Flush after Close took %v", elapsed)
	}
	log.Close()
	if records := readLogRecords(t, dir); len(records) != 1 || records[0].Event != "before.close" {
		t.Fatalf("disk records = %+v, want only before.close", records)
	}
}

func TestCloseIdempotentConcurrent(t *testing.T) {
	log, err := New(Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 20; i++ {
		log.Event(Event{Event: "close.test", Level: LevelInfo})
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Close()
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Close calls hung")
	}
}

func TestWriteErrorDoesNotPanic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	log, err := New(Options{Dir: dir, MaxFileBytes: 64, MaxBytes: 4096})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	for i := 0; i < 5; i++ {
		log.Event(Event{Event: "write.error", Level: LevelInfo, Fields: map[string]any{"pad": strings.Repeat("p", 300)}})
	}
	log.Flush()
	if got := log.fileErrors.Load(); got == 0 {
		t.Fatal("file errors were not counted")
	}
	log.Close()
}

func TestConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	log := newTestLog(t, Options{Dir: dir, RingSize: 256})
	baseline := runtime.NumGoroutine()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				log.Event(Event{
					Event:  "concurrent.event",
					Level:  LevelInfo,
					Fields: map[string]any{"g": g, "i": i},
				})
			}
		}(g)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = log.Snapshot(time.Time{}, 10)
			_ = log.Incidents()
			_ = log.CurrentIncident()
			_ = log.Level()
			_ = log.Dropped()
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			id, err := log.StartIncident("concurrent", "")
			if err == nil {
				log.StopIncident(id)
			}
			log.SetLevel(LevelDebug)
			log.SetLevel(LevelInfo)
		}
	}()
	wg.Wait()
	log.Flush()

	records := log.Snapshot(time.Time{}, 0)
	if len(records) == 0 {
		t.Fatal("no records after concurrent use")
	}
	if len(records) > 256 {
		t.Fatalf("snapshot has %d records, ring size is 256", len(records))
	}
	if got := log.Dropped(); got > 800 {
		t.Fatalf("dropped = %d, more than the events submitted", got)
	}

	runtime.GC()
	after := runtime.NumGoroutine()
	if after > baseline+8 {
		t.Fatalf("goroutines grew from %d to %d", baseline, after)
	}
}
