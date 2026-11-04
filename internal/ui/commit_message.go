package viffui

import (
	"strings"
	"unicode/utf8"

	"github.com/vivaui/viva/ui"

	"viff/internal/domain"
)

const (
	commitMessageSubjectLineH = 20
	commitMessageBodyLineH    = 19
	// commitMessageFooterH is the room the hash/author, date and branch rows
	// need under the message, plus the breathing room the panel keeps below
	// them when scrolled to the end.
	commitMessageFooterH = 70

	// commitMessageWrapChunk bounds how much of a paragraph one WrapLines call
	// sees. WrapLines measures the whole remaining paragraph once per emitted
	// line, so wrapping a very long paragraph in one call is quadratic in its
	// length; a paragraph is wrapped in overlapping windows instead.
	commitMessageWrapChunk = 512
	// commitMessageWrapLookahead is the text a window keeps past the chunk so
	// its last kept line breaks exactly where the whole paragraph would. It
	// only has to cover one line at the widest panel.
	commitMessageWrapLookahead = 512
)

// commitMessageLayout is a commit message wrapped for one view width. The Log
// tab repaints on every scroll step, so a long message is wrapped once and
// reused until the selection, the message or the panel width changes.
type commitMessageLayout struct {
	gen     uint64
	hash    string
	subject string
	body    string
	width   float32

	subjectLines []string
	bodyLines    []string
	total        float32
}

// wrapCommitMessageParagraph wraps one paragraph (which has no newline) and
// keeps every line. Short paragraphs go straight to WrapLines; long ones are
// wrapped in overlapping windows, which keeps the full typography while the
// work stays near-linear in the paragraph length.
func (w *Window) wrapCommitMessageParagraph(text string, weight, width float32) []string {
	if text == "" || width <= 0 || w.c == nil {
		return nil
	}
	if len(text) <= commitMessageWrapChunk {
		return w.c.WrapLines(nil, text, fontUI, weight, width, utf8.RuneCountInString(text)+1)
	}
	lines := make([]string, 0, len(text)/24)
	rest := text
	for len(rest) > commitMessageWrapChunk {
		end := min(len(rest), commitMessageWrapChunk+commitMessageWrapLookahead)
		wrapped := w.c.WrapLines(nil, rest[:end], fontUI, weight, width, utf8.RuneCountInString(rest[:end])+1)
		// Keep the lines that lie wholly inside the chunk; the line crossing
		// the chunk boundary may have been cut by the window and is wrapped
		// again from its own start in the next window.
		cursor, kept := 0, 0
		for _, line := range wrapped {
			if line == "" {
				continue
			}
			pos := strings.Index(rest[cursor:], line)
			if pos < 0 {
				break
			}
			start, stop := cursor+pos, cursor+pos+len(line)
			if start >= commitMessageWrapChunk || stop > commitMessageWrapChunk {
				break
			}
			lines = append(lines, line)
			cursor, kept = stop, stop
		}
		if kept == 0 {
			// No complete line in this window (one word wider than the
			// window): let WrapLines deal with the rest in one call.
			return append(lines, w.c.WrapLines(nil, rest, fontUI, weight, width, utf8.RuneCountInString(rest)+1)...)
		}
		// WrapLines trims the whitespace a line break falls on; a window that
		// starts mid-paragraph sees that whitespace as text, so drop it too.
		rest = strings.TrimLeft(rest[kept:], " ")
	}
	return append(lines, w.c.WrapLines(nil, rest, fontUI, weight, width, utf8.RuneCountInString(rest)+1)...)
}

// commitMessageBody is the message under the subject. Git stores the subject as
// the message's first paragraph, so drop that duplicate: the heading shows it.
func commitMessageBody(d *domain.CommitDetail) string {
	body := strings.TrimSpace(d.Body)
	if strings.HasPrefix(body, d.Subject) {
		body = strings.TrimSpace(body[len(d.Subject):])
	}
	return body
}

