//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

func menuItem(items []item, title string) (item, bool) {
	for _, it := range items {
		if it.title == title {
			return it, true
		}
	}
	return item{}, false
}

func menuTitles(items []item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.title)
	}
	return out
}

// TestBranchUpdateMenus covers the two entry points: "Update from Remote" for
// a local branch this window has not checked out, and the unshallow action
// that only a shallow clone offers.
func TestBranchUpdateMenus(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	rs := f.win.repo()
	var side, cur domain.Ref
	for _, r := range rs.Refs {
		if r.Kind != domain.RefLocal {
			continue
		}
		switch {
		case r.Short == "feature/search":
			side = r
		case r.Current:
			cur = r
		}
	}
	if side.Short == "" {
		t.Fatalf("no feature/search among %+v", rs.Refs)
	}
	upd, ok := menuItem(f.win.refActions(side), "Update from Remote")
	if !ok {
		t.Fatalf("no update item in %v", menuTitles(f.win.refActions(side)))
	}
	if upd.disabled {
		t.Fatal("update item disabled although the repository has a remote")
	}
	if _, ok := menuItem(f.win.refActions(cur), "Update from Remote"); ok {
		t.Fatal("the current branch must be updated by pulling, not from this item")
	}

	if _, ok := menuItem(f.win.gitWindowMenu(), "Fetch Full History (Unshallow)"); ok {
		t.Fatal("unshallow is offered for a complete clone")
	}
	rs.Shallow = true
	if _, ok := menuItem(f.win.gitWindowMenu(), "Fetch Full History (Unshallow)"); !ok {
		t.Fatalf("unshallow missing for a shallow clone: %v", menuTitles(f.win.gitWindowMenu()))
	}
	rs.Shallow = false
}

// TestUpdateBranchFromMenu runs the menu action end to end against the
// fixture's real remote.
func TestUpdateBranchFromMenu(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	remote := filepath.Join(filepath.Dir(f.dir), "remote.git")
	push := filepath.Join(t.TempDir(), "push")
	f.gitIn(filepath.Dir(push), "clone", "-q", remote, push)
	f.gitIn(push, "config", "user.name", "Other")
	f.gitIn(push, "config", "user.email", "other@example.com")
	f.gitIn(push, "config", "commit.gpgsign", "false")
	f.gitIn(push, "checkout", "-q", "feature/search")
	if err := os.WriteFile(filepath.Join(push, "search.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.gitIn(push, "add", "-A")
	f.gitIn(push, "commit", "-q", "-m", "Search from someone else")
	tip := strings.TrimSpace(f.gitIn(push, "rev-parse", "HEAD"))
	f.gitIn(push, "push", "-q", "origin", "feature/search")

	var side domain.Ref
	for _, r := range f.win.repo().Refs {
		if r.Kind == domain.RefLocal && r.Short == "feature/search" {
			side = r
		}
	}
	upd, ok := menuItem(f.win.refActions(side), "Update from Remote")
	if !ok {
		t.Fatalf("no update item in %v", menuTitles(f.win.refActions(side)))
	}
	// Menu handlers run on the UI thread; the test goroutine only queues it.
	f.win.after(func() { upd.run() })
	f.settle()

	if got := strings.TrimSpace(f.git("rev-parse", "refs/heads/feature/search")); got != tip {
		t.Fatalf("branch = %s, want %s", got, tip)
	}
	if got := strings.TrimSpace(f.git("rev-parse", "refs/remotes/origin/feature/search")); got != tip {
		t.Fatalf("tracking = %s, want %s", got, tip)
	}
	n := f.ws.Notifications[len(f.ws.Notifications)-1]
	if n.Kind != vapp.NoticeSuccess || n.Title != "Updated feature/search" {
		t.Fatalf("notice = %+v", n)
	}
	if !strings.Contains(n.Body, "1 new commits") {
		t.Fatalf("notice body = %q", n.Body)
	}
}
