package app

import "testing"

// A content search delivers its hits on the UI thread through Drain.
func TestSearchContent(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "alpha\nneedle here\n")
	f.write("b.txt", "nothing\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "init")
	f.open()

	var got SearchResult
	var gotErr error
	calls := 0
	f.w.SearchContent("needle", SearchContentOptions{Tool: "git"}, func(res SearchResult, err error) {
		calls++
		got, gotErr = res, err
	})
	f.settle(func() bool { return calls == 1 })
	if gotErr != nil {
		t.Fatal(gotErr)
	}
	if len(got.Hits) != 1 || got.Hits[0].Path != "a.txt" || got.Hits[0].Line != 2 {
		t.Fatalf("hits = %+v", got.Hits)
	}
}

// Starting another query makes the previous one stale: only the newest answer
// reaches the callback, even when both were started before the next Drain.
func TestSearchContentDropsStaleAnswer(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "alpha\nneedle\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "init")
	f.open()

	calls := 0
	last := ""
	f.w.SearchContent("alpha", SearchContentOptions{Tool: "git"}, func(SearchResult, error) {
		calls++
		last = "alpha"
	})
	f.w.SearchContent("needle", SearchContentOptions{Tool: "git"}, func(SearchResult, error) {
		calls++
		last = "needle"
	})
	f.settle(func() bool { return calls == 1 && !f.w.Pending() })
	if last != "needle" {
		t.Fatalf("last answer = %q, want the newest query", last)
	}
}
