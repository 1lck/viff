//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	vapp "viff/internal/app"
	"viff/internal/i18n"
)

// TestCreateAndApplyPatchFromMenus drives the feature the way a user does:
// right-click a change, save a patch, revert the change, then apply the patch
// back through the Git tool window menu.
func TestCreateAndApplyPatchFromMenus(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	patched := string(f.read("main.go"))
	dest := filepath.Join(t.TempDir(), "viff-changes.patch")
	if err := f.s.Driver().QueueSaveFile(dest, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RightClickLabel("main.go"); err != nil {
		t.Fatalf("right click main.go: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if err := f.s.ClickLabel("Create Patch…"); err != nil {
		t.Fatalf("click Create Patch…: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.settle()

	req, ok := f.s.Driver().LastSaveFileRequest()
	if !ok || req.DefaultExtension != "patch" || !strings.HasSuffix(req.SuggestedName, "-changes.patch") {
		t.Fatalf("save request = %+v", req)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("patch not written: %v", err)
	}
	if !strings.Contains(string(data), "diff --git a/main.go b/main.go") {
		t.Fatalf("patch = %q", data)
	}
	last := f.ws.Notifications[len(f.ws.Notifications)-1]
	if last.Kind != vapp.NoticeSuccess || last.Title != "Patch created" {
		t.Fatalf("notification = %+v", last)
	}
	// The reveal action names the platform's own file manager, so the expected
	// title comes from the message table instead of a literal.
	if want := i18n.New(i18n.EN).T(i18n.ShowInFileManager); len(last.Actions) != 1 || last.Actions[0].Title != want {
		t.Fatalf("notification actions = %+v, want one %q action", last.Actions, want)
	}

	// Discard the change, then apply the saved patch through the window menu.
	f.git("checkout", "--", "main.go")
	f.ws.Refresh(true)
	f.settle()
	if got := string(f.read("main.go")); got == patched {
		t.Fatal("main.go was not reverted")
	}

	// Balloons sit over the bottom-right corner; close them so they cannot
	// cover the Git tool window header.
	for _, n := range append([]vapp.Notification(nil), f.ws.Notifications...) {
		f.ws.Dismiss(n.ID)
	}
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)

	if err := f.s.Driver().QueueOpenFiles([]string{dest}, nil); err != nil {
		t.Fatal(err)
	}
	f.click("Git Actions")
	if err := f.s.ClickLabel("Apply Patch…"); err != nil {
		t.Fatalf("click Apply Patch…: %v\nhits: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.settle()

	reqOpen, ok := f.s.Driver().LastOpenFilesRequest()
	if !ok || len(reqOpen.Filters) == 0 {
		t.Fatalf("open request = %+v", reqOpen)
	}
	if got := string(f.read("main.go")); got != patched {
		t.Fatalf("applied main.go = %q, want %q", got, patched)
	}
	last = f.ws.Notifications[len(f.ws.Notifications)-1]
	if last.Kind != vapp.NoticeSuccess || last.Title != "Patch applied" {
		t.Fatalf("apply notification = %+v", last)
	}
}

// TestApplyPatchFailureShowsGitMessage applies a patch for other content and
// checks the window reports Git's message and leaves the file alone.
func TestApplyPatchFailureShowsGitMessage(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	before := string(f.read("main.go"))
	stale := filepath.Join(t.TempDir(), "stale.patch")
	body := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-missing\n+patched\n"
	if err := os.WriteFile(stale, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Driver().QueueOpenFiles([]string{stale}, nil); err != nil {
		t.Fatal(err)
	}
	for _, n := range append([]vapp.Notification(nil), f.ws.Notifications...) {
		f.ws.Dismiss(n.ID)
	}
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)

	f.click("Git Actions")
	if err := f.s.ClickLabel("Apply Patch…"); err != nil {
		t.Fatalf("click Apply Patch…: %v", err)
	}
	f.settle()

	if got := string(f.read("main.go")); got != before {
		t.Fatalf("failed apply changed main.go")
	}
	last := f.ws.Notifications[len(f.ws.Notifications)-1]
	if last.Kind != vapp.NoticeError || last.Title != "Cannot apply patch" {
		t.Fatalf("notification = %+v", last)
	}
	if !strings.Contains(last.Body, "patch failed") && !strings.Contains(last.Body, "does not apply") {
		t.Fatalf("notification body = %q", last.Body)
	}
}

func (f *uiFixture) read(name string) []byte {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(name)))
	if err != nil {
		f.t.Fatal(err)
	}
	return data
}
