package domain

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestParseConflictMarkersMergeStyle(t *testing.T) {
	text := "a\n<<<<<<< HEAD\nours\n=======\ntheirs1\ntheirs2\n>>>>>>> feature\nb\n"
	lines, _ := SplitLines(text)
	p := ParseConflictMarkers(lines)
	if !p.Reliable || len(p.Blocks) != 1 {
		t.Fatalf("parse = %+v", p)
	}
	b := p.Blocks[0]
	if b.Begin != 1 || b.OursStart != 2 || b.OursEnd != 3 || b.TheirsStart != 4 || b.TheirsEnd != 6 || b.Finish != 6 {
		t.Fatalf("block = %+v", b)
	}
	if b.HasBase() || b.OursLabel != "HEAD" || b.TheirsLabel != "feature" {
		t.Fatalf("labels = %+v", b)
	}
}

func TestParseConflictMarkersDiff3AndWidth(t *testing.T) {
	text := strings.Join([]string{
		"<<<<<<<<<< ours", "x", "||||||||||  base", "y", "=======", "not a separator (width 7)", "==========", "z", ">>>>>>>>>> theirs",
		"<<<<<<< two", "p", "=======", "q", ">>>>>>> two",
	}, "\n")
	lines, noNL := SplitLines(text)
	if !noNL {
		t.Fatal("expected missing final newline")
	}
	p := ParseConflictMarkers(lines)
	if !p.Reliable || len(p.Blocks) != 2 {
		t.Fatalf("parse = %+v", p)
	}
	b := p.Blocks[0]
	if !b.HasBase() || b.BaseLabel != "base" || b.MarkerSize != 10 || b.BaseStart != 3 || b.BaseEnd != 6 || b.TheirsStart != 7 {
		t.Fatalf("block = %+v", b)
	}
}

func TestParseConflictMarkersUnterminated(t *testing.T) {
	lines, _ := SplitLines("<<<<<<< a\nx\n=======\ny\n")
	p := ParseConflictMarkers(lines)
	if p.Reliable || len(p.Blocks) != 0 {
		t.Fatalf("parse = %+v", p)
	}
	lines, _ = SplitLines("no markers <<<<<<< here\n=======\n")
	if p := ParseConflictMarkers(lines); !p.Reliable || len(p.Blocks) != 0 {
		t.Fatalf("parse = %+v", p)
	}
}

func TestSplitLinesCRLF(t *testing.T) {
	lines, noNL := SplitLines("a\r\nb\r\n")
	if !reflect.DeepEqual(lines, []string{"a", "b"}) || noNL {
		t.Fatalf("lines = %q %v", lines, noNL)
	}
	if DetectLineEnding("a\r\nb") != "CRLF" || DetectLineEnding("a\nb") != "LF" || DetectLineEnding("a") != "" {
		t.Fatal("line ending detection")
	}
}

// apply rebuilds the new text from the old one and the blocks, which is the
// property every correct diff must satisfy.
func apply(oldLines, newLines []string, blocks []Block) []string {
	var out []string
	x := 0
	for _, b := range blocks {
		out = append(out, oldLines[x:b.OldStart]...)
		out = append(out, newLines[b.NewStart:b.NewEnd]...)
		x = b.OldEnd
	}
	return append(out, oldLines[x:]...)
}

func TestDiffLinesRebuilds(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabet := []string{"a", "b", "c", "d", "e"}
	for iter := 0; iter < 2000; iter++ {
		a := make([]string, rng.Intn(30))
		for i := range a {
			a[i] = alphabet[rng.Intn(len(alphabet))]
		}
		b := make([]string, rng.Intn(30))
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}
		blocks := DiffLines(a, b, WhitespaceExact, 0)
		got := apply(a, b, blocks)
		if !reflect.DeepEqual(got, b) && !(len(got) == 0 && len(b) == 0) {
			t.Fatalf("iter %d: a=%v b=%v blocks=%v got=%v", iter, a, b, blocks, got)
		}
		for i := 1; i < len(blocks); i++ {
			if blocks[i].OldStart < blocks[i-1].OldEnd || blocks[i].NewStart < blocks[i-1].NewEnd {
				t.Fatalf("blocks overlap: %v", blocks)
			}
		}
	}
}

func TestDiffLinesMinimalAndWhitespace(t *testing.T) {
	a := []string{"one", "two", "three", "four"}
	b := []string{"one", "2", "three", "four", "five"}
	blocks := DiffLines(a, b, WhitespaceExact, 0)
	want := []Block{{1, 2, 1, 2}, {4, 4, 4, 5}}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %v", blocks)
	}
	if k := blocks[0].Kind(); k != ChangeModified {
		t.Fatalf("kind = %v", k)
	}
	ws := DiffLines([]string{"a  b", " c"}, []string{"a b", "c "}, WhitespaceIgnore, 0)
	if len(ws) != 0 {
		t.Fatalf("ignore whitespace = %v", ws)
	}
	trim := DiffLines([]string{"a  b", " c"}, []string{"a b", "c "}, WhitespaceTrim, 0)
	if len(trim) != 1 || trim[0] != (Block{0, 1, 0, 1}) {
		t.Fatalf("trim = %v", trim)
	}
}

