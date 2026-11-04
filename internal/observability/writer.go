package observability

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// File naming and retention of the on-disk log. These are part of the package
// contract documented in the package comment at the top of logger.go.
const (
	// logFilePrefix and logFileSuffix identify files the logger owns. Anything
	// else in the directory is left alone.
	logFilePrefix = "viff-"
	logFileSuffix = ".jsonl"
	// logFileTimeLayout is the date+time part of a file name. Seconds are
	// enough to separate runs; the pid and, on collision, a counter make the
	// name unique even for two starts within the same second.
	logFileTimeLayout = "20060102-150405"
)

// fileWriter appends records as JSON lines to a rotating set of files in one
// directory:
//
//   - the active file rotates when the next line would exceed MaxFileBytes;
//   - names are viff-YYYYMMDD-HHMMSS-<pid>.jsonl, with -1, -2, ... suffixes
//     when the name is already taken, so a second start never overwrites an
//     earlier log;
//   - before a new file is opened the directory is pruned to MaxFiles files,
//     MaxBytes in total, and fileMaxAge per file, oldest first;
//   - directories are created 0700 and files 0600.
//
// A fileWriter is not safe for concurrent use; the Log writer goroutine is its
// only caller.
type fileWriter struct {
	dir          string
	maxBytes     int64
	maxFileBytes int64
	maxFiles     int

	f    *os.File
	size int64
	path string
}

// fileMaxAge is how long a log file is kept regardless of the other limits.
// The four-week window matches docs/observability.md §5 (14 days).
const fileMaxAge = 14 * 24 * time.Hour

// newFileWriter prunes the directory and opens the first file for this process.
// It returns an error when the directory cannot be written, which is what lets
// New report a broken log location instead of failing later.
func newFileWriter(dir string, maxBytes, maxFileBytes int64, maxFiles int) (*fileWriter, error) {
	if maxFiles < 1 {
		maxFiles = 1
	}
	w := &fileWriter{
		dir:          dir,
		maxBytes:     maxBytes,
		maxFileBytes: maxFileBytes,
		maxFiles:     maxFiles,
	}
	// Reserve one slot for the file opened below.
	w.prune(maxFiles - 1)
	if err := w.openNewFile(); err != nil {
		return nil, fmt.Errorf("observability: open log file: %w", err)
	}
	return w, nil
}

// WriteRecord appends one record as a JSON line, rotating first when the
// active file is full. The caller owns error accounting; a write error is not
// fatal.
func (w *fileWriter) WriteRecord(rec Record) error {
	line, err := marshalRecord(rec)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if w.f == nil {
		if err := w.openNewFile(); err != nil {
			return err
		}
	}
	if w.size > 0 && w.size+int64(len(line)) > w.maxFileBytes {
		if err := w.rotate(); err != nil {
			return err
		}
	}
	n, err := w.f.Write(line)
	w.size += int64(n)
	return err
}

// Sync flushes the active file to the operating system's stable storage.
func (w *fileWriter) Sync() error {
	if w.f == nil {
		return nil
	}
	return w.f.Sync()
}

// Close syncs and closes the active file. It is safe to call more than once.
func (w *fileWriter) Close() error {
	if w.f == nil {
		return nil
	}
	syncErr := w.f.Sync()
	closeErr := w.f.Close()
	w.f = nil
	w.path = ""
	w.size = 0
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// rotate closes the active file, prunes the directory and opens the next file.
// Errors from the sync of the closing file are deliberately not returned: the
// records are already written, and Flush reports sync errors on its own.
func (w *fileWriter) rotate() error {
	_ = w.Close()
	w.prune(w.maxFiles - 1)
	return w.openNewFile()
}

// openNewFile creates the next uniquely named log file.
func (w *fileWriter) openNewFile() error {
	stamp := time.Now().Format(logFileTimeLayout)
	base := logFilePrefix + stamp + fmt.Sprintf("-%d", os.Getpid())
	for i := 0; i < 1000; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		name += logFileSuffix
		path := filepath.Join(w.dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o600)
		if err == nil {
			w.f = f
			w.path = path
			w.size = 0
			return nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return errors.New("observability: cannot find a free log file name")
}

// prune deletes the oldest log files until at most limit remain, the total
// size is at most maxBytes, and no file is older than fileMaxAge. limit is the
// number of old files to keep; the caller adds the new active file afterwards.
// Read errors are ignored: pruning is best effort and must never break logging.
func (w *fileWriter) prune(limit int) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	type logFile struct {
		path string
		size int64
		mod  time.Time
	}
	files := make([]logFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, logFilePrefix) || !strings.HasSuffix(name, logFileSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, logFile{
			path: filepath.Join(w.dir, name),
			size: info.Size(),
			mod:  info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	total := int64(0)
	for _, f := range files {
		total += f.size
	}
	kept := len(files)
	now := time.Now()
	for i, f := range files {
		remove := false
		if fileMaxAge > 0 && now.Sub(f.mod) > fileMaxAge {
			remove = true
		}
		if kept > limit {
			remove = true
		}
		// Never remove the newest file just because it alone is larger than
		// the total budget; there is nowhere better to put it.
		if total > w.maxBytes && i < len(files)-1 {
			remove = true
		}
		if !remove {
			continue
		}
		if err := os.Remove(f.path); err == nil {
			total -= f.size
			kept--
		}
	}
}
