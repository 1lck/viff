//go:build vivahead

package viffui

import (
	"context"
	"strings"
	"testing"
	"time"
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
