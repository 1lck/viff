//go:build windows

package updater

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// installWindows replaces the running viff.exe from the portable ZIP. The
// installer and registry entries stay untouched; a rename of the running
// executable is enough for the detached helper to swap in the new binary.
func (c *Client) installWindows(u *Update, downloaded, exe string) (*InstallResult, error) {
	if !strings.HasSuffix(strings.ToLower(downloaded), ".zip") {
		return nil, fmt.Errorf("%w: Windows updates use the portable ZIP", ErrUnsupported)
	}
	if err := requireWritableDir(filepath.Dir(exe), exe, downloaded,
		"the installation folder is not writable; extract the downloaded ZIP manually"); err != nil {
		return nil, err
	}
	dir, err := c.ensureCacheDir()
	if err != nil {
		return nil, err
	}
	staged := filepath.Join(dir, fmt.Sprintf("viff-new-%d.exe", time.Now().UnixNano()))
	if err := extractWindowsExecutable(downloaded, staged); err != nil {
		return nil, err
	}
	// A ZIP can contain any bytes; refuse to schedule a helper for something
	// that is not a Windows executable.
	header := make([]byte, 2)
	file, err := os.Open(staged)
	if err != nil {
		return nil, fmt.Errorf("updater: open staged executable: %w", err)
	}
	_, readErr := io.ReadFull(file, header)
	_ = file.Close()
	if readErr != nil || header[0] != 'M' || header[1] != 'Z' {
		_ = os.Remove(staged)
		return nil, fmt.Errorf("updater: download is not a Windows executable")
	}

	script := filepath.Join(dir, c.helperName("bat"))
	if err := os.WriteFile(script, []byte(windowsHelperScript), 0o600); err != nil {
		_ = os.Remove(staged)
		return nil, fmt.Errorf("updater: write install helper: %w", err)
	}
	cmd := exec.Command("cmd", windowsHelperArgs(script, exe, staged, currentPID(), os.Args[1:])...)
	if err := startDetached(cmd); err != nil {
		return nil, fmt.Errorf("updater: start install helper: %w", err)
	}
	return &InstallResult{Target: exe}, nil
}

// extractWindowsExecutable writes the viff.exe member of a portable ZIP to
// dst. Only the base name is matched, so no archive path can escape dst.
func extractWindowsExecutable(archivePath, dst string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("updater: open update ZIP: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.EqualFold(filepath.Base(file.Name), "viff.exe") {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return fmt.Errorf("updater: open viff.exe in ZIP: %w", err)
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
		if err != nil {
			_ = src.Close()
			return fmt.Errorf("updater: create staged executable: %w", err)
		}
		if _, err := io.Copy(out, src); err != nil {
			_ = src.Close()
			_ = out.Close()
			_ = os.Remove(dst)
			return fmt.Errorf("updater: extract viff.exe: %w", err)
		}
		_ = src.Close()
		if err := out.Close(); err != nil {
			_ = os.Remove(dst)
			return fmt.Errorf("updater: close staged executable: %w", err)
		}
		return nil
	}
	return fmt.Errorf("updater: update ZIP has no viff.exe")
}

// installPlatform calls installWindows.
func (c *Client) installPlatform(u *Update, downloaded, exe string) (*InstallResult, error) {
	return c.installWindows(u, downloaded, exe)
}
