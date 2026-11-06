package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// TestSubmoduleManageCommands drives the add, deinit and remove use cases
// through the Workspace: every step must refresh the repository, report Git's
// failures instead of a success, and leave real Git state behind.
func TestSubmoduleManageCommands(t *testing.T) {
	child := newFixture(t)
	child.write("lib.txt", "v1\n")
	child.commitAll("child")

	f := newFixture(t)
	f.write("p.txt", "parent\n")
	f.commitAll("parent")
	// Local submodules use the file transport, which Git only allows when
	// asked; production repositories use https or ssh. The workspace holds the
	// backend contract, so the Git runner is addressed directly here: the test
	// is about Git's own transport policy, not about the contract.
	runner := f.w.Git.(*gitcli.Runner)
	runner.Env = append(runner.Env, "GIT_ALLOW_PROTOCOL=file")
	f.open()

	var (
		direct   map[string]bool
		directEr error
		directOK bool
	)
	f.w.DirectSubmodules(func(paths map[string]bool, err error) { direct, directEr, directOK = paths, err, true })
	f.settle(func() bool { return directOK })
	if directEr != nil || len(direct) != 0 {
		t.Fatalf("direct submodules = %v, %v", direct, directEr)
	}

	const path = "deps/lib dir"
	var (
		addEr error
		addOK bool
	)
	f.w.AddSubmodule(child.dir, path, func(err error) { addEr, addOK = err, true })
	f.settle(func() bool { return addOK && f.idle() })
	if addEr != nil {
		t.Fatalf("add: %v", addEr)
	}
	if n := f.lastNotice(); n.Kind != NoticeSuccess || n.Title != "Submodule added" {
		t.Fatalf("add notification = %+v", n)
	}
	if got := f.git("submodule", "status"); !strings.Contains(got, path) {
		t.Fatalf("git does not list the new submodule: %q", got)
	}
	// Refresh ran: the staged gitlink and .gitmodules are visible.
	staged := f.w.Repo.Entries(domain.AreaStaged)
	foundLink, foundModules := false, false
	for _, e := range staged {
		if e.Path == path {
			foundLink = true
		}
		if e.Path == ".gitmodules" {
			foundModules = true
		}
	}
	if !foundLink || !foundModules {
		t.Fatalf("workspace not refreshed after add: %+v", staged)
	}

	f.commitAll("add submodule")
	f.settle(func() bool { return f.idle() })

	var (
		dirty   bool
		dirtyEr error
		dirtyOK bool
	)
	f.w.SubmoduleDirty(path, func(d bool, err error) { dirty, dirtyEr, dirtyOK = d, err, true })
	f.settle(func() bool { return dirtyOK })
	if dirtyEr != nil || dirty {
		t.Fatalf("clean submodule reported dirty=%v err=%v", dirty, dirtyEr)
	}

	var (
		deinitEr error
		deinitOK bool
	)
	f.w.DeinitSubmodule(path, false, func(err error) { deinitEr, deinitOK = err, true })
	f.settle(func() bool { return deinitOK && f.idle() })
	if deinitEr != nil {
		t.Fatalf("deinit: %v", deinitEr)
	}
	if n := f.lastNotice(); n.Kind != NoticeSuccess || n.Title != "Submodule deinitialized" {
		t.Fatalf("deinit notification = %+v", n)
	}
	if entries, err := os.ReadDir(filepath.Join(f.dir, filepath.FromSlash(path))); err == nil && len(entries) != 0 {
		t.Fatalf("deinit left files: %v", entries)
	}
	if got := f.git("submodule", "status"); !strings.HasPrefix(strings.TrimSpace(got), "-") {
		t.Fatalf("deinit did not uninitialize: %q", got)
	}

	var (
		updEr error
		updOK bool
	)
	f.w.UpdateSubmodule(path, true, func(err error) { updEr, updOK = err, true })
	f.settle(func() bool { return updOK && f.idle() })
	if updEr != nil {
		t.Fatalf("update after deinit: %v", updEr)
	}

	// Local work inside the submodule refuses a non-forced removal without
	// changing anything.
	lib := filepath.Join(f.dir, filepath.FromSlash(path), "lib.txt")
	if err := os.WriteFile(lib, []byte("user work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, dirtyEr, dirtyOK = false, nil, false
	f.w.SubmoduleDirty(path, func(d bool, err error) { dirty, dirtyEr, dirtyOK = d, err, true })
	f.settle(func() bool { return dirtyOK })
	if dirtyEr != nil || !dirty {
		t.Fatalf("dirty submodule reported dirty=%v err=%v", dirty, dirtyEr)
	}
	var (
		rmEr error
		rmOK bool
	)
	f.w.RemoveSubmodule(path, false, func(err error) { rmEr, rmOK = err, true })
	f.settle(func() bool { return rmOK && f.idle() })
	if !errors.Is(rmEr, gitcli.ErrSubmoduleNeedsForce) {
		t.Fatalf("remove of a dirty submodule = %v, want ErrSubmoduleNeedsForce", rmEr)
	}
	if got, err := os.ReadFile(lib); err != nil || string(got) != "user work\n" {
		t.Fatalf("refused removal changed the checkout: %q, %v", got, err)
	}

	var (
		forceEr error
		forceOK bool
	)
	f.w.RemoveSubmodule(path, true, func(err error) { forceEr, forceOK = err, true })
	f.settle(func() bool { return forceOK && f.idle() })
	if forceEr != nil {
		t.Fatalf("forced remove: %v", forceEr)
	}
	if n := f.lastNotice(); n.Kind != NoticeSuccess || n.Title != "Submodule removed" {
		t.Fatalf("remove notification = %+v", n)
	}
	if got := strings.TrimSpace(f.git("submodule", "status")); got != "" {
		t.Fatalf("git still lists a submodule: %q", got)
	}
	if _, err := os.Stat(filepath.Join(f.dir, filepath.FromSlash(path))); !os.IsNotExist(err) {
		t.Fatalf("removal left the working tree: %v", err)
	}
	modules, err := os.ReadFile(filepath.Join(f.dir, ".gitmodules"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(modules), path) {
		t.Fatalf(".gitmodules still records %q:\n%s", path, modules)
	}
}

// TestSubmoduleManageFailures checks that Git failures reach the callback and
// are not reported as a success.
func TestSubmoduleManageFailures(t *testing.T) {
	f := newFixture(t)
	f.write("p.txt", "parent\n")
	f.commitAll("parent")
	f.open()
	before := len(f.w.Notifications)

	var (
		addEr error
		addOK bool
	)
	f.w.AddSubmodule(filepath.Join(t.TempDir(), "missing"), "deps/missing", func(err error) { addEr, addOK = err, true })
	f.settle(func() bool { return addOK && f.idle() })
	if addEr == nil {
		t.Fatal("adding a missing repository succeeded")
	}
	if got := f.errorsSince(before); len(got) != 0 {
		t.Fatalf("failure was also shown as its own notification: %+v", got)
	}
	if got := strings.TrimSpace(f.git("submodule", "status")); got != "" {
		t.Fatalf("failed add left a submodule: %q", got)
	}

	var (
		rmEr error
		rmOK bool
	)
	f.w.RemoveSubmodule("p.txt", false, func(err error) { rmEr, rmOK = err, true })
	f.settle(func() bool { return rmOK && f.idle() })
	if rmEr == nil || !strings.Contains(rmEr.Error(), "not a submodule") {
		t.Fatalf("remove of a plain file = %v", rmEr)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "p.txt")); err != nil {
		t.Fatalf("refused removal deleted the file: %v", err)
	}

	var (
		direct   map[string]bool
		directEr error
		directOK bool
	)
	f.w.DirectSubmodules(func(paths map[string]bool, err error) { direct, directEr, directOK = paths, err, true })
	f.settle(func() bool { return directOK })
	if directEr != nil || len(direct) != 0 {
		t.Fatalf("direct submodules = %v, %v", direct, directEr)
	}
}

// TestSubmoduleManageNestedRefused checks that the workspace refuses to act on
// a nested submodule through the parent repository and keeps the checkout.
func TestSubmoduleManageNestedRefused(t *testing.T) {
	leaf := newFixture(t)
	leaf.write("leaf.txt", "leaf\n")
	leaf.commitAll("leaf")

	mid := newFixture(t)
	mid.write("mid.txt", "mid\n")
	mid.commitAll("mid")
	mid.git("-c", "protocol.file.allow=always", "submodule", "add", leaf.dir, "leaf")
	mid.commitAll("add leaf")

	top := newFixture(t)
	top.write("top.txt", "top\n")
	top.commitAll("top")
	top.git("-c", "protocol.file.allow=always", "submodule", "add", mid.dir, "mid")
	top.commitAll("add mid")
	top.git("-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")
	top.open()

	var (
		direct   map[string]bool
		directEr error
		directOK bool
	)
	top.w.DirectSubmodules(func(paths map[string]bool, err error) { direct, directEr, directOK = paths, err, true })
	top.settle(func() bool { return directOK })
	if directEr != nil {
		t.Fatal(directEr)
	}
	if !direct["mid"] || direct["mid/leaf"] {
		t.Fatalf("direct submodules = %v, want only mid", direct)
	}

	var (
		subs   []domain.Submodule
		listed bool
	)
	top.w.Submodules(func(list []domain.Submodule, err error) {
		if err != nil {
			t.Errorf("list submodules: %v", err)
		}
		subs, listed = list, true
	})
	top.settle(func() bool { return listed })
	seenNested := false
	for _, sm := range subs {
		if sm.Path == "mid/leaf" {
			seenNested = true
		}
	}
	if !seenNested {
		t.Fatalf("nested submodule not listed: %+v", subs)
	}

	var (
		rmEr error
		rmOK bool
	)
	top.w.RemoveSubmodule("mid/leaf", false, func(err error) { rmEr, rmOK = err, true })
	top.settle(func() bool { return rmOK && top.idle() })
	if rmEr == nil || !strings.Contains(rmEr.Error(), "nested submodule") {
		t.Fatalf("nested remove = %v", rmEr)
	}
	if _, err := os.Stat(filepath.Join(top.dir, "mid", "leaf", "leaf.txt")); err != nil {
		t.Fatalf("refused nested removal changed the checkout: %v", err)
	}
}
