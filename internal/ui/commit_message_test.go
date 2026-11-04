//go:build vivahead

package viffui

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/ui"

	"viff/internal/domain"
)

// cmPaint repaints the window and returns every text it painted.
func cmPaint(f *uiFixture) []string {
	f.t.Helper()
	f.win.Invalidate()
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		f.t.Fatalf("settle: %v", err)
	}
	return f.s.Texts()
}

// cmLayout returns the commit-message layout the last paint cached.
func cmLayout(f *uiFixture) *commitMessageLayout {
	f.t.Helper()
	l := f.win.lg.msgLayout
	if l == nil {
		f.t.Fatal("no commit-message layout was cached")
	}
	return l
}

func cmTextsContain(texts []string, substring string) bool {
	for _, s := range texts {
		if strings.Contains(s, substring) {
			return true
		}
	}
	return false
}

// cmCommit writes the message to Git's standard input instead of its command
// line. Windows caps a command line at about 32k, and the very-long-body case
// is far past it; Git itself gets the message the same way the product commits.
func cmCommit(f *uiFixture, subject, body string) {
	f.t.Helper()
	message := subject
	if body != "" {
		message += "\n\n" + body
	}
	cmd := exec.Command("git", "commit", "--allow-empty", "-F", "-")
	cmd.Dir = f.dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	cmd.Stdin = strings.NewReader(message)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("commit: %v\n%s", err, out)
	}
}

// cmOpen adds one commit and selects it in the Log tab.
func cmOpen(t *testing.T, subject, body string) *uiFixture {
	t.Helper()
	f := newUIFixture(t)
	cmCommit(f, subject, body)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()
	f.click(subject)
	f.settle()
	if d := f.ws.Repo.Detail; d == nil || d.Hash != f.ws.Repo.DetailHash {
		t.Fatal("the commit was not selected")
	}
	return f
}

// cmScrollToLineAlt scrolls the message so the first body line containing the
// marker sits at the top of the panel.
func cmScrollToBodyMarker(f *uiFixture, layout *commitMessageLayout, marker string) {
	f.t.Helper()
	for i, line := range layout.bodyLines {
		if strings.Contains(line, marker) {
			f.win.lg.msgScroll.Y = float32(len(layout.subjectLines))*commitMessageSubjectLineH + 8 +
				float32(i)*commitMessageBodyLineH - 2
			return
		}
	}
	f.t.Fatalf("marker %q is not in the wrapped body", marker)
}

// A long single-paragraph message must be reachable in full: the old detail
// panel ellipsized every body paragraph after 20 lines, so the tail could never
// be shown no matter where the user scrolled.
func TestCommitMessageLongBodyShowsTail(t *testing.T) {
	head := "LONG_BODY_HEAD_501"
	tail := "LONG_BODY_TAIL_771"
	body := head + " " + strings.Repeat("lorem ipsum dolor sit amet ", 400) + tail
	f := cmOpen(t, "Long body commit", body)
	detail := f.ws.Repo.Detail
	if !strings.Contains(detail.Body, tail) {
		t.Fatal("git truncated the message before the UI")
	}

	// The message starts at the top: head visible, tail far below the fold.
	texts := cmPaint(f)
	if f.win.lg.msgScroll.Y != 0 {
		t.Errorf("message does not start at the top: scroll=%v", f.win.lg.msgScroll.Y)
	}
	if !cmTextsContain(texts, head) {
		t.Error("top of a long message is not painted")
	}
	if cmTextsContain(texts, tail) {
		t.Error("tail is painted while at the top; rows are not virtualized or the message fits")
	}

	// Scrolled to the end the tail and the commit footer must be readable.
	f.win.lg.msgScroll.Y = 1 << 20
	texts = cmPaint(f)
	if f.win.lg.msgScroll.Y <= 0 {
		t.Fatalf("long message has no scroll range: scroll=%v", f.win.lg.msgScroll.Y)
	}
	if !cmTextsContain(texts, tail) {
		t.Error("tail of a long message is not readable at the bottom")
	}
	if !cmTextsContain(texts, domain.ShortHash(detail.Hash)) {
		t.Error("the commit footer under the message is not readable at the bottom")
	}
	if cmTextsContain(texts, head) {
		t.Error("rows far above the viewport are still painted; scrolling is not virtualized")
	}

	layout := cmLayout(f)
	if len(layout.bodyLines) <= 20 {
		t.Errorf("body is still capped at 20 lines: got %d", len(layout.bodyLines))
	}
	for _, line := range layout.bodyLines {
		if strings.HasSuffix(line, "…") {
			t.Fatalf("body line was ellipsized instead of wrapped: %q", line)
		}
	}

	// A repaint of the same commit reuses the wrapped layout instead of
	// rewrapping the whole message on every scroll step.
	if again := cmLayout(cmRepaint(f)); again != layout {
		t.Error("the message was rewrapped for an unchanged commit and width")
	}
}

