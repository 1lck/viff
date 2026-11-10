package viffui

import (
	"slices"
	"strings"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// The Project panel: the working tree's files, grouped by directory and opened
// read-only in the existing single-version viewer. See files.go for the model.

const fileIndent = 14

// idProjectFindFiles identifies the toolbar's Find in Files action. The ID is
// declared next to its only use because the panel owns it.
var idProjectFindFiles = tree.ID(fProject, "find-in-files")

// ensureFiles loads the path list when the repository changed or Git reported
// a newer status. The workspace drops answers for a repository that is no
// longer active; seq drops answers that a newer request replaced.
func (w *Window) ensureFiles() {
	rs := w.repo()
	if rs == nil {
		return
	}
	v := &w.fm
	if v.loading {
		return
	}
	if v.gen == rs.Gen && !v.at.Before(rs.StatusTime) && (v.loaded || v.err != nil) {
		return
	}
	v.loading = true
	v.seq++
	seq := v.seq
	w.ws.WorktreeFiles(func(list []string, err error) {
		if seq != v.seq {
			return
		}
		v.loading = false
		v.loaded = true
		v.err = err
		v.entries = list
		v.gen = rs.Gen
		v.at = rs.StatusTime
		v.status = fileStatuses(rs)
		v.rootName = rs.Repo.Name()
		if v.sel == "" {
			v.sel = projectRootKey
		}
		v.rebuild()
		w.Invalidate()
	})
}

// syncProjectRepositories gives the row model the current workspace list. The
// active root is the repository whose file tree is loaded; a missing active
// root falls back to the summary matching the open repository, then to that
// repository's own root, so the panel never loses its active row.
func (w *Window) syncProjectRepositories(v *filesView) {
	roots := make([]string, 0, len(w.ws.Repositories))
	for _, s := range w.ws.Repositories {
		roots = append(roots, s.Root)
	}
	active := w.ws.ActiveRoot
	if active == "" {
		for _, s := range w.ws.Repositories {
			if w.repositoryActive(s.Root) {
				active = s.Root
				break
			}
		}
	}
	if active == "" {
		if rs := w.repo(); rs != nil {
			active = rs.Repo.Root
		}
	}
	if active != "" && !slices.Contains(roots, active) {
		roots = append(roots, active)
	}
	v.syncRepositories(roots, active)
}

// projectRepositoryHint is the muted summary of a repository row: the branch
// and change counts for a loaded snapshot, the failure for a broken one, or
// the action that will load it. The active single-repository layout keeps the
// path hint it had before the workspace row existed.
func (w *Window) projectRepositoryHint(root string, active bool) (string, ui.Color) {
	if active {
		if rs := w.repo(); rs != nil {
			if !w.fm.grouped() {
				return repoRootLabel(rs.Repo.Root), w.pal.TextSecond
			}
			if rs.StatusLoaded {
				return repositoryBranchLabel(rs.Status) + " · " + repositoryCounts(rs.Status), w.pal.TextSecond
			}
			return w.t(i18n.ReposLoadingStatus), w.pal.TextSecond
		}
	}
	i := w.repositoryIndex(root)
	if i < 0 {
		return "", w.pal.TextSecond
	}
	s := w.ws.Repositories[i]
	switch {
	case s.Err != nil:
		return w.t(i18n.ReposCannotReadStatus, repositoryOneLine(w.ws.ErrorText(s.Err))), w.pal.Error
	case !s.Loaded:
		return w.t(i18n.WorkspaceRepoOpenToLoad), w.pal.TextSecond
	default:
		return w.repositorySubtitle(s), w.pal.TextSecond
	}
}

func (w *Window) paintProject(r ui.Rect) {
	rs := w.repo()
	if rs == nil {
		return
	}
	v := &w.fm
	if v.filter == nil {
		v.init()
	}
	// The workspace list changes when a repository is added, removed or
	// switched; the rows follow it before the file list is requested, so a
	// repository row exists even while its tree is still loading.
	w.syncProjectRepositories(v)
	w.ensureFiles()
	w.island(r)

	// Header: the tool window title and its actions, like IntelliJ's Project
	// view. New/delete are absent because the panel is read-only.
	hr := splitTop(&r, 40)
	panelTitle := w.t(i18n.WorkspaceHintProject)
	titleW := w.p.width(panelTitle, fontUI, semibold)
	title := rect(hr.X+8, hr.Y+6, titleW+32, hr.Height-12)
	if w.hovered(idProjectMenu) {
		w.p.round(title, 5, w.pal.ToolHover)
	}
	w.p.text(panelTitle, rect(title.X+4, title.Y, titleW+2, title.Height), fontUI, semibold, w.pal.Text)
	w.p.iconIn(icons.ChevronDown, rect(title.X+titleW+8, title.Y+2, 16, title.Height-4))
	w.c.HitWithOptions(idProjectMenu, title, w.t(i18n.WorkspaceTipProjectOptions), ui.HitOptions{Role: ui.RoleButton, SkipFocus: true})
	if w.clicked(idProjectMenu, nil) {
		w.after(func() {
			w.openMenuBelow(title, w.projectWindowMenu(), 220)
		})
	}
	rx := right(hr) - 8 - actionSize
	if w.action(idProjectHide, rx, hr.Y+(hr.Height-actionSize)/2, icons.Hide, w.t(i18n.WorkspaceActionHide)+"\t⌘1", actOpts{}) {
		w.after(w.toggleProject)
	}
	rx -= actionSize + 2
	if w.action(idProjectFindFiles, rx, hr.Y+(hr.Height-actionSize)/2, icons.Find, w.t(i18n.SearchMenuFindInFiles)+"\t⇧⌘F", actOpts{}) {
		w.after(w.openFindInFiles)
	}
	rx -= actionSize + 2
	if w.action(idProjectQuick, rx, hr.Y+(hr.Height-actionSize)/2, icons.Search, w.t(i18n.ChromeMenuOpenFile)+"\t⇧⌘O", actOpts{}) {
		w.after(w.openQuickOpen)
	}
	rx -= actionSize + 2
	if w.action(idProjectExpand, rx, hr.Y+(hr.Height-actionSize)/2, icons.ExpandAll, w.t(i18n.WorkspaceActionExpandAll), actOpts{}) {
		w.after(w.fm.expandAll)
	}
	rx -= actionSize + 2
	if w.action(idProjectCollapse, rx, hr.Y+(hr.Height-actionSize)/2, icons.CollapseAll, w.t(i18n.WorkspaceActionCollapseAll), actOpts{}) {
		w.after(w.fm.collapseAll)
	}
	rx -= actionSize + 2
	if w.action(idProjectRefresh, rx, hr.Y+(hr.Height-actionSize)/2, icons.Refresh, w.t(i18n.WorkspaceActionRefresh), actOpts{}) {
		w.after(func() {
			// gen 0 never matches a live repository, so the next paint asks
			// again even after a failed listing whose status time is unchanged.
			w.fm.gen = 0
			w.ws.Refresh(true)
			w.Invalidate()
		})
	}
	if v.loaded && v.err == nil {
		info := w.pluralT(len(v.entries), i18n.WorkspaceCountFilesOne, i18n.WorkspaceCountFilesMany)
		iw := w.p.width(info, fontSmall, regular)
		if rx-iw-10 > hr.X+140 {
			w.p.text(info, rect(rx-iw-10, hr.Y, iw+2, hr.Height), fontSmall, regular, w.pal.TextSecond)
		}
	}
	w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
	r.Y++
	r.Height--

	fr := splitTop(&r, 34)
	field := rect(fr.X+8, fr.Y+4, fr.Width-16, 26)
	if w.field(idProjectFilter, v.filter, field, fieldOpts{icon: icons.Search, placeholder: w.t(i18n.RevisionFilter)}) {
		if i := v.firstFile(); i >= 0 {
			w.openRepoFile(v.rows[i].path, true)
		}
	}
	if q := strings.TrimSpace(v.filter.String()); q != v.query {
		v.rebuild()
		if i := v.rowIndex(v.sel); i >= 0 {
			v.scroll.reveal(float32(i)*rowH, rowH, r.Height)
		}
	}
	w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
	r.Y++
	r.Height--

	w.list(idProjectList, scProjectRows, r, w.t(i18n.WorkspaceHintProject))
	w.onKeys(idProjectList, w.projectKey)
	content := float32(len(v.rows))*rowH + 8
	v.scroll.clamp(content, r.Height)
	w.onScrollIn(r, func(dx, dy float32) {
		v.scroll.Y += dy
		v.scroll.clamp(content, r.Height)
	})

	switch {
	case v.loading && !v.loaded:
		w.p.textCenter(w.t(i18n.WorkspaceLoadingFiles), r, fontUI, regular, w.pal.TextSecond)
		return
	case v.err != nil:
		w.p.textCenter(w.ws.ErrorText(v.err), r, fontUI, regular, w.pal.Error)
		return
	case len(v.entries) == 0:
		w.p.textCenter(w.t(i18n.WorkspaceEmptyNoFiles), r, fontUI, regular, w.pal.TextSecond)
		return
	case v.query != "" && !v.hasTreeContent():
		w.p.textCenter(w.t(i18n.RevisionNoMatch), r, fontUI, regular, w.pal.TextSecond)
		return
	}

	w.c.Clip(r)
	first := max(0, int(v.scroll.Y/rowH)-1)
	last := min(len(v.rows), first+int(r.Height/rowH)+3)
	for k := first; k < last; k++ {
		row := v.rows[k]
		y := r.Y + 4 + float32(k)*rowH - v.scroll.Y
		rr := rect(r.X+4, y, r.Width-8, rowH)
		id := scProjectRows.At(k)
		selected := v.sel == row.key
		if selected {
			w.p.round(rr, 4, w.pal.Selection)
		} else if w.hovered(id) {
			w.p.round(rr, 4, w.pal.Hover)
		}
		rx := rr.X + 8 + float32(row.depth)*fileIndent
		switch {
		case row.workspace:
			expanded := v.workspaceOpen
			chev := icons.ChevronRight
			if expanded {
				chev = icons.ChevronDown
			}
			w.p.iconIn(chev, rect(rx, y, 16, rowH))
			rx += 18
			w.p.iconIn(icons.Project, rect(rx, y, 16, rowH))
			rx += 20
			name := w.t(i18n.WorkspaceProjectRoot)
			nw := w.p.text(name, rect(rx, y, max(0, right(rr)-rx-6), rowH), fontUI, semibold, w.pal.Text)
			hint := w.pluralT(len(v.repoRoots), i18n.WorkspaceCountRepositoriesOne, i18n.WorkspaceCountRepositoriesMany)
			hx := rx + nw + 8
			if hx < right(rr)-20 {
				w.p.text(w.p.truncateMiddle(hint, fontSmall, regular, right(rr)-hx-8), rect(hx, y, right(rr)-hx-8, rowH), fontSmall, regular, w.pal.TextSecond)
			}
			w.c.HitWithOptions(id, rr, name, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: selected, Expandable: true, Expanded: expanded})
		case row.repo:
			expanded := row.root && v.rootOpen
			if row.root {
				chev := icons.ChevronRight
				if expanded {
					chev = icons.ChevronDown
				}
				w.p.iconIn(chev, rect(rx, y, 16, rowH))
			}
			rx += 18
			icon := icons.Vcs
			weight := regular
			if row.root {
				icon = icons.Project
				weight = semibold
			}
			w.p.iconIn(icon, rect(rx, y, 16, rowH))
			rx += 20
			name := row.name
			if name == "" {
				name = repoRootName(row.repoRoot)
			}
			nw := w.p.text(name, rect(rx, y, max(0, right(rr)-rx-6), rowH), fontUI, weight, w.pal.Text)
			hint, hintColor := w.projectRepositoryHint(row.repoRoot, row.root)
			hx := rx + nw + 8
			if hint != "" && hx < right(rr)-20 {
				w.p.text(w.p.truncateMiddle(hint, fontSmall, regular, right(rr)-hx-8), rect(hx, y, right(rr)-hx-8, rowH), fontSmall, regular, hintColor)
			}
			opts := ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: selected}
			if row.root {
				opts.Expandable = true
				opts.Expanded = expanded
			}
			w.c.HitWithOptions(id, rr, name, opts)
			w.tip(id, repoRootHint(row.repoRoot))
		case row.dir:
			chev := icons.ChevronRight
			expanded := !v.collapsed[row.path]
			if expanded {
				chev = icons.ChevronDown
			}
			w.p.iconIn(chev, rect(rx, y, 16, rowH))
			rx += 18
			w.p.iconIn(icons.Folder, rect(rx, y, 16, rowH))
			rx += 20
			w.p.text(row.name, rect(rx, y, max(0, right(rr)-rx-6), rowH), fontUI, semibold, w.pal.Text)
			w.c.HitWithOptions(id, rr, row.path, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: selected, Expandable: true, Expanded: expanded})
		default:
			w.p.iconIn(fileIcon(row.path), rect(rx, y, 16, rowH))
			rx += 22
			fg := w.pal.Text
			if row.known {
				fg = w.statusColor(row.kind)
			}
			tw := max(0, right(rr)-rx-6)
			w.p.text(w.p.truncateMiddle(row.name, fontUI, regular, tw), rect(rx, y, tw, rowH), fontUI, regular, fg)
			w.c.HitWithOptions(id, rr, row.path, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: selected})
		}
		if w.c.Clicked(id) {
			k := k
			w.after(func() { w.clickFileRow(k) })
		}
		switch {
		case !row.dir:
			w.onDouble(id, func() { w.openRepoFile(row.path, true) })
		case row.workspace:
			w.onDouble(id, func() {
				v.workspaceOpen = !v.workspaceOpen
				v.rebuild()
			})
		case row.repo:
			w.onDouble(id, func() {
				if row.root {
					v.rootOpen = !v.rootOpen
					v.rebuild()
					return
				}
				w.switchRepository(row.repoRoot)
			})
		default:
			w.onDouble(id, func() {
				v.collapsed[row.path] = false
				v.rebuild()
			})
		}
		w.onContext(id, func(px, py float32) {
			v.sel = row.key
			w.openMenu(px, py, w.filesMenu(row))
		})
	}
	w.c.Unclip()
	w.vbar(idProjectBar, r, &v.scroll, content)
}

