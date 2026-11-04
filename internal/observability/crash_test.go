package observability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCrashDir(t *testing.T) {
	cases := []struct {
		logDir string
		want   string
	}{
		{filepath.Join(os.TempDir(), "cfg", "Viff", "logs"), filepath.Join(os.TempDir(), "cfg", "Viff", "crash")},
		{filepath.Join(os.TempDir(), "cfg", "Viff", "logs") + string(filepath.Separator), filepath.Join(os.TempDir(), "cfg", "Viff", "crash")},
		{"logs", "crash"},
		{"", "crash"},
	}
	for _, tc := range cases {
		if got := CrashDir(tc.logDir); got != tc.want {
			t.Fatalf("CrashDir(%q) = %q, want %q", tc.logDir, got, tc.want)
		}
	}
}

func TestWriteCrash(t *testing.T) {
	crashDir := filepath.Join(t.TempDir(), "crash")
	context := []Record{
		{Time: time.Now().UTC(), Seq: 7, Session: "deadbeef", Level: "info", Event: EventRepoOpen, Repo: "abcd1234"},
		{Time: time.Now().UTC(), Seq: 8, Session: "deadbeef", Level: "error", Event: EventGitFail, Op: "push", Code: "push.rejected"},
	}
	path, err := WriteCrash(crashDir, Crash{
		Where:   "main",
		Message: "boom",
		Stack:   []byte("goroutine 1 [running]:\nmain.run()\n"),
		Context: context,
	})
	if err != nil {
		t.Fatalf("WriteCrash: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(path), crashFilePrefix) || !strings.HasSuffix(path, crashFileSuffix) {
		t.Fatalf("unexpected crash file name %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat crash file: %v", err)
	}
	checkPerm(t, info, 0o600, "crash file")
	if info.Size() == 0 {
		t.Fatal("crash file is empty")
	}
	dirInfo, err := os.Stat(crashDir)
	if err != nil {
		t.Fatalf("stat crash dir: %v", err)
	}
	checkPerm(t, dirInfo, 0o700, "crash dir")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read crash file: %v", err)
	}
	text := string(content)
	for _, want := range []string{"viff crash report", "where: main", "message: boom", "stack:", "main.run()", "recent events:", EventRepoOpen, EventGitFail} {
		if !strings.Contains(text, want) {
			t.Fatalf("crash file does not contain %q:\n%s", want, text)
		}
	}

	// The temporary file used for the atomic write must not be left behind.
	entries, err := os.ReadDir(crashDir)
	if err != nil {
		t.Fatalf("read crash dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".last-crash-") {
			t.Fatalf("temporary crash file left behind: %q", entry.Name())
		}
	}
}

func TestWriteCrashCreatesDir(t *testing.T) {
	crashDir := filepath.Join(t.TempDir(), "nested", "crash")
	if _, err := WriteCrash(crashDir, Crash{Where: "main", Message: "x"}); err != nil {
		t.Fatalf("WriteCrash with missing dir: %v", err)
	}
	if _, err := os.Stat(crashDir); err != nil {
		t.Fatalf("crash dir not created: %v", err)
	}
}

func TestWriteCrashEmptyDir(t *testing.T) {
	if _, err := WriteCrash("", Crash{Message: "x"}); err == nil {
		t.Fatal("WriteCrash with empty dir should fail")
	}
}

func TestWriteCrashRedacts(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}
	crashDir := t.TempDir()
	path, err := WriteCrash(crashDir, Crash{
		Where:   "AppKit callback",
		Message: "token=ghp_0123456789abcdefghij at " + filepath.Join(home, "work", "repo") + " and /var/log/x",
		Stack:   []byte("goroutine 1 [running]:\n\t" + filepath.Join(home, "work", "repo", "main.go") + ":12\n"),
	})
	if err != nil {
		t.Fatalf("WriteCrash: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read crash file: %v", err)
	}
	text := string(content)
	mustNotContain(t, text, "ghp_0123456789abcdefghij", home, "/var/log", filepath.Join(home, "work", "repo"))
	if !strings.Contains(text, redactedValue) {
		t.Fatalf("token placeholder missing:\n%s", text)
	}
}

func TestPreviousCrashNewest(t *testing.T) {
	crashDir := t.TempDir()
	older, err := WriteCrash(crashDir, Crash{Where: "main", Message: "older"})
	if err != nil {
		t.Fatalf("WriteCrash: %v", err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	newer, err := WriteCrash(crashDir, Crash{Where: "main", Message: "newer"})
	if err != nil {
		t.Fatalf("WriteCrash: %v", err)
	}
	got, ok := PreviousCrash(crashDir, 7*24*time.Hour)
	if !ok {
		t.Fatal("PreviousCrash found nothing")
	}
	if got != newer {
		t.Fatalf("PreviousCrash = %q, want newest %q (older %q)", got, newer, older)
	}
}

func TestPreviousCrashAge(t *testing.T) {
	crashDir := t.TempDir()
	path, err := WriteCrash(crashDir, Crash{Where: "main", Message: "stale"})
	if err != nil {
		t.Fatalf("WriteCrash: %v", err)
	}
	old := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if _, ok := PreviousCrash(crashDir, 7*24*time.Hour); ok {
		t.Fatal("PreviousCrash reported a stale crash")
	}
	if got, ok := PreviousCrash(crashDir, 0); !ok || got != path {
		t.Fatalf("PreviousCrash(maxAge=0) = %q, %v; want %q, true (any age)", got, ok, path)
	}
}

func TestPreviousCrashIgnoresSeenAndForeignFiles(t *testing.T) {
	crashDir := t.TempDir()
	path, err := WriteCrash(crashDir, Crash{Where: "main", Message: "crash"})
	if err != nil {
		t.Fatalf("WriteCrash: %v", err)
	}
	seen := path + ".20261104-130551.000.seen"
	if err := os.Rename(path, seen); err != nil {
		t.Fatalf("Rename to .seen: %v", err)
	}
	foreign := filepath.Join(crashDir, "notes.txt")
	if err := os.WriteFile(foreign, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got, ok := PreviousCrash(crashDir, 7*24*time.Hour); ok {
		t.Fatalf("PreviousCrash reported %q, want nothing", got)
	}
}

func TestPreviousCrashMissingDir(t *testing.T) {
	if got, ok := PreviousCrash(filepath.Join(t.TempDir(), "missing"), time.Hour); ok {
		t.Fatalf("PreviousCrash on missing dir = %q, true", got)
	}
}
