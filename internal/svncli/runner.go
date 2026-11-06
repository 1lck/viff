package svncli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"viff/internal/process"
	"viff/internal/vcs"
)

// stderrLimit bounds the stderr kept for an error message. Subversion prints
// its machine-readable output on stdout, so stderr is diagnosis only.
const stderrLimit = 64 << 10

// nonInteractive is prepended to every invocation. A GUI process has no
// terminal: without this flag svn blocks forever on the first prompt that
// needs an answer (conflict resolution, certificate acceptance, credentials),
// freezing the window. This is the single most important invariant of this
// adapter — see docs/decisions/0024-svn-support.md (finding S11).
var nonInteractive = []string{"--non-interactive"}

// Runner executes the svn client. The zero value uses "svn" from PATH.
type Runner struct {
	// Path is the svn executable; "" means look it up on PATH.
	Path string
	// Env is added to the inherited environment.
	Env []string
	// Log, when set, receives one line per finished command.
	Log vcs.CommandLog

	resolveMu sync.Mutex
	resolved  string
	findErr   error
}

// SetPath changes the executable and forgets the resolved path.
func (r *Runner) SetPath(path string) {
	r.resolveMu.Lock()
	r.Path = path
	r.resolved = ""
	r.findErr = nil
	r.resolveMu.Unlock()
}

// SetCommandLog installs the hook that records finished commands.
func (r *Runner) SetCommandLog(fn vcs.CommandLog) { r.Log = fn }

// SetConfig is a no-op: Subversion has no equivalent of the Git configuration
// Viff injects (signing, credential helpers). The settings page pushes one
// value to whichever backend is active, so this has to exist and accept it.
func (r *Runner) SetConfig(vcs.Config) {}

// Version returns the client version, for the diagnostics summary.
func (r *Runner) Version(ctx context.Context) (string, error) {
	out, err := r.Run(ctx, "", "--version", "--quiet")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ExecutableIn resolves the svn client. Subversion has no per-repository
// executable, so the answer is the same for every root and needs no probe.
func (r *Runner) ExecutableIn(ctx context.Context, root string) (string, error) {
	return r.svn()
}

// CachedExecutableIn reports the svn client without starting a process: the
// resolved path once it is known, otherwise the configured one. "" means the
// path has not been resolved yet, which is what the settings echo shows as
// "automatic".
func (r *Runner) CachedExecutableIn(root string) (string, error) {
	r.resolveMu.Lock()
	defer r.resolveMu.Unlock()
	if r.resolved != "" {
		return r.resolved, nil
	}
	return r.Path, nil
}

func (r *Runner) svn() (string, error) {
	r.resolveMu.Lock()
	defer r.resolveMu.Unlock()
	if r.resolved != "" {
		return r.resolved, nil
	}
	if r.findErr != nil {
		return "", r.findErr
	}
	if r.Path != "" {
		// A configured path is validated here, not left to exec: a wrong path
		// in the settings must reach the user as "not found" so the UI can
		// offer the install/path hint, instead of surfacing a raw
		// "executable file not found in $PATH" from the failed fork.
		if _, err := os.Stat(r.Path); err != nil {
			r.findErr = &vcs.Error{Kind: vcs.ErrNoGit, Tool: "svn", Err: err}
			return "", r.findErr
		}
		r.resolved = r.Path
		return r.resolved, nil
	}
	p, err := exec.LookPath("svn")
	if err != nil {
		r.findErr = &vcs.Error{Kind: vcs.ErrNoGit, Tool: "svn", Err: err}
		return "", r.findErr
	}
	r.resolved = p
	return r.resolved, nil
}

// command builds the exec.Cmd with the mandatory non-interactive flag and the
// console-window suppression every child process needs on Windows.
func (r *Runner) command(ctx context.Context, dir string, args []string) (*exec.Cmd, error) {
	svn, err := r.svn()
	if err != nil {
		return nil, err
	}
	full := make([]string, 0, len(nonInteractive)+len(args))
	full = append(full, nonInteractive...)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, svn, full...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(r.Env) > 0 {
		cmd.Env = append(cmd.Environ(), r.Env...)
	}
	process.HideConsoleWindow(cmd)
	return cmd, nil
}

// Run executes svn in dir and returns stdout. A failure is classified from the
// stderr E-number, never from the exit status: svn exits 0 even when the
// working copy has conflicts (finding S5).
func (r *Runner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return r.RunInput(ctx, dir, nil, args...)
}

// RunInput executes svn with stdin, for commands that read a message or a
// patch from a stream.
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
		r.Log(dir, append(append([]string{}, nonInteractive...), args...), time.Since(start), err)
	}
	return stdout.Bytes(), err
}

// RunProgress executes svn and forwards each stderr line, which is where svn
// reports transfer progress. Only the tail is kept for the error message.
func (r *Runner) RunProgress(ctx context.Context, dir string, progress func(string), args ...string) ([]byte, error) {
	start := time.Now()
	cmd, err := r.command(ctx, dir, args)
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	stderr := &limitedBuffer{limit: stderrLimit, progress: progress}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		err = r.wrap(ctx, args, err, stderr.buf.String(), stdout.String())
	}
	if r.Log != nil {
		r.Log(dir, append(append([]string{}, nonInteractive...), args...), time.Since(start), err)
	}
	return stdout.Bytes(), err
}

