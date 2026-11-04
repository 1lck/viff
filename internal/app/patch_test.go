package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
)

func readAppFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestWorkspaceCreateAndApplyPatch covers the round trip the menus drive:
// build a patch from the changes, discard them, apply the patch back.
func TestWorkspaceCreateAndApplyPatch(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("base")
	f.open()

	f.write("a.txt", "two\n")
	f.sync()

	var patch []byte
	var patchErr error
	created := false
	f.w.CreatePatch([]string{"a.txt"}, false, func(b []byte, err error) {
		patch, patchErr, created = b, err, true
	})
	f.settle(func() bool { return created })
	if patchErr != nil {
		t.Fatal(patchErr)
	}
	if !bytes.Contains(patch, []byte("+two")) {
		t.Fatalf("patch = %q", patch)
	}

	f.w.Rollback([]string{"a.txt"}, nil, false)
	f.settle(f.idle)
	if got := readAppFile(t, f.dir, "a.txt"); got != "one\n" {
		t.Fatalf("after rollback = %q", got)
	}

	file := filepath.Join(t.TempDir(), "change.patch")
	if err := os.WriteFile(file, patch, 0o644); err != nil {
		t.Fatal(err)
	}
	var applyErr error
	applied := false
	f.w.ApplyPatchFile(file, false, func(err error) { applyErr, applied = err, true })
	f.settle(func() bool { return applied })
	if applyErr != nil {
		t.Fatal(applyErr)
	}
	f.settle(f.idle)
	if got := readAppFile(t, f.dir, "a.txt"); got != "two\n" {
		t.Fatalf("after apply = %q", got)
	}
	if n := len(f.w.Repo.Entries(domain.AreaUnstaged)); n != 1 {
		t.Fatalf("unstaged entries = %d, want 1", n)
	}
}

// TestWorkspaceApplyPatchToIndex keeps the working tree when the patch goes to
// the index only.
func TestWorkspaceApplyPatchToIndex(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("base")
	f.open()

	f.write("a.txt", "two\n")
	f.git("add", "a.txt")
	f.sync()

	var patch []byte
	created := false
	f.w.CreatePatch([]string{"a.txt"}, true, func(b []byte, err error) {
		patch, created = b, true
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle(func() bool { return created })

	f.w.Rollback([]string{"a.txt"}, nil, true)
	f.settle(f.idle)
	if got := readAppFile(t, f.dir, "a.txt"); got != "one\n" {
		t.Fatalf("after rollback = %q", got)
	}

	file := filepath.Join(t.TempDir(), "staged.patch")
	if err := os.WriteFile(file, patch, 0o644); err != nil {
		t.Fatal(err)
	}
	applied := false
	f.w.ApplyPatchFile(file, true, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
		applied = true
	})
	f.settle(func() bool { return applied })
	f.settle(f.idle)

	if got := readAppFile(t, f.dir, "a.txt"); got != "one\n" {
		t.Fatalf("working tree changed: %q", got)
	}
	staged := f.w.Repo.Entries(domain.AreaStaged)
	if len(staged) != 1 || staged[0].Path != "a.txt" {
		t.Fatalf("staged = %+v", staged)
	}
}

// TestApplyPatchFailureKeepsWorkspace applies a patch built for other content:
// the failure keeps Git's message and the file on disk.
func TestApplyPatchFailureKeepsWorkspace(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("base")
	f.open()

	f.write("a.txt", "local edit\n")
	f.sync()

	stale := "diff --git a/a.txt b/a.txt\n" +
		"--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-missing line\n+patched\n"
	file := filepath.Join(t.TempDir(), "stale.patch")
	if err := os.WriteFile(file, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	var applyErr error
	done := false
	f.w.ApplyPatchFile(file, false, func(err error) { applyErr, done = err, true })
	f.settle(func() bool { return done })
	if applyErr == nil {
		t.Fatal("a patch for other content applied")
	}
	f.settle(f.idle)
	if got := readAppFile(t, f.dir, "a.txt"); got != "local edit\n" {
		t.Fatalf("failed apply changed the workspace: %q", got)
	}
	n := f.w.Notifications[len(f.w.Notifications)-1]
	if n.Kind != NoticeError || n.Title != "Cannot apply patch" {
		t.Fatalf("notification = %+v", n)
	}
	if !strings.Contains(n.Body, "patch failed") && !strings.Contains(n.Body, "does not apply") {
		t.Fatalf("notification body loses Git's message: %q", n.Body)
	}
}
