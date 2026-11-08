package viffui

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/ai"
	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/sshpath"
	"viff/internal/ui/icons"
)

// commitView is the Commit tool window's view state.
type commitView struct {
	msg          *text.Editor
	layout       components.TextareaLayout
	amend        bool
	saved        string // the message typed before Amend loaded HEAD's
	loaded       string // HEAD's message as loaded for Amend
	signOff      bool
	noVerify     bool
	collapsed    map[string]bool
	sel          map[string]bool
	anchor       string
	cursor       string
	scroll       scroller
	rows         []changeRow
	msgH         float32
	msgCollapsed bool
	aiBusy       bool
	aiLast       string // last generated message, so a rerun asks for other wording
	// aiRect is where the AI button was last painted. The confirmation bubble
	// opens just above it, so the anchor is the button the user actually
	// clicked rather than a fixed guess at its position.
	aiRect ui.Rect
	// listID filters the changes tree by change list. "" shows every list,
	// which keeps the historical behavior; any other value is the selected
	// list's ID.
	listID string
}

// changeRow is one row of the staging tree.
type changeRow struct {
	key   string
	area  domain.Area
	kind  int // rowGroup, rowDir, rowFile
	dir   string
	entry domain.StatusEntry
	count int
	depth int
	// files are the selection keys a group or directory row covers. They are
	// kept even while the row is collapsed, so its checkbox can still toggle
	// files that have no row of their own.
	files []string
}

const (
	rowGroup = iota
	rowDir
	rowFile
)

var areaOrder = []domain.Area{domain.AreaConflicts, domain.AreaStaged, domain.AreaUnstaged, domain.AreaUntracked}

// areaTitle names the change-tree group a file belongs to.
func (w *Window) areaTitle(a domain.Area) string {
	group := i18n.WorkspaceAreaConflicts
	switch a {
	case domain.AreaStaged:
		group = i18n.WorkspaceAreaStaged
	case domain.AreaUnstaged:
		group = i18n.WorkspaceAreaChanges
	case domain.AreaUntracked:
		group = i18n.WorkspaceAreaUntracked
	}
	return w.t(group)
}

// pluralT fills in n and picks the one or many message the way i18n.Plural
// does; Chinese repeats its single form in both keys.
func (w *Window) pluralT(n int, one, many i18n.Key, args ...any) string {
	key := many
	if n == 1 {
		key = one
	}
	return w.t(key, append([]any{n}, args...)...)
}

func (v *commitView) init() {
	v.msg = text.NewEditor("")
	v.collapsed = map[string]bool{}
	v.sel = map[string]bool{}
	v.msgH = 116
}

func (v *commitView) reset() {
	setText(v.msg, "")
	v.msg.ClearHistory()
	v.amend = false
	v.saved, v.loaded = "", ""
	clear(v.sel)
	clear(v.collapsed)
	v.anchor, v.cursor = "", ""
	v.scroll = scroller{}
	v.listID = ""
}

func rowKey(a domain.Area, p string) string { return string(rune('0'+a)) + ":" + p }

// buildRows flattens the status into tree rows.
func (w *Window) buildRows() {
	v := &w.cm
	v.rows = v.rows[:0]
	tree := w.ws.Settings.CommitTree
	// A backend without a staging area (Subversion) reports every change in
	// the unstaged column, so a "Staged" group would always be empty. Skipping
	// it keeps one list instead of a permanent dead section.
	indexable := w.ws.Caps.Index
	for _, a := range areaOrder {
		if a == domain.AreaStaged && !indexable {
			continue
		}
		entries := w.changeListEntries(a)
		if len(entries) == 0 && a != domain.AreaStaged && a != domain.AreaUnstaged {
			continue
		}
		if len(entries) == 0 && a == domain.AreaStaged {
			continue
		}
		gk := rowKey(a, "")
		v.rows = append(v.rows, changeRow{key: gk, area: a, kind: rowGroup, count: len(entries), files: entryKeys(a, entries)})
		if v.collapsed[gk] {
			continue
		}
		if !tree {
			for _, e := range entries {
				v.rows = append(v.rows, changeRow{key: rowKey(a, e.Path), area: a, kind: rowFile, entry: e, depth: 1})
			}
			continue
		}
		byDir := map[string][]domain.StatusEntry{}
		for _, e := range entries {
			d := domain.DirName(e.Path)
			byDir[d] = append(byDir[d], e)
		}
		dirs := sortedKeys(byDir)
		for _, d := range dirs {
			files := byDir[d]
			depth := 1
			if d != "" {
				dk := rowKey(a, d+"/")
				v.rows = append(v.rows, changeRow{key: dk, area: a, kind: rowDir, dir: d, count: len(files), depth: 1, files: entryKeys(a, files)})
				if v.collapsed[dk] {
					continue
				}
				depth = 2
			}
			for _, e := range files {
				v.rows = append(v.rows, changeRow{key: rowKey(a, e.Path), area: a, kind: rowFile, entry: e, depth: depth})
			}
		}
	}
}

