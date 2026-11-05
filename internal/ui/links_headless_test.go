//go:build vivahead

package viffui

import (
	"errors"
	"strings"
	"testing"
	"time"

	vapp "viff/internal/app"
)

// openLinkFixture is the UI fixture with a remote that has a web page: the
// repository's own remote is a local bare path, so the URL is changed after the
// pushes. The opener is replaced, so an action records the address it would
// open instead of starting a browser, and the branch that is compared has an
// upstream like a pushed branch does.
func openLinkFixture(t *testing.T, remoteURL string) (*uiFixture, *[]string) {
	t.Helper()
	f := newUIFixture(t)
	f.git("remote", "set-url", "origin", remoteURL)
	f.git("branch", "--set-upstream-to=origin/feature/search", "feature/search")
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	opened := []string{}
	f.win.openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	return f, &opened
}

// openBranchSubmenu opens the submenu of a branch row of the branch popup.
// Hovering is what the user does; the test takes the same path so the result
// does not depend on where the pointer happens to be.
func openBranchSubmenu(t *testing.T, f *uiFixture, branch string) {
	t.Helper()
	if len(f.win.popups) == 0 {
		t.Fatal("the branch popup is not open")
	}
	for i, it := range f.win.popups[0].items {
		if it.title == branch {
			f.win.openSubmenu(0, i, rect(240, 60, 160, 24))
			f.win.Invalidate()
			f.s.Settle(40*time.Millisecond, time.Second)
			return
		}
	}
	t.Fatalf("the branch popup has no row %q: %s", branch, strings.Join(f.s.Texts(), " | "))
}

func openBranchMenu(t *testing.T, f *uiFixture, branch string) {
	t.Helper()
	f.win.openBranchPopup(rect(240, 60, 160, 24))
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	openBranchSubmenu(t, f, branch)
}

// A local branch's submenu offers the pages of its service, and the action opens
// the address built from the remote: the branch page here, the comparison and
// the pull request form next.
func TestBranchLinkMenu(t *testing.T) {
	f, opened := openLinkFixture(t, "https://github.com/acme/viff-demo.git")
	openBranchMenu(t, f, "feature/search")
	for _, want := range []string{"Open feature/search on GitHub", "Compare with main", "Create Pull Request…"} {
		if !f.s.HasText(want) {
			t.Fatalf("the branch submenu has no %q: %s", want, strings.Join(f.s.Texts(), " | "))
		}
	}
	f.click("Open feature/search on GitHub")
	if len(*opened) != 1 || (*opened)[0] != "https://github.com/acme/viff-demo/tree/feature/search" {
		t.Fatalf("opened %v, want the branch page", *opened)
	}

	openBranchMenu(t, f, "feature/search")
	f.click("Create Pull Request…")
	want := "https://github.com/acme/viff-demo/compare/main...feature/search?expand=1"
	if len(*opened) != 2 || (*opened)[1] != want {
		t.Fatalf("opened %v, want %s", *opened, want)
	}
	f.assertIdle()
}

// Without an upstream, a remote HEAD, main or master, the base branch is not
// guessed: the entry says "Compare with…" and the click asks which branch to
// use from the list of candidates.
func TestBaseBranchPicker(t *testing.T) {
	f, _ := openLinkFixture(t, "https://github.com/acme/viff-demo.git")
	openBranchMenu(t, f, "main")
	if !f.s.HasText("Compare with…") {
		t.Fatalf("the submenu does not offer a base to choose: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.click("Compare with…")
	f.s.Settle(60*time.Millisecond, time.Second)
	if len(f.win.popups) != 1 || f.win.popups[0].title != "Choose Base Branch" {
		t.Fatalf("the base branch picker did not open: %+v", f.win.popups)
	}
	if !f.s.HasText("feature/search") || !f.s.HasText("fix/store-locking") {
		t.Fatalf("the picker has no candidates: %s", strings.Join(f.s.Texts(), " | "))
	}
}

// A service whose comparison page Viff does not build opens the branch page and
// says so, instead of opening a made-up address without a word.
func TestCompareFallsBackOnBitbucket(t *testing.T) {
	f, opened := openLinkFixture(t, "https://bitbucket.org/acme/viff-demo.git")
	openBranchMenu(t, f, "feature/search")
	if !f.s.HasText("Open feature/search on Bitbucket") {
		t.Fatalf("the submenu names the service: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.click("Compare with main")
	if len(*opened) != 1 || (*opened)[0] != "https://bitbucket.org/acme/viff-demo/branch/feature/search" {
		t.Fatalf("opened %v, want the branch page", *opened)
	}
	n := len(f.ws.Notifications)
	if n == 0 {
		t.Fatal("the fallback was silent")
	}
	last := f.ws.Notifications[n-1]
	if last.Kind != vapp.NoticeInfo || !strings.Contains(last.Title, "Bitbucket") {
		t.Fatalf("notice = %+v, want an info notice naming Bitbucket", last)
	}
}

// The log's commit menu opens the commit on the service, and a browser that
// cannot start is reported instead of leaving the entry silently dead.
func TestCommitLinkMenu(t *testing.T) {
	f, opened := openLinkFixture(t, "https://github.com/acme/viff-demo.git")
	hash := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	subject := strings.TrimSpace(f.git("log", "-1", "--format=%s"))
	if err := f.s.RightClickLabel(subject); err != nil {
		t.Fatalf("right-click the commit: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if !f.s.HasText("Open Commit on GitHub") {
		t.Fatalf("the commit menu has no service entry: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.click("Open Commit on GitHub")
	want := "https://github.com/acme/viff-demo/commit/" + hash
	if len(*opened) != 1 || (*opened)[0] != want {
		t.Fatalf("opened %v, want %s", *opened, want)
	}

	f.win.openURL = func(string) error { return errors.New("no browser here") }
	if err := f.s.RightClickLabel(subject); err != nil {
		t.Fatalf("right-click the commit again: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("Open Commit on GitHub")
	if n := len(f.ws.Notifications); n == 0 {
		t.Fatal("a failing opener was not reported")
	} else if last := f.ws.Notifications[n-1]; last.Kind != vapp.NoticeError {
		t.Fatalf("notice = %+v, want an error", last)
	}
}
