// Command viff is a desktop Git client in the style of IntelliJ IDEA.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/platform"
	"viff/internal/storage"
	viffui "viff/internal/ui"
)

func main() {
	platform.RegisterFonts()
	settings := storage.Load(storage.DefaultPath())
	git := &gitcli.Runner{Path: settings.GitPath}

	var win *viffui.Window
	ws := vapp.NewWorkspace(git, settings, func() {
		if win != nil {
			win.Invalidate()
		}
	})
	ws.Watch = func(repo domain.Repo, changed func(meta bool)) (vapp.Watcher, error) {
		return platform.Watch(repo, changed)
	}
	win = viffui.New(ws)

	// A path argument opens that repository; otherwise the last one, as
	// IntelliJ reopens the last project.
	dir := ""
	if len(os.Args) > 1 {
		if abs, err := filepath.Abs(os.Args[1]); err == nil {
			dir = abs
		}
	} else if len(settings.Recent) > 0 {
		if _, err := os.Stat(settings.Recent[0].Path); err == nil {
			dir = settings.Recent[0].Path
		}
	}
	win.Start(dir)

	// RunCanvas, not Run: Run opens a plain native window and ignores Paint
	// and the event handlers.
	if err := app.RunCanvas(win.Config(), &ui.Canvas{}); err != nil {
		fmt.Fprintln(os.Stderr, "viff:", err)
		os.Exit(1)
	}
}
