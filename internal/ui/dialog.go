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
	foldCount int
}

const dlgPad = 20

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
	}
	return false
}

func (w *Window) paintDialog(full ui.Rect) {
	d := w.dialog
	w.c.PushLayer(10)
	defer w.c.PopLayer()
	w.c.HitWithOptions(idDialogScrim, full, "", ui.HitOptions{Backdrop: true, SkipFocus: true})

	height := 44 + d.contentH + 24 + buttonH + dlgPad
	width := min(d.width, full.Width-40)
	r := rect((full.Width-width)/2, max(40, (full.Height-height)*0.38), width, height)
	w.c.Shadow(r, islandRadius, ui.ShadowStyle{OffsetY: 8, Blur: 32}, ui.Color{Alpha: 0.45})
	w.p.outline(r, islandRadius, 1, w.pal.PopupBorder, w.pal.Dialog)
	w.c.HitWithOptions(idDialogPanel, r, d.title, ui.HitOptions{Role: ui.RoleGroup, SkipFocus: true})
	w.c.PushFocusScope(idDialogPanel)
	w.c.TrapFocus(idDialogPanel)
	w.c.SetEscapeTarget(idDialogCancel)

	title := rect(r.X+dlgPad, r.Y, r.Width-2*dlgPad-30, 44)
	w.p.text(d.title, title, fontUI, semibold, w.pal.Text)
	if w.action(idDialogClose, right(r)-38, r.Y+9, icons.Close, "Close", actOpts{}) {
		w.after(w.closeDialog)
	}

	f := &form{w: w, x: r.X + dlgPad, y: r.Y + 44, width: r.Width - 2*dlgPad, labelW: 0, collapsed: d.collapsed}
	start := f.y
	if d.body != nil {
		d.body(f)
	}
	if d.err != "" {
		f.gap(4)
		f.message(d.err, w.pal.Error)
	}
	if h := f.y - start; h != d.contentH {
		d.contentH = h
		w.c.RequestFrame()
	}

	by := bottom(r) - dlgPad - buttonH
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

// ---- form rows --------------------------------------------------------------

func (f *form) gap(h float32) { f.y += h }

// message paints wrapped text.
func (f *form) message(s string, c ui.Color) {
	lines := f.w.c.WrapLines(nil, s, fontUI, regular, f.width, 12)
	for _, l := range lines {
		f.w.p.text(l, rect(f.x, f.y, f.width, 20), fontUI, regular, c)
		f.y += 20
	}
	f.y += 4
}

func (f *form) text(s string)  { f.message(s, f.w.pal.Text) }
func (f *form) muted(s string) { f.message(s, f.w.pal.TextSecond) }

// bold paints a one-line emphasized row.
func (f *form) bold(s string) {
	f.w.p.text(s, rect(f.x, f.y, f.width, 22), fontUI, semibold, f.w.pal.Text)
	f.y += 26
}

// collapsibleGroup paints a settings group header with a disclosure arrow and
// reports whether the group is expanded, so the caller can skip its rows. The
// state lives in the dialog: the form is rebuilt on every paint.
func (f *form) collapsibleGroup(key, title string) bool {
	open := !f.collapsed[key]
	id := scDialogGroups.At(f.foldCount)
	f.foldCount++
	f.custom(26, func(r ui.Rect) {
		if f.w.hovered(id) {
			f.w.p.round(r, 4, f.w.pal.Hover)
		}
		arrow := "▾"
		if !open {
			arrow = "▸"
		}
		f.w.p.text(arrow, rect(r.X+2, r.Y, 16, 22), fontUI, regular, f.w.pal.TextSecond)
		f.w.p.text(title, rect(r.X+20, r.Y, r.Width-20, 22), fontUI, semibold, f.w.pal.Text)
		f.w.c.HitWithOptions(id, r, title, ui.HitOptions{Role: ui.RoleButton, Cursor: ui.PointerCursorDefault})
		if f.w.c.Clicked(id) {
			f.w.after(func() { f.collapsed[key] = !f.collapsed[key] })
		}
	})
	return open
}

// field is a labelled text field. With labelW 0 the label sits above.
func (f *form) field(label string, ed *text.Editor, placeholder string, invalid bool) ui.ID {
	id := scDialogFields.At(f.fields)
	f.fields++
	x, width := f.x, f.width
	if label != "" {
		if f.labelW > 0 {
			f.w.p.text(label, rect(f.x, f.y, f.labelW, fieldH), fontUI, regular, f.w.pal.Text)
			x += f.labelW
			width -= f.labelW
		} else {
			f.w.p.text(label, rect(f.x, f.y, f.width, 20), fontUI, regular, f.w.pal.Text)
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
	x, width := f.x, f.width
	if label != "" && f.labelW > 0 {
		f.w.p.text(label, rect(f.x, f.y, f.labelW, fieldH), fontUI, regular, f.w.pal.Text)
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
	if f.w.checkbox(id, f.x+f.labelW, f.y, 24, label, *v, false) {
		f.w.after(func() { *v = !*v })
	}
	f.y += 28
}

func (f *form) checkDisabled(label string, v bool) {
	id := scDialogChecks.At(f.checks)
	f.checks++
	f.w.checkbox(id, f.x+f.labelW, f.y, 24, label, v, true)
	f.y += 28
}

func (f *form) radio(options []string, v *int) {
	for i, o := range options {
		id := scDialogRadios.At(f.radios)
		f.radios++
		if f.w.radio(id, f.x+f.labelW, f.y, 24, o, *v == i) {
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
	x, width := f.x, f.width
	if label != "" {
		if f.labelW > 0 {
			f.w.p.text(label, rect(f.x, f.y, f.labelW, fieldH), fontUI, regular, f.w.pal.Text)
			x += f.labelW
			width -= f.labelW
		} else {
			f.w.p.text(label, rect(f.x, f.y, f.width, 20), fontUI, regular, f.w.pal.Text)
			f.y += 24
		}
	}
	r := rect(x, f.y, width, fieldH)
	if f.w.combo(id, r, value, false) {
		f.w.after(func() { f.w.openMenuBelow(r, choices(), r.Width) })
	}
	f.y += fieldH + 12
}

// custom reserves h pixels for caller drawing.
func (f *form) custom(h float32, paint func(r ui.Rect)) {
	paint(rect(f.x, f.y, f.width, h))
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
