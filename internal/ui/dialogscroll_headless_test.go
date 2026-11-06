//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

// joinTexts lists what the last frame painted, for a failure message.
func joinTexts(f *uiFixture) string {
	return strings.Join(f.s.Texts(), " | ")
}

// scrollDialogTo wheels the dialog body until label has a hit inside it. A
// dialog whose body fits needs nothing. Tests that click a control low in a
// long form call this first: the body clip removes the hit of anything scrolled
// out of view, so the click would otherwise fail with "no control labelled".
func scrollDialogTo(t *testing.T, f *uiFixture, label string) {
	t.Helper()
	d := f.win.dialog
	if d == nil || !d.scrollable {
		return
	}
	area, ok := f.win.areas[idDialogBar]
	if !ok || area.Height <= 0 {
		t.Fatalf("the body has no scroll area: %+v (present=%v)", area, ok)
	}
	for i := 0; i < 40; i++ {
		if hit, ok := f.s.Hit(label); ok && hit.Bounds.Y >= area.Y && hit.Bounds.Y < bottom(area) {
			return
		}
		if err := f.s.Scroll(ui.ScrollEvent{
			X: area.X + area.Width/2, Y: area.Y + area.Height/2, DeltaY: 120,
		}); err != nil {
			t.Fatalf("scroll: %v", err)
		}
		f.s.Settle(40*time.Millisecond, time.Second)
		if area, ok = f.win.areas[idDialogBar]; !ok || area.Height <= 0 {
			t.Fatalf("the body lost its scroll area: %+v", area)
		}
	}
	t.Fatalf("%q never became reachable by scrolling:\n%s", label, joinTexts(f))
}

// openTallSettings opens the settings dialog on its General page with every
// group expanded, which is the case that used to push the button row off the
// window: the form is taller than the screen there.
func openTallSettings(t *testing.T, f *uiFixture) *dialog {
	t.Helper()
	f.win.openSettings()
	f.win.Invalidate()
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 5*time.Second); err != nil {
		t.Fatalf("the settings dialog did not open: %v", err)
	}
	f.s.Settle(80*time.Millisecond, 2*time.Second)
	// The uncollapsed General page is the tall one; make sure nothing is folded
	// so the test covers the worst case rather than a lucky short page.
	if d := f.win.dialog; d != nil {
		clear(d.collapsed)
	}
	f.win.Invalidate()
	f.s.Settle(80*time.Millisecond, 2*time.Second)
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil && f.win.dialog.contentH > 0 }, 5*time.Second); err != nil {
		t.Fatalf("the settings form never measured itself: %v", err)
	}
	f.s.Settle(80*time.Millisecond, 2*time.Second)
	return f.win.dialog
}

// TestDialogBodyScrollsWhenTall covers the bug this fix is for: a settings page
// longer than the window used to grow the dialog past the screen, so OK and
// Cancel could not be clicked. The dialog is now capped, its body scrolls, and
// the buttons stay inside the window.
func TestDialogBodyScrollsWhenTall(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	d := openTallSettings(t, f)

	if !d.scrollable {
		t.Fatalf("the General page fits the window, so the test cannot cover the tall case (contentH=%.0f bodyH=%.0f)",
			d.contentH, d.bodyH)
	}
	if d.contentH <= d.bodyH {
		t.Fatalf("content %.0f is not taller than the body %.0f", d.contentH, d.bodyH)
	}
	// The button row must be reachable: both buttons have to exist in this frame.
	for _, label := range []string{d.ok, d.cancel} {
		hit, ok := f.s.Hit(label)
		if !ok {
			t.Fatalf("%q is not clickable in a tall dialog:\n%s", label, joinTexts(f))
		}
		if hit.Bounds.Y+hit.Bounds.Height > float32(f.win.size.Height) {
			t.Fatalf("%q falls outside the window: %+v", label, hit.Bounds)
		}
	}
	if _, ok := f.s.Hit("Fonts"); !ok {
		t.Fatalf("the last group is unreachable, so the body is not painted at all:\n%s", joinTexts(f))
	}
	f.assertIdle()
}

// TestDialogBodyScrollsWithWheel checks the wheel over the body moves it, is
// clamped to the content, and that a control scrolled out of view stops being a
// target (the body clip trims its hit away).
func TestDialogBodyScrollsWithWheel(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	d := openTallSettings(t, f)

	area, ok := f.win.areas[idDialogBar]
	if !ok || area.Height <= 0 {
		t.Fatalf("the body has no scroll bar: %+v (present=%v)", area, ok)
	}
	if d.scroll.Y != 0 {
		t.Fatalf("the body did not start at the top: %v", d.scroll.Y)
	}
	// The form's last group sits at the bottom of the content, so at offset
	// zero it is outside the visible body and the clip must have removed its
	// hit. This is what proves a scrolled-out control cannot be clicked.
	lastVisible := func() bool {
		hit, ok := f.s.Hit("Search")
		return ok && hit.Bounds.Y >= area.Y && hit.Bounds.Y < bottom(area)
	}
	if lastVisible() {
		t.Fatalf("the last group is visible before scrolling, so the clip is not exercised (body=%+v)", area)
	}

	scroll := func(dy float32) {
		if err := f.s.Scroll(ui.ScrollEvent{
			X: area.X + area.Width/2, Y: area.Y + area.Height/2, DeltaY: dy,
		}); err != nil {
			t.Fatalf("scroll: %v", err)
		}
		f.s.Settle(60*time.Millisecond, time.Second)
	}
	scroll(80)
	if d.scroll.Y <= 0 {
		t.Fatalf("the wheel did not move the body: %v", d.scroll.Y)
	}
	// Scrolling far past the end clamps instead of running away.
	for i := 0; i < 40; i++ {
		scroll(200)
	}
	want := max(d.contentH-d.bodyH, 0)
	if d.scroll.Y > want+0.5 {
		t.Fatalf("the body scrolled past its end: %v > %v", d.scroll.Y, want)
	}
	if !lastVisible() {
		t.Fatalf("the last group is still unreachable after scrolling to the end: %+v", f.win.areas[idDialogBar])
	}
	f.assertIdle()
}

