package viffui

import (
	"strings"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/i18n"
	"viff/internal/platform"
)

// checkForUpdates starts a manual update check and opens the update dialog.
// The dialog renders the state the check posts; it never performs the network
// request itself.
func (w *Window) checkForUpdates() {
	owner := w.updateWindow()
	if owner != w {
		owner.show()
	}
	owner.ws.CheckForUpdates(true)
	owner.openUpdateDialog()
	owner.Invalidate()
}

// openUpdateDialog shows the in-app update flow. Its body and action label are
// read from the workspace on every paint, so downloading and installing
// progress appear without reopening the dialog.
func (w *Window) openUpdateDialog() {
	w.openDialog(&dialog{
		title:      w.t(i18n.UpdateTitle),
		width:      560,
		ok:         w.t(i18n.UpdateButtonClose),
		cancel:     w.t(i18n.UpdateButtonClose),
		okLabel:    w.updateActionLabel,
		okDisabled: w.updateActionDisabled,
		onOK:       w.submitUpdateAction,
		onCancel:   w.cancelUpdateDownload,
		body:       w.paintUpdate,
	})
}

// updateActionLabel is the primary button for the current phase.
func (w *Window) updateActionLabel() string {
	switch w.ws.Update.Phase {
	case vapp.UpdateAvailable:
		return w.t(i18n.UpdateActionUpdate)
	case vapp.UpdateDownloading:
		return w.t(i18n.UpdateActionDownloading)
	case vapp.UpdateReady:
		return w.t(i18n.UpdateActionRestart)
	case vapp.UpdateInstalling:
		return w.t(i18n.UpdateActionInstalling)
	case vapp.UpdateManual:
		return w.t(i18n.UpdateActionShowDownload)
	case vapp.UpdateError:
		if w.ws.HasDownloadedUpdate() {
			return w.t(i18n.UpdateActionRetryInstall)
		}
		if w.ws.HasUpdate() {
			return w.t(i18n.UpdateActionRetryDownload)
		}
		return w.t(i18n.UpdateActionCheckAgain)
	case vapp.UpdateChecking:
		return w.t(i18n.UpdateActionChecking)
	}
	return w.t(i18n.UpdateButtonClose)
}

// updateActionDisabled reports whether the primary button is a no-op spinner.
func (w *Window) updateActionDisabled() bool {
	switch w.ws.Update.Phase {
	case vapp.UpdateChecking, vapp.UpdateDownloading, vapp.UpdateInstalling:
		return true
	}
	return false
}

// submitUpdateAction advances the flow. It returns false to keep the dialog
// open while the background operation runs.
func (w *Window) submitUpdateAction() bool {
	state := w.ws.Update
	switch state.Phase {
	case vapp.UpdateAvailable:
		w.ws.DownloadUpdate()
		return false
	case vapp.UpdateReady:
		w.installUpdate()
		return false
	case vapp.UpdateManual:
		if state.ManualPath != "" {
			_ = platform.Reveal(state.ManualPath)
		}
		return false
	case vapp.UpdateError:
		if w.ws.HasDownloadedUpdate() {
			w.installUpdate()
		} else if w.ws.HasUpdate() {
			w.ws.DownloadUpdate()
		} else {
			w.ws.CheckForUpdates(true)
		}
		return false
	}
	return true
}

func (w *Window) installUpdate() {
	install := func() {
		// The confirmation may have been shown in another window. Its action
		// is stale if the window that held the downloaded update has closed.
		if w.updateWindow() != w {
			return
		}
		w.ws.InstallUpdate()
		if w.ws.Update.Phase == vapp.UpdateInstalling {
			w.openUpdateDialog()
			if w.windows != nil {
				w.windows.invalidate()
			} else {
				w.Invalidate()
			}
		}
	}
	if w.windows != nil {
		w.windows.ConfirmQuit(install)
	} else {
		w.confirmDiscardEdits(install)
	}
}

