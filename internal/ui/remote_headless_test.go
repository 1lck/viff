//go:build vivahead

package viffui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/sshpath"
)

// uiLocalSSH maps the remote identity to the fixture checkout. The window,
// the application layer and the real Git adapter all run; only ssh is
// replaced, so this test exercises the read-only path on every platform.
type uiLocalSSH struct {
	dir string
	env []string
}

func (l *uiLocalSSH) Command(ctx context.Context, _ sshpath.Repo, _ string, args []string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", l.dir}, args...)...)
	cmd.Env = append(os.Environ(), l.env...)
	return cmd, nil
}

func (l *uiLocalSSH) ReadFile(_ context.Context, _ domain.Repo, rel string) (gitcli.Content, error) {
	full := filepath.Join(l.dir, filepath.FromSlash(rel))
	st, err := os.Stat(full)
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

func (l *uiLocalSSH) Operation(context.Context, domain.Repo) (domain.Operation, error) {
	return domain.Operation{}, nil
}

func (l *uiLocalSSH) IndexRevision(_ context.Context, _ domain.Repo, _ string) (string, error) {
	st, err := os.Stat(filepath.Join(l.dir, ".git", "index"))
	if os.IsNotExist(err) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(st.ModTime().UnixNano(), 10), nil
}

// TestRemoteRepositoryReadOnly opens a repository through the SSH path with
// the system ssh replaced by a local transport, then checks what the window
// promises: status, refs, a readable file, no editor and no write.
func TestRemoteRepositoryReadOnly(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	tr := &uiLocalSSH{dir: f.dir, env: f.env}
	runner := &gitcli.Runner{Env: f.env, SSH: tr}
	f.ws.Git = runner
	f.ws.Caps = vapp.CapabilitiesOf(runner)
	root, err := sshpath.URLOf("ssh://test@example/", f.dir)
	if err != nil {
		t.Fatal(err)
	}
	f.win.openRepo(root)
	f.settle()

	rs := f.ws.Repo
	if rs == nil || !rs.Repo.IsSSH() || !f.win.readOnly() {
		t.Fatalf("repository is not read-only: %+v", rs)
	}
	if len(rs.Status.Entries) == 0 {
		t.Fatal("remote status is empty")
	}
	if len(rs.Refs) == 0 {
		t.Fatal("remote refs are empty")
	}
	if !f.s.HasText("SSH · read-only") {
		t.Fatal("status bar does not show the read-only SSH badge")
	}
	f.win.openRemote()
	f.win.Invalidate()
	f.s.Settle(20*time.Millisecond, 2*time.Second)
	if !f.s.HasText("Open Remote Repository") || !f.s.HasText("Read-only over SSH.") {
		t.Fatal("remote dialog did not show its title and read-only note")
	}
	f.win.closeDialog()
	f.win.Invalidate()
	f.s.Settle(20*time.Millisecond, 2*time.Second)
	if len(f.ws.Settings.Recent) == 0 || f.ws.Settings.Recent[0].Path != rs.Repo.Root {
		t.Fatalf("recent = %+v", f.ws.Settings.Recent)
	}

	f.win.openFileView("README.md", "", false)
	f.settleTab()
	tab := f.win.ed.cur()
	if tab == nil || tab.res == nil || !tab.res.New.Content.Exists {
		t.Fatalf("remote file did not load: %+v", tab)
	}
	if !strings.Contains(string(tab.res.New.Content.Data), "Viff demo") {
		t.Fatalf("remote file content = %q", tab.res.New.Content.Data)
	}
	if f.win.canEdit(tab) {
		t.Fatal("the editor is offered on a read-only remote file")
	}

	called := false
	f.ws.Run("Test remote write", func(context.Context, domain.Repo, func(string)) error {
		called = true
		return nil
	}, vapp.RunOptions{})
	f.s.Settle(30*time.Millisecond, 2*time.Second)
	if called {
		t.Fatal("a write ran on a read-only remote repository")
	}

	if !remoteURLValid("dev", "/srv/app") {
		t.Fatal("a valid remote host/path was rejected")
	}
	if remoteURLValid("dev", "relative") || remoteURLValid("-bad", "/srv/app") {
		t.Fatal("invalid remote fields were accepted")
	}
	f.assertIdle()
}
