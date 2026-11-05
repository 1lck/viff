package viffui

import (
	"fmt"
	"strings"

	vapp "viff/internal/app"
	"viff/internal/platform"
)

// checkForUpdates starts a manual update check and opens the update dialog.
// The dialog renders the state the check posts; it never performs the network
// request itself.
func (w *Window) checkForUpdates() {
	w.ws.CheckForUpdates(true)
	w.openUpdateDialog()
}

// openUpdateDialog shows the in-app update flow. Its body and action label are
// read from the workspace on every paint, so downloading and installing
// progress appear without reopening the dialog.
func (w *Window) openUpdateDialog() {
	w.openDialog(&dialog{
		title:      "Update Viff",
		width:      560,
		ok:         "Close",
		cancel:     "Close",
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
		return "Update"
	case vapp.UpdateDownloading:
		return "Downloading…"
	case vapp.UpdateReady:
		return "Restart to Update"
	case vapp.UpdateInstalling:
		return "Installing…"
	case vapp.UpdateManual:
		return "Show Download"
	case vapp.UpdateError:
		if w.ws.HasDownloadedUpdate() {
			return "Retry Install"
		}
		if w.ws.HasUpdate() {
			return "Retry Download"
		}
		return "Check Again"
	case vapp.UpdateChecking:
		return "Checking…"
	}
	return "Close"
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
		w.ws.InstallUpdate()
		return false
	case vapp.UpdateManual:
		if state.ManualPath != "" {
			_ = platform.Reveal(state.ManualPath)
		}
		return false
	case vapp.UpdateError:
		if w.ws.HasDownloadedUpdate() {
			w.ws.InstallUpdate()
		} else if w.ws.HasUpdate() {
			w.ws.DownloadUpdate()
		} else {
			w.ws.CheckForUpdates(true)
		}
		return false
	}
	return true
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
		f.bold("Checking for updates…")
		f.muted("Viff " + state.Current)
	case vapp.UpdateUpToDate:
		f.bold("Viff is up to date.")
		f.muted("Version " + state.Current + " is the newest release.")
	case vapp.UpdateAvailable:
		f.bold("Viff " + state.Version + " is available.")
		f.muted("You have " + state.Current + ".")
		if notes := updateNotes(state.Notes); notes != "" {
			f.gap(6)
			f.muted("Release notes")
			f.text(notes)
		}
	case vapp.UpdateDownloading:
		f.bold("Downloading Viff " + state.Version + "…")
		if state.Total > 0 {
			percent := state.Progress * 100 / state.Total
			f.muted(fmt.Sprintf("%d%% · %s of %s", percent, humanBytes(state.Progress), humanBytes(state.Total)))
		} else {
			f.muted(humanBytes(state.Progress) + " downloaded")
		}
	case vapp.UpdateReady:
		f.bold("Viff " + state.Version + " has been downloaded.")
		f.muted("Restart Viff to finish the update. Your repositories and settings are kept.")
	case vapp.UpdateInstalling:
		f.bold("Installing update…")
		f.muted("Viff will close and start the new version automatically.")
	case vapp.UpdateManual:
		f.bold("Viff " + state.Version + " was downloaded.")
		if state.Message != "" {
			f.text(state.Message)
		}
		if state.ManualPath != "" {
			f.muted("Package: " + state.ManualPath)
		}
		f.muted("Install it with your package manager, then start Viff again.")
	case vapp.UpdateError:
		f.bold("Update failed.")
		if state.Message != "" {
			f.text(state.Message)
		}
		f.muted("You can retry, or download the release from the project page.")
	default:
		f.bold("Check for updates")
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
