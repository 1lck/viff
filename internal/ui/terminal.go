package viffui

import (
	"os"
	"runtime"
	"time"
	"unicode/utf8"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/i18n"
	"viff/internal/platform/pty"
	"viff/internal/ui/term"
)

const (
	// terminalScrollback is the number of lines the terminal keeps above the
	// screen, as IntelliJ's console does.
	terminalScrollback = 5000
	// terminalParseBudget bounds how much output one wake parses. A flood can
	// leave a megabyte waiting; parsing it in one go costs milliseconds on the
	// window thread, so the rest stays queued for the next frame.
	terminalParseBudget = 128 << 10
	// terminalMinHeight is the shortest useful tool window.
	terminalMinHeight = 160
	// terminalBlinkHalf is how long the cursor stays lit, then dark. It stops
	// blinking after terminalBlinkSpan without input, so an idle window asks for
	// no frames.
	terminalBlinkHalf = 530 * time.Millisecond
	terminalBlinkSpan = 20 * time.Second
)

// terminalView is the Terminal tool window's state: one local shell, its
// emulator, and everything the paint needs. It is owned by the window thread.
type terminalView struct {
	shell *pty.Shell
	emu   *term.Terminal

	// rx holds output drained from the shell but not parsed yet; it survives a
	// frame that used its whole budget.
	rx []byte
	// dir is where the running shell started; the header names it, so a
	// repository switch does not make the panel claim the wrong directory.
	dir string
	// err is why the shell could not start, empty when it did.
	err string
	// ended says the shell exited; the message is in the grid already.
	ended bool
	// wantCols and wantRows are the last size the panel asked for; the shell
	// starts at a conventional 80x24 and the first paint fits it.
	wantCols, wantRows int

	cache   termRowCache
	input   app.TextInput
	preedit app.TextState

	// Selection is kept in buffer points, so it stays on its text while output
	// scrolls.
	anchor, focus term.Point
	selecting     bool
	hasSelection  bool

	blink    terminalBlink
	geometry terminalGeometry
	wheelY   float32
	// focusPending asks the next paint to take the keyboard for the grid.
	focusPending bool
}

// terminalGeometry is where the grid is painted and how many cells fit.
type terminalGeometry struct {
	area       ui.Rect
	left, top  float32
	cols, rows int
	cellW      float32
	cellH      float32
	fontSize   float32
}

// toggleTerminal shows or hides the Terminal tool window, starting the shell the
// first time it is shown.
func (w *Window) toggleTerminal() {
	if w.termOpen {
		w.termOpen = false
		return
	}
	w.openTerminal()
}

// openTerminal shows the tool window and gives the grid the keyboard. It needs
// a repository: the terminal belongs to the project, as IntelliJ's does, and the
// welcome screen has no place to dock it.
func (w *Window) openTerminal() {
	if w.repo() == nil {
		return
	}
	if w.readOnly() {
		w.ws.Notify(vapp.NoticeInfo, w.t(i18n.ChromeStatusReadOnly), w.t(i18n.WorkspaceTipReadOnly))
		return
	}
	w.termOpen = true
	if w.term.shell == nil && w.term.err == "" {
		w.startTerminal()
	}
	w.term.focusPending = true
	w.Invalidate()
}

// startTerminal starts a local shell in the current repository, or the home
// directory when no repository is open. It runs on the window thread; the pty
// fork is the only blocking step and is short.
func (w *Window) startTerminal() {
	v := &w.term
	v.err = ""
	v.ended = false
	v.dir = w.terminalDir()
	const startCols, startRows = 80, 24
	v.wantCols, v.wantRows = startCols, startRows
	v.emu = term.New(startCols, startRows, terminalScrollback)
	v.emu.SetReply(func(reply []byte) {
		if v.shell != nil {
			v.shell.Write(reply)
		}
	})
	v.emu.SetClipboardWrite(func(write term.ClipboardWrite) {
		if write.Text != "" {
			_ = app.SetClipboardText(write.Text)
		}
	})
	shell, err := pty.Start(pty.Config{
		Cols:   startCols,
		Rows:   startRows,
		Dir:    v.dir,
		Notify: w.Invalidate,
	})
	if err != nil {
		v.err = err.Error()
		v.emu = nil
		w.Invalidate()
		return
	}
	v.shell = shell
	v.hasSelection = false
	v.selecting = false
	w.Invalidate()
}

