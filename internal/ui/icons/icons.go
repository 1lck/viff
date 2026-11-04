// Package icons holds the IntelliJ Platform icons compiled for Viva.
package icons

//go:generate go run ../../../tools/genicons ../../../assets/icons/intellij .

import "github.com/vivaui/viva/ui"

// Part is one single-paint layer of a glyph: an outline tinted with its own
// colour, or filled layers that carry their colours.
type Part struct {
	Icon ui.Icon
	Tint ui.Color
}

// Glyph is an icon drawn as ordered parts.
type Glyph []Part

// Pair is an icon drawn differently in the light and the dark theme.
type Pair struct{ Light, Dark Glyph }

// For returns the drawing for a theme.
func (p Pair) For(dark bool) Glyph {
	if dark {
		return p.Dark
	}
	return p.Light
}

// Draw paints the glyph in bounds. alpha scales every part (disabled icons).
func (g Glyph) Draw(c *ui.Canvas, bounds ui.Rect, alpha float32) {
	for _, p := range g {
		t := p.Tint
		t.Alpha *= alpha
		c.DrawIcon(p.Icon, bounds, t)
	}
}

// DrawTinted paints every part in one colour (an icon over an accent).
func (g Glyph) DrawTinted(c *ui.Canvas, bounds ui.Rect, tint ui.Color) {
	for _, p := range g {
		c.DrawIcon(p.Icon, bounds, tint)
	}
}