// clickFileRow selects a row, folds workspace/repository/directory rows, and
// opens a file in the preview tab. A non-active repository row switches the
// active repository instead of expanding: its tree is loaded only after the
// switch, never in parallel for every repository (ADR 0046).
func (w *Window) clickFileRow(i int) {
	v := &w.fm
	if i < 0 || i >= len(v.rows) {
		return
	}
	row := v.rows[i]
	v.sel = row.key
	switch {
	case row.workspace:
		v.workspaceOpen = !v.workspaceOpen
		v.rebuild()
	case row.repo:
		if row.root {
			v.rootOpen = !v.rootOpen
			v.rebuild()
			return
		}
		w.switchRepository(row.repoRoot)
	case row.dir:
		v.collapsed[row.path] = !v.collapsed[row.path]
		v.rebuild()
		if k := v.rowIndex(row.key); k >= 0 {
			v.scroll.reveal(float32(k)*rowH, rowH, w.areas[idProjectList].Height)
		}
	default:
		w.openRepoFile(row.path, false)
	}
}

// openRepoFile opens a working-tree file in the read-only viewer. A preview
// follows the selection until the tab is pinned.
func (w *Window) openRepoFile(p string, pin bool) {
	w.showDiff(vapp.DiffSource{Path: p, View: true}, pin)
}

