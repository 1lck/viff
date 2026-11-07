//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"viff/internal/domain"
	"viff/internal/i18n"
)

// branchRowIndex is where a local branch's row sits in the panel's row list, so
// a test can address the row's own controls instead of guessing at pixels.
func branchRowIndex(t *testing.T, w *Window, branch string) int {
	t.Helper()
	w.buildBranchRows()
	for i, row := range w.lg.brRows {
		if row.ref != nil && row.ref.Kind == domain.RefLocal && row.ref.Short == branch {
			return i
		}
	}
	t.Fatalf("%q is not in the branch rows", branch)
	return -1
}

// TestBranchDoubleClickChecksOutOnlyWhenEnabled pins the setting issue #50 asked
// for: a double-click on a branch row selects it and filters the log, and only
// checks the branch out when that setting is on. Local changes still stop the
// checkout behind the usual confirmation, and the row's context menu keeps its
// explicit checkout either way.
func TestBranchDoubleClickChecksOutOnlyWhenEnabled(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	// openRepo may already have the Git tool window up, so show it rather than
	// toggling it shut; tab 0 is the log, whose left column lists the branches.
	// The remote group is folded, because "origin/feature/search" carries the
	// same row title as the local branch and a label click would be ambiguous.
	f.win.gitOpen, f.win.gitTab = true, 0
	f.win.lg.brClosed = map[string]bool{"g:remote": true}
	f.win.Invalidate()
	f.settle()

	head := func() string { return strings.TrimSpace(f.git("rev-parse", "--abbrev-ref", "HEAD")) }
	if got := head(); got != "main" {
		t.Fatalf("starting branch = %q, want main", got)
	}
	if f.ws.Settings.CheckoutOnDoubleClick {
		t.Fatal("a fresh fixture enabled checkout on double-click")
	}

	// The gesture itself, with the setting off: select the row and filter the
	// log without touching the working tree. The fixture has uncommitted
	// changes, which is what a checkout would have to work around, and no dialog
	// may appear because nothing tries to check anything out.
	if err := f.s.DoubleClickLabel("feature/search"); err != nil {
		t.Fatalf("double-click the branch: %v\npainted: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.settle()
	if got := head(); got != "main" {
		t.Fatalf("double-click checked out %q with the setting off", got)
	}
	if f.win.dialog != nil {
		t.Fatalf("double-click opened %q with the setting off", f.win.dialog.title)
	}
	if got := f.ws.Repo.Log.Filter.Branch; got != "feature/search" {
		t.Fatalf("double-click did not filter the log: branch filter = %q", got)
	}
	if got := f.win.lg.brSel; got != "refs/heads/feature/search" {
		t.Fatalf("double-click did not select the row: %q", got)
	}

	// The handler every paint registers for the row, invoked directly: the log
	// paints a branch badge with the same text, so a label click from here on
	// could land on the badge instead of the row.
	doubleRow := func() {
		t.Helper()
		id := scBranchRows.At(branchRowIndex(t, f.win, "feature/search"))
		fn, ok := f.win.doubles[id]
		if !ok {
			t.Fatal("the branch row registered no double-click handler")
		}
		fn()
		f.win.Invalidate()
		f.s.Settle(40*time.Millisecond, 2*time.Second)
		f.settle()
	}

	// Off: the same handler must stay a no-op.
	doubleRow()
	if got := head(); got != "main" {
		t.Fatalf("the row handler checked out %q with the setting off", got)
	}

	// On, with local changes: the confirmation still stands between the click and
	// the working tree, so nothing is stashed or discarded behind the user.
	f.ws.Settings.CheckoutOnDoubleClick = true
	f.win.Invalidate()
	f.settle()
	doubleRow()
	if f.win.dialog == nil {
		t.Fatal("double-click checked out over local changes without asking")
	}
	if got := head(); got != "main" {
		t.Fatalf("double-click left the branch at %q past the confirmation", got)
	}
	if out := f.git("stash", "list"); strings.TrimSpace(out) != "" {
		t.Fatalf("double-click stashed the local changes: %q", out)
	}
	f.s.Keys("escape")
	f.settle()

	// On, clean tree: the same gesture checks the branch out, through the same
	// entry the context menu uses.
	f.git("stash", "push", "-q", "-u", "-m", "test dirty tree")
	f.ws.Refresh(true)
	f.settle()
	doubleRow()
	if got := head(); got != "feature/search" {
		t.Fatalf("double-click left the branch at %q with the setting on", got)
	}

	// The context menu keeps an explicit checkout whatever the setting says.
	f.git("checkout", "-q", "main")
	f.ws.Refresh(true)
	f.settle()
	f.ws.Settings.CheckoutOnDoubleClick = false
	f.win.Invalidate()
	f.settle()
	var checkout item
	found := false
	for _, ref := range f.ws.Repo.RefsOf(domain.RefLocal) {
		if ref.Short != "feature/search" {
			continue
		}
		for _, it := range f.win.refActions(ref) {
			if it.title == f.win.t(i18n.ChromeActionCheckout) {
				checkout, found = it, true
			}
		}
	}
	if !found {
		t.Fatal("the branch's context menu offers no checkout")
	}
	checkout.run()
	f.settle()
	if got := head(); got != "feature/search" {
		t.Fatalf("the context menu left the branch at %q", got)
	}
	f.assertIdle()
}
