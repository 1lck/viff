package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"viff/internal/ai"
	"viff/internal/i18n"
	"viff/internal/storage"
)

// builtinPromptRuleID names the virtual rule: the built-in Conventional
// Commits rules of package ai. It is never stored, because the file would then
// freeze a copy of text a newer Viff may have improved.
const builtinPromptRuleID = "builtin"

// PromptRule is one commit-message rule set offered to the AI generator. Text is
// what the model receives as its system prompt; Name only labels the rule in the
// interface. The built-in rule is virtual: its text comes from package ai and it
// cannot be edited or deleted.
type PromptRule struct {
	ID      string
	Name    string
	Text    string
	Builtin bool
}

// PromptManager keeps the user's commit-message rules and the rule each
// repository selected. One repository is identified by its canonical root, so
// two checkouts keep independent choices and a linked worktree has its own.
//
// The manager is created lazily by Workspace.prompts and its document is only
// ever replaced as a whole after a successful save: a failed write leaves both
// memory and the old file untouched. Reads look at the published document
// directly, so a repaint never touches the disk.
type PromptManager struct {
	mu sync.Mutex

	// path is the JSON file next to the settings; "" disables persistence
	// (settings without a path, mostly tests).
	path string

	doc     *storage.PromptRules
	loaded  bool
	loadErr error
	warned  bool

	// roots caches the canonical key of a Repo.Root after the first use, so a
	// read never resolves symlinks again.
	roots map[string]string
}

var errPromptRuleBuiltin = errors.New("the built-in commit message rules cannot be changed")

func newPromptManager(settingsPath string) *PromptManager {
	return &PromptManager{path: storage.PromptRulesPath(settingsPath)}
}

// prompts returns the manager, creating it on first use. Workspace owns the
// field; this file only fills it in, so a workspace that never generates a
// commit message pays nothing.
func (w *Workspace) prompts() *PromptManager {
	if m := w.Prompts; m != nil {
		return m
	}
	settingsPath := ""
	if w.Settings != nil {
		settingsPath = w.Settings.Path()
	}
	m := newPromptManager(settingsPath)
	w.Prompts = m
	return m
}

// docLocked loads the document once and caches the result. A corrupt file never
// makes the message generator unusable: reads degrade to the built-in rules and
// every change reports the error instead of overwriting the file.
func (m *PromptManager) docLocked() (*storage.PromptRules, error) {
	if m.loaded {
		return m.doc, m.loadErr
	}
	m.loaded = true
	if m.path == "" {
		m.doc = storage.NewPromptRules()
		return m.doc, nil
	}
	doc, err := storage.LoadPromptRules(m.path)
	if err != nil {
		m.doc = storage.NewPromptRules()
		m.loadErr = err
		return m.doc, err
	}
	m.doc = doc
	return doc, nil
}

// canonicalRootLocked maps root to the key used in the rules file, resolving
// symlinks once per distinct root. canonicalPath is the package's one canonical
// form, so the rules and the change lists of a repository are keyed the same
// way. The caller holds m.mu.
func (m *PromptManager) canonicalRootLocked(root string) string {
	if m.roots == nil {
		m.roots = make(map[string]string)
	}
	if canonical, ok := m.roots[root]; ok {
		return canonical
	}
	canonical := canonicalPath(root)
	m.roots[root] = canonical
	return canonical
}

// warnPrompts reports a broken file once instead of on every frame. The caller
// holds m.mu; w.Notify must not touch the manager.
func (w *Workspace) warnPrompts(m *PromptManager, err error) {
	if err == nil || m.warned {
		return
	}
	m.warned = true
	w.Notify(NoticeWarning, w.tr(i18n.AppNoticePromptRulesUnavailable), err.Error())
}

// builtinPromptRule is the virtual rule: the built-in text of package ai. Its
// label is translated and says the rules cannot be changed, because the row is
// the fixed point the generator falls back to.
func (w *Workspace) builtinPromptRule() PromptRule {
	return PromptRule{
		ID:      builtinPromptRuleID,
		Name:    w.tr(i18n.WorkspaceAIRulesBuiltinName),
		Text:    ai.DefaultRules,
		Builtin: true,
	}
}

// promptRuleAt returns the stored rule with id, and whether such a rule exists.
// It never returns the built-in rules: those are not stored.
func promptRuleAt(doc *storage.PromptRules, id string) (storage.PromptRule, bool) {
	index := promptRuleIndex(doc, id)
	if index < 0 {
		return storage.PromptRule{}, false
	}
	return doc.Rules[index], true
}

