package i18n

import "testing"

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
