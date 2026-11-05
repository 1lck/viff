package viffui

import (
	vapp "viff/internal/app"
	"viff/internal/ui/icons"
)

// helpMenu is the toolbar Help menu. Reporting a problem starts a diagnostics
// recording; while one runs the menu offers to stop it and save the bundle.
// Nothing here uploads anything: the bundle is only written where the user
// chooses (docs/observability.md §8).
func (w *Window) helpMenu() []item {
	items := []item{
		{title: "Report a Problem…", icon: icons.Problems, run: w.openReportProblem},
	}
	if w.ws.ProblemRecording() {
		items = append(items, item{title: "Stop Recording and Save Diagnostics…", icon: icons.Archive, run: w.stopRecordingAndSave})
	} else {
		items = append(items, item{title: "Save Diagnostics…", icon: icons.Archive, run: w.saveDiagnostics})
	}
	title := "Check for Updates…"
	if w.ws.Update.Phase == vapp.UpdateAvailable && w.ws.Update.Version != "" {
		title = "Update Viff to " + w.ws.Update.Version + "…"
	}
	items = append(items,
		item{title: "Open Log Folder", icon: icons.Folder, run: w.openLogFolder},
		sep(),
		item{title: title, icon: icons.Update, run: w.checkForUpdates},
		sep(),
		item{title: "About Viff", icon: icons.Info, run: w.openAbout},
	)
	return items
}
