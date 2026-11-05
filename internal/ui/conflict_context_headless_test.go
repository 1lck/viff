//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
)

// copySeam installs a clipboard recorder on the window. The test never reads
// or writes the real clipboard; the seam is the same one the application fills
// with Viva's clipboard.
func copySeam(f *uiFixture) *[]string {
	copied := &[]string{}
	f.win.clipboard = func(s string) error {
		*copied = append(*copied, s)
		return nil
	}
	return copied
}

// TestConflictViewerCopiesContextForAI drives the whole copy path on a real
// merge: the button exists, copies the structured context, and tells the user
// what happens next. The block action copies only the selected block.
func TestConflictViewerCopiesContextForAI(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	copied := copySeam(f)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()

	f.click("conflict.txt")
	f.settleTab()

	all, ok := f.s.Hit("Copy for AI")
	if !ok {
		t.Fatalf("Copy for AI is missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	if all.Role != ui.RoleButton || all.Disabled {
		t.Fatalf("Copy for AI hit = %+v, want an enabled button", all)
	}
	if _, ok := f.s.Hit("Copy This Block"); !ok {
		t.Fatalf("Copy This Block is missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}

	f.click("Copy for AI")
	if len(*copied) != 1 {
		t.Fatalf("copied %d texts, want 1", len(*copied))
	}
	text := (*copied)[0]
	for _, want := range []string{
		"- Repository: " + f.dir,
		"- File: conflict.txt\n",
		"- Operation: merge\n",
		"- Ours (stage 2): main (HEAD)\n",
		"- Theirs (stage 3): other",
		"- Conflict blocks: 1\n",
		"## Instructions",
		"Remove every conflict marker",
		"## Conflict block 1 of 1",
		"`<<<<<<<` ours, `=======`, `>>>>>>>` theirs",
		"### ours (stage 2, line 2) - main (HEAD)",
		"### theirs (stage 3, line 4)",
		"ours",
		"theirs",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("copied context is missing %q:\n%s", want, text)
		}
	}
	if f.ws.Notifications == nil {
		t.Fatal("no notification after the copy")
	}
	last := f.ws.Notifications[len(f.ws.Notifications)-1]
	if last.Kind != vapp.NoticeInfo || last.Title != "Conflict context copied" {
		t.Fatalf("notification = %+v", last)
	}
	if last.Body != "Copied. Viff refreshes the file automatically after it is edited." {
		t.Fatalf("notification body = %q", last.Body)
	}

	f.click("Copy This Block")
	if len(*copied) != 2 {
		t.Fatalf("block copy made %d copies, want 2", len(*copied))
	}
	if !strings.Contains((*copied)[1], "includes block 1 only") {
		t.Fatalf("block copy is not limited to one block:\n%s", (*copied)[1])
	}
	f.snap("conflict-copy-for-ai")
	f.assertIdle()
}

// TestConflictViewerCopyForAIKeyboardTab reaches the button with the keyboard
// and activates it with Enter, so the action is not pointer-only.
func TestConflictViewerCopyForAIKeyboardTab(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "conflict.txt", "base\n", "ours\n", "theirs\n")
	f.start(1280, 800)
	copied := copySeam(f)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	f.click("conflict.txt")
	f.settleTab()

	hit, ok := f.s.Hit("Copy for AI")
	if !ok {
		t.Fatalf("Copy for AI is missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	focusable := false
	for i := 0; i < 40; i++ {
		if err := f.s.Keys("tab"); err != nil {
			t.Fatalf("tab: %v", err)
		}
		if f.s.Snapshot().Focused == hit.ID {
			focusable = true
			break
		}
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if !focusable {
		t.Fatalf("Copy for AI never took keyboard focus; final focus = %v", f.s.Snapshot().Focused)
	}
	if err := f.s.Keys("enter"); err != nil {
		t.Fatalf("enter: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(*copied) != 1 || !strings.Contains((*copied)[0], "Conflict block 1 of 1") {
		t.Fatalf("Enter on the focused button copied %d texts", len(*copied))
	}
}

// paintedHit finds a painted control by its accessibility label, disabled ones
// included: Viva's hit test only returns enabled targets.
func paintedHit(f *uiFixture, label string) (ui.Hit, bool) {
	hits := f.s.Snapshot().Hits
	for i := len(hits) - 1; i >= 0; i-- {
		if hits[i].Label == label {
			return hits[i], true
		}
	}
	return ui.Hit{}, false
}

// TestConflictViewerCopyForAIDisabledWithoutText checks the boundary the
// context builder cannot describe: a binary conflict has no blocks, so the
// action is shown disabled and clicking it copies nothing.
func TestConflictViewerCopyForAIDisabledWithoutText(t *testing.T) {
	f := newUIFixture(t)
	uiMergeConflict(t, f, "b.bin", "\x00base", "\x00ours", "\x00theirs")
	f.start(1280, 800)
	copied := copySeam(f)
	f.win.openRepo(f.dir)
	f.settle()
	uiWaitConflicts(t, f)
	f.settle()
	f.click("b.bin")
	f.settleTab()

	// A disabled control has no hit-test target, so read the painted region
	// from the snapshot; an enabled button would also have been reachable.
	hit, ok := paintedHit(f, "Copy for AI")
	if !ok {
		t.Fatalf("Copy for AI is missing for a binary conflict:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	if !hit.Disabled {
		t.Fatalf("Copy for AI is enabled for a binary conflict: %+v", hit)
	}
	if _, ok := f.s.Hit("Copy This Block"); ok {
		t.Fatal("Copy This Block is offered without a reliable conflict block")
	}
	if err := f.s.Click(hit.Bounds.X+hit.Bounds.Width/2, hit.Bounds.Y+hit.Bounds.Height/2); err != nil {
		t.Fatalf("click disabled button: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(*copied) != 0 {
		t.Fatalf("a disabled button copied %d texts", len(*copied))
	}
}
