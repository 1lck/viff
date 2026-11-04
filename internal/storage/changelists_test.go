package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestChangeListsPathBesideSettings(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "Viff", "settings.json")
	got := ChangeListsPath(settings)
	want := filepath.Join(filepath.Dir(settings), "changelists.json")
	if got != want {
		t.Fatalf("ChangeListsPath(%q) = %q, want %q", settings, got, want)
	}
	if got := ChangeListsPath(""); got != "" {
		t.Fatalf("ChangeListsPath(\"\") = %q, want empty", got)
	}
}

func TestChangeListsRoundTrip(t *testing.T) {
	path := ChangeListsPath(filepath.Join(t.TempDir(), "settings.json"))
	if got, err := LoadChangeLists(path); err != nil || got == nil || len(got.Repos) != 0 {
		t.Fatalf("missing file should give an empty document: %v %v", got, err)
	}

	doc := NewChangeLists()
	root := filepath.Join(t.TempDir(), "repo")
	doc.Repos[root] = RepoChangeLists{
		Lists: []ChangeListRecord{
			{ID: DefaultChangeListID, Name: DefaultChangeListName},
			{ID: "list-a", Name: "Feature"},
		},
		Members: map[string]string{"a.txt": "list-a", "b.txt": DefaultChangeListID},
	}
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := LoadChangeLists(path)
	if err != nil {
		t.Fatal(err)
	}
	repo, ok := back.Repos[root]
	if !ok {
		t.Fatalf("repository %q missing after load: %+v", root, back.Repos)
	}
	if len(repo.Lists) != 2 || repo.Lists[0].ID != DefaultChangeListID || repo.Lists[1].Name != "Feature" {
		t.Fatalf("lists = %+v", repo.Lists)
	}
	if repo.Members["a.txt"] != "list-a" || repo.Members["b.txt"] != DefaultChangeListID {
		t.Fatalf("members = %+v", repo.Members)
	}
	if back.ReadOnly() {
		t.Fatal("a file written by this build should be writable")
	}
}

func TestChangeListsCorruptFileIsReportedAndKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changelists.json")
	broken := []byte(`{"version":1,"repositories":{"x":`)
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadChangeLists(path); err == nil {
		t.Fatal("corrupt file should be an error")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(broken) {
		t.Fatalf("corrupt file was modified: %q %v", data, err)
	}
}

func TestChangeListsNewerFormatIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changelists.json")
	root := filepath.Join(dir, "repo")
	future := map[string]any{
		"version": ChangeListsVersion + 1,
		"repositories": map[string]any{
			root: map[string]any{
				"lists":   []map[string]any{{"id": "future-list", "name": "From the future"}},
				"members": map[string]string{"a.txt": "future-list"},
			},
		},
		"unknownFutureField": true,
	}
	raw, err := json.Marshal(future)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadChangeLists(path)
	if err != nil {
		t.Fatalf("newer format should load for reading: %v", err)
	}
	if !doc.ReadOnly() {
		t.Fatal("newer format must be read-only")
	}
	repo, ok := doc.Repos[root]
	if !ok || len(repo.Lists) != 2 || repo.Lists[1].Name != "From the future" {
		t.Fatalf("newer lists were not readable: %+v", doc.Repos)
	}
	if err := doc.Clone().Save(path); err != ErrChangeListsReadOnly {
		t.Fatalf("saving a newer file = %v, want %v", err, ErrChangeListsReadOnly)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(raw) {
		t.Fatalf("newer file was modified: %q %v", after, err)
	}
}

func TestChangeListsNormalizeInconsistentEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changelists.json")
	root := filepath.Join(dir, "repo")
	// Build the file through the JSON encoder: a canonical Windows root
	// contains backslashes, which a literal string would turn into invalid
	// escapes (and the file would be rejected as corrupt, not normalized).
	inconsistent := map[string]any{
		"version": 1,
		"repositories": map[string]any{
			root: map[string]any{
				"lists": []map[string]any{
					{"id": "a", "name": "A"},
					{"id": "a", "name": "Duplicate"},
					{"id": "", "name": "No id"},
					{"id": "b", "name": ""},
				},
				"members": map[string]string{
					"x.txt": "missing",
					"":      "a",
					"y.txt": "a",
				},
			},
		},
	}
	raw, err := json.Marshal(inconsistent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadChangeLists(path)
	if err != nil {
		t.Fatal(err)
	}
	repo := doc.Repos[root]
	if len(repo.Lists) != 2 || repo.Lists[0].ID != DefaultChangeListID || repo.Lists[1].ID != "a" {
		t.Fatalf("lists = %+v", repo.Lists)
	}
	if _, ok := repo.Members["x.txt"]; ok {
		t.Fatalf("member of a missing list should fall back to default: %+v", repo.Members)
	}
	if _, ok := repo.Members[""]; ok {
		t.Fatalf("empty member path should be dropped: %+v", repo.Members)
	}
	if repo.Members["y.txt"] != "a" {
		t.Fatalf("member of a known list was lost: %+v", repo.Members)
	}
}

func TestChangeListsSaveFailureKeepsOldFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changelists.json")
	doc := NewChangeLists()
	doc.Repos[filepath.Join(dir, "repo")] = RepoChangeLists{
		Lists:   []ChangeListRecord{{ID: "keep", Name: "Keep"}},
		Members: map[string]string{"a.txt": "keep"},
	}
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Make the next save fail on every platform without skipping. On Unix a
	// read-only directory stops the temporary file before the rename; on
	// Windows directory attributes do not prevent creating children, so the
	// read-only destination file and the open handle below make MoveFileEx
	// fail (a read-only destination cannot be replaced, and a handle opened
	// without FILE_SHARE_DELETE blocks replacing the file as well). Closing
	// the handle and restoring the modes makes saving possible again.
	locked, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	restore := func() {
		if err := locked.Close(); err != nil {
			t.Errorf("close metadata file: %v", err)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			t.Errorf("restore metadata mode: %v", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Errorf("restore directory mode: %v", err)
		}
	}
	defer restore()
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}

	next := doc.Clone()
	next.Repos[filepath.Join(dir, "repo2")] = RepoChangeLists{Lists: []ChangeListRecord{{ID: "x", Name: "X"}}}
	if err := next.Save(path); err == nil {
		t.Fatal("saving over an unwritable file should fail")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".changelists-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("failed save left temporary files behind: %v", leftovers)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed save modified the file: %q %v", after, err)
	}
	if len(doc.Repos) != 1 {
		t.Fatalf("failed save changed the source document: %+v", doc.Repos)
	}
}
