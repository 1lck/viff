package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"viff/internal/domain"
	"viff/internal/process"
	"viff/internal/sshpath"
)

// SSHTransport is the seam between Viff and a repository that lives on
// another machine. Production leaves Runner.SSH nil and uses the system ssh;
// tests replace it with a transport that runs the same command locally, so the
// transport-independent layers are exercised on every platform without a
// network or an SSH server.
type SSHTransport interface {
	// Command builds the local process that runs Git on the server. remoteDir
	// is the absolute path Git must treat as the working directory.
	Command(ctx context.Context, target sshpath.Repo, remoteDir string, args []string) (*exec.Cmd, error)
	// ReadFile reads one working-tree file, bounded by ContentLimit.
	ReadFile(ctx context.Context, repo domain.Repo, rel string) (Content, error)
	// Operation rebuilds the in-progress operation from the server's Git
	// directory.
	Operation(ctx context.Context, repo domain.Repo) (domain.Operation, error)
}

// sshTransport runs commands through the system ssh.
type sshTransport struct{ r *Runner }

func (r *Runner) ssh() SSHTransport {
	if r.SSH != nil {
		return r.SSH
	}
	return sshTransport{r: r}
}

// Command builds the ssh process for one Git command. The remote shell joins
// the arguments, so every value is quoted; the local process starts ssh
// directly and never goes through a shell.
func (t sshTransport) Command(ctx context.Context, target sshpath.Repo, remoteDir string, args []string) (*exec.Cmd, error) {
	return t.r.sshCommand(ctx, target, remoteGitCommand(remoteDir, args))
}

