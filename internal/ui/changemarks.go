package viffui

import (
	"github.com/vivaui/viva/ui"

	"viff/internal/domain"
	"viff/internal/i18n"
)

// The change marks are a narrow ruler on the right edge of the diff. They
// answer one question — where in this file are the changes — and draw nothing
// else: one rounded mark per changed stretch, tinted by the kind of change,
// with the visible range framed over them.
//
// The ruler takes the column the overlay scroll bar would use, and carrying the
// visible-range frame makes it the scroll bar: dragging it scrolls, so the two
// never sit side by side fighting for the same edge.
//
// Marks are collected once per load into line ranges, so a repaint only scales
// and draws them; the cost follows the number of changed hunks, not the file.

const (
	// marksWidth is the ruler's width: enough for a rounded mark and a frame,
	// narrow enough to read as a ruler rather than a column of its own.
	marksWidth = 12
	// markMinH keeps a one-line change visible at any file length.
	markMinH = 3
	// marksInset keeps marks off the ruler's edges so the frame stays readable.
	marksInset = 3
)

// miniMark is one changed stretch of display lines, [from, to).
type miniMark struct {
	from, to int
	kind     domain.ChangeKind
}

// buildMarks collects the changed stretches of one pane. blk is the pane's
// block map (line → block index, -1 for unchanged), so consecutive changed
// lines of the same block become one mark.
func buildMarks(blk []int32, blocks []domain.Block) []miniMark {
	var out []miniMark
	for i := 0; i < len(blk); {
		bi := blk[i]
		if bi < 0 || int(bi) >= len(blocks) {
			i++
			continue
		}
		start := i
		for i < len(blk) && blk[i] == bi {
			i++
		}
		out = append(out, miniMark{from: start, to: i, kind: blocks[bi].Kind()})
	}
	return out
}

// buildMarksUnified is buildMarks for the unified viewer, whose rows interleave
// both sides: a run of rows belonging to the same block becomes one mark.
func buildMarksUnified(t *diffTab) []miniMark {
	if t.res == nil {
		return nil
	}
	var out []miniMark
	for i := 0; i < len(t.unified); {
		bi := t.unified[i].block
		if bi < 0 || bi >= len(t.res.Blocks) {
			i++
			continue
		}
		start := i
		for i < len(t.unified) && t.unified[i].block == bi {
			i++
		}
		out = append(out, miniMark{from: start, to: i, kind: t.res.Blocks[bi].Kind()})
	}
	return out
}

// paintChangeMarks draws the marks into r, scaled to total display lines, and
// frames the range the editor shows. top and view are the editor's scroll
// offset and height in pixels; lineH is its line height.
func (w *Window) paintChangeMarks(t *diffTab, r ui.Rect, marks []miniMark, total int, top, view, lineH float32) {
	if total <= 0 || r.Width <= 0 || r.Height <= 0 {
		return
	}
	w.p.fill(r, w.pal.Island)
	w.p.vline(r.X, r.Y, r.Height, w.pal.Border)
	at := func(line float32) float32 {
		return r.Y + clampF(line/float32(total), 0, 1)*r.Height
	}
	w.c.Clip(r)
	for _, m := range marks {
		y0 := at(float32(m.from))
		h := max(at(float32(m.to))-y0, markMinH)
		w.p.round(rect(r.X+marksInset, y0, max(r.Width-2*marksInset, 1), h), 2, w.blockColor(m.kind))
	}
	// The frame goes on top: the marks underneath stay visible through it, and
	// the frame is what says how much of the file is on screen.
	if lineH > 0 {
		y0 := at(top / lineH)
		h := max(at((top+view)/lineH)-y0, 3)
		frame := rect(r.X+0.5, y0, r.Width-1.5, h)
		w.p.fill(frame, w.pal.ScrollThumb.WithAlpha(0.14))
		w.p.outline(frame, 1.5, 1, w.pal.ScrollThumb, ui.Color{})
	}
	w.c.Unclip()
	w.c.HitWithOptions(idDiffMarks, r, w.t(i18n.DiffMarksLabel), ui.HitOptions{SkipFocus: true, SkipTab: true})
	w.areas[idDiffMarks] = r
	w.tip(idDiffMarks, w.t(i18n.DiffMarksTip))
	// A press jumps there and a drag keeps following, like the scroll bar it
	// stands in for.
	w.onDrag(idDiffMarks, func(e dragEvent) {
		switch e.Phase {
		case dragBegin, dragMove:
			line := (e.Y - r.Y) / max(r.Height, 1) * float32(total)
			t.y = max(0, line*lineH)
		}
	})
}