// RunProgressInput is RunProgress with stdin, for a commit that carries its
// message on a stream while still reporting transfer progress.
func (r *Runner) RunProgressInput(ctx context.Context, dir string, progress func(string), stdin []byte, args ...string) ([]byte, error) {
	start := time.Now()
	cmd, err := r.command(ctx, dir, args)
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	stderr := &limitedBuffer{limit: stderrLimit, progress: progress}
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
		r.Log(dir, append(append([]string{}, nonInteractive...), args...), time.Since(start), err)
	}
	return stdout.Bytes(), err
}

func (r *Runner) wrap(ctx context.Context, args []string, err error, stderr, stdout string) error {
	kind := classify(stderr, err, ctx.Err())
	return &vcs.Error{
		Tool:   "svn",
		Kind:   kind,
		Args:   args,
		Stderr: stderr,
		Stdout: stdout,
		Err:    err,
	}
}

// limitedBuffer keeps at most limit bytes and optionally echoes whole lines to
// progress. It never blocks the child on a full pipe.
type limitedBuffer struct {
	buf      bytes.Buffer
	limit    int
	progress func(string)
	partial  []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.progress != nil {
		b.partial = append(b.partial, p...)
		for {
			i := bytes.IndexByte(b.partial, '\n')
			if i < 0 {
				break
			}
			line := strings.TrimRight(string(b.partial[:i]), "\r")
			b.partial = b.partial[i+1:]
			if line != "" {
				b.progress(line)
			}
		}
	}
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) <= room {
			b.buf.Write(p)
		} else {
			b.buf.Write(p[:room])
		}
	}
	return len(p), nil
}

// svnErrorCodes maps the E-numbers this adapter has actually observed on svn
// 1.15.0 (Windows x64) onto the shared kinds. Numbers not listed here fall
// through to the generic cases in classify; do not add one without a test that
// provokes the real command, because a wrong mapping mislabels a failure the
// user then cannot act on.
var svnErrorCodes = []struct {
	code string
	kind vcs.ErrorKind
}{
	{"E155007", vcs.ErrNotRepository}, // not a working copy
	{"E155004", vcs.ErrLocked},        // working copy locked
	{"E155011", vcs.ErrOutOfDate},     // commit refused, directory out of date
	{"E170013", vcs.ErrNetwork},       // unable to connect to a repository
	{"E180001", vcs.ErrNetwork},       // unable to open repository
	{"E170001", vcs.ErrAuth},          // authorization failed
	{"E160006", vcs.ErrNoRevision},    // no such revision
	{"E195012", vcs.ErrNotFound},      // path not found at that revision
	// A path missing at a revision is reported as a warning, not an error:
	// "svn: warning: W160013: File not found: revision 4, path /x". The
	// E-form is checked too because some commands promote it.
	{"W160013", vcs.ErrNotFound},
	{"E160013", vcs.ErrNotFound},
}

// conflictCodes are the "would conflict" family: svn reports them when an
// update or merge cannot proceed because of local changes.
var conflictCodes = []string{"E155015", "E195020", "E155023"}

func classify(stderr string, err error, ctxErr error) vcs.ErrorKind {
	if ctxErr != nil {
		return vcs.ErrCancelled
	}
	if errors.Is(err, context.Canceled) {
		return vcs.ErrCancelled
	}
	for _, c := range svnErrorCodes {
		if strings.Contains(stderr, c.code) {
			return c.kind
		}
	}
	for _, code := range conflictCodes {
		if strings.Contains(stderr, code) {
			return vcs.ErrConflict
		}
	}
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "authentication failed"), strings.Contains(s, "authorization failed"),
		strings.Contains(s, "could not authenticate"), strings.Contains(s, "e170001"):
		return vcs.ErrAuth
	case strings.Contains(s, "is not a working copy"):
		return vcs.ErrNotRepository
	case strings.Contains(s, "conflict"):
		return vcs.ErrConflict
	case strings.Contains(s, "out of date"):
		return vcs.ErrOutOfDate
	case strings.Contains(s, "locked"):
		return vcs.ErrLocked
	case strings.Contains(s, "unable to connect"), strings.Contains(s, "could not resolve host"):
		return vcs.ErrNetwork
	case strings.Contains(s, "does not exist"), strings.Contains(s, "no such revision"),
		strings.Contains(s, "path not found"):
		return vcs.ErrNotFound
	}
	return vcs.ErrOther
}

// errorCode renders an unexpected shape as a classified error so callers never
// have to fabricate one.
func errorf(kind vcs.ErrorKind, format string, args ...any) error {
	return &vcs.Error{Tool: "svn", Kind: kind, Stderr: fmt.Sprintf(format, args...)}
}
