package viffui

import (
	"unicode/utf8"

	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
)

// The code map is the minimap of the unified viewer: the whole file drawn at
// one pixel per character, so indentation, comment blocks and code density read
// as the shape of the text rather than as a bar chart.
//
// The rules follow VS Code's minimap, which is the reference for this:
//
//   - A character occupies a fixed column, never a share of the width, so a
//     short line ends where its text ends instead of stretching.
//   - Space and tab advance the cursor and paint nothing. Indentation is
//     therefore visible as the gap it is, and that gap is what makes real code
//     read as code.
//   - Painting stops at the column limit instead of wrapping or squashing.
//
// Viva has no glyph rasterizer to sample, so a character is drawn as a solid
// cell in its syntax colour. At one pixel per character that is what VS Code's
// text mode looks like once the sampled glyph is downscaled below a pixel.
//
// The image is rasterized once per load and kept as a bitmap, so a repaint is
// one DrawBitmap: the cost follows the file once, not once per frame.

const (
	// codemapWidth is the column's width: wide enough for a readable shape,
	// narrow enough to leave the diff most of the pane.
	codemapWidth = 72
	// codemapBarW is the scroll bar's own column, kept outside the map so the
	// two never draw over each other.
	codemapBarW = 12
	// codemapGutter keeps the drawing off the panel edge, as VS Code's
	// MINIMAP_GUTTER_WIDTH does.
	codemapGutter = 3
	// codemapCharW is the width of one character column, in points. Two reads
	// as a word's silhouette at a glance; one dissolved into hairline noise.
	codemapCharW = 2
	// codemapMaxRuns bounds the runs kept per line, so an extreme line cannot
	// turn the cache into a per-character copy of itself.
	codemapMaxRuns = 32
	// codemapMinCols and codemapMaxCols bound what the column width allows.
	codemapMinCols = 8
	codemapMaxCols = 120
	// codemapThumbMin is the shortest the map's scroll handle may get, so a
	// file of thousands of lines still leaves something to grab.
	codemapThumbMin = 24
)

// codemapRun is one stretch of non-blank characters, in character columns.
type codemapRun struct {
	from, to int
	kind     tokKind
}

// codemapLine is the cached shape of one display line.
type codemapLine struct {
	runs  []codemapRun
	block int32
}

// codemapCols reports how many character columns fit in width.
func codemapCols(width float32) int {
	n := int((width - 2*codemapGutter) / codemapCharW)
	return max(codemapMinCols, min(n, codemapMaxCols))
}

// buildCodemap compresses display lines, their syntax spans and their diff
// blocks into character columns. Blank characters advance the cursor and emit
// nothing, which is what leaves the indentation visible.
func buildCodemap(disp []string, syn [][]span, blk []int32) []codemapLine {
	if len(disp) == 0 {
		return nil
	}
	out := make([]codemapLine, len(disp))
	for i, line := range disp {
		ml := codemapLine{block: -1}
		if i < len(blk) {
			ml.block = blk[i]
		}
		col := 0
		// add records [from, to) of painted characters, merging into the
		// previous run when the kind matches so a line of many small tokens
		// stays a handful of runs.
		add := func(from, to int, k tokKind) {
			if to <= from {
				return
			}
			if n := len(ml.runs); n > 0 && ml.runs[n-1].kind == k {
				last := &ml.runs[n-1]
				if to > last.to {
					last.to = to
				}
				return
			}
			if len(ml.runs) >= codemapMaxRuns {
				last := &ml.runs[len(ml.runs)-1]
				if to > last.to {
					last.to = to
				}
				return
			}
			ml.runs = append(ml.runs, codemapRun{from: from, to: to, kind: k})
		}
		// paint walks bytes [from, to) of the line, emitting the non-blank
		// characters and skipping the rest the way the minimap skips them.
		paint := func(from, to int, k tokKind) {
			for j := from; j < to && col < codemapMaxCols; {
				r, size := utf8.DecodeRuneInString(line[j:])
				j += size
				cells := cellWidthOf(r)
				// Space, tab and control characters take their cells and paint
				// nothing; that gap is what makes indentation visible in the
				// shape. Tabs are already expanded by expandLine.
				if r >= 0x20 && r != 0x7f && r != ' ' {
					add(col, min(col+cells, codemapMaxCols), k)
				}
				col += cells
			}
		}
		var spans []span
		if i < len(syn) {
			spans = syn[i]
		}
		pos := 0
		for _, sp := range spans {
			if sp.start < pos || sp.end > len(line) || sp.start >= sp.end {
				continue
			}
			paint(pos, sp.start, tkPlain)
			paint(sp.start, sp.end, sp.kind)
			pos = sp.end
		}
		paint(pos, len(line), tkPlain)
		out[i] = ml
	}
	return out
}

// codemapColor is the tint of one token kind in the map.
func (w *Window) codemapColor(k tokKind) ui.Color {
	switch k {
	case tkPlain:
		return w.pal.EditorText.WithAlpha(0.65)
	case tkComment:
		return w.pal.SynComment.WithAlpha(0.75)
	}
	return w.tokColor(k).WithAlpha(0.85)
}

