//go:build vivahead

package viffui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/ui"
)

// clickMenuRow picks a row of the open dropdown by coordinate. A popup is
// drawn on its own layer, so the label lookup that reaches form controls does
// not reach its rows (see the font-picker tests).
func clickMenuRow(t *testing.T, f *uiFixture, label string) {
	t.Helper()
	var row ui.Rect
	found := false
	for _, hit := range f.s.Snapshot().Hits {
		if hit.Label == label {
			row, found = hit.Bounds, true
		}
	}
	if !found {
		t.Fatalf("the menu has no row for %q; texts: %s", label, strings.Join(f.s.Texts(), " | "))
	}
	if err := f.s.Click(row.X+row.Width/2, row.Y+row.Height/2); err != nil {
		t.Fatalf("click %q: %v", label, err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
}

// TestSearchSettingsDialog picks the Git grep backend on the General page and
// checks that OK stores it next to the trimmed ripgrep path.
func TestSearchSettingsDialog(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	if !f.s.HasText("Search") || !f.s.HasText("Search tool:") || !f.s.HasText("ripgrep path:") {
		t.Fatalf("the search section is missing; texts: %s", strings.Join(f.s.Texts(), " | "))
	}

	f.click("Auto (ripgrep if installed)")
	f.s.Settle(40*time.Millisecond, time.Second)
	clickMenuRow(t, f, "Git grep")

	f.click("ripgrep path:")
	if err := f.s.Type("  /opt/homebrew/bin/rg  "); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)

	f.click("OK")
	f.s.Settle(60*time.Millisecond, time.Second)
	if got := f.ws.Settings.SearchTool; got != "git" {
		t.Fatalf("SearchTool = %q, want git", got)
	}
	if got, want := f.ws.Settings.RipgrepPath, "/opt/homebrew/bin/rg"; got != want {
		t.Fatalf("RipgrepPath = %q, want %q", got, want)
	}
	f.assertIdle()
}

// A path that does not exist is reported instead of breaking the dialog: the
// machine may simply have no ripgrep, and Git grep stays usable.
func TestSearchSettingsMissingRipgrep(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	missing := filepath.Join(t.TempDir(), "no-such-ripgrep")
	f.ws.Settings.RipgrepPath = missing
	opened := ""
	f.win.openURL = func(url string) error { opened = url; return nil }
	f.win.openSettings()
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	if !f.s.HasText("ripgrep not found") {
		t.Fatalf("the missing backend was not reported; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("Not bundled. Leave the path empty") {
		t.Fatalf("the ripgrep hint is missing; texts: %s", strings.Join(f.s.Texts(), " | "))
	}

	f.click("Download ripgrep")
	f.s.Settle(40*time.Millisecond, time.Second)
	if want := "https://github.com/BurntSushi/ripgrep/releases"; opened != want {
		t.Fatalf("opened URL = %q, want %q", opened, want)
	}

	// Saving a path Viff cannot resolve is allowed: the field may name a
	// program the user installs later.
	f.click("OK")
	f.s.Settle(60*time.Millisecond, time.Second)
	if got := f.ws.Settings.RipgrepPath; got != missing {
		t.Fatalf("RipgrepPath = %q, want %q", got, missing)
	}
	f.assertIdle()
}
