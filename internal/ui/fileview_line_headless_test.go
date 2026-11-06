//go:build vivahead

package viffui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
)

// TestFileViewOpenAtLine covers content-search navigation: opening a file at a
// line scrolls there once the text is ready, and a second hit in the same tab
// moves the viewport again instead of keeping the old offset.
func TestFileViewOpenAtLine(t *testing.T) {
	f := newUIFixture(t)
	const file = "internal/store/long.go"
	var b strings.Builder
	b.WriteString("package store\n")
	for i := 2; i <= 240; i++ {
		fmt.Fprintf(&b, "// generated line %d\n", i)
	}
	f.write(file, b.String())

	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.win.openRepoFileAt(file, 30, true)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return tab != nil && !tab.loading && tab.src.View && tab.src.Path == file &&
			tab.pendingLine == 0 && tab.y > 0
	}, 10*time.Second); err != nil {
		t.Fatalf("open at line 30 did not scroll the file view: %v", err)
	}
	f.settle()
	first := f.win.ed.cur().y
	if first <= 0 {
		t.Fatalf("line 30 left the view at the top: y=%v", first)
	}

	// The same file and tab, a different line: only the viewport moves.
	f.win.openRepoFileAt(file, 180, true)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		tab := f.win.ed.cur()
		return tab != nil && !tab.loading && tab.src.Path == file &&
			tab.pendingLine == 0 && tab.y > first
	}, 10*time.Second); err != nil {
		t.Fatalf("open at line 180 did not move the viewport: %v", err)
	}
	f.settle()
	tab := f.win.ed.cur()
	if tab.y <= first {
		t.Fatalf("second line did not advance the scroll offset: first %v, now %v", first, tab.y)
	}
	if len(f.win.ed.tabs) != 1 {
		t.Fatalf("the same file opened a second tab: %d tabs", len(f.win.ed.tabs))
	}
}