// TestDialogFoldUsesChevron checks the disclosure header keeps its state, is
// announced as expandable, and that folding actually drops the group's rows.
func TestDialogFoldUsesChevron(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	d := openTallSettings(t, f)

	hit, ok := f.s.Hit("Fonts")
	if !ok {
		t.Fatalf("the Fonts group header is missing:\n%s", joinTexts(f))
	}
	if !hit.Expandable || !hit.Expanded {
		t.Fatalf("the group header is not announced as an expanded disclosure: %+v", hit)
	}
	// Its first row is the interface font combo, which carries the font name.
	before := d.contentH
	f.click("Fonts")
	f.s.Settle(80*time.Millisecond, time.Second)
	if d.collapsed["fonts"] != true {
		t.Fatalf("clicking the header did not fold the group: %v", d.collapsed)
	}
	f.s.Settle(80*time.Millisecond, time.Second)
	if d.contentH >= before {
		t.Fatalf("folding did not shrink the form: %.0f -> %.0f", before, d.contentH)
	}
	// Folded: the first row of that group is gone, and the header now reads as
	// collapsed.
	if _, ok := f.s.Hit("Interface font:"); ok {
		t.Fatalf("a folded group still reports its rows:\n%s", joinTexts(f))
	}
	if hit, ok := f.s.Hit("Fonts"); !ok || hit.Expanded {
		t.Fatalf("the folded header is not announced as collapsed: %+v (present=%v)", hit, ok)
	}
	f.assertIdle()
}

// TestShortDialogDoesNotScroll checks the cap only applies to tall content: a
// confirmation keeps its natural height and offers no scroll bar.
func TestShortDialogDoesNotScroll(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	want := 0
	f.win.confirm("Discard changes", "This cannot be undone.", "Discard", func() { want++ })
	f.win.Invalidate()
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog != nil }, 5*time.Second); err != nil {
		t.Fatalf("the confirmation did not open: %v", err)
	}
	f.s.Settle(80*time.Millisecond, 2*time.Second)
	d := f.win.dialog
	if d.scrollable {
		t.Fatalf("a short confirmation is scrollable (contentH=%.0f bodyH=%.0f)", d.contentH, d.bodyH)
	}
	if d.scroll.Y != 0 {
		t.Fatalf("a short confirmation has a scroll offset: %v", d.scroll.Y)
	}
	if area, drawn := f.win.areas[idDialogBar]; drawn && area.Height > 0 {
		t.Fatalf("a short confirmation drew a scroll bar: %+v", area)
	}
	// Its buttons still work, which is what the cap must not break.
	f.click("Discard")
	if want != 1 {
		t.Fatalf("the confirmation button did not run its action: %d", want)
	}
	f.assertIdle()
}

// TestSettingsTabsStayPinned covers the follow-up: the page tabs live in the
// dialog header, above the scrolling body, so they keep their place at any
// offset and remain clickable while the form under them is scrolled away.
func TestSettingsTabsStayPinned(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	d := openTallSettings(t, f)

	tabs := []string{"General", "Git", "AI"}
	before := make([]ui.Hit, len(tabs))
	for i, label := range tabs {
		hit, ok := f.s.Hit(label)
		if !ok {
			t.Fatalf("%q tab is missing:\n%s", label, joinTexts(f))
		}
		before[i] = hit
	}
	area, ok := f.win.areas[idDialogBar]
	if !ok || area.Height <= 0 {
		t.Fatalf("the body has no scroll area: %+v (present=%v)", area, ok)
	}
	// Every tab sits above the scrolling body, never inside it.
	for i, hit := range before {
		if hit.Bounds.Y+hit.Bounds.Height > area.Y+0.5 {
			t.Fatalf("%q tab overlaps the scrolling body: tab=%+v body=%+v", tabs[i], hit.Bounds, area)
		}
	}

	// Scroll the body to the end, then check the tabs did not move and can
	// still be clicked.
	for i := 0; i < 40; i++ {
		if err := f.s.Scroll(ui.ScrollEvent{
			X: area.X + area.Width/2, Y: area.Y + area.Height/2, DeltaY: 200,
		}); err != nil {
			t.Fatalf("scroll: %v", err)
		}
	}
	f.s.Settle(80*time.Millisecond, time.Second)
	if d.scroll.Y <= 0 {
		t.Fatalf("the body did not scroll, so the test proves nothing: %v", d.scroll.Y)
	}
	for i, label := range tabs {
		hit, ok := f.s.Hit(label)
		if !ok {
			t.Fatalf("%q tab is gone after scrolling:\n%s", label, joinTexts(f))
		}
		if hit.Bounds != before[i].Bounds {
			t.Fatalf("%q tab moved while the body scrolled: %+v -> %+v", label, before[i].Bounds, hit.Bounds)
		}
	}
	// Switching pages from the bottom of a long page works and resets the
	// offset, so the new page starts at its top.
	f.click("Git")
	if d.scroll.Y != 0 {
		t.Fatalf("switching pages kept the scroll offset: %v", d.scroll.Y)
	}
	if _, ok := f.s.Hit("SSH command:"); !ok {
		t.Fatalf("the Git page did not open from a scrolled page:\n%s", joinTexts(f))
	}
	f.assertIdle()
}
