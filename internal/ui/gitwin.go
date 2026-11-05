package viffui

import (
	"sort"
	"strings"
	"time"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// logView is the Log tab's view state.
type logView struct {
	search    *text.Editor
	queryAt   time.Duration
	sel       map[string]bool
	cursor    string
	anchor    string
	scroll    scroller
	gen       uint64
	brScroll  scroller
	brSel     string
	brClosed  map[string]bool
	dtScroll  scroller
	dtSel     int
	msgScroll scroller
	// msgLayout caches the commit message wrapped for the current panel width,
	// so a repaint does not rewrap it.
	msgLayout *commitMessageLayout
	favorites map[string]bool
	brRows    []brRow
	// reveal is a commit to select once the log shows it; viewH is the
	// table's last painted height. revealTries bounds how often showInLog
	// re-filters when the commit stays out of reach.
	reveal      string
	revealGen   uint64
	revealTries int
	viewH       float32
	// multi is the detail panel while more than one commit is selected: the
	// deduplicated union of the commits' first-parent changes.
	multi multiCommitView
}

type brRow struct {
	key    string
	title  string
	ref    *domain.Ref
	group  bool
	depth  int
	count  int
	remote string
}

type reflogView struct {
	entries []domain.ReflogEntry
	loaded  bool
	loading bool
	scroll  scroller
	sel     int
}

func (v *logView) init() {
	v.search = text.NewEditor("")
	v.sel = map[string]bool{}
	v.brClosed = map[string]bool{}
	v.favorites = map[string]bool{}
	v.dtSel = -1
}

func (v *logView) reset() {
	setText(v.search, "")
	clear(v.sel)
	v.cursor, v.anchor, v.brSel = "", "", ""
	v.scroll, v.brScroll, v.dtScroll = scroller{}, scroller{}, scroller{}
	v.dtSel = -1
	v.gen = 0
	v.multi.stop()
}

const (
	laneW    = 11
	logRowH  = 24
	authorW  = 150
	dateW    = 150
	maxLanes = 14
)

func (w *Window) paintGitWindow(r ui.Rect) {
	w.island(r)
	// Subversion has no reflog, so the tab is absent rather than always empty.
	tabs := []string{w.t(i18n.ChromeGitTabLog), w.t(i18n.ChromeGitTabConsole)}
	reflogTab := -1
	if w.ws.Caps.Reflog != nil {
		reflogTab = len(tabs)
		tabs = append(tabs, w.t(i18n.ChromeGitTabReflog))
	}
	content, ax := w.toolHeader(r, scGitTabs, tabs, w.gitTab, func(i int) {
		w.gitTab = i
		if i == reflogTab {
			w.loadReflog()
		}
	})
	ay := r.Y + (headerH-actionSize)/2
	more := rect(ax-actionSize*2-4, ay, actionSize, actionSize)
	if w.action(idOptGit, more.X, more.Y, icons.MoreHorizontal, w.t(i18n.ChromeGitActions), actOpts{}) {
		w.after(func() { w.openMenuBelow(rect(right(more)-200, ay, 200, actionSize), w.gitWindowMenu(), 220) })
	}
	if w.action(idHideGit, ax-actionSize, ay, icons.Hide, w.t(i18n.ChromeGitHide), actOpts{}) {
		w.after(func() { w.gitOpen = false })
	}
	w.p.hline(content.X, content.Y, content.Width, w.pal.Border)
	content.Y++
	content.Height--
	switch w.gitTab {
	case 1:
		w.paintConsole(content)
	case 2:
		w.paintReflog(content)
	default:
		w.paintLog(content)
	}
}

// ---- log ----------------------------------------------------------------

func (w *Window) paintLog(r ui.Rect) {
	s := w.ws.Settings
	bw := clampF(s.BranchesWidth, 160, max(160, r.Width/3))
	branches := splitLeft(&r, bw)
	w.paintBranches(branches)
	w.p.vline(right(branches), r.Y, r.Height, w.pal.Border)
	start := bw
	w.splitter(idSplitBranches, rect(right(branches)-3, r.Y, 6, r.Height), true,
		func() { start = bw },
		func(d float32) { s.BranchesWidth = clampF(start+d, 160, 520) }, w.saveSettings)
	dw := clampF(s.DetailsWidth, 260, max(260, r.Width/2))
	details := splitRight(&r, dw)
	w.p.vline(details.X, r.Y, r.Height, w.pal.Border)
	dstart := dw
	w.splitter(idSplitDetails, rect(details.X-3, r.Y, 6, r.Height), true,
		func() { dstart = dw },
		func(d float32) { s.DetailsWidth = clampF(dstart-d, 260, 700) }, w.saveSettings)
	w.paintDetails(rect(details.X+1, details.Y, details.Width-1, details.Height))

	bar := splitTop(&r, 38)
	w.paintLogFilters(bar)
	w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
	r.Y++
	r.Height--
	w.paintLogTable(r)
}

func (w *Window) paintLogFilters(r ui.Rect) {
	rs := w.repo()
	v := &w.lg
	f := rs.Log.Filter
	fr := rect(r.X+8, r.Y+5, min(260, r.Width*0.32), 28)
	if w.field(idLogSearch, v.search, fr, fieldOpts{icon: icons.Search, placeholder: w.t(i18n.ChromeLogPlaceholder)}) {
		w.after(func() { w.applyLogText() })
	}
	w.onKeys(idLogSearch, func(e ui.KeyEvent) bool {
		if e.Key == ui.KeyEscape && v.search.String() != "" {
			setText(v.search, "")
			w.applyLogText()
			return true
		}
		if e.Key == ui.KeyDown {
			w.focusNext = idLogTable
			return true
		}
		return false
	})
	// Apply typing after a short pause, as IntelliJ's log does.
	if q := strings.TrimSpace(v.search.String()); q != f.Text {
		now := w.c.Now()
		if v.queryAt == 0 {
			v.queryAt = now + 400*time.Millisecond
		}
		if now >= v.queryAt {
			w.after(w.applyLogText)
		} else {
			w.c.RequestFrameIn(v.queryAt - now)
		}
	} else {
		v.queryAt = 0
	}
	x := right(fr) + 8
	cy := r.Y + 6
	branchVal := f.Branch
	clicked, cw := w.filterChip(idLogBranchF, x, cy, 26, w.t(i18n.ChromeLogFilterBranch), branchVal)
	if clicked {
		ar := rect(x, cy, cw, 26)
		w.after(func() { w.openMenuBelow(ar, w.branchFilterMenu(), 240) })
	}
	x += cw + 2
	clicked, cw = w.filterChip(idLogUserF, x, cy, 26, w.t(i18n.ChromeLogFilterUser), f.Author)
	if clicked {
		ar := rect(x, cy, cw, 26)
		w.after(func() { w.openMenuBelow(ar, w.userFilterMenu(), 220) })
	}
	x += cw + 2
	clicked, cw = w.filterChip(idLogDateF, x, cy, 26, w.t(i18n.ChromeLogFilterDate), sinceTitle(f.Since))
	if clicked {
		ar := rect(x, cy, cw, 26)
		w.after(func() { w.openMenuBelow(ar, w.dateFilterMenu(), 180) })
	}
	x += cw + 2
	clicked, cw = w.filterChip(idLogPathF, x, cy, 26, w.t(i18n.ChromeLogFilterPaths), f.Path)
	if clicked {
		ar := rect(x, cy, cw, 26)
		w.after(func() { w.openPathFilter(ar) })
	}
	x += cw + 4
	ax := right(r) - 8 - actionSize
	ay := r.Y + (r.Height-actionSize)/2
	if w.action(idLogFetch, ax, ay, icons.Fetch, w.t(i18n.ChromeTbFetch), actOpts{disabled: len(rs.Remotes) == 0}) {
		w.after(func() { w.ws.Fetch(false) })
	}
	ax -= actionSize + 2
	if w.action(idLogRefresh, ax, ay, icons.Refresh, w.t(i18n.ChromeActionRefresh), actOpts{}) {
		w.after(func() { w.ws.ReloadLog(); w.ws.Refresh(true) })
	}
	if f.Active() {
		ax -= actionSize + 2
		if w.action(idLogClearF, ax, ay, icons.Close, w.t(i18n.ChromeLogClearFilters), actOpts{}) {
			w.after(func() {
				setText(v.search, "")
				v.brSel = ""
				w.ws.SetLogFilter(vapp.LogFilter{})
			})
		}
	}
}

func (w *Window) applyLogText() {
	rs := w.repo()
	if rs == nil {
		return
	}
	f := rs.Log.Filter
	f.Text = strings.TrimSpace(w.lg.search.String())
	w.lg.queryAt = 0
	w.ws.SetLogFilter(f)
}

func (w *Window) setFilter(mut func(f *vapp.LogFilter)) {
	f := w.repo().Log.Filter
	mut(&f)
	w.ws.SetLogFilter(f)
}

func sinceTitle(d time.Duration) string {
	switch d {
	case 0:
		return ""
	case 24 * time.Hour:
		return "Last 24 hours"
	case 7 * 24 * time.Hour:
		return "Last 7 days"
	case 30 * 24 * time.Hour:
		return "Last 30 days"
	case 365 * 24 * time.Hour:
		return "Last year"
	}
	return "Custom"
}

func (w *Window) branchFilterMenu() []item {
	rs := w.repo()
	cur := rs.Log.Filter.Branch
	items := []item{
		{title: w.t(i18n.ChromeLogFilterAll), checked: cur == "", run: func() { w.lg.brSel = ""; w.setFilter(func(f *vapp.LogFilter) { f.Branch = "" }) }},
		{title: "HEAD", checked: cur == "HEAD", run: func() { w.setFilter(func(f *vapp.LogFilter) { f.Branch = "HEAD" }) }},
	}
	add := func(title string, refs []domain.Ref) {
		if len(refs) == 0 {
			return
		}
		items = append(items, sep(), header(title))
		for _, ref := range refs {
			name := ref.Short
			icon := icons.BranchLabel
			if ref.Kind == domain.RefTag {
				icon = icons.BranchLabel
			}
			items = append(items, item{title: name, icon: icon, checked: cur == name, run: func() {
				w.lg.brSel = ref.Name
				w.setFilter(func(f *vapp.LogFilter) { f.Branch = name })
			}})
		}
	}
	add(w.t(i18n.ChromeRefsLocal), rs.RefsOf(domain.RefLocal))
	add(w.t(i18n.ChromeRefsRemote), rs.RefsOf(domain.RefRemote))
	add(w.t(i18n.ChromeRefsTags), rs.RefsOf(domain.RefTag))
	return items
}

func (w *Window) userFilterMenu() []item {
	rs := w.repo()
	cur := rs.Log.Filter.Author
	seen := map[string]int{}
	for _, c := range rs.Log.Commits {
		seen[c.Author]++
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if seen[names[i]] != seen[names[j]] {
			return seen[names[i]] > seen[names[j]]
		}
		return names[i] < names[j]
	})
	items := []item{{title: w.t(i18n.ChromeLogFilterAll), checked: cur == "", run: func() { w.setFilter(func(f *vapp.LogFilter) { f.Author = "" }) }}}
	if me := w.myName(); me != "" {
		items = append(items, item{title: w.t(i18n.ChromeLogFilterMe), hint: me, checked: cur == me, run: func() { w.setFilter(func(f *vapp.LogFilter) { f.Author = me }) }})
	}
	items = append(items, sep())
	for i, n := range names {
		if i >= 30 {
			break
		}
		n := n
		items = append(items, item{title: n, checked: cur == n, run: func() { w.setFilter(func(f *vapp.LogFilter) { f.Author = n }) }})
	}
	return items
}

