package viffui

import (
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

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
	filter   string // text matched by the speed search; "" = title
	bold     bool
}

func sep() item                 { return item{sep: true} }
func header(title string) item  { return item{header: title} }
func (it item) selectable() bool { return !it.sep && it.header == "" && !it.disabled }
func (it item) hasSub() bool     { return len(it.sub) > 0 || it.subFn != nil }

// popup is an open menu.
type popup struct {
	x, y     float32 // requested top-left
	anchor   ui.Rect // for submenus: the parent row
	items    []item
	sel      int
	minWidth float32
	maxRows  int
	title    string
	search   *text.Editor // speed search field (branch popup)
	query    string
	shown    []int // indexes of items shown (after filtering)
	scroll   scroller
	r        ui.Rect // painted bounds
	byKey    bool
	onClose  func()
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
func (w *Window) openMenuBelow(anchor ui.Rect, items []item, minWidth float32) {
	w.popups = []*popup{{x: anchor.X, y: bottom(anchor) + 4, items: items, sel: -1, minWidth: minWidth}}
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
	for _, i := range p.visible() {
		it := p.items[i]
		switch {
		case it.sep:
			h += 9
		case it.header != "":
			h += headerRowH
		default:
			h += popupRowH
			tw := 36 + w.p.width(it.title, fontUI, regular) + 24
			if it.hint != "" {
				tw += w.p.width(it.hint, fontUI, regular) + 10
			}
			if it.shortcut != "" {
				tw += w.p.width(it.shortcut, fontUI, regular) + 24
			}
			if it.hasSub() {
				tw += 20
			}
			if it.star != nil {
				tw += 20
			}
			width = max(width, tw)
		}
	}
	if len(p.items) == 0 {
		h += popupRowH
	}
	return min(width, 520), h
}

func (w *Window) paintPopups(full ui.Rect) {
	// A press outside every popup closes the stack.
	w.c.HitWithOptions(idPopupBackdrop, full, "Close menu", ui.HitOptions{Backdrop: true, SkipFocus: true, PassSecondary: false})
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
		if w.field(idPopupSearch, p.search, fr, fieldOpts{icon: icons.Search, placeholder: "Search", nav: true, borderless: false}) {
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
		w.p.text("Nothing found", rect(r.X+16, ry, r.Width-32, popupRowH), fontUI, regular, w.pal.TextSecond)
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
			sw := w.p.width(it.shortcut, fontUI, regular)
			w.p.text(it.shortcut, rect(rightEdge-sw, row.Y, sw, h), fontUI, regular, w.pal.TextSecond)
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
			w.c.HitWithOptions(sid, sr, "Favorite", ui.HitOptions{SkipFocus: true, Role: ui.RoleButton})
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
