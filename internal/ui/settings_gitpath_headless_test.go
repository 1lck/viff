//go:build vivahead

package viffui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// TestSettingsEchoesResolvedGitPath checks that an empty Path to Git shows the
// binary the open repository's commands run. The field used to say only
// "Detected automatically", which hid the executable behind every command.
func TestSettingsEchoesResolvedGitPath(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	f.click("General")
	f.s.Settle(40*time.Millisecond, time.Second)

	exe, err := f.ws.Git.ExecutableIn(t.Context(), "")
	if err != nil {
		t.Fatalf("ExecutableIn: %v", err)
	}
	texts := strings.Join(f.s.Texts(), " | ")
	if !strings.Contains(texts, exe) {
		t.Fatalf("the Git field does not echo %q\ntexts: %s", exe, texts)
	}
}

// TestSettingsGitPathEchoFollowsTheRepository checks the echo against what the
// open repository actually runs: for a WSL repository that is the git inside
// the distribution, never the Windows default.
func TestSettingsGitPathEchoFollowsTheRepository(t *testing.T) {
	distro := wslDistroForUITest(t)
	f := newUIFixture(t)
	f.start(1440, 900)

	// A WSL repository is only used for its path here; the echo comes from the
	// probe the runner caches, not from querying the repository again.
	linuxGit, err := exec.Command("wsl.exe", "-d", distro, "--exec", "sh", "-c", "command -v git").Output()
	if err != nil {
		t.Skipf("cannot resolve the distro git: %v", err)
	}
	want := strings.TrimSpace(string(linuxGit))
	f.ws.Repo = &vapp.RepoState{Repo: domain.Repo{Root: `\\wsl.localhost\` + distro + `\home\momo\repo`}}

	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	f.click("General")
	f.s.Settle(40*time.Millisecond, time.Second)

	texts := strings.Join(f.s.Texts(), " | ")
	if !strings.Contains(texts, want) {
		t.Fatalf("the field does not echo the distro git %q\ntexts: %s", want, texts)
	}
	if strings.Contains(texts, `Program Files`) {
		t.Fatalf("the field echoes the Windows git for a WSL repository\ntexts: %s", texts)
	}
}

// TestSettingsGitPathEchoStopsAtConfiguredPath checks that a configured path
// takes over the field: no detected path is shown next to it.
func TestSettingsGitPathEchoStopsAtConfiguredPath(t *testing.T) {
	f := newUIFixture(t)
	f.ws.Settings.GitPath = "/custom/bin/git"
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	f.click("General")
	f.s.Settle(40*time.Millisecond, time.Second)

	texts := strings.Join(f.s.Texts(), " | ")
	if !strings.Contains(texts, "/custom/bin/git") {
		t.Fatalf("the configured path is not shown\ntexts: %s", texts)
	}
	if exe, err := f.ws.Git.Executable(); err == nil && exe != "/custom/bin/git" && strings.Contains(texts, exe) {
		t.Fatalf("the field also echoes the detected %q\ntexts: %s", exe, texts)
	}
}

// wslDistroForUITest returns the default distro, skipping without WSL.
func wslDistroForUITest(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("wsl.exe", "--exec", "sh", "-c", `printf %s "$WSL_DISTRO_NAME"`).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Skip("no WSL distribution available on this machine")
	}
	return strings.TrimSpace(string(out))
}
