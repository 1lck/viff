package viffui

import (
	"math"
	"strings"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/ui/icons"
)

const dividerW = 30

// metrics returns the editor font's cell size and the line height.
func (w *Window) metrics() (cellW, lineH, fs float32) {
	fs = w.ws.Settings.FontSize
	if fs < 8 {
		fs = 13
	}
	cellW, _ = w.c.MonoCellSize(fs, w.mono)
	lineH = float32(math.Round(float64(fs) * 1.45))
	return
}

func (w *Window) blockColor(k domain.ChangeKind) ui.Color {
	switch k {
	case domain.ChangeAdded:
		return w.pal.DiffInserted
	case domain.ChangeDeleted:
		return w.pal.DiffDeleted
	}
	return w.pal.DiffModified
}

// wordColor is the stronger tint of changed words inside a block.
func (w *Window) wordColor(k domain.ChangeKind) ui.Color {
	c := w.blockColor(k)
	if w.pal.Dark {
		return lighten(c, 0.14)
	}
	return lighten(c, -0.10)
}

func (w *Window) paintDiff(t *diffTab, r ui.Rect) {
	res := t.res
	tb := splitTop(&r, 36)
	w.paintDiffToolbar(t, tb)
	w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
	r.Y++
	r.Height--
	if t.searchOpen {
		sb := splitTop(&r, 36)
		w.paintSearchBar(t, sb)
		w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
		r.Y++
		r.Height--
	}
	if res.Notice != "" {
		w.paintCentered(r, res.Notice, "", w.pal.TextMuted)
		return
	}
	if res.ModeChange != "" || res.Identical() {
		msg := "Contents are identical"
		if res.ModeChange != "" {
			msg = "File mode changed: " + res.ModeChange
			if !res.Identical() {
				msg += " (content changed too)"
			}
		}
		br := splitTop(&r, 30)
		w.p.fill(br, w.pal.Banner)
		w.p.iconIn(icons.Info, rect(br.X+8, br.Y, 16, br.Height))
		w.p.text(msg, rect(br.X+30, br.Y, br.Width-40, br.Height), fontUI, regular, w.pal.Text)
	}
	titles := splitTop(&r, 28)
	w.p.fill(titles, w.pal.IslandAlt)
	w.p.hline(titles.X, bottom(titles)-1, titles.Width, w.pal.Border)
	if w.ws.Settings.DiffUnified {
		w.paintTitle(rect(titles.X+10, titles.Y, titles.Width-20, titles.Height), res.Old.Title+"  →  "+res.New.Title, "")
		w.paintUnified(t, r)
		return
	}
	paneW := (r.Width - dividerW) / 2
	left := rect(r.X, r.Y, paneW, r.Height)
	div := rect(r.X+paneW, r.Y, dividerW, r.Height)
	rightP := rect(right(div), r.Y, r.Width-paneW-dividerW, r.Height)
	oldPath := ""
	if t.src.OldPath != "" && t.src.OldPath != t.src.Path {
		oldPath = t.src.OldPath
	}
	w.paintTitle(rect(left.X+10, titles.Y, left.Width-16, titles.Height), res.Old.Title, oldPath)
	w.paintTitle(rect(rightP.X+10, titles.Y, rightP.Width-16, titles.Height), res.New.Title, "")
	w.p.vline(div.X+dividerW/2, titles.Y, titles.Height, w.pal.Border)

	cellW, lineH, _ := w.metrics()
	h := r.Height
	newContent := float32(len(t.newDisp)+3) * lineH
	oldContent := float32(len(t.oldDisp)+3) * lineH
	sync := w.ws.Settings.DiffSyncScroll
	t.y = clampF(t.y, 0, max(0, newContent-h))
	var oldY float32
	if sync {
		oldY = float32(domain.MapLine(res.Blocks, float64(t.y/lineH), false)) * lineH
		oldY = clampF(oldY, 0, max(0, oldContent-3*lineH))
	} else {
		t.yOld = clampF(t.yOld, 0, max(0, oldContent-h))
		oldY = t.yOld
	}
	gutter := w.gutterWidth(max(len(t.oldDisp), len(t.newDisp)), cellW)
	maxX := max(0, float32(t.maxCells)*cellW+40-(paneW-gutter))
	t.x = clampF(t.x, 0, maxX)

	w.onScrollIn(left, func(dx, dy float32) {
		t.x = clampF(t.x+dx, 0, maxX)
		if sync {
			t.y += dy
		} else {
			t.yOld += dy
		}
	})
	w.onScrollIn(rightP, func(dx, dy float32) {
		t.x = clampF(t.x+dx, 0, maxX)
		t.y += dy
	})
	w.onScrollIn(div, func(dx, dy float32) { t.y += dy })
	w.diffKeys(t, idDiffRight, lineH, h)
	w.diffKeys(t, idDiffLeft, lineH, h)

	w.paintPane(t, left, true, oldY, gutter, idDiffLeft)
	w.paintPane(t, rightP, false, t.y, gutter, idDiffRight)
	w.paintDivider(t, div, oldY, t.y, lineH)
	w.vbar(idDiffScrollR, rightP, &scroller{Y: t.y}, newContent)
	if !sync {
		sc := scroller{Y: t.yOld}
		w.vbar(idDiffScrollL, left, &sc, oldContent)
	}
	// The scroll bars work on copies; drags write back through closures.
	w.wrapBarDrag(idDiffScrollR, func(y float32) { t.y = y }, func() float32 { return t.y }, newContent, rightP.Height)
	if !sync {
		w.wrapBarDrag(idDiffScrollL, func(y float32) { t.yOld = y }, func() float32 { return t.yOld }, oldContent, left.Height)
	}
}

