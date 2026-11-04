package viffui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/platform"
	"viff/internal/version"
)

// Diagnostics entry points behind Help > Report a Problem and Save
// Diagnostics. A recording raises the log level for the reproduction window;
// an export freezes that window (or the last 30 minutes without one) into a
// zip the user attaches to a report. Nothing is uploaded: the bundle is
// written only where the user chooses.
//
// UI copy for these flows lives in docs/observability.md §9.

// textareaField is a multi-line input painted inside a dialog body. It keeps
// its editor and layout between paints, like the commit message editor.
type textareaField struct {
	ed          *text.Editor
	layout      components.TextareaLayout
	label       string
	placeholder string
}

// paint draws the field and routes scrolling, keys and text input to it.
func (t *textareaField) paint(w *Window, id ui.ID, r ui.Rect) {
	value := t.ed.String()
	sel := t.ed.Selection()
	ss, se := runeToByte(value, min(sel.Start, sel.End)), runeToByte(value, max(sel.Start, sel.End))
	components.PaintTextarea(w.c, w.theme, r, components.TextareaSnapshot{
		ID: id, Label: t.label, Value: value, Placeholder: t.placeholder,
		ShowCaret: true, HideEditorText: true, Layout: &t.layout,
		Caret: runeToByte(value, t.ed.Caret()), SelectionStart: ss, SelectionEnd: se,
	})
	w.editors[id] = t.ed
	w.onScrollIn(r, func(dx, dy float32) { t.layout.ScrollBy(dy) })
	w.onKeys(id, func(e ui.KeyEvent) bool {
		mv, extend := components.TextareaMovementForKey(e.Key, e.Modifiers)
		if mv == components.TextareaMoveNone || mv == components.TextareaMoveLineStart || mv == components.TextareaMoveLineEnd {
			return false
		}
		val := t.ed.String()
		nb := t.layout.Move(val, runeToByte(val, t.ed.Caret()), mv)
		t.ed.MoveTo(byteToRune(val, nb), extend)
		return true
	})
}

// openReportProblem asks what went wrong and starts a diagnostics recording.
// The description is optional and is kept for the issue template.
func (w *Window) openReportProblem() {
	if w.ws.ProblemRecording() {
		w.ws.Notify(vapp.NoticeInfo, "Diagnostics",
			"A problem is already being recorded. Reproduce it, then use Help > Stop Recording and Save Diagnostics…")
		return
	}
	desc := &textareaField{
		ed:          text.NewEditor(""),
		label:       "What happened?",
		placeholder: "What were you doing when the problem appeared? (optional)",
	}
	w.openDialog(&dialog{
		title: "Report a Problem", ok: "Start Recording", width: 560,
		textarea: idReportProblemText,
		focus:    idReportProblemText,
		body: func(f *form) {
			f.text("Start a detailed diagnostics recording, reproduce the problem, then save a bundle to attach to your report.")
			f.muted("Diagnostics stay on this computer unless you save and share them. No repository contents are included.")
			f.gap(8)
			f.custom(120, func(r ui.Rect) { desc.paint(w, idReportProblemText, r) })
			f.gap(2)
			f.muted("The description is optional. Recording keeps going until you stop it from the Help menu.")
		},
		onOK: func() bool {
			description := strings.TrimSpace(desc.ed.String())
			if err := w.ws.StartProblemRecording(description); err != nil {
				w.ws.NotifyError("Cannot start diagnostics recording", err)
				return false
			}
			w.diagDesc = description
			w.ws.Notify(vapp.NoticeInfo, "Diagnostics",
				"Recording started. Reproduce the problem, then use Help > Stop Recording and Save Diagnostics…")
			return true
		},
	})
}

// stopRecordingAndSave ends the active recording and exports its window.
func (w *Window) stopRecordingAndSave() {
	w.ws.StopProblemRecording()
	w.exportDiagnostics()
}

// saveDiagnostics exports the last 30 minutes without a recording.
func (w *Window) saveDiagnostics() {
	w.exportDiagnostics()
}

// exportDiagnostics asks where to write the bundle, writes it and reports the
// outcome. Cancelling the save dialog is not an error.
func (w *Window) exportDiagnostics() {
	name := "viff-diagnostics-" + time.Now().Format("20060102-150405") + ".zip"
	dest, err := app.SaveFile(app.SaveFileOptions{
		Title:            "Save Diagnostics",
		SuggestedName:    name,
		DefaultExtension: "zip",
		Filters:          []app.FileFilter{{Name: "Zip archives", Pattern: "*.zip"}},
	})
	if err != nil {
		if !errors.Is(err, app.ErrCanceled) {
			w.ws.NotifyError("Cannot choose where to save diagnostics", err)
		}
		return
	}
	if dest == "" {
		return
	}
	res, err := w.ws.ExportDiagnostics(dest)
	if err != nil {
		w.ws.NotifyError("Cannot save diagnostics", err)
		return
	}
	w.notifyDiagnosticsSaved(res)
}

// notifyDiagnosticsSaved tells the user where the bundle is and offers the
// issue template on the clipboard.
func (w *Window) notifyDiagnosticsSaved(res vapp.DiagnosticResult) {
	body := "Attach this file to your report.\n" + res.Path
	w.ws.Notify(vapp.NoticeInfo, "Diagnostics saved", body,
		vapp.NoticeAction{Title: "Copy Issue Template", Run: func() { w.copyText(w.issueTemplate(res)) }})
}

// issueTemplate is the text to paste into a report: the problem description,
// the build environment and how to attach the bundle.
func (w *Window) issueTemplate(res vapp.DiagnosticResult) string {
	var b strings.Builder
	b.WriteString("### What happened\n\n")
	if desc := strings.TrimSpace(w.diagDesc); desc != "" {
		b.WriteString(desc)
	} else {
		b.WriteString("<!-- Describe the problem and what you did just before it appeared. -->")
	}
	b.WriteString("\n\n### Environment\n\n")
	fmt.Fprintf(&b, "- Viff: %s\n", version.String())
	fmt.Fprintf(&b, "- Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- Go: %s\n", version.GoVersion())
	fmt.Fprintf(&b, "- Viva: %s\n", version.VivaVersion())
	if git := w.gitVersion(); git != "" {
		fmt.Fprintf(&b, "- Git: %s\n", git)
	}
	b.WriteString("\n### Diagnostics\n\n")
	if res.Path != "" {
		fmt.Fprintf(&b, "Attached: `%s` (%d files, %s)\n", filepath.Base(res.Path), res.Files, humanBytes(res.Bytes))
	} else {
		b.WriteString("Attached: `viff-diagnostics-*.zip`\n")
	}
	b.WriteString("\nThe bundle contains a redacted event log, an environment summary and recent Git command output. It does not include repository contents, remotes or credentials.")
	return b.String()
}

// openLogFolder reveals the directory the application logs are written to.
func (w *Window) openLogFolder() {
	dir := strings.TrimSpace(w.ws.LogDirectory())
	if dir == "" {
		w.ws.Notify(vapp.NoticeInfo, "Logs", "This build writes no log files, so there is no log folder to open.")
		return
	}
	if err := platform.Reveal(dir); err != nil {
		w.ws.NotifyError("Cannot open log folder", err)
	}
}

// gitVersion is the Git version Viff is using, or "" when Git is missing.
func (w *Window) gitVersion() string {
	v, err := w.ws.Git.Version(context.Background())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

// humanBytes formats a byte count for a notification.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
