package viffui

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
)

// maxPictureSide bounds the texture an image diff decodes to. A photograph
// larger than this is sampled down: the panes never show it bigger than a
// screenful, and a 12-megapixel texture would cost tens of megabytes.
const maxPictureSide = 2048

// picture returns the decoded picture of one side, decoding a tab's side once.
func (t *diffTab) picture(old bool, res *vapp.DiffResult) (*ui.Bitmap, string) {
	if old {
		if t.picOld != nil || t.picOldErr != "" {
			return t.picOld, t.picOldErr
		}
		b, err := decodePicture(res.Old.Content.Data)
		if err != nil {
			t.picOldErr = err.Error()
			return nil, t.picOldErr
		}
		t.picOld = b
		return b, ""
	}
	if t.picNew != nil || t.picNewErr != "" {
		return t.picNew, t.picNewErr
	}
	b, err := decodePicture(res.New.Content.Data)
	if err != nil {
		t.picNewErr = err.Error()
		return nil, t.picNewErr
	}
	t.picNew = b
	return b, ""
}

// decodePicture turns encoded bytes into premultiplied BGRA pixels, the form
// Canvas.DrawBitmap takes. It samples a picture that is larger than
// maxPictureSide down to it, which is enough for any pane.
func decodePicture(data []byte) (*ui.Bitmap, error) {
	if len(data) == 0 {
		return nil, nil
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, fmt.Errorf("the picture has no pixels")
	}
	step := 1
	for max(sw, sh)/step > maxPictureSide {
		step++
	}
	dw, dh := sw/step, sh/step
	if dw <= 0 || dh <= 0 {
		return nil, fmt.Errorf("the picture has no pixels")
	}
	pix := make([]byte, dw*dh*4)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			// RGBA reports premultiplied 16-bit values, which is what the
			// renderer expects.
			r, g, blue, a := src.At(b.Min.X+x*step, b.Min.Y+y*step).RGBA()
			i := (y*dw + x) * 4
			pix[i] = byte(blue >> 8)
			pix[i+1] = byte(g >> 8)
			pix[i+2] = byte(r >> 8)
			pix[i+3] = byte(a >> 8)
		}
	}
	h := fnv.New64a()
	_, _ = h.Write(data)
	return &ui.Bitmap{Pix: pix, Width: dw, Height: dh, Stride: dw * 4, ID: h.Sum64(), Version: 1}, nil
}

// fitPicture centres a picture of w×h inside area at its own aspect ratio.
func fitPicture(area ui.Rect, w, h int) ui.Rect {
	if w <= 0 || h <= 0 || area.Width <= 0 || area.Height <= 0 {
		return area
	}
	scale := min(area.Width/float32(w), area.Height/float32(h))
	dw, dh := float32(w)*scale, float32(h)*scale
	return rect(area.X+(area.Width-dw)/2, area.Y+(area.Height-dh)/2, dw, dh)
}

// paintImageDiff draws the two versions of a picture side by side. A picture
// has no lines to unify, so the unified mode shows the same two panes.
func (w *Window) paintImageDiff(t *diffTab, r ui.Rect) {
	res := t.res
	titles := splitTop(&r, 28)
	w.p.fill(titles, w.pal.IslandAlt)
	w.p.hline(titles.X, bottom(titles)-1, titles.Width, w.pal.Border)
	half := titles.Width / 2
	w.paintTitle(rect(titles.X+10, titles.Y, half-16, titles.Height), res.Old.Title, "")
	w.paintTitle(rect(titles.X+half+10, titles.Y, max(half-16, 0), titles.Height), res.New.Title, "")

	w.p.fill(r, w.pal.Island)
	w.p.vline(r.X+half, r.Y, r.Height, w.pal.Border)
	w.paintPicturePane(t, rect(r.X, r.Y, half, r.Height), true, res)
	w.paintPicturePane(t, rect(r.X+half, r.Y, r.Width-half, r.Height), false, res)
}

// paintPicturePane draws one side: the picture scaled to the pane, or why
// there is nothing to draw.
func (w *Window) paintPicturePane(t *diffTab, r ui.Rect, old bool, res *vapp.DiffResult) {
	if r.Width <= 0 || r.Height <= 0 {
		return
	}
	side, content := res.Image.New, res.New.Content
	if old {
		side, content = res.Image.Old, res.Old.Content
	}
	const pad = 16
	inner := inset(r, pad, pad)
	if inner.Width <= 0 || inner.Height <= 0 {
		return
	}
	if side.Format == "" {
		msg := "No new version"
		if old {
			msg = "No previous version"
		}
		w.p.fill(inner, w.pal.IslandAlt)
		w.paintCentered(inner, msg, "The file does not exist on this side.", w.pal.TextSecond)
		return
	}
	label := fmt.Sprintf("%s %d×%d  %s", side.Format, side.Width, side.Height, sizeLabel(content.Size))
	b, imgErr := t.picture(old, res)
	if imgErr != "" {
		w.paintCentered(r, "Cannot decode picture", imgErr, w.pal.Error)
		return
	}
	if b == nil {
		w.paintCentered(r, label, "", w.pal.TextMuted)
		return
	}
	area := inner
	area.Height -= 22 // room for the label under the picture
	w.p.fill(area, w.pal.IslandAlt)
	dst := fitPicture(inset(area, 10, 10), b.Width, b.Height)
	w.p.outline(dst, 4, 1, w.pal.ControlBorder, ui.Color{})
	w.c.DrawBitmap(*b, dst)
	w.p.textCenter(label, rect(r.X, bottom(dst)+6, r.Width, 18), fontSmall, regular, w.pal.TextMuted)
}

// sizeLabel writes a byte count the way the rest of the diff views do.
func sizeLabel(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	case n > 0:
		return fmt.Sprintf("%d bytes", n)
	}
	return ""
}
