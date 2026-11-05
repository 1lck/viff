//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	vapp "viff/internal/app"
)

// TestUndoShortcut reverses a branch creation with Command-Z, names the
// operation in the project menu, and leaves Command-Z to the commit message
// editor while that has focus.
func TestUndoShortcut(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	mainHead := strings.TrimSpace(f.git("rev-parse", "main"))

	// The fixture has local changes, so checkout would be blocked; creating
	// and switching to a branch is the operation this test can drive cleanly.
	f.ws.NewBranch("undo-check", "", true, false)
	f.settle()
	if got := f.ws.Repo.CurrentBranch(); got != "undo-check" {
		t.Fatalf("branch after create = %q, want undo-check", got)
	}
	if got := f.ws.UndoName(); got != "Create branch" {
		t.Fatalf("UndoName = %q, want Create branch", got)
	}

	// The project menu is the discoverable copy of the shortcut and names the
	// operation it would reverse.
	f.click("Project: viff-demo")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		_, ok := f.s.Hit("Undo Create branch")
		return ok
	}, 5*time.Second); err != nil {
		t.Fatalf("project menu has no Undo Create branch entry: %v\n%s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.snap("90-undo-menu")
	if err := f.s.Keys("escape"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)

	if err := f.s.Keys("cmd+z"); err != nil {
		t.Fatal(err)
	}
	f.settle()
	if got := f.ws.Repo.CurrentBranch(); got != "main" {
		t.Fatalf("branch after undo = %q, want main", got)
	}
	if got := strings.TrimSpace(f.git("rev-parse", "HEAD")); got != mainHead {
		t.Fatalf("HEAD after undo = %s, want %s", got, mainHead)
	}
	last := f.ws.Notifications[len(f.ws.Notifications)-1]
	if last.Kind != vapp.NoticeSuccess || last.Title != "Undid Create branch" {
		t.Fatalf("notification = %+v, want a success named Undid Checkout", last)
	}
	f.assertIdle()

	// A focused text field keeps Command-Z for its own undo: the repository
	// operation is neither reversed nor consumed.
	f.ws.NewBranch("undo-guard", "", true, false)
	f.settle()
	if f.ws.UndoName() != "Create branch" {
		t.Fatalf("UndoName = %q, want Create branch", f.ws.UndoName())
	}
	before := len(f.ws.Notifications)
	f.win.commitOpen, f.win.projectOpen = true, false
	f.win.focusNext = idCmMessage
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	if !f.win.focused(idCmMessage) {
		t.Fatalf("the commit message editor is not focused; focused id = %v", f.win.c.Focused())
	}
	if err := f.s.Keys("cmd+z"); err != nil {
		t.Fatal(err)
	}
	f.settle()
	if got := f.ws.Repo.CurrentBranch(); got != "undo-guard" {
		t.Fatalf("Command-Z in the message editor undid the repository: branch = %q", got)
	}
	if n := len(f.ws.Notifications); n != before {
		t.Fatalf("Command-Z in the message editor showed %d new notifications", n-before)
	}
	if got := f.ws.UndoName(); got != "Create branch" {
		t.Fatalf("Command-Z in the message editor consumed the record: UndoName = %q", got)
	}
}
