// Package gitcli runs the official Git CLI and parses its machine-readable
// output. It builds argument arrays (never shell strings), consumes stderr
// with a bound, and classifies failures. It knows nothing about the UI.
package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"viff/internal/process"
	"viff/internal/wslpath"
)

// stderrLimit bounds the stderr kept for an error message.
const stderrLimit = 64 << 10

// ErrorKind classifies a Git failure so the UI can offer the right action.
type ErrorKind uint8

const (
	ErrOther ErrorKind = iota
	ErrCancelled
	ErrAuth
	ErrLocked
	ErrNotRepository
	ErrConflict
	ErrNotFound
	ErrRejected // push rejected (non-fast-forward)
	ErrUntrackedOverwrite
	ErrLocalChangesOverwrite
	ErrNotMerged
	ErrNothingToCommit
	ErrHookFailed
	ErrNoGit
	ErrNoWSL    // WSL is missing or unusable on this machine
	ErrNoDistro // the distro named by the path does not exist
	ErrNoWSLGit // the distro has no git executable
)

// Error is a failed Git invocation with the context needed to diagnose it.
type Error struct {
	Kind     ErrorKind
	Args     []string
	ExitCode int
	Stderr   string
	Stdout   string
	Err      error
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(e.Stdout)
	}
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("git %s: %s", firstArg(e.Args), msg)
}

// Message is the Git output meant for the user, without the command prefix.
func (e *Error) Message() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(e.Stdout)
	}
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

func firstArg(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" || args[i] == "-C" {
			i++
			continue
		}
		return args[i]
	}
	return ""
}

// KindOf returns the error kind of err, ErrOther for foreign errors.
func KindOf(err error) ErrorKind {
	var ge *Error
	if errors.As(err, &ge) {
		return ge.Kind
	}
	if errors.Is(err, context.Canceled) {
		return ErrCancelled
	}
	return ErrOther
}

// String is the stable name of the kind written to logs next to the code. It
// is a log format: rename one only together with the log format.
func (k ErrorKind) String() string {
	switch k {
	case ErrCancelled:
		return "cancelled"
	case ErrAuth:
		return "auth"
	case ErrLocked:
		return "locked"
	case ErrNotRepository:
		return "not_repository"
	case ErrConflict:
		return "conflict"
	case ErrNotFound:
		return "not_found"
	case ErrRejected:
		return "rejected"
	case ErrUntrackedOverwrite:
		return "untracked_overwrite"
	case ErrLocalChangesOverwrite:
		return "local_changes"
	case ErrNotMerged:
		return "not_merged"
	case ErrNothingToCommit:
		return "nothing_to_commit"
	case ErrHookFailed:
		return "hook_failed"
	case ErrNoGit:
		return "no_git"
	case ErrNoWSL:
		return "no_wsl"
	case ErrNoDistro:
		return "no_distro"
	case ErrNoWSLGit:
		return "no_wsl_git"
	}
	return "other"
}

// Code is the stable, user-facing error code for the kind. It is what the UI,
// the log and the diagnostics bundle show, so users can paste one token into
// an issue and be understood. It never changes without a migration plan.
func (k ErrorKind) Code() string {
	switch k {
	case ErrAuth:
		return "auth.failed"
	case ErrLocked:
		return "repo.locked"
	case ErrNotRepository:
		return "repo.not_a_repository"
	case ErrConflict:
		return "git.conflict"
	case ErrNotFound:
		return "git.not_found"
	case ErrRejected:
		return "push.rejected"
	case ErrUntrackedOverwrite:
		return "worktree.untracked_overwrite"
	case ErrLocalChangesOverwrite:
		return "worktree.local_changes"
	case ErrNotMerged:
		return "branch.not_merged"
	case ErrNothingToCommit:
		return "commit.empty"
	case ErrHookFailed:
		return "hook.failed"
	case ErrNoGit:
		return "git.missing"
	case ErrNoWSL:
		return "wsl.unavailable"
	case ErrNoDistro:
		return "wsl.distro_missing"
	case ErrNoWSLGit:
		return "wsl.git_missing"
	case ErrCancelled:
		return "git.cancelled"
	}
	return "git.failed"
}

// Code returns the error code of e, "git.failed" for a nil error.
func (e *Error) Code() string {
	if e == nil {
		return "git.failed"
	}
	return e.Kind.Code()
}

// CodeOf returns the stable error code for err: the code of its kind, with
// "git.failed" for errors Viff did not classify and "" for nil. Callers pass
// the result to the UI and to log events; a failure event always has a code.
func CodeOf(err error) string {
	if err == nil {
		return ""
	}
	return KindOf(err).Code()
}

