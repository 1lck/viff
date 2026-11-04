package app

import (
	"os"
	"path/filepath"
	"testing"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/storage"
)

// changeListPersist makes the fixture use settings at a temporary path, which
// is what enables change list persistence: the metadata file lives next to the
// settings file, never in the repository.
func changeListPersist(t *testing.T, f *fixture) string {
	t.Helper()
	settingsPath := filepath.Join(t.TempDir(), "Viff", "settings.json")
	f.w = NewWorkspace(&gitcli.Runner{Env: f.env}, storage.Load(settingsPath), nil)
	return settingsPath
}

// changeListOpen opens dir and waits until that repository is active.
func changeListOpen(t *testing.T, f *fixture, dir string) {
	t.Helper()
	want := dir
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		want = resolved
	}
	f.w.Open(dir, func(err error) {
		if err != nil {
			t.Fatalf("open %s: %v", dir, err)
		}
	})
	f.settle(func() bool {
		if f.w.Repo == nil || !f.w.Repo.StatusLoaded || !f.idle() {
			return false
		}
		got := f.w.Repo.Repo.Root
		if resolved, err := filepath.EvalSymlinks(got); err == nil {
			got = resolved
		}
		return got == want
	})
}

func changeListNames(lists []ChangeList) []string {
	names := make([]string, 0, len(lists))
	for _, l := range lists {
		names = append(names, l.ID+":"+l.Name)
	}
	return names
}

