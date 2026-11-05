package viffui

import (
	"runtime"
	"strings"
	"testing"
)

func TestKbdHintByPlatform(t *testing.T) {
	cases := []struct{ in, mac, other string }{
		{"", "", ""},
		{"⌘K", "⌘K", "Ctrl+K"},
		{"⇧⌘K", "⇧⌘K", "Ctrl+Shift+K"},
		{"⌥⌘N", "⌥⌘N", "Ctrl+Alt+N"},
		{"⌥⇧`", "⌥⇧`", "Alt+Shift+`"},
		{"⇧⎋", "⇧⎋", "Shift+Esc"},
		{"⌘⏎", "⌘⏎", "Ctrl+Enter"},
		{"⇧F7", "⇧F7", "Shift+F7"},
		{"⌘,", "⌘,", "Ctrl+,"},
		{"⌃⌘F", "⌃⌘F", "Ctrl+F"},
	}
	for _, c := range cases {
		want := c.other
		if runtime.GOOS == "darwin" {
			want = c.mac
		}
		if got := kbdHint(c.in); got != want {
			t.Errorf("kbdHint(%q) = %q, want %q", c.in, got, want)
		}
	}
}

// A platform whose UI font has no ⌘ ⇧ ⌥ glyph must not be handed one: that is
// what left a menu reading "PushK" instead of "Ctrl+Shift+K".
func TestKbdHintDropsModifierGlyphsElsewhere(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS draws the glyphs")
	}
	for _, in := range []string{"⌘K", "⇧⌘K", "⌥⌘N", "⇧⎋", "⌘⏎", "⌃⌘F"} {
		if got := kbdHint(in); strings.ContainsAny(got, "⌘⇧⌥⌃") {
			t.Errorf("kbdHint(%q) = %q keeps a modifier glyph", in, got)
		}
	}
}
