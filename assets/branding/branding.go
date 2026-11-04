// Package branding supplies the approved, text-free Viff application icon.
package branding

import (
	_ "embed"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/app/iconpng"
	"github.com/vivaui/viva/ui"
)

//go:embed icon.png
var iconPNG []byte

var Icon = decodeIcon()
var Bitmap = bitmap()

func decodeIcon() app.WindowIcon {
	icon, err := iconpng.Decode(iconPNG)
	if err != nil {
		panic(err)
	}
	return icon
}

func bitmap() ui.Bitmap {
	pixels := append([]byte(nil), Icon.RGBA...)
	for i := 0; i < len(pixels); i += 4 {
		r, g, b, a := uint32(pixels[i]), uint32(pixels[i+1]), uint32(pixels[i+2]), uint32(pixels[i+3])
		pixels[i], pixels[i+1], pixels[i+2] = byte(b*a/255), byte(g*a/255), byte(r*a/255)
	}
	return ui.Bitmap{Pix: pixels, Width: Icon.Size, Height: Icon.Size, Stride: Icon.Size * 4, ID: 0x76696666, Version: 1}
}
