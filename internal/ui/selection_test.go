package viffui

import (
	"testing"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
)

// TestTextSelNormalize orders a backwards drag the way text is read.
func TestTextSelNormalize(t *testing.T) {
	s := textSel{anchor: selPoint{line: 3, cell: 7}, focus: selPoint{line: 1, cell: 2}, set: true}
	start, end := s.normalize()
	if start != (selPoint{line: 1, cell: 2}) || end != (selPoint{line: 3, cell: 7}) {
		t.Fatalf("normalize() = %+v, %+v", start, end)
	}
	same := textSel{anchor: selPoint{line: 2, cell: 9}, focus: selPoint{line: 2, cell: 4}, set: true}
	start, end = same.normalize()
	if start != (selPoint{line: 2, cell: 4}) || end != (selPoint{line: 2, cell: 9}) {
		t.Fatalf("normalize() on one line = %+v, %+v", start, end)
	}
}

// TestTextSelEmpty covers the caret without a range, a zero selection and the
// selection that only spans a newline.
func TestTextSelEmpty(t *testing.T) {
	if !(textSel{}).empty() {
		t.Error("zero selection is not empty")
	}
	caret := textSel{anchor: selPoint{line: 1, cell: 4}, focus: selPoint{line: 1, cell: 4}, set: true}
	if !caret.empty() {
		t.Error("collapsed caret is not empty")
	}
	rng := textSel{anchor: selPoint{line: 1, cell: 4}, focus: selPoint{line: 1, cell: 5}, set: true}
	if rng.empty() {
		t.Error("one-cell range reports empty")
	}
	// A selection from the end of one line to the start of the next covers just
	// the newline: no cells to paint, but text to copy.
	newline := textSel{anchor: selPoint{line: 0, cell: 3}, focus: selPoint{line: 1, cell: 0}, set: true}
	if newline.empty() {
		t.Error("newline-only range reports empty")
	}
	if _, _, ok := newline.span(0, 3); ok {
		t.Error("newline-only range paints the first line")
	}
	if got := newline.text([]string{"abc", "def"}); got != "\n" {
		t.Errorf("newline-only copy = %q, want %q", got, "\n")
	}
}

// TestTextSelSpan checks the per-line cell ranges, including a selection that
// ends at the start of a line.
func TestTextSelSpan(t *testing.T) {
	s := textSel{anchor: selPoint{line: 1, cell: 2}, focus: selPoint{line: 3, cell: 1}, set: true}
	cases := []struct {
		line, cells int
		from, to    int
		ok          bool
	}{
		{0, 5, 0, 0, false},
		{1, 5, 2, 5, true},
		{2, 4, 0, 4, true},
		{3, 6, 0, 1, true},
		{4, 6, 0, 0, false},
	}
	for _, c := range cases {
		from, to, ok := s.span(c.line, c.cells)
		if ok != c.ok || (ok && (from != c.from || to != c.to)) {
			t.Errorf("span(%d, %d) = %d, %d, %v; want %d, %d, %v", c.line, c.cells, from, to, ok, c.from, c.to, c.ok)
		}
	}
	if _, _, ok := s.span(3, 6); !ok {
		t.Error("last line disappears")
	}
	// A range that starts at the end of a line has nothing to paint there.
	end := textSel{anchor: selPoint{line: 0, cell: 5}, focus: selPoint{line: 1, cell: 2}, set: true}
	if _, _, ok := end.span(0, 5); ok {
		t.Error("range starting at the line end paints the line")
	}
}

// TestCellOffset maps display cells to UTF-8 bytes and snaps wide runes whole.
func TestCellOffset(t *testing.T) {
	line := "a世b" // 'a' 1 byte, 世 3 bytes (2 cells), b 1 byte
	cases := map[int]int{0: 0, 1: 1, 2: 1, 3: 4, 4: 5, 9: 5, -1: 0}
	for cell, want := range cases {
		if got := cellOffset(line, cell); got != want {
			t.Errorf("cellOffset(%q, %d) = %d, want %d", line, cell, got, want)
		}
	}
	if got := cellOffset("", 3); got != 0 {
		t.Errorf("cellOffset of an empty line = %d", got)
	}
}

