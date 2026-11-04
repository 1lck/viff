package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ResolveChoice names the content of one conflict block that the user accepts.
type ResolveChoice uint8

const (
	// ResolveOurs keeps the block's ours lines: the current branch for a
	// merge, the current HEAD for a cherry-pick, the onto side for a rebase.
	ResolveOurs ResolveChoice = iota
	// ResolveTheirs keeps the block's theirs lines: the other branch or the
	// commit that is being brought in.
	ResolveTheirs
	// ResolveBoth keeps ours followed by theirs, in that order.
	ResolveBoth
)

// String is the choice as shown in the UI: "Ours", "Theirs" or "Both".
func (c ResolveChoice) String() string {
	switch c {
	case ResolveOurs:
		return "Ours"
	case ResolveTheirs:
		return "Theirs"
	case ResolveBoth:
		return "Both"
	}
	return "Unknown"
}

// Valid reports whether c is a choice ResolveConflictText understands.
func (c ResolveChoice) Valid() bool { return c <= ResolveBoth }

// Errors returned when a file cannot be resolved safely. The file is never
// changed when one of these is returned.
var (
	// ErrConflictAbsent means the text holds no conflict block.
	ErrConflictAbsent = errors.New("the file has no conflict blocks")
	// ErrConflictBlock means the requested block does not exist; the view the
	// caller used is probably stale.
	ErrConflictBlock = errors.New("the conflict block does not exist")
	// ErrConflictMarkers means the markers are incomplete, out of order or
	// were edited by hand, so block boundaries cannot be trusted.
	ErrConflictMarkers = errors.New("the conflict markers are incomplete or out of order")
	// ErrConflictAmbiguous means the resolved text would still contain
	// marker-like lines that make the remaining blocks ambiguous, so nothing
	// is written.
	ErrConflictAmbiguous = errors.New("resolving the block would leave ambiguous conflict markers")
)

// conflictBOM is the UTF-8 byte-order mark. The diff viewer drops it before
// showing lines; resolving keeps it where it was.
const conflictBOM = "\xef\xbb\xbf"

// ResolveConflictText accepts one conflict block of text and returns the whole
// text with that block replaced by the chosen side(s). Lines outside the block
// are copied byte for byte, so other blocks and line endings stay as they are.
//
// The marker lines are dropped, as is a diff3 base section: accepting "both"
// keeps ours followed by theirs, never the base. The block index counts the
// blocks ParseConflictMarkers finds, the same numbering the conflict viewer
// uses.
//
// It returns an error, and never a partly resolved text, when the markers are
// not well formed, when the block does not exist, or when the result would
// leave ambiguous conflict markers behind.
func ResolveConflictText(text string, block int, choice ResolveChoice) (string, error) {
	if !choice.Valid() {
		return "", fmt.Errorf("unknown conflict choice %d", choice)
	}
	prefix := ""
	if strings.HasPrefix(text, conflictBOM) {
		prefix, text = conflictBOM, text[len(conflictBOM):]
	}
	lines := strings.Split(text, "\n")
	parse := ParseConflictMarkers(lines)
	if !parse.Reliable {
		return "", ErrConflictMarkers
	}
	if len(parse.Blocks) == 0 {
		return "", ErrConflictAbsent
	}
	if block < 0 || block >= len(parse.Blocks) {
		return "", fmt.Errorf("%w: block %d of %d", ErrConflictBlock, block, len(parse.Blocks))
	}
	b := parse.Blocks[block]
	if err := checkConflictBlock(b, len(lines)); err != nil {
		return "", err
	}
	out := make([]string, 0, len(lines))
	out = append(out, lines[:b.Begin]...)
	switch choice {
	case ResolveOurs:
		out = append(out, lines[b.OursStart:b.OursEnd]...)
	case ResolveTheirs:
		out = append(out, lines[b.TheirsStart:b.TheirsEnd]...)
	case ResolveBoth:
		out = append(out, lines[b.OursStart:b.OursEnd]...)
		out = append(out, lines[b.TheirsStart:b.TheirsEnd]...)
	}
	out = append(out, lines[b.Finish+1:]...)
	result := strings.Join(out, "\n")
	// A block that ends a file without a final newline can leave the chosen
	// line's carriage return as the last byte once the closing marker is
	// gone: the text belongs to a CRLF line whose terminator is now unused.
	if b.Finish == len(lines)-1 && !strings.HasSuffix(text, "\n") && strings.HasSuffix(result, "\r") {
		result = strings.TrimSuffix(result, "\r")
	}
	// Resolving one block must not disturb another: the result must parse as
	// the same blocks minus the one that was accepted.
	after := ParseConflictMarkers(strings.Split(result, "\n"))
	if !after.Reliable || len(after.Blocks) != len(parse.Blocks)-1 {
		return "", ErrConflictAmbiguous
	}
	return prefix + result, nil
}

// checkConflictBlock verifies the ranges of a parsed block point forward and
// stay inside the text. A reliable parse implies this, but the resolver
// depends on it, so it is checked before any line is copied.
func checkConflictBlock(b ConflictBlock, n int) error {
	// TheirsEnd and Finish are the same line: ParseConflictMarkers records
	// the closing marker as both the exclusive end of the theirs section and
	// the marker line itself.
	ok := b.Begin >= 0 && b.Begin < b.OursStart && b.OursStart <= b.OursEnd &&
		b.OursEnd < b.TheirsStart && b.TheirsStart <= b.TheirsEnd &&
		b.TheirsEnd <= b.Finish && b.Finish < n
	if b.HasBase() {
		ok = ok && b.BaseStart == b.OursEnd+1 && b.BaseEnd >= b.BaseStart && b.BaseEnd+1 == b.TheirsStart
	}
	if !ok {
		return ErrConflictMarkers
	}
	return nil
}
