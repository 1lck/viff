package viffui

import (
	"slices"
	"testing"

	"viff/internal/domain"
)

// Stage Selected stages only the checked rows the index can take. Staged rows
// stay untouched, and conflicted or ignored paths keep their own flows.
func TestStageSelectedPaths(t *testing.T) {
	rows := []changeRow{
		{area: domain.AreaStaged, entry: domain.StatusEntry{Path: "staged.txt", Index: 'M'}},
		{area: domain.AreaConflicts, entry: domain.StatusEntry{Path: "conflict.txt", Index: 'U', Worktree: 'U'}},
		{area: domain.AreaUntracked, entry: domain.StatusEntry{Path: "ignored.txt", Index: '!', Worktree: '!'}},
		{area: domain.AreaUnstaged, entry: domain.StatusEntry{Path: "modified.txt", Index: '.', Worktree: 'M'}},
		{area: domain.AreaUntracked, entry: domain.StatusEntry{Path: "new.txt", Index: '?', Worktree: '?'}},
		{area: domain.AreaUnstaged, entry: domain.StatusEntry{Path: "modified.txt", Index: '.', Worktree: 'M'}},
	}
	want := []string{"modified.txt", "new.txt"}
	if got := stageSelectedPaths(rows); !slices.Equal(got, want) {
		t.Fatalf("stageSelectedPaths = %v, want %v", got, want)
	}
}