// openRepoFileAt opens a working-tree file and asks the viewer to reveal line
// (1-based), for a content-search result. It reuses the tab of an already open
// file, so a second result in the same file only moves the viewport.
func (w *Window) openRepoFileAt(p string, line int, pin bool) {
	w.showDiff(vapp.DiffSource{Path: p, View: true}, pin)
	if t := w.ed.cur(); t != nil {
		t.pendingLine = max(0, line)
	}
	w.Invalidate()
}

// firstFile returns the first file row (directories do not open).
func (v *filesView) firstFile() int {
	for i, row := range v.rows {
		if !row.dir {
			return i
		}
	}
	return -1
}

func (w *Window) projectKey(e ui.KeyEvent) bool {
	v := &w.fm
	if len(v.rows) == 0 {
		return false
	}
	cur := v.rowIndex(v.sel)
	move := func(to int) {
		to = max(0, min(len(v.rows)-1, to))
		row := v.rows[to]
		v.sel = row.key
		v.scroll.reveal(float32(to)*rowH, rowH+8, w.areas[idProjectList].Height)
		if !row.dir {
			w.openRepoFile(row.path, false)
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
			switch {
			case row.workspace && v.workspaceOpen:
				v.workspaceOpen = false
				v.rebuild()
				return true
			case row.root && v.rootOpen:
				v.rootOpen = false
				v.rebuild()
				return true
			case row.dir && !row.repo && !row.workspace && !v.collapsed[row.path]:
				v.collapsed[row.path] = true
				v.rebuild()
				return true
			}
			for k := cur - 1; k >= 0; k-- {
				if v.rows[k].dir && v.rows[k].depth < row.depth {
					move(k)
					return true
				}
			}
			move(max(0, cur-1))
		}
	case ui.KeyRight:
		if cur >= 0 {
			row := v.rows[cur]
			switch {
			case row.workspace:
				if !v.workspaceOpen {
					v.workspaceOpen = true
					v.rebuild()
				} else if cur+1 < len(v.rows) {
					move(cur + 1)
				}
				return true
			case row.repo:
				if row.root {
					if !v.rootOpen {
						v.rootOpen = true
						v.rebuild()
					} else if cur+1 < len(v.rows) {
						move(cur + 1)
					}
					return true
				}
				// A non-active row has no loaded tree: expanding it means
				// switching to it, which loads the tree exactly once.
				w.switchRepository(row.repoRoot)
				return true
			case row.dir:
				if v.collapsed[row.path] {
					delete(v.collapsed, row.path)
					v.rebuild()
				} else if cur+1 < len(v.rows) && v.rows[cur+1].depth > row.depth {
					move(cur + 1)
				}
			}
		}
	case ui.KeyEnter:
		if cur >= 0 {
			row := v.rows[cur]
			switch {
			case row.workspace:
				v.workspaceOpen = !v.workspaceOpen
				v.rebuild()
			case row.repo:
				if row.root {
					v.rootOpen = !v.rootOpen
					v.rebuild()
					return true
				}
				w.switchRepository(row.repoRoot)
			case row.dir:
				v.collapsed[row.path] = !v.collapsed[row.path]
				v.rebuild()
			default:
				w.openRepoFile(row.path, true)
			}
		}
	case ui.KeySpace:
		if cur >= 0 {
			row := v.rows[cur]
			switch {
			case row.workspace:
				v.workspaceOpen = !v.workspaceOpen
				v.rebuild()
			case row.repo:
				if row.root {
					v.rootOpen = !v.rootOpen
					v.rebuild()
					return true
				}
				w.switchRepository(row.repoRoot)
			case row.dir:
				v.collapsed[row.path] = !v.collapsed[row.path]
				v.rebuild()
			}
		}
	default:
		return false
	}
	return true
}

