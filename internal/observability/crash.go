package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Crash file naming. PreviousCrash only considers these names, and skips the
// ones main renames to *.seen after telling the user about them.
const (
	crashFilePrefix = "last-crash-"
	crashFileSuffix = ".txt"
	crashSeenSuffix = ".seen"
	crashTimeLayout = "20060102-150405.000"
)

// Crash is one crash report: where it happened, the panic value, the stack, and
// the last events from the in-memory ring. It is written synchronously on the
// crash path, before the process exits, so it must stay small and simple.
type Crash struct {
	// Where names the boundary that caught the panic: "main", "AppKit
	// callback", ...
	Where string
	// Message is the panic value or fault text.
	Message string
	// Stack is debug.Stack() or the framework's stack bytes.
	Stack []byte
	// Context is the most recent records, oldest first. May be nil.
	Context []Record
}

// CrashDir returns the crash directory for a log directory: a sibling of the
// log directory, so a user sharing the logs never has to look elsewhere. An
// empty log directory yields "crash".
func CrashDir(logDir string) string {
	logDir = strings.TrimSpace(logDir)
	if logDir == "" {
		return "crash"
	}
	return filepath.Join(filepath.Dir(filepath.Clean(logDir)), "crash")
}

// WriteCrash writes one crash report as UTF-8 text, mode 0600, in crashDir
// (created with mode 0700 when missing), and returns its path. The file is
// written through a temporary file and renamed so a crash during the write
// cannot leave a half report behind. Paths, URLs and credentials in the report
// are redacted with the standard profile before they reach the disk.
func WriteCrash(crashDir string, c Crash) (string, error) {
	if crashDir == "" {
		return "", errors.New("observability: empty crash directory")
	}
	if err := os.MkdirAll(crashDir, 0o700); err != nil {
		return "", fmt.Errorf("observability: create crash dir: %w", err)
	}
	_ = os.Chmod(crashDir, 0o700)

	redaction := DefaultRedaction()
	var buf bytes.Buffer
	buf.WriteString("viff crash report\n")
	buf.WriteString("time: " + time.Now().UTC().Format(time.RFC3339Nano) + "\n")
	buf.WriteString("where: " + Redact(c.Where, redaction) + "\n")
	buf.WriteString("message: " + Redact(c.Message, redaction) + "\n")
	if len(c.Stack) > 0 {
		buf.WriteString("\nstack:\n")
		stack := Redact(string(c.Stack), redaction)
		buf.WriteString(stack)
		if !strings.HasSuffix(stack, "\n") {
			buf.WriteString("\n")
		}
	}
	if len(c.Context) > 0 {
		buf.WriteString("\nrecent events:\n")
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		for i := range c.Context {
			if err := enc.Encode(c.Context[i]); err != nil {
				break
			}
		}
	}

	tmp, err := os.CreateTemp(crashDir, ".last-crash-*.tmp")
	if err != nil {
		return "", fmt.Errorf("observability: create crash file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		// Best effort: on success the temporary name no longer exists.
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("observability: write crash file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("observability: sync crash file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("observability: close crash file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return "", fmt.Errorf("observability: chmod crash file: %w", err)
	}

	finalPath := crashFilePath(crashDir)
	if err := os.Rename(tmpName, finalPath); err != nil {
		return "", fmt.Errorf("observability: save crash file: %w", err)
	}
	return finalPath, nil
}

// crashFilePath returns a currently unused crash file name.
func crashFilePath(crashDir string) string {
	// Milliseconds plus pid already make collisions very unlikely; the loop
	// only protects against two crashes in the same millisecond.
	stamp := time.Now().UTC().Format(crashTimeLayout)
	base := crashFilePrefix + stamp + fmt.Sprintf("-%d", os.Getpid())
	for i := 0; i < 1000; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		name += crashFileSuffix
		path := filepath.Join(crashDir, name)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return path
		}
	}
	return filepath.Join(crashDir, base+crashFileSuffix)
}

// PreviousCrash returns the newest crash report in crashDir that is not older
// than maxAge. A non-positive maxAge means "any age". Files ending in .seen are
// ignored: main renames a report after showing the user, so a report is only
// announced once. It returns false when the directory is missing, empty, or
// holds only stale reports.
func PreviousCrash(crashDir string, maxAge time.Duration) (string, bool) {
	entries, err := os.ReadDir(crashDir)
	if err != nil {
		return "", false
	}
	now := time.Now()
	var (
		newest     string
		newestTime time.Time
	)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, crashFilePrefix) || !strings.HasSuffix(name, crashFileSuffix) {
			continue
		}
		if strings.HasSuffix(name, crashSeenSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if maxAge > 0 && now.Sub(info.ModTime()) > maxAge {
			continue
		}
		if newest != "" && !info.ModTime().After(newestTime) {
			continue
		}
		newest = filepath.Join(crashDir, name)
		newestTime = info.ModTime()
	}
	return newest, newest != ""
}
