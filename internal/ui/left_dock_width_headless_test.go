//go:build vivahead

package viffui

import (
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	"viff/internal/storage"
)

// TestLeftDockWidthStaysPutAcrossTabs pins issue #87: the Project tree and the
// Commit panel are two pages of the same left dock, so they must use one width
// and one clamp. Switching pages keeps the splitter where it was, and a drag on
// either page is visible on the other without another drag.
func TestLeftDockWidthStaysPutAcrossTabs(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	if got := f.ws.Settings.LeftPanelWidth; got != storage.DefaultLeftPanelWidth {
		t.Fatalf("default left dock width = %v, want %v", got, storage.DefaultLeftPanelWidth)
	}
	commitX := leftDockSplitterX(t, f, idSplitCommit)

	// Drag the Commit page's splitter. The one shared field follows the pointer.
	dragLeftDockSplitter(t, f, idSplitCommit, 64)
	width := f.ws.Settings.LeftPanelWidth
	if want := storage.DefaultLeftPanelWidth + 64; width < want-1 || width > want+1 {
		t.Fatalf("dragged width = %v, want about %v", width, want)
	}
	movedX := leftDockSplitterX(t, f, idSplitCommit)
	if want := commitX + 64; movedX < want-1 || movedX > want+1 {
		t.Fatalf("Commit splitter x = %v after a 64px drag, want about %v", movedX, want)
	}
	commitX = movedX

	// Switch to the Project page: the splitter must stay put and the shared
	// value must not change.
	switchLeftDockPage(t, f, "cmd+1")
	projectX := leftDockSplitterX(t, f, idSplitProject)
	if diff := projectX - commitX; diff < -0.5 || diff > 0.5 {
		t.Fatalf("splitter moved on the tab switch: Commit %v, Project %v", commitX, projectX)
	}
	if got := f.ws.Settings.LeftPanelWidth; got != width {
		t.Fatalf("width changed on the tab switch: %v -> %v", width, got)
	}

	// Drag the Project page's splitter; the Commit page must agree when it
	// comes back.
	dragLeftDockSplitter(t, f, idSplitProject, -40)
	narrowed := f.ws.Settings.LeftPanelWidth
	if want := width - 40; narrowed < want-1 || narrowed > want+1 {
		t.Fatalf("width after the Project drag = %v, want about %v", narrowed, want)
	}
	projectX = leftDockSplitterX(t, f, idSplitProject)

	switchLeftDockPage(t, f, "cmd+0")
	commitX = leftDockSplitterX(t, f, idSplitCommit)
	if diff := commitX - projectX; diff < -0.5 || diff > 0.5 {
		t.Fatalf("Commit page did not follow the Project drag: Project %v, Commit %v", projectX, commitX)
	}
	if got := f.ws.Settings.LeftPanelWidth; got != narrowed {
		t.Fatalf("width = %v after switching back, want %v", got, narrowed)
	}
}

// switchLeftDockPage presses a page shortcut and waits for the new page to be
// painted. Its splitter rect is read from that paint.
func switchLeftDockPage(t *testing.T, f *uiFixture, key string) {
	t.Helper()
	if err := f.s.Keys(key); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		if key == "cmd+1" {
			return f.win.projectOpen && !f.win.commitOpen && f.win.fm.loaded
		}
		return f.win.commitOpen && !f.win.projectOpen
	}, 10*time.Second); err != nil {
		t.Fatalf("%s did not open the expected page: %v", key, err)
	}
	f.win.Invalidate()
	if err := f.s.Settle(80*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
}

// leftDockSplitterX returns the x of the open page's splitter, which is where
// the panel ends and the editor starts.
func leftDockSplitterX(t *testing.T, f *uiFixture, id ui.ID) float32 {
	t.Helper()
	for _, hit := range f.s.Snapshot().Hits {
		if hit.ID == id {
			return hit.Bounds.X
		}
	}
	t.Fatalf("left dock splitter %v is missing", id)
	return 0
}

// dragLeftDockSplitter drags the open page's splitter horizontally by dx.
func dragLeftDockSplitter(t *testing.T, f *uiFixture, id ui.ID, dx float32) {
	t.Helper()
	var hit ui.Hit
	found := false
	for _, h := range f.s.Snapshot().Hits {
		if h.ID == id {
			hit, found = h, true
			break
		}
	}
	if !found {
		t.Fatalf("left dock splitter %v is missing", id)
	}
	x := hit.Bounds.X + hit.Bounds.Width/2
	y := hit.Bounds.Y + hit.Bounds.Height/2
	if err := f.s.Press(x, y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Move(x+dx, y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Release(x+dx, y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(80*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
}
