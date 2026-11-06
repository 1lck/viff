package viffui

import (
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// item is one row of a popup menu.
type item struct {
	title    string
	icon     icons.Pair
	tint     ui.Color // optional icon tint (branch colours)
	shortcut string
	hint     string // muted text after the title (tracking branch, counts)
	disabled bool
	checked  bool
	sep      bool   // a separator line
	header   string // a group header row
	sub      []item
	subFn    func() []item // built lazily when the submenu opens
	run      func()
	star     *bool // favourite toggle (branch popup)
	// switchOn and switchOnSet turn a row into a real switch: the row draws a
	// track and a knob on the right, and clicking it flips the value without
	// closing the menu (a switch is a setting, not a command).
	switchOn    bool
	switchOnSet func(bool)
	// label draws a plain statement row: no icon, no hit target, no action. It
	// is how a confirmation asks its question inside a menu.
	label bool
	// buttons turns the row into a button bar, painted left to right in the
	// order given. Each entry needs only a title and a run.
	buttons []item
	filter  string // text matched by the speed search; "" = title
	bold    bool
}

func sep() item                { return item{sep: true} }
func header(title string) item { return item{header: title} }
func (it item) selectable() bool {
	if it.sep || it.header != "" || it.disabled || it.label {
		return false
	}
	if it.buttons != nil {
		// A button row is one keyboard stop: Enter runs its default button, so a
		// confirmation built from a menu stays usable without a pointer.
		return it.defaultButton() >= 0
	}
	return true
}
func (it item) hasSub() bool { return len(it.sub) > 0 || it.subFn != nil }

// defaultButton is the button Enter and Space activate: the right-most enabled
// one, which is the default button the row paints last.
func (it item) defaultButton() int {
	for n := len(it.buttons) - 1; n >= 0; n-- {
		if !it.buttons[n].disabled {
			return n
		}
	}
	return -1
}

// switchW and switchH are the track of the setting switches in a menu.
const (
	switchW = 32
	switchH = 18
)

// popup is an open menu.
type popup struct {
	x, y     float32 // requested top-left
	anchor   ui.Rect // for submenus: the parent row
	items    []item
	sel      int
	minWidth float32
	maxRows  int
	title    string
	search   *text.Editor // optional speed search field
	query    string
	shown    []int // indexes of items shown (after filtering)
	scroll   scroller
	r        ui.Rect // painted bounds
	byKey    bool
	// above opens the popup with its bottom edge just over the anchor, so a
	// short confirmation can sit above a button instead of covering it.
	above   bool
	onClose func()
}

const (
	popupRowH  = 28
	popupPadY  = 6
	popupPadX  = 6
	popupMaxH  = 560
	headerRowH = 26
)

// openMenu opens a context menu at a point.
func (w *Window) openMenu(x, y float32, items []item) {
	w.popups = []*popup{{x: x, y: y, items: items, sel: -1}}
	w.tipID = ui.ID{}
}

// openMenuBelow opens a menu under an anchor rectangle.
func (w *Window) openMenuBelow(anchor ui.Rect, items []item, minWidth float32) *popup {
	p := &popup{x: anchor.X, y: bottom(anchor) + 4, items: items, sel: -1, minWidth: minWidth}
	w.popups = []*popup{p}
	w.tipID = ui.ID{}
	return p
}

// openMenuAbove opens a menu with its bottom edge just over the anchor, so a
// short confirmation can sit above the button that raised it instead of
// covering it. The anchor is kept for the height calculation in paintPopup.
func (w *Window) openMenuAbove(anchor ui.Rect, items []item, minWidth float32) {
	p := &popup{
		x: anchor.X, anchor: anchor, above: true,
		items: items, sel: -1, minWidth: minWidth,
	}
	// A bubble above a button is a question with a default answer, so its row is
	// preselected: Enter answers the question instead of doing nothing until the
	// user walks into the row with the arrow keys.
	p.sel = firstSelectable(p, -1, 1)
	w.popups = []*popup{p}
	w.tipID = ui.ID{}
}

func (w *Window) closePopups() {
	for _, p := range w.popups {
		if p.onClose != nil {
			p.onClose()
		}
	}
	w.popups = nil
}

// visible returns the indexes of the items the search lets through. Headers
// and separators survive only when an item after them does.
func (p *popup) visible() []int {
	q := ""
	if p.search != nil {
		q = strings.ToLower(strings.TrimSpace(p.search.String()))
	}
	if q == p.query && p.shown != nil {
		return p.shown
	}
	if q != p.query {
		// Show the first matches even if the full list was scrolled down.
		p.scroll.Y, p.sel, p.byKey = 0, -1, true
	}
	p.query = q
	p.shown = p.shown[:0]
	if q == "" {
		for i := range p.items {
			p.shown = append(p.shown, i)
		}
		return p.shown
	}
	pendingHeader := -1
	for i, it := range p.items {
		switch {
		case it.header != "":
			pendingHeader = i
		case it.sep:
		case it.run == nil && !it.hasSub():
		default:
			f := it.filter
			if f == "" {
				f = it.title
			}
			if fuzzy(strings.ToLower(f), q) {
				if pendingHeader >= 0 {
					p.shown = append(p.shown, pendingHeader)
					pendingHeader = -1
				}
				p.shown = append(p.shown, i)
			}
		}
	}
	p.sel = -1
	for _, i := range p.shown {
		if p.items[i].selectable() {
			p.sel = i
			break
		}
	}
	return p.shown
}

// fuzzy matches the characters of q in order, as IntelliJ speed search does.
func fuzzy(s, q string) bool {
	if strings.Contains(s, q) {
		return true
	}
	i := 0
	for _, r := range s {
		if i < len(q) && r == rune(q[i]) {
			i++
		}
	}
	return i == len(q)
}

func (w *Window) popupSize(p *popup) (float32, float32) {
	width := max(p.minWidth, 180)
	h := float32(popupPadY * 2)
	if p.search != nil {
		h += 40
	}
	shown := p.visible()
	for _, i := range shown {
		it := p.items[i]
		switch {
		case it.sep:
			h += 9
		case it.header != "":
			h += headerRowH
		case it.buttons != nil:
			h += buttonH + 8
			tw := float32(2*popupPadX + 8)
			for n, b := range it.buttons {
				if n > 0 {
					tw += 10
				}
				tw += w.buttonWidth(b.title)
			}
			width = max(width, tw)
		case it.label:
			h += popupRowH
			// A statement has no icon column, but it is painted inside the row
			// padding with 8 points on each side, so the popup must reserve both
			// or the question is elided to "…with…" instead of being asked.
			width = max(width, float32(2*popupPadX)+w.p.width(it.title, fontUI, regular)+16)
		default:
			h += popupRowH
			tw := 36 + w.p.width(it.title, fontUI, regular) + 24
			if it.hint != "" {
				tw += w.p.width(it.hint, fontUI, regular) + 10
			}
			if it.shortcut != "" {
				tw += w.p.width(kbdHint(it.shortcut), fontUI, regular) + 24
			}
			if it.hasSub() {
				tw += 20
			}
			if it.star != nil {
				tw += 20
			}
			if it.switchOnSet != nil {
				tw += switchW + 28
			}
			width = max(width, tw)
		}
	}
	if len(shown) == 0 {
		h += popupRowH
	}
	return min(width, 520), h
}

func (w *Window) paintPopups(full ui.Rect) {
	// A press outside every popup closes the stack.
	w.c.HitWithOptions(idPopupBackdrop, full, w.t(i18n.ChromePopupCloseMenu), ui.HitOptions{Backdrop: true, SkipFocus: true, PassSecondary: false})
	w.clicked(idPopupBackdrop, w.closePopups)
	w.onContext(idPopupBackdrop, func(x, y float32) { w.closePopups() })
	for level, p := range w.popups {
		w.paintPopup(level, p, full)
	}
}

func (w *Window) paintPopup(level int, p *popup, full ui.Rect) {
	width, height := w.popupSize(p)
	height = min(height, min(popupMaxH, full.Height-16))
	x, y := p.x, p.y
	if p.above {
		// Sit the bottom edge just over the anchor with the right edges aligned,
		// so the bubble grows to the left instead of covering the commit box.
		// The height is only known here, which is why it is resolved at paint
		// time and not at open time.
		y = max(8, p.anchor.Y-height-6)
		x = right(p.anchor) - width
	}
	if level > 0 {
		x, y = right(p.anchor)+2, p.anchor.Y-popupPadY
		if x+width > full.Width-8 {
			x = p.anchor.X - width - 2
		}
	}
	if x+width > full.Width-8 {
		x = full.Width - 8 - width
	}
	if y+height > full.Height-8 {
		if level == 0 && p.y > full.Height/2 {
			y = max(8, p.y-height-8)
		} else {
			y = max(8, full.Height-8-height)
		}
	}
	x = max(8, x)
	r := rect(x, y, width, height)
	p.r = r

	layer := uint16(20 + level)
	w.c.PushLayer(layer)
	defer w.c.PopLayer()
	w.c.Shadow(r, radiusPopup, ui.ShadowStyle{OffsetY: 4, Blur: 16}, ui.Color{Alpha: 0.35})
	w.p.outline(r, radiusPopup, 1, w.pal.PopupBorder, w.pal.Popup)
	panel := idPopupPanel
	if level > 0 {
		panel = idSubPanel
	}
	w.c.HitWithOptions(panel, r, p.title, ui.HitOptions{Role: ui.RoleMenu, SkipFocus: true})

	cy := r.Y + popupPadY
	if p.search != nil {
		fr := rect(r.X+8, cy+2, r.Width-16, 30)
		if w.field(idPopupSearch, p.search, fr, fieldOpts{icon: icons.Search, placeholder: w.t(i18n.ChromePopupSearch), nav: true, borderless: false}) {
			if p.sel >= 0 {
				w.after(func() { w.activate(level, p.sel) })
			}
		}
		w.c.SetInitialFocus(idPopupSearch)
		if !w.focused(idPopupSearch) {
			w.c.RequestFocus(idPopupSearch)
		}
		cy += 40
		w.p.hline(r.X, cy-3, r.Width, w.pal.Separator)
	}
	body := rect(r.X, cy, r.Width, bottom(r)-popupPadY-cy)
	shown := p.visible()
	content := float32(0)
	for _, i := range shown {
		content += rowHeight(p.items[i])
	}
	p.scroll.clamp(content, body.Height)
	w.onScrollIn(r, func(dx, dy float32) { p.scroll.Y += dy; p.scroll.clamp(content, body.Height) })
	if p.byKey && p.sel >= 0 {
		top := float32(0)
		for _, i := range shown {
			if i == p.sel {
				p.scroll.reveal(top, popupRowH, body.Height)
				break
			}
			top += rowHeight(p.items[i])
		}
	}
	w.c.Clip(body)
	scope := scPopupItems
	if level > 0 {
		scope = scSubItems
	}
	ry := body.Y - p.scroll.Y
	if len(shown) == 0 {
		w.p.text(w.t(i18n.ChromePopupEmpty), rect(r.X+16, ry, r.Width-32, popupRowH), fontUI, regular, w.pal.TextSecond)
	}
	for _, i := range shown {
		it := p.items[i]
		h := rowHeight(it)
		row := rect(r.X+popupPadX, ry, r.Width-2*popupPadX, h)
		ry += h
		if bottom(row) < body.Y || row.Y > bottom(body) {
			continue
		}
		switch {
		case it.sep:
			w.p.hline(r.X+8, row.Y+4, r.Width-16, w.pal.Separator)
			continue
		case it.header != "":
			w.p.text(it.header, rect(row.X+8, row.Y+2, row.Width-16, h-2), fontSmall, semibold, w.pal.TextSecond)
			continue
		case it.label:
			// A statement, not a command: it has no hit target, no icon and no
			// hover, so it reads as the question the buttons below answer.
			w.p.text(it.title, rect(row.X+8, row.Y, row.Width-16, h), fontUI, regular, w.pal.Text)
			continue
		case it.switchOnSet != nil:
			// A switch is a setting, so the whole row is the control: the track
			// sits before the label and a click anywhere on the row flips it
			// without closing the menu. The row must NOT also be registered as a
			// menu item below — that hit is registered later and therefore covers
			// this one, which is why the click used to be swallowed and the menu
			// closed with nothing changed.
			sid := scPopupSwitch.At(i)
			track := rect(row.X+8, row.Y+(h-switchH)/2, switchW, switchH)
			tone := w.pal.ControlBorder
			if it.switchOn {
				tone = w.pal.Brand
			}
			if w.hovered(sid) {
				tone = w.pal.Focus
			}
			w.p.round(track, switchH/2, tone)
			knob := float32(switchH) - 4
			kx := track.X + 2
			if it.switchOn {
				kx = right(track) - knob - 2
			}
			w.p.round(rect(kx, track.Y+2, knob, knob), knob/2, w.pal.BrandText)
			tx := right(track) + 10
			w.p.text(it.title, rect(tx, row.Y, max(0, right(row)-tx-8), h), fontUI, regular, w.pal.Text)
			w.c.HitWithOptions(sid, row, it.title, ui.HitOptions{SkipFocus: true, Role: ui.RoleButton, Checked: it.switchOn})
			if w.c.Clicked(sid) {
				fn := it.switchOnSet
				on := !it.switchOn
				// The menu stays open, so paint from the new value immediately
				// instead of waiting for it to be reopened. Painting already
				// happened for this frame, so ask for the next one.
				p.items[i].switchOn = on
				w.c.RequestFrame()
				w.after(func() { fn(on) })
			}
			continue
		case it.buttons != nil:
			// Painted left to right in the order given, so the caller decides
			// which one sits on the right; no icon is drawn inside a button.
			bx := row.X + 8
			for n, b := range it.buttons {
				if n > 0 {
					bx += 10
				}
				bw := w.buttonWidth(b.title)
				bid := scPopupButtons.At(i*8 + n)
				kind := btnNormal
				if n == len(it.buttons)-1 {
					kind = btnDefault
				}
				if w.button(bid, rect(bx, row.Y+4, bw, buttonH), b.title, kind, b.disabled) {
					if fn := b.run; fn != nil {
						w.after(func() { w.closePopups(); fn() })
					} else {
						w.after(w.closePopups)
					}
				}
				bx += bw
			}
			continue
		}
		id := scope.At(i)
		if !it.disabled && w.hovered(id) && !p.byKey && p.sel != i {
			w.after(func() {
				p.sel = i
				w.openSubmenu(level, i, row)
			})
		}
		selected := p.sel == i && !it.disabled
		fg := w.pal.Text
		if it.disabled {
			fg = w.pal.TextDisable
		}
		if selected {
			w.p.round(row, 4, w.pal.Selection)
		}
		ix := row.X + 8
		switch {
		case it.checked:
			w.p.iconIn(icons.Checked, rect(ix, row.Y, 16, h))
		case len(it.icon.Dark) > 0:
			if it.tint.Alpha > 0 {
				w.p.iconTinted(it.icon, rect(ix, row.Y, 16, h), it.tint)
			} else if it.disabled {
				w.p.iconAlpha(it.icon, rect(ix, row.Y, 16, h), 0.4)
			} else {
				w.p.iconIn(it.icon, rect(ix, row.Y, 16, h))
			}
		}
		tx := row.X + 32
		weight := float32(regular)
		if it.bold {
			weight = semibold
		}
		rightEdge := right(row) - 8
		if it.hasSub() {
			w.p.iconIn(icons.ChevronRight, rect(rightEdge-16, row.Y, 16, h))
			rightEdge -= 22
		}
		if it.shortcut != "" {
			short := kbdHint(it.shortcut)
			sw := w.p.width(short, fontUI, regular)
			w.p.text(short, rect(rightEdge-sw, row.Y, sw, h), fontUI, regular, w.pal.TextSecond)
			rightEdge -= sw + 16
		}
		if it.star != nil {
			sid := scPopupStar.At(i)
			sr := rect(rightEdge-18, row.Y+(h-18)/2, 18, 18)
			on := *it.star
			if on || w.hovered(sid) || selected {
				pair := icons.StarEmpty
				if on {
					pair = icons.Star
				}
				w.p.iconIn(pair, sr)
			}
			w.c.HitWithOptions(sid, sr, w.t(i18n.ChromePopupFavorite), ui.HitOptions{SkipFocus: true, Role: ui.RoleButton})
			if w.c.Clicked(sid) {
				st := it.star
				w.after(func() { *st = !*st; p.shown = nil })
			}
			rightEdge -= 24
		}
		tw := w.p.text(it.title, rect(tx, row.Y, rightEdge-tx, h), fontUI, weight, fg)
		if it.hint != "" {
			hx := tx + tw + 8
			w.p.text(it.hint, rect(hx, row.Y, rightEdge-hx, h), fontUI, regular, w.pal.TextSecond)
		}
		w.c.HitWithOptions(id, row, it.title, ui.HitOptions{Role: ui.RoleMenuItem, SkipFocus: true, Disabled: it.disabled, Expandable: it.hasSub()})
		if w.c.Clicked(id) {
			w.after(func() { w.activate(level, i) })
		}
	}
	w.c.Unclip()
	w.vbar(scLogScrollbars.At(90+level), body, &p.scroll, content)
	if level == 0 && p.search == nil {
		w.c.RequestFocus(idPopupPanel)
	}
}

func rowHeight(it item) float32 {
	switch {
	case it.sep:
		return 9
	case it.header != "":
		return headerRowH
	case it.buttons != nil:
		return buttonH + 8
	}
	return popupRowH
}

// openSubmenu opens (or closes) the submenu of a hovered item.
func (w *Window) openSubmenu(level, i int, row ui.Rect) {
	if level >= len(w.popups) {
		return
	}
	p := w.popups[level]
	w.popups = w.popups[:level+1]
	it := p.items[i]
	if !it.hasSub() {
		return
	}
	sub := it.sub
	if it.subFn != nil {
		sub = it.subFn()
	}
	w.popups = append(w.popups, &popup{anchor: row, items: sub, sel: -1})
}

// activate runs an item or opens its submenu.
func (w *Window) activate(level, i int) {
	if level >= len(w.popups) {
		return
	}
	p := w.popups[level]
	if i < 0 || i >= len(p.items) {
		return
	}
	it := p.items[i]
	if !it.selectable() {
		return
	}
	if it.switchOnSet != nil {
		// Enter on a switch flips it and leaves the menu open, the same way a
		// click on the row does.
		fn := it.switchOnSet
		on := !it.switchOn
		p.items[i].switchOn = on
		w.c.RequestFrame()
		w.after(func() { fn(on) })
		return
	}
	if it.buttons != nil {
		// Enter runs the default button, the same one a click on the right-most
		// button runs, so a confirmation asks nothing a keyboard cannot answer.
		n := it.defaultButton()
		if n < 0 {
			return
		}
		btn := it.buttons[n]
		w.closePopups()
		if btn.run != nil {
			btn.run()
		}
		return
	}
	if it.hasSub() {
		w.openSubmenu(level, i, rowRectOf(p, i))
		if level+1 < len(w.popups) {
			sp := w.popups[level+1]
			sp.byKey = true
			sp.sel = firstSelectable(sp, 0, 1)
		}
		return
	}
	w.closePopups()
	if it.run != nil {
		it.run()
	}
}

// rowRectOf finds the painted row of item i (for keyboard-opened submenus).
func rowRectOf(p *popup, i int) ui.Rect {
	y := p.r.Y + popupPadY - p.scroll.Y
	if p.search != nil {
		y += 40
	}
	for _, j := range p.visible() {
		h := rowHeight(p.items[j])
		if j == i {
			return rect(p.r.X+popupPadX, y, p.r.Width-2*popupPadX, h)
		}
		y += h
	}
	return rect(p.r.X, p.r.Y, p.r.Width, popupRowH)
}

func firstSelectable(p *popup, from, dir int) int {
	shown := p.visible()
	if len(shown) == 0 {
		return -1
	}
	pos := 0
	for k, i := range shown {
		if i == from {
			pos = k
		}
	}
	if from < 0 {
		pos = -1
		if dir < 0 {
			pos = len(shown)
		}
	}
	for n := 0; n < len(shown); n++ {
		pos += dir
		if pos < 0 {
			pos = len(shown) - 1
		}
		if pos >= len(shown) {
			pos = 0
		}
		if p.items[shown[pos]].selectable() {
			return shown[pos]
		}
	}
	return -1
}

// popupKey handles keyboard navigation in the top-most popup.
func (w *Window) popupKey(e ui.KeyEvent) bool {
	level := len(w.popups) - 1
	p := w.popups[level]
	switch e.Key {
	case ui.KeyEscape:
		if level > 0 {
			w.popups = w.popups[:level]
		} else {
			w.closePopups()
		}
		return true
	case ui.KeyDown:
		p.byKey = true
		if p.sel < 0 {
			p.sel = firstSelectable(p, -1, 1)
		} else {
			p.sel = firstSelectable(p, p.sel, 1)
		}
		w.popups = w.popups[:level+1]
		return true
	case ui.KeyUp:
		p.byKey = true
		if p.sel < 0 {
			p.sel = firstSelectable(p, -1, -1)
		} else {
			p.sel = firstSelectable(p, p.sel, -1)
		}
		w.popups = w.popups[:level+1]
		return true
	case ui.KeyRight:
		if p.sel >= 0 && p.items[p.sel].hasSub() {
			w.activate(level, p.sel)
			return true
		}
	case ui.KeyLeft:
		if level > 0 {
			w.popups = w.popups[:level]
			return true
		}
	case ui.KeyEnter, ui.KeySpace:
		if e.Key == ui.KeySpace && p.search != nil {
			return false
		}
		if p.sel >= 0 {
			w.activate(level, p.sel)
		}
		return true
	}
	return false
}
