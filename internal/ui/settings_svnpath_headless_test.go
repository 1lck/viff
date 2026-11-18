//go:build vivahead

package viffui

import (
	"path/filepath"
	"testing"

	"viff/internal/storage"
	"viff/internal/svncli"
)

func TestSettingsCanConfigureSVNFromGitRepo(t *testing.T) {
	f := newUIFixture(t)
	svn := &svncli.Runner{}
	f.ws.SetBackends(f.ws.Git, svn)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	if f.ws.BackendName() != "Git" {
		t.Fatal("expected a Git repository to be open")
	}

	openTallSettings(t, f)
	f.snap("svn-path-settings")
	const label = "Path to svn:"
	if !f.s.HasText(label) {
		t.Fatal("SVN path is missing from General settings while a Git repo is open")
	}
	f.click(label)
	path := filepath.Join(t.TempDir(), "custom-svn")
	if err := f.s.Type(path); err != nil {
		t.Fatal(err)
	}
	f.click("OK")
	if got := storage.Load(f.ws.Settings.Path()).SVNPath; got != path {
		t.Fatalf("saved svn path = %q, want %q", got, path)
	}
	if got, err := svn.CachedExecutableIn(""); err != nil || got != path {
		t.Fatalf("runner svn path = %q, %v; want %q", got, err, path)
	}
}
