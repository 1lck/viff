package main

import (
	"fmt"
	"image"
	"image/draw"
	"math"

	"github.com/vivaui/viva/tools/appbundle"
)

// plateMargin is the transparent safe margin on each side of the rounded
// plate, as a fraction of the canvas. plateCorner is the corner radius as a
// fraction of the plate side.
const (
	plateMargin = 0.04
	plateCorner = 0.20
)

// roundedPlate turns full-bleed artwork into a canvas-sized icon whose white
// background is a rounded plate inset by a transparent safe margin. The
// artwork is scaled as a whole, so the orange figure keeps its shape,
// proportions and internal whitespace; nothing is cropped or redrawn.
func roundedPlate(artwork image.Image) (*image.NRGBA, error) {
	bounds := artwork.Bounds()
	if bounds.Dx() != bounds.Dy() {
		return nil, fmt.Errorf("brandassets: artwork must be square, got %dx%d", bounds.Dx(), bounds.Dy())
	}
	canvas := bounds.Dx()
	if canvas == 0 {
		return nil, fmt.Errorf("brandassets: artwork is empty")
	}
	margin := int(math.Round(float64(canvas) * plateMargin))
	side := canvas - 2*margin
	out := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	plate := appbundle.Downscale(artwork, side)
	draw.Draw(out, image.Rect(margin, margin, margin+side, margin+side), plate, image.Point{}, draw.Src)
	roundCorners(out, margin, side)
	return out, nil
}

// roundCorners replaces the plate's alpha with antialiased coverage of a
// rounded rectangle, leaving the area outside it fully transparent.
func roundCorners(img *image.NRGBA, margin, side int) {
	radius := float64(side) * plateCorner
	half := float64(side) / 2
	center := float64(margin) + half
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			// Signed distance in pixels from the rounded rectangle's edge.
			qx := math.Abs(float64(x)+0.5-center) - (half - radius)
			qy := math.Abs(float64(y)+0.5-center) - (half - radius)
			distance := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
			cover := math.Max(0, math.Min(1, 0.5-distance))
			img.Pix[img.PixOffset(x, y)+3] = uint8(cover*255 + 0.5)
		}
	}
}