// myName is the user.name of the repository's configuration.
func (w *Window) myName() string {
	rs := w.repo()
	for _, c := range rs.Log.Commits {
		if rs.HasHead() && c.Hash == rs.Status.Branch.Head {
			return c.Author
		}
	}
	return ""
}

func (w *Window) dateFilterMenu() []item {
	cur := w.repo().Log.Filter.Since
	opt := func(title string, d time.Duration) item {
		return item{title: title, checked: cur == d, run: func() { w.setFilter(func(f *vapp.LogFilter) { f.Since = d }) }}
	}
	return []item{opt(w.t(i18n.ChromeLogFilterAll), 0), opt(w.t(i18n.ChromeLogSinceDay), 24*time.Hour), opt(w.t(i18n.ChromeLogSinceWeek), 7*24*time.Hour), opt(w.t(i18n.ChromeLogSinceMonth), 30*24*time.Hour), opt(w.t(i18n.ChromeLogSinceYear), 365*24*time.Hour)}
}

func (w *Window) openPathFilter(anchor ui.Rect) {
	ed := text.NewEditor(w.repo().Log.Filter.Path)
	w.openDialog(&dialog{
		title: w.t(i18n.ChromeDialogFilterPath), ok: w.t(i18n.ChromeActionApply), width: 460,
		body: func(f *form) {
			f.muted(w.t(i18n.ChromeDialogFilterPathHint))
			id := f.field("", ed, "src/main", false)
			f.w.dialog.focus = id
		},
		extra: w.t(i18n.ChromeActionClear), onExtra: func() { w.dialog = nil; w.setFilter(func(f *vapp.LogFilter) { f.Path = "" }) },
		onOK: func() bool {
			p := strings.Trim(strings.TrimSpace(ed.String()), "/")
			w.setFilter(func(f *vapp.LogFilter) { f.Path = p })
			return true
		},
	})
}

