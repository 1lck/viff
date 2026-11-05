package app

import (
	"fmt"
	"strings"

	"viff/internal/domain"
)

// ConflictContextAll is the ConflictContextOptions.Block value that copies
// every conflict block of the file.
const ConflictContextAll = -1

// Limits of one copied conflict context. They keep a paste within what an
// external assistant accepts, and they make the size predictable: one huge
// block cannot crowd out the other blocks, and a file with many blocks still
// copies in bounded time and memory. A block over a limit is cut with a note
// at the cut, never silently.
const (
	maxContextBlockLines = 200
	maxContextBlockBytes = 20 << 10
	maxContextBytes      = 200 << 10
	// contextTailReserve stays free while blocks are appended, so the note
	// that says how much was left out always fits in maxContextBytes.
	contextTailReserve = 512
)

// ConflictContextOptions selects what a copied conflict context contains.
type ConflictContextOptions struct {
	// RepoRoot is the working-tree root the file lives in. It is copied
	// verbatim, so the recipient can open the same file.
	RepoRoot string
	// Operation is the merge, rebase, cherry-pick or revert that stopped on
	// the conflict; the zero value states that Git reports none.
	Operation domain.Operation
	// Block is the 0-based index of one block to copy, or
	// ConflictContextAll. A stale index copies every block instead of an
	// empty text, and the scope line says what was copied.
	Block int
}

// ConflictContextText renders the structured context of one unmerged file for
// an external AI assistant: repository, path, operation, both side labels, and
// each conflict block's ours, base and theirs text with its line range.
//
// It is a pure function of the view the conflict tab already shows: it reads no
// file, starts no process and speaks to no model. Viff itself only puts the
// result on the clipboard; the user decides where it goes.
//
// A nil view or a view without a working-tree text renders "". Blocks whose
// marker parse is not reliable are still rendered as parsed, so callers that
// cannot trust the parse must not offer the action (the conflict viewer already
// gates on MarkerParse.Reliable).
func ConflictContextText(cv *ConflictView, o ConflictContextOptions) string {
	if cv == nil {
		return ""
	}
	text, _ := conflictContext(cv, o)
	return text
}

// conflictContext builds the text and also reports whether it had to leave
// anything out, so tests can assert truncation without matching prose.
func conflictContext(cv *ConflictView, o ConflictContextOptions) (string, bool) {
	blocks := cv.Markers.Blocks
	single := o.Block >= 0 && o.Block < len(blocks)
	var b strings.Builder
	writeContextHeader(&b, cv, o, len(blocks), single)
	writeContextInstructions(&b, single)

	truncated := false
	for i, block := range blocks {
		if o.Block >= 0 && o.Block < len(blocks) && i != o.Block {
			continue
		}
		var bb strings.Builder
		writeContextBlock(&bb, cv, block, i+1, len(blocks))
		blockText := bb.String()
		if b.Len()+len(blockText)+contextTailReserve > maxContextBytes {
			left := remainingBlocks(blocks, o, i)
			fmt.Fprintf(&b, "## Context truncated\n\nViff stopped here: %s would push the text past the %d KB limit, so %s not shown. Everything above is complete.\n",
				blockCount(left), maxContextBytes>>10, blockVerb(left))
			truncated = true
			break
		}
		b.WriteString(blockText)
	}
	text := b.String()
	if len(text) > maxContextBytes {
		// The header and the instructions are bounded well below the limit, so
		// this only guards a future header change; cut at a rune boundary and
		// keep the whole text inside the cap including the note itself.
		note := "\n[... Viff truncated the text at the 200 KB limit ...]\n"
		text = truncateContextText(text, maxContextBytes-len(note)) + note
		truncated = true
	}
	return text, truncated
}

// remainingBlocks counts the blocks from index from on that the options still
// select.
func remainingBlocks(blocks []domain.ConflictBlock, o ConflictContextOptions, from int) int {
	n := 0
	for i := from; i < len(blocks); i++ {
		if o.Block >= 0 && o.Block < len(blocks) && i != o.Block {
			continue
		}
		n++
	}
	return n
}

