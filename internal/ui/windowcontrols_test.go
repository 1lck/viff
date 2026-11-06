package viffui

import (
	"testing"

	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
)

// The titlebar buttons take their ink from the palette on every paint, so a
// theme switch mid-session repaints them in the new theme instead of keeping
// the color derived when the window opened. Viva calls the style per frame,
// which is what makes reading the palette here the fix.
func TestWindowControlsFollowThePalette(t *testing.T) {
	for _, tc := range []struct {
		name string
		pal  *Palette
	}{
		{"dark", DarkPalette()},
		{"light", LightPalette()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &Window{pal: tc.pal}
			style := w.windowControlsStyle()
			if style.Foreground != tc.pal.Text {
				t.Fatalf("Foreground = %v, want the palette text %v", style.Foreground, tc.pal.Text)
			}
			if style.Hover != tc.pal.ToolHover {
				t.Fatalf("Hover = %v, want the toolbar hover %v", style.Hover, tc.pal.ToolHover)
			}
			if style.Pressed != tc.pal.ToolPressed {
				t.Fatalf("Pressed = %v, want the toolbar press %v", style.Pressed, tc.pal.ToolPressed)
			}
			// The close button keeps Viva's constant red and white glyph: the
			// palette's Error is the pale text red, and white on it reads at
			// 2.6:1 in the dark theme.
			if style.Close != (ui.Color{}) || style.CloseForeground != (ui.Color{}) {
				t.Fatalf("close colors = %v / %v, want the platform defaults", style.Close, style.CloseForeground)
			}
			if style.Minimize != "Minimize" || style.Maximize != "Maximize" ||
				style.Restore != "Restore" || style.CloseName != "Close" {
				t.Fatalf("names = %q/%q/%q/%q, want the English defaults",
					style.Minimize, style.Maximize, style.Restore, style.CloseName)
			}
		})
	}
}

// The regression this guards: the ink must follow a theme switch made after the
// window exists, not the clear color captured when it opened.
func TestWindowControlsFollowAThemeSwitch(t *testing.T) {
	w := &Window{pal: DarkPalette()}
	dark := w.windowControlsStyle().Foreground
	w.pal = LightPalette()
	light := w.windowControlsStyle().Foreground
	if dark == light {
		t.Fatalf("ink stayed %v after switching to the light theme", dark)
	}
	if light != LightPalette().Text {
		t.Fatalf("ink = %v, want the light palette text %v", light, LightPalette().Text)
	}
}

// The interface language names the buttons for screen readers; Viva's own names
// are English whatever the settings say.
func TestWindowControlsSpeakTheChosenLanguage(t *testing.T) {
	w := &Window{pal: DarkPalette(), tr: i18n.New(i18n.ZHCN)}
	style := w.windowControlsStyle()
	if style.Minimize != "最小化" || style.Maximize != "最大化" ||
		style.Restore != "还原" || style.CloseName != "关闭" {
		t.Fatalf("names = %q/%q/%q/%q, want the Chinese names",
			style.Minimize, style.Maximize, style.Restore, style.CloseName)
	}
}
