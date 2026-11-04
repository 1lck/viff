package gitcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
)

// conflictMergeRepo builds a repository whose main and other branches both
// changed f.txt, then merges other and leaves the file conflicted.
func conflictMergeRepo(t *testing.T, base, ours, theirs string) *testRepo {
	t.Helper()
	tr := newTestRepo(t)
	tr.write("f.txt", base)
	tr.commitAll("base")
	tr.git("checkout", "-q", "-b", "other")
	tr.write("f.txt", theirs)
	tr.commitAll("theirs")
	tr.git("checkout", "-q", "main")
	tr.write("f.txt", ours)
	tr.commitAll("ours")
	if _, err := tr.r.Merge(ctx, tr.repo, "other", false, false); KindOf(err) != ErrConflict {
		t.Fatalf("merge should conflict, got %v", err)
	}
	return tr
}

func readWorking(t *testing.T, tr *testRepo, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tr.dir, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func resolveRequest(tr *testRepo, choice domain.ResolveChoice) ConflictResolveRequest {
	return ConflictResolveRequest{
		Path:     "f.txt",
		Block:    0,
		Choice:   choice,
		Expected: readWorking(tr.t, tr, "f.txt"),
	}
}

func TestResolveConflictBlockWritesOneSideWithoutStaging(t *testing.T) {
	for _, c := range []struct {
		choice domain.ResolveChoice
		want   string
	}{
		{domain.ResolveOurs, "ours\n"},
		{domain.ResolveTheirs, "theirs\n"},
		{domain.ResolveBoth, "ours\ntheirs\n"},
	} {
		tr := conflictMergeRepo(t, "base\n", "ours\n", "theirs\n")
		res, err := tr.r.ResolveConflictBlock(ctx, tr.repo, resolveRequest(tr, c.choice))
		if err != nil {
			t.Fatalf("%v: %v", c.choice, err)
		}
		if got := string(readWorking(t, tr, "f.txt")); got != c.want {
			t.Fatalf("%v wrote %q, want %q", c.choice, got, c.want)
		}
		if res.BlocksBefore != 1 || res.BlocksAfter != 0 || res.Path != "f.txt" ||
			res.Choice != c.choice || res.LineEnding != "LF" || !res.FinalNewline {
			t.Fatalf("%v result = %+v", c.choice, res)
		}
		// Still unmerged: the resolver never stages anything.
		st := tr.status()
		e, ok := find(st, "f.txt")
		if !ok || !e.Conflicted() {
			t.Fatalf("%v staged or lost the conflict: %+v", c.choice, st.Entries)
		}
		out := tr.git("ls-files", "-u", "--", "f.txt")
		if strings.Count(out, "\n") != 3 {
			t.Fatalf("%v changed the index stages: %q", c.choice, out)
		}
		// The ordinary Mark-as-Resolved path still works afterwards.
		if err := tr.r.Stage(ctx, tr.repo, []string{"f.txt"}); err != nil {
			t.Fatal(err)
		}
		st = tr.status()
		if e, ok := find(st, "f.txt"); ok && e.Conflicted() {
			t.Fatalf("%v: mark resolved left a conflict: %+v", c.choice, st.Entries)
		}
		if _, _, _, conflicts := st.Counts(); conflicts != 0 {
			t.Fatalf("%v: conflicts left = %d", c.choice, conflicts)
		}
	}
}

func TestResolveConflictBlockLeavesTemporaryFiles(t *testing.T) {
	tr := conflictMergeRepo(t, "base\n", "ours\n", "theirs\n")
	path := filepath.Join(tr.dir, "f.txt")
	// Windows has no POSIX permission bits, so the expectation is the mode the
	// file had before: the resolver must keep it rather than impose one.
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	req := resolveRequest(tr, domain.ResolveOurs)
	if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, req); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(tr.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "viff-resolve") {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode = %v, want %v (unchanged)", info.Mode().Perm(), before.Mode().Perm())
	}
}

func TestResolveConflictBlockKeepsExecutableMode(t *testing.T) {
	tr := conflictMergeRepo(t, "base\n", "ours\n", "theirs\n")
	path := filepath.Join(tr.dir, "f.txt")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	// Compare with the mode the platform actually applied: Windows has no
	// executable bit, so a hard-coded 0755 would only be reachable there by
	// accident. What the resolver must do is leave the mode alone.
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, resolveRequest(tr, domain.ResolveTheirs)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode = %v, want %v (unchanged)", info.Mode().Perm(), before.Mode().Perm())
	}
}

