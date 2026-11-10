//go:build vivahead

package viffui

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/ui"

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

// Opening a checked group or directory's menu must retain its file checks,
// including selections in another area and files hidden by collapsed rows.
func TestChangeTreeContextMenuPreservesChecks(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflicts/file.txt", "base\n", "ours\n", "theirs\n")
	f.write("staged/one.txt", "staged one\n")
	f.write("staged/two.txt", "staged two\n")
	f.git("add", "staged")
	f.write("main.go", mainV1)
	f.write("internal/store/store.go", storeV2)
	f.write("untracked/one.txt", "untracked one\n")
	f.write("untracked/two.txt", "untracked two\n")
	f.start(1440, 1200)
	f.win.openRepo(f.dir)
	f.settle()
	for _, mode := range []struct {
		name  string
		tree  bool
		theme string
	}{
		{"flat-dark", false, "dark"},
		{"tree-dark", true, "dark"},
		{"tree-light", true, "light"},
	} {
		f.ws.Settings.CommitTree = mode.tree
		f.ws.Settings.Theme = mode.theme
		f.win.applyTheme()
		for _, area := range areaOrder {
			for _, kind := range []int{rowGroup, rowDir} {
				if kind == rowDir && !mode.tree {
					continue
				}
				for _, collapsed := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/kind=%d/collapsed=%t", mode.name, f.win.areaTitle(area), kind, collapsed), func(t *testing.T) {
						v := &f.win.cm
						f.win.closePopups()
						clear(v.sel)
						clear(v.collapsed)
						f.win.buildRows()
						i := slices.IndexFunc(v.rows, func(row changeRow) bool { return row.area == area && row.kind == kind })
						if i < 0 || len(v.rows[i].files) == 0 {
							t.Fatal("fixture has no nonempty target row")
						}
						row := v.rows[i]
						v.collapsed[row.key] = collapsed
						f.win.Invalidate()
						f.settle()
						i = f.win.rowIndex(row.key)
						hit := changeCheckHit(t, f, scChangeChecks.At(i))
						if err := f.s.Click(hit.Bounds.X+8, hit.Bounds.Y+hit.Bounds.Height/2); err != nil {
							t.Fatal(err)
						}
						f.settle()
						for _, key := range row.files {
							if !v.sel[key] {
								t.Fatalf("checkbox did not select %q", key)
							}
						}
						// An already checked row keeps the entire multiselection.
						outside := slices.IndexFunc(v.rows, func(r changeRow) bool { return r.kind == rowFile && r.area != area })
						if outside < 0 {
							t.Fatal("fixture has no file outside the target area")
						}
						v.sel[v.rows[outside].key] = true
						v.cursor, v.anchor = v.rows[outside].key, v.rows[outside].key
						want := maps.Clone(v.sel)
						cursor, anchor := v.cursor, v.anchor
						f.win.Invalidate()
						f.settle()
						f.snap("checked-before-context-menu")
						if err := f.s.RightClickLabel(f.win.rowLabel(row)); err != nil {
							t.Fatal(err)
						}
						f.settle()
						f.snap("checked-after-context-menu")
						if !maps.Equal(v.sel, want) {
							t.Fatalf("right click changed checks: got %v, want %v", v.sel, want)
						}
						if v.cursor != cursor || v.anchor != anchor {
							t.Fatal("right click moved the selection cursor or anchor")
						}
						if len(f.win.popups) == 0 {
							t.Fatal("right click did not open a menu")
						}
						assertChangeTargets(t, f, want)
						// Check the state actually handed to the painted
						// checkboxes. Rows scrolled below the viewport have no
						// painted hit; their checks are covered by the action
						// targets asserted above.
						for j, r := range v.rows {
							if r.kind != rowFile || !want[r.key] {
								continue
							}
							hit, ok := lookupHit(f, scChangeChecks.At(j))
							if !ok {
								continue
							}
							if !hit.Checked {
								t.Errorf("file checkbox lost its check: %s", r.key)
							}
						}
						if !changeCheckHit(t, f, scChangeChecks.At(i)).Checked {
							t.Error("target checkbox lost its check")
						}
					})
				}
			}
		}
	}
}

func lookupHit(f *uiFixture, id ui.ID) (ui.Hit, bool) {
	for _, hit := range f.s.Snapshot().Hits {
		if hit.ID == id {
			return hit, true
		}
	}
	return ui.Hit{}, false
}

func changeCheckHit(t *testing.T, f *uiFixture, id ui.ID) ui.Hit {
	t.Helper()
	for _, hit := range f.s.Snapshot().Hits {
		if hit.ID == id {
			return hit
		}
	}
	t.Fatalf("missing checkbox hit %v", id)
	return ui.Hit{}
}

func assertChangeTargets(t *testing.T, f *uiFixture, want map[string]bool) {
	t.Helper()
	got := map[string]bool{}
	for _, row := range f.win.selectedEntries() {
		if got[row.key] {
			t.Errorf("duplicate action target %q", row.key)
		}
		got[row.key] = true
	}
	if !maps.Equal(got, want) {
		t.Fatalf("action targets = %v, want %v", got, want)
	}
}

