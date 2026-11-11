//go:build vivahead

package viffui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	"viff/internal/domain"
)

// uiMergeConflict leaves the repository stopped on a real merge of "other"
// into the current branch with one conflicted file.
func uiMergeConflict(t *testing.T, f *uiFixture, path, base, ours, theirs string) {
	t.Helper()
	f.write(path, base)
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base "+path)
	f.git("checkout", "-q", "-b", "other")
	f.write(path, theirs)
	f.git("commit", "-q", "-am", "theirs")
	f.git("checkout", "-q", "main")
	f.write(path, ours)
	f.git("commit", "-q", "-am", "ours")
	cmd := exec.Command("git", "merge", "other")
	cmd.Dir = f.dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("merge unexpectedly succeeded:\n%s", out)
	}
}

// conflictWait bounds the condition waits below. The work they wait for takes
// milliseconds, but a shared Windows runner can stall for seconds: a 20s
// ceiling once expired on CI while every other shard passed. The ceiling is
// raised rather than the condition relaxed, so a real regression still fails.
const conflictWait = 45 * time.Second

// uiWaitConflicts waits until the workspace has the unmerged entry.
func uiWaitConflicts(t *testing.T, f *uiFixture) {
	t.Helper()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && len(f.ws.Repo.Entries(domain.AreaConflicts)) > 0
	}, conflictWait); err != nil {
		t.Fatalf("conflict never appeared: %v", err)
	}
}

// uiWaitFile waits until the working file has want, then for the tab to reload,
// so assertions see the state after the write rather than the frame before it.
func uiWaitFile(t *testing.T, f *uiFixture, name, want string) {
	t.Helper()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(name)))
		return err == nil && string(data) == want
	}, conflictWait); err != nil {
		t.Fatalf("%s never became %q: %v", name, want, err)
	}
	f.settleTab()
}

// TestConflictResolveBinaryHasNoControls checks that a format the resolver
// cannot handle is offered no accept controls instead of being mis-parsed.
func TestConflictResolveBinaryHasNoControls(t *testing.T) {
	f := newUIFixture(t)
	f.write("b.bin", "\x00base")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base bin")
	f.git("checkout", "-q", "-b", "other")
	f.write("b.bin", "\x00theirs")
	f.git("commit", "-q", "-am", "theirs bin")
	f.git("checkout", "-q", "main")
	f.write("b.bin", "\x00ours")
	f.git("commit", "-q", "-am", "ours bin")
	cmd := exec.Command("git", "merge", "other")
	cmd.Dir = f.dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("binary merge unexpectedly succeeded:\n%s", out)
	}
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	f.click("b.bin")
	f.settleTab()
	if f.s.HasText("Accept Ours") || f.s.HasText("Accept Theirs") || f.s.HasText("Accept Both") {
		t.Fatalf("accept controls shown for a binary conflict:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	if data := f.read("b.bin"); string(data) != "\x00ours" {
		t.Fatalf("binary file = %q", data)
	}
}