// TestNextCell steps over wide runes whole.
func TestNextCell(t *testing.T) {
	line := "a世b"
	if cell, ok := nextCell(line, 1, 1); !ok || cell != 3 {
		t.Errorf("nextCell right from 1 = %d, %v; want 3, true", cell, ok)
	}
	if cell, ok := nextCell(line, 1, -1); !ok || cell != 0 {
		t.Errorf("nextCell left from 1 = %d, %v; want 0, true", cell, ok)
	}
	if _, ok := nextCell(line, 0, -1); ok {
		t.Error("nextCell left at the line start reports a cell")
	}
	if _, ok := nextCell(line, 4, 1); ok {
		t.Error("nextCell right at the line end reports a cell")
	}
}

// TestWordSpan selects the word under a cell, not the punctuation around it.
func TestWordSpan(t *testing.T) {
	line := "func main()"
	if from, to := wordSpan(line, 6); from != 5 || to != 9 {
		t.Errorf("wordSpan inside main = %d, %d; want 5, 9", from, to)
	}
	if from, to := wordSpan(line, 8); from != 5 || to != 9 {
		t.Errorf("wordSpan at the word end = %d, %d; want 5, 9", from, to)
	}
	if from, to := wordSpan(line, 10); from != to {
		t.Errorf("wordSpan on a paren = %d, %d; want an empty range", from, to)
	}
	if from, to := wordSpan("wide 世界", 6); from != 5 || to != 9 {
		t.Errorf("wordSpan around a wide word = %d, %d; want 5, 9", from, to)
	}
}

// TestMoveCaret covers line wrapping, column clamping and wide runes.
func TestMoveCaret(t *testing.T) {
	lines := []string{"abc", "de世", "f"}
	steps := []struct {
		from selPoint
		key  ui.Key
		want selPoint
	}{
		{selPoint{line: 0, cell: 0}, ui.KeyLeft, selPoint{line: 0, cell: 0}},
		{selPoint{line: 0, cell: 0}, ui.KeyRight, selPoint{line: 0, cell: 1}},
		{selPoint{line: 0, cell: 3}, ui.KeyRight, selPoint{line: 1, cell: 0}},
		{selPoint{line: 1, cell: 0}, ui.KeyLeft, selPoint{line: 0, cell: 3}},
		{selPoint{line: 1, cell: 2}, ui.KeyRight, selPoint{line: 1, cell: 4}},
		{selPoint{line: 1, cell: 4}, ui.KeyRight, selPoint{line: 2, cell: 0}},
		{selPoint{line: 0, cell: 2}, ui.KeyDown, selPoint{line: 1, cell: 2}},
		{selPoint{line: 1, cell: 4}, ui.KeyUp, selPoint{line: 0, cell: 3}},
		{selPoint{line: 2, cell: 0}, ui.KeyDown, selPoint{line: 2, cell: 0}},
		{selPoint{line: 1, cell: 4}, ui.KeyHome, selPoint{line: 1, cell: 0}},
		{selPoint{line: 1, cell: 0}, ui.KeyEnd, selPoint{line: 1, cell: 4}},
	}
	for _, s := range steps {
		if got := moveCaret(lines, s.from, s.key); got != s.want {
			t.Errorf("moveCaret(%+v, %v) = %+v, want %+v", s.from, s.key, got, s.want)
		}
	}
}

