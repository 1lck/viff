//go:build vivahead

package viffui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
)

// TestSearchDialogContents drives both pages of the search dialog: the Files
// tab, a switch to Contents, a debounced query answered from the working tree,
// opening the hit at its line, Esc closing, and the no-match state.
func TestSearchDialogContents(t *testing.T) {
	f := newUIFixture(t)

	// A file long enough that revealing a late line actually scrolls the
	// viewer; the fixture's own files fit on one screen.
	lines := make([]string, 400)
	for i := range lines {
		lines[i] = "padding line " + strconv.Itoa(i)
	}
	lines[349] = "const viffSearchNeedle = 1"
	f.write("long/search.txt", strings.Join(lines, "\n")+"\n")

	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	// Files is still the default page; its tab leaves the path filter in place.
	if err := f.s.Keys("cmd+shift+o"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 5*time.Second); err != nil {
		t.Fatalf("Open File did not open: %v", err)
	}
	if !f.s.HasText("Files") || !f.s.HasText("Contents") {
		t.Fatalf("the dialog has no page tabs; texts: %s", strings.Join(f.s.Texts(), " | "))
	}

	f.click("Contents")
	f.s.Settle(80*time.Millisecond, time.Second)
	t.Logf("focused after Contents: %v texts: %s", f.win.c.Focused(), strings.Join(f.s.Texts(), " | "))
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.focused(idSearchInput) }, 5*time.Second); err != nil {
		t.Fatalf("Contents page did not take the text focus: %v", err)
	}
	if !f.s.HasText("Type to search the working tree") {
		t.Fatalf("the empty Contents page has no hint; texts: %s", strings.Join(f.s.Texts(), " | "))
	}

	if err := f.s.Type("viffSearchNeedle"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.s.HasText("1 match") }, 10*time.Second); err != nil {
		t.Fatalf("the query returned no counted result: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("long/search.txt:350") {
		t.Fatalf("the hit row is missing; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("54-find-in-files")

	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return f.win.dialog == nil && tab != nil && !tab.loading && tab.src.View &&
			tab.src.Path == "long/search.txt" && tab.pendingLine == 0 && tab.y > 0
	}, 10*time.Second); err != nil {
		t.Fatalf("Enter did not open the hit at line 350: %v", err)
	}

	// The shortcut opens the Contents page directly; Esc closes it.
	if err := f.s.Keys("cmd+shift+f"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 5*time.Second); err != nil {
		t.Fatalf("Find in Files did not open: %v", err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.focused(idSearchInput) }, 5*time.Second); err != nil {
		t.Fatalf("Find in Files did not focus the content field: %v", err)
	}
	if err := f.s.Type("viff-no-such-needle"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.s.HasText("No matches in the working tree") }, 10*time.Second); err != nil {
		t.Fatalf("the empty answer was not reported: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}

	if err := f.s.Keys("escape"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog == nil }, 5*time.Second); err != nil {
		t.Fatalf("Esc did not close the dialog: %v", err)
	}

	f.assertIdle()
}