// remoteGitCommand is the command string ssh hands to the remote login shell.
// It is a fixed env prefix plus git and its quoted arguments; the only
// variable parts are quoted with shellQuote (the same POSIX quoting the
// interactive-rebase editor uses).
func remoteGitCommand(remoteDir string, args []string) string {
	parts := []string{
		"env",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"LC_ALL=C",
		"LANGUAGE=C",
		"git",
		"-C", shellQuote(remoteDir),
	}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// sshCommand builds the local ssh process. BatchMode makes a missing key, a
// passphrase or an unknown host fail instead of waiting on a prompt the GUI
// cannot show; StrictHostKeyChecking is deliberately left at the user's
// configured value so Viff never silently trusts a host.
func (r *Runner) sshCommand(ctx context.Context, target sshpath.Repo, remote string) (*exec.Cmd, error) {
	binary, err := exec.LookPath("ssh")
	if err != nil {
		return nil, &Error{Kind: ErrNoSSH, Args: []string{"ssh"}, Err: err}
	}
	host := target.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if target.User != "" {
		host = target.User + "@" + host
	}
	args := []string{
		"-T",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
	}
	if target.Port != 0 && target.Port != 22 {
		args = append(args, "-p", strconv.Itoa(target.Port))
	}
	args = append(args, host, remote)
	cmd := exec.CommandContext(ctx, binary, args...)
	process.HideConsoleWindow(cmd)
	// Keep ssh's own diagnostics in English so the error classification does
	// not depend on the user's locale. The remote Git environment is set by
	// remoteGitCommand instead.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	cmd.WaitDelay = 2 * time.Second
	return cmd, nil
}

// ReadFile reads one working-tree file through a small POSIX script. The
// script runs with the path as $1 (never interpolated into the script text),
// reports a one-line header and then the bytes; metadata and content are
// separated by the header so a binary file stays binary.
func (t sshTransport) ReadFile(ctx context.Context, repo domain.Repo, rel string) (Content, error) {
	path, err := sshpath.AbsPath(repo.Root, rel)
	if err != nil {
		return Content{}, err
	}
	out, err := t.r.runRemoteScript(ctx, repo, remoteFileScript(path), path)
	if err != nil {
		return Content{}, err
	}
	return parseRemoteContent(rel, out)
}

// remoteFileScript is the POSIX sh program used for one working-tree read.
// -c is not POSIX but is supported by GNU, BSD and BusyBox head; the servers
// this feature targets are Linux and macOS.
func remoteFileScript(path string) string {
	limit := strconv.FormatInt(ContentLimit, 10)
	return `p=$1
if [ -L "$p" ]; then printf 'L\n'; readlink "$p"; exit 0; fi
if [ -d "$p" ]; then printf 'D\n'; exit 0; fi
if [ ! -e "$p" ]; then printf 'M\n'; exit 0; fi
if [ ! -f "$p" ]; then printf 'O\n'; exit 0; fi
m=100644
[ -x "$p" ] && m=100755
s=$(wc -c < "$p" | tr -d ' ')
[ -n "$s" ] || exit 1
printf 'F %s %s\n' "$s" "$m"
if [ "$s" -gt ` + limit + ` ]; then
	head -c 8000 "$p"
else
	cat "$p"
fi`
}

// parseRemoteContent turns the script's header plus bytes into a Content. It
// mirrors readWorktree: a missing path is absent, a directory is a submodule
// entry, a symlink carries its target, and a file larger than ContentLimit is
// too large and only contributes the first bytes for binary detection.
func parseRemoteContent(rel string, out []byte) (Content, error) {
	nl := bytes.IndexByte(out, '\n')
	if nl < 0 {
		return Content{}, fmt.Errorf("read %s on the server: the reply has no header", rel)
	}
	header := string(out[:nl])
	body := out[nl+1:]
	fields := strings.Fields(header)
	if len(fields) == 0 {
		return Content{}, fmt.Errorf("read %s on the server: empty reply header", rel)
	}
	switch fields[0] {
	case "M":
		return Content{}, nil
	case "D":
		return Content{Exists: true, Submodule: true}, nil
	case "O":
		return Content{Exists: true}, nil
	case "L":
		target := strings.TrimSuffix(string(body), "\n")
		return Content{Exists: true, Symlink: true, Mode: "120000", Size: int64(len(target)), Data: []byte(target)}, nil
	case "F":
		if len(fields) < 3 {
			return Content{}, fmt.Errorf("read %s on the server: malformed file header %q", rel, header)
		}
		size, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || size < 0 {
			return Content{}, fmt.Errorf("read %s on the server: bad size in %q", rel, header)
		}
		c := Content{Exists: true, Size: size, Mode: fields[2], TooLarge: size > ContentLimit}
		c.Data = body
		if !c.TooLarge {
			if int64(len(c.Data)) < size {
				return Content{}, fmt.Errorf("read %s on the server: got %d of %d bytes", rel, len(c.Data), size)
			}
			if int64(len(c.Data)) > size {
				c.Data = c.Data[:size]
			}
		}
		c.Binary = isBinary(c.Data)
		return c, nil
	default:
		return Content{}, fmt.Errorf("read %s on the server: unknown reply %q", rel, header)
	}
}

// Operation rebuilds merge/rebase/cherry-pick state with one remote script.
// A local repository reads the same files directly; this is the remote half.
func (t sshTransport) Operation(ctx context.Context, repo domain.Repo) (domain.Operation, error) {
	dir, err := sshpath.GitPath(repo.MetaDir)
	if err != nil {
		return domain.Operation{}, err
	}
	out, err := t.r.runRemoteScript(ctx, repo, remoteOperationScript(), dir)
	if err != nil {
		return domain.Operation{}, err
	}
	op, err := parseRemoteOperation(out)
	if err != nil {
		return domain.Operation{}, err
	}
	if op.Incoming != "" && op.Kind != domain.OpBisect {
		if out, err := t.r.Run(ctx, repo.Root, "log", "-1", "--format=%s", op.Incoming, "--"); err == nil {
			op.IncomingSubject = strings.TrimSpace(string(out))
		}
	}
	return op, nil
}

// remoteOperationScript prints nine existence flags and fourteen file values,
// each followed by NUL. The order is part of the parser; the files are the
// same ones the local Operation reads.
func remoteOperationScript() string {
	return `d=$1
has() { if [ -e "$d/$1" ]; then printf '1\0'; else printf '0\0'; fi; }
val() { if [ -f "$d/$1" ]; then cat "$d/$1"; fi; printf '\0'; }
has rebase-merge
has rebase-apply
has rebase-apply/applying
has MERGE_HEAD
has CHERRY_PICK_HEAD
has REVERT_HEAD
has BISECT_LOG
has rebase-merge/interactive
has REBASE_HEAD
val rebase-merge/head-name
val rebase-merge/onto
val rebase-merge/msgnum
val rebase-merge/end
val rebase-merge/stopped-sha
val rebase-apply/head-name
val rebase-apply/onto
val rebase-apply/next
val rebase-apply/last
val MERGE_HEAD
val MERGE_MSG
val CHERRY_PICK_HEAD
val REVERT_HEAD
val REBASE_HEAD`
}

func parseRemoteOperation(out []byte) (domain.Operation, error) {
	fields := bytes.Split(out, []byte{0})
	const nFlags, nValues = 9, 14
	if len(fields) < nFlags+nValues {
		return domain.Operation{}, fmt.Errorf("read the remote operation state: got %d fields, want %d", len(fields), nFlags+nValues)
	}
	flag := func(i int) bool { return len(fields[i]) == 1 && fields[i][0] == '1' }
	value := func(i int) string { return strings.TrimSpace(string(fields[nFlags+i])) }
	var op domain.Operation
	switch {
	case flag(0):
		op.Kind = domain.OpRebase
		op.HeadName = strings.TrimPrefix(value(0), "refs/heads/")
		op.Onto = value(1)
		op.Step, _ = strconv.Atoi(value(2))
		op.Total, _ = strconv.Atoi(value(3))
		op.Interactive = flag(7)
		op.Incoming = value(4)
		if h := value(13); h != "" {
			op.Incoming = h
		}
	case flag(1):
		if flag(2) {
			op.Kind = domain.OpApplyMailbox
		} else {
			op.Kind = domain.OpRebase
			op.HeadName = strings.TrimPrefix(value(5), "refs/heads/")
			op.Onto = value(6)
		}
		op.Step, _ = strconv.Atoi(value(7))
		op.Total, _ = strconv.Atoi(value(8))
		op.Incoming = value(13)
	case flag(3):
		op.Kind = domain.OpMerge
		heads := strings.Fields(value(9))
		if len(heads) > 0 {
			op.Incoming = heads[0]
		}
		op.MergeMessage = value(10)
	case flag(4):
		op.Kind = domain.OpCherryPick
		op.Incoming = value(11)
		op.MergeMessage = value(10)
	case flag(5):
		op.Kind = domain.OpRevert
		op.Incoming = value(12)
		op.MergeMessage = value(10)
	case flag(6):
		op.Kind = domain.OpBisect
	}
	// The values array is indexed by position, not by file name; a rebase
	// record keeps its incoming in stopped-sha (or REBASE_HEAD), while the
	// merge message is only meaningful for merge/pick/revert.
	switch op.Kind {
	case domain.OpApplyMailbox:
		op.Incoming = value(13)
	case domain.OpBisect:
		op.MergeMessage = ""
	}
	return op, nil
}

// runRemoteScript runs a POSIX sh program on the server. The program is
// static; its arguments are quoted individually, so a path with spaces,
// quotes or non-ASCII bytes stays one argument.
func (r *Runner) runRemoteScript(ctx context.Context, repo domain.Repo, script string, args ...string) ([]byte, error) {
	target, err := sshpath.Parse(repo.Root)
	if err != nil {
		return nil, err
	}
	parts := []string{"sh", "-c", shellQuote(script), "sh"}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	cmd, err := r.sshCommand(ctx, target, strings.Join(parts, " "))
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	stderr := &limitedBuffer{limit: stderrLimit}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), r.wrap(ctx, repo.Root, []string{"sh", "-c"}, err, stderr.buf.String(), stdout.String())
	}
	return stdout.Bytes(), nil
}

