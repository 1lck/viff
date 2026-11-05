package viffui

import (
	"runtime"
	"strings"
)

// The interface spells shortcuts the way IntelliJ does on macOS. Windows and
// Linux UI fonts have no glyph for ⌘ ⇧ ⌥ ⌃, so a chord like "⇧⌘K" would draw
// with the symbols missing - the user sees "PushK" - which is why kbdHint
// spells the modifiers out there.
//
// ⌘ becomes Ctrl rather than the Super key because shortcut() accepts
// ModifierMeta or ModifierControl on every platform, so Ctrl is what actually
// triggers the command.
//
// kbdKey is a glyph that names a key rather than a character.
func kbdKey(r rune) (string, bool) {
	switch r {
	case '⎋':
		return "Esc", true
	case '⏎', '⌤':
		return "Enter", true
	case '⌫':
		return "Backspace", true
	case '⌦':
		return "Delete", true
	case '⇥':
		return "Tab", true
	case '␣':
		return "Space", true
	}
	return "", false
}

// kbdHint rewrites one shortcut for the platform: macOS keeps the glyphs and
// everything else writes Ctrl, Alt and Shift, in the order those systems use.
func kbdHint(s string) string {
	if runtime.GOOS == "darwin" || s == "" {
		return s
	}
	var ctrl, alt, shift bool
	var key strings.Builder
	for _, r := range s {
		switch r {
		case '⌃', '⌘':
			ctrl = true
			continue
		case '⌥':
			alt = true
			continue
		case '⇧':
			shift = true
			continue
		}
		if name, ok := kbdKey(r); ok {
			key.WriteString(name)
			continue
		}
		key.WriteRune(r)
	}
	parts := make([]string, 0, 4)
	if ctrl {
		parts = append(parts, "Ctrl")
	}
	if alt {
		parts = append(parts, "Alt")
	}
	if shift {
		parts = append(parts, "Shift")
	}
	if len(parts) == 0 {
		return key.String()
	}
	if k := key.String(); k != "" {
		parts = append(parts, k)
	}
	return strings.Join(parts, "+")
}