// cmRepaint repaints without touching any state and returns the fixture.
func cmRepaint(f *uiFixture) *uiFixture {
	f.t.Helper()
	f.win.Invalidate()
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		f.t.Fatalf("settle: %v", err)
	}
	return f
}

// A subject long enough to need more than four lines used to be cut with an
// ellipsis. The detail heading now shows all of it.
func TestCommitMessageLongSubjectNotTruncated(t *testing.T) {
	subject := "Subject start " + strings.Repeat("words that keep going ", 40) + "SUBJECT_TAIL_773"
	f := cmOpen(t, subject, "")
	if !strings.Contains(f.ws.Repo.Detail.Subject, "SUBJECT_TAIL_773") {
		t.Fatal("git truncated the subject before the UI")
	}
	layout := cmLayout(f)
	if len(layout.subjectLines) <= 4 {
		t.Fatalf("subject is still capped at 4 lines: got %d", len(layout.subjectLines))
	}
	for _, line := range layout.subjectLines {
		if strings.HasSuffix(line, "…") {
			t.Fatalf("subject line was ellipsized instead of wrapped: %q", line)
		}
	}
	f.win.lg.msgScroll.Y = 1 << 20
	texts := cmPaint(f)
	if !cmTextsContain(texts, "SUBJECT_TAIL_773") {
		t.Error("tail of a long subject is not readable at the bottom")
	}
}

// Chinese text wraps at any character, so a long paragraph must stay inside the
// panel width and remain fully readable.
func TestCommitMessageLongCJKParagraph(t *testing.T) {
	// CJK may break between any two characters, so leading spaces keep these
	// mixed-script sentinels whole.
	head := "中文开头标记 HEAD_780"
	tail := "中文尾部标记 TAIL_778"
	body := head + " " + strings.Repeat("这是一段很长的中文提交说明，用于验证自动换行不会截断内容。", 120) + " " + tail
	f := cmOpen(t, "中文长提交说明", body)

	layout := cmLayout(f)
	if len(layout.bodyLines) <= 20 {
		t.Fatalf("CJK body is still capped: got %d lines", len(layout.bodyLines))
	}
	for _, line := range layout.bodyLines {
		if strings.HasSuffix(line, "…") {
			t.Fatalf("CJK body line was ellipsized: %q", line)
		}
		if got := f.win.p.width(line, fontUI, regular); got > layout.width+0.5 {
			t.Fatalf("CJK line %q is %.1fpx wide, panel is %.1fpx", line, got, layout.width)
		}
	}

	texts := cmPaint(f)
	if !cmTextsContain(texts, "HEAD_780") {
		t.Error("top of the CJK message is not painted")
	}
	f.win.lg.msgScroll.Y = 1 << 20
	texts = cmPaint(f)
	if !cmTextsContain(texts, "TAIL_778") {
		t.Error("tail of the CJK message is not readable at the bottom")
	}
}

