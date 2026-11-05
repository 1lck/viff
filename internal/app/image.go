package app

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"viff/internal/gitcli"
)

// The picture formats Viff shows in a diff. Only what the Go standard library
// decodes stands here: anything else would need a dependency.
const (
	imagePNG  = "PNG"
	imageJPEG = "JPEG"
	imageGIF  = "GIF"
)

// ImageSide is one picture of an image diff: its format and pixel size. The
// bytes stay in the Content of the side; the interface decodes them when the
// tab is shown, so loading a diff never pays for a picture nobody looks at.
type ImageSide struct {
	Format string
	Width  int
	Height int
}

// ImageDiff is set when the two sides are pictures instead of text. A side is
// the zero value when the file does not exist there (added or deleted).
type ImageDiff struct {
	Old, New ImageSide
}

// ImageFormat names the picture format of data, or "" when Viff cannot draw
// it. The bytes decide, not the file name: a picture keeps its type through a
// rename, and a text file called .png is not one.
func ImageFormat(data []byte) string {
	switch {
	case len(data) > 8 && bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return imagePNG
	case len(data) > 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return imageJPEG
	case len(data) > 6 && (bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))):
		return imageGIF
	}
	return ""
}

// imageSide describes one side, or the zero value when it is not a picture.
func imageSide(c gitcli.Content) ImageSide {
	if !c.Exists || len(c.Data) == 0 {
		return ImageSide{}
	}
	format := ImageFormat(c.Data)
	if format == "" {
		return ImageSide{}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(c.Data))
	if err != nil {
		return ImageSide{}
	}
	return ImageSide{Format: format, Width: cfg.Width, Height: cfg.Height}
}

// imageDiffInfo returns both picture sizes when this diff compares pictures: at
// least one side is one, and no side that exists is something else.
func imageDiffInfo(o, n gitcli.Content) *ImageDiff {
	oldSide, newSide := imageSide(o), imageSide(n)
	if oldSide.Format == "" && newSide.Format == "" {
		return nil
	}
	if o.Exists && oldSide.Format == "" {
		return nil
	}
	if n.Exists && newSide.Format == "" {
		return nil
	}
	return &ImageDiff{Old: oldSide, New: newSide}
}
