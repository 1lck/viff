//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	vapp "viff/internal/app"
)

// clickWhenReady presses label once the rendered frame carries a hit for it.
// A slow CI runner can paint a dialog's content before its controls take part
// in hit testing; clicking on the fixed settle was a source of flakes.
func clickWhenReady(t *testing.T, f *uiFixture, label string) {
	t.Helper()
	if err := f.s.WaitFor(func(s app.Snapshot) bool {
		for _, h := range s.Hits {
			if h.Label == label {
				return true
			}
		}
		return false
	}, 20*time.Second); err != nil {
		t.Fatalf("control %q never became clickable: %v\ntexts: %s", label, err, strings.Join(f.s.Texts(), " | "))
	}
	f.click(label)
}

// waitSubmoduleDialog waits for a dialog with the given title that also shows
// every wanted text.
func waitSubmoduleDialog(t *testing.T, f *uiFixture, title string, want ...string) {
	t.Helper()
	f.win.Invalidate()
	err := f.s.WaitFor(func(app.Snapshot) bool {
		if f.win.dialog == nil || f.win.dialog.title != title {
			return false
		}
		for _, w := range want {
			if !f.s.HasText(w) {
				return false
			}
		}
		return true
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("dialog %q never showed %v: %v\ntexts: %s", title, want, err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
}

func openSubmodulesDialog(t *testing.T, f *uiFixture) {
	t.Helper()
	f.win.openSubmodules()
	f.win.Invalidate()
	err := f.s.WaitFor(func(app.Snapshot) bool {
		if f.win.dialog == nil || f.win.dialog.title != "Submodules" {
			return false
		}
		if f.s.HasText("Loading submodules…") {
			return false
		}
		return f.s.HasText("Up to date") || f.s.HasText("This repository has no submodules.")
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("submodule list never loaded: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(80*time.Millisecond, 2*time.Second)
	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
}

// TestSubmoduleAddDialog adds a submodule from the dialog: the URL is typed,
// Git clones into the derived path, the list reloads with the new row, and a
// failing clone stays in the dialog with Git's message instead of a success.
func TestSubmoduleAddDialog(t *testing.T) {
	f := newUIFixture(t)
	root := filepath.Dir(f.dir)
	child := filepath.Join(root, "child lib")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	f.gitIn(child, "init", "-q", "-b", "main")
	f.gitIn(child, "config", "user.name", "Ada Lovelace")
	f.gitIn(child, "config", "user.email", "ada@example.com")
	f.gitIn(child, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(child, "lib.go"), []byte("package lib\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.gitIn(child, "add", "-A")
	f.gitIn(child, "commit", "-q", "-m", "Add lib")
	f.ws.Git.Env = append(f.ws.Git.Env, "GIT_ALLOW_PROTOCOL=file")

	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	openSubmodulesDialog(t, f)
	f.click("Add Submodule…")
	waitSubmoduleDialog(t, f, "Add Submodule")

	f.click("URL:")
	if err := f.s.Type(child); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(80*time.Millisecond, 2*time.Second)
	f.snap("submodule-add-form")
	f.click("Add")
	waitSubmoduleDialog(t, f, "Submodules", "child lib")
	f.snap("submodule-added")

	if got := f.git("submodule", "status"); !strings.Contains(got, "child lib") {
		t.Fatalf("git does not list the new submodule: %q", got)
	}
	modules, err := os.ReadFile(filepath.Join(f.dir, ".gitmodules"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(modules), "child lib") {
		t.Fatalf(".gitmodules does not record the path:\n%s", modules)
	}

	// A URL Git cannot reach leaves the dialog open with Git's own message.
	f.ws.Git.Env = append(f.ws.Git.Env, "GIT_ALLOW_PROTOCOL=file")
	missing := filepath.Join(root, "missing repo")
	f.click("Add Submodule…")
	waitSubmoduleDialog(t, f, "Add Submodule")
	f.click("URL:")
	if err := f.s.Type(missing); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(80*time.Millisecond, 2*time.Second)
	beforeFailure := len(f.ws.Notifications)
	f.click("Add")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && f.win.dialog.title == "Add Submodule" && f.win.dialog.err != ""
	}, 10*time.Second); err != nil {
		t.Fatalf("failed add stayed silent: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	// Git's own wording differs by platform and version: a local path is
	// "does not exist" on Unix and "does not appear to be a git repository"
	// on Windows. The point is that the failure is shown and not a success.
	if msg := f.win.dialog.err; !strings.Contains(msg, "does not exist") && !strings.Contains(msg, "does not appear to be a git repository") {
		t.Fatalf("failed add message = %q", msg)
	}
	for _, n := range f.ws.Notifications[beforeFailure:] {
		if n.Kind == vapp.NoticeSuccess {
			t.Fatalf("failed add reported a success: %+v", n)
		}
	}
	if got := strings.TrimSpace(f.git("submodule", "status")); strings.Contains(got, "missing repo") {
		t.Fatalf("failed add left a submodule: %q", got)
	}

	// Escape returns to the list instead of closing both dialogs.
	f.s.Keys("escape")
	waitSubmoduleDialog(t, f, "Submodules")
}

// TestSubmoduleRemoveDialog removes a clean submodule, then shows the force
// confirmation for one with local work, then keeps a .gitmodules failure in
// front of the user without touching the checkout.
func TestSubmoduleRemoveDialog(t *testing.T) {
	f := newUIFixture(t)
	child := filepath.Join(filepath.Dir(f.dir), "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	f.gitIn(child, "init", "-q", "-b", "main")
	f.gitIn(child, "config", "user.name", "Ada Lovelace")
	f.gitIn(child, "config", "user.email", "ada@example.com")
	f.gitIn(child, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(child, "lib.go"), []byte("package lib\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.gitIn(child, "add", "-A")
	f.gitIn(child, "commit", "-q", "-m", "Add lib")
	f.git("-c", "protocol.file.allow=always", "submodule", "add", "--force", child, "deps/lib")
	f.commit("Add submodule", "2026-10-13T09:00:00")

	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	openSubmodulesDialog(t, f)
	f.ws.Git.Env = append(f.ws.Git.Env, "GIT_ALLOW_PROTOCOL=file")
	f.snap("submodule-management-actions")
	f.click("Deinit")
	waitSubmoduleDialog(t, f, "Deinit Submodule")
	f.snap("submodule-deinit-confirmation")
	f.click("Deinit")
	waitSubmoduleDialog(t, f, "Submodules", "Not initialized")
	if _, err := os.Stat(filepath.Join(f.dir, "deps/lib/lib.go")); !os.IsNotExist(err) {
		t.Fatalf("deinit kept file: %v", err)
	}
	f.snap("submodule-deinitialized")
	f.click("Update")
	waitSubmoduleDialog(t, f, "Submodules", "Up to date")
	if _, err := os.Stat(filepath.Join(f.dir, "deps/lib/lib.go")); err != nil {
		t.Fatal("update did not restore submodule:", err)
	}
	clickWhenReady(t, f, "Remove…")
	waitSubmoduleDialog(t, f, "Remove Submodule", "deps/lib")
	f.click("Remove")
	waitSubmoduleDialog(t, f, "Submodules")
	if got := strings.TrimSpace(f.git("submodule", "status")); got != "" {
		t.Fatalf("removed submodule still listed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "deps/lib")); !os.IsNotExist(err) {
		t.Fatalf("removal left the working tree: %v", err)
	}
	modules, err := os.ReadFile(filepath.Join(f.dir, ".gitmodules"))
	if err == nil && strings.Contains(string(modules), "deps/lib") {
		t.Fatalf(".gitmodules still records the removed submodule:\n%s", modules)
	}

	// Local work in the submodule turns the confirmation into the force one.
	f.commit("Remove submodule", "2026-10-13T10:00:00")
	f.git("-c", "protocol.file.allow=always", "submodule", "add", "--force", child, "deps/lib")
	f.commit("Add submodule again", "2026-10-14T09:00:00")
	if err := os.WriteFile(filepath.Join(f.dir, "deps/lib/lib.go"), []byte("package lib // local work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.ws.Refresh(true)
	f.settle()
	openSubmodulesDialog(t, f)
	clickWhenReady(t, f, "Remove…")
	waitSubmoduleDialog(t, f, "Remove Submodule", "Force Remove", "local changes")
	f.snap("submodule-force-confirmation")
	f.click("Force Remove")
	waitSubmoduleDialog(t, f, "Submodules")
	if _, err := os.Stat(filepath.Join(f.dir, "deps/lib")); !os.IsNotExist(err) {
		t.Fatalf("forced removal left the working tree: %v", err)
	}

	// A locally modified .gitmodules makes git rm refuse; the dialog shows the
	// refusal and the checkout stays as it was.
	f.commit("Force remove submodule", "2026-10-14T10:00:00")
	f.git("-c", "protocol.file.allow=always", "submodule", "add", "--force", child, "deps/lib")
	f.commit("Add submodule a third time", "2026-10-15T09:00:00")
	modulesPath := filepath.Join(f.dir, ".gitmodules")
	data, err := os.ReadFile(modulesPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modulesPath, append(data, []byte("# local note\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	f.ws.Refresh(true)
	f.settle()
	openSubmodulesDialog(t, f)
	clickWhenReady(t, f, "Remove…")
	waitSubmoduleDialog(t, f, "Remove Submodule", "deps/lib")
	beforeFailure := len(f.ws.Notifications)
	f.click("Remove")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && f.win.dialog.title == "Remove Submodule" && f.win.dialog.err != ""
	}, 10*time.Second); err != nil {
		t.Fatalf("refused removal stayed silent: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	if msg := f.win.dialog.err; !strings.Contains(msg, "please stage your changes to .gitmodules") {
		t.Fatalf("refused removal message = %q", msg)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "deps/lib/lib.go")); err != nil {
		t.Fatalf("refused removal changed the checkout: %v", err)
	}
	for _, n := range f.ws.Notifications[beforeFailure:] {
		if n.Kind == vapp.NoticeSuccess {
			t.Fatalf("refused removal reported a success: %+v", n)
		}
	}
}

// TestSubmoduleNestedHint checks that the list says where a nested submodule
// belongs instead of offering to manage it through the parent repository.
func TestSubmoduleNestedHint(t *testing.T) {
	f := newUIFixture(t)
	root := filepath.Dir(f.dir)
	leaf := filepath.Join(root, "leaf")
	mid := filepath.Join(root, "mid")
	for _, dir := range []string{leaf, mid} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f.gitIn(dir, "init", "-q", "-b", "main")
		f.gitIn(dir, "config", "user.name", "Ada Lovelace")
		f.gitIn(dir, "config", "user.email", "ada@example.com")
		f.gitIn(dir, "config", "commit.gpgsign", "false")
		if err := os.WriteFile(filepath.Join(dir, "file.go"), []byte("package file\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		f.gitIn(dir, "add", "-A")
		f.gitIn(dir, "commit", "-q", "-m", "init")
	}
	f.gitIn(mid, "-c", "protocol.file.allow=always", "submodule", "add", leaf, "leaf")
	f.gitIn(mid, "commit", "-q", "-m", "add leaf")
	f.git("-c", "protocol.file.allow=always", "submodule", "add", mid, "mid")
	f.commit("Add mid", "2026-10-13T09:00:00")
	f.git("-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")

	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()
	openSubmodulesDialog(t, f)
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.s.HasText("mid/leaf") && f.s.HasText("belongs to the repository that records it")
	}, 10*time.Second); err != nil {
		t.Fatalf("nested submodule hint missing: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	if !strings.Contains(strings.Join(f.s.Texts(), "\n"), "·  nested") {
		t.Fatalf("nested row marker missing; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
}
