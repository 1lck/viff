//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	"viff/internal/domain"
)

// TestEditHistoryDialogs opens each history-editing dialog, checks its title
// and that Escape closes it, so the textarea and form painting run headless.
func TestEditHistoryDialogs(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	var c domain.Commit
	for _, x := range f.ws.Repo.Log.Commits {
		if len(x.Parents) == 1 {
			c = x
			break
		}
	}
	if c.Hash == "" {
		t.Fatal("no commit with exactly one parent in the log")
	}

	open := func(name string, action func(domain.Commit), title string) {
		t.Helper()
		action(c)
		f.win.Invalidate()
		if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 10*time.Second); err != nil {
			t.Fatalf("%s: dialog never opened: %v", name, err)
		}
		f.s.Settle(40*time.Millisecond, time.Second)
		if got := f.win.dialog.title; !strings.Contains(got, title) {
			t.Fatalf("%s: title = %q, want %q", name, got, title)
		}
		if !f.s.HasText(f.win.dialog.title) {
			t.Fatalf("%s: dialog was not painted", name)
		}
		f.snap(name + "-dialog")
		f.s.Keys("escape")
		f.s.Settle(40*time.Millisecond, time.Second)
		if f.win.dialog != nil {
			t.Fatalf("%s: Escape did not close the dialog", name)
		}
	}

	open("reword", f.win.openRewordCommit, "Edit Commit Message")
	open("squash", f.win.openSquashCommit, "Squash")
	open("drop", f.win.openDropCommit, "Drop Commit")
}
