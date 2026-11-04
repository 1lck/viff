package domain

import (
	"fmt"
	"strings"
)

// PatchSide is one text of a patch: its lines and whether it lacks a final
// newline.
type PatchSide struct {
	Lines          []string
	NoFinalNewline bool
}

// BuildPatch writes a unified diff that turns old into the text obtained by
// applying only the selected lines of the blocks: a deleted old line i is
// removed when selOld(i), an inserted new line j is added when selNew(j).
// Unselected deletions stay as context; unselected insertions are left out.
// Selecting every line of a block stages the whole hunk; a subset stages
// individual lines. It returns nil when nothing is selected.
func BuildPatch(path string, old, new PatchSide, blocks []Block, selOld, selNew func(int) bool) []byte {
	const context = 3
	type line struct {
		kind    byte // ' ', '-', '+'
		text    string
		old     int // old line index for ' ' and '-', -1 for '+'
		eofNoNL bool
	}
	delta := 0 // applied size change so far, to compute new-side starts
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", path, path, path, path)
	any := false
	i := 0
	for i < len(blocks) {
		// Group blocks whose context windows touch into one hunk.
		j := i
		for j+1 < len(blocks) && blocks[j+1].OldStart-blocks[j].OldEnd <= 2*context {
			j++
		}
		from := max(blocks[i].OldStart-context, 0)
		to := min(blocks[j].OldEnd+context, len(old.Lines))
		var h []line
		selected := false
		x := from
		for k := i; k <= j; k++ {
			bl := blocks[k]
			for ; x < bl.OldStart; x++ {
				h = append(h, line{' ', old.Lines[x], x, false})
			}
			for o := bl.OldStart; o < bl.OldEnd; o++ {
				if selOld(o) {
					h = append(h, line{'-', old.Lines[o], o, false})
					selected = true
				} else {
					h = append(h, line{' ', old.Lines[o], o, false})
				}
			}
			for n := bl.NewStart; n < bl.NewEnd; n++ {
				if selNew(n) {
					h = append(h, line{'+', new.Lines[n], -1, n == len(new.Lines)-1 && new.NoFinalNewline})
					selected = true
				}
			}
			x = bl.OldEnd
		}
		for ; x < to; x++ {
			h = append(h, line{' ', old.Lines[x], x, false})
		}
		if selected {
			any = true
			oldCount, newCount := 0, 0
			for _, l := range h {
				if l.kind != '+' {
					oldCount++
				}
				if l.kind != '-' {
					newCount++
				}
			}
			oldStart := from + 1
			if oldCount == 0 {
				oldStart = from
			}
			newStart := from + 1 + delta
			if newCount == 0 {
				newStart = from + delta
			}
			fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
			lastOld := len(old.Lines) - 1
			// DiffSides makes a final line without newline differ from the same
			// text with one, so a context line is the final line of both texts.
			for _, l := range h {
				b.WriteByte(l.kind)
				b.WriteString(l.text)
				b.WriteByte('\n')
				noNL := false
				switch l.kind {
				case '-':
					noNL = l.old == lastOld && old.NoFinalNewline
				case ' ':
					noNL = l.old == lastOld && old.NoFinalNewline
				case '+':
					noNL = l.eofNoNL
				}
				if noNL {
					b.WriteString("\\ No newline at end of file\n")
				}
			}
			delta += newCount - oldCount
		}
		i = j + 1
	}
	if !any {
		return nil
	}
	return []byte(b.String())
}

// SelectAll selects every line.
func SelectAll(int) bool { return true }

// SelectRange selects lines in [from, to).
func SelectRange(from, to int) func(int) bool {
	return func(i int) bool { return i >= from && i < to }
}

// SelectNone selects nothing.
func SelectNone(int) bool { return false }

// eofSentinel marks a final line that has no newline, so that it compares
// unequal to the same text followed by a newline.
const eofSentinel = "\x00\\ No newline at end of file"

// DiffSides diffs two texts line by line, treating a missing final newline
// as a difference on the last line, which is how Git and patches see it.
func DiffSides(old, new PatchSide, mode WhitespaceMode) []Block {
	return DiffLines(markEOF(old), markEOF(new), mode, 0)
}

func markEOF(s PatchSide) []string {
	if !s.NoFinalNewline || len(s.Lines) == 0 {
		return s.Lines
	}
	out := make([]string, len(s.Lines))
	copy(out, s.Lines)
	out[len(out)-1] += eofSentinel
	return out
}
