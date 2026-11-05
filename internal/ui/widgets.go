package viffui

import (
	"time"
	"unicode/utf8"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// ---- icon actions ---------------------------------------------------------

type actOpts struct {
	disabled bool
	toggled  bool
	size     float32 // button box; 0 = actionSize
}

// action paints an IntelliJ toolbar action: a 16px icon in a rounded box
// that lights up on hover. It reports a click.
func (w *Window) action(id ui.ID, x, y float32, pair icons.Pair, tooltip string, o actOpts) bool {
	s := o.size
	if s == 0 {
		s = actionSize
	}
	r := rect(x, y, s, s)
	bg := ui.Color{}
	switch {
	case o.disabled:
	case w.pressed(id):
		bg = w.pal.ToolPressed
	case o.toggled:
		bg = w.pal.ToolPressed
	case w.hovered(id):
		bg = w.pal.ToolHover
	}
	w.p.round(r, 6, bg)
	if o.disabled {
		w.p.iconAlpha(pair, r, 0.35)
	} else {
		w.p.iconIn(pair, r)
	}
	w.c.HitWithOptions(id, r, tooltip, ui.HitOptions{Role: ui.RoleButton, SkipFocus: true, Disabled: o.disabled, Checked: o.toggled})
	w.tip(id, tooltip)
	return !o.disabled && w.c.Clicked(id)
}

// ---- buttons ----------------------------------------------------------------

type btnKind uint8

const (
	btnNormal btnKind = iota
	btnDefault
	btnBorderless
)

func (w *Window) buttonWidth(label string) float32 {
	return max(72, w.p.width(label, fontUI, regular)+28)
}

// button paints an IntelliJ push button. A default button is filled with the
// brand colour.
func (w *Window) button(id ui.ID, r ui.Rect, label string, kind btnKind, disabled bool) bool {
	hover, press := w.hovered(id), w.pressed(id)
	fg := w.pal.Text
	switch kind {
	case btnDefault:
		bg := w.pal.Brand
		if disabled {
			bg = w.pal.ControlRaised
			fg = w.pal.TextDisable
			w.p.outline(r, radiusCtl, 1, w.pal.ControlBorder, bg)
		} else {
			if hover {
				bg = lighten(bg, 0.08)
			}
			if press {
				bg = lighten(bg, -0.08)
			}
			w.p.round(r, radiusCtl, bg)
			fg = w.pal.BrandText
		}
	case btnBorderless:
		if !disabled && (hover || press) {
			w.p.round(r, radiusCtl, w.pal.ToolHover)
		}
		if disabled {
			fg = w.pal.TextDisable
		}
	default:
		bg := w.pal.ControlRaised
		if !disabled && press {
			bg = w.pal.ToolPressed
		} else if !disabled && hover {
			bg = blend(bg, w.pal.ToolHover)
		}
		w.p.outline(r, radiusCtl, 1, w.pal.ControlBorder, bg)
		if disabled {
			fg = w.pal.TextDisable
		}
	}
	if w.focused(id) && !disabled {
		w.focusRing(r, radiusCtl)
	}
	w.p.textCenter(label, r, fontUI, regular, fg)
	w.c.HitWithOptions(id, r, label, ui.HitOptions{Role: ui.RoleButton, Disabled: disabled, Cursor: ui.PointerCursorDefault})
	return !disabled && w.c.Clicked(id)
}

func (w *Window) focusRing(r ui.Rect, radius float32) {
	w.c.RoundedOutline(rect(r.X-2, r.Y-2, r.Width+4, r.Height+4), radius+2, 2, w.pal.FocusRing)
}

// link paints clickable link text and returns its width.
func (w *Window) link(id ui.ID, x, y, h float32, label string) (bool, float32) {
	tw := w.p.width(label, fontUI, regular)
	r := rect(x, y, tw, h)
	c := w.pal.Link
	w.p.text(label, r, fontUI, regular, c)
	if w.hovered(id) {
		w.p.hline(x, y+h/2+8, tw, c)
	}
	w.c.HitWithOptions(id, r, label, ui.HitOptions{Role: ui.RoleLink, Cursor: ui.PointerCursorHand, SkipFocus: true})
	return w.c.Clicked(id), tw
}

// ---- checkbox and radio -----------------------------------------------------

// checkbox paints a 16px IntelliJ checkbox with a label; the hit covers both.
func (w *Window) checkbox(id ui.ID, x, y, h float32, label string, checked, disabled bool) bool {
	box := rect(x, y+(h-16)/2, 16, 16)
	if checked && !disabled {
		w.p.round(box, 3, w.pal.Brand)
		w.drawCheck(box, w.pal.BrandText)
	} else {
		border := w.pal.ControlBorder
		if w.hovered(id) && !disabled {
			border = w.pal.TextSecond
		}
		w.p.outline(box, 3, 1, border, w.pal.ControlBg)
		if checked {
			w.drawCheck(box, w.pal.TextDisable)
		}
	}
	if w.focused(id) {
		w.focusRing(box, 3)
	}
	fg := w.pal.Text
	if disabled {
		fg = w.pal.TextDisable
	}
	tw := float32(0)
	if label != "" {
		tw = w.p.text(label, rect(x+22, y, 2000, h), fontUI, regular, fg) + 8
	}
	w.c.HitWithOptions(id, rect(x, y, 22+tw, h), label, ui.HitOptions{Role: ui.RoleCheckbox, Checked: checked, Disabled: disabled})
	return !disabled && w.c.Clicked(id)
}

func (w *Window) drawCheck(box ui.Rect, c ui.Color) {
	pb := w.c.PathBuilder()
	pb.MoveTo(4, 8.2)
	pb.LineTo(6.8, 11)
	pb.LineTo(12, 5.2)
	w.c.DrawPath(pb.Path(16, 16), box, ui.PathStyle{Stroke: c, StrokeWidth: 1.6, Cap: ui.CapRound, Join: ui.JoinRound})
}

func (w *Window) radio(id ui.ID, x, y, h float32, label string, on bool) bool {
	box := rect(x, y+(h-16)/2, 16, 16)
	if on {
		w.p.round(box, 8, w.pal.Brand)
		w.p.round(inset(box, 5, 5), 3, w.pal.BrandText)
	} else {
		w.p.outline(box, 8, 1, w.pal.ControlBorder, w.pal.ControlBg)
	}
	if w.focused(id) {
		w.focusRing(box, 8)
	}
	tw := w.p.text(label, rect(x+22, y, 2000, h), fontUI, regular, w.pal.Text) + 8
	w.c.HitWithOptions(id, rect(x, y, 22+tw, h), label, ui.HitOptions{Role: ui.RoleRadio, Checked: on})
	return w.c.Clicked(id)
}

// ---- text fields ------------------------------------------------------------

type fieldOpts struct {
	icon        icons.Pair
	invalid     bool
	disabled    bool
	mono        bool
	nav         bool // deliver Up/Down to OnKey
	borderless  bool
	label       string
	placeholder string
	rightPad    float32
	// display replaces what the field shows without changing the editor, which
	// is how a masked secret stays editable. An empty display shows the text.
	display string
}

// field paints an IntelliJ text field bound to an editor. It returns true
// when Enter is pressed in it.
func (w *Window) field(id ui.ID, ed *text.Editor, r ui.Rect, o fieldOpts) bool {
	focused := w.focused(id)
	if !o.borderless {
		border := w.pal.ControlBorder
		if o.invalid {
			border = w.pal.Error
		}
		if focused {
			border = w.pal.Focus
		}
		w.p.outline(r, radiusCtl, 1, border, w.pal.ControlBg)
		if focused {
			c := w.pal.FocusRing
			if o.invalid {
				c = w.pal.Error.WithAlpha(0.5)
			}
			w.c.RoundedOutline(rect(r.X-1, r.Y-1, r.Width+2, r.Height+2), radiusCtl+1, 2, c)
		}
	}
	tx := r.X + 8
	if len(o.icon.Dark) > 0 {
		w.p.iconIn(o.icon, rect(r.X+4, r.Y, 20, r.Height))
		tx = r.X + 28
	}
	content := rect(tx, r.Y, max(0, right(r)-tx-8-o.rightPad), r.Height)
	value := ed.String()
	masked := o.display != ""
	if masked {
		value = o.display
	}
	platform := focused && value != "" && !masked
	fg := w.pal.Text
	if o.disabled {
		fg = w.pal.TextDisable
	}
	if !platform {
		w.c.Clip(content)
		if value == "" {
			w.p.text(o.placeholder, content, fontUI, regular, w.pal.TextSecond)
		} else {
			w.p.text(value, content, fontUI, regular, fg)
		}
		if focused {
			w.p.fill(rect(content.X, r.Y+(r.Height-16)/2, 1, 16), w.pal.Text)
		}
		w.c.Unclip()
	}
	w.editors[id] = ed
	label := o.label
	if label == "" {
		label = o.placeholder
	}
	w.c.HitWithOptions(id, r, label, ui.HitOptions{
		Role: ui.RoleTextField, Cursor: ui.PointerCursorText, Value: value, Disabled: o.disabled,
		NavigationKeys: o.nav, TextBounds: content, TextSize: fontUI, TextColor: fg,
	})
	return w.c.ClickedByKey(id)
}

// setText replaces an editor's text and puts the caret at the end.
func setText(ed *text.Editor, s string) {
	ed.SetSelection(text.Range{Start: 0, End: ed.RuneCount()})
	ed.Insert(s)
}

// ---- combo ------------------------------------------------------------------

// combo paints a dropdown box showing value; a click is reported so the
// caller opens the choices.
func (w *Window) combo(id ui.ID, r ui.Rect, value string, disabled bool) bool {
	border := w.pal.ControlBorder
	if w.focused(id) {
		border = w.pal.Focus
	}
	bg := w.pal.ControlBg
	if w.hovered(id) && !disabled {
		bg = blend(bg, w.pal.Hover)
	}
	w.p.outline(r, radiusCtl, 1, border, bg)
	fg := w.pal.Text
	if disabled {
		fg = w.pal.TextDisable
	}
	w.p.text(value, rect(r.X+8, r.Y, r.Width-34, r.Height), fontUI, regular, fg)
	w.p.iconIn(icons.ChevronDown, rect(right(r)-26, r.Y, 22, r.Height))
	w.c.HitWithOptions(id, r, value, ui.HitOptions{Role: ui.RoleCombobox, Disabled: disabled})
	return !disabled && w.c.Clicked(id)
}

// filterChip paints a log filter like "Branch ▾": label in muted text, the
// value when set, and a chevron. It returns the click and the width used.
func (w *Window) filterChip(id ui.ID, x, y, h float32, label, value string) (bool, float32) {
	s := label
	if value != "" {
		s = label + ": " + value
	}
	tw := w.p.width(s, fontUI, regular)
	r := rect(x, y, tw+30, h)
	if w.pressed(id) {
		w.p.round(r, 6, w.pal.ToolPressed)
	} else if w.hovered(id) {
		w.p.round(r, 6, w.pal.ToolHover)
	}
	fg := w.pal.TextMuted
	if value != "" {
		fg = w.pal.Text
	}
	w.p.text(s, rect(x+8, y, tw, h), fontUI, regular, fg)
	w.p.iconIn(icons.ChevronDown, rect(x+tw+8, y, 18, h))
	w.c.HitWithOptions(id, r, s, ui.HitOptions{Role: ui.RoleCombobox, SkipFocus: true})
	return w.c.Clicked(id), r.Width
}

// ---- scrolling --------------------------------------------------------------

// scroller is a scroll offset clamped to its content.
type scroller struct {
	Y, X float32
}

func (s *scroller) clamp(content, view float32) {
	s.Y = clampF(s.Y, 0, max(0, content-view))
}

func (s *scroller) clampX(content, view float32) {
	s.X = clampF(s.X, 0, max(0, content-view))
}

// reveal scrolls so [top, top+h) is visible in a view of height view.
func (s *scroller) reveal(top, h, view float32) {
	if top < s.Y {
		s.Y = top
	} else if top+h > s.Y+view {
		s.Y = top + h - view
	}
}

func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// vbar paints a vertical overlay scrollbar for an area and lets its thumb be
// dragged.
func (w *Window) vbar(id ui.ID, area ui.Rect, s *scroller, content float32) {
	view := area.Height
	if content <= view+0.5 || view <= 0 {
		return
	}
	th := max(view*view/content, 24)
	ty := area.Y + (view-th)*(s.Y/(content-view))
	hot := w.hovered(id) || (w.drag != nil && w.drag.id == id)
	tw := float32(6)
	if hot {
		tw = 8
	}
	r := rect(right(area)-tw-2, ty, tw, th)
	c := w.pal.ScrollThumb
	if hot {
		c = w.pal.ScrollThumbHover
	}
	w.p.round(r, tw/2, c)
	w.c.HitWithOptions(id, rect(right(area)-12, area.Y, 12, area.Height), w.t(i18n.ChromeA11yScrollBar), ui.HitOptions{SkipFocus: true, SkipTab: true})
	start := s.Y
	w.onDrag(id, func(e dragEvent) {
		switch e.Phase {
		case dragBegin:
			if e.Y < ty || e.Y > ty+th {
				// A press in the track jumps there.
				s.Y = (e.Y - area.Y - th/2) / (view - th) * (content - view)
				s.clamp(content, view)
			}
			start = s.Y
		case dragMove:
			s.Y = start + (e.Y-e.StartY)/(view-th)*(content-view)
			s.clamp(content, view)
		}
	})
}

// hbar paints a horizontal overlay scrollbar.
func (w *Window) hbar(id ui.ID, area ui.Rect, s *scroller, content float32) {
	view := area.Width
	if content <= view+0.5 || view <= 0 {
		return
	}
	th := max(view*view/content, 24)
	tx := area.X + (view-th)*(s.X/(content-view))
	hot := w.hovered(id) || (w.drag != nil && w.drag.id == id)
	tw := float32(6)
	if hot {
		tw = 8
	}
	c := w.pal.ScrollThumb
	if hot {
		c = w.pal.ScrollThumbHover
	}
	w.p.round(rect(tx, bottom(area)-tw-2, th, tw), tw/2, c)
	w.c.HitWithOptions(id, rect(area.X, bottom(area)-12, area.Width, 12), w.t(i18n.ChromeA11yScrollBar), ui.HitOptions{SkipFocus: true, SkipTab: true})
	start := s.X
	w.onDrag(id, func(e dragEvent) {
		switch e.Phase {
		case dragBegin:
			start = s.X
		case dragMove:
			s.X = clampF(start+(e.X-e.StartX)/(view-th)*(content-view), 0, content-view)
		}
	})
}

// ---- splitters --------------------------------------------------------------

// splitter declares a resize handle; fn receives the pointer offset since
// the press (dx for vertical handles, dy for horizontal ones).
func (w *Window) splitter(id ui.ID, r ui.Rect, vertical bool, begin func(), fn func(delta float32), end func()) {
	cursor := ui.PointerCursorResizeVertical
	if vertical {
		cursor = ui.PointerCursorResizeHorizontal
	}
	w.c.HitWithOptions(id, r, w.t(i18n.ChromeA11yResize), ui.HitOptions{Cursor: cursor, SkipFocus: true, SkipTab: true})
	w.onDrag(id, func(e dragEvent) {
		switch e.Phase {
		case dragBegin:
			begin()
		case dragMove:
			if vertical {
				fn(e.X - e.StartX)
			} else {
				fn(e.Y - e.StartY)
			}
		case dragEnd:
			if end != nil {
				end()
			}
		}
	})
}

// ---- tooltip ----------------------------------------------------------------

const tipDelay = 700 * time.Millisecond

func (w *Window) paintTooltip(full ui.Rect) {
	hov := w.c.Hovered()
	text, ok := w.tips[hov]
	if !ok || w.drag != nil || len(w.popups) > 0 {
		w.tipID = ui.ID{}
		return
	}
	now := w.c.Now()
	if hov != w.tipID {
		w.tipID, w.tipSince, w.tipAt = hov, now, w.mouse
	}
	if wait := tipDelay - (now - w.tipSince); wait > 0 {
		w.c.RequestFrameIn(wait)
		return
	}
	main, short := splitShortcut(text)
	short = kbdHint(short)
	tw := w.p.width(main, fontUI, regular)
	sw := float32(0)
	if short != "" {
		sw = w.p.width(short, fontUI, regular) + 12
	}
	r := rect(w.tipAt.X+4, w.tipAt.Y+20, tw+sw+20, 30)
	if right(r) > full.Width-6 {
		r.X = full.Width - 6 - r.Width
	}
	if bottom(r) > full.Height-6 {
		r.Y = w.tipAt.Y - 8 - r.Height
	}
	w.c.PushLayer(40)
	w.c.Shadow(r, 6, ui.ShadowStyle{OffsetY: 2, Blur: 8}, ui.Color{Alpha: 0.25})
	w.p.outline(r, 6, 1, w.pal.PopupBorder, w.pal.Tooltip)
	w.p.text(main, rect(r.X+10, r.Y, tw, r.Height), fontUI, regular, w.pal.TooltipText)
	if short != "" {
		w.p.text(short, rect(r.X+10+tw+12, r.Y, sw, r.Height), fontUI, regular, w.pal.TextSecond)
	}
	w.c.PopLayer()
}

// splitShortcut splits "Commit\t⌘K" into its title and shortcut.
func splitShortcut(s string) (string, string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\t' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

// ---- small drawing helpers --------------------------------------------------

// counter paints a rounded count badge and returns its width.
func (w *Window) counter(x, y float32, n int, bg, fg ui.Color) float32 {
	s := itoa(n)
	tw := w.p.width(s, fontTiny, semibold)
	r := rect(x, y, max(16, tw+8), 16)
	w.p.round(r, 8, bg)
	w.p.textCenter(s, r, fontTiny, semibold, fg)
	return r.Width
}

func lighten(c ui.Color, amount float32) ui.Color {
	if amount >= 0 {
		return ui.Color{Red: c.Red + (1-c.Red)*amount, Green: c.Green + (1-c.Green)*amount, Blue: c.Blue + (1-c.Blue)*amount, Alpha: c.Alpha}
	}
	a := 1 + amount
	return ui.Color{Red: c.Red * a, Green: c.Green * a, Blue: c.Blue * a, Alpha: c.Alpha}
}

// blend composites over (with its alpha) onto base.
func blend(base, over ui.Color) ui.Color {
	a := over.Alpha
	return ui.Color{
		Red:   base.Red*(1-a) + over.Red*a,
		Green: base.Green*(1-a) + over.Green*a,
		Blue:  base.Blue*(1-a) + over.Blue*a,
		Alpha: max(base.Alpha, a),
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func plural(n int, one, many string) string {
	if n == 1 {
		return itoa(n) + " " + one
	}
	return itoa(n) + " " + many
}

// byteToRune converts a byte offset in s to a rune index.
func byteToRune(s string, b int) int {
	return utf8.RuneCountInString(s[:min(b, len(s))])
}