// codemapImage is the rasterized map and what it was built for.
type codemapImage struct {
	bmp    *ui.Bitmap
	cols   int
	rows   int
	dark   bool
	lines  int
	width  float32
	height float32
}

// paintCodemap draws the map into r and lays its scroll handle over it. top is
// the editor's scroll offset in pixels, view its visible height, lineH its line
// height, and content the total scrollable height, so the handle is computed by
// the same rule the overlay scroll bar uses and the two always agree: they read
// and write the same t.y.
//
// A press on the handle drags it proportionally; a press anywhere else on the
// map jumps there, which is the quick way to reach a far part of a long file.
// paintCodemap draws the minimap into r and lays a visible viewport mask
// over it, matching VS Code / Sublime Text minimap behavior.
//
// The viewport mask represents the currently visible portion of the document.
// Dragging inside this mask smoothly scrolls the editor (proportional tracking),
// while clicking outside jumps the viewport to that position.
func (w *Window) paintCodemap(t *diffTab, r ui.Rect, lines []codemapLine, top, view, lineH, content float32) {
	if len(lines) == 0 || r.Width <= 0 || r.Height <= 0 {
		return
	}
	// Background matches editor background; left border subtly separates from code
	w.p.fill(r, w.pal.Island)
	w.p.vline(r.X, r.Y, r.Height, w.pal.Border)
	if bmp := w.codemapBitmap(t, lines, r.Width, r.Height); bmp != nil {
		w.c.DrawBitmap(*bmp, r)
	}

	// Elevate the compositing layer so the viewport mask definitely paints
	// above the bitmap texture in Viva's Direct3D/Vulkan backend.
	w.c.PushLayer(1)
	index := w.codemapHandle(r, top, view, lineH, content)
	w.c.PopLayer()

	w.c.HitWithOptions(idDiffCodemap, r, w.t(i18n.DiffCodemapLabel), ui.HitOptions{SkipFocus: true, SkipTab: true})
	w.areas[idDiffCodemap] = r
	w.tip(idDiffCodemap, w.t(i18n.DiffCodemapTip))

	// Wheel scrolling directly moves the document
	w.onScrollIn(r, func(dx, dy float32) {
		t.y = clampF(t.y+dy, 0, max(0, content-view))
	})

	// Click to jump or drag the viewport mask
	w.onDrag(idDiffCodemap, func(e dragEvent) {
		if content <= view {
			return
		}
		maxScroll := content - view
		switch e.Phase {
		case dragBegin:
			if index.onHandle(e.Y) {
				index.grabbing = true
				index.start = t.y
				return
			}
			index.grabbing = false
			// Clicked outside mask: center the viewport mask on click Y
			targetY := (e.Y - r.Y - index.thumbH/2) / max(r.Height-index.thumbH, 1) * maxScroll
			t.y = clampF(targetY, 0, maxScroll)
			index.start = t.y
		case dragMove:
			if index.grabbing {
				// Dragging from inside the mask: proportional dragging
				travel := r.Height - index.thumbH
				if travel > 0 {
					deltaY := (e.Y - e.StartY) / travel * maxScroll
					t.y = clampF(index.start+deltaY, 0, maxScroll)
				}
			} else {
				// Dragging after jumping outside
				targetY := (e.Y - r.Y - index.thumbH/2) / max(r.Height-index.thumbH, 1) * maxScroll
				t.y = clampF(targetY, 0, maxScroll)
			}
		}
	})
}

// codemapHandle is where the map's scroll handle sits and how tall it is.
type codemapHandle struct {
	y, thumbH float32
	start     float32
	grabbing  bool
}

// onHandle reports whether y is within the handle.
func (h codemapHandle) onHandle(y float32) bool {
	return y >= h.y && y <= h.y+h.thumbH
}

// codemapHandle calculates and draws the prominent viewport mask.
func (w *Window) codemapHandle(r ui.Rect, top, view, lineH, content float32) codemapHandle {
	h := codemapHandle{}
	if content <= 0 || view <= 0 {
		return h
	}

	// Calculate viewport mask height and Y position proportional to visible document area
	ratio := view / content
	if ratio >= 1.0 {
		// All content fits: cover the entire minimap height
		h.thumbH = r.Height
		h.y = r.Y
	} else {
		h.thumbH = max(r.Height*ratio, codemapThumbMin)
		h.y = r.Y + (r.Height-h.thumbH)*(top/max(content-view, 1))
	}

	hot := w.hovered(idDiffCodemap) || w.hovered(idDiffScrollR) ||
		(w.drag != nil && (w.drag.id == idDiffCodemap || w.drag.id == idDiffScrollR))

	// Prominent viewport mask like VS Code / Sublime minimap:
	// A distinct, semi-transparent highlight block that clearly frames the visible range
	// with a visible border and prominent top line.
	var maskFill, borderCol, accentCol ui.Color
	if w.pal.Dark {
		maskFill = ui.RGB24(0xFFFFFF).WithAlpha(0.24)
		borderCol = ui.RGB24(0xFFFFFF).WithAlpha(0.50)
		accentCol = ui.RGB24(0x56A8F5) // Solid light-blue top accent line
		if hot {
			maskFill = ui.RGB24(0xFFFFFF).WithAlpha(0.35)
			borderCol = ui.RGB24(0xFFFFFF).WithAlpha(0.75)
		}
	} else {
		maskFill = ui.RGB24(0x000000).WithAlpha(0.16)
		borderCol = ui.RGB24(0x000000).WithAlpha(0.40)
		accentCol = ui.RGB24(0x2F5EB9)
		if hot {
			maskFill = ui.RGB24(0x000000).WithAlpha(0.24)
			borderCol = ui.RGB24(0x000000).WithAlpha(0.60)
		}
	}

	maskRect := rect(r.X, h.y, r.Width, h.thumbH)
	// Fill the viewport mask block with smooth rounded corners
	w.p.round(maskRect, 4, maskFill)
	w.p.outline(maskRect, 4, 1.5, borderCol, ui.Color{})
	// Top indicator line (prominent 2.5px accent line across the top)
	w.p.round(rect(r.X+1, h.y, r.Width-2, 2.5), 1, accentCol)

	return h
}

