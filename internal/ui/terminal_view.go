package viffui

import (
	"unicode/utf8"

	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
	"viff/internal/ui/icons"
	"viff/internal/ui/term"
)

// termPalette is the emulator's colors resolved against the window palette.
type termPalette struct {
	fg, bg, cursor, selection ui.Color
	ansi                      [16][3]uint8
}

func newTermPalette(p *Palette) termPalette {
	pal := termPalette{fg: p.TermFG, bg: p.TermBG, cursor: p.TermCursor, selection: p.TermSelection}
	for i, c := range p.TermANSI {
		pal.ansi[i] = [3]uint8{byte(c.Red*255 + 0.5), byte(c.Green*255 + 0.5), byte(c.Blue*255 + 0.5)}
	}
	return pal
}

// color resolves a terminal color, with fallback standing in for the default.
func (pal *termPalette) color(c term.Color, fallback ui.Color) ui.Color {
	switch c.Kind() {
	case term.PaletteColor:
		red, green, blue := term.Resolve(c.Index(), &pal.ansi)
		return ui.RGB(red, green, blue)
	case term.TrueColor:
		red, green, blue := c.Components()
		return ui.RGB(red, green, blue)
	}
	return fallback
}

// termSpan is a stretch of cells painted with a background color.
type termSpan struct {
	from, to int
	color    ui.Color
}

// termCellSpan is a stretch of covered cells; the zero value is empty.
type termCellSpan struct {
	from, to int
}

func (span termCellSpan) holds(column int) bool { return column >= span.from && column < span.to }

// termRowEntry is one terminal line prepared for painting. It stays valid while
// the line's identity, version, cursor, covered cells and palette stay the
// same, so an unchanged line costs nothing to paint again and a line that only
// moved (the screen scrolled) is found again by its ID.
type termRowEntry struct {
	id      uint64
	version uint32
	cursor  int
	covered termCellSpan
	epoch   uint32
	cols    int
	used    uint32
	text    string
	runs    []ui.TextRun
	spans   []termSpan
}

// termRowCache holds the prepared lines of one terminal.
type termRowCache struct {
	entries    map[uint64]*termRowEntry
	free       []*termRowEntry
	generation uint32
	scratch    []byte
}

func (cache *termRowCache) reset() {
	cache.entries = nil
	cache.free = nil
	cache.scratch = nil
}

func (cache *termRowCache) begin() { cache.generation++ }

// end releases the entries the pass did not use, so the cache holds about one
// screenful however long the terminal has run.
func (cache *termRowCache) end(visible int) {
	if len(cache.entries) <= visible*2+8 {
		return
	}
	for id, entry := range cache.entries {
		if entry.used != cache.generation {
			delete(cache.entries, id)
			cache.free = append(cache.free, entry)
		}
	}
}

// row returns the prepared form of a terminal line, building it when the line
// is new or changed.
func (cache *termRowCache) row(line term.Row, cols, cursor int, covered termCellSpan, pal *termPalette, epoch uint32) *termRowEntry {
	entry := cache.entries[line.ID]
	if entry == nil {
		if cache.entries == nil {
			cache.entries = make(map[uint64]*termRowEntry)
		}
		if last := len(cache.free) - 1; last >= 0 {
			entry, cache.free = cache.free[last], cache.free[:last]
		} else {
			entry = &termRowEntry{}
		}
		cache.entries[line.ID] = entry
		entry.id = line.ID
		entry.version = ^uint32(0)
	}
	entry.used = cache.generation
	if entry.version != line.Version || entry.cursor != cursor || entry.covered != covered || entry.epoch != epoch || entry.cols != cols {
		cache.build(entry, line, cols, cursor, covered, pal)
		entry.version, entry.cursor, entry.covered, entry.epoch, entry.cols = line.Version, cursor, covered, epoch, cols
	}
	return entry
}

// termStyle is what a cell looks like once its colors and attributes are
// resolved against the palette.
type termStyle struct {
	fg        ui.Color
	bg        ui.Color
	hasBG     bool
	weight    float32
	italic    bool
	underline bool
	strike    bool
}

// termCellKey is the part of a cell resolveCell reads.
type termCellKey struct {
	fg   term.Color
	bg   term.Color
	attr term.Attr
}

