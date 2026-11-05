package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Load(path) // missing file: defaults
	if s.SSHCommand != "" || s.GPGProgram != "" || s.SigningKey != "" || s.SignFormat != "" || s.CommitSigning || s.CredentialHelper != "" {
		t.Fatalf("new settings should leave Git settings empty: %+v", s)
	}
	s.SSHCommand = "/usr/bin/ssh -i ~/.ssh/id_ed25519"
	s.GPGProgram = "/opt/homebrew/bin/gpg"
	s.SigningKey = "0123456789ABCDEF"
	s.SignFormat = "ssh"
	s.CommitSigning = true
	s.CredentialHelper = "osxkeychain"
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	back := Load(path)
	if back.Warning != "" {
		t.Fatalf("unexpected warning: %s", back.Warning)
	}
	if back.SSHCommand != s.SSHCommand || back.GPGProgram != s.GPGProgram ||
		back.SigningKey != s.SigningKey || back.SignFormat != s.SignFormat ||
		back.CommitSigning != s.CommitSigning || back.CredentialHelper != s.CredentialHelper {
		t.Fatalf("round trip lost values:\n got %+v\nwant %+v", back, s)
	}
}

// An old settings file has none of the Git fields. It must load with empty
// values and keep everything else, so upgrading never changes behaviour.
func TestSettingsFileWithoutGitFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	old := `{"version":1,"theme":"light","fontSize":14,"pullMode":"rebase","autoFetchMins":5}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(path)
	if s.Warning != "" {
		t.Fatalf("unexpected warning: %s", s.Warning)
	}
	if s.Theme != "light" || s.FontSize != 14 || s.PullMode != "rebase" {
		t.Fatalf("old values lost: %+v", s)
	}
	if s.SSHCommand != "" || s.GPGProgram != "" || s.SigningKey != "" || s.SignFormat != "" || s.CommitSigning || s.CredentialHelper != "" {
		t.Fatalf("Git settings should default to empty: %+v", s)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	if probe["theme"] != "light" || probe["pullMode"] != "rebase" {
		t.Fatalf("saved file lost old values: %s", raw)
	}
	if _, ok := probe["sshCommand"]; ok {
		t.Fatalf("empty Git settings should be omitted: %s", raw)
	}
}

func TestSignFormatSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"signFormat":"gpg"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Load(path).SignFormat; got != "" {
		t.Fatalf("unknown signFormat = %q, want empty", got)
	}
}

// An old settings file has no update field; the default must keep automatic
// checks on, while an explicit false must survive a round trip.
func TestAutoUpdateCheckDefaultsOnAndPersistsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(path)
	if !s.AutoUpdateCheck {
		t.Fatal("old settings did not default autoUpdateCheck to true")
	}
	s.AutoUpdateCheck = false
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if back := Load(path); back.AutoUpdateCheck {
		t.Fatal("explicit autoUpdateCheck=false was not persisted")
	}
}

// EditorFont is the family the settings page picked from the system's list. A
// value that could not name a family must not survive a load, and a usable one
// is kept without its surrounding spaces.
func TestEditorFontSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write := func(editorFont string) {
		t.Helper()
		data, err := json.Marshal(map[string]any{"version": 1, "editorFont": editorFont})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("   ")
	if got := Load(path).EditorFont; got != "" {
		t.Fatalf("blank editorFont = %q, want empty", got)
	}

	write(strings.Repeat("x", 97))
	if got := Load(path).EditorFont; got != "" {
		t.Fatalf("over-long editorFont kept: %q", got)
	}

	write("  Cascadia Mono  ")
	if got := Load(path).EditorFont; got != "Cascadia Mono" {
		t.Fatalf("editorFont = %q, want the trimmed name", got)
	}
}