// showPathHistory filters the log by one file.
func (w *Window) showPathHistory(p string) {
	w.gitOpen = true
	w.gitTab = 0
	w.setFilter(func(f *vapp.LogFilter) { f.Path = p })
}

func (w *Window) focusLogSearch() {
	w.gitOpen = true
	w.gitTab = 0
	w.focusNext = idLogSearch
}

func (w *Window) paintLogTable(r ui.Rect) {
	rs := w.repo()
	l := &rs.Log
	v := &w.lg
	if v.gen != l.Gen {
		v.gen = l.Gen
		v.scroll = scroller{}
		keep := map[string]bool{}
		for h := range v.sel {
			if _, ok := l.IndexOf(h); ok {
				keep[h] = true
			}
		}
		v.sel = keep
	}
	w.list(idLogTable, scLogRows, r, w.t(i18n.ChromeListCommits))
	w.onKeys(idLogTable, w.logKey)
	v.viewH = r.Height
	if v.reveal != "" && l.Gen != v.revealGen && l.Loaded() && !l.Loading {
		h := v.reveal
		v.reveal = ""
		if _, ok := l.IndexOf(h); ok {
			w.after(func() { w.showInLog(h) })
		}
	}
	n := len(l.Commits)
	content := float32(n) * logRowH
	if !l.Done {
		content += logRowH
	}
	v.scroll.clamp(content, r.Height)
	w.onScrollIn(r, func(dx, dy float32) { v.scroll.Y += dy; v.scroll.clamp(content, r.Height) })
	if n == 0 {
		msg := w.t(i18n.ChromeLogEmpty)
		switch {
		case l.Loading || !l.Loaded():
			msg = w.t(i18n.ChromeCommonLoading)
		case l.Err != nil:
			msg = w.ws.ErrorText(l.Err)
		case l.Filter.Active():
			msg = w.t(i18n.ChromeLogEmptyFiltered)
		case !rs.HasHead():
			msg = w.t(i18n.ChromeLogEmptyNoHead)
		}
		w.p.textCenter(msg, rect(r.X, r.Y+r.Height*0.35, r.Width, 22), fontUI, regular, w.pal.TextSecond)
		return
	}
	first := max(0, int(v.scroll.Y/logRowH))
	last := min(n, int((v.scroll.Y+r.Height)/logRowH)+1)
	if last >= n-40 && !l.Done && !l.Loading {
		w.after(w.ws.LoadMoreLog)
	}
	focused := w.focused(idLogTable)
	lanes := min(max(l.MaxWidth, 1), maxLanes)
	graphW := float32(lanes)*laneW + 10
	subjectW := r.Width - authorW - dateW
	head := rs.Status.Branch.Head
	now := time.Now()
	w.c.Clip(r)
	for i := first; i < last; i++ {
		c := l.Commits[i]
		y := r.Y + float32(i)*logRowH - v.scroll.Y
		rr := rect(r.X, y, r.Width, logRowH)
		id := scLogRows.At(i)
		selected := v.sel[c.Hash]
		if selected {
			col := w.pal.SelectionInac
			if focused {
				col = w.pal.Selection
			}
			w.p.fill(rr, col)
		} else if w.hovered(id) {
			w.p.fill(rr, w.pal.Hover)
		}
		if i < len(l.Rows) {
			w.paintGraphRow(l.Rows[i], rect(r.X+6, y, graphW, logRowH), c.Hash == head)
		}
		sx := r.X + 6 + graphW
		fg := w.pal.Text
		weight := float32(regular)
		if c.Hash == head {
			weight = semibold
		}
		refs := rs.RefsByHash[c.Hash]
		refsW := float32(0)
		if len(refs) > 0 {
			refsW = w.paintRefLabels(refs, rect(sx, y, subjectW-graphW-12, logRowH), true)
		}
		w.p.text(c.Subject, rect(sx, y, subjectW-graphW-12-refsW, logRowH), fontUI, weight, fg)
		w.p.text(c.Author, rect(r.X+subjectW, y, authorW-10, logRowH), fontUI, regular, w.pal.TextMuted)
		w.p.text(ago(c.AuthorTime, now), rect(r.X+subjectW+authorW, y, dateW-14, logRowH), fontUI, regular, w.pal.TextMuted)
		w.c.HitWithOptions(id, rr, c.Subject, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: selected})
		if w.c.Clicked(id) {
			i := i
			w.after(func() { w.clickCommit(i) })
		}
		hash := c.Hash
		w.onContext(id, func(x, y float32) {
			if !v.sel[hash] {
				clear(v.sel)
				v.sel[hash] = true
				v.cursor, v.anchor = hash, hash
				w.ws.SelectCommit(hash)
			}
			w.openMenu(x, y, w.commitMenu())
		})
	}
	if !l.Done && last >= n {
		w.p.textCenter(w.t(i18n.ChromeLogLoadingMore), rect(r.X, r.Y+float32(n)*logRowH-v.scroll.Y, r.Width, logRowH), fontSmall, regular, w.pal.TextSecond)
	}
	w.c.Unclip()
	w.vbar(scLogScrollbars.At(2), r, &v.scroll, content)
}

