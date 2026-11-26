//go:build vivahead

package viffui

import "testing"

func TestCopyBranchName(t *testing.T) {
	const local = "codex/feature/copy-name"
	f := newUIFixture(t)
	f.git("checkout", "-q", "-b", local)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	f.win.gitOpen, f.win.gitTab = true, 0

	for _, theme := range []string{"light", "dark"} {
		f.ws.Settings.Theme = theme
		f.win.applyTheme()
		f.win.Invalidate()
		f.settle()
		f.s.Driver().SetClipboard("")
		if err := f.s.RightClickLabel("option:" + local); err != nil {
			t.Fatal(err)
		}
		f.settle()
		f.click("Copy Branch Name")
		if got := f.s.Driver().Clipboard(); got != local {
			t.Fatalf("%s theme: clipboard = %q, want %q", theme, got, local)
		}
	}

	const remote = "origin/feature/search"
	openBranchMenu(t, f, remote)
	f.click("Copy Branch Name")
	if got := f.s.Driver().Clipboard(); got != remote {
		t.Fatalf("clipboard = %q, want %q", got, remote)
	}
}
