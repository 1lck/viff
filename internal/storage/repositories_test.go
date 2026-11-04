package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceRepositoriesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	r := LoadRepositories(settingsPath)
	if want := filepath.Join(dir, "workspace.json"); r.Path() != want {
		t.Fatalf("workspace path = %q, want %q", r.Path(), want)
	}
	if len(r.Repositories) != 0 || r.Active != "" || r.Warning != "" || r.ReadOnly() {
		t.Fatalf("fresh workspace = %+v", r)
	}

	r.Repositories = []string{"/repos/a", "/repos/b"}
	r.Active = "/repos/b"
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(r.Path())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("workspace file is not JSON: %v\n%s", err, data)
	}
	if got := int(raw["version"].(float64)); got != WorkspaceSchemaVersion {
		t.Fatalf("written version = %d", got)
	}

	loaded := LoadRepositories(settingsPath)
	if loaded.Version != WorkspaceSchemaVersion || len(loaded.Repositories) != 2 ||
		loaded.Repositories[0] != "/repos/a" || loaded.Repositories[1] != "/repos/b" ||
		loaded.Active != "/repos/b" || loaded.Warning != "" {
		t.Fatalf("reloaded workspace = %+v", loaded)
	}
}

func TestWorkspaceRepositoriesNewerVersionIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.json")
	original := []byte(`{"version": 99, "repositories": ["/repos/a"], "active": "/repos/a"}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	r := LoadRepositories(filepath.Join(dir, "settings.json"))
	if !r.ReadOnly() || r.Warning == "" {
		t.Fatalf("newer workspace not protected: %+v", r)
	}
	// The known rows are still usable read-only, but a save never overwrites
	// the file written by the newer version.
	if len(r.Repositories) != 1 || r.Repositories[0] != "/repos/a" {
		t.Fatalf("newer workspace rows = %+v", r.Repositories)
	}
	r.Repositories = []string{"/repos/b"}
	r.Active = "/repos/b"
	if err := r.Save(); !errors.Is(err, ErrWorkspaceReadOnly) {
		t.Fatalf("save of a read-only workspace = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("newer workspace was overwritten: %s", data)
	}
}

func TestWorkspaceRepositoriesMalformedFileIsReadOnlyAndUntouched(t *testing.T) {
	for name, contents := range map[string]string{
		"broken json":   `{broken`,
		"type mismatch": `{"version": 1, "repositories": 5, "active": true}`,
		"bad version":   `{"version": -3, "repositories": []}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "workspace.json")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			r := LoadRepositories(filepath.Join(dir, "settings.json"))
			if !r.ReadOnly() || r.Warning == "" {
				t.Fatalf("malformed workspace not protected: %+v", r)
			}
			if len(r.Repositories) != 0 || r.Active != "" {
				t.Fatalf("malformed workspace kept partial data: %+v", r)
			}
			// The file is never moved or overwritten: a future version may be
			// able to repair it, and dropping it would lose the user's list.
			if err := r.Save(); !errors.Is(err, ErrWorkspaceReadOnly) {
				t.Fatalf("save of a malformed workspace = %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "workspace.json" {
				t.Fatalf("malformed file was moved or replaced: %v", entries)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != contents {
				t.Fatalf("malformed file changed: %q", data)
			}
		})
	}
}

func TestWorkspacePathWithoutSettingsPath(t *testing.T) {
	if got := WorkspacePath(""); got != "" {
		t.Fatalf("WorkspacePath(\"\") = %q", got)
	}
	r := LoadRepositories("")
	if r.Path() != "" || r.Warning != "" {
		t.Fatalf("pathless workspace = %+v", r)
	}
	r.Repositories = []string{"/repos/a"}
	if err := r.Save(); err != nil {
		t.Fatalf("pathless save = %v", err)
	}
}
