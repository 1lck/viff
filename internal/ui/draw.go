package viffui

import (
	"strings"
	"unicode/utf8"

	"github.com/vivaui/viva/ui"

	"viff/internal/ui/icons"
)

// rect is a shorthand constructor.
func rect(x, y, w, h float32) ui.Rect { return ui.Rect{X: x, Y: y, Width: w, Height: h} }

func inset(r ui.Rect, dx, dy float32) ui.Rect {
	return ui.Rect{X: r.X + dx, Y: r.Y + dy, Width: max(r.Width-2*dx, 0), Height: max(r.Height-2*dy, 0)}
}

func contains(r ui.Rect, x, y float32) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.Width && y < r.Y+r.Height
}

func right(r ui.Rect) float32  { return r.X + r.Width }
func bottom(r ui.Rect) float32 { return r.Y + r.Height }

// splitLeft cuts w pixels off the left of r.
func splitLeft(r *ui.Rect, w float32) ui.Rect {
	w = min(w, r.Width)
	out := ui.Rect{X: r.X, Y: r.Y, Width: w, Height: r.Height}
	r.X += w
	r.Width -= w
	return out
}

func splitRight(r *ui.Rect, w float32) ui.Rect {
	w = min(w, r.Width)
	r.Width -= w
	return ui.Rect{X: r.X + r.Width, Y: r.Y, Width: w, Height: r.Height}
}

func splitTop(r *ui.Rect, h float32) ui.Rect {
	h = min(h, r.Height)
	out := ui.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: h}
	r.Y += h
	r.Height -= h
	return out
}

func splitBottom(r *ui.Rect, h float32) ui.Rect {
	h = min(h, r.Height)
	r.Height -= h
	return ui.Rect{X: r.X, Y: r.Y + r.Height, Width: r.Width, Height: h}
}

// pen draws with the window's palette.
type pen struct {
	c    *ui.Canvas
	pal  *Palette
	mono ui.MonoFace
}

func (p *pen) fill(r ui.Rect, c ui.Color) {
	if c.Alpha > 0 && r.Width > 0 && r.Height > 0 {
		p.c.Fill(r, c)
	}
}

func (p *pen) round(r ui.Rect, radius float32, c ui.Color) {
	if c.Alpha > 0 && r.Width > 0 && r.Height > 0 {
		p.c.Rounded(r, radius, c)
	}
}

// outline draws a rounded 1px border and fill.
func (p *pen) outline(r ui.Rect, radius, width float32, border, fill ui.Color) {
	if r.Width <= 0 || r.Height <= 0 {
		return
	}
	if fill.Alpha > 0 {
		p.c.Rounded(r, radius, fill)
	}
	if border.Alpha > 0 {
		p.c.RoundedOutline(r, radius, width, border)
	}
}

func (p *pen) hline(x, y, w float32, c ui.Color) { p.fill(rect(x, y, w, 1), c) }
func (p *pen) vline(x, y, h float32, c ui.Color) { p.fill(rect(x, y, 1, h), c) }

// style returns a text style of the UI font.
func style(size float32, weight float32, c ui.Color) ui.TextStyle {
	return ui.TextStyle{Size: size, Weight: weight, Color: c}
}

const (
	regular  = ui.WeightRegular
	semibold = ui.WeightSemibold
)

// text draws one line vertically centred in r, clipped with an ellipsis to
// r's width. It returns the width drawn.
func (p *pen) text(s string, r ui.Rect, size, weight float32, c ui.Color) float32 {
	if s == "" || r.Width <= 0 {
		return 0
	}
	w := p.c.TextWidthWeight(s, size, weight)
	if w > r.Width {
		s = p.c.FitLine(s, size, weight, r.Width)
		w = p.c.TextWidthWeight(s, size, weight)
	}
	p.c.Label(s, r, style(size, weight, c))
	return w
}

// textRight draws a line right-aligned in r.
func (p *pen) textRight(s string, r ui.Rect, size, weight float32, c ui.Color) {
	if s == "" {
		return
	}
	p.c.LabelAligned(s, r, style(size, weight, c), ui.AlignRight)
}

func (p *pen) textCenter(s string, r ui.Rect, size, weight float32, c ui.Color) {
	if s == "" {
		return
	}
	p.c.LabelCenter(s, r, style(size, weight, c))
}

func (p *pen) width(s string, size, weight float32) float32 {
	return p.c.TextWidthWeight(s, size, weight)
}

// icon draws an icon pair for the palette in a box of size px at (x, y).
func (p *pen) icon(pair icons.Pair, x, y, size float32) {
	pair.For(p.pal.Dark).Draw(p.c, rect(x, y, size, size), 1)
}

// iconAlpha draws an icon faded (disabled).
func (p *pen) iconAlpha(pair icons.Pair, r ui.Rect, alpha float32) {
	pair.For(p.pal.Dark).Draw(p.c, rect(r.X+(r.Width-iconSize)/2, r.Y+(r.Height-iconSize)/2, iconSize, iconSize), alpha)
}

// iconIn centres a 16px icon in r.
func (p *pen) iconIn(pair icons.Pair, r ui.Rect) {
	p.icon(pair, r.X+(r.Width-iconSize)/2, r.Y+(r.Height-iconSize)/2, iconSize)
}

// iconTinted draws a monochrome icon in one colour (over accent backgrounds).
func (p *pen) iconTinted(pair icons.Pair, r ui.Rect, c ui.Color) {
	pair.For(p.pal.Dark).DrawTinted(p.c, rect(r.X+(r.Width-iconSize)/2, r.Y+(r.Height-iconSize)/2, iconSize, iconSize), c)
}

// mono draws fixed-pitch text with coloured runs.
func (p *pen) monoText(s string, r ui.Rect, size float32, c ui.Color) {
	p.c.Label(s, r, ui.TextStyle{Size: size, Color: c, Mono: true, MonoFace: p.mono})
}

// truncateMiddle shortens a path in the middle so its file name stays.
func (p *pen) truncateMiddle(s string, size, weight, width float32) string {
	if p.width(s, size, weight) <= width {
		return s
	}
	runes := []rune(s)
	for keep := len(runes) - 1; keep > 4; keep-- {
		head := keep / 2
		tail := keep - head
		cand := string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
		if p.width(cand, size, weight) <= width {
			return cand
		}
	}
	return p.c.FitLine(s, size, weight, width)
}

// firstLine returns the first line of s.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// expandTabs replaces tabs with spaces for fixed-pitch drawing.
func expandTabs(s string, tab int) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := tab - col%tab
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col += cellWidthOf(r)
	}
	return b.String()
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// initials returns the two-letter project badge text.
func initials(name string) string {
	var out []rune
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == ' ' || r == '.' })
	for _, w := range words {
		r, _ := utf8.DecodeRuneInString(w)
		out = append(out, r)
		if len(out) == 2 {
			break
		}
	}
	if len(out) < 2 {
		rs := []rune(name)
		if len(rs) >= 2 {
			out = rs[:2]
		} else {
			out = rs
		}
	}
	return strings.ToUpper(string(out))
}