// stopTerminal ends the shell and forgets the screen. Hiding the tool window
// does not do this: only Restart and closing the window do.
func (w *Window) stopTerminal() {
	v := &w.term
	if v.shell != nil {
		v.shell.Close()
	}
	v.shell, v.emu, v.err, v.ended, v.dir = nil, nil, "", false, ""
	v.rx = v.rx[:0]
	v.hasSelection, v.selecting = false, false
	v.preedit = app.TextState{}
	v.cache.reset()
}

// restartTerminal ends the running shell and starts a new one.
func (w *Window) restartTerminal() {
	w.stopTerminal()
	w.startTerminal()
	w.term.focusPending = true
}

// terminalDir is the directory a new shell starts in: the open repository, else
// the user's home directory.
func (w *Window) terminalDir() string {
	if rs := w.repo(); rs != nil && rs.Repo.Root != "" {
		return rs.Repo.Root
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

// pumpTerminal moves the shell's output into the emulator and notices its end.
// It runs in onWake, before the paint that the shell's Notify asked for, so the
// paint only reads state.
func (w *Window) pumpTerminal() {
	v := &w.term
	if v.shell == nil {
		return
	}
	if v.emu == nil {
		// No screen to parse into: drop the output rather than keep it
		// forever. This cannot happen while start and stop keep the pair
		// together, but a future caller must not grow the buffer silently.
		v.rx = v.rx[:0]
		return
	}
	finished := false
	select {
	case <-v.shell.Done():
		finished = true
	default:
	}
	v.rx = v.shell.Drain(v.rx)
	limit := len(v.rx)
	if limit > terminalParseBudget && !finished {
		limit = terminalParseBudget
	}
	if limit > 0 && v.emu != nil {
		_, _ = v.emu.Write(v.rx[:limit])
		if limit < len(v.rx) {
			// Keep the unparsed suffix at the front; the next wake appends new
			// output after it, so order is kept.
			v.rx = v.rx[:copy(v.rx, v.rx[limit:])]
		} else {
			v.rx = v.rx[:0]
			if cap(v.rx) > 4<<20 {
				v.rx = nil // do not keep a flood's buffer
			}
		}
	}
	if finished && !v.ended {
		v.ended = true
		status := v.shell.ExitStatus()
		w.writeExit(status)
		w.Invalidate()
	}
}

// writeExit prints the message IntelliJ leaves when the shell ends.
func (w *Window) writeExit(status int) {
	if v := &w.term; v.emu != nil {
		message := w.t(i18n.TermExited)
		if status != 0 {
			message = w.t(i18n.TermExitedCode, status)
		}
		_, _ = v.emu.Write([]byte("\r\n\x1b[90m" + message + "\x1b[0m\r\n"))
	}
}

// fitTerminal records the size the panel wants and applies it after the paint,
// so resizing never blocks the render path.
func (w *Window) fitTerminal(cols, rows int) {
	v := &w.term
	if cols < 1 || rows < 1 || (cols == v.wantCols && rows == v.wantRows) {
		return
	}
	v.wantCols, v.wantRows = cols, rows
	w.after(func() { w.applyTerminalSize() })
}

// applyTerminalSize tells the emulator and the pty the new cell grid.
func (w *Window) applyTerminalSize() {
	v := &w.term
	if v.emu == nil {
		return
	}
	cols, rows := v.wantCols, v.wantRows
	if c, r := v.emu.Size(); c == cols && r == rows {
		return
	}
	v.emu.Resize(cols, rows)
	if v.shell != nil {
		v.shell.Resize(cols, rows)
	}
	w.Invalidate()
}

// terminalFocused reports whether the grid owns the keyboard.
func (w *Window) terminalFocused() bool {
	return w.termOpen && w.focused(idTerminal)
}

// terminalShortcut takes the few chords the terminal itself owns. Everything
// else falls back to the application, except Control chords that belong to the
// shell (see terminalOwnsChord).
func (w *Window) terminalShortcut(e ui.KeyEvent) (ui.EventResult, bool) {
	if !w.terminalFocused() {
		return ui.EventResult{}, false
	}
	mod := e.Modifiers
	primary := mod&ui.ModifierMeta != 0
	if runtime.GOOS != "darwin" {
		primary = mod&ui.ModifierControl != 0
	}
	if !primary || e.Release {
		return ui.EventResult{}, false
	}
	handled := ui.EventResult{Handled: true, Repaint: true}
	shift := mod&ui.ModifierShift != 0
	switch ch := keyChar(e); {
	case ch == "c" && !shift:
		w.copyTerminalSelection()
		return handled, true
	case ch == "c" && shift:
		// Ctrl+Shift+C is the terminal convention outside macOS.
		if w.copyTerminalSelection() {
			return handled, true
		}
	case ch == "v" && shift:
		if text, err := app.ClipboardText(); err == nil {
			w.pasteTerminal(text)
			return handled, true
		}
	case ch == "v" && !shift && runtime.GOOS == "windows":
		// Windows terminals paste with Control+V; Linux keeps it for the shell.
		if text, err := app.ClipboardText(); err == nil {
			w.pasteTerminal(text)
			return handled, true
		}
	case ch == "k" && !shift:
		if v := &w.term; v.emu != nil {
			v.emu.ClearScrollback()
			v.emu.ClearScreen()
		}
		return handled, true
	case ch == "w" && !shift && mod&ui.ModifierMeta != 0:
		// Command+W closes the tool window on macOS, as it closes IntelliJ's
		// terminal tab. Control+W on other platforms belongs to the shell.
		w.termOpen = false
		w.Invalidate()
		return handled, true
	}
	return ui.EventResult{}, false
}

// pasteTerminal sends clipboard text the way a paste event would, with the
// bracketed-paste markers a program asked for.
func (w *Window) pasteTerminal(text string) {
	v := &w.term
	if v.shell == nil || v.emu == nil || text == "" {
		return
	}
	v.hasSelection, v.selecting = false, false
	v.emu.ScrollToBottom()
	v.shell.Write(term.EncodePaste(text, v.emu.Modes().BracketedPaste))
}

// terminalOwnsChord reports whether a Control chord must reach the shell rather
// than the application's shortcut table. On macOS Control is the shell's; on
// Windows and Linux, where the application uses Control for its own chords, the
// terminal keeps plain Control+letter and leaves Control+Shift for Viff.
func (w *Window) terminalOwnsChord(e ui.KeyEvent) bool {
	if !w.terminalFocused() || e.Release {
		return false
	}
	if e.Modifiers&ui.ModifierMeta != 0 || e.Modifiers&ui.ModifierShift != 0 {
		return false
	}
	return e.Modifiers&ui.ModifierControl != 0
}

// terminalKey encodes one key for the shell. It runs from the hit's key handler
// after the shortcut table declined the event.
func (w *Window) terminalKey(e ui.KeyEvent) bool {
	v := &w.term
	if v.emu == nil {
		if e.Key == ui.KeyEnter && !e.Release && v.err != "" {
			w.startTerminal()
			return true
		}
		return false
	}
	if v.preedit.Composing && e.Key == ui.KeyEnter && e.Modifiers == 0 && !v.emu.Modes().AltScreen {
		return true
	}
	modes := v.emu.Modes()
	if modes.AltScreen {
		// The alternate screen owns the wheel through the application's arrow
		// keys; the handler in terminalScroll sends those.
	} else if e.Modifiers&ui.ModifierShift != 0 && e.Modifiers&ui.ModifierMeta == 0 {
		_, rows := v.emu.Size()
		switch e.Key {
		case ui.KeyPageUp:
			v.emu.Scroll(rows - 1)
			return true
		case ui.KeyPageDown:
			v.emu.Scroll(-(rows - 1))
			return true
		}
	}
	if e.Modifiers&ui.ModifierMeta != 0 {
		// Command chords the application did not take belong to it; a
		// terminal program has no use for a Super key. On macOS the line-edit
		// chords still map to their readline equivalents, as Motty does.
		if runtime.GOOS != "darwin" {
			return true
		}
		letter, ok := term.CommandLineEdit(e.Key, e.Modifiers)
		if !ok {
			return true
		}
		e.Key, e.Modifiers, e.Text, e.Typed = ui.KeyUnknown, ui.ModifierControl, letter, ""
	}
	var data []byte
	if modes.Win32Input {
		data = term.EncodeWin32Key(e)
	}
	if len(data) == 0 {
		data = term.EncodeKeyEvent(e, v.emu.KeyboardFlags(), modes.AppCursorKeys)
	}
	if len(data) == 0 {
		return true
	}
	v.hasSelection, v.selecting = false, false
	v.emu.ScrollToBottom() // typing returns to the live screen, as in IntelliJ
	if v.shell != nil {
		v.shell.Write(data)
	}
	return true
}

// terminalText applies a native text-system event: committed text, paste, and
// input-method composition.
func (w *Window) terminalText(event app.TextEvent) app.TextState {
	v := &w.term
	switch event.Kind {
	case app.TextInsert:
		v.preedit = app.TextState{}
		v.hasSelection, v.selecting = false, false
		if v.emu != nil {
			v.emu.ScrollToBottom()
		}
		if v.shell != nil {
			v.shell.Write([]byte(event.Text))
		}
	case app.TextPaste:
		v.preedit = app.TextState{}
		v.hasSelection, v.selecting = false, false
		if v.emu != nil {
			v.emu.ScrollToBottom()
		}
		if v.shell != nil && v.emu != nil {
			v.shell.Write(term.EncodePaste(event.Text, v.emu.Modes().BracketedPaste))
		}
	case app.TextSetMarked:
		runes := utf8.RuneCountInString(event.Text)
		v.preedit = app.TextState{Value: event.Text, Selection: event.MarkedSelection, Composition: app.TextRange{End: runes}, Composing: runes > 0}
	case app.TextUnmark:
		v.preedit = app.TextState{}
	}
	return v.preedit
}

// copyTerminalSelection puts the selected text on the clipboard and reports
// whether there was anything to copy.
func (w *Window) copyTerminalSelection() bool {
	v := &w.term
	if v.emu == nil || !v.hasSelection {
		return false
	}
	text := v.emu.SelectedRange(v.anchor, v.focus)
	if text == "" {
		return false
	}
	_ = app.SetClipboardText(text)
	return true
}

// terminalScroll handles the wheel over the grid: accumulated so a trackpad's
// fraction of a line is not lost, and turned into arrow keys on the alternate
// screen where the program owns scrolling.
func (w *Window) terminalScroll(dy float32) {
	v := &w.term
	if v.emu == nil || v.geometry.cellH <= 0 {
		return
	}
	modes := v.emu.Modes()
	if modes.AltScreen {
		if modes.Mouse != term.MouseOff || !modes.AlternateScroll {
			return
		}
	}
	v.wheelY += dy / v.geometry.cellH
	lines := int(v.wheelY)
	if lines == 0 {
		return
	}
	v.wheelY -= float32(lines)
	if modes.AltScreen {
		key := ui.KeyDown
		if lines < 0 {
			key, lines = ui.KeyUp, -lines
		}
		for range min(lines, 5) {
			if v.shell != nil {
				v.shell.Write(term.EncodeKey(key, 0, "", modes.AppCursorKeys))
			}
		}
		return
	}
	v.emu.Scroll(-lines)
}

// terminalPoint maps a view coordinate to a buffer point, clamped to the grid.
func (v *terminalView) terminalPoint(x, y float32) term.Point {
	g := v.geometry
	col := int((x - g.left) / g.cellW)
	row := int((y - g.top) / g.cellH)
	col = max(0, min(col, g.cols-1))
	row = max(0, min(row, g.rows-1))
	return v.emu.PointAt(term.Position{Row: row, Column: col})
}

// terminalBlink is the cursor's phase. A cursor that moved restarts it; after
// terminalBlinkSpan it stays lit so an idle window requests no frames.
type terminalBlink struct {
	since time.Duration
	x, y  int
	seen  bool
}

func (b *terminalBlink) on(canvas *ui.Canvas, x, y int) bool {
	now := canvas.Now()
	if !b.seen || b.x != x || b.y != y {
		b.since, b.x, b.y, b.seen = now, x, y, true
	}
	elapsed := max(now-b.since, 0)
	if elapsed >= terminalBlinkSpan || canvas.ReduceMotion() {
		return true
	}
	phase := elapsed % (2 * terminalBlinkHalf)
	if phase < terminalBlinkHalf {
		canvas.RequestFrameIn(terminalBlinkHalf - phase)
		return true
	}
	canvas.RequestFrameIn(2*terminalBlinkHalf - phase)
	return false
}

// terminalSelectionSpan returns the selected cells of one view row.
func (v *terminalView) terminalSelectionSpan(row int) (from, to int, ok bool) {
	if !v.hasSelection || v.emu == nil {
		return 0, 0, false
	}
	anchor, focus := v.anchor, v.focus
	if focus.Line < anchor.Line || (focus.Line == anchor.Line && focus.Column < anchor.Column) {
		anchor, focus = focus, anchor
	}
	line := v.emu.LineNumber(row)
	if line < anchor.Line || line > focus.Line {
		return 0, 0, false
	}
	from, to = 0, v.geometry.cols-1
	if line == anchor.Line {
		from = anchor.Column
	}
	if line == focus.Line {
		to = focus.Column
	}
	return max(from, 0), min(to, v.geometry.cols-1), to >= from
}

// ensureTerminalInput returns the native text surface for the grid: the cursor
// cell where an input method composes, and the receiver of committed text.
func (w *Window) ensureTerminalInput() *app.TextInput {
	v := &w.term
	if v.input.OnEvent == nil {
		v.input.OnEvent = func(event app.TextEvent) (app.TextState, error) {
			return w.terminalText(event), nil
		}
	}
	v.input.FontSize = v.geometry.fontSize
	v.input.MonoFace = w.mono
	v.input.Color = w.pal.TermFG
	v.input.State = v.preedit
	// Option as Meta would turn every Option chord into an escape prefix; the
	// terminal keeps the platform's composition until a setting asks otherwise.
	v.input.AltSendsEscape = false
	if v.emu != nil {
		v.input.KeyReleases = v.emu.KeyboardFlags() != 0 || v.emu.Modes().Win32Input
		cursor := v.emu.Cursor()
		g := v.geometry
		if g.cellW > 0 && g.cellH > 0 {
			cells := 1
			if v.preedit.Composing {
				cells += displayCells(v.preedit.Value)
			}
			v.input.Bounds = ui.Rect{
				X:     g.left + float32(cursor.X)*g.cellW,
				Y:     g.top + float32(cursor.Y)*g.cellH,
				Width: float32(cells) * g.cellW, Height: g.cellH,
			}
		}
	}
	return &v.input
}

// terminalStatus is the line under the title: the shell's directory, or why it
// could not start.
func (w *Window) terminalStatus() string {
	if w.term.err != "" {
		return w.term.err
	}
	return w.term.dir
}

// closeProject ends the terminal and closes the repository. The shell belongs
// to the project, so it does not outlive the project in a window with no dock;
// a running shell is worth one confirmation before it is killed.
func (w *Window) closeProject() {
	if v := &w.term; v.shell != nil && !v.ended {
		w.confirm(w.t(i18n.ChromeMenuCloseProject), w.t(i18n.TermCloseRunning), w.t(i18n.ChromeMenuCloseProject), func() {
			w.stopTerminal()
			w.termOpen = false
			w.ws.Close()
		})
		return
	}
	w.stopTerminal()
	w.termOpen = false
	w.ws.Close()
}

// terminalTitle is the tool window's title with the shell's name when known.
func (w *Window) terminalTitle() string { return w.t(i18n.TermTitle) }
