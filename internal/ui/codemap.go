package viffui

import (
	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
)

// The code map is the optional minimap: the whole file squeezed into a narrow
// column beside the diff, one sliver per display line, tinted by the line's
// syntax so indentation and code density read as shape. It is off by default
// because it costs width and a compress pass; the change marks beside it are
// the cheap answer to "where are the changes".
//
// The shape is compressed once per load into runs counted in column pixels, so
// a repaint never measures text again. Runs carry a token kind rather than a
// colour, which keeps one cache valid for both themes.

const (
	// codemapWidth is the column's width: wide enough to show code shape,
	// narrow enough to leave the diff most of the pane.
	codemapWidth = 72
	// codemapMaxRuns bounds the runs kept per line, so a very long line cannot
	// turn the cache into a per-pixel copy of itself.
	codemapMaxRuns = 24
	// codemapDenseBelow is the line height at which the column switches to a
	// density bar per line, keeping the draw calls near the column's height
	// however long the file is.
	codemapDenseBelow = 1.5
)

// codemapRun is one coloured stretch of a line, in column pixel columns.
type codemapRun struct {
	from, to float32
	kind     tokKind
}

// codemapLine is the cached shape of one display line.
type codemapLine struct {
	runs  []codemapRun
	block int32
}

// buildCodemap compresses display lines, their syntax spans and their diff
// blocks into the column's shape. maxCells is the widest line of the tab, which
// sets the scale: the longest line reaches the right edge and every other line
// keeps its relative length.
func buildCodemap(disp []string, syn [][]span, blk []int32, maxCells int) []codemapLine {
	if len(disp) == 0 || maxCells <= 0 {
		return nil
	}
	px := func(cells int) float32 {
		return float32(cells) * codemapWidth / float32(maxCells)
	}
	out := make([]codemapLine, len(disp))
	for i, line := range disp {
		ml := codemapLine{block: -1}
		if i < len(blk) {
			ml.block = blk[i]
		}
		cells, pos := 0, 0
		// add appends a stretch, merging it into the previous run when the kind
		// matches, so a line of alternating quotes stays cheap.
		add := func(from, to int, k tokKind) {
			if to <= from {
				return
			}
			x0, x1 := px(from), px(to)
			if n := len(ml.runs); n > 0 && ml.runs[n-1].kind == k {
				last := &ml.runs[n-1]
				if x1 > last.to {
					last.to = x1
				}
				return
			}
			if len(ml.runs) >= codemapMaxRuns {
				// Keep the tail of an extreme line visible instead of dropping it.
				last := &ml.runs[len(ml.runs)-1]
				if x1 > last.to {
					last.to = x1
				}
				return
			}
			ml.runs = append(ml.runs, codemapRun{from: x0, to: x1, kind: k})
		}
		var spans []span
		if i < len(syn) {
			spans = syn[i]
		}
		for _, sp := range spans {
			if sp.start < pos || sp.end > len(line) || sp.start >= sp.end {
				continue
			}
			plain := displayCells(line[pos:sp.start])
			add(cells, cells+plain, tkPlain)
			cells += plain
			tinted := displayCells(line[sp.start:sp.end])
			add(cells, cells+tinted, sp.kind)
			cells += tinted
			pos = sp.end
		}
		add(cells, cells+displayCells(line[pos:]), tkPlain)
		out[i] = ml
	}
	return out
}

// buildCodemapUnified is buildCodemap for the unified viewer: its rows
// interleave both sides, so each row's text and syntax are taken from the side
// it belongs to.
func buildCodemapUnified(t *diffTab, maxCells int) []codemapLine {
	n := len(t.unified)
	if n == 0 || maxCells <= 0 {
		return nil
	}
	disp := make([]string, 0, n)
	syn := make([][]span, 0, n)
	blk := make([]int32, 0, n)
	for _, u := range t.unified {
		var line string
		var spans []span
		b := int32(-1)
		switch {
		case u.old >= 0 && u.old < len(t.oldDisp):
			line = t.oldDisp[u.old]
			if u.old < len(t.oldSyn) {
				spans = t.oldSyn[u.old]
			}
		case u.new >= 0 && u.new < len(t.newDisp):
			line = t.newDisp[u.new]
			if u.new < len(t.newSyn) {
				spans = t.newSyn[u.new]
			}
		}
		if u.block >= 0 {
			b = int32(u.block)
		}
		disp = append(disp, line)
		syn = append(syn, spans)
		blk = append(blk, b)
	}
	return buildCodemap(disp, syn, blk, maxCells)
}

