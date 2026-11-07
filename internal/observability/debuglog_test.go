package observability

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDebugLogAppendsWithinWindow(t *testing.T) {
	dir := t.TempDir()
	d := OpenDebugLog(dir)
	if d == nil {
		t.Fatal("OpenDebugLog returned nil")
	}
	d.Step("phase one %dms", 12)
	d.Close()
	d = OpenDebugLog(dir)
	if d == nil {
		t.Fatal("second OpenDebugLog returned nil")
	}
	d.Step("phase two")
	d.Close()

	data, err := os.ReadFile(filepath.Join(dir, "debug.txt"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Count(text, "#epoch ") != 1 {
		t.Fatalf("expected one epoch header, got:\n%s", text)
	}
	if !strings.Contains(text, "phase one 12ms") || !strings.Contains(text, "phase two") {
		t.Fatalf("expected both steps in one window, got:\n%s", text)
	}
}

func TestDebugLogExpiresAfterTTL(t *testing.T) {
	dir := t.TempDir()
	d := OpenDebugLog(dir)
	d.Step("old session")
	d.Close()

	// Backdate the window start beyond the TTL.
	path := filepath.Join(dir, "debug.txt")
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-DebugTTL - time.Minute).Format(time.RFC3339)
	if _, err := f.WriteString(debugEpochPrefix + stale + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	d = OpenDebugLog(dir)
	if d == nil {
		t.Fatal("OpenDebugLog returned nil")
	}
	d.Step("new session")
	d.Close()

	f, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) != 3 { // header, process start, new session
		t.Fatalf("expected a fresh 3-line window, got %d lines:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if strings.Contains(strings.Join(lines, "\n"), "old session") {
		t.Fatal("expired content survived the rewrite")
	}
}

func TestDebugLogNilIsSafe(t *testing.T) {
	var d *DebugLog
	d.Step("no-op")
	d.Close() // must not panic
}
