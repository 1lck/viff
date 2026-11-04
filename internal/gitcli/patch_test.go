package gitcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (tr *testRepo) readFile(name string) []byte {
	tr.t.Helper()
	data, err := os.ReadFile(filepath.Join(tr.dir, filepath.FromSlash(name)))
	if err != nil {
		tr.t.Fatal(err)
	}
	return data
}

// TestCreatePatchRoundTrip writes a patch, reverts the change and applies the
// patch back; a second apply of the same patch must fail without a write.
func TestCreatePatchRoundTrip(t *testing.T) {
	tr := newTestRepo(t)
	tr.git("config", "core.autocrlf", "false")
	tr.write("a.txt", "one\ntwo\nthree\n")
	tr.commitAll("base")

	tr.write("a.txt", "one\nTWO\nthree\nfour\n")
	patch, err := tr.r.CreatePatch(ctx, tr.repo, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(patch, []byte("diff --git a/a.txt b/a.txt")) || !bytes.Contains(patch, []byte("+four")) {
		t.Fatalf("patch = %q", patch)
	}

	tr.git("checkout", "--", ".")
	if got := string(tr.readFile("a.txt")); got != "one\ntwo\nthree\n" {
		t.Fatalf("after checkout = %q", got)
	}
	if err := tr.r.ApplyPatch(ctx, tr.repo, patch, false, false); err != nil {
		t.Fatal(err)
	}
	if got := string(tr.readFile("a.txt")); got != "one\nTWO\nthree\nfour\n" {
		t.Fatalf("after apply = %q", got)
	}

	// The same patch no longer applies; the file keeps what it has.
	if err := tr.r.ApplyPatch(ctx, tr.repo, patch, false, false); err == nil {
		t.Fatal("applying a stale patch succeeded")
	}
	if got := string(tr.readFile("a.txt")); got != "one\nTWO\nthree\nfour\n" {
		t.Fatalf("stale apply changed the file: %q", got)
	}
}

// TestCreatePatchBinaryAndCRLF covers content Git treats as binary and files
// with CRLF line endings, both of which must round-trip byte for byte.
func TestCreatePatchBinaryAndCRLF(t *testing.T) {
	tr := newTestRepo(t)
	tr.git("config", "core.autocrlf", "false")
	logo := []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02, 0x00, 0xff}
	tr.write("logo.png", string(logo))
	tr.write("crlf.txt", "a\r\nb\r\n")
	tr.commitAll("base")

	changed := append([]byte(nil), logo...)
	changed[7] = 0x42
	tr.write("logo.png", string(changed))
	tr.write("crlf.txt", "a\r\nB\r\n")
	patch, err := tr.r.CreatePatch(ctx, tr.repo, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(patch, []byte("GIT binary patch")) {
		t.Fatalf("binary patch missing: %q", patch)
	}
	tr.git("checkout", "--", ".")
	if err := tr.r.ApplyPatch(ctx, tr.repo, patch, false, false); err != nil {
		t.Fatal(err)
	}
	if got := tr.readFile("logo.png"); !bytes.Equal(got, changed) {
		t.Fatalf("logo = %x, want %x", got, changed)
	}
	if got := string(tr.readFile("crlf.txt")); got != "a\r\nB\r\n" {
		t.Fatalf("crlf = %q", got)
	}
}

// TestCreatePatchPathsAndEmpty checks that a paths filter limits the patch and
// that a clean selection yields no patch at all.
func TestCreatePatchPathsAndEmpty(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "a\n")
	tr.write("b.txt", "b\n")
	tr.commitAll("base")

	tr.write("a.txt", "A\n")
	tr.write("b.txt", "B\n")
	only, err := tr.r.CreatePatch(ctx, tr.repo, []string{"b.txt"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(only, []byte("a.txt")) || !bytes.Contains(only, []byte("b.txt")) {
		t.Fatalf("filtered patch = %q", only)
	}
	tr.git("checkout", "--", ".")
	if err := tr.r.ApplyPatch(ctx, tr.repo, only, false, false); err != nil {
		t.Fatal(err)
	}
	if got := string(tr.readFile("b.txt")); got != "B\n" {
		t.Fatalf("b.txt = %q", got)
	}
	if got := string(tr.readFile("a.txt")); got != "a\n" {
		t.Fatalf("a.txt was touched: %q", got)
	}

	tr.git("checkout", "--", ".")
	empty, err := tr.r.CreatePatch(ctx, tr.repo, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("clean repo patch = %q", empty)
	}
}

// TestCreatePatchStagedAndRename applies a staged patch to the index only,
// keeping the working tree, and keeps a rename a rename when both names are
// passed.
func TestCreatePatchStagedAndRename(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("s.txt", "a\nb\n")
	tr.write("old.txt", "keep\nme\n")
	tr.commitAll("base")

	tr.write("s.txt", "a\nB\n")
	tr.git("add", "s.txt")
	staged, err := tr.r.CreatePatch(ctx, tr.repo, []string{"s.txt"}, true)
	if err != nil {
		t.Fatal(err)
	}
	tr.git("reset", "--hard")
	if err := tr.r.ApplyPatch(ctx, tr.repo, staged, true, false); err != nil {
		t.Fatal(err)
	}
	if out := tr.git("diff", "--cached", "--name-only"); !strings.Contains(out, "s.txt") {
		t.Fatalf("index = %q", out)
	}
	if got := string(tr.readFile("s.txt")); got != "a\nb\n" {
		t.Fatalf("working tree changed: %q", got)
	}

	tr.git("reset", "--hard")
	tr.git("mv", "old.txt", "new.txt")
	rename, err := tr.r.CreatePatch(ctx, tr.repo, []string{"new.txt", "old.txt"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rename, []byte("rename from old.txt")) || !bytes.Contains(rename, []byte("rename to new.txt")) {
		t.Fatalf("rename patch = %q", rename)
	}
	tr.git("reset", "--hard")
	if err := tr.r.ApplyPatch(ctx, tr.repo, rename, true, false); err != nil {
		t.Fatal(err)
	}
	if out := tr.git("diff", "--cached", "--name-status"); !strings.Contains(out, "R100\told.txt\tnew.txt") {
		t.Fatalf("index after rename = %q", out)
	}
}

// TestCreatePatchIgnoresDiffConfig keeps the patch appliable when the user's
// diff.* settings change the prefixes or relative paths of `git diff`.
func TestCreatePatchIgnoresDiffConfig(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.commitAll("base")
	tr.git("config", "diff.noprefix", "true")
	tr.git("config", "diff.mnemonicPrefix", "true")
	tr.git("config", "diff.relative", "true")

	tr.write("a.txt", "two\n")
	patch, err := tr.r.CreatePatch(ctx, tr.repo, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(patch, []byte("diff --git a/a.txt b/a.txt")) {
		t.Fatalf("patch = %q", patch)
	}
	tr.git("checkout", "--", ".")
	if err := tr.r.ApplyPatch(ctx, tr.repo, patch, false, false); err != nil {
		t.Fatal(err)
	}
	if got := string(tr.readFile("a.txt")); got != "two\n" {
		t.Fatalf("a.txt = %q", got)
	}
}