// installingUpdate is shared by all windows. Once the helper starts, it will
// replace and relaunch Viff on exit, so editing stays paused from confirmation
// until either the application quits or the install attempt fails.
func (w *Window) installingUpdate() bool {
	owner := w.updateWindow()
	return owner != nil && owner.ws.Update.Phase == vapp.UpdateInstalling
}

func (w *Window) paintInstallingUpdate(full ui.Rect) {
	w.p.textCenter(w.t(i18n.UpdateNoticeInstalling), rect(0, full.Height/2-rowH, full.Width, rowH), fontUI, semibold, w.pal.Text)
	w.p.textCenter(w.t(i18n.UpdateNoticeInstallHint), rect(0, full.Height/2, full.Width, rowH), fontUI, regular, w.pal.TextSecond)
}

// cancelUpdateDownload cancels an in-flight download before closing. The
// dialog otherwise just closes; a known update stays available for later.
func (w *Window) cancelUpdateDownload() {
	if w.ws.Update.Phase == vapp.UpdateDownloading {
		w.ws.CancelUpdate()
	}
}

// paintUpdate renders the current update phase. Notes are shortened so a long
// release body cannot grow the dialog beyond the window.
func (w *Window) paintUpdate(f *form) {
	state := w.ws.Update
	switch state.Phase {
	case vapp.UpdateChecking:
		f.bold(w.t(i18n.UpdateNoticeChecking))
		f.muted("Viff " + state.Current)
	case vapp.UpdateUpToDate:
		f.bold(w.t(i18n.UpdateNoticeUpToDate))
		f.muted(w.t(i18n.UpdateNoticeNewest, state.Current))
	case vapp.UpdateAvailable:
		f.bold(w.t(i18n.UpdateNoticeAvailable, state.Version))
		f.muted(w.t(i18n.UpdateNoticeYouHave, state.Current))
		if notes := updateNotes(state.Notes); notes != "" {
			f.gap(6)
			f.muted(w.t(i18n.UpdateLabelReleaseNotes))
			f.text(notes)
		}
	case vapp.UpdateDownloading:
		f.bold(w.t(i18n.UpdateNoticeDownloading, state.Version))
		if state.Total > 0 {
			percent := state.Progress * 100 / state.Total
			f.muted(w.t(i18n.UpdateNoticeProgress, percent, humanBytes(state.Progress), humanBytes(state.Total)))
		} else {
			f.muted(w.t(i18n.UpdateNoticeDownloaded, humanBytes(state.Progress)))
		}
	case vapp.UpdateReady:
		f.bold(w.t(i18n.UpdateNoticeReady, state.Version))
		f.muted(w.t(i18n.UpdateNoticeRestartHint))
	case vapp.UpdateManual:
		f.bold(w.t(i18n.UpdateNoticeManualDownload, state.Version))
		if state.Message != "" {
			f.text(state.Message)
		}
		if state.ManualPath != "" {
			f.muted(w.t(i18n.UpdateLabelPackage, state.ManualPath))
		}
		f.muted(w.t(i18n.UpdateNoticeManualHint))
	case vapp.UpdateError:
		f.bold(w.t(i18n.UpdateNoticeFailed))
		if state.Message != "" {
			f.text(state.Message)
		}
		f.muted(w.t(i18n.UpdateNoticeFailedHint))
	default:
		f.bold(w.t(i18n.UpdateNoticeDefault))
		f.muted("Viff " + state.Current)
	}
}

// updateNotes shortens release notes for the dialog: at most 14 lines and
// 1600 characters, so a long body stays inside the window.
func updateNotes(notes string) string {
	const (
		maxLines = 14
		maxChars = 1600
	)
	trimmed := strings.TrimSpace(notes)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	more := len(lines) > maxLines
	if more {
		lines = lines[:maxLines]
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(text) > maxChars {
		return truncateText(text, maxChars) + "…"
	}
	if more {
		text += "\n…"
	}
	return text
}