func TestResolveConflictBlockRefusesStaleView(t *testing.T) {
	tr := conflictMergeRepo(t, "base\n", "ours\n", "theirs\n")
	req := resolveRequest(tr, domain.ResolveOurs)
	edited := "<<<<<<< HEAD\nedit by another program\n=======\ntheirs\n>>>>>>> other\n"
	if err := os.WriteFile(filepath.Join(tr.dir, "f.txt"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, req); !errors.Is(err, ErrResolveStale) {
		t.Fatalf("err = %v, want stale", err)
	}
	if got := string(readWorking(t, tr, "f.txt")); got != edited {
		t.Fatalf("external edit overwritten: %q", got)
	}
	// The reverse: the view is the edited file and the disk changed back.
	req = ConflictResolveRequest{Path: "f.txt", Block: 0, Choice: domain.ResolveOurs, Expected: []byte(edited)}
	if err := os.WriteFile(filepath.Join(tr.dir, "f.txt"), []byte("<<<<<<< HEAD\nother edit\n=======\ntheirs\n>>>>>>> other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, req); !errors.Is(err, ErrResolveStale) {
		t.Fatalf("err = %v, want stale", err)
	}
}

func TestResolveConflictBlockRefusesUnmergedGone(t *testing.T) {
	tr := conflictMergeRepo(t, "base\n", "ours\n", "theirs\n")
	tr.write("f.txt", "resolved elsewhere\n")
	tr.git("add", "f.txt")
	req := ConflictResolveRequest{Path: "f.txt", Block: 0, Choice: domain.ResolveOurs, Expected: []byte("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> other\n")}
	if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, req); !errors.Is(err, ErrResolveNotConflicted) {
		t.Fatalf("err = %v, want not conflicted", err)
	}
	if got := string(readWorking(t, tr, "f.txt")); got != "resolved elsewhere\n" {
		t.Fatalf("staged file overwritten: %q", got)
	}
	// An ordinary, non-conflicted path is refused too.
	if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, ConflictResolveRequest{Path: "README.md", Expected: []byte("x")}); !errors.Is(err, ErrResolveNotConflicted) {
		t.Fatalf("plain path err = %v", err)
	}
}

func TestResolveConflictBlockRefusesUnsupportedFiles(t *testing.T) {
	// A binary conflict.
	tr := newTestRepo(t)
	tr.write("f.bin", "\x00base")
	tr.commitAll("base")
	tr.git("checkout", "-q", "-b", "other")
	tr.write("f.bin", "\x00theirs")
	tr.commitAll("theirs")
	tr.git("checkout", "-q", "main")
	tr.write("f.bin", "\x00ours")
	tr.commitAll("ours")
	if _, err := tr.r.Merge(ctx, tr.repo, "other", false, false); KindOf(err) != ErrConflict {
		t.Fatalf("binary merge = %v", err)
	}
	data := readWorking(t, tr, "f.bin")
	if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, ConflictResolveRequest{Path: "f.bin", Choice: domain.ResolveOurs, Expected: data}); !errors.Is(err, ErrResolveBinary) {
		t.Fatalf("binary err = %v", err)
	}
	if got := readWorking(t, tr, "f.bin"); string(got) != "\x00ours" {
		t.Fatalf("binary file changed: %q", got)
	}

	// A conflicted path whose working file was removed by another tool.
	tr2 := conflictMergeRepo(t, "base\n", "ours\n", "theirs\n")
	req := resolveRequest(tr2, domain.ResolveOurs)
	if err := os.Remove(filepath.Join(tr2.dir, "f.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := tr2.r.ResolveConflictBlock(ctx, tr2.repo, req); !errors.Is(err, ErrResolveMissing) {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tr2.dir, "f.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file was recreated: %v", err)
	}

	// Paths that leave the repository are refused before any read.
	for _, p := range []string{"../escape.txt", "/etc/passwd", "", "a\x00b"} {
		if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, ConflictResolveRequest{Path: p, Expected: data}); !errors.Is(err, ErrResolvePath) {
			t.Fatalf("path %q err = %v", p, err)
		}
	}
}