func classify(stderr string, exit int, ctxErr error) ErrorKind {
	if ctxErr != nil {
		return ErrCancelled
	}
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "authentication failed"), strings.Contains(s, "could not read username"),
		strings.Contains(s, "permission denied (publickey"), strings.Contains(s, "terminal prompts disabled"),
		strings.Contains(s, "invalid username or password"), strings.Contains(s, "returned error: 403"), strings.Contains(s, "returned error: 401"):
		return ErrAuth
	case strings.Contains(s, "index.lock"), strings.Contains(s, "unable to create") && strings.Contains(s, ".lock"):
		return ErrLocked
	case strings.Contains(s, "not a git repository"):
		return ErrNotRepository
	case strings.Contains(s, "untracked working tree files would be overwritten"):
		return ErrUntrackedOverwrite
	case strings.Contains(s, "your local changes to the following files would be overwritten"):
		return ErrLocalChangesOverwrite
	case strings.Contains(s, "conflict"), strings.Contains(s, "could not apply"), strings.Contains(s, "fix conflicts"):
		return ErrConflict
	case strings.Contains(s, "[rejected]"), strings.Contains(s, "non-fast-forward"), strings.Contains(s, "fetch first"), strings.Contains(s, "stale info"):
		return ErrRejected
	case strings.Contains(s, "is not fully merged"):
		return ErrNotMerged
	case strings.Contains(s, "nothing to commit"), strings.Contains(s, "nothing added to commit"):
		return ErrNothingToCommit
	case strings.Contains(s, "hook") && (strings.Contains(s, "failed") || strings.Contains(s, "exit")):
		return ErrHookFailed
	case strings.Contains(s, "does not exist"), strings.Contains(s, "unknown revision"), strings.Contains(s, "bad revision"), strings.Contains(s, "did not match any"):
		return ErrNotFound
	}
	return ErrOther
}

// Config is the Git configuration Viff passes to its own invocations. It
// comes from the user's settings and never changes files on disk: empty
// fields are left out so Git's configuration and defaults still apply.
type Config struct {
	SSHCommand       string // GIT_SSH_COMMAND
	GPGProgram       string // gpg.program
	SigningKey       string // user.signingkey
	SignFormat       string // gpg.format: "openpgp" or "ssh"
	CommitSigning    bool   // commit.gpgsign=true
	CredentialHelper string // credential.helper
}

// args renders the configuration as -c arguments. Git applies them after the
// system and user configuration and before the command, so they win without
// editing the repository or the user's config.
func (c Config) args() []string {
	var out []string
	add := func(key, value string) {
		if value != "" {
			out = append(out, "-c", key+"="+value)
		}
	}
	add("gpg.program", c.GPGProgram)
	add("user.signingkey", c.SigningKey)
	add("gpg.format", c.SignFormat)
	add("credential.helper", c.CredentialHelper)
	if c.CommitSigning {
		out = append(out, "-c", "commit.gpgsign=true")
	}
	return out
}

// Runner executes git. The zero value uses "git" from PATH.
type Runner struct {
	// Path is the git executable; "" means look it up on PATH.
	Path string
	// Env is added to the inherited environment.
	Env []string
	// Log, when set, receives one line per finished command.
	Log func(dir string, args []string, took time.Duration, err error)
	// LFSCommand runs one git-lfs subcommand; args are those after "lfs". It
	// is nil in production, where the real executable runs through Path, and
	// tests set it to simulate a missing or installed git-lfs.
	LFSCommand func(ctx context.Context, dir string, args ...string) ([]byte, error)

	lfsMu        sync.Mutex
	lfsProbeKey  string
	lfsProbed    bool
	lfsInstalled bool
	lfsVersion   string

	resolveMu sync.Mutex
	pathReady bool
	resolved  string
	findErr   error

	cfgMu sync.Mutex
	cfg   Config

	// WSL state: which distros were probed to have a working git, and the
	// resolved wsl.exe. Guarded by wslMu; wslProbe and wslExeOverride are the
	// test seams (like LFSCommand).
	wslMu          sync.Mutex
	wslReady       map[string]bool
	wslGitPath     map[string]string
	wslExe         string
	wslExeKnown    bool
	wslExeErr      error
	wslProbe       func(ctx context.Context, distro string) error
	wslExeOverride string
}