func (pal *termPalette) resolveCell(cell term.Cell) termStyle {
	bold := cell.Attr&term.Bold != 0
	fg := cell.FG
	if bold && fg.Kind() == term.PaletteColor && fg.Index() < 8 {
		fg = term.Indexed(fg.Index() + 8)
	}
	style := termStyle{fg: pal.color(fg, pal.fg)}
	if cell.BG != 0 {
		style.bg, style.hasBG = pal.color(cell.BG, pal.bg), true
	}
	if cell.Attr&term.Inverse != 0 {
		inkSource := pal.color(cell.BG, pal.bg)
		style.bg, style.hasBG = pal.color(cell.FG, pal.fg), true
		style.fg = inkSource
	}
	if cell.Attr&term.Dim != 0 {
		style.fg.Alpha *= 0.6
	}
	if bold {
		style.weight = ui.WeightBold
	}
	style.italic = cell.Attr&term.Italic != 0
	style.underline = cell.Attr&term.Underline != 0
	style.strike = cell.Attr&term.Strike != 0
	return style
}

// build turns a terminal line into text, runs and background spans. Every
// non-ASCII character is a run of its own, placed at its cell by the renderer,
// so a wide or fallback-font glyph can never push the text after it off its
// column.
func (cache *termRowCache) build(entry *termRowEntry, line term.Row, cols, cursor int, covered termCellSpan, pal *termPalette) {
	entry.runs = entry.runs[:0]
	entry.spans = entry.spans[:0]
	text := cache.scratch[:0]
	cells := line.Cells
	count := min(len(cells), cols)
	if cursor >= 0 && cursor < count && cells[cursor].Attr&term.WideTail != 0 && cursor > 0 {
		cursor-- // the cursor sits on the head of a wide character
	}
	if covered.from > 0 && covered.from < count && cells[covered.from].Attr&term.WideTail != 0 {
		covered.from-- // the cover starts on the tail of a wide character: take its head too
	}
	cursorStyle := termStyle{fg: pal.bg, bg: pal.cursor, hasBG: true}
	run := -1
	runASCII := false
	marks := line.Marks
	var (
		key    termCellKey
		style  termStyle
		styled bool
	)
	var (
		spanFrom  = -1
		spanTo    int
		spanColor ui.Color
	)
	closeSpan := func() {
		if spanFrom >= 0 {
			entry.spans = append(entry.spans, termSpan{from: spanFrom, to: spanTo, color: spanColor})
			spanFrom = -1
		}
	}
	for x := 0; x < count; x++ {
		cell := cells[x]
		if cell.Attr&term.WideTail != 0 {
			continue
		}
		width := 1
		if cell.Attr&term.WideHead != 0 {
			width = 2
		}
		if covered.holds(x) {
			closeSpan()
			continue
		}
		if k := (termCellKey{fg: cell.FG, bg: cell.BG, attr: cell.Attr}); !styled || k != key {
			key, style, styled = k, pal.resolveCell(cell), true
		}
		ink := style
		if x == cursor {
			ink = cursorStyle
		}
		switch {
		case !ink.hasBG:
			closeSpan()
		case spanFrom >= 0 && spanColor == ink.bg && spanTo == x:
			spanTo = x + width
		default:
			closeSpan()
			spanFrom, spanTo, spanColor = x, x+width, ink.bg
		}
		character := cell.R
		if character < 0x20 || (character >= 0x7f && character < 0xa0) {
			character = 0
		}
		blank := character == 0 || character == ' ' || cell.Attr&term.Hidden != 0
		if blank && !ink.underline && !ink.strike {
			if run >= 0 && runASCII && !entry.runs[run].Underline && !entry.runs[run].Strike &&
				int(entry.runs[run].Column+entry.runs[run].Cells) == x {
				entry.runs[run].Bytes++
				entry.runs[run].Cells += int32(width)
				text = append(text, ' ')
			}
			continue
		}
		if blank {
			character = ' '
		}
		for len(marks) > 0 && int(marks[0].Col) < x {
			marks = marks[1:]
		}
		ascii := character < 0x7f && !(len(marks) > 0 && int(marks[0].Col) == x)
		if run >= 0 && ascii && runASCII && int(entry.runs[run].Column+entry.runs[run].Cells) == x &&
			sameTermInk(entry.runs[run], ink) {
			entry.runs[run].Bytes++
			entry.runs[run].Cells++
			text = append(text, byte(character))
			continue
		}
		before := len(text)
		text = utf8.AppendRune(text, character)
		for len(marks) > 0 && int(marks[0].Col) == x {
			text = utf8.AppendRune(text, marks[0].R)
			marks = marks[1:]
		}
		entry.runs = append(entry.runs, ui.TextRun{
			Column: int32(x), Bytes: int32(len(text) - before), Cells: int32(width),
			Color: ink.fg, Weight: ink.weight, Italic: ink.italic, Underline: ink.underline, Strike: ink.strike,
		})
		run = len(entry.runs) - 1
		runASCII = ascii
	}
	closeSpan()
	if cursor >= count && cursor >= 0 {
		entry.spans = append(entry.spans, termSpan{from: cursor, to: cursor + 1, color: pal.cursor})
	}
	// Blanks at the end of the last run are not worth drawing.
	for run = len(entry.runs) - 1; run >= 0; run = len(entry.runs) - 1 {
		last := &entry.runs[run]
		if len(text) == 0 || text[len(text)-1] != ' ' || last.Underline || last.Strike {
			break
		}
		text = text[:len(text)-1]
		last.Bytes--
		last.Cells--
		if last.Bytes == 0 {
			entry.runs = entry.runs[:run]
		}
	}
	cache.scratch = text
	entry.text = string(text)
	// A run of plain ASCII with room for a substitution is shaped as one line by
	// Viva, so a programming font's ligatures appear; single cells and non-ASCII
	// runs keep the one-glyph-per-cell path.
	offset := 0
	for index := range entry.runs {
		r := &entry.runs[index]
		end := offset + int(r.Bytes)
		r.Ligatures = r.Cells >= 2 && asciiOnly(entry.text[offset:end])
		offset = end
	}
}

