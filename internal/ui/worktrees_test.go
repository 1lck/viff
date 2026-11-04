//go:build vivahead

package viffui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// hit finds a painted control by its accessibility label, including disabled
// ones: Session.Hit skips disabled and covered regions, which is exactly what
// these tests assert about.
func hit(f *uiFixture, label string) (ui.Hit, bool) {
	hits := f.s.Snapshot().Hits
	for i := len(hits) - 1; i >= 0; i-- {
		if hits[i].Label == label {
			return hits[i], true
		}
	}
	return ui.Hit{}, false
}

// waitStableControl flushes background layout changes before ClickLabel reads
// coordinates. ClickLabel reads a snapshot before Driver.Click pumps wakes,
// so a stale snapshot can otherwise click a different control after the pump.
func waitStableControl(t *testing.T, f *uiFixture, label string) {
	t.Helper()
	f.win.Invalidate()
	var previous ui.Rect
	var since time.Time
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		h, ok := hit(f, label)
		if !ok || h.Disabled || f.ws.Busy() || f.ws.Pending() {
			since = time.Time{}
			return false
		}
		if since.IsZero() || h.Bounds != previous {
			previous, since = h.Bounds, time.Now()
			return false
		}
		return time.Since(since) >= 150*time.Millisecond
	}, 10*time.Second); err != nil {
		t.Fatalf("control %q did not stabilize: %v", label, err)
	}
}

// fieldValue is the text of a labelled dialog field.
func fieldValue(t *testing.T, f *uiFixture, label string) string {
	t.Helper()
	h, ok := hit(f, label)
	if !ok {
		t.Fatalf("field %q not painted; texts: %s", label, strings.Join(f.s.Texts(), " | "))
	}
	return h.Value
}

// waitValue waits for a dialog field to hold want.
func waitValue(t *testing.T, f *uiFixture, label, want string) {
	t.Helper()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		h, ok := hit(f, label)
		return ok && h.Value == want
	}, 10*time.Second); err != nil {
		t.Fatalf("field %q = %q, want %q: %v\ntexts: %s", label, fieldValue(t, f, label), want, err, strings.Join(f.s.Texts(), " | "))
	}
}