// Several paragraphs, blank lines and single newlines are kept, and every
// paragraph can be scrolled to.
func TestCommitMessageMultipleParagraphs(t *testing.T) {
	body := "PARA_ONE_781 " + strings.Repeat("first paragraph words ", 60) +
		"\n\n" + strings.Repeat("second paragraph words ", 240) + "PARA_TWO_TAIL_782" +
		"\n\nPARA_THREE_783"
	f := cmOpen(t, "Paragraph commit", body)
	layout := cmLayout(f)

	empty := 0
	for _, line := range layout.bodyLines {
		if line == "" {
			empty++
		}
	}
	if empty < 2 {
		t.Errorf("blank lines between paragraphs were dropped: %d empty lines", empty)
	}

	texts := cmPaint(f)
	if !cmTextsContain(texts, "PARA_ONE_781") {
		t.Error("first paragraph is not painted at the top")
	}
	if cmTextsContain(texts, "PARA_TWO_TAIL_782") || cmTextsContain(texts, "PARA_THREE_783") {
		t.Error("later paragraphs are painted while at the top")
	}

	cmScrollToBodyMarker(f, layout, "PARA_TWO_TAIL_782")
	texts = cmPaint(f)
	if !cmTextsContain(texts, "PARA_TWO_TAIL_782") {
		t.Error("the end of the second paragraph cannot be scrolled to")
	}
	if cmTextsContain(texts, "PARA_ONE_781") {
		t.Error("the first paragraph is still painted while scrolled to the middle")
	}

	f.win.lg.msgScroll.Y = 1 << 20
	texts = cmPaint(f)
	if !cmTextsContain(texts, "PARA_THREE_783") {
		t.Error("the last paragraph is not readable at the bottom")
	}
}

// A narrow window wraps much more aggressively; the whole message still has to
// be reachable and every line must fit the panel.
func TestCommitMessageNarrowWindow(t *testing.T) {
	head := "NARROW_HEAD_790"
	tail := "NARROW_TAIL_791"
	body := head + " " + strings.Repeat("narrow window words ", 300) + tail
	f := newUIFixture(t)
	f.git("commit", "--allow-empty", "-m", "Narrow window commit", "-m", body)
	f.start(560, 640)
	f.win.openRepo(f.dir)
	f.settle()
	f.click("Narrow window commit")
	f.settle()

	layout := cmLayout(f)
	if layout.width <= 0 || layout.width >= 400 {
		t.Fatalf("unexpected message panel width at a narrow window: %v", layout.width)
	}
	if len(layout.bodyLines) <= 20 {
		t.Fatalf("narrow body is still capped: got %d lines", len(layout.bodyLines))
	}
	texts := cmPaint(f)
	if !cmTextsContain(texts, head) {
		t.Error("top of the message is not painted in a narrow window")
	}
	f.win.lg.msgScroll.Y = 1 << 20
	texts = cmPaint(f)
	if !cmTextsContain(texts, tail) {
		t.Error("tail of the message is not readable in a narrow window")
	}
}

// Selecting another commit must forget the previous scroll offset: the old code
// kept msgScroll, so a long commit could leave the next one blank or at its end.
func TestCommitMessageScrollResetsOnSelectionChange(t *testing.T) {
	f := newUIFixture(t)
	firstHead := "FIRST_HEAD_795"
	firstTail := "FIRST_TAIL_796"
	secondHead := "SECOND_HEAD_797"
	secondTail := "SECOND_TAIL_798"
	f.git("commit", "--allow-empty", "-m", "First long message", "-m",
		firstHead+" "+strings.Repeat("first body words ", 300)+firstTail)
	f.git("commit", "--allow-empty", "-m", "Second long message", "-m",
		secondHead+" "+strings.Repeat("second body words ", 300)+secondTail)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	f.click("First long message")
	f.settle()
	f.win.lg.msgScroll.Y = 1 << 20
	cmPaint(f)
	if f.win.lg.msgScroll.Y <= 0 {
		t.Fatal("first commit did not scroll")
	}
	if !cmTextsContain(f.s.Texts(), firstTail) {
		t.Fatal("tail of the first commit is not readable at the bottom")
	}

	f.click("Second long message")
	f.settle()
	if texts := cmPaint(f); !cmTextsContain(texts, secondHead) {
		t.Error("a new selection does not start at the top of its message")
	}
	if f.win.lg.msgScroll.Y != 0 {
		t.Errorf("scroll offset leaked across selections: %v", f.win.lg.msgScroll.Y)
	}
	if cmTextsContain(f.s.Texts(), secondTail) {
		t.Error("a new selection is still painted at the previous bottom position")
	}
	if got := cmLayout(f).hash; got != f.ws.Repo.Detail.Hash {
		t.Errorf("cached layout belongs to %s, want %s", domain.ShortHash(got), domain.ShortHash(f.ws.Repo.Detail.Hash))
	}
}