func (w *Window) paintCommitWindow(r ui.Rect) {
	w.island(r)
	// Subversion has no stash, so the tab is absent rather than empty. The
	// second tab's index stays 1 whenever it exists, which is what commitTab
	// and paintStash rely on.
	tabs := []string{w.t(i18n.WorkspacePanelCommit)}
	if w.ws.Caps.Stash != nil && !w.readOnly() {
		tabs = append(tabs, w.t(i18n.WorkspacePanelStash))
	}
	if w.readOnly() {
		w.commitTab = 0
	}
	content, ax := w.toolHeader(r, idBanner, tabs, w.commitTab, func(i int) { w.commitTab = i })
	if w.action(idHideCommit, ax-actionSize, r.Y+(headerH-actionSize)/2, icons.Hide, w.t(i18n.WorkspaceActionHide)+"\t⇧⎋", actOpts{}) {
		w.after(func() { w.commitOpen = false })
	}
	if w.commitTab == 1 {
		w.paintStash(content)
		return
	}
	rs := w.repo()
	if rs.Operation.Active() {
		content = w.paintOperationBanner(content)
	}
	// The LFS warning sits below the operation banner: an in-progress merge or
	// rebase is the more urgent state, and both clear once the effective
	// problem is resolved.
	if rs.LFS.NeedsInstall() {
		content = w.paintLFSBanner(content)
	}
	if rs.Repo.IsSSH() {
		content = w.paintRemoteBanner(content)
	}

	// Toolbar.
	tb := splitTop(&content, 34)
	x, y := tb.X+8, tb.Y+(tb.Height-actionSize)/2
	if w.action(idCmRefresh, x, y, icons.Refresh, w.t(i18n.WorkspaceActionRefresh), actOpts{}) {
		w.after(func() { w.ws.Refresh(true) })
	}
	x += actionSize + 2
	sel := w.selectedEntries()
	if w.action(idCmRollback, x, y, icons.Revert, w.t(i18n.WorkspaceActionRollback)+"\t⌥⌘Z", actOpts{disabled: w.readOnly() || len(w.rollbackTargets()) == 0}) {
		w.after(w.openRollback)
	}
	x += actionSize + 2
	if w.action(idCmDiff, x, y, icons.Diff, w.t(i18n.WorkspaceActionShowDiff)+"\t⌘D", actOpts{disabled: len(sel) == 0}) {
		w.after(func() { w.openSelectedDiff(true) })
	}
	x += actionSize + 8
	w.p.vline(x-4, y+5, actionSize-10, w.pal.Separator)
	staged, _, _, _ := rs.Status.Counts()
	// Stage All follows the filter: with a list selected it stages only the
	// files that list shows, never the hidden ones.
	if w.action(idCmStage, x, y, icons.Add, w.t(i18n.WorkspaceActionStageAll), actOpts{disabled: w.readOnly() || w.stageableCount() == 0}) {
		w.after(w.stageAllVisible)
	}
	if w.cm.listID != "" {
		w.tip(idCmStage, w.t(i18n.WorkspaceActionStageAllIn, w.changeListFilterName()))
	}
	x += actionSize + 2
	if w.action(idCmUnstage, x, y, icons.Hide, w.t(i18n.WorkspaceActionUnstageAll), actOpts{disabled: w.readOnly() || staged == 0}) {
		w.after(func() { w.unstageAll() })
	}
	x += actionSize + 8
	w.p.vline(x-4, y+5, actionSize-10, w.pal.Separator)
	if w.action(idCmGroup, x, y, icons.ShowAsTree, w.t(i18n.WorkspaceActionGroupByDirectory), actOpts{toggled: w.ws.Settings.CommitTree}) {
		w.after(func() { w.ws.Settings.CommitTree = !w.ws.Settings.CommitTree; w.saveSettings() })
	}
	x += actionSize + 2
	if w.action(idCmExpand, x, y, icons.ExpandAll, w.t(i18n.WorkspaceActionExpandAll)+"\t⌘+", actOpts{}) {
		w.after(func() { clear(w.cm.collapsed) })
	}
	x += actionSize + 2
	if w.action(idCmCollapse, x, y, icons.CollapseAll, w.t(i18n.WorkspaceActionCollapseAll)+"\t⌘-", actOpts{}) {
		w.after(func() {
			for _, a := range areaOrder {
				w.cm.collapsed[rowKey(a, "")] = true
			}
		})
	}
	x += actionSize + 2
	if w.ws.Caps.Stash != nil {
		if w.action(idCmStash, x, y, icons.Shelve, w.t(i18n.WorkspaceActionStashChanges), actOpts{disabled: w.readOnly() || rs.Status.Clean()}) {
			w.after(w.openStashDialog)
		}
		x += actionSize + 2
	}

	// Change-list bar: the active filter and how many files it shows. It also
	// states that committing is not scoped to the selected list.
	bar := splitTop(&content, w.changeListsBarHeight())
	w.paintChangeListsBar(bar)

	// Message section and buttons at the bottom.
	v := &w.cm
	// The changes tree keeps at least ~110px (a few rows) before the message
	// area grows; a small window shrinks the message down to 60px first.
	msgH := clampF(v.msgH, 60, max(60, content.Height-84-110))
	if v.msgCollapsed {
		msgH = 0
	}
	bottomH := msgH + 40 + 44
	if v.msgCollapsed {
		// The message area and the commit buttons fold away with it, leaving
		// just the Amend row and the two small buttons beside it.
		bottomH = 40
	}
	bottomR := splitBottom(&content, bottomH)
	w.p.hline(bottomR.X, bottomR.Y, bottomR.Width, w.pal.Border)
	if !v.msgCollapsed {
		startH := msgH
		w.splitter(idSplitMessage, rect(bottomR.X, bottomR.Y-3, bottomR.Width, 6), false,
			func() { startH = msgH },
			func(d float32) { v.msgH = clampF(startH-d, 60, max(60, content.Height+msgH-110)) }, nil)
	}
	w.paintChanges(content)
	w.paintCommitMessage(bottomR, msgH)
}

// paintOperationBanner shows a merge/rebase/cherry-pick in progress.
func (w *Window) paintOperationBanner(r ui.Rect) ui.Rect {
	rs := w.repo()
	op := rs.Operation
	conflicts := len(rs.Entries(domain.AreaConflicts))
	title := w.t(i18n.WorkspaceBannerInProgress, op.Kind.Title())
	detail := ""
	switch op.Kind {
	case domain.OpRebase:
		if op.Onto != "" {
			detail = w.t(i18n.WorkspaceBannerRebasing, op.HeadName, domain.ShortHash(op.Onto))
		}
		if op.Total > 0 {
			detail += " (" + itoa(op.Step) + "/" + itoa(op.Total) + ")"
		}
	case domain.OpMerge:
		detail = w.t(i18n.WorkspaceBannerMerging, rs.HeadLabel())
	case domain.OpCherryPick, domain.OpRevert:
		if op.Incoming != "" {
			detail = domain.ShortHash(op.Incoming) + " " + op.IncomingSubject
		}
	}
	if conflicts > 0 {
		detail = strings.TrimSpace(detail + " · " + w.pluralT(conflicts, i18n.WorkspaceCountConflictsOne, i18n.WorkspaceCountConflictsMany))
	}
	h := float32(78)
	b := splitTop(&r, h+12)
	br := rect(b.X+8, b.Y+4, b.Width-16, h)
	bg, border := w.pal.Banner, w.pal.BannerBorder
	if conflicts > 0 {
		bg, border = w.pal.BannerWarn, w.pal.BannerWarnBorder
	}
	w.p.outline(br, 8, 1, border, bg)
	icon := icons.Info
	if conflicts > 0 {
		icon = icons.Warning
	}
	w.p.iconIn(icon, rect(br.X+8, br.Y+6, 20, 22))
	w.p.text(title, rect(br.X+32, br.Y+6, br.Width-40, 22), fontUI, semibold, w.pal.Text)
	w.p.text(detail, rect(br.X+32, br.Y+26, br.Width-40, 18), fontSmall, regular, w.pal.TextMuted)
	bx, by := br.X+32, br.Y+h-34
	if w.readOnly() {
		return r
	}
	if op.CanContinue() {
		label := w.t(i18n.WorkspaceButtonContinue)
		bw := w.buttonWidth(label)
		if w.button(idCmContinue, rect(bx, by, bw, 26), label, btnDefault, conflicts > 0) {
			w.after(func() { w.ws.OperationAction("continue") })
		}
		if conflicts > 0 {
			w.tip(idCmContinue, w.t(i18n.WorkspaceTipResolveFirst))
		}
		bx += bw + 8
	}
	if op.CanSkip() {
		label := w.t(i18n.WorkspaceButtonSkip)
		bw := w.buttonWidth(label)
		if w.button(idCmSkip, rect(bx, by, bw, 26), label, btnNormal, false) {
			w.after(func() { w.ws.OperationAction("skip") })
		}
		bx += bw + 8
	}
	if op.CanAbort() {
		label := w.t(i18n.WorkspaceButtonAbort)
		bw := w.buttonWidth(label)
		if w.button(idCmAbort, rect(bx, by, bw, 26), label, btnNormal, false) {
			w.after(func() {
				w.confirm(w.t(i18n.WorkspaceConfirmAbort, op.Kind.Title()), w.t(i18n.WorkspaceConfirmAbortBody, strings.ToLower(op.Kind.Title())), label, func() {
					w.ws.OperationAction("abort")
				})
			})
		}
	}
	return r
}

