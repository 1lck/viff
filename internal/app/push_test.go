package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// remoteRef reads a ref from the bare remote, returning "" when it is missing.
func (f *fixture) remoteRef(remote, ref string) string {
	f.t.Helper()
	cmd := exec.Command("git", "--git-dir", remote, "rev-parse", "--verify", "--quiet", ref)
	cmd.Env = append(os.Environ(), f.env...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// lastPushNotice returns the most recent notification.
func (f *fixture) lastPushNotice() Notification {
	f.t.Helper()
	if len(f.w.Notifications) == 0 {
		f.t.Fatal("no notifications")
	}
	return f.w.Notifications[len(f.w.Notifications)-1]
}

// TestPushUpToHeadToAndRejection covers the three push variants against a real
// bare remote: an older commit, HEAD under another name, and Git's rejection
// of a non-fast-forward.
func TestPushUpToHeadToAndRejection(t *testing.T) {
	f, bare, other := remoteRepo(t)
	f.write("b.txt", "b\n")
	f.commitAll("B")
	b := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	f.write("c.txt", "c\n")
	f.commitAll("C")
	c := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	f.open()

	// Push the older commit B to the remote's main; C stays local.
	f.w.PushUpTo("origin", "main", b, false)
	f.sync()
	if got := f.remoteRef(bare, "refs/heads/main"); got != b {
		t.Fatalf("remote main = %q, want %q", got, b)
	}
	if n := f.lastPushNotice(); n.Kind != NoticeSuccess || !strings.HasPrefix(n.Title, "Pushed") {
		t.Fatalf("success notice = %+v", n)
	}

	// Move the remote forward from the other clone.
	f.gitIn(other, "fetch", "-q")
	f.gitIn(other, "reset", "-q", "--hard", "origin/main")
	if err := os.WriteFile(filepath.Join(other, "other.txt"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.gitIn(other, "add", "-A")
	f.gitIn(other, "commit", "-q", "-m", "Other's work")
	f.gitIn(other, "push", "-q", "origin", "main")
	remoteHead := strings.TrimSpace(f.gitIn(other, "rev-parse", "HEAD"))

	// The same push is now a non-fast-forward and must stay rejected.
	f.w.PushUpTo("origin", "main", b, false)
	f.sync()
	if got := f.remoteRef(bare, "refs/heads/main"); got != remoteHead {
		t.Fatalf("rejected push moved the remote to %q", got)
	}
	n := f.lastPushNotice()
	if n.Kind != NoticeError || n.Title != "Push rejected" {
		t.Fatalf("rejection notice = %+v", n)
	}
	if !strings.Contains(n.Body, "The remote has commits you do not have") {
		t.Fatalf("rejection body = %q", n.Body)
	}
	if !strings.Contains(n.Body, "non-fast-forward") && !strings.Contains(n.Body, "rejected") {
		t.Fatalf("Git's message missing from %q", n.Body)
	}

	// HEAD under another branch name.
	f.w.PushHeadTo("origin", "review", false)
	f.sync()
	if got := f.remoteRef(bare, "refs/heads/review"); got != c {
		t.Fatalf("remote review = %q, want %q", got, c)
	}

	// A force-with-lease against stale tracking information is refused.
	f.w.PushUpTo("origin", "main", c, true)
	f.sync()
	n = f.lastPushNotice()
	if n.Kind != NoticeError || n.Title != "Push rejected" {
		t.Fatalf("stale lease notice = %+v", n)
	}
	if !strings.Contains(n.Body, "force push was refused") {
		t.Fatalf("stale lease body = %q", n.Body)
	}
	if got := f.remoteRef(bare, "refs/heads/main"); got != remoteHead {
		t.Fatalf("stale force push moved the remote to %q", got)
	}

	// After a fetch the lease matches and the same force push succeeds.
	f.w.Fetch(false)
	f.sync()
	f.w.PushUpTo("origin", "main", c, true)
	f.sync()
	if got := f.remoteRef(bare, "refs/heads/main"); got != c {
		t.Fatalf("remote main after force = %q, want %q", got, c)
	}
	if rs := f.w.Repo; rs.Status.Branch.Ahead != 0 {
		t.Fatalf("branch still ahead after push: %+v", rs.Status.Branch)
	}
}

// TestDeleteRemoteTagCommand deletes the tag on the remote and keeps the
// local one.
func TestDeleteRemoteTagCommand(t *testing.T) {
	f, bare, _ := remoteRepo(t)
	f.git("tag", "v1")
	f.open()

	f.w.PushTag("origin", "v1")
	f.sync()
	if got := f.remoteRef(bare, "refs/tags/v1"); got == "" {
		t.Fatal("tag was not pushed")
	}

	f.w.DeleteRemoteTag("origin", "v1")
	f.sync()
	if got := f.remoteRef(bare, "refs/tags/v1"); got != "" {
		t.Fatalf("remote tag still present at %q", got)
	}
	if out := strings.TrimSpace(f.git("tag", "-l", "v1")); out != "v1" {
		t.Fatalf("local tag removed too: %q", out)
	}
	if n := f.lastPushNotice(); n.Kind != NoticeSuccess || n.Title != "Deleted tag v1 on origin" {
		t.Fatalf("delete notice = %+v", n)
	}
}
