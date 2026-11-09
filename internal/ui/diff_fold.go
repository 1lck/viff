package viffui

import (
	"sort"

	"viff/internal/domain"
)

// foldContext is how many unchanged lines stay visible on each side of a
// change. JetBrains and other diff viewers use the same small window: enough
// to read the change in context, few enough that a long file stays short.
const foldContext = 3

// foldRange names an unchanged half-open range in source coordinates. Both
// panes collapse the same context, even when earlier edits changed line counts.
// The unified projection reuses oldStart/newStart as row indexes, because its
// context rows interleave the two sides one to one.
type foldRange struct {
	oldStart, newStart, count int
}

// diffFolds leaves foldContext lines beside every change. Short gaps stay
// intact; replacing a single line with a fold would save no space.
func diffFolds(blocks []domain.Block, oldN, newN int) []foldRange {
	if len(blocks) == 0 {
		return nil
	}
	var folds []foldRange
	old, newLine := 0, 0
	add := func(oldEnd, newEnd int, leading, trailing bool) {
		n := min(oldEnd-old, newEnd-newLine)
		before, after := foldContext, foldContext
		if leading {
			before = 0
		}
		if trailing {
			after = 0
		}
		// One folded line still takes a row, so only two or more hidden lines
		// make the view shorter.
		if n-before-after > 1 {
			folds = append(folds, foldRange{old + before, newLine + before, n - before - after})
		}
	}
	for i, b := range blocks {
		add(b.OldStart, b.NewStart, i == 0, false)
		old, newLine = b.OldEnd, b.NewEnd
	}
	add(oldN, newN, false, true)
	return folds
}

// foldedLines projects source lines onto visible rows without rewriting file
// contents or Git block coordinates. A fold occupies one row; source positions
// inside it map to that row. End-of-file positions remain valid boundaries.
type foldedLines struct {
	spans []foldSpan
	total int
}

// foldSpan is one collapsed range: source lines [start, end) are hidden behind
// row, and fold is the index in the tab's fold list so every projection of the
// same fold shares the expansion state.
type foldSpan struct {
	start, end, row, fold int
}

func projectFolds(n int, folds []foldRange, old bool, expanded map[int]bool) foldedLines {
	p := foldedLines{total: n}
	removed := 0
	for i, f := range folds {
		if f.count <= 0 || expanded[i] {
			continue
		}
		start := f.newStart
		if old {
			start = f.oldStart
		}
		if start < 0 || start+f.count > n {
			continue
		}
		p.spans = append(p.spans, foldSpan{start, start + f.count, start - removed, i})
		removed += f.count - 1
	}
	p.total -= removed
	return p
}

func (p foldedLines) row(line int) int {
	i := sort.Search(len(p.spans), func(i int) bool { return p.spans[i].end > line })
	if i < len(p.spans) && line >= p.spans[i].start {
		return p.spans[i].row
	}
	if i == 0 {
		return line
	}
	s := p.spans[i-1]
	return line - (s.end - s.row - 1)
}

func (p foldedLines) source(row int) int {
	i := sort.Search(len(p.spans), func(i int) bool { return p.spans[i].row >= row })
	if i < len(p.spans) && p.spans[i].row == row {
		return p.spans[i].start
	}
	if i == 0 {
		return row
	}
	s := p.spans[i-1]
	return row + s.end - s.row - 1
}

// hiddenAt reports the fold drawn at row and how many source lines it hides.
// row is not a fold row when fold is -1.
func (p foldedLines) hiddenAt(row int) (fold, count int) {
	i := sort.Search(len(p.spans), func(i int) bool { return p.spans[i].row >= row })
	if i < len(p.spans) && p.spans[i].row == row {
		return p.spans[i].fold, p.spans[i].end - p.spans[i].start
	}
	return -1, 0
}

// rebuildFolds runs on content, expansion or toggle changes, never for each
// painted row. Both side-by-side panes and the unified rows share one fold
// list, so expanding a block expands it in every view of the tab.
func (t *diffTab) rebuildFolds() {
	t.folds = nil
	if t.collapse && t.res != nil && !t.src.View && t.res.Conflict == nil {
		t.folds = diffFolds(t.res.Blocks, len(t.oldDisp), len(t.newDisp))
	}
	t.foldOld = projectFolds(len(t.oldDisp), t.folds, true, t.expanded)
	t.foldNew = projectFolds(len(t.newDisp), t.folds, false, t.expanded)
	// The unified viewer's context rows are the same unchanged lines, one row
	// per line, so each side-by-side fold has a matching row range. Keep one
	// placeholder per fold (count 0 is skipped) so expansion indexes stay
	// aligned across the three projections.
	unified := make([]foldRange, len(t.folds))
	next := 0
	for row, u := range t.unified {
		if next == len(t.folds) {
			break
		}
		f := t.folds[next]
		if u.block < 0 && u.old == f.oldStart && u.new == f.newStart {
			unified[next] = foldRange{oldStart: row, newStart: row, count: f.count}
			next++
		}
	}
	t.foldUni = projectFolds(len(t.unified), unified, false, t.expanded)
}

func (t *diffTab) projection(kind selKind) foldedLines {
	switch kind {
	case selOldPane:
		return t.foldOld
	case selNewPane:
		return t.foldNew
	case selUnified:
		return t.foldUni
	default:
		return foldedLines{total: len(t.newDisp)}
	}
}
