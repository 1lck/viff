//go:build windows

package gitcli

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
	"viff/internal/wslpath"
)

// utf16le renders s as the byte stream wsl.exe writes to a pipe: little-endian
// UTF-16. withBOM adds the byte-order mark some versions emit.
func utf16le(s string, withBOM bool) string {
	var b []byte
	if withBOM {
		b = []byte{0xFF, 0xFE}
	}
	for _, r := range s {
		var buf [2]byte
		binary.LittleEndian.PutUint16(buf[:], uint16(r))
		b = append(b, buf[:]...)
	}
	return string(b)
}

func TestDecodeWSLText(t *testing.T) {
	diag := "不存在具有所提供名称的分发。\r\n错误代码： Wsl/Service/WSL_E_DISTRO_NOT_FOUND\r\n"
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"utf16 with BOM", utf16le(diag, true), diag},
		{"utf16 without BOM (measured pipe output)", utf16le(diag, false), diag},
		{"git output passes through", "git version 2.53.0\n", "git version 2.53.0\n"},
		{"utf8 diagnostics pass through", "fatal: not a git repository\n", "fatal: not a git repository\n"},
		{"empty", "", ""},
		{"lone marker byte is not a BOM", "\xff", "\xff"},
	}
	for _, c := range cases {
		if got := decodeWSLText(c.in); got != c.want {
			t.Errorf("%s: decodeWSLText = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestClassifyWSLError pins the locale-independent markers: the same failures
// must classify the same way on a Chinese, German or English Windows.
func TestClassifyWSLError(t *testing.T) {
	cases := []struct {
		stderr string
		want   ErrorKind
	}{
		{"������ Wsl/Service/WSL_E_DISTRO_NOT_FOUND\r\n", ErrNoDistro},
		{"Error code: Wsl/Service/WSL_E_WSL_NOT_INSTALLED\r\n", ErrNoWSL},
		{"<3>WSL (10 - Relay) ERROR: CreateProcessCommon:875: execvpe(git) failed: No such file or directory", ErrNoWSLGit},
		{"fatal: not a git repository (or any of the parent directories): .git", ErrOther},
		{"", ErrOther},
	}
	for _, c := range cases {
		if got := classifyWSLError(c.stderr); got != c.want {
			t.Errorf("classifyWSLError(%q) = %v, want %v", c.stderr, got, c.want)
		}
	}
}

func TestWSLEnvForward(t *testing.T) {
	got := wslEnvForward("")
	for _, want := range []string{"GIT_TERMINAL_PROMPT/u", "GIT_OPTIONAL_LOCKS/u", "LC_ALL/u", "LANGUAGE/u", "GIT_SSH_COMMAND/u"} {
		if !strings.Contains(got, want) {
			t.Errorf("wslEnvForward(\"\") = %q, missing %q", got, want)
		}
	}
	// A name the user already forwards keeps its flags and is not repeated.
	got = wslEnvForward("LC_ALL/p:MY_VAR")
	if strings.Contains(got, "LC_ALL/u") {
		t.Errorf("wslEnvForward overwrote the user's LC_ALL flags: %q", got)
	}
	if !strings.Contains(got, "MY_VAR") {
		t.Errorf("wslEnvForward dropped the user's variable: %q", got)
	}
	if n := strings.Count(got, "LC_ALL"); n != 1 {
		t.Errorf("wslEnvForward repeated LC_ALL %d times: %q", n, got)
	}
}

// ---- real WSL ------------------------------------------------------------

// wslTestDistro returns the default distro when this machine can run one, and
// skips the test otherwise. CI runners have no WSL, so these tests are opt-in
// by environment rather than by build tag: they run on a developer's Windows
// machine exactly as the app would.
func wslTestDistro(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("wsl.exe", "--exec", "sh", "-c", `printf %s "$WSL_DISTRO_NAME"`).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Skip("no WSL distribution available on this machine")
	}
	return strings.TrimSpace(string(out))
}

// wslRun runs a command inside distro and fails the test on error.
func wslRun(t *testing.T, distro, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-d", distro}, wslDirArgs(dir)...)
	full = append(full, "--exec")
	full = append(full, args...)
	out, err := exec.Command("wsl.exe", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("wsl %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func wslDirArgs(dir string) []string {
	if dir == "" {
		return nil
	}
	return []string{"--cd", dir}
}

// newWSLTestRepo creates a repository inside distro and returns the Runner and
// the repository as Viff sees it: a \\wsl.localhost path plus the Linux path
// Git reported.
func newWSLTestRepo(t *testing.T, distro string) (*Runner, domain.Repo, string) {
	t.Helper()
	linuxDir := wslRun(t, distro, "", "sh", "-c", "mktemp -d -p /tmp viff-wsl-test.XXXXXX")
	t.Cleanup(func() {
		exec.Command("wsl.exe", "-d", distro, "--exec", "rm", "-rf", linuxDir).Run()
	})
	wslRun(t, distro, linuxDir, "git", "init", "-q", "-b", "main")
	wslRun(t, distro, linuxDir, "git", "config", "user.name", "Viff Test")
	wslRun(t, distro, linuxDir, "git", "config", "user.email", "test@example.com")
	wslRun(t, distro, linuxDir, "git", "config", "commit.gpgsign", "false")

	unc, ok := wslpath.FromLinux(distro, linuxDir), true
	if !wslpath.Is(unc) {
		t.Fatalf("FromLinux(%q, %q) = %q, not a WSL path", distro, linuxDir, unc)
	}
	_ = ok
	r := &Runner{}
	repo, err := r.Discover(context.Background(), unc)
	if err != nil {
		t.Fatalf("Discover(%q): %v", unc, err)
	}
	return r, repo, linuxDir
}

// TestWSLRepositoryRunsDistroGit is the end-to-end check: discovery, file
// reads over 9P, status, staging and commit all work on a real distro
// repository while every path the app sees stays a Windows UNC path.
func TestWSLRepositoryRunsDistroGit(t *testing.T) {
	distro := wslTestDistro(t)
	r, repo, linuxDir := newWSLTestRepo(t, distro)
	ctx := context.Background()

	// Identity: every path is the Windows spelling, and the Linux path Git
	// reported is gone from it.
	if !wslpath.Is(repo.Root) || !wslpath.Is(repo.GitDir) || !wslpath.Is(repo.CommonDir) {
		t.Fatalf("discovery left Linux paths in the identity: %+v", repo)
	}
	if strings.Contains(repo.Root, "/tmp") || strings.HasPrefix(repo.Root, "/") {
		t.Fatalf("root %q is not the UNC spelling", repo.Root)
	}
	if _, d, _ := wslpath.Split(repo.Root); d != linuxDir {
		t.Fatalf("root %q does not name the distro directory %q", repo.Root, linuxDir)
	}

	// File IO goes through the 9P redirector: the app writes and reads with
	// os.* on the UNC path, exactly as it does on a local disk.
	if err := os.WriteFile(filepath.Join(repo.Root, "hello.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write through the UNC path: %v", err)
	}
	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(st.Entries) != 1 || st.Entries[0].Path != "hello.txt" {
		t.Fatalf("status entries = %+v, want the untracked hello.txt", st.Entries)
	}

	// A write operation runs the distro's git and then refreshes.
	if _, err := r.Run(ctx, repo.Root, "add", "hello.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := r.Run(ctx, repo.Root, "commit", "-q", "-m", "first"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	st, err = r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("Status after commit: %v", err)
	}
	if len(st.Entries) != 0 || st.Branch.Head == "" {
		t.Fatalf("after commit: entries=%+v head=%q", st.Entries, st.Branch.Head)
	}
}

// TestWSLTwoAliasesNameOneRepository pins the dedup identity: the legacy
// \\wsl$ spelling and the current \\wsl.localhost spelling resolve to the same
// root, so the workspace list cannot hold one repository twice.
func TestWSLTwoAliasesNameOneRepository(t *testing.T) {
	distro := wslTestDistro(t)
	r, repo, linuxDir := newWSLTestRepo(t, distro)

	legacy := `\\wsl$\` + distro + strings.ReplaceAll(linuxDir, "/", `\`)
	other, err := r.Discover(context.Background(), legacy)
	if err != nil {
		t.Fatalf("Discover(%q): %v", legacy, err)
	}
	if other.Root != repo.Root || other.CommonDir != repo.CommonDir {
		t.Fatalf("aliases diverge:\n  %+v\n  %+v", repo, other)
	}
}

// TestWSLWorktreeOnWindowsDrive checks the one crossing that goes the other
// way: a worktree created on the Windows file system from a repository in the
// distro. Git receives /mnt/<drive>/... and reports the Linux spelling back,
// which has to become a drive path again for the app.
func TestWSLWorktreeOnWindowsDrive(t *testing.T) {
	distro := wslTestDistro(t)
	r, repo, _ := newWSLTestRepo(t, distro)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(repo.Root, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, repo.Root, "add", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, repo.Root, "commit", "-q", "-m", "first"); err != nil {
		t.Fatal(err)
	}

	// A Windows temp directory on the same drive as the test binary.
	base := t.TempDir()
	wtPath := filepath.Join(base, "wt-side")
	if err := r.AddWorktree(ctx, repo, wtPath, "side", "main"); err != nil {
		t.Fatalf("AddWorktree(%q): %v", wtPath, err)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "a.txt")); err != nil {
		t.Fatalf("worktree on the Windows drive is not readable: %v", err)
	}

	list, err := r.Worktrees(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, wt := range list {
		if strings.EqualFold(filepath.Clean(wt.Path), filepath.Clean(wtPath)) {
			found = true
		}
		if wslpath.Is(wt.Path) && strings.Contains(wt.Path, "/") {
			t.Fatalf("worktree path %q kept a Linux spelling", wt.Path)
		}
	}
	if !found {
		t.Fatalf("worktree list %+v does not name %q", list, wtPath)
	}
}

// TestWSLMissingDistroIsClassified checks the failure path the UI turns into a
// message: a path naming a distro that does not exist fails with a stable code
// instead of a raw wsl.exe error.
func TestWSLMissingDistroIsClassified(t *testing.T) {
	wslTestDistro(t) // skip where WSL itself is unavailable
	r := &Runner{}
	_, err := r.Discover(context.Background(), `\\wsl.localhost\viff-no-such-distro\home\you\repo`)
	if err == nil {
		t.Fatal("discovery in a missing distro succeeded")
	}
	var ge *Error
	if !errors.As(err, &ge) || ge.Kind != ErrNoDistro {
		t.Fatalf("kind = %v (err %v), want ErrNoDistro", KindOf(err), err)
	}
	if got := CodeOf(err); got != "wsl.distro_missing" {
		t.Errorf("code = %q", got)
	}
}
