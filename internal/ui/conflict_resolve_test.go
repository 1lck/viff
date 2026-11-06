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

	vapp "viff/internal/app"
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

// TestConflictResolveAcceptsSideInApp drives the whole in-app path: open the
// conflicted file, accept one side, keep the index unmerged, then use the
// existing Mark as Resolved step and continue the merge.
func TestConflictResolveAcceptsSideInApp(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()

	f.click("conflict.txt")
	f.settleTab()
	if !f.s.HasText("Accept Ours") || !f.s.HasText("Accept Theirs") || !f.s.HasText("Accept Both") {
		t.Fatalf("resolve controls missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("conflict-before-accept")
	f.click("Accept Ours")
	uiWaitFile(t, f, "conflict.txt", "ours\n")
	// The view follows the disk: markers are gone, and the existing Mark as
	// Resolved step becomes available.
	if f.s.HasText("Accept Ours") {
		t.Fatal("resolve controls still shown after the markers were removed")
	}
	if !f.s.HasText("No conflict markers left in the file on disk") {
		t.Fatalf("resolved banner missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	// Nothing was staged by the resolution itself.
	status := f.git("status", "--porcelain")
	if !strings.Contains(status, "UU conflict.txt") {
		t.Fatalf("conflict was staged by the resolver:\n%s", status)
	}
	f.snap("conflict-resolved-not-staged")
	f.click("Mark as Resolved")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && len(f.ws.Repo.Entries(domain.AreaConflicts)) == 0
	}, conflictWait); err != nil {
		t.Fatalf("conflicts left after Mark as Resolved: %v", err)
	}
	f.settle()
	f.click("Continue")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.ws.Repo != nil && !f.ws.Repo.Operation.Active()
	}, conflictWait); err != nil {
		t.Fatalf("merge did not continue: %v (%+v)", err, f.ws.Repo.Operation)
	}
	f.settle()
	if parents := strings.Fields(f.git("log", "-1", "--format=%P")); len(parents) != 2 {
		t.Fatalf("merge commit parents = %v", parents)
	}
}

// TestConflictResolvePerBlockBoth keeps a CRLF file with two blocks intact and
// resolves them one at a time, navigating with the existing Next Conflict
// control.
func TestConflictResolvePerBlockBoth(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	text := "top\r\n" +
		"<<<<<<< ours\r\n" +
		"a-ours\r\n" +
		"=======\r\n" +
		"a-theirs\r\n" +
		">>>>>>> theirs\r\n" +
		"middle\r\n" +
		"<<<<<<< ours\r\n" +
		"b-ours\r\n" +
		"||||||| base\r\n" +
		"b-base\r\n" +
		"=======\r\n" +
		"b-theirs\r\n" +
		">>>>>>> theirs\r\n" +
		"tail"
	f.write("conflict.txt", text)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	f.click("conflict.txt")
	f.settleTab()

	if !f.s.HasText("Conflict 1 of 2: accept") {
		t.Fatalf("block counter missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	f.click("Next Conflict")
	if !f.s.HasText("Conflict 2 of 2: accept") {
		t.Fatalf("navigation did not select block 2:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("conflict-second-block")
	f.click("Accept Both")
	want := "top\r\n" +
		"<<<<<<< ours\r\n" +
		"a-ours\r\n" +
		"=======\r\n" +
		"a-theirs\r\n" +
		">>>>>>> theirs\r\n" +
		"middle\r\n" +
		"b-ours\r\n" +
		"b-theirs\r\n" +
		"tail"
	uiWaitFile(t, f, "conflict.txt", want)
	// The first block is untouched and is now the only one left.
	if !f.s.HasText("Conflict 1 of 1: accept") {
		t.Fatalf("remaining block not selected:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	f.click("Accept Ours")
	want = "top\r\na-ours\r\nmiddle\r\nb-ours\r\nb-theirs\r\ntail"
	uiWaitFile(t, f, "conflict.txt", want)
	if !f.s.HasText("No conflict markers left in the file on disk") {
		t.Fatalf("resolved banner missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}
}

// TestConflictResolveRefusesStaleView edits the file outside Viff after the
// view was loaded: the click must not overwrite the external edit, must warn,
// and the reloaded view must then resolve the file as it is on disk.
func TestConflictResolveRefusesStaleView(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	f.click("conflict.txt")
	f.settleTab()

	external := "<<<<<<< HEAD\nexternal edit\n=======\ntheirs\n>>>>>>> other\n"
	f.write("conflict.txt", external)
	notifications := len(f.ws.Notifications)
	f.click("Accept Ours")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return len(f.ws.Notifications) > notifications
	}, conflictWait); err != nil {
		t.Fatal("stale resolution was not reported")
	}
	if got := string(f.read("conflict.txt")); got != external {
		t.Fatalf("external edit overwritten: %q", got)
	}
	last := f.ws.Notifications[len(f.ws.Notifications)-1]
	if last.Kind != vapp.NoticeWarning || last.Title != "File changed" {
		t.Fatalf("notification = %+v", last)
	}
	// The tab reloaded from disk, so the next click operates on the new view.
	f.settleTab()
	if !f.s.HasText("Accept Theirs") {
		t.Fatalf("reloaded view lost the resolve controls:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	f.click("Accept Theirs")
	uiWaitFile(t, f, "conflict.txt", "theirs\n")
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
