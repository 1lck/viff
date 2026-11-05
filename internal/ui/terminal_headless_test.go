//go:build vivahead

package viffui

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

// terminalFixture opens the Terminal tool window and waits until its shell has
// a grid and the keyboard. The headless runner needs a pty, so these tests run
// on Unix; Windows runs them on CI with ConPTY.
func terminalFixture(t *testing.T) *uiFixture {
	t.Helper()
	f := newUIFixture(t)
	f.start(1280, 820)
	f.win.openRepo(f.dir)
	f.settle()
	// The shell inherits the test process's environment. Keep it away from the
	// developer's dotfiles and pin a plain shell, so the prompt is stable and
	// the test reads no real user settings.
	if runtime.GOOS != "windows" {
		t.Setenv("SHELL", "/bin/sh")
		t.Setenv("HOME", filepath.Join(f.dir, "..", "home"))
		t.Setenv("ZDOTDIR", "")
	}
	if err := f.s.Keys("alt+f12"); err != nil {
		t.Fatalf("toggle terminal: %v", err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.termOpen && f.win.term.emu != nil && f.win.term.shell != nil
	}, 10*time.Second); err != nil {
		t.Fatalf("terminal did not start: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	if err := f.s.Settle(40*time.Millisecond, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	// Opening the tool window must hand the keyboard to the grid, so typing
	// needs no click.
	if err := f.s.WaitFor(func(snapshot app.Snapshot) bool {
		return snapshot.Focused == idTerminal
	}, 5*time.Second); err != nil {
		for _, hit := range f.s.Snapshot().Hits {
			t.Logf("hit %v label=%q bounds=%+v role=%v", hit.ID, hit.Label, hit.Bounds, hit.Role)
		}
		t.Fatalf("the grid did not take the keyboard: %v", err)
	}
	return f
}

func snapshotHasText(snapshot app.Snapshot, needle string) bool {
	for _, text := range snapshot.Texts {
		if strings.Contains(text.Value, needle) {
			return true
		}
	}
	return false
}

func TestTerminalRunsACommandAndShowsItsOutput(t *testing.T) {
	f := terminalFixture(t)
	if err := f.s.Type("git status --short"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(snapshot app.Snapshot) bool {
		return snapshotHasText(snapshot, "?? notes/")
	}, 10*time.Second); err != nil {
		t.Fatalf("the shell's output was not painted: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	// A blinking cursor asks for frames only while it blinks; an idle window
	// must not spin.
	f.assertIdle()
}

func TestTerminalFollowsThePanelSize(t *testing.T) {
	f := terminalFixture(t)
	beforeCols, beforeRows := f.win.term.emu.Size()
	if err := f.s.Resize(980, 700); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		cols, rows := f.win.term.emu.Size()
		return cols != beforeCols || rows != beforeRows
	}, 5*time.Second); err != nil {
		t.Fatalf("the pty did not follow the panel: %v", err)
	}
}

func TestTerminalHidesWithoutEndingTheShell(t *testing.T) {
	f := terminalFixture(t)
	shell := f.win.term.shell
	if err := f.s.Keys("alt+f12"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if f.win.termOpen {
		t.Fatal("the tool window stayed open")
	}
	if f.win.term.shell != shell {
		t.Fatal("hiding the window ended the shell")
	}
	if err := f.s.Keys("alt+f12"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if !f.win.termOpen || f.win.term.shell != shell {
		t.Fatal("showing the window started another shell")
	}
}

func TestTerminalCopiesASelection(t *testing.T) {
	f := terminalFixture(t)
	if err := f.s.Type("git status --short"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(snapshot app.Snapshot) bool {
		return snapshotHasText(snapshot, "?? notes/")
	}, 10*time.Second); err != nil {
		t.Fatalf("output missing: %v", err)
	}
	// Drag over the row that holds the output, then copy it with the platform
	// chord.
	geo := f.win.term.geometry
	if geo.rows == 0 {
		t.Fatal("the grid has no geometry")
	}
	row := -1
	for index := 0; index < geo.rows; index++ {
		line := f.win.term.emu.RowText(index)
		if strings.Contains(line, "?? notes/") {
			row = index
			break
		}
	}
	if row < 0 {
		t.Fatalf("the output row is not on screen: %q", strings.Join(f.s.Texts(), " | "))
	}
	y := geo.top + float32(row)*geo.cellH + geo.cellH/2
	x0 := geo.left
	x1 := geo.left + float32(geo.cols-1)*geo.cellW
	start := ui.Point{X: x0, Y: y}
	end := ui.Point{X: x1, Y: y}
	if err := f.s.Press(start.X, start.Y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Move(end.X, end.Y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Release(end.X, end.Y); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	chord := "ctrl+c"
	if runtime.GOOS == "darwin" {
		chord = "cmd+c"
	}
	if err := f.s.Keys(chord); err != nil {
		t.Fatal(err)
	}
	if got := f.s.Driver().Clipboard(); !strings.Contains(got, "?? notes/") {
		t.Fatalf("clipboard = %q, want the selected output", got)
	}
}

func TestTerminalPastesWithThePlatformChord(t *testing.T) {
	f := terminalFixture(t)
	f.s.Driver().SetClipboard("git status --short")
	chord := "ctrl+shift+v"
	switch runtime.GOOS {
	case "darwin":
		chord = "cmd+v"
	case "windows":
		chord = "ctrl+v"
	}
	if err := f.s.Keys(chord); err != nil {
		t.Fatalf("paste chord %s: %v", chord, err)
	}
	if err := f.s.Keys("enter"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(snapshot app.Snapshot) bool {
		return snapshotHasText(snapshot, "?? notes/")
	}, 10*time.Second); err != nil {
		t.Fatalf("pasted text was not run: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
}

func TestTerminalExplainsAFailedShellStart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows chooses its shell from COMSPEC, not SHELL")
	}
	f := newUIFixture(t)
	f.start(1280, 820)
	f.win.openRepo(f.dir)
	f.settle()
	t.Setenv("SHELL", filepath.Join(t.TempDir(), "missing-shell"))
	if err := f.s.Keys("alt+f12"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WaitFor(func(snapshot app.Snapshot) bool {
		return snapshotHasText(snapshot, "Could not start the shell")
	}, 5*time.Second); err != nil {
		t.Fatalf("the failure was not shown: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
	if f.win.term.err == "" {
		t.Fatal("the terminal state has no error")
	}
	if f.win.term.emu != nil || f.win.term.shell != nil {
		t.Fatal("a failed start left a terminal behind")
	}
}

func TestClosingTheProjectEndsTheShell(t *testing.T) {
	f := terminalFixture(t)
	shell := f.win.term.shell
	f.win.closeProject()
	f.win.Invalidate()
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if f.win.dialog == nil {
		t.Fatal("closing a project with a running shell asked no confirmation")
	}
	if err := f.s.ClickLabel("Close Project"); err != nil {
		t.Fatalf("confirm close: %v", err)
	}
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if f.win.term.shell != nil || f.win.term.emu != nil {
		t.Fatal("the terminal survived the project")
	}
	select {
	case <-shell.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("closing the project left the shell running")
	}
}
