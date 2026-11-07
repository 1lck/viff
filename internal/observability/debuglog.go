package observability

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DebugTTL is the validity of debug.txt: the file is cleared three days
// after it was started, whether or not it kept being written. The start of
// the current window is stored in the file's first line.
const DebugTTL = 72 * time.Hour

// debugEpochPrefix marks the header line of debug.txt.
const debugEpochPrefix = "#epoch "

// DebugLog writes human-readable phase timings to debug.txt next to the
// settings. Unlike the JSONL event log it is meant to be opened at a glance;
// it records durations, counts and booleans only — never repository paths,
// arguments, output or content, the same redaction boundary as every other
// file this package writes.
//
// Every method tolerates a nil receiver: a caller that failed to open the
// file (or runs where there is no config directory) simply does not trace.
type DebugLog struct {
	mu    sync.Mutex
	f     *os.File
	start time.Time
}

// OpenDebugLog opens (and creates) debug.txt in dir. The file is valid for
// DebugTTL from its first line; a file whose window has expired is truncated
// and a new one started, so it never holds more than the last three days. It
// returns nil when dir is empty or the file cannot be opened; tracing is
// then silently disabled.
func OpenDebugLog(dir string) *DebugLog {
	if dir == "" {
		return nil
	}
	path := filepath.Join(dir, "debug.txt")
	now := time.Now()
	epoch, expired := readDebugEpoch(path)
	flags := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if expired || now.Sub(epoch) >= DebugTTL {
		flags |= os.O_TRUNC
		epoch = now
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return nil
	}
	d := &DebugLog{f: f, start: now}
	if flags&os.O_TRUNC != 0 {
		// New window: record when it started, so any later process can tell
		// whether the file is still within its three days.
		fmt.Fprintf(f, "%s%s\n", debugEpochPrefix, epoch.Format(time.RFC3339))
	}
	d.Step("process start (pid %d)", os.Getpid())
	return d
}

// readDebugEpoch reports the start of the current debug.txt window and
// whether the header is missing or unreadable (a file without a valid header
// is treated as expired and rewritten).
func readDebugEpoch(path string) (time.Time, bool) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, true
	}
	defer f.Close()
	line, err := bufio.NewReader(f).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, debugEpochPrefix) {
		return time.Time{}, true
	}
	epoch, err := time.Parse(time.RFC3339, strings.TrimPrefix(strings.TrimSpace(line), debugEpochPrefix))
	if err != nil {
		return time.Time{}, true
	}
	return epoch, false
}

// Step records one trace line, prefixed by wall-clock time and the time since
// process start.
func (d *DebugLog) Step(format string, args ...any) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f == nil {
		return
	}
	now := time.Now()
	fmt.Fprintf(d.f, "[%s] +%7dms %s\n",
		now.Format("15:04:05.000"), now.Sub(d.start).Milliseconds(), fmt.Sprintf(format, args...))
}

// Close flushes and closes the file; later steps are dropped.
func (d *DebugLog) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f != nil {
		d.f.Close()
		d.f = nil
	}
}