// wrapBarDrag rebinds a scroll bar's drag to a tab field.
func (w *Window) wrapBarDrag(id ui.ID, set func(float32), get func() float32, content, view float32) {
	if _, ok := w.drags[id]; !ok {
		return
	}
	th := max(view*view/content, 24)
	var start float32
	w.onDrag(id, func(e dragEvent) {
		switch e.Phase {
		case dragBegin:
			start = get()
		case dragMove:
			set(clampF(start+(e.Y-e.StartY)/(view-th)*(content-view), 0, content-view))
		}
	})
}

func (w *Window) paintTitle(r ui.Rect, title, sub string) {
	tw := w.p.text(title, r, fontSmall, semibold, w.pal.TextMuted)
	if sub != "" {
		w.p.text("("+sub+")", rect(r.X+tw+8, r.Y, r.Width-tw-8, r.Height), fontSmall, regular, w.pal.TextSecond)
	}
}

func (w *Window) gutterWidth(lines int, cellW float32) float32 {
	d := len(itoa(max(lines, 99)))
	return float32(d)*cellW + 28
}

// diffKeys scrolls the focused pane with the keyboard.
func (w *Window) diffKeys(t *diffTab, id ui.ID, lineH, view float32) {
	w.onKeys(id, func(e ui.KeyEvent) bool { return w.scrollKey(t, e, lineH, view) })
}

// scrollKey is the viewer scrolling the diff panes and the file view share.
func (w *Window) scrollKey(t *diffTab, e ui.KeyEvent, lineH, view float32) bool {
	switch e.Key {
	case ui.KeyDown:
		t.y += lineH
	case ui.KeyUp:
		t.y -= lineH
	case ui.KeyPageDown, ui.KeySpace:
		t.y += view - lineH
	case ui.KeyPageUp:
		t.y -= view - lineH
	case ui.KeyHome:
		t.y = 0
	case ui.KeyEnd:
		t.y = 1e9
	case ui.KeyF7:
		w.jumpBlock(t, e.Modifiers&ui.ModifierShift == 0)
	default:
		return false
	}
	return true
}

