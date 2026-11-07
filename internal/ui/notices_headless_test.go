//go:build vivahead

package viffui

import (
	"testing"

	vapp "viff/internal/app"
	"viff/internal/i18n"
)

// TestNoticeBalloonStepsAsideForDialogs pins the paint order between the two.
// Balloons are drawn after the window content, so one left over from an earlier
// action painted above an open dialog and swallowed the click meant for the
// dialog's buttons: the submodule dialog lost its "Remove…" button whenever a
// "Submodule removed" notice was still on screen.
func TestNoticeBalloonStepsAsideForDialogs(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	const title = "Submodule removed"
	f.ws.Notify(vapp.NoticeSuccess, title, "deps/lib")
	f.win.Invalidate()
	f.settle()
	if !f.s.HasText(title) {
		t.Fatal("the balloon is missing with no dialog open")
	}

	f.win.openNewBranch("", "main")
	f.win.Invalidate()
	f.settle()
	if f.win.dialog == nil {
		t.Fatal("the new branch dialog did not open")
	}
	if f.s.HasText(title) {
		t.Fatal("the balloon is painted over an open dialog")
	}
	// The dialog keeps taking clicks while the notice is pending.
	f.click(f.win.t(i18n.CommonCancel))
	f.settle()
	if f.win.dialog != nil {
		t.Fatalf("the dialog stayed open after Cancel: %q", f.win.dialog.title)
	}

	// The notice was not lost: it shows again once the dialog is gone.
	f.win.Invalidate()
	f.settle()
	if !f.s.HasText(title) {
		t.Fatal("the balloon did not come back after the dialog closed")
	}
	f.assertIdle()
}
