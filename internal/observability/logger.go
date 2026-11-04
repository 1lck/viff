// Package observability implements Viff's local event log: an asynchronous,
// bounded, redacted JSON-lines recorder, the redaction helpers shared by the
// diagnostic bundle, and crash reports.
//
// # File naming and rotation
//
// Records are written as one JSON object per line (JSONL) to the directory
// returned by Log.Dir:
//
//	viff-YYYYMMDD-HHMMSS-<pid>.jsonl
//
// The date, time and pid make the name unique, and a numeric suffix (-1, -2,
// ...) is added if the name is already taken, so a restart never overwrites an
// earlier log. A file rotates when the next line would exceed
// Options.MaxFileBytes (2 MiB by default). The directory keeps at most
// Options.MaxFiles files (5 by default) and Options.MaxBytes in total (10 MiB
// by default), and deletes files older than 14 days, oldest first. Pruning runs
// at startup, before the first file is created, and before every rotation. The
// directory is created with mode 0700 and files with 0600.
//
// # Levels and incidents
//
// Events below the current level are dropped before serialization. LevelError
// is 0 and the highest priority, so LevelDebug events are only kept when the
// level is debug; Options.Level defaults to LevelInfo. While a diagnostic
// incident is active the logger forces debug so a user reproduction keeps its
// details, and StopIncident restores the level that StartIncident saved.
// Records written during an incident carry its ID and are kept per incident
// (up to 20000 records, retained for the 5 most recent incidents) so a
// diagnostic bundle can include a recording even after those records age out
// of the in-memory ring.
//
// # Concurrency and failure behavior
//
// Event never blocks: records are serialized on the caller's goroutine, kept in
// a 4096-record in-memory ring for Snapshot, and handed to one writer goroutine
// through a bounded channel. When that channel is full the new record is
// dropped and Dropped increments, so a slow disk cannot stall the UI. File
// errors are counted internally and never panic. New returns an error when the
// log directory cannot be created or written; callers then keep running
// without file logging (for example with Nop).
//
// # Usage
//
//	log, err := observability.New(observability.Options{
//		Dir:   logDir,
//		Level: observability.ParseLevel(level),
//	})
//	if err != nil {
//		// File logging is unavailable; keep running with observability.Nop{}.
//	}
//	defer log.Close()
//	log.Event(observability.Event{
//		Event: observability.EventRepoOpen,
//		Level: observability.LevelInfo,
//		Repo:  root,
//	})
//
// Every string in an Event.Fields value is redacted before it is stored or
// written; see Redact, RedactPath and RepoHash.
package observability

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// Limits and defaults that are part of the observable behavior.
const (
	// queueCapacity is how many records may wait for the writer goroutine.
	// When it is full the event is dropped and Dropped increments.
	queueCapacity = 4096
	// ringSizeDefault is how many recent records stay in memory by default.
	ringSizeDefault = 4096
	// defaultMaxBytes bounds the log directory; see docs/observability.md §5.
	defaultMaxBytes = 10 << 20
	// defaultMaxFileBytes bounds one log file.
	defaultMaxFileBytes = 2 << 20
	// defaultMaxFiles is how many files the directory keeps.
	defaultMaxFiles = 5
	// maxMsgBytes caps the one-line message taken from Fields["msg"].
	maxMsgBytes = 512
	// maxRecordBytes caps one serialized record. Larger records lose data and
	// gain data.truncated = true.
	maxRecordBytes = 4096
	// maxIncidents is how many diagnostic sessions are remembered.
	maxIncidents = 5
	// incidentEventLimit caps the records retained per incident.
	incidentEventLimit = 20000
	// flushTimeout bounds how long Flush and Close wait for the writer.
	flushTimeout = 2 * time.Second
)

