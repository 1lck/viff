package app

// Git scenario matrix: each test builds a real repository in a temporary
// directory, drives the Workspace the way the UI does, and checks HEAD, the
// index and the working tree with git itself afterwards.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// gitIn runs git in another directory with the fixture's isolated config.
func (f *fixture) gitIn(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// gitFails runs git and expects it to fail (a conflict, say).
func (f *fixture) gitFails(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err == nil {
		f.t.Fatalf("git %v unexpectedly succeeded:\n%s", args, out)
	}
	return string(out)
}

func (f *fixture) commitAll(msg string) {
	f.t.Helper()
	f.git("add", "-A")
	f.git("commit", "-q", "-m", msg)
}

// sync waits for running tasks, then for a fresh status snapshot.
func (f *fixture) sync() {
	f.t.Helper()
	f.settle(f.idle)
	t0 := time.Now()
	f.w.Refresh(true)
	f.settle(func() bool { return f.w.Repo.StatusTime.After(t0) && f.idle() })
}

// errorsSince returns the error notifications raised after the first n.
func (f *fixture) errorsSince(n int) []Notification {
	var out []Notification
	for _, x := range f.w.Notifications[min(n, len(f.w.Notifications)):] {
		if x.Kind == NoticeError {
			out = append(out, x)
		}
	}
	return out
}

func (f *fixture) loadDiff(src DiffSource) *DiffResult {
	f.t.Helper()
	var res *DiffResult
	var err error
	done := false
	f.w.LoadDiff(src, domain.WhitespaceExact, func(r *DiffResult, e error) { res, err, done = r, e, true })
	f.settle(func() bool { return done })
	if err != nil {
		f.t.Fatalf("diff %s: %v", src.Path, err)
	}
	return res
}

func (f *fixture) entry(path string) (domain.StatusEntry, bool) {
	for _, e := range f.w.Repo.Status.Entries {
		if e.Path == path {
			return e, true
		}
	}
	return domain.StatusEntry{}, false
}

func (f *fixture) head() string { return strings.TrimSpace(f.git("rev-parse", "HEAD")) }

func (f *fixture) subject() string { return strings.TrimSpace(f.git("log", "-1", "--format=%s")) }

// ---------------------------------------------------------------------------

func TestScenarioEmptyRepository(t *testing.T) {
	f := newFixture(t)
	f.open()
	rs := f.w.Repo
	if rs.HasHead() || len(rs.Log.Commits) != 0 || rs.CurrentBranch() != "main" {
		t.Fatalf("unborn repo: hasHead=%v commits=%d branch=%q", rs.HasHead(), len(rs.Log.Commits), rs.CurrentBranch())
	}
	f.write("hello.txt", "hi\n")
	f.sync()
	res := f.loadDiff(DiffSource{Path: "hello.txt", Area: domain.AreaUntracked, Working: true, Kind: domain.ChangeUntracked})
	if len(res.Blocks) != 1 || res.Old.Content.Exists {
		t.Fatalf("untracked diff in unborn repo: blocks=%d oldExists=%v", len(res.Blocks), res.Old.Content.Exists)
	}
	f.w.StagePaths([]string{"hello.txt"})
	f.sync()
	f.w.UnstagePaths([]string{"hello.txt"}) // no HEAD yet: must use rm --cached
	f.sync()
	if e, _ := f.entry("hello.txt"); !e.Untracked() {
		t.Fatalf("unstage in unborn repo left %+v", e)
	}
	f.w.StagePaths([]string{"hello.txt"})
	f.sync()
	f.w.Commit(CommitRequest{Message: "First"}, nil)
	f.sync()
	f.settle(func() bool { return len(f.w.Repo.Log.Commits) == 1 && f.idle() })
	if f.subject() != "First" || !f.w.Repo.HasHead() {
		t.Fatalf("first commit: subject %q", f.subject())
	}
}

func TestScenarioDetachedHead(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "1\n")
	f.commitAll("A")
	first := f.head()
	f.write("a.txt", "2\n")
	f.commitAll("B")
	f.open()

	f.w.CheckoutRevision(first)
	f.sync()
	st := f.w.Repo.Status.Branch
	if !st.Detached || f.w.Repo.CurrentBranch() != "" || f.head() != first {
		t.Fatalf("after checkout of %s: %+v head=%s", first, st, f.head())
	}
	// Work done while detached is kept by a new branch.
	f.write("b.txt", "new\n")
	f.w.StagePaths([]string{"b.txt"})
	f.sync()
	f.w.Commit(CommitRequest{Message: "Detached work"}, nil)
	f.sync()
	f.w.NewBranch("rescue", "HEAD", true, false)
	f.sync()
	if f.w.Repo.CurrentBranch() != "rescue" || f.subject() != "Detached work" {
		t.Fatalf("rescue branch: current=%q subject=%q", f.w.Repo.CurrentBranch(), f.subject())
	}
	if got := strings.TrimSpace(f.git("rev-parse", "rescue^")); got != first {
		t.Fatalf("rescue parent = %s, want %s", got, first)
	}
}

