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

// This file drives every method of the vcs.Backend contract against a real
// Subversion repository over file://, one subtest per method, so a missing or
// wrong behaviour shows up as a named failure instead of hiding behind an
// aggregate result. Nothing here is mocked: svn really runs.
//
// The fixture creates the conventional layout, a trunk working copy with a
// directory, a file with several lines, and a binary file, then commits twice
// so history has something to page through.

type fullFixture struct {
	t    *testing.T
	r    *Runner
	repo domain.Repo
	wc   string
	url  string
	root string
}

func newFullFixture(t *testing.T) *fullFixture {
	t.Helper()
	svn, admin := svnBin(t)
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	cmd := exec.Command(admin, "create", repoPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("svnadmin create: %v: %s", err, out)
	}
	url := "file:///" + filepath.ToSlash(repoPath)
	r := &Runner{Path: svn}
	ctx := context.Background()

	// r1: the conventional layout, in ONE commit so the revision numbers stay
	// predictable: r1 layout, r2 base content, r3 greet.
	if out, err := r.Run(ctx, "", "mkdir", "-m", "layout",
		url+"/trunk", url+"/branches", url+"/tags"); err != nil {
		t.Fatalf("mkdir layout: %v: %s", err, out)
	}
	parent := filepath.Join(root, "co")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	wc := filepath.Join(parent, "wc")
	if err := r.Clone(ctx, parent, url+"/trunk", "wc", nil); err != nil {
		t.Fatalf("clone: %v", err)
	}

	f := &fullFixture{t: t, r: r, wc: wc, url: url + "/trunk", root: root}

	// r2: a directory, a text file and a binary file.
	f.write("src/main.go", "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n")
	f.write("README.md", "readme line\n")
	f.write("blob.bin", "\x00\x01\x02binary\x00data")
	if err := r.Stage(ctx, mustDiscover(t, r, wc), []string{"src", "README.md", "blob.bin"}); err != nil {
		t.Fatalf("stage base: %v", err)
	}
	if out, err := r.Commit(ctx, mustDiscover(t, r, wc), vcs.CommitOptions{Message: "base content\n\nwith a body line"}); err != nil {
		t.Fatalf("commit base: %v: %s", err, out.Stdout)
	}

	// r3: a second commit so the log has two entries and the parent differs.
	f.write("src/main.go", "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n")
	if out, err := r.Commit(ctx, mustDiscover(t, r, wc), vcs.CommitOptions{Message: "greet", Paths: []string{"src/main.go"}}); err != nil {
		t.Fatalf("commit greet: %v: %s", err, out.Stdout)
	}

	repo, err := r.Discover(ctx, wc)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	f.repo = repo
	return f
}

func mustDiscover(t *testing.T, r *Runner, dir string) domain.Repo {
	t.Helper()
	repo, err := r.Discover(context.Background(), dir)
	if err != nil {
		t.Fatalf("discover %s: %v", dir, err)
	}
	return repo
}

