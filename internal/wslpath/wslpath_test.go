package wslpath

import (
	"errors"
	"testing"
)

func TestIs(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{`\\wsl.localhost\Ubuntu\home\momo\repo`, true},
		{`\\wsl$\Ubuntu\home\momo\repo`, true},
		{`\\WSL$\Ubuntu\repo`, true},          // alias case-insensitive
		{`//wsl.localhost/Ubuntu/repo`, true}, // forward slashes
		{`\\wsl.localhost\Ubuntu`, true},
		// The extended-length and device prefixes Windows tools add, and the
		// doubled separators a join can produce, name the same repository.
		{`\\?\UNC\wsl.localhost\Ubuntu\home\momo\repo`, true},
		{`\\?\UNC\wsl$\Ubuntu\repo`, true},
		{`\\wsl.localhost\\Ubuntu\home\momo\repo`, true},
		{`\\wsl.localhost\Ubuntu\`, true},
		{`C:\Users\momo\repo`, false},
		{`\\server\share\repo`, false},
		{`\\wsl.localhost\`, false}, // no distro
		{`\\wsl$`, false},           // no distro
		{`\\?\UNC\wsl.localhost\`, false},
		{`/home/momo/repo`, false},
		{``, false},
	}
	for _, c := range cases {
		if got := Is(c.path); got != c.want {
			t.Errorf("Is(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{`\\wsl.localhost\Ubuntu\home\momo\repo`, `\\wsl.localhost\ubuntu\home\momo\repo`},
		{`\\wsl$\Ubuntu\home\momo\repo`, `\\wsl.localhost\ubuntu\home\momo\repo`},
		{`\\WSL.localhost\Ubuntu`, `\\wsl.localhost\ubuntu`},
		{`//wsl$/Ubuntu/repo/`, `\\wsl.localhost\ubuntu\repo`},
		{`\\wsl.localhost\Ubuntu\repo\..\other`, `\\wsl.localhost\ubuntu\other`},
		{`\\wsl.localhost\Ubuntu\`, `\\wsl.localhost\ubuntu`},
		{`\\wsl.localhost\Ubuntu\Repo`, `\\wsl.localhost\ubuntu\Repo`}, // path case preserved
		// Every spelling of one repository normalizes to one identity, which is
		// what keeps the workspace list and the write lock from splitting it.
		{`\\?\UNC\wsl.localhost\Ubuntu\home\repo`, `\\wsl.localhost\ubuntu\home\repo`},
		{`\\wsl.localhost\\Ubuntu\home\repo`, `\\wsl.localhost\ubuntu\home\repo`},
	}
	for _, c := range cases {
		got, ok := Normalize(c.path)
		if !ok {
			t.Errorf("Normalize(%q) not recognized", c.path)
			continue
		}
		if got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.path, got, c.want)
		}
	}
	// Both aliases of one repository normalize to the same identity: the
	// workspace list dedup depends on it.
	a, _ := Normalize(`\\wsl.localhost\Ubuntu\home\repo`)
	b, _ := Normalize(`\\wsl$\ubuntu\home\repo`)
	if a != b {
		t.Errorf("aliases diverge: %q vs %q", a, b)
	}
	for _, p := range []string{`C:\repo`, `\\server\share`, `/home/repo`, ``} {
		if got, ok := Normalize(p); ok {
			t.Errorf("Normalize(%q) = %q, want not-ok", p, got)
		}
	}
}

func TestSplit(t *testing.T) {
	distro, linux, ok := Split(`\\wsl$\Ubuntu\home\momo\my repo`)
	if !ok || distro != "ubuntu" || linux != "/home/momo/my repo" {
		t.Errorf("Split = %q, %q, %v", distro, linux, ok)
	}
	distro, linux, ok = Split(`\\wsl.localhost\Ubuntu`)
	if !ok || distro != "ubuntu" || linux != "/" {
		t.Errorf("Split root = %q, %q, %v", distro, linux, ok)
	}
	if _, _, ok := Split(`C:\repo`); ok {
		t.Error("Split accepted a drive path")
	}
}

func TestFromLinux(t *testing.T) {
	cases := []struct {
		distro, linuxPath, want string
	}{
		{"ubuntu", "/home/momo/repo", `\\wsl.localhost\ubuntu\home\momo\repo`},
		{"ubuntu", "/", `\\wsl.localhost\ubuntu`},
		{"Ubuntu", "/home/中文 目录/repo", `\\wsl.localhost\ubuntu\home\中文 目录\repo`},
		{"ubuntu", "/mnt/c/Users/momo/repo", `C:\Users\momo\repo`},
		{"ubuntu", "/mnt/c/repo", `C:\repo`},
		{"ubuntu", "/mnt/d/data", `D:\data`},
		// A distro directory named like a drive letter is not a mount.
		{"ubuntu", "/mnt/repo", `\\wsl.localhost\ubuntu\mnt\repo`},
		// A trailing space is part of the name on a Linux file system; only the
		// line breaks command output adds are stripped.
		{"ubuntu", "/home/momo/repo \n", `\\wsl.localhost\ubuntu\home\momo\repo `},
		// Without a distro the UNC spelling would name no distribution at all.
		{"", "/home/momo/repo", ""},
		{"  ", "/home/momo/repo", ""},
		{"ubuntu", "relative/path", ""},
		{"ubuntu", "", ""},
	}
	for _, c := range cases {
		if got := FromLinux(c.distro, c.linuxPath); got != c.want {
			t.Errorf("FromLinux(%q, %q) = %q, want %q", c.distro, c.linuxPath, got, c.want)
		}
	}
}

func TestToLinuxArg(t *testing.T) {
	cases := []struct {
		distro, path, want string
	}{
		// Same distro UNC -> Linux path for execution.
		{"ubuntu", `\\wsl.localhost\Ubuntu\home\momo\repo`, "/home/momo/repo"},
		{"ubuntu", `\\wsl$\ubuntu\repo`, "/repo"},
		{"ubuntu", `\\?\UNC\wsl.localhost\Ubuntu\repo`, "/repo"},
		// Already a Linux path passes through.
		{"ubuntu", "/home/momo/repo", "/home/momo/repo"},
		// Windows drive paths map to the distro's drive mounts, so a worktree
		// can live on the Windows file system. Both separators appear in what
		// Windows hands over.
		{"ubuntu", `C:\Users\momo\repo`, "/mnt/c/Users/momo/repo"},
		{"ubuntu", `d:\data`, "/mnt/d/data"},
		{"ubuntu", "C:/Users/momo/repo", "/mnt/c/Users/momo/repo"},
		{"ubuntu", `C:\`, "/mnt/c"},
		{"ubuntu", "c:", "/mnt/c"},
		// Relative paths are resolved by Git against its cwd.
		{"ubuntu", "sub/dir", "sub/dir"},
	}
	for _, c := range cases {
		got, err := ToLinuxArg(c.distro, c.path)
		if err != nil {
			t.Errorf("ToLinuxArg(%q, %q): %v", c.distro, c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("ToLinuxArg(%q, %q) = %q, want %q", c.distro, c.path, got, c.want)
		}
	}
	// A path this distro cannot reach is refused: Git would read the text as a
	// relative path and act on a directory named after it.
	for _, p := range []string{
		`\\wsl.localhost\ubuntu\repo`, // another distro
		`\\wsl$\ubuntu\repo`,
		`\\server\share\repo`, // another machine
		`\\?\UNC\server\share\repo`,
	} {
		if got, err := ToLinuxArg("debian", p); err == nil {
			t.Errorf("ToLinuxArg(debian, %q) = %q, want an error", p, got)
		} else if !errors.Is(err, ErrOutsideDistro) {
			t.Errorf("ToLinuxArg(debian, %q) = %v, want ErrOutsideDistro", p, err)
		}
	}
}

// TestRoundTrip checks that a path crossing the boundary and back stays the
// same repository identity: worktree paths reported by WSL Git must compare
// equal to the roots Viff hands out.
func TestRoundTrip(t *testing.T) {
	for _, p := range []string{
		`\\wsl.localhost\Ubuntu\home\momo\my repo`,
		`\\wsl.localhost\Ubuntu\home\中文\仓库`,
		`\\wsl$\Ubuntu\repo`,
	} {
		distro, linux, ok := Split(p)
		if !ok {
			t.Fatalf("Split(%q) failed", p)
		}
		want, _ := Normalize(p)
		if got := FromLinux(distro, linux); got != want {
			t.Errorf("round trip of %q: got %q", p, got)
		}
	}
}
