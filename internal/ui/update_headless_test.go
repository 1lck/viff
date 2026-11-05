//go:build vivahead

package viffui

import (
	"testing"
	"time"

	vapp "viff/internal/app"
)

// TestHelpMenuHasCheckForUpdates covers the always-available manual entry.
func TestHelpMenuHasCheckForUpdates(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	for _, entry := range f.win.helpMenu() {
		if entry.title == "Check for Updates…" {
			return
		}
	}
	t.Fatalf("Help menu has no Check for Updates entry: %+v", f.win.helpMenu())
}

// TestUpdateDialogStates renders each phase without touching the network.
func TestUpdateDialogStates(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	show := func(state vapp.UpdateState) {
		t.Helper()
		f.ws.Update = state
		if f.win.dialog == nil {
			f.win.openUpdateDialog()
		}
		f.win.Invalidate()
		f.s.Settle(60*time.Millisecond, 2*time.Second)
	}

	show(vapp.UpdateState{Phase: vapp.UpdateChecking, Current: "v0.1.0"})
	if !f.s.HasText("Checking for updates…") {
		t.Fatal("checking state is not rendered")
	}

	show(vapp.UpdateState{
		Phase:   vapp.UpdateAvailable,
		Current: "v0.1.0",
		Version: "v0.1.1",
		Notes:   "Fixes and improvements.",
	})
	for _, want := range []string{"Viff v0.1.1 is available.", "You have v0.1.0.", "Fixes and improvements."} {
		if !f.s.HasText(want) {
			t.Fatalf("available state is missing %q", want)
		}
	}

	show(vapp.UpdateState{
		Phase:    vapp.UpdateDownloading,
		Current:  "v0.1.0",
		Version:  "v0.1.1",
		Progress: 1024,
		Total:    2048,
	})
	for _, want := range []string{"Downloading Viff v0.1.1…", "50%"} {
		if !f.s.HasText(want) {
			t.Fatalf("downloading state is missing %q", want)
		}
	}

	show(vapp.UpdateState{Phase: vapp.UpdateReady, Current: "v0.1.0", Version: "v0.1.1"})
	for _, want := range []string{"Viff v0.1.1 has been downloaded.", "Restart Viff to finish the update."} {
		if !f.s.HasText(want) {
			t.Fatalf("ready state is missing %q", want)
		}
	}
	if !f.s.HasText("Restart to Update") {
		t.Fatal("ready state does not offer Restart to Update")
	}

	show(vapp.UpdateState{
		Phase:      vapp.UpdateManual,
		Current:    "v0.1.0",
		Version:    "v0.1.1",
		Message:    "this installation is read-only",
		ManualPath: "/tmp/viff-v0.1.1-linux-amd64.deb",
	})
	for _, want := range []string{"this installation is read-only", "/tmp/viff-v0.1.1-linux-amd64.deb", "Show Download"} {
		if !f.s.HasText(want) {
			t.Fatalf("manual state is missing %q", want)
		}
	}
}
