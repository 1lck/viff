package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hex64 returns a valid 64-character digest that starts with prefix.
func hex64(prefix string) string {
	return prefix + strings.Repeat("0", 64-len(prefix))
}

func TestParseChecksums(t *testing.T) {
	data := []byte(hex64("aa") + "  ./viff-v0.1.0-darwin-arm64.dmg\n" +
		hex64("bb") + "  viff-v0.1.0-windows-amd64.zip\n" +
		"\n# comment\n" +
		hex64("cc") + "  *viff-v0.1.0-linux-amd64.tar.gz\n")
	sums, err := parseChecksums(data)
	if err != nil {
		t.Fatalf("parseChecksums: %v", err)
	}
	for name, want := range map[string]string{
		"viff-v0.1.0-darwin-arm64.dmg":   hex64("aa"),
		"viff-v0.1.0-windows-amd64.zip":  hex64("bb"),
		"viff-v0.1.0-linux-amd64.tar.gz": hex64("cc"),
	} {
		if got := sums[name]; got != want {
			t.Errorf("sums[%q] = %q, want %q", name, got, want)
		}
	}
	if len(sums) != 3 {
		t.Errorf("parsed %d entries, want 3", len(sums))
	}
}

func TestParseChecksumsRejectsBadInput(t *testing.T) {
	for _, data := range []string{
		"",
		"# only a comment\n",
		"nothex  viff.dmg\n",
		hex64("aa")[:63] + "  viff.dmg\n",
		hex64("aa") + "  a.dmg\n" + hex64("aa") + "  a.dmg\n",
		hex64("aa") + "  ./a.dmg\n" + hex64("bb") + "  same/a.dmg\n",
		hex64("aa") + "\n",
	} {
		if _, err := parseChecksums([]byte(data)); err == nil {
			t.Errorf("parseChecksums(%q) accepted invalid input", data)
		}
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.bin")
	if err := os.WriteFile(path, []byte("hello viff"), 0o600); err != nil {
		t.Fatal(err)
	}
	good := "a998aada563f03b8d86beea78c27a8f5b2b8dd0a966fe9b3ddfecfba2d882546"
	if err := verifyFileSHA256(path, "asset.bin", strings.Repeat("0", 64)); err == nil {
		t.Fatal("verifyFileSHA256 accepted a wrong checksum")
	}
	if err := verifyFileSHA256(path, "asset.bin", good); err != nil {
		t.Fatalf("verifyFileSHA256 rejected the correct checksum: %v", err)
	}
}
