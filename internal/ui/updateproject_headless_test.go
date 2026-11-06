//go:build vivahead

package viffui

import (
	"testing"
	"time"

	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
)

// TestUpdateProjectRemembersTheChoice covers the dialog's "Don't ask again"
// checkbox: once it is ticked, the next update runs with the stored method
// instead of asking again.
func TestUpdateProjectRemembersTheChoice(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.win.openUpdate()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	if f.win.dialog == nil {
		t.Fatal("Update Project did not open its dialog")
	}

	f.click(f.win.t(i18n.DialogUpdateDontAsk))
	f.click(f.win.t(i18n.CommonOK))
	f.settle()

	if !f.ws.Settings.SkipUpdateDialog {
		t.Fatal(`"Don't ask again" was not saved`)
	}
	if f.win.dialog != nil {
		t.Fatal("the dialog stayed open after OK")
	}
}

// TestUpdateProjectSkipsDialogWhenNotAsking covers the short path: the shortcut
// starts the update with the stored method and reports it, with no dialog.
func TestUpdateProjectSkipsDialogWhenNotAsking(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	s := f.ws.Settings
	s.SkipUpdateDialog = true
	s.PullMode = "ff-only"

	f.win.openUpdate()
	f.settle()

	if f.win.dialog != nil {
		t.Fatal("the dialog opened although updating was told not to ask")
	}
	want := f.win.t(i18n.AppNoticeUpdatingProject, f.ws.Repo.CurrentBranch())
	found := false
	for _, n := range f.ws.Notifications {
		if n.Title == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("no %q notification: %+v", want, f.ws.Notifications)
	}
}

// The dialog must still appear while the choice is unset, so the shortcut keeps
// its reminder for everyone who never opted out.
func TestUpdateProjectAsksWhenTheSettingIsOff(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.ws.Settings.SkipUpdateDialog = false
	f.win.openUpdate()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	if f.win.dialog == nil {
		t.Fatal("Update Project did not ask although the setting is off")
	}
	if !f.s.HasText(f.win.t(i18n.DialogUpdateDontAsk)) {
		t.Fatalf("dialog has no way to stop asking: %v", f.s.Texts())
	}
}

// TestSettingsToggleRestoresTheUpdatePrompt covers the way back: the Git page's
// checkbox is the only place that turns the reminder on again.
func TestSettingsToggleRestoresTheUpdatePrompt(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	toggle := func() {
		t.Helper()
		f.win.openSettings()
		f.win.Invalidate()
		f.s.Settle(60*time.Millisecond, 2*time.Second)
		f.click(f.win.t(i18n.SettingsTabGit))
		f.click(f.win.t(i18n.SettingsCheckAskUpdate))
		f.click(f.win.t(i18n.CommonOK))
		f.s.Settle(60*time.Millisecond, 2*time.Second)
	}

	toggle()
	if !f.ws.Settings.SkipUpdateDialog {
		t.Fatal("turning the reminder off in the settings did not persist")
	}
	toggle()
	if f.ws.Settings.SkipUpdateDialog {
		t.Fatal("turning the reminder back on did not persist")
	}
}

// TestWindowPositionRestoredAndRecorded covers both halves of the position
// memory: a saved corner reaches Config.Origin, and a moved window writes the
// new one back. A display left of the primary one puts the coordinates below
// zero, which must survive unchanged.
func TestWindowPositionRestoredAndRecorded(t *testing.T) {
	f := newUIFixture(t)
	win := &f.ws.Settings.Window
	win.X, win.Y, win.Positioned = -1280, 40, true

	cfg := f.win.Config()
	if cfg.Origin == nil {
		t.Fatal("a saved position did not reach Config.Origin")
	}
	if cfg.Origin.X != -1280 || cfg.Origin.Y != 40 {
		t.Fatalf("Origin = %+v, want the saved corner", *cfg.Origin)
	}

	f.start(1440, 900)
	f.win.onFrameChange(ui.Rect{X: -640, Y: 120, Width: 1440, Height: 900})
	if !win.Positioned || win.X != -640 || win.Y != 120 {
		t.Fatalf("the moved window was not recorded: %+v", *win)
	}

	// The origin is a real position, so it must not be mistaken for "unset".
	f.win.onFrameChange(ui.Rect{Width: 1440, Height: 900})
	if !win.Positioned || win.X != 0 || win.Y != 0 {
		t.Fatalf("a window at the origin lost its position: %+v", *win)
	}
	if cfg := f.win.Config(); cfg.Origin == nil || cfg.Origin.X != 0 || cfg.Origin.Y != 0 {
		t.Fatalf("Origin at the screen corner was dropped: %+v", cfg.Origin)
	}
}
