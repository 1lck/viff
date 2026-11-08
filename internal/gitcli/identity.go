package gitcli

import "viff/internal/sshpath"

// toIdentityPath maps an absolute path Git reported into the domain the rest
// of Viff uses: the \\wsl.localhost spelling for a WSL checkout, the ssh://
// URL for a remote one, and the path itself otherwise. Git output stays in its
// own execution domain; this is the only place the two are joined.
//
// A remote path that cannot be mapped is an error rather than the raw path:
// returning the raw path would make domain.Repo.IsSSH false and silently drop
// every remote write guard.
func toIdentityPath(dir, p string) (string, error) {
	if sshpath.Is(dir) {
		return sshpath.URLOf(dir, p)
	}
	return toWinPath(dir, p), nil
}