// Options configures a Log. The zero value is usable: New fills in the
// defaults, and Dir == "" keeps everything in memory only (tests).
type Options struct {
	// Dir is the directory log files are written to. Empty keeps the log in
	// memory (used by tests and by a build without a writable config dir).
	Dir string
	// Level is the initial level. ParseLevel is the right way to build it.
	// The zero value means LevelInfo here, because ParseLevel("") returns
	// LevelInfo and a zero-valued Options must be usable.
	Level Level
	// MaxBytes bounds the whole log directory. Default 10 MiB.
	MaxBytes int64
	// MaxFileBytes bounds one file. Default 2 MiB.
	MaxFileBytes int64
	// MaxFiles is the number of files kept. Default 5.
	MaxFiles int
	// RingSize is how many recent records stay in memory. Default 4096.
	RingSize int
	// Version is the build version string. It is reserved for callers that
	// want it in an app.start payload; the logger itself does not emit events.
	Version string
}

// Log is the Recorder and IncidentController implementation: it filters,
// redacts, serializes, remembers and writes events. It is safe for concurrent
// use. The zero value is not usable; call New.
type Log struct {
	opts       Options
	queue      chan Record
	flushCh    chan chan struct{}
	closeCh    chan struct{}
	writer     *fileWriter
	writerDone chan struct{}
	closeOnce  sync.Once

	seq        atomic.Uint64
	dropped    atomic.Uint64
	fileErrors atomic.Uint64

	session   string
	redaction Redaction

	// mu guards the fields below. Event takes it for short, non-blocking
	// sections only; file I/O never happens under it.
	mu              sync.RWMutex
	level           Level
	closed          bool
	ring            *ringBuffer
	currentIncident string
	activeIncidents []string
	baseLevel       Level
	hasBaseLevel    bool
	incidents       map[string]*incidentState
	incidentOrder   []*incidentState
}

// incidentState is one diagnostic recording. records is a ring so a long
// session keeps its newest records without copying the whole slice on every
// event.
type incidentState struct {
	meta    Incident
	records *ringBuffer
}

var (
	_ Recorder           = (*Log)(nil)
	_ IncidentController = (*Log)(nil)
)

// New creates a Log. When Dir is set it creates the directory with mode 0700,
// prunes old files and opens the first log file; a failure there is returned so
// the caller can degrade to memory-only logging. New itself does not record an
// event: app.start belongs to main.
func New(opts Options) (*Log, error) {
	return newLog(opts, queueCapacity, true)
}

// newLog is the shared constructor. queueCap and startWriter exist so tests can
// exercise the drop path without a live writer goroutine; production code uses
// New.
func newLog(opts Options, queueCap int, startWriter bool) (*Log, error) {
	opts = normalizeOptions(opts)
	if queueCap <= 0 {
		queueCap = queueCapacity
	}
	l := &Log{
		opts:      opts,
		level:     opts.Level,
		session:   randomHex(4),
		redaction: DefaultRedaction(),
		ring:      newRingBuffer(opts.RingSize),
		incidents: make(map[string]*incidentState),
	}
	if opts.Dir == "" {
		return l, nil
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("observability: create log dir: %w", err)
	}
	// MkdirAll does not tighten an existing directory; logs are private.
	_ = os.Chmod(opts.Dir, 0o700)
	if !startWriter {
		l.queue = make(chan Record, queueCap)
		return l, nil
	}
	w, err := newFileWriter(opts.Dir, opts.MaxBytes, opts.MaxFileBytes, opts.MaxFiles)
	if err != nil {
		return nil, err
	}
	l.writer = w
	l.queue = make(chan Record, queueCap)
	l.flushCh = make(chan chan struct{})
	l.closeCh = make(chan struct{})
	l.writerDone = make(chan struct{})
	go l.writeLoop()
	return l, nil
}

// normalizeOptions fills in the defaults documented on Options.
func normalizeOptions(opts Options) Options {
	if opts.Level == LevelError {
		// The zero value must mean "unset" so Options{} works; only errors
		// can be selected later with SetLevel.
		opts.Level = LevelInfo
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultMaxBytes
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = defaultMaxFileBytes
	}
	if opts.MaxFileBytes > opts.MaxBytes {
		opts.MaxFileBytes = opts.MaxBytes
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = defaultMaxFiles
	}
	if opts.RingSize <= 0 {
		opts.RingSize = ringSizeDefault
	}
	return opts
}

