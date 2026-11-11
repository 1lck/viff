package viffui

import (
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// conflictPane is one column of the conflict viewer.
type conflictPane struct {
	title string
	disp  []string
	syn   [][]span
	mark  []uint8 // per line: 0 plain, 1 changed vs base, 2 conflict body, 3 marker line
}

// The per-block resolve controls, the Result editor and the copy actions for an
// external AI tool have an identity scope of their own, made in init so the
// shared tree (ids.go) already has its features, and declared in this file
// rather than in ids.go: the conflict viewer's read-only identities stay
// unchanged.
var (
	scConflictResolve   ui.Scope
	idResolveOurs       ui.ID
	idResolveTheirs     ui.ID
	idResolveBoth       ui.ID
	idConflictCopyAI    ui.ID
	idConflictCopyBlock ui.ID
	idConflictText      ui.ID
	idConflictSave      ui.ID
	idConflictRevert    ui.ID
)

func init() {
	scConflictResolve = tree.Sub(fDiff, tree.Name("conflict-resolve"))
	idResolveOurs = scConflictResolve.ID(tree.Name("ours"))
	idResolveTheirs = scConflictResolve.ID(tree.Name("theirs"))
	idResolveBoth = scConflictResolve.ID(tree.Name("both"))
	idConflictCopyAI = scConflictResolve.ID(tree.Name("copy-ai"))
	idConflictCopyBlock = scConflictResolve.ID(tree.Name("copy-block"))
	idConflictText = scConflictResolve.ID(tree.Name("result"))
	idConflictSave = scConflictResolve.ID(tree.Name("save"))
	idConflictRevert = scConflictResolve.ID(tree.Name("revert"))
}

func (w *Window) conflictPanes(t *diffTab) []conflictPane {
	cv := t.res.Conflict
	mk := func(title string, s []string, base []string) conflictPane {
		p := conflictPane{title: title, disp: expandAll(s)}
		p.syn = highlight(t.src.Path, p.disp)
		p.mark = make([]uint8, len(p.disp))
		if base != nil {
			for _, b := range domain.DiffLines(base, s, domain.WhitespaceExact, 0) {
				for k := b.NewStart; k < min(b.NewEnd, len(s)); k++ {
					p.mark[k] = 1
				}
			}
		}
		return p
	}
	base := cv.Base.Lines
	if !cv.Base.Content.Exists {
		base = nil
	}
	ours := mk(cv.OursLabel, cv.Ours.Lines, base)
	theirs := mk(cv.TheirsLabel, cv.Theirs.Lines, base)
	var cur conflictPane
	if e := t.cedit; e != nil {
		// While the Result pane edits, its buffer is the truth: markers,
		// highlighting and line numbers move with what the user typed.
		e.refresh()
		cur = conflictPane{title: w.t(i18n.ConflictLabelResult), disp: expandAll(e.lines)}
		cur.syn = highlight(t.src.Path, cur.disp)
		cur.mark = conflictMarks(len(cur.disp), e.markers)
	} else {
		cur = mk(w.t(i18n.ConflictLabelResult), cv.Current.Lines, nil)
		cur.mark = conflictMarks(len(cur.disp), cv.Markers)
	}
	return []conflictPane{ours, cur, theirs}
}

// conflictMarks shades conflict bodies and marker lines. The same function
// serves the loaded view and the edited buffer so a block looks identical
// before and after an accept.
func conflictMarks(n int, markers domain.MarkerParse) []uint8 {
	mark := make([]uint8, n)
	for _, b := range markers.Blocks {
		for k := b.Begin; k <= min(b.Finish, n-1); k++ {
			mark[k] = 2
		}
		for _, k := range []int{b.Begin, b.OursEnd, b.Finish} {
			if k >= 0 && k < n {
				mark[k] = 3
			}
		}
		if b.BaseStart >= 0 && b.BaseStart-1 < n && b.BaseStart > 0 {
			mark[b.BaseStart-1] = 3
		}
	}
	return mark
}

func (w *Window) paintConflict(t *diffTab, r ui.Rect) {
	cv := t.res.Conflict
	if t.cedit != nil {
		// The banner and the accept row read the dirty flag and the block list;
		// sync them before either is painted.
		t.cedit.refresh()
	}
	// In-app resolution needs a readable working-tree text whose markers form
	// complete blocks. Anything else stays read-only here: a binary file, a
	// symlink or a submodule has no sides to accept as text, markers the
	// parser cannot trust would make "block 2" mean the wrong lines, and a file
	// above the editor's limit cannot be written back by Save. Rewriting a
	// conflict block in place is a capability, not a given: a Subversion
	// working copy has no in-app resolution at all (its conflict view stays
	// read-only, and "Mark as Resolved" runs svn resolve instead).
	// A buffer exists only when the tab was editable at load; from then on the
	// buffer itself decides what the controls offer, so a later disk change
	// cannot hide Save under an unsaved result.
	resolvable := t.cedit != nil
	// The copied context is built from the same buffer (or the loaded view) the
	// accept controls use: a stale block would name the wrong lines. It does
	// not need the resolver, only a trustworthy parse, because the clipboard
	// path sends text to the user's own assistant.
	copyView := w.conflictViewForContext(t)
	canCopy := copyView != nil && copyView.Conflict.Kind.Textual() && copyView.Current.Content.Exists && copyView.Current.Text &&
		!copyView.Current.Content.Binary && !copyView.Current.Content.TooLarge &&
		!copyView.Current.Content.Submodule && !copyView.Current.Content.Symlink &&
		copyView.Markers.Reliable && len(copyView.Markers.Blocks) > 0
	blocks := w.conflictBlocks(t)
	n := len(blocks)
	if n > 0 {
		t.conflictBlock = min(t.conflictBlock, n-1)
	} else {
		t.conflictBlock = 0
	}
	dirty := t.cedit != nil && t.cedit.dirty
	diskResolved := cv.Resolved || (cv.Conflict.Kind.Textual() && len(cv.Markers.Blocks) == 0 && cv.Current.Content.Exists)
	resolved := diskResolved && !dirty
	// Banner.
	bh := float32(88)
	if resolvable {
		bh = 122 // room for the per-block accept row
	}
	br := splitTop(&r, bh)
	inner := rect(br.X+10, br.Y+8, br.Width-20, bh-16)
	bg, border := w.pal.BannerWarn, w.pal.BannerWarnBorder
	if resolved {
		bg, border = w.pal.Banner, w.pal.BannerBorder
	}
	w.p.outline(inner, 8, 1, border, bg)
	icon := icons.Warning
	title := w.t(i18n.ConflictTitleMerge, cv.Conflict.Kind.Description())
	sub := w.t(i18n.ConflictSubResolve)
	if resolved {
		icon = icons.Success
		title = w.t(i18n.ConflictTitleResolved)
		sub = w.t(i18n.ConflictSubResolved)
	} else if n > 0 {
		if n == 1 {
			sub = w.t(i18n.ConflictSubFoundOne, n)
		} else {
			sub = w.t(i18n.ConflictSubFoundMany, n)
		}
		if resolvable {
			if n == 1 {
				sub = w.t(i18n.ConflictSubAcceptOne, n)
			} else {
				sub = w.t(i18n.ConflictSubAcceptMany, n)
			}
		}
		if !copyView.Markers.Reliable {
			sub = w.t(i18n.ConflictSubUnreliable)
		}
	} else if t.cedit != nil {
		if t.cedit.markers.Reliable {
			sub = w.t(i18n.ConflictSubResultClean)
		} else {
			sub = w.t(i18n.ConflictSubUnreliable)
		}
	} else if !cv.Conflict.Kind.Textual() {
		sub = w.t(i18n.ConflictSubNoText)
	}
	w.p.iconIn(icon, rect(inner.X+10, inner.Y+8, 18, 22))
	tw := w.p.text(title, rect(inner.X+34, inner.Y+8, inner.Width-46, 22), fontUI, semibold, w.pal.Text)
	if sx := inner.X + 34 + tw + 12; right(inner)-sx > 60 {
		w.p.text(sub, rect(sx, inner.Y+8, right(inner)-sx-12, 22), fontUI, regular, w.pal.TextMuted)
	}
	bx, by := inner.X+34, inner.Y+38
	label := w.t(i18n.ConflictActionMarkAsResolved)
	bw := w.buttonWidth(label)
	stageDisabled := w.readOnly() || dirty || !resolved && cv.Conflict.Kind.Textual()
	if w.button(idConflictStage, rect(bx, by, bw, 26), label, btnDefault, stageDisabled) {
		p := t.src.Path
		w.after(func() { w.markResolved(p) })
	}
	switch {
	case dirty:
		w.tip(idConflictStage, w.t(i18n.ConflictTipSaveFirst))
	case !resolved && cv.Conflict.Kind.Textual():
		w.tip(idConflictStage, w.t(i18n.ConflictTipMarkAsResolved))
	}
	bx += bw + 8
	if resolvable {
		saveLabel := w.t(i18n.ConflictActionSave)
		sw := w.buttonWidth(saveLabel)
		if w.button(idConflictSave, rect(bx, by, sw, 26), saveLabel, btnNormal, !dirty || t.cedit.saving || t.busy || t.loading) {
			w.after(func() { w.saveConflict(t) })
		}
		w.tip(idConflictSave, w.t(i18n.ConflictTipSave))
		bx += sw + 8
		revertLabel := w.t(i18n.ConflictActionRevert)
		rw := w.buttonWidth(revertLabel)
		if w.button(idConflictRevert, rect(bx, by, rw, 26), revertLabel, btnBorderless, !dirty || t.cedit.saving || t.busy || t.loading) {
			w.after(func() { w.revertConflict(t) })
		}
		w.tip(idConflictRevert, w.t(i18n.ConflictTipRevert))
		bx += rw + 8
	}
	ow := w.buttonWidth(w.t(i18n.ConflictActionOpenEditor))
	if w.button(idConflictOpen, rect(bx, by, ow, 26), w.t(i18n.ConflictActionOpenEditor), btnNormal, w.readOnly() || !cv.Current.Content.Exists) {
		p := t.src.Path
		line := 0
		if t.conflictBlock < len(blocks) && len(blocks) > 0 {
			line = blocks[t.conflictBlock].Begin + 1
		}
		w.after(func() { w.openExternal(p, line) })
	}
	bx += ow + 8
	cw := w.buttonWidth(w.t(i18n.ConflictActionCopyPath))
	if w.button(idConflictCopy, rect(bx, by, cw, 26), w.t(i18n.ConflictActionCopyPath), btnBorderless, false) {
		full := w.repoPath(t.src.Path)
		w.after(func() { w.copyText(full) })
	}
	bx += cw + 8
	// The conflict context goes to the user's own assistant through the
	// clipboard: Viff starts no request and calls no model. It is offered
	// exactly when the text behind the accept controls can be trusted.
	aiLabel := w.t(i18n.ConflictActionCopyForAI)
	aiw := w.buttonWidth(aiLabel)
	if w.button(idConflictCopyAI, rect(bx, by, aiw, 26), aiLabel, btnNormal, !canCopy || t.busy || t.loading) {
		w.after(func() { w.copyConflictForAI(t, vapp.ConflictContextAll) })
	}
	if canCopy {
		w.tip(idConflictCopyAI, w.t(i18n.ConflictTipCopyForAI))
	} else {
		w.tip(idConflictCopyAI, w.t(i18n.ConflictTipCopyNothing))
	}
	// Conflict navigation.
	if n > 0 {
		nx := right(inner) - 8 - actionSize
		if w.action(scConflictNav.At(1), nx, by, icons.Down, w.t(i18n.ConflictActionNext), actOpts{disabled: t.conflictBlock >= n-1}) {
			w.after(func() { t.conflictBlock++; w.scrollToConflict(t) })
		}
		nx -= actionSize + 2
		if w.action(scConflictNav.At(0), nx, by, icons.Up, w.t(i18n.ConflictActionPrevious), actOpts{disabled: t.conflictBlock == 0}) {
			w.after(func() { t.conflictBlock--; w.scrollToConflict(t) })
		}
		s := w.t(i18n.ConflictCount, itoa(t.conflictBlock+1), itoa(n))
		sw := w.p.width(s, fontUI, regular)
		w.p.text(s, rect(nx-sw-10, by, sw+2, 26), fontUI, regular, w.pal.TextMuted)
	}
	// Per-block resolution: accepting a side edits the Result buffer, not the
	// file on disk. Save is the only write path and the only stale check.
	if resolvable && n > 0 {
		rx := inner.X + 34
		label := w.t(i18n.ConflictAcceptTitle, itoa(t.conflictBlock+1), itoa(n))
		w.p.text(label, rect(rx, inner.Y+72, 230, 26), fontUI, regular, w.pal.TextMuted)
		rx += 240
		disabled := t.busy || t.loading || w.readOnly() || !t.cedit.markers.Reliable
		choices := []struct {
			id     ui.ID
			label  string
			tip    string
			choice domain.ResolveChoice
		}{
			{idResolveOurs, w.t(i18n.ConflictAcceptOurs), w.t(i18n.ConflictTipKeepSide, cv.OursLabel), domain.ResolveOurs},
			{idResolveTheirs, w.t(i18n.ConflictAcceptTheirs), w.t(i18n.ConflictTipKeepSide, cv.TheirsLabel), domain.ResolveTheirs},
			{idResolveBoth, w.t(i18n.ConflictAcceptBoth), w.t(i18n.ConflictTipKeepBoth, cv.OursLabel, cv.TheirsLabel), domain.ResolveBoth},
		}
		for _, c := range choices {
			cw := w.buttonWidth(c.label)
			if w.button(c.id, rect(rx, inner.Y+72, cw, 26), c.label, btnNormal, disabled) {
				choice := c.choice
				w.after(func() { w.acceptConflictBlock(t, choice) })
			}
			w.tip(c.id, c.tip)
			rx += cw + 8
		}
		// The block-level counterpart of Copy for AI, pinned to the right of the
		// accept row so it cannot push the accept controls out of the banner.
		cbLabel := w.t(i18n.ConflictActionCopyBlock)
		cbw := w.buttonWidth(cbLabel)
		if cx := right(inner) - 8 - cbw; cx > rx+8 {
			if w.button(idConflictCopyBlock, rect(cx, inner.Y+72, cbw, 26), cbLabel, btnBorderless, disabled) {
				w.after(func() { w.copyConflictForAI(t, t.conflictBlock) })
			}
			w.tip(idConflictCopyBlock, w.t(i18n.ConflictTipCopyBlock))
		}
	}

	if !cv.Conflict.Kind.Textual() && !cv.Current.Content.Exists && !cv.Ours.Content.Exists && !cv.Theirs.Content.Exists {
		return
	}
	if t.unified == nil {
		t.unified = []uline{} // marks the panes as prepared
	}
	panes := w.conflictPanesCached(t)
	titles := splitTop(&r, 28)
	w.p.fill(titles, w.pal.IslandAlt)
	w.p.hline(titles.X, bottom(titles)-1, titles.Width, w.pal.Border)
	gap := float32(1)
	pw := (r.Width - 2*gap) / 3
	cellW, lineH, _ := w.metrics()
	maxLines := 0
	for _, p := range panes {
		maxLines = max(maxLines, len(p.disp))
	}
	content := float32(maxLines+3) * lineH
	if t.cedit != nil {
		gutter := w.gutterWidth(len(t.cedit.lines), cellW)
		if w.focused(idConflictText) {
			t.y, t.x = w.followBufferCaret(&t.cedit.textBuffer, t.y, t.x, pw-gutter, r.Height, cellW, lineH)
		}
		maxX := max(0, float32(t.cedit.maxCells)*cellW+40-(pw-gutter))
		t.x = clampF(t.x, 0, maxX)
	}
	t.y = clampF(t.y, 0, max(0, content-r.Height))
	w.onScrollIn(r, func(dx, dy float32) { t.y += dy; t.x = max(0, t.x+dx) })
	w.diffKeys(t, idDiffRight, lineH, r.Height)
	for i, p := range panes {
		pr := rect(r.X+float32(i)*(pw+gap), r.Y, pw, r.Height)
		sub := ""
		var edit *conflictEdit
		if i == 1 {
			edit = t.cedit
			if edit != nil {
				sub = w.t(i18n.ConflictLabelEditable)
			}
		}
		w.paintTitle(rect(pr.X+10, titles.Y, pr.Width-16, titles.Height), p.title, sub)
		w.paintConflictPane(p, pr, t, cellW, lineH, edit)
		if i > 0 {
			w.p.vline(pr.X-1, titles.Y, r.Height+titles.Height, w.pal.Border)
		}
	}
	sc := scroller{Y: t.y}
	w.vbar(idDiffScrollR, r, &sc, content)
	w.wrapBarDrag(idDiffScrollR, func(y float32) { t.y = y }, func() float32 { return t.y }, content, r.Height)
}

// conflictPanesCached keeps the panes with the tab between paints. While the
// Result pane edits, the buffer text is part of the key: a repaint with the
// same text reuses the display rows, an accept or a keystroke rebuilds them.
func (w *Window) conflictPanesCached(t *diffTab) []conflictPane {
	built := ""
	if t.cedit != nil {
		t.cedit.refresh()
		built = t.cedit.built
	}
	if c, ok := conflictCache[t]; ok && c.res == t.res && c.tr == w.tr && c.built == built {
		return c.panes
	}
	panes := w.conflictPanes(t)
	conflictCache[t] = conflictEntry{res: t.res, tr: w.tr, panes: panes, built: built}
	return panes
}

type conflictEntry struct {
	res   any
	tr    *i18n.Translator
	panes []conflictPane
	built string
}

var conflictCache = map[*diffTab]conflictEntry{}

func (w *Window) paintConflictPane(p conflictPane, r ui.Rect, t *diffTab, cellW, lineH float32, edit *conflictEdit) {
	_, _, fs := w.metrics()
	gutter := w.gutterWidth(len(p.disp), cellW)
	gr := rect(r.X, r.Y, gutter, r.Height)
	tr := rect(r.X+gutter, r.Y, r.Width-gutter, r.Height)
	style := ui.TextStyle{Size: fs, Color: w.pal.EditorText, Mono: true, MonoFace: w.mono}
	numStyle := ui.TextStyle{Size: fs - 1, Color: w.pal.LineNumber, Mono: true, MonoFace: w.mono}
	if edit != nil {
		edit.tr, edit.top, edit.scrollX = tr, t.y, t.x
		edit.cellW, edit.lineH, edit.painted = cellW, lineH, true
	}
	w.c.Clip(r)
	first := max(0, int(t.y/lineH))
	last := min(len(p.disp), int((t.y+r.Height)/lineH)+1)
	var runs []ui.TextRun
	for i := first; i < last; i++ {
		y := r.Y + float32(i)*lineH - t.y
		switch p.mark[i] {
		case 1:
			w.p.fill(rect(tr.X, y, tr.Width, lineH), w.pal.DiffModified)
		case 2:
			w.p.fill(rect(tr.X, y, tr.Width, lineH), w.pal.DiffConflict)
		case 3:
			w.p.fill(rect(tr.X, y, tr.Width, lineH), lighten(w.pal.DiffConflict, 0.12))
		}
		if edit != nil {
			w.paintBufferSelection(&edit.textBuffer, idConflictText, i, displayCells(p.disp[i]), tr, y, t.x, cellW, lineH)
		}
		w.c.LabelAligned(itoa(i+1), rect(gr.X, y, gr.Width-14, lineH), numStyle, ui.AlignRight)
		if s := p.disp[i]; s != "" {
			var spans []span
			if i < len(p.syn) && p.mark[i] != 3 {
				spans = p.syn[i]
			}
			base := w.pal.EditorText
			if p.mark[i] == 3 {
				base = w.pal.TextMuted
			}
			runs = w.runsFor(s, spans, runs, base)
			w.c.Clip(tr)
			w.c.LabelRuns(s, rect(tr.X+6-t.x, y, float32(displayCells(s)+1)*cellW, lineH), style, cellW, runs)
			w.c.Unclip()
		}
		if edit != nil {
			w.paintBufferComposition(&edit.textBuffer, i, displayCells(p.disp[i]), tr, y, t.x, cellW, lineH)
			w.paintBufferCaret(&edit.textBuffer, idConflictText, i, tr, y, t.x, cellW, lineH)
		}
	}
	if edit != nil {
		if len(p.disp) == 0 {
			w.paintBufferCaret(&edit.textBuffer, idConflictText, 0, tr, r.Y, t.x, cellW, lineH)
		}
		// The Result pane is the only text field in this view: the two side
		// panes stay read-only and register no editable hit.
		w.c.HitWithOptions(idConflictText, tr, w.t(i18n.ConflictEditLabel), ui.HitOptions{
			Role: ui.RoleTextField, Value: edit.built, Multiline: true,
			TextBounds: rect(tr.X+6, tr.Y, tr.Width-6, tr.Height),
			TextSize:   fs, TextColor: w.pal.EditorText,
			HideNativeOverlays: true, Placement: &edit.textBuffer,
		})
		w.editors[idConflictText] = edit.ed
	}
	w.c.Unclip()
	w.p.vline(gr.X+gr.Width-1, r.Y, r.Height, w.pal.Border)
}

func (w *Window) scrollToConflict(t *diffTab) {
	blocks := w.conflictBlocks(t)
	if t.conflictBlock < 0 || t.conflictBlock >= len(blocks) {
		return
	}
	_, lineH, _ := w.metrics()
	t.y = max(0, float32(blocks[t.conflictBlock].Begin-4)*lineH)
}

// copyConflictForAI builds the structured context of the conflict tab and puts
// it on the clipboard. Viff itself runs no model and opens no connection: the
// text is for whatever assistant the user pastes it into. block is a 0-based
// conflict block, or app.ConflictContextAll for the whole file. While the Result
// pane edits, the context comes from that buffer so it matches the screen.
func (w *Window) copyConflictForAI(t *diffTab, block int) {
	if t == nil || t.res == nil || t.res.Conflict == nil || w.ws.Repo == nil {
		return
	}
	cv := w.conflictViewForContext(t)
	if cv == nil || (block != vapp.ConflictContextAll && (block < 0 || block >= len(cv.Markers.Blocks))) {
		return
	}
	rs := w.ws.Repo
	text := vapp.ConflictContextText(cv, vapp.ConflictContextOptions{
		RepoRoot:  rs.Repo.Root,
		Operation: rs.Operation,
		Block:     block,
	})
	if text == "" {
		return
	}
	if err := w.setClipboardText(text); err != nil {
		w.ws.NotifyError(w.t(i18n.WorkspaceErrorCannotCopy), err)
		return
	}
	w.ws.Notify(vapp.NoticeInfo, w.t(i18n.ConflictNoticeCopied), w.t(i18n.ConflictNoticeCopiedBody))
}
