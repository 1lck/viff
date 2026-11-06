//go:build vivahead

package viffui

import (
	"testing"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// TestWorktreePathAcceptsLinuxSpellingForWSLRepositories pins where a typed
// path is translated: only a WSL repository accepts the Linux spelling its
// file manager shows, and it becomes the identity form everything else uses.
// A Windows repository keeps the path untouched — "/home/x/wt" is not a
// directory there.
func TestWorktreePathAcceptsLinuxSpellingForWSLRepositories(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	w := f.win

	f.ws.Repo = &vapp.RepoState{Repo: domain.Repo{Root: `\\wsl.localhost\ubuntu\home\momo\repo`}}
	if got, want := w.worktreePath("/home/momo/wt"), `\\wsl.localhost\ubuntu\home\momo\wt`; got != want {
		t.Errorf("WSL repository: worktreePath(%q) = %q, want %q", "/home/momo/wt", got, want)
	}
	// The UNC spelling and relative paths pass through unchanged.
	if got := w.worktreePath(`\\wsl.localhost\ubuntu\home\momo\wt`); got != `\\wsl.localhost\ubuntu\home\momo\wt` {
		t.Errorf("UNC path was rewritten: %q", got)
	}

	f.ws.Repo = &vapp.RepoState{Repo: domain.Repo{Root: `C:\Users\momo\repo`}}
	if got, want := w.worktreePath("/home/momo/wt"), "/home/momo/wt"; got != want {
		t.Errorf("Windows repository: worktreePath(%q) = %q, want %q", "/home/momo/wt", got, want)
	}

	f.ws.Repo = nil
	if got, want := w.worktreePath("/home/momo/wt"), "/home/momo/wt"; got != want {
		t.Errorf("no repository: worktreePath(%q) = %q, want %q", "/home/momo/wt", got, want)
	}
}
