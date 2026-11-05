//go:build vivahead

package viffui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

// TestGitSettingsDialog edits the Git page and checks that the values are
// stored and that the next Git command already sees them.
func TestGitSettingsDialog(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	f.click("Git") // the second page of the dialog
	f.s.Settle(40*time.Millisecond, time.Second)

	f.click("SSH command:")
	if err := f.s.Type("/usr/bin/ssh -i /tmp/viff-key"); err != nil {
		t.Fatal(err)
	}
	f.click("SSH") // signing format radio button
	f.click("Sign commits by default")
	f.click("Credential helper:")
	if err := f.s.Type("store"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.snap("git-settings-before-save")
	f.click("OK")
	f.s.Settle(60*time.Millisecond, time.Second)

	s := f.ws.Settings
	if got, want := s.SSHCommand, "/usr/bin/ssh -i /tmp/viff-key"; got != want {
		t.Errorf("SSHCommand = %q, want %q", got, want)
	}
	if s.SignFormat != "ssh" {
		t.Errorf("SignFormat = %q, want ssh", s.SignFormat)
	}
	if !s.CommitSigning {
		t.Error("CommitSigning was not saved")
	}
	if got, want := s.CredentialHelper, "store"; got != want {
		t.Errorf("CredentialHelper = %q, want %q", got, want)
	}

	// Saved settings must reach the runner without a restart.
	if got := runGit(t, f, "config", "--get", "credential.helper"); got != "store" {
		t.Errorf("credential.helper seen by Git = %q, want store", got)
	}
	if got := runGit(t, f, "config", "--get", "commit.gpgsign"); got != "true" {
		t.Errorf("commit.gpgsign seen by Git = %q, want true", got)
	}
}

// A command that does not exist is pointed out under the field, but the
// dialog still saves: the program may only exist in the user's shell.
func TestGitSettingsWarnsAboutMissingCommand(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	f.click("Git")
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("GPG program:")
	if err := f.s.Type("/no/such/gpg"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	if !f.s.HasText("Not found: /no/such/gpg") {
		t.Fatalf("missing command was not reported; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("git-settings-before-save")
	f.click("OK")
	if got := f.ws.Settings.GPGProgram; got != "/no/such/gpg" {
		t.Fatalf("GPGProgram = %q, want the typed value", got)
	}
}

func runGit(t *testing.T, f *uiFixture, args ...string) string {
	t.Helper()
	out, err := f.ws.Git.Run(context.Background(), f.dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// The General page ends with the interface font picker.
func TestInterfaceFontSectionShown(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	if !f.s.HasText("Fonts") || !f.s.HasText("Interface font:") || !f.s.HasText("Diff font:") {
		t.Fatalf("the font section is missing; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
}

// A family the system does not have must fall back quietly instead of breaking
// the next paint, and clearing the setting hands the face back to the platform.
func TestInterfaceFontFallsBack(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)

	f.ws.Settings.EditorFont = "Viff Missing Family"
	f.win.applyFont()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, time.Second)
	if got := f.ws.Settings.EditorFont; got != "Viff Missing Family" {
		t.Fatalf("EditorFont = %q, want the chosen value", got)
	}

	f.ws.Settings.EditorFont = ""
	f.win.applyFont()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, time.Second)
	if f.ws.Settings.EditorFont != "" {
		t.Fatal("EditorFont should be empty after clearing")
	}
	f.assertIdle()
}

// Picking a family in the dropdown stores it. The row is clicked by coordinate
// because a popup is drawn on its own layer: the label lookup that reaches form
// controls does not reach a row of an open menu.
func TestInterfaceFontSelection(t *testing.T) {
	families := app.FontFamilies()
	if len(families) == 0 {
		t.Skip("the platform reports no font families")
	}
	target := families[0]

	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	f.click("Default (system interface font)")
	f.s.Settle(40*time.Millisecond, time.Second)

	var row ui.Rect
	found := false
	for _, hit := range f.s.Snapshot().Hits {
		if hit.Label == target {
			row, found = hit.Bounds, true
		}
	}
	if !found {
		t.Fatalf("the dropdown has no row for %q", target)
	}
	if err := f.s.Click(row.X+row.Width/2, row.Y+row.Height/2); err != nil {
		t.Fatalf("click %q: %v", target, err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)

	f.click("OK")
	f.s.Settle(60*time.Millisecond, time.Second)
	if got := f.ws.Settings.EditorFont; got != target {
		t.Fatalf("EditorFont = %q, want %q", got, target)
	}
	f.assertIdle()
}

// Picking a fixed-pitch family for diffs stores it and leaves the interface font
// alone; the two settings are independent.
func TestDiffFontSelection(t *testing.T) {
	mono := app.MonospaceFamilies()
	if len(mono) == 0 {
		t.Skip("the platform reports no fixed-pitch families")
	}
	target := mono[0]

	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	f.click("Default (bundled or system fixed-pitch)")
	f.s.Settle(40*time.Millisecond, time.Second)

	var row ui.Rect
	found := false
	for _, hit := range f.s.Snapshot().Hits {
		if hit.Label == target {
			row, found = hit.Bounds, true
		}
	}
	if !found {
		t.Fatalf("the diff font menu has no row for %q", target)
	}
	if err := f.s.Click(row.X+row.Width/2, row.Y+row.Height/2); err != nil {
		t.Fatalf("click %q: %v", target, err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)

	f.click("OK")
	f.s.Settle(60*time.Millisecond, time.Second)
	if got := f.ws.Settings.DiffFont; got != target {
		t.Fatalf("DiffFont = %q, want %q", got, target)
	}
	if got := f.ws.Settings.EditorFont; got != "" {
		t.Errorf("EditorFont = %q, want empty: the diff choice must not touch it", got)
	}
	f.assertIdle()
}