// commitMessageLayoutFor returns the layout for d at the given panel width,
// building it when the commit, the message or the width changed.
func (w *Window) commitMessageLayoutFor(d *domain.CommitDetail, width float32) *commitMessageLayout {
	v := &w.lg
	body := commitMessageBody(d)
	gen := uint64(0)
	if rs := w.repo(); rs != nil {
		gen = rs.Gen
	}
	if l := v.msgLayout; l != nil && l.gen == gen && l.hash == d.Hash && l.width == width && l.subject == d.Subject && l.body == body {
		return l
	}
	if v.msgLayout != nil && (v.msgLayout.gen != gen || v.msgLayout.hash != d.Hash) {
		// A new commit (or repository) starts at its own top, not where the
		// previous message was left scrolled to.
		v.msgScroll.Y = 0
	}
	l := &commitMessageLayout{gen: gen, hash: d.Hash, subject: d.Subject, body: body, width: width}
	l.subjectLines = w.wrapCommitMessageParagraph(d.Subject, semibold, width)
	if body != "" {
		for _, para := range strings.Split(body, "\n") {
			if para == "" {
				l.bodyLines = append(l.bodyLines, "")
				continue
			}
			l.bodyLines = append(l.bodyLines, w.wrapCommitMessageParagraph(para, regular, width)...)
		}
	}
	l.total = float32(len(l.subjectLines))*commitMessageSubjectLineH + 8 +
		float32(len(l.bodyLines))*commitMessageBodyLineH + commitMessageFooterH
	v.msgLayout = l
	return l
}

// paintMessageRows paints the wrapped lines that intersect r, keeping drawing
// cost proportional to the visible rows however long the message is.
func (w *Window) paintMessageRows(lines []string, r ui.Rect, top, lineH, weight float32) {
	if len(lines) == 0 {
		return
	}
	first := int((r.Y - top) / lineH)
	if first < 0 {
		first = 0
	}
	last := int((bottom(r)-top)/lineH) + 1
	if last > len(lines) {
		last = len(lines)
	}
	if last < first {
		last = first
	}
	for i := first; i < last; i++ {
		w.p.text(lines[i], rect(r.X, top+float32(i)*lineH, r.Width, lineH), fontUI, weight, w.pal.Text)
	}
}

func (w *Window) paintLogCommitMessage(msg ui.Rect, d *domain.CommitDetail) {
	rs := w.repo()
	v := &w.lg
	// Message.
	mr := inset(msg, 14, 10)
	layout := w.commitMessageLayoutFor(d, mr.Width)
	v.msgScroll.clamp(layout.total, mr.Height)
	w.onScrollIn(msg, func(dx, dy float32) {
		v.msgScroll.Y += dy
		v.msgScroll.clamp(layout.total, mr.Height)
	})
	w.c.Clip(msg)
	yy := mr.Y - v.msgScroll.Y
	w.paintMessageRows(layout.subjectLines, mr, yy, commitMessageSubjectLineH, semibold)
	yy += float32(len(layout.subjectLines))*commitMessageSubjectLineH + 8
	w.paintMessageRows(layout.bodyLines, mr, yy, commitMessageBodyLineH, regular)
	yy += float32(len(layout.bodyLines)) * commitMessageBodyLineH
	if len(layout.bodyLines) > 0 {
		yy += 8
	}
	meta := domain.ShortHash(d.Hash) + "  " + d.Author + " <" + d.AuthorEmail + ">"
	w.p.text(meta, rect(mr.X, yy, mr.Width, 19), fontSmall, regular, w.pal.TextMuted)
	yy += 19
	when := "on " + d.AuthorTime.Format("Jan 2, 2006 at 15:04")
	if d.Committer != d.Author {
		when += " · committed by " + d.Committer
	}
	w.p.text(when, rect(mr.X, yy, mr.Width, 19), fontSmall, regular, w.pal.TextMuted)
	yy += 19
	if br := rs.DetailBranches; len(br) > 0 {
		s := "In " + plural(len(br), "branch", "branches") + ": " + strings.Join(br, ", ")
		w.p.text(s, rect(mr.X, yy, mr.Width, 19), fontSmall, regular, w.pal.TextSecond)
	}
	w.c.Unclip()
}
