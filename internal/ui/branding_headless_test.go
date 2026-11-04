//go:build vivahead

package viffui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vivaui/viva/headless"
	"viff/assets/branding"
	vapp "viff/internal/app"
	"viff/internal/gitcli"
	"viff/internal/platform"
	"viff/internal/storage"
)

// This test opens no repository and executes no Git writes.
func TestApplicationBranding(t *testing.T) {
	platform.RegisterFonts()
	ws := vapp.NewWorkspace(&gitcli.Runner{}, storage.Defaults(), func() {})
	defer ws.Close()
	w := New(ws)
	cfg := w.Config()
	if cfg.Icon.Size != 1024 || cfg.Class != "viff" {
		t.Fatal("missing native app icon or desktop class")
	}
	session, err := headless.Open(cfg, nil, headless.Options{Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	check := func(name string, size float32) {
		t.Helper()
		session.Settle(40*time.Millisecond, time.Second)
		found := false
		for _, image := range w.c.Images() {
			if image.Bitmap.ID == branding.Bitmap.ID && image.Bounds.Width == size && image.Bounds.Height == size {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s does not draw the square application icon", name)
		}
		if dir := os.Getenv("VIFF_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := session.Screenshot(filepath.Join(dir, name+".png")); err != nil {
				t.Fatal(err)
			}
		}
	}
	check("branding-welcome", 36)
	w.openAbout()
	w.Invalidate()
	check("branding-about", 44)
}