// A partial or unselected row still becomes the context target; right-clicking
// an already selected file keeps its peers, just like a fully checked group.
func TestChangeTreeContextMenuSelection(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	f.ws.Settings.CommitTree = true
	f.win.buildRows()
	v := &f.win.cm
	group := v.rows[f.win.rowIndex(rowKey(domain.AreaUnstaged, ""))]
	file := v.rows[f.win.rowIndex(group.files[0])]
	for _, tc := range []struct {
		name string
		row  changeRow
		sel  map[string]bool
		want map[string]bool
	}{
		{"unselected-group", group, map[string]bool{}, map[string]bool{group.key: true}},
		{"partial-group", group, map[string]bool{file.key: true}, map[string]bool{group.key: true}},
		{"selected-group", group, map[string]bool{group.key: true}, map[string]bool{group.key: true}},
		{"unselected-file", file, map[string]bool{group.files[1]: true}, map[string]bool{file.key: true}},
		{"selected-file", file, map[string]bool{file.key: true, group.files[1]: true}, map[string]bool{file.key: true, group.files[1]: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.win.closePopups()
			v.sel = maps.Clone(tc.sel)
			f.win.Invalidate()
			f.settle()
			if err := f.s.RightClickLabel(f.win.rowLabel(tc.row)); err != nil {
				t.Fatal(err)
			}
			f.settle()
			if !maps.Equal(v.sel, tc.want) {
				t.Fatalf("selection = %v, want %v", v.sel, tc.want)
			}
		})
	}
}

func TestChangeTreeCollapsedSelectionTargets(t *testing.T) {
	f := newUIFixture(t)
	f.write("internal/store/store_test.go", "package store\n// changed test\n")
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	f.ws.Settings.CommitTree = true
	v := &f.win.cm
	group := rowKey(domain.AreaUnstaged, "")
	dir := rowKey(domain.AreaUnstaged, "internal/store/")
	first := rowKey(domain.AreaUnstaged, "internal/store/store.go")
	second := rowKey(domain.AreaUnstaged, "internal/store/store_test.go")
	for _, tc := range []struct {
		name string
		hide string
		sel  map[string]bool
		want map[string]bool
	}{
		{"group-partial-files", group, map[string]bool{first: true}, map[string]bool{first: true}},
		{"directory-partial-files", dir, map[string]bool{first: true}, map[string]bool{first: true}},
		{"group-selected-directory", group, map[string]bool{dir: true}, map[string]bool{first: true, second: true}},
		{"group-overlapping-selection", group, map[string]bool{dir: true, first: true}, map[string]bool{first: true, second: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clear(v.collapsed)
			v.collapsed[tc.hide] = true
			v.sel = maps.Clone(tc.sel)
			f.win.buildRows()
			assertChangeTargets(t, f, tc.want)
		})
	}
}

