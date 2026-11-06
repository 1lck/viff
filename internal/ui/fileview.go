package viffui

import (
	"strings"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// The file view shows one version of a file read-only, like IntelliJ's
// "Open Repository Version", with an optional Git blame gutter.

// openFileView opens path at rev ("" for the working tree) in a pinned tab.
func (w *Window) openFileView(path, rev string, annotate bool) {
	w.showDiff(vapp.DiffSource{Path: path, Commit: rev, View: true}, true)
	if t := w.ed.cur(); t != nil && annotate && !t.annotate {
		w.toggleAnnotate(t)
	}
}

func (w *Window) toggleAnnotate(t *diffTab) {
	t.annotate = !t.annotate
	if t.annotate {
		w.loadBlame(t)
	}
}

func (w *Window) loadBlame(t *diffTab) {
	w.ed.seq++
	seq := w.ed.seq
	t.blameSeq = seq
	w.ws.Blame(t.src.Commit, t.src.Path, func(b *domain.Blame, err error) {
		if t.blameSeq != seq {
			return
		}
		t.blame, t.blameErr = b, err
		if b != nil {
			t.blameRank, t.blameN = b.AgeRank()
		}
	})
}

func (w *Window) paintFileView(t *diffTab, r ui.Rect) {
	res := t.res
	tb := splitTop(&r, 36)
	w.paintFileToolbar(t, tb)
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
	titles := splitTop(&r, 28)
	w.p.fill(titles, w.pal.IslandAlt)
	w.p.hline(titles.X, bottom(titles)-1, titles.Width, w.pal.Border)
	rev := w.t(i18n.FileviewWorkingTree)
	if t.src.Commit != "" {
		rev = "Revision " + domain.ShortHash(t.src.Commit)
	}
	w.paintTitle(rect(titles.X+10, titles.Y, titles.Width-20, titles.Height), t.src.Path, rev)
	if res.Notice != "" {
		w.paintCentered(r, res.Notice, "", w.pal.TextMuted)
		return
	}
	if t.annotate && t.blameErr != nil {
		br := splitTop(&r, 30)
		w.p.fill(br, w.pal.Banner)
		w.p.text(w.t(i18n.FileviewCannotAnnotate, vapp.ErrorText(t.blameErr)), rect(br.X+12, br.Y, br.Width-24, br.Height), fontUI, regular, w.pal.Error)
	}

	cellW, lineH, _ := w.metrics()
	content := float32(len(t.newDisp)+3) * lineH
	t.y = clampF(t.y, 0, max(0, content-r.Height))
	area := r
	if t.annotate && t.blame != nil {
		ar := splitLeft(&r, w.annotationWidth(t))
		w.paintAnnotations(t, ar, t.y, lineH)
	}
	_ = area
	// The code map takes its column off the text area's trailing edge, followed by
	// an independent scrollbar column on the outermost right edge.
	// Layout (left to right): [Code Editor] | [Codemap 72px] | [Scrollbar 14px]
	codemapRect := ui.Rect{}
	scrollRect := ui.Rect{}
	showMap := w.ws.Settings.DiffCodemap && !t.annotate
	if showMap {
		scrollRect = splitRight(&r, codemapBarW)
		codemapRect = splitRight(&r, codemapWidth)
	} else {
		scrollRect = r
	}
	gutter := w.gutterWidth(len(t.newDisp), cellW)
	maxX := max(0, float32(t.maxCells)*cellW+40-(r.Width-gutter))
	t.x = clampF(t.x, 0, maxX)
	w.onScrollIn(r, func(dx, dy float32) {
		t.x = clampF(t.x+dx, 0, maxX)
		t.y += dy
	})
	w.diffKeys(t, idDiffRight, lineH, r.Height)
	w.paintPane(t, r, false, t.y, gutter, idDiffRight)
	if showMap {
		w.paintCodemap(t, codemapRect, t.codeMap, t.y, r.Height, lineH, content)
	}
	w.vbar(idDiffScrollR, scrollRect, &scroller{Y: t.y}, content)
	w.wrapBarDrag(idDiffScrollR, func(y float32) { t.y = y }, func() float32 { return t.y }, content, r.Height)
}

func (w *Window) paintFileToolbar(t *diffTab, r ui.Rect) {
	x := r.X + 8
	label := w.t(i18n.FileviewAnnotate)
	if t.annotate {
		label = "Close Annotations"
	}
	bw := w.p.width(label, fontUI, regular) + 20
	disabled := t.res.Notice != ""
	if w.button(idFileAnnotate, rect(x, r.Y+(r.Height-26)/2, bw, 26), label, btnBorderless, disabled) {
		w.after(func() { w.toggleAnnotate(t) })
	}
	x += bw + 4
	hw := w.p.width(w.t(i18n.FileviewShowHistory), fontUI, regular) + 20
	if w.button(idFileHistory, rect(x, r.Y+(r.Height-26)/2, hw, 26), w.t(i18n.FileviewShowHistory), btnBorderless, false) {
		p := t.src.Path
		w.after(func() { w.showPathHistory(p) })
	}
	rx := right(r) - 8
	if t.src.Commit == "" {
		y := r.Y + (r.Height-actionSize)/2
		if w.action(idDiffOpen, rx-actionSize, y, icons.Edit, w.t(i18n.FileviewOpenExternal)+"\tF4", actOpts{}) {
			p := t.src.Path
			w.after(func() { w.openExternal(p, w.currentLine(t)) })
		}
		rx -= actionSize + 10
	}
	info := ""
	if t.res.Notice == "" {
		info = plural(len(t.newDisp), "line", "lines")
		if le := t.res.New.LineEnding; le != "" {
			info += "  ·  " + le
		}
	}
	if t.annotate && t.blame == nil && t.blameErr == nil {
		info = "Annotating…"
	}
	iw := w.p.width(info, fontUI, regular)
	if rx-iw > x+10 {
		w.p.text(info, rect(rx-iw, r.Y, iw+2, r.Height), fontUI, regular, w.pal.TextSecond)
	}
}

// annotationWidth fits the widest "date  author" of the blame.
func (w *Window) annotationWidth(t *diffTab) float32 {
	widest := float32(0)
	for _, c := range t.blame.Commits {
		widest = max(widest, w.p.width(annotationText(c), fontSmall, regular))
	}
	return min(max(widest+24, 120), 300)
}

func annotationText(c domain.BlameCommit) string {
	if c.Uncommitted() {
		return ""
	}
	return c.Time.Format("Jan 2, 2006") + "  " + c.Author
}

// paintAnnotations draws the blame gutter: each line's commit date and author,
// shaded by age, the hovered commit's lines highlighted, with the commit as a
// tooltip. Clicking shows the commit in the log.
func (w *Window) paintAnnotations(t *diffTab, r ui.Rect, top, lineH float32) {
	b := t.blame
	w.p.fill(r, w.pal.IslandAlt)
	w.c.HitWithOptions(idAnnotations, r, "Annotations", ui.HitOptions{SkipFocus: true, Cursor: ui.PointerCursorHand})
	hover := -1
	if w.hovered(idAnnotations) {
		line := int((w.mouse.Y - r.Y + top) / lineH)
		if line >= 0 && line < len(b.Lines) {
			hover = int(b.Lines[line])
		}
	}
	w.c.Clip(r)
	first := max(0, int(top/lineH))
	last := min(len(b.Lines), len(t.newDisp), int((top+r.Height)/lineH)+1)
	for i := first; i < last; i++ {
		y := r.Y + float32(i)*lineH - top
		ci := int(b.Lines[i])
		c := b.Commits[ci]
		if c.Uncommitted() {
			continue
		}
		// Newer commits get a stronger tint, like IntelliJ's age colouring.
		age := float32(1)
		if t.blameN > 1 {
			age = float32(t.blameRank[ci]) / float32(t.blameN-1)
		}
		w.p.fill(rect(r.X, y, 4, lineH), w.pal.Brand.WithAlpha(0.15+0.6*age))
		col := w.pal.TextMuted
		if ci == hover {
			w.p.fill(rect(r.X+4, y, r.Width-4, lineH), w.pal.Hover)
			col = w.pal.Text
		}
		// A run of lines from one commit is labelled once.
		if i > 0 && b.Lines[i-1] == b.Lines[i] && i != first {
			continue
		}
		w.p.text(annotationText(c), rect(r.X+12, y, r.Width-18, lineH), fontSmall, regular, col)
	}
	w.c.Unclip()
	w.p.vline(right(r)-1, r.Y, r.Height, w.pal.Border)
	if hover >= 0 {
		w.tip(idAnnotations, blameTip(b.Commits[hover]))
	}
	commitAt := func(y float32) (domain.BlameCommit, bool) {
		return b.CommitAt(int((y - r.Y + top) / lineH))
	}
	if w.c.Clicked(idAnnotations) {
		if c, ok := commitAt(w.mouse.Y); ok && !c.Uncommitted() {
			w.after(func() { w.showInLog(c.Hash) })
		}
	}
	w.onContext(idAnnotations, func(x, y float32) {
		c, ok := commitAt(y)
		if !ok || c.Uncommitted() {
			w.openMenu(x, y, []item{{title: w.t(i18n.FileviewCloseAnnotations), run: func() { t.annotate = false }}})
			return
		}
		short := domain.ShortHash(c.Hash)
		path := c.Path
		if path == "" {
			path = t.src.Path
		}
		w.openMenu(x, y, []item{
			{title: w.t(i18n.FileviewShowDiff), icon: icons.Diff, run: func() {
				w.showDiff(vapp.DiffSource{Path: path, OldPath: c.PreviousPath, Commit: c.Hash, Parent: c.Previous}, true)
			}},
			{title: w.t(i18n.FileviewShowInLog), run: func() { w.showInLog(c.Hash) }},
			{title: w.t(i18n.FileviewAnnotateRev, short), disabled: c.Previous == "", run: func() { w.openFileView(c.PreviousPath, c.Previous, true) }},
			sep(),
			{title: w.t(i18n.FileviewCopyRev), icon: icons.Copy, run: func() { w.copyText(c.Hash) }},
			sep(),
			{title: w.t(i18n.FileviewCloseAnnotations), run: func() { t.annotate = false }},
		})
	})
}

func blameTip(c domain.BlameCommit) string {
	var s strings.Builder
	s.WriteString(domain.ShortHash(c.Hash) + "  " + c.Author)
	if c.AuthorEmail != "" {
		s.WriteString(" <" + c.AuthorEmail + ">")
	}
	s.WriteString("\n" + c.Time.Format("Jan 2, 2006 at 15:04") + "\n\n" + c.Summary)
	return s.String()
}

// showInLog selects a commit in the log, searching for it when it is not
// loaded under the current filters.
func (w *Window) showInLog(hash string) {
	rs := w.repo()
	if rs == nil {
		return
	}
	w.gitOpen = true
	w.gitTab = 0
	v := &w.lg
	if i, ok := rs.Log.IndexOf(hash); ok {
		v.reveal, v.revealGen, v.revealTries = "", 0, 0
		clear(v.sel)
		v.sel[hash] = true
		v.anchor, v.cursor = hash, hash
		v.dtSel = -1
		v.scroll.reveal(float32(i)*logRowH, logRowH, v.viewH)
		w.ws.SelectCommit(hash)
		return
	}
	// Not loaded: add the hash to the text filter, which reads the commit even
	// when it starts outside the filtered history. The pending reveal selects
	// it once the reload arrives. Give up when the filter cannot change or the
	// commit is still missing, so a later reload does not re-apply the search.
	if rs.Log.Filter.Text == hash || v.revealTries >= 2 {
		v.reveal, v.revealGen, v.revealTries = "", 0, 0
		return
	}
	v.reveal, v.revealGen = hash, rs.Log.Gen
	v.revealTries++
	setText(v.search, hash)
	w.applyLogText()
}