// paintGraphRow draws a commit graph row: lane edges and the node.
func (w *Window) paintGraphRow(row domain.GraphRow, r ui.Rect, head bool) {
	lx := func(lane int) float32 { return r.X + float32(lane)*laneW + laneW/2 }
	top, mid, bot := r.Y, r.Y+r.Height/2, r.Y+r.Height
	for _, e := range row.Edges {
		if e.From >= maxLanes || e.To >= maxLanes {
			continue
		}
		c := w.pal.Graph[e.Color%len(w.pal.Graph)]
		y0, y1 := top, mid
		if e.Lower {
			y0, y1 = mid, bot
		}
		x0, x1 := lx(e.From), lx(e.To)
		if x0 == x1 {
			w.p.fill(rect(x0-0.75, y0, 1.5, y1-y0), c)
			continue
		}
		bx := min(x0, x1) - 2
		by := y0 - 1
		bw := abs32(x1-x0) + 4
		bh := y1 - y0 + 2
		pb := w.c.PathBuilder()
		pb.MoveTo(x0-bx, y0-by)
		pb.LineTo(x1-bx, y1-by)
		w.c.DrawPath(pb.Path(bw, bh), rect(bx, by, bw, bh), ui.PathStyle{Stroke: c, StrokeWidth: 1.5, Cap: ui.CapRound})
	}
	if row.Lane < maxLanes {
		c := w.pal.Graph[row.Color%len(w.pal.Graph)]
		cx := lx(row.Lane)
		rad := float32(3.5)
		if row.IsMerge {
			rad = 3
		}
		if head {
			w.p.round(rect(cx-rad-1.5, mid-rad-1.5, 2*rad+3, 2*rad+3), rad+1.5, c)
			w.p.round(rect(cx-rad+0.5, mid-rad+0.5, 2*rad-1, 2*rad-1), rad, w.pal.Island)
			return
		}
		w.p.round(rect(cx-rad, mid-rad, 2*rad, 2*rad), rad, c)
	}
}

