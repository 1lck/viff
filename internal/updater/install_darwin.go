//go:build darwin

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// installDarwin replaces the running .app bundle from the downloaded DMG.
// hdiutil and ditto are system tools; the detached helper performs the swap
// only after Viff exits, so the running bundle is never modified in place.
func (c *Client) installDarwin(u *Update, downloaded, exe string) (*InstallResult, error) {
	if !strings.HasSuffix(strings.ToLower(downloaded), ".dmg") {
		return nil, fmt.Errorf("%w: macOS updates must be DMG packages", ErrUnsupported)
	}
	bundle, ok := bundleRoot(exe)
	if !ok {
		return nil, fmt.Errorf("%w: this executable is not inside a Viff.app bundle", ErrUnsupported)
	}
	if err := requireWritableDir(filepath.Dir(bundle), bundle, downloaded,
		"the application folder is not writable; install the downloaded DMG manually"); err != nil {
		return nil, err
	}

	dir, err := c.ensureCacheDir()
	if err != nil {
		return nil, err
	}
	mount := filepath.Join(dir, fmt.Sprintf("mount-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(mount, 0o700); err != nil {
		return nil, fmt.Errorf("updater: create mount point: %w", err)
	}
	defer os.RemoveAll(mount)

	if err := runTool("hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", mount, downloaded); err != nil {
		return nil, fmt.Errorf("updater: mount %s: %w", filepath.Base(downloaded), err)
	}
	source, err := findAppBundle(mount)
	if err != nil {
		_ = runTool("hdiutil", "detach", mount)
		return nil, err
	}
	staged := filepath.Join(dir, fmt.Sprintf("staged-%d", time.Now().UnixNano()), "Viff.app")
	if err := os.MkdirAll(filepath.Dir(staged), 0o700); err != nil {
		_ = runTool("hdiutil", "detach", mount)
		return nil, fmt.Errorf("updater: create staging directory: %w", err)
	}
	copyErr := runTool("ditto", source, staged)
	detachErr := runTool("hdiutil", "detach", mount)
	if copyErr != nil {
		_ = os.RemoveAll(filepath.Dir(staged))
		return nil, fmt.Errorf("updater: copy new application: %w", copyErr)
	}
	if detachErr != nil {
		_ = os.RemoveAll(filepath.Dir(staged))
		return nil, fmt.Errorf("updater: unmount update: %w", detachErr)
	}
	// The release DMG is ad-hoc signed. Verify the copied bundle before
	// scheduling a swap; a damaged signature is not worth installing.
	if err := runTool("codesign", "--verify", "--deep", "--strict", staged); err != nil {
		_ = os.RemoveAll(filepath.Dir(staged))
		return nil, fmt.Errorf("updater: verify new application signature: %w", err)
	}

	script := filepath.Join(dir, c.helperName("sh"))
	if err := os.WriteFile(script, []byte(darwinHelperScript), 0o700); err != nil {
		_ = os.RemoveAll(filepath.Dir(staged))
		return nil, fmt.Errorf("updater: write install helper: %w", err)
	}
	cmd := exec.Command("/bin/sh", shHelperArgs(script, bundle, staged, currentPID(), os.Args[1:])...)
	if err := startDetached(cmd); err != nil {
		return nil, fmt.Errorf("updater: start install helper: %w", err)
	}
	return &InstallResult{Target: bundle}, nil
}

// bundleRoot returns the .app directory containing exe.
func bundleRoot(exe string) (string, bool) {
	clean := filepath.Clean(exe)
	parts := strings.Split(clean, string(filepath.Separator))
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.HasSuffix(parts[i], ".app") {
			return strings.Join(parts[:i+1], string(filepath.Separator)), true
		}
	}
	return "", false
}

// findAppBundle returns the first .app directory directly under mount.
func findAppBundle(mount string) (string, error) {
	entries, err := os.ReadDir(mount)
	if err != nil {
		return "", fmt.Errorf("updater: read mounted DMG: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".app") {
			return filepath.Join(mount, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("updater: mounted DMG has no .app bundle")
}

// installPlatform calls installDarwin.
func (c *Client) installPlatform(u *Update, downloaded, exe string) (*InstallResult, error) {
	return c.installDarwin(u, downloaded, exe)
}