// remoteReadCommand is the allowlist of Git commands a read-only remote
// repository may run. It is deliberately conservative: a command missing here
// fails closed instead of mutating the server. Only read-only commands and
// their read-only argument shapes are accepted.
func remoteReadCommand(args []string) bool {
	sub, rest, ok := gitSubcommand(args)
	if !ok {
		return false
	}
	if sub == "--version" || sub == "version" {
		return true
	}
	switch sub {
	case "status", "log", "show", "diff", "cat-file", "rev-parse", "rev-list",
		"for-each-ref", "show-ref", "merge-base", "blame", "ls-files", "ls-tree",
		"grep", "describe", "check-ref-format", "count-objects", "diff-tree",
		"diff-index", "check-ignore", "check-attr", "name-rev", "shortlog", "var",
		"help":
		return true
	case "symbolic-ref":
		for _, a := range rest {
			if a == "-d" || a == "--delete" || a == "-m" || a == "--message" {
				return false
			}
		}
		return countPositionals(rest) <= 1
	case "branch":
		return readOnlyBranch(rest)
	case "tag":
		return readOnlyTag(rest)
	case "stash":
		return len(rest) > 0 && (rest[0] == "list" || rest[0] == "show")
	case "remote":
		if len(rest) == 0 {
			return true
		}
		if len(rest) == 1 && (rest[0] == "-v" || rest[0] == "--verbose") {
			return true
		}
		switch rest[0] {
		case "show", "get-url":
			return true
		}
		return false
	case "config":
		return readOnlyConfig(rest)
	case "worktree":
		return len(rest) > 0 && rest[0] == "list"
	case "submodule":
		return len(rest) > 0 && rest[0] == "status"
	case "lfs":
		if len(rest) == 0 {
			return false
		}
		switch rest[0] {
		case "version", "ls-files", "env":
			return true
		}
		return false
	case "reflog":
		return len(rest) > 0 && (rest[0] == "show" || rest[0] == "exists")
	case "hash-object":
		for _, a := range rest {
			if a == "-w" || a == "--write" {
				return false
			}
		}
		return true
	}
	return false
}

