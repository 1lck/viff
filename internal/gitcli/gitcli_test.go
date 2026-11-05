package gitcli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"viff/internal/domain"
)

// testRepo is a real repository in a temporary directory that the testing
// package removes when the test ends.
type testRepo struct {
	t    *testing.T
	dir  string
	r    *Runner
	repo domain.Repo
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	dir := t.TempDir()
	// Resolve symlinks (macOS /var → /private/var) so paths compare equal.
	dir, _ = filepath.EvalSymlinks(dir)
	tr := &testRepo{t: t, dir: dir, r: &Runner{Env: isolatedEnv(t)}}
	tr.git("init", "-q", "-b", "main")
	tr.git("config", "user.name", "Viff Test")
	tr.git("config", "user.email", "test@example.com")
	tr.git("config", "commit.gpgsign", "false")
	repo, err := tr.r.Discover(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	tr.repo = repo
	return tr
}

// isolatedEnv keeps the user's global configuration out of tests.
func isolatedEnv(tb testing.TB) []string {
	home := tb.TempDir()
	return []string{"HOME=" + home, "XDG_CONFIG_HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig")}
}

func (tr *testRepo) git(args ...string) string {
	tr.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = tr.dir
	cmd.Env = append(os.Environ(), tr.r.Env...)
	cmd.Env = append(cmd.Env, "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		tr.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (tr *testRepo) gitMayFail(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = tr.dir
	cmd.Env = append(os.Environ(), tr.r.Env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (tr *testRepo) write(name, content string) {
	tr.t.Helper()
	p := filepath.Join(tr.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		tr.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		tr.t.Fatal(err)
	}
}

func (tr *testRepo) commitAll(msg string) {
	tr.git("add", "-A")
	tr.git("commit", "-q", "-m", msg)
}

func (tr *testRepo) status() domain.Status {
	tr.t.Helper()
	st, err := tr.r.Status(context.Background(), tr.repo)
	if err != nil {
		tr.t.Fatal(err)
	}
	return st
}

func find(st domain.Status, path string) (domain.StatusEntry, bool) {
	for _, e := range st.Entries {
		if e.Path == path {
			return e, true
		}
	}
	return domain.StatusEntry{}, false
}

var ctx = context.Background()

func TestStatusPathsAndKinds(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a.txt", "one\n")
	tr.write("dir with space/b.txt", "two\n")
	tr.write("中文/文件.md", "three\n")
	tr.write("-dash.txt", "dash\n")
	tr.commitAll("init")

	st := tr.status()
	if st.Branch.Branch != "main" || st.Branch.Head == "" || !st.Clean() {
		t.Fatalf("clean status = %+v", st)
	}
	tr.write("a.txt", "one\nmore\n")
	tr.git("mv", "dir with space/b.txt", "dir with space/c.txt")
	tr.write("中文/文件.md", "changed\n")
	tr.write("new file.txt", "x")
	if runtime.GOOS != "windows" {
		tr.write("line\nbreak.txt", "nl")
	}
	if err := os.Remove(filepath.Join(tr.dir, "-dash.txt")); err != nil {
		t.Fatal(err)
	}
	st = tr.status()
	if e, ok := find(st, "a.txt"); !ok || !e.Unstaged() || e.Staged() {
		t.Fatalf("a.txt = %+v", e)
	}
	if e, ok := find(st, "dir with space/c.txt"); !ok || e.Index != 'R' || e.OrigPath != "dir with space/b.txt" {
		t.Fatalf("rename = %+v %v", e, st.Entries)
	}
	if e, ok := find(st, "中文/文件.md"); !ok || !e.Unstaged() {
		t.Fatalf("unicode = %+v", st.Entries)
	}
	if e, ok := find(st, "new file.txt"); !ok || !e.Untracked() {
		t.Fatalf("untracked = %+v", e)
	}
	if runtime.GOOS != "windows" {
		if _, ok := find(st, "line\nbreak.txt"); !ok {
			t.Fatalf("newline path missing: %q", st.Entries)
		}
	}
	if e, ok := find(st, "-dash.txt"); !ok || e.Worktree != 'D' {
		t.Fatalf("deleted = %+v", e)
	}

	// Stage and unstage through the pathspec file, including the odd names.
	paths := []string{"a.txt", "中文/文件.md", "new file.txt", "-dash.txt"}
	if runtime.GOOS != "windows" {
		paths = append(paths, "line\nbreak.txt")
	}
	if err := tr.r.Stage(ctx, tr.repo, paths); err != nil {
		t.Fatal(err)
	}
	st = tr.status()
	for _, p := range paths {
		if e, ok := find(st, p); !ok || !e.Staged() || e.Unstaged() {
			t.Fatalf("after stage %q = %+v", p, e)
		}
	}
	if err := tr.r.Unstage(ctx, tr.repo, []string{"a.txt", "new file.txt"}, true); err != nil {
		t.Fatal(err)
	}
	st = tr.status()
	if e, _ := find(st, "a.txt"); e.Staged() || !e.Unstaged() {
		t.Fatalf("after unstage = %+v", e)
	}
	if e, _ := find(st, "new file.txt"); !e.Untracked() {
		t.Fatalf("after unstage new = %+v", e)
	}
}

// TestPathspecIsLiteral keeps a path that Git would read as a glob from
// matching its sibling. Brackets are used instead of "*": they carry the same
// pathspec meaning, but a file name may contain them on Windows, where "*" is
// not a legal name at all.
func TestPathspecIsLiteral(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("x[1].txt", "bracket\n")
	tr.write("x1.txt", "plain\n")
	tr.commitAll("init")
	tr.write("x[1].txt", "changed\n")
	tr.write("x1.txt", "changed\n")
	if err := tr.r.Stage(ctx, tr.repo, []string{"x[1].txt"}); err != nil {
		t.Fatal(err)
	}
	st := tr.status()
	if e, _ := find(st, "x1.txt"); e.Staged() {
		t.Fatal("glob pathspec staged x1.txt")
	}
	if e, _ := find(st, "x[1].txt"); !e.Staged() {
		t.Fatal("literal path not staged")
	}
}

func TestInitialCommitUnstage(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a", "a\n")
	tr.git("add", "a")
	st := tr.status()
	if st.Branch.Head != "" {
		t.Fatalf("head before first commit = %q", st.Branch.Head)
	}
	if err := tr.r.Unstage(ctx, tr.repo, []string{"a"}, false); err != nil {
		t.Fatal(err)
	}
	if e, _ := find(tr.status(), "a"); !e.Untracked() {
		t.Fatalf("a = %+v", e)
	}
	// Log of an empty repository is empty, not an error.
	lr, err := tr.r.OpenLog(ctx, tr.repo, LogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	commits, done, err := lr.Next(10)
	if err != nil || !done || len(commits) != 0 {
		t.Fatalf("empty log = %v %v %v", commits, done, err)
	}
}

func TestCommitAmendAndLog(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a", "1\n")
	tr.git("add", "a")
	if _, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "First\n\nBody line"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		tr.write("a", strings.Repeat("x", i+2)+"\n")
		tr.commitAll("c" + string(rune('a'+i)))
	}
	if _, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "nothing"}); KindOf(err) != ErrNothingToCommit {
		t.Fatalf("empty commit err = %v (%v)", err, KindOf(err))
	}
	msg, _ := tr.r.LastCommitMessage(ctx, tr.repo)
	if msg != "cy" {
		t.Fatalf("last message = %q", msg)
	}
	if _, err := tr.r.Commit(ctx, tr.repo, CommitOptions{Message: "amended", Amend: true}); err != nil {
		t.Fatal(err)
	}
	lr, err := tr.r.OpenLog(ctx, tr.repo, LogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	defer lr.Close()
	page1, done, err := lr.Next(10)
	if err != nil || done || len(page1) != 10 || page1[0].Subject != "amended" {
		t.Fatalf("page1 = %d %v %v %+v", len(page1), done, err, page1[0])
	}
	var all []domain.Commit
	all = append(all, page1...)
	for !done {
		var more []domain.Commit
		more, done, err = lr.Next(10)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, more...)
	}
	if len(all) != 26 || all[25].Subject != "First" || len(all[25].Parents) != 0 || all[0].Parents[0] != all[1].Hash {
		t.Fatalf("log = %d", len(all))
	}
	d, err := tr.r.CommitDetail(ctx, tr.repo, all[25].Hash)
	if err != nil || d.Body != "First\n\nBody line" || len(d.Changes) != 1 || d.Changes[0].Kind != domain.ChangeAdded {
		t.Fatalf("detail = %+v %v", d, err)
	}
	// Filters.
	lr2, _ := tr.r.OpenLog(ctx, tr.repo, LogFilter{Text: "AMEND"})
	got, _, _ := lr2.Next(100)
	if len(got) != 1 {
		t.Fatalf("grep = %d", len(got))
	}
}

func TestBranchesTagsRefs(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a", "1\n")
	tr.commitAll("one")
	if err := tr.r.CreateBranch(ctx, tr.repo, "feature/x", "", true, false); err != nil {
		t.Fatal(err)
	}
	tr.write("a", "2\n")
	tr.commitAll("two")
	if err := tr.r.CreateTag(ctx, tr.repo, "v1", "HEAD", "release"); err != nil {
		t.Fatal(err)
	}
	if err := tr.r.Checkout(ctx, tr.repo, domain.Ref{Kind: domain.RefLocal, Short: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := tr.r.DeleteBranch(ctx, tr.repo, "feature/x", false); KindOf(err) != ErrNotMerged {
		t.Fatalf("delete unmerged = %v", err)
	}
	refs, err := tr.r.Refs(ctx, tr.repo)
	if err != nil {
		t.Fatal(err)
	}
	var sawTag, sawCurrent bool
	head := strings.TrimSpace(tr.git("rev-parse", "feature/x"))
	for _, r := range refs {
		if r.Kind == domain.RefTag && r.Short == "v1" && r.Target == head {
			sawTag = true
		}
		if r.Kind == domain.RefLocal && r.Short == "main" && r.Current {
			sawCurrent = true
		}
	}
	if !sawTag || !sawCurrent {
		t.Fatalf("refs = %+v", refs)
	}
	if err := tr.r.RenameBranch(ctx, tr.repo, "feature/x", "feature/y"); err != nil {
		t.Fatal(err)
	}
	if err := tr.r.DeleteBranch(ctx, tr.repo, "feature/y", true); err != nil {
		t.Fatal(err)
	}
}

// The remote's default branch is read from refs/remotes/<remote>/HEAD and
// nothing else: the query must stay local, so a remote that is unreachable or
// slow cannot hang a refresh, and a remote without the symbolic ref is simply
// absent instead of guessed.
func TestRemoteHeads(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a", "1\n")
	tr.commitAll("one")
	heads, err := tr.r.RemoteHeads(ctx, tr.repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(heads) != 0 {
		t.Fatalf("heads without a remote = %v", heads)
	}
	tr.git("update-ref", "refs/remotes/origin/main", "HEAD")
	tr.git("update-ref", "refs/remotes/origin/topic", "HEAD")
	tr.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	tr.git("update-ref", "refs/remotes/upstream/main", "HEAD")
	heads, err = tr.r.RemoteHeads(ctx, tr.repo)
	if err != nil {
		t.Fatal(err)
	}
	if heads["origin"] != "main" {
		t.Fatalf("heads = %v, want origin -> main", heads)
	}
	// A remote branch is not a default branch, and a remote without HEAD stays
	// out of the map.
	if _, ok := heads["upstream"]; ok {
		t.Fatalf("heads = %v, upstream has no HEAD", heads)
	}
	if heads["origin"] == "topic" {
		t.Fatalf("heads = %v, topic is not the default branch", heads)
	}
}

func TestRemotePushFetchTracking(t *testing.T) {
	remote := newTestRepo(t)
	remote.git("config", "receive.denyCurrentBranch", "updateInstead")
	remote.write("r", "r\n")
	remote.commitAll("remote init")

	tr := newTestRepo(t)
	tr.git("remote", "add", "origin", remote.dir)
	if err := tr.r.Fetch(ctx, tr.repo, "", true, nil); err != nil {
		t.Fatal(err)
	}
	refs, _ := tr.r.Refs(ctx, tr.repo)
	var originMain domain.Ref
	for _, r := range refs {
		if r.Short == "origin/main" {
			originMain = r
		}
	}
	if originMain.Remote() != "origin" || originMain.BranchName() != "main" {
		t.Fatalf("remote ref = %+v", originMain)
	}
	if err := tr.r.CreateBranch(ctx, tr.repo, "work", "origin/main", true, false); err != nil {
		t.Fatal(err)
	}
	tr.write("w", "w\n")
	tr.commitAll("work")
	var lines []string
	if _, err := tr.r.Push(ctx, tr.repo, PushOptions{Remote: "origin", Branch: "work", SetUpstream: true}, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	st := tr.status()
	if st.Branch.Upstream != "origin/work" || !st.Branch.HasAB || st.Branch.Ahead != 0 {
		t.Fatalf("after push = %+v", st.Branch)
	}
	remotes, _ := tr.r.Remotes(ctx, tr.repo)
	if len(remotes) != 1 || remotes[0].FetchURL != remote.dir {
		t.Fatalf("remotes = %+v", remotes)
	}
	// A rejected push is classified.
	remote.git("checkout", "-q", "work")
	remote.write("w", "remote change\n")
	remote.commitAll("remote work")
	remote.git("checkout", "-q", "main")
	tr.write("w", "local change\n")
	tr.commitAll("local work")
	if _, err := tr.r.Push(ctx, tr.repo, PushOptions{Remote: "origin", Branch: "work"}, nil); KindOf(err) != ErrRejected {
		t.Fatalf("push = %v kind %v", err, KindOf(err))
	}
}

func TestMergeConflictStateAndStages(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("f.txt", "base\n")
	tr.write("gone.txt", "keep\n")
	tr.commitAll("base")
	tr.git("checkout", "-q", "-b", "other")
	tr.write("f.txt", "theirs\n")
	tr.write("gone.txt", "modified by them\n")
	tr.commitAll("their change")
	tr.git("checkout", "-q", "main")
	tr.write("f.txt", "ours\n")
	tr.git("rm", "-q", "gone.txt")
	tr.commitAll("our change")
	_, err := tr.r.Merge(ctx, tr.repo, "other", false, false)
	if KindOf(err) != ErrConflict {
		t.Fatalf("merge err = %v kind %v", err, KindOf(err))
	}
	op := tr.r.Operation(ctx, tr.repo)
	if op.Kind != domain.OpMerge || op.IncomingSubject != "their change" {
		t.Fatalf("op = %+v", op)
	}
	st := tr.status()
	conflicts, err := tr.r.Conflicts(ctx, tr.repo, st)
	if err != nil || len(conflicts) != 2 {
		t.Fatalf("conflicts = %+v %v", conflicts, err)
	}
	byPath := map[string]domain.Conflict{}
	for _, c := range conflicts {
		byPath[c.Path] = c
	}
	f := byPath["f.txt"]
	if f.Kind != domain.ConflictBothModified || len(f.Stages) != 3 {
		t.Fatalf("f = %+v", f)
	}
	g := byPath["gone.txt"]
	if g.Kind != domain.ConflictDeletedByUs {
		t.Fatalf("gone = %+v", g)
	}
	if _, ok := g.Stage(2); ok {
		t.Fatal("deleted-by-us should have no stage 2")
	}
	ours, _ := tr.r.Read(ctx, tr.repo, RevStage(2), "f.txt")
	theirs, _ := tr.r.Read(ctx, tr.repo, RevStage(3), "f.txt")
	work, _ := tr.r.Read(ctx, tr.repo, RevWorktree, "f.txt")
	if string(ours.Data) != "ours\n" || string(theirs.Data) != "theirs\n" {
		t.Fatalf("stages = %q %q", ours.Data, theirs.Data)
	}
	lines, _ := domain.SplitLines(string(work.Data))
	if p := domain.ParseConflictMarkers(lines); !p.Reliable || len(p.Blocks) != 1 {
		t.Fatalf("markers = %+v in %q", p, work.Data)
	}
	// An external tool resolves and stages; the state follows Git.
	tr.write("f.txt", "resolved\n")
	tr.git("add", "f.txt")
	tr.git("rm", "-q", "gone.txt")
	st = tr.status()
	if _, _, _, c := st.Counts(); c != 0 {
		t.Fatalf("still conflicted: %+v", st.Entries)
	}
	if _, err := tr.r.OperationCommand(ctx, tr.repo, domain.OpMerge, "continue"); err != nil {
		t.Fatal(err)
	}
	if op := tr.r.Operation(ctx, tr.repo); op.Active() {
		t.Fatalf("op after continue = %+v", op)
	}
}

func TestRebaseConflictAbort(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("f", "base\n")
	tr.commitAll("base")
	tr.git("checkout", "-q", "-b", "topic")
	tr.write("f", "topic\n")
	tr.commitAll("topic change")
	tr.git("checkout", "-q", "main")
	tr.write("f", "main\n")
	tr.commitAll("main change")
	tr.git("checkout", "-q", "topic")
	if _, err := tr.r.Rebase(ctx, tr.repo, "main"); KindOf(err) != ErrConflict {
		t.Fatalf("rebase err = %v kind %v", err, KindOf(err))
	}
	op := tr.r.Operation(ctx, tr.repo)
	if op.Kind != domain.OpRebase || op.HeadName != "topic" || op.Onto == "" || op.IncomingSubject != "topic change" {
		t.Fatalf("op = %+v", op)
	}
	if _, err := tr.r.OperationCommand(ctx, tr.repo, domain.OpRebase, "abort"); err != nil {
		t.Fatal(err)
	}
	if op := tr.r.Operation(ctx, tr.repo); op.Active() {
		t.Fatalf("after abort = %+v", op)
	}
}

func TestApplyPatchHunkAndLine(t *testing.T) {
	tr := newTestRepo(t)
	base := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n"
	tr.write("f", base)
	tr.commitAll("base")
	work := "1\nTWO\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\nFOURTEEN\nnew\n15"
	tr.write("f", work)
	oldLines, oldNoNL := domain.SplitLines(base)
	newLines, newNoNL := domain.SplitLines(work)
	old := domain.PatchSide{Lines: oldLines, NoFinalNewline: oldNoNL}
	nw := domain.PatchSide{Lines: newLines, NoFinalNewline: newNoNL}
	blocks := domain.DiffSides(old, nw, domain.WhitespaceExact)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %v", blocks)
	}
	// Stage only the second hunk.
	second := blocks[1]
	patch := domain.BuildPatch("f", old, nw, blocks,
		domain.SelectRange(second.OldStart, second.OldEnd), domain.SelectRange(second.NewStart, second.NewEnd))
	if err := tr.r.ApplyPatch(ctx, tr.repo, patch, true, false); err != nil {
		t.Fatalf("apply: %v\n%s", err, patch)
	}
	index, _ := tr.r.Read(ctx, tr.repo, RevIndex, "f")
	want := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\nFOURTEEN\nnew\n15"
	if string(index.Data) != want {
		t.Fatalf("index = %q\npatch:\n%s", index.Data, patch)
	}
	// Now stage only the first line of the first hunk's insertion side from a
	// fresh index→worktree diff: line-level staging.
	idxLines, idxNoNL := domain.SplitLines(string(index.Data))
	idx := domain.PatchSide{Lines: idxLines, NoFinalNewline: idxNoNL}
	blocks = domain.DiffSides(idx, nw, domain.WhitespaceExact)
	if len(blocks) != 1 {
		t.Fatalf("remaining = %v", blocks)
	}
	patch = domain.BuildPatch("f", idx, nw, blocks, domain.SelectAll, domain.SelectAll)
	if err := tr.r.ApplyPatch(ctx, tr.repo, patch, true, false); err != nil {
		t.Fatalf("apply2: %v\n%s", err, patch)
	}
	index, _ = tr.r.Read(ctx, tr.repo, RevIndex, "f")
	if string(index.Data) != work {
		t.Fatalf("index2 = %q", index.Data)
	}
	// Unstage the first hunk in reverse against HEAD→index.
	head, _ := tr.r.Read(ctx, tr.repo, RevHead, "f")
	hl, hn := domain.SplitLines(string(head.Data))
	hs := domain.PatchSide{Lines: hl, NoFinalNewline: hn}
	blocks = domain.DiffSides(hs, nw, domain.WhitespaceExact)
	first := blocks[0]
	patch = domain.BuildPatch("f", hs, nw, blocks, domain.SelectRange(first.OldStart, first.OldEnd), domain.SelectRange(first.NewStart, first.NewEnd))
	// Reverse application needs the patch built from the index side; build
	// it as index→HEAD instead.
	rev := domain.BuildPatch("f", nw, hs, domain.DiffSides(nw, hs, domain.WhitespaceExact),
		domain.SelectRange(first.NewStart, first.NewEnd), domain.SelectRange(first.OldStart, first.OldEnd))
	_ = patch
	if err := tr.r.ApplyPatch(ctx, tr.repo, rev, true, false); err != nil {
		t.Fatalf("unstage hunk: %v\n%s", err, rev)
	}
	index, _ = tr.r.Read(ctx, tr.repo, RevIndex, "f")
	if !strings.HasPrefix(string(index.Data), "1\n2\n3\n") || !strings.Contains(string(index.Data), "FOURTEEN") {
		t.Fatalf("index3 = %q", index.Data)
	}
}

func TestPatchFinalNewlineCases(t *testing.T) {
	cases := []struct{ base, work string }{
		{"a\nb", "a\nb\nc\n"},
		{"a\nb\n", "a\nb"},
		{"a\nb", "a\nc"},
		{"", "new\n"},
		{"x\n", ""},
	}
	for _, c := range cases {
		tr := newTestRepo(t)
		tr.write("f", c.base)
		tr.commitAll("base")
		tr.write("f", c.work)
		ol, on := domain.SplitLines(c.base)
		nl, nn := domain.SplitLines(c.work)
		old := domain.PatchSide{Lines: ol, NoFinalNewline: on}
		nw := domain.PatchSide{Lines: nl, NoFinalNewline: nn}
		blocks := domain.DiffSides(old, nw, domain.WhitespaceExact)
		patch := domain.BuildPatch("f", old, nw, blocks, domain.SelectAll, domain.SelectAll)
		if err := tr.r.ApplyPatch(ctx, tr.repo, patch, true, false); err != nil {
			t.Fatalf("%q→%q: %v\n%s", c.base, c.work, err, patch)
		}
		index, _ := tr.r.Read(ctx, tr.repo, RevIndex, "f")
		if string(index.Data) != c.work {
			t.Fatalf("%q→%q: index = %q\n%s", c.base, c.work, index.Data, patch)
		}
	}
}

func TestStashAndReflog(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a", "1\n")
	tr.commitAll("one")
	tr.write("a", "2\n")
	tr.write("u", "untracked\n")
	if err := tr.r.StashPush(ctx, tr.repo, "wip stuff", true, false); err != nil {
		t.Fatal(err)
	}
	if !tr.status().Clean() {
		t.Fatal("not clean after stash")
	}
	list, err := tr.r.Stashes(ctx, tr.repo)
	if err != nil || len(list) != 1 || !strings.Contains(list[0].Message, "wip stuff") || list[0].Ref != "stash@{0}" {
		t.Fatalf("stashes = %+v %v", list, err)
	}
	if st := tr.status(); st.Branch.Stashes != 1 {
		t.Fatalf("stash count = %d", st.Branch.Stashes)
	}
	if _, err := tr.r.StashApply(ctx, tr.repo, "stash@{0}", true, false); err != nil {
		t.Fatal(err)
	}
	if e, ok := find(tr.status(), "u"); !ok || !e.Untracked() {
		t.Fatal("untracked not restored")
	}
	rl, err := tr.r.Reflog(ctx, tr.repo, 50)
	if err != nil || len(rl) == 0 || !strings.HasPrefix(rl[0].Selector, "HEAD@{") {
		t.Fatalf("reflog = %+v %v", rl, err)
	}
}

func TestWorktreesAndDiscover(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("a", "1\n")
	tr.commitAll("one")
	wt := filepath.Join(t.TempDir(), "wt")
	if err := tr.r.AddWorktree(ctx, tr.repo, wt, "side", ""); err != nil {
		t.Fatal(err)
	}
	list, err := tr.r.Worktrees(ctx, tr.repo)
	if err != nil || len(list) != 2 || list[1].Branch != "side" {
		t.Fatalf("worktrees = %+v %v", list, err)
	}
	repo, err := tr.r.Discover(ctx, wt)
	if err != nil || !repo.IsLinkedWorktree() {
		t.Fatalf("linked = %+v %v", repo, err)
	}
	if _, err := tr.r.Discover(ctx, t.TempDir()); KindOf(err) != ErrNotRepository {
		t.Fatalf("not a repo = %v", err)
	}
	if err := tr.r.RemoveWorktree(ctx, tr.repo, wt, false); err != nil {
		t.Fatal(err)
	}
}

func TestReadBinaryAndMissing(t *testing.T) {
	tr := newTestRepo(t)
	tr.write("bin", "a\x00b")
	tr.commitAll("bin")
	c, err := tr.r.Read(ctx, tr.repo, RevHead, "bin")
	if err != nil || !c.Exists || !c.Binary {
		t.Fatalf("binary = %+v %v", c, err)
	}
	c, err = tr.r.Read(ctx, tr.repo, RevHead, "missing")
	if err != nil || c.Exists {
		t.Fatalf("missing = %+v %v", c, err)
	}
	c, err = tr.r.Read(ctx, tr.repo, RevWorktree, "missing")
	if err != nil || c.Exists {
		t.Fatalf("missing worktree = %+v %v", c, err)
	}
}

func TestHeadState(t *testing.T) {
	tr := newTestRepo(t)
	// An unborn branch has no commit yet but still names the branch.
	hs, err := tr.r.Head(ctx, tr.repo)
	if err != nil || hs.Head != "" || hs.Branch != "main" {
		t.Fatalf("unborn head = %+v %v", hs, err)
	}
	tr.write("a", "1\n")
	tr.commitAll("one")
	head := tr.git("rev-parse", "HEAD")
	hs, err = tr.r.Head(ctx, tr.repo)
	if err != nil || hs.Head != strings.TrimSpace(head) || hs.Branch != "main" {
		t.Fatalf("head = %+v %v", hs, err)
	}
	tr.git("checkout", "-q", "--detach", "HEAD")
	hs, err = tr.r.Head(ctx, tr.repo)
	if err != nil || hs.Head != strings.TrimSpace(head) || hs.Branch != "" {
		t.Fatalf("detached head = %+v %v", hs, err)
	}
}