// projectWindowMenu is the tool window's own menu, opened from its title.
func (w *Window) projectWindowMenu() []item {
	out := []item{
		{title: w.t(i18n.ChromeMenuOpenFile), icon: icons.Search, shortcut: "⇧⌘O", run: w.openQuickOpen},
	}
	// Content search is a backend capability: Subversion has no equivalent,
	// so the entry is absent rather than failing after being pressed.
	if w.ws.SupportsContentSearch() {
		out = append(out, item{title: w.t(i18n.SearchMenuFindInFiles), icon: icons.Search, shortcut: "⇧⌘F", run: w.openFindInFiles})
	}
	return append(out, []item{
		sep(),
		{title: w.t(i18n.WorkspaceActionExpandAll), icon: icons.ExpandAll, run: w.fm.expandAll},
		{title: w.t(i18n.WorkspaceActionCollapseAll), icon: icons.CollapseAll, run: w.fm.collapseAll},
		sep(),
		{title: w.t(i18n.WorkspaceMenuHideProject), shortcut: "⌘1", run: w.toggleProject},
	}...)
}

// filesMenu is the context menu of one Files row.
func (w *Window) filesMenu(row fileRow) []item {
	if row.workspace {
		return []item{
			{title: w.t(i18n.WorkspaceActionExpandAll), icon: icons.ExpandAll, run: w.fm.expandAll},
			{title: w.t(i18n.WorkspaceActionCollapseAll), icon: icons.CollapseAll, run: w.fm.collapseAll},
		}
	}
	if row.repo && !row.root {
		return []item{
			{title: w.t(i18n.ReposSwitch), icon: icons.Vcs, run: func() { w.switchRepository(row.repoRoot) }},
			sep(),
			{title: w.t(i18n.WorkspaceActionCopyPath), icon: icons.Copy, run: func() { w.copyText(row.repoRoot) }},
		}
	}
	if row.root {
		return []item{
			{title: w.t(i18n.WorkspaceActionRefresh), icon: icons.Refresh, run: func() { w.ws.Refresh(true) }},
			{title: w.t(i18n.WorkspaceActionExpandAll), icon: icons.ExpandAll, run: w.fm.expandAll},
			{title: w.t(i18n.WorkspaceActionCollapseAll), icon: icons.CollapseAll, run: w.fm.collapseAll},
			sep(),
			{title: w.t(i18n.WorkspaceActionCopyPath), icon: icons.Copy, run: func() { w.copyText(row.repoRoot) }},
		}
	}
	if row.dir {
		expanded := w.fm.query != "" || !w.fm.collapsed[row.path]
		label := w.t(i18n.WorkspaceMenuExpand)
		if expanded {
			label = w.t(i18n.WorkspaceMenuCollapse)
		}
		return []item{
			{title: label, icon: icons.ChevronDown, run: func() {
				w.fm.collapsed[row.path] = expanded
				w.fm.rebuild()
			}},
			sep(),
			{title: w.t(i18n.WorkspaceActionCopyRelativePath), icon: icons.Copy, run: func() { w.copyText(row.path) }},
		}
	}
	full := w.repoPath(row.path)
	items := []item{
		{title: w.t(i18n.WorkspaceMenuOpen), icon: icons.OpenNewTab, run: func() { w.openRepoFile(row.path, true) }},
	}
	if !w.readOnly() {
		items = append(items, item{title: w.t(i18n.WorkspaceMenuOpenInEditor), icon: icons.Edit, shortcut: "F4", run: func() { w.openExternal(row.path, 0) }})
	}
	items = append(items,
		sep(),
		item{title: w.t(i18n.WorkspaceMenuAnnotate), icon: icons.Vcs, run: func() { w.openFileView(row.path, "", true) }},
		item{title: w.t(i18n.WorkspaceMenuShowHistory), icon: icons.History, run: func() { w.showPathHistory(row.path) }},
		sep(),
		item{title: w.t(i18n.WorkspaceActionCopyPath), icon: icons.Copy, shortcut: "⇧⌘C", run: func() { w.copyText(full) }},
		item{title: w.t(i18n.WorkspaceActionCopyRelativePath), run: func() { w.copyText(row.path) }},
	)
	if !w.readOnly() {
		items = append(items, item{title: w.t(i18n.WorkspaceMenuRevealInFinder), icon: icons.Folder, run: func() { revealPath(full) }})
	}
	return items
}