// SetConfig replaces the configuration used by commands started afterwards.
// A command already running keeps the configuration it started with, so a
// concurrent refresh never sees a half-applied setting.
func (r *Runner) SetConfig(c Config) {
	r.cfgMu.Lock()
	r.cfg = c
	r.cfgMu.Unlock()
}

func (r *Runner) config() Config {
	r.cfgMu.Lock()
	defer r.cfgMu.Unlock()
	return r.cfg
}

// SetPath changes the Git executable and forgets the path resolved before, so
// the next command runs the new binary instead of the cached one.
func (r *Runner) SetPath(path string) {
	r.resolveMu.Lock()
	r.Path = path
	r.pathReady = false
	r.resolved = ""
	r.findErr = nil
	r.resolveMu.Unlock()
}

// Executable reports the Git binary commands would run: the configured Path,
// or the one found on PATH and the well-known macOS locations. The error is
// classified ErrNoGit when there is none. The settings dialog shows it so the
// user can see what "automatic" resolved to.
func (r *Runner) Executable() (string, error) {
	return r.git()
}

// ExecutableIn reports the Git binary the commands of the repository at root
// run: a WSL repository runs the git inside its distribution, anything else
// uses Executable. The settings dialog asks this so the echoed path names the
// binary that will really serve the open repository, not the Windows default.
//
// A distro this process has not probed yet is probed here; the answer is then
// cached like every other probe, so opening the dialog is cheap.
func (r *Runner) ExecutableIn(ctx context.Context, root string) (string, error) {
	distro := wslDistroOf(root)
	if distro == "" {
		return r.Executable()
	}
	if exe := r.cachedWSLGit(distro); exe != "" {
		return exe, nil
	}
	if err := r.probeWSL(ctx, distro); err != nil {
		return "", err
	}
	if exe := r.cachedWSLGit(distro); exe != "" {
		return exe, nil
	}
	// A probed distro always records its path; "git" is the lookup Git itself
	// performs through PATH inside the distro.
	return "git", nil
}

func (r *Runner) cachedWSLGit(distro string) string {
	r.wslMu.Lock()
	defer r.wslMu.Unlock()
	return r.wslGitPath[distro]
}

func (r *Runner) setWSLGit(distro, exe string) {
	r.wslMu.Lock()
	if r.wslGitPath == nil {
		r.wslGitPath = map[string]string{}
	}
	if r.wslReady == nil {
		r.wslReady = map[string]bool{}
	}
	r.wslGitPath[distro] = exe
	r.wslReady[distro] = true
	r.wslMu.Unlock()
}

func (r *Runner) git() (string, error) {
	r.resolveMu.Lock()
	defer r.resolveMu.Unlock()
	if r.pathReady {
		return r.resolved, r.findErr
	}
	r.pathReady = true
	if r.Path != "" {
		r.resolved = r.Path
		return r.resolved, nil
	}
	p, err := exec.LookPath("git")
	if err != nil {
		// GUI processes on macOS start without the user's shell PATH.
		for _, c := range []string{"/opt/homebrew/bin/git", "/usr/local/bin/git", "/usr/bin/git"} {
			if st, e := os.Stat(c); e == nil && !st.IsDir() {
				r.resolved = c
				return r.resolved, nil
			}
		}
		r.findErr = &Error{Kind: ErrNoGit, Err: err}
		return "", r.findErr
	}
	r.resolved = p
	return r.resolved, nil
}

// baseArgs are prepended to every command: stable, uncolored, unquoted
// output regardless of the user's configuration.
var baseArgs = []string{"-c", "core.quotepath=false", "-c", "color.ui=false", "-c", "log.showSignature=false"}

func (r *Runner) command(ctx context.Context, dir string, args []string) (*exec.Cmd, error) {
	if wslpath.Is(dir) {
		// A WSL repository runs the distro's own git through wsl.exe; see
		// wsl_windows.go and docs/decisions/0037-wsl-repositories.md.
		return r.wslCommand(ctx, dir, args)
	}
	git, err := r.git()
	if err != nil {
		return nil, err
	}
	cfg := r.config()
	full := make([]string, 0, len(baseArgs)+len(args)+8)
	full = append(full, baseArgs...)
	full = append(full, cfg.args()...)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, git, full...)
	// A GUI build has no console; without this every Git command would open
	// and flash its own console window on Windows.
	process.HideConsoleWindow(cmd)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // a GUI has no terminal to prompt on
		"GIT_OPTIONAL_LOCKS=0",  // reads never take the index lock from an external tool
		"LC_ALL=C",
		"LANGUAGE=C",
	)
	cmd.Env = append(cmd.Env, r.Env...)
	if cfg.SSHCommand != "" {
		// After Env so the setting wins; Git hands this to its ssh invocation.
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND="+cfg.SSHCommand)
	}
	cmd.WaitDelay = 2 * time.Second
	return cmd, nil
}

