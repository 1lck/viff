package app

import (
	"context"
	"errors"
	"time"

	"viff/internal/i18n"
	"viff/internal/observability"
	"viff/internal/updater"
	"viff/internal/version"
)

// UpdatePhase is the stage of the in-app update flow shown by the dialog.
type UpdatePhase uint8

const (
	// UpdateIdle is the state before the first check.
	UpdateIdle UpdatePhase = iota
	// UpdateChecking runs the release check.
	UpdateChecking
	// UpdateUpToDate means the newest release is the running one.
	UpdateUpToDate
	// UpdateAvailable means a newer release can be downloaded.
	UpdateAvailable
	// UpdateDownloading fetches the package for the running platform.
	UpdateDownloading
	// UpdateReady means the package is downloaded and verified.
	UpdateReady
	// UpdateInstalling means the swap helper is being prepared and the
	// process is about to quit.
	UpdateInstalling
	// UpdateManual means Viff cannot replace its own installation and the
	// user has to install the downloaded package.
	UpdateManual
	// UpdateError means the last step failed; Message explains it.
	UpdateError
)

// UpdateState is the observable state of the in-app update flow. It lives on
// the Workspace so the UI renders it and never talks to the updater itself.
type UpdateState struct {
	Phase   UpdatePhase
	Current string // running version, from internal/version
	Version string // newer release tag, "" until one is known
	Notes   string // release notes of Version
	PageURL string // release page of Version

	// Progress and Total are downloaded and advertised bytes. Total is 0
	// when the release asset does not advertise a size.
	Progress int64
	Total    int64

	// Message is a user-facing explanation for Manual and Error.
	Message string
	// ManualPath is the downloaded package to install by hand (Manual).
	ManualPath string
	// CheckedAt is when the last completed check started.
	CheckedAt time.Time
	// ShouldQuit tells the UI to quit so a scheduled install can replace the
	// binary; set by InstallUpdate after the helper is launched.
	ShouldQuit bool
}

// UpdateService is the update backend the use cases need. *updater.Client
// implements it; cmd/viff wires the real one and tests use a fake.
type UpdateService interface {
	// Check returns a newer release, or nil when the running version is current.
	Check(ctx context.Context) (*updater.Update, error)
	// Download fetches the release package and returns its local path.
	// progress is called from the download goroutine.
	Download(ctx context.Context, u *updater.Update, progress func(done, total int64)) (string, error)
	// Install schedules replacing the running installation. The caller must
	// quit the process when it returns without error.
	Install(u *updater.Update, downloaded string) (*updater.InstallResult, error)
}

// updateMachine is the private state behind Workspace.Update. The UI thread
// owns all of it; background work only posts closures that apply results.
type updateMachine struct {
	svc UpdateService
	// update is the release being downloaded or installed.
	update *updater.Update
	// path is the downloaded package handed to Install.
	path string
	// cancel stops an in-flight download, if any.
	cancel context.CancelFunc
	// prompt asks the UI to open the dialog on its next wake. The notification
	// action set on an automatic check calls PromptUpdate, and the window
	// consumes the request after Drain.
	prompt bool
	// seq numbers background operations. A result whose seq is no longer
	// current is dropped, the same way repository generations drop stale
	// queries.
	seq uint64
}

const (
	// updateCheckTimeout bounds one release check. A check is a small HTTPS
	// request; 30s covers slow networks without leaving a goroutine around.
	updateCheckTimeout = 30 * time.Second
	// updateProgressEvery throttles download progress posts: the UI only
	// needs a few updates per second, not one per network chunk.
	updateProgressEvery = 100 * time.Millisecond
)

// updateUnavailableMessage explains why the dialog cannot check.
const updateUnavailableMessage = "Update checks are not available in this build."

// updateDevMessage is the readable manual-check result for development builds.
const updateDevMessage = "Update checks are disabled in development builds. Install a release build to receive updates."

// initUpdateState records the running version before the first check.
func (w *Workspace) initUpdateState() {
	w.Update = UpdateState{Current: version.Version}
}

// SetUpdateService installs the update backend. cmd/viff calls it after
// creating the workspace; a nil service means updates are unavailable and the
// UI reports that on a manual check.
func (w *Workspace) SetUpdateService(s UpdateService) {
	w.updates.svc = s
	w.Update.Current = version.Version
}

// releaseBuild reports whether this binary is a release that can check for
// updates. Development builds ("dev") must not query the release feed.
func releaseBuild() bool {
	return version.Version != "" && version.Version != "dev"
}

