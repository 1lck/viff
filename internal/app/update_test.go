package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"viff/internal/gitcli"
	"viff/internal/storage"
	"viff/internal/updater"
	"viff/internal/version"
)

// fakeUpdates is a deterministic UpdateService. Tests set the result of each
// step and wait for the workspace to post it.
type fakeUpdates struct {
	release       *updater.Update
	checkErr      error
	downloadPath  string
	downloadErr   error
	blockDownload bool
	downloadDone  chan struct{}
	installResult *updater.InstallResult
	installErr    error
}

func (f *fakeUpdates) Check(context.Context) (*updater.Update, error) { return f.release, f.checkErr }

func (f *fakeUpdates) Download(ctx context.Context, u *updater.Update, progress func(done, total int64)) (string, error) {
	if f.blockDownload {
		<-ctx.Done()
		if f.downloadDone != nil {
			close(f.downloadDone)
		}
		return "", ctx.Err()
	}
	if progress != nil {
		progress(512, 2048)
	}
	return f.downloadPath, f.downloadErr
}

func (f *fakeUpdates) Install(*updater.Update, string) (*updater.InstallResult, error) {
	return f.installResult, f.installErr
}

func (f *fakeUpdates) DownloadDir() string { return "" }
func (f *fakeUpdates) Cleanup()            {}

func withVersion(t *testing.T, v string) {
	t.Helper()
	previous := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = previous })
}

func newUpdateWorkspace(t *testing.T) *Workspace {
	t.Helper()
	withVersion(t, "v0.1.0")
	return NewWorkspace(&gitcli.Runner{}, storage.Defaults(), nil)
}

// settleUpdate drains posted closures until cond holds.
func settleUpdate(t *testing.T, w *Workspace, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w.Drain()
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("update state did not settle: %+v", w.Update)
}

func testRelease() *updater.Update {
	return &updater.Update{
		Version: "v0.1.1",
		Notes:   "Fixes and improvements.",
		PageURL: "https://example.invalid/releases/v0.1.1",
		Asset: updater.Asset{
			Name: "viff-v0.1.1-darwin-arm64.dmg",
			URL:  "https://example.invalid/update.dmg",
			Size: 2048,
		},
		Checksums: map[string]string{"viff-v0.1.1-darwin-arm64.dmg": "sum"},
	}
}

func TestCheckForUpdatesFindsReleaseAndPrompts(t *testing.T) {
	w := newUpdateWorkspace(t)
	fake := &fakeUpdates{release: testRelease()}
	w.SetUpdateService(fake)

	w.CheckForUpdates(true)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateAvailable })
	if w.Update.Version != "v0.1.1" || w.Update.Current != version.Version {
		t.Fatalf("available state = %+v", w.Update)
	}
	if len(w.Notifications) != 0 {
		t.Fatal("a manual check should not raise an automatic notification")
	}

	w.PromptUpdate()
	if !w.ConsumeUpdatePrompt() {
		t.Fatal("PromptUpdate did not request the dialog")
	}
	if w.ConsumeUpdatePrompt() {
		t.Fatal("the prompt was consumed twice")
	}
}

func TestAutomaticCheckNotifiesQuietly(t *testing.T) {
	w := newUpdateWorkspace(t)
	w.SetUpdateService(&fakeUpdates{release: testRelease()})
	w.CheckForUpdates(false)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateAvailable })

	if len(w.Notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(w.Notifications))
	}
	notification := w.Notifications[0]
	if notification.Title != "Update available" || len(notification.Actions) != 1 {
		t.Fatalf("notification = %+v", notification)
	}
	if w.updates.prompt {
		t.Fatal("an automatic check must not open a dialog by itself")
	}
	notification.Actions[0].Run()
	if !w.ConsumeUpdatePrompt() {
		t.Fatal("the notification action did not request the dialog")
	}
}

func TestCheckForUpdatesUpToDate(t *testing.T) {
	w := newUpdateWorkspace(t)
	w.SetUpdateService(&fakeUpdates{})
	w.CheckForUpdates(true)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateUpToDate })
}

func TestCheckForUpdatesFailureIsSilentWhenAutomatic(t *testing.T) {
	w := newUpdateWorkspace(t)
	w.SetUpdateService(&fakeUpdates{checkErr: errors.New("offline")})
	w.CheckForUpdates(false)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateIdle })
	if w.Update.Message != "" {
		t.Fatalf("automatic failure left a message: %q", w.Update.Message)
	}

	w.CheckForUpdates(true)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateError })
	if w.Update.Message == "" {
		t.Fatal("manual failure has no message")
	}
}

