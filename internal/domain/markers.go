package domain

import "strings"

// ConflictBlock is one conflict region of a file with conflict markers.
// Line numbers are 0-based indexes into the file's lines; ranges are
// half-open [Start, End). Base is empty (BaseStart == BaseEnd == -1) for the
// two-way "merge" style.
type ConflictBlock struct {
	Begin       int // line of <<<<<<<
	OursStart   int
	OursEnd     int
	BaseStart   int
	BaseEnd     int
	TheirsStart int
	TheirsEnd   int
	Finish      int // line of >>>>>>>
	OursLabel   string
	BaseLabel   string
	TheirsLabel string
	MarkerSize  int
}

// HasBase reports a diff3/zdiff3 block with a base section.
func (b ConflictBlock) HasBase() bool { return b.BaseStart >= 0 }

// MarkerParse is the result of scanning a file for conflict markers.
type MarkerParse struct {
	Blocks []ConflictBlock
	// Reliable is false when markers were found but do not form complete,
	// well-ordered blocks. The UI then falls back to a file-level notice and
	// shows no line numbers it cannot back up.
	Reliable bool
}

// markerOf reports whether line is a conflict marker made of c. It returns
// the marker's width and the label after it. A marker is a run of at least
// seven c characters at the start of the line followed by a space or the end
// of the line (Git's conflict-marker-size may make it longer).
func markerOf(line string, c byte) (size int, label string, ok bool) {
	n := 0
	for n < len(line) && line[n] == c {
		n++
	}
	if n < 7 {
		return 0, "", false
	}
	rest := strings.TrimRight(line[n:], "\r")
	if rest == "" {
		return n, "", true
	}
	if rest[0] != ' ' {
		return 0, "", false
	}
	return n, strings.TrimSpace(rest[1:]), true
}

// ParseConflictMarkers finds conflict blocks in lines (without terminators).
// It understands the merge, diff3 and zdiff3 styles and any marker width,
// matching each block's markers by width so nested or literal marker-like
// lines of another width do not end a block early.
func ParseConflictMarkers(lines []string) MarkerParse {
	result := MarkerParse{Reliable: true}
	const (
		outside = iota
		inOurs
		inBase
		inTheirs
	)
	state := outside
	var block ConflictBlock
	for i, line := range lines {
		switch state {
		case outside:
			if size, label, ok := markerOf(line, '<'); ok {
				block = ConflictBlock{Begin: i, OursStart: i + 1, BaseStart: -1, BaseEnd: -1, OursLabel: label, MarkerSize: size}
				state = inOurs
				continue
			}
			// A separator or closer outside a block means the markers were
			// edited by hand; keep scanning but report it.
			if size, _, ok := markerOf(line, '>'); ok && size >= 7 {
				result.Reliable = false
			}
		case inOurs:
			if size, label, ok := markerOf(line, '|'); ok && size == block.MarkerSize {
				block.OursEnd = i
				block.BaseStart = i + 1
				block.BaseLabel = label
				state = inBase
			} else if isSeparator(line, block.MarkerSize) {
				block.OursEnd = i
				block.TheirsStart = i + 1
				state = inTheirs
			} else if size, _, ok := markerOf(line, '<'); ok && size == block.MarkerSize {
				result.Reliable = false
				block = ConflictBlock{Begin: i, OursStart: i + 1, BaseStart: -1, BaseEnd: -1, MarkerSize: size}
			}
		case inBase:
			if isSeparator(line, block.MarkerSize) {
				block.BaseEnd = i
				block.TheirsStart = i + 1
				state = inTheirs
			}
		case inTheirs:
			if size, label, ok := markerOf(line, '>'); ok && size == block.MarkerSize {
				block.TheirsEnd = i
				block.Finish = i
				block.TheirsLabel = label
				result.Blocks = append(result.Blocks, block)
				state = outside
			} else if isSeparator(line, block.MarkerSize) {
				result.Reliable = false
			}
		}
	}
	if state != outside {
		result.Reliable = false
	}
	return result
}

func isSeparator(line string, size int) bool {
	line = strings.TrimRight(line, "\r")
	if len(line) != size {
		return false
	}
	for i := 0; i < len(line); i++ {
		if line[i] != '=' {
			return false
		}
	}
	return true
}

// SplitLines splits text into lines without their terminators. A final
// newline does not produce an empty last line. It also reports whether the
// text ended without a newline.
func SplitLines(text string) (lines []string, noFinalNewline bool) {
	if text == "" {
		return nil, false
	}
	lines = strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	} else {
		noFinalNewline = true
	}
	for i, l := range lines {
		if strings.HasSuffix(l, "\r") {
			lines[i] = l[:len(l)-1]
		}
	}
	return lines, noFinalNewline
}

// DetectLineEnding reports "CRLF", "LF" or "" for text without newlines.
func DetectLineEnding(text string) string {
	i := strings.IndexByte(text, '\n')
	if i < 0 {
		return ""
	}
	if i > 0 && text[i-1] == '\r' {
		return "CRLF"
	}
	return "LF"
}