func sameTermInk(run ui.TextRun, style termStyle) bool {
	return run.Color == style.fg && run.Weight == style.weight &&
		run.Italic == style.italic && run.Underline == style.underline && run.Strike == style.strike
}

func asciiOnly(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < 0x20 || value[index] >= 0x7f {
			return false
		}
	}
	return true
}

// paintTerminal draws the Terminal tool window: its header, the character grid
// with its scrollback, and the actions that restart, clear or hide it.
func (w *Window) paintTerminal(r ui.Rect) {
	w.island(r)
	v := &w.term
	header := splitTop(&r, 32)
	pad := float32(10)
	w.p.icon(icons.Shell, header.X+8, header.Y+(header.Height-16)/2, 16)
	textX := header.X + 30
	textW := max(right(header)-8-actionSize*3-6-textX, 60)
	w.p.text(w.terminalTitle(), rect(textX, header.Y+3, textW, 15), 12, semibold, w.pal.Text)
	w.p.text(w.terminalStatus(), rect(textX, header.Y+17, textW, 12), 10, regular, w.pal.TextSecond)

	ax := right(header) - 8 - actionSize
	if w.action(idTerminalHide, ax, header.Y+(header.Height-actionSize)/2, icons.Hide, w.t(i18n.TermHide), actOpts{}) {
		w.after(func() { w.termOpen = false })
	}
	ax -= actionSize + 2
	if w.action(idTerminalClear, ax, header.Y+(header.Height-actionSize)/2, icons.Delete, w.t(i18n.TermClear), actOpts{}) {
		w.after(func() {
			if v.emu != nil {
				v.emu.ClearScreen()
				v.emu.ClearScrollback()
			}
		})
	}
	ax -= actionSize + 2
	if w.action(idTerminalRestart, ax, header.Y+(header.Height-actionSize)/2, icons.Refresh, w.t(i18n.TermRestart), actOpts{}) {
		w.after(w.restartTerminal)
	}
	w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
	r = inset(r, 1, 0)

	if v.emu == nil {
		w.p.fill(r, w.pal.TermBG)
		message := w.t(i18n.TermStarting)
		if v.err != "" {
			message = w.t(i18n.TermFailed) + ": " + v.err
		}
		w.p.textCenter(message, rect(r.X+pad, r.Y+r.Height/2-20, r.Width-2*pad, 20), 13, regular, w.pal.TextMuted)
		if v.err != "" {
			label := w.t(i18n.TermRetry)
			bw := w.c.TextWidthWeight(label, 12, regular) + 28
			br := rect(r.X+(r.Width-bw)/2, r.Y+r.Height/2+10, bw, 26)
			if w.button(idTerminalRetry, br, label, btnNormal, false) {
				w.after(w.restartTerminal)
			}
		}
		w.c.HitWithOptions(idTerminal, r, w.t(i18n.TermA11y), ui.HitOptions{Role: ui.RoleTerminal})
		w.onKeys(idTerminal, w.terminalKey)
		return
	}

	fs := w.ws.Settings.FontSize
	if fs < 8 {
		fs = 13
	}
	cellW, cellH := w.c.MonoCellSize(fs, w.mono)
	if cellW <= 0 || cellH <= 0 {
		return
	}
	area := inset(r, pad, 6)
	availCols := max(1, int(area.Width/cellW))
	availRows := max(1, int(area.Height/cellH))
	w.fitTerminal(availCols, availRows)
	// Until the deferred resize runs, the emulator still has its previous grid:
	// draw that, clipped to the room the panel has, rather than asking for rows
	// it does not have.
	cols, rows := v.emu.Size()
	cols, rows = max(1, min(cols, availCols)), max(1, min(rows, availRows))
	geo := terminalGeometry{
		area: area, left: area.X, top: area.Y,
		cols: cols, rows: rows, cellW: cellW, cellH: cellH, fontSize: fs,
	}
	v.geometry = geo

	w.p.fill(area, w.pal.TermBG)
	cursor := v.emu.Cursor()
	blinkOn := true
	focused := w.focused(idTerminal)
	if cursor.Visible && focused && !v.preedit.Composing {
		blinkOn = v.blink.on(w.c, cursor.X, cursor.Y)
	} else {
		v.blink.seen = false
	}
	block := cursor.Visible && cursor.Style == term.CursorBlock && focused && !v.preedit.Composing && blinkOn
	var covered termCellSpan
	if v.preedit.Composing && cursor.Y < rows {
		covered = termCellSpan{from: cursor.X, to: cursor.X + displayCells(v.preedit.Value)}
	}
	pal := newTermPalette(w.pal)
	epoch := uint32(0)
	if w.pal.Dark {
		epoch = 1
	}
	style := ui.TextStyle{Size: fs, MonoFace: w.mono}

	w.c.Clip(area)
	v.cache.begin()
	for row := 0; row < rows; row++ {
		column := -1
		if block && row == cursor.Y {
			column = cursor.X
		}
		line := v.emu.Row(row)
		entry := v.cache.row(line, cols, column, coveredOnRow(covered, row == cursor.Y), &pal, epoch)
		y := geo.top + float32(row)*cellH
		for _, span := range entry.spans {
			from := geo.left + float32(span.from)*cellW
			to := geo.left + float32(span.to)*cellW
			w.p.fill(ui.Rect{X: from, Y: y, Width: to - from, Height: cellH}, span.color)
		}
		if from, to, ok := v.terminalSelectionSpan(row); ok {
			left := geo.left + float32(from)*cellW
			right := geo.left + float32(to+1)*cellW
			w.p.fill(ui.Rect{X: left, Y: y, Width: right - left, Height: cellH}, w.pal.TermSelection)
		}
		if len(entry.runs) > 0 {
			labelW := float32(cols)*cellW + cellW
			w.c.LabelRuns(entry.text, rect(geo.left, y, labelW, cellH), style, cellW, entry.runs)
		}
	}
	v.cache.end(rows)
	w.c.Unclip()

	if v.preedit.Composing && cursor.Y < rows {
		x := geo.left + float32(cursor.X)*cellW
		y := geo.top + float32(cursor.Y)*cellH
		w.p.fill(rect(x, y, float32(max(1, displayCells(v.preedit.Value)))*cellW, cellH), w.pal.TermBG)
	}
	if cursor.Visible && cursor.Y < rows && blinkOn && !v.preedit.Composing {
		x := geo.left + float32(cursor.X)*cellW
		y := geo.top + float32(cursor.Y)*cellH
		color := w.pal.TermCursor
		if !focused {
			// A hollow block: the grid does not own the keyboard.
			w.p.fill(rect(x, y, cellW, 1), color)
			w.p.fill(rect(x, y+cellH-1, cellW, 1), color)
			w.p.fill(rect(x, y, 1, cellH), color)
			w.p.fill(rect(x+cellW-1, y, 1, cellH), color)
		} else {
			switch cursor.Style {
			case term.CursorUnderline:
				w.p.fill(rect(x, y+cellH-2, cellW, 2), color)
			case term.CursorBar:
				w.p.fill(rect(x, y, 2, cellH), color)
			}
		}
	}
	w.paintTerminalScrollbar(geo, v)
	v.input.Bounds = ui.Rect{
		X:     geo.left + float32(min(cursor.X, cols-1))*cellW,
		Y:     geo.top + float32(min(cursor.Y, rows-1))*cellH,
		Width: cellW, Height: cellH,
	}
	w.c.HitWithOptions(idTerminal, area, w.t(i18n.TermA11y), ui.HitOptions{Role: ui.RoleTerminal, Autofocus: v.focusPending})
	v.focusPending = false
	w.onKeys(idTerminal, w.terminalKey)
	w.onScrollIn(area, func(_, dy float32) { w.terminalScroll(dy) })
	w.onDrag(idTerminal, func(e dragEvent) {
		switch e.Phase {
		case dragBegin:
			v.anchor = v.terminalPoint(e.X, e.Y)
			v.focus = v.anchor
			v.selecting, v.hasSelection = true, false
		case dragMove:
			if v.selecting {
				v.focus = v.terminalPoint(e.X, e.Y)
				v.hasSelection = v.anchor != v.focus
			}
		case dragEnd:
			v.selecting = false
			v.hasSelection = v.anchor != v.focus
		}
	})
	w.onDouble(idTerminal, func() {
		point := v.terminalPoint(w.mouse.X, w.mouse.Y)
		from, to := v.emu.WordRange(point)
		v.anchor, v.focus = from, to
		v.hasSelection = true
	})
}

