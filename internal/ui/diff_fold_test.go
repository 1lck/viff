package viffui

import (
	"testing"

	"viff/internal/domain"
)

func TestDiffFoldProjection(t *testing.T) {
	blocks := []domain.Block{
		{OldStart: 20, OldEnd: 21, NewStart: 20, NewEnd: 25},
		{OldStart: 50, OldEnd: 52, NewStart: 54, NewEnd: 54},
	}
	folds := diffFolds(blocks, 80, 82)
	if len(folds) != 3 {
		t.Fatalf("folds = %+v, want leading, between and trailing", folds)
	}
	for _, old := range []bool{true, false} {
		n := 82
		if old {
			n = 80
		}
		p := projectFolds(n, folds, old, nil)
		if p.total >= n {
			t.Fatalf("old=%v: no row was folded: total %d of %d", old, p.total, n)
		}
		seen := make(map[int]bool, p.total)
		for row := 0; row < p.total; row++ {
			fold, hidden := p.hiddenAt(row)
			if hidden == 0 {
				source := p.source(row)
				if seen[source] {
					t.Fatalf("old=%v: source line %d appears twice", old, source)
				}
				seen[source] = true
				if got := p.row(source); got != row {
					t.Fatalf("old=%v: source %d mapped back to row %d, want %d", old, source, got, row)
				}
				continue
			}
			if fold < 0 || fold >= len(folds) {
				t.Fatalf("row %d names fold %d of %d", row, fold, len(folds))
			}
			if hidden < 2 {
				t.Fatalf("row %d folds a single line, which saves no space", row)
			}
			start, end := p.source(row), p.source(row)+hidden
			for line := start; line < end; line++ {
				if got := p.row(line); got != row {
					t.Fatalf("hidden line %d mapped to row %d, want %d", line, got, row)
				}
			}
		}
		if p.source(p.total) != n {
			t.Fatalf("old=%v: EOF boundary maps to %d, want %d", old, p.source(p.total), n)
		}
		expanded := make(map[int]bool, len(folds))
		for i := range folds {
			expanded[i] = true
		}
		full := projectFolds(n, folds, old, expanded)
		if full.total != n || full.row(n) != n || full.source(n) != n {
			t.Fatalf("old=%v: expanding did not restore every line: total %d", old, full.total)
		}
	}
}

func TestDiffFoldContext(t *testing.T) {
	if got := diffFolds(nil, 100, 100); len(got) != 0 {
		t.Fatalf("identical file folded: %+v", got)
	}
	// A change near the start or end must keep its context and fold the rest.
	b := []domain.Block{{OldStart: 10, OldEnd: 11, NewStart: 10, NewEnd: 11}}
	folds := diffFolds(b, 40, 40)
	if len(folds) != 2 {
		t.Fatalf("folds = %+v, want one leading and one trailing", folds)
	}
	if folds[0].count != 7 || folds[1].count != 26 {
		t.Fatalf("context around the change is wrong: %+v", folds)
	}
	// Gaps no longer than two context windows stay whole: folding them would
	// hide only one or two lines and make the change harder to read.
	near := []domain.Block{
		{OldStart: 0, OldEnd: 1, NewStart: 0, NewEnd: 1},
		{OldStart: 8, OldEnd: 9, NewStart: 8, NewEnd: 9},
	}
	if got := diffFolds(near, 20, 20); len(got) != 1 || got[0].count != 8 {
		t.Fatalf("short gap folding = %+v, want only the trailing fold", got)
	}
}

func TestDiffFoldSkipsInvalidSpans(t *testing.T) {
	// Defensive: a stale fold that no longer fits the content must not corrupt
	// the projection (a reload races with a paint in principle).
	folds := []foldRange{{oldStart: 90, newStart: 90, count: 20}, {oldStart: 5, newStart: 5, count: 0}}
	p := projectFolds(100, folds, false, nil)
	if p.total != 100 || len(p.spans) != 0 {
		t.Fatalf("invalid folds were applied: total %d spans %+v", p.total, p.spans)
	}
}
