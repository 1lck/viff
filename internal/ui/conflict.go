package viffui

import (
	"path"

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

// The per-block resolve controls and the copy actions for an external AI tool
// have an identity scope of their own, made in init so the shared tree (ids.go)
// already has its features, and declared in this file rather than in ids.go:
// the conflict viewer's read-only identities stay unchanged.
var (
	scConflictResolve   ui.Scope
	idResolveOurs       ui.ID
	idResolveTheirs     ui.ID
	idResolveBoth       ui.ID
	idConflictCopyAI    ui.ID
	idConflictCopyBlock ui.ID
)

func init() {
	scConflictResolve = tree.Sub(fDiff, tree.Name("conflict-resolve"))
	idResolveOurs = scConflictResolve.ID(tree.Name("ours"))
	idResolveTheirs = scConflictResolve.ID(tree.Name("theirs"))
	idResolveBoth = scConflictResolve.ID(tree.Name("both"))
	idConflictCopyAI = scConflictResolve.ID(tree.Name("copy-ai"))
	idConflictCopyBlock = scConflictResolve.ID(tree.Name("copy-block"))
}

func (w *Window) conflictPanes(t *diffTab) []conflictPane {
	cv := t.res.Conflict
	mk := func(title string, s []string, base []string) conflictPane {
		p := conflictPane{title: title, disp: expandAll(s)}
		p.syn = highlight(t.src.Path, p.disp)
		p.mark = make([]uint8, len(s))
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
	cur := mk(w.t(i18n.ConflictLabelResult), cv.Current.Lines, nil)
	for _, b := range cv.Markers.Blocks {
		for k := b.Begin; k <= min(b.Finish, len(cur.mark)-1); k++ {
			cur.mark[k] = 2
		}
		for _, k := range []int{b.Begin, b.OursEnd, b.Finish} {
			if k >= 0 && k < len(cur.mark) {
				cur.mark[k] = 3
			}
		}
		if b.BaseStart >= 0 && b.BaseStart-1 < len(cur.mark) && b.BaseStart > 0 {
			cur.mark[b.BaseStart-1] = 3
		}
	}
	return []conflictPane{ours, cur, theirs}
}

func (w *Window) paintConflict(t *diffTab, r ui.Rect) {
	cv := t.res.Conflict
	rs := w.repo()
	// In-app resolution needs a readable working-tree text whose markers form
	// complete blocks. Anything else stays read-only here: a binary file, a
	// symlink or a submodule has no sides to accept as text, and markers the
	// parser cannot trust would make "block 2" mean the wrong lines.
	resolvable := cv.Conflict.Kind.Textual() && cv.Current.Content.Exists && cv.Current.Text &&
		!cv.Current.Content.Binary && !cv.Current.Content.TooLarge &&
		!cv.Current.Content.Submodule && !cv.Current.Content.Symlink &&
		cv.Markers.Reliable && len(cv.Markers.Blocks) > 0
	// The copied context is built from the same marker parse the accept
	// controls rely on: without a reliable parse a block would name the wrong
	// lines, so neither action is offered.
	canCopy := resolvable
	// Banner.
	bh := float32(88)
	if resolvable {
		bh = 122 // room for the per-block accept row
	}
	br := splitTop(&r, bh)
	inner := rect(br.X+10, br.Y+8, br.Width-20, bh-16)
	bg, border := w.pal.BannerWarn, w.pal.BannerWarnBorder
	resolved := cv.Resolved || (cv.Conflict.Kind.Textual() && len(cv.Markers.Blocks) == 0 && cv.Current.Content.Exists)
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
	} else if n := len(cv.Markers.Blocks); n > 0 {
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
		if !cv.Markers.Reliable {
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
	if w.button(idConflictStage, rect(bx, by, bw, 26), label, btnDefault, !resolved && cv.Conflict.Kind.Textual()) {
		p := t.src.Path
		w.after(func() { w.markResolved(p) })
	}
	if !resolved && cv.Conflict.Kind.Textual() {
		w.tip(idConflictStage, w.t(i18n.ConflictTipMarkAsResolved))
	}
	bx += bw + 8
	ow := w.buttonWidth(w.t(i18n.ConflictActionOpenEditor))
	if w.button(idConflictOpen, rect(bx, by, ow, 26), w.t(i18n.ConflictActionOpenEditor), btnNormal, !cv.Current.Content.Exists) {
		p := t.src.Path
		line := 0
		if t.conflictBlock < len(cv.Markers.Blocks) && len(cv.Markers.Blocks) > 0 {
			line = cv.Markers.Blocks[t.conflictBlock].Begin + 1
		}
		w.after(func() { w.openExternal(p, line) })
	}
	bx += ow + 8
	cw := w.buttonWidth(w.t(i18n.ConflictActionCopyPath))
	if w.button(idConflictCopy, rect(bx, by, cw, 26), w.t(i18n.ConflictActionCopyPath), btnBorderless, false) {
		full := path.Join(rs.Repo.Root, t.src.Path)
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
	n := len(cv.Markers.Blocks)
	if n > 0 {
		t.conflictBlock = min(t.conflictBlock, n-1)
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
	// Per-block resolution: accepting a side rewrites only that block on disk.
	if resolvable {
		rx := inner.X + 34
		label := w.t(i18n.ConflictAcceptTitle, itoa(t.conflictBlock+1), itoa(n))
		w.p.text(label, rect(rx, inner.Y+72, 230, 26), fontUI, regular, w.pal.TextMuted)
		rx += 240
		disabled := t.busy || t.loading
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
				w.after(func() { w.resolveConflictBlock(t, choice) })
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
	t.y = clampF(t.y, 0, max(0, content-r.Height))
	w.onScrollIn(r, func(dx, dy float32) { t.y += dy; t.x = max(0, t.x+dx) })
	w.diffKeys(t, idDiffRight, lineH, r.Height)
	for i, p := range panes {
		pr := rect(r.X+float32(i)*(pw+gap), r.Y, pw, r.Height)
		w.paintTitle(rect(pr.X+10, titles.Y, pr.Width-16, titles.Height), p.title, "")
		w.paintConflictPane(p, pr, t, cellW, lineH)
		if i > 0 {
			w.p.vline(pr.X-1, titles.Y, r.Height+titles.Height, w.pal.Border)
		}
	}
	sc := scroller{Y: t.y}
	w.vbar(idDiffScrollR, r, &sc, content)
	w.wrapBarDrag(idDiffScrollR, func(y float32) { t.y = y }, func() float32 { return t.y }, content, r.Height)
}

// conflictPanesCached keeps the panes with the tab between paints.
func (w *Window) conflictPanesCached(t *diffTab) []conflictPane {
	if c, ok := conflictCache[t]; ok && c.res == t.res && c.tr == w.tr {
		return c.panes
	}
	panes := w.conflictPanes(t)
	conflictCache[t] = conflictEntry{res: t.res, tr: w.tr, panes: panes}
	return panes
}

type conflictEntry struct {
	res   any
	tr    *i18n.Translator
	panes []conflictPane
}

var conflictCache = map[*diffTab]conflictEntry{}

func (w *Window) paintConflictPane(p conflictPane, r ui.Rect, t *diffTab, cellW, lineH float32) {
	_, _, fs := w.metrics()
	gutter := w.gutterWidth(len(p.disp), cellW)
	gr := rect(r.X, r.Y, gutter, r.Height)
	tr := rect(r.X+gutter, r.Y, r.Width-gutter, r.Height)
	style := ui.TextStyle{Size: fs, Color: w.pal.EditorText, Mono: true, MonoFace: w.mono}
	numStyle := ui.TextStyle{Size: fs - 1, Color: w.pal.LineNumber, Mono: true, MonoFace: w.mono}
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
	}
	w.c.Unclip()
	w.p.vline(gr.X+gr.Width-1, r.Y, r.Height, w.pal.Border)
}

func (w *Window) scrollToConflict(t *diffTab) {
	cv := t.res.Conflict
	if cv == nil || t.conflictBlock < 0 || t.conflictBlock >= len(cv.Markers.Blocks) {
		return
	}
	_, lineH, _ := w.metrics()
	t.y = max(0, float32(cv.Markers.Blocks[t.conflictBlock].Begin-4)*lineH)
}

// resolveConflictBlock accepts one side of the selected block in the working
// file. The workspace re-reads the file, refuses a stale or non-textual view
// and reports the reason; the tab is reloaded either way, so after an external
// edit the panes show what is actually on disk.
func (w *Window) resolveConflictBlock(t *diffTab, choice domain.ResolveChoice) {
	res := t.res
	if res == nil || res.Conflict == nil || t.loading || t.busy {
		return
	}
	block := t.conflictBlock
	if block < 0 || block >= len(res.Conflict.Markers.Blocks) {
		return
	}
	t.busy = true
	w.ws.ResolveConflict(res, block, choice, func(error) {
		t.busy = false
		w.loadTab(t)
	})
}

// copyConflictForAI builds the structured context of the conflict tab and puts
// it on the clipboard. Viff itself runs no model and opens no connection: the
// text is for whatever assistant the user pastes it into. block is a 0-based
// conflict block, or app.ConflictContextAll for the whole file.
func (w *Window) copyConflictForAI(t *diffTab, block int) {
	if t == nil || t.res == nil || t.res.Conflict == nil || w.ws.Repo == nil {
		return
	}
	rs := w.ws.Repo
	text := vapp.ConflictContextText(t.res.Conflict, vapp.ConflictContextOptions{
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
