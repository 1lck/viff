//go:build vivahead

package viffui

import (
	"context"
	"errors"
	"testing"

	"viff/internal/gitcli"
)

// TestLFSStatusMarkerShowsManagedFiles checks the quiet part: a repository
// that uses Git LFS gets a status marker with the number of files git-lfs
// reports, and no warning while git-lfs is installed.
func TestLFSStatusMarkerShowsManagedFiles(t *testing.T) {
	f := newUIFixture(t)
	f.ws.Git.(*gitcli.Runner).LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) == 1 && args[0] == "version" {
			return []byte("git-lfs/9.9.9 (fake build)"), nil
		}
		if len(args) > 0 && args[0] == "ls-files" {
			return []byte("assets/model.bin\x00assets/tex.bin\x00"), nil
		}
		return nil, errors.New("unexpected git-lfs command")
	}
	f.write(".gitattributes", "*.bin filter=lfs diff=lfs merge=lfs -text\n")
	f.commit("Track binaries with Git LFS", "2026-10-14T10:00:00")
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	rs := f.ws.Repo
	if !rs.LFS.Installed || !rs.LFS.Managed || len(rs.LFS.Files) != 2 {
		t.Fatalf("LFS state = %+v", rs.LFS)
	}
	if !f.s.HasText("LFS 2") {
		t.Fatalf("status marker missing; texts: %v", f.s.Texts())
	}
	if f.s.HasText("Git LFS is not installed") {
		t.Fatalf("warning shown although git-lfs is installed")
	}
	f.snap("40-lfs-status")
	f.assertIdle()
}

// TestLFSWarningWhenGitLFSMissing checks the warning path: the Commit panel
// banner names the missing dependency and offers the install command to copy,
// and the status marker is still present.
func TestLFSWarningWhenGitLFSMissing(t *testing.T) {
	f := newUIFixture(t)
	f.ws.Git.(*gitcli.Runner).LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("git: 'lfs' is not a git command")
	}
	f.write(".gitattributes", "*.bin filter=lfs diff=lfs merge=lfs -text\n*.psd filter=lfs\n")
	f.commit("Track binaries with Git LFS", "2026-10-14T10:00:00")
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	rs := f.ws.Repo
	if rs.LFS.Installed || !rs.LFS.Managed || !rs.LFS.NeedsInstall() {
		t.Fatalf("LFS state = %+v", rs.LFS)
	}
	if !f.s.HasText("LFS") {
		t.Fatalf("status marker missing; texts: %v", f.s.Texts())
	}
	if !f.s.HasText("Git LFS is not installed") {
		t.Fatalf("missing-dependency warning absent; texts: %v", f.s.Texts())
	}
	if !f.s.HasText("2 LFS rules") {
		t.Fatalf("Commit banner does not state the rule count; texts: %v", f.s.Texts())
	}
	if !f.s.HasText("Copy Install Command") {
		t.Fatalf("install command action absent; texts: %v", f.s.Texts())
	}
	// The notification carries the full hint and the copy action; it must be
	// raised once for this repository generation.
	countNotices := func() int {
		n := 0
		for _, notice := range f.ws.Notifications {
			if notice.Title == "Git LFS is not installed" {
				n++
				if len(notice.Actions) != 1 {
					t.Fatalf("notification actions = %+v", notice.Actions)
				}
			}
		}
		return n
	}
	if got := countNotices(); got != 1 {
		t.Fatalf("git-lfs notifications = %d, want 1", got)
	}
	f.ws.Refresh(true)
	f.settle()
	if got := countNotices(); got != 1 {
		t.Fatalf("notification repeated after a refresh: %d", got)
	}
	f.snap("41-lfs-warning")
	f.assertIdle()
}
