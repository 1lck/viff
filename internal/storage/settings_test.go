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

// An old settings file has no AI confirmation field; asking before a request
// leaves the machine must stay the default, while an explicit false must
// survive a round trip.
func TestAIConfirmGenerateDefaultsOnAndPersistsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(path)
	if !s.AIConfirmGenerate {
		t.Fatal("old settings did not default aiConfirmGenerate to true")
	}
	s.AIConfirmGenerate = false
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if back := Load(path); back.AIConfirmGenerate {
		t.Fatal("explicit aiConfirmGenerate=false was not persisted")
	}
}

// EditorFont and DiffFont are family names the settings page picks from the
// system's lists. A value that could not name a family must not survive a load,
// and a usable one is kept without its surrounding spaces.
func TestFontFamiliesSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	load := func(field, value string) *Settings {
		t.Helper()
		data, err := json.Marshal(map[string]any{"version": 1, field: value})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return Load(path)
	}
	family := func(s *Settings, field string) string {
		if field == "diffFont" {
			return s.DiffFont
		}
		return s.EditorFont
	}

	for _, field := range []string{"editorFont", "diffFont"} {
		if got := family(load(field, "   "), field); got != "" {
			t.Errorf("blank %s = %q, want empty", field, got)
		}
		if got := family(load(field, strings.Repeat("x", 97)), field); got != "" {
			t.Errorf("over-long %s kept: %q", field, got)
		}
		if got := family(load(field, "  Cascadia Mono  "), field); got != "Cascadia Mono" {
			t.Errorf("%s = %q, want the trimmed name", field, got)
		}
	}
}

// An unknown language falls back to the default, English, and a known one is
// kept as written.
func TestLanguageSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	load := func(value string) *Settings {
		t.Helper()
		data, err := json.Marshal(map[string]any{"version": 1, "language": value})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return Load(path)
	}

	if got := load("").Language; got != "" {
		t.Errorf("missing language = %q, want empty, which means the default", got)
	}
	if got := load("  zh-CN  ").Language; got != "zh-CN" {
		t.Errorf("language = %q, want the trimmed zh-CN", got)
	}
	if got := load("klingon").Language; got != "" {
		t.Errorf("unknown language kept: %q", got)
	}
}

// The window position survives a round trip even when it is negative, which is
// where a display left of or above the primary one puts it.
func TestWindowPositionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Load(path)
	if s.Window.Positioned {
		t.Fatalf("fresh settings claim a window position: %+v", s.Window)
	}
	s.Window.X, s.Window.Y, s.Window.Positioned = -1280, 42, true
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	back := Load(path)
	if !back.Window.Positioned || back.Window.X != -1280 || back.Window.Y != 42 {
		t.Fatalf("window position lost: %+v", back.Window)
	}
}

// (0,0) is a real place on the primary display, so Positioned rather than the
// coordinates decides whether a position was saved.
func TestWindowPositionAtOriginIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Load(path)
	s.Window.X, s.Window.Y, s.Window.Positioned = 0, 0, true
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if back := Load(path); !back.Window.Positioned {
		t.Fatalf("position at the origin was dropped: %+v", back.Window)
	}
}

// An old settings file has no skip field, so updating asks as before; an
// explicit choice is kept together with the update method it applies.
func TestSkipUpdateDialogPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"pullMode":"rebase"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(path)
	if s.SkipUpdateDialog {
		t.Fatal("old settings already skipped the update dialog")
	}
	s.SkipUpdateDialog = true
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if back := Load(path); !back.SkipUpdateDialog || back.PullMode != "rebase" {
		t.Fatalf("skipUpdateDialog was not kept: %+v", back)
	}
}

// The search backend follows the same rule as the language: an old file has
// none of the fields, and an unknown value means auto rather than a failure.
func TestSearchSettingsDefaultsAndSanitize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if s := Load(path); s.SearchTool != "auto" || s.RipgrepPath != "" {
		t.Fatalf("defaults = %+v", s)
	}

	s := Load(path)
	s.SearchTool = "  rg  "
	s.RipgrepPath = "  C:\\tools\\rg.exe  "
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	back := Load(path)
	if back.SearchTool != "rg" || back.RipgrepPath != `C:\tools\rg.exe` {
		t.Fatalf("round trip = %+v", back)
	}

	load := func(tool string) *Settings {
		t.Helper()
		data := []byte(`{"version":1,"searchTool":"` + tool + `"}`)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return Load(path)
	}
	if got := load("klingon").SearchTool; got != "auto" {
		t.Errorf("unknown tool = %q, want auto", got)
	}
	if got := load("git").SearchTool; got != "git" {
		t.Errorf("git = %q", got)
	}
}