// coveredOnRow is covered when it is on the row being prepared, else nothing.
func coveredOnRow(covered termCellSpan, onRow bool) termCellSpan {
	if onRow {
		return covered
	}
	return termCellSpan{}
}

// paintTerminalScrollbar draws the history thumb on the grid's right edge and
// turns a drag on the track into a view offset.
func (w *Window) paintTerminalScrollbar(geo terminalGeometry, v *terminalView) {
	scrollback := v.emu.Scrollback()
	if scrollback <= 0 || geo.rows <= 1 {
		return
	}
	track := ui.Rect{X: geo.area.X + geo.area.Width - 10, Y: geo.top, Width: 10, Height: float32(geo.rows) * geo.cellH}
	content := float32(scrollback+geo.rows) * geo.cellH
	view := float32(geo.rows) * geo.cellH
	thumbH := max(view*view/content, 24)
	offset := float32(scrollback-v.emu.ViewOffset()) * geo.cellH
	thumbY := track.Y + offset/(content-view)*(view-thumbH)
	hot := w.hovered(idTerminalScroll) || (w.drag != nil && w.drag.id == idTerminalScroll)
	tw := float32(6)
	if hot {
		tw = 8
	}
	r := rect(track.X+track.Width-tw-2, thumbY, tw, thumbH)
	w.p.round(r, tw/2, w.pal.ScrollThumb)
	w.c.HitWithOptions(idTerminalScroll, track, w.t(i18n.TermA11yScroll), ui.HitOptions{SkipFocus: true, SkipTab: true})
	w.onDrag(idTerminalScroll, func(e dragEvent) {
		if e.Phase == dragMove || e.Phase == dragBegin {
			travel := view - thumbH
			if travel <= 0 {
				return
			}
			fraction := clampF((e.Y-track.Y-thumbH/2)/travel, 0, 1)
			v.emu.Scroll(int((1-fraction)*float32(scrollback)) - v.emu.ViewOffset())
		}
	})
}