func TestDevelopmentBuildSkipsCheck(t *testing.T) {
	w := newUpdateWorkspace(t)
	withVersion(t, "dev")
	fake := &fakeUpdates{release: testRelease()}
	w.SetUpdateService(fake)
	w.CheckForUpdates(true)
	if w.Update.Phase != UpdateError || w.Update.Message == "" {
		t.Fatalf("development build state = %+v", w.Update)
	}
	if w.updates.update != nil {
		t.Fatal("development build consulted the release feed")
	}
}

func TestDownloadUpdateProgressesToReady(t *testing.T) {
	w := newUpdateWorkspace(t)
	fake := &fakeUpdates{release: testRelease(), downloadPath: "/tmp/update.dmg"}
	w.SetUpdateService(fake)
	w.CheckForUpdates(true)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateAvailable })

	w.DownloadUpdate()
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateReady })
	if w.Update.Progress != w.Update.Total || w.Update.Total != 2048 {
		t.Fatalf("progress after download = %d/%d", w.Update.Progress, w.Update.Total)
	}
	if w.updates.path != "/tmp/update.dmg" {
		t.Fatalf("downloaded path = %q", w.updates.path)
	}
}

func TestCancelDownloadReturnsToAvailable(t *testing.T) {
	w := newUpdateWorkspace(t)
	fake := &fakeUpdates{release: testRelease(), blockDownload: true, downloadDone: make(chan struct{})}
	w.SetUpdateService(fake)
	w.CheckForUpdates(true)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateAvailable })

	w.DownloadUpdate()
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateDownloading })
	w.CancelUpdate()
	if w.Update.Phase != UpdateAvailable {
		t.Fatalf("state after cancel = %d", w.Update.Phase)
	}
	<-fake.downloadDone
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateAvailable })
}

func TestInstallManualKeepsDownloadPath(t *testing.T) {
	w := newUpdateWorkspace(t)
	fake := &fakeUpdates{
		release:      testRelease(),
		downloadPath: "/tmp/update.dmg",
		installErr: &updater.ManualInstallError{
			Target: "/usr/bin/viff",
			Path:   "/tmp/update.dmg",
			Reason: "the installation is read-only",
		},
	}
	w.SetUpdateService(fake)
	w.CheckForUpdates(true)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateAvailable })
	w.DownloadUpdate()
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateReady })

	w.InstallUpdate()
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateManual })
	if w.Update.ManualPath != "/tmp/update.dmg" || w.Update.Message == "" {
		t.Fatalf("manual state = %+v", w.Update)
	}
	if w.Update.ShouldQuit {
		t.Fatal("manual install requested a quit")
	}
}

func TestInstallSuccessRequestsQuit(t *testing.T) {
	w := newUpdateWorkspace(t)
	fake := &fakeUpdates{
		release:       testRelease(),
		downloadPath:  "/tmp/update.dmg",
		installResult: &updater.InstallResult{Target: "/Applications/Viff.app"},
	}
	w.SetUpdateService(fake)
	w.CheckForUpdates(true)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateAvailable })
	w.DownloadUpdate()
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateReady })

	w.InstallUpdate()
	settleUpdate(t, w, func() bool { return w.Update.ShouldQuit })
	if w.Update.Phase != UpdateInstalling {
		t.Fatalf("phase after install = %d", w.Update.Phase)
	}
}

func TestDismissUpdateResetsState(t *testing.T) {
	w := newUpdateWorkspace(t)
	w.SetUpdateService(&fakeUpdates{release: testRelease()})
	w.CheckForUpdates(true)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateAvailable })
	w.DismissUpdate()
	if w.Update.Phase != UpdateIdle || w.Update.Version != "" || w.HasUpdate() {
		t.Fatalf("state after dismiss = %+v", w.Update)
	}
}

func TestInstallRetryReusesDownloadedPackage(t *testing.T) {
	w := newUpdateWorkspace(t)
	fake := &fakeUpdates{
		release:      testRelease(),
		downloadPath: "/tmp/update.dmg",
		installErr:   errors.New("helper could not start"),
	}
	w.SetUpdateService(fake)
	w.CheckForUpdates(true)
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateAvailable })
	w.DownloadUpdate()
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateReady })

	w.InstallUpdate()
	settleUpdate(t, w, func() bool { return w.Update.Phase == UpdateError })
	if !w.HasDownloadedUpdate() {
		t.Fatal("a failed install dropped the verified package")
	}

	fake.installErr = nil
	fake.installResult = &updater.InstallResult{Target: "/Applications/Viff.app"}
	w.InstallUpdate()
	settleUpdate(t, w, func() bool { return w.Update.ShouldQuit })
}