// Wheel scrolling over the message box must keep working: the panel registers
// its own scroll region on the message area.
func TestCommitMessageWheelScrolls(t *testing.T) {
	body := "WHEEL_HEAD_799 " + strings.Repeat("wheel scroll body ", 300) + "WHEEL_TAIL_799"
	f := cmOpen(t, "Wheel scroll commit", body)
	cmPaint(f)
	// The message box is the lowest, right-most scroll region on screen: the
	// details pane ends at the bottom of the Git panel.
	var msg ui.Rect
	for _, region := range f.win.scrolls {
		if bottom(region.r) > bottom(msg) || (bottom(region.r) == bottom(msg) && region.r.X > msg.X) {
			msg = region.r
		}
	}
	if msg.Height < 60 || msg.Height > 400 || msg.Width <= 0 {
		t.Fatalf("message scroll region looks wrong: %+v", msg)
	}
	if f.win.lg.msgScroll.Y != 0 {
		t.Fatalf("message starts scrolled: %v", f.win.lg.msgScroll.Y)
	}
	if err := f.s.Scroll(ui.ScrollEvent{X: msg.X + msg.Width/2, Y: msg.Y + msg.Height/2, DeltaY: 120}); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if f.win.lg.msgScroll.Y == 0 {
		t.Error("wheel over the message box does not scroll the message")
	}
}

// A very large message must remain reachable and must not be rewrapped on
// every repaint.
func TestCommitMessageVeryLongBody(t *testing.T) {
	head := "HUGE_HEAD_802"
	tail := "HUGE_TAIL_803"
	body := head + " " + strings.Repeat("performance matters for long commit messages ", 5000) + tail
	f := cmOpen(t, "Huge commit message", body)
	layout := cmLayout(f)
	if len(layout.bodyLines) < 1000 {
		t.Fatalf("very long body wrapped into only %d lines", len(layout.bodyLines))
	}
	f.win.lg.msgScroll.Y = 1 << 20
	texts := cmPaint(f)
	if !cmTextsContain(texts, tail) {
		t.Error("tail of a very long message is not readable at the bottom")
	}
	if cmTextsContain(texts, head) {
		t.Error("a very long message paints off-screen rows")
	}
	if again := cmLayout(cmRepaint(f)); again != layout {
		t.Error("a very long message was rewrapped for an unchanged commit and width")
	}
}

// Wrapping a long paragraph in overlapping windows must produce exactly the
// lines one WrapLines call would: the panel must not reflow text differently
// once a paragraph grows past a window.
func TestCommitMessageLongParagraphWrapMatchesDirect(t *testing.T) {
	f := cmOpen(t, "Wrap parity commit", "x")
	cases := []struct {
		name string
		text string
	}{
		{"english", "head " + strings.Repeat("performance matters for long commit messages ", 80) + "tail"},
		{"double-spaces", "head  " + strings.Repeat("a  b   c ", 200) + " tail"},
		{"cjk", "头 " + strings.Repeat("这是一段很长的中文提交说明，用于验证自动换行不会截断内容。", 50) + " 尾"},
		{"punctuation", strings.Repeat("word, word. word! word? (word) [word] word; ", 100)},
		{"path-tokens", strings.Repeat("/very/long/path/name-with-dashes_and_underscores.go ", 80)},
		{"tabs", "head\t" + strings.Repeat("alpha\tbeta ", 120)},
	}
	for _, c := range cases {
		fast := f.win.wrapCommitMessageParagraph(c.text, regular, 360)
		direct := f.win.c.WrapLines(nil, c.text, fontUI, regular, 360, len([]rune(c.text))+1)
		if len(fast) != len(direct) {
			t.Errorf("%s: line count fast=%d direct=%d", c.name, len(fast), len(direct))
			continue
		}
		for i := range fast {
			if fast[i] != direct[i] {
				t.Errorf("%s: line %d differs\n fast:   %q\n direct: %q", c.name, i, fast[i], direct[i])
				break
			}
		}
	}
}
