package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/sshpath"
	"viff/internal/storage"
)

// appLocalSSH is the test transport for the application layer. It maps the
// remote identity to a local checkout, so open, refresh and file reads use the
// real Git adapter while no ssh process runs.
type appLocalSSH struct {
	dir string
	env []string
}

func (l *appLocalSSH) Command(ctx context.Context, _ sshpath.Repo, _ string, args []string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", l.dir}, args...)...)
	cmd.Env = append(os.Environ(), l.env...)
	return cmd, nil
}

func (l *appLocalSSH) ReadFile(_ context.Context, _ domain.Repo, rel string) (gitcli.Content, error) {
	full := filepath.Join(l.dir, filepath.FromSlash(rel))
	st, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return gitcli.Content{}, nil
		}
		return gitcli.Content{}, err
	}
	if st.IsDir() {
		return gitcli.Content{Exists: true, Submodule: true}, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return gitcli.Content{}, err
	}
	return gitcli.Content{Exists: true, Data: data, Size: st.Size(), Mode: "100644"}, nil
}

func (l *appLocalSSH) Operation(context.Context, domain.Repo) (domain.Operation, error) {
	return domain.Operation{}, nil
}

func remoteIdentity(t *testing.T, dir string) string {
	t.Helper()
	u, err := sshpath.URLOf("ssh://test@example/", dir)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestSSHPathNormalizationAndIdentity(t *testing.T) {
	if got := canonicalPath("ssh://ME@DEV:22/srv/app/"); got != "ssh://ME@dev/srv/app" {
		t.Errorf("canonicalPath = %q", got)
	}
	if got := entryKey("ssh://ME@DEV:22/srv/app/"); got != "ssh://ME@dev/srv/app" {
		t.Errorf("entryKey = %q", got)
	}
	if !isRemotePath("ssh://dev/srv/app") {
		t.Error("isRemotePath(SSH) = false")
	}
	if !samePath("ssh://dev/srv/app", "ssh://DEV:22/srv/app") {
		t.Error("samePath did not normalize the remote identity")
	}
	if samePath("ssh://dev/srv/app", "/srv/app") {
		t.Error("samePath matched a remote URL against a local path")
	}
	if got := displayBase("ssh://dev/srv/app"); got != "app" {
		t.Errorf("displayBase = %q", got)
	}
}

func TestOpenRemoteRepositoryIsReadOnly(t *testing.T) {
	f := newFixture(t)
	f.write("base.txt", "base\n")
	f.git("add", "base.txt")
	f.git("commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(f.dir, "remote-change.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := &appLocalSSH{dir: f.dir, env: f.env}
	w := NewWorkspace(&gitcli.Runner{Env: f.env, SSH: tr}, storage.Defaults(), nil)
	t.Cleanup(w.Close)
	root := remoteIdentity(t, f.dir)
	opened := make(chan error, 1)
	w.Open(root, func(err error) { opened <- err })
	wait := time.Now().Add(20 * time.Second)
	for {
		w.Drain()
		rs := w.Repo
		if rs != nil && rs.StatusLoaded && rs.RefsLoaded && !rs.refreshing && len(w.Tasks) == 0 && !w.Pending() {
			break
		}
		if time.Now().After(wait) {
			t.Fatal("remote repository did not finish opening")
		}
		select {
		case err := <-opened:
			if err != nil {
				t.Fatal(err)
			}
			opened <- nil
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !w.ReadOnly() {
		t.Fatal("remote repository is not marked read-only")
	}
	rs := w.Repo
	if rs == nil || !rs.Repo.IsSSH() {
		t.Fatalf("repository = %+v", rs)
	}
	if len(rs.Status.Entries) == 0 {
		t.Fatal("remote status did not list the fixture changes")
	}
	if len(rs.Refs) == 0 {
		t.Fatal("remote refs were not loaded")
	}
	if len(w.Settings.Recent) == 0 || w.Settings.Recent[0].Path != rs.Repo.Root {
		t.Fatalf("recent projects = %+v", w.Settings.Recent)
	}

	called := false
	w.Run("Test write", func(context.Context, domain.Repo, func(string)) error {
		called = true
		return nil
	}, RunOptions{})
	w.Drain()
	if called {
		t.Fatal("a write task ran on a read-only remote repository")
	}
}

func TestRemoteStatusKeyChanges(t *testing.T) {
	a := domain.Status{Branch: domain.BranchStatus{Head: "abc", Branch: "main"}}
	b := a
	if remoteStatusKey(a) != remoteStatusKey(b) {
		t.Fatal("equal status produced different keys")
	}
	b.Entries = []domain.StatusEntry{{Path: "a.txt", Index: '.', Worktree: 'M'}}
	if remoteStatusKey(a) == remoteStatusKey(b) {
		t.Fatal("a changed entry did not change the key")
	}
	c := a
	c.Branch.Ahead = 1
	if remoteStatusKey(a) == remoteStatusKey(c) {
		t.Fatal("a changed branch counter did not change the key")
	}
}
