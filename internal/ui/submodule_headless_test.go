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

// TestSubmodulesDialogAndMenu covers the two entry points: the changes list
// menu for a submodule entry and the Submodules dialog wired to the Git
// tool window's options menu.
func TestSubmodulesDialogAndMenu(t *testing.T) {
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
	f.ws.Git.Env = append(f.ws.Git.Env, "GIT_ALLOW_PROTOCOL=file")
	f.git("-c", "protocol.file.allow=always", "submodule", "add", child, "deps/lib")

	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	// The staged submodule entry offers update and open.
	var key string
	for _, row := range f.win.cm.rows {
		if row.kind == rowFile && row.entry.Path == "deps/lib" && row.entry.IsSubmodule() {
			key = row.key
		}
	}
	if key == "" {
		t.Fatalf("no submodule row in %+v", f.win.cm.rows)
	}
	f.win.cm.sel = map[string]bool{key: true}
	titles := map[string]bool{}
	for _, it := range f.win.changesMenu() {
		titles[it.title] = true
	}
	if !titles["Update Submodule"] || !titles["Open Submodule"] {
		t.Fatalf("submodule menu titles = %v", titles)
	}
	f.win.cm.sel = map[string]bool{}

	// The dialog lists the submodule with its state and actions.
	f.win.openSubmodules()
	f.win.Invalidate()
	err := f.s.WaitFor(func(app.Snapshot) bool {
		return strings.Contains(strings.Join(f.s.Texts(), "\n"), "deps/lib")
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("submodule dialog never listed deps/lib: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(60*time.Millisecond, 2*time.Second)
	texts := strings.Join(f.s.Texts(), "\n")
	for _, want := range []string{"Up to date", "Update All", "Sync All"} {
		if !strings.Contains(texts, want) {
			t.Fatalf("dialog is missing %q:\ntexts: %s", want, texts)
		}
	}

	f.snap("submodules-status")

	// Sync All runs and the dialog reloads instead of closing.
	f.click("Sync All")
	f.settle()
	if f.win.dialog == nil || f.win.dialog.title != "Submodules" {
		t.Fatalf("dialog closed after sync: %+v", f.win.dialog)
	}
	for _, n := range f.ws.Notifications {
		if n.Kind == vapp.NoticeError {
			t.Fatalf("sync failed: %+v", n)
		}
	}
	f.s.Keys("escape")
	if f.win.dialog != nil {
		t.Fatal("escape did not close the dialog")
	}
}