// TestTextSelText copies the expanded display lines: tabs arrive as the spaces
// the viewer shows and wide characters stay whole.
func TestTextSelText(t *testing.T) {
	lines := []string{"alpha   beta", "wide 世界 line", "last line"}
	cases := []struct {
		name          string
		anchor, focus selPoint
		want          string
	}{
		{
			name:   "partial first and last lines",
			anchor: selPoint{line: 0, cell: 2},
			focus:  selPoint{line: 2, cell: 4},
			want:   "pha   beta\nwide 世界 line\nlast",
		},
		{
			name:   "ends at the start of a line keeps the newline",
			anchor: selPoint{line: 0, cell: 0},
			focus:  selPoint{line: 1, cell: 0},
			want:   "alpha   beta\n",
		},
		{
			name:   "reversed drag",
			anchor: selPoint{line: 1, cell: 14},
			focus:  selPoint{line: 0, cell: 0},
			want:   "alpha   beta\nwide 世界 line",
		},
		{
			name:   "half a wide rune snaps back",
			anchor: selPoint{line: 1, cell: 5},
			focus:  selPoint{line: 1, cell: 6},
			want:   "",
		},
		{
			name:   "a whole wide rune",
			anchor: selPoint{line: 1, cell: 5},
			focus:  selPoint{line: 1, cell: 7},
			want:   "世",
		},
		{
			name:   "one line past the content clamps",
			anchor: selPoint{line: 0, cell: 0},
			focus:  selPoint{line: 9, cell: 0},
			want:   "alpha   beta\nwide 世界 line\nlast line",
		},
	}
	for _, c := range cases {
		s := textSel{anchor: c.anchor, focus: c.focus, set: true}
		if got := s.text(lines); got != c.want {
			t.Errorf("%s: text() = %q, want %q", c.name, got, c.want)
		}
	}
	if got := (textSel{}).text(lines); got != "" {
		t.Errorf("empty selection copies %q", got)
	}
	// Copying is a byte-exact slice of the display line, so multi-byte runes
	// survive whole.
	s := textSel{anchor: selPoint{line: 0, cell: 1}, focus: selPoint{line: 0, cell: 3}, set: true}
	if got := s.text([]string{"héllo"}); got != "él" {
		t.Errorf("multi-byte copy = %q, want %q", got, "él")
	}
}

// TestTabTextSelection checks the tab helpers act on the line list the reader
// is in: the file view's own text, a diff pane, or the unified view's rows.
func TestTabTextSelection(t *testing.T) {
	view := &diffTab{src: vapp.DiffSource{View: true}, res: &vapp.DiffResult{}, newDisp: []string{"alpha   beta", "wide 世界 line"}}
	if !view.selectAllText(false) {
		t.Fatal("selectAllText() did not select a non-empty file")
	}
	if got, want := view.selectionText(), "alpha   beta\nwide 世界 line"; got != want {
		t.Errorf("select all = %q, want %q", got, want)
	}
	if view.textSel.kind != selFile {
		t.Errorf("file view selection kind = %v, want selFile", view.textSel.kind)
	}
	view.clearTextSel()
	if view.hasTextSelection() || view.selectionText() != "" {
		t.Error("clearTextSel left a selection")
	}
	empty := &diffTab{src: vapp.DiffSource{View: true}, res: &vapp.DiffResult{}, newDisp: []string{""}}
	if empty.selectAllText(false) {
		t.Error("selectAllText() reports a range for one empty line")
	}

	// A diff selects text too. Without a clicked pane Select All takes the new
	// side, and a unified diff takes its rows.
	diff := &diffTab{res: &vapp.DiffResult{}, newDisp: []string{"abc"}, unifDisp: []string{"abc", "", "abc"}}
	if !diff.selectAllText(false) {
		t.Fatal("a side-by-side diff took no selection")
	}
	if diff.textSel.kind != selNewPane {
		t.Errorf("diff selection kind = %v, want selNewPane", diff.textSel.kind)
	}
	if got := diff.selectionText(); got != "abc" {
		t.Errorf("new pane select all = %q, want %q", got, "abc")
	}
	diff.clearTextSel()
	if !diff.selectAllText(true) {
		t.Fatal("a unified diff took no selection")
	}
	if diff.textSel.kind != selUnified {
		t.Errorf("unified selection kind = %v, want selUnified", diff.textSel.kind)
	}
	if got := diff.selectionText(); got != "abc\n\nabc" {
		t.Errorf("unified select all = %q, want %q", got, "abc\n\nabc")
	}

	// A pane that was clicked keeps its side.
	oldOnly := &diffTab{res: &vapp.DiffResult{}, oldDisp: []string{"old"}, newDisp: []string{"new"}}
	oldOnly.textSel = textSel{anchor: selPoint{}, focus: selPoint{line: 0, cell: 1}, set: true, kind: selOldPane}
	if !oldOnly.selectAllText(false) {
		t.Fatal("the old pane took no selection")
	}
	if oldOnly.textSel.kind != selOldPane {
		t.Errorf("old pane selection kind = %v, want selOldPane", oldOnly.textSel.kind)
	}
	if got := oldOnly.selectionText(); got != "old" {
		t.Errorf("old pane select all = %q, want %q", got, "old")
	}
}
