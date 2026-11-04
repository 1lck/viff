package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testGit runs git in dir with the isolated environment of the test.
func testGit(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRemoteRepo builds an origin bare repository with a base commit on main
// and a "side" branch, plus a local clone that holds both branches.
func newRemoteRepo(t *testing.T) (tr *testRepo, remote, root string) {
	t.Helper()
	root, _ = filepath.EvalSymlinks(t.TempDir())
	env := isolatedEnv(t)
	remote = filepath.Join(root, "origin.git")
	os.MkdirAll(remote, 0o755)
	testGit(t, root, env, "init", "-q", "--bare", "-b", "main", remote)

	dir := filepath.Join(root, "work")
	os.MkdirAll(dir, 0o755)
	tr = &testRepo{t: t, dir: dir, r: &Runner{Env: env}}
	tr.git("init", "-q", "-b", "main")
	tr.git("config", "user.name", "Viff Test")
	tr.git("config", "user.email", "test@example.com")
	tr.git("config", "commit.gpgsign", "false")
	tr.git("remote", "add", "origin", remote)
	tr.write("a.txt", "one\n")
	tr.commitAll("Base")
	tr.git("push", "-q", "-u", "origin", "main")

	tr.git("checkout", "-q", "-b", "side")
	tr.write("side.txt", "side one\n")
	tr.commitAll("Side one")
	tr.git("push", "-q", "-u", "origin", "side")
	tr.git("checkout", "-q", "main")

	repo, err := tr.r.Discover(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	tr.repo = repo
	return tr, remote, root
}

// commitIn writes file in dir with a fresh commit and returns the new hash.
func commitIn(t *testing.T, dir string, env []string, file, content, msg string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, dir, env, "add", "-A")
	testGit(t, dir, env, "commit", "-q", "-m", msg)
	return testGit(t, dir, env, "rev-parse", "HEAD")
}

// cloneForPush clones remote into a second checkout that can push to it.
func cloneForPush(t *testing.T, root, remote string, env []string) string {
	t.Helper()
	dir := filepath.Join(root, "push-"+strings.ReplaceAll(t.Name(), "/", "-")+"-"+filepath.Base(root))
	cmd := exec.Command("git", "clone", "-q", remote, dir)
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	testGit(t, dir, env, "config", "user.name", "Other")
	testGit(t, dir, env, "config", "user.email", "other@example.com")
	testGit(t, dir, env, "config", "commit.gpgsign", "false")
	return dir
}

func TestUpdateBranchFastForwards(t *testing.T) {
	tr, remote, root := newRemoteRepo(t)
	ctx := context.Background()

	// Someone else pushes a commit to the side branch.
	push := cloneForPush(t, root, remote, tr.r.Env)
	testGit(t, push, tr.r.Env, "checkout", "-q", "side")
	tip := commitIn(t, push, tr.r.Env, "side.txt", "side two\n", "Side two")
	testGit(t, push, tr.r.Env, "push", "-q", "origin", "side")

	old := strings.TrimSpace(tr.git("rev-parse", "refs/heads/side"))
	res, err := tr.r.UpdateBranch(ctx, tr.repo, "origin", "side", nil)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if res.UpToDate || res.Commits != 1 {
		t.Fatalf("result = %+v", res)
	}
	if res.Old != old || res.Branch != "side" {
		t.Fatalf("result = %+v, old %s", res, old)
	}
	if res.New != tip {
		t.Fatalf("new = %s, want %s", res.New, tip)
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "refs/heads/side")); got != tip {
		t.Fatalf("branch = %s, want %s", got, tip)
	}
	// The tracking ref follows, so the popup's ahead/behind stays right.
	if got := strings.TrimSpace(tr.git("rev-parse", "refs/remotes/origin/side")); got != tip {
		t.Fatalf("tracking = %s, want %s", got, tip)
	}
	// The working tree was not touched: main is still checked out.
	if got := strings.TrimSpace(tr.git("rev-parse", "HEAD")); got == tip {
		t.Fatal("checkout moved")
	}

	// A second update has nothing to do.
	res, err = tr.r.UpdateBranch(ctx, tr.repo, "origin", "side", nil)
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if !res.UpToDate || res.Commits != 0 || res.New != tip {
		t.Fatalf("second result = %+v", res)
	}
}

func TestUpdateBranchRefusesDiverged(t *testing.T) {
	tr, remote, root := newRemoteRepo(t)
	ctx := context.Background()

	// The local branch gains a commit the remote does not have.
	tr.git("checkout", "-q", "side")
	tr.write("side.txt", "local work\n")
	tr.commitAll("Local side")
	local := strings.TrimSpace(tr.git("rev-parse", "refs/heads/side"))
	tr.git("checkout", "-q", "main")

	// The remote branch gains another one.
	push := cloneForPush(t, root, remote, tr.r.Env)
	testGit(t, push, tr.r.Env, "checkout", "-q", "side")
	commitIn(t, push, tr.r.Env, "side.txt", "remote work\n", "Remote side")
	testGit(t, push, tr.r.Env, "push", "-q", "origin", "side")

	_, err := tr.r.UpdateBranch(ctx, tr.repo, "origin", "side", nil)
	if !errors.Is(err, ErrBranchDiverged) {
		t.Fatalf("err = %v, want ErrBranchDiverged", err)
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "refs/heads/side")); got != local {
		t.Fatalf("branch moved to %s, want %s", got, local)
	}
}