func TestChangeListCreateRenameDelete(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a\n")
	if got := f.w.Lists(); got != nil {
		t.Fatalf("lists without an open repository = %+v, want nil", got)
	}
	f.open()

	defaultID := storage.DefaultChangeListID
	lists := f.w.Lists()
	if len(lists) != 1 || lists[0].ID != defaultID || lists[0].Name != storage.DefaultChangeListName {
		t.Fatalf("initial lists = %+v, want only the default list", lists)
	}
	if got := f.w.ChangeListFor("a.txt"); got != defaultID {
		t.Fatalf("unassigned path list = %q, want %q", got, defaultID)
	}

	id, err := f.w.CreateChangeList("  Feature  ")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || id == defaultID {
		t.Fatalf("created id = %q", id)
	}
	lists = f.w.Lists()
	if len(lists) != 2 || lists[1].ID != id || lists[1].Name != "Feature" {
		t.Fatalf("lists after create = %+v", lists)
	}

	for _, name := range []string{"Feature", "feature", "  "} {
		if _, err := f.w.CreateChangeList(name); err == nil {
			t.Fatalf("creating %q should fail", name)
		}
	}
	second, err := f.w.CreateChangeList("Second")
	if err != nil || second == id {
		t.Fatalf("second list = %q %v", second, err)
	}

	if err := f.w.RenameChangeList(defaultID, "Other"); err == nil {
		t.Fatal("renaming the default list should fail")
	}
	if err := f.w.RenameChangeList("missing", "Other"); err == nil {
		t.Fatal("renaming an unknown list should fail")
	}
	if err := f.w.RenameChangeList(id, "Second"); err == nil {
		t.Fatal("renaming to a used name should fail")
	}
	if err := f.w.RenameChangeList(id, "   "); err == nil {
		t.Fatal("renaming to a blank name should fail")
	}
	if err := f.w.RenameChangeList(id, "Renamed"); err != nil {
		t.Fatal(err)
	}
	lists = f.w.Lists()
	if lists[1].ID != id || lists[1].Name != "Renamed" {
		t.Fatalf("lists after rename = %+v, id must stay stable", lists)
	}

	if err := f.w.MoveToChangeList([]string{"a.txt", "a.txt"}, id); err != nil {
		t.Fatal(err)
	}
	if got := f.w.ChangeListFor("a.txt"); got != id {
		t.Fatalf("moved path list = %q, want %q", got, id)
	}
	if got := f.w.ChangeListFor("b.txt"); got != defaultID {
		t.Fatalf("unknown path list = %q, want %q", got, defaultID)
	}

	if err := f.w.DeleteChangeList(defaultID); err == nil {
		t.Fatal("deleting the default list should fail")
	}
	if err := f.w.DeleteChangeList("missing"); err == nil {
		t.Fatal("deleting an unknown list should fail")
	}
	if err := f.w.DeleteChangeList(id); err != nil {
		t.Fatal(err)
	}
	if got := f.w.ChangeListFor("a.txt"); got != defaultID {
		t.Fatalf("deleted list should return members to default, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "a.txt")); err != nil {
		t.Fatalf("deleting a list must not touch files: %v", err)
	}
	if len(f.w.Repo.Status.Entries) != 1 || f.w.Repo.Status.Entries[0].Path != "a.txt" {
		t.Fatalf("deleting a list must not change Git status: %+v", f.w.Repo.Status.Entries)
	}
	if names := changeListNames(f.w.Lists()); len(names) != 2 || names[0] != defaultID+":Default" || names[1] != second+":Second" {
		t.Fatalf("lists after delete = %v", names)
	}
}

func TestChangeListValidation(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a\n")
	f.open()

	if err := f.w.MoveToChangeList([]string{"a.txt"}, "missing"); err == nil {
		t.Fatal("moving to an unknown list should fail")
	}
	for _, bad := range []string{"", ".", "../up.txt", "dir/../a.txt", "/etc/passwd", "./a.txt", "a.txt/", "dir//a.txt"} {
		if err := f.w.MoveToChangeList([]string{bad}, storage.DefaultChangeListID); err == nil {
			t.Fatalf("path %q should be refused", bad)
		}
	}
	if err := f.w.MoveToChangeList(nil, storage.DefaultChangeListID); err != nil {
		t.Fatalf("empty path list should be a no-op: %v", err)
	}
	if got := f.w.ChangeListFor("not/in/status.txt"); got != storage.DefaultChangeListID {
		t.Fatalf("unknown path list = %q", got)
	}
	if err := f.w.RenameChangeList("missing", "X"); err == nil {
		t.Fatal("renaming an unknown list should fail")
	}
	if err := f.w.DeleteChangeList("missing"); err == nil {
		t.Fatal("deleting an unknown list should fail")
	}
}

// Two repositories with the same relative path keep independent lists, and a
// linked worktree is a different canonical root, so it starts with its own
// lists too.
func TestChangeListRepositoriesAreIsolated(t *testing.T) {
	f := newFixture(t)
	settingsPath := changeListPersist(t, f)
	f.write("same.txt", "one\n")
	changeListOpen(t, f, f.dir)
	mainList, err := f.w.CreateChangeList("Main")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.MoveToChangeList([]string{"same.txt"}, mainList); err != nil {
		t.Fatal(err)
	}

	other := newFixture(t)
	other.write("same.txt", "two\n")
	changeListOpen(t, f, other.dir)
	if got := f.w.ChangeListFor("same.txt"); got != storage.DefaultChangeListID {
		t.Fatalf("second repository inherited a list from the first: %q", got)
	}
	if lists := f.w.Lists(); len(lists) != 1 {
		t.Fatalf("second repository lists = %+v, want only default", lists)
	}
	otherList, err := f.w.CreateChangeList("Other")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.MoveToChangeList([]string{"same.txt"}, otherList); err != nil {
		t.Fatal(err)
	}

	changeListOpen(t, f, f.dir)
	if got := f.w.ChangeListFor("same.txt"); got != mainList {
		t.Fatalf("first repository list = %q, want %q", got, mainList)
	}
	if got := f.w.Lists(); len(got) != 2 || got[1].ID != mainList {
		t.Fatalf("first repository lists = %+v", got)
	}

	// A linked worktree has a different root and therefore its own lists.
	wt := filepath.Join(t.TempDir(), "wt")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	f.git("worktree", "add", "-q", "-b", "side", wt)
	changeListOpen(t, f, wt)
	if got := f.w.ChangeListFor("same.txt"); got != storage.DefaultChangeListID {
		t.Fatalf("linked worktree inherited the main checkout's list: %q", got)
	}
	wtList, err := f.w.CreateChangeList("Worktree")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.MoveToChangeList([]string{"same.txt"}, wtList); err != nil {
		t.Fatal(err)
	}
	changeListOpen(t, f, f.dir)
	if got := f.w.ChangeListFor("same.txt"); got != mainList {
		t.Fatalf("main checkout list after worktree use = %q, want %q", got, mainList)
	}

	// The metadata file lives next to the settings, not in either checkout.
	metadata := storage.ChangeListsPath(settingsPath)
	if _, err := os.Stat(metadata); err != nil {
		t.Fatalf("metadata file %s missing: %v", metadata, err)
	}
	for _, dir := range []string{f.dir, other.dir, wt} {
		if _, err := os.Stat(filepath.Join(dir, "changelists.json")); !os.IsNotExist(err) {
			t.Fatalf("metadata was written into repository %s: %v", dir, err)
		}
	}
}

func TestChangeListReloadPersists(t *testing.T) {
	f := newFixture(t)
	settingsPath := changeListPersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)
	id, err := f.w.CreateChangeList("Feature")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.MoveToChangeList([]string{"a.txt"}, id); err != nil {
		t.Fatal(err)
	}

	// A new workspace (a restart) loads the lists from the settings directory.
	reloaded := newFixture(t)
	reloaded.w = NewWorkspace(&gitcli.Runner{Env: reloaded.env}, storage.Load(settingsPath), nil)
	changeListOpen(t, reloaded, f.dir)
	lists := reloaded.w.Lists()
	if len(lists) != 2 || lists[1].ID != id || lists[1].Name != "Feature" {
		t.Fatalf("lists after reload = %+v", lists)
	}
	if got := reloaded.w.ChangeListFor("a.txt"); got != id {
		t.Fatalf("membership after reload = %q, want %q", got, id)
	}
	if err := reloaded.w.RenameChangeList(id, "Renamed"); err != nil {
		t.Fatal(err)
	}
	again := newFixture(t)
	again.w = NewWorkspace(&gitcli.Runner{Env: again.env}, storage.Load(settingsPath), nil)
	changeListOpen(t, again, f.dir)
	if lists := again.w.Lists(); len(lists) != 2 || lists[1].Name != "Renamed" || lists[1].ID != id {
		t.Fatalf("renamed lists after reload = %+v", lists)
	}
}

