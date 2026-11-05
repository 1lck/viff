//go:build linux

package updater

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// installLinux replaces the running binary from the portable tar.gz. A
// system-installed /usr/bin/viff is root-owned, so the writability preflight
// turns that case into a manual-install message instead of a failed swap.
func (c *Client) installLinux(u *Update, downloaded, exe string) (*InstallResult, error) {
	if strings.HasSuffix(strings.ToLower(downloaded), ".deb") {
		// The DEB exists for system-managed installs. Replacing /usr/bin/viff
		// without the package manager would desynchronise dpkg, so hand the
		// verified package to the user with the exact command to run.
		return nil, &ManualInstallError{
			Target: exe,
			Path:   downloaded,
			Reason: "this installation is managed by the system package manager. Run: sudo apt install " + downloaded,
		}
	}
	if !strings.HasSuffix(downloaded, ".tar.gz") {
		return nil, fmt.Errorf("%w: Linux updates use the portable tar.gz or the DEB", ErrUnsupported)
	}
	if err := requireWritableDir(filepath.Dir(exe), exe, downloaded,
		"this installation is read-only; install the downloaded package with your package manager (for example: sudo apt install <downloaded package>)"); err != nil {
		return nil, err
	}
	dir, err := c.ensureCacheDir()
	if err != nil {
		return nil, err
	}
	staged := filepath.Join(dir, fmt.Sprintf("viff-new-%d", time.Now().UnixNano()))
	if err := extractLinuxExecutable(downloaded, staged); err != nil {
		return nil, err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		_ = os.Remove(staged)
		return nil, fmt.Errorf("updater: make staged executable runnable: %w", err)
	}

	script := filepath.Join(dir, c.helperName("sh"))
	if err := os.WriteFile(script, []byte(linuxHelperScript), 0o700); err != nil {
		_ = os.Remove(staged)
		return nil, fmt.Errorf("updater: write install helper: %w", err)
	}
	cmd := exec.Command("/bin/sh", shHelperArgs(script, exe, staged, currentPID(), os.Args[1:])...)
	if err := startDetached(cmd); err != nil {
		return nil, fmt.Errorf("updater: start install helper: %w", err)
	}
	return &InstallResult{Target: exe}, nil
}

// extractLinuxExecutable writes the viff member of the portable tar.gz to dst.
func extractLinuxExecutable(archivePath, dst string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("updater: open update archive: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("updater: read update archive: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("updater: read update archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != "viff" {
			continue
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
		if err != nil {
			return fmt.Errorf("updater: create staged executable: %w", err)
		}
		if _, err := io.Copy(out, reader); err != nil {
			_ = out.Close()
			_ = os.Remove(dst)
			return fmt.Errorf("updater: extract viff: %w", err)
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(dst)
			return fmt.Errorf("updater: close staged executable: %w", err)
		}
		return nil
	}
	return fmt.Errorf("updater: update archive has no viff binary")
}

// installPlatform calls installLinux.
func (c *Client) installPlatform(u *Update, downloaded, exe string) (*InstallResult, error) {
	return c.installLinux(u, downloaded, exe)
}