// paintPane draws one side of a side-by-side diff.
func (w *Window) paintPane(t *diffTab, r ui.Rect, old bool, top float32, gutter float32, id ui.ID) {
	res := t.res
	cellW, lineH, fs := w.metrics()
	disp, syn, blk, orig := t.newDisp, t.newSyn, t.newBlk, res.New.Lines
	sel := t.selNew
	if old {
		disp, syn, blk, orig = t.oldDisp, t.oldSyn, t.oldBlk, res.Old.Lines
		sel = t.selOld
	}
	w.c.HitWithOptions(id, r, "Diff", ui.HitOptions{Role: ui.RoleGroup})
	gr := rect(r.X, r.Y, gutter, r.Height)
	tr := rect(r.X+gutter, r.Y, r.Width-gutter, r.Height)
	// The read-only file view shares this painting with the diff panes; only
	// its single new side gets selectable text (see selection.go).
	view := !old && t.src.View
	w.c.Clip(r)
	first := max(0, int(top/lineH))
	last := min(len(disp), int((top+r.Height)/lineH)+1)
	style := ui.TextStyle{Size: fs, Color: w.pal.EditorText, Mono: true, MonoFace: w.mono}
	words := w.ws.Settings.DiffHighlightWords
	var runs []ui.TextRun
	for i := first; i < last; i++ {
		y := r.Y + float32(i)*lineH - top
		bi := blk[i]
		if bi >= 0 {
			b := res.Blocks[bi]
			c := w.blockColor(b.Kind())
			w.p.fill(rect(tr.X, y, tr.Width, lineH), c)
			w.p.fill(rect(gr.X, y, gr.Width, lineH), c.WithAlpha(0.55))
			if words && b.Kind() == domain.ChangeModified && i < len(orig) {
				var ranges []domain.Range
				if old && bi < int32(len(res.InlineOld)) && i-b.OldStart < len(res.InlineOld[bi]) {
					ranges = res.InlineOld[bi][i-b.OldStart]
				}
				if !old && bi < int32(len(res.InlineNew)) && i-b.NewStart < len(res.InlineNew[bi]) {
					ranges = res.InlineNew[bi][i-b.NewStart]
				}
				wc := w.wordColor(b.Kind())
				for _, rg := range ranges {
					x0 := float32(columnOf(orig[i], rg.Start, 4))*cellW + tr.X + 6 - t.x
					x1 := float32(columnOf(orig[i], rg.End, 4))*cellW + tr.X + 6 - t.x
					x0 = max(x0, tr.X)
					if x1 > x0 {
						w.p.round(rect(x0, y+1, x1-x0, lineH-2), 3, wc)
					}
				}
			}
		}
		if view {
			w.paintTextSelection(t, i, disp[i], tr, y, cellW, lineH)
		}
		if sel[i] {
			w.p.fill(rect(gr.X, y, gr.Width, lineH), w.pal.Selection)
			w.p.fill(rect(gr.X, y, 3, lineH), w.pal.Brand)
		}
		nc := w.pal.LineNumber
		if bi >= 0 || sel[i] {
			nc = w.pal.LineNumberCaret
		}
		w.p.c.LabelAligned(itoa(i+1), rect(gr.X, y, gr.Width-14, lineH), ui.TextStyle{Size: fs - 1, Color: nc, Mono: true, MonoFace: w.mono}, ui.AlignRight)
		if s := disp[i]; s != "" {
			var spans []span
			if i < len(syn) {
				spans = syn[i]
			}
			runs = w.runsFor(s, spans, runs, w.pal.EditorText)
			cells := float32(displayCells(s))
			w.c.Clip(tr)
			w.c.LabelRuns(s, rect(tr.X+6-t.x, y, cells*cellW+cellW, lineH), style, cellW, runs)
			w.c.Unclip()
		}
		if view {
			w.paintTextCaret(t, i, disp[i], tr, y, cellW, lineH)
		}
	}
	// Insertions and deletions show as a line on the side that lacks them.
	for _, b := range res.Blocks {
		s, e := b.NewStart, b.NewEnd
		if old {
			s, e = b.OldStart, b.OldEnd
		}
		if s != e {
			continue
		}
		y := r.Y + float32(s)*lineH - top
		if y < r.Y-2 || y > bottom(r)+2 {
			continue
		}
		c := lighten(w.blockColor(b.Kind()), 0.2)
		w.p.fill(rect(r.X, y-1, r.Width, 2), c)
	}
	if !old {
		w.paintHits(t, tr, top, cellW, lineH)
	}
	w.c.Unclip()
	if view {
		w.textPointer(t, tr, top, cellW, lineH)
	}
	w.p.vline(gr.X+gr.Width-1, r.Y, r.Height, w.pal.Border)

	// Line selection for partial staging: click changed line numbers.
	if w.canApply(t) {
		gid := scDiffLineSel.At(0)
		if old {
			gid = scDiffLineSel.At(1)
		}
		w.c.HitWithOptions(gid, gr, "Select lines", ui.HitOptions{SkipFocus: true, Cursor: ui.PointerCursorHand})
		w.tip(gid, "Click changed lines to select them for a partial stage or revert")
		w.onDrag(gid, func(e dragEvent) {
			if e.Phase != dragBegin {
				return
			}
			line := int((e.Y - r.Y + top) / lineH)
			if line < 0 || line >= len(blk) || blk[line] < 0 {
				return
			}
			if e.Mods&ui.ModifierShift != 0 && t.anchor >= 0 {
				for k := min(t.anchor, line); k <= max(t.anchor, line); k++ {
					if blk[k] >= 0 {
						sel[k] = true
					}
				}
			} else if sel[line] {
				delete(sel, line)
			} else {
				sel[line] = true
			}
			t.anchor = line
		})
	}
}

