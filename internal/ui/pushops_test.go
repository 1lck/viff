//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	"viff/internal/domain"
)

func hasItem(items []item, title string) (item, bool) {
	for _, it := range items {
		if it.title == title {
			return it, true
		}
	}
	return item{}, false
}

// TestPushVariantEntries checks the three new entries: the log's push up to a
// commit, the branch popup's push to another remote, and remote tag deletion.
func TestPushVariantEntries(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	// The branch popup for the current branch offers the alternative remote.
	cur, ok := f.ws.Repo.CurrentRef()
	if !ok {
		t.Fatal("fixture has no current branch")
	}
	if _, ok := hasItem(f.win.refActions(cur), "Push to Another Remote…"); !ok {
		t.Fatal("branch actions miss Push to Another Remote…")
	}

	// A tag's actions offer deleting it on the remote.
	var tag domain.Ref
	for _, r := range f.ws.Repo.Refs {
		if r.Kind == domain.RefTag && r.Short == "v0.1.0" {
			tag = r
		}
	}
	if tag.Name == "" {
		t.Fatal("fixture tag missing")
	}
	if _, ok := hasItem(f.win.refActions(tag), "Delete Remote Tag…"); !ok {
		t.Fatal("tag actions miss Delete Remote Tag…")
	}

	// Both dialogs paint and close on Escape.
	f.win.openDeleteRemoteTag("v0.1.0")
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && f.win.dialog.title == "Delete Remote Tag"
	}, 10*time.Second); err != nil {
		t.Fatalf("delete remote tag dialog: %+v", f.win.dialog)
	}
	f.snap("50-delete-remote-tag")
	f.s.Keys("escape")
	f.s.Settle(40*time.Millisecond, time.Second)

	f.win.openPushAnother(cur)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && strings.HasPrefix(f.win.dialog.title, "Push HEAD")
	}, 10*time.Second); err != nil {
		t.Fatalf("push another dialog: %+v", f.win.dialog)
	}
	f.snap("51-push-another")
	f.s.Keys("escape")
	f.s.Settle(40*time.Millisecond, time.Second)

	// The log menu offers pushing up to a non-merge commit, through the real
	// context menu.
	var older domain.Commit
	for _, c := range f.ws.Repo.Log.Commits {
		if c.Subject == "Add store tests" {
			older = c
		}
	}
	if older.Hash == "" {
		t.Fatal("fixture commit missing")
	}
	f.win.lg.sel = map[string]bool{older.Hash: true}
	f.win.Invalidate()
	if err := f.s.RightClickLabel(older.Subject); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if _, ok := hasItem(f.win.commitMenu(), "Push Up to Commit…"); !ok {
		t.Fatalf("log menu misses the entry; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.click("Push Up to Commit…")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && strings.HasPrefix(f.win.dialog.title, "Push Up to")
	}, 10*time.Second); err != nil {
		t.Fatalf("push up to dialog: %+v", f.win.dialog)
	}
	if !strings.Contains(f.win.dialog.title, domain.ShortHash(older.Hash)) {
		t.Fatalf("dialog title = %q", f.win.dialog.title)
	}
	f.snap("52-push-up-to")
	f.s.Keys("escape")
	f.s.Settle(40*time.Millisecond, time.Second)

	// A merge commit makes "up to" ambiguous, so the entry is disabled.
	var merge domain.Commit
	for _, c := range f.ws.Repo.Log.Commits {
		if len(c.Parents) > 1 {
			merge = c
			break
		}
	}
	if merge.Hash == "" {
		t.Fatal("fixture merge commit missing")
	}
	f.win.lg.sel = map[string]bool{merge.Hash: true}
	it, ok := hasItem(f.win.commitMenu(), "Push Up to Commit…")
	if !ok || !it.disabled {
		t.Fatalf("merge commit entry = %+v, present %v", it, ok)
	}
}
