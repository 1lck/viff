package viffui

import (
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// Change lists group the working-tree files of one repository. They are a
// view concern: Git's index stays the single source of truth, so a list never
// isolates a commit. The filter therefore applies only to unstaged and
// untracked files; staged files and conflicts are always shown, and every
// commit still includes all staged files, whichever list they belong to.

const (
	// allChangeLists shows every file and is the default. It keeps the
	// historical Commit window behavior.
	allChangeLists = ""
	// defaultChangeListID is the backend's built-in list. It cannot be
	// renamed or deleted.
	defaultChangeListID = "default"

	changeListBarH   = 28
	changeListHintH  = 24
	changeListComboW = 196
)

// The Change Lists widgets get their own feature scope so ids.go, which other
// work also edits, does not need to change. The scope is created in init so it
// registers after the ids.go declarations, as the tree convention expects.
var (
	fChangeLists  ui.Scope
	idChangeLists ui.ID
)

func init() {
	fChangeLists = tree.Feature("changelists")
	idChangeLists = tree.ID(fChangeLists, "select")
}

func (w *Window) changeLists() []vapp.ChangeList {
	if w.ws == nil {
		return nil
	}
	return w.ws.Lists()
}

func (w *Window) changeListByID(id string) (vapp.ChangeList, bool) {
	for _, l := range w.changeLists() {
		if l.ID == id {
			return l, true
		}
	}
	return vapp.ChangeList{}, false
}

// changeListFilterName is the label of the active filter.
func (w *Window) changeListFilterName() string {
	if w.cm.listID == "" {
		return w.t(i18n.ChangelistAllLists)
	}
	if l, ok := w.changeListByID(w.cm.listID); ok {
		return l.Name
	}
	return w.cm.listID
}

// changeListEntries returns the status entries of area a that the active
// filter shows. Conflicts and staged files are never filtered: a commit takes
// every staged file, so hiding them would misrepresent what Commit does.
func (w *Window) changeListEntries(a domain.Area) []domain.StatusEntry {
	rs := w.repo()
	if rs == nil {
		return nil
	}
	entries := rs.Entries(a)
	id := w.cm.listID
	if id == allChangeLists || a == domain.AreaStaged || a == domain.AreaConflicts {
		return entries
	}
	out := make([]domain.StatusEntry, 0, len(entries))
	for _, e := range entries {
		if w.ws.ChangeListFor(e.Path) == id {
			out = append(out, e)
		}
	}
	return out
}

// changeListCount counts the files assigned to a list. The all-lists entry
// counts every changed file.
func (w *Window) changeListCount(id string) int {
	rs := w.repo()
	if rs == nil {
		return 0
	}
	n := 0
	for _, e := range rs.Status.Entries {
		if e.Ignored() {
			continue
		}
		if id == allChangeLists || w.ws.ChangeListFor(e.Path) == id {
			n++
		}
	}
	return n
}

// visibleChangeFileCount is the number of distinct files the filter shows.
func (w *Window) visibleChangeFileCount() int {
	seen := map[string]bool{}
	n := 0
	for _, a := range areaOrder {
		for _, e := range w.changeListEntries(a) {
			if seen[e.Path] {
				continue
			}
			seen[e.Path] = true
			n++
		}
	}
	return n
}

// stageableCount is the number of unstaged or untracked files the filter
// shows, which is exactly what Stage All would stage.
func (w *Window) stageableCount() int {
	return len(w.changeListEntries(domain.AreaUnstaged)) + len(w.changeListEntries(domain.AreaUntracked))
}

// stageAllVisible stages the unstaged and untracked files the active filter
// shows. With no filter this is every such file, as before.
func (w *Window) stageAllVisible() {
	if w.readOnly() {
		return
	}
	var paths []string
	for _, a := range []domain.Area{domain.AreaUnstaged, domain.AreaUntracked} {
		for _, e := range w.changeListEntries(a) {
			paths = append(paths, e.Path)
		}
	}
	if len(paths) > 0 {
		w.ws.StagePaths(paths)
	}
}

// setChangeList switches the filter and clears the old selection so actions
// cannot touch files the new filter hides.
func (w *Window) setChangeList(id string) {
	v := &w.cm
	if v.listID == id {
		return
	}
	v.listID = id
	w.clearChangeListSelection()
	v.scroll = scroller{}
}

func (w *Window) clearChangeListSelection() {
	clear(w.cm.sel)
	w.cm.anchor, w.cm.cursor = "", ""
}

// normalizeChangeList drops a filter whose list no longer exists (deleted, or
// a different repository).
func (w *Window) normalizeChangeList() {
	v := &w.cm
	if v.listID == allChangeLists {
		return
	}
	if _, ok := w.changeListByID(v.listID); ok {
		return
	}
	v.listID = allChangeLists
	w.clearChangeListSelection()
	v.scroll = scroller{}
}

// changeListsBarHeight reserves one row for the combo and, while a list is
// selected, a second full-width row for the commit-scope warning. The warning
// never shares a row with the combo: the Commit panel can be as narrow as
// 240px, where there is no space left for it.
func (w *Window) changeListsBarHeight() float32 {
	w.normalizeChangeList()
	if w.cm.listID != allChangeLists {
		return changeListBarH + changeListHintH
	}
	return changeListBarH
}

// paintChangeListsBar draws the filter combo, the visible file count and,
// while a list is selected, the warning that Commit is not list-scoped.
func (w *Window) paintChangeListsBar(r ui.Rect) {
	row := rect(r.X, r.Y, r.Width, changeListBarH)
	// Leave room for the count even at the narrowest panel.
	countW := float32(62)
	comboW := clampF(r.Width-8-10-countW-8, 96, changeListComboW)
	combo := rect(row.X+8, row.Y+2, comboW, 24)
	bg := ui.Color{}
	if w.pressed(idChangeLists) {
		bg = w.pal.ToolPressed
	} else if w.hovered(idChangeLists) {
		bg = w.pal.ToolHover
	}
	w.p.round(combo, 4, bg)
	w.p.outline(combo, 4, 1, w.pal.ControlBorder, w.pal.ControlBg)
	w.p.iconIn(icons.Changelist, rect(combo.X+4, combo.Y+4, 16, 16))
	titleW := combo.Width - 48
	title := w.p.truncateMiddle(w.changeListFilterName(), fontUI, regular, titleW)
	w.p.text(title, rect(combo.X+26, combo.Y, titleW, combo.Height), fontUI, regular, w.pal.Text)
	w.p.iconIn(icons.ChevronDown, rect(right(combo)-20, combo.Y+4, 16, 16))
	w.c.HitWithOptions(idChangeLists, combo, w.t(i18n.ChangelistTitle), ui.HitOptions{Role: ui.RoleCombobox, Expandable: true})
	w.tip(idChangeLists, w.t(i18n.ChangelistTitle))
	if w.c.Clicked(idChangeLists) {
		w.after(func() { w.openMenuBelow(combo, w.changeListsMenu(), 300) })
	}

	x := right(combo) + 10
	n := w.visibleChangeFileCount()
	w.p.text(w.pluralT(n, i18n.ChangelistFilesOne, i18n.ChangelistFilesMany), rect(x, row.Y, max(0, right(r)-x-8), row.Height), fontUI, regular, w.pal.TextSecond)

	if w.cm.listID == allChangeLists {
		return
	}
	hint := rect(r.X+10, row.Y+changeListBarH, r.Width-20, changeListHintH)
	w.p.iconIn(icons.Warning, rect(hint.X, hint.Y+3, 14, 14))
	w.p.text(w.t(i18n.ChangelistCommitHint), rect(hint.X+20, hint.Y, hint.Width-20, hint.Height), fontSmall, regular, w.pal.Warning)
}

// changeListsMenu is the Change Lists combo menu: pick a filter, create a
// list, or manage the selected one.
func (w *Window) changeListsMenu() []item {
	v := &w.cm
	items := []item{
		header(w.t(i18n.ChangelistTitle)),
		{
			title: w.t(i18n.ChangelistAllLists), icon: icons.Changes, checked: v.listID == allChangeLists,
			hint: w.pluralT(w.changeListCount(allChangeLists), i18n.ChangelistFilesOne, i18n.ChangelistFilesMany),
			run:  func() { w.setChangeList(allChangeLists) },
		},
	}
	for _, l := range w.changeLists() {
		l := l
		items = append(items, item{
			title: l.Name, icon: icons.Changelist, checked: v.listID == l.ID,
			hint: w.pluralT(w.changeListCount(l.ID), i18n.ChangelistFilesOne, i18n.ChangelistFilesMany),
			run:  func() { w.setChangeList(l.ID) },
		})
	}
	// The built-in list cannot be renamed or deleted, and neither can "All
	// lists", so those actions stay disabled for both.
	locked := v.listID == allChangeLists || v.listID == defaultChangeListID
	items = append(items,
		sep(),
		item{title: w.t(i18n.ChangelistNew), icon: icons.Add, run: w.openNewChangeList},
		item{title: w.t(i18n.ChangelistRename), icon: icons.Edit, disabled: locked, run: w.openRenameChangeList},
		item{title: w.t(i18n.ChangelistDelete), icon: icons.Delete, disabled: locked, run: w.openDeleteChangeList},
	)
	return items
}

// changeListMoveItems is the context submenu that reassigns the selected
// files to a list.
func (w *Window) changeListMoveItems(sel []changeRow) []item {
	paths := movableChangeListPaths(sel)
	if len(paths) == 0 {
		return nil
	}
	common := ""
	for i, p := range paths {
		id := w.ws.ChangeListFor(p)
		if i == 0 {
			common = id
		} else if common != id {
			common = ""
			break
		}
	}
	lists := w.changeLists()
	if len(lists) == 0 {
		lists = []vapp.ChangeList{{ID: defaultChangeListID, Name: w.t(i18n.ChangelistDefault)}}
	}
	items := make([]item, 0, len(lists))
	for _, l := range lists {
		l := l
		items = append(items, item{
			title: l.Name, icon: icons.Changelist, checked: common == l.ID,
			hint: w.pluralT(w.changeListCount(l.ID), i18n.ChangelistFilesOne, i18n.ChangelistFilesMany),
			run:  func() { w.moveToChangeList(paths, l.ID) },
		})
	}
	return items
}

// movableChangeListPaths is the selected files a list can own. Conflicts are
// not moved: they are always shown and resolve through staging.
func movableChangeListPaths(sel []changeRow) []string {
	seen := map[string]bool{}
	var paths []string
	for _, r := range sel {
		if r.area == domain.AreaConflicts || r.entry.Ignored() {
			continue
		}
		if seen[r.entry.Path] {
			continue
		}
		seen[r.entry.Path] = true
		paths = append(paths, r.entry.Path)
	}
	return paths
}

func (w *Window) moveToChangeList(paths []string, id string) {
	if len(paths) == 0 {
		return
	}
	if err := w.ws.MoveToChangeList(paths, id); err != nil {
		w.ws.NotifyError(w.t(i18n.ChangelistCannotMove), err)
		return
	}
	// With a filter active the moved files may disappear from the tree, so
	// drop the selection rather than leaving keys whose rows are hidden. In
	// the All-lists view every row stays, and the selection is worth keeping.
	if w.cm.listID != allChangeLists {
		w.clearChangeListSelection()
	}
	w.Invalidate()
}

// changeListNameProblem is a local validation message; the backend remains
// the authority and its error is shown if it still refuses the name.
func (w *Window) changeListNameProblem(name, exceptID string) string {
	if strings.TrimSpace(name) == "" {
		return w.t(i18n.ChangelistNameRequired)
	}
	for _, l := range w.changeLists() {
		if l.ID != exceptID && strings.EqualFold(l.Name, name) {
			return w.t(i18n.ChangelistNameExists, name)
		}
	}
	return ""
}

// openNewChangeList is IntelliJ's New Change List dialog.
func (w *Window) openNewChangeList() {
	ed := text.NewEditor("")
	problem, last := "", ""
	w.openDialog(&dialog{
		title: w.t(i18n.ChangelistNewTitle), ok: w.t(i18n.ChangelistCreate), width: 420,
		body: func(f *form) {
			n := strings.TrimSpace(ed.String())
			if n != last {
				last, problem = n, ""
			}
			if p := w.changeListNameProblem(n, ""); p != "" {
				problem = p
			}
			id := f.field(w.t(i18n.ChangelistName), ed, w.t(i18n.ChangelistNamePlaceholder), problem != "")
			f.w.dialog.focus = id
			if problem != "" {
				f.message(problem, w.pal.Error)
			}
		},
		onOK: func() bool {
			n := strings.TrimSpace(ed.String())
			if p := w.changeListNameProblem(n, ""); p != "" {
				problem = p
				return false
			}
			id, err := w.ws.CreateChangeList(n)
			if err != nil {
				problem = err.Error()
				return false
			}
			w.setChangeList(id)
			return true
		},
	})
}

// openRenameChangeList renames the selected list; its files stay assigned.
func (w *Window) openRenameChangeList() {
	id := w.cm.listID
	if id == allChangeLists || id == defaultChangeListID {
		return
	}
	cur, ok := w.changeListByID(id)
	if !ok {
		return
	}
	ed := text.NewEditor(cur.Name)
	problem, last := "", cur.Name
	w.openDialog(&dialog{
		title: w.t(i18n.ChangelistRenameTitle), ok: w.t(i18n.ChangelistRenameButton), width: 420,
		body: func(f *form) {
			n := strings.TrimSpace(ed.String())
			if n != last {
				last, problem = n, ""
			}
			if p := w.changeListNameProblem(n, id); p != "" {
				problem = p
			}
			fid := f.field(w.t(i18n.ChangelistNewName), ed, "", problem != "")
			f.w.dialog.focus = fid
			if problem != "" {
				f.message(problem, w.pal.Error)
			}
		},
		onOK: func() bool {
			n := strings.TrimSpace(ed.String())
			if p := w.changeListNameProblem(n, id); p != "" {
				problem = p
				return false
			}
			if n == cur.Name {
				return true
			}
			if err := w.ws.RenameChangeList(id, n); err != nil {
				problem = err.Error()
				return false
			}
			return true
		},
	})
}

// openDeleteChangeList deletes the selected list after confirmation; its
// files fall back to the default list.
func (w *Window) openDeleteChangeList() {
	id := w.cm.listID
	if id == allChangeLists || id == defaultChangeListID {
		return
	}
	cur, ok := w.changeListByID(id)
	if !ok {
		return
	}
	n := w.changeListCount(id)
	w.confirm(w.t(i18n.ChangelistDeleteTitle), w.t(i18n.ChangelistDeleteBody, cur.Name, w.pluralT(n, i18n.ChangelistFilesOne, i18n.ChangelistFilesMany)), w.t(i18n.ChangelistDeleteButton), func() {
		if err := w.ws.DeleteChangeList(id); err != nil {
			w.ws.NotifyError(w.t(i18n.ChangelistCannotDelete), err)
			return
		}
		if w.cm.listID == id {
			w.setChangeList(allChangeLists)
		}
		w.clearChangeListSelection()
		w.Invalidate()
	})
}