// canApply reports whether block actions are possible on this diff.
func (w *Window) canApply(t *diffTab) bool {
	if !t.src.Working || t.res == nil || t.res.Mode != domain.WhitespaceExact || t.busy {
		return false
	}
	return t.src.Area == domain.AreaUnstaged || t.src.Area == domain.AreaStaged
}

func (w *Window) paintDivider(t *diffTab, div ui.Rect, oldY, newY, lineH float32) {
	res := t.res
	w.c.Clip(div)
	for bi, b := range res.Blocks {
		lt := float32(b.OldStart)*lineH - oldY
		lb := float32(b.OldEnd)*lineH - oldY
		rt := float32(b.NewStart)*lineH - newY
		rb := float32(b.NewEnd)*lineH - newY
		if max(lb, rb) < -4 || min(lt, rt) > div.Height+4 {
			continue
		}
		c := w.blockColor(b.Kind())
		pb := w.c.PathBuilder()
		W := float32(dividerW)
		mid := W / 2
		pb.MoveTo(0, lt)
		pb.CubicTo(mid, lt, mid, rt, W, rt)
		pb.LineTo(W, max(rb, rt+1))
		pb.CubicTo(mid, max(rb, rt+1), mid, max(lb, lt+1), 0, max(lb, lt+1))
		pb.Close()
		w.c.DrawPath(pb.Path(W, div.Height), div, ui.PathStyle{Fill: c})
		_ = bi
	}
	w.c.Unclip()
	if !w.canApply(t) {
		return
	}
	for bi, b := range res.Blocks {
		lt := div.Y + float32(b.OldStart)*lineH - oldY
		rt := div.Y + float32(b.NewStart)*lineH - newY
		if t.src.Area == domain.AreaUnstaged {
			if rt >= div.Y-2 && rt < bottom(div)-14 {
				id := scDiffBlockB.At(bi)
				if w.blockButton(id, rect(right(div)-16, rt+1, 15, 15), icons.ArrowLeft, w.applyTip(t, bi, "Stage")) {
					bi := bi
					w.after(func() { w.applyBlock(t, bi, vapp.BlockStage) })
				}
			}
		}
		if lt >= div.Y-2 && lt < bottom(div)-14 {
			id := scDiffBlockA.At(bi)
			act, label := vapp.BlockRevert, "Revert"
			if t.src.Area == domain.AreaStaged {
				act, label = vapp.BlockUnstage, "Unstage"
			}
			if w.blockButton(id, rect(div.X+1, lt+1, 15, 15), icons.ArrowRight, w.applyTip(t, bi, label)) {
				bi := bi
				w.after(func() { w.applyBlock(t, bi, act) })
			}
		}
	}
}

func (w *Window) applyTip(t *diffTab, bi int, verb string) string {
	if n := t.selectedIn(bi); n > 0 {
		return verb + " " + plural(n, "Selected Line", "Selected Lines")
	}
	return verb + " Change"
}

func (t *diffTab) selectedIn(bi int) int {
	b := t.res.Blocks[bi]
	n := 0
	for k := range t.selOld {
		if k >= b.OldStart && k < b.OldEnd {
			n++
		}
	}
	for k := range t.selNew {
		if k >= b.NewStart && k < b.NewEnd {
			n++
		}
	}
	return n
}

func (w *Window) blockButton(id ui.ID, r ui.Rect, pair icons.Pair, tip string) bool {
	bg := w.pal.Island
	if w.hovered(id) {
		bg = w.pal.ToolPressed
	}
	w.p.round(rect(r.X-1, r.Y-1, r.Width+2, r.Height+2), 4, bg)
	w.p.iconIn(pair, rect(r.X-0.5, r.Y-0.5, r.Width+1, r.Height+1))
	w.c.HitWithOptions(id, rect(r.X-2, r.Y-2, r.Width+4, r.Height+4), tip, ui.HitOptions{Role: ui.RoleButton, SkipFocus: true})
	w.tip(id, tip)
	return w.c.Clicked(id)
}