func abs32(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

// paintRefLabels draws branch and tag labels right-aligned in r and returns
// the width used.
func (w *Window) paintRefLabels(refs []domain.Ref, r ui.Rect, rightAlign bool) float32 {
	type lab struct {
		text string
		icon icons.Pair
		c    ui.Color
	}
	var labs []lab
	for _, ref := range refs {
		if ref.Kind == domain.RefHead || ref.Kind == domain.RefStash {
			continue
		}
		l := lab{text: ref.Short, icon: icons.BranchLabel, c: w.pal.RefLocal}
		switch ref.Kind {
		case domain.RefRemote:
			l.c = w.pal.RefRemote
		case domain.RefTag:
			l.icon, l.c = icons.BranchLabel, w.pal.RefTag
		}
		if ref.Current {
			l.icon, l.c = icons.CurrentBranchLabel, w.pal.RefHead
		}
		labs = append(labs, l)
	}
	if len(labs) == 0 {
		return 0
	}
	maxW := min(r.Width*0.5, 320)
	used := float32(0)
	shown := 0
	for _, l := range labs {
		lw := 18 + min(w.p.width(l.text, fontSmall, regular), 140) + 8
		if used+lw > maxW && shown > 0 {
			break
		}
		used += lw + 4
		shown++
	}
	extra := len(labs) - shown
	if extra > 0 {
		used += w.p.width("+"+itoa(extra), fontSmall, regular) + 8
	}
	x := right(r) - used
	if !rightAlign {
		x = r.X
	}
	for _, l := range labs[:shown] {
		tw := min(w.p.width(l.text, fontSmall, regular), 140)
		lr := rect(x, r.Y+(r.Height-18)/2, 18+tw+8, 18)
		w.p.round(lr, 4, w.pal.RefLabelBg)
		w.p.iconTinted(l.icon, rect(lr.X+2, lr.Y, 16, 18), l.c)
		w.p.text(l.text, rect(lr.X+18, lr.Y, tw+2, 18), fontSmall, regular, w.pal.TextMuted)
		x = right(lr) + 4
	}
	if extra > 0 {
		w.p.text("+"+itoa(extra), rect(x, r.Y, 40, r.Height), fontSmall, regular, w.pal.TextSecond)
	}
	return used + 8
}

func (w *Window) clickCommit(i int) {
	rs := w.repo()
	l := &rs.Log
	v := &w.lg
	if i >= len(l.Commits) {
		return
	}
	h := l.Commits[i].Hash
	switch {
	case w.mods&ui.ModifierShift != 0 && v.anchor != "":
		a, ok := l.IndexOf(v.anchor)
		if !ok {
			a = i
		}
		clear(v.sel)
		for k := min(a, i); k <= max(a, i); k++ {
			v.sel[l.Commits[k].Hash] = true
		}
	case w.mods&ui.ModifierMeta != 0:
		if v.sel[h] {
			delete(v.sel, h)
		} else {
			v.sel[h] = true
		}
		v.anchor = h
	default:
		clear(v.sel)
		v.sel[h] = true
		v.anchor = h
	}
	v.cursor = h
	v.dtSel = -1
	v.dtScroll = scroller{}
	if len(v.sel) == 1 {
		w.ws.SelectCommit(h)
	}
}

func (w *Window) logKey(e ui.KeyEvent) bool {
	rs := w.repo()
	l := &rs.Log
	v := &w.lg
	if len(l.Commits) == 0 {
		return false
	}
	cur, ok := l.IndexOf(v.cursor)
	if !ok {
		cur = -1
	}
	move := func(to int) {
		to = max(0, min(len(l.Commits)-1, to))
		h := l.Commits[to].Hash
		if e.Modifiers&ui.ModifierShift == 0 {
			clear(v.sel)
			v.anchor = h
		}
		v.sel[h] = true
		v.cursor = h
		v.scroll.reveal(float32(to)*logRowH, logRowH, w.areas[idLogTable].Height)
		v.dtSel = -1
		w.ws.SelectCommit(h)
	}
	page := int(w.areas[idLogTable].Height/logRowH) - 1
	switch e.Key {
	case ui.KeyDown:
		move(cur + 1)
	case ui.KeyUp:
		move(cur - 1)
	case ui.KeyPageDown:
		move(cur + page)
	case ui.KeyPageUp:
		move(cur - page)
	case ui.KeyHome:
		move(0)
	case ui.KeyEnd:
		move(len(l.Commits) - 1)
	default:
		return false
	}
	return true
}

// selectedCommits returns the selected commits in log order.
func (w *Window) selectedCommits() []domain.Commit {
	var out []domain.Commit
	for _, c := range w.repo().Log.Commits {
		if w.lg.sel[c.Hash] {
			out = append(out, c)
		}
	}
	return out
}

// commitMenu is the log's context menu.
func (w *Window) commitMenu() []item {
	rs := w.repo()
	sel := w.selectedCommits()
	if len(sel) == 0 {
		return nil
	}
	c := sel[0]
	single := len(sel) == 1
	short := domain.ShortHash(c.Hash)
	branch := rs.CurrentBranch()
	isHead := c.Hash == rs.Status.Branch.Head
	hashes := make([]string, len(sel))
	for i := range sel {
		hashes[len(sel)-1-i] = sel[i].Hash // oldest first
	}
	items := []item{
		{title: w.t(i18n.ChromeMenuCopyRevision), icon: icons.Copy, shortcut: "⌥⇧⌘C", run: func() {
			var s []string
			for _, x := range sel {
				s = append(s, x.Hash)
			}
			w.copyText(strings.Join(s, " "))
		}},
		sep(),
		{title: w.t(i18n.ChromeMenuCherryPick), icon: icons.CherryPick, disabled: rs.Operation.Active(), run: func() { w.ws.CherryPick(hashes) }},
		{title: w.t(i18n.ChromeMenuCheckoutRevision, short), disabled: !single, run: func() {
			w.confirmCheckoutRevision(c.Hash)
		}},
		{title: w.t(i18n.ChromeMenuShowDiffWorking), icon: icons.Diff, disabled: !single, run: func() { w.compareWithWorking(c.Hash) }},
	}
	if len(sel) == 2 {
		// The log lists the newest commit first; compare oldest → newest.
		older, newer := sel[1], sel[0]
		items = append(items, item{title: w.t(i18n.ChromeMenuCompareCommits), icon: icons.Diff, run: func() {
			w.openCompareCommits(older.Hash, newer.Hash)
		}})
	}
	// Rewriting committed history needs a backend that can rewrite it at all:
	// a Subversion revision is immutable once committed.
	canRewrite := w.ws.Caps.HistoryWriter != nil
	items = append(items,
		sep(),
		item{title: w.t(i18n.ChromeMenuResetHere), icon: icons.Reset, disabled: !canRewrite || !single || branch == "" && !rs.Status.Branch.Detached, run: func() { w.openReset(c) }},
		item{title: w.t(i18n.ChromeMenuRevert), icon: icons.Revert, disabled: !canRewrite || rs.Operation.Active(), run: func() { w.ws.RevertCommits(reverse(hashes)) }},
		item{title: w.t(i18n.ChromeMenuInteractiveRebase), disabled: !canRewrite || !single || branch == "" || rs.Operation.Active(), run: func() { w.openInteractiveRebase(c) }},
	)
	if canRewrite && single && isHead && len(c.Parents) == 1 && rs.Status.Branch.Ahead > 0 {
		items = append(items, item{title: w.t(i18n.ChromeMenuUndoCommit), icon: icons.Undo, run: func() {
			w.confirm(w.t(i18n.ChromeDialogUndoCommit), w.t(i18n.ChromeDialogUndoCommitMessage, c.Subject), w.t(i18n.ChromeDialogUndoCommit), func() {
				w.ws.Reset(gitcli.ResetSoft, "HEAD~1")
			})
		}})
	}
	if canRewrite && single {
		// Editing history replays the commits after the target, so it needs
		// a commit with exactly one parent and no operation in flight.
		canEdit := len(c.Parents) == 1 && !rs.Operation.Active()
		items = append(items, sep(),
			item{title: w.t(i18n.ChromeMenuEditCommitMessage), disabled: !canEdit, run: func() { w.openRewordCommit(c) }},
			item{title: w.t(i18n.ChromeMenuSquashIntoParent), disabled: !canEdit, run: func() { w.openSquashCommit(c) }},
			item{title: w.t(i18n.ChromeMenuDropCommit), disabled: !canEdit, run: func() { w.openDropCommit(c) }},
		)
	}
	items = append(items, sep(),
		item{title: w.t(i18n.ChromeMenuNewBranch), icon: icons.Add, disabled: !single, run: func() { w.openNewBranch(c.Hash, short) }},
		item{title: w.t(i18n.ChromeMenuNewTag), icon: icons.BranchLabel, disabled: !single, run: func() { w.openNewTag(c.Hash, short) }},
		// A merge commit has more than one history, so "up to" is ambiguous.
		item{title: w.t(i18n.ChromeMenuPushUpToCommit), icon: icons.Push, disabled: !single || len(c.Parents) > 1 || len(rs.Remotes) == 0, run: func() { w.openPushUpToCommit(c) }},
		item{title: w.t(i18n.ChromeMenuBrowseAtRevision), icon: icons.Folder, disabled: !single, run: func() { w.openRevisionBrowser(c) }},
	)
	// The service name comes from the remote, so the entry reads "Open Commit on
	// GitHub" and names the host for a self-hosted instance. It is omitted, not
	// disabled, when the remote has no web page at all.
	if link, err := rs.LinkFor(branch); err == nil {
		hash := c.Hash
		items = append(items, item{title: w.t(i18n.LinksOpenCommit, link.Label()), disabled: !single, run: func() {
			w.openCommitOnHost(hash)
		}})
	}
	return items
}

func reverse(s []string) []string {
	out := make([]string, len(s))
	for i, x := range s {
		out[len(s)-1-i] = x
	}
	return out
}

// compareWithWorking lists files that differ between a commit and the
// working tree and opens the first.
func (w *Window) confirmCheckoutRevision(hash string) {
	short := domain.ShortHash(hash)
	w.confirm(w.t(i18n.ChromeDialogCheckoutRevision), w.t(i18n.ChromeDialogCheckoutRevisionMessage, short), w.t(i18n.ChromeActionCheckout), func() {
		w.ws.CheckoutRevision(hash)
	})
}

func (w *Window) compareWithWorking(hash string) {
	w.ws.CompareWith(hash, "", func(changes []domain.Change, err error) {
		if err != nil {
			w.ws.NotifyError(w.t(i18n.ChromeNotifyCannotCompare), err)
			return
		}
		if len(changes) == 0 {
			w.ws.Notify(vapp.NoticeInfo, w.t(i18n.ChromeNotifyNoDifferences), w.t(i18n.ChromeNotifyNoDifferencesMessage, domain.ShortHash(hash)))
			return
		}
		var items []item
		for _, ch := range changes {
			ch := ch
			items = append(items, item{title: domain.FileName(ch.Path), icon: fileIcon(ch.Path), hint: domain.DirName(ch.Path), filter: ch.Path, run: func() {
				w.showDiff(vapp.DiffSource{Path: ch.Path, OldPath: ch.OldPath, Commit: "", Parent: hash, Kind: ch.Kind}, true)
			}})
		}
		p := &popup{x: w.size.Width/2 - 220, y: 120, items: items, sel: -1, minWidth: 440, title: w.t(i18n.ChromePopupChanges), search: text.NewEditor("")}
		w.popups = []*popup{p}
	})
}

// ---- branches tree ----------------------------------------------------------

func (w *Window) buildBranchRows() {
	rs := w.repo()
	v := &w.lg
	v.brRows = v.brRows[:0]
	if cur, ok := rs.CurrentRef(); ok {
		v.brRows = append(v.brRows, brRow{key: "HEAD", title: w.t(i18n.ChromeRefsHeadCurrent), ref: &cur})
	} else if rs.HasHead() {
		v.brRows = append(v.brRows, brRow{key: "HEAD", title: w.t(i18n.ChromeRefsHeadHash, domain.ShortHash(rs.Status.Branch.Head))})
	}
	locals := rs.RefsOf(domain.RefLocal)
	v.brRows = append(v.brRows, brRow{key: "g:local", title: w.t(i18n.ChromeRefsLocal), group: true, count: len(locals)})
	if !v.brClosed["g:local"] {
		for i := range locals {
			v.brRows = append(v.brRows, brRow{key: locals[i].Name, title: locals[i].Short, ref: &locals[i], depth: 1})
		}
	}
	remotes := rs.RefsOf(domain.RefRemote)
	if len(remotes) > 0 {
		v.brRows = append(v.brRows, brRow{key: "g:remote", title: w.t(i18n.ChromeRefsRemote), group: true, count: len(remotes)})
		if !v.brClosed["g:remote"] {
			byRemote := map[string][]int{}
			for i, r := range remotes {
				byRemote[r.Remote()] = append(byRemote[r.Remote()], i)
			}
			for _, name := range sortedKeys(byRemote) {
				key := "g:remote:" + name
				v.brRows = append(v.brRows, brRow{key: key, title: name, group: true, depth: 1, count: len(byRemote[name]), remote: name})
				if v.brClosed[key] {
					continue
				}
				for _, i := range byRemote[name] {
					v.brRows = append(v.brRows, brRow{key: remotes[i].Name, title: remotes[i].BranchName(), ref: &remotes[i], depth: 2})
				}
			}
		}
	}
	tags := rs.RefsOf(domain.RefTag)
	if len(tags) > 0 {
		v.brRows = append(v.brRows, brRow{key: "g:tags", title: w.t(i18n.ChromeRefsTags), group: true, count: len(tags)})
		if !v.brClosed["g:tags"] {
			for i := range tags {
				v.brRows = append(v.brRows, brRow{key: tags[i].Name, title: tags[i].Short, ref: &tags[i], depth: 1})
			}
		}
	}
}

func (w *Window) paintBranches(r ui.Rect) {
	v := &w.lg
	rs := w.repo()
	tb := splitTop(&r, 38)
	x, y := tb.X+8, tb.Y+(tb.Height-actionSize)/2
	if w.action(idBrNew, x, y, icons.Add, w.t(i18n.ChromeMenuNewBranch), actOpts{disabled: !rs.HasHead()}) {
		w.after(func() { w.openNewBranch("", rs.HeadLabel()) })
	}
	x += actionSize + 2
	if w.action(idBrFetch, x, y, icons.Fetch, w.t(i18n.ChromeTbFetch), actOpts{disabled: len(rs.Remotes) == 0}) {
		w.after(func() { w.ws.Fetch(false) })
	}
	w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
	r.Y++
	r.Height--
	w.buildBranchRows()
	w.list(idBranchesTree, scBranchRows, r, w.t(i18n.ChromeListBranches))
	content := float32(len(v.brRows))*rowH + 8
	v.brScroll.clamp(content, r.Height)
	w.onScrollIn(r, func(dx, dy float32) { v.brScroll.Y += dy; v.brScroll.clamp(content, r.Height) })
	focused := w.focused(idBranchesTree)
	w.c.Clip(r)
	yy := r.Y + 4 - v.brScroll.Y
	for i, row := range v.brRows {
		rr := rect(r.X, yy, r.Width, rowH)
		yy += rowH
		if bottom(rr) < r.Y || rr.Y > bottom(r) {
			continue
		}
		id := scBranchRows.At(i)
		hl := rect(rr.X+6, rr.Y, rr.Width-12, rr.Height)
		selected := v.brSel == row.key && row.key != ""
		if selected {
			c := w.pal.SelectionInac
			if focused {
				c = w.pal.Selection
			}
			w.p.round(hl, 4, c)
		} else if w.hovered(id) {
			w.p.round(hl, 4, w.pal.Hover)
		}
		ix := rr.X + 10 + float32(row.depth)*16
		if row.group {
			chev := icons.ChevronDown
			if v.brClosed[row.key] {
				chev = icons.ChevronRight
			}
			w.p.iconIn(chev, rect(ix, rr.Y, 16, rowH))
			ix += 18
			icon := icons.Folder
			if row.remote != "" {
				icon = icons.FolderGithub
			}
			if row.depth > 0 {
				w.p.iconIn(icon, rect(ix, rr.Y, 16, rowH))
				ix += 20
			}
			tw := w.p.text(row.title, rect(ix, rr.Y, right(rr)-ix-10, rowH), fontUI, regular, w.pal.Text)
			_ = tw
		} else {
			icon, tint := icons.BranchLabel, w.pal.RefLocal
			if row.ref != nil {
				switch {
				case row.ref.Current:
					icon, tint = icons.CurrentBranchLabel, w.pal.RefHead
				case row.ref.Kind == domain.RefRemote:
					tint = w.pal.RefRemote
				case row.ref.Kind == domain.RefTag:
					icon, tint = icons.BranchLabel, w.pal.RefTag
				}
			} else {
				tint = w.pal.RefHead
			}
			if row.key == "HEAD" {
				w.p.iconTinted(icons.CurrentBranchLabel, rect(ix, rr.Y, 16, rowH), w.pal.RefHead)
			} else {
				w.p.iconTinted(icon, rect(ix, rr.Y, 16, rowH), tint)
			}
			ix += 22
			tw := w.p.text(row.title, rect(ix, rr.Y, right(rr)-ix-10, rowH), fontUI, regular, w.pal.Text)
			if row.ref != nil && row.ref.Kind == domain.RefLocal {
				hint := ""
				if row.ref.Gone {
					hint = w.t(i18n.ChromeRefsGoneWord)
				} else if row.ref.Ahead > 0 || row.ref.Behind > 0 {
					if row.ref.Ahead > 0 {
						hint += "↑" + itoa(row.ref.Ahead)
					}
					if row.ref.Behind > 0 {
						hint += " ↓" + itoa(row.ref.Behind)
					}
				}
				if hint != "" {
					w.p.text(strings.TrimSpace(hint), rect(ix+tw+6, rr.Y, right(rr)-ix-tw-12, rowH), fontSmall, regular, w.pal.Link)
				}
			}
		}
		w.c.HitWithOptions(id, hl, row.title, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: selected})
		if w.c.Clicked(id) {
			row := row
			w.after(func() { w.clickBranchRow(row) })
		}
		if row.ref != nil && row.key != "HEAD" {
			ref := *row.ref
			w.onDouble(id, func() {
				if ref.Kind != domain.RefTag && !ref.Current {
					w.ws.Checkout(ref)
				}
			})
			w.onContext(id, func(x, y float32) { w.openMenu(x, y, w.refActions(ref)) })
		}
	}
	w.c.Unclip()
	w.vbar(scLogScrollbars.At(3), r, &v.brScroll, content)
}