// CheckForUpdates queries the release feed in the background. manual checks
// report failures in the dialog; automatic ones (startup) stay silent unless
// an update is found, and then show a notification. Results are applied on the
// UI thread; a check already running or an update already downloading is
// ignored, so double clicks cannot start overlapping requests.
func (w *Workspace) CheckForUpdates(manual bool) {
	switch w.Update.Phase {
	case UpdateChecking, UpdateDownloading, UpdateInstalling:
		return
	}
	if w.updates.svc == nil {
		if manual {
			w.Update.Phase = UpdateError
			w.Update.CheckedAt = w.now()
			w.Update.Message = updateUnavailableMessage
		}
		return
	}
	if !releaseBuild() {
		if manual {
			w.Update.Phase = UpdateError
			w.Update.CheckedAt = w.now()
			w.Update.Message = updateDevMessage
		}
		return
	}

	current := version.Version
	w.updates.seq++
	seq := w.updates.seq
	w.Update.Phase = UpdateChecking
	w.Update.Current = current
	w.Update.Message = ""
	w.updates.update = nil
	w.updates.path = ""
	started := w.now()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		u, err := w.updates.svc.Check(ctx)
		w.post(func() {
			if seq != w.updates.seq {
				return // a newer operation superseded this check
			}
			w.Update.CheckedAt = w.now()
			w.Update.Current = current
			if err != nil {
				w.recordUpdateEvent(observability.EventUpdateCheck, observability.LevelWarn, started,
					map[string]any{"from": current, "manual": manual, "result": "error"})
				if manual {
					w.Update.Phase = UpdateError
					w.Update.Message = "Could not check for updates: " + updateErrorText(err)
				} else {
					// Automatic checks stay quiet: the user did not ask, so a
					// failed request must not produce a notification.
					w.Update.Phase = UpdateIdle
				}
				return
			}
			if u == nil {
				w.Update.Phase = UpdateUpToDate
				w.Update.Version, w.Update.Notes, w.Update.PageURL = "", "", ""
				w.Update.Message = ""
				w.recordUpdateEvent(observability.EventUpdateCheck, observability.LevelInfo, started,
					map[string]any{"from": current, "manual": manual, "result": "up-to-date"})
				return
			}
			w.updates.update = u
			w.Update.Phase = UpdateAvailable
			w.Update.Version, w.Update.Notes, w.Update.PageURL = u.Version, u.Notes, u.PageURL
			w.Update.Message = ""
			w.recordUpdateEvent(observability.EventUpdateCheck, observability.LevelInfo, started,
				map[string]any{"from": current, "to": u.Version, "manual": manual, "result": "available"})
			if !manual {
				// The notification is the quiet entry point: a startup check
				// must not interrupt the user with a modal dialog. Its Update
				// action opens the dialog through updatePrompt (set on the UI
				// thread by PromptUpdate).
				w.notify(NoticeInfo, w.tr(i18n.AppNoticeUpdateAvailable),
					w.tr(i18n.AppNoticeUpdateAvailableBody, u.Version, current), "",
					NoticeAction{Title: w.tr(i18n.AppActionUpdate), Run: w.PromptUpdate})
			}
		})
	}()
}

// DownloadUpdate fetches the release found by CheckForUpdates. It is callable
// from Available (a new update) and Error (retry after a failed download);
// progress is posted on the UI thread roughly ten times per second and the
// completed package makes the state Ready. Cancelling the context returns the
// state to Available instead of Error.
func (w *Workspace) DownloadUpdate() {
	if w.Update.Phase != UpdateAvailable && w.Update.Phase != UpdateError {
		return
	}
	u := w.updates.update
	if w.updates.svc == nil || u == nil {
		w.Update.Phase = UpdateError
		w.Update.Message = "There is no update to download."
		return
	}

	w.updates.seq++
	seq := w.updates.seq
	w.Update.Phase = UpdateDownloading
	w.Update.Progress = 0
	w.Update.Total = u.Asset.Size
	w.Update.Message = ""
	started := w.now()
	ctx, cancel := context.WithCancel(context.Background())
	w.updates.cancel = cancel

	go func() {
		defer cancel()
		last := time.Time{}
		progress := func(done, total int64) {
			// Throttle, but always post the final count so a finished
			// download never leaves the dialog at 99%.
			if total <= 0 || done < total {
				now := w.now()
				if now.Sub(last) < updateProgressEvery {
					return
				}
				last = now
			}
			w.post(func() {
				if seq != w.updates.seq || w.Update.Phase != UpdateDownloading {
					return
				}
				w.Update.Progress = done
				if total > 0 {
					w.Update.Total = total
				}
			})
		}
		path, err := w.updates.svc.Download(ctx, u, progress)
		w.post(func() {
			if seq != w.updates.seq {
				return
			}
			w.updates.cancel = nil
			switch {
			case err == nil && w.Update.Phase == UpdateDownloading:
				w.updates.path = path
				w.Update.Phase = UpdateReady
				if w.Update.Total > 0 {
					w.Update.Progress = w.Update.Total
				}
				w.recordUpdateEvent(observability.EventUpdateDownload, observability.LevelInfo, started,
					map[string]any{"to": u.Version, "bytes": w.Update.Progress, "result": "downloaded"})
			case errors.Is(err, context.Canceled) || w.Update.Phase != UpdateDownloading:
				// CancelUpdate already moved the state back to Available; a
				// download that succeeded after cancelling is simply unused.
				if w.Update.Phase == UpdateDownloading {
					w.Update.Phase = UpdateAvailable
					w.Update.Progress = 0
				}
				w.recordUpdateEvent(observability.EventUpdateDownload, observability.LevelInfo, started,
					map[string]any{"to": u.Version, "result": "cancelled"})
			default:
				w.Update.Phase = UpdateError
				w.Update.Message = "Could not download the update: " + updateErrorText(err)
				w.recordUpdateEvent(observability.EventUpdateDownload, observability.LevelWarn, started,
					map[string]any{"to": u.Version, "result": "error"})
			}
		})
	}()
}

