package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// logFiles returns the files the logger owns in dir, newest name last.
func logFiles(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	var out []os.DirEntry
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, logFilePrefix) || !strings.HasSuffix(name, logFileSuffix) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// readLogRecords decodes every JSONL line of every log file in dir and returns
// the records ordered by sequence number.
func readLogRecords(t *testing.T, dir string) []Record {
	t.Helper()
	var records []Record
	for _, entry := range logFiles(t, dir) {
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", entry.Name(), err)
		}
		if len(content) > 0 && content[len(content)-1] != '\n' {
			t.Fatalf("log file %q does not end with a newline", entry.Name())
		}
		for _, line := range strings.Split(strings.TrimSuffix(string(content), "\n"), "\n") {
			if line == "" {
				continue
			}
			var rec Record
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("log file %q has an invalid JSON line: %v\n%s", entry.Name(), err, line)
			}
			records = append(records, rec)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Seq < records[j].Seq })
	return records
}

func createOldLogFile(t *testing.T, dir, name string, size int, mtime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Repeat("x", size)+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", name, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("Chtimes(%q): %v", name, err)
	}
	return path
}

func TestWriterFileNamingAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	log, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(log.Close)
	log.Event(Event{Event: "writer.test", Level: LevelInfo})
	log.Flush()

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("log dir mode = %o, want 700", got)
	}
	files := logFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("got %d log files, want 1", len(files))
	}
	namePattern := regexp.MustCompile(`^viff-\d{8}-\d{6}-\d+(-\d+)?\.jsonl$`)
	if !namePattern.MatchString(files[0].Name()) {
		t.Fatalf("log file name %q does not match %s", files[0].Name(), namePattern)
	}
	info, err := files[0].Info()
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("log file mode = %o, want 600", got)
	}
	if records := readLogRecords(t, dir); len(records) != 1 || records[0].Event != "writer.test" {
		t.Fatalf("unexpected records on disk: %+v", records)
	}
}

func TestWriterRotation(t *testing.T) {
	dir := t.TempDir()
	log, err := New(Options{Dir: dir, MaxFileBytes: 512, MaxBytes: 4096, MaxFiles: 4})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(log.Close)
	for i := 0; i < 40; i++ {
		log.Event(Event{
			Event:  "writer.rotate",
			Level:  LevelInfo,
			Fields: map[string]any{"i": i, "pad": strings.Repeat("r", 120)},
		})
	}
	log.Flush()

	files := logFiles(t, dir)
	if len(files) < 2 {
		t.Fatalf("expected rotation, got %d file(s)", len(files))
	}
	if len(files) > 4 {
		t.Fatalf("kept %d files, MaxFiles is 4", len(files))
	}
	for _, entry := range files {
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("stat %q: %v", entry.Name(), err)
		}
		if info.Size() > 512 {
			t.Fatalf("file %q is %d bytes, MaxFileBytes is 512", entry.Name(), info.Size())
		}
	}
	if records := readLogRecords(t, dir); len(records) == 0 {
		t.Fatal("no records survived rotation")
	}
}

func TestWriterKeepsAllRecords(t *testing.T) {
	dir := t.TempDir()
	log, err := New(Options{Dir: dir, MaxFileBytes: 300, MaxBytes: 1 << 20, MaxFiles: 100})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(log.Close)
	const events = 20
	for i := 0; i < events; i++ {
		log.Event(Event{
			Event:  "writer.keep",
			Level:  LevelInfo,
			Fields: map[string]any{"i": i, "pad": strings.Repeat("k", 100)},
		})
	}
	log.Flush()

	records := readLogRecords(t, dir)
	if len(records) != events {
		t.Fatalf("read %d records, wrote %d", len(records), events)
	}
	want := make([]uint64, events)
	for i := range want {
		want[i] = uint64(i + 1)
	}
	got := make([]uint64, len(records))
	for i := range records {
		got[i] = records[i].Seq
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sequence numbers = %v, want %v", got, want)
	}
	if len(logFiles(t, dir)) < 2 {
		t.Fatal("expected the records to span more than one file")
	}
}

func TestWriterRestartDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	first, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatalf("New first: %v", err)
	}
	first.Event(Event{Event: "writer.first", Level: LevelInfo})
	first.Close()

	second, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatalf("New second: %v", err)
	}
	second.Event(Event{Event: "writer.second", Level: LevelInfo})
	second.Close()

	files := logFiles(t, dir)
	if len(files) != 2 {
		t.Fatalf("got %d log files, want 2 (one per start)", len(files))
	}
	if files[0].Name() == files[1].Name() {
		t.Fatalf("second start reused the file name %q", files[0].Name())
	}
	records := readLogRecords(t, dir)
	events := make(map[string]bool, len(records))
	for _, rec := range records {
		events[rec.Event] = true
	}
	if !events["writer.first"] || !events["writer.second"] {
		t.Fatalf("events across restarts = %v, want both starts preserved", events)
	}
}

func TestWriterCleanupAge(t *testing.T) {
	dir := t.TempDir()
	old := createOldLogFile(t, dir, "viff-20200101-000000-999.jsonl", 10, time.Now().Add(-15*24*time.Hour))
	recent := createOldLogFile(t, dir, "viff-20260101-000000-998.jsonl", 10, time.Now().Add(-13*24*time.Hour))

	log, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Close()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("file older than 14 days was not deleted (err=%v)", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("file younger than 14 days was deleted: %v", err)
	}
}

func TestWriterCleanupCount(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	names := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		name := "viff-2026010" + string(rune('1'+i)) + "-000000-" + string(rune('1'+i)) + ".jsonl"
		createOldLogFile(t, dir, name, 100, now.Add(-time.Duration(i+1)*time.Hour))
		names = append(names, name)
	}
	log, err := New(Options{Dir: dir, MaxFiles: 3, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Close()

	files := logFiles(t, dir)
	if len(files) != 3 {
		t.Fatalf("got %d files after cleanup, MaxFiles is 3", len(files))
	}
	kept := make(map[string]bool, len(files))
	for _, entry := range files {
		kept[entry.Name()] = true
	}
	// The two newest old files survive; the three oldest are pruned.
	for i, name := range names {
		_, statErr := os.Stat(filepath.Join(dir, name))
		if i >= 2 && !os.IsNotExist(statErr) {
			t.Fatalf("old file %q was not pruned", name)
		}
		if i < 2 && statErr != nil {
			t.Fatalf("new file %q was pruned: %v", name, statErr)
		}
	}
}

func TestWriterCleanupTotalSize(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	names := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		name := "viff-2026020" + string(rune('1'+i)) + "-000000-" + string(rune('1'+i)) + ".jsonl"
		createOldLogFile(t, dir, name, 500, now.Add(-time.Duration(i+1)*time.Hour))
		names = append(names, name)
	}
	log, err := New(Options{Dir: dir, MaxBytes: 1200, MaxFiles: 10})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Close()

	total := int64(0)
	for _, entry := range logFiles(t, dir) {
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("stat %q: %v", entry.Name(), err)
		}
		total += info.Size()
	}
	if total > 1200 {
		t.Fatalf("log directory is %d bytes, MaxBytes is 1200", total)
	}
	// The two newest 500-byte files fit in the budget; the older two do not.
	for i, name := range names {
		_, statErr := os.Stat(filepath.Join(dir, name))
		if i >= 2 && !os.IsNotExist(statErr) {
			t.Fatalf("old file %q was not pruned", name)
		}
		if i < 2 && statErr != nil {
			t.Fatalf("recent file %q was pruned: %v", name, statErr)
		}
	}
}

func TestWriterLeavesForeignFiles(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(foreign, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	log, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Event(Event{Event: "writer.foreign", Level: LevelInfo})
	log.Close()
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign file was removed: %v", err)
	}
	content, err := os.ReadFile(foreign)
	if err != nil || string(content) != "keep me" {
		t.Fatalf("foreign file changed: %q, %v", content, err)
	}
}

func TestWriterSyncThroughFlush(t *testing.T) {
	dir := t.TempDir()
	log, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(log.Close)
	log.Event(Event{Event: "writer.sync", Level: LevelInfo, Fields: map[string]any{"msg": "hello"}})
	log.Flush()
	// Flush synced the file, so the record is readable without Close.
	records := readLogRecords(t, dir)
	if len(records) != 1 || records[0].Msg != "hello" {
		t.Fatalf("records after Flush = %+v", records)
	}
}
