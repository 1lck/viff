package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testGit runs Git in dir with the fixture's isolated environment. The fixture
// helper only runs in its main repository; discovery tests need several.
func testGit(t *testing.T, f *fixture, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), f.env...), "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func mustTestGit(t *testing.T, f *fixture, dir string, args ...string) string {
	t.Helper()
	out, err := testGit(t, f, dir, args...)
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return out
}

func scanRepositories(t *testing.T, f *fixture, root string, opts DiscoveryOptions) (DiscoveryResult, error) {
	t.Helper()
	var (
		res  DiscoveryResult
		err  error
		done bool
	)
	f.w.DiscoverRepositories(root, opts, func(r DiscoveryResult, scanErr error) {
		res, err, done = r, scanErr, true
	})
	f.settle(func() bool { return done })
	return res, err
}

func discoveredSet(res DiscoveryResult) map[string]bool {
	got := make(map[string]bool, len(res.Repositories))
	for _, repo := range res.Repositories {
		got[canonicalPath(repo.Root)] = true
	}
	return got
}

// TestDiscoverRepositoriesFindsNestedAndLinkedWorktrees covers the layouts a
// workspace folder can have: sibling repositories, a repository nested inside
// another one, a linked worktree (a `.git` file) and a submodule (also a `.git`
// file). All of them are separate working trees and must be listed.
func TestDiscoverRepositoriesFindsNestedAndLinkedWorktrees(t *testing.T) {
	f := newFixture(t)
	root := filepath.Dir(f.dir)
	sibling := filepath.Join(root, "sibling")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	mustTestGit(t, f, sibling, "init", "-q", "-b", "main")

	inner := filepath.Join(f.dir, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	mustTestGit(t, f, inner, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(inner, "inner.txt"), []byte("inner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustTestGit(t, f, inner, "-c", "user.name=Viff Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "add", "-A")
	mustTestGit(t, f, inner, "-c", "user.name=Viff Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "commit", "-q", "-m", "seed inner")

	// A worktree needs a commit to point at.
	if err := os.WriteFile(filepath.Join(f.dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustTestGit(t, f, f.dir, "-c", "user.name=Viff Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "add", "-A")
	mustTestGit(t, f, f.dir, "-c", "user.name=Viff Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "commit", "-q", "-m", "seed")

	wt := filepath.Join(root, "linked-wt")
	mustTestGit(t, f, f.dir, "worktree", "add", "-q", "-b", "topic", wt)

	// A submodule is a repository inside another repository whose `.git` is a
	// file pointing at the parent's modules directory.
	subSrc := filepath.Join(root, "subsrc")
	if err := os.MkdirAll(subSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	mustTestGit(t, f, subSrc, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(subSrc, "sub.txt"), []byte("sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustTestGit(t, f, subSrc, "-c", "user.name=Viff Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "add", "-A")
	mustTestGit(t, f, subSrc, "-c", "user.name=Viff Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "commit", "-q", "-m", "seed submodule")
	sub := filepath.Join(f.dir, "sub")
	if out, err := testGit(t, f, f.dir, "-c", "protocol.file.allow=always",
		"-c", "user.name=Viff Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "submodule", "add", subSrc, "sub"); err != nil {
		t.Skipf("Git cannot create a local submodule in this environment: %v\n%s", err, out)
	}

	res, err := scanRepositories(t, f, root, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	got := discoveredSet(res)
	for _, want := range []string{f.dir, sibling, inner, wt, sub} {
		if !got[canonicalPath(want)] {
			t.Errorf("repository %s was not discovered; got %v", want, res.Repositories)
		}
	}
	if res.Truncated {
		t.Errorf("scan of a small tree was truncated: %+v", res)
	}
}

// TestDiscoverRepositoriesSkipsBuildTreesAndHonoursLimits pins the budget: a
// repository inside node_modules is skipped by name, a repository below the
// depth limit is not reported, and both limits report Truncated instead of
// pretending the tree ended.
func TestDiscoverRepositoriesSkipsBuildTreesAndHonoursLimits(t *testing.T) {
	f := newFixture(t)
	scanRoot := filepath.Join(f.dir, "workspace")
	deep := filepath.Join(scanRoot, "a", "b", "c", "d", "deep")
	shallow := filepath.Join(scanRoot, "a", "b", "shallow")
	deps := filepath.Join(scanRoot, "node_modules", "dependency")
	for _, dir := range []string{deep, shallow, deps} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		mustTestGit(t, f, dir, "init", "-q", "-b", "main")
	}

	res, err := scanRepositories(t, f, scanRoot, DiscoveryOptions{MaxDepth: 3})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	got := discoveredSet(res)
	if !got[canonicalPath(shallow)] {
		t.Errorf("repository at depth 3 was not discovered: %v", res.Repositories)
	}
	if got[canonicalPath(deep)] {
		t.Errorf("repository below MaxDepth was discovered: %v", res.Repositories)
	}
	if got[canonicalPath(deps)] {
		t.Errorf("repository inside node_modules was discovered: %v", res.Repositories)
	}

	res, err = scanRepositories(t, f, scanRoot, DiscoveryOptions{MaxDepth: 10, MaxRepos: 1})
	if err != nil {
		t.Fatalf("limited scan: %v", err)
	}
	if len(res.Repositories) != 1 || !res.Truncated {
		t.Fatalf("MaxRepos=1: got %d repositories, truncated=%v", len(res.Repositories), res.Truncated)
	}

	res, err = scanRepositories(t, f, scanRoot, DiscoveryOptions{MaxDepth: 10, MaxDirs: 1})
	if err != nil {
		t.Fatalf("directory-limited scan: %v", err)
	}
	if !res.Truncated {
		t.Fatalf("MaxDirs=1 did not report truncation: %+v", res)
	}
}

// TestDiscoverRepositoriesCancelledBeforeWalking keeps the cancellation path
// honest: a cancelled read must stop instead of returning a partial list as if
// it were complete.
func TestDiscoverRepositoriesCancelledBeforeWalking(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.w.scanRepositories(ctx, f.dir, DiscoveryOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scan error = %v, want context.Canceled", err)
	}
}

// TestAddRepositoriesBatchAndDeduplicates adds the same repository twice in one
// batch and expects one row, one skipped entry and a persisted workspace list.
func TestAddRepositoriesBatchAndDeduplicates(t *testing.T) {
	f := newFixture(t)
	var (
		res  AddRepositoriesResult
		done bool
	)
	f.w.AddRepositories([]string{f.dir, f.dir}, func(r AddRepositoriesResult) {
		res, done = r, true
	})
	f.settle(func() bool { return done })
	if res.Added != 1 || res.Skipped != 1 || res.Failed != 0 {
		t.Fatalf("batch result = %+v, want added 1, skipped 1", res)
	}
	if len(f.w.Repositories) != 1 {
		t.Fatalf("workspace rows = %d, want 1", len(f.w.Repositories))
	}
	if canonicalPath(f.w.Repositories[0].Root) != canonicalPath(f.dir) {
		t.Fatalf("row root = %s, want %s", f.w.Repositories[0].Root, f.dir)
	}
}

// TestIsNotRepository makes sure the UI's branch from Git's raw error is based
// on the error kind, not on the message text.
func TestIsNotRepository(t *testing.T) {
	f := newFixture(t)
	_, err := f.w.Git.Discover(context.Background(), t.TempDir())
	if !IsNotRepository(err) {
		t.Fatalf("Discover outside a repository = %v, want ErrNotRepository", err)
	}
	if IsNotRepository(nil) {
		t.Fatal("nil error reported as ErrNotRepository")
	}
}
