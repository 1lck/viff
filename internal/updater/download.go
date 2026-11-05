package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// downloadBufferSize is the chunk size for streaming downloads and progress
// callbacks.
const downloadBufferSize = 64 * 1024

// Download fetches u.Asset into the client's cache directory, verifies its
// SHA-256 against u.Checksums, and returns the local path. progress may be
// called from the download goroutine.
func (c *Client) Download(ctx context.Context, u *Update, progress func(done, total int64)) (string, error) {
	if u == nil {
		return "", fmt.Errorf("updater: download: no update")
	}
	if u.Asset.URL == "" {
		return "", fmt.Errorf("updater: download: asset %q has no URL", u.Asset.Name)
	}
	name := filepath.Base(u.Asset.Name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "", fmt.Errorf("updater: download: invalid asset name %q", u.Asset.Name)
	}
	expected := u.Checksums[name]
	if expected == "" {
		return "", fmt.Errorf("updater: download: no sha256 checksum for %s", name)
	}
	dir, err := c.ensureCacheDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, name)
	tmp := dst + ".part"

	resp, err := c.get(ctx, u.Asset.URL, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	total := u.Asset.Size
	if total <= 0 {
		total = resp.ContentLength
		if total < 0 {
			total = 0
		}
	}

	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("updater: create download %s: %w", tmp, err)
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(tmp)
		}
	}()

	hasher := sha256.New()
	buf := make([]byte, downloadBufferSize)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("updater: download %s canceled: %w", name, err)
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				return "", fmt.Errorf("updater: write %s: %w", tmp, err)
			}
			hasher.Write(buf[:n])
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", fmt.Errorf("updater: download %s canceled: %w", name, ctxErr)
			}
			return "", fmt.Errorf("updater: read %s: %w", name, readErr)
		}
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("updater: close download %s: %w", tmp, err)
	}
	if u.Asset.Size > 0 && written != u.Asset.Size {
		return "", fmt.Errorf("updater: size mismatch for %s: expected %d bytes, got %d", name, u.Asset.Size, written)
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if err := checkSHA256(name, expected, actual); err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	_ = os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		return "", fmt.Errorf("updater: finalize %s: %w", dst, err)
	}
	complete = true
	return dst, nil
}
