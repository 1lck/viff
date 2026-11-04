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

// Runner executes git. The zero value uses "git" from PATH.
type Runner struct {
	// Path is the git executable; "" means look it up on PATH.
	Path string
	// Env is added to the inherited environment.
	Env []string
	// Log, when set, receives one line per finished command.
	Log func(dir string, args []string, took time.Duration, err error)

	once     sync.Once
	resolved string
	findErr  error
}

func (r *Runner) git() (string, error) {
	r.once.Do(func() {
		if r.Path != "" {
			r.resolved = r.Path
			return
		}
		p, err := exec.LookPath("git")
		if err != nil {
			// GUI processes on macOS start without the user's shell PATH.
			for _, c := range []string{"/opt/homebrew/bin/git", "/usr/local/bin/git", "/usr/bin/git"} {
				if st, e := os.Stat(c); e == nil && !st.IsDir() {
					r.resolved = c
					return
				}
			}
			r.findErr = &Error{Kind: ErrNoGit, Err: err}
			return
		}
		r.resolved = p
	})
	return r.resolved, r.findErr
}

// baseArgs are prepended to every command: stable, uncolored, unquoted
// output regardless of the user's configuration.
var baseArgs = []string{"-c", "core.quotepath=false", "-c", "color.ui=false", "-c", "log.showSignature=false"}

func (r *Runner) command(ctx context.Context, dir string, args []string) (*exec.Cmd, error) {
	git, err := r.git()
	if err != nil {
		return nil, err
	}
	full := append(append([]string{}, baseArgs...), args...)
	cmd := exec.CommandContext(ctx, git, full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // a GUI has no terminal to prompt on
		"GIT_OPTIONAL_LOCKS=0",  // reads never take the index lock from an external tool
		"LC_ALL=C",
		"LANGUAGE=C",
	)
	cmd.Env = append(cmd.Env, r.Env...)
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
