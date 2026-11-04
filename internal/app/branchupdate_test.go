package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
)

// gitInDir runs git in dir with the fixture's isolated environment.
func gitInDir(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// addRemote gives the fixture an origin with main and side pushed.
func (f *fixture) addRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "origin.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInDir(t, root, f.env, "init", "-q", "--bare", "-b", "main", remote)
	f.git("remote", "add", "origin", remote)
	f.git("push", "-q", "-u", "origin", "main")
	f.git("checkout", "-q", "-b", "side")
	f.write("side.txt", "side one\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "Side one")
	f.git("push", "-q", "-u", "origin", "side")
	f.git("checkout", "-q", "main")
	return remote
}

// advanceRemote pushes another commit to branch from a scratch checkout.
func (f *fixture) advanceRemote(t *testing.T, remote, branch, file, content, msg string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "push")
	gitInDir(t, filepath.Dir(dir), f.env, "clone", "-q", remote, dir)
	gitInDir(t, dir, f.env, "config", "user.name", "Other")
	gitInDir(t, dir, f.env, "config", "user.email", "other@example.com")
	gitInDir(t, dir, f.env, "config", "commit.gpgsign", "false")
	gitInDir(t, dir, f.env, "checkout", "-q", branch)
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInDir(t, dir, f.env, "add", "-A")
	gitInDir(t, dir, f.env, "commit", "-q", "-m", msg)
	tip := gitInDir(t, dir, f.env, "rev-parse", "HEAD")
	gitInDir(t, dir, f.env, "push", "-q", "origin", branch)
	return tip
}

func TestUpdateBranchFromRemote(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "Base")
	remote := f.addRemote(t)
	f.open()

	tip := f.advanceRemote(t, remote, "side", "side.txt", "side two\n", "Side two")
	old := strings.TrimSpace(f.git("rev-parse", "refs/heads/side"))

	f.w.UpdateBranch("origin", "side")
	f.settle(func() bool { return f.idle() })
	if got := strings.TrimSpace(f.git("rev-parse", "refs/heads/side")); got != tip {
		t.Fatalf("branch = %s, want %s", got, tip)
	}
	n := f.w.Notifications[len(f.w.Notifications)-1]
	if n.Kind != NoticeSuccess || n.Title != "Updated side" {
		t.Fatalf("notice = %+v", n)
	}
	if !strings.Contains(n.Body, "1 new commits") || !strings.Contains(n.Body, domain.ShortHash(old)) {
		t.Fatalf("notice body = %q", n.Body)
	}

	// Running it again reports that there is nothing to do.
	f.w.UpdateBranch("origin", "side")
	f.settle(func() bool { return f.idle() })
	n = f.w.Notifications[len(f.w.Notifications)-1]
	if n.Kind != NoticeSuccess || n.Title != "side is up to date" {
		t.Fatalf("second notice = %+v", n)
	}
}

func TestUpdateBranchRefusals(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "Base")
	remote := f.addRemote(t)
	f.open()

	// The current branch is updated by pulling, not here.
	f.w.UpdateBranch("origin", "main")
	f.settle(func() bool { return f.idle() })
	n := f.w.Notifications[len(f.w.Notifications)-1]
	if n.Kind != NoticeWarning || n.Title != "Cannot update main" {
		t.Fatalf("current branch notice = %+v", n)
	}

	// A diverged branch is refused and keeps its commit.
	f.git("checkout", "-q", "side")
	f.write("side.txt", "local work\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "Local side")
	local := strings.TrimSpace(f.git("rev-parse", "refs/heads/side"))
	f.git("checkout", "-q", "main")
	f.advanceRemote(t, remote, "side", "side.txt", "remote work\n", "Remote side")

	f.w.UpdateBranch("origin", "side")
	f.settle(func() bool { return f.idle() })
	n = f.w.Notifications[len(f.w.Notifications)-1]
	if n.Kind != NoticeWarning || n.Title != "Cannot fast-forward side" {
		t.Fatalf("diverged notice = %+v", n)
	}
	if got := strings.TrimSpace(f.git("rev-parse", "refs/heads/side")); got != local {
		t.Fatalf("branch moved to %s, want %s", got, local)
	}
}

func TestUnshallowRepositoryOnCompleteClone(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "Base")
	f.open()

	f.w.UnshallowRepository()
	f.settle(func() bool { return f.idle() })
	n := f.w.Notifications[len(f.w.Notifications)-1]
	if n.Kind != NoticeInfo || n.Title != "History is complete" {
		t.Fatalf("notice = %+v", n)
	}
}