func TestScenarioRenameModeSymlinkBinaryDelete(t *testing.T) {
	f := newFixture(t)
	f.write("old.txt", strings.Repeat("line\n", 20))
	f.write("run.sh", "echo hi\n")
	f.write("target.txt", "target\n")
	f.write("link.txt", "plain file\n")
	f.write("gone.txt", "bye\n")
	f.write("logo.bin", "\x00\x01\x02binary")
	f.commitAll("Base")

	f.git("mv", "old.txt", "new.txt")
	f.write("new.txt", strings.Repeat("line\n", 20)+"added\n")
	f.git("add", "new.txt")
	if err := os.Chmod(filepath.Join(f.dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(f.dir, "link.txt"))
	if err := os.Symlink("target.txt", filepath.Join(f.dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(f.dir, "gone.txt"))
	f.write("logo.bin", "\x00\x09\x09other binary")
	f.open()

	ren, ok := f.entry("new.txt")
	if !ok || ren.OrigPath != "old.txt" || ren.Index != 'R' {
		t.Fatalf("rename entry = %+v (found %v)", ren, ok)
	}
	res := f.loadDiff(DiffSource{Path: "new.txt", OldPath: "old.txt", Area: domain.AreaStaged, Working: true, Kind: domain.ChangeRenamed})
	if !res.Old.Content.Exists || len(res.Blocks) != 1 {
		t.Fatalf("renamed diff: oldExists=%v blocks=%d", res.Old.Content.Exists, len(res.Blocks))
	}
	mode := f.loadDiff(DiffSource{Path: "run.sh", Area: domain.AreaUnstaged, Working: true})
	if !strings.Contains(mode.ModeChange, "100755") {
		t.Fatalf("mode change not reported: %q notice=%q", mode.ModeChange, mode.Notice)
	}
	link := f.loadDiff(DiffSource{Path: "link.txt", Area: domain.AreaUnstaged, Working: true, Kind: domain.ChangeTypeChanged})
	if !link.New.Content.Symlink || !bytes.Contains(link.New.Content.Data, []byte("target.txt")) {
		t.Fatalf("symlink side: %+v notice=%q", link.New.Content, link.Notice)
	}
	bin := f.loadDiff(DiffSource{Path: "logo.bin", Area: domain.AreaUnstaged, Working: true})
	if bin.Notice == "" || len(bin.Blocks) != 0 {
		t.Fatalf("binary diff should be a notice, got blocks=%d notice=%q", len(bin.Blocks), bin.Notice)
	}
	del := f.loadDiff(DiffSource{Path: "gone.txt", Area: domain.AreaUnstaged, Working: true, Kind: domain.ChangeDeleted})
	if del.New.Content.Exists || len(del.Blocks) != 1 {
		t.Fatalf("deleted diff: newExists=%v blocks=%d", del.New.Content.Exists, len(del.Blocks))
	}

	// Stage everything and commit; the commit's own change list keeps the rename.
	f.w.StagePaths([]string{"run.sh", "link.txt", "gone.txt", "logo.bin"})
	f.sync()
	f.w.Commit(CommitRequest{Message: "Special changes"}, nil)
	f.sync()
	if out := f.git("status", "--porcelain"); out != "" {
		t.Fatalf("tree not clean after commit:\n%s", out)
	}
	var changes []domain.Change
	done := false
	f.w.CompareWith("HEAD^", "HEAD", func(c []domain.Change, err error) { changes, done = c, true })
	f.settle(func() bool { return done })
	kinds := map[string]domain.ChangeKind{}
	for _, c := range changes {
		kinds[c.Path] = c.Kind
		if c.Path == "new.txt" && c.OldPath != "old.txt" {
			t.Fatalf("commit rename lost its old path: %+v", c)
		}
	}
	if kinds["new.txt"] != domain.ChangeRenamed || kinds["gone.txt"] != domain.ChangeDeleted || kinds["link.txt"] != domain.ChangeTypeChanged {
		t.Fatalf("commit change kinds = %v", kinds)
	}
}

func TestScenarioSpecialPaths(t *testing.T) {
	f := newFixture(t)
	names := []string{"中文 文件.txt", "-leading-dash.txt", "dir with space/a b.txt", "emoji-😀.md", "quote\"d.txt", "tab\tname.txt"}
	f.write("keep.txt", "x\n")
	f.commitAll("Base")
	for _, n := range names {
		f.write(n, "content of "+n+"\n")
	}
	f.open()
	for _, n := range names {
		if e, ok := f.entry(n); !ok || !e.Untracked() {
			t.Fatalf("untracked %q not listed (entries %+v)", n, f.w.Repo.Status.Entries)
		}
	}
	f.w.StagePaths(names)
	f.sync()
	for _, n := range names {
		if e, _ := f.entry(n); e.Index != 'A' {
			t.Fatalf("%q not staged: %+v", n, e)
		}
	}
	f.w.UnstagePaths(names[:2])
	f.sync()
	if e, _ := f.entry(names[0]); !e.Untracked() {
		t.Fatalf("unstage %q: %+v", names[0], e)
	}
	// Roll back the untracked ones (deletes them) and commit the rest.
	f.w.Rollback(nil, names[:2], false)
	f.sync()
	for _, n := range names[:2] {
		if _, err := os.Stat(filepath.Join(f.dir, n)); !os.IsNotExist(err) {
			t.Fatalf("rollback kept %q", n)
		}
	}
	f.w.Commit(CommitRequest{Message: "Odd names"}, nil)
	f.sync()
	listed := f.git("ls-tree", "-r", "-z", "--name-only", "HEAD")
	for _, n := range names[2:] {
		if !strings.Contains(listed, n+"\x00") {
			t.Fatalf("%q missing from commit: %q", n, listed)
		}
	}
	res := f.loadDiff(DiffSource{Path: names[3], Commit: f.head(), Parent: f.head() + "^", Kind: domain.ChangeAdded})
	if len(res.Blocks) != 1 {
		t.Fatalf("commit diff of %q: %d blocks", names[3], len(res.Blocks))
	}
}

// conflictRepo returns a fixture on main with branch "other" diverged.
func conflictRepo(t *testing.T) *fixture {
	f := newFixture(t)
	f.write("both.txt", "base\n")
	f.write("delete-theirs.txt", "keep me\n")
	f.write("delete-ours.txt", "keep me\n")
	f.write("pic.bin", "\x00base")
	f.commitAll("Base")
	f.git("checkout", "-q", "-b", "other")
	f.write("both.txt", "theirs\n")
	os.Remove(filepath.Join(f.dir, "delete-theirs.txt"))
	f.write("delete-ours.txt", "changed by theirs\n")
	f.write("added.txt", "theirs version\n")
	f.write("pic.bin", "\x00theirs")
	f.commitAll("Theirs")
	f.git("checkout", "-q", "main")
	f.write("both.txt", "ours\n")
	f.write("delete-theirs.txt", "changed by ours\n")
	os.Remove(filepath.Join(f.dir, "delete-ours.txt"))
	f.write("added.txt", "ours version\n")
	f.write("pic.bin", "\x00ours")
	f.commitAll("Ours")
	return f
}

func TestScenarioMergeConflictKinds(t *testing.T) {
	f := conflictRepo(t)
	f.open()
	f.w.Merge("other", false)
	f.sync()
	rs := f.w.Repo
	if rs.Operation.Kind != domain.OpMerge {
		t.Fatalf("operation = %+v", rs.Operation)
	}
	kinds := map[string]domain.ConflictKind{}
	for _, c := range rs.Conflicts {
		kinds[c.Path] = c.Kind
	}
	want := map[string]domain.ConflictKind{
		"both.txt":          domain.ConflictBothModified,
		"added.txt":         domain.ConflictBothAdded,
		"delete-theirs.txt": domain.ConflictDeletedByThem,
		"delete-ours.txt":   domain.ConflictDeletedByUs,
		"pic.bin":           domain.ConflictBothModified,
	}
	for p, k := range want {
		if got, ok := kinds[p]; !ok || got != k {
			t.Fatalf("conflict %s = %v (present %v), want %v; all %v", p, got, ok, k, kinds)
		}
	}
	// Every kind opens a conflict view without error.
	for p := range want {
		res := f.loadDiff(DiffSource{Path: p, Area: domain.AreaConflicts, Working: true, Kind: domain.ChangeConflicted})
		if res.Conflict == nil {
			t.Fatalf("%s: no conflict view", p)
		}
	}
	text := f.loadDiff(DiffSource{Path: "both.txt", Area: domain.AreaConflicts, Working: true, Kind: domain.ChangeConflicted})
	if len(text.Conflict.Markers.Blocks) != 1 || text.Conflict.Resolved {
		t.Fatalf("both.txt markers=%d resolved=%v", len(text.Conflict.Markers.Blocks), text.Conflict.Resolved)
	}
	// An external tool fixes the file: the view reports it as resolved but unstaged.
	f.write("both.txt", "merged\n")
	f.sync()
	fixed := f.loadDiff(DiffSource{Path: "both.txt", Area: domain.AreaConflicts, Working: true, Kind: domain.ChangeConflicted})
	if !fixed.Conflict.Resolved {
		t.Fatalf("file without markers should read as resolved: %+v", fixed.Conflict.Markers)
	}
	// Resolve the rest the way a user would and continue.
	f.write("added.txt", "both\n")
	f.write("pic.bin", "\x00ours")
	f.git("rm", "-q", "delete-ours.txt")
	f.w.StagePaths([]string{"both.txt", "added.txt", "pic.bin", "delete-theirs.txt"})
	f.sync()
	if n := len(f.w.Repo.Conflicts); n != 0 {
		t.Fatalf("still %d conflicts: %+v", n, f.w.Repo.Conflicts)
	}
	f.w.OperationAction("continue")
	f.sync()
	if f.w.Repo.Operation.Active() {
		t.Fatalf("merge still active after continue")
	}
	if parents := strings.Fields(f.git("log", "-1", "--format=%P")); len(parents) != 2 {
		t.Fatalf("merge commit parents = %v", parents)
	}
}

func TestScenarioCherryPickSkipAndAbort(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.commitAll("Base")
	f.git("checkout", "-q", "-b", "topic")
	f.write("a.txt", "topic\n")
	f.commitAll("Conflicting change")
	conflicting := f.head()
	f.write("b.txt", "clean\n")
	f.commitAll("Clean change")
	clean := f.head()
	f.git("checkout", "-q", "main")
	f.write("a.txt", "main\n")
	f.commitAll("Main change")
	f.open()

	f.w.CherryPick([]string{conflicting, clean})
	f.sync()
	if f.w.Repo.Operation.Kind != domain.OpCherryPick || len(f.w.Repo.Conflicts) != 1 {
		t.Fatalf("cherry-pick state: %+v conflicts=%d", f.w.Repo.Operation, len(f.w.Repo.Conflicts))
	}
	f.w.OperationAction("skip")
	f.sync()
	if f.w.Repo.Operation.Active() || f.subject() != "Clean change" {
		t.Fatalf("after skip: op=%+v subject=%q", f.w.Repo.Operation, f.subject())
	}
	before := f.head()
	f.w.CherryPick([]string{conflicting})
	f.sync()
	f.w.OperationAction("abort")
	f.sync()
	if f.w.Repo.Operation.Active() || f.head() != before || f.git("status", "--porcelain") != "" {
		t.Fatalf("abort left op=%+v head=%s status=%q", f.w.Repo.Operation, f.head(), f.git("status", "--porcelain"))
	}
}

func TestScenarioRevertConflictAbort(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("One")
	f.write("a.txt", "two\n")
	f.commitAll("Two")
	two := f.head()
	f.write("a.txt", "three\n")
	f.commitAll("Three")
	f.open()
	f.w.RevertCommits([]string{two})
	f.sync()
	if f.w.Repo.Operation.Kind != domain.OpRevert {
		t.Fatalf("revert of a buried change should stop on a conflict: %+v", f.w.Repo.Operation)
	}
	f.w.OperationAction("abort")
	f.sync()
	if f.w.Repo.Operation.Active() || f.subject() != "Three" {
		t.Fatalf("after abort: %+v %q", f.w.Repo.Operation, f.subject())
	}
}

func TestScenarioRebaseResolveContinue(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.commitAll("Base")
	f.git("checkout", "-q", "-b", "feature")
	f.write("a.txt", "feature\n")
	f.commitAll("Feature edit")
	f.write("f.txt", "more\n")
	f.commitAll("Feature second")
	f.git("checkout", "-q", "main")
	f.write("a.txt", "main\n")
	f.commitAll("Main edit")
	f.git("checkout", "-q", "feature")
	f.open()

	f.w.Rebase("main")
	f.sync()
	rs := f.w.Repo
	if rs.Operation.Kind != domain.OpRebase || len(rs.Conflicts) != 1 {
		t.Fatalf("rebase state %+v conflicts=%d", rs.Operation, len(rs.Conflicts))
	}
	// During a rebase "ours" is the branch being rebased onto.
	view := f.loadDiff(DiffSource{Path: "a.txt", Area: domain.AreaConflicts, Working: true, Kind: domain.ChangeConflicted})
	if !strings.Contains(view.Conflict.OursLabel, "main") || strings.Contains(strings.ToLower(view.Conflict.OursLabel), "yours") {
		t.Fatalf("rebase ours label = %q theirs = %q", view.Conflict.OursLabel, view.Conflict.TheirsLabel)
	}
	f.write("a.txt", "resolved\n")
	f.w.StagePaths([]string{"a.txt"})
	f.sync()
	f.w.OperationAction("continue")
	f.sync()
	if f.w.Repo.Operation.Active() || f.w.Repo.CurrentBranch() != "feature" {
		t.Fatalf("rebase did not finish: %+v branch=%q", f.w.Repo.Operation, f.w.Repo.CurrentBranch())
	}
	if log := f.git("log", "--format=%s", "main..feature"); log != "Feature second\nFeature edit\n" {
		t.Fatalf("rebased commits = %q", log)
	}
	if !strings.Contains(f.git("log", "--format=%s", "-n", "3"), "Main edit") {
		t.Fatal("feature is not on top of main")
	}
}

func TestScenarioStashApplyConflict(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.commitAll("Base")
	f.write("a.txt", "stashed edit\n")
	f.write("new.txt", "untracked\n")
	f.open()
	f.w.StashPush("WIP", true, false)
	f.sync()
	if len(f.w.Repo.Stashes) != 1 || f.git("status", "--porcelain") != "" {
		t.Fatalf("stash push: stashes=%d status=%q", len(f.w.Repo.Stashes), f.git("status", "--porcelain"))
	}
	f.write("a.txt", "committed meanwhile\n")
	f.commitAll("Meanwhile")
	n := len(f.w.Notifications)
	f.w.StashApply(f.w.Repo.Stashes[0], true, false)
	f.sync()
	if len(f.w.Repo.Conflicts) != 1 {
		t.Fatalf("stash pop should conflict on a.txt: %+v", f.w.Repo.Status.Entries)
	}
	if len(f.errorsSince(n)) == 0 && len(f.w.Notifications) == n {
		t.Fatal("a conflicting stash pop must tell the user")
	}
	// A conflicting pop keeps the stash, as git does.
	if len(f.w.Repo.Stashes) != 1 {
		t.Fatalf("stash dropped despite conflict: %d", len(f.w.Repo.Stashes))
	}
	if _, err := os.Stat(filepath.Join(f.dir, "new.txt")); err != nil {
		t.Fatalf("untracked file not restored: %v", err)
	}
}

func TestScenarioHooks(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "x\n")
	f.commitAll("Base")
	hook := filepath.Join(f.dir, ".git", "hooks", "pre-commit")
	os.MkdirAll(filepath.Dir(hook), 0o755)
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint failed: trailing spaces' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.write("a.txt", "y\n")
	f.open()
	f.w.StagePaths([]string{"a.txt"})
	f.sync()
	n := len(f.w.Notifications)
	f.w.Commit(CommitRequest{Message: "Blocked"}, nil)
	f.sync()
	errs := f.errorsSince(n)
	if len(errs) == 0 || !strings.Contains(errs[0].Title+errs[0].Body, "lint failed") {
		t.Fatalf("hook output not surfaced: %+v", errs)
	}
	if f.subject() != "Base" {
		t.Fatal("commit went through despite the hook")
	}
	f.w.Commit(CommitRequest{Message: "Skip hooks", NoVerify: true}, nil)
	f.sync()
	if f.subject() != "Skip hooks" {
		t.Fatalf("--no-verify commit failed: %q", f.subject())
	}
}

// remoteRepo gives the fixture a bare origin and a second clone that pushes to it.
func remoteRepo(t *testing.T) (*fixture, string, string) {
	f := newFixture(t)
	root := filepath.Dir(f.dir)
	bare := filepath.Join(root, "origin.git")
	f.gitIn(root, "init", "-q", "--bare", "-b", "main", bare)
	f.write("a.txt", "base\n")
	f.commitAll("Base")
	f.git("remote", "add", "origin", bare)
	f.git("push", "-q", "-u", "origin", "main")
	other := filepath.Join(root, "other-clone")
	f.gitIn(root, "clone", "-q", bare, other)
	f.gitIn(other, "config", "user.name", "Other")
	f.gitIn(other, "config", "user.email", "other@example.com")
	return f, bare, other
}

func TestScenarioPushRejectedThenPullRebase(t *testing.T) {
	f, _, other := remoteRepo(t)
	os.WriteFile(filepath.Join(other, "b.txt"), []byte("from other\n"), 0o644)
	f.gitIn(other, "add", "-A")
	f.gitIn(other, "commit", "-q", "-m", "Other's work")
	f.gitIn(other, "push", "-q")

	f.write("c.txt", "mine\n")
	f.commitAll("My work")
	f.open()
	n := len(f.w.Notifications)
	f.w.Push(gitcli.PushOptions{Remote: "origin", Branch: "main"}, nil)
	f.sync()
	if errs := f.errorsSince(n); len(errs) == 0 {
		t.Fatal("non-fast-forward push should fail visibly")
	}
	f.w.Pull(gitcli.PullRebase)
	f.sync()
	if log := f.git("log", "--format=%s", "-n", "3"); log != "My work\nOther's work\nBase\n" {
		t.Fatalf("after pull --rebase: %q", log)
	}
	n = len(f.w.Notifications)
	f.w.Push(gitcli.PushOptions{Remote: "origin", Branch: "main"}, nil)
	f.sync()
	if errs := f.errorsSince(n); len(errs) != 0 {
		t.Fatalf("push after rebase failed: %+v", errs)
	}
	if st := f.w.Repo.Status.Branch; st.Ahead != 0 || st.Behind != 0 {
		t.Fatalf("ahead/behind after push = %d/%d", st.Ahead, st.Behind)
	}

	// Amend a pushed commit and push with lease: allowed while nobody else pushed.
	f.git("commit", "-q", "--amend", "-m", "My work (amended)")
	f.sync()
	f.w.Push(gitcli.PushOptions{Remote: "origin", Branch: "main", ForceWithLease: true}, nil)
	f.sync()
	if got := strings.TrimSpace(f.git("log", "-1", "--format=%s", "origin/main")); got != "My work (amended)" {
		t.Fatalf("force-with-lease push: origin/main = %q", got)
	}
	// Someone else pushes again; the lease now protects their work.
	f.gitIn(other, "pull", "-q", "--rebase")
	os.WriteFile(filepath.Join(other, "d.txt"), []byte("later\n"), 0o644)
	f.gitIn(other, "add", "-A")
	f.gitIn(other, "commit", "-q", "-m", "Later work")
	f.gitIn(other, "push", "-q", "--force")
	f.git("commit", "-q", "--amend", "-m", "Second amend")
	n = len(f.w.Notifications)
	f.w.Push(gitcli.PushOptions{Remote: "origin", Branch: "main", ForceWithLease: true}, nil)
	f.sync()
	if errs := f.errorsSince(n); len(errs) == 0 {
		t.Fatal("stale lease must reject the force push")
	}
	if got := strings.TrimSpace(f.gitIn(other, "log", "-1", "--format=%s", "origin/main")); got != "Later work" {
		t.Fatalf("other's work was overwritten: %q", got)
	}
}

func TestScenarioMultipleRemotesAndPrune(t *testing.T) {
	f, bare, _ := remoteRepo(t)
	root := filepath.Dir(f.dir)
	fork := filepath.Join(root, "fork.git")
	f.gitIn(root, "clone", "-q", "--bare", bare, fork)
	f.git("push", "-q", "origin", "main:to-delete")
	f.open()
	f.w.AddRemote("fork", fork)
	f.sync()
	f.w.Fetch(true)
	f.sync()
	names := map[string]bool{}
	for _, r := range f.w.Repo.Remotes {
		names[r.Name] = true
	}
	if !names["origin"] || !names["fork"] {
		t.Fatalf("remotes = %+v", f.w.Repo.Remotes)
	}
	hasRef := func(name string) bool {
		for _, r := range f.w.Repo.Refs {
			if r.Kind == domain.RefRemote && r.Short == name {
				return true
			}
		}
		return false
	}
	if !hasRef("fork/main") || !hasRef("origin/to-delete") {
		t.Fatalf("remote refs after fetch: %+v", f.w.Repo.Refs)
	}
	f.gitIn(root, "--git-dir="+bare, "branch", "-D", "to-delete")
	f.w.Fetch(true) // prune is on by default
	f.sync()
	if hasRef("origin/to-delete") {
		t.Fatal("deleted remote branch survived a pruning fetch")
	}
	f.w.RemoveRemote("fork")
	f.sync()
	if hasRef("fork/main") || len(f.w.Repo.Remotes) != 1 {
		t.Fatalf("remove remote left %+v", f.w.Repo.Remotes)
	}
}

func TestScenarioSubmodule(t *testing.T) {
	f := newFixture(t)
	root := filepath.Dir(f.dir)
	lib := filepath.Join(root, "lib")
	os.MkdirAll(lib, 0o755)
	f.gitIn(lib, "init", "-q", "-b", "main")
	f.gitIn(lib, "config", "user.name", "Lib")
	f.gitIn(lib, "config", "user.email", "lib@example.com")
	os.WriteFile(filepath.Join(lib, "lib.go"), []byte("package lib\n"), 0o644)
	f.gitIn(lib, "add", "-A")
	f.gitIn(lib, "commit", "-q", "-m", "Lib")
	f.write("main.go", "package main\n")
	f.commitAll("Base")
	f.git("-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "vendor/lib")
	f.git("commit", "-q", "-m", "Add submodule")

	// A new commit inside the submodule shows as a submodule change.
	sub := filepath.Join(f.dir, "vendor", "lib")
	f.gitIn(sub, "config", "user.name", "Lib")
	f.gitIn(sub, "config", "user.email", "lib@example.com")
	os.WriteFile(filepath.Join(sub, "more.go"), []byte("package lib\n"), 0o644)
	f.gitIn(sub, "add", "-A")
	f.gitIn(sub, "commit", "-q", "-m", "More")
	f.open()
	e, ok := f.entry("vendor/lib")
	if !ok || !strings.HasPrefix(e.Submodule, "S") {
		t.Fatalf("submodule entry = %+v (found %v)", e, ok)
	}
	res := f.loadDiff(DiffSource{Path: "vendor/lib", Area: domain.AreaUnstaged, Working: true})
	if res.Notice == "" && !res.New.Content.Submodule {
		t.Fatalf("submodule diff should explain itself: %+v", res.New.Content)
	}

	// Opening the submodule's folder opens the submodule repository itself.
	w2 := NewWorkspace(&gitcli.Runner{Env: f.env}, f.w.Settings, nil)
	var openErr error
	opened := false
	w2.Open(sub, func(err error) { openErr, opened = err, true })
	deadline := time.Now().Add(10 * time.Second)
	for !opened && time.Now().Before(deadline) {
		w2.Drain()
		time.Sleep(5 * time.Millisecond)
	}
	if openErr != nil || w2.Repo == nil || w2.Repo.Repo.Root != sub {
		t.Fatalf("open submodule: err=%v repo=%+v", openErr, w2.Repo)
	}
	if !strings.Contains(w2.Repo.Repo.GitDir, filepath.Join(".git", "modules")) {
		t.Fatalf("submodule git dir = %s", w2.Repo.Repo.GitDir)
	}
	w2.Close()
}

func TestScenarioLinkedWorktree(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "base\n")
	f.commitAll("Base")
	wt := filepath.Join(filepath.Dir(f.dir), "wt-feature")
	f.git("worktree", "add", "-q", "-b", "feature", wt)

	main := f.dir
	f.dir = wt // drive the linked worktree
	f.open()
	rs := f.w.Repo
	if rs.Repo.Root != wt || rs.Repo.GitDir == rs.Repo.CommonDir || rs.CurrentBranch() != "feature" {
		t.Fatalf("linked worktree repo = %+v branch=%q", rs.Repo, rs.CurrentBranch())
	}
	f.write("wt.txt", "in worktree\n")
	f.sync()
	f.w.StagePaths([]string{"wt.txt"})
	f.sync()
	f.w.Commit(CommitRequest{Message: "From worktree"}, nil)
	f.sync()
	// Refs are shared: the main checkout sees the new commit on feature.
	if got := strings.TrimSpace(f.gitIn(main, "log", "-1", "--format=%s", "feature")); got != "From worktree" {
		t.Fatalf("main worktree sees feature at %q", got)
	}
	// main is checked out in the other worktree, so checking it out here fails visibly.
	n := len(f.w.Notifications)
	for _, r := range f.w.Repo.Refs {
		if r.Kind == domain.RefLocal && r.Short == "main" {
			f.w.Checkout(r)
		}
	}
	f.sync()
	if len(f.errorsSince(n)) == 0 || f.w.Repo.CurrentBranch() != "feature" {
		t.Fatalf("checkout of a branch used by another worktree: errors=%v branch=%q", f.errorsSince(n), f.w.Repo.CurrentBranch())
	}
	var wts []domain.Worktree
	done := false
	f.w.Worktrees(func(list []domain.Worktree, err error) { wts, done = list, true })
	f.settle(func() bool { return done })
	if len(wts) != 2 {
		t.Fatalf("worktrees = %+v", wts)
	}
}

func TestScenarioOpenSubdirBareAndPlainFolder(t *testing.T) {
	f := newFixture(t)
	f.write("src/deep/a.txt", "x\n")
	f.commitAll("Base")
	open := func(dir string) error {
		w := NewWorkspace(&gitcli.Runner{Env: f.env}, f.w.Settings, nil)
		defer w.Close()
		var err error
		done := false
		w.Open(dir, func(e error) { err, done = e, true })
		deadline := time.Now().Add(10 * time.Second)
		for !done && time.Now().Before(deadline) {
			w.Drain()
			time.Sleep(5 * time.Millisecond)
		}
		if err == nil && (w.Repo == nil || w.Repo.Repo.Root != f.dir) {
			t.Fatalf("open %s: root = %+v", dir, w.Repo)
		}
		return err
	}
	if err := open(filepath.Join(f.dir, "src", "deep")); err != nil {
		t.Fatalf("subdirectory should open its repository: %v", err)
	}
	root := filepath.Dir(f.dir)
	bare := filepath.Join(root, "bare.git")
	f.gitIn(root, "init", "-q", "--bare", bare)
	if err := open(bare); err == nil {
		t.Fatal("a bare repository has no working tree and must be refused")
	}
	plain := filepath.Join(root, "plain")
	os.MkdirAll(plain, 0o755)
	if err := open(plain); err == nil {
		t.Fatal("a folder outside any repository must be refused")
	}
}

func TestScenarioShallowCloneIgnoredLargeAndCRLF(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 5; i++ {
		f.write("a.txt", strings.Repeat("v\n", i+1))
		f.commitAll("C" + string(rune('0'+i)))
	}
	root := filepath.Dir(f.dir)
	shallow := filepath.Join(root, "shallow")
	f.gitIn(root, "clone", "-q", "--depth", "1", "file://"+f.dir, shallow)
	f.gitIn(shallow, "config", "user.name", "S")
	f.gitIn(shallow, "config", "user.email", "s@example.com")
	f.dir = shallow
	f.open()
	if n := len(f.w.Repo.Log.Commits); n != 1 {
		t.Fatalf("shallow clone log has %d commits", n)
	}

	f.write(".gitignore", "build/\n*.log\n")
	f.write("build/out.o", "obj")
	f.write("debug.log", "log")
	f.write("big.dat", strings.Repeat("0123456789abcdef", (gitcli.ContentLimit/16)+64))
	f.write("crlf.txt", "one\r\ntwo\r\nthree\r\n")
	f.commitAll("Files")
	f.write("crlf.txt", "one\r\nTWO\r\nthree\r\n")
	f.write("big.dat", strings.Repeat("fedcba9876543210", (gitcli.ContentLimit/16)+64))
	f.sync()
	for _, e := range f.w.Repo.Status.Entries {
		if strings.HasPrefix(e.Path, "build/") || e.Path == "debug.log" {
			t.Fatalf("ignored file listed: %+v", e)
		}
	}
	crlf := f.loadDiff(DiffSource{Path: "crlf.txt", Area: domain.AreaUnstaged, Working: true})
	if len(crlf.Blocks) != 1 {
		t.Fatalf("CRLF edit of one line: %d blocks", len(crlf.Blocks))
	}
	start := time.Now()
	big := f.loadDiff(DiffSource{Path: "big.dat", Area: domain.AreaUnstaged, Working: true})
	if big.Notice == "" || len(big.Blocks) != 0 || !big.New.Content.TooLarge {
		t.Fatalf("large file should get a restricted preview: notice=%q blocks=%d", big.Notice, len(big.Blocks))
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("large file diff took %v", d)
	}
}

func TestScenarioResetModes(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "1\n")
	f.commitAll("One")
	one := f.head()
	f.write("a.txt", "2\n")
	f.commitAll("Two")
	f.open()
	f.w.Reset(gitcli.ResetSoft, one)
	f.sync()
	if f.head() != one || !strings.HasPrefix(f.git("status", "--porcelain"), "M  a.txt") {
		t.Fatalf("soft reset: head=%s status=%q", f.head(), f.git("status", "--porcelain"))
	}
	f.git("commit", "-q", "-m", "Two again")
	f.w.Reset(gitcli.ResetMixed, one)
	f.sync()
	if !strings.HasPrefix(f.git("status", "--porcelain"), " M a.txt") {
		t.Fatalf("mixed reset status = %q", f.git("status", "--porcelain"))
	}
	f.w.Reset(gitcli.ResetHard, one)
	f.sync()
	if f.git("status", "--porcelain") != "" || f.head() != one {
		t.Fatalf("hard reset left %q", f.git("status", "--porcelain"))
	}
	// Reflog still knows the lost commit: a branch can be made from it.
	var entries []domain.ReflogEntry
	done := false
	f.w.Reflog(func(e []domain.ReflogEntry, err error) { entries, done = e, true })
	f.settle(func() bool { return done })
	var lost string
	for _, e := range entries {
		if strings.Contains(e.Message, "Two again") {
			lost = e.Hash
		}
	}
	if lost == "" {
		t.Fatalf("reflog lacks the reset-away commit: %+v", entries)
	}
	f.w.NewBranch("recovered", lost, false, false)
	f.sync()
	if got := strings.TrimSpace(f.git("log", "-1", "--format=%s", "recovered")); got != "Two again" {
		t.Fatalf("recovered branch at %q", got)
	}
}
