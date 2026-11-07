// Package i18n holds the interface's messages and the language that selects
// them. It has no dependencies and knows nothing about the interface, so tests
// can assert message text without a window.
package i18n

import (
	"fmt"
	"runtime"
	"strings"
)

// Lang is a language the interface speaks.
type Lang string

const (
	EN   Lang = "en"
	ZHCN Lang = "zh-CN"
)

// Default is the language used when the settings name none, which is what an
// older settings file reads as.
const Default = EN

// Parse maps a stored language name to a language, falling back to Default.
func Parse(name string) Lang {
	if Lang(strings.TrimSpace(name)) == ZHCN {
		return ZHCN
	}
	return Default
}

// Key names one message. Keys are dot-separated and declared as constants, so a
// misspelling fails to compile instead of reaching the window.
type Key string

// Translator speaks one language.
type Translator struct{ lang Lang }

// New returns a translator for lang.
func New(lang Lang) *Translator { return &Translator{lang: lang} }

// Lang reports the translator's language.
func (t *Translator) Lang() Lang { return t.lang }

// T returns the message for key formatted with args. A message that needs a
// literal percent sign writes %% the way fmt does, because a lone % starts a
// verb; TestPercentInMessages pins that. A message missing from this language
// falls back to English, then to the key itself, so a gap shows up in the
// interface instead of blank text.
func (t *Translator) T(key Key, args ...any) string {
	text := message(t.lang, key)
	if text == "" && t.lang != EN {
		text = message(EN, key)
	}
	if text == "" {
		return string(key)
	}
	if len(args) == 0 {
		// Nothing to format, but a literal percent sign is still written the
		// fmt way, so the same message reads correctly with and without args.
		return strings.ReplaceAll(text, "%%", "%")
	}
	return fmt.Sprintf(text, args...)
}

// Plural returns the singular message for one and the plural one otherwise.
// Chinese uses a single form for both counts.
func (t *Translator) Plural(n int, one, many Key, args ...any) string {
	if t.lang != EN || n == 1 {
		return t.T(one, args...)
	}
	return t.T(many, args...)
}

// tables holds each language's messages. Modules register their own map from an
// init in their own file, so several of them can add keys without sharing one
// literal and without a merge conflict.
var tables = map[Lang]map[Key]string{EN: {}, ZHCN: {}}

// revealLabel is the "show in the file manager" action in the platform's own
// wording. macOS calls it Finder, Windows File Explorer and the rest just the
// file manager, so a fixed "Show in Finder" was wrong on every other system.
// The message files call it while building their tables, which keeps every
// caller on one key.
func revealLabel(lang Lang) string { return revealLabelFor(lang, runtime.GOOS) }

// revealLabelFor is revealLabel for one platform name, so the tests can pin
// Windows and Linux wording while running anywhere.
func revealLabelFor(lang Lang, goos string) string {
	name, latin := fileManagerName(lang, goos)
	if lang != ZHCN {
		return "Show in " + name
	}
	if latin {
		// A Latin name keeps a space on each side, the way Finder's own menus
		// write it.
		return "在 " + name + " 中显示"
	}
	return "在" + name + "中显示"
}

// fileManagerName is what the platform calls its file manager, and whether the
// name is Latin (which decides the spacing around it in Chinese).
func fileManagerName(lang Lang, goos string) (name string, latin bool) {
	switch goos {
	case "darwin":
		return "Finder", true
	case "windows":
		if lang == ZHCN {
			return "文件资源管理器", false
		}
		return "File Explorer", true
	default:
		if lang == ZHCN {
			return "文件管理器", false
		}
		return "File Manager", true
	}
}

func message(lang Lang, key Key) string {
	if table, ok := tables[lang]; ok {
		return table[key]
	}
	return ""
}

// register adds one module's messages. It runs from init, so the order of the
// message files does not matter.
func register(lang Lang, messages map[Key]string) {
	table, ok := tables[lang]
	if !ok {
		return
	}
	for key, text := range messages {
		table[key] = text
	}
}