func TestResolveConflictBlockMultipleBlocksAndCRLF(t *testing.T) {
	tr := conflictMergeRepo(t, "base\n", "ours\n", "theirs\n")
	// A user may have a multi-block file with other helpers' markers; the
	// resolver works from the bytes on disk, so replace the working file with
	// two CRLF blocks and no final newline.
	text := "top\r\n" +
		"<<<<<<< ours\r\n" +
		"a-ours\r\n" +
		"=======\r\n" +
		"a-theirs\r\n" +
		">>>>>>> theirs\r\n" +
		"middle\r\n" +
		"<<<<<<< ours\r\n" +
		"b-ours\r\n" +
		"||||||| base\r\n" +
		"b-base\r\n" +
		"=======\r\n" +
		"b-theirs\r\n" +
		">>>>>>> theirs\r\n" +
		"tail"
	path := filepath.Join(tr.dir, "f.txt")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tr.r.ResolveConflictBlock(ctx, tr.repo, ConflictResolveRequest{
		Path: "f.txt", Block: 1, Choice: domain.ResolveBoth, Expected: []byte(text),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.BlocksBefore != 2 || res.BlocksAfter != 1 || res.LineEnding != "CRLF" || res.FinalNewline {
		t.Fatalf("result = %+v", res)
	}
	want := "top\r\n" +
		"<<<<<<< ours\r\n" +
		"a-ours\r\n" +
		"=======\r\n" +
		"a-theirs\r\n" +
		">>>>>>> theirs\r\n" +
		"middle\r\n" +
		"b-ours\r\n" +
		"b-theirs\r\n" +
		"tail"
	if got := string(readWorking(t, tr, "f.txt")); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// The untouched block is byte-identical.
	if !strings.Contains(string(readWorking(t, tr, "f.txt")), "<<<<<<< ours\r\na-ours\r\n=======\r\na-theirs\r\n>>>>>>> theirs\r\n") {
		t.Fatal("first block was altered")
	}
	// Resolving the remaining block leaves no markers and keeps the CRLF.
	res, err = tr.r.ResolveConflictBlock(ctx, tr.repo, ConflictResolveRequest{
		Path: "f.txt", Block: 0, Choice: domain.ResolveOurs, Expected: readWorking(t, tr, "f.txt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.BlocksAfter != 0 || res.FinalNewline {
		t.Fatalf("second result = %+v", res)
	}
	if got := string(readWorking(t, tr, "f.txt")); got != "top\r\na-ours\r\nmiddle\r\nb-ours\r\nb-theirs\r\ntail" {
		t.Fatalf("final = %q", got)
	}
}

func TestResolveConflictBlockRefusesBadMarkers(t *testing.T) {
	tr := conflictMergeRepo(t, "base\n", "ours\n", "theirs\n")
	path := filepath.Join(tr.dir, "f.txt")
	// Markers without a closing line cannot be trusted.
	broken := "one\n<<<<<<< ours\nours\n=======\ntheirs\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, ConflictResolveRequest{
		Path: "f.txt", Block: 0, Choice: domain.ResolveOurs, Expected: []byte(broken),
	}); !errors.Is(err, domain.ErrConflictMarkers) {
		t.Fatalf("broken markers err = %v", err)
	}
	if got := string(readWorking(t, tr, "f.txt")); got != broken {
		t.Fatalf("broken file changed: %q", got)
	}
	// A valid file with an out-of-range block index is refused.
	good := "<<<<<<< ours\nours\n=======\ntheirs\n>>>>>>> theirs\n"
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.r.ResolveConflictBlock(ctx, tr.repo, ConflictResolveRequest{
		Path: "f.txt", Block: 2, Choice: domain.ResolveOurs, Expected: []byte(good),
	}); !errors.Is(err, domain.ErrConflictBlock) {
		t.Fatalf("out of range err = %v", err)
	}
	if got := string(readWorking(t, tr, "f.txt")); got != good {
		t.Fatalf("file changed after bad index: %q", got)
	}
}

func TestResolveWorktreePathResolvesRealParent(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, filepath.Join(root, "alias")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	repo := domain.Repo{Root: root}
	// A plain path resolves against the root.
	if got, err := resolveWorktreePath(repo, "real/f.txt"); err != nil || got != filepath.Join(realDir, "f.txt") {
		t.Fatalf("plain = %q %v", got, err)
	}
	// A directory symlink inside the repository resolves to the real
	// directory, so the write and the reads use the same place.
	if got, err := resolveWorktreePath(repo, "alias/f.txt"); err != nil || got != filepath.Join(realDir, "f.txt") {
		t.Fatalf("alias = %q %v", got, err)
	}
	// A directory symlink that leaves the repository is refused.
	if _, err := resolveWorktreePath(repo, "escape/f.txt"); !errors.Is(err, ErrResolvePath) {
		t.Fatalf("escape err = %v", err)
	}
	// The root itself may be opened through a symlink.
	linkRoot := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(root, linkRoot); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	if got, err := resolveWorktreePath(domain.Repo{Root: linkRoot}, "real/f.txt"); err != nil || got != filepath.Join(realDir, "f.txt") {
		t.Fatalf("symlinked root = %q %v", got, err)
	}
	// Anything that is not a clean relative Git path is refused.
	for _, p := range []string{"", "/abs", "../f.txt", "a/../f.txt", "a//f.txt", "./f.txt", "f.txt/", "a\x00b"} {
		if _, err := resolveWorktreePath(repo, p); !errors.Is(err, ErrResolvePath) {
			t.Fatalf("path %q err = %v", p, err)
		}
	}
	// A missing parent directory is reported as a missing file, not created.
	if _, err := resolveWorktreePath(repo, "missing/f.txt"); !errors.Is(err, ErrResolveMissing) {
		t.Fatalf("missing parent err = %v", err)
	}
}

func TestResolveConflictBlockRefusesEscapingSymlinkParent(t *testing.T) {
	tr := conflictMergeRepo(t, "base\n", "ours\n", "theirs\n")
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(tr.dir, "link")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	_, err = tr.r.ResolveConflictBlock(ctx, tr.repo, ConflictResolveRequest{
		Path: "link/f.txt", Block: 0, Choice: domain.ResolveOurs, Expected: []byte("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> other\n"),
	})
	if !errors.Is(err, ErrResolvePath) {
		t.Fatalf("err = %v, want a refused path", err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("write escaped the repository: %v %v", entries, err)
	}
	if got := string(readWorking(t, tr, "f.txt")); !strings.Contains(got, "<<<<<<<") {
		t.Fatalf("real conflict file was changed: %q", got)
	}
}
