package ai

import (
	"strings"
	"testing"
)

// TestSystemPromptDefaultRules pins the built-in path: no rules means the
// built-in text, and no language means no suffix at all.
func TestSystemPromptDefaultRules(t *testing.T) {
	if got := SystemPrompt("", ""); got != DefaultRules {
		t.Fatalf("SystemPrompt(\"\", \"\") = %q, want the default rules", got)
	}
	// Whitespace only is a rule text the user never really wrote, so it keeps
	// the built-in rules instead of sending an empty system message.
	if got := SystemPrompt("  \n\t ", "en"); got != DefaultRules {
		t.Fatalf("whitespace rules = %q, want the default rules", got)
	}
	// English (and any en-* tag) is the wording the rules are already written
	// in, so no language instruction is appended.
	if got := SystemPrompt("", "en"); got != DefaultRules {
		t.Fatalf("SystemPrompt(\"\", \"en\") = %q, want the default rules", got)
	}
}

// TestSystemPromptBuiltinKeepsConventionalCommitsWording pins that the built-in
// path still names the Conventional Commits tokens: the suffix was written for
// that format and the output must stay what it was before rules existed.
func TestSystemPromptBuiltinKeepsConventionalCommitsWording(t *testing.T) {
	got := SystemPrompt("", "zh-CN")
	if !strings.HasPrefix(got, DefaultRules) {
		t.Fatalf("built-in rules are missing from the system prompt: %q", got)
	}
	if !strings.Contains(got, "BCP 47 tag zh-CN") {
		t.Fatalf("system prompt does not name the language: %q", got)
	}
	if !strings.Contains(got, "Conventional Commits tokens") {
		t.Fatalf("built-in path lost the Conventional Commits wording: %q", got)
	}
}

// TestSystemPromptCustomRules pins the neutral suffix: a user rule text may
// describe any format, so only the tokens the rules themselves define are kept
// in English.
func TestSystemPromptCustomRules(t *testing.T) {
	rules := "Use one short lowercase sentence, no prefix."
	got := SystemPrompt(rules, "zh-CN")
	if !strings.HasPrefix(got, rules) {
		t.Fatalf("system prompt does not start with the custom rules: %q", got)
	}
	if !strings.Contains(got, "zh-CN") {
		t.Fatalf("system prompt does not name the language: %q", got)
	}
	if strings.Contains(got, "Conventional Commits tokens") {
		t.Fatalf("custom rules kept the built-in wording: %q", got)
	}
	if !strings.Contains(got, customPrecedenceSuffix) {
		t.Fatalf("custom rules are not protected from the context examples: %q", got)
	}
}

// TestSystemPromptCustomRulesWithoutLanguage keeps a lone custom text intact:
// the system message is the rules, trimmed, followed by the precedence note
// that stops the context examples from overriding them.
func TestSystemPromptCustomRulesWithoutLanguage(t *testing.T) {
	want := "Use sentence case.\n\n" + customPrecedenceSuffix
	if got := SystemPrompt("\n  Use sentence case.  \n", ""); got != want {
		t.Fatalf("SystemPrompt custom rules = %q, want %q", got, want)
	}
}
