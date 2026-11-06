package app

import (
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// TestSubmoduleCommands drives the workspace commands the UI uses: list,
// update one submodule back to the recorded commit, sync a changed URL and
// report a failure through a notification.
func TestSubmoduleCommands(t *testing.T) {
	child := newFixture(t)
	child.write("lib.txt", "one\n")
	child.commitAll("child one")
	child.write("lib.txt", "two\n")
	child.commitAll("child two")
	want := strings.TrimSpace(child.git("rev-parse", "HEAD"))

	f := newFixture(t)
	f.write("p.txt", "parent\n")
	f.commitAll("parent")
	// Local submodules use the file transport, which Git only allows when
	// asked; production repositories use https or ssh. The workspace holds the
	// backend contract, so the Git runner is addressed directly here: the test
	// is about Git's own transport policy, not about the contract.
	runner := f.w.Git.(*gitcli.Runner)
	runner.Env = append(runner.Env, "GIT_ALLOW_PROTOCOL=file")
	f.git("-c", "protocol.file.allow=always", "submodule", "add", child.dir, "deps/lib")
	f.commitAll("add submodule")
	f.open()

	var (
		subs   []domain.Submodule
		listEr error
		loaded bool
	)
	f.w.Submodules(func(list []domain.Submodule, err error) {
		subs, listEr, loaded = list, err, true
	})
	f.settle(func() bool { return loaded })
	if listEr != nil {
		t.Fatal(listEr)
	}
	if len(subs) != 1 || subs[0].Path != "deps/lib" || subs[0].State != domain.SubmoduleClean || subs[0].SHA != want {
		t.Fatalf("submodules = %+v", subs)
	}

	// An older checkout inside the submodule is reported as modified and the
	// update puts it back.
	f.git("-C", "deps/lib", "checkout", "-q", "HEAD~1")
	f.settle(func() bool { return f.idle() })
	var (
		updEr error
		updOK bool
	)
	f.w.UpdateSubmodule("deps/lib", true, func(err error) { updEr, updOK = err, true })
	f.settle(func() bool { return updOK && f.idle() })
	if updEr != nil {
		t.Fatalf("update: %v", updEr)
	}
	if got := strings.TrimSpace(f.git("-C", "deps/lib", "rev-parse", "HEAD")); got != want {
		t.Fatalf("submodule HEAD = %s, want %s", got, want)
	}

	// Sync writes the .gitmodules URL into .git/config. The URL is rewritten
	// through Git itself: .gitmodules escapes backslashes (Windows paths) and
	// quotes, so editing the raw text does not match what Git stored.
	other := newFixture(t)
	other.write("lib.txt", "other\n")
	other.commitAll("other")
	modules := filepath.Join(f.dir, ".gitmodules")
	f.git("config", "-f", modules, "submodule.deps/lib.url", other.dir)
	var (
		syncEr error
		syncOK bool
	)
	f.w.SyncSubmodules("deps/lib", func(err error) { syncEr, syncOK = err, true })
	f.settle(func() bool { return syncOK && f.idle() })
	if syncEr != nil {
		t.Fatalf("sync: %v", syncEr)
	}
	if got := strings.TrimSpace(f.git("config", "--get", "submodule.deps/lib.url")); got != other.dir {
		t.Fatalf("submodule url = %q, want %q", got, other.dir)
	}

	// A failure reaches the caller and the notification list.
	before := len(f.w.Notifications)
	var badEr error
	badDone := false
	f.w.UpdateSubmodule("no-such-submodule", false, func(err error) { badEr, badDone = err, true })
	f.settle(func() bool { return badDone && f.idle() })
	if badEr == nil {
		t.Fatal("updating a missing submodule succeeded")
	}
	if notices := f.errorsSince(before); len(notices) == 0 {
		t.Fatal("no error notification for a failed update")
	}
}