// Checking files has never staged them, so a checked file must not make
// Commit look ready on its own. The panel states the real scope and offers the
// missing step next to the button; Commit stays disabled until the file is
// actually in the index (issue #82).
func TestCommitPanelStageSelectedFromCheckedFiles(t *testing.T) {
	f := newUIFixture(t)
	// Start with an empty index so the test can prove that a commit contains
	// only what Stage Selected staged, not every checked file.
	f.git("reset", "-q", "README.md")
	// Commit and Push is disabled without a remote; the panel must say why.
	f.git("remote", "remove", "origin")
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	if got := stagedNames(f); len(got) != 0 {
		t.Fatalf("test needs an empty index, staged: %v", got)
	}
	f.win.buildRows()
	idx := f.win.rowIndex(rowKey(domain.AreaUnstaged, "main.go"))
	if idx < 0 {
		t.Fatalf("main.go is not an unstaged row: %+v", f.win.cm.rows)
	}
	hit := changeCheckHit(t, f, scChangeChecks.At(idx))
	if err := f.s.Click(hit.Bounds.X+8, hit.Bounds.Y+hit.Bounds.Height/2); err != nil {
		t.Fatal(err)
	}
	f.settle()
	if !f.win.cm.sel[rowKey(domain.AreaUnstaged, "main.go")] {
		t.Fatalf("checkbox did not check main.go: %v", f.win.cm.sel)
	}

	f.click("Commit message")
	if err := f.s.Type("Commit the checked file"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)

	if got := f.ws.CommittableCount(); got != 0 {
		t.Fatalf("CommittableCount = %d with an empty index", got)
	}
	if !commitControl(t, f, idCmCommit).Disabled {
		t.Fatal("Commit is enabled while the checked file is not staged")
	}
	for _, want := range []string{
		"Commit staged files only",
		"1 selected file is not staged",
		"Stage Selected",
	} {
		if !f.s.HasText(want) {
			t.Fatalf("missing %q\ntexts: %s", want, strings.Join(f.s.Texts(), " | "))
		}
	}
	if got := f.win.tips[idCmCommitPush]; got != "Add a remote to push" {
		t.Fatalf("Commit and Push tooltip = %q, want the missing-remote hint", got)
	}
	if !commitControl(t, f, idCmCommitPush).Disabled {
		t.Fatal("Commit and Push is enabled without a remote")
	}
	f.snap("82-before-stage-selected")

	// The hint and the action must stay visible at the Commit panel's minimum
	// width, where the change-list bar already needs a full row of its own.
	f.ws.Settings.CommitPanelWidth = 240
	f.win.Invalidate()
	f.s.Settle(80*time.Millisecond, time.Second)
	for _, want := range []string{"Commit staged files only", "Stage Selected"} {
		if !f.s.HasText(want) {
			t.Fatalf("missing %q at a 240px panel\ntexts: %s", want, strings.Join(f.s.Texts(), " | "))
		}
	}
	f.snap("82-narrow-stage-selected")
	f.ws.Settings.CommitPanelWidth = 340
	f.win.Invalidate()
	f.s.Settle(80*time.Millisecond, time.Second)

	// Stage Selected is reachable with Tab and activates with Enter.
	stage, ok := paintedHit(f, "Stage Selected")
	if !ok {
		t.Fatalf("Stage Selected has no hit\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	focused := false
	for i := 0; i < 40; i++ {
		if err := f.s.Keys("tab"); err != nil {
			t.Fatal(err)
		}
		if f.s.Snapshot().Focused == stage.ID {
			focused = true
			break
		}
	}
	if !focused {
		t.Fatalf("Stage Selected never took keyboard focus; focus = %v", f.s.Snapshot().Focused)
	}
	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	f.settle()

	staged := stagedNames(f)
	if len(staged) != 1 || !staged["main.go"] {
		t.Fatalf("Stage Selected staged %v, want only main.go", staged)
	}
	if got := f.ws.CommittableCount(); got != 1 {
		t.Fatalf("CommittableCount = %d after staging main.go", got)
	}
	if commitControl(t, f, idCmCommit).Disabled {
		t.Fatal("Commit stayed disabled after Stage Selected")
	}
	if f.s.HasText("Stage Selected") || f.s.HasText("is not staged") {
		t.Fatalf("staging hint stayed visible after staging\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("82-after-stage-selected")

	f.click("Commit")
	f.settle()
	if subj := strings.TrimSpace(f.git("log", "-1", "--format=%s")); subj != "Commit the checked file" {
		t.Fatalf("last commit = %q", subj)
	}
	committed := committedPaths(f)
	if len(committed) != 1 || !committed["main.go"] {
		t.Fatalf("commit contains %v, want only main.go", committed)
	}
}

// Existing staged content is the real commit scope even while an unstaged file
// is checked: pressing Commit must not silently stage the checked file, and
// the warning must say that it is not part of the commit yet.
func TestCommitScopeWithExistingStagedContent(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	// The fixture stages README.md and leaves main.go unstaged.
	if got := stagedNames(f); !got["README.md"] || got["main.go"] {
		t.Fatalf("fixture index = %v", got)
	}
	f.win.buildRows()
	idx := f.win.rowIndex(rowKey(domain.AreaUnstaged, "main.go"))
	if idx < 0 {
		t.Fatalf("main.go is not an unstaged row: %+v", f.win.cm.rows)
	}
	f.win.toggleRowCheck(idx)
	if err := f.s.ClickLabel("Commit message"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Type("Commit the staged file"); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)

	if got := f.ws.CommittableCount(); got != 1 {
		t.Fatalf("CommittableCount = %d, want the staged README", got)
	}
	if commitControl(t, f, idCmCommit).Disabled {
		t.Fatal("Commit is disabled although README.md is staged and the message is set")
	}
	for _, want := range []string{
		"Commit staged files only",
		"1 selected file is not staged",
		"Stage Selected",
	} {
		if !f.s.HasText(want) {
			t.Fatalf("missing %q\ntexts: %s", want, strings.Join(f.s.Texts(), " | "))
		}
	}
	f.snap("82-staged-scope-with-checked-unstaged")

	// Commit without Stage Selected: the checked file is not added silently.
	f.click("Commit")
	f.settle()
	committed := committedPaths(f)
	if len(committed) != 1 || !committed["README.md"] {
		t.Fatalf("commit contains %v, want only the staged README.md", committed)
	}
	if got := stagedNames(f); len(got) != 0 {
		t.Fatalf("index not clean after commit: %v", got)
	}
}

// commitControl returns the painted hit of a commit-panel control, disabled
// ones included.
func commitControl(t *testing.T, f *uiFixture, id ui.ID) ui.Hit {
	t.Helper()
	for _, hit := range f.s.Snapshot().Hits {
		if hit.ID == id {
			return hit
		}
	}
	t.Fatalf("missing control %v\ntexts: %s", id, strings.Join(f.s.Texts(), " | "))
	return ui.Hit{}
}

// committedPaths lists the files the newest commit touched.
func committedPaths(f *uiFixture) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(f.git("show", "--name-only", "--format=", "HEAD")), "\n") {
		if line != "" {
			out[line] = true
		}
	}
	return out
}