func (f *fullFixture) write(name, content string) {
	f.t.Helper()
	p := filepath.Join(f.wc, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fullFixture) ctx() context.Context { return context.Background() }

// TestAllReads covers Opener.Discover and every Reader method.
func TestAllReads(t *testing.T) {
	f := newFullFixture(t)
	r, repo := f.r, f.repo
	ctx := f.ctx()

	t.Run("Discover", func(t *testing.T) {
		if repo.Kind != domain.KindSVN {
			t.Fatalf("kind = %v", repo.Kind)
		}
		if repo.Root == "" || repo.MetaDir == "" || repo.URL == "" || repo.RepoRoot == "" || repo.UUID == "" {
			t.Fatalf("incomplete identity: %+v", repo)
		}
		if !strings.HasSuffix(filepath.ToSlash(repo.MetaDir), ".svn") {
			t.Fatalf("MetaDir = %q", repo.MetaDir)
		}
	})

	t.Run("Clone", func(t *testing.T) {
		// The fixture already cloned; clone once more into a second directory.
		parent := filepath.Join(f.root, "co2")
		if err := os.MkdirAll(parent, 0o755); err != nil {
			t.Fatal(err)
		}
		var progress []string
		if err := r.Clone(ctx, parent, f.url, "wc2", func(s string) { progress = append(progress, s) }); err != nil {
			t.Fatalf("clone: %v", err)
		}
		if _, err := os.Stat(filepath.Join(parent, "wc2", "README.md")); err != nil {
			t.Fatalf("cloned file missing: %v", err)
		}
		if len(progress) == 0 {
			t.Log("note: no progress lines were emitted (small local clone)")
		}
	})

	t.Run("Init_reports_unsupported", func(t *testing.T) {
		// Subversion has no local repository: a working copy is always a
		// checkout of a server-side path. It must fail with a clear message.
		err := r.Init(ctx, t.TempDir())
		if err == nil {
			t.Fatal("Init succeeded; Subversion has no local repository")
		}
		if !strings.Contains(err.Error(), "repository URL") {
			t.Fatalf("message should point at Clone: %v", err)
		}
	})

	t.Run("Status_clean", func(t *testing.T) {
		st, err := r.Status(ctx, repo)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if len(st.Entries) != 0 {
			t.Fatalf("expected clean, got %+v", st.Entries)
		}
		if st.Branch.Head == "" || st.Branch.Branch != "trunk" {
			t.Fatalf("branch = %+v, want revision and trunk", st.Branch)
		}
	})

	t.Run("Status_modified_and_untracked", func(t *testing.T) {
		f.write("README.md", "readme changed\n")
		defer func() {
			if err := r.Discard(ctx, repo, []string{"README.md"}); err != nil {
				t.Fatalf("cleanup: %v", err)
			}
		}()
		f.write("loose.txt", "not versioned\n")
		defer os.Remove(filepath.Join(f.wc, "loose.txt"))

		st, err := r.Status(ctx, repo)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		var modified, untracked bool
		for _, e := range st.Entries {
			switch e.Path {
			case "README.md":
				modified = e.Worktree == 'M' && e.Unstaged() && !e.Staged()
			case "loose.txt":
				untracked = e.Untracked()
			}
		}
		if !modified {
			t.Fatalf("README.md not reported modified+unstaged: %+v", st.Entries)
		}
		if !untracked {
			t.Fatalf("loose.txt not reported untracked: %+v", st.Entries)
		}
	})

	t.Run("Operation_none", func(t *testing.T) {
		op := r.Operation(ctx, repo)
		if op.Active() {
			t.Fatalf("Subversion has no rebase/merge state machine, got %+v", op)
		}
	})

	t.Run("OpenLog_and_history", func(t *testing.T) {
		reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{})
		if err != nil {
			t.Fatalf("open log: %v", err)
		}
		defer reader.Close()
		commits, done, err := reader.Next(50)
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		if !done {
			t.Fatalf("expected complete history, got %d", len(commits))
		}
		if len(commits) < 3 {
			t.Fatalf("expected at least 3 revisions (layout, base, greet), got %d", len(commits))
		}
		if commits[0].Subject != "greet" {
			t.Fatalf("newest = %q, want greet", commits[0].Subject)
		}
		// Newest first and strictly descending revision numbers.
		for i := 1; i < len(commits); i++ {
			if revNumber(commits[i-1].Hash) <= revNumber(commits[i].Hash) {
				t.Fatalf("history not descending at %d: %+v", i, commits)
			}
		}
	})

	t.Run("OpenLog_text_filter", func(t *testing.T) {
		reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{Text: "greet"})
		if err != nil {
			t.Fatalf("open log: %v", err)
		}
		defer reader.Close()
		commits, _, err := reader.Next(50)
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		if len(commits) != 1 || commits[0].Subject != "greet" {
			t.Fatalf("filter result = %+v, want only greet", commits)
		}
	})

	t.Run("CommitDetail", func(t *testing.T) {
		reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{})
		if err != nil {
			t.Fatal(err)
		}
		commits, _, _ := reader.Next(50)
		reader.Close()

		// The "base content" revision has a body and three files.
		var baseRev string
		for _, c := range commits {
			if c.Subject == "base content" {
				baseRev = c.Hash
			}
		}
		if baseRev == "" {
			t.Fatalf("base content commit not found in %+v", commits)
		}
		detail, err := r.CommitDetail(ctx, repo, baseRev)
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		if !strings.Contains(detail.Body, "with a body line") {
			t.Fatalf("body = %q, want the full message", detail.Body)
		}
		var sawSrc, sawReadme, sawBin bool
		for _, c := range detail.Changes {
			switch c.Path {
			case "src/main.go":
				sawSrc = true
			case "README.md":
				sawReadme = true
			case "blob.bin":
				sawBin = true
			}
			if strings.HasPrefix(c.Path, "trunk/") {
				t.Fatalf("path %q kept the working copy prefix", c.Path)
			}
		}
		if !sawSrc || !sawReadme || !sawBin {
			t.Fatalf("changes = %+v, want all three files (directories are filtered)", detail.Changes)
		}
		for _, c := range detail.Changes {
			if c.Path == "src" {
				t.Fatal("a directory leaked into the change list")
			}
		}
	})

	t.Run("Read_commit_worktree_and_base", func(t *testing.T) {
		reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{})
		if err != nil {
			t.Fatal(err)
		}
		commits, _, _ := reader.Next(50)
		reader.Close()
		head := commits[0].Hash

		c, err := r.Read(ctx, repo, vcs.RevCommit(head), "src/main.go")
		if err != nil {
			t.Fatalf("read commit: %v", err)
		}
		if !c.Exists || !strings.Contains(string(c.Data), `println("hello")`) {
			t.Fatalf("committed content = %q", c.Data)
		}

		w, err := r.Read(ctx, repo, vcs.RevWorktree, "src/main.go")
		if err != nil {
			t.Fatalf("read worktree: %v", err)
		}
		if string(w.Data) != string(c.Data) {
			t.Fatalf("worktree and HEAD should match on a clean copy")
		}

		// The index/base side must resolve through the working copy path; a
		// keyword with a URL is rejected by svn.
		b, err := r.Read(ctx, repo, vcs.RevIndex, "src/main.go")
		if err != nil {
			t.Fatalf("read base: %v", err)
		}
		if !b.Exists {
			t.Fatal("base side missing")
		}

		// A path that never existed at that revision is an absent side.
		missing, err := r.Read(ctx, repo, vcs.RevCommit("1"), "src/main.go")
		if err != nil {
			t.Fatalf("read missing: %v", err)
		}
		if missing.Exists {
			t.Fatal("src/main.go should not exist at r1")
		}
	})

	t.Run("Read_binary_detection", func(t *testing.T) {
		reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{})
		if err != nil {
			t.Fatal(err)
		}
		commits, _, _ := reader.Next(50)
		reader.Close()
		c, err := r.Read(ctx, repo, vcs.RevCommit(commits[0].Hash), "blob.bin")
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !c.Exists || !c.Binary {
			t.Fatalf("binary file not detected: exists=%v binary=%v", c.Exists, c.Binary)
		}
	})

	t.Run("Blame", func(t *testing.T) {
		b, err := r.Blame(ctx, repo, "", "src/main.go")
		if err != nil {
			t.Fatalf("blame: %v", err)
		}
		if b == nil || len(b.Lines) == 0 {
			t.Fatal("blame returned no lines")
		}
		if len(b.Lines) != len(b.Orig) {
			t.Fatalf("Lines and Orig disagree: %d vs %d", len(b.Lines), len(b.Orig))
		}
		if len(b.Commits) == 0 {
			t.Fatal("blame returned no commits")
		}
		for i, idx := range b.Lines {
			if int(idx) < 0 || int(idx) >= len(b.Commits) {
				t.Fatalf("line %d points at commit index %d of %d", i, idx, len(b.Commits))
			}
		}
		if b.Commits[0].Hash == "" {
			t.Fatal("blame commit has no revision")
		}
	})

	t.Run("DiffNames_worktree", func(t *testing.T) {
		f.write("README.md", "readme changed\n")
		defer r.Discard(ctx, repo, []string{"README.md"})
		changes, err := r.DiffNames(ctx, repo, "", "")
		if err != nil {
			t.Fatalf("diffnames: %v", err)
		}
		if len(changes) != 1 || changes[0].Path != "README.md" || changes[0].Kind != domain.ChangeModified {
			t.Fatalf("changes = %+v, want README.md modified", changes)
		}
	})

	t.Run("DiffNames_revisions", func(t *testing.T) {
		changes, err := r.DiffNames(ctx, repo, "2", "3")
		if err != nil {
			t.Fatalf("diffnames: %v", err)
		}
		if len(changes) != 1 || changes[0].Path != "src/main.go" {
			t.Fatalf("changes = %+v, want src/main.go", changes)
		}
	})

	t.Run("DiffNames_rejects_keyword", func(t *testing.T) {
		// A working-copy keyword with a URL is invalid in svn; the adapter must
		// say so in its own words instead of passing it through.
		if _, err := r.DiffNames(ctx, repo, "BASE", "HEAD"); err == nil {
			t.Fatal("expected a clear refusal for BASE:HEAD")
		}
	})

	t.Run("TreeAt", func(t *testing.T) {
		entries, err := r.TreeAt(ctx, repo, "3")
		if err != nil {
			t.Fatalf("tree: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("empty tree")
		}
		var sawFile, sawDir bool
		for _, e := range entries {
			switch e.Path {
			case "src/main.go":
				sawFile = e.Type == "blob"
			case "src":
				sawDir = e.Type == "tree"
			}
			if strings.Contains(e.Path, "<") {
				t.Fatalf("XML leaked into a path: %q", e.Path)
			}
		}
		if !sawFile {
			t.Fatalf("src/main.go missing or wrong type: %+v", entries)
		}
		if !sawDir {
			t.Fatalf("src directory missing: %+v", entries)
		}
	})

	t.Run("MultiCommitChanges", func(t *testing.T) {
		reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{})
		if err != nil {
			t.Fatal(err)
		}
		commits, _, _ := reader.Next(2)
		reader.Close()
		res := r.MultiCommitChanges(ctx, repo, commits)
		if len(res) != len(commits) {
			t.Fatalf("got %d results for %d commits", len(res), len(commits))
		}
		for i, one := range res {
			if one.Err != nil {
				t.Fatalf("commit %d failed: %v", i, one.Err)
			}
			if one.Commit.Hash != commits[i].Hash {
				t.Fatalf("order changed at %d", i)
			}
		}
		if len(res[0].Changes) == 0 {
			t.Fatal("newest commit reported no changes")
		}
	})

	t.Run("Config_returns_empty", func(t *testing.T) {
		// Subversion keeps its configuration in its own files; Viff does not
		// read them, so every key is empty rather than an error.
		if got := r.Config(ctx, repo, "anything"); got != "" {
			t.Fatalf("Config = %q, want empty", got)
		}
	})

	t.Run("WorktreeFiles", func(t *testing.T) {
		files, err := r.WorktreeFiles(ctx, repo)
		if err != nil {
			t.Fatalf("worktree files: %v", err)
		}
		want := map[string]bool{"README.md": false, "src/main.go": false, "blob.bin": false}
		for _, p := range files {
			if _, ok := want[p]; ok {
				want[p] = true
			}
			if strings.Contains(p, "\\") {
				t.Fatalf("path not slash-separated: %q", p)
			}
		}
		for name, seen := range want {
			if !seen {
				t.Fatalf("%s missing from %+v", name, files)
			}
		}
		for _, p := range files {
			if p == "src" {
				t.Fatalf("a directory leaked into the file list: %+v", files)
			}
		}
	})

	t.Run("WorktreeFiles_includes_untracked", func(t *testing.T) {
		f.write("loose.txt", "x\n")
		defer os.Remove(filepath.Join(f.wc, "loose.txt"))
		files, err := r.WorktreeFiles(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, p := range files {
			if p == "loose.txt" {
				found = true
			}
		}
		if !found {
			t.Fatalf("untracked file missing from %+v", files)
		}
	})
}

