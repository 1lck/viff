package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// WhitespaceMode decides which whitespace differences a diff ignores.
type WhitespaceMode uint8

const (
	WhitespaceExact  WhitespaceMode = iota
	WhitespaceTrim                  // ignore leading and trailing whitespace
	WhitespaceIgnore                // ignore all whitespace
)

// Title is the label used in the diff toolbar.
func (m WhitespaceMode) Title() string {
	switch m {
	case WhitespaceTrim:
		return "Trim whitespaces"
	case WhitespaceIgnore:
		return "Ignore whitespaces"
	}
	return "Do not ignore"
}

// Block is one changed region between two texts: lines [OldStart, OldEnd)
// of the old text were replaced by [NewStart, NewEnd) of the new. A pure
// insertion has OldStart == OldEnd, a pure deletion NewStart == NewEnd.
type Block struct {
	OldStart, OldEnd int
	NewStart, NewEnd int
}

// Kind reports whether the block inserts, deletes or modifies lines.
func (b Block) Kind() ChangeKind {
	switch {
	case b.OldStart == b.OldEnd:
		return ChangeAdded
	case b.NewStart == b.NewEnd:
		return ChangeDeleted
	}
	return ChangeModified
}

// DiffLines compares two texts line by line and returns the changed blocks
// in order. maxCost bounds the work for pathological inputs; past it the
// remaining region is reported as one block rather than spending unbounded
// time (0 means a default).
func DiffLines(oldLines, newLines []string, mode WhitespaceMode, maxCost int) []Block {
	if maxCost <= 0 {
		maxCost = 4000
	}
	ids := make(map[string]int, len(oldLines)+len(newLines))
	key := func(s string) string {
		switch mode {
		case WhitespaceTrim:
			return strings.TrimSpace(s)
		case WhitespaceIgnore:
			return strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) {
					return -1
				}
				return r
			}, s)
		}
		return s
	}
	intern := func(lines []string) []int {
		out := make([]int, len(lines))
		for i, l := range lines {
			k := key(l)
			id, ok := ids[k]
			if !ok {
				id = len(ids)
				ids[k] = id
			}
			out[i] = id
		}
		return out
	}
	a, b := intern(oldLines), intern(newLines)
	ops := diffInts(a, b, maxCost)
	return blocksOf(ops)
}

// op is a run of equal, deleted or inserted elements.
type op struct {
	kind  byte // '=', '-', '+'
	count int
}

func blocksOf(ops []op) []Block {
	var blocks []Block
	x, y := 0, 0
	var cur *Block
	for _, o := range ops {
		switch o.kind {
		case '=':
			if cur != nil {
				blocks = append(blocks, *cur)
				cur = nil
			}
			x += o.count
			y += o.count
		case '-':
			if cur == nil {
				cur = &Block{OldStart: x, OldEnd: x, NewStart: y, NewEnd: y}
			}
			x += o.count
			cur.OldEnd = x
		case '+':
			if cur == nil {
				cur = &Block{OldStart: x, OldEnd: x, NewStart: y, NewEnd: y}
			}
			y += o.count
			cur.NewEnd = y
		}
	}
	if cur != nil {
		blocks = append(blocks, *cur)
	}
	return blocks
}

// diffInts is Myers' O(ND) diff with the linear-space bisection of
// diff-match-patch, over interned elements.
func diffInts(a, b []int, maxCost int) []op {
	var ops []op
	emit := func(kind byte, n int) {
		if n <= 0 {
			return
		}
		if len(ops) > 0 && ops[len(ops)-1].kind == kind {
			ops[len(ops)-1].count += n
			return
		}
		ops = append(ops, op{kind, n})
	}
	budget := maxCost * 64
	var rec func(a, b []int)
	rec = func(a, b []int) {
		// Common prefix and suffix.
		p := 0
		for p < len(a) && p < len(b) && a[p] == b[p] {
			p++
		}
		s := 0
		for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
			s++
		}
		emit('=', p)
		ma, mb := a[p:len(a)-s], b[p:len(b)-s]
		switch {
		case len(ma) == 0:
			emit('+', len(mb))
		case len(mb) == 0:
			emit('-', len(ma))
		default:
			x, y, ok := bisect(ma, mb, maxCost, &budget)
			if !ok {
				emit('-', len(ma))
				emit('+', len(mb))
			} else {
				rec(ma[:x], mb[:y])
				rec(ma[x:], mb[y:])
			}
		}
		emit('=', s)
	}
	rec(a, b)
	return ops
}

