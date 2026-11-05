package svncli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// TestEdgesNonWorkingCopy covers the failure the user hits when opening a
// directory that is not a Subversion working copy, and that the error is
// classified so the UI can say something useful.
func TestEdgesNonWorkingCopy(t *testing.T) {
	svn, _ := svnBin(t)
	r := &Runner{Path: svn}
	ctx := context.Background()

	dir := t.TempDir()
	_, err := r.Discover(ctx, dir)
	if err == nil {
		t.Fatal("Discover succeeded outside a working copy")
	}
	if got := vcs.KindOf(err); got != vcs.ErrNotRepository {
		t.Fatalf("kind = %v, want ErrNotRepository (E155007)", got)
	}
	if got := vcs.CodeOf(err); got != "repo.not_a_repository" {
		t.Fatalf("code = %q", got)
	}

	// Detect must answer locally, without running svn, for the same directory.
	if r.Detect(dir) {
		t.Fatal("Detect claimed an ordinary directory is a working copy")
	}
	if r.Name() != "Subversion" {
		t.Fatalf("Name = %q", r.Name())
	}
}

// TestEdgesMissingExecutable covers a machine without svn, which is the state
// of a fresh Windows install until the user sets the path.
func TestEdgesMissingExecutable(t *testing.T) {
	r := &Runner{Path: "/nonexistent/definitely-not-svn"}
	ctx := context.Background()

	_, err := r.Version(ctx)
	if err == nil {
		t.Fatal("Version succeeded with a bogus path")
	}
	if got := vcs.KindOf(err); got != vcs.ErrNoGit {
		t.Fatalf("kind = %v, want ErrNoGit so the UI shows the install hint", got)
	}
	if got := vcs.CodeOf(err); got != "git.missing" {
		t.Fatalf("code = %q", got)
	}
	// The adapter must name itself in the message, not say "git".
	if !strings.Contains(err.Error(), "svn") {
		t.Fatalf("message should name svn: %v", err)
	}
}