func (w *Window) paintChanges(r ui.Rect) {
	v := &w.cm
	rs := w.repo()
	w.buildRows()
	w.list(idChangesList, scChangeRows, r, w.t(i18n.WorkspacePanelChanges))
	content := float32(len(v.rows)) * rowH
	v.scroll.clamp(content+8, r.Height)
	w.onScrollIn(r, func(dx, dy float32) { v.scroll.Y += dy; v.scroll.clamp(content+8, r.Height) })
	focused := w.focused(idChangesList)
	w.onKeys(idChangesList, w.changesKey)

	if !rs.StatusLoaded {
		w.p.textCenter(w.t(i18n.WorkspaceStateLoading), r, fontUI, regular, w.pal.TextSecond)
		return
	}
	if rs.Status.Clean() && len(v.rows) <= 1 {
		ec := rect(r.X, r.Y+r.Height*0.3, r.Width, 22)
		w.p.textCenter(w.t(i18n.WorkspaceEmptyNoChanges), ec, fontUI, regular, w.pal.TextSecond)
		w.p.textCenter(w.t(i18n.WorkspaceEmptyWorkingTreeClean), rect(ec.X, ec.Y+22, ec.Width, 20), fontSmall, regular, w.pal.TextDisable)
		return
	}
	w.c.Clip(r)
	y := r.Y + 4 - v.scroll.Y
	for i, row := range v.rows {
		rr := rect(r.X, y, r.Width, rowH)
		y += rowH
		if bottom(rr) < r.Y || rr.Y > bottom(r) {
			continue
		}
		id := scChangeRows.At(i)
		hl := rect(rr.X+6, rr.Y, rr.Width-12, rr.Height)
		selected := v.sel[row.key]
		if selected {
			c := w.pal.SelectionInac
			if focused {
				c = w.pal.Selection
			}
			w.p.round(hl, 4, c)
		} else if w.hovered(id) || w.hovered(scChangeAction.At(i)) {
			w.p.round(hl, 4, w.pal.Hover)
		}
		x := rr.X + 10 + float32(row.depth)*18
		cbX := x
		x += 22
		switch row.kind {
		case rowGroup, rowDir:
			chev := icons.ChevronDown
			if v.collapsed[row.key] {
				chev = icons.ChevronRight
			}
			w.p.iconIn(chev, rect(x, rr.Y, 16, rowH))
			x += 20
			title := w.areaTitle(row.area)
			if row.kind == rowDir {
				w.p.iconIn(icons.Folder, rect(x, rr.Y, 16, rowH))
				x += 22
				title = row.dir
			}
			tw := w.p.text(title, rect(x, rr.Y, r.Width-x-40, rowH), fontUI, regular, w.pal.Text)
			if row.kind == rowGroup {
				w.p.text(w.pluralT(row.count, i18n.WorkspaceCountFilesOne, i18n.WorkspaceCountFilesMany), rect(x+tw+8, rr.Y, 120, rowH), fontUI, regular, w.pal.TextSecond)
			} else {
				w.p.text(itoa(row.count), rect(x+tw+8, rr.Y, 60, rowH), fontUI, regular, w.pal.TextSecond)
			}
		default:
			e := row.entry
			kind := e.KindIn(row.area)
			w.p.iconIn(fileIcon(e.Path), rect(x, rr.Y, 16, rowH))
			x += 22
			name := domain.FileName(e.Path)
			fg := w.statusColor(kind)
			tw := w.p.text(name, rect(x, rr.Y, right(rr)-x-40, rowH), fontUI, regular, fg)
			if kind == domain.ChangeDeleted {
				w.p.hline(x, rr.Y+rowH/2, tw, fg)
			}
			hint := domain.DirName(e.Path)
			if w.ws.Settings.CommitTree {
				hint = ""
			}
			if e.OrigPath != "" && (row.area == domain.AreaStaged || kind == domain.ChangeRenamed) {
				hint = "← " + e.OrigPath
			}
			if e.IsSubmodule() {
				hint = strings.TrimSpace(w.t(i18n.WorkspaceLabelSubmodule) + " " + hint)
			}
			if hint != "" {
				hx := x + tw + 8
				w.p.text(w.p.truncateMiddle(hint, fontUI, regular, max(0, right(rr)-hx-40)), rect(hx, rr.Y, right(rr)-hx-36, rowH), fontUI, regular, w.pal.TextSecond)
			}
		}
		w.c.HitWithOptions(id, hl, w.rowLabel(row), ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: selected, Expandable: row.kind != rowFile, Expanded: row.kind != rowFile && !v.collapsed[row.key]})
		// The checkbox is painted after the row's hit area, so a click on it
		// toggles the check instead of selecting the row.
		checked := v.sel[row.key]
		if row.kind != rowFile {
			checked = w.changesRowChecked(i)
		}
		if w.checkbox(scChangeChecks.At(i), cbX, rr.Y, rowH, "", checked, w.readOnly()) {
			i := i
			w.after(func() { w.toggleRowCheck(i) })
		}
		w.rowAction(i, row, hl)
		if w.c.Clicked(id) {
			i := i
			w.after(func() { w.clickRow(i) })
		}
		w.onDouble(id, func() { w.activateRow(row) })
		w.onContext(id, func(x, y float32) {
			if !v.sel[row.key] {
				clear(v.sel)
				v.sel[row.key] = true
				v.cursor, v.anchor = row.key, row.key
			}
			w.openMenu(x, y, w.changesMenu())
		})
	}
	w.c.Unclip()
	w.vbar(scLogScrollbars.At(1), r, &v.scroll, content+8)
}

