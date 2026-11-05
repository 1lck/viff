package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// downloadRetention is how long a downloaded package or interrupted-install
// leftover is kept before Cleanup removes it.
const downloadRetention = 7 * 24 * time.Hour

// cacheDir resolves the download directory. CacheDir is a test/build override;
// otherwise the per-user cache keeps packages across releases.
func (c *Client) cacheDir() string {
	if c.CacheDir != "" {
		return c.CacheDir
	}
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Viff", "updates")
}

// ensureCacheDir creates the download directory with private permissions.
func (c *Client) ensureCacheDir() (string, error) {
	dir := c.cacheDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("updater: create cache dir %s: %w", dir, err)
	}
	return dir, nil
}

// cleanupDir removes interrupted-install leftovers and packages older than
// downloadRetention. Every failure is ignored: cleanup must never prevent the
// application from starting.
func cleanupDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := entry.Name()
		leftover := strings.HasPrefix(name, helperScriptPrefix) ||
			strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".old")
		if leftover || now.Sub(info.ModTime()) > downloadRetention {
			_ = os.RemoveAll(filepath.Join(dir, name))
		}
	}
}

// currentPID is the running process ID encoded into helper names.
func currentPID() int { return os.Getpid() }

// DownloadDir returns the directory update packages are written to.
func (c *Client) DownloadDir() string { return c.cacheDir() }

// Cleanup removes interrupted-install leftovers. It is safe to call at
// startup and never reports an error.
func (c *Client) Cleanup() { cleanupDir(c.cacheDir()) }

// Install schedules replacement of the running installation and a relaunch of
// the new version. The caller must quit when this returns without an error.
func (c *Client) Install(u *Update, downloaded string) (*InstallResult, error) {
	if u == nil {
		return nil, fmt.Errorf("updater: install: no update")
	}
	if downloaded == "" {
		return nil, fmt.Errorf("updater: install: no downloaded package")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("updater: locate running executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return c.installPlatform(u, downloaded, exe)
}

// requireWritableDir proves that dir can be written before a helper is
// scheduled. A read-only installation (typically a root-owned DEB) becomes a
// ManualInstallError with the package the user can install by hand.
func requireWritableDir(dir, target, downloaded, reason string) error {
	probe, err := os.CreateTemp(dir, ".viff-update-probe-*")
	if err != nil {
		return &ManualInstallError{Target: target, Path: downloaded, Reason: reason}
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// runTool runs a system tool and reports its output when it fails. Arguments
// are passed as an array; nothing is ever interpolated into a shell command.
func runTool(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if len(text) > 500 {
			text = text[:500] + "…"
		}
		return fmt.Errorf("%s failed: %w (%s)", name, err, text)
	}
	return nil
}
