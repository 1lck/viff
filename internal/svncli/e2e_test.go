package svncli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// svnBin resolves the svn client, skipping the test when Subversion is not
// installed. VIFF_SVN overrides the lookup so a Windows install outside PATH
// (the common case) can still run these tests.
func svnBin(t *testing.T) (svn, admin string) {
	t.Helper()
	svn = os.Getenv("VIFF_SVN")
	if svn == "" {
		p, err := exec.LookPath("svn")
		if err != nil {
			t.Skip("svn is not installed; set VIFF_SVN to run this test")
		}
		svn = p
	}
	admin = os.Getenv("VIFF_SVNADMIN")
	if admin == "" {
		p, err := exec.LookPath("svnadmin")
		if err != nil {
			t.Skip("svnadmin is not installed; set VIFF_SVNADMIN to run this test")
		}
		admin = p
	}
	return svn, admin
}

// newLocalRepo creates a repository and a working copy over file://, which is
// what the repository rules require: no network, no credentials, no login.
func newLocalRepo(t *testing.T) (r *Runner, repo domain.Repo, wc string) {
	t.Helper()
	svn, admin := svnBin(t)
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	if out, err := exec.Command(admin, "create", repoPath).CombinedOutput(); err != nil {
		t.Fatalf("svnadmin create: %v: %s", err, out)
	}
	// Windows needs the three-slash form; two slashes parse the drive letter as
	// a host name and the URL fails to resolve.
	url := "file:///" + filepath.ToSlash(repoPath)

	r = &Runner{Path: svn}
	ctx := context.Background()
	for _, p := range []string{"/trunk", "/branches", "/tags"} {
		if out, err := r.Run(ctx, "", "mkdir", "-m", "layout", url+p); err != nil {
			t.Fatalf("mkdir %s: %v: %s", p, err, out)
		}
	}
	parent := filepath.Join(root, "co")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.Clone(ctx, parent, url+"/trunk", "wc", nil); err != nil {
		t.Fatalf("clone: %v", err)
	}
	wc = filepath.Join(parent, "wc")
	repo, err := r.Discover(ctx, wc)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return r, repo, wc
}

// write puts a file into the working copy.
func write(t *testing.T, wc, name, content string) {
	t.Helper()
	full := filepath.Join(wc, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEndToEndWorkingCopy(t *testing.T) {
	r, repo, wc := newLocalRepo(t)
	ctx := context.Background()

	if repo.Kind != domain.KindSVN {
		t.Fatalf("kind = %v, want KindSVN", repo.Kind)
	}
	if repo.URL == "" || repo.RepoRoot == "" || repo.UUID == "" {
		t.Fatalf("repository identity incomplete: %+v", repo)
	}

	// Commit a first file: add, then commit through the target-path form.
	write(t, wc, "main.txt", "alpha\nbeta\n")
	if err := r.Stage(ctx, repo, []string{"main.txt"}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if out, err := r.Commit(ctx, repo, vcs.CommitOptions{Message: "initial content"}); err != nil {
		t.Fatalf("commit: %v: %s", err, out.Stdout)
	}

	// A clean working copy reports no entries and a revision.
	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(st.Entries) != 0 {
		t.Fatalf("expected a clean working copy, got %+v", st.Entries)
	}
	if st.Branch.Head == "" {
		t.Fatal("status has no revision")
	}

	// A modification shows up as one unstaged entry with no staged column.
	write(t, wc, "main.txt", "alpha\nBETA\n")
	st, err = r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("status after edit: %v", err)
	}
	if len(st.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %+v", st.Entries)
	}
	e := st.Entries[0]
	if e.Path != "main.txt" || e.Worktree != 'M' {
		t.Fatalf("entry = %+v, want main.txt with worktree M", e)
	}
	if e.Staged() {
		t.Fatalf("Subversion has no staging area, but the entry reports staged: %+v", e)
	}
	if !e.Unstaged() {
		t.Fatalf("entry should be unstaged: %+v", e)
	}

	// History: the explicit range is what makes the newest revision appear.
	reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{})
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer reader.Close()
	commits, done, err := reader.Next(10)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("no history returned")
	}
	if !done {
		t.Fatalf("expected the whole history in one page, got %d", len(commits))
	}
	// Newest first.
	if commits[0].Subject != "initial content" {
		t.Fatalf("newest subject = %q, want the last commit", commits[0].Subject)
	}

	// Read the committed revision, not the edited working copy.
	c, err := r.Read(ctx, repo, vcs.RevCommit(commits[0].Hash), "main.txt")
	if err != nil {
		t.Fatalf("read at revision: %v", err)
	}
	if !c.Exists || string(c.Data) != "alpha\nbeta\n" {
		t.Fatalf("content at %s = %q (exists=%v), want the committed text", commits[0].Hash, c.Data, c.Exists)
	}

	// The working copy side differs, which is what the diff view compares.
	cw, err := r.Read(ctx, repo, vcs.RevWorktree, "main.txt")
	if err != nil {
		t.Fatalf("read worktree: %v", err)
	}
	if string(cw.Data) != "alpha\nBETA\n" {
		t.Fatalf("worktree content = %q", cw.Data)
	}

	// Commit the edit so the next steps work on a clean copy.
	if out, err := r.Commit(ctx, repo, vcs.CommitOptions{Message: "change beta", Paths: []string{"main.txt"}}); err != nil {
		t.Fatalf("commit partial: %v: %s", err, out.Stdout)
	}
	st, err = r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("status after second commit: %v", err)
	}
	if len(st.Entries) != 0 {
		t.Fatalf("expected clean after commit, got %+v", st.Entries)
	}
}