func (w *Window) rowLabel(row changeRow) string {
	switch row.kind {
	case rowGroup:
		return w.areaTitle(row.area)
	case rowDir:
		return row.dir
	}
	return row.entry.Path
}

// entryKeys names the selection keys of the files a group or directory row
// covers.
func entryKeys(a domain.Area, entries []domain.StatusEntry) []string {
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, rowKey(a, e.Path))
	}
	return keys
}

// toggleRowCheck flips one row's checkbox: a file toggles itself, a group takes
// every file in its area, and a directory takes only its own files. The set
// comes from the row, not from the rows below it, so a collapsed row toggles
// files that are not painted.
func (w *Window) toggleRowCheck(i int) {
	v := &w.cm
	if i < 0 || i >= len(v.rows) {
		return
	}
	row := v.rows[i]
	if row.kind == rowFile {
		if v.sel[row.key] {
			delete(v.sel, row.key)
		} else {
			v.sel[row.key] = true
		}
		return
	}
	on := !w.changesRowChecked(i)
	// A row key covers its files through selectedEntries; once the box drives
	// the choice, the file keys are the precise statement and the row key goes.
	delete(v.sel, row.key)
	for _, key := range row.files {
		if on {
			v.sel[key] = true
		} else {
			delete(v.sel, key)
		}
	}
}

// changesRowChecked reports whether every file a row covers is selected, which
// is what fills a group's or directory's checkbox.
func (w *Window) changesRowChecked(i int) bool {
	if i < 0 || i >= len(w.cm.rows) {
		return false
	}
	row := w.cm.rows[i]
	if row.kind == rowFile {
		return w.cm.sel[row.key]
	}
	// A selected group or directory row already covers its files, the way a
	// row click selects them; the box must show that state too.
	if w.cm.sel[row.key] {
		return true
	}
	if len(row.files) == 0 {
		return false
	}
	for _, key := range row.files {
		if !w.cm.sel[key] {
			return false
		}
	}
	return true
}

// rowAction paints the hover +/- action at the right of a row.
func (w *Window) rowAction(i int, row changeRow, hl ui.Rect) {
	if w.readOnly() {
		return
	}
	id := scChangeAction.At(i)
	if !(w.hovered(scChangeRows.At(i)) || w.hovered(id)) {
		return
	}
	// Without an index only scheduling is left: `svn add` takes an untracked
	// file under version control, while an already-versioned file is committed
	// as it stands and `svn add` on it fails with E200009. The action is
	// hidden rather than offered and refused.
	if !w.ws.Caps.Index && row.area != domain.AreaUntracked {
		return
	}
	var pair icons.Pair
	var tip string
	switch row.area {
	case domain.AreaStaged:
		pair, tip = icons.Hide, w.t(i18n.WorkspaceMenuUnstage)
	case domain.AreaConflicts:
		return
	default:
		pair, tip = icons.Add, w.t(i18n.WorkspaceMenuStage)
	}
	if row.kind == rowGroup {
		tip = w.t(i18n.WorkspaceActionUnstageAll)
		if row.area != domain.AreaStaged {
			tip = w.t(i18n.WorkspaceActionStageAll)
		}
	}
	if w.action(id, right(hl)-24, hl.Y+1, pair, tip, actOpts{size: 22}) {
		w.after(func() {
			paths := w.rowPaths(row)
			if row.area == domain.AreaStaged {
				w.ws.UnstagePaths(paths)
			} else {
				w.ws.StagePaths(paths)
			}
		})
	}
}

// rowPaths returns the files a row stands for.
func (w *Window) rowPaths(row changeRow) []string {
	switch row.kind {
	case rowFile:
		ps := []string{row.entry.Path}
		if row.area == domain.AreaStaged && row.entry.OrigPath != "" {
			ps = append(ps, row.entry.OrigPath)
		}
		return ps
	case rowDir:
		var out []string
		for _, e := range w.changeListEntries(row.area) {
			if domain.DirName(e.Path) == row.dir {
				out = append(out, e.Path)
			}
		}
		return out
	}
	var out []string
	for _, e := range w.changeListEntries(row.area) {
		out = append(out, e.Path)
		if row.area == domain.AreaStaged && e.OrigPath != "" {
			out = append(out, e.OrigPath)
		}
	}
	return out
}

func (w *Window) statusColor(k domain.ChangeKind) ui.Color {
	switch k {
	case domain.ChangeAdded, domain.ChangeCopied:
		return w.pal.StatusAdded
	case domain.ChangeDeleted:
		return w.pal.StatusDeleted
	case domain.ChangeRenamed:
		return w.pal.StatusRenamed
	case domain.ChangeUntracked:
		return w.pal.StatusUnknown
	case domain.ChangeConflicted:
		return w.pal.StatusConflict
	case domain.ChangeIgnored:
		return w.pal.StatusIgnored
	}
	return w.pal.StatusModified
}

// clickRow selects a row (with Shift/Cmd extension) and previews its diff.
func (w *Window) clickRow(i int) {
	v := &w.cm
	if i >= len(v.rows) {
		return
	}
	row := v.rows[i]
	mods := w.mods
	switch {
	case mods&ui.ModifierShift != 0 && v.anchor != "":
		a := w.rowIndex(v.anchor)
		if a < 0 {
			a = i
		}
		lo, hi := min(a, i), max(a, i)
		clear(v.sel)
		for k := lo; k <= hi; k++ {
			v.sel[v.rows[k].key] = true
		}
	case mods&ui.ModifierMeta != 0:
		if v.sel[row.key] {
			delete(v.sel, row.key)
		} else {
			v.sel[row.key] = true
		}
		v.anchor = row.key
	default:
		if row.kind != rowFile && v.sel[row.key] && len(v.sel) == 1 {
			// A second click on a selected node toggles it, as the chevron does.
			v.collapsed[row.key] = !v.collapsed[row.key]
		}
		clear(v.sel)
		v.sel[row.key] = true
		v.anchor = row.key
	}
	v.cursor = row.key
	if row.kind == rowFile && len(v.sel) == 1 {
		w.showDiff(w.sourceFor(row), false)
	}
}

func (w *Window) rowIndex(key string) int {
	for i, r := range w.cm.rows {
		if r.key == key {
			return i
		}
	}
	return -1
}

