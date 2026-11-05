package viffui

import (
	"path/filepath"
	"sort"
	"strings"

	vivaapp "github.com/vivaui/viva/app"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// repositories.go is the workspace-wide Repositories overview. The workspace
// list is explicit: the user adds repositories, switches between them, and
// removes them from the list here, but the files stay on disk untouched.
//
// Only the active repository takes Git writes. Staging, committing and the
// other write actions stay in the normal Commit and Git tool windows of the
// active project; this dialog never pretends to commit several repositories at
// once. Its file rows are the shortcut into a repository: choosing one first
// makes that repository active, waits for its status, and only then opens the
// real working-tree diff, so a root-level file that exists in two repositories
// cannot be read from the wrong one.

const (
	repoOverviewRowH      = 56
	repoOverviewGroupH    = 26
	repoOverviewFileH     = 24
	repoOverviewListWidth = 320
	repoOverviewBodyH     = 360 // tallest body; shrinks in short windows
	repoOverviewHeaderH   = 78
)

// The overview names its controls in a scope of its own. Declaring the scope in
// init, after the shared tree exists, keeps the order in ids.go untouched (the
// same pattern as conflict.go), so ids.go does not need to know this dialog.
var (
	scRepositories      ui.Scope
	scRepositoryRows    ui.Scope
	scRepositoryFiles   ui.Scope
	scRepositoryBar     ui.Scope
	scRepositoryFileBar ui.Scope

	idRepositoriesPath     ui.ID
	idRepositoriesAdd      ui.ID
	idRepositoriesBrowse   ui.ID
	idRepositoriesRefresh  ui.ID
	idRepositoriesRemove   ui.ID
	idRepositoriesList     ui.ID
	idRepositoriesFileList ui.ID

	// Welcome screen entry: the workspace overview is reachable before a
	// repository is open, so the remaining rows are not stranded there.
	idWelcomeRepositories ui.ID
)

func init() {
	scRepositories = tree.Sub(fDialog, tree.Name("repositories"))
	scRepositoryRows = tree.Sub(scRepositories, tree.Name("rows"))
	scRepositoryFiles = tree.Sub(scRepositories, tree.Name("files"))
	scRepositoryBar = tree.Sub(scRepositories, tree.Name("scroll"))
	scRepositoryFileBar = tree.Sub(scRepositories, tree.Name("file-scroll"))

	idRepositoriesPath = tree.ID(scRepositories, "path")
	idRepositoriesAdd = tree.ID(scRepositories, "add")
	idRepositoriesBrowse = tree.ID(scRepositories, "browse")
	idRepositoriesRefresh = tree.ID(scRepositories, "refresh")
	idRepositoriesRemove = tree.ID(scRepositories, "remove")
	idRepositoriesList = tree.ID(scRepositories, "list")
	idRepositoriesFileList = tree.ID(scRepositories, "file-list")
	idWelcomeRepositories = tree.ID(fWelcome, "repositories")
}

// RepositorySummary is the UI's read-only copy of one workspace row.
type repositorySummary = vapp.RepositorySummary

// repositoriesView is the dialog's state. It lives in the open dialog closure
// and is dropped when the dialog closes; the workspace list itself is the
// model, so a background result only has to ask for a repaint.
type repositoriesView struct {
	path    *text.Editor
	sel     int
	scroll  scroller
	files   scroller
	loading bool
	adding  bool
	err     string
}

// openRepositories shows the workspace overview. It is also reachable with no
// active repository (the window keeps painting dialogs over the welcome
// screen); adding and switching still work from there.
func (w *Window) openRepositories() {
	v := &repositoriesView{path: text.NewEditor("")}
	if i := w.activeRepositoryIndex(); i >= 0 {
		v.sel = i
	}
	w.openDialog(&dialog{
		title: w.t(i18n.ReposTitle), width: 900, ok: w.t(i18n.ReposSwitch), cancel: w.t(i18n.CommonClose),
		body: func(f *form) { w.paintRepositories(f, v) },
		okDisabled: func() bool {
			_, ok := w.repositoryAt(v.sel)
			return !ok
		},
		onOK: func() bool {
			if s, ok := w.repositoryAt(v.sel); ok {
				w.switchRepository(s.Root)
			}
			return true
		},
	})
	// Opening the overview is the explicit refresh path of the workspace list:
	// non-active rows have no watcher, so their last known status may be old.
	w.refreshRepositories(v)
}

// repositoryAt returns the row at i of the current workspace list.
func (w *Window) repositoryAt(i int) (repositorySummary, bool) {
	if i < 0 || i >= len(w.ws.Repositories) {
		return repositorySummary{}, false
	}
	return w.ws.Repositories[i], true
}

// repositoryActive reports that root is the repository whose Git state the
// window shows. The workspace's canonical root is the identity, not the path
// the user typed.
func (w *Window) repositoryActive(root string) bool {
	if w.ws.Repo == nil {
		return false
	}
	if w.ws.ActiveRoot != "" {
		return w.ws.ActiveRoot == root
	}
	return filepath.Clean(w.ws.Repo.Repo.Root) == filepath.Clean(root)
}

func (w *Window) activeRepositoryIndex() int {
	for i, s := range w.ws.Repositories {
		if w.repositoryActive(s.Root) {
			return i
		}
	}
	return -1
}

// repositoryIndex finds a row by root.
func (w *Window) repositoryIndex(root string) int {
	for i, s := range w.ws.Repositories {
		if s.Root == root {
			return i
		}
	}
	return -1
}

// paintRepositories is the dialog body: the add row, the status line, the
// repository list and the files of the selected row.
func (w *Window) paintRepositories(f *form, v *repositoriesView) {
	list := w.ws.Repositories
	if len(list) == 0 {
		v.sel = 0
	} else if v.sel >= len(list) {
		v.sel = len(list) - 1
	}

	f.custom(30, func(r ui.Rect) {
		aw := w.buttonWidth("Add")
		bw := w.buttonWidth("Browse…")
		fieldW := max(160, r.Width-aw-bw-actionSize-32)
		fr := rect(r.X, r.Y+1, fieldW, fieldH)
		if w.field(idRepositoriesPath, v.path, fr, fieldOpts{placeholder: "Repository path", icon: icons.Folder}) {
			w.after(func() { w.addRepository(v) })
		}
		w.onKeys(idRepositoriesPath, func(e ui.KeyEvent) bool {
			if e.Key == ui.KeyEnter {
				w.after(func() { w.addRepository(v) })
				return true
			}
			return false
		})
		x := right(fr) + 8
		if w.button(idRepositoriesAdd, rect(x, r.Y+1, aw, fieldH), "Add", btnDefault, v.adding) {
			w.after(func() { w.addRepository(v) })
		}
		x += aw + 8
		if w.button(idRepositoriesBrowse, rect(x, r.Y+1, bw, fieldH), "Browse…", btnNormal, false) {
			w.after(func() { w.browseRepositoryPath(v) })
		}
		if w.action(idRepositoriesRefresh, right(r)-actionSize, r.Y+2, icons.Refresh, "Refresh", actOpts{disabled: v.loading}) {
			w.after(func() { w.refreshRepositories(v) })
		}
	})
	f.custom(22, func(r ui.Rect) {
		msg := ""
		col := w.pal.TextSecond
		switch {
		case v.err != "":
			msg, col = w.p.truncateMiddle(v.err, fontUI, regular, r.Width), w.pal.Error
		case v.adding:
			msg = "Adding…"
		case v.loading:
			msg = "Refreshing…"
		case len(list) == 0:
			msg = w.t(i18n.ReposNone)
		default:
			msg = plural(len(list), "repository", "repositories") + " in the workspace"
			if active := w.activeRepositoryIndex(); active >= 0 {
				msg += " · active: " + filepath.Base(list[active].Root)
			} else {
				msg += " · no active repository"
			}
		}
		w.p.text(msg, rect(r.X, r.Y, r.Width, 20), fontUI, regular, col)
	})
	f.gap(6)
	// The dialog is modal and its button row has a fixed height: the body has
	// to give way in a 900x600 window instead of pushing the buttons off the
	// screen. The size only changes when the window does, so the measured
	// content height stays stable while painting.
	f.custom(w.repositoryOverviewBodyHeight(), func(r ui.Rect) { w.paintRepositoryPanes(v, r) })
	f.gap(8)
	f.custom(28, func(r ui.Rect) {
		_, ok := w.repositoryAt(v.sel)
		if w.button(idRepositoriesRemove, rect(r.X, r.Y, w.buttonWidth("Remove from Workspace"), buttonH), "Remove from Workspace", btnNormal, !ok) {
			w.after(func() { w.removeRepository(v) })
		}
		note := w.t(i18n.ReposActiveOnly)
		nw := w.p.width(note, fontSmall, regular)
		if nw < r.Width-220 {
			w.p.text(note, rect(right(r)-nw, r.Y, nw, buttonH), fontSmall, regular, w.pal.TextDisable)
		}
	})
}

// repositoryOverviewBodyHeight keeps the whole dialog inside the window,
// including its title, status line, button rows and padding.
func (w *Window) repositoryOverviewBodyHeight() float32 {
	return clampF(w.size.Height-270, 200, repoOverviewBodyH)
}

// paintRepositoryPanes paints the repository list on the left and the selected
// repository's branch/status/files on the right. Both scroll: the list has no
// row limit, and the file list shows the whole status instead of an excerpt.
func (w *Window) paintRepositoryPanes(v *repositoriesView, r ui.Rect) {
	list := w.ws.Repositories
	leftW := min(repoOverviewListWidth, max(220, r.Width*0.38))
	left := rect(r.X, r.Y, leftW, r.Height)
	w.p.outline(left, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
	inner := inset(left, 1, 1)

	content := float32(len(list)) * repoOverviewRowH
	v.scroll.clamp(content, inner.Height)
	w.onScrollIn(inner, func(dx, dy float32) {
		v.scroll.Y += dy
		v.scroll.clamp(content, inner.Height)
	})
	w.list(idRepositoriesList, scRepositoryRows, inner, "Repositories")
	w.onKeys(idRepositoriesList, func(e ui.KeyEvent) bool {
		switch e.Key {
		case ui.KeyDown:
			w.after(func() { v.sel = min(len(list)-1, v.sel+1) })
		case ui.KeyUp:
			w.after(func() { v.sel = max(0, v.sel-1) })
		case ui.KeyEnter:
			if s, ok := w.repositoryAt(v.sel); ok {
				w.after(func() { w.switchRepository(s.Root) })
			}
		default:
			return false
		}
		return true
	})
	w.c.Clip(inner)
	if len(list) == 0 {
		w.p.textCenter("No repositories", rect(inner.X, inner.Y+inner.Height/2-10, inner.Width, 20), fontUI, regular, w.pal.TextSecond)
	}
	for i, s := range list {
		row := rect(inner.X, inner.Y+float32(i)*repoOverviewRowH-v.scroll.Y, inner.Width, repoOverviewRowH)
		if bottom(row) <= inner.Y || row.Y >= bottom(inner) {
			continue
		}
		w.paintRepositoryRow(v, i, s, row)
	}
	w.c.Unclip()
	w.vbar(scRepositoryBar.At(0), inner, &v.scroll, content)

	rightR := rect(right(left)+16, r.Y, r.Width-leftW-16, r.Height)
	s, ok := w.repositoryAt(v.sel)
	if !ok {
		w.p.outline(rightR, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
		w.p.textCenter(w.t(i18n.ReposSelectHint), rightR, fontUI, regular, w.pal.TextSecond)
		return
	}
	w.paintRepositoryDetails(v, s, rightR)
}

func (w *Window) paintRepositoryRow(v *repositoriesView, i int, s repositorySummary, r ui.Rect) {
	id := scRepositoryRows.At(i)
	active := w.repositoryActive(s.Root)
	selected := v.sel == i
	switch {
	case selected:
		w.p.round(r, 4, w.pal.Selection)
	case w.hovered(id):
		w.p.round(r, 4, w.pal.Hover)
	}
	if active {
		w.p.round(rect(r.X+2, r.Y+10, 3, r.Height-20), 1.5, w.pal.Brand)
	}
	w.p.iconIn(icons.Vcs, rect(r.X+12, r.Y+9, 16, 16))
	tx := r.X + 36
	name := filepath.Base(s.Root)
	nameW := r.Width - 48
	if active {
		badge := "Active"
		bw := w.p.width(badge, fontTiny, semibold) + 14
		br := rect(right(r)-bw-10, r.Y+10, bw, 16)
		w.p.round(br, 8, w.pal.Brand)
		w.p.textCenter(badge, br, fontTiny, semibold, w.pal.BrandText)
		nameW -= bw + 12
	}
	w.p.text(name, rect(tx, r.Y+6, nameW, 20), fontUI, semibold, w.pal.Text)

	sub := w.repositorySubtitle(s)
	col := w.pal.TextSecond
	switch {
	case s.Err != nil:
		col = w.pal.Error
	case selected:
		col = w.pal.Text // the muted grey is hard to read on the selection
	}
	w.p.text(sub, rect(tx, r.Y+28, r.Width-44, 18), fontSmall, regular, col)

	w.c.HitWithOptions(id, r, name, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: selected})
	w.tip(id, s.Root)
	if w.c.Clicked(id) {
		w.after(func() {
			if v.sel != i {
				v.files = scroller{}
			}
			v.sel = i
		})
	}
	w.onDouble(id, func() {
		w.after(func() { w.switchRepository(s.Root) })
	})
}

// repositorySubtitle is the muted second line of a list row: the branch and a
// short status summary, or the failure that hides the status.
func (w *Window) repositorySubtitle(s repositorySummary) string {
	if s.Err != nil {
		return "Status error: " + repositoryOneLine(vapp.ErrorText(s.Err))
	}
	branch := repositoryBranchLabel(s.Status)
	if !s.Loaded {
		return branch + " · loading…"
	}
	return branch + " · " + repositoryCounts(s.Status)
}

// repositoryBranchLabel names the checked-out state of a repository snapshot.
func repositoryBranchLabel(st domain.Status) string {
	b := st.Branch
	if b.Detached {
		if b.Head != "" {
			return "detached " + domain.ShortHash(b.Head)
		}
		return "detached"
	}
	if b.Branch != "" {
		return b.Branch
	}
	if b.Head != "" {
		return domain.ShortHash(b.Head)
	}
	return "No commits"
}

// repositoryCounts summarizes a status the same way a row does in the Commit
// tool window.
func repositoryCounts(st domain.Status) string {
	staged, unstaged, untracked, conflicts := st.Counts()
	var parts []string
	if conflicts > 0 {
		parts = append(parts, plural(conflicts, "conflict", "conflicts"))
	}
	if staged > 0 {
		parts = append(parts, itoa(staged)+" staged")
	}
	if unstaged > 0 {
		parts = append(parts, itoa(unstaged)+" changed")
	}
	if untracked > 0 {
		parts = append(parts, itoa(untracked)+" unversioned")
	}
	if len(parts) == 0 {
		return "Clean"
	}
	return strings.Join(parts, " · ")
}

// paintRepositoryDetails paints the selected repository's identity, branch and
// status counts, then its changed files.
func (w *Window) paintRepositoryDetails(v *repositoriesView, s repositorySummary, r ui.Rect) {
	head := rect(r.X, r.Y, r.Width, repoOverviewHeaderH)
	w.projectBadge(rect(head.X+2, head.Y+4, 34, 34), filepath.Base(s.Root), 7)
	tx := head.X + 46
	active := w.repositoryActive(s.Root)
	name := filepath.Base(s.Root)
	nameW := head.Width - 50
	if active {
		badge := "Active"
		bw := w.p.width(badge, fontTiny, semibold) + 14
		br := rect(right(head)-bw, head.Y+8, bw, 16)
		w.p.round(br, 8, w.pal.Brand)
		w.p.textCenter(badge, br, fontTiny, semibold, w.pal.BrandText)
		nameW -= bw + 12
	}
	w.p.text(name, rect(tx, head.Y+2, nameW, 22), 15, semibold, w.pal.Text)
	w.p.text(shortenHome(s.Root), rect(tx, head.Y+24, head.Width-50, 18), fontSmall, regular, w.pal.TextSecond)

	by := head.Y + 48
	w.p.iconIn(icons.Vcs, rect(tx, by+1, 14, 16))
	branch := repositoryBranchLabel(s.Status)
	bw := w.p.text(branch, rect(tx+20, by, 240, 18), fontUI, regular, w.pal.TextMuted)
	counts := ""
	if s.Loaded {
		counts = repositoryCounts(s.Status)
	} else if s.Err == nil {
		counts = "loading…"
	}
	if counts != "" && tx+20+bw+12 < right(head)-8 {
		w.p.text(counts, rect(tx+20+bw+12, by, right(head)-(tx+20+bw+12), 18), fontSmall, regular, w.pal.TextSecond)
	}

	box := rect(r.X, r.Y+repoOverviewHeaderH, r.Width, r.Height-repoOverviewHeaderH)
	w.p.outline(box, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
	inner := inset(box, 1, 1)
	filesTop := inner.Y
	if s.Err != nil {
		br := rect(inner.X, inner.Y, inner.Width, 24)
		w.p.fill(br, w.pal.BannerError)
		msg := w.t(i18n.ReposCannotReadStatus, repositoryOneLine(vapp.ErrorText(s.Err)))
		w.p.text(msg, rect(br.X+8, br.Y, br.Width-16, br.Height), fontSmall, regular, w.pal.Error)
		filesTop += 24
	}
	files := rect(inner.X, filesTop, inner.Width, bottom(inner)-filesTop)
	w.paintRepositoryFiles(v, s, files)
}

// repositoryFileRow is a group header or one changed file in the details list.
type repositoryFileRow struct {
	header bool
	area   domain.Area
	entry  domain.StatusEntry
}

// repositoryFileRows flattens a status into group headers and file rows, in the
// same area order the Commit tool window uses. Every entry and every area is
// kept: the list scrolls instead of cutting the changes off.
func repositoryFileRows(st domain.Status) []repositoryFileRow {
	var rows []repositoryFileRow
	for _, a := range areaOrder {
		entries := repositoryEntries(st, a)
		if len(entries) == 0 {
			continue
		}
		rows = append(rows, repositoryFileRow{header: true, area: a})
		for _, e := range entries {
			rows = append(rows, repositoryFileRow{area: a, entry: e})
		}
	}
	return rows
}

// repositoryEntries filters a status snapshot by area, sorted by path.
func repositoryEntries(st domain.Status, a domain.Area) []domain.StatusEntry {
	var out []domain.StatusEntry
	for _, e := range st.Entries {
		switch a {
		case domain.AreaStaged:
			if e.Staged() {
				out = append(out, e)
			}
		case domain.AreaUnstaged:
			if e.Unstaged() {
				out = append(out, e)
			}
		case domain.AreaUntracked:
			if e.Untracked() {
				out = append(out, e)
			}
		case domain.AreaConflicts:
			if e.Conflicted() {
				out = append(out, e)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (w *Window) paintRepositoryFiles(v *repositoriesView, s repositorySummary, r ui.Rect) {
	rows := repositoryFileRows(s.Status)
	content := float32(0)
	for _, row := range rows {
		if row.header {
			content += repoOverviewGroupH
		} else {
			content += repoOverviewFileH
		}
	}
	v.files.clamp(content, r.Height)
	w.onScrollIn(r, func(dx, dy float32) {
		v.files.Y += dy
		v.files.clamp(content, r.Height)
	})
	w.list(idRepositoriesFileList, scRepositoryFiles, r, w.t(i18n.ReposChangedFiles))
	if len(rows) == 0 {
		msg := w.t(i18n.ReposClean)
		switch {
		case s.Err != nil:
			msg = w.t(i18n.ReposNoCached)
		case !s.Loaded:
			msg = w.t(i18n.ReposLoadingStatus)
		}
		w.p.textCenter(msg, r, fontUI, regular, w.pal.TextSecond)
		return
	}
	w.c.Clip(r)
	y := r.Y - v.files.Y
	for i, row := range rows {
		if row.header {
			hr := rect(r.X, y, r.Width, repoOverviewGroupH)
			if bottom(hr) > r.Y && hr.Y < bottom(r) {
				w.p.text(w.areaTitle(row.area), rect(hr.X+10, hr.Y, hr.Width-20, hr.Height), fontUI, semibold, w.pal.Text)
				count := len(repositoryEntries(s.Status, row.area))
				cw := w.p.width(itoa(count), fontSmall, regular)
				w.p.text(itoa(count), rect(right(hr)-cw-10, hr.Y, cw, hr.Height), fontSmall, regular, w.pal.TextSecond)
			}
			y += repoOverviewGroupH
			continue
		}
		e := row.entry
		rr := rect(r.X+8, y, r.Width-16, repoOverviewFileH)
		if bottom(rr) <= r.Y || rr.Y >= bottom(r) {
			y += repoOverviewFileH
			continue
		}
		id := scRepositoryFiles.At(i)
		if w.hovered(id) {
			w.p.round(rr, 4, w.pal.Hover)
		}
		kind := e.KindIn(row.area)
		w.p.iconIn(fileIcon(e.Path), rect(rr.X, rr.Y, 16, rr.Height))
		x := rr.X + 22
		name := e.Path
		col := w.statusColor(kind)
		tw := w.p.text(name, rect(x, rr.Y, right(rr)-x-8, rr.Height), fontUI, regular, col)
		if kind == domain.ChangeDeleted {
			w.p.hline(x, rr.Y+rr.Height/2, tw, col)
		}
		if e.OrigPath != "" && row.area == domain.AreaStaged {
			hint := "← " + e.OrigPath
			hx := x + tw + 10
			if hx < right(rr)-20 {
				w.p.text(hint, rect(hx, rr.Y, right(rr)-hx-8, rr.Height), fontSmall, regular, w.pal.TextSecond)
			}
		}
		w.c.HitWithOptions(id, rr, e.Path, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true})
		root, path, area := s.Root, e.Path, row.area
		if w.c.Clicked(id) {
			w.after(func() {
				w.dialog = nil
				w.openRepositoryFile(root, path, area)
			})
		}
		y += repoOverviewFileH
	}
	w.c.Unclip()
	w.vbar(scRepositoryFileBar.At(0), r, &v.files, content)
}

// ---- actions ---------------------------------------------------------------

// refreshRepositories asks for every row's status. The backend answers on the
// UI thread when all rows are done.
func (w *Window) refreshRepositories(v *repositoriesView) {
	if v.loading {
		return
	}
	v.loading = true
	w.ws.RefreshRepositories(func() {
		w.after(func() {
			v.loading = false
			w.Invalidate()
		})
	})
	w.Invalidate()
}

// addRepository adds the path currently typed. The dialog stays open on
// failure and shows Git's own message: an error must not look like a row.
func (w *Window) addRepository(v *repositoriesView) {
	if v.adding {
		return
	}
	dir := expandRepositoryPath(strings.TrimSpace(v.path.String()))
	if dir == "" {
		v.err = w.t(i18n.ReposPathRequired)
		w.Invalidate()
		return
	}
	v.err = ""
	v.adding = true
	w.ws.AddRepository(dir, func(err error) {
		w.after(func() {
			v.adding = false
			if err != nil {
				v.err = w.t(i18n.ReposCannotAdd, repositoryOneLine(vapp.ErrorText(err)))
				w.Invalidate()
				return
			}
			setText(v.path, "")
			if i := w.repositoryIndex(canonicalRepositoryRoot(dir)); i >= 0 {
				v.sel = i
				v.files = scroller{}
			}
			// The new row is unloaded; the overview's refresh path gives it a
			// status without waiting for a watcher that only the active
			// repository has.
			w.refreshRepositories(v)
		})
	})
	w.Invalidate()
}

// browseRepositoryPath fills the path field from the native folder chooser.
// Tests never use it: they type the path, so adding does not depend on a
// platform dialog.
func (w *Window) browseRepositoryPath(v *repositoriesView) {
	dir, err := vivaapp.OpenDirectory(vivaapp.OpenDirectoryOptions{Title: "Add Repository"})
	if err != nil || dir == "" {
		return
	}
	setText(v.path, dir)
	w.Invalidate()
}

// removeRepository drops the selected row from the workspace list. It is not a
// disk operation: the files and their Git data stay where they are.
func (w *Window) removeRepository(v *repositoriesView) {
	s, ok := w.repositoryAt(v.sel)
	if !ok {
		return
	}
	if err := w.ws.RemoveRepository(s.Root); err != nil {
		v.err = w.t(i18n.ReposCannotRemove, repositoryOneLine(vapp.ErrorText(err)))
		w.Invalidate()
		return
	}
	v.err = ""
	if w.pendingRepositoryFile != nil && w.pendingRepositoryFile.root == s.Root {
		w.pendingRepositoryFile = nil
	}
	if n := len(w.ws.Repositories); n == 0 {
		v.sel = 0
	} else if v.sel >= n {
		v.sel = n - 1
	}
	v.files = scroller{}
	w.Invalidate()
}

// switchRepository makes root the active repository and closes the overview.
// The switch is asynchronous; the window repaints when it lands. The backend
// is called even for the current root: a new activation request supersedes an
// older one that is still in flight, so a switch must never look like a no-op
// just because the click happens to hit the current row.
func (w *Window) switchRepository(root string) {
	w.dialog = nil
	// An explicit switch wins over a file the user clicked earlier: that
	// pending target must not open a diff after this activation lands.
	w.pendingRepositoryFile = nil
	w.ws.ActivateRepository(root, func(err error) {
		w.after(func() {
			if err != nil {
				w.ws.NotifyError("Cannot switch repository "+filepath.Base(root), err)
			}
			w.Invalidate()
		})
	})
	w.Invalidate()
}

// ---- opening a file from the overview --------------------------------------

// repositoryOpenTarget is a file the user chose in the overview before its
// repository was active and its status was known. It waits across the
// repository switch instead of opening a diff with the previous repository's
// data (or, worse, reading a same-named file from it). Window owns the current
// target, so closing the window drops it with the window; the type itself is
// declared here because only this dialog creates one.
type repositoryOpenTarget struct {
	// from is the active root at the time of the click. If another repository
	// becomes active instead, the request is stale and is dropped.
	from string
	root string
	path string
	area domain.Area
}

// openRepositoryFile opens path as a working-tree diff of root. When root is
// not active, it becomes active first; the diff is opened by
// applyPendingRepositoryFile only after that repository's status has loaded.
func (w *Window) openRepositoryFile(root, path string, area domain.Area) {
	// The newest file choice replaces any request still waiting for its
	// repository: an older target must never open over this selection.
	w.pendingRepositoryFile = nil
	t := &repositoryOpenTarget{from: w.ws.ActiveRoot, root: root, path: path, area: area}
	if w.repositoryActive(root) {
		if rs := w.repo(); rs != nil && rs.StatusLoaded {
			if src, ok := repositoryDiffSource(rs, path, area); ok {
				w.showDiff(src, true)
			} else {
				w.notifyRepositoryFileGone(t)
			}
			return
		}
		w.pendingRepositoryFile = t
		w.Invalidate()
		return
	}
	w.pendingRepositoryFile = t
	w.ws.ActivateRepository(root, func(err error) {
		if w.pendingRepositoryFile != t {
			return // superseded by a newer request or cleared by the user
		}
		if err != nil {
			w.pendingRepositoryFile = nil
			w.ws.NotifyError("Cannot open repository "+filepath.Base(root), err)
			w.Invalidate()
			return
		}
		// The status query may still be running; the paint path opens the file
		// when it has landed.
		w.Invalidate()
	})
	w.Invalidate()
}

// applyPendingRepositoryFile runs from paintMain: once the requested repository
// is active and its status is loaded, it opens the real diff. Keeping the wait
// in the paint path means no polling loop and no state outside the window.
func (w *Window) applyPendingRepositoryFile() {
	t := w.pendingRepositoryFile
	if t == nil {
		return
	}
	rs := w.repo()
	if rs == nil {
		return
	}
	if !w.repositoryActive(t.root) {
		// Still waiting for the activation the click started, unless another
		// repository won the race and this request can never match.
		if w.ws.ActiveRoot != "" && w.ws.ActiveRoot != t.from {
			w.pendingRepositoryFile = nil
			w.ws.Notify(vapp.NoticeInfo, "Repository switched", "The file was not opened because another repository became active.")
		}
		return
	}
	if !rs.StatusLoaded {
		return
	}
	w.pendingRepositoryFile = nil
	src, ok := repositoryDiffSource(rs, t.path, t.area)
	if !ok {
		w.notifyRepositoryFileGone(t)
		return
	}
	// The diff is opened after this paint. The repository may have been
	// switched again by then (a second activation would reset the editor), so
	// the deferred callback re-checks that it still talks to the same state
	// object and generation the source was built from.
	gen := rs.Gen
	w.after(func() {
		if w.repo() != rs || rs.Gen != gen {
			return
		}
		w.showDiff(src, true)
	})
}

// repositoryDiffSource builds the diff source for path from the active
// repository's current status. The area the overview showed wins; if the file
// moved (for example it was staged meanwhile), the area that still holds it is
// used instead of guessing.
func repositoryDiffSource(rs *vapp.RepoState, path string, area domain.Area) (vapp.DiffSource, bool) {
	if rs == nil {
		return vapp.DiffSource{}, false
	}
	if src, ok := repositoryDiffSourceIn(rs, path, area); ok {
		return src, true
	}
	for _, a := range areaOrder {
		if a == area {
			continue
		}
		if src, ok := repositoryDiffSourceIn(rs, path, a); ok {
			return src, true
		}
	}
	return vapp.DiffSource{}, false
}

func repositoryDiffSourceIn(rs *vapp.RepoState, path string, area domain.Area) (vapp.DiffSource, bool) {
	for _, e := range rs.Entries(area) {
		if e.Path != path {
			continue
		}
		src := vapp.DiffSource{Path: e.Path, Area: area, Working: true, Kind: e.KindIn(area)}
		if area == domain.AreaStaged {
			src.OldPath = e.OrigPath
		}
		return src, true
	}
	return vapp.DiffSource{}, false
}

func (w *Window) notifyRepositoryFileGone(t *repositoryOpenTarget) {
	w.ws.Notify(vapp.NoticeWarning, "No changes to show",
		t.path+" has no local changes in "+filepath.Base(t.root)+" any more.")
}

// ---- path helpers ----------------------------------------------------------

// expandRepositoryPath accepts ~ like a shell, so a typed path behaves like the
// one the user has in mind.
func expandRepositoryPath(p string) string {
	switch {
	case p == "~":
		return homeDir()
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(homeDir(), p[2:])
	}
	return p
}

// canonicalRepositoryRoot is the identity the workspace list uses: absolute,
// symlinks resolved, cleaned.
func canonicalRepositoryRoot(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return filepath.Clean(abs)
}

// repositoryOneLine keeps a multibyte Git message readable on one dialog line.
func repositoryOneLine(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
