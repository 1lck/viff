//go:build !windows

package gitcli

import (
	"context"
	"errors"
	"os/exec"

	"viff/internal/wslpath"
)

// wslCommand reports that a WSL repository cannot be served on this platform.
// wslpath only classifies \\wsl.localhost spellings, which on non-Windows
// systems cannot point at a distro; a caller only gets here by handing one of
// those strings over, and the error says so instead of running the Windows git
// against an unusable path.
func (r *Runner) wslCommand(ctx context.Context, dir string, args []string) (*exec.Cmd, error) {
	distro, _, ok := wslpath.Split(dir)
	if !ok {
		return nil, &Error{Kind: ErrNoWSL, Args: args, Err: errors.New("not a WSL path")}
	}
	return nil, &Error{Kind: ErrNoWSL, Args: args, Err: errors.New("WSL repositories can only be opened from Windows (distro " + distro + ")")}
}

func (r *Runner) wslProgram(ctx context.Context, dir string, _ []string) (*exec.Cmd, error) {
	return r.wslCommand(ctx, dir, nil)
}

// probeWSL reports that no distro can serve a repository here. ExecutableIn
// reaches it when a caller asks which git a WSL path would run on a platform
// without WSL.
func (r *Runner) probeWSL(ctx context.Context, distro string) error {
	return &Error{Kind: ErrNoWSL, Err: errors.New("WSL repositories can only be opened from Windows (distro " + distro + ")")}
}