func TestChangeListSaveFailureKeepsState(t *testing.T) {
	f := newFixture(t)
	settingsPath := changeListPersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)
	id, err := f.w.CreateChangeList("Keep")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.MoveToChangeList([]string{"a.txt"}, id); err != nil {
		t.Fatal(err)
	}
	metadata := storage.ChangeListsPath(settingsPath)
	before, err := os.ReadFile(metadata)
	if err != nil {
		t.Fatal(err)
	}

	// Make the next save fail on every platform without skipping. On Unix a
	// read-only directory stops the temporary file before the rename; on
	// Windows directory attributes do not prevent creating children, so the
	// read-only destination file and the open handle below make MoveFileEx
	// fail (a read-only destination cannot be replaced, and a handle opened
	// without FILE_SHARE_DELETE blocks replacing the file as well). The
	// already saved metadata and the in-memory lists must survive; closing the
	// handle and restoring the modes makes saving possible again.
	dir := filepath.Dir(metadata)
	locked, err := os.Open(metadata)
	if err != nil {
		t.Fatal(err)
	}
	restore := func() {
		if locked != nil {
			if err := locked.Close(); err != nil {
				t.Errorf("close metadata file: %v", err)
			}
			locked = nil
		}
		if err := os.Chmod(metadata, 0o600); err != nil {
			t.Errorf("restore metadata mode: %v", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Errorf("restore directory mode: %v", err)
		}
	}
	defer restore()
	if err := os.Chmod(metadata, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}

	if _, err := f.w.CreateChangeList("Lost"); err == nil {
		t.Fatal("creating a list should fail when the metadata cannot be written")
	}
	if err := f.w.MoveToChangeList([]string{"a.txt"}, storage.DefaultChangeListID); err == nil {
		t.Fatal("moving a path should fail when the metadata cannot be written")
	}
	lists := f.w.Lists()
	if len(lists) != 2 || lists[1].ID != id || lists[1].Name != "Keep" {
		t.Fatalf("memory changed after a failed save: %+v", lists)
	}
	if got := f.w.ChangeListFor("a.txt"); got != id {
		t.Fatalf("membership changed after a failed save: %q", got)
	}
	after, err := os.ReadFile(metadata)
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed save modified the file: %q %v", after, err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".changelists-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("failed save left temporary files behind: %v", leftovers)
	}

	// Once writing is possible again the state can still be changed.
	restore()
	if err := f.w.DeleteChangeList(id); err != nil {
		t.Fatalf("change lists did not recover after the failed save: %v", err)
	}
}