func (w *Window) sourceFor(row changeRow) vapp.DiffSource {
	e := row.entry
	src := vapp.DiffSource{Path: e.Path, Area: row.area, Working: true, Kind: e.KindIn(row.area)}
	if row.area == domain.AreaStaged {
		src.OldPath = e.OrigPath
	}
	return src
}

func (w *Window) activateRow(row changeRow) {
	if row.kind == rowFile {
		w.showDiff(w.sourceFor(row), true)
		return
	}
	w.cm.collapsed[row.key] = !w.cm.collapsed[row.key]
}

func (w *Window) changesKey(e ui.KeyEvent) bool {
	v := &w.cm
	if len(v.rows) == 0 {
		return false
	}
	cur := w.rowIndex(v.cursor)
	move := func(to int) {
		to = max(0, min(len(v.rows)-1, to))
		row := v.rows[to]
		if e.Modifiers&ui.ModifierShift != 0 {
			v.sel[row.key] = true
		} else {
			clear(v.sel)
			v.sel[row.key] = true
			v.anchor = row.key
		}
		v.cursor = row.key
		v.scroll.reveal(float32(to)*rowH, rowH+8, w.areas[idChangesList].Height)
		if row.kind == rowFile {
			w.showDiff(w.sourceFor(row), false)
		}
	}
	switch e.Key {
	case ui.KeyDown:
		move(cur + 1)
	case ui.KeyUp:
		if cur < 0 {
			cur = len(v.rows)
		}
		move(cur - 1)
	case ui.KeyHome:
		move(0)
	case ui.KeyEnd:
		move(len(v.rows) - 1)
	case ui.KeyPageDown:
		move(cur + 15)
	case ui.KeyPageUp:
		move(cur - 15)
	case ui.KeyLeft:
		if cur >= 0 {
			row := v.rows[cur]
			if row.kind != rowFile && !v.collapsed[row.key] {
				v.collapsed[row.key] = true
			} else {
				for k := cur - 1; k >= 0; k-- {
					if v.rows[k].kind != rowFile && v.rows[k].depth < row.depth {
						move(k)
						break
					}
				}
			}
		}
	case ui.KeyRight:
		if cur >= 0 && v.rows[cur].kind != rowFile {
			v.collapsed[v.rows[cur].key] = false
		}
	case ui.KeyEnter:
		if cur >= 0 {
			w.activateRow(v.rows[cur])
		}
	case ui.KeySpace:
		if !w.readOnly() {
			w.toggleStageSelection()
		}
	case ui.KeyDelete, ui.KeyBackspace:
		if !w.readOnly() && len(w.rollbackTargets()) > 0 {
			w.openRollback()
		}
	default:
		return false
	}
	return true
}

// selectedEntries returns the selected file rows (directories and groups
// expand to their files).
func (w *Window) selectedEntries() []changeRow {
	v := &w.cm
	var out []changeRow
	seen := map[string]bool{}
	for _, row := range v.rows {
		if !v.sel[row.key] {
			continue
		}
		switch row.kind {
		case rowFile:
			if !seen[row.key] {
				seen[row.key] = true
				out = append(out, row)
			}
		default:
			for _, e := range w.changeListEntries(row.area) {
				if row.kind == rowDir && domain.DirName(e.Path) != row.dir {
					continue
				}
				k := rowKey(row.area, e.Path)
				if !seen[k] {
					seen[k] = true
					out = append(out, changeRow{key: k, area: row.area, kind: rowFile, entry: e})
				}
			}
		}
	}
	return out
}

func (w *Window) toggleStageSelection() {
	var stage, unstage []string
	for _, r := range w.selectedEntries() {
		switch r.area {
		case domain.AreaStaged:
			unstage = append(unstage, r.entry.Path)
			if r.entry.OrigPath != "" {
				unstage = append(unstage, r.entry.OrigPath)
			}
		case domain.AreaUnstaged, domain.AreaUntracked:
			// A modified file is already under version control where there is
			// no index, so scheduling it again would only fail.
			if !w.ws.Caps.Index && r.area == domain.AreaUnstaged {
				continue
			}
			stage = append(stage, r.entry.Path)
		case domain.AreaConflicts:
			w.markResolved(r.entry.Path)
		}
	}
	if len(stage) > 0 {
		w.ws.StagePaths(stage)
	}
	if len(unstage) > 0 {
		w.ws.UnstagePaths(unstage)
	}
}

func (w *Window) stageArea(areas ...domain.Area) {
	var paths []string
	for _, a := range areas {
		for _, e := range w.repo().Entries(a) {
			paths = append(paths, e.Path)
		}
	}
	if len(paths) > 0 {
		w.ws.StagePaths(paths)
	}
}

func (w *Window) unstageAll() {
	var paths []string
	for _, e := range w.repo().Entries(domain.AreaStaged) {
		paths = append(paths, e.Path)
		if e.OrigPath != "" {
			paths = append(paths, e.OrigPath)
		}
	}
	if len(paths) > 0 {
		w.ws.UnstagePaths(paths)
	}
}

// markResolved stages a conflicted file once it has no conflict markers.
func (w *Window) markResolved(p string) {
	w.ws.StagePaths([]string{p})
}

func (w *Window) openSelectedDiff(pin bool) {
	sel := w.selectedEntries()
	if len(sel) == 0 {
		return
	}
	w.showDiff(w.sourceFor(sel[0]), pin)
}

// rollbackTargets returns the selection's files that can be rolled back.
func (w *Window) rollbackTargets() []changeRow {
	var out []changeRow
	for _, r := range w.selectedEntries() {
		if r.area != domain.AreaConflicts {
			out = append(out, r)
		}
	}
	return out
}