// bisect finds the middle snake's split point. It fails when no useful split
// exists within the cost bounds.
func bisect(t1, t2 []int, maxCost int, budget *int) (int, int, bool) {
	len1, len2 := len(t1), len(t2)
	maxD := (len1 + len2 + 1) / 2
	if maxD > maxCost {
		maxD = maxCost
	}
	vOffset := (len1 + len2 + 1) / 2
	vLength := 2 * vOffset
	if vLength < 2 {
		vLength = 2
	}
	v1 := make([]int, vLength+2)
	v2 := make([]int, vLength+2)
	for i := range v1 {
		v1[i], v2[i] = -1, -1
	}
	v1[vOffset+1], v2[vOffset+1] = 0, 0
	delta := len1 - len2
	front := delta%2 != 0
	k1start, k1end, k2start, k2end := 0, 0, 0, 0
	for d := 0; d < maxD; d++ {
		*budget -= d + 1
		if *budget < 0 {
			return 0, 0, false
		}
		for k1 := -d + k1start; k1 <= d-k1end; k1 += 2 {
			k1Offset := vOffset + k1
			var x1 int
			if k1 == -d || (k1 != d && v1[k1Offset-1] < v1[k1Offset+1]) {
				x1 = v1[k1Offset+1]
			} else {
				x1 = v1[k1Offset-1] + 1
			}
			y1 := x1 - k1
			for x1 < len1 && y1 < len2 && t1[x1] == t2[y1] {
				x1++
				y1++
			}
			v1[k1Offset] = x1
			if x1 > len1 {
				k1end += 2
			} else if y1 > len2 {
				k1start += 2
			} else if front {
				k2Offset := vOffset + delta - k1
				if k2Offset >= 0 && k2Offset < vLength && v2[k2Offset] != -1 {
					x2 := len1 - v2[k2Offset]
					if x1 >= x2 {
						return x1, y1, x1 > 0 || y1 > 0
					}
				}
			}
		}
		for k2 := -d + k2start; k2 <= d-k2end; k2 += 2 {
			k2Offset := vOffset + k2
			var x2 int
			if k2 == -d || (k2 != d && v2[k2Offset-1] < v2[k2Offset+1]) {
				x2 = v2[k2Offset+1]
			} else {
				x2 = v2[k2Offset-1] + 1
			}
			y2 := x2 - k2
			for x2 < len1 && y2 < len2 && t1[len1-x2-1] == t2[len2-y2-1] {
				x2++
				y2++
			}
			v2[k2Offset] = x2
			if x2 > len1 {
				k2end += 2
			} else if y2 > len2 {
				k2start += 2
			} else if !front {
				k1Offset := vOffset + delta - k2
				if k1Offset >= 0 && k1Offset < vLength && v1[k1Offset] != -1 {
					x1 := v1[k1Offset]
					y1 := vOffset + x1 - k1Offset
					x2 = len1 - x2
					if x1 >= x2 {
						return x1, y1, x1 > 0 || y1 > 0
					}
				}
			}
		}
	}
	return 0, 0, false
}

// Range is a half-open byte range within one line.
type Range struct{ Start, End int }

// InlineDiff compares the old and new lines of a modified block word by word
// and returns, for each line, the byte ranges that differ. Whitespace-only
// differences are ignored when mode is not exact.
func InlineDiff(oldLines, newLines []string, mode WhitespaceMode) (oldRanges, newRanges [][]Range) {
	type token struct {
		line, start, end int
		text             string
	}
	tokenize := func(lines []string) []token {
		var toks []token
		for li, l := range lines {
			i := 0
			for i < len(l) {
				r, size := utf8.DecodeRuneInString(l[i:])
				j := i + size
				switch {
				case isWordRune(r):
					for j < len(l) {
						r2, s2 := utf8.DecodeRuneInString(l[j:])
						if !isWordRune(r2) {
							break
						}
						j += s2
					}
				case r == ' ' || r == '\t':
					for j < len(l) && (l[j] == ' ' || l[j] == '\t') {
						j++
					}
				}
				toks = append(toks, token{li, i, j, l[i:j]})
				i = j
			}
			toks = append(toks, token{li, len(l), len(l), "\n"})
		}
		return toks
	}
	ta, tb := tokenize(oldLines), tokenize(newLines)
	if len(ta)*len(tb) > 4_000_000 {
		return nil, nil
	}
	ids := map[string]int{}
	norm := func(s string) string {
		if mode != WhitespaceExact && strings.TrimSpace(s) == "" && s != "\n" {
			return " "
		}
		return s
	}
	intern := func(toks []token) []int {
		out := make([]int, len(toks))
		for i, t := range toks {
			k := norm(t.text)
			id, ok := ids[k]
			if !ok {
				id = len(ids)
				ids[k] = id
			}
			out[i] = id
		}
		return out
	}
	ops := diffInts(intern(ta), intern(tb), 2000)
	oldRanges = make([][]Range, len(oldLines))
	newRanges = make([][]Range, len(newLines))
	add := func(dst [][]Range, t token) {
		if t.text == "\n" || (mode != WhitespaceExact && strings.TrimSpace(t.text) == "") {
			return
		}
		rs := dst[t.line]
		if n := len(rs); n > 0 && rs[n-1].End == t.start {
			rs[n-1].End = t.end
		} else {
			rs = append(rs, Range{t.start, t.end})
		}
		dst[t.line] = rs
	}
	x, y := 0, 0
	for _, o := range ops {
		switch o.kind {
		case '=':
			x += o.count
			y += o.count
		case '-':
			for i := 0; i < o.count; i++ {
				add(oldRanges, ta[x+i])
			}
			x += o.count
		case '+':
			for i := 0; i < o.count; i++ {
				add(newRanges, tb[y+i])
			}
			y += o.count
		}
	}
	return oldRanges, newRanges
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// MapLine maps a line index of one side to the corresponding position on
// the other side, interpolating inside changed blocks. It is what keeps the
// two panes of a side-by-side diff aligned while scrolling. fromOld selects
// the direction. The result is fractional so scrolling inside a block of
// unequal heights moves smoothly.
func MapLine(blocks []Block, line float64, fromOld bool) float64 {
	shift := 0.0
	for _, b := range blocks {
		fs, fe, ts, te := b.OldStart, b.OldEnd, b.NewStart, b.NewEnd
		if !fromOld {
			fs, fe, ts, te = ts, te, fs, fe
		}
		if line < float64(fs) {
			break
		}
		if line < float64(fe) || (fs == fe && line < float64(fe)) {
			frac := (line - float64(fs)) / float64(fe-fs)
			return float64(ts) + frac*float64(te-ts)
		}
		shift = float64(te - fe)
	}
	return line + shift
}