func blockCount(n int) string {
	if n == 1 {
		return "1 conflict block"
	}
	return fmt.Sprintf("%d conflict blocks", n)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func blockVerb(n int) string {
	if n == 1 {
		return "it is"
	}
	return "they are"
}

func writeContextHeader(b *strings.Builder, cv *ConflictView, o ConflictContextOptions, blocks int, single bool) {
	root := strings.TrimSpace(o.RepoRoot)
	if root == "" {
		root = "(unknown)"
	}
	b.WriteString("# Viff conflict context\n\n")
	fmt.Fprintf(b, "- Repository: %s\n", root)
	fmt.Fprintf(b, "- File: %s\n", cv.Conflict.Path)
	fmt.Fprintf(b, "- Operation: %s\n", contextOperation(o.Operation))
	fmt.Fprintf(b, "- Ours (stage 2): %s\n", contextSource(cv.OursLabel))
	fmt.Fprintf(b, "- Theirs (stage 3): %s\n", contextSource(cv.TheirsLabel))
	fmt.Fprintf(b, "- Conflict type: %s\n", cv.Conflict.Kind.Description())
	if style := contextStyle(cv, blocks); style != "" {
		fmt.Fprintf(b, "- Conflict style: %s\n", style)
	}
	if cv.Current.Content.Exists && cv.Current.Text {
		if e := cv.Current.LineEnding; e != "" {
			fmt.Fprintf(b, "- Line endings: %s, final newline: %s\n", e, contextYesNo(!cv.Current.NoEOL))
		} else {
			b.WriteString("- Line endings: none (the file has no line break)\n")
		}
	}
	switch {
	case blocks == 0:
		b.WriteString("- Conflict blocks: none in the working-tree file\n")
	case single:
		fmt.Fprintf(b, "- Conflict blocks: %d; this text includes block %d only\n", blocks, o.Block+1)
	default:
		fmt.Fprintf(b, "- Conflict blocks: %d\n", blocks)
	}
	b.WriteString("\nThe working-tree file holds these blocks between conflict markers.")
	b.WriteString(" The sections below are copied from between them, ours first.\n")
}

func writeContextInstructions(b *strings.Builder, single bool) {
	b.WriteString(`
## Instructions

- Change only the file named above. Leave every other file alone.
- Remove every conflict marker (<<<<<<<, |||||||, =======, >>>>>>>) and keep the code both sides need.
- Keep the file's line endings and its final-newline state as listed above.
- Do not stage, commit, continue or abort the operation; the user does those steps in Viff.
- Keep the rest of the file unchanged. If you cannot edit the file yourself, reply with the resolved text for each block below, in order.
- When the file is edited, return to Viff: it refreshes the file automatically, and the user marks it as resolved once no markers remain.
`)
	if single {
		b.WriteString("- Only the block below is included in this text; the other blocks of the file are not shown.\n")
	}
}

func writeContextBlock(b *strings.Builder, cv *ConflictView, block domain.ConflictBlock, number, total int) {
	lines := cv.Current.Lines
	fmt.Fprintf(b, "\n## Conflict block %d of %d (working-tree %s)\n", number, total, contextRange(block.Begin, block.Finish+1, len(lines)))
	if block.HasBase() {
		b.WriteString("\nMarkers in the file: `<<<<<<<` ours, `|||||||` base, `=======`, `>>>>>>>` theirs.\n")
	} else {
		b.WriteString("\nMarkers in the file: `<<<<<<<` ours, `=======`, `>>>>>>>` theirs.\n")
	}
	budget := contextBudget{lines: maxContextBlockLines, bytes: maxContextBlockBytes}
	writeContextSide(b, "ours", "stage 2", cv.OursLabel, block.OursStart, block.OursEnd, lines, &budget)
	if block.HasBase() {
		writeContextSide(b, "base", "stage 1", block.BaseLabel, block.BaseStart, block.BaseEnd, lines, &budget)
	}
	writeContextSide(b, "theirs", "stage 3", cv.TheirsLabel, block.TheirsStart, block.TheirsEnd, lines, &budget)
}

// contextBudget is the remaining room of one block. Sections take from it in
// file order, so the first oversized side cannot hide the others.
type contextBudget struct {
	lines int
	bytes int
}

// contextSideLines clips lines to the budget and reports the note that names
// what was left out.
func (bud *contextBudget) take(lines []string) (kept []string, dropped int) {
	for _, line := range lines {
		if bud.lines <= 0 || bud.bytes < len(line)+1 {
			break
		}
		kept = append(kept, line)
		bud.lines--
		bud.bytes -= len(line) + 1
	}
	return kept, len(lines) - len(kept)
}

func writeContextSide(b *strings.Builder, name, stage, source string, start, end int, file []string, bud *contextBudget) {
	start, end = clampRange(start, end, len(file))
	header := fmt.Sprintf("### %s (%s, %s)", name, stage, contextRange(start, end, len(file)))
	// The marker's own label is only added when it names something other than
	// the side, so a conventional "||||||| base" does not read as "base - base".
	if s := strings.TrimSpace(source); s != "" && !strings.EqualFold(s, name) {
		header += " - " + s
	}
	b.WriteString("\n" + header + "\n\n")
	lines := file[start:end]
	if len(lines) == 0 {
		b.WriteString("(no content on this side)\n")
		return
	}
	kept, dropped := bud.take(lines)
	writeContextFence(b, kept, dropped)
}

// writeContextFence writes one fenced block. A fence longer than any backtick
// run in the content keeps file text from ending the fence early.
func writeContextFence(b *strings.Builder, lines []string, dropped int) {
	longest := 2
	for _, line := range lines {
		run := 0
		for _, r := range line {
			if r != '`' {
				run = 0
				continue
			}
			run++
			if run > longest {
				longest = run
			}
		}
	}
	fence := strings.Repeat("`", longest+1)
	b.WriteString(fence + "text\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if dropped > 0 {
		fmt.Fprintf(b, "[... Viff truncated this section at the %d-line / %d KB block limit; %d %s not shown ...]\n",
			maxContextBlockLines, maxContextBlockBytes>>10, dropped, plural(dropped, "line", "lines"))
	}
	b.WriteString(fence + "\n")
}

// contextRange labels a half-open 0-based range with 1-based line numbers.
func contextRange(start, end, total int) string {
	start, end = clampRange(start, end, total)
	switch {
	case start >= end:
		return "no lines"
	case end == start+1:
		return fmt.Sprintf("line %d", start+1)
	}
	return fmt.Sprintf("lines %d-%d", start+1, end)
}

func clampRange(start, end, total int) (int, int) {
	start = max(0, min(start, total))
	end = max(start, min(end, total))
	return start, end
}

func contextOperation(op domain.Operation) string {
	if !op.Active() {
		return "none reported by Git (unmerged entries only)"
	}
	return strings.ToLower(op.Kind.Title())
}

func contextSource(label string) string {
	if s := strings.TrimSpace(label); s != "" {
		return s
	}
	return "unknown (Git did not report a branch or commit)"
}

func contextStyle(cv *ConflictView, blocks int) string {
	if blocks == 0 {
		return ""
	}
	for _, block := range cv.Markers.Blocks {
		if block.HasBase() {
			return "diff3 or zdiff3 (base sections included)"
		}
	}
	return "merge (no base sections)"
}

func contextYesNo(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

// truncateContextText cuts text to at most limit bytes at a rune boundary.
func truncateContextText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8Start(text[limit]) {
		limit--
	}
	return text[:limit]
}

// utf8Start reports whether b starts a UTF-8 sequence.
func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
