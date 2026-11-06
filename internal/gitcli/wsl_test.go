package gitcli

import (
	"errors"
	"testing"

	"viff/internal/wslpath"
)

// A worktree or clone target in another distro cannot be opened by the git that
// runs in this one. Passing the UNC text through would make Git read it as a
// relative path and create or open a directory named after it, reporting
// success for a path that does not exist.
func TestToGitPathRefusesUnreachablePaths(t *testing.T) {
	repo := `\\wsl.localhost\Ubuntu\home\me\repo`
	for _, p := range []string{
		`\\wsl.localhost\Debian\home\me\wt`,
		`\\server\share\wt`,
	} {
		if got, err := toGitPath(repo, p); err == nil {
			t.Fatalf("toGitPath(%q) = %q, want an error", p, got)
		} else if !errors.Is(err, wslpath.ErrOutsideDistro) {
			t.Fatalf("toGitPath(%q) = %v, want ErrOutsideDistro", p, err)
		}
	}
}

// The paths that are reachable still convert: this is the conversion a worktree
// and a clone depend on.
func TestToGitPathConvertsReachablePaths(t *testing.T) {
	repo := `\\wsl.localhost\Ubuntu\home\me\repo`
	cases := []struct{ path, want string }{
		// Same distro, spelled either way, becomes the Linux path.
		{`\\wsl.localhost\ubuntu\home\me\wt`, "/home/me/wt"},
		{`\\wsl$\Ubuntu\home\me\wt`, "/home/me/wt"},
		// A Windows drive path becomes the distro's mount.
		{`C:\wt`, "/mnt/c/wt"},
		// Already Linux stays as it is.
		{"/home/me/wt", "/home/me/wt"},
		{"wt", "wt"},
	}
	for _, c := range cases {
		got, err := toGitPath(repo, c.path)
		if err != nil {
			t.Fatalf("toGitPath(%q): %v", c.path, err)
		}
		if got != c.want {
			t.Errorf("toGitPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
	// Outside WSL nothing is converted: the conversion is defined against a
	// distro, and a plain Windows repository must not acquire one.
	if got, err := toGitPath(`C:\repo`, `\\wsl.localhost\Debian\wt`); err != nil || got != `\\wsl.localhost\Debian\wt` {
		t.Errorf("plain repo = %q, %v; want the path unchanged", got, err)
	}
}

// Paths Git reports come back in the distro's spelling and must land in the
// Windows identity domain the workspace compares roots with.
func TestToWinPath(t *testing.T) {
	repo := `\\wsl.localhost\Ubuntu\home\me\repo`
	cases := []struct{ path, want string }{
		{"/home/me/wt", `\\wsl.localhost\ubuntu\home\me\wt`},
		{"/mnt/c/wt", `C:\wt`},
	}
	for _, c := range cases {
		if got := toWinPath(repo, c.path); got != c.want {
			t.Errorf("toWinPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
	// A repository outside WSL keeps the paths Git gave it.
	if got := toWinPath(`C:\repo`, "/home/me/wt"); got != "/home/me/wt" {
		t.Errorf("plain repo = %q, want the path unchanged", got)
	}
}