// limitedBuffer keeps at most limit bytes.
type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

// Run executes git in dir and returns its stdout.
func (r *Runner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return r.RunInput(ctx, dir, nil, args...)
}

// RunInput executes git with stdin.
func (r *Runner) RunInput(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
	start := time.Now()
	cmd, err := r.command(ctx, dir, args)
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	stderr := &limitedBuffer{limit: stderrLimit}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	err = cmd.Run()
	if err != nil {
		err = r.wrap(ctx, args, err, stderr.buf.String(), stdout.String())
	}
	if r.Log != nil {
		r.Log(dir, args, time.Since(start), err)
	}
	return stdout.Bytes(), err
}

// RunProgress executes a long-running command and reports each line Git
// writes to stderr (progress output uses carriage returns; both count).
func (r *Runner) RunProgress(ctx context.Context, dir string, progress func(line string), args ...string) ([]byte, error) {
	start := time.Now()
	cmd, err := r.command(ctx, dir, args)
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, r.wrap(ctx, args, err, "", "")
	}
	stderr := &limitedBuffer{limit: stderrLimit}
	buf := make([]byte, 4096)
	var line []byte
	for {
		n, rerr := pipe.Read(buf)
		for _, c := range buf[:n] {
			if c == '\r' || c == '\n' {
				if len(line) > 0 && progress != nil {
					progress(string(line))
				}
				line = line[:0]
			} else {
				line = append(line, c)
			}
		}
		stderr.Write(buf[:n])
		if rerr != nil {
			break
		}
	}
	if len(line) > 0 && progress != nil {
		progress(string(line))
	}
	err = cmd.Wait()
	if err != nil {
		err = r.wrap(ctx, args, err, stderr.buf.String(), stdout.String())
	}
	if r.Log != nil {
		r.Log(dir, args, time.Since(start), err)
	}
	return stdout.Bytes(), err
}

func (r *Runner) wrap(ctx context.Context, args []string, err error, stderr, stdout string) error {
	exit := -1
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		exit = ee.ExitCode()
	}
	// wsl.exe writes its own diagnostics as UTF-16LE with a BOM while the Git
	// it execs passes UTF-8 through; the BOM decides (see wsl.go). Git's own
	// output never starts with one, so decoding unconditionally is safe.
	stderr, stdout = decodeWSLText(stderr), decodeWSLText(stdout)
	return &Error{Kind: classify(stderr+"\n"+stdout, exit, ctx.Err()), Args: args, ExitCode: exit, Stderr: stderr, Stdout: stdout, Err: err}
}

// Stream starts git and returns its stdout for incremental reading. Closing
// the stream stops the process.
func (r *Runner) Stream(ctx context.Context, dir string, args ...string) (*Stream, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd, err := r.command(ctx, dir, args)
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr := &limitedBuffer{limit: stderrLimit}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, r.wrap(ctx, args, err, "", "")
	}
	return &Stream{cmd: cmd, out: out, cancel: cancel, stderr: stderr, args: args, runner: r, ctx: ctx}, nil
}

// Stream is a running git process whose stdout is read incrementally.
type Stream struct {
	cmd    *exec.Cmd
	out    io.ReadCloser
	cancel context.CancelFunc
	stderr *limitedBuffer
	args   []string
	runner *Runner
	ctx    context.Context
	closed bool
}

func (s *Stream) Read(p []byte) (int, error) { return s.out.Read(p) }

// Close stops the process and returns its error, if it failed by itself.
func (s *Stream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.cancel()
	err := s.cmd.Wait()
	if err != nil && s.ctx.Err() == nil {
		return s.runner.wrap(s.ctx, s.args, err, s.stderr.buf.String(), "")
	}
	return nil
}

// Wait waits for a stream read to its end and returns the process error.
func (s *Stream) Wait() error {
	if s.closed {
		return nil
	}
	s.closed = true
	err := s.cmd.Wait()
	if err != nil {
		err = s.runner.wrap(s.ctx, s.args, err, s.stderr.buf.String(), "")
	}
	s.cancel()
	return err
}

// Version returns the output of git --version, e.g. "2.53.0".
func (r *Runner) Version(ctx context.Context) (string, error) {
	out, err := r.Run(ctx, "", "--version")
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(out))
	v = strings.TrimPrefix(v, "git version ")
	return v, nil
}