// rebuildDiffEdge regenerates both right-edge caches of every tab from the diff
// data it already holds, so toggling either part never reloads a diff.
func (w *Window) rebuildDiffEdge() {
	s := w.ws.Settings
	for _, t := range w.ed.tabs {
		t.marksOld, t.marksNew, t.marksUni = nil, nil, nil
		t.mapOld, t.mapNew, t.mapUni = nil, nil, nil
		if t.res == nil {
			continue
		}
		if s.DiffMarks {
			t.marksOld = buildMarks(t.oldBlk, t.res.Blocks)
			t.marksNew = buildMarks(t.newBlk, t.res.Blocks)
		}
		if s.DiffCodemap {
			t.mapOld = buildCodemap(t.oldDisp, t.oldSyn, t.oldBlk, t.maxCells)
			t.mapNew = buildCodemap(t.newDisp, t.newSyn, t.newBlk, t.maxCells)
		}
		if s.DiffUnified {
			if s.DiffMarks {
				t.marksUni = buildMarksUnified(t)
			}
			if s.DiffCodemap {
				t.mapUni = buildCodemapUnified(t, t.maxCells)
			}
		}
	}
	w.Invalidate()
}
func (w *Window) codemapColor(k tokKind) ui.Color {
	switch k {
	case tkPlain:
		return w.pal.TextSecond.WithAlpha(0.55)
	case tkComment:
		return w.pal.SynComment.WithAlpha(0.8)
	}
	return w.tokColor(k).WithAlpha(0.9)
}

// paintCodemap draws lines into r. top and view are the editor's scroll offset
// and height in pixels; lineH is its line height. The visible range is framed
// over the shape, and a press or drag scrolls the editor there.
func (w *Window) paintCodemap(t *diffTab, r ui.Rect, lines []codemapLine, top, view, lineH float32) {
	if len(lines) == 0 || r.Width <= 0 || r.Height <= 0 {
		return
	}
	w.p.fill(r, w.pal.Island)
	w.p.vline(r.X, r.Y, r.Height, w.pal.Border)
	n := float32(len(lines))
	rh := r.Height / n
	tx, tw := r.X+3, max(r.Width-6, 1)
	dense := rh < codemapDenseBelow
	w.c.Clip(r)
	for i := range lines {
		ml := &lines[i]
		y := r.Y + float32(i)*rh
		if ml.block >= 0 && t.res != nil && int(ml.block) < len(t.res.Blocks) {
			c := w.blockColor(t.res.Blocks[ml.block].Kind())
			w.p.fill(rect(r.X+1, y, r.Width-2, max(rh, 0.75)), c.WithAlpha(0.5))
		}
		if dense {
			if len(ml.runs) == 0 {
				continue
			}
			// One sliver per line: prefer a syntax colour so keywords and
			// strings still read as texture.
			k := tkPlain
			for _, run := range ml.runs {
				if run.kind != tkPlain {
					k = run.kind
					break
				}
			}
			w.p.fill(rect(tx, y, tw, max(rh, 0.75)), w.codemapColor(k))
			continue
		}
		for _, run := range ml.runs {
			x0 := tx + run.from/float32(codemapWidth)*tw
			x1 := tx + run.to/float32(codemapWidth)*tw
			w.p.fill(rect(x0, y, max(x1-x0, 0.5), max(rh-0.35, 0.6)), w.codemapColor(run.kind))
		}
	}
	if lineH > 0 {
		y0 := r.Y + clampF(top/lineH, 0, n)*rh
		h := max(r.Y+clampF((top+view)/lineH, 0, n)*rh-y0, 3)
		w.p.fill(rect(r.X+1, y0, r.Width-2, h), w.pal.ScrollThumb.WithAlpha(0.22))
		w.p.outline(rect(r.X+1, y0, r.Width-2, h), 1, 1, w.pal.ScrollThumb, ui.Color{})
	}
	w.c.Unclip()
	w.c.HitWithOptions(idDiffCodemap, r, w.t(i18n.DiffCodemapLabel), ui.HitOptions{SkipFocus: true, SkipTab: true})
	w.areas[idDiffCodemap] = r
	w.tip(idDiffCodemap, w.t(i18n.DiffCodemapTip))
	w.onDrag(idDiffCodemap, func(e dragEvent) {
		switch e.Phase {
		case dragBegin, dragMove:
			line := (e.Y - r.Y) / max(r.Height, 1) * n
			t.y = max(0, line*lineH)
		}
	})
}