func TestChangeListCorruptFileIsReported(t *testing.T) {
	f := newFixture(t)
	settingsPath := changeListPersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)
	metadata := storage.ChangeListsPath(settingsPath)
	if err := os.MkdirAll(filepath.Dir(metadata), 0o700); err != nil {
		t.Fatal(err)
	}
	broken := []byte(`{"version":1,`)
	if err := os.WriteFile(metadata, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	if lists := f.w.Lists(); len(lists) != 1 || lists[0].ID != storage.DefaultChangeListID {
		t.Fatalf("corrupt metadata should degrade to the default list: %+v", lists)
	}
	if _, err := f.w.CreateChangeList("Feature"); err == nil {
		t.Fatal("creating a list should report the corrupt metadata")
	}
	if got := f.w.ChangeListFor("a.txt"); got != storage.DefaultChangeListID {
		t.Fatalf("corrupt metadata membership = %q", got)
	}
	if data, err := os.ReadFile(metadata); err != nil || string(data) != string(broken) {
		t.Fatalf("corrupt metadata was overwritten: %q %v", data, err)
	}
}

func TestChangeListSyncRename(t *testing.T) {
	f := newFixture(t)
	f.write("old.txt", "content\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	f.open()
	id, err := f.w.CreateChangeList("Tracked")
	if err != nil {
		t.Fatal(err)
	}
	// A committed, clean path keeps its list even though it is not in status.
	if err := f.w.MoveToChangeList([]string{"old.txt"}, id); err != nil {
		t.Fatal(err)
	}
	if got := f.w.ChangeListFor("old.txt"); got != id {
		t.Fatalf("clean path list = %q, want %q", got, id)
	}

	f.git("mv", "old.txt", "new.txt")
	f.w.Refresh(true)
	f.settle(func() bool {
		if !f.idle() || !f.w.Repo.StatusLoaded || len(f.w.Repo.Status.Entries) != 1 {
			return false
		}
		e := f.w.Repo.Status.Entries[0]
		return e.Path == "new.txt" && e.OrigPath == "old.txt"
	})
	// The rename is visible immediately without waiting for the persisted move.
	if got := f.w.ChangeListFor("new.txt"); got != id {
		t.Fatalf("renamed path list = %q, want %q", got, id)
	}
	if err := f.w.SyncChangeListRenames(); err != nil {
		t.Fatal(err)
	}
	if got := f.w.ChangeListFor("old.txt"); got != storage.DefaultChangeListID {
		t.Fatalf("old path left in list %q after migration", got)
	}
	if got := f.w.ChangeListFor("new.txt"); got != id {
		t.Fatalf("new path list after migration = %q, want %q", got, id)
	}
	// Running it again is a no-op.
	if err := f.w.SyncChangeListRenames(); err != nil {
		t.Fatal(err)
	}
}

// Git also sets OrigPath on a copy ("C"). A copied path is a new file: its
// source must keep its list and the copy starts unassigned, unlike a rename
// where the old path is gone.
func TestChangeListCopyKeepsSourceMembership(t *testing.T) {
	f := newFixture(t)
	f.write("old.txt", "one\n")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	f.open()
	id, err := f.w.CreateChangeList("Tracked")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.MoveToChangeList([]string{"old.txt"}, id); err != nil {
		t.Fatal(err)
	}
	f.w.Repo.Status = domain.Status{Entries: []domain.StatusEntry{
		{Path: "copy.txt", OrigPath: "old.txt", Index: 'C', Worktree: '.'},
	}}
	if got := f.w.ChangeListFor("copy.txt"); got != storage.DefaultChangeListID {
		t.Fatalf("copy inherited the source list: %q", got)
	}
	if err := f.w.SyncChangeListRenames(); err != nil {
		t.Fatal(err)
	}
	if got := f.w.ChangeListFor("old.txt"); got != id {
		t.Fatalf("copy migration removed the source membership: %q", got)
	}
	if got := f.w.ChangeListFor("copy.txt"); got != storage.DefaultChangeListID {
		t.Fatalf("copy path membership after sync = %q", got)
	}
}
