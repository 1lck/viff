//go:build vivahead

package viffui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	"viff/internal/domain"
)

// TestRevisionBrowser opens a commit's tree from the log, filters it, opens a
// file at that revision and closes the dialog with Escape.
func TestRevisionBrowser(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	var c domain.Commit
	for _, x := range f.ws.Repo.Log.Commits {
		if x.Subject == "Add store tests" {
			c = x
		}
	}
	if c.Hash == "" {
		t.Fatalf("fixture commit missing: %v", f.ws.Repo.Log.Commits)
	}

	// The log entry offers the browser for a single selected commit.
	f.win.lg.sel[c.Hash] = true
	f.win.Invalidate()
	var found bool
	for _, it := range f.win.commitMenu() {
		if strings.HasPrefix(it.title, "Browse Repository at Revision") {
			found = true
			if it.disabled {
				t.Fatal("browser item disabled for a single commit")
			}
		}
	}
	if !found {
		t.Fatal("commit menu has no revision browser item")
	}

	f.win.openRevisionBrowser(c)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && strings.HasPrefix(f.win.dialog.title, "Repository at ")
	}, 10*time.Second); err != nil {
		t.Fatalf("browser dialog did not open: %+v", f.win.dialog)
	}
	if got := f.win.dialog.title; !strings.Contains(got, domain.ShortHash(c.Hash)) {
		t.Fatalf("title = %q, want the short hash", got)
	}

	// The tree of "Add store tests" has the earlier files too, not just the
	// file the commit changed.
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		_, ok := f.s.Hit("internal/store/store.go")
		return ok
	}, 10*time.Second); err != nil {
		t.Fatalf("tree rows missing; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	for _, p := range []string{"README.md", "config.json", "internal/store/store_test.go"} {
		if _, ok := f.s.Hit(p); !ok {
			t.Fatalf("%s not listed in the tree", p)
		}
	}
	f.snap("48-revision-browser")

	// Filtering narrows the list; Enter opens the only remaining match.
	f.click("Filter files")
	if err := f.s.Type("config"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	if _, ok := f.s.Hit("internal/store/store.go"); ok {
		t.Fatal("filtered-out file still painted")
	}
	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return tab != nil && !tab.loading && tab.src.Path == "config.json" && tab.src.Commit == c.Hash && tab.src.View
	}, 10*time.Second); err != nil {
		t.Fatalf("Enter did not open config.json at the revision; dialog %+v", f.win.dialog)
	}
	if f.win.dialog != nil {
		t.Fatal("dialog stayed open after opening a file")
	}

	// Double-clicking a row opens that file at the same revision.
	f.win.openRevisionBrowser(c)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		_, ok := f.s.Hit("internal/store/store.go")
		return ok
	}, 10*time.Second); err != nil {
		t.Fatalf("browser did not reopen: %v", err)
	}
	// Let the dialog take focus before the pointer arrives.
	f.s.Settle(120*time.Millisecond, 2*time.Second)
	if err := f.s.DoubleClickLabel("internal/store/store.go"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return tab != nil && !tab.loading && tab.src.Path == "internal/store/store.go" && tab.src.Commit == c.Hash && tab.src.View
	}, 10*time.Second); err != nil {
		t.Fatalf("double-click did not open store.go: %v", err)
	}

	// Escape closes the dialog.
	f.win.openRevisionBrowser(c)
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Keys("escape"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog == nil }, 5*time.Second); err != nil {
		t.Fatal("Escape did not close the browser")
	}
}

// TestRevisionBrowserVirtualizesLongTrees checks that a tree with hundreds of
// files only registers the rows that are on screen.
func TestRevisionBrowserVirtualizesLongTrees(t *testing.T) {
	f := newUIFixture(t)
	for i := 0; i < 300; i++ {
		f.write(fmt.Sprintf("bulk/%03d.txt", i), "x\n")
	}
	f.commit("Bulk import", "2026-10-13T09:00:00")
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	var c domain.Commit
	for _, x := range f.ws.Repo.Log.Commits {
		if x.Subject == "Bulk import" {
			c = x
		}
	}
	if c.Hash == "" {
		t.Fatal("bulk commit missing")
	}
	f.win.openRevisionBrowser(c)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		_, ok := f.s.Hit("bulk/000.txt")
		return ok
	}, 10*time.Second); err != nil {
		t.Fatalf("first bulk row missing: %v", err)
	}
	if _, ok := f.s.Hit("bulk/299.txt"); ok {
		t.Fatal("a row far off screen was painted")
	}
	f.snap("49-revision-browser-long")
}