// promptRuleIndex is the position of the rule with id, or -1.
func promptRuleIndex(doc *storage.PromptRules, id string) int {
	if id == "" {
		return -1
	}
	for i, r := range doc.Rules {
		if r.ID == id {
			return i
		}
	}
	return -1
}

// newPromptRuleID returns an ID that is stable across renames and reloads and
// does not depend on a rule's position or name, mirroring newChangeListID.
func newPromptRuleID(doc *storage.PromptRules) string {
	for i := 0; i < 100; i++ {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			break
		}
		id := "rule-" + hex.EncodeToString(b[:])
		if _, taken := promptRuleAt(doc, id); !taken {
			return id
		}
	}
	for n := 1; ; n++ {
		id := fmt.Sprintf("rule-%d", n)
		if _, taken := promptRuleAt(doc, id); !taken {
			return id
		}
	}
}

// promptUpdate applies fn to a copy of the document and writes the whole file.
// fn reports whether anything changed; when it did not, nothing is written. On
// any error the cached document, and therefore every read, keeps the last
// successfully saved state.
func (w *Workspace) promptUpdate(fn func(doc *storage.PromptRules) (bool, error)) error {
	m := w.prompts()
	m.mu.Lock()
	defer m.mu.Unlock()

	var doc *storage.PromptRules
	if m.path == "" {
		// No file: the published document is the only copy, and Save is a no-op
		// success, so a missing path never fails an action.
		loaded, err := m.docLocked()
		if err != nil {
			return err
		}
		doc = loaded
	} else {
		// Read the file again: another window (or a manual edit) may have changed
		// it since this manager loaded it. A broken file is reported instead of
		// being replaced by this manager's stale copy.
		loaded, err := storage.LoadPromptRules(m.path)
		if err != nil {
			m.loadErr = err
			return err
		}
		doc = loaded
	}
	if doc.ReadOnly() {
		// A file written by a newer Viff is readable but never written back, so
		// the message is refused; the notice is raised once per session.
		err := fmt.Errorf("cannot change the commit message rules: %w", storage.ErrPromptRulesReadOnly)
		w.warnPrompts(m, err)
		return err
	}
	next := doc.Clone()
	changed, err := fn(next)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if m.path != "" {
		if err := next.Save(m.path); err != nil {
			return fmt.Errorf("cannot save the commit message rules: %w", err)
		}
	}
	m.doc = next
	m.loaded = true
	m.loadErr = nil
	m.warned = false
	return nil
}

// PromptRules returns the rules offered to the message generator: the built-in
// Conventional Commits rules first, then the user's rules sorted by name. It
// works without an open repository, because the rules are a preference of the
// user; only the selection is per repository.
func (w *Workspace) PromptRules() []PromptRule {
	out := []PromptRule{w.builtinPromptRule()}
	m := w.prompts()
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.docLocked()
	if err != nil {
		w.warnPrompts(m, err)
		return out
	}
	saved := append([]storage.PromptRule(nil), doc.Rules...)
	sort.SliceStable(saved, func(i, j int) bool {
		left, right := strings.ToLower(saved[i].Name), strings.ToLower(saved[j].Name)
		if left == right {
			return saved[i].ID < saved[j].ID
		}
		return left < right
	})
	for _, r := range saved {
		out = append(out, PromptRule{ID: r.ID, Name: r.Name, Text: r.Text})
	}
	return out
}

// promptRuleText is the value ai.Config.Rules carries for a rule. The built-in
// rule maps to an empty string, which is what ai.Config documents for "the
// built-in Conventional Commits rules": that branch of ai.SystemPrompt also
// picks the Conventional Commits language instruction, so sending the built-in
// text as if it were a user rule would silently change what a non-English user
// is told. A user rule with no text falls back to the same built-in path.
func promptRuleText(rule PromptRule) string {
	if rule.Builtin {
		return ""
	}
	return strings.TrimSpace(rule.Text)
}

// SelectedPromptRule returns the rule the active repository uses.
//
// The selection is resolved in three steps so that a repository the user has
// not chosen for yet follows what they picked last instead of silently falling
// back to text they never chose: the repository's own entry wins, then the rule
// selected most recently anywhere (doc.LastRule), then the built-in rules. An
// entry whose rule was deleted is skipped rather than returned, and the chain
// always ends at the built-in rules: an empty system prompt would leave the
// model without any instruction.
func (w *Workspace) SelectedPromptRule() PromptRule {
	m := w.prompts()
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, err := m.docLocked()
	if err != nil {
		w.warnPrompts(m, err)
		return w.builtinPromptRule()
	}
	if root, ok := w.changeListRepoRoot(); ok {
		if rule, found := promptRuleAt(doc, doc.Repos[m.canonicalRootLocked(root)]); found {
			return PromptRule{ID: rule.ID, Name: rule.Name, Text: rule.Text}
		}
	}
	if rule, found := promptRuleAt(doc, doc.LastRule); found {
		return PromptRule{ID: rule.ID, Name: rule.Name, Text: rule.Text}
	}
	return w.builtinPromptRule()
}

