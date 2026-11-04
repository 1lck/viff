package gitcli

// Regression coverage for Git issues reported against Lithe (1lck/Lithe-IDEA),
// exercised through Viff's Git CLI adapter with real temporary repositories.
// These tests diagnose behaviour only; they do not change product code.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Lithe #1445: ignoring something must not put it under version control, but an
// ignore rule also must not hide a file Git already tracks. Status is the only
// input the app stages from, so this is the layer that decides both cases.
func TestIssue1445IgnoredUntrackedVersusTracked(t *testing.T) {
	tr := newTestRepo(t)
	tr.write(".gitignore", "*.log\n")
	tr.write("debug.log", "untracked\n")

	st := tr.status()
	if e, ok := find(st, "debug.log"); ok {
		t.Fatalf("ignored untracked file is listed and would be staged: %+v", e)
	}

	// A file that matches the rule but is already tracked stays visible.
	tr.write("kept.log", "one\n")
	tr.git("add", "-f", "kept.log")
	tr.git("commit", "-q", "-m", "track kept.log")
	tr.write("kept.log", "two\n")

	st = tr.status()
	e, ok := find(st, "kept.log")
	if !ok {
		t.Fatalf("tracked file disappeared behind an ignore rule: %+v", st.Entries)
	}
	if e.Worktree != 'M' {
		t.Fatalf("tracked modified file = %+v, want worktree M", e)
	}

	// Staging everything the status reports must not add the ignored file.
	var paths []string
	for _, entry := range st.Entries {
		paths = append(paths, entry.Path)
	}
	if err := tr.r.Stage(ctx, tr.repo, paths); err != nil {
		t.Fatal(err)
	}
	if out, err := tr.gitMayFail("ls-files", "--error-unmatch", "debug.log"); err == nil {
		t.Fatalf("ignored file entered the index:\n%s", out)
	}
}

// Lithe #1652: the Log "load more" failed after the cursor session was idle for
// 120 seconds. Viff has no server-side cursor: a LogReader wraps one live
// git log process, and only an explicit reload/close ends it. No test sleeps
// 120s; this drives the implementation path across an idle pause and checks
// that pages keep coming with no cursor error.
func TestIssue1652LogPaginationHasNoCursorExpiry(t *testing.T) {
	tr := newTestRepo(t)
	const commits = 250
	fastImportHistory(t, tr, commits)

	reader, err := tr.r.OpenLog(ctx, tr.repo, LogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	page, done, err := reader.Next(100)
	if err != nil || len(page) != 100 || done {
		t.Fatalf("first page = %d done=%v err=%v", len(page), done, err)
	}

	// The idle window that used to invalidate a cursor. No goroutine, timer or
	// deadline is attached to the reader, so any pause is just time passing.
	time.Sleep(300 * time.Millisecond)

	page, done, err = reader.Next(100)
	if err != nil {
		t.Fatalf("second page after idle: %v", err)
	}
	if len(page) != 100 || done {
		t.Fatalf("second page = %d done=%v", len(page), done)
	}

	page, done, err = reader.Next(100)
	if err != nil {
		t.Fatalf("third page after idle: %v", err)
	}
	if len(page) != commits-200 || !done {
		t.Fatalf("third page = %d done=%v, want %d done", len(page), done, commits-200)
	}
	if strings.Contains(errString(err), "cursor") {
		t.Fatalf("unexpected cursor error: %v", err)
	}
}

// fastImportHistory writes n linear commits to refs/heads/main in one process.
func fastImportHistory(t *testing.T, tr *testRepo, n int) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < n; i++ {
		msg := fmt.Sprintf("commit %d", i)
		body := fmt.Sprintf("%d\n", i)
		fmt.Fprintf(&b, "commit refs/heads/main\ncommitter Viff Test <test@example.com> %d +0000\ndata %d\n%s\n",
			1700000000+i, len(msg), msg)
		fmt.Fprintf(&b, "M 100644 inline file.txt\ndata %d\n%s\n", len(body), body)
	}
	cmd := exec.Command("git", "fast-import", "--quiet")
	cmd.Dir = tr.dir
	cmd.Env = append(os.Environ(), tr.r.Env...)
	cmd.Stdin = strings.NewReader(b.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fast-import: %v\n%s", err, out)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
