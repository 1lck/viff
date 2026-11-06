//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"
)

// TestSettingsEchoesResolvedGitPath checks that the Git page answers "which
// git runs?" for an empty Path to Git: the field used to say only
// "Detected automatically", which hid the binary behind every command.
func TestSettingsEchoesResolvedGitPath(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	f.click("General")
	f.s.Settle(40*time.Millisecond, time.Second)

	exe, err := f.ws.Git.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	texts := strings.Join(f.s.Texts(), " | ")
	if !strings.Contains(texts, "Using:") {
		t.Fatalf("the Git page does not echo the executable (resolved %q)\ntexts: %s", exe, texts)
	}
	// The path may be elided in the middle, so its ends are what to look for.
	if base := exe[strings.LastIndexAny(exe, `/\`)+1:]; !strings.Contains(texts, base) {
		t.Fatalf("the echo does not name %q\ntexts: %s", base, texts)
	}
	// The scope of the setting is stated where the user edits it.
	if !strings.Contains(texts, "WSL repositories always use the git inside their distribution.") {
		t.Fatalf("the WSL scope hint is missing\ntexts: %s", texts)
	}
}

// TestSettingsGitPathEchoFollowsTheField checks that a configured path takes
// over the echo: once the user names an executable, the line no longer claims
// a different one is in use.
func TestSettingsGitPathEchoFollowsTheField(t *testing.T) {
	f := newUIFixture(t)
	f.ws.Settings.GitPath = "/custom/bin/git"
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	f.click("General")
	f.s.Settle(40*time.Millisecond, time.Second)

	texts := strings.Join(f.s.Texts(), " | ")
	if strings.Contains(texts, "Using:") {
		t.Fatalf("the echo claims a detected path although one is configured\ntexts: %s", texts)
	}
	if !strings.Contains(texts, "/custom/bin/git") {
		t.Fatalf("the configured path is not shown\ntexts: %s", texts)
	}
}