// SelectPromptRule chooses the rule the active repository uses.
//
// The choice is written twice: as this repository's own entry, and as LastRule,
// the default for repositories the user has not chosen for yet. Without the
// second write, opening a repository for the first time would silently fall back
// to the built-in rules and the user's last choice would look ignored.
//
// Selecting the built-in rules forgets the repository's entry instead of storing
// it, and clears LastRule for the same reason: the built-in text is virtual, and
// storing its ID would pin the choice to text a newer Viff may rewrite.
// DeletePromptRule, in contrast, refuses the built-in ID, because there is
// nothing stored for it to remove.
func (w *Workspace) SelectPromptRule(id string) error {
	root, ok := w.changeListRepoRoot()
	if !ok {
		return errChangeListNoRepo
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("a commit message rule must be named")
	}
	m := w.prompts()
	return w.promptUpdate(func(doc *storage.PromptRules) (bool, error) {
		key := m.canonicalRootLocked(root)
		if id == builtinPromptRuleID {
			_, hasEntry := doc.Repos[key]
			if !hasEntry && doc.LastRule == "" {
				return false, nil
			}
			delete(doc.Repos, key)
			doc.LastRule = ""
			return true, nil
		}
		if _, ok := promptRuleAt(doc, id); !ok {
			return false, fmt.Errorf("commit message rule %q does not exist", id)
		}
		if doc.Repos[key] == id && doc.LastRule == id {
			return false, nil
		}
		doc.Repos[key] = id
		doc.LastRule = id
		return true, nil
	})
}

// SavePromptRule creates or updates a rule and returns the ID of the saved one.
// An empty ID creates a rule with a fresh ID; a known ID updates that rule in
// place, so saving keeps the name stable and every repository that selected the
// rule keeps its selection. The built-in rules cannot be overwritten.
func (w *Workspace) SavePromptRule(rule PromptRule) (string, error) {
	if rule.Builtin || rule.ID == builtinPromptRuleID {
		return "", errPromptRuleBuiltin
	}
	name := strings.TrimSpace(rule.Name)
	if name == "" {
		return "", errors.New("a commit message rule needs a name")
	}
	text := strings.TrimSpace(rule.Text)
	if text == "" {
		return "", errors.New("a commit message rule needs its rules text")
	}
	id := strings.TrimSpace(rule.ID)
	var saved string
	err := w.promptUpdate(func(doc *storage.PromptRules) (bool, error) {
		if id != "" {
			// A known ID is updated in place, so every repository that selected
			// the rule keeps its selection across the rename.
			if index := promptRuleIndex(doc, id); index >= 0 {
				saved = id
				if doc.Rules[index].Name == name && doc.Rules[index].Text == text {
					return false, nil
				}
				doc.Rules[index] = storage.PromptRule{ID: id, Name: name, Text: text}
				return true, nil
			}
		} else {
			id = newPromptRuleID(doc)
		}
		// A new rule, or an ID that is not stored (its rule was deleted while the
		// editor was open) is created with the ID the caller asked for.
		saved = id
		doc.Rules = append(doc.Rules, storage.PromptRule{ID: id, Name: name, Text: text})
		return true, nil
	})
	if err != nil {
		return "", err
	}
	return saved, nil
}

// DeletePromptRule removes a rule. Every repository that selected it falls back
// to the built-in rules. The built-in rules themselves cannot be deleted.
func (w *Workspace) DeletePromptRule(id string) error {
	id = strings.TrimSpace(id)
	if id == builtinPromptRuleID {
		return errPromptRuleBuiltin
	}
	if id == "" {
		return errors.New("a commit message rule must be named")
	}
	return w.promptUpdate(func(doc *storage.PromptRules) (bool, error) {
		index := promptRuleIndex(doc, id)
		if index < 0 {
			return false, fmt.Errorf("commit message rule %q does not exist", id)
		}
		doc.Rules = append(doc.Rules[:index:index], doc.Rules[index+1:]...)
		// Every repository that selected the rule falls back to the built-in
		// rules: forgetting the entry is exactly what "built-in" means here.
		for root, selected := range doc.Repos {
			if selected == id {
				delete(doc.Repos, root)
			}
		}
		// The last-used default must not name a rule that is gone, or every
		// repository without its own entry would resolve to nothing.
		if doc.LastRule == id {
			doc.LastRule = ""
		}
		return true, nil
	})
}
