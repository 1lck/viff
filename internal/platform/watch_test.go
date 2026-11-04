package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"viff/internal/domain"
)

func TestClassify(t *testing.T) {
	repo := domain.Repo{Root: "/r", GitDir: "/r/.git", CommonDir: "/r/.git"}
	cases := []struct {
		path           string
		relevant, meta bool
	}{
		{"/r/a.txt", true, false},
		{"/r/.git/HEAD", true, true},
		{"/r/.git/refs/heads/main", true, true},
		{"/r/.git/objects/ab/cdef", false, false},
		{"/r/.git/index.lock", false, false},
	}
	for _, c := range cases {
		r, m := classify(repo, c.path)
		if r != c.relevant || m != c.meta {
			t.Errorf("%s = %v %v", c.path, r, m)
		}
	}
}

func TestWatchReportsChangesOnce(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	cmd := exec.Command("git", "init", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	repo := domain.Repo{Root: dir, GitDir: filepath.Join(dir, ".git"), CommonDir: filepath.Join(dir, ".git")}
	var worktree, meta atomic.Int32
	w, err := Watch(repo, func(m bool) {
		if m {
			meta.Add(1)
		} else {
			worktree.Add(1)
		}
	})
	if err != nil {
		t.Skip("native watcher unavailable:", err)
	}
	defer w.Close()
	time.Sleep(300 * time.Millisecond)
	// A burst of writes is coalesced.
	for i := 0; i < 50; i++ {
		os.WriteFile(filepath.Join(dir, "f"+string(rune('a'+i%26))), []byte("x"), 0o644)
	}
	// The non-macOS backend scans the worktree every six seconds.
	wait := 5 * time.Second
	if runtime.GOOS != "darwin" {
		wait = 12 * time.Second
	}
	deadline := time.Now().Add(wait)
	for worktree.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if worktree.Load() == 0 {
		t.Fatal("no worktree event")
	}
	time.Sleep(600 * time.Millisecond)
	if n := worktree.Load(); n > 3 {
		t.Fatalf("burst produced %d callbacks", n)
	}
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/other\n"), 0o644)
	deadline = time.Now().Add(wait)
	for meta.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if meta.Load() == 0 {
		t.Fatal("no meta event")
	}
}
