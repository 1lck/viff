package wslpath

import "testing"

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
		{`C:\Users\momo\repo`, false},
		{`\\server\share\repo`, false},
		{`\\wsl.localhost\`, false}, // no distro
		{`\\wsl$`, false},           // no distro
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
		// Already a Linux path passes through.
		{"ubuntu", "/home/momo/repo", "/home/momo/repo"},
		// Windows drive paths map to the distro's drive mounts, so a worktree
		// can live on the Windows file system.
		{"ubuntu", `C:\Users\momo\repo`, "/mnt/c/Users/momo/repo"},
		{"ubuntu", `d:\data`, "/mnt/d/data"},
		// Relative paths are resolved by Git against its cwd.
		{"ubuntu", "sub/dir", "sub/dir"},
		// Another distro is unreachable; Git reports the failure.
		{"debian", `\\wsl.localhost\ubuntu\repo`, `\\wsl.localhost\ubuntu\repo`},
	}
	for _, c := range cases {
		if got := ToLinuxArg(c.distro, c.path); got != c.want {
			t.Errorf("ToLinuxArg(%q, %q) = %q, want %q", c.distro, c.path, got, c.want)
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
