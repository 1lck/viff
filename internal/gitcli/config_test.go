package gitcli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
//
// The stub is this test binary itself: TestMain notices VIFF_GIT_STUB before
// the testing flags are parsed and exits with the output the test expects.
// A shell script with a "#!/bin/sh" line would work on Unix but not on
// Windows, where a script cannot be executed directly.
func TestSSHCommandEnvironment(t *testing.T) {
	stub, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{Path: stub, Env: []string{"VIFF_GIT_STUB=env:GIT_SSH_COMMAND"}}
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
	// Two copies of the stub report different output, based on their file
	// name, so a cached first path is observable.
	first, second := testBinaryCopy(t, dir, "git-one"), testBinaryCopy(t, dir, "git-two")
	r := &Runner{Path: first, Env: []string{"VIFF_GIT_STUB=name"}}
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

// TestMain lets the test binary double as the fake Git executable the Runner
// starts. VIFF_GIT_STUB is set by the tests above and inherited by the child;
// it is unset in a normal `go test` run, where the tests run as usual.
func TestMain(m *testing.M) {
	if mode := os.Getenv("VIFF_GIT_STUB"); mode != "" {
		writeStubOutput(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// writeStubOutput is the fake Git. "env:NAME" prints the value of the
// environment variable NAME, which is how a setting that Git passes through
// the environment is observed. "name" derives the output from the name of the
// stub executable, so TestSetPathTakesEffect can tell the two binaries apart.
func writeStubOutput(mode string) {
	if name, ok := strings.CutPrefix(mode, "env:"); ok {
		os.Stdout.WriteString(os.Getenv(name))
		return
	}
	exe, err := os.Executable()
	if err == nil && strings.Contains(filepath.Base(exe), "two") {
		os.Stdout.WriteString("two")
		return
	}
	os.Stdout.WriteString("one")
}

// testBinaryCopy copies the running test executable into dir as name (with the
// platform's executable extension), so it can serve as a fake Git binary on
// platforms where shell scripts are not executable.
func testBinaryCopy(t *testing.T, dir, name string) string {
	t.Helper()
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return dst
}
