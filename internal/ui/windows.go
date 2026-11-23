package viffui

import (
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/i18n"
)

// Windows owns Viff's desktop windows on the native event-loop thread.
// Each window has its own canvas and workspace. The first live window also
// hosts the process-wide updater.
type Windows struct {
	openNative func(app.Config, *ui.Canvas) (*app.Window, error)
	quit       func()
	workspace  func(source *vapp.Workspace, invalidate func()) *vapp.Workspace
	updates    vapp.UpdateService
	windows    []*Window
}

func NewWindows(application *app.Application, workspace func(*vapp.Workspace, func()) *vapp.Workspace, updates vapp.UpdateService) *Windows {
	return &Windows{openNative: application.OpenWindow, quit: application.Quit, workspace: workspace, updates: updates}
}

func (d *Windows) primary() *Window {
	if len(d.windows) == 0 {
		return nil
	}
	return d.windows[0]
}

// Open presents the startup window. Start loads the initial repository.
func (d *Windows) Open() (*Window, error) {
	return d.open(nil)
}

func (d *Windows) open(from *Window) (*Window, error) {
	var source *vapp.Workspace
	if primary := d.primary(); primary != nil {
		source = primary.ws
	}

	var w *Window
	ws := d.workspace(source, func() {
		if w != nil {
			w.Invalidate()
		}
	})
	w = New(ws)
	w.windows = d
	w.secondary = source != nil
	cfg := w.Config()
	if from != nil {
		if frame, ok := from.native.Frame(); ok {
			cfg.Origin = &ui.Point{X: frame.X + 24, Y: frame.Y + 24}
		}
	}
	native, err := d.openNative(cfg, &ui.Canvas{})
	if err != nil {
		if w.saveTimer != nil {
			w.saveTimer.Stop()
		}
		ws.Dispose()
		return nil, err
	}
	w.native = native
	d.windows = append(d.windows, w)
	if source == nil {
		ws.SetUpdateService(d.updates)
	}
	return w, nil
}

func (d *Windows) closed(w *Window) {
	for i, candidate := range d.windows {
		if candidate != w {
			continue
		}
		copy(d.windows[i:], d.windows[i+1:])
		d.windows[len(d.windows)-1] = nil
		d.windows = d.windows[:len(d.windows)-1]
		if i == 0 && len(d.windows) > 0 {
			d.windows[0].ws.SetUpdateService(d.updates)
		}
		return
	}
}

// Show brings a live window forward from the status item or macOS Dock.
func (d *Windows) Show() {
	if n := len(d.windows); n > 0 {
		d.windows[n-1].show()
	}
}

// ConfirmQuit applies the same unsaved-editor protection to every window.
func (d *Windows) ConfirmQuit(allow func()) {
	for _, w := range d.windows {
		if w.dirtyEdit() == nil {
			continue
		}
		w.show()
		w.confirm(w.t(i18n.ChromeDiscardWindowsTitle), w.t(i18n.ChromeDiscardWindowsBody),
			w.t(i18n.FileviewDiscardButton), allow)
		w.Invalidate()
		return
	}
	allow()
}

// TrayMenu is the application's status-item menu.
func (d *Windows) TrayMenu() []app.MenuItem {
	return []app.MenuItem{
		{Title: "Show Viff", OnSelect: d.Show},
		{Separator: true},
		{Title: "Quit Viff", OnSelect: func() { d.ConfirmQuit(d.quit) }},
	}
}

func (w *Window) show() {
	if w.native != nil {
		w.native.Show()
		w.Invalidate()
	}
}

func (w *Window) chooseRepoInNewWindow() {
	dir, err := w.openDirectory(app.OpenDirectoryOptions{Title: w.t(i18n.ChromeDialogOpenRepository)})
	if err != nil {
		w.ws.NotifyError(w.t(i18n.ChromeCannotOpenNewWindow), err)
		return
	}
	if dir == "" {
		return
	}
	next, err := w.windows.open(w)
	if err != nil {
		w.ws.NotifyError(w.t(i18n.ChromeCannotOpenNewWindow), err)
		return
	}
	next.Start(dir)
}

func (w *Window) updateWindow() *Window {
	if w.windows != nil {
		return w.windows.primary()
	}
	return w
}
