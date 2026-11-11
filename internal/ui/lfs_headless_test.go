//go:build vivahead

package viffui

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	vapp "viff/internal/app"
	"viff/internal/gitcli"
)

func TestLFSStatusMarkerCountsOnKeyboardRequest(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) { testLFSStatusMarker(t, theme) })
	}
}

func testLFSStatusMarker(t *testing.T, theme string) {
	f := newUIFixture(t)
	f.ws.Settings.Theme = theme
	release := make(chan struct{})
	var outcome atomic.Int32
	f.ws.Git.(*gitcli.Runner).LFSCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) == 1 && args[0] == "version" {
			return []byte("git-lfs/9.9.9 (fake build)"), nil
		}
		if len(args) > 0 && args[0] == "ls-files" {
			select {
			case <-release:
				switch outcome.Load() {
				case 1:
					return nil, nil
				case 2:
					return nil, errors.New("listing failed")
				}
				return []byte("assets/model.bin\x00assets/tex.bin\x00"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return nil, errors.New("unexpected git-lfs command")
	}
	f.write(".gitattributes", "*.bin filter=lfs diff=lfs merge=lfs -text\n")
	f.commit("Track binaries with Git LFS", "2026-10-14T10:00:00")
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	rs := f.ws.Repo
	if err := f.s.WaitFor(func(app.Snapshot) bool { return rs.LFS.Installed && f.s.HasText("LFS") }, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	if !rs.LFS.Managed || rs.LFSStats.Status != vapp.LFSStatsUnqueried || f.s.HasText("LFS 2") {
		t.Fatalf("initial LFS state = %+v, %+v", rs.LFS, rs.LFSStats)
	}
	for i := 0; i < 50 && f.s.Snapshot().Focused != idStatusLFS; i++ {
		if err := f.s.Keys("tab"); err != nil {
			t.Fatal(err)
		}
	}
	if f.s.Snapshot().Focused != idStatusLFS {
		t.Fatal("Tab did not reach the LFS marker")
	}
	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return rs.LFSStats.Status == vapp.LFSStatsLoading && f.s.HasText("LFS · Counting…")
	}, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return rs.LFSStats.Status == vapp.LFSStatsCurrent && f.s.HasText("LFS 2")
	}, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	if tip := f.win.lfsStatusTip(rs); !strings.Contains(tip, "Last counted:") || !strings.Contains(tip, "2 managed files") {
		t.Fatalf("result tooltip = %q", tip)
	}
	copied := copySeam(f)
	for _, item := range f.win.gitMenu() {
		if item.title == "Copy LFS File List" && !item.disabled {
			item.run()
		}
	}
	if len(*copied) != 1 || (*copied)[0] != "assets/model.bin\nassets/tex.bin" {
		t.Fatalf("copied LFS paths = %v", *copied)
	}
	if f.s.HasText("Git LFS is not installed") {
		t.Fatalf("warning shown although git-lfs is installed")
	}
	f.snap("40-lfs-status")
	outcome.Store(1)
	f.ws.RefreshLFSStats()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return rs.LFSStats.Status == vapp.LFSStatsCurrent && f.s.HasText("LFS 0")
	}, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	outcome.Store(2)
	f.ws.RefreshLFSStats()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return rs.LFSStats.Status == vapp.LFSStatsFailed && f.s.HasText("LFS 0 · Count failed")
	}, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	if tip := f.win.lfsStatusTip(rs); !strings.Contains(tip, "Last counted:") || !strings.Contains(tip, "git.failed") {
		t.Fatalf("failure hid the last successful zero or code: %q", tip)
	}
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
	if err := f.s.WaitFor(func(app.Snapshot) bool { return rs.LFS.NeedsInstall() && f.s.HasText("LFS") }, 15*time.Second); err != nil {
		t.Fatal(err)
	}
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
