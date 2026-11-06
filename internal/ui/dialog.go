package viffui

import (
	"strings"
	"unicode/utf8"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// dialog is a modal IntelliJ dialog: a title, a form body and a button row.
type dialog struct {
	// textarea, when set, is a multi-line field where Enter types a newline.
	textarea ui.ID
	// shortcut, when set, gets Cmd/Ctrl chords while the dialog is open.
	shortcut func(e ui.KeyEvent) bool
	title    string
	width    float32
	contentH float32
	body     func(f *form)
	ok       string
	// okLabel, when set, replaces ok at paint time. Dialogs whose action
	// depends on live state (the update dialog) change the label without
	// being reopened. The value must never be empty; openDialog falls back to
	// ok when it is.
	okLabel    func() string
	okDisabled func() bool
	onOK       func() bool // false keeps the dialog open
	cancel     string
	extra      string
	onExtra    func()
	onCancel   func()
	err        string
	focus      ui.ID
	focused    bool
	noCancel   bool
	// collapsed holds the folded state of a dialog's collapsible groups by
	// key. The form is rebuilt on every paint, so the state lives here.
	collapsed map[string]bool
	// scroll is the body's vertical offset. The window caps a dialog's height,
	// so a form taller than the screen scrolls instead of pushing the button
	// row out of the window. A dialog whose body fits keeps it at zero and
	// scrollable false.
	scroll     scroller
	scrollable bool
	// header paints above the scrolling body and never scrolls with it: the
	// settings page tabs stay put while the form under them moves, so the reader
	// can switch pages at any offset. headerH is its measured height.
	header  func(f *form)
	headerH float32
	// bodyH is the visible height of the scrolling body, recorded by the paint
	// so key handling can page it without measuring again.
	bodyH float32
	// noClose hides the X in the title row. A dialog that is edited rather than
	// confirmed still keeps Cancel, so the window has one obvious way out
	// instead of two that do slightly different things.
	noClose bool
}

// form lays out dialog content top to bottom.
type form struct {
	w      *Window
	x, y   float32
	width  float32
	labelW float32
	fields int
	checks int
	combos int
	radios int
	links  int
	// collapsed and foldCount belong to collapsibleGroup. The map comes from
	// the dialog so a fold survives the form being rebuilt on every paint.
	collapsed map[string]bool
	// indent shifts the rows that follow. A collapsible group sets it for its
	// own contents so they read as a level under the group title; the next
	// group header clears it again.
	indent    float32
	foldCount int
}

const (
	dlgPad = 20
	// dlgEdge keeps a capped dialog off the window's top and bottom edges, so
	// it still reads as a floating panel rather than a full-screen sheet.
	dlgEdge = 80
	// groupIndent is how far a collapsible group's rows sit in from the form's
	// left edge. It lines them up under the group title's text rather than
	// under its chevron, so the rows read as one level below the header.
	groupIndent = 22
)

func (w *Window) openDialog(d *dialog) {
	if d.width == 0 {
		d.width = 480
	}
	if d.ok == "" {
		d.ok = w.t(i18n.CommonOK)
	}
	if d.cancel == "" {
		d.cancel = w.t(i18n.CommonCancel)
	}
	w.closePopups()
	w.dialog = d
}

func (w *Window) closeDialog() {
	if d := w.dialog; d != nil && d.onCancel != nil {
		d.onCancel()
	}
	w.dialog = nil
}

func (w *Window) submitDialog() {
	d := w.dialog
	if d == nil || (d.okDisabled != nil && d.okDisabled()) {
		return
	}
	if d.onOK == nil || d.onOK() {
		if w.dialog == d {
			w.dialog = nil
		}
	}
}

func (w *Window) dialogKey(e ui.KeyEvent) bool {
	switch e.Key {
	case ui.KeyEscape:
		w.closeDialog()
		return true
	case ui.KeyEnter:
		if e.Modifiers&(ui.ModifierShift|ui.ModifierAlt) != 0 {
			return false
		}
		if d := w.dialog; d != nil && d.textarea != (ui.ID{}) && e.Target == d.textarea {
			return false // Enter starts a new line in the dialog's text area
		}
		w.submitDialog()
		return true
	case ui.KeyPageUp:
		// Home and End are left to the text fields: a field consumes them for
		// its caret before the dialog sees the key, and taking them here would
		// make the caret jump around instead of the body.
		return w.scrollDialogBy(-pageScroll(w.dialog), w.dialogBodyH())
	case ui.KeyPageDown:
		return w.scrollDialogBy(pageScroll(w.dialog), w.dialogBodyH())
	}
	return false
}

// pageScroll is a page's worth of rows: one screen of body minus one row of
// overlap, so the reader keeps a line of context.
func pageScroll(d *dialog) float32 {
	if d == nil {
		return 0
	}
	return -max(d.bodyH-rowH, rowH)
}

// dialogBodyH is the visible body height the last paint recorded.
func (w *Window) dialogBodyH() float32 {
	if w.dialog == nil {
		return 0
	}
	return w.dialog.bodyH
}

func (w *Window) paintDialog(full ui.Rect) {
	d := w.dialog
	w.c.PushLayer(10)
	defer w.c.PopLayer()
	w.c.HitWithOptions(idDialogScrim, full, "", ui.HitOptions{Backdrop: true, SkipFocus: true})

	// A dialog's height follows its content, but never past the window: the
	// button row must stay reachable however long the form grows. When the
	// content does not fit, the body scrolls and the buttons stay put.
	natural := 44 + d.headerH + d.contentH + 24 + buttonH + dlgPad
	height := min(natural, max(200, full.Height-dlgEdge))
	width := min(d.width, full.Width-40)
	r := rect((full.Width-width)/2, max(40, (full.Height-height)*0.38), width, height)
	// The header (the settings page tabs) is pinned under the title and stays
	// out of the scrolling body, so it never moves and its buttons are always
	// reachable. The body is the room below it, down to the button row.
	header := rect(r.X+dlgPad, r.Y+44, r.Width-2*dlgPad, d.headerH)
	body := rect(r.X+dlgPad, bottom(header), r.Width-2*dlgPad, 0)
	by := bottom(r) - dlgPad - buttonH
	body.Height = max(by-12-body.Y, 0)
	d.bodyH = body.Height
	d.scrollable = natural > height
	w.c.Shadow(r, islandRadius, ui.ShadowStyle{OffsetY: 8, Blur: 32}, ui.Color{Alpha: 0.45})
	w.p.outline(r, islandRadius, 1, w.pal.PopupBorder, w.pal.Dialog)
	w.c.HitWithOptions(idDialogPanel, r, d.title, ui.HitOptions{Role: ui.RoleGroup, SkipFocus: true})
	w.c.PushFocusScope(idDialogPanel)
	w.c.TrapFocus(idDialogPanel)
	w.c.SetEscapeTarget(idDialogCancel)

	title := rect(r.X+dlgPad, r.Y, r.Width-2*dlgPad-30, 44)
	w.p.text(d.title, title, fontUI, semibold, w.pal.Text)
	if !d.noClose {
		if w.action(idDialogClose, right(r)-38, r.Y+9, icons.Close, "Close", actOpts{}) {
			w.after(w.closeDialog)
		}
	}

	// The header is measured and painted first: its height sets where the
	// scrolling body starts, so it has to settle before the body is laid out.
	// It is drawn outside any clip, which is what keeps the page tabs still
	// while the form under them scrolls.
	if d.header != nil {
		hf := &form{w: w, x: header.X, y: header.Y, width: header.Width, labelW: 0}
		hstart := hf.y
		d.header(hf)
		if h := hf.y - hstart; h != d.headerH {
			d.headerH = h
			w.c.RequestFrame()
		}
	}

	// The form lays its rows out in absolute coordinates, so the offset is
	// applied once here and every row method stays unaware of scrolling. The
	// measured height is a difference of two offsets, which the shift cancels:
	// contentH still reports the whole form, not the visible part.
	f := &form{w: w, x: r.X + dlgPad, y: body.Y - d.scroll.Y, width: r.Width - 2*dlgPad, labelW: 0, collapsed: d.collapsed}
	start := f.y
	// The body's scroll region is declared before the form registers its own,
	// because onScroll tries the most recently added region first: a list or
	// menu inside the dialog keeps the wheel, and the body takes what falls
	// outside it.
	if d.scrollable {
		w.onScrollIn(body, func(dx, dy float32) {
			d.scroll.Y += dy
			d.scroll.clamp(d.contentH, body.Height)
		})
		w.c.Clip(body)
	}
	if d.body != nil {
		d.body(f)
	}
	if d.err != "" {
		f.gap(4)
		f.message(d.err, w.pal.Error)
	}
	if h := f.y - start; h != d.contentH {
		d.contentH = h
		// The first paint measures the form, so a dialog that grew past the
		// window only becomes scrollable on the next frame.
		w.c.RequestFrame()
	}
	if d.scrollable {
		w.c.Unclip()
		d.scroll.clamp(d.contentH, body.Height)
		// vbar draws and handles its own drag, but it records no area, so the
		// body rectangle is published here for hit testing and for callers that
		// need the visible part of the form.
		w.areas[idDialogBar] = body
		w.vbar(idDialogBar, rect(body.X, body.Y, r.Width-dlgPad, body.Height), &d.scroll, d.contentH)
		w.revealDialogFocus(d, body)
	}

	bx := right(r) - dlgPad
	okLabel := d.ok
	if d.okLabel != nil {
		if label := d.okLabel(); label != "" {
			okLabel = label
		}
	}
	okW := w.buttonWidth(okLabel)
	okDisabled := d.okDisabled != nil && d.okDisabled()
	if w.button(idDialogOK, rect(bx-okW, by, okW, buttonH), okLabel, btnDefault, okDisabled) {
		w.after(w.submitDialog)
	}
	bx -= okW + 12
	if !d.noCancel {
		cw := w.buttonWidth(d.cancel)
		if w.button(idDialogCancel, rect(bx-cw, by, cw, buttonH), d.cancel, btnNormal, false) {
			w.after(w.closeDialog)
		}
		bx -= cw + 12
	}
	if d.extra != "" {
		ew := w.buttonWidth(d.extra)
		if w.button(idDialogExtra, rect(r.X+dlgPad, by, ew, buttonH), d.extra, btnNormal, false) {
			fn := d.onExtra
			w.after(func() {
				if fn != nil {
					fn()
				}
			})
		}
	}
	if !d.focused {
		target := d.focus
		if target == (ui.ID{}) {
			target = idDialogOK
		}
		w.c.SetInitialFocus(target)
		w.c.RequestFocus(target)
		w.after(func() { d.focused = true })
	}
	w.c.PopFocusScope()
}

// revealDialogFocus keeps keyboard navigation usable in a scrolling body: the
// focus ring moves through fields that may sit outside the visible part, so the
// body scrolls to whatever the keyboard just focused. It runs after the body is
// painted, once the hits of this frame exist.
func (w *Window) revealDialogFocus(d *dialog, body ui.Rect) {
	if !d.scrollable {
		return
	}
	target := w.c.Focused()
	if target == (ui.ID{}) {
		return
	}
	for _, hit := range w.c.Hits() {
		if hit.ID != target {
			continue
		}
		// Only a control inside the scrolling body can pull it around. The
		// pinned header and the button row sit outside it, so they must not:
		// checking the live rectangle first is what keeps the page tabs from
		// scrolling the form just because one of them took the focus.
		if hit.Bounds.Y+hit.Bounds.Height <= body.Y || hit.Bounds.Y >= bottom(body) {
			return
		}
		// Body coordinates: undo the scroll, then stay inside the content. A hit
		// that is not part of the form (a header button after scrolling) must
		// never move the body.
		top := hit.Bounds.Y + d.scroll.Y - body.Y
		if top < 0 || top >= d.contentH {
			return
		}
		d.scroll.reveal(top, hit.Bounds.Height, body.Height)
		d.scroll.clamp(d.contentH, body.Height)
		return
	}
}

// scrollDialogBy moves the body by dy pixels and reports whether it consumed
// the key, so the dialog key handler can fall through when nothing scrolls.
func (w *Window) scrollDialogBy(dy float32, view float32) bool {
	d := w.dialog
	if d == nil || !d.scrollable {
		return false
	}
	before := d.scroll.Y
	d.scroll.Y += dy
	d.scroll.clamp(d.contentH, view)
	if d.scroll.Y == before {
		return false
	}
	w.c.RequestFrame()
	return true
}

// ---- form rows --------------------------------------------------------------

func (f *form) gap(h float32) { f.y += h }

// rowX and rowW are the left edge and usable width of a row at the current
// indent. Group headers reset the indent, their rows set it, so everything
// under a header reads as one level below it.
func (f *form) rowX() float32 { return f.x + f.indent }

func (f *form) rowW() float32 { return f.width - f.indent }

// message paints wrapped text.
func (f *form) message(s string, c ui.Color) {
	lines := f.w.c.WrapLines(nil, s, fontUI, regular, f.rowW(), 12)
	for _, l := range lines {
		f.w.p.text(l, rect(f.rowX(), f.y, f.rowW(), 20), fontUI, regular, c)
		f.y += 20
	}
	f.y += 4
}

func (f *form) text(s string)  { f.message(s, f.w.pal.Text) }
func (f *form) muted(s string) { f.message(s, f.w.pal.TextSecond) }

// bold paints a one-line emphasized row.
func (f *form) bold(s string) {
	f.w.p.text(s, rect(f.rowX(), f.y, f.rowW(), 22), fontUI, semibold, f.w.pal.Text)
	f.y += 26
}

// collapsibleGroup paints a settings group header with a disclosure chevron and
// reports whether the group is expanded, so the caller can skip its rows. The
// state lives in the dialog: the form is rebuilt on every paint.
//
// The chevron is the same icon pair the tree rows and the commit message's
// disclosure use, rather than a text arrow: a glyph inherits the font's
// advances and weight, which never lines up with the 16px icons around it.
func (f *form) collapsibleGroup(key, title string) bool {
	open := !f.collapsed[key]
	id := scDialogGroups.At(f.foldCount)
	f.foldCount++
	// A header always sits at the form's own left edge; the indent belongs to
	// the rows below it. Clearing it here also ends the previous group.
	f.indent = 0
	f.custom(26, func(r ui.Rect) {
		if f.w.hovered(id) {
			f.w.p.round(r, 4, f.w.pal.Hover)
		}
		chev := icons.ChevronRight
		if open {
			chev = icons.ChevronDown
		}
		f.w.p.iconIn(chev, rect(r.X+2, r.Y+3, iconSize, iconSize))
		f.w.p.text(title, rect(r.X+22, r.Y, r.Width-22, 22), fontUI, semibold, f.w.pal.Text)
		f.w.c.HitWithOptions(id, r, title, ui.HitOptions{
			Role: ui.RoleButton, Cursor: ui.PointerCursorDefault,
			Expandable: true, Expanded: open,
		})
		if f.w.c.Clicked(id) {
			f.w.after(func() { f.collapsed[key] = !f.collapsed[key] })
		}
	})
	if open {
		f.indent = groupIndent
	}
	return open
}

// field is a labelled text field. With labelW 0 the label sits above.
func (f *form) field(label string, ed *text.Editor, placeholder string, invalid bool) ui.ID {
	id := scDialogFields.At(f.fields)
	f.fields++
	x, width := f.rowX(), f.rowW()
	if label != "" {
		if f.labelW > 0 {
			f.w.p.text(label, rect(f.rowX(), f.y, f.labelW, fieldH), fontUI, regular, f.w.pal.Text)
			x += f.labelW
			width -= f.labelW
		} else {
			f.w.p.text(label, rect(f.rowX(), f.y, f.rowW(), 20), fontUI, regular, f.w.pal.Text)
			f.y += 24
		}
	}
	if f.w.field(id, ed, rect(x, f.y, width, fieldH), fieldOpts{placeholder: placeholder, invalid: invalid, label: label}) {
		f.w.after(f.w.submitDialog)
	}
	f.y += fieldH + 12
	return id
}

// secretField is a masked input with an eye button that reveals the text.
func (f *form) secretField(label string, ed *text.Editor, shown *bool, placeholder string) ui.ID {
	id := scDialogFields.At(f.fields)
	f.fields++
	x, width := f.rowX(), f.rowW()
	if label != "" && f.labelW > 0 {
		f.w.p.text(label, rect(f.rowX(), f.y, f.labelW, fieldH), fontUI, regular, f.w.pal.Text)
		x += f.labelW
		width -= f.labelW
	}
	const eyeW = float32(28)
	display := ""
	if !*shown {
		display = strings.Repeat("•", utf8.RuneCountInString(ed.String()))
	}
	if f.w.field(id, ed, rect(x, f.y, width-eyeW-4, fieldH), fieldOpts{placeholder: placeholder, label: label, display: display}) {
		f.w.after(f.w.submitDialog)
	}
	eye := icons.EyeClosed
	if *shown {
		eye = icons.Eye
	}
	eyeX := right(rect(x, f.y, width, fieldH)) - eyeW + (eyeW-actionSize)/2
	if f.w.action(idAIEye, eyeX, f.y+(fieldH-actionSize)/2, eye, f.w.t(i18n.GitAIShowKey), actOpts{}) {
		f.w.after(func() { *shown = !*shown })
	}
	f.y += fieldH + 12
	return id
}

func (f *form) check(label string, v *bool) {
	id := scDialogChecks.At(f.checks)
	f.checks++
	if f.w.checkbox(id, f.rowX()+f.labelW, f.y, 24, label, *v, false) {
		f.w.after(func() { *v = !*v })
	}
	f.y += 28
}

func (f *form) checkDisabled(label string, v bool) {
	id := scDialogChecks.At(f.checks)
	f.checks++
	f.w.checkbox(id, f.rowX()+f.labelW, f.y, 24, label, v, true)
	f.y += 28
}

func (f *form) radio(options []string, v *int) {
	for i, o := range options {
		id := scDialogRadios.At(f.radios)
		f.radios++
		if f.w.radio(id, f.rowX()+f.labelW, f.y, 24, o, *v == i) {
			i := i
			f.w.after(func() { *v = i })
		}
		f.y += 28
	}
}

// combo is a labelled dropdown whose choices are built on click.
func (f *form) combo(label, value string, choices func() []item) {
	id := scDialogCombos.At(f.combos)
	f.combos++
	x, width := f.rowX(), f.rowW()
	if label != "" {
		if f.labelW > 0 {
			f.w.p.text(label, rect(f.rowX(), f.y, f.labelW, fieldH), fontUI, regular, f.w.pal.Text)
			x += f.labelW
			width -= f.labelW
		} else {
			f.w.p.text(label, rect(f.rowX(), f.y, f.rowW(), 20), fontUI, regular, f.w.pal.Text)
			f.y += 24
		}
	}
	r := rect(x, f.y, width, fieldH)
	if f.w.combo(id, r, value, false) {
		f.w.after(func() { f.w.openMenuBelow(r, choices(), r.Width) })
	}
	f.y += fieldH + 12
}

// custom reserves h pixels for caller drawing, at the current indent.
func (f *form) custom(h float32, paint func(r ui.Rect)) {
	paint(rect(f.rowX(), f.y, f.rowW(), h))
	f.y += h
}

// confirm opens a yes/no dialog.
func (w *Window) confirm(title, message, ok string, run func()) {
	w.openDialog(&dialog{
		title: title, ok: ok, width: 440,
		body: func(f *form) { f.text(message) },
		onOK: func() bool { run(); return true },
	})
}