func (w *Window) openRollback() {
	targets := w.rollbackTargets()
	if len(targets) == 0 {
		return
	}
	var tracked, untracked, staged []string
	for _, r := range targets {
		switch r.area {
		case domain.AreaUntracked:
			untracked = append(untracked, r.entry.Path)
		case domain.AreaStaged:
			staged = append(staged, r.entry.Path)
			if r.entry.OrigPath != "" {
				staged = append(staged, r.entry.OrigPath)
			}
		default:
			tracked = append(tracked, r.entry.Path)
		}
	}
	deleteNew := false
	files := append(append(append([]string{}, staged...), tracked...), untracked...)
	sort.Strings(files)
	w.openDialog(&dialog{
		title: w.t(i18n.WorkspaceDialogRollback), ok: w.t(i18n.WorkspaceButtonRollback), width: 520,
		body: func(f *form) {
			f.text(w.t(i18n.WorkspaceConfirmRollbackBody, w.pluralT(len(files), i18n.WorkspaceCountFilesOne, i18n.WorkspaceCountFilesMany)))
			n := min(len(files), 8)
			f.custom(float32(n)*22+4, func(r ui.Rect) {
				w.p.outline(r, 4, 1, w.pal.ControlBorder, w.pal.ControlBg)
				for i := 0; i < n; i++ {
					rr := rect(r.X+8, r.Y+2+float32(i)*22, r.Width-16, 22)
					w.p.iconIn(fileIcon(files[i]), rect(rr.X, rr.Y, 16, 22))
					w.p.text(files[i], rect(rr.X+22, rr.Y, rr.Width-22, 22), fontUI, regular, w.pal.Text)
				}
			})
			if len(files) > n {
				f.muted(w.t(i18n.WorkspaceLabelAndMore, len(files)-n))
			}
			f.gap(8)
			if len(untracked) > 0 {
				f.check(w.t(i18n.WorkspaceCheckDeleteUntracked, w.pluralT(len(untracked), i18n.WorkspaceCountUnversionedOne, i18n.WorkspaceCountUnversionedMany)), &deleteNew)
			}
			if len(staged) > 0 {
				f.muted(w.t(i18n.WorkspaceMutedStagedReset))
			}
		},
		onOK: func() bool {
			del := []string(nil)
			if deleteNew {
				del = untracked
			}
			if len(staged) > 0 {
				w.ws.Rollback(append(staged, tracked...), del, true)
			} else if len(tracked) > 0 || len(del) > 0 {
				w.ws.Rollback(tracked, del, false)
			}
			return true
		},
	})
}

// changesMenu is the staging tree's context menu.
func (w *Window) changesMenu() []item {
	sel := w.selectedEntries()
	if len(sel) == 0 {
		return nil
	}
	readOnly := w.readOnly()
	var stage, unstage bool
	conflict := false
	untrackedOnly := true
	for _, r := range sel {
		switch r.area {
		case domain.AreaStaged:
			unstage = true
		case domain.AreaConflicts:
			conflict = true
		default:
			stage = true
		}
		if r.area != domain.AreaUntracked {
			untrackedOnly = false
		}
	}
	first := sel[0]
	full := w.repoPath(first.entry.Path)
	items := []item{
		{title: w.t(i18n.WorkspaceActionShowDiff), icon: icons.Diff, shortcut: "⌘D", run: func() { w.showDiff(w.sourceFor(first), true) }},
	}
	if !readOnly && len(sel) == 1 && first.entry.IsSubmodule() {
		sub := first.entry.Path
		items = append(items,
			item{title: w.t(i18n.WorkspaceMenuUpdateSubmodule), icon: icons.Download, run: func() { w.ws.UpdateSubmodule(sub, true, nil) }},
			item{title: w.t(i18n.WorkspaceMenuOpenSubmodule), icon: icons.Folder, run: func() { w.openSubmodule(sub) }},
		)
	}
	if !readOnly {
		if stage {
			items = append(items, item{title: w.t(i18n.WorkspaceMenuStage), icon: icons.Add, shortcut: "Space", run: w.toggleStageSelection})
		}
		if unstage {
			items = append(items, item{title: w.t(i18n.WorkspaceMenuUnstage), icon: icons.Hide, shortcut: "Space", run: w.toggleStageSelection})
		}
		if conflict {
			items = append(items, item{title: w.t(i18n.WorkspaceMenuMarkResolved), icon: icons.Checked, run: func() { w.markResolved(first.entry.Path) }})
		}
		if move := w.changeListMoveItems(sel); len(move) > 0 {
			items = append(items, item{title: w.t(i18n.WorkspaceMenuMoveToChangeList), icon: icons.Changelist, sub: move})
		}
		items = append(items, item{title: w.t(i18n.WorkspaceActionRollback), icon: icons.Revert, shortcut: "⌥⌘Z", disabled: len(w.rollbackTargets()) == 0, run: w.openRollback}, sep())
		if untrackedOnly {
			items = append(items, item{title: w.t(i18n.WorkspaceMenuAddToGitignore), icon: icons.Gitignore, run: func() { w.addToGitignore(sel) }})
		}
	}
	items = append(items,
		item{title: w.t(i18n.WorkspaceMenuShowHistory), icon: icons.History, run: func() { w.showPathHistory(first.entry.Path) }},
	)
	if w.canAnnotate(first) {
		items = append(items, item{title: w.t(i18n.WorkspaceMenuAnnotate), run: func() { w.openFileView(first.entry.Path, "", true) }})
	}
	if !readOnly {
		items = append(items,
			item{title: w.t(i18n.WorkspaceMenuOpenInEditor), icon: icons.Edit, shortcut: "F4", run: func() { w.openExternal(first.entry.Path, 0) }},
			item{title: w.t(i18n.WorkspaceMenuRevealInFinder), icon: icons.Folder, run: func() { revealPath(full) }},
		)
	}
	items = append(items,
		sep(),
		item{title: w.t(i18n.WorkspaceActionCopyPath), icon: icons.Copy, shortcut: "⇧⌘C", run: func() { w.copyText(full) }},
		item{title: w.t(i18n.WorkspaceActionCopyRelativePath), run: func() { w.copyText(first.entry.Path) }},
	)
	patchPaths, patchStaged, canPatch := patchTargets(sel)
	items = append(items, sep(),
		item{title: w.t(i18n.WorkspaceMenuCreatePatch), icon: icons.Patch, disabled: !canPatch, run: func() { w.savePatch(patchPaths, patchStaged) }})
	return items
}

// canAnnotate reports whether blame can be opened for this row. A remote row
// has no local file to stat; the status entry already says whether it is a
// submodule, and a deleted/untracked row cannot be blamed.
func (w *Window) canAnnotate(r changeRow) bool {
	if r.area == domain.AreaUntracked || r.area == domain.AreaConflicts || r.entry.IsSubmodule() {
		return false
	}
	if w.readOnly() {
		return true
	}
	st, err := os.Stat(w.repoPath(r.entry.Path))
	return err == nil && st.Mode().IsRegular()
}

func (w *Window) copyText(s string) {
	if err := w.setClipboardText(s); err != nil {
		w.ws.NotifyError(w.t(i18n.WorkspaceErrorCannotCopy), err)
	}
}

// setClipboardText writes text through the window's clipboard seam.
func (w *Window) setClipboardText(s string) error {
	write := w.clipboard
	if write == nil {
		write = app.SetClipboardText
	}
	return write(s)
}

// ---- message and commit buttons ------------------------------------------------