func (w *Window) clickBranchRow(row brRow) {
	v := &w.lg
	if row.group {
		v.brClosed[row.key] = !v.brClosed[row.key]
		return
	}
	if v.brSel == row.key {
		v.brSel = ""
		w.setFilter(func(f *vapp.LogFilter) { f.Branch = "" })
		return
	}
	v.brSel = row.key
	name := "HEAD"
	if row.ref != nil && row.key != "HEAD" {
		name = row.ref.Short
	}
	w.setFilter(func(f *vapp.LogFilter) { f.Branch = name })
}

// ---- details --------------------------------------------------------------

func (w *Window) paintDetails(r ui.Rect) {
	rs := w.repo()
	v := &w.lg
	sel := w.selectedCommits()
	n := len(sel)
	if n > 1 {
		// The union of each commit's first-parent changes, not a diff between
		// the first and last selection.
		w.paintMultiCommitDetails(r, sel)
		return
	}
	// A single commit ends any multi-commit query in flight, so its Git
	// processes stop instead of finishing a selection the user has left.
	v.multi.stop()
	if n == 0 || rs.Detail == nil && !rs.DetailLoading {
		w.p.textCenter(w.t(i18n.ChromeDetailsEmpty), rect(r.X, r.Y+r.Height*0.4, r.Width, 22), fontUI, regular, w.pal.TextSecond)
		return
	}
	d := rs.Detail
	if d == nil || d.Hash != rs.DetailHash {
		w.p.textCenter(w.t(i18n.ChromeCommonLoading), rect(r.X, r.Y+r.Height*0.4, r.Width, 22), fontUI, regular, w.pal.TextSecond)
		return
	}
	// Files on top, message below.
	msgH := clampF(r.Height*0.42, 110, 260)
	files := rect(r.X, r.Y, r.Width, r.Height-msgH)
	msg := rect(r.X, bottom(files), r.Width, msgH)
	w.p.hline(msg.X, msg.Y, msg.Width, w.pal.Border)

	w.list(idDetailsFiles, scDetailRows, files, w.t(i18n.ChromeListChangedFiles))
	content := float32(len(d.Changes))*rowH + 30
	v.dtScroll.clamp(content, files.Height)
	w.onScrollIn(files, func(dx, dy float32) { v.dtScroll.Y += dy; v.dtScroll.clamp(content, files.Height) })
	focused := w.focused(idDetailsFiles)
	w.onKeys(idDetailsFiles, func(e ui.KeyEvent) bool {
		switch e.Key {
		case ui.KeyDown:
			v.dtSel = min(len(d.Changes)-1, v.dtSel+1)
		case ui.KeyUp:
			v.dtSel = max(0, v.dtSel-1)
		case ui.KeyEnter:
		default:
			return false
		}
		if v.dtSel >= 0 && v.dtSel < len(d.Changes) {
			w.showCommitFile(d, v.dtSel, e.Key == ui.KeyEnter)
		}
		return true
	})
	w.c.Clip(files)
	y := files.Y + 4 - v.dtScroll.Y
	w.p.text(w.t(i18n.ChromeDetailsFilesChanged, len(d.Changes)), rect(files.X+12, y, files.Width-24, 24), fontSmall, regular, w.pal.TextSecond)
	y += 24
	for i, ch := range d.Changes {
		rr := rect(files.X, y, files.Width, rowH)
		y += rowH
		if bottom(rr) < files.Y || rr.Y > bottom(files) {
			continue
		}
		id := scDetailRows.At(i)
		hl := rect(rr.X+6, rr.Y, rr.Width-12, rr.Height)
		if v.dtSel == i {
			c := w.pal.SelectionInac
			if focused {
				c = w.pal.Selection
			}
			w.p.round(hl, 4, c)
		} else if w.hovered(id) {
			w.p.round(hl, 4, w.pal.Hover)
		}
		x := rr.X + 14
		w.p.iconIn(fileIcon(ch.Path), rect(x, rr.Y, 16, rowH))
		x += 22
		fg := w.statusColor(ch.Kind)
		tw := w.p.text(domain.FileName(ch.Path), rect(x, rr.Y, right(rr)-x-10, rowH), fontUI, regular, fg)
		hint := domain.DirName(ch.Path)
		if ch.OldPath != "" {
			hint = "← " + ch.OldPath
		}
		if hint != "" {
			hx := x + tw + 8
			w.p.text(w.p.truncateMiddle(hint, fontUI, regular, max(0, right(rr)-hx-12)), rect(hx, rr.Y, right(rr)-hx-10, rowH), fontUI, regular, w.pal.TextSecond)
		}
		w.c.HitWithOptions(id, hl, ch.Path, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: v.dtSel == i})
		if w.c.Clicked(id) {
			i := i
			w.after(func() { v.dtSel = i; w.showCommitFile(d, i, false) })
		}
		ii := i
		w.onDouble(id, func() { v.dtSel = ii; w.showCommitFile(d, ii, true) })
		chPath := ch.Path
		deleted := ch.Kind == domain.ChangeDeleted
		hash := d.Hash
		w.onContext(id, func(x, y float32) {
			v.dtSel = ii
			w.openMenu(x, y, []item{
				{title: w.t(i18n.ChromeMenuShowDiff), icon: icons.Diff, shortcut: "⌘D", run: func() { w.showCommitFile(d, ii, true) }},
				{title: w.t(i18n.ChromeMenuOpenRepoVersion), disabled: deleted, run: func() { w.openFileView(chPath, hash, false) }},
				{title: w.t(i18n.ChromeMenuAnnotateRevision), disabled: deleted, run: func() { w.openFileView(chPath, hash, true) }},
				{title: w.t(i18n.ChromeMenuShowHistory), icon: icons.History, run: func() { w.showPathHistory(chPath) }},
				sep(),
				{title: w.t(i18n.ChromeMenuCopyRelativePath), icon: icons.Copy, run: func() { w.copyText(chPath) }},
			})
		})
	}
	w.c.Unclip()
	w.vbar(scLogScrollbars.At(4), files, &v.dtScroll, content)

	w.paintLogCommitMessage(msg, d)
}