func (w *Window) applyBlock(t *diffTab, bi int, act vapp.BlockAction) {
	if t.res == nil || bi >= len(t.res.Blocks) {
		return
	}
	var so, sn func(int) bool
	if t.selectedIn(bi) > 0 {
		selO, selN := cloneSet(t.selOld), cloneSet(t.selNew)
		so = func(i int) bool { return selO[i] }
		sn = func(i int) bool { return selN[i] }
	}
	t.busy = true
	w.ws.ApplyBlock(t.res, bi, act, so, sn, func(err error) {
		t.busy = false
		if err == vapp.ErrStale {
			w.loadTab(t)
		}
	})
}

func cloneSet(m map[int]bool) map[int]bool {
	out := make(map[int]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ---- unified viewer -------------------------------------------------------------

func (w *Window) paintUnified(t *diffTab, r ui.Rect) {
	res := t.res
	cellW, lineH, fs := w.metrics()
	n := len(t.unified)
	content := float32(n+3) * lineH
	t.y = clampF(t.y, 0, max(0, content-r.Height))
	numW := float32(len(itoa(max(max(len(t.oldDisp), len(t.newDisp)), 99))))*cellW + 12
	gutter := numW*2 + 24
	gr := rect(r.X, r.Y, gutter, r.Height)
	tr := rect(r.X+gutter, r.Y, r.Width-gutter, r.Height)
	maxX := max(0, float32(t.maxCells)*cellW+40-tr.Width)
	t.x = clampF(t.x, 0, maxX)
	w.onScrollIn(r, func(dx, dy float32) { t.y += dy; t.x = clampF(t.x+dx, 0, maxX) })
	w.c.HitWithOptions(idDiffRight, r, "Diff", ui.HitOptions{Role: ui.RoleGroup})
	w.diffKeys(t, idDiffRight, lineH, r.Height)
	style := ui.TextStyle{Size: fs, Color: w.pal.EditorText, Mono: true, MonoFace: w.mono}
	numStyle := ui.TextStyle{Size: fs - 1, Color: w.pal.LineNumber, Mono: true, MonoFace: w.mono}
	words := w.ws.Settings.DiffHighlightWords
	first := max(0, int(t.y/lineH))
	last := min(n, int((t.y+r.Height)/lineH)+1)
	var runs []ui.TextRun
	w.c.Clip(r)
	prevBlock := -1
	if first > 0 {
		prevBlock = t.unified[first-1].block
	}
	for i := first; i < last; i++ {
		u := t.unified[i]
		y := r.Y + float32(i)*lineH - t.y
		var disp string
		var spans []span
		var orig string
		var ranges []domain.Range
		kind := domain.ChangeKind(255)
		if u.block >= 0 {
			b := res.Blocks[u.block]
			if u.old >= 0 {
				kind = domain.ChangeDeleted
				if b.Kind() == domain.ChangeModified && u.old-b.OldStart < len(res.InlineOld[u.block]) {
					ranges = res.InlineOld[u.block][u.old-b.OldStart]
				}
			} else {
				kind = domain.ChangeAdded
				if b.Kind() == domain.ChangeModified && u.new-b.NewStart < len(res.InlineNew[u.block]) {
					ranges = res.InlineNew[u.block][u.new-b.NewStart]
				}
			}
		}
		if u.new >= 0 && u.old < 0 || (u.new >= 0 && u.block < 0) {
			disp, orig = t.newDisp[u.new], res.New.Lines[u.new]
			if u.new < len(t.newSyn) {
				spans = t.newSyn[u.new]
			}
		} else if u.old >= 0 {
			disp, orig = t.oldDisp[u.old], res.Old.Lines[u.old]
			if u.old < len(t.oldSyn) {
				spans = t.oldSyn[u.old]
			}
		}
		if kind != 255 {
			c := w.blockColor(kind)
			w.p.fill(rect(tr.X, y, tr.Width, lineH), c)
			w.p.fill(rect(gr.X, y, gr.Width, lineH), c.WithAlpha(0.55))
			if words {
				wc := w.wordColor(kind)
				for _, rg := range ranges {
					x0 := float32(columnOf(orig, rg.Start, 4))*cellW + tr.X + 6 - t.x
					x1 := float32(columnOf(orig, rg.End, 4))*cellW + tr.X + 6 - t.x
					if x1 > x0 {
						w.p.round(rect(x0, y+1, x1-x0, lineH-2), 3, wc)
					}
				}
			}
		}
		if u.old >= 0 {
			w.c.LabelAligned(itoa(u.old+1), rect(gr.X, y, numW, lineH), numStyle, ui.AlignRight)
		}
		if u.new >= 0 {
			w.c.LabelAligned(itoa(u.new+1), rect(gr.X+numW, y, numW, lineH), numStyle, ui.AlignRight)
		}
		if disp != "" {
			runs = w.runsFor(disp, spans, runs, w.pal.EditorText)
			w.c.Clip(tr)
			w.c.LabelRuns(disp, rect(tr.X+6-t.x, y, float32(displayCells(disp)+1)*cellW, lineH), style, cellW, runs)
			w.c.Unclip()
		}
		// Block actions at the first row of each block.
		if u.block >= 0 && u.block != prevBlock && w.canApply(t) {
			bi := u.block
			if t.src.Area == domain.AreaUnstaged {
				if w.blockButton(scDiffBlockB.At(bi), rect(gr.X+gutter-38, y+(lineH-15)/2, 15, 15), icons.Add, w.applyTip(t, bi, "Stage")) {
					w.after(func() { w.applyBlock(t, bi, vapp.BlockStage) })
				}
				if w.blockButton(scDiffBlockA.At(bi), rect(gr.X+gutter-20, y+(lineH-15)/2, 15, 15), icons.Revert, w.applyTip(t, bi, "Revert")) {
					w.after(func() { w.applyBlock(t, bi, vapp.BlockRevert) })
				}
			} else {
				if w.blockButton(scDiffBlockA.At(bi), rect(gr.X+gutter-20, y+(lineH-15)/2, 15, 15), icons.Hide, w.applyTip(t, bi, "Unstage")) {
					w.after(func() { w.applyBlock(t, bi, vapp.BlockUnstage) })
				}
			}
		}
		prevBlock = u.block
	}
	w.c.Unclip()
	w.p.vline(gr.X+gr.Width-1, r.Y, r.Height, w.pal.Border)
	sc := scroller{Y: t.y}
	w.vbar(idDiffScrollR, r, &sc, content)
	w.wrapBarDrag(idDiffScrollR, func(y float32) { t.y = y }, func() float32 { return t.y }, content, r.Height)
}

// ---- toolbar ---------------------------------------------------------------

func (w *Window) paintDiffToolbar(t *diffTab, r ui.Rect) {
	res := t.res
	s := w.ws.Settings
	x, y := r.X+8, r.Y+(r.Height-actionSize)/2
	nb := len(res.Blocks)
	if w.action(idDiffPrev, x, y, icons.Up, "Previous Difference\t⇧F7", actOpts{disabled: nb == 0}) {
		w.after(func() { w.jumpBlock(t, false) })
	}
	x += actionSize + 2
	if w.action(idDiffNext, x, y, icons.Down, "Next Difference\tF7", actOpts{disabled: nb == 0}) {
		w.after(func() { w.jumpBlock(t, true) })
	}
	x += actionSize + 8
	w.p.vline(x-4, y+5, actionSize-10, w.pal.Separator)
	mode := "Side-by-side viewer"
	if s.DiffUnified {
		mode = "Unified viewer"
	}
	clicked, cw := w.filterChip(idDiffMode, x, y, actionSize, mode, "")
	if clicked {
		ar := rect(x, y, cw, actionSize)
		w.after(func() {
			w.openMenuBelow(ar, []item{
				{title: "Side-by-side viewer", icon: icons.SideBySide, checked: !s.DiffUnified, run: func() { s.DiffUnified = false; w.saveSettings() }},
				{title: "Unified viewer", icon: icons.Unified, checked: s.DiffUnified, run: func() { s.DiffUnified = true; w.saveSettings() }},
			}, 200)
		})
	}
	x += cw + 2
	ws := domain.WhitespaceMode(s.DiffWhitespace)
	clicked, cw = w.filterChip(idDiffWS, x, y, actionSize, ws.Title(), "")
	if clicked {
		ar := rect(x, y, cw, actionSize)
		w.after(func() {
			var items []item
			for _, m := range []domain.WhitespaceMode{domain.WhitespaceExact, domain.WhitespaceTrim, domain.WhitespaceIgnore} {
				m := m
				items = append(items, item{title: m.Title(), checked: m == ws, run: func() {
					s.DiffWhitespace = int(m)
					w.saveSettings()
					for _, tab := range w.ed.tabs {
						w.loadTab(tab)
					}
				}})
			}
			w.openMenuBelow(ar, items, 200)
		})
	}
	x += cw + 6
	if w.action(idDiffWords, x, y, icons.Highlighting, "Highlight Words", actOpts{toggled: s.DiffHighlightWords}) {
		w.after(func() { s.DiffHighlightWords = !s.DiffHighlightWords; w.saveSettings() })
	}
	x += actionSize + 2
	if w.action(idDiffSync, x, y, icons.SynchronizeScrolling, "Synchronize Scrolling", actOpts{toggled: s.DiffSyncScroll, disabled: s.DiffUnified}) {
		w.after(func() { s.DiffSyncScroll = !s.DiffSyncScroll; w.saveSettings() })
	}
	x += actionSize + 2

	// Right side: counts and file actions.
	rx := right(r) - 8
	if t.src.Working && t.src.Area != domain.AreaConflicts {
		if w.action(idDiffOpen, rx-actionSize, y, icons.Edit, "Open in External Editor\tF4", actOpts{}) {
			p := t.src.Path
			w.after(func() { w.openExternal(p, w.currentLine(t)) })
		}
		rx -= actionSize + 6
		label := "Stage File"
		if t.src.Area == domain.AreaStaged {
			label = "Unstage File"
		}
		bw := w.p.width(label, fontUI, regular) + 20
		if w.button(idDiffStageFile, rect(rx-bw, r.Y+(r.Height-26)/2, bw, 26), label, btnBorderless, t.busy) {
			src := t.src
			w.after(func() {
				paths := []string{src.Path}
				if src.Area == domain.AreaStaged {
					if src.OldPath != "" {
						paths = append(paths, src.OldPath)
					}
					w.ws.UnstagePaths(paths)
				} else {
					w.ws.StagePaths(paths)
				}
			})
		}
		rx -= bw + 10
	}
	count := "No differences"
	if nb > 0 {
		count = plural(nb, "difference", "differences")
		if t.cur >= 0 && t.cur < nb {
			count = itoa(t.cur+1) + " of " + itoa(nb) + " differences"
			if nb == 1 {
				count = "1 difference"
			}
		}
	}
	if res.Notice != "" {
		count = ""
	}
	cw2 := w.p.width(count, fontUI, regular)
	if rx-cw2 > x+10 {
		w.p.text(count, rect(rx-cw2, r.Y, cw2+2, r.Height), fontUI, regular, w.pal.TextSecond)
	}
}

// currentLine is the 1-based line at the top of the new side.
func (w *Window) currentLine(t *diffTab) int {
	_, lineH, _ := w.metrics()
	return int(t.y/lineH) + 1
}

// jumpBlock moves to the next or previous changed block.
func (w *Window) jumpBlock(t *diffTab, next bool) {
	if t.res == nil || len(t.res.Blocks) == 0 {
		return
	}
	n := len(t.res.Blocks)
	if next {
		t.cur++
		if t.cur >= n {
			t.cur = n - 1
			return
		}
	} else {
		t.cur--
		if t.cur < 0 {
			t.cur = 0
			return
		}
	}
	w.scrollToBlock(t, t.cur)
}

func (w *Window) scrollToBlock(t *diffTab, bi int) {
	if t.res == nil || bi < 0 || bi >= len(t.res.Blocks) {
		return
	}
	_, lineH, _ := w.metrics()
	b := t.res.Blocks[bi]
	line := b.NewStart
	if w.ws.Settings.DiffUnified {
		for i, u := range t.unified {
			if u.block == bi {
				line = i
				break
			}
		}
	}
	t.y = max(0, float32(line-4)*lineH)
	t.yOld = max(0, float32(b.OldStart-4)*lineH)
}

// ---- search ---------------------------------------------------------------

func (w *Window) openDiffSearch() {
	t := w.ed.cur()
	if t == nil || t.res == nil {
		return
	}
	t.searchOpen = true
	w.focusNext = idDiffSearch
}

func (w *Window) paintSearchBar(t *diffTab, r ui.Rect) {
	w.p.fill(r, w.pal.IslandAlt)
	fr := rect(r.X+8, r.Y+4, min(360, r.Width-200), 28)
	if w.field(idDiffSearch, t.search, fr, fieldOpts{icon: icons.Search, placeholder: "Search in the new version"}) {
		w.after(func() { w.nextHit(t, true) })
	}
	w.onKeys(idDiffSearch, func(e ui.KeyEvent) bool {
		switch e.Key {
		case ui.KeyEscape:
			t.searchOpen = false
			return true
		case ui.KeyDown:
			w.nextHit(t, true)
			return true
		case ui.KeyUp:
			w.nextHit(t, false)
			return true
		}
		return false
	})
	q := strings.ToLower(t.search.String())
	if q != t.hitQuery {
		t.hitQuery = q
		t.hits = t.hits[:0]
		t.hitIdx = -1
		if q != "" {
			for i, l := range t.newDisp {
				low := strings.ToLower(l)
				off := 0
				for {
					k := strings.Index(low[off:], q)
					if k < 0 {
						break
					}
					t.hits = append(t.hits, searchHit{line: i, start: off + k, end: off + k + len(q)})
					off += k + max(1, len(q))
					if len(t.hits) > 5000 {
						break
					}
				}
			}
		}
	}
	x := right(fr) + 10
	status := ""
	switch {
	case q == "":
	case len(t.hits) == 0:
		status = "0 results"
	case t.hitIdx >= 0:
		status = itoa(t.hitIdx+1) + "/" + itoa(len(t.hits))
	default:
		status = plural(len(t.hits), "result", "results")
	}
	c := w.pal.TextSecond
	if q != "" && len(t.hits) == 0 {
		c = w.pal.Error
	}
	sw := w.p.text(status, rect(x, r.Y, 120, r.Height), fontUI, regular, c)
	x += sw + 10
	ay := r.Y + (r.Height-actionSize)/2
	if w.action(idDiffSearchPrev, x, ay, icons.Up, "Previous Occurrence\t⇧⏎", actOpts{disabled: len(t.hits) == 0}) {
		w.after(func() { w.nextHit(t, false) })
	}
	x += actionSize + 2
	if w.action(idDiffSearchNext, x, ay, icons.Down, "Next Occurrence\t⏎", actOpts{disabled: len(t.hits) == 0}) {
		w.after(func() { w.nextHit(t, true) })
	}
	if w.action(idDiffSearchClose, right(r)-8-actionSize, ay, icons.Close, "Close\t⎋", actOpts{}) {
		w.after(func() { t.searchOpen = false })
	}
}

func (w *Window) nextHit(t *diffTab, forward bool) {
	if len(t.hits) == 0 {
		return
	}
	if forward {
		t.hitIdx = (t.hitIdx + 1) % len(t.hits)
	} else {
		t.hitIdx--
		if t.hitIdx < 0 {
			t.hitIdx = len(t.hits) - 1
		}
	}
	_, lineH, _ := w.metrics()
	h := t.hits[t.hitIdx]
	t.y = max(0, float32(h.line-6)*lineH)
}

func (w *Window) paintHits(t *diffTab, tr ui.Rect, top, cellW, lineH float32) {
	if !t.searchOpen || len(t.hits) == 0 || w.ws.Settings.DiffUnified {
		return
	}
	for k, h := range t.hits {
		y := tr.Y + float32(h.line)*lineH - top
		if y < tr.Y-lineH || y > bottom(tr) {
			continue
		}
		l := t.newDisp[h.line]
		x0 := tr.X + 6 - t.x + float32(displayCells(l[:h.start]))*cellW
		x1 := tr.X + 6 - t.x + float32(displayCells(l[:h.end]))*cellW
		c := w.pal.SearchMatch
		if k == t.hitIdx {
			c = lighten(c, 0.25)
		}
		w.c.RoundedOutline(rect(x0-1, y+1, x1-x0+2, lineH-2), 3, 1, lighten(c, 0.3))
		w.p.round(rect(x0-1, y+1, x1-x0+2, lineH-2), 3, c.WithAlpha(0.6))
	}
}