func TestUpdateBranchRefusesCheckedOut(t *testing.T) {
	tr, _, root := newRemoteRepo(t)
	ctx := context.Background()

	if _, err := tr.r.UpdateBranch(ctx, tr.repo, "origin", "main", nil); !errors.Is(err, ErrBranchCheckedOut) {
		t.Fatalf("current branch err = %v, want ErrBranchCheckedOut", err)
	}

	// A linked worktree holding the branch blocks the update too.
	wt := filepath.Join(root, "wt-side")
	tr.git("worktree", "add", "-q", wt, "side")
	if _, err := tr.r.UpdateBranch(ctx, tr.repo, "origin", "side", nil); !errors.Is(err, ErrBranchCheckedOut) {
		t.Fatalf("other worktree err = %v, want ErrBranchCheckedOut", err)
	}
	if out := tr.git("worktree", "list", "--porcelain"); !strings.Contains(out, wt) {
		t.Fatalf("worktree went away:\n%s", out)
	}
}

func TestUnshallow(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	env := isolatedEnv(t)
	remote := filepath.Join(root, "origin.git")
	os.MkdirAll(remote, 0o755)
	testGit(t, root, env, "init", "-q", "--bare", "-b", "main", remote)

	seed := filepath.Join(root, "seed")
	os.MkdirAll(seed, 0o755)
	testGit(t, seed, env, "init", "-q", "-b", "main")
	testGit(t, seed, env, "config", "user.name", "Viff Test")
	testGit(t, seed, env, "config", "user.email", "test@example.com")
	testGit(t, seed, env, "config", "commit.gpgsign", "false")
	testGit(t, seed, env, "remote", "add", "origin", remote)
	for i, msg := range []string{"One", "Two", "Three"} {
		commitIn(t, seed, env, "a.txt", strings.Repeat("x\n", i+1), msg)
	}
	testGit(t, seed, env, "push", "-q", "-u", "origin", "main")

	dir := filepath.Join(root, "shallow")
	testGit(t, root, env, "clone", "-q", "--depth", "1", "file://"+remote, dir)
	r := &Runner{Env: env}
	repo, err := r.Discover(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	shallow, err := r.IsShallow(ctx, repo)
	if err != nil || !shallow {
		t.Fatalf("IsShallow = %v, %v", shallow, err)
	}
	before := strings.TrimSpace(testGit(t, dir, env, "rev-list", "--count", "HEAD"))
	if before != "1" {
		t.Fatalf("shallow clone has %s commits", before)
	}

	if err := r.Unshallow(ctx, repo, nil); err != nil {
		t.Fatalf("unshallow: %v", err)
	}
	if shallow, err := r.IsShallow(ctx, repo); err != nil || shallow {
		t.Fatalf("after unshallow IsShallow = %v, %v", shallow, err)
	}
	after := strings.TrimSpace(testGit(t, dir, env, "rev-list", "--count", "HEAD"))
	if after != "3" {
		t.Fatalf("after unshallow HEAD has %s commits, want 3", after)
	}
}

func TestIsShallowOnCompleteRepository(t *testing.T) {
	tr, _, _ := newRemoteRepo(t)
	if shallow, err := tr.r.IsShallow(context.Background(), tr.repo); err != nil || shallow {
		t.Fatalf("IsShallow = %v, %v", shallow, err)
	}
}

// TestUpdateBranchMissingBranch documents the error for a branch that does
// not exist locally: the caller never sees a moved ref.
func TestUpdateBranchMissingBranch(t *testing.T) {
	tr, _, _ := newRemoteRepo(t)
	res, err := tr.r.UpdateBranch(context.Background(), tr.repo, "origin", "nope", nil)
	if err == nil {
		t.Fatalf("update of a missing branch succeeded: %+v", res)
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v", err)
	}
}

// TestUpdateBranchLocalAheadIsUpToDate documents that unpushed commits are
// not a divergence: the remote has nothing newer, so there is nothing to do.
func TestUpdateBranchLocalAheadIsUpToDate(t *testing.T) {
	tr, _, _ := newRemoteRepo(t)

	tr.git("checkout", "-q", "side")
	tr.write("side.txt", "local only\n")
	tr.commitAll("Local only")
	local := strings.TrimSpace(tr.git("rev-parse", "refs/heads/side"))
	tr.git("checkout", "-q", "main")

	res, err := tr.r.UpdateBranch(context.Background(), tr.repo, "origin", "side", nil)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !res.UpToDate || res.New != local || res.Commits != 0 {
		t.Fatalf("result = %+v", res)
	}
	if got := strings.TrimSpace(tr.git("rev-parse", "refs/heads/side")); got != local {
		t.Fatalf("branch = %s, want %s", got, local)
	}
}
