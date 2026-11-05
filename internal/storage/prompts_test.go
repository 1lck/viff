package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPromptRulesPathBesideSettings(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "Viff", "settings.json")
	got := PromptRulesPath(settings)
	want := filepath.Join(filepath.Dir(settings), "prompts.json")
	if got != want {
		t.Fatalf("PromptRulesPath(%q) = %q, want %q", settings, got, want)
	}
	if got := PromptRulesPath(""); got != "" {
		t.Fatalf("PromptRulesPath(\"\") = %q, want empty", got)
	}
}

func TestPromptRulesRoundTrip(t *testing.T) {
	path := PromptRulesPath(filepath.Join(t.TempDir(), "settings.json"))
	if got, err := LoadPromptRules(path); err != nil || got == nil || len(got.Rules) != 0 || len(got.Repos) != 0 {
		t.Fatalf("missing file should give an empty document: %v %v", got, err)
	}

	doc := NewPromptRules()
	doc.Rules = []PromptRule{
		{ID: "terse", Name: "Terse", Text: "One short sentence, no prefix."},
		{ID: "ticket", Name: "Ticket", Text: "Start with the ticket number."},
	}
	root := filepath.Join(t.TempDir(), "repo")
	doc.Repos[root] = "ticket"
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := LoadPromptRules(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.ReadOnly() {
		t.Fatal("a file written by this build should be writable")
	}
	if len(back.Rules) != 2 || back.Rules[0].ID != "terse" || back.Rules[1].Text != "Start with the ticket number." {
		t.Fatalf("rules = %+v", back.Rules)
	}
	if back.Repos[root] != "ticket" {
		t.Fatalf("selection = %+v", back.Repos)
	}
	if back.Version != PromptRulesVersion {
		t.Fatalf("version = %d", back.Version)
	}
}

func TestPromptRulesCorruptFileIsReportedAndKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	broken := []byte(`{"version":1,"rules":[`)
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPromptRules(path); err == nil {
		t.Fatal("corrupt file should be an error")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(broken) {
		t.Fatalf("corrupt file was modified: %q %v", data, err)
	}
}

func TestPromptRulesNewerFormatIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	root := filepath.Join(dir, "repo")
	future := make(map[string]any)
	future["version"] = PromptRulesVersion + 1
	future["rules"] = []map[string]any{{"id": "future", "name": "From the future", "text": "Say it briefly."}}
	future["repositories"] = map[string]string{root: "future"}
	future["unknownFutureField"] = true
	raw, err := json.Marshal(future)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadPromptRules(path)
	if err != nil {
		t.Fatalf("newer format should load for reading: %v", err)
	}
	if !doc.ReadOnly() {
		t.Fatal("newer format must be read-only")
	}
	if len(doc.Rules) != 1 || doc.Rules[0].Name != "From the future" {
		t.Fatalf("newer rules were not readable: %+v", doc.Rules)
	}
	if err := doc.Save(path); err != ErrPromptRulesReadOnly {
		t.Fatalf("saving a newer file = %v, want %v", err, ErrPromptRulesReadOnly)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(raw) {
		t.Fatalf("newer file was modified: %q %v", after, err)
	}
}

func TestPromptRulesNormalizeInconsistentEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	root := filepath.Join(dir, "repo")
	orphan := filepath.Join(dir, "orphan")
	// Build the file through the JSON encoder: a canonical Windows root
	// contains backslashes, which a literal string would turn into invalid
	// escapes (and the file would be rejected as corrupt, not normalized).
	inconsistent := make(map[string]any)
	inconsistent["version"] = PromptRulesVersion
	inconsistent["rules"] = []map[string]any{
		{"id": "terse", "name": "  Terse  ", "text": "  One sentence.  "},
		{"id": "terse", "name": "Duplicate", "text": "Second definition."},
		{"id": "", "name": "No id", "text": "text"},
		{"id": "nameless", "name": "   ", "text": "text"},
	}
	selections := make(map[string]string)
	selections[root] = "terse"
	selections[orphan] = "missing"
	selections[""] = "terse"
	inconsistent["repositories"] = selections
	raw, err := json.Marshal(inconsistent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadPromptRules(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Rules) != 1 {
		t.Fatalf("rules = %+v, want only the first usable definition", doc.Rules)
	}
	if got := doc.Rules[0]; got.ID != "terse" || got.Name != "Terse" || got.Text != "One sentence." {
		t.Fatalf("rule = %+v, want the trimmed first definition", got)
	}
	if doc.Repos[root] != "terse" {
		t.Fatalf("selection of an existing rule was lost: %+v", doc.Repos)
	}
	if _, ok := doc.Repos[orphan]; ok {
		t.Fatalf("selection of a missing rule should be dropped: %+v", doc.Repos)
	}
	if _, ok := doc.Repos[""]; ok {
		t.Fatalf("empty repository root should be dropped: %+v", doc.Repos)
	}
}

// TestPromptRulesNormalizeClearsStaleLastRule keeps the default from naming a
// rule that is not in the file: the fallback would otherwise resolve to nothing
// for every repository without its own entry.
func TestPromptRulesNormalizeClearsStaleLastRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	stale := make(map[string]any)
	stale["version"] = PromptRulesVersion
	stale["rules"] = []map[string]any{{"id": "kept", "name": "Kept", "text": "text"}}
	stale["lastRule"] = "deleted"
	raw, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadPromptRules(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.LastRule != "" {
		t.Fatalf("lastRule = %q, want it cleared when it names a missing rule", doc.LastRule)
	}

	// A LastRule that names a rule in the file survives, and so does Clone.
	kept := make(map[string]any)
	kept["version"] = PromptRulesVersion
	kept["rules"] = []map[string]any{{"id": "kept", "name": "Kept", "text": "text"}}
	kept["lastRule"] = "kept"
	raw, err = json.Marshal(kept)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err = LoadPromptRules(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.LastRule != "kept" {
		t.Fatalf("lastRule = %q, want %q", doc.LastRule, "kept")
	}
	if clone := doc.Clone(); clone.LastRule != "kept" {
		t.Fatalf("Clone dropped lastRule: %q", clone.LastRule)
	}
}

func TestPromptRulesSaveFailureKeepsOldFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	doc := NewPromptRules()
	doc.Rules = []PromptRule{{ID: "keep", Name: "Keep", Text: "Keep this."}}
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Make the next save fail on every platform without skipping: on Unix a
	// read-only directory stops the temporary file before the rename; on
	// Windows the read-only destination file and the open handle below make
	// MoveFileEx fail, the same way the change list test does it.
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

	next := NewPromptRules()
	next.Rules = []PromptRule{{ID: "other", Name: "Other", Text: "Never written."}}
	if err := next.Save(path); err == nil {
		t.Fatal("saving over an unwritable file should fail")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".prompts-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("failed save left temporary files behind: %v", leftovers)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed save modified the file: %q %v", after, err)
	}
}
