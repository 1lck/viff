package gitcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// config.helper can be read back with git config, so the test sees exactly
// what Git received on the command line.
func TestConfigIsInjected(t *testing.T) {
	tr := newTestRepo(t)
	tr.r.SetConfig(Config{
		GPGProgram:       "/opt/homebrew/bin/gpg",
		SigningKey:       "0123456789ABCDEF",
		SignFormat:       "ssh",
		CommitSigning:    true,
		CredentialHelper: "store",
	})
	for key, want := range map[string]string{
		"gpg.program":       "/opt/homebrew/bin/gpg",
		"user.signingkey":   "0123456789ABCDEF",
		"gpg.format":        "ssh",
		"commit.gpgsign":    "true",
		"credential.helper": "store",
	} {
		out, err := tr.r.Run(context.Background(), tr.dir, "config", "--get", key)
		if err != nil {
			t.Fatalf("config --get %s: %v", key, err)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// A new value must reach the very next command; nothing is cached from the
// previous setting.
func TestConfigChangeAppliesToNextCommand(t *testing.T) {
	tr := newTestRepo(t)
	tr.r.SetConfig(Config{CredentialHelper: "store"})
	if got := tr.credentialHelper(t); got != "store" {
		t.Fatalf("credential.helper = %q, want store", got)
	}
	tr.r.SetConfig(Config{CredentialHelper: "cache --timeout=60"})
	if got := tr.credentialHelper(t); got != "cache --timeout=60" {
		t.Fatalf("credential.helper after change = %q", got)
	}
	tr.r.SetConfig(Config{})
	if got := tr.credentialHelper(t); got != "" {
		t.Fatalf("credential.helper after clearing = %q, want empty", got)
	}
}

func (tr *testRepo) credentialHelper(t *testing.T) string {
	t.Helper()
	out, err := tr.r.Run(context.Background(), tr.dir, "config", "--get", "credential.helper")
	if err != nil {
		if KindOf(err) == ErrNotFound || strings.Contains(err.Error(), "exit status 1") {
			return ""
		}
		t.Fatalf("config --get credential.helper: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// Empty settings must not override the repository or the user's config.
func TestEmptyConfigLeavesGitConfigAlone(t *testing.T) {
	tr := newTestRepo(t) // sets commit.gpgsign=false in the repository
	if got := tr.gitConfig(t, "commit.gpgsign"); got != "false" {
		t.Fatalf("repository commit.gpgsign = %q, want false", got)
	}
	tr.r.SetConfig(Config{CommitSigning: true})
	if got := tr.gitConfig(t, "commit.gpgsign"); got != "true" {
		t.Fatalf("with signing enabled commit.gpgsign = %q, want true", got)
	}
}

func (tr *testRepo) gitConfig(t *testing.T, key string) string {
	t.Helper()
	out, err := tr.r.Run(context.Background(), tr.dir, "config", "--get", key)
	if err != nil {
		t.Fatalf("config --get %s: %v", key, err)
	}
	return strings.TrimSpace(string(out))
}

// The SSH command cannot be read back with git config; a stub Git that prints
// its environment proves the variable reaches the child process.
func TestSSHCommandEnvironment(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "git-stub")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$GIT_SSH_COMMAND\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Path: script}
	r.SetConfig(Config{SSHCommand: "/usr/bin/ssh -i /tmp/viff-key"})
	out, err := r.Run(context.Background(), "", "version")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "/usr/bin/ssh -i /tmp/viff-key" {
		t.Fatalf("GIT_SSH_COMMAND = %q", got)
	}
	r.SetConfig(Config{})
	out, err = r.Run(context.Background(), "", "version")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "" {
		t.Fatalf("GIT_SSH_COMMAND with empty setting = %q, want empty", got)
	}
}

// Changing the Git executable in settings must use the new binary, not the
// path resolved for the first command.
func TestSetPathTakesEffect(t *testing.T) {
	dir := t.TempDir()
	makeStub := func(name, out string) string {
		p := filepath.Join(dir, name)
		body := "#!/bin/sh\nprintf '" + out + "'\n"
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	first, second := makeStub("git-one", "one"), makeStub("git-two", "two")
	r := &Runner{Path: first}
	out, err := r.Run(context.Background(), "", "version")
	if err != nil || string(out) != "one" {
		t.Fatalf("first run = %q, %v", out, err)
	}
	r.SetPath(second)
	out, err = r.Run(context.Background(), "", "version")
	if err != nil || string(out) != "two" {
		t.Fatalf("run after SetPath = %q, %v (cached path not reset)", out, err)
	}
}
