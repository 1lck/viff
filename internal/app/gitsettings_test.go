package app

import (
	"context"
	"strings"
	"testing"

	"viff/internal/gitcli"
	"viff/internal/storage"
)

// configValue reads a configuration key through the workspace's runner, so the
// test sees what Git actually received for that command.
//
// The workspace holds the backend contract now, so the concrete runner is
// recovered here: these tests assert what Git received on its command line,
// which is deliberately below the contract.
func configValue(t *testing.T, g Git, dir, key string) (string, error) {
	t.Helper()
	r, ok := g.(*gitcli.Runner)
	if !ok {
		t.Fatalf("backend is not a Git runner: %T", g)
	}
	out, err := r.Run(context.Background(), dir, "config", "--get", key)
	return strings.TrimSpace(string(out)), err
}

// A setting changed while Viff is running must apply to the next command, not
// only after a restart.
func TestApplyGitSettingsTakesEffectOnNextCommand(t *testing.T) {
	f := newFixture(t)
	if got, err := configValue(t, f.w.Git, f.dir, "credential.helper"); err == nil {
		t.Fatalf("credential.helper unexpectedly set to %q", got)
	}

	f.w.Settings.CredentialHelper = "store"
	f.w.ApplyGitSettings()
	if got, err := configValue(t, f.w.Git, f.dir, "credential.helper"); err != nil || got != "store" {
		t.Fatalf("after apply credential.helper = %q, %v", got, err)
	}

	f.w.Settings.CredentialHelper = "cache --timeout=60"
	f.w.ApplyGitSettings()
	if got, err := configValue(t, f.w.Git, f.dir, "credential.helper"); err != nil || got != "cache --timeout=60" {
		t.Fatalf("after second apply credential.helper = %q, %v", got, err)
	}
}

// Settings loaded at start-up reach the runner without any explicit apply.
func TestNewWorkspaceAppliesSavedGitSettings(t *testing.T) {
	f := newFixture(t)
	settings := storage.Defaults()
	settings.CommitSigning = true
	settings.SigningKey = "0123456789ABCDEF"
	settings.SignFormat = "ssh"
	w := NewWorkspace(&gitcli.Runner{Env: f.env}, settings, nil)

	for key, want := range map[string]string{
		"commit.gpgsign":  "true",
		"user.signingkey": "0123456789ABCDEF",
		"gpg.format":      "ssh",
	} {
		got, err := configValue(t, w.Git, f.dir, key)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", key, got, err, want)
		}
	}
}