func TestEndToEndConflictIsDetected(t *testing.T) {
	r, repo, wc := newLocalRepo(t)
	ctx := context.Background()

	write(t, wc, "f.txt", "one\ntwo\nthree\n")
	if err := r.Stage(ctx, repo, []string{"f.txt"}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if out, err := r.Commit(ctx, repo, vcs.CommitOptions{Message: "base"}); err != nil {
		t.Fatalf("commit: %v: %s", err, out.Stdout)
	}

	// A second working copy changes the same line and commits it.
	parent := filepath.Dir(wc)
	if err := r.Clone(ctx, parent, repo.URL, "wc2", nil); err != nil {
		t.Fatalf("clone second: %v", err)
	}
	wc2 := filepath.Join(parent, "wc2")
	write(t, wc2, "f.txt", "one\nTHEIRS\nthree\n")
	repo2, err := r.Discover(ctx, wc2)
	if err != nil {
		t.Fatalf("discover second: %v", err)
	}
	if out, err := r.Commit(ctx, repo2, vcs.CommitOptions{Message: "theirs", Paths: []string{"f.txt"}}); err != nil {
		t.Fatalf("commit second: %v: %s", err, out.Stdout)
	}

	// The first working copy edits the same line and updates into a conflict.
	write(t, wc, "f.txt", "one\nOURS\nthree\n")
	if _, err := r.Pull(ctx, repo, vcs.PullMerge, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var conflicted bool
	for _, e := range st.Entries {
		if IsConflictHelper(e.Path) {
			t.Fatalf("conflict side file leaked into the status: %s", e.Path)
		}
		if e.Conflicted() {
			conflicted = true
			if e.Path != "f.txt" {
				t.Fatalf("conflicted path = %s, want f.txt", e.Path)
			}
		}
	}
	if !conflicted {
		t.Fatalf("the update did not produce a conflict: %+v", st.Entries)
	}

	list, err := r.Conflicts(ctx, repo, st)
	if err != nil {
		t.Fatalf("conflicts: %v", err)
	}
	if len(list) != 1 || list[0].Path != "f.txt" {
		t.Fatalf("conflicts = %+v, want f.txt", list)
	}
}
