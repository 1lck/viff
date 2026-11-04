package gitcli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitIn runs git in another directory, such as a second clone of the remote.
func (tr *testRepo) gitIn(dir string, args ...string) string {
	tr.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), tr.r.Env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		tr.t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// remoteRef reads a ref from a bare repository, returning "" when it is gone.
func (tr *testRepo) remoteRef(remote, ref string) string {
	tr.t.Helper()
	out, err := tr.gitMayFail("--git-dir", remote, "rev-parse", "--verify", "--quiet", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// bareRemote creates an empty bare repository next to the test repository.
func (tr *testRepo) bareRemote() string {
	tr.t.Helper()
	dir := filepath.Join(tr.t.TempDir(), "remote.git")
	tr.git("init", "-q", "--bare", "-b", "main", dir)
	return dir
}

// cloneRemote clones a bare repository into another temporary directory so a
// test can move the remote forward behind the working repository's back.
func (tr *testRepo) cloneRemote(remote string) string {
	tr.t.Helper()
	dir := filepath.Join(tr.t.TempDir(), "other")
	tr.git("clone", "-q", remote, dir)
	tr.gitIn(dir, "config", "user.name", "Other")
	tr.gitIn(dir, "config", "user.email", "other@example.com")
	tr.gitIn(dir, "config", "commit.gpgsign", "false")
	return dir
}

func (tr *testRepo) revParse(rev string) string {
	tr.t.Helper()
	return strings.TrimSpace(tr.git("rev-parse", rev))
}

// TestPushRefUpToCommit pushes an older commit and later fast-forwards it.
func TestPushRefUpToCommit(t *testing.T) {
	tr := newTestRepo(t)
	remote := tr.bareRemote()
	tr.git("remote", "add", "origin", remote)

	tr.write("a.txt", "one\n")
	tr.commitAll("A")
	a := tr.revParse("HEAD")
	tr.write("a.txt", "one\ntwo\n")
	tr.commitAll("B")
	b := tr.revParse("HEAD")

	ctx := context.Background()
	if err := tr.r.PushRef(ctx, tr.repo, "origin", a, "main", false, nil); err != nil {
		t.Fatalf("push up to A: %v", err)
	}
	if got := tr.remoteRef(remote, "refs/heads/main"); got != a {
		t.Fatalf("remote main = %q, want %q", got, a)
	}

	// The same primitive fast-forwards an existing remote branch.
	if err := tr.r.PushRef(ctx, tr.repo, "origin", b, "main", false, nil); err != nil {
		t.Fatalf("push up to B: %v", err)
	}
	if got := tr.remoteRef(remote, "refs/heads/main"); got != b {
		t.Fatalf("remote main = %q, want %q", got, b)
	}
}

// TestPushRefOtherBranchName pushes HEAD to a branch that does not exist yet.
func TestPushRefOtherBranchName(t *testing.T) {
	tr := newTestRepo(t)
	remote := tr.bareRemote()
	tr.git("remote", "add", "origin", remote)
	tr.write("a.txt", "one\n")
	tr.commitAll("A")
	head := tr.revParse("HEAD")

	if err := tr.r.PushRef(context.Background(), tr.repo, "origin", "HEAD", "other", false, nil); err != nil {
		t.Fatalf("push HEAD to other: %v", err)
	}
	if got := tr.remoteRef(remote, "refs/heads/other"); got != head {
		t.Fatalf("remote other = %q, want %q", got, head)
	}
}

// TestPushRefRejectsNonFastForward keeps Git's own rejection and never forces.
func TestPushRefRejectsNonFastForward(t *testing.T) {
	tr := newTestRepo(t)
	remote := tr.bareRemote()
	tr.git("remote", "add", "origin", remote)

	tr.write("f.txt", "1\n")
	tr.commitAll("A")
	a := tr.revParse("HEAD")
	tr.write("f.txt", "1\n2\n")
	tr.commitAll("B")
	b := tr.revParse("HEAD")
	ctx := context.Background()
	if err := tr.r.PushRef(ctx, tr.repo, "origin", b, "main", false, nil); err != nil {
		t.Fatalf("initial push: %v", err)
	}

	// Move the remote forward from another clone.
	other := tr.cloneRemote(remote)
	if err := os.WriteFile(filepath.Join(other, "f.txt"), []byte("1\n2\n3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr.gitIn(other, "add", "-A")
	tr.gitIn(other, "commit", "-q", "-m", "C")
	tr.gitIn(other, "push", "-q", "origin", "main")
	c := strings.TrimSpace(tr.gitIn(other, "rev-parse", "HEAD"))

	err := tr.r.PushRef(ctx, tr.repo, "origin", a, "main", false, nil)
	if err == nil {
		t.Fatal("pushing an older commit should be rejected")
	}
	if KindOf(err) != ErrRejected {
		t.Fatalf("kind = %v, want ErrRejected: %v", KindOf(err), err)
	}
	if msg := err.Error(); !strings.Contains(msg, "non-fast-forward") && !strings.Contains(msg, "rejected") {
		t.Fatalf("Git's rejection missing from %q", msg)
	}
	if got := tr.remoteRef(remote, "refs/heads/main"); got != c {
		t.Fatalf("rejected push moved the remote to %q", got)
	}

	// Without a fetch the lease has stale information, so even force is refused.
	err = tr.r.PushRef(ctx, tr.repo, "origin", b, "main", true, nil)
	if err == nil || KindOf(err) != ErrRejected {
		t.Fatalf("stale force-with-lease = %v (kind %v), want ErrRejected", err, KindOf(err))
	}
	if got := tr.remoteRef(remote, "refs/heads/main"); got != c {
		t.Fatalf("stale force push moved the remote to %q", got)
	}

	// After fetching, the lease matches and the force push goes through.
	if err := tr.r.Fetch(ctx, tr.repo, "origin", true, nil); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := tr.r.PushRef(ctx, tr.repo, "origin", b, "main", true, nil); err != nil {
		t.Fatalf("force-with-lease after fetch: %v", err)
	}
	if got := tr.remoteRef(remote, "refs/heads/main"); got != b {
		t.Fatalf("remote main = %q, want %q", got, b)
	}
}

// TestDeleteRemoteTag deletes only the remote tag and keeps the local one.
func TestDeleteRemoteTag(t *testing.T) {
	tr := newTestRepo(t)
	remote := tr.bareRemote()
	tr.git("remote", "add", "origin", remote)
	tr.write("a.txt", "one\n")
	tr.commitAll("A")
	tr.git("tag", "v1")
	ctx := context.Background()

	if err := tr.r.PushTag(ctx, tr.repo, "origin", "v1", nil); err != nil {
		t.Fatalf("push tag: %v", err)
	}
	if got := tr.remoteRef(remote, "refs/tags/v1"); got == "" {
		t.Fatal("tag not on the remote")
	}
	if err := tr.r.DeleteRemoteTag(ctx, tr.repo, "origin", "v1", nil); err != nil {
		t.Fatalf("delete remote tag: %v", err)
	}
	if got := tr.remoteRef(remote, "refs/tags/v1"); got != "" {
		t.Fatalf("remote tag still present at %q", got)
	}
	if out := tr.git("tag", "-l", "v1"); strings.TrimSpace(out) != "v1" {
		t.Fatalf("local tag removed too: %q", out)
	}

	// A remote that cannot be reached keeps Git's error text.
	err := tr.r.DeleteRemoteTag(ctx, tr.repo, "missing-remote", "v1", nil)
	if err == nil {
		t.Fatal("deleting on an unknown remote should fail")
	}
	if msg := err.Error(); !strings.Contains(msg, "does not appear to be a git repository") && !strings.Contains(msg, "Could not read") {
		t.Fatalf("Git's error missing from %q", msg)
	}
}