// CancelUpdate stops an in-flight download and returns to Available. The
// download goroutine reports the cancellation; its result cannot override the
// state because CancelUpdate already changed it.
func (w *Workspace) CancelUpdate() {
	if w.Update.Phase != UpdateDownloading {
		return
	}
	if w.updates.cancel != nil {
		w.updates.cancel()
		w.updates.cancel = nil
	}
	w.Update.Phase = UpdateAvailable
	w.Update.Progress = 0
	w.Update.Message = ""
}

// InstallUpdate hands the downloaded package to the updater, which schedules
// the replacement and relaunch after Viff exits. On success the UI must quit
// (ShouldQuit); a *updater.ManualInstallError means the user has to install
// the package themselves (Manual with the download path). Install itself runs
// in the background; it only writes files.
func (w *Workspace) InstallUpdate() {
	if (w.Update.Phase != UpdateReady && w.Update.Phase != UpdateError) || w.updates.svc == nil {
		return
	}
	u, path := w.updates.update, w.updates.path
	if u == nil || path == "" {
		return
	}

	w.updates.seq++
	seq := w.updates.seq
	w.Update.Phase = UpdateInstalling
	w.Update.Message = ""
	started := w.now()

	go func() {
		_, err := w.updates.svc.Install(u, path)
		w.post(func() {
			if seq != w.updates.seq {
				return
			}
			var manual *updater.ManualInstallError
			switch {
			case err == nil:
				// Keep the phase Installing: the dialog stays up until the
				// window sees ShouldQuit and exits for the helper.
				w.Update.ShouldQuit = true
				w.recordUpdateEvent(observability.EventUpdateInstall, observability.LevelInfo, started,
					map[string]any{"to": u.Version, "result": "scheduled"})
			case errors.As(err, &manual):
				w.Update.Phase = UpdateManual
				w.Update.ManualPath = manual.Path
				w.Update.Message = manual.Reason
				w.recordUpdateEvent(observability.EventUpdateInstall, observability.LevelWarn, started,
					map[string]any{"to": u.Version, "result": "manual"})
			default:
				w.Update.Phase = UpdateError
				w.Update.Message = "Could not install the update: " + updateErrorText(err)
				w.recordUpdateEvent(observability.EventUpdateInstall, observability.LevelWarn, started,
					map[string]any{"to": u.Version, "result": "error"})
			}
		})
	}()
}

// PromptUpdate asks the window to open the update dialog on its next wake. It
// is the notification action, so it runs on the UI thread.
func (w *Workspace) PromptUpdate() { w.updates.prompt = true }

// ConsumeUpdatePrompt reports whether the window should open the update
// dialog, clearing the request. The window calls it after Drain.
func (w *Workspace) ConsumeUpdatePrompt() bool {
	if !w.updates.prompt {
		return false
	}
	w.updates.prompt = false
	return true
}

// DismissUpdate closes the update flow: an in-flight download is cancelled and
// stale results are dropped, leaving Idle. The version the running build
// reports is kept.
func (w *Workspace) DismissUpdate() {
	if w.updates.cancel != nil {
		w.updates.cancel()
		w.updates.cancel = nil
	}
	w.updates.seq++
	w.updates.prompt = false
	w.updates.update = nil
	w.updates.path = ""
	w.Update = UpdateState{Current: version.Version}
}

// ResetUpdate forgets everything about updates (used when the composition root
// replaces the service in tests).
func (w *Workspace) ResetUpdate() { w.DismissUpdate() }

// HasUpdate reports whether a release is known, which an Error retry needs to
// decide between downloading and re-checking.
func (w *Workspace) HasUpdate() bool { return w.updates.update != nil }

// HasDownloadedUpdate reports whether the known release already has a verified
// package on disk, so a failed install can be retried without downloading it
// again.
func (w *Workspace) HasDownloadedUpdate() bool { return w.updates.path != "" }

// recordUpdateEvent records one fact of the update flow. Fields carry only
// versions, byte counts, the manual flag, and the result; no URLs or paths.
func (w *Workspace) recordUpdateEvent(name string, level observability.Level, started time.Time, fields map[string]any) {
	w.record(observability.Event{
		Event:  name,
		Level:  level,
		Op:     name[len("update."):],
		Dur:    w.now().Sub(started),
		Fields: fields,
	})
}

// updateErrorText turns an updater error into one short user-facing line.
func updateErrorText(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "The request timed out."
	case errors.Is(err, context.Canceled):
		return "Cancelled."
	case errors.Is(err, updater.ErrUnsupported):
		return "This build cannot update itself. Download the new version from the release page."
	}
	return truncateBytes(err.Error(), 500)
}
