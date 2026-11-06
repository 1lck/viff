package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viff/internal/ai"
	"viff/internal/gitcli"
	"viff/internal/storage"
)

// promptRulePersist points the fixture at a temporary settings file, which is
// what enables persistence: the rules file lives next to it, never in a
// repository.
func promptRulePersist(t *testing.T, f *fixture) string {
	t.Helper()
	settingsPath := filepath.Join(t.TempDir(), "Viff", "settings.json")
	f.w = NewWorkspace(&gitcli.Runner{Env: f.env}, storage.Load(settingsPath), nil)
	return settingsPath
}

// promptRuleReopen creates another workspace over the same settings path, as a
// restart would, and opens dir in it.
func promptRuleReopen(t *testing.T, settingsPath, dir string) *Workspace {
	t.Helper()
	restarted := newFixture(t)
	restarted.w = NewWorkspace(&gitcli.Runner{Env: restarted.env}, storage.Load(settingsPath), nil)
	changeListOpen(t, restarted, dir)
	return restarted.w
}

// TestPromptRuleSaveAndReload round-trips a user rule through the file: saving
// it, restarting, and reading the same name and text back.
func TestPromptRuleSaveAndReload(t *testing.T) {
	f := newFixture(t)
	settingsPath := promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	rules := f.w.PromptRules()
	if len(rules) != 1 {
		t.Fatalf("initial rules = %+v, want only the built-in one", rules)
	}
	builtin := rules[0]
	if builtin.ID != builtinPromptRuleID || !builtin.Builtin {
		t.Fatalf("first rule = %+v, want the built-in one", builtin)
	}
	if builtin.Text != ai.DefaultRules {
		t.Fatalf("built-in text is not ai.DefaultRules: %q", builtin.Text)
	}

	id, err := f.w.SavePromptRule(PromptRule{Name: "  Terse  ", Text: "  One short line, no prefix.\n"})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || id == builtinPromptRuleID {
		t.Fatalf("saved id = %q", id)
	}
	rules = f.w.PromptRules()
	if len(rules) != 2 || rules[1].ID != id || rules[1].Name != "Terse" || rules[1].Text != "One short line, no prefix." {
		t.Fatalf("rules after save = %+v", rules)
	}
	if rules[1].Builtin {
		t.Fatal("a user rule was reported as built-in")
	}

	// The rules file lives next to the settings, not in the repository.
	path := storage.PromptRulesPath(settingsPath)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("rules file %s missing: %v", path, err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "prompts.json")); !os.IsNotExist(err) {
		t.Fatalf("rules were written into the repository: %v", err)
	}
	// The built-in rule is virtual and never stored.
	var onDisk storage.PromptRules
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	for _, r := range onDisk.Rules {
		if r.ID == builtinPromptRuleID {
			t.Fatal("the built-in rule was written to the file")
		}
	}

	reloaded := promptRuleReopen(t, settingsPath, f.dir)
	rules = reloaded.PromptRules()
	if len(rules) != 2 || rules[1].ID != id || rules[1].Name != "Terse" || rules[1].Text != "One short line, no prefix." {
		t.Fatalf("rules after reload = %+v", rules)
	}

	// A second save with the same ID updates the rule in place.
	if _, err := reloaded.SavePromptRule(PromptRule{ID: id, Name: "Terser", Text: "Even shorter."}); err != nil {
		t.Fatal(err)
	}
	again := promptRuleReopen(t, settingsPath, f.dir)
	rules = again.PromptRules()
	if len(rules) != 2 || rules[1].ID != id || rules[1].Name != "Terser" || rules[1].Text != "Even shorter." {
		t.Fatalf("rules after update = %+v", rules)
	}
}

// TestPromptRuleValidationRefusesEmptyInput keeps a nameless or empty rule out
// of the file, and refuses to overwrite the built-in one.
func TestPromptRuleValidationRefusesEmptyInput(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "a\n")
	f.open()

	var refused []PromptRule
	refused = append(refused, PromptRule{Name: "  ", Text: "text"})
	refused = append(refused, PromptRule{Name: "Name", Text: "  "})
	refused = append(refused, PromptRule{ID: builtinPromptRuleID, Name: "Name", Text: "text"})
	refused = append(refused, PromptRule{Builtin: true, Name: "Name", Text: "text"})
	for _, rule := range refused {
		if _, err := f.w.SavePromptRule(rule); err == nil {
			t.Fatalf("saving %+v should fail", rule)
		}
	}
	if rules := f.w.PromptRules(); len(rules) != 1 {
		t.Fatalf("a refused save stored a rule: %+v", rules)
	}
}

