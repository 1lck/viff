package i18n

import (
	"strings"
	"testing"
)

// Every language must define every key with non-empty text. A half-filled table
// would show a mix of translated and fallback text instead of one language, and
// a key that only exists in one table is almost always a typo in the other.
func TestTablesAreComplete(t *testing.T) {
	for lang, table := range tables {
		for key, text := range table {
			if text == "" {
				t.Errorf("%s: %s is empty", lang, key)
			}
		}
	}
	for key := range tables[EN] {
		if _, ok := tables[ZHCN][key]; !ok {
			t.Errorf("%s has no %s text", key, ZHCN)
		}
	}
	for key := range tables[ZHCN] {
		if _, ok := tables[EN][key]; !ok {
			t.Errorf("%s has no %s text", key, EN)
		}
	}
}

// An unknown language falls back to English, and an unknown key falls back to
// the key itself, so a gap shows up instead of blank text.
func TestTranslatorFallsBack(t *testing.T) {
	zh := New(ZHCN)
	if got := zh.T(SettingsTitle); got != "设置" {
		t.Fatalf("zh title = %q, want 设置", got)
	}
	if got := zh.T(Key("no.such.key")); got != "no.such.key" {
		t.Fatalf("missing key = %q, want the key itself", got)
	}
	if got := New(Parse("")).T(SettingsTitle); got != "Settings" {
		t.Fatalf("empty language = %q, want English", got)
	}
}

// A message formats its arguments, so a literal percent sign has to be written
// %% the way fmt does. A lone % would be read as a verb and come out as
// "%!s(MISSING)" in the interface; this pins the rule for both languages.
func TestPercentInMessages(t *testing.T) {
	const literal = Key("test.percentLiteral")
	const withArg = Key("test.percentArg")
	register(EN, map[Key]string{
		literal: "Saving 100%% of the files",
		withArg: "%d%% of %s",
	})
	register(ZHCN, map[Key]string{
		literal: "已保存 100%% 的文件",
		withArg: "%d%%，共 %s",
	})
	if got := New(EN).T(literal); got != "Saving 100% of the files" {
		t.Fatalf("EN literal = %q", got)
	}
	if got := New(ZHCN).T(literal); got != "已保存 100% 的文件" {
		t.Fatalf("ZH literal = %q", got)
	}
	if got := New(EN).T(withArg, 42, "files"); got != "42% of files" {
		t.Fatalf("EN with arg = %q", got)
	}
	if got := New(ZHCN).T(withArg, 42, "3 个文件"); got != "42%，共 3 个文件" {
		t.Fatalf("ZH with arg = %q", got)
	}
}

// Every language must use the same placeholder sequence for a key. The callers
// pass one argument list, so a translation that reorders "%d" and "%s" would
// print "%!s(int=…)" instead of a number, as the discovery message once did.
func TestPlaceholderSequencesMatch(t *testing.T) {
	for key, text := range tables[EN] {
		other, ok := tables[ZHCN][key]
		if !ok {
			continue // TestTablesAreComplete reports the missing key
		}
		if a, b := placeholders(text), placeholders(other); strings.Join(a, ",") != strings.Join(b, ",") {
			t.Errorf("%s: EN %q uses %v, ZHCN %q uses %v", key, text, a, other, b)
		}
	}
}

// placeholders lists the verbs of a message in order. Doubled percent signs are
// literal text, and width/precision flags do not change the argument order.
func placeholders(s string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && strings.ContainsRune("+-# 0.", rune(s[j])) {
			j++
		}
		if j < len(s) {
			out = append(out, s[j:j+1])
			i = j
		}
	}
	return out
}
