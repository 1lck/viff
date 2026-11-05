package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"viff/internal/gitcli"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 60, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The bytes decide the format: a text file named .png is not a picture.
func TestImageFormatSniffsBytes(t *testing.T) {
	cases := map[string][]byte{
		"PNG":  testPNG(t, 2, 2),
		"JPEG": append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 8)...),
		"GIF":  []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"),
	}
	for want, data := range cases {
		if got := ImageFormat(data); got != want {
			t.Errorf("ImageFormat = %q, want %q", got, want)
		}
	}
	if got := ImageFormat([]byte("<html>not a picture</html>")); got != "" {
		t.Errorf("text identified as %q, want empty", got)
	}
	if got := ImageFormat(nil); got != "" {
		t.Errorf("empty identified as %q, want empty", got)
	}
}

func TestImageDiffInfoReadsSizes(t *testing.T) {
	old := testPNG(t, 3, 2)
	next := testPNG(t, 5, 4)
	o := gitcli.Content{Exists: true, Data: old, Size: int64(len(old)), Binary: true}
	n := gitcli.Content{Exists: true, Data: next, Size: int64(len(next)), Binary: true}

	got := imageDiffInfo(o, n)
	if got == nil {
		t.Fatal("picture pair not recognised")
	}
	if got.Old.Width != 3 || got.Old.Height != 2 || got.Old.Format != "PNG" {
		t.Errorf("old side = %+v, want 3x2 PNG", got.Old)
	}
	if got.New.Width != 5 || got.New.Height != 4 || got.New.Format != "PNG" {
		t.Errorf("new side = %+v, want 5x4 PNG", got.New)
	}

	// An added picture has no old side, a deleted one no new side.
	added := imageDiffInfo(gitcli.Content{}, n)
	if added == nil || added.Old.Format != "" || added.New.Width != 5 {
		t.Errorf("added = %+v, want only the new side", added)
	}
	deleted := imageDiffInfo(o, gitcli.Content{})
	if deleted == nil || deleted.New.Format != "" || deleted.Old.Width != 3 {
		t.Errorf("deleted = %+v, want only the old side", deleted)
	}
}

// A picture against text stays a binary diff: half a picture pair is not a
// picture view.
func TestImageDiffInfoRejectsMixedSides(t *testing.T) {
	pic := gitcli.Content{Exists: true, Data: testPNG(t, 2, 2), Binary: true}
	text := gitcli.Content{Exists: true, Data: []byte("plain text"), Binary: true}
	if got := imageDiffInfo(pic, text); got != nil {
		t.Errorf("picture against text recognised: %+v", got)
	}
	if got := imageDiffInfo(text, pic); got != nil {
		t.Errorf("text against picture recognised: %+v", got)
	}
	pair := gitcli.Content{Exists: true, Data: []byte("a"), Binary: true}
	if got := imageDiffInfo(pair, pair); got != nil {
		t.Errorf("text pair recognised as pictures: %+v", got)
	}
}
