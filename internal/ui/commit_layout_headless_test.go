//go:build vivahead

package viffui

import (
	"testing"

	"github.com/vivaui/viva/ui"
)

// TestCommitPanelStaysInsideDockAtMinimumHeight pins issue #101: at small
// window heights the bottom Git panel leaves the left dock less vertical room
// than the Commit panel's fixed chrome needs. The message area must shrink to
// fit (and the Git panel must yield enough room) instead of the Amend row, the
// scope hint and the Commit buttons spilling over the Log panel below.
func TestCommitPanelStaysInsideDockAtMinimumHeight(t *testing.T) {
	f := newUIFixture(t)
	f.start(900, 560)
	f.win.openRepo(f.dir)
	f.settle()

	gitTop := dockHitBounds(t, f, idSplitGit).Y
	for _, id := range []ui.ID{idCmCommit, idCmCommitPush, idCmMessage} {
		b := dockHitBounds(t, f, id)
		if bottom(b) > gitTop+0.5 {
			t.Fatalf("%v bottom %v crosses the Git panel top %v", id, bottom(b), gitTop)
		}
	}
	// The minimum window size is a supported configuration: the buttons keep
	// their full height there instead of being clipped away.
	if got := dockHitBounds(t, f, idCmCommit).Height; got < buttonH-0.5 {
		t.Fatalf("Commit button height = %v at the minimum window size, want %v", got, buttonH)
	}
}

// TestCommitPanelStaysInsideDockDuringMerge covers the squeeze path: a merge
// in progress adds the operation banner and scope warnings, so even at the
// minimum window height the bottom section no longer fits and the message
// editor must shrink past its usual floor. Whatever the squeeze leaves, no
// dock control may cross into the Git panel (issue #101).
func TestCommitPanelStaysInsideDockDuringMerge(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(900, 560)
	f.win.openRepo(f.dir)
	f.settle()

	gitTop := dockHitBounds(t, f, idSplitGit).Y
	dockRight := dockHitBounds(t, f, idSplitCommit).X
	for _, h := range f.s.Snapshot().Hits {
		if h.ID == idSplitGit || h.ID == idSplitCommit || h.ID == idSplitMessage {
			continue
		}
		// The Git panel spans the full width below the dock, so hits are
		// filtered by the dock's x range; anything in that range starting
		// above the boundary must not straddle it.
		cx := h.Bounds.X + h.Bounds.Width/2
		if cx < stripeW || cx >= dockRight || h.Bounds.Y >= gitTop-0.5 {
			continue
		}
		if bottom(h.Bounds) > gitTop+0.5 {
			t.Fatalf("hit %v (%q) bottom %v crosses the Git panel top %v", h.ID, h.Label, bottom(h.Bounds), gitTop)
		}
	}
}

func dockHitBounds(t *testing.T, f *uiFixture, id ui.ID) ui.Rect {
	t.Helper()
	for _, h := range f.s.Snapshot().Hits {
		if h.ID == id {
			return h.Bounds
		}
	}
	t.Fatalf("missing rendered control %v", id)
	return ui.Rect{}
}
