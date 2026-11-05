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
