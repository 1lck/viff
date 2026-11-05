package updater

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCacheDirUsesOverrideAndCreatesDirectory(t *testing.T) {
	root := t.TempDir()
	client := NewClient("v0.1.0")
	client.CacheDir = filepath.Join(root, "updates")
	dir, err := client.ensureCacheDir()
	if err != nil {
		t.Fatalf("ensureCacheDir: %v", err)
	}
	if dir != client.CacheDir {
		t.Fatalf("ensureCacheDir = %q, want %q", dir, client.CacheDir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("cache dir mode = %o, want 700", info.Mode().Perm())
	}
	if client.DownloadDir() != client.CacheDir {
		t.Fatalf("DownloadDir = %q, want %q", client.DownloadDir(), client.CacheDir)
	}
}

func TestCleanupDirRemovesOnlyStaleAndInterruptedFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, age time.Duration) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if age > 0 {
			stamp := time.Now().Add(-age)
			if err := os.Chtimes(path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
		}
	}
	write("download.dmg", 0)
	write("interrupted.part", 0)
	write("previous.old", 0)
	write(helperScriptPrefix+"1.sh", 0)
	write("stale.dmg", 8*24*time.Hour)

	cleanupDir(dir)

	for _, name := range []string{"interrupted.part", "previous.old", helperScriptPrefix + "1.sh", "stale.dmg"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s was not cleaned up", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "download.dmg")); err != nil {
		t.Errorf("fresh download was removed: %v", err)
	}
}

func TestRequireWritableDir(t *testing.T) {
	if err := requireWritableDir(t.TempDir(), "/tmp/target", "/tmp/pkg", "reason"); err != nil {
		t.Fatalf("writable directory rejected: %v", err)
	}
	if os.Getuid() == 0 {
		t.Skip("root can write to a read-only directory")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	err := requireWritableDir(dir, "/tmp/target", "/tmp/pkg", "the folder is read-only")
	if err == nil {
		t.Fatal("read-only directory was accepted")
	}
	var manual *ManualInstallError
	if !errors.As(err, &manual) || !errors.Is(err, ErrManualInstall) {
		t.Fatalf("error = %v, want *ManualInstallError", err)
	}
	if manual.Path != "/tmp/pkg" || manual.Reason != "the folder is read-only" {
		t.Fatalf("manual error = %+v", manual)
	}
}

func TestHelperArguments(t *testing.T) {
	sh := shHelperArgs("/tmp/helper.sh", "/Applications/Viff.app", "/tmp/new.app", 42, []string{"repo"})
	want := []string{"/tmp/helper.sh", "/Applications/Viff.app", "/tmp/new.app", "42", "repo"}
	if strings.Join(sh, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("shHelperArgs = %v, want %v", sh, want)
	}
	win := windowsHelperArgs(`C:\helper.bat`, `C:\Viff\viff.exe`, `C:\new.exe`, 7, []string{"C:\\repo"})
	if win[0] != "/c" || win[1] != `C:\helper.bat` || win[4] != "7" || len(win) != 6 {
		t.Fatalf("windowsHelperArgs = %v", win)
	}
}

func TestHelperScriptsContainPlatformSteps(t *testing.T) {
	for name, script := range map[string]string{
		"darwin":  darwinHelperScript,
		"linux":   linuxHelperScript,
		"windows": windowsHelperScript,
	} {
		if !strings.Contains(script, "target") || !strings.Contains(script, "pid") {
			t.Errorf("%s helper script does not use argv parameters", name)
		}
	}
	for _, step := range []string{"ditto", "open -n"} {
		if !strings.Contains(darwinHelperScript, step) {
			t.Errorf("darwin helper is missing %q", step)
		}
	}
	for _, step := range []string{"install -m 0755", "exec"} {
		if !strings.Contains(linuxHelperScript, step) {
			t.Errorf("linux helper is missing %q", step)
		}
	}
	for _, step := range []string{"move /y", "start", "tasklist"} {
		if !strings.Contains(windowsHelperScript, step) {
			t.Errorf("windows helper is missing %q", step)
		}
	}
}

func TestHelperNameMarksItForCleanup(t *testing.T) {
	client := NewClient("v0.1.0")
	client.Now = func() time.Time { return time.Unix(1700000000, 42) }
	name := client.helperName("sh")
	if !strings.HasPrefix(name, helperScriptPrefix) || !strings.HasSuffix(name, ".sh") {
		t.Fatalf("helper name = %q", name)
	}
	if !strings.Contains(name, "1700000000000000042") {
		t.Fatalf("helper name does not carry the unique timestamp: %q", name)
	}
}

func TestUnderSystemPrefix(t *testing.T) {
	for path, want := range map[string]bool{
		"/usr/bin/viff":         true,
		"/usr/local/bin/viff":   true,
		"/usr":                  true,
		"/home/ada/.local/viff": false,
		"/opt/viff/viff":        false,
		"usr/bin/viff":          false,
		"/usr2/bin/viff":        false,
	} {
		if got := underSystemPrefix(path); got != want {
			t.Errorf("underSystemPrefix(%q) = %v, want %v", path, got, want)
		}
	}
}
