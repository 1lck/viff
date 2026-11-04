//go:build vivahead

package viffui

import (
	"errors"
	"strings"
	"testing"
	"time"

	vivaapp "github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// multiCommitFixture builds a controlled history on top of the shared fixture
// repository: two commits touch one file, a file is added and later deleted, a
// file is renamed, and empty commits separate the interesting ones.
func multiCommitFixture(t *testing.T) *uiFixture {
	t.Helper()
	f := newUIFixture(t)
	// The shared fixture ends with local changes; start from a clean tree.
	f.git("reset", "--hard", "-q")
	f.git("clean", "-fdq")

	f.write("common.txt", "common\n")
	f.write("moved.txt", strings.Repeat("moved content\n", 4))
	f.write("later.txt", "later\n")
	f.commit("Audit base files", "2026-10-13T09:00:00")

	f.write("common.txt", "common\nsecond\n")
	f.commit("Audit modify common", "2026-10-13T10:00:00")

	f.write("temp.txt", "temporary\n")
	f.commit("Audit add temp", "2026-10-13T11:00:00")
	f.git("rm", "-q", "temp.txt")
	f.commit("Audit delete temp", "2026-10-13T12:00:00")

	f.git("mv", "moved.txt", "renamed.txt")
	f.commit("Audit rename moved", "2026-10-13T13:00:00")

	f.write("later.txt", "later\nsecond\n")
	f.commit("Audit modify later", "2026-10-13T14:00:00")
	return f
}

// multiCommitSelect selects commits by subject and waits for the aggregate that
// belongs to this selection, not merely for any aggregate.
func (f *uiFixture) multiCommitSelect(t *testing.T, subjects ...string) *vapp.MultiCommitSummary {
	t.Helper()
	want := map[string]bool{}
	for _, s := range subjects {
		want[s] = true
	}
	sel := map[string]bool{}
	var commits []domain.Commit
	for _, c := range f.ws.Repo.Log.Commits {
		if want[c.Subject] {
			sel[c.Hash] = true
			commits = append(commits, c)
		}
	}
	if len(commits) != len(subjects) {
		t.Fatalf("selected %d commits for %v, want %d", len(commits), subjects, len(subjects))
	}
	f.win.lg.sel = sel
	f.win.Invalidate()
	key := multiCommitSelectionKey(f.ws.Repo, commits)
	if err := f.s.WaitFor(func(vivaapp.Snapshot) bool {
		v := &f.win.lg.multi
		return v.key == key && !v.loading && v.summary != nil
	}, 15*time.Second); err != nil {
		t.Fatalf("aggregate for %v did not load: %v", subjects, err)
	}
	f.s.Settle(50*time.Millisecond, time.Second)
	return f.win.lg.multi.summary
}

func multiCommitPick(t *testing.T, sum *vapp.MultiCommitSummary, path string) vapp.MultiCommitFile {
	t.Helper()
	for _, f := range sum.Files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("aggregated files %v miss %q", multiCommitSummaryPaths(sum), path)
	return vapp.MultiCommitFile{}
}

func multiCommitSummaryPaths(sum *vapp.MultiCommitSummary) []string {
	out := make([]string, len(sum.Files))
	for i, f := range sum.Files {
		out[i] = f.Path
	}
	return out
}

func multiCommitBounds(t *testing.T, f *uiFixture, id ui.ID) ui.Rect {
	t.Helper()
	for _, h := range f.s.Snapshot().Hits {
		if h.ID == id {
			return h.Bounds
		}
	}
	t.Fatalf("missing rendered multi-commit control %v; texts: %v", id, f.s.Texts())
	return ui.Rect{}
}

// TestMultiCommitUnionDetails checks the union rule on screen: each selected
// commit contributes its own first-parent changes, a shared file is one row
// with several sources, and the rows are really painted (hit regions exist).
func TestMultiCommitUnionDetails(t *testing.T) {
	f := multiCommitFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	sum := f.multiCommitSelect(t, "Audit modify later", "Audit base files")
	if sum.Err != nil {
		t.Fatalf("Err = %v", sum.Err)
	}
	if len(sum.Commits) != 2 || sum.Commits[0].Subject != "Audit modify later" || sum.Commits[1].Subject != "Audit base files" {
		t.Fatalf("commits = %+v, want the log's selection order", sum.Commits)
	}
	// The base commit added three files; the newer one changed only later.txt.
	if got := multiCommitSummaryPaths(sum); len(got) != 3 || got[0] != "common.txt" || got[1] != "later.txt" || got[2] != "moved.txt" {
		t.Fatalf("files = %v, want the union sorted by path", got)
	}
	common := multiCommitPick(t, sum, "common.txt")
	if len(common.Sources) != 1 || common.Sources[0].Commit.Subject != "Audit base files" || common.Kind != domain.ChangeAdded {
		t.Fatalf("common.txt = %+v, want one added source", common)
	}
	later := multiCommitPick(t, sum, "later.txt")
	if len(later.Sources) != 2 || later.Sources[0].Commit.Subject != "Audit modify later" || later.Kind != domain.ChangeModified {
		t.Fatalf("later.txt = %+v, want two deduplicated sources", later)
	}
	for i := range sum.Files {
		if b := multiCommitBounds(t, f, scMultiCommitRows.At(i)); b.Width <= 0 || b.Height <= 0 {
			t.Fatalf("file row %d has no painted hit region: %+v", i, b)
		}
	}
	f.snap("multi-commit-union")
}

// TestMultiCommitDeleteAndRenameExplained checks that a delete and a rename
// stay attributed to the commits that made them.
func TestMultiCommitDeleteAndRenameExplained(t *testing.T) {
	f := multiCommitFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	// Add then delete: one row, both sources, the summary reads as a deletion.
	sum := f.multiCommitSelect(t, "Audit add temp", "Audit delete temp")
	if len(sum.Files) != 1 || sum.Files[0].Path != "temp.txt" {
		t.Fatalf("files = %v, want temp.txt deduplicated", multiCommitSummaryPaths(sum))
	}
	temp := sum.Files[0]
	if temp.Kind != domain.ChangeDeleted {
		t.Fatalf("temp.txt kind = %v, want deleted", temp.Kind)
	}
	if len(temp.Sources) != 2 || temp.Sources[0].Change.Kind != domain.ChangeDeleted || temp.Sources[1].Change.Kind != domain.ChangeAdded {
		t.Fatalf("temp.txt sources = %+v, want newest delete then older add", temp.Sources)
	}

	// Rename next to an unrelated modification: the row explains its old path.
	sum = f.multiCommitSelect(t, "Audit modify later", "Audit rename moved")
	renamed := multiCommitPick(t, sum, "renamed.txt")
	if renamed.Kind != domain.ChangeRenamed || renamed.RenamedFrom != "moved.txt" {
		t.Fatalf("renamed.txt = %+v, want a rename from moved.txt", renamed)
	}
	if len(renamed.Sources) != 1 || renamed.Sources[0].Change.OldPath != "moved.txt" {
		t.Fatalf("renamed.txt sources = %+v", renamed.Sources)
	}
	f.snap("multi-commit-delete-rename")
}

// TestMultiCommitSelectionSwitch checks that changing the selection replaces
// the aggregate, and that returning to one commit ends the multi-commit view
// (and its query) instead of leaving stale rows.
func TestMultiCommitSelectionSwitch(t *testing.T) {
	f := multiCommitFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	first := f.multiCommitSelect(t, "Audit add temp", "Audit delete temp")
	if len(first.Files) != 1 {
		t.Fatalf("first aggregate = %v", multiCommitSummaryPaths(first))
	}
	second := f.multiCommitSelect(t, "Audit modify later", "Audit base files")
	if len(second.Files) != 3 {
		t.Fatalf("second aggregate = %v", multiCommitSummaryPaths(second))
	}
	if _, ok := multiCommitFileByPath(second, "temp.txt"); ok {
		t.Fatal("new selection kept a file from the old one")
	}
	if second.Commits[0].Subject != "Audit modify later" {
		t.Fatalf("second commits = %+v", second.Commits)
	}

	// A plain click selects one commit: the aggregate and its request go away.
	idx := -1
	var single domain.Commit
	for i, c := range f.ws.Repo.Log.Commits {
		if c.Subject == "Audit rename moved" {
			idx, single = i, c
			break
		}
	}
	if idx < 0 {
		t.Fatal("commit is not in the log")
	}
	f.win.mods = 0
	f.win.clickCommit(idx)
	f.settle()
	if err := f.s.WaitFor(func(vivaapp.Snapshot) bool {
		v := &f.win.lg.multi
		return v.key == "" && v.summary == nil && !v.loading
	}, 5*time.Second); err != nil {
		t.Fatal("multi-commit view was not stopped for a single selection:", err)
	}
	if d := f.ws.Repo.Detail; d == nil || d.Hash != single.Hash {
		t.Fatalf("single detail = %+v, want %s", d, single.Hash)
	}

	// Selecting several commits again brings the aggregate back.
	back := f.multiCommitSelect(t, "Audit base files", "Audit modify later")
	if len(back.Files) != 3 {
		t.Fatalf("aggregate after returning = %v", multiCommitSummaryPaths(back))
	}
	f.snap("multi-commit-selection-switch")
}

// TestMultiCommitPartialResultsVisible checks that a failed commit is stated
// on screen, not left to a trailing line that a long selection can crowd out:
// the rows must never pass themselves off as the complete union.
func TestMultiCommitPartialResultsVisible(t *testing.T) {
	f := multiCommitFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	sum := f.multiCommitSelect(t,
		"Audit modify later", "Audit rename moved", "Audit delete temp",
		"Audit add temp", "Audit modify common", "Audit base files")
	if len(sum.Commits) != 6 {
		t.Fatalf("commits = %d", len(sum.Commits))
	}
	partial := *sum
	partial.Err = errors.New("commit abc123: read failed")
	f.win.lg.multi.summary = &partial
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, time.Second)

	if !f.s.HasText("Partial results") {
		t.Fatalf("partial result has no visible status; texts: %v", f.s.Texts())
	}
	if !f.s.HasText("read failed") {
		t.Fatalf("partial status hides the failure reason; texts: %v", f.s.Texts())
	}
	for i := range partial.Files {
		if b := multiCommitBounds(t, f, scMultiCommitRows.At(i)); b.Width <= 0 {
			t.Fatalf("file row %d is not painted below the status", i)
		}
	}
	f.snap("multi-commit-partial-results")
	// The error remains visible even when its full reason scrolls away.
	partial.Files = append(partial.Files, partial.Files...)
	partial.Files = append(partial.Files, partial.Files...)
	f.win.lg.multi.scroll.Y = 100000
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, time.Second)
	if f.win.lg.multi.scroll.Y == 0 || !f.s.HasText("Partial results") {
		t.Fatal("partial status disappeared after scrolling")
	}
	f.snap("multi-commit-partial-scrolled")
}