// Event records one event without blocking. Events below the current level are
// dropped; during an incident the level is forced to debug. Repo is hashed and
// every string field is redacted before anything is stored.
func (l *Log) Event(e Event) {
	l.mu.RLock()
	if l.closed {
		l.mu.RUnlock()
		return
	}
	level := l.level
	incident := l.currentIncident
	l.mu.RUnlock()
	if incident != "" {
		level = LevelDebug
	}
	if e.Level > level {
		return
	}
	rec := l.newRecord(e, incident)

	l.mu.Lock()
	l.ring.add(rec)
	if incident != "" {
		if st := l.incidents[incident]; st != nil {
			st.records.add(rec)
		}
	}
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return
	}
	if l.queue == nil {
		return
	}
	select {
	case l.queue <- rec:
	default:
		l.dropped.Add(1)
	}
}

// newRecord serializes and redacts one event. It runs on the caller's
// goroutine; locks are not held.
func (l *Log) newRecord(e Event, incident string) Record {
	rec := Record{
		Time:     time.Now().UTC().Truncate(time.Millisecond),
		Seq:      l.seq.Add(1),
		Session:  l.session,
		Level:    e.Level.String(),
		Event:    e.Event,
		Incident: incident,
		Op:       e.Op,
		Code:     e.Code,
		DurMS:    e.Dur.Milliseconds(),
	}
	if e.Repo != "" {
		rec.Repo = RepoHash(e.Repo)
	}
	if len(e.Fields) > 0 {
		// Copy so the logger never mutates the caller's map.
		data := make(map[string]any, len(e.Fields))
		for k, v := range e.Fields {
			data[k] = v
		}
		if raw, ok := data["msg"]; ok {
			delete(data, "msg")
			msg := Redact(fmt.Sprint(raw), l.redaction)
			rec.Msg = truncateUTF8(msg, maxMsgBytes)
		}
		if len(data) > 0 {
			for k, v := range data {
				data[k] = redactValue(v, l.redaction)
			}
			rec.Data = data
		}
	}
	if len(rec.Data) > 0 {
		truncateRecord(&rec)
	}
	return rec
}

// SetLevel changes the configured level. While an incident is active the
// effective level stays debug; StopIncident restores the level that was
// configured when recording started.
func (l *Log) SetLevel(level Level) {
	l.mu.Lock()
	l.level = level
	l.mu.Unlock()
}

// Level returns the configured level.
func (l *Log) Level() Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

// Snapshot returns remembered records at or after since, newest last. A zero
// since means everything remembered; limit <= 0 means no limit.
func (l *Log) Snapshot(since time.Time, limit int) []Record {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.ring.snapshot(since, limit)
}

// StartIncident begins a diagnostic recording, forces debug detail, and
// returns its ID (inc-<8 hex digits>).
func (l *Log) StartIncident(kind, description string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return "", errors.New("observability: log is closed")
	}
	id := l.newIncidentIDLocked()
	if len(l.activeIncidents) == 0 {
		l.baseLevel = l.level
		l.hasBaseLevel = true
	}
	st := &incidentState{
		meta: Incident{
			ID:          id,
			Kind:        strings.TrimSpace(kind),
			Description: Redact(description, l.redaction),
			Start:       time.Now().UTC().Truncate(time.Millisecond),
			Active:      true,
		},
		records: newRingBuffer(incidentEventLimit),
	}
	l.incidents[id] = st
	l.incidentOrder = append(l.incidentOrder, st)
	l.activeIncidents = append(l.activeIncidents, id)
	l.currentIncident = id
	l.level = LevelDebug
	l.evictIncidentsLocked()
	return id, nil
}

