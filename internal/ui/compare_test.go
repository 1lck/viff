//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	"viff/internal/domain"
)

// TestCompareCommitsView selects two commits in the log, opens the comparison
// from the context menu, and doubles a changed file into a diff tab.
func TestCompareCommitsView(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	var older, newer domain.Commit
	for _, c := range f.ws.Repo.Log.Commits {
		switch c.Subject {
		case "Initial commit":
			older = c
		case "Add store tests":
			newer = c
		}
	}
	if older.Hash == "" || newer.Hash == "" {
		t.Fatalf("fixture commits missing: %v", f.ws.Repo.Log.Commits)
	}

	// Select both rows the way a ⌘-click does.
	f.win.lg.sel[older.Hash] = true
	f.win.lg.sel[newer.Hash] = true
	f.win.Invalidate()
	if err := f.s.RightClickLabel(newer.Subject); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("Compare Commits")

	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && strings.HasPrefix(f.win.dialog.title, "Compare ")
	}, 10*time.Second); err != nil {
		t.Fatalf("compare dialog did not open: %+v", f.win.dialog)
	}
	if got := f.win.dialog.title; !strings.Contains(got, domain.ShortHash(older.Hash)) || !strings.Contains(got, domain.ShortHash(newer.Hash)) {
		t.Fatalf("title = %q", got)
	}

	path := "internal/store/store_test.go"
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		_, ok := f.s.Hit(path)
		return ok
	}, 10*time.Second); err != nil {
		t.Fatalf("file row missing; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if _, ok := f.s.Hit("README.md"); ok {
		t.Fatal("unchanged file listed")
	}
	f.snap("40-compare-commits")

	if err := f.s.DoubleClickLabel(path); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return f.win.dialog == nil && tab != nil && !tab.loading
	}, 10*time.Second); err != nil {
		t.Fatalf("diff tab did not open: dialog=%+v", f.win.dialog)
	}
	tab := f.win.ed.cur()
	if tab.src.Commit != newer.Hash || tab.src.Parent != older.Hash || tab.src.Path != path {
		t.Fatalf("tab source = %+v", tab.src)
	}
	if tab.res == nil || len(tab.res.New.Lines) == 0 || tab.res.New.Lines[0] != "package store" {
		t.Fatalf("diff result = %+v", tab.res)
	}
	f.snap("41-compare-diff")
	f.assertIdle()
}

// TestCompareBranchFiles opens the same view for two branch tips.
func TestCompareBranchFiles(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	rs := f.ws.Repo
	var feature domain.Ref
	for _, r := range rs.RefsOf(domain.RefLocal) {
		if r.Short == "feature/search" {
			feature = r
		}
	}
	if feature.Target == "" {
		t.Fatalf("feature branch missing: %+v", rs.Refs)
	}
	head := rs.Status.Branch.Head
	f.win.compareBranchFiles(feature)

	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && strings.Contains(f.win.dialog.title, domain.ShortHash(head)) && strings.Contains(f.win.dialog.title, domain.ShortHash(feature.Target))
	}, 10*time.Second); err != nil {
		t.Fatalf("branch compare dialog: %+v", f.win.dialog)
	}
	want := strings.Fields(strings.TrimSpace(f.git("diff", "--name-only", "main", "feature/search")))
	if len(want) == 0 {
		t.Fatal("fixture branches do not differ")
	}
	f.s.Settle(60*time.Millisecond, time.Second)
	for _, p := range want {
		if err := f.s.WaitFor(func(app.Snapshot) bool { _, ok := f.s.Hit(p); return ok }, 10*time.Second); err != nil {
			t.Fatalf("branch compare misses %s: %s", p, strings.Join(f.s.Texts(), " | "))
		}
	}
	f.snap("42-compare-branches")
}
