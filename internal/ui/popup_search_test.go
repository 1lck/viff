package viffui

import (
	"fmt"
	"slices"
	"testing"

	"github.com/vivaui/viva/text"
)

func TestPopupSearchLargeList(t *testing.T) {
	p := &popup{search: text.NewEditor(""), sel: -1}
	p.items = append(p.items, header("Local"))
	for i := range 5000 {
		name := fmt.Sprintf("feature/branch-%04d", i)
		if i >= 4998 {
			name += "/match"
		}
		p.items = append(p.items, item{title: name, run: func() {}})
	}
	p.items = append(p.items, sep(), header("Remote"),
		item{title: "origin/feature/search", run: func() {}},
		sep(), header("Tags"), item{title: "release/v1", run: func() {}})
	p.visible()
	p.scroll.Y = 20000

	setText(p.search, " MATCH ")
	shown := p.visible()
	if len(shown) != 3 || p.items[shown[0]].header != "Local" {
		t.Fatalf("expected only the local header and two matches, got %v", shown)
	}
	if p.scroll.Y != 0 || p.sel != shown[1] {
		t.Fatalf("search left its first match offscreen or unselected: scroll=%v sel=%d", p.scroll.Y, p.sel)
	}
	// Cached results must not reset a user's subsequent scrolling/selection.
	p.sel, p.scroll.Y = shown[2], 28
	if !slices.Equal(p.visible(), shown) || p.sel != shown[2] || p.scroll.Y != 28 {
		t.Fatal("repainting the same query reset the result position")
	}

	setText(p.search, "ofsrch")
	shown = p.visible()
	if len(shown) != 2 || p.items[shown[0]].header != "Remote" || p.items[shown[1]].title != "origin/feature/search" {
		t.Fatalf("fuzzy search failed or left empty groups: %v", shown)
	}
	setText(p.search, "missing")
	if len(p.visible()) != 0 || p.sel != -1 {
		t.Fatal("an empty search retained an actionable selection")
	}
	// The empty message needs a row inside the popup's clipping bounds.
	if _, height := (&Window{}).popupSize(p); height != 2*popupPadY+40+popupRowH {
		t.Fatalf("empty result height = %v", height)
	}

	setText(p.search, "")
	if len(p.visible()) != len(p.items) || p.scroll.Y != 0 || p.sel != -1 {
		t.Fatal("clearing the query did not restore the full menu at the top")
	}
	if got := firstSelectable(p, -1, 1); got != 1 {
		t.Fatalf("first item after clearing = %d, want 1", got)
	}
}