// StopIncident ends a recording and restores the level that was configured
// when the first incident started. Unknown or already stopped IDs are ignored.
func (l *Log) StopIncident(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	idx := -1
	for i, active := range l.activeIncidents {
		if active == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	l.activeIncidents = append(l.activeIncidents[:idx], l.activeIncidents[idx+1:]...)
	if st := l.incidents[id]; st != nil {
		st.meta.Active = false
		st.meta.End = time.Now().UTC().Truncate(time.Millisecond)
	}
	if len(l.activeIncidents) == 0 {
		l.currentIncident = ""
		if l.hasBaseLevel {
			l.level = l.baseLevel
			l.hasBaseLevel = false
		}
		return
	}
	l.currentIncident = l.activeIncidents[len(l.activeIncidents)-1]
}

// Incidents returns the most recent diagnostic sessions, newest last.
func (l *Log) Incidents() []Incident {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.incidentOrder) == 0 {
		return nil
	}
	out := make([]Incident, 0, len(l.incidentOrder))
	for _, st := range l.incidentOrder {
		meta := st.meta
		meta.Events = st.records.len()
		out = append(out, meta)
	}
	return out
}

// IncidentEvents returns the records tagged with one incident, newest last. It
// returns nil for an unknown (or already evicted) ID.
func (l *Log) IncidentEvents(id string) []Record {
	l.mu.RLock()
	defer l.mu.RUnlock()
	st := l.incidents[id]
	if st == nil {
		return nil
	}
	return st.records.snapshot(time.Time{}, 0)
}

// CurrentIncident returns the active incident ID, or "".
func (l *Log) CurrentIncident() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.currentIncident
}

// Dir returns the log directory, or "" when file logging is disabled.
func (l *Log) Dir() string {
	return l.opts.Dir
}

// Dropped returns how many records were lost because the writer queue was
// full.
func (l *Log) Dropped() uint64 {
	return l.dropped.Load()
}

// Flush waits until the queued records are written and syncs the files. It
// waits at most two seconds so a stuck disk cannot hang shutdown.
func (l *Log) Flush() {
	if l.flushCh == nil || l.writerDone == nil {
		return
	}
	l.mu.RLock()
	closed := l.closed
	l.mu.RUnlock()
	if closed {
		return
	}
	l.requestFlush(flushTimeout)
}

// Close flushes and releases the log files. It is idempotent: later calls do
// nothing, and Event after Close is silently dropped.
func (l *Log) Close() {
	l.closeOnce.Do(func() {
		l.mu.Lock()
		l.closed = true
		l.mu.Unlock()
		if l.flushCh == nil || l.writerDone == nil {
			return
		}
		l.requestFlush(flushTimeout)
		// Closing rather than sending never blocks, even when the writer is
		// stuck in a write.
		close(l.closeCh)
		select {
		case <-l.writerDone:
		case <-time.After(flushTimeout):
		}
	})
}

// requestFlush asks the writer goroutine to drain the queue and sync, and waits
// for it no longer than timeout.
func (l *Log) requestFlush(timeout time.Duration) {
	if l.flushCh == nil || l.writerDone == nil {
		return
	}
	done := make(chan struct{})
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case l.flushCh <- done:
	case <-l.writerDone:
		return
	case <-timer.C:
		return
	}
	select {
	case <-done:
	case <-l.writerDone:
	case <-timer.C:
	}
}

// writeLoop is the only goroutine that touches the files.
func (l *Log) writeLoop() {
	defer close(l.writerDone)
	for {
		select {
		case rec := <-l.queue:
			l.writeRecord(rec)
		case done := <-l.flushCh:
			l.drainQueue()
			if err := l.writer.Sync(); err != nil {
				l.fileErrors.Add(1)
			}
			close(done)
		case <-l.closeCh:
			l.drainQueue()
			if err := l.writer.Close(); err != nil {
				l.fileErrors.Add(1)
			}
			return
		}
	}
}

// drainQueue writes whatever is queued right now.
func (l *Log) drainQueue() {
	for {
		select {
		case rec := <-l.queue:
			l.writeRecord(rec)
		default:
			return
		}
	}
}

// writeRecord accounts a file error instead of propagating it: a broken log
// must never break the application.
func (l *Log) writeRecord(rec Record) {
	if err := l.writer.WriteRecord(rec); err != nil {
		l.fileErrors.Add(1)
	}
}

