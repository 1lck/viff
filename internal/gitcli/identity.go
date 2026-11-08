package gitcli

import "viff/internal/sshpath"

// toIdentityPath maps an absolute path Git reported into the domain the rest
// of Viff uses: the \\wsl.localhost spelling for a WSL checkout, the ssh://
// URL for a remote one, and the path itself otherwise. Git output stays in its
// own execution domain; this is the only place the two are joined.
func toIdentityPath(dir, p string) string {
	if sshpath.Is(dir) {
		if u, err := sshpath.URLOf(dir, p); err == nil {
			return u
		}
		return p
	}
	return toWinPath(dir, p)
}
