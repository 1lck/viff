//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
)

// makeDiscoverableRepo creates a real repository next to the fixture's
// repository, so scanning the parent folder finds several rows.
func makeDiscoverableRepo(t *testing.T, f *uiFixture, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f.gitIn(dir, "init", "-q", "-b", "main")
	f.gitIn(dir, "config", "user.name", "Ada Lovelace")
	f.gitIn(dir, "config", "user.email", "ada@example.com")
	f.gitIn(dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.gitIn(dir, "add", "-A")
	f.gitIn(dir, "commit", "-q", "-m", "Initial commit")
}

func waitDiscoveryDialog(t *testing.T, f *uiFixture) {
	t.Helper()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && f.win.dialog.title == "Discover Repositories" && len(f.ws.Tasks) == 0
	}, 20*time.Second); err != nil {
		t.Fatalf("discovery dialog never finished scanning: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(80*time.Millisecond, 2*time.Second)
}

// TestOpenNonRepositoryFolderDiscoversNestedRepositories is the regression for
// the "not a git repository" report: opening a folder that holds several
// projects offers discovery instead of Git's error. The candidate list includes
// a repository nested inside the fixture's own repository, and importing them
// adds every root to the workspace.
func TestOpenNonRepositoryFolderDiscoversNestedRepositories(t *testing.T) {
	f := newUIFixture(t)
	parent := filepath.Dir(f.dir)
	repoB := filepath.Join(parent, "repo-b")
	makeDiscoverableRepo(t, f, repoB)
	nested := filepath.Join(f.dir, "nested")
	makeDiscoverableRepo(t, f, nested)

	f.start(1280, 800)
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	// openRepo goes through the same path as File → Open: Git reports
	// ErrNotRepository, the window must not show that error but offer discovery.
	f.win.openRepo(parent)
	waitDiscoveryDialog(t, f)

	if !f.s.HasText("Discover Repositories") {
		t.Fatalf("discovery dialog title missing: %s", strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("nested") || !f.s.HasText("repo-b") || !f.s.HasText("viff-demo") {
		t.Fatalf("candidate rows missing:\n%s", strings.Join(f.s.Texts(), " | "))
	}
	if f.s.HasText("not a git repository") {
		t.Fatalf("Git's raw error is still shown: %s", strings.Join(f.s.Texts(), " | "))
	}

	// All candidates start selected; importing adds each root once and lands on
	// the overview, where the rows can be inspected.
	f.click("Import Selected")
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return len(f.ws.Repositories) == 3 && f.win.dialog != nil && f.win.dialog.title == "Repositories"
	}, 20*time.Second); err != nil {
		t.Fatalf("import did not add three repositories: %v\nrows: %+v\ntexts: %s",
			err, f.ws.Repositories, strings.Join(f.s.Texts(), " | "))
	}
	got := make(map[string]bool, len(f.ws.Repositories))
	for _, s := range f.ws.Repositories {
		got[canonicalRootNoFatal(s.Root)] = true
	}
	for _, want := range []string{f.dir, repoB, nested} {
		if !got[canonicalRootNoFatal(want)] {
			t.Errorf("%s was not imported; rows: %+v", want, f.ws.Repositories)
		}
	}
}

// TestRepositoriesAddNonRepositoryPathStartsDiscovery covers the second entry:
// typing a parent folder into the Repositories overview switches to discovery
// instead of showing "Cannot add repository: not a git repository".
func TestRepositoriesAddNonRepositoryPathStartsDiscovery(t *testing.T) {
	f := newUIFixture(t)
	parent := filepath.Dir(f.dir)
	repoB := filepath.Join(parent, "repo-b")
	makeDiscoverableRepo(t, f, repoB)

	f.start(1280, 800)
	f.win.Invalidate()
	f.s.Settle(60*time.Millisecond, 2*time.Second)

	f.win.openRepositories()
	waitOverview(t, f)
	f.click("Repository path")
	if err := f.s.Type(parent); err != nil {
		t.Fatal(err)
	}
	f.s.Settle(50*time.Millisecond, time.Second)
	f.click("Add")
	waitDiscoveryDialog(t, f)
	if f.s.HasText("Cannot add repository") {
		t.Fatalf("overview showed Git's raw error instead of discovery: %s", strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("viff-demo") || !f.s.HasText("repo-b") {
		t.Fatalf("candidates missing after Add: %s", strings.Join(f.s.Texts(), " | "))
	}
}
