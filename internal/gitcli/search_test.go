package gitcli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// A search returns the matching lines of tracked text files, keeps paths with
// spaces and non-ASCII characters verbatim, and skips binaries.
func TestSearchContentGit(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("src/a.go", "package a\n// needle here\nfunc f() {}\n")
	tr.write("dir with space/中文.txt", "first\nneedle 中文\n")
	tr.write("bin.dat", "needle\x00binary")
	tr.commitAll("init")

	res, err := tr.r.SearchContent(context.Background(), tr.repo, "needle", SearchOptions{Tool: SearchGit})
	if err != nil {
		t.Fatal(err)
	}
	if res.Tool != SearchGit {
		t.Fatalf("tool = %q, want %q", res.Tool, SearchGit)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("hits = %+v, want the two text files", res.Hits)
	}
	byPath := map[string]SearchHit{}
	for _, h := range res.Hits {
		byPath[h.Path] = h
	}
	if h, ok := byPath["src/a.go"]; !ok || h.Line != 2 || h.Text != "// needle here" {
		t.Fatalf("src/a.go hit = %+v", h)
	}
	if h, ok := byPath["dir with space/中文.txt"]; !ok || h.Line != 2 || h.Text != "needle 中文" {
		t.Fatalf("unicode path hit = %+v", h)
	}
}

// Git exits 1 when nothing matched; that is an empty answer, not an error, and
// a missing ripgrep is distinguishable for the settings hint.
func TestSearchContentNoMatchAndMissingRipgrep(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "alpha\n")
	tr.commitAll("init")

	res, err := tr.r.SearchContent(context.Background(), tr.repo, "zzzz-not-present", SearchOptions{Tool: SearchGit})
	if err != nil {
		t.Fatalf("no match returned an error: %v", err)
	}
	if len(res.Hits) != 0 || res.Truncated {
		t.Fatalf("no match = %+v", res)
	}

	missing := filepath.Join(t.TempDir(), "no-such-rg")
	_, err = tr.r.SearchContent(context.Background(), tr.repo, "alpha", SearchOptions{Tool: SearchRipgrep, RipgrepPath: missing})
	if !errors.Is(err, ErrRipgrepMissing) {
		t.Fatalf("err = %v, want ErrRipgrepMissing", err)
	}
}

// Untracked files join only when asked, and the result cap stops the scan.
func TestSearchContentUntrackedAndLimit(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("tracked.txt", "needle one\n")
	tr.commitAll("init")
	tr.write("untracked.txt", "needle two\n")

	res, err := tr.r.SearchContent(context.Background(), tr.repo, "needle", SearchOptions{Tool: SearchGit})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 || res.Hits[0].Path != "tracked.txt" {
		t.Fatalf("default search = %+v, want only the tracked file", res.Hits)
	}

	res, err = tr.r.SearchContent(context.Background(), tr.repo, "needle", SearchOptions{Tool: SearchGit, IncludeUntracked: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("untracked search = %+v, want both files", res.Hits)
	}

	tr.write("many.txt", strings.Repeat("needle\n", 5))
	tr.commitAll("more")
	res, err = tr.r.SearchContent(context.Background(), tr.repo, "needle", SearchOptions{Tool: SearchGit, Max: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 2 || !res.Truncated {
		t.Fatalf("limited search = %d hits, truncated=%v", len(res.Hits), res.Truncated)
	}
}

// ripgrep is optional; when it is installed it must return the same hits as
// Git grep for the same working tree.
func TestSearchContentRipgrepMatchesGit(t *testing.T) {
	if _, err := findRipgrep(""); err != nil {
		t.Skip("ripgrep is not installed")
	}
	tr := newTestRepo(t)
	tr.write("a.txt", "alpha\nalpha beta\n")
	tr.write("b.txt", "nothing\n")
	tr.commitAll("init")

	want, err := tr.r.SearchContent(context.Background(), tr.repo, "alpha", SearchOptions{Tool: SearchGit})
	if err != nil {
		t.Fatal(err)
	}
	got, err := tr.r.SearchContent(context.Background(), tr.repo, "alpha", SearchOptions{Tool: SearchRipgrep})
	if err != nil {
		t.Fatal(err)
	}
	if got.Tool != SearchRipgrep {
		t.Fatalf("tool = %q", got.Tool)
	}
	if len(got.Hits) != len(want.Hits) {
		t.Fatalf("ripgrep = %+v, git = %+v", got.Hits, want.Hits)
	}
	for i := range want.Hits {
		if got.Hits[i] != want.Hits[i] {
			t.Fatalf("hit %d = %+v, want %+v", i, got.Hits[i], want.Hits[i])
		}
	}
}
