//go:build vivahead

package viffui

import "testing"

// The window asks the platform to report minimization: that callback is what
// moves Viff from the taskbar into the status area.
func TestConfigReportsMinimize(t *testing.T) {
	f := newUIFixture(t)
	cfg := f.win.Config()
	if cfg.OnMinimize == nil {
		t.Fatal("Config carries no OnMinimize, so the window cannot hide into the status area")
	}
	// A headless run has no native handle; the callback must not panic.
	cfg.OnMinimize(true)
	cfg.OnMinimize(false)
}
