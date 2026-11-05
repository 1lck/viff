package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// checksumFileSizeLimit caps how much of a SHA256SUMS asset is read. Real files
// are a few hundred bytes; the limit keeps a hostile response bounded.
const checksumFileSizeLimit = 1 << 20

// parseChecksums parses the sha256sum format used by the release workflow: one
// "<hex>  <name>" line per asset, where name may be prefixed with "./" or use
// the "*" binary marker. Only file names are kept; a leading directory is
// dropped so lookups by asset name work for both "./name" and "name".
func parseChecksums(data []byte) (map[string]string, error) {
	sums := make(map[string]string)
	for lineNo, line := range strings.Split(string(data), "\n") {
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 {
			return nil, fmt.Errorf("SHA256SUMS line %d: expected \"<hex>  <name>\", got %q", lineNo+1, text)
		}
		sum := strings.ToLower(fields[0])
		if len(sum) != sha256.Size*2 {
			return nil, fmt.Errorf("SHA256SUMS line %d: checksum %q is not %d hex characters", lineNo+1, fields[0], sha256.Size*2)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return nil, fmt.Errorf("SHA256SUMS line %d: checksum %q is not hexadecimal: %w", lineNo+1, fields[0], err)
		}
		name := strings.TrimPrefix(fields[1], "*")
		name = strings.TrimPrefix(name, "./")
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		if name == "" {
			return nil, fmt.Errorf("SHA256SUMS line %d: missing file name", lineNo+1)
		}
		if _, exists := sums[name]; exists {
			return nil, fmt.Errorf("SHA256SUMS line %d: duplicate entry for %q", lineNo+1, name)
		}
		sums[name] = sum
	}
	if len(sums) == 0 {
		return nil, fmt.Errorf("SHA256SUMS has no entries")
	}
	return sums, nil
}

// checkSHA256 compares a computed digest with the release checksum and reports
// both values when they differ.
func checkSHA256(assetName, expected, actual string) error {
	if expected == "" {
		return fmt.Errorf("no expected sha256 for %s", assetName)
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", assetName, strings.ToLower(expected), strings.ToLower(actual))
	}
	return nil
}

// hashReader hashes everything read from r and returns the hex digest.
func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyFileSHA256 re-reads a completed download and verifies it. Download
// hashes while writing instead; this helper keeps the check available for tests
// and for callers that verify an existing file.
func verifyFileSHA256(path, assetName, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s for sha256: %w", path, err)
	}
	defer f.Close()
	actual, err := hashReader(f)
	if err != nil {
		return fmt.Errorf("hash %s: %w", path, err)
	}
	return checkSHA256(assetName, expected, actual)
}
