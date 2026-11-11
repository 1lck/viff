package gitcli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"viff/internal/domain"
	"viff/internal/sshpath"
)

// localSSH is the test transport: it runs the remote Git command against a
// local repository and reads files from that checkout. It exercises every
// layer above the process boundary without a network or an SSH server.
type localSSH struct {
	dir string
	env []string
}

func (l *localSSH) Command(ctx context.Context, _ sshpath.Repo, _ string, args []string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", l.dir}, args...)...)
	cmd.Env = append(os.Environ(), l.env...)
	return cmd, nil
}

func (l *localSSH) ReadFile(_ context.Context, _ domain.Repo, rel string) (Content, error) {
	return readWorktree(filepath.Join(l.dir, filepath.FromSlash(rel)))
}

func (l *localSSH) IndexRevision(_ context.Context, _ domain.Repo, _ string) (string, error) {
	st, err := os.Stat(filepath.Join(l.dir, ".git", "index"))
	if os.IsNotExist(err) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(st.ModTime().UnixNano(), 10), nil
}

func (l *localSSH) Operation(context.Context, domain.Repo) (domain.Operation, error) {
	return domain.Operation{}, nil
}

func remoteRepo(t *testing.T, dir string) string {
	t.Helper()
	u, err := sshpath.URLOf("ssh://test@example/", dir)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestRemoteReadOnlyAllowlist(t *testing.T) {
	allowed := [][]string{
		{"status", "--porcelain=v2", "-z"},
		{"-c", "core.quotePath=false", "log", "-1"},
		{"--literal-pathspecs", "ls-files", "-z"},
		{"rev-parse", "--show-toplevel"},
		{"for-each-ref", "--format=%(refname)"},
		{"symbolic-ref", "--quiet", "--short", "HEAD"},
		{"branch", "--show-current"},
		{"branch", "-a"},
		{"tag", "-l"},
		{"stash", "list"},
		{"remote", "-v"},
		{"config", "-z", "-f", ".gitmodules", "--get-regexp", "x"},
		{"worktree", "list", "--porcelain", "-z"},
		{"submodule", "status", "--recursive"},
		{"lfs", "ls-files", "--name-only", "-z"},
		{"reflog", "show", "-1"},
		{"grep", "-n", "-e", "x", "--"},
	}
	for _, args := range allowed {
		if !remoteReadCommand(args) {
			t.Errorf("remoteReadCommand(%q) = false, want true", args)
		}
	}
	denied := [][]string{
		{"commit", "-m", "x"},
		{"fetch", "origin"},
		{"push", "origin", "main"},
		{"branch", "-D", "x"},
		{"branch", "new"},
		{"tag", "v1"},
		{"stash", "push"},
		{"remote", "add", "o", "u"},
		{"remote", "-v", "add", "o", "u"},
		{"config", "--unset", "x"},
		{"config", "-z", "user.name", "x"},
		{"config", "--show-origin", "user.name", "x"},
		{"symbolic-ref", "--delete", "HEAD"},
		{"worktree", "add", "/tmp/x", "main"},
		{"submodule", "update"},
		{"lfs", "track", "*.bin"},
		{"symbolic-ref", "HEAD", "refs/heads/x"},
		{"clean", "-f"},
		{"diff", "--output=patch.txt"},
		{"log", "--output=log.txt"},
		{"grep", "-O", "pattern"},
		{"grep", "--open-files-in-pager=less", "pattern"},
		{"cat-file", "--filters", "HEAD:file"},
		{"diff", "--ext-diff"},
		{"show", "--textconv", "HEAD"},
		{"help", "-a"},
		{"update-index", "--refresh"},
		{"gc"},
	}
	for _, args := range denied {
		if remoteReadCommand(args) {
			t.Errorf("remoteReadCommand(%q) = true, want false", args)
		}
	}
}

func TestRemoteGitCommandQuotes(t *testing.T) {
	got := remoteGitCommand("/srv/my app", []string{"status", "--", "a b", "x'y"})
	for _, want := range []string{"env GIT_TERMINAL_PROMPT=0", "git -C '/srv/my app'", "'status'", "'a b'", `'x'\''y'`} {
		if !strings.Contains(got, want) {
			t.Errorf("remote command %q does not contain %q", got, want)
		}
	}
}

func TestDiscoverAndReadThroughRemoteTransport(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	env := []string{"HOME=" + filepath.Join(dir, "home"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(dir, "gitconfig")}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "Viff Test")
	run("config", "user.email", "test@example.com")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("remote hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-q", "-m", "first")

	tr := &localSSH{dir: dir, env: env}
	r := &Runner{Env: env, SSH: tr}
	root := remoteRepo(t, dir)
	repo, err := r.Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.IsSSH() || !sshpath.Is(repo.Root) {
		t.Fatalf("Discover returned %+v", repo)
	}
	status, err := r.Status(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Entries) != 0 || status.Branch.Branch != "main" {
		t.Fatalf("status = %+v", status)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err = r.Status(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Entries) != 1 || status.Entries[0].Path != "README.md" {
		t.Fatalf("status after edit = %+v", status)
	}
	c, err := r.Read(context.Background(), repo, RevWorktree, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(c.Data) != "changed\n" || !c.Exists {
		t.Fatalf("worktree content = %+v", c)
	}
	if _, err := r.Run(context.Background(), repo.Root, "commit", "-m", "nope"); KindOf(err) != ErrReadOnlyRemote {
		t.Fatalf("write on a remote repository returned %v, want ErrReadOnlyRemote", err)
	}
}

func TestParseRemoteContent(t *testing.T) {
	c, err := parseRemoteContent("a.txt", []byte("F 7 100644\nhello\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Exists || c.Size != 7 || c.Mode != "100644" || string(c.Data) != "hello\n\n" || c.Binary {
		t.Fatalf("content = %+v", c)
	}
	c, err = parseRemoteContent("link", []byte("L\n../target\n"))
	if err != nil || !c.Symlink || string(c.Data) != "../target" || c.Mode != "120000" {
		t.Fatalf("symlink = %+v, %v", c, err)
	}
	c, err = parseRemoteContent("dir", []byte("D\n"))
	if err != nil || !c.Exists || !c.Submodule {
		t.Fatalf("directory = %+v, %v", c, err)
	}
	c, err = parseRemoteContent("missing", []byte("M\n"))
	if err != nil || c.Exists {
		t.Fatalf("missing = %+v, %v", c, err)
	}
	if _, err := parseRemoteContent("bad", []byte("nonsense")); err == nil {
		t.Fatal("malformed reply was accepted")
	}
}

func TestParseRemoteOperation(t *testing.T) {
	flags := []string{"0", "0", "0", "1", "0", "0", "0", "0", "0"}
	values := make([]string, 14)
	values[9] = "abc123\n"
	values[10] = "Merge branch 'x'\n"
	out := strings.Join(append(flags, values...), "\x00") + "\x00"
	op, err := parseRemoteOperation([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != domain.OpMerge || op.Incoming != "abc123" || op.MergeMessage != "Merge branch 'x'" {
		t.Fatalf("operation = %+v", op)
	}

	flags = []string{"1", "0", "0", "0", "0", "0", "0", "1", "1"}
	values = make([]string, 14)
	values[0] = "refs/heads/topic\n"
	values[1] = "deadbeef\n"
	values[2] = "2\n"
	values[3] = "5\n"
	values[13] = "cafebabe\n"
	out = strings.Join(append(flags, values...), "\x00") + "\x00"
	op, err = parseRemoteOperation([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != domain.OpRebase || op.HeadName != "topic" || op.Step != 2 || op.Total != 5 || !op.Interactive || op.Incoming != "cafebabe" {
		t.Fatalf("rebase = %+v", op)
	}
}

func TestRemoteScriptsWithLocalShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the remote script is POSIX sh; Windows tests use the transport seam")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "a b.txt")
	if err := os.WriteFile(file, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(path string) []byte {
		t.Helper()
		cmd := exec.Command("sh", "-c", remoteFileScript(path), "sh", path)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("script %s: %v", path, err)
		}
		return out
	}
	c, err := parseRemoteContent("a b.txt", run(file))
	if err != nil || string(c.Data) != "hello\n" || !c.Exists {
		t.Fatalf("file = %+v, %v", c, err)
	}
	c, err = parseRemoteContent("sub", run(filepath.Join(dir, "sub")))
	if err != nil || !c.Submodule {
		t.Fatalf("dir = %+v, %v", c, err)
	}
	c, err = parseRemoteContent("missing", run(filepath.Join(dir, "missing")))
	if err != nil || c.Exists {
		t.Fatalf("missing = %+v, %v", c, err)
	}
}

func TestClassifySSH(t *testing.T) {
	cases := []struct {
		stderr string
		exit   int
		want   ErrorKind
		ok     bool
	}{
		{"Host key verification failed.", 255, ErrSSHHostKey, true},
		{"Permission denied (publickey).", 255, ErrSSHAuth, true},
		{"ssh: connect to host dev port 22: Connection refused", 255, ErrSSHConnect, true},
		{"Could not resolve hostname dev: Name or service not known", 255, ErrSSHConnect, true},
		{"bash: git: command not found", 127, ErrSSHGitMissing, true},
		{"fatal: not a git repository", 128, ErrOther, false},
		{"Permission denied (publickey).", 128, ErrOther, false},
	}
	for _, c := range cases {
		got, ok := classifySSH(c.stderr, "", c.exit)
		if got != c.want || ok != c.ok {
			t.Errorf("classifySSH(%q, %d) = %v, %v; want %v, %v", c.stderr, c.exit, got, ok, c.want, c.ok)
		}
	}
}

func TestRemoteOperationScriptWithLocalShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the remote script is POSIX sh; Windows tests use the transport seam")
	}
	dir := t.TempDir()
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("MERGE_HEAD", "abc123\n")
	write("MERGE_MSG", "Merge branch 'topic'\n")
	cmd := exec.Command("sh", "-c", remoteOperationScript(), "sh", dir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	op, err := parseRemoteOperation(out)
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != domain.OpMerge || op.Incoming != "abc123" || op.MergeMessage != "Merge branch 'topic'" {
		t.Fatalf("operation = %+v", op)
	}

	os.Remove(filepath.Join(dir, "MERGE_HEAD"))
	os.Remove(filepath.Join(dir, "MERGE_MSG"))
	if err := os.MkdirAll(filepath.Join(dir, "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rebase-merge", "head-name"), []byte("refs/heads/topic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rebase-merge", "onto"), []byte("deadbeef\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rebase-merge", "msgnum"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rebase-merge", "end"), []byte("5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rebase-merge", "interactive"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "REBASE_HEAD"), []byte("cafebabe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("sh", "-c", remoteOperationScript(), "sh", dir)
	out, err = cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	op, err = parseRemoteOperation(out)
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != domain.OpRebase || op.HeadName != "topic" || op.Step != 2 || op.Total != 5 || !op.Interactive || op.Incoming != "cafebabe" {
		t.Fatalf("rebase = %+v", op)
	}
}

func TestSSHArgs(t *testing.T) {
	cases := []struct {
		name   string
		target sshpath.Repo
		want   []string
	}{
		{
			name:   "alias default port",
			target: sshpath.Repo{User: "me", Host: "dev", Path: "/srv/app"},
			want:   []string{"me@dev", "cmd"},
		},
		{
			name:   "ipv6 raw host and port",
			target: sshpath.Repo{User: "me", Host: "::1", Port: 2222, Path: "/srv/app"},
			want:   []string{"-p", "2222", "me@::1", "cmd"},
		},
		{
			name:   "ipv6 without user",
			target: sshpath.Repo{Host: "2001:db8::1", Path: "/"},
			want:   []string{"2001:db8::1", "cmd"},
		},
	}
	for _, c := range cases {
		args := sshArgs(c.target, "cmd")
		joined := strings.Join(args, " ")
		for _, want := range c.want {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: args %q do not contain %q", c.name, joined, want)
			}
		}
		if strings.Contains(joined, "[") || strings.Contains(joined, "]") {
			t.Errorf("%s: brackets must not reach ssh: %q", c.name, joined)
		}
	}
}

func TestSSHAuthClassificationWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell to produce an exit status")
	}
	cmd := exec.Command("sh", "-c", "exit 255")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected an exit error")
	}
	r := &Runner{}
	got := r.wrap(context.Background(), "ssh://me@dev/srv/app", []string{"status"}, err,
		"Permission denied (publickey).", "")
	if KindOf(got) != ErrSSHAuth {
		t.Fatalf("classify = %v, want ErrSSHAuth (%v)", KindOf(got), got)
	}
}

func TestParseRemoteContentLargeAndDirectory(t *testing.T) {
	large, err := parseRemoteContent("big.bin", []byte("F 9000000 100644\npreview"))
	if err != nil {
		t.Fatal(err)
	}
	if !large.Exists || !large.TooLarge || large.Data != nil || large.Size != 9000000 {
		t.Fatalf("large content = %+v", large)
	}
	dir, err := parseRemoteContent("sub", []byte("D\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !dir.Exists || !dir.Submodule || dir.Mode != "160000" {
		t.Fatalf("directory content = %+v", dir)
	}
}

func TestSafeObjectID(t *testing.T) {
	for _, good := range []string{"abcd", "0123456789abcdef0123456789abcdef01234567"} {
		if !safeObjectID(good) {
			t.Errorf("safeObjectID(%q) = false", good)
		}
	}
	for _, bad := range []string{"", "abc", "--output=/tmp/x", "abcd ", "HEAD", "refs/heads/main", "xyz1"} {
		if safeObjectID(bad) {
			t.Errorf("safeObjectID(%q) = true", bad)
		}
	}
}

func TestRemoteExecutableLabel(t *testing.T) {
	r := &Runner{}
	got, err := r.CachedExecutableIn("ssh://me@dev:2222/srv/app")
	if err != nil {
		t.Fatal(err)
	}
	if got != "git (on dev)" {
		t.Fatalf("CachedExecutableIn = %q", got)
	}
	got, err = r.ExecutableIn(context.Background(), "ssh://me@dev/srv/app")
	if err != nil || got != "git (on dev)" {
		t.Fatalf("ExecutableIn = %q, %v", got, err)
	}
}