// TestAllReferences covers ReferenceBrowser.
func TestAllReferences(t *testing.T) {
	f := newFullFixture(t)
	r, repo := f.r, f.repo
	ctx := f.ctx()

	t.Run("Refs", func(t *testing.T) {
		refs, err := r.Refs(ctx, repo)
		if err != nil {
			t.Fatalf("refs: %v", err)
		}
		var trunk, current bool
		for _, r := range refs {
			if r.Short == "trunk" {
				trunk = true
				current = r.Current
			}
			if strings.Contains(r.Name, "^") {
				t.Fatalf("a ^ marker leaked into a ref name: %q", r.Name)
			}
		}
		if !trunk {
			t.Fatalf("trunk missing from %+v", refs)
		}
		if !current {
			t.Fatal("trunk should be the current ref")
		}
	})

	t.Run("Remotes", func(t *testing.T) {
		remotes, err := r.Remotes(ctx, repo)
		if err != nil {
			t.Fatalf("remotes: %v", err)
		}
		if len(remotes) != 1 || remotes[0].FetchURL == "" {
			t.Fatalf("remotes = %+v, want the repository URL", remotes)
		}
	})

	t.Run("ResolveRef", func(t *testing.T) {
		got, err := r.ResolveRef(ctx, repo, "3")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got != "3" {
			t.Fatalf("ResolveRef(3) = %q", got)
		}
		if _, err := r.ResolveRef(ctx, repo, "99999"); err == nil {
			t.Fatal("a missing revision should fail")
		}
	})

	t.Run("IsAncestor", func(t *testing.T) {
		if !r.IsAncestor(ctx, repo, "2", "3") {
			t.Fatal("2 should be an ancestor of 3 in linear history")
		}
		if r.IsAncestor(ctx, repo, "3", "2") {
			t.Fatal("3 must not be an ancestor of 2")
		}
	})

	t.Run("ContainingBranches_returns_nothing", func(t *testing.T) {
		// svn cannot answer this cheaply, so the adapter reports no markers
		// instead of inventing them.
		got, err := r.ContainingBranches(ctx, repo, "3", 10)
		if err != nil {
			t.Fatalf("containing branches: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no markers, got %+v", got)
		}
	})

	t.Run("CheckoutRevision", func(t *testing.T) {
		if err := r.CheckoutRevision(ctx, repo, "2"); err != nil {
			t.Fatalf("checkout revision: %v", err)
		}
		// Back to the newest so later assertions see HEAD content.
		if _, err := r.Pull(ctx, repo, vcs.PullMerge, nil); err != nil {
			t.Fatalf("update back: %v", err)
		}
		if err := r.CheckoutRevision(ctx, repo, "3"); err != nil {
			t.Fatalf("checkout revision 3: %v", err)
		}
	})

	t.Run("Checkout_switches_branch_directory", func(t *testing.T) {
		// A branch is made with "svn copy" from trunk, not "svn mkdir": a fresh
		// directory shares no ancestry, and svn then refuses to switch to it
		// (E195012).
		if out, err := r.Run(ctx, "", "copy", "-m", "make branch",
			f.repo.RepoRoot+"/trunk", f.repo.RepoRoot+"/branches/feature"); err != nil {
			t.Fatalf("copy branch: %v: %s", err, out)
		}
		defer func() {
			// Restore trunk so the fixture is left usable.
			if err := r.Checkout(ctx, repo, domain.Ref{Short: "trunk"}); err != nil {
				t.Logf("restore trunk: %v", err)
			}
		}()
		refs, err := r.Refs(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		var feature domain.Ref
		for _, x := range refs {
			if x.Short == "feature" {
				feature = x
			}
		}
		if feature.Short == "" {
			t.Fatalf("branches/feature not listed in %+v", refs)
		}
		if err := r.Checkout(ctx, repo, feature); err != nil {
			t.Fatalf("switch: %v", err)
		}
		after, err := r.Discover(ctx, f.wc)
		if err != nil {
			t.Fatalf("discover after switch: %v", err)
		}
		if !strings.HasSuffix(after.URL, "/branches/feature") {
			t.Fatalf("still at %q after switch", after.URL)
		}
	})
}

// TestAllLifecycle covers Lifecycle.
func TestAllLifecycle(t *testing.T) {
	f := newFullFixture(t)
	r := f.r

	t.Run("Version", func(t *testing.T) {
		v, err := r.Version(f.ctx())
		if err != nil {
			t.Fatalf("version: %v", err)
		}
		if !strings.HasPrefix(v, "1.") {
			t.Fatalf("version = %q", v)
		}
	})

	t.Run("SetConfig_is_noop", func(t *testing.T) {
		// Subversion has none of the Git configuration Viff injects; the call
		// must exist and change nothing rather than fail.
		r.SetConfig(vcs.Config{SSHCommand: "ignored", SigningKey: "ignored"})
		if _, err := r.Status(f.ctx(), f.repo); err != nil {
			t.Fatalf("status after SetConfig: %v", err)
		}
	})

	t.Run("SetPath", func(t *testing.T) {
		old := r.Path
		r.SetPath("/nonexistent/svn")
		if _, err := r.Version(f.ctx()); err == nil {
			t.Fatal("a bogus path should fail")
		}
		r.SetPath(old)
		if _, err := r.Version(f.ctx()); err != nil {
			t.Fatalf("version after restoring the path: %v", err)
		}
	})

	t.Run("SetCommandLog", func(t *testing.T) {
		// The hook must receive one call per finished command and nothing when
		// it is nil.
		var count int
		r.SetCommandLog(func(dir string, args []string, took time.Duration, err error) { count++ })
		if _, err := r.Version(f.ctx()); err != nil {
			t.Fatalf("version: %v", err)
		}
		if count != 1 {
			t.Fatalf("hook ran %d times, want 1", count)
		}
		r.SetCommandLog(nil)
		if _, err := r.Version(f.ctx()); err != nil {
			t.Fatalf("version after clearing the hook: %v", err)
		}
		if count != 1 {
			t.Fatalf("cleared hook still ran: %d", count)
		}
	})
}
