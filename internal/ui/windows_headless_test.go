//go:build vivahead

package viffui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/gitcli"
	"viff/internal/i18n"
)

type headlessWindows struct {
	*Windows
	sessions []*headless.Session
	failOpen bool
}

func newHeadlessWindows(f *uiFixture) *headlessWindows {
	f.t.Helper()
	settings := f.ws.Settings
	settings.AutoUpdateCheck = false
	group := &headlessWindows{}
	group.Windows = &Windows{
		workspace: func(source *vapp.Workspace, invalidate func()) *vapp.Workspace {
			runner := &gitcli.Runner{Env: f.env}
			if source == nil {
				return vapp.NewWorkspace(runner, settings, invalidate)
			}
			return vapp.NewSiblingWorkspace(runner, source, invalidate)
		},
		openNative: func(cfg app.Config, canvas *ui.Canvas) (*app.Window, error) {
			if group.failOpen {
				return nil, errors.New("simulated window creation failure")
			}
			s, err := headless.Open(cfg, canvas, headless.Options{Scale: 2})
			if err == nil {
				group.sessions = append(group.sessions, s)
				f.t.Cleanup(s.Close)
			}
			return nil, err
		},
	}
	return group
}

func TestRepositoryWindowsPreserveIndependentSessions(t *testing.T) {
	first := newUIFixture(t)
	secondDir := filepath.Join(filepath.Dir(first.dir), "second-project")
	if err := os.Mkdir(secondDir, 0o755); err != nil {
		t.Fatal(err)
	}
	first.gitIn(secondDir, "init", "-q", "-b", "second")
	if err := os.WriteFile(filepath.Join(secondDir, "notes.txt"), []byte("Second repository\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first.gitIn(secondDir, "add", "notes.txt")
	first.gitIn(secondDir, "-c", "user.name=Viff Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "Second project")

	windows := newHeadlessWindows(first)
	win, err := windows.Open()
	if err != nil {
		t.Fatal(err)
	}
	first.win, first.ws, first.s = win, win.ws, windows.sessions[0]
	win.Start(first.dir)
	first.settle()
	firstTab := openEditableFile(t, first, "config.json")
	first.click("Edit\t⌘E")
	waitEditing(t, first, firstTab)
	if err := first.s.Type("unsaved in first window"); err != nil {
		t.Fatal(err)
	}
	const draft = "first window draft"
	setText(win.cm.msg, draft)

	var picked string
	win.openDirectory = func(app.OpenDirectoryOptions) (string, error) { return picked, nil }
	openMenu := func() {
		first.click(win.t(i18n.ChromeA11yProject, first.ws.Repo.Repo.Name()))
	}
	openSibling := func() { first.click(win.t(i18n.ChromeMenuOpenInNewWindow)) }
	assertFirst := func() {
		t.Helper()
		if first.ws.Repo == nil || first.ws.Repo.Repo.Root != first.dir || win.dirtyEdit() != firstTab || win.cm.msg.String() != draft {
			t.Fatal("first window lost its repository, dirty editor, or commit draft")
		}
	}

	first.snap("01-first-window")
	openMenu()
	first.snap("02-open-in-new-window")
	openSibling() // Cancelled picker.
	if len(windows.windows) != 1 {
		t.Fatal("cancelling the picker created a window")
	}
	assertFirst()

	picked = secondDir
	windows.failOpen = true
	openMenu()
	openSibling()
	if len(windows.windows) != 1 || !first.s.HasText(win.t(i18n.ChromeCannotOpenNewWindow)) {
		t.Fatal("window creation failure did not leave one window with an error notice")
	}
	assertFirst()

	windows.failOpen = false
	openMenu()
	openSibling()
	if len(windows.windows) != 2 {
		t.Fatalf("windows = %d, want 2", len(windows.windows))
	}
	secondWin := windows.windows[1]
	second := &uiFixture{t: t, dir: secondDir, ws: secondWin.ws, win: secondWin, s: windows.sessions[1], shot: first.shot}
	second.settle()
	assertFirst()
	if second.ws.Repo.Repo.Root != secondDir || secondWin.ed.cur() != nil || secondWin.cm.msg.String() != "" {
		t.Fatal("second window did not open its own repository with a fresh editor and draft")
	}
	second.snap("03-second-window")
	first.snap("04-first-window-still-open")
	second.s.Close()
	if len(windows.windows) != 1 || windows.primary() != win || second.ws.Repo != nil {
		t.Fatal("closing the second window did not dispose its workspace")
	}
	assertFirst()

	openMenu()
	openSibling()
	if len(windows.windows) != 2 {
		t.Fatalf("reopened windows = %d, want 2", len(windows.windows))
	}
	secondWin = windows.windows[1]
	second = &uiFixture{t: t, dir: secondDir, ws: secondWin.ws, win: secondWin, s: windows.sessions[2]}
	second.settle()
	first.s.Close()
	if windows.primary() != secondWin || first.ws.Repo != nil || second.ws.Repo.Repo.Root != secondDir {
		t.Fatal("closing the first window did not leave the second window active")
	}
}

func TestRepositoryWindowsQuitChecksSecondEditor(t *testing.T) {
	f := newUIFixture(t)
	windows := newHeadlessWindows(f)
	first, err := windows.Open()
	if err != nil {
		t.Fatal(err)
	}
	second, err := windows.open(first)
	if err != nil {
		t.Fatal(err)
	}
	f.win, f.ws, f.s = second, second.ws, windows.sessions[1]
	second.Start(f.dir)
	f.settle()
	tab := openEditableFile(t, f, "config.json")
	f.click("Edit\t⌘E")
	waitEditing(t, f, tab)
	if err := f.s.Type("unsaved in second window"); err != nil {
		t.Fatal(err)
	}
	allowed := false
	quit := func() { allowed = true }
	windows.ConfirmQuit(quit)
	if allowed || first.dialog != nil || second.dialog == nil || second.dialog.title != second.t(i18n.ChromeDiscardWindowsTitle) {
		t.Fatal("quit did not ask for confirmation in the second window")
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click("Cancel")
	if allowed || second.dirtyEdit() != tab {
		t.Fatal("cancelling quit did not preserve the dirty editor")
	}
	windows.ConfirmQuit(quit)
	f.s.Settle(40*time.Millisecond, time.Second)
	f.click(second.t(i18n.FileviewDiscardButton))
	if !allowed {
		t.Fatal("confirming quit did not run the quit action")
	}
}
