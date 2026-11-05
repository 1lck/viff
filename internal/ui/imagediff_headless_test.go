//go:build vivahead

package viffui

import (
	"image/color"
	"strings"
	"testing"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// A picture file opens as two panes with the picture sizes instead of the
// "binary files differ" notice.
func TestImageDiffShowsPicture(t *testing.T) {
	f := newUIFixture(t)
	before := testPicture(t, 4, 3, color.RGBA{R: 200, A: 255})
	after := testPicture(t, 6, 5, color.RGBA{R: 30, G: 120, A: 255})
	f.write("logo.png", string(before))
	f.git("add", "logo.png")
	f.gitDated("2026-10-20T10:00:00", "commit", "-q", "-m", "Add logo")
	f.write("logo.png", string(after))
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	f.win.showDiff(vapp.DiffSource{
		Path:    "logo.png",
		Area:    domain.AreaUnstaged,
		Working: true,
		Kind:    domain.ChangeModified,
	}, true)
	f.settleTab()

	texts := strings.Join(f.s.Texts(), " | ")
	if strings.Contains(texts, "Binary file") {
		t.Fatalf("picture still reported as binary: %s", texts)
	}
	if !strings.Contains(texts, "PNG 4×3") {
		t.Errorf("old pane label missing: %s", texts)
	}
	if !strings.Contains(texts, "PNG 6×5") {
		t.Errorf("new pane label missing: %s", texts)
	}
	f.assertIdle()
}