// TestPromptRuleSelectionPersists pins the per-repository choice: it survives a
// restart and is read back by SelectedPromptRule.
func TestPromptRuleSelectionPersists(t *testing.T) {
	f := newFixture(t)
	settingsPath := promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	if got := f.w.SelectedPromptRule(); got.ID != builtinPromptRuleID {
		t.Fatalf("default selection = %+v, want the built-in rule", got)
	}
	id, err := f.w.SavePromptRule(PromptRule{Name: "Custom", Text: "Custom rules text."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatal(err)
	}
	if got := f.w.SelectedPromptRule(); got.ID != id || got.Text != "Custom rules text." || got.Builtin {
		t.Fatalf("selection = %+v, want the custom rule", got)
	}

	reloaded := promptRuleReopen(t, settingsPath, f.dir)
	if got := reloaded.SelectedPromptRule(); got.ID != id || got.Text != "Custom rules text." {
		t.Fatalf("selection after reload = %+v, want the custom rule", got)
	}
}

// TestPromptRuleSelectionFallsBackToBuiltin covers every way a selection can
// resolve to nothing: an unknown ID, a deleted rule, and a repository with no
// entry at all.
func TestPromptRuleSelectionFallsBackToBuiltin(t *testing.T) {
	f := newFixture(t)
	promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	if err := f.w.SelectPromptRule("missing"); err == nil {
		t.Fatal("selecting an unknown rule should fail")
	}
	if got := f.w.SelectedPromptRule(); got.ID != builtinPromptRuleID || !got.Builtin {
		t.Fatalf("selection after a refused select = %+v, want the built-in rule", got)
	}

	id, err := f.w.SavePromptRule(PromptRule{Name: "Custom", Text: "Custom rules text."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatal(err)
	}

	// Deleting the selected rule makes the repository fall back to built-in.
	if err := f.w.DeletePromptRule(id); err != nil {
		t.Fatal(err)
	}
	got := f.w.SelectedPromptRule()
	if got.ID != builtinPromptRuleID || !got.Builtin {
		t.Fatalf("selection after deleting the rule = %+v, want the built-in rule", got)
	}
	// The built-in row is labelled as read-only: it is the fixed point the
	// generator falls back to, so its name must not suggest it is editable.
	if got.Text != ai.DefaultRules || got.Name != "Default (read-only)" {
		t.Fatalf("fallback rule = %+v, want the built-in text and its read-only label", got)
	}
	if rules := f.w.PromptRules(); len(rules) != 1 {
		t.Fatalf("rules after delete = %+v, want only the built-in one", rules)
	}

	// An ID that names nothing stored, reached through the file, also falls back.
	if err := f.w.DeletePromptRule("missing"); err == nil {
		t.Fatal("deleting an unknown rule should fail")
	}
	if err := f.w.DeletePromptRule(""); err == nil {
		t.Fatal("deleting an empty ID should fail")
	}
	if err := f.w.SelectPromptRule(""); err == nil {
		t.Fatal("selecting an empty ID should fail")
	}
}

// TestPromptRuleBuiltinIsVirtual pins the asymmetry: selecting the built-in ID
// is how a repository returns to the default rules, so it succeeds and stores
// nothing; deleting it is refused, because there is nothing stored to delete.
func TestPromptRuleBuiltinIsVirtual(t *testing.T) {
	f := newFixture(t)
	settingsPath := promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	if err := f.w.DeletePromptRule(builtinPromptRuleID); err == nil {
		t.Fatal("deleting the built-in rule should fail")
	}
	if _, err := f.w.SavePromptRule(PromptRule{ID: builtinPromptRuleID, Name: "Overwrite", Text: "text"}); err == nil {
		t.Fatal("overwriting the built-in rule should fail")
	}

	id, err := f.w.SavePromptRule(PromptRule{Name: "Custom", Text: "Custom rules text."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(builtinPromptRuleID); err != nil {
		t.Fatalf("selecting the built-in rule should succeed: %v", err)
	}
	if got := f.w.SelectedPromptRule(); got.ID != builtinPromptRuleID {
		t.Fatalf("selection = %+v, want the built-in rule", got)
	}

	// Selecting it again is a no-op and still leaves no entry behind.
	if err := f.w.SelectPromptRule(builtinPromptRuleID); err != nil {
		t.Fatal(err)
	}
	path := storage.PromptRulesPath(settingsPath)
	doc, err := storage.LoadPromptRules(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Repos) != 0 {
		t.Fatalf("the built-in selection was stored: %+v", doc.Repos)
	}
	// The rule itself is still there, only the selection went away.
	if rules := f.w.PromptRules(); len(rules) != 2 {
		t.Fatalf("rules = %+v, want the built-in and the custom one", rules)
	}
}

// TestPromptRuleNewRepositoryUsesTheLastChoice pins the second step of the
// fallback chain: a repository the user has never chosen for follows the rule
// they selected last anywhere, instead of silently dropping back to the
// built-in rules and making the earlier choice look ignored.
func TestPromptRuleNewRepositoryUsesTheLastChoice(t *testing.T) {
	f := newFixture(t)
	promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	id, err := f.w.SavePromptRule(PromptRule{Name: "Custom", Text: "Custom rules text."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatal(err)
	}

	other := newFixture(t)
	other.write("a.txt", "a\n")
	changeListOpen(t, f, other.dir)
	got := f.w.SelectedPromptRule()
	if got.ID != id {
		t.Fatalf("a new repository = %+v, want the rule selected last (%q)", got, id)
	}

	// Its own choice then wins and survives a switch away and back.
	second, err := f.w.SavePromptRule(PromptRule{Name: "Second", Text: "Second rules text."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(second); err != nil {
		t.Fatal(err)
	}
	changeListOpen(t, f, f.dir)
	if got := f.w.SelectedPromptRule(); got.ID != id {
		t.Fatalf("first repository selection = %+v, want %q", got, id)
	}
	changeListOpen(t, f, other.dir)
	if got := f.w.SelectedPromptRule(); got.ID != second {
		t.Fatalf("second repository selection = %+v, want %q", got, second)
	}

	// A linked worktree has its own root and no entry, so it follows the last
	// choice rather than the repository it was created from.
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	wt := filepath.Join(t.TempDir(), "wt")
	f.git("worktree", "add", "-q", "-b", "side", wt)
	changeListOpen(t, f, wt)
	if got := f.w.SelectedPromptRule(); got.ID != second {
		t.Fatalf("linked worktree = %+v, want the rule selected last (%q)", got, second)
	}
	changeListOpen(t, f, f.dir)
	if got := f.w.SelectedPromptRule(); got.ID != id {
		t.Fatalf("main checkout selection after worktree use = %+v, want %q", got, id)
	}
}

// TestPromptRuleLastChoiceIsPersisted keeps the default across a restart: the
// rule used last is on disk, not just in memory.
func TestPromptRuleLastChoiceIsPersisted(t *testing.T) {
	f := newFixture(t)
	settingsPath := promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	id, err := f.w.SavePromptRule(PromptRule{Name: "Custom", Text: "Custom rules text."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatal(err)
	}

	doc, err := storage.LoadPromptRules(storage.PromptRulesPath(settingsPath))
	if err != nil {
		t.Fatal(err)
	}
	if doc.LastRule != id {
		t.Fatalf("lastRule = %q, want %q", doc.LastRule, id)
	}

	restarted := promptRuleReopen(t, settingsPath, f.dir)
	if got := restarted.SelectedPromptRule(); got.ID != id {
		t.Fatalf("selection after a restart = %+v, want %q", got, id)
	}
}

// TestPromptRuleChoosingBuiltinClearsTheLastChoice covers the way back: picking
// the built-in rules must not leave a custom rule as the default for the next
// repository the user opens.
func TestPromptRuleChoosingBuiltinClearsTheLastChoice(t *testing.T) {
	f := newFixture(t)
	settingsPath := promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	id, err := f.w.SavePromptRule(PromptRule{Name: "Custom", Text: "Custom rules text."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(builtinPromptRuleID); err != nil {
		t.Fatalf("selecting the built-in rules was refused: %v", err)
	}

	doc, err := storage.LoadPromptRules(storage.PromptRulesPath(settingsPath))
	if err != nil {
		t.Fatal(err)
	}
	if doc.LastRule != "" {
		t.Fatalf("lastRule = %q, want it cleared", doc.LastRule)
	}
	if len(doc.Repos) != 0 {
		t.Fatalf("repositories = %v, want no stored selection", doc.Repos)
	}

	other := newFixture(t)
	other.write("a.txt", "a\n")
	changeListOpen(t, f, other.dir)
	if got := f.w.SelectedPromptRule(); got.ID != builtinPromptRuleID {
		t.Fatalf("a new repository = %+v, want the built-in rules", got)
	}
}

// TestDeletingTheLastChoiceFallsBackToBuiltin keeps a dangling default from
// pointing at a rule that no longer exists.
func TestDeletingTheLastChoiceFallsBackToBuiltin(t *testing.T) {
	f := newFixture(t)
	settingsPath := promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	id, err := f.w.SavePromptRule(PromptRule{Name: "Custom", Text: "Custom rules text."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatal(err)
	}
	if err := f.w.DeletePromptRule(id); err != nil {
		t.Fatal(err)
	}

	doc, err := storage.LoadPromptRules(storage.PromptRulesPath(settingsPath))
	if err != nil {
		t.Fatal(err)
	}
	if doc.LastRule != "" {
		t.Fatalf("lastRule = %q after deleting the rule, want it cleared", doc.LastRule)
	}
	if got := f.w.SelectedPromptRule(); got.ID != builtinPromptRuleID {
		t.Fatalf("selection after deleting the rule = %+v, want the built-in rules", got)
	}
}

// TestPromptRuleWithoutSettingsPath keeps a workspace built without a settings
// file working: the selection lives in memory and nothing is written anywhere.
func TestPromptRuleWithoutSettingsPath(t *testing.T) {
	f := newFixture(t) // settings.Defaults() has no path
	f.write("a.txt", "a\n")
	f.open()

	id, err := f.w.SavePromptRule(PromptRule{Name: "Memory", Text: "Kept in memory only."})
	if err != nil {
		t.Fatalf("saving without a settings path should succeed: %v", err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatalf("selecting without a settings path should succeed: %v", err)
	}
	if got := f.w.SelectedPromptRule(); got.ID != id || got.Text != "Kept in memory only." {
		t.Fatalf("in-memory selection = %+v", got)
	}
	if rules := f.w.PromptRules(); len(rules) != 2 || rules[1].Name != "Memory" {
		t.Fatalf("in-memory rules = %+v", rules)
	}
	if err := f.w.SelectPromptRule(builtinPromptRuleID); err != nil {
		t.Fatalf("selecting the built-in rule without a settings path should succeed: %v", err)
	}
	if got := f.w.SelectedPromptRule(); got.ID != builtinPromptRuleID {
		t.Fatalf("in-memory fallback = %+v, want the built-in rule", got)
	}
}

// TestPromptRuleSaveFailureKeepsState pins the copy-then-save order: a write
// that fails leaves both the published rules and the old file as they were.
func TestPromptRuleSaveFailureKeepsState(t *testing.T) {
	f := newFixture(t)
	settingsPath := promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	id, err := f.w.SavePromptRule(PromptRule{Name: "Keep", Text: "Keep this text."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatal(err)
	}
	path := storage.PromptRulesPath(settingsPath)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// The open handle and a read-only file make the rename fail on every
	// platform without skipping; the already saved rules must survive.
	dir := filepath.Dir(path)
	locked, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	restore := func() {
		if locked != nil {
			if err := locked.Close(); err != nil {
				t.Errorf("close rules file: %v", err)
			}
			locked = nil
		}
		if err := os.Chmod(path, 0o600); err != nil {
			t.Errorf("restore rules mode: %v", err)
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

	if _, err := f.w.SavePromptRule(PromptRule{Name: "Lost", Text: "Lost text."}); err == nil {
		t.Fatal("saving a rule should fail when the file cannot be written")
	}
	if err := f.w.SelectPromptRule(builtinPromptRuleID); err == nil {
		t.Fatal("selecting should fail when the file cannot be written")
	}
	if err := f.w.DeletePromptRule(id); err == nil {
		t.Fatal("deleting should fail when the file cannot be written")
	}

	rules := f.w.PromptRules()
	if len(rules) != 2 || rules[1].ID != id || rules[1].Name != "Keep" || rules[1].Text != "Keep this text." {
		t.Fatalf("memory changed after a failed save: %+v", rules)
	}
	if got := f.w.SelectedPromptRule(); got.ID != id {
		t.Fatalf("selection changed after a failed save: %+v", got)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("a failed save modified the file: %q %v", after, err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".prompts-*.tmp")); len(leftovers) != 0 {
		t.Fatalf("a failed save left temporary files behind: %v", leftovers)
	}

	// Once writing is possible again the state can still be changed.
	restore()
	if err := f.w.DeletePromptRule(id); err != nil {
		t.Fatalf("the rules did not recover after the failed save: %v", err)
	}
}

// TestPromptRuleReadOnlyRefusesChanges covers a file written by a newer Viff:
// it can be read, but every change is refused with the storage error.
func TestPromptRuleReadOnlyRefusesChanges(t *testing.T) {
	f := newFixture(t)
	settingsPath := promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	path := storage.PromptRulesPath(settingsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	newer := []byte(`{"version":99,"rules":[{"id":"future","name":"Future","text":"From a newer Viff."}]}`)
	if err := os.WriteFile(path, newer, 0o600); err != nil {
		t.Fatal(err)
	}

	rules := f.w.PromptRules()
	if len(rules) != 2 || rules[1].ID != "future" {
		t.Fatalf("read-only rules = %+v, want the stored rule", rules)
	}
	if err := f.w.SelectPromptRule("future"); !errors.Is(err, storage.ErrPromptRulesReadOnly) {
		t.Fatalf("select err = %v, want the read-only error", err)
	}
	if _, err := f.w.SavePromptRule(PromptRule{Name: "New", Text: "New text."}); !errors.Is(err, storage.ErrPromptRulesReadOnly) {
		t.Fatalf("save err = %v, want the read-only error", err)
	}
	if err := f.w.DeletePromptRule("future"); !errors.Is(err, storage.ErrPromptRulesReadOnly) {
		t.Fatalf("delete err = %v, want the read-only error", err)
	}
	// The notice is raised once, not once per refusal.
	warnings := 0
	for _, n := range f.w.Notifications {
		if strings.Contains(n.Title, "rules are unavailable") {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("read-only notices = %d, want exactly one", warnings)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(newer) {
		t.Fatalf("a newer file was overwritten: %q %v", data, err)
	}
}

// TestPromptRuleCorruptFileIsReported keeps a broken file usable: reads degrade
// to the built-in rules, changes report the problem, and the file is left alone.
func TestPromptRuleCorruptFileIsReported(t *testing.T) {
	f := newFixture(t)
	settingsPath := promptRulePersist(t, f)
	f.write("a.txt", "a\n")
	changeListOpen(t, f, f.dir)

	path := storage.PromptRulesPath(settingsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	broken := []byte(`{"version":1,`)
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatal(err)
	}

	rules := f.w.PromptRules()
	if len(rules) != 1 || rules[0].ID != builtinPromptRuleID {
		t.Fatalf("corrupt file should degrade to the built-in rules: %+v", rules)
	}
	if got := f.w.SelectedPromptRule(); got.ID != builtinPromptRuleID {
		t.Fatalf("corrupt file selection = %+v, want the built-in rule", got)
	}
	if _, err := f.w.SavePromptRule(PromptRule{Name: "Feature", Text: "text"}); err == nil {
		t.Fatal("saving should report the corrupt file")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(broken) {
		t.Fatalf("the corrupt file was overwritten: %q %v", data, err)
	}
}