func (w *Window) paintCommitMessage(r ui.Rect, msgH float32) {
	v := &w.cm
	rs := w.repo()
	if w.readOnly() {
		w.paintReadOnlyCommitMessage(r)
		return
	}
	// Amend + options row. A backend that cannot rewrite a committed revision
	// (Subversion) does not offer Amend at all, so the checkbox is absent and
	// v.amend can never be set from the window.
	top := rect(r.X+10, r.Y+6, r.Width-20, 28)
	if w.ws.Caps.Amend {
		if w.checkbox(idCmAmend, top.X, top.Y, top.Height, w.t(i18n.WorkspaceCheckAmend), v.amend, !rs.HasHead()) {
			w.after(w.toggleAmend)
		}
	}
	aiTip := w.promptRuleTip()
	if v.aiBusy {
		aiTip = w.t(i18n.WorkspaceAIGenerating)
	}
	aiRect := rect(right(top)-actionSize*3-12, top.Y+1, actionSize, actionSize)
	v.aiRect = aiRect
	if w.action(idCmAI, aiRect.X, aiRect.Y, icons.Bot, aiTip, actOpts{disabled: v.aiBusy}) {
		w.after(w.generateAICommitMessage)
	}
	// The picker hangs off the right click so the Amend row keeps its three
	// buttons; a left click still generates a message.
	w.onContext(idCmAI, func(x, y float32) { w.openPromptRulesMenu(x, y) })
	chevron, chevronTip := icons.ChevronDown, w.t(i18n.WorkspaceTipCollapseMessage)
	if v.msgCollapsed {
		chevron, chevronTip = icons.ChevronUp, w.t(i18n.WorkspaceTipExpandMessage)
	}
	if w.action(idCmOptionsDropdown, right(top)-actionSize*2-6, top.Y+1, chevron, chevronTip, actOpts{}) {
		w.after(func() { v.msgCollapsed = !v.msgCollapsed })
	}
	if w.action(idCmOptions, right(top)-actionSize, top.Y+1, icons.Settings, w.t(i18n.WorkspaceTipCommitOptions), actOpts{}) {
		w.after(func() { w.openMenuBelow(rect(right(top)-200, top.Y, 200, top.Height), w.commitOptionsMenu(), 220) })
	}
	mr := rect(r.X+10, bottom(top)+4, r.Width-20, msgH)
	value := v.msg.String()
	if msgH > 0 {
		sel := v.msg.Selection()
		caret := runeToByte(value, v.msg.Caret())
		ss, se := runeToByte(value, min(sel.Start, sel.End)), runeToByte(value, max(sel.Start, sel.End))
		components.PaintTextarea(w.c, w.theme, mr, components.TextareaSnapshot{
			ID: idCmMessage, Label: w.t(i18n.WorkspaceLabelCommitMessage), Value: value, Placeholder: w.t(i18n.WorkspaceLabelCommitMessage),
			ShowCaret: true, HideEditorText: true, Layout: &v.layout,
			Caret: caret, SelectionStart: ss, SelectionEnd: se,
		})
		w.editors[idCmMessage] = v.msg
		w.onScrollIn(mr, func(dx, dy float32) { v.layout.ScrollBy(dy) })
		w.onKeys(idCmMessage, func(e ui.KeyEvent) bool {
			if e.Key == ui.KeyEnter && e.Modifiers&ui.ModifierMeta != 0 {
				w.doCommit(false)
				return true
			}
			mv, extend := components.TextareaMovementForKey(e.Key, e.Modifiers)
			if mv == components.TextareaMoveNone || mv == components.TextareaMoveLineStart || mv == components.TextareaMoveLineEnd {
				return false
			}
			val := v.msg.String()
			nb := v.layout.Move(val, runeToByte(val, v.msg.Caret()), mv)
			v.msg.MoveTo(byteToRune(val, nb), extend)
			return true
		})
	}

	if v.msgCollapsed {
		return
	}

	// Buttons.
	by := bottom(mr) + 10
	committable := w.ws.CommittableCount()
	_, _, _, conflicts := rs.Status.Counts()
	empty := strings.TrimSpace(value) == ""
	can := !empty && (committable > 0 || v.amend) && conflicts == 0
	label := w.t(i18n.WorkspaceButtonCommit)
	if v.amend {
		label = w.t(i18n.WorkspaceButtonAmendCommit)
	}
	bw := w.buttonWidth(label)
	if w.button(idCmCommit, rect(r.X+10, by, bw, buttonH), label, btnDefault, !can) {
		w.after(func() { w.doCommit(false) })
	}
	switch {
	case conflicts > 0:
		w.tip(idCmCommit, w.t(i18n.WorkspaceTipResolveBeforeCommit))
	case committable == 0 && !v.amend:
		if w.ws.Caps.Index {
			w.tip(idCmCommit, w.t(i18n.WorkspaceTipStageToCommit))
		} else {
			w.tip(idCmCommit, w.t(i18n.WorkspaceTipNoChangesToCommit))
		}
	case empty:
		w.tip(idCmCommit, w.t(i18n.WorkspaceTipEnterMessage))
	default:
		w.tip(idCmCommit, label+"\t⌘⏎")
	}
	pl := w.t(i18n.WorkspaceButtonCommitAndPush)
	if v.amend {
		pl = w.t(i18n.WorkspaceButtonAmendCommitAndPush)
	}
	// Without a separate publish step the commit already reached the server,
	// so a second button would either repeat the commit or do nothing.
	if w.ws.Caps.SeparatePush {
		pw := w.buttonWidth(pl)
		if w.button(idCmCommitPush, rect(r.X+10+bw+10, by, pw, buttonH), pl, btnNormal, !can || len(rs.Remotes) == 0) {
			w.after(func() { w.doCommit(true) })
		}
	}
}

// generateAICommitMessage asks the configured endpoint for a message and puts
// it in the box. A second run passes the previous one so the wording changes.
// A missing endpoint or a failed request is reported as a notification, which
// carries the link to the settings page instead of covering the message box.
//
// A generated message leaves the machine, so the click asks first unless the
// user turned that off in the AI button's context menu.
func (w *Window) generateAICommitMessage() {
	v := &w.cm
	if v.aiBusy {
		return
	}
	s := w.ws.Settings
	cfg := ai.Config{
		Protocol: ai.ParseProtocol(s.AIProtocol),
		Endpoint: s.AIEndpoint,
		APIKey:   s.AIKey,
		Model:    s.AIModel,
		Language: s.Language,
	}
	openSettings := vapp.NoticeAction{Title: w.t(i18n.WorkspaceAIOpenSettings), Run: func() { w.openSettingsAt(2) }}
	if !cfg.Ready() {
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.WorkspaceAINotConfigured), "", openSettings)
		return
	}
	if w.confirmAIGenerate() {
		// A modal dialog would cover the commit box the answer is about to fill,
		// so the question sits in a small bubble just above the button. Both
		// paths call the same request function, so the two behaviors cannot
		// drift apart.
		w.confirmAIAbove(v.aiRect, cfg, openSettings)
		return
	}
	w.requestAICommitMessage(cfg, openSettings)
}