// codemapBitmap returns the rasterized map, rebuilding it only when the shape
// it was built for changed: the palette, the size of the column, or the lines.
func (w *Window) codemapBitmap(t *diffTab, lines []codemapLine, width, height float32) *ui.Bitmap {
	cols := codemapCols(width)
	rows := int(height)
	if rows <= 0 || cols <= 0 {
		return nil
	}
	img := t.codemapImg
	if img != nil && img.bmp != nil && img.cols == cols && img.rows == rows &&
		img.dark == w.pal.Dark && img.lines == len(lines) {
		return img.bmp
	}
	pix := make([]byte, rows*cols*4)
	// The background is the panel's island colour, flattened to the 8-bit
	// premultiplied form the renderer uploads.
	bg := w.pal.Island
	bgR, bgG, bgB := float32(bg.Red), float32(bg.Green), float32(bg.Blue)
	if bgR <= 0 && bgG <= 0 && bgB <= 0 {
		bgR, bgG, bgB = 0.1, 0.1, 0.11
	}
	for i := 0; i < rows*cols; i++ {
		pix[i*4] = byte(bgB * 255)
		pix[i*4+1] = byte(bgG * 255)
		pix[i*4+2] = byte(bgR * 255)
		pix[i*4+3] = 255
	}
	n := len(lines)
	// Render lines. When vertical space allows, keep a 1px baseline gap between
	// lines to maintain typographic rhythm, exactly like an editor's minimap.
	for li := 0; li < n; li++ {
		from := li * rows / n
		to := min(max((li+1)*rows/n, from+1), rows)
		last := to
		if to-from >= 2 {
			last = to - 1 // 1px line gap
		}
		for _, run := range lines[li].runs {
			x0 := codemapGutter + run.from*codemapCharW
			x1 := codemapGutter + run.to*codemapCharW
			x0, x1 = min(x0, cols), min(x1, cols)
			if x1 <= x0 {
				continue
			}
			c := w.codemapColor(run.kind)
			r8, g8, b8 := clamp01(c.Red), clamp01(c.Green), clamp01(c.Blue)
			a := clamp01(c.Alpha)
			for y := from; y < last; y++ {
				for x := x0; x < x1; x++ {
					o := (y*cols + x) * 4
					pix[o] = byte((b8*a + bgB*(1-a)) * 255)
					pix[o+1] = byte((g8*a + bgG*(1-a)) * 255)
					pix[o+2] = byte((r8*a + bgR*(1-a)) * 255)
				}
			}
		}
	}
	bmp := &ui.Bitmap{Pix: pix, Width: cols, Height: rows, Stride: cols * 4, ID: w.codemapID(), Version: 1}
	t.codemapImg = &codemapImage{bmp: bmp, cols: cols, rows: rows, dark: w.pal.Dark, lines: len(lines), width: width, height: height}
	return bmp
}

// codemapID is a stable identity for a freshly built bitmap. The pixels are new
// each build, so a counter is enough; the renderer only uses it to tell one
// upload from another.
func (w *Window) codemapID() uint64 {
	w.codemapSeq++
	return w.codemapSeq
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// rebuildDiffEdge regenerates the right edge's caches of every tab from the diff
// data it already holds, so toggling either part never reloads a diff.
func (w *Window) rebuildDiffEdge() {
	s := w.ws.Settings
	for _, t := range w.ed.tabs {
		t.marksOld, t.marksNew, t.marksUni = nil, nil, nil
		t.codeMap, t.codemapImg = nil, nil
		if t.res == nil {
			continue
		}
		if s.DiffMarks {
			t.marksOld = buildMarks(t.oldBlk, t.res.Blocks)
			t.marksNew = buildMarks(t.newBlk, t.res.Blocks)
			t.marksUni = buildMarksUnified(t)
		}
		if s.DiffCodemap {
			t.codeMap = buildCodemap(t.newDisp, t.newSyn, nil)
		}
	}
	w.Invalidate()
}
