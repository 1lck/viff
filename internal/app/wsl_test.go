package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/wslpath"
)

// TestCanonicalPathWSLIdentity pins the dedup identity of a WSL repository:
// both UNC aliases and any distro-name casing name one row, and the path below
// the distro keeps its case because the Linux file system is case-sensitive.
func TestCanonicalPathWSLIdentity(t *testing.T) {
	cases := []string{
		`\\wsl.localhost\Ubuntu\home\momo\repo`,
		`\\wsl$\Ubuntu\home\momo\repo`,
		`\\wsl$\ubuntu\home\momo\repo`,
		`//wsl.localhost/ubuntu/home/momo/repo`,
	}
	want := canonicalPath(cases[0])
	if !wslpath.Is(want) {
		t.Fatalf("canonicalPath(%q) = %q, not a WSL path", cases[0], want)
	}
	for _, c := range cases[1:] {
		if got := canonicalPath(c); got != want {
			t.Errorf("canonicalPath(%q) = %q, want %q", c, got, want)
		}
	}
	for _, c := range cases {
		if got := entryKey(c); got != want {
			t.Errorf("entryKey(%q) = %q, want %q", c, got, want)
		}
	}
	// Case below the distro distinguishes two real directories.
	a := canonicalPath(`\\wsl.localhost\Ubuntu\home\repo`)
	b := canonicalPath(`\\wsl.localhost\Ubuntu\home\Repo`)
	if a == b {
		t.Errorf("canonicalPath collapsed a case-sensitive Linux path: %q", a)
	}
}

// wslDistroForTest returns the default distro, skipping on machines without
// WSL (every CI runner).
func wslDistroForTest(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("wsl.exe", "--exec", "sh", "-c", `printf %s "$WSL_DISTRO_NAME"`).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Skip("no WSL distribution available on this machine")
	}
	return strings.TrimSpace(string(out))
}

// TestAddRepositoryDedupsWSLAliases adds the same WSL repository twice under
// its two UNC spellings. The workspace must keep one row: the aliases name one
// file system, and a second row would let two write locks guard one index.
func TestAddRepositoryDedupsWSLAliases(t *testing.T) {
	distro := wslDistroForTest(t)
	linuxDir, err := exec.Command("wsl.exe", "-d", distro, "--exec", "sh", "-c", "mktemp -d -p /tmp viff-app-test.XXXXXX").Output()
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	dir := strings.TrimSpace(string(linuxDir))
	t.Cleanup(func() {
		exec.Command("wsl.exe", "-d", distro, "--exec", "rm", "-rf", dir).Run()
	})
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "Viff Test"}, {"config", "user.email", "test@example.com"}} {
		cmd := exec.Command("wsl.exe", "-d", distro, "--cd", dir, "--exec", "git")
		cmd.Args = append(cmd.Args, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	f := newFixture(t)
	modern := wslpath.FromLinux(distro, dir)
	legacy := `\\wsl$\` + distro + strings.ReplaceAll(dir, "/", `\`)

	add := func(p string) error {
		var addErr error
		ran := false
		f.w.AddRepository(p, func(e error) { addErr, ran = e, true })
		f.settle(func() bool { return ran })
		return addErr
	}
	if err := add(modern); err != nil {
		t.Fatalf("AddRepository(%q): %v", modern, err)
	}
	if err := add(legacy); err != nil {
		t.Fatalf("AddRepository(%q): %v", legacy, err)
	}
	if n := len(f.w.Repositories); n != 1 {
		t.Fatalf("workspace holds %d rows for one repository: %+v", n, f.w.Repositories)
	}
	root := f.w.Repositories[0].Root
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("the row's root is not readable through the Windows path: %v", err)
	}
	repo, err := f.w.Git.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("discover the WSL row: %v", err)
	}
	if _, err := f.w.Git.Status(context.Background(), repo); err != nil {
		t.Fatalf("status of the WSL row: %v", err)
	}
}