// confirmAIAbove asks about one generation in a bubble over the AI button.
func (w *Window) confirmAIAbove(anchor ui.Rect, cfg ai.Config, openSettings vapp.NoticeAction) {
	items := []item{
		// The question is a statement, not a command: it has no action of its own.
		{title: w.t(i18n.WorkspaceAIConfirmAsk), label: true},
		// Cancel on the left, Generate on the right, matching the button order of
		// every other dialog. Neither carries an icon: these two are plain text
		// buttons.
		{buttons: []item{
			{title: w.t(i18n.CommonCancel)},
			{title: w.t(i18n.WorkspaceAIConfirmGenerate), run: func() {
				w.requestAICommitMessage(cfg, openSettings)
			}},
		}},
	}
	w.openMenuAbove(anchor, items, 210)
}

// requestAICommitMessage starts the request and fills the message box. It is
// reached either directly or after the confirmation.
func (w *Window) requestAICommitMessage(cfg ai.Config, openSettings vapp.NoticeAction) {
	v := &w.cm
	if v.aiBusy {
		return
	}
	v.aiBusy = true
	previous := v.aiLast
	w.ws.GenerateCommitMessage(context.Background(), cfg, previous, func(message string, err error) {
		v.aiBusy = false
		if err != nil {
			w.ws.Notify(vapp.NoticeError, w.t(i18n.WorkspaceAIGenerateFailed), err.Error(), openSettings)
			return
		}
		v.aiLast = message
		setText(v.msg, message)
	})
}

func (w *Window) commitOptionsMenu() []item {
	v := &w.cm
	return []item{
		header(w.t(i18n.WorkspaceMenuCommitChecks)),
		{title: w.t(i18n.WorkspaceCheckRunHooks), checked: !v.noVerify, run: func() { v.noVerify = !v.noVerify }},
		{title: w.t(i18n.WorkspaceCheckSignOff), checked: v.signOff, run: func() { v.signOff = !v.signOff }},
	}
}

func (w *Window) toggleAmend() {
	v := &w.cm
	v.amend = !v.amend
	if v.amend {
		v.saved = v.msg.String()
		if strings.TrimSpace(v.saved) == "" {
			w.ws.LastMessage(func(m string) {
				if v.amend && strings.TrimSpace(v.msg.String()) == "" {
					m = strings.TrimRight(m, "\n")
					v.loaded = m
					setText(v.msg, m)
				}
			})
		}
		return
	}
	if v.msg.String() == v.loaded {
		setText(v.msg, v.saved)
	}
	v.loaded = ""
}

func (w *Window) doCommit(push bool) {
	v := &w.cm
	rs := w.repo()
	committable := w.ws.CommittableCount()
	_, _, _, conflicts := rs.Status.Counts()
	msg := strings.TrimSpace(v.msg.String())
	if msg == "" || conflicts > 0 || (committable == 0 && !v.amend) {
		return
	}
	// A backend without an index commits the rows the user checked; without
	// them svn commit takes the whole working copy, so the boxes would be
	// decoration. Checked rows that the backend cannot schedule (untracked and
	// ignored files) are left out, so the commit never fails over a file that
	// was never under version control.
	var paths []string
	if !w.ws.Caps.Index {
		for _, row := range w.selectedEntries() {
			if row.entry.Untracked() || row.entry.Ignored() {
				continue
			}
			paths = append(paths, row.entry.Path)
		}
	}
	req := vapp.CommitRequest{Message: v.msg.String(), Amend: v.amend, SignOff: v.signOff, NoVerify: v.noVerify, Paths: paths}
	w.ws.Commit(req, func() {
		setText(v.msg, "")
		v.msg.ClearHistory()
		v.amend, v.saved, v.loaded = false, "", ""
		if push {
			w.openPush()
		}
	})
}

// focusCommit shows the Commit tool window and focuses the message.
func (w *Window) focusCommit() {
	w.commitOpen = true
	w.projectOpen = false
	w.commitTab = 0
	w.focusNext = idCmMessage
}

func runeToByte(s string, r int) int {
	if r <= 0 {
		return 0
	}
	n := 0
	for i := range s {
		if n == r {
			return i
		}
		n++
	}
	return len(s)
}

// canAnnotate reports whether a working-tree file has history to blame: it
// is tracked, not conflicted and still on disk.

// paintRemoteBanner states the read-only contract at the top of the Commit
// panel. It mirrors the operation banner so the two never overlap: the
// operation state is the more urgent fact, and both are cleared by the same
// refresh.
func (w *Window) paintRemoteBanner(r ui.Rect) ui.Rect {
	rs := w.repo()
	host := rs.Repo.Root
	if target, err := sshpath.Parse(rs.Repo.Root); err == nil {
		host = target.Display()
	}
	const h = float32(72)
	b := splitTop(&r, h+8)
	br := rect(b.X+8, b.Y+4, b.Width-16, h)
	w.p.outline(br, 8, 1, w.pal.BannerBorder, w.pal.Banner)
	w.p.iconIn(icons.Vcs, rect(br.X+8, br.Y+6, 20, 22))
	w.p.text(w.t(i18n.WorkspaceBannerRemoteTitle), rect(br.X+32, br.Y+6, br.Width-40, 20), fontUI, semibold, w.pal.Text)
	w.p.text(w.t(i18n.WorkspaceBannerRemoteBody, host), rect(br.X+32, br.Y+26, br.Width-40, 18), fontSmall, regular, w.pal.TextMuted)
	w.p.text(w.t(i18n.WorkspaceTipReadOnly), rect(br.X+32, br.Y+44, br.Width-40, 16), fontSmall, regular, w.pal.TextDisable)
	return r
}

// paintReadOnlyCommitMessage replaces the editor and the commit buttons. A
// disabled editor would still invite typing; a short statement says where the
// change belongs instead.
func (w *Window) paintReadOnlyCommitMessage(r ui.Rect) {
	inner := rect(r.X+10, r.Y+6, r.Width-20, r.Height-12)
	w.p.text(w.t(i18n.WorkspaceRemoteCommitHint), rect(inner.X, inner.Y, inner.Width, 20), fontUI, semibold, w.pal.Text)
	w.p.text(w.t(i18n.WorkspaceRemoteCommitBody), rect(inner.X, inner.Y+22, inner.Width, 18), fontSmall, regular, w.pal.TextMuted)
}