func multiCommitFileByPath(sum *vapp.MultiCommitSummary, path string) (vapp.MultiCommitFile, bool) {
	for _, f := range sum.Files {
		if f.Path == path {
			return f, true
		}
	}
	return vapp.MultiCommitFile{}, false
}

// TestMultiCommitFileOpensSourceDiff checks that opening a file uses one source
// commit's real first-parent diff: several sources ask which commit, one source
// opens directly, and no merged net diff is fabricated.
func TestMultiCommitFileOpensSourceDiff(t *testing.T) {
	f := multiCommitFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	sum := f.multiCommitSelect(t, "Audit add temp", "Audit delete temp")
	file := multiCommitPick(t, sum, "temp.txt")
	if len(file.Sources) != 2 {
		t.Fatalf("temp.txt sources = %d", len(file.Sources))
	}
	f.win.popups = nil
	// Click the real painted row: the hit region must route to the picker.
	row0 := multiCommitBounds(t, f, scMultiCommitRows.At(0))
	if err := f.s.Click(row0.X+row0.Width/2, row0.Y+row0.Height/2); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(f.win.popups) != 1 {
		t.Fatalf("popups = %d, want a source-commit picker", len(f.win.popups))
	}
	p := f.win.popups[0]
	if len(p.items) != 2 {
		t.Fatalf("source items = %d, want both commits", len(p.items))
	}
	if !strings.Contains(p.items[0].hint, "deleted") || !strings.Contains(p.items[1].hint, "added") {
		t.Fatalf("source hints = %q, %q", p.items[0].hint, p.items[1].hint)
	}
	if !strings.Contains(p.items[0].title, domain.ShortHash(file.Sources[0].Commit.Hash)) {
		t.Fatalf("source title %q misses the commit hash", p.items[0].title)
	}
	p.items[0].run()
	f.win.popups = nil
	tab := f.win.ed.cur()
	if tab == nil {
		t.Fatal("no diff tab opened for the source commit")
	}
	want := file.Sources[0]
	if tab.src.Commit != want.Commit.Hash || tab.src.Parent != want.Parent || tab.src.Path != "temp.txt" || tab.src.Kind != domain.ChangeDeleted {
		t.Fatalf("diff source = %+v, want the deleted source commit %s with parent %s", tab.src, want.Commit.Hash, want.Parent)
	}
	if want.Parent != file.Sources[1].Commit.Hash {
		t.Fatalf("delete commit's first parent = %s, want the add commit %s", want.Parent, file.Sources[1].Commit.Hash)
	}

	// A row with one source opens its real diff directly instead of a picker.
	sum = f.multiCommitSelect(t, "Audit base files", "Audit modify later")
	common := multiCommitPick(t, sum, "common.txt")
	if len(common.Sources) != 1 {
		t.Fatalf("common.txt sources = %d", len(common.Sources))
	}
	row := -1
	for i, f := range sum.Files {
		if f.Path == "common.txt" {
			row = i
		}
	}
	f.win.popups = nil
	rowRect := multiCommitBounds(t, f, scMultiCommitRows.At(row))
	if err := f.s.Click(rowRect.X+rowRect.Width/2, rowRect.Y+rowRect.Height/2); err != nil {
		t.Fatal(err)
	}
	f.settleTab()
	if len(f.win.popups) != 0 {
		t.Fatal("single-source row opened a picker")
	}
	tab = f.win.ed.cur()
	if tab == nil || tab.src.Commit != common.Sources[0].Commit.Hash || tab.src.Parent != common.Sources[0].Parent {
		t.Fatalf("direct diff source = %+v, want %s", tab, common.Sources[0].Commit.Hash)
	}
	if tab.src.Kind != domain.ChangeAdded || tab.src.Path != "common.txt" {
		t.Fatalf("direct diff = %+v, want the added common.txt", tab.src)
	}
	f.snap("multi-commit-source-diff")
}
