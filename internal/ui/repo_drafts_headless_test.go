//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
)

func TestRepositoryDraftsHeadless(t *testing.T) {
	f := newUIFixture(t)
	f.start(1100, 750)
	f.win.openRepo(f.dir)
	f.settle()
	f.click("Commit message")
	if err := f.s.Type("Repository A unfinished draft"); err != nil {
		t.Fatal(err)
	}
	f.snap("01-repository-a-draft")
	b := filepath.Join(filepath.Dir(f.dir), "second-project")
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}
	f.gitIn(b, "init", "-q", "-b", "main")
	f.gitIn(b, "config", "user.name", "Viff test")
	f.gitIn(b, "config", "user.email", "test@example.com")
	f.gitIn(b, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "Initial")
	activate := func(root string) {
		f.win.openRepo(root)
		if err := f.s.WaitFor(func(app.Snapshot) bool { return f.ws.Repo != nil && f.ws.Repo.Repo.Root == root }, 20*time.Second); err != nil {
			t.Fatal(err)
		}
		f.settle()
	}
	activate(b)
	if got := f.win.cm.msg.String(); got != "" {
		t.Fatalf("A draft leaked into B: %q", got)
	}
	f.click("Commit message")
	if err := f.s.Type("Repository B separate draft"); err != nil {
		t.Fatal(err)
	}
	f.snap("02-repository-b-draft")
	activate(f.dir)
	if got := f.win.cm.msg.String(); got != "Repository A unfinished draft" {
		t.Fatalf("A draft lost: %q", got)
	}
	f.snap("03-repository-a-draft-restored")
	activate(b)
	if got := f.win.cm.msg.String(); got != "Repository B separate draft" {
		t.Fatalf("B draft lost: %q", got)
	}
	f.snap("04-repository-b-draft-restored")
}