// waitDialog opens the dialog titled title (test code changes the window
// outside a pointer event, so it must ask for the frame) and lets it settle.
func waitDialog(t *testing.T, f *uiFixture, title string) {
	t.Helper()
	f.win.Invalidate()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && f.win.dialog.title == title
	}, 10*time.Second); err != nil {
		t.Fatalf("dialog %q did not open: %v\ntexts: %s", title, err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
}

// selectExistingMode switches the New Worktree dialog to its existing-branch
// mode and returns once the swapped-in fields are painted.
func selectExistingMode(t *testing.T, f *uiFixture) {
	t.Helper()
	f.click("Existing branch")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		radio, ok := hit(f, "Existing branch")
		return ok && radio.Checked && f.s.HasText("Branch:") && !f.s.HasText("Start point:")
	}, 10*time.Second); err != nil {
		t.Fatalf("existing-branch mode did not appear: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
}

// suggestedPath is the directory the dialog suggests for a branch.
func suggestedPath(f *uiFixture, branch string) string {
	name := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(filepath.Dir(f.dir), filepath.Base(f.dir)+"-"+name)
}

// sameWorktreePath compares a worktree path as Git reported it, which uses
// forward slashes on Windows too, with a path built by filepath.Join, which
// uses the platform separator.
func sameWorktreePath(a, b string) bool {
	return filepath.ToSlash(a) == filepath.ToSlash(b)
}

// porcelainNamesPath reports whether git worktree list --porcelain names path.
func porcelainNamesPath(porcelain, path string) bool {
	return strings.Contains(porcelain, filepath.ToSlash(path))
}

// TestNewWorktreeExistingBranchDialog drives the dialog the way a user does:
// open it from the worktree list, switch to the existing branch, see why a
// checked-out branch is refused, pick a free one, keep a hand-edited path, and
// check the branch out in a new worktree.
func TestNewWorktreeExistingBranchDialog(t *testing.T) {
	f := newUIFixture(t)
	// One branch is already checked out elsewhere, one more is free.
	featureWT := filepath.Join(t.TempDir(), "viff-demo-feature")
	f.git("worktree", "add", "-q", featureWT, "feature/search")
	f.git("branch", "release/1.0", "HEAD")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	mainHead := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	fixTip := strings.TrimSpace(f.git("rev-parse", "fix/store-locking"))
	statusBefore := f.git("status", "--porcelain")

	// The Worktrees dialog is the way in.
	f.win.openWorktrees()
	waitDialog(t, f, "Worktrees")
	f.click("New Worktree…")
	waitDialog(t, f, "New Worktree")

	// New branch is the default: the old fields are there.
	if radio, ok := hit(f, "New branch"); !ok || !radio.Checked {
		t.Fatalf("New branch radio = %+v / %v, want the default mode", radio, ok)
	}
	if _, ok := hit(f, "Existing branch"); !ok {
		t.Fatal("Existing branch radio is missing")
	}
	if !f.s.HasText("Branch name:") || !f.s.HasText("Start point:") {
		t.Fatalf("new-branch mode lost its fields; texts: %s", strings.Join(f.s.Texts(), " | "))
	}

	// Existing branch swaps in the branch choice and hides the start point,
	// which means nothing when the branch is already there.
	selectExistingMode(t, f)
	if f.s.HasText("Branch name:") || f.s.HasText("Start point:") {
		t.Fatalf("existing-branch mode kept new-branch fields; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	waitValue(t, f, "Path:", suggestedPath(f, "fix/store-locking"))
	if !f.s.HasText("checked out in viff-demo-feature") || !f.s.HasText("checked out in this checkout") {
		t.Fatalf("checked-out branches are not explained; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("01-existing-branch")

	// The list is inside the dialog, so occupied branches are real disabled
	// rows with the reason next to them, and the free ones are clickable.
	for _, branch := range []string{"feature/search", "main"} {
		h, ok := hit(f, branch)
		if !ok || !h.Disabled {
			t.Fatalf("%s list row = %+v / %v, want disabled", branch, h, ok)
		}
	}
	if h, ok := hit(f, "release/1.0"); !ok || h.Disabled {
		t.Fatalf("free branch list row = %+v / %v, want enabled", h, ok)
	}
	if h, ok := hit(f, "fix/store-locking"); !ok || h.Disabled || !h.Checked {
		t.Fatalf("default branch row = %+v / %v, want enabled and checked", h, ok)
	}
	if _, ok := hit(f, "origin/main"); ok {
		t.Fatal("a remote branch was offered for a worktree")
	}

	// Picking another free branch moves the suggested path with it.
	f.click("release/1.0")
	releasePath := suggestedPath(f, "release/1.0")
	waitValue(t, f, "Path:", releasePath)

	// A hand-edited path is not overwritten when the branch changes again.
	f.click("Path:")
	if err := f.s.Keys("end"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("-mine"); err != nil {
		t.Fatal(err)
	}
	picked := releasePath + "-mine"
	waitValue(t, f, "Path:", picked)
	f.click("fix/store-locking")
	waitValue(t, f, "Path:", picked)

	// Create checks the free branch out in the edited directory.
	if h, ok := hit(f, "Create"); !ok || h.Disabled {
		t.Fatalf("Create = %+v / %v, want enabled", h, ok)
	}
	f.click("Create")
	f.settle()
	waitDialog(t, f, "Worktrees")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		for _, wt := range f.ws.Repo.Worktrees {
			if wt.Branch == "fix/store-locking" && sameWorktreePath(wt.Path, picked) {
				return true
			}
		}
		return false
	}, 10*time.Second); err != nil {
		t.Fatalf("worktree state missing %s: %v\nworktrees: %+v", picked, err, f.ws.Repo.Worktrees)
	}

	out := f.git("worktree", "list", "--porcelain")
	if !porcelainNamesPath(out, picked) || !strings.Contains(out, "refs/heads/fix/store-locking") {
		t.Fatalf("new worktree missing from git:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(picked, "main.go")); err != nil {
		t.Fatalf("worktree was not checked out: %v", err)
	}
	if got := strings.TrimSpace(f.git("rev-parse", "fix/store-locking")); got != fixTip {
		t.Fatalf("branch tip moved from %s to %s", fixTip, got)
	}
	if got := strings.TrimSpace(f.git("rev-parse", "HEAD")); got != mainHead {
		t.Fatalf("current checkout moved from %s to %s", mainHead, got)
	}
	if got := f.git("status", "--porcelain"); got != statusBefore {
		t.Fatalf("original checkout changed:\nbefore:\n%s\nafter:\n%s", statusBefore, got)
	}
	if got := strings.TrimSpace(f.gitIn(picked, "rev-parse", "--abbrev-ref", "HEAD")); got != "fix/store-locking" {
		t.Fatalf("worktree HEAD = %q, want fix/store-locking", got)
	}
	last := f.ws.Notifications[len(f.ws.Notifications)-1]
	if last.Kind != vapp.NoticeSuccess || last.Title != "Worktree created" {
		t.Fatalf("notification = %+v", last)
	}
}

// TestNewWorktreeExistingBranchRefusesOccupied makes every local branch
// checked out in a worktree, including one whose directory is gone, and checks
// that the existing mode offers nothing and refuses to create.
func TestNewWorktreeExistingBranchRefusesOccupied(t *testing.T) {
	f := newUIFixture(t)
	featureWT := filepath.Join(t.TempDir(), "viff-demo-feature")
	f.git("worktree", "add", "-q", featureWT, "feature/search")
	fixWT := filepath.Join(t.TempDir(), "viff-demo-fix")
	f.git("worktree", "add", "-q", fixWT, "fix/store-locking")
	// Git still counts a prunable worktree's branch as checked out, and so
	// must the dialog: Git would refuse to add it elsewhere.
	if err := os.RemoveAll(featureWT); err != nil {
		t.Fatal(err)
	}
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	before := f.git("worktree", "list", "--porcelain")
	f.win.openNewWorktree()
	waitDialog(t, f, "New Worktree")
	selectExistingMode(t, f)

	if !f.s.HasText("Every local branch is already checked out in a worktree") {
		t.Fatalf("all-occupied case is not explained; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if h, ok := hit(f, "Create"); !ok || !h.Disabled {
		t.Fatalf("Create = %+v / %v, want disabled", h, ok)
	}
	for _, branch := range []string{"feature/search", "fix/store-locking", "main"} {
		h, ok := hit(f, branch)
		if !ok || !h.Disabled {
			t.Fatalf("%s list row = %+v / %v, want disabled", branch, h, ok)
		}
	}
	f.snap("occupied-branches-rejected")
	f.s.Keys("escape")
	f.s.Settle(60*time.Millisecond, time.Second)
	waitDialog(t, f, "Worktrees")
	if got := f.git("worktree", "list", "--porcelain"); got != before {
		t.Fatalf("a worktree was created despite the refusal:\n%s", got)
	}
}

// TestNewWorktreeExistingBranchRefusesMissing deletes the chosen branch
// behind the open dialog and checks that creating is refused and recovers when
// another branch is picked.
func TestNewWorktreeExistingBranchRefusesMissing(t *testing.T) {
	f := newUIFixture(t)
	f.git("branch", "release/1.0", "HEAD")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	f.win.openNewWorktree()
	waitDialog(t, f, "New Worktree")
	selectExistingMode(t, f)
	waitValue(t, f, "Path:", suggestedPath(f, "feature/search"))

	// The branch goes away in another terminal; the state refresh reports it.
	// The refresh is asynchronous: on a slow machine the refs land after
	// settle, so wait for the dialog itself to explain the missing branch
	// instead of reading the paint that preceded the refresh.
	f.git("branch", "-D", "feature/search")
	f.ws.Refresh(true)
	f.win.Invalidate()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		create, ok := hit(f, "Create")
		return f.s.HasText("is not a local branch any more") && ok && create.Disabled
	}, 10*time.Second); err != nil {
		t.Fatalf("missing branch is not explained and refused: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, time.Second)

	f.snap("deleted-branch-rejected")

	// Picking a branch that still exists recovers.
	if branch, ok := hit(f, "fix/store-locking"); !ok || branch.Disabled {
		t.Fatalf("remaining branch not offered: %+v / %v\ntexts: %s", branch, ok, strings.Join(f.s.Texts(), " | "))
	}
	f.click("fix/store-locking")
	waitValue(t, f, "Path:", suggestedPath(f, "fix/store-locking"))
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		h, ok := hit(f, "Create")
		return ok && !h.Disabled
	}, 10*time.Second); err != nil {
		t.Fatalf("Create did not recover after picking an existing branch: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Keys("escape")
	f.s.Settle(60*time.Millisecond, time.Second)
}

// TestNewWorktreeNewBranchStillWorks keeps the old flow: the default mode
// creates a branch at the start point and checks it out in the suggested
// directory.
func TestNewWorktreeNewBranchStillWorks(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	mainHead := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	f.win.openNewWorktree()
	waitDialog(t, f, "New Worktree")

	if radio, ok := hit(f, "New branch"); !ok || !radio.Checked {
		t.Fatalf("New branch = %+v / %v, want the default", radio, ok)
	}
	if existing, ok := hit(f, "Existing branch"); !ok || existing.Checked {
		t.Fatalf("Existing branch = %+v / %v, want unchecked", existing, ok)
	}
	if !f.s.HasText("Start point:") || fieldValue(t, f, "Start point:") != "main" {
		t.Fatalf("start point = %q, want the current branch; texts: %s",
			fieldValue(t, f, "Start point:"), strings.Join(f.s.Texts(), " | "))
	}
	f.snap("00-new-branch")
	f.click("Branch name:")
	if err := f.s.Type("feature-worktree"); err != nil {
		t.Fatal(err)
	}
	want := suggestedPath(f, "feature-worktree")
	waitValue(t, f, "Path:", want)
	waitStableControl(t, f, "Create")
	if h, ok := hit(f, "Create"); !ok || h.Disabled {
		t.Fatalf("Create = %+v / %v, want enabled", h, ok)
	}
	f.click("Create")
	f.settle()
	waitDialog(t, f, "Worktrees")

	if got := strings.TrimSpace(f.git("rev-parse", "refs/heads/feature-worktree")); got != mainHead {
		t.Fatalf("new branch at %s, want %s", got, mainHead)
	}
	if got := strings.TrimSpace(f.gitIn(want, "rev-parse", "--abbrev-ref", "HEAD")); got != "feature-worktree" {
		t.Fatalf("worktree HEAD = %q, want feature-worktree", got)
	}
	if _, err := os.Stat(filepath.Join(want, "main.go")); err != nil {
		t.Fatalf("worktree was not checked out: %v", err)
	}
	if got := strings.TrimSpace(f.git("rev-parse", "--abbrev-ref", "HEAD")); got != "main" {
		t.Fatalf("current branch = %q, want main", got)
	}
	if out := f.git("worktree", "list", "--porcelain"); !porcelainNamesPath(out, want) {
		t.Fatalf("worktree list missing %s:\n%s", want, out)
	}
	var found domain.Worktree
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		for _, wt := range f.ws.Repo.Worktrees {
			if sameWorktreePath(wt.Path, want) {
				found = wt
				return true
			}
		}
		return false
	}, 10*time.Second); err != nil {
		t.Fatalf("state missing the new worktree: %v\nworktrees: %+v", err, f.ws.Repo.Worktrees)
	}
	if found.Branch != "feature-worktree" {
		t.Fatalf("worktree branch = %q, want feature-worktree", found.Branch)
	}
}

// TestNewWorktreeExistingBranchKeyboard switches modes and picks a branch
// with the keyboard only: Tab to the mode radio, Space to switch, then the
// arrow keys, which skip the branches a worktree already holds.
func TestNewWorktreeExistingBranchKeyboard(t *testing.T) {
	f := newUIFixture(t)
	featureWT := filepath.Join(t.TempDir(), "viff-demo-feature")
	f.git("worktree", "add", "-q", featureWT, "feature/search")
	f.git("branch", "release/1.0", "HEAD")
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	f.win.openNewWorktree()
	waitDialog(t, f, "New Worktree")

	// Tab from the branch name field to the mode radio, Space to pick it.
	radio, ok := hit(f, "Existing branch")
	if !ok {
		t.Fatalf("Existing branch radio missing; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	focusedRadio := false
	for i := 0; i < 8 && !focusedRadio; i++ {
		if err := f.s.Keys("tab"); err != nil {
			t.Fatal(err)
		}
		f.s.Settle(40*time.Millisecond, time.Second)
		focusedRadio = f.s.Snapshot().Focused == radio.ID
	}
	if !focusedRadio {
		t.Fatalf("Tab never reached the mode radio; focused=%v texts: %s",
			f.s.Snapshot().Focused, strings.Join(f.s.Texts(), " | "))
	}
	if err := f.s.Keys("space"); err != nil {
		t.Fatal(err)
	}

	// Switching to the existing mode hands the keyboard to the branch list.
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.s.Snapshot().Focused == idDialogBranchList
	}, 10*time.Second); err != nil {
		t.Fatalf("branch list did not take focus: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	if h, ok := hit(f, "fix/store-locking"); !ok || !h.Checked {
		t.Fatalf("default branch row = %+v / %v; texts: %s", h, ok, strings.Join(f.s.Texts(), " | "))
	}

	keys := func(key string) {
		t.Helper()
		if err := f.s.Keys(key); err != nil {
			t.Fatal(err)
		}
		f.s.Settle(40*time.Millisecond, time.Second)
	}
	checked := func(branch string) bool {
		h, ok := hit(f, branch)
		return ok && h.Checked
	}

	// Down steps over main, which the current checkout has, to release/1.0.
	keys("down")
	if !checked("release/1.0") {
		t.Fatalf("Down did not select release/1.0; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	waitValue(t, f, "Path:", suggestedPath(f, "release/1.0"))
	keys("down") // nothing free below; the choice stays
	if !checked("release/1.0") {
		t.Fatal("Down moved past the last free branch")
	}
	keys("up") // back over main
	if !checked("fix/store-locking") {
		t.Fatalf("Up did not select fix/store-locking; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	keys("up") // feature/search is taken, so the choice stays
	if !checked("fix/store-locking") {
		t.Fatal("Up selected or skipped onto a checked-out branch")
	}
	keys("home")
	if !checked("fix/store-locking") {
		t.Fatal("Home did not land on the first free branch")
	}
	keys("end")
	if !checked("release/1.0") {
		t.Fatal("End did not land on the last free branch")
	}
	f.snap("02-existing-branch-keyboard")

	// Enter submits the dialog from the list.
	keys("enter")
	f.settle()
	waitDialog(t, f, "Worktrees")
	path := suggestedPath(f, "release/1.0")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		for _, wt := range f.ws.Repo.Worktrees {
			if wt.Branch == "release/1.0" && sameWorktreePath(wt.Path, path) {
				return true
			}
		}
		return false
	}, 10*time.Second); err != nil {
		t.Fatalf("worktree state missing %s: %v\nworktrees: %+v", path, err, f.ws.Repo.Worktrees)
	}
	if got := strings.TrimSpace(f.gitIn(path, "rev-parse", "--abbrev-ref", "HEAD")); got != "release/1.0" {
		t.Fatalf("worktree HEAD = %q, want release/1.0", got)
	}
}

// TestNewWorktreeExistingBranchKeyboardScrolls gives the list more branches
// than it shows, selects one past the last visible row with the keyboard, and
// checks that it scrolls the choice into view and back.
func TestNewWorktreeExistingBranchKeyboardScrolls(t *testing.T) {
	f := newUIFixture(t)
	f.git("worktree", "add", "-q", filepath.Join(t.TempDir(), "viff-demo-feature"), "feature/search")
	for i := 1; i <= 7; i++ {
		f.git("branch", fmt.Sprintf("topic/%d", i), "HEAD")
	}
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	f.win.openNewWorktree()
	waitDialog(t, f, "New Worktree")
	selectExistingMode(t, f)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.s.Snapshot().Focused == idDialogBranchList
	}, 10*time.Second); err != nil {
		t.Fatalf("branch list did not take focus: %v", err)
	}

	// The last branch is outside the six visible rows until it is selected.
	if _, ok := hit(f, "topic/7"); ok {
		t.Fatal("an off-screen row was painted")
	}
	for i := 0; i < 7; i++ { // fix/store-locking, then topic/1 … topic/7
		if err := f.s.Keys("down"); err != nil {
			t.Fatal(err)
		}
		f.s.Settle(40*time.Millisecond, time.Second)
	}
	h, ok := hit(f, "topic/7")
	if !ok || !h.Checked {
		t.Fatalf("topic/7 = %+v / %v, want selected and scrolled into view", h, ok)
	}
	if err := f.s.Keys("home"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if h, ok := hit(f, "fix/store-locking"); !ok || !h.Checked {
		t.Fatalf("Home = %+v / %v, want the first free branch scrolled back", h, ok)
	}
}
