package viffui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func testPicture(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// decodePicture hands Canvas.DrawBitmap premultiplied BGRA, so an opaque red
// pixel is stored blue = 0, green = 0, red = 255, alpha = 255.
func TestDecodePicture(t *testing.T) {
	data := testPicture(t, 4, 3, color.RGBA{R: 255, A: 255})
	b, err := decodePicture(data)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil {
		t.Fatal("no bitmap")
	}
	if b.Width != 4 || b.Height != 3 || b.Stride != 16 {
		t.Fatalf("bitmap = %dx%d stride %d, want 4x3 stride 16", b.Width, b.Height, b.Stride)
	}
	if len(b.Pix) != 4*3*4 {
		t.Fatalf("pixels = %d bytes, want 48", len(b.Pix))
	}
	if b.Pix[0] != 0 || b.Pix[1] != 0 || b.Pix[2] != 255 || b.Pix[3] != 255 {
		t.Fatalf("first pixel = %v, want premultiplied BGRA red", b.Pix[:4])
	}
	if b.ID == 0 {
		t.Error("bitmap ID must be set: a zero ID is uploaded again every frame")
	}
}

func TestDecodePictureRejectsGarbage(t *testing.T) {
	if _, err := decodePicture([]byte("not a picture")); err == nil {
		t.Fatal("garbage decoded without an error")
	}
	b, err := decodePicture(nil)
	if err != nil || b != nil {
		t.Fatalf("empty side = %v, %v; want no bitmap and no error", b, err)
	}
}

// fitPicture keeps the aspect ratio and centres what it fits.
func TestFitPictureKeepsAspect(t *testing.T) {
	square := fitPicture(rect(0, 0, 200, 100), 400, 400)
	if square.Width != 100 || square.Height != 100 || square.X != 50 || square.Y != 0 {
		t.Errorf("square = %+v, want 100x100 at (50,0)", square)
	}
	wide := fitPicture(rect(0, 0, 200, 200), 400, 100)
	if wide.Width != 200 || wide.Height != 50 || wide.Y != 75 {
		t.Errorf("wide = %+v, want 200x50 at y 75", wide)
	}
}
