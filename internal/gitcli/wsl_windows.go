//go:build windows

package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"viff/internal/process"
	"viff/internal/wslpath"
)

// wslCommand wraps a Git invocation for a WSL repository. The command runs on
// the git inside the distro, reached through wsl.exe --exec: execv without a
// shell, so the argument-array discipline holds and no Linux-side quoting or
// interpolation can reinterpret a path. --cd places Git at the repository
// because cmd.Dir cannot name a Linux directory.
//
// Neither the Windows git path (Runner.Path) nor the settings that configure
// it participate. Those values name Windows programs and Windows file system
// paths — gpg.program, credential.helper and GIT_SSH_COMMAND point at
// executables the distro cannot run — so handing them over would replace the
// Linux side's own configuration with broken copies of the Windows one. A WSL
// repository is served by the distro's git, whose configuration, credentials,
// signing and hooks live on the Linux side (ADR 0038).
func (r *Runner) wslCommand(ctx context.Context, dir string, args []string) (*exec.Cmd, error) {
	return r.wslProgram(ctx, dir, append([]string{"git"}, append(append([]string{}, baseArgs...), args...)...))
}

// wslProgram also serves the fixed, EOF-guarded LFS read wrapper. Values remain
// separate argv entries even when the program itself is a static shell script.
func (r *Runner) wslProgram(ctx context.Context, dir string, program []string) (*exec.Cmd, error) {
	distro, linuxDir, ok := wslpath.Split(dir)
	if !ok {
		return nil, &Error{Kind: ErrNoWSL, Err: fmt.Errorf("%s is not a WSL path", dir)}
	}
	if err := r.probeWSL(ctx, distro); err != nil {
		return nil, err
	}
	wsl, err := r.wslExecutable()
	if err != nil {
		return nil, err
	}
	full := make([]string, 0, len(program)+16)
	full = append(full, "-d", distro, "--cd", linuxDir, "--exec")
	full = append(full, program...)
	cmd := exec.CommandContext(ctx, wsl, full...)
	// A GUI build has no console; without this every Git command would open
	// and flash its own console window on Windows.
	process.HideConsoleWindow(cmd)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		// The fixed Git environment reaches the Linux side only through
		// WSLENV; values are set below so they exist to forward.
		"WSLENV="+wslEnvForward(os.Getenv("WSLENV")),
		// wsl.exe writes its own diagnostics as UTF-16LE on a pipe unless this
		// is set; UTF-8 keeps one encoding for the CLI and for Git's output.
		"WSL_UTF8=1",
		"GIT_TERMINAL_PROMPT=0", // a GUI has no terminal to prompt on
		"GIT_OPTIONAL_LOCKS=0",  // reads never take the index lock from an external tool
		"LC_ALL=C",
		"LANGUAGE=C",
	)
	// r.Env carries the test and caller environment as-is: it is not one of the
	// settings above, and a test that stubs a program needs it inside the
	// distro too.
	cmd.Env = append(cmd.Env, r.Env...)
	cmd.WaitDelay = 2 * time.Second
	return cmd, nil
}

// wslEnvForward merges the environment names Viff needs inside the distro
// into WSLENV. Flags are /u (Win32 to WSL only); a name the user already
// forwards is left untouched so their flags survive. GIT_SSH_COMMAND is absent
// on purpose: the value comes from the Windows settings and names a Windows
// program, so the distro keeps its own.
func wslEnvForward(existing string) string {
	names := []string{"GIT_TERMINAL_PROMPT", "GIT_OPTIONAL_LOCKS", "LC_ALL", "LANGUAGE"}
	have := map[string]bool{}
	for _, e := range strings.Split(existing, ":") {
		// Each entry is NAME or NAME/flags; compare the name only.
		have[strings.TrimSpace(strings.SplitN(e, "/", 2)[0])] = true
	}
	out := existing
	for _, n := range names {
		if have[n] {
			continue
		}
		if out != "" {
			out += ":"
		}
		out += n + "/u"
	}
	return out
}

// probeWSL reports whether distro has a usable git, caching successes per
// distro for the process lifetime. Failures are not cached: a git installed
// into the distro later must start working without a restart, and the probe
// costs one process at repository discovery only.
func (r *Runner) probeWSL(ctx context.Context, distro string) error {
	if r.wslProbe != nil { // test seam, like LFSCommand
		return r.wslProbe(ctx, distro)
	}
	r.wslMu.Lock()
	if r.wslReady[distro] {
		r.wslMu.Unlock()
		return nil
	}
	r.wslMu.Unlock()

	err := r.runWSLProbe(ctx, distro)
	if err == nil {
		// Remember the binary as well, so the settings dialog can name it
		// instead of the Windows default.
		r.setWSLGit(distro, r.wslGitExecutable(ctx, distro))
	}
	return err
}

// wslGitExecutable asks the distro where its git lives. The script is a
// constant: no path and no user input crosses into the shell, so the
// argument-array discipline is not weakened for the commands that matter.
func (r *Runner) wslGitExecutable(ctx context.Context, distro string) string {
	wsl, err := r.wslExecutable()
	if err != nil {
		return ""
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(pctx, wsl, "-d", distro, "--exec", "sh", "-c", "command -v git")
	process.HideConsoleWindow(cmd)
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// runWSLProbe runs `git --version` inside distro. Success proves that WSL is
// installed, the distro exists and its git runs; every failure carries the
// classified kind so the UI can tell the three apart.
func (r *Runner) runWSLProbe(ctx context.Context, distro string) error {
	wsl, err := r.wslExecutable()
	if err != nil {
		return err
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(pctx, wsl, "-d", distro, "--exec", "git", "--version")
	process.HideConsoleWindow(cmd)
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		ge := &Error{
			Args:     []string{"git", "--version"},
			Stderr:   decodeWSLText(stderr.String()),
			Stdout:   decodeWSLText(stdout.String()),
			Err:      err,
			ExitCode: -1,
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			ge.ExitCode = ee.ExitCode()
		}
		switch {
		case ctx.Err() != nil || pctx.Err() != nil:
			ge.Kind = ErrCancelled
		default:
			ge.Kind = classifyWSLError(ge.Stderr + "\n" + ge.Stdout)
		}
		return ge
	}
	return nil
}

// wslExecutable resolves wsl.exe once; wslExeOverride is the test seam.
func (r *Runner) wslExecutable() (string, error) {
	r.wslMu.Lock()
	defer r.wslMu.Unlock()
	if r.wslExeKnown {
		return r.wslExe, r.wslExeErr
	}
	r.wslExeKnown = true
	if r.wslExeOverride != "" {
		r.wslExe = r.wslExeOverride
		return r.wslExe, nil
	}
	p, err := exec.LookPath("wsl.exe")
	if err != nil {
		r.wslExeErr = &Error{Kind: ErrNoWSL, Err: fmt.Errorf("wsl.exe is not on PATH: %w", err)}
		return "", r.wslExeErr
	}
	r.wslExe = p
	return p, nil
}
