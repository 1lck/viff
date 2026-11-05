package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PromptRulesVersion is the prompt rule format this build writes. A file with a
// higher version belongs to a newer Viff: it is loaded read-only and is never
// overwritten.
const PromptRulesVersion = 1

// ErrPromptRulesReadOnly reports a prompt rule file written by a newer Viff.
var ErrPromptRulesReadOnly = errors.New("prompt rules were written by a newer Viff")

// PromptRule is one named rules text. Text is what the model receives; Name
// only labels the rule in the interface, so a rule can be recognized without
// reading its whole text.
type PromptRule struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Text string `json:"text"`
}

// PromptRules is the persisted rule document. It lives next to the user's
// settings file, never in a repository or in Git's own metadata. The built-in
// rules of the program are virtual: they are supplied by package ai and are
// never stored here, so the file holds only the rules the user wrote plus the
// rule each repository uses.
//
// The selection is resolved in two steps, so a new repository inherits what the
// user last chose instead of silently falling back to the built-in rules: the
// repository's own entry wins, and a repository without one uses LastRule. Only
// when both are absent does the built-in rule apply.
type PromptRules struct {
	Version int          `json:"version"`
	Rules   []PromptRule `json:"rules"`

	// Repos maps a canonical repository root to the ID of the rule it uses.
	Repos map[string]string `json:"repositories,omitempty"`

	// LastRule is the rule selected most recently by any repository. It is the
	// default for repositories the user has not chosen for yet. Empty means the
	// built-in rules were chosen last.
	LastRule string `json:"lastRule,omitempty"`

	readOnly bool
}

// NewPromptRules returns an empty writable document.
func NewPromptRules() *PromptRules {
	return &PromptRules{Version: PromptRulesVersion, Rules: []PromptRule{}, Repos: make(map[string]string)}
}

// PromptRulesPath is where the prompt rules of a settings file live: the same
// directory, in their own file. An empty settings path disables persistence.
func PromptRulesPath(settingsPath string) string {
	if settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), "prompts.json")
}

// ReadOnly reports a document written by a newer Viff. Its rules can be shown,
// but every change is refused so the file is never overwritten.
func (p *PromptRules) ReadOnly() bool { return p.readOnly }

// Clone returns a detached copy that a caller may modify before publishing it
// as the new document. A failed save then leaves the published document, and
// therefore every read, on the last successfully saved state.
func (p *PromptRules) Clone() *PromptRules {
	next := &PromptRules{
		Version:  p.Version,
		Rules:    append([]PromptRule(nil), p.Rules...),
		Repos:    make(map[string]string, len(p.Repos)),
		LastRule: p.LastRule,
		readOnly: p.readOnly,
	}
	for root, id := range p.Repos {
		next.Repos[root] = id
	}
	return next
}

// LoadPromptRules reads the document at path. A missing file gives an empty
// document. A corrupt file is an error and is left untouched; a file from a
// newer Viff is returned read-only.
func LoadPromptRules(path string) (*PromptRules, error) {
	if path == "" {
		return nil, errors.New("prompt rules: no path")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewPromptRules(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("prompt rules: cannot read %s: %w", path, err)
	}
	var p PromptRules
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("prompt rules: %s is unreadable: %w", path, err)
	}
	if p.Version < 0 {
		return nil, fmt.Errorf("prompt rules: %s has an invalid version %d", path, p.Version)
	}
	if p.Version > PromptRulesVersion {
		p.readOnly = true
	} else {
		p.Version = PromptRulesVersion
	}
	// A valid file with inconsistent entries (an empty name, a duplicated ID, a
	// repository naming a rule that does not exist) is repaired in memory
	// instead of being rejected; nothing is written back until a change is
	// requested.
	p.normalize()
	return &p, nil
}

// Save writes the document atomically: a temporary file in the same directory,
// flushed and renamed over the target. It refuses a read-only document, and a
// failed save leaves the old file untouched.
func (p *PromptRules) Save(path string) error {
	if p.readOnly {
		return ErrPromptRulesReadOnly
	}
	if path == "" {
		return errors.New("prompt rules: no path")
	}
	p.Version = PromptRulesVersion
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("prompt rules: cannot encode: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("prompt rules: cannot create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".prompts-*.tmp")
	if err != nil {
		return fmt.Errorf("prompt rules: cannot create a temporary file in %s: %w", dir, err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("prompt rules: cannot write %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("prompt rules: cannot sync %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("prompt rules: cannot close %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return fmt.Errorf("prompt rules: cannot replace %s: %w", path, err)
	}
	return nil
}

// normalize repairs a loaded document in memory: it drops rules that have no ID
// or no name, keeps the first of a repeated ID, trims the label and text, and
// forgets the repository selections of rules that no longer exist (those
// repositories fall back to the built-in rules).
func (p *PromptRules) normalize() {
	rules := []PromptRule{}
	seen := make(map[string]bool, len(p.Rules))
	for _, r := range p.Rules {
		id := strings.TrimSpace(r.ID)
		name := strings.TrimSpace(r.Name)
		if id == "" || name == "" || seen[id] {
			continue
		}
		seen[id] = true
		rules = append(rules, PromptRule{ID: id, Name: name, Text: strings.TrimSpace(r.Text)})
	}
	p.Rules = rules
	if p.Repos == nil {
		p.Repos = make(map[string]string)
	}
	for root, id := range p.Repos {
		if root == "" || !seen[id] {
			delete(p.Repos, root)
		}
	}
	// LastRule is a fallback, so a stale value would quietly send a rule the
	// user deleted. Clearing it falls back to the built-in rules, which is what
	// a repository whose rule disappeared already gets.
	p.LastRule = strings.TrimSpace(p.LastRule)
	if p.LastRule != "" && !seen[p.LastRule] {
		p.LastRule = ""
	}
}
