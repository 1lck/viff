//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// TestSmartCheckoutDialog drives the stashing checkout from the UI: a dirty
// working tree blocks the branch switch, the confirmation appears, and the
// choice is carried out. Escape must leave everything untouched.
func TestSmartCheckoutDialog(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	var ref domain.Ref
	for _, r := range f.ws.Repo.Refs {
		if r.Kind == domain.RefLocal && r.Short == "feature/search" {
			ref = r
		}
	}
	if ref.Short == "" {
		t.Fatal("branch feature/search not found")
	}

	waitDialog := func() {
		t.Helper()
		if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 10*time.Second); err != nil {
			t.Fatalf("confirmation never opened: %v\nnotices: %s", err, notices(f))
		}
		f.s.Settle(40*time.Millisecond, time.Second)
	}

	// Escape keeps the current branch and creates no stash.
	f.win.checkoutBranch(ref)
	waitDialog()
	if got := f.win.dialog.title; got != "Stash Changes and Checkout" {
		t.Fatalf("dialog title = %q", got)
	}
	f.s.Keys("escape")
	f.s.Settle(40*time.Millisecond, time.Second)
	f.settle()
	if branch := strings.TrimSpace(f.git("rev-parse", "--abbrev-ref", "HEAD")); branch != "main" {
		t.Fatalf("Escape switched branch to %q", branch)
	}
	if list := strings.TrimSpace(f.git("stash", "list")); list != "" {
		t.Fatalf("Escape created a stash:\n%s", list)
	}

	// Accepting stashes, checks out and restores; the fixture's local edits
	// overlap the branch, so the changes stay in the stash.
	f.win.checkoutBranch(ref)
	waitDialog()
	f.click("Stash and Checkout")
	f.settle()

	if branch := strings.TrimSpace(f.git("rev-parse", "--abbrev-ref", "HEAD")); branch != "feature/search" {
		t.Fatalf("branch after the stashing checkout = %q, want feature/search", branch)
	}
	if list := strings.TrimSpace(f.git("stash", "list")); !strings.Contains(list, "viff: checkout feature/search") {
		t.Fatalf("conflicting local changes must stay in the stash:\n%s", list)
	}
	var warning bool
	for _, n := range f.ws.Notifications {
		if n.Kind == vapp.NoticeWarning && strings.Contains(n.Body, "still in the stash") {
			warning = true
		}
	}
	if !warning {
		t.Fatalf("no warning about the kept stash: %s", notices(f))
	}
}

// notices formats the notifications for a failure message.
func notices(f *uiFixture) string {
	var b strings.Builder
	for _, n := range f.ws.Notifications {
		b.WriteString(n.Title)
		b.WriteString(": ")
		b.WriteString(strings.ReplaceAll(n.Body, "\n", " "))
		b.WriteString(" | ")
	}
	return b.String()
}
