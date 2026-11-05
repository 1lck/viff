//go:build vivahead

package viffui

import (
	"slices"
	"testing"

	"viff/internal/domain"
)

// TestChangeTreeCheckboxes covers the three checkbox kinds: a file toggles
// itself, a group covers its whole area, and a directory covers only its own
// files. It also checks that a collapsed group still toggles the files it
// hides, since their rows are not built at all.
func TestChangeTreeCheckboxes(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.ws.Settings.CommitTree = true
	f.win.buildRows()
	v := &f.win.cm
	clear(v.sel)

	groupIndex := func(area domain.Area) int {
		for i, r := range v.rows {
			if r.kind == rowGroup && r.area == area {
				return i
			}
		}
		return -1
	}

	group := groupIndex(domain.AreaUnstaged)
	if group < 0 {
		t.Fatalf("no unstaged group in %+v", v.rows)
	}
	covered := v.rows[group].files
	if len(covered) < 2 {
		t.Fatalf("fixture should have at least two unstaged files, rows: %+v", v.rows)
	}

	// A group row checks every file in its area.
	f.win.toggleRowCheck(group)
	if !f.win.changesRowChecked(group) {
		t.Fatalf("group checkbox did not fill: %v", v.sel)
	}
	for _, key := range covered {
		if !v.sel[key] {
			t.Fatalf("group check left %q unselected: %v", key, v.sel)
		}
	}

	// A second click clears the same set and nothing else.
	f.win.toggleRowCheck(group)
	if f.win.changesRowChecked(group) {
		t.Fatalf("group checkbox did not clear: %v", v.sel)
	}
	if len(v.sel) != 0 {
		t.Fatalf("group check cleared into %v", v.sel)
	}

	// A directory row covers its own files only.
	dir := -1
	for i, r := range v.rows {
		if r.kind == rowDir && r.area == domain.AreaUnstaged {
			dir = i
			break
		}
	}
	if dir < 0 {
		t.Fatalf("no unstaged directory row in %+v", v.rows)
	}
	dirFiles := v.rows[dir].files
	if len(dirFiles) == 0 {
		t.Fatalf("directory row covers no files: %+v", v.rows[dir])
	}
	f.win.toggleRowCheck(dir)
	if !f.win.changesRowChecked(dir) {
		t.Fatalf("directory checkbox did not fill: %v", v.sel)
	}
	for i, r := range v.rows {
		if i == dir || r.kind != rowFile || r.area != domain.AreaUnstaged {
			continue
		}
		if !slices.Contains(dirFiles, r.key) && v.sel[r.key] {
			t.Fatalf("directory check selected %q outside it: %v", r.key, v.sel)
		}
	}

	// Clicking the row itself selects the group key, which selectedEntries
	// already expands to every file, so the box has to show that state too.
	clear(v.sel)
	f.win.clickRow(group)
	if !f.win.changesRowChecked(group) {
		t.Fatalf("row selection did not fill the group checkbox: %v", v.sel)
	}
	f.win.toggleRowCheck(group)
	if f.win.changesRowChecked(group) || len(v.sel) != 0 {
		t.Fatalf("clearing the box left %v", v.sel)
	}

	// Collapsing the group removes the file rows; its checkbox must still work.
	clear(v.sel)
	v.collapsed[v.rows[group].key] = true
	f.win.buildRows()
	group = groupIndex(domain.AreaUnstaged)
	if group < 0 {
		t.Fatalf("collapsed group vanished from %+v", v.rows)
	}
	f.win.toggleRowCheck(group)
	if !f.win.changesRowChecked(group) {
		t.Fatalf("collapsed group checkbox did not fill: %v", v.sel)
	}
	for _, key := range covered {
		if !v.sel[key] {
			t.Fatalf("collapsed group check left %q unselected: %v", key, v.sel)
		}
	}
}

// TestChangeTreeCheckboxFile keeps a file's own checkbox independent from its
// row selection: checking it selects that file and no other.
func TestChangeTreeCheckboxFile(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.ws.Settings.CommitTree = true
	f.win.buildRows()
	v := &f.win.cm
	clear(v.sel)

	file := -1
	for i, r := range v.rows {
		if r.kind == rowFile && r.area == domain.AreaUnstaged {
			file = i
			break
		}
	}
	if file < 0 {
		t.Fatalf("no unstaged file row in %+v", v.rows)
	}
	f.win.toggleRowCheck(file)
	if !v.sel[v.rows[file].key] {
		t.Fatalf("file check did not select %q: %v", v.rows[file].key, v.sel)
	}
	if len(v.sel) != 1 {
		t.Fatalf("file check selected more than itself: %v", v.sel)
	}
	f.win.toggleRowCheck(file)
	if len(v.sel) != 0 {
		t.Fatalf("file check did not clear: %v", v.sel)
	}
}
