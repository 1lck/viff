package ai

import (
	"fmt"
	"strings"
)

// DefaultRules is the built-in instruction text: Conventional Commits, chosen
// because it is the format the rest of the program already understands. It is
// used whenever the user has not written rules of their own.
const DefaultRules = `You write Git commit messages that follow the Conventional Commits format.

Format: <type>[optional scope]: <description>
An optional body follows one blank line after the description; footers such as
BREAKING CHANGE: follow one blank line after the body.

Rules:
- type is one of feat, fix, docs, style, refactor, perf, test, build, ci, chore,
  revert, chosen by what the change actually does.
- scope is optional, lowercase, and names the area touched (a package, module or
  directory). Omit it when the change is broad.
- description is imperative mood ("add", not "added"), lowercase, without a
  trailing period, and at most 72 characters.
- Add a body only when the diff needs explaining beyond the description.
- Mark incompatible changes with "BREAKING CHANGE:" in a footer.
- Read the diff: ignore formatting noise, machine-generated files and lock
  files when choosing the type.

Return only the commit message, with no quotes, no code fences and no
explanation.`

// Language instruction appended when the interface is not English. The built-in
// rules name Conventional Commits, so the suffix can name those tokens; a
// custom rules text may define a completely different format, and the neutral
// variant therefore only asks to keep the tokens the rules themselves define.
const builtinLanguageSuffix = "Write the whole message in the language named by the BCP 47 tag %s; keep the Conventional Commits tokens (type, scope, BREAKING CHANGE) in English."

const neutralLanguageSuffix = "Write the message in the language named by the BCP 47 tag %s; keep any literal tokens defined by the rules above in English."

// customPrecedenceSuffix settles the conflict between a user rule and the Git
// context in the user message: recent commits and a previous generated message
// are examples the model otherwise copies, and a rule such as "no prefix" would
// lose against them. It is never appended to the built-in rules, so that path
// stays byte-for-byte what it was before rules existed.
const customPrecedenceSuffix = "These rules take precedence over any example in the context; do not imitate the format of recent commits or previous messages."

// SystemPrompt assembles the system message: the user's rules when they set
// any, otherwise DefaultRules, followed by the precedence note for custom rules
// and then the language instruction.
func SystemPrompt(rules, language string) string {
	base := strings.TrimSpace(rules)
	suffix := neutralLanguageSuffix
	if base == "" {
		base = DefaultRules
		suffix = builtinLanguageSuffix
	} else {
		base += "\n\n" + customPrecedenceSuffix
	}
	if lang := strings.TrimSpace(language); lang != "" && !strings.HasPrefix(lang, "en") {
		base += "\n\n" + fmt.Sprintf(suffix, lang)
	}
	return base
}