// newIncidentIDLocked returns an incident ID that is not in use.
func (l *Log) newIncidentIDLocked() string {
	for i := 0; i < 100; i++ {
		id := "inc-" + randomHex(4)
		if _, used := l.incidents[id]; !used {
			return id
		}
	}
	return "inc-" + randomHex(8)
}

// evictIncidentsLocked drops the oldest incidents beyond maxIncidents, and
// stops them if they were somehow still active.
func (l *Log) evictIncidentsLocked() {
	for len(l.incidentOrder) > maxIncidents {
		old := l.incidentOrder[0]
		l.incidentOrder = l.incidentOrder[1:]
		delete(l.incidents, old.meta.ID)
		if old.meta.Active {
			old.meta.Active = false
			old.meta.End = time.Now().UTC().Truncate(time.Millisecond)
		}
		for i, active := range l.activeIncidents {
			if active == old.meta.ID {
				l.activeIncidents = append(l.activeIncidents[:i], l.activeIncidents[i+1:]...)
				break
			}
		}
		if len(l.activeIncidents) == 0 {
			l.currentIncident = ""
			if l.hasBaseLevel {
				l.level = l.baseLevel
				l.hasBaseLevel = false
			}
		} else {
			l.currentIncident = l.activeIncidents[len(l.activeIncidents)-1]
		}
	}
}

// redactValue redacts strings inside the small JSON-friendly values event
// fields use. Other types pass through unchanged.
func redactValue(v any, r Redaction) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return Redact(x, r)
	case []string:
		out := make([]string, len(x))
		for i := range x {
			out[i] = Redact(x[i], r)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = redactValue(x[i], r)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(x))
		for k, s := range x {
			out[k] = Redact(s, r)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, s := range x {
			out[k] = redactValue(s, r)
		}
		return out
	default:
		return v
	}
}

// truncateRecord enforces the 4 KiB cap on one serialized record. Oversized
// records lose most data, keep their small fields, and gain data.truncated and
// data.bytes so a reader can tell that something was removed.
func truncateRecord(rec *Record) {
	b, err := marshalRecord(*rec)
	if err != nil || len(b) <= maxRecordBytes {
		return
	}
	original := len(b)
	// Keep the small fields; drop values that are large or cannot be encoded.
	keys := make([]string, 0, len(rec.Data))
	for k := range rec.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	const fieldBudget = maxRecordBytes / 2
	kept := make(map[string]any, len(rec.Data)+2)
	used := 0
	for _, k := range keys {
		if len(k) > 64 {
			continue
		}
		v := rec.Data[k]
		if s, ok := v.(string); ok && len(s) > maxMsgBytes {
			v = truncateUTF8(s, maxMsgBytes)
		}
		vb, err := json.Marshal(v)
		if err != nil || len(vb) > maxMsgBytes {
			continue
		}
		if used+len(k)+len(vb) > fieldBudget {
			continue
		}
		kept[k] = v
		used += len(k) + len(vb)
	}
	kept["truncated"] = true
	kept["bytes"] = original
	rec.Data = kept
	if b, err := marshalRecord(*rec); err == nil && len(b) <= maxRecordBytes {
		return
	}
	// The 4 KiB cap is strict: keep only the marker when even the small fields
	// do not fit.
	rec.Data = map[string]any{"truncated": true, "bytes": original}
	if b, err := marshalRecord(*rec); err == nil && len(b) <= maxRecordBytes {
		return
	}
	rec.Msg = truncateUTF8(rec.Msg, 128)
}

// marshalRecord encodes one record without HTML escaping so placeholders such
// as <repo> stay readable in the file. The caller appends the newline.
func marshalRecord(rec Record) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// truncateUTF8 cuts s to at most max bytes without splitting a rune.
func truncateUTF8(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// randomHex returns n random bytes encoded as 2n hexadecimal characters, and
// falls back to a time+pid hash if the system source is unavailable.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())))
	return hex.EncodeToString(sum[:])[:n*2]
}
