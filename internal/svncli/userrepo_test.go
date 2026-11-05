package svncli

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// TestUserDemoRepo replays the exact scenario that failed: a working copy whose
// root is trunk, opening revision 4's detail and reading both diff sides.
func TestUserDemoRepo(t *testing.T) {
	wc := os.Getenv("VIFF_SVN_WC")
	if wc == "" {
		t.Skip("set VIFF_SVN_WC to a working copy to inspect it")
	}
	svn := os.Getenv("VIFF_SVN")
	if svn == "" {
		p, lerr := exec.LookPath("svn")
		if lerr != nil {
			t.Skip("svn is not installed; set VIFF_SVN to inspect a working copy")
		}
		svn = p
	}
	r := &Runner{Path: svn}
	ctx := context.Background()

	repo, err := r.Discover(ctx, wc)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	t.Logf("root=%s url=%s rel=%s", repo.Root, repo.URL, repo.RelURL)

	reader, err := r.OpenLog(ctx, repo, vcs.LogFilter{})
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer reader.Close()
	commits, _, err := reader.Next(10)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	t.Logf("commits: %d", len(commits))

	for _, c := range commits {
		detail, derr := r.CommitDetail(ctx, repo, c.Hash)
		if derr != nil {
			t.Fatalf("detail r%s: %v", c.Hash, derr)
		}
		t.Logf("r%s %q changes=%d", c.Hash, c.Subject, len(detail.Changes))
		for _, ch := range detail.Changes {
			// The two sides the diff view reads.
			oldC, oerr := r.Read(ctx, repo, vcs.RevCommit(c.Hash), ch.Path)
			newC, nerr := r.Read(ctx, repo, vcs.RevCommit(prevRev(commits, c.Hash)), ch.Path)
			if oerr != nil {
				t.Fatalf("r%s read %q at %s: %v", c.Hash, ch.Path, c.Hash, oerr)
			}
			if nerr != nil {
				t.Fatalf("r%s read %q at prev: %v", c.Hash, ch.Path, nerr)
			}
			t.Logf("  %-8s %-24s new(exists=%v %dB) old(exists=%v %dB)",
				domain.ChangeKind(ch.Kind).Letter(), ch.Path,
				newC.Exists, len(newC.Data), oldC.Exists, len(oldC.Data))
		}
	}
	// Working-copy diff side.
	cw, err := r.Read(ctx, repo, vcs.RevIndex, "notes.txt")
	if err != nil {
		t.Fatalf("read base: %v", err)
	}
	t.Logf("base notes.txt: exists=%v %q", cw.Exists, cw.Data)
}

func prevRev(commits []domain.Commit, hash string) string {
	for i, c := range commits {
		if c.Hash == hash && i+1 < len(commits) {
			return commits[i+1].Hash
		}
	}
	return "1"
}
