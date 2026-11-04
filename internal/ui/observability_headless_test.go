//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	vapp "viff/internal/app"
	"viff/internal/observability"
	"viff/internal/version"
)

// TestObservabilityHelpMenu opens the toolbar Help menu headlessly and checks
// the diagnostics entry points and the version line in About.
func TestObservabilityHelpMenu(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	// The Help button lives next to Settings and opens the diagnostics menu.
	f.click("Help")
	if len(f.win.popups) != 1 {
		t.Fatalf("Help did not open one menu: %d", len(f.win.popups))
	}
	titles := map[string]bool{}
	for _, it := range f.win.popups[0].items {
		if it.title != "" {
			titles[it.title] = true
		}
	}
	for _, want := range []string{"Report a Problem…", "Save Diagnostics…", "Open Log Folder", "About Viff"} {
		if !titles[want] {
			t.Errorf("Help menu is missing %q; items: %v", want, titles)
		}
	}
	if titles["Stop Recording and Save Diagnostics…"] {
		t.Error("Help menu offers to stop a recording that was never started")
	}
	f.snap("help-menu")

	// About repeats the build identity internal/version reports.
	f.click("About Viff")
	if f.win.dialog == nil || f.win.dialog.title != "About Viff" {
		t.Fatalf("About dialog did not open: %+v", f.win.dialog)
	}
	f.win.Invalidate()
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if !f.s.HasText("Viff " + version.String()) {
		t.Errorf("About does not show %q: %s", "Viff "+version.String(), strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("Viva") {
		t.Error("About does not show the Viva version")
	}
	f.snap("about")
}

// TestObservabilityConsoleCopy checks the Console's new Copy All action and the
// duration shown at the start of a command row.
func TestObservabilityConsoleCopy(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	f.ws.Console = []vapp.ConsoleEntry{{
		Time:    time.Date(2026, 11, 4, 14, 3, 22, 0, time.Local),
		Command: "git push",
		Output:  "first line\nsecond line\n",
		Took:    1500 * time.Millisecond,
	}}
	f.win.gitTab = 1
	f.win.Invalidate()
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if !f.s.HasText("(1.5s)") {
		t.Errorf("console row hides the command duration: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.click("Copy All")
	got := f.s.Driver().Clipboard()
	want := "14:03:22  git push  (1.5s)\n          first line\n          second line\n"
	if got != want {
		t.Errorf("Copy All clipboard = %q, want %q", got, want)
	}
}

// TestObservabilityProblemRecording starts a recording through Help > Report a
// Problem, checks the quiet status marker and stops it from the Help menu to
// save a bundle.
func TestObservabilityProblemRecording(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	log, err := observability.New(observability.Options{Dir: filepath.Join(t.TempDir(), "logs")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)
	f.ws.SetRecorder(log, log)

	f.click("Help")
	f.click("Report a Problem…")
	if f.win.dialog == nil || f.win.dialog.title != "Report a Problem" {
		t.Fatalf("Report a Problem dialog did not open: %+v", f.win.dialog)
	}
	f.win.Invalidate()
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	f.snap("report-problem-dialog")
	if err := f.s.Type("Pushing a branch fails with a rejected error"); err != nil {
		t.Fatal(err)
	}
	f.click("Start Recording")
	if !f.ws.ProblemRecording() {
		t.Fatal("Start Recording did not begin a problem recording")
	}
	if err := f.s.Settle(60*time.Millisecond, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if !f.s.HasText("Recording diagnostics") {
		t.Errorf("status bar does not show the recording marker: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("problem-recording")

	// While recording, the Help menu offers to stop and save.
	f.click("Help")
	stop := false
	for _, it := range f.win.popups[0].items {
		stop = stop || it.title == "Stop Recording and Save Diagnostics…"
	}
	if !stop {
		t.Error("Help menu does not offer to stop the recording")
	}
	f.s.Keys("escape")
	if err := f.s.Settle(40*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "viff-diagnostics.zip")
	if err := f.s.Driver().QueueSaveFile(dest, nil); err != nil {
		t.Fatal(err)
	}
	f.click("Help")
	f.click("Stop Recording and Save Diagnostics…")
	if err := f.s.Settle(60*time.Millisecond, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if f.ws.ProblemRecording() {
		t.Error("the recording is still active after Stop Recording and Save")
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("Stop Recording and Save wrote no bundle: %v", err)
	}
	if !f.s.HasText("Diagnostics saved") {
		t.Errorf("no success notification; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
}

// TestObservabilitySaveDiagnostics exports a bundle through Help > Save
// Diagnostics and checks the notification's issue-template action.
func TestObservabilitySaveDiagnostics(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	dest := filepath.Join(t.TempDir(), "viff-diagnostics.zip")
	if err := f.s.Driver().QueueSaveFile(dest, nil); err != nil {
		t.Fatal(err)
	}
	f.click("Help")
	f.click("Save Diagnostics…")
	if err := f.s.Settle(60*time.Millisecond, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("Save Diagnostics wrote no bundle: %v", err)
	}
	if !f.s.HasText("Diagnostics saved") {
		t.Fatalf("no success notification; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	f.snap("diagnostics-saved")

	f.click("Copy Issue Template")
	template := f.s.Driver().Clipboard()
	for _, want := range []string{"### What happened", "### Environment", "- Viff: " + version.String(), "### Diagnostics"} {
		if !strings.Contains(template, want) {
			t.Errorf("issue template is missing %q:\n%s", want, template)
		}
	}
}

// TestObservabilityCrashNotice checks the warning main shows after a crash and
// that its Report action opens the recording dialog.
func TestObservabilityCrashNotice(t *testing.T) {
	f := newUIFixture(t)
	f.start(1280, 800)
	f.win.openRepo(f.dir)
	f.settle()

	const crash = "/tmp/viff-last-crash-20261104.txt"
	f.win.ShowCrashNotice(crash)
	f.win.Invalidate()
	if err := f.s.Settle(60*time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}
	if !f.s.HasText("Viff closed unexpectedly last time") {
		t.Fatalf("no crash warning painted; texts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText(crash) {
		t.Errorf("the crash notice does not name the crash file")
	}
	f.snap("crash-notice")

	f.click("Report a Problem…")
	if f.win.dialog == nil || f.win.dialog.title != "Report a Problem" {
		t.Fatalf("the crash notice's Report action did not open the dialog: %+v", f.win.dialog)
	}
}