// TestEdgesConflict covers a real conflict: markers on disk, the status letter,
// the side files, and Conflicts() describing them.
func TestEdgesConflict(t *testing.T) {
	f := newFullFixture(t)
	r, repo := f.r, f.repo
	ctx := f.ctx()

	// A second checkout changes the same line and commits.
	parent := filepath.Join(f.root, "conflict")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.Clone(ctx, parent, f.url, "other", nil); err != nil {
		t.Fatalf("clone: %v", err)
	}
	otherWC := filepath.Join(parent, "other")
	otherRepo, err := r.Discover(ctx, otherWC)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherWC, "README.md"), []byte("THEIRS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := r.Commit(ctx, otherRepo, vcs.CommitOptions{Message: "theirs", Paths: []string{"README.md"}}); err != nil {
		t.Fatalf("commit other: %v: %s", err, out.Stdout)
	}

	// The first copy changes it differently, then updates into the conflict.
	f.write("README.md", "OURS\n")
	if _, err := r.Pull(ctx, repo, vcs.PullMerge, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var conflicted bool
	for _, e := range st.Entries {
		if e.Path == "README.md" && e.Conflicted() {
			conflicted = true
		}
		if IsConflictHelper(e.Path) {
			t.Fatalf("conflict side file leaked into the status: %s", e.Path)
		}
	}
	if !conflicted {
		t.Fatalf("README.md not reported conflicted: %+v", st.Entries)
	}

	// The working file holds diff3 markers the viewer can parse.
	data, err := os.ReadFile(filepath.Join(f.wc, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, marker := range []string{"<<<<<<<", "=======", ">>>>>>>"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("working file lacks %q:\n%s", marker, text)
		}
	}
	if parsed := domain.ParseConflictMarkers(strings.Split(text, "\n")); !parsed.Reliable || len(parsed.Blocks) == 0 {
		t.Fatalf("the markers are not parseable: %+v", parsed)
	}

	list, err := r.Conflicts(ctx, repo, st)
	if err != nil {
		t.Fatalf("conflicts: %v", err)
	}
	if len(list) != 1 || list[0].Path != "README.md" {
		t.Fatalf("conflicts = %+v", list)
	}
	if n := len(list[0].Stages); n != 3 {
		t.Fatalf("expected three sides (base, mine, theirs), got %d: %+v", n, list[0].Stages)
	}

	// svn exits 0 on a conflicted status, so the classification must not have
	// treated the conflict as a failure of the query itself.
	if _, err := r.Status(ctx, repo); err != nil {
		t.Fatalf("status on a conflicted copy must still succeed: %v", err)
	}
}

// TestEdgesNoGitStyleOperationState pins that a Subversion working copy never
// reports an in-progress rebase/merge: the UI keys the operation banner on it.
func TestEdgesNoOperationState(t *testing.T) {
	f := newFullFixture(t)
	op := f.r.Operation(f.ctx(), f.repo)
	if op.Active() {
		t.Fatalf("got an active operation: %+v", op)
	}
	if op.Kind != domain.OpNone {
		t.Fatalf("kind = %v", op.Kind)
	}
}

// TestEdgesDeletedAndMissing covers the two ways a tracked file can be absent:
// removed from disk but still recorded ("missing"), and scheduled for deletion.
func TestEdgesDeletedAndMissing(t *testing.T) {
	f := newFullFixture(t)
	r, repo := f.r, f.repo
	ctx := f.ctx()

	target := filepath.Join(f.wc, "README.md")
	orig, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	var missing bool
	for _, e := range st.Entries {
		if e.Path == "README.md" && e.Worktree == '!' {
			missing = true
		}
	}
	if !missing {
		t.Fatalf("a file gone from disk should be '!': %+v", st.Entries)
	}

	// Reading the base side must still work: the recorded version exists even
	// though the file does not.
	base, err := r.Read(ctx, repo, vcs.RevIndex, "README.md")
	if err != nil {
		t.Fatalf("read base of a missing file: %v", err)
	}
	if !base.Exists || len(base.Data) == 0 {
		t.Fatalf("base side of a missing file = %+v", base)
	}

	if err := os.WriteFile(target, orig, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEdgesEmptyRepository covers a repository whose trunk has no content yet:
// the log exists (the layout commit) but a file listing is empty.
func TestEdgesEmptyRepository(t *testing.T) {
	svn, admin := svnBin(t)
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	if out, err := exec.Command(admin, "create", repoPath).CombinedOutput(); err != nil {
		t.Fatalf("svnadmin create: %v: %s", err, out)
	}
	url := "file:///" + filepath.ToSlash(repoPath)
	r := &Runner{Path: svn}
	ctx := context.Background()
	if out, err := r.Run(ctx, "", "mkdir", "-m", "layout", url+"/trunk", url+"/branches", url+"/tags"); err != nil {
		t.Fatalf("mkdir: %v: %s", err, out)
	}
	parent := filepath.Join(root, "co")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.Clone(ctx, parent, url+"/trunk", "wc", nil); err != nil {
		t.Fatalf("clone: %v", err)
	}
	wc := filepath.Join(parent, "wc")
	repo, err := r.Discover(ctx, wc)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("status on an empty trunk: %v", err)
	}
	if len(st.Entries) != 0 {
		t.Fatalf("expected no entries, got %+v", st.Entries)
	}

	files, err := r.WorktreeFiles(ctx, repo)
	if err != nil {
		t.Fatalf("worktree files: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected no files, got %+v", files)
	}

	tree, err := r.TreeAt(ctx, repo, "1")
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if len(tree) != 0 {
		t.Fatalf("expected an empty tree, got %+v", tree)
	}

	// A file listing of nothing must not be confused with a failure.
	if _, err := r.OpenLog(ctx, repo, vcs.LogFilter{}); err != nil {
		t.Fatalf("log: %v", err)
	}
}

// TestEdgesClassification pins the E-code mapping this adapter relies on. Each
// case is an exit path the user can hit.
func TestEdgesClassification(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		ctxErr error
		want   vcs.ErrorKind
	}{
		{"not a working copy", "svn: E155007: '/tmp/x' is not a working copy", nil, vcs.ErrNotRepository},
		{"out of date", "svn: E155011: Commit failed\nsvn: E155011: Directory is out of date", nil, vcs.ErrOutOfDate},
		{"auth", "svn: E170001: Authorization failed", nil, vcs.ErrAuth},
		{"network", "svn: E170013: Unable to connect to a repository", nil, vcs.ErrNetwork},
		{"no such revision", "svn: E160006: No such revision 99999", nil, vcs.ErrNoRevision},
		{"path not found", "svn: E195012: Unable to find repository location", nil, vcs.ErrNotFound},
		{"missing as warning", "svn: warning: W160013: File not found: revision 4", nil, vcs.ErrNotFound},
		{"locked", "svn: E155004: Working copy locked", nil, vcs.ErrLocked},
		{"conflict", "svn: E155015: Aborting update: remains in conflict", nil, vcs.ErrConflict},
		{"unclassified", "svn: something unexpected happened", nil, vcs.ErrOther},
	}
	for _, c := range cases {
		got := classify(c.stderr, os.ErrPermission, c.ctxErr)
		if got != c.want {
			t.Errorf("%s: classify = %v, want %v", c.name, got, c.want)
		}
	}

	// A cancelled context always wins over the stderr text.
	if got := classify("svn: E170001: Authorization failed", nil, context.Canceled); got != vcs.ErrCancelled {
		t.Errorf("cancelled context: classify = %v, want ErrCancelled", got)
	}
}

// TestEdgesNonInteractive pins the invariant that keeps a GUI from freezing:
// every invocation carries --non-interactive.
func TestEdgesNonInteractive(t *testing.T) {
	svn, _ := svnBin(t)
	r := &Runner{Path: svn}
	var seen []string
	r.SetCommandLog(func(dir string, args []string, took time.Duration, err error) {
		seen = append(seen, strings.Join(args, " "))
	})
	if _, err := r.Version(context.Background()); err != nil {
		t.Fatalf("version: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("no command was recorded")
	}
	for _, line := range seen {
		if !strings.HasPrefix(line, "--non-interactive") {
			t.Fatalf("command without --non-interactive: %q", line)
		}
	}
}

// TestEdgesMixedRevision covers a working copy whose paths sit at different
// revisions after a partial update, which is normal in Subversion and must not
// be mistaken for a broken state.
func TestEdgesMixedRevision(t *testing.T) {
	f := newFullFixture(t)
	r, repo := f.r, f.repo
	ctx := f.ctx()

	// Change and commit a file from a second checkout.
	parent := filepath.Join(f.root, "mixed")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.Clone(ctx, parent, f.url, "other", nil); err != nil {
		t.Fatal(err)
	}
	otherWC := filepath.Join(parent, "other")
	otherRepo, err := r.Discover(ctx, otherWC)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherWC, "src", "main.go"), []byte("package main\n// newer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := r.Commit(ctx, otherRepo, vcs.CommitOptions{Message: "newer", Paths: []string{"src/main.go"}}); err != nil {
		t.Fatalf("commit: %v: %s", err, out.Stdout)
	}

	// Update only that path in the first copy: the rest stays at its revision.
	if _, err := r.Run(ctx, f.wc, "update", "src/main.go"); err != nil {
		t.Fatalf("partial update: %v", err)
	}

	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("status on a mixed-revision copy: %v", err)
	}
	// svn reports mixed revisions as a plain status, not an error.
	for _, e := range st.Entries {
		if e.Conflicted() {
			t.Fatalf("unexpected conflict: %+v", e)
		}
	}
	// The base side of a stale path still reads.
	if _, err := r.Read(ctx, repo, vcs.RevIndex, "README.md"); err != nil {
		t.Fatalf("read base on a mixed-revision copy: %v", err)
	}
}
