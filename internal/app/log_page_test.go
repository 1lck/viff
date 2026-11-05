package app

import (
	"fmt"
	"strings"
	"testing"

	"viff/internal/domain"
)

// fastImportStream builds a linear history of n commits on main, refs extra
// branch tips, and an optional merge. mergeAt is the commit number that merges
// a side commit (created from mergeFrom) into the line, so the graph has a
// lane that crosses a page boundary.
func fastImportStream(n, refs, mergeAt, mergeFrom int) string {
	var b strings.Builder
	const base = 1_600_000_000
	sideMark := n + 1
	for i := 1; i <= n; i++ {
		if i == mergeAt {
			sideMsg := fmt.Sprintf("Side %d\n", i)
			sideContent := fmt.Sprintf("side %d\n", i)
			fmt.Fprintf(&b, "commit refs/heads/perf-side-src\nmark :%d\ncommitter Viff Test <test@example.com> %d +0000\ndata %d\n%s",
				sideMark, base+i*60-5, len(sideMsg), sideMsg)
			fmt.Fprintf(&b, "from :%d\n", mergeFrom)
			fmt.Fprintf(&b, "M 100644 inline side.txt\ndata %d\n%s\n", len(sideContent), sideContent)
		}
		msg := fmt.Sprintf("Commit %d\n", i)
		content := fmt.Sprintf("value %d\n", i)
		fmt.Fprintf(&b, "commit refs/heads/main\nmark :%d\ncommitter Viff Test <test@example.com> %d +0000\ndata %d\n%s",
			i, base+i*60, len(msg), msg)
		if i > 1 {
			fmt.Fprintf(&b, "from :%d\n", i-1)
		}
		if i == mergeAt {
			fmt.Fprintf(&b, "merge :%d\n", sideMark)
		}
		fmt.Fprintf(&b, "M 100644 inline file.txt\ndata %d\n%s\n", len(content), content)
	}
	if mergeAt > 0 {
		b.WriteString("reset refs/heads/perf-side-src\n")
	}
	// Extra branch tips spread over the history; they do not add commits, so
	// the log content stays the same.
	for j := 1; j <= refs; j++ {
		fmt.Fprintf(&b, "reset refs/heads/perf-%04d\nfrom :%d\n", j, 1+(j*7919)%n)
	}
	b.WriteString("done\n")
	return b.String()
}

// assertGraphJoint checks that the lower lanes of row a are exactly the upper
// lanes of row b: an edge never ends between two adjacent rows, even when the
// rows come from different pages.
func assertGraphJoint(t *testing.T, a, b domain.GraphRow) {
	t.Helper()
	bottom := map[int]bool{}
	top := map[int]bool{}
	for _, e := range a.Edges {
		if e.Lower {
			bottom[e.To] = true
		}
	}
	for _, e := range b.Edges {
		if !e.Lower {
			top[e.From] = true
		}
	}
	for lane := range bottom {
		if !top[lane] {
			t.Fatalf("lane %d ends between rows, bottom=%v top=%v", lane, bottom, top)
		}
	}
	for lane := range top {
		if !bottom[lane] {
			t.Fatalf("lane %d starts from nowhere, bottom=%v top=%v", lane, bottom, top)
		}
	}
}

// TestLogPaginationBoundaryAndGraph reads more commits than one page and
// checks the page boundary: the second page continues the graph exactly where
// the first ended, including the merge lane, and the last page marks the log
// done. The fixture has 451 commits: 450 on main, one side commit, and a merge
// as the last row of the first page whose second-parent lane starts on the
// second page.
func TestLogPaginationBoundaryAndGraph(t *testing.T) {
	const commits = LogPage + 50
	f := newFixture(t)
	f.fastImport(fastImportStream(commits, 0, 51, 30))
	f.open()
	rs := f.w.Repo
	if got := len(rs.Log.Commits); got != LogPage {
		t.Fatalf("first page = %d commits, want %d", got, LogPage)
	}
	if rs.Log.Done {
		t.Fatal("log is done after the first page")
	}
	if len(rs.Log.Rows) != len(rs.Log.Commits) {
		t.Fatalf("rows = %d, commits = %d", len(rs.Log.Rows), len(rs.Log.Commits))
	}
	if _, ok := rs.Log.IndexOf(rs.Log.Commits[0].Hash); !ok {
		t.Fatal("head commit missing from the log index")
	}
	f.w.LoadMoreLog()
	f.settle(func() bool { return f.idle() && f.w.Repo.Log.Done && len(f.w.Repo.Log.Commits) == commits+1 })
	rs = f.w.Repo
	if len(rs.Log.Rows) != len(rs.Log.Commits) {
		t.Fatalf("rows = %d, commits = %d", len(rs.Log.Rows), len(rs.Log.Commits))
	}
	seen := map[string]bool{}
	for _, c := range rs.Log.Commits {
		if seen[c.Hash] {
			t.Fatalf("duplicate commit %s after paging", c.Hash)
		}
		seen[c.Hash] = true
	}
	// The last row of the first page is the merge commit; its lower lane
	// must continue into the first row of the second page.
	if !rs.Log.Rows[LogPage-1].IsMerge {
		t.Fatalf("row %d is not the merge: %+v", LogPage-1, rs.Log.Rows[LogPage-1])
	}
	assertGraphJoint(t, rs.Log.Rows[LogPage-1], rs.Log.Rows[LogPage])
	// Loading past the end is a no-op, not another query.
	f.w.LoadMoreLog()
	f.w.Drain()
	if got := len(f.w.Repo.Log.Commits); got != commits+1 {
		t.Fatalf("LoadMoreLog after done changed the log to %d commits", got)
	}
}

// TestReloadLogWhileLoading reloads twice before the first page arrives, which
// is what a metadata refresh during the first load does on a large repository.
// The canceled load must not leave the log stuck in its loading state.
func TestReloadLogWhileLoading(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "one")
	f.open()

	f.w.ReloadLog()
	if !f.w.Repo.Log.Loading {
		t.Fatal("ReloadLog did not start loading")
	}
	f.w.ReloadLog()
	f.settle(func() bool { return f.idle() && f.w.Repo.Log.Done && len(f.w.Repo.Log.Commits) == 1 })
	if len(f.w.Repo.Log.Rows) != 1 {
		t.Fatalf("rows = %d", len(f.w.Repo.Log.Rows))
	}
}

// TestSelectCommitClearStopsLoading covers clearing the selection while its
// detail is in flight: the panel must not stay on "loading" and the late
// result must be dropped.
func TestSelectCommitClearStopsLoading(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "one")
	f.open()
	head := f.w.Repo.Log.Commits[0].Hash

	f.w.SelectCommit(head)
	f.w.SelectCommit("")
	f.settle(func() bool { return !f.w.Repo.DetailLoading && !f.w.Pending() })
	f.w.Drain()
	if f.w.Repo.Detail != nil || len(f.w.Repo.DetailBranches) != 0 {
		t.Fatalf("cleared selection kept detail: %+v branches=%v", f.w.Repo.Detail, f.w.Repo.DetailBranches)
	}
}
