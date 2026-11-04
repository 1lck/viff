package viffui

import (
	"reflect"
	"testing"

	"viff/internal/domain"
)

func TestPatchTargets(t *testing.T) {
	row := func(area domain.Area, path, orig string) changeRow {
		return changeRow{area: area, kind: rowFile, entry: domain.StatusEntry{Path: path, OrigPath: orig}}
	}
	cases := []struct {
		name   string
		sel    []changeRow
		paths  []string
		staged bool
		ok     bool
	}{
		{
			name:  "one unstaged file",
			sel:   []changeRow{row(domain.AreaUnstaged, "a.txt", "")},
			paths: []string{"a.txt"},
			ok:    true,
		},
		{
			name:   "a staged file takes the index",
			sel:    []changeRow{row(domain.AreaStaged, "a.txt", "")},
			paths:  []string{"a.txt"},
			staged: true,
			ok:     true,
		},
		{
			name:   "a rename keeps both names",
			sel:    []changeRow{row(domain.AreaStaged, "new.txt", "old.txt")},
			paths:  []string{"old.txt", "new.txt"},
			staged: true,
			ok:     true,
		},
		{
			name: "mixed areas need two patches",
			sel:  []changeRow{row(domain.AreaStaged, "a.txt", ""), row(domain.AreaUnstaged, "b.txt", "")},
		},
		{
			name: "untracked files have no diff",
			sel:  []changeRow{row(domain.AreaUntracked, "note.txt", "")},
		},
		{
			name: "conflicts have no patch",
			sel:  []changeRow{row(domain.AreaConflicts, "c.txt", "")},
		},
		{
			name:  "untracked next to a changed file is skipped",
			sel:   []changeRow{row(domain.AreaUntracked, "note.txt", ""), row(domain.AreaUnstaged, "a.txt", "")},
			paths: []string{"a.txt"},
			ok:    true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			paths, staged, ok := patchTargets(c.sel)
			if ok != c.ok || staged != c.staged || !reflect.DeepEqual(paths, c.paths) {
				t.Fatalf("patchTargets = %v %v %v, want %v %v %v", paths, staged, ok, c.paths, c.staged, c.ok)
			}
		})
	}
}
