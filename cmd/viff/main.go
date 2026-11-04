// Command viff is a desktop Git client in the style of IntelliJ IDEA.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/observability"
	"viff/internal/platform"
	"viff/internal/storage"
	viffui "viff/internal/ui"
	"viff/internal/version"
)

func main() {
	os.Exit(run())
}

// run is the whole process. It is separate from main so every exit path can
// flush diagnostics and show a system dialog when the window cannot start:
// a GUI process started from Finder or Explorer has no visible stderr.
func run() (code int) {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(version.String())
		return 0
	}
	started := time.Now()
	settingsPath := storage.DefaultPath()
	settings := storage.Load(settingsPath)

	logDir := filepath.Join(filepath.Dir(settingsPath), "logs")
	crashDir := observability.CrashDir(logDir)
	level := observability.ParseLevel(settings.LogLevel)
	if envLevel := os.Getenv("VIFF_LOG"); envLevel != "" {
		level = observability.ParseLevel(envLevel)
	}

	var rec observability.Recorder = observability.Nop{}
	var incidents observability.IncidentController = observability.Nop{}
	var log *observability.Log
	var logErr error
	if l, err := observability.New(observability.Options{Dir: logDir, Level: level}); err == nil {
		log, rec, incidents = l, l, l
	} else {
		logErr = err
	}
	defer func() {
		if log != nil {
			rec.Event(observability.Event{
				Event: observability.EventAppStop,
				Level: observability.LevelInfo,
				Fields: map[string]any{
					"reason":    "exit",
					"uptime_ms": time.Since(started).Milliseconds(),
					"dropped":   log.Dropped(),
				},
			})
			log.Flush()
			log.Close()
		}
	}()

	// Panics at the framework boundary (AppKit/Win32 callbacks) are recovered
	// by Viva; without a handler they vanish without a trace.
	app.SetFaultHandler(func(f app.Fault) {
		path, _ := observability.WriteCrash(crashDir, observability.Crash{
			Where:   f.Where,
			Message: f.Message,
			Stack:   f.Stack,
			Context: rec.Snapshot(time.Time{}, 200),
		})
		rec.Event(observability.Event{
			Event:  observability.EventFault,
			Level:  observability.LevelError,
			Fields: map[string]any{"where": f.Where, "msg": f.Message, "crash": filepath.Base(path)},
		})
		rec.Flush()
	})

	// A panic in the window thread or a background goroutine is fatal for the
	// process; write the crash file and tell the user where it is.
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			path, _ := observability.WriteCrash(crashDir, observability.Crash{
				Where:   "main",
				Message: fmt.Sprint(r),
				Stack:   stack,
				Context: rec.Snapshot(time.Time{}, 200),
			})
			rec.Event(observability.Event{
				Event:  observability.EventPanic,
				Level:  observability.LevelError,
				Fields: map[string]any{"where": "main", "msg": fmt.Sprint(r), "crash": filepath.Base(path)},
			})
			rec.Flush()
			app.ShowFatalError("Viff crashed",
				fmt.Sprintf("Viff stopped because of an internal error.\n\nA crash report was saved to:\n%s\n\nPlease report it at https://github.com/1lck/viff/issues and attach the crash report.", path))
			code = 1
		}
	}()

	settingsLevel := observability.LevelInfo
	settingsFields := map[string]any{"schema": settings.Version, "theme": settings.Theme}
	if settings.Warning != "" {
		settingsLevel = observability.LevelWarn
		settingsFields["warning"] = settings.Warning
	}
	rec.Event(observability.Event{Event: observability.EventSettingsLoad, Level: settingsLevel, Fields: settingsFields})

	git := &gitcli.Runner{Path: settings.GitPath}
	gitCtx, cancelGit := context.WithTimeout(context.Background(), 2*time.Second)
	gitVersion, gitVersionErr := git.Version(gitCtx)
	cancelGit()
	gitSource := "path"
	if settings.GitPath != "" {
		gitSource = "settings"
	}
	if gitVersionErr != nil {
		gitSource = "missing"
		gitVersion = ""
	}
	rec.Event(observability.Event{
		Event: observability.EventAppStart,
		Level: observability.LevelInfo,
		Fields: map[string]any{
			"version":         version.Version,
			"commit":          version.Commit,
			"build_date":      version.BuildDate,
			"go":              runtime.Version(),
			"viva":            version.VivaVersion(),
			"os":              runtime.GOOS,
			"arch":            runtime.GOARCH,
			"settings_schema": storage.SchemaVersion,
			"theme":           settings.Theme,
			"scale":           settings.UIScale,
			"log_level":       level.String(),
			"log_dir":         filepath.Join("<cfg>", "Viff", "logs"),
			"git_version":     gitVersion,
			"git_source":      gitSource,
		},
	})

	var win *viffui.Window
	ws := vapp.NewWorkspace(git, settings, func() {
		if win != nil {
			win.Invalidate()
		}
	})
	ws.SetRecorder(rec, incidents)
	ws.SetLogLevel(settings.LogLevel)
	ws.Watch = func(repo domain.Repo, changed func(meta bool)) (vapp.Watcher, error) {
		return platform.Watch(repo, changed)
	}
	win = viffui.New(ws)

	if logErr != nil {
		ws.Notify(vapp.NoticeWarning, "Logging disabled", "Viff cannot write its diagnostics log: "+logErr.Error())
	}

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

	if crash, ok := observability.PreviousCrash(crashDir, 7*24*time.Hour); ok {
		rec.Event(observability.Event{
			Event:  observability.EventAppRecover,
			Level:  observability.LevelWarn,
			Fields: map[string]any{"crash": filepath.Base(crash)},
		})
		win.ShowCrashNotice(crash)
	}

	// RunCanvas, not Run: Run opens a plain native window and ignores Paint
	// and the event handlers.
	if err := app.RunCanvas(win.Config(), &ui.Canvas{}); err != nil {
		rec.Event(observability.Event{
			Event:  observability.EventAppFatal,
			Level:  observability.LevelError,
			Fields: map[string]any{"phase": "window", "msg": err.Error()},
		})
		fmt.Fprintln(os.Stderr, "viff:", err)
		app.ShowFatalError("Viff cannot start", fmt.Sprintf("%s\n\nLogs: %s", err, logDir))
		return 1
	}
	return 0
}