func gitSubcommand(args []string) (string, []string, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--version":
			return "--version", nil, true
		case a == "-c" || a == "-C" || a == "--git-dir" || a == "--work-tree" || a == "--exec-path" || a == "--namespace":
			i++
		case strings.HasPrefix(a, "-c") && len(a) > 2:
		case strings.HasPrefix(a, "-C") && len(a) > 2:
		case strings.HasPrefix(a, "--git-dir="), strings.HasPrefix(a, "--work-tree="):
		case strings.HasPrefix(a, "-"):
			// A global flag without a value (-p, --no-pager, ...).
		default:
			return a, args[i+1:], true
		}
	}
	return "", nil, false
}

func countPositionals(args []string) int {
	n := 0
	for _, a := range args {
		if !strings.HasPrefix(a, "-") && a != "" {
			n++
		}
	}
	return n
}

func readOnlyBranch(rest []string) bool {
	if len(rest) == 0 {
		return true
	}
	read := false
	for _, a := range rest {
		switch a {
		case "-l", "--list", "--show-current", "--contains", "--no-contains",
			"--merged", "--no-merged", "--points-at", "--format", "-a", "-r",
			"-v", "-vv", "--verbose":
			read = true
		case "-d", "-D", "-m", "-M", "-c", "-C", "-f", "--delete", "--move",
			"--copy", "--force", "--set-upstream-to", "-u", "--unset-upstream",
			"--edit-description":
			return false
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		if !read {
			return false
		}
	}
	return read
}

func readOnlyTag(rest []string) bool {
	if len(rest) == 0 {
		return true
	}
	read := false
	for _, a := range rest {
		switch a {
		case "-l", "--list", "-n", "--contains", "--merged", "--no-merged",
			"--points-at", "--format", "-v", "--verify":
			read = true
		case "-d", "--delete", "-a", "--annotate", "-s", "--sign", "-f",
			"--force", "-m", "--message", "-F", "--file", "-u", "--local-user",
			"--cleanup", "--create-reflog":
			return false
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		if !read {
			return false
		}
	}
	return read
}

func readOnlyConfig(rest []string) bool {
	read := false
	for _, a := range rest {
		switch a {
		case "--get", "--get-all", "--get-regexp", "--list", "-l":
			read = true
		case "--add", "--replace-all", "--unset", "--unset-all", "--rename-section",
			"--remove-section", "--edit", "-e", "--set":
			return false
		}
	}
	return read
}

// readOnlyRemote refuses a write on an SSH repository before any local file
// or process is touched. The command allowlist catches the process boundary;
// this catches the methods that prepare local temporary files first.
func readOnlyRemote(repo domain.Repo, what string) error {
	if !repo.IsSSH() {
		return nil
	}
	return &Error{Kind: ErrReadOnlyRemote, Args: []string{what}, Stderr: "remote repositories are read-only in Viff"}
}
