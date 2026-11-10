package main

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestRoundedPlate(t *testing.T) {
	const canvas = 256
	artwork := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	for y := 0; y < canvas; y++ {
		for x := 0; x < canvas; x++ {
			artwork.SetNRGBA(x, y, color.NRGBA{0xFE, 0xFE, 0xFE, 0xFF})
		}
	}

	got, err := roundedPlate(artwork)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != artwork.Bounds() {
		t.Fatalf("roundedPlate bounds = %v, want %v", got.Bounds(), artwork.Bounds())
	}
	alpha := func(x, y int) uint8 { return got.Pix[got.PixOffset(x, y)+3] }

	for _, corner := range [][2]int{{0, 0}, {canvas - 1, 0}, {0, canvas - 1}, {canvas - 1, canvas - 1}} {
		if value := alpha(corner[0], corner[1]); value != 0 {
			t.Errorf("corner %v alpha = %d, want 0", corner, value)
		}
	}
	if value := alpha(canvas/2, canvas/2); value != 255 {
		t.Errorf("center alpha = %d, want 255", value)
	}

	margin := int(math.Round(canvas * plateMargin))
	bounds := alphaBounds(got)
	if bounds.Min.X != margin || bounds.Min.Y != margin ||
		canvas-bounds.Max.X != margin || canvas-bounds.Max.Y != margin {
		t.Fatalf("opaque bounds = %v, want a %d px margin on every side", bounds, margin)
	}
	if value := alpha(canvas/2, bounds.Min.Y); value != 255 {
		t.Errorf("top edge midpoint alpha = %d, want 255", value)
	}
	if value := alpha(bounds.Min.X+1, bounds.Min.Y+1); value != 0 {
		t.Errorf("inside corner alpha = %d, want 0 (corners must be rounded, not square)", value)
	}
}

func TestRoundedPlateRejectsInvalidArtwork(t *testing.T) {
	if _, err := roundedPlate(image.NewNRGBA(image.Rect(0, 0, 2, 1))); err == nil {
		t.Error("roundedPlate accepted non-square artwork")
	}
	if _, err := roundedPlate(image.NewNRGBA(image.Rect(0, 0, 0, 0))); err == nil {
		t.Error("roundedPlate accepted empty artwork")
	}
}

func alphaBounds(img *image.NRGBA) image.Rectangle {
	bounds := img.Bounds()
	result := image.Rectangle{}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.Pix[img.PixOffset(x, y)+3] != 0 {
				result = result.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return result
}