func TestInlineDiff(t *testing.T) {
	o, n := InlineDiff([]string{"return foo(bar)"}, []string{"return foo(baz)"}, WhitespaceExact)
	if !reflect.DeepEqual(o[0], []Range{{11, 14}}) || !reflect.DeepEqual(n[0], []Range{{11, 14}}) {
		t.Fatalf("inline = %v %v", o, n)
	}
}

func TestMapLine(t *testing.T) {
	blocks := []Block{{2, 3, 2, 6}} // one old line became four
	if got := MapLine(blocks, 1, true); got != 1 {
		t.Fatal(got)
	}
	if got := MapLine(blocks, 2.5, true); got != 4 {
		t.Fatal(got)
	}
	if got := MapLine(blocks, 10, true); got != 13 {
		t.Fatal(got)
	}
	if got := MapLine(blocks, 13, false); got != 10 {
		t.Fatal(got)
	}
}

func TestGraphLayoutMerge(t *testing.T) {
	// m merges b into a; history: m -> a1 -> root, m -> b1 -> root
	commits := []Commit{
		{Hash: "m", Parents: []string{"a1", "b1"}},
		{Hash: "b1", Parents: []string{"root"}},
		{Hash: "a1", Parents: []string{"root"}},
		{Hash: "root"},
	}
	var g GraphLayout
	rows := make([]GraphRow, len(commits))
	for i, c := range commits {
		rows[i] = g.Append(c)
	}
	if rows[0].Lane != 0 || !rows[0].IsMerge {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1].Lane != 1 {
		t.Fatalf("b1 lane = %+v", rows[1])
	}
	if rows[2].Lane != 0 {
		t.Fatalf("a1 lane = %+v", rows[2])
	}
	if rows[3].Lane != 0 {
		t.Fatalf("root lane = %+v", rows[3])
	}
	// root collects both lanes in its upper half.
	converge := 0
	for _, e := range rows[3].Edges {
		if !e.Lower && e.To == 0 {
			converge++
		}
	}
	if converge != 2 {
		t.Fatalf("root edges = %+v", rows[3].Edges)
	}
	if len(rows[3].Edges) != 2 {
		t.Fatalf("root should have no lower edges: %+v", rows[3].Edges)
	}
}

func TestGraphLayoutContinuityAcrossPages(t *testing.T) {
	// The lower lanes of one row must equal the upper lanes of the next.
	rng := rand.New(rand.NewSource(7))
	var commits []Commit
	n := 300
	for i := 0; i < n; i++ {
		c := Commit{Hash: string(rune('A' + i%26)) + string(rune('0'+i/26%10)) + string(rune('a'+i/260))}
		commits = append(commits, c)
	}
	for i := range commits {
		if i+1 < n {
			commits[i].Parents = append(commits[i].Parents, commits[i+1].Hash)
		}
		if i+5 < n && rng.Intn(6) == 0 {
			commits[i].Parents = append(commits[i].Parents, commits[i+2+rng.Intn(3)].Hash)
		}
	}
	var g GraphLayout
	var prevBottom map[int]bool
	for _, c := range commits {
		r := g.Append(c)
		top := map[int]bool{}
		bottom := map[int]bool{}
		for _, e := range r.Edges {
			if e.Lower {
				bottom[e.To] = true
			} else {
				top[e.From] = true
			}
		}
		if prevBottom != nil {
			for lane := range prevBottom {
				if !top[lane] {
					t.Fatalf("lane %d ends between rows at %s", lane, c.Hash)
				}
			}
			for lane := range top {
				if !prevBottom[lane] {
					t.Fatalf("lane %d starts from nowhere at %s", lane, c.Hash)
				}
			}
		}
		prevBottom = bottom
	}
}

func TestStatusEntryAreas(t *testing.T) {
	e := StatusEntry{Path: "a", Index: 'M', Worktree: 'M'}
	if !e.Staged() || !e.Unstaged() || e.Conflicted() {
		t.Fatal("MM")
	}
	u := StatusEntry{Path: "b", Index: 'U', Worktree: 'U'}
	if u.Staged() || !u.Conflicted() {
		t.Fatal("UU")
	}
	s := Status{Entries: []StatusEntry{e, u, {Path: "c", Index: '?', Worktree: '?'}}}
	st, un, ut, cf := s.Counts()
	if st != 1 || un != 1 || ut != 1 || cf != 1 {
		t.Fatal(st, un, ut, cf)
	}
}

func TestSideLabelsRebaseDoesNotSayMine(t *testing.T) {
	op := Operation{Kind: OpRebase, Onto: "0123456789abcdef", Incoming: "fedcba9876543210", IncomingSubject: "Fix"}
	ours, theirs := op.SideLabels("feature")
	if strings.Contains(strings.ToLower(ours), "your") || !strings.Contains(ours, "01234567") || !strings.Contains(theirs, "Fix") {
		t.Fatal(ours, theirs)
	}
}