func (w *Window) showCommitFile(d *domain.CommitDetail, i int, pin bool) {
	if i < 0 || i >= len(d.Changes) {
		return
	}
	ch := d.Changes[i]
	parent := ""
	if len(d.Parents) > 0 {
		parent = d.Parents[0]
	}
	w.showDiff(vapp.DiffSource{Path: ch.Path, OldPath: ch.OldPath, Commit: d.Hash, Parent: parent, Kind: ch.Kind}, pin)
}

// ---- console ----------------------------------------------------------------

// ---- reflog -----------------------------------------------------------------

func (w *Window) loadReflog() {
	v := &w.rf
	if v.loading {
		return
	}
	v.loading = true
	w.ws.Reflog(func(entries []domain.ReflogEntry, err error) {
		v.loading = false
		v.loaded = true
		if err != nil {
			w.ws.NotifyError(w.t(i18n.ChromeNotifyCannotReadReflog), err)
			return
		}
		v.entries = entries
	})
}

func (w *Window) paintReflog(r ui.Rect) {
	v := &w.rf
	if !v.loaded && !v.loading {
		w.after(w.loadReflog)
	}
	tb := splitTop(&r, 34)
	w.p.text(w.t(i18n.ChromeReflogHint), rect(tb.X+12, tb.Y, tb.Width-60, tb.Height), fontSmall, regular, w.pal.TextSecond)
	if w.action(scStashActions.At(9), right(tb)-8-actionSize, tb.Y+4, icons.Refresh, w.t(i18n.ChromeActionRefresh), actOpts{}) {
		w.after(w.loadReflog)
	}
	w.list(idReflogList, scReflogRows, r, w.t(i18n.ChromeGitTabReflog))
	content := float32(len(v.entries))*rowH + 8
	v.scroll.clamp(content, r.Height)
	w.onScrollIn(r, func(dx, dy float32) { v.scroll.Y += dy; v.scroll.clamp(content, r.Height) })
	if len(v.entries) == 0 {
		msg := w.t(i18n.ChromeReflogEmpty)
		if v.loading {
			msg = w.t(i18n.ChromeCommonLoading)
		}
		w.p.textCenter(msg, rect(r.X, r.Y+r.Height*0.35, r.Width, 22), fontUI, regular, w.pal.TextSecond)
		return
	}
	focused := w.focused(idReflogList)
	now := time.Now()
	w.c.Clip(r)
	_, _, fs := w.metrics()
	for i, e := range v.entries {
		y := r.Y + 4 + float32(i)*rowH - v.scroll.Y
		if y+rowH < r.Y || y > bottom(r) {
			continue
		}
		rr := rect(r.X, y, r.Width, rowH)
		id := scReflogRows.At(i)
		if v.sel == i {
			c := w.pal.SelectionInac
			if focused {
				c = w.pal.Selection
			}
			w.p.fill(rr, c)
		} else if w.hovered(id) {
			w.p.fill(rr, w.pal.Hover)
		}
		w.c.Label(e.Selector, rect(rr.X+12, y, 110, rowH), ui.TextStyle{Size: fs - 1, Color: w.pal.TextMuted, Mono: true, MonoFace: w.mono})
		w.c.Label(domain.ShortHash(e.Hash), rect(rr.X+124, y, 80, rowH), ui.TextStyle{Size: fs - 1, Color: w.pal.Link, Mono: true, MonoFace: w.mono})
		w.p.text(e.Message, rect(rr.X+210, y, rr.Width-210-dateW, rowH), fontUI, regular, w.pal.Text)
		w.p.text(ago(e.Time, now), rect(right(rr)-dateW, y, dateW-12, rowH), fontUI, regular, w.pal.TextMuted)
		w.c.HitWithOptions(id, rr, e.Message, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: v.sel == i})
		if w.c.Clicked(id) {
			i := i
			w.after(func() { v.sel = i })
		}
		hash := e.Hash
		short := domain.ShortHash(hash)
		ii := i
		w.onContext(id, func(x, y float32) {
			v.sel = ii
			w.openMenu(x, y, []item{
				{title: w.t(i18n.ChromeMenuCopyRevision), icon: icons.Copy, run: func() { w.copyText(hash) }},
				{title: w.t(i18n.ChromeMenuNewBranch), icon: icons.Add, run: func() { w.openNewBranch(hash, short) }},
				{title: w.t(i18n.ChromeMenuCheckoutRevision, short), run: func() { w.confirmCheckoutRevision(hash) }},
				{title: w.t(i18n.ChromeMenuResetHere), icon: icons.Reset, run: func() { w.openReset(domain.Commit{Hash: hash, Subject: e.Message}) }},
			})
		})
	}
	w.c.Unclip()
	w.vbar(scLogScrollbars.At(6), r, &v.scroll, content)
}
