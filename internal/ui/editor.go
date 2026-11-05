package viffui

import (
	"path"
	"strings"
	"time"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/platform"
	"viff/internal/ui/icons"
)

// editorView is the editor island: tabs of diffs.
type editorView struct {
	tabs   []*diffTab
	active int
	seq    uint64
}

// diffTab is one open diff.
type diffTab struct {
	src      vapp.DiffSource
	preview  bool
	res      *vapp.DiffResult
	err      error
	loading  bool
	seq      uint64
	statusAt time.Time

	y, yOld float32 // vertical scroll in pixels (new side; old side when unsynced)
	x       float32
	cur     int
	selOld  map[int]bool
	selNew  map[int]bool
	anchor  int

	oldDisp, newDisp []string
	oldSyn, newSyn   [][]span
	oldBlk, newBlk   []int32
	unified          []uline
	maxCells         int

	searchOpen bool
	search     *text.Editor
	hits       []searchHit
	hitIdx     int
	hitQuery   string

	conflictBlock int
	busy          bool

	// File view annotations (Git blame).
	annotate  bool
	blame     *domain.Blame
	blameErr  error
	blameSeq  uint64
	blameRank []int
	blameN    int
}

// uline is one row of the unified view.
type uline struct {
	old, new int // line indexes, -1 when absent
	block    int // -1 for unchanged
}

type searchHit struct {
	line       int
	start, end int // byte offsets in the display line
}

func (v *editorView) reset() {
	v.tabs = nil
	v.active = 0
}

func (v *editorView) cur() *diffTab {
	if v.active >= 0 && v.active < len(v.tabs) {
		return v.tabs[v.active]
	}
	return nil
}

func (t *diffTab) title() string {
	name := domain.FileName(t.src.Path)
	if t.src.View && t.src.Commit != "" {
		name += " (" + domain.ShortHash(t.src.Commit) + ")"
	}
	return name
}

// showDiff opens a source in the preview tab, or in a pinned tab.
func (w *Window) showDiff(src vapp.DiffSource, pin bool) {
	v := &w.ed
	key := src.Key()
	for i, t := range v.tabs {
		if t.src.Key() == key {
			v.active = i
			if pin {
				t.preview = false
			}
			if t.res == nil && !t.loading {
				w.loadTab(t)
			}
			return
		}
	}
	t := &diffTab{src: src, preview: !pin, selOld: map[int]bool{}, selNew: map[int]bool{}, search: text.NewEditor(""), anchor: -1}
	for i, o := range v.tabs {
		if o.preview {
			v.tabs[i] = t
			v.active = i
			w.loadTab(t)
			return
		}
	}
	v.tabs = append(v.tabs, t)
	v.active = len(v.tabs) - 1
	w.loadTab(t)
}

func (w *Window) closeTab(i int) {
	v := &w.ed
	if i < 0 || i >= len(v.tabs) {
		return
	}
	v.tabs = append(v.tabs[:i], v.tabs[i+1:]...)
	if v.active >= len(v.tabs) {
		v.active = len(v.tabs) - 1
	} else if v.active > i {
		v.active--
	}
	if v.active < 0 {
		v.active = 0
	}
}

func (w *Window) whitespace() domain.WhitespaceMode {
	return domain.WhitespaceMode(w.ws.Settings.DiffWhitespace)
}

// loadTab (re)loads a tab's diff, keeping its scroll position.
func (w *Window) loadTab(t *diffTab) {
	w.ed.seq++
	seq := w.ed.seq
	t.seq = seq
	t.loading = true
	if rs := w.repo(); rs != nil {
		t.statusAt = rs.StatusTime
	}
	w.ws.LoadDiff(t.src, w.whitespace(), func(res *vapp.DiffResult, err error) {
		if t.seq != seq {
			return
		}
		t.loading = false
		t.err = err
		if err != nil {
			return
		}
		first := t.res == nil
		t.res = res
		w.prepareTab(t)
		if first {
			t.cur = -1
			if len(res.Blocks) > 0 {
				t.cur = 0
				w.scrollToBlock(t, 0)
			}
		} else if t.cur >= len(res.Blocks) {
			t.cur = len(res.Blocks) - 1
		}
		clear(t.selOld)
		clear(t.selNew)
		t.hitQuery = "\x00"
	})
}

// prepareTab derives the display lines, syntax and block maps.
func (w *Window) prepareTab(t *diffTab) {
	res := t.res
	oldLines, newLines := res.Old.Lines, res.New.Lines
	if res.Conflict != nil {
		oldLines = res.Conflict.Current.Lines
		newLines = nil
	}
	t.oldDisp = expandAll(oldLines)
	t.newDisp = expandAll(newLines)
	t.oldSyn = highlight(t.src.Path, t.oldDisp)
	t.newSyn = highlight(t.src.Path, t.newDisp)
	t.maxCells = 0
	for _, l := range t.oldDisp {
		t.maxCells = max(t.maxCells, displayCells(l))
	}
	for _, l := range t.newDisp {
		t.maxCells = max(t.maxCells, displayCells(l))
	}
	t.oldBlk = blockMap(len(t.oldDisp), res.Blocks, true)
	t.newBlk = blockMap(len(t.newDisp), res.Blocks, false)
	t.unified = t.unified[:0]
	o, n := 0, 0
	for bi, b := range res.Blocks {
		for o < min(b.OldStart, len(oldLines)) && n < min(b.NewStart, len(newLines)) {
			t.unified = append(t.unified, uline{old: o, new: n, block: -1})
			o++
			n++
		}
		for k := b.OldStart; k < min(b.OldEnd, len(oldLines)); k++ {
			t.unified = append(t.unified, uline{old: k, new: -1, block: bi})
		}
		for k := b.NewStart; k < min(b.NewEnd, len(newLines)); k++ {
			t.unified = append(t.unified, uline{old: -1, new: k, block: bi})
		}
		o, n = max(o, b.OldEnd), max(n, b.NewEnd)
	}
	for o < len(oldLines) && n < len(newLines) {
		t.unified = append(t.unified, uline{old: o, new: n, block: -1})
		o++
		n++
	}
	for ; n < len(newLines); n++ {
		t.unified = append(t.unified, uline{old: -1, new: n, block: -1})
	}
	for ; o < len(oldLines); o++ {
		t.unified = append(t.unified, uline{old: o, new: -1, block: -1})
	}
}

func expandAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = expandLine(l)
	}
	return out
}

func blockMap(n int, blocks []domain.Block, old bool) []int32 {
	m := make([]int32, n)
	for i := range m {
		m[i] = -1
	}
	for bi, b := range blocks {
		s, e := b.NewStart, b.NewEnd
		if old {
			s, e = b.OldStart, b.OldEnd
		}
		for k := s; k < min(e, n); k++ {
			m[k] = int32(bi)
		}
	}
	return m
}

// refreshTabs reloads working-tree diffs after the status changed.
func (w *Window) refreshTabs() {
	rs := w.repo()
	if rs == nil {
		return
	}
	v := &w.ed
	for i := len(v.tabs) - 1; i >= 0; i-- {
		t := v.tabs[i]
		live := t.src.Working || t.src.View && t.src.Commit == ""
		if !live || t.loading || !t.statusAt.Before(rs.StatusTime) {
			continue
		}
		if t.src.View {
			w.loadTab(t)
			if t.annotate {
				w.loadBlame(t)
			}
			continue
		}
		if t.preview && !inArea(rs.Entries(t.src.Area), t.src.Path) {
			// The file left this area (staged, committed or rolled back):
			// a preview follows the selection, so it closes like IntelliJ's.
			w.after(func() { w.closeTabOf(t) })
			continue
		}
		w.loadTab(t)
	}
}

// ---- painting -------------------------------------------------------------------

func (w *Window) paintEditor(r ui.Rect) {
	w.island(r)
	w.refreshTabs()
	v := &w.ed
	if len(v.tabs) == 0 {
		w.paintEditorEmpty(r)
		return
	}
	tabs := splitTop(&r, tabsH)
	w.paintTabs(tabs)
	w.p.hline(r.X, r.Y, r.Width, w.pal.Border)
	r.Y++
	r.Height--
	t := v.cur()
	if t == nil {
		return
	}
	switch {
	case t.err != nil:
		w.paintCentered(r, "Cannot show diff", vapp.ErrorText(t.err), w.pal.Error)
	case t.res == nil:
		if t.loading {
			w.paintCentered(r, "Loading…", "", w.pal.TextSecond)
		}
	case t.res.Conflict != nil:
		w.paintConflict(t, r)
	case t.src.View:
		w.paintFileView(t, r)
	default:
		w.paintDiff(t, r)
	}
}

func (w *Window) paintCentered(r ui.Rect, title, body string, c ui.Color) {
	y := r.Y + r.Height*0.4
	w.p.textCenter(title, rect(r.X, y, r.Width, 22), fontUI, semibold, c)
	if body != "" {
		lines := w.c.WrapLines(nil, body, fontUI, regular, min(r.Width-40, 560), 6)
		for i, l := range lines {
			w.p.textCenter(l, rect(r.X, y+26+float32(i)*20, r.Width, 20), fontUI, regular, w.pal.TextSecond)
		}
	}
}

func (w *Window) paintTabs(r ui.Rect) {
	v := &w.ed
	x := r.X + 8
	for i, t := range v.tabs {
		id := scTabs.At(i)
		cid := scTabClose.At(i)
		name := t.title()
		tw := w.p.width(name, fontUI, regular)
		width := 8 + 16 + 6 + tw + 6 + 18 + 6
		tr := rect(x, r.Y+(r.Height-30)/2, width, 30)
		if right(tr) > right(r)-8 {
			break
		}
		active := i == v.active
		switch {
		case active:
			w.p.outline(tr, 6, 1, w.pal.TabSelectedBorder, w.pal.TabSelected)
		case w.hovered(id) || w.hovered(cid):
			w.p.round(tr, 6, w.pal.ToolHover)
		}
		w.p.iconIn(fileIcon(t.src.Path), rect(tr.X+8, tr.Y, 16, tr.Height))
		fg := w.statusColor(t.src.Kind)
		if !t.src.Working {
			fg = w.pal.Text
		}
		w.p.text(name, rect(tr.X+30, tr.Y, tw+2, tr.Height), fontUI, regular, fg)
		if t.preview {
			// IntelliJ shows the preview tab in italics; Viff underlines the
			// name faintly instead (the UI font has no italic face here).
			w.p.hline(tr.X+30, tr.Y+tr.Height-8, tw, w.pal.TextDisable)
		}
		w.c.HitWithOptions(id, tr, t.src.Path, ui.HitOptions{Role: ui.RoleTab, Selected: active, SkipFocus: true})
		w.tip(id, w.tabTip(t))
		cr := rect(right(tr)-24, tr.Y+(tr.Height-18)/2, 18, 18)
		if active || w.hovered(id) || w.hovered(cid) {
			if w.hovered(cid) {
				w.p.round(cr, 4, w.pal.ToolHover)
			}
			w.p.iconIn(icons.CloseSmall, cr)
		}
		w.c.HitWithOptions(cid, cr, "Close", ui.HitOptions{Role: ui.RoleButton, SkipFocus: true})
		if w.c.Clicked(cid) {
			i := i
			w.after(func() { w.closeTab(i) })
		} else if w.c.Clicked(id) {
			i := i
			w.after(func() { v.active = i })
		}
		w.onDouble(id, func() { t.preview = false })
		w.onContext(id, func(x, y float32) {
			i := i
			w.openMenu(x, y, []item{
				{title: "Close", shortcut: "⌘W", run: func() { w.closeTab(i) }},
				{title: "Close Other Tabs", run: func() {
					v.tabs = []*diffTab{t}
					v.active = 0
				}},
				{title: "Close All Tabs", run: func() { v.reset() }},
				sep(),
				{title: "Keep Tab Open", disabled: !t.preview, run: func() { t.preview = false }},
				{title: "Copy Path", icon: icons.Copy, run: func() { w.copyText(path.Join(w.repo().Repo.Root, t.src.Path)) }},
			})
		})
		x = right(tr) + 4
	}
}

func (w *Window) tabTip(t *diffTab) string {
	s := t.src.Path
	if t.src.Working {
		return s + " (" + strings.ToLower(areaTitle(t.src.Area)) + ")"
	}
	return s + " @ " + domain.ShortHash(t.src.Commit)
}

// paintEditorEmpty shows the editor's empty-state hints like IntelliJ.
func (w *Window) paintEditorEmpty(r ui.Rect) {
	hints := [][2]string{
		{"Commit", "⌘K"},
		{"Push", "⇧⌘K"},
		{"Update Project", "⌘T"},
		{"Branches", "⌥⇧`"},
		{"Show Diff of Selection", "⌘D"},
	}
	rs := w.repo()
	if len(rs.Remotes) == 0 {
		hints = [][2]string{{"Commit", "⌘K"}, {"Branches", "⌥⇧`"}, {"Show Diff of Selection", "⌘D"}}
	}
	lineH := float32(30)
	total := float32(len(hints)) * lineH
	y := r.Y + (r.Height-total)/2 - 20
	cx := r.X + r.Width/2
	for _, h := range hints {
		lw := w.p.width(h[0], fontUI, regular)
		w.p.text(h[0], rect(cx-lw-6, y, lw+2, lineH), fontUI, regular, w.pal.TextSecond)
		w.p.text(kbdHint(h[1]), rect(cx+6, y, 200, lineH), fontUI, regular, w.pal.Link)
		y += lineH
	}
	w.p.textCenter("Select a file in the Commit tool window or the Git log to see its changes", rect(r.X, y+14, r.Width, 22), fontSmall, regular, w.pal.TextDisable)
}

// ---- file icons ----------------------------------------------------------------

func fileIcon(p string) icons.Pair {
	base := strings.ToLower(path.Base(p))
	switch base {
	case ".gitignore", ".gitattributes", ".gitmodules":
		return icons.Gitignore
	case "dockerfile", "docker-compose.yml", "docker-compose.yaml", ".dockerignore":
		return icons.Docker
	case ".editorconfig":
		return icons.EditorConfig
	case "build.gradle", "settings.gradle", "build.gradle.kts", "settings.gradle.kts":
		return icons.Gradle
	case "manifest.mf":
		return icons.Manifest
	}
	switch strings.ToLower(path.Ext(p)) {
	case ".java":
		return icons.Java
	case ".js", ".mjs", ".cjs", ".jsx":
		return icons.JavaScript
	case ".ts", ".tsx":
		return icons.TypeScript
	case ".json", ".jsonc", ".json5":
		return icons.Json
	case ".md", ".markdown":
		return icons.Markdown
	case ".xml", ".plist", ".iml":
		return icons.Xml
	case ".yml", ".yaml":
		return icons.Yaml
	case ".toml":
		return icons.Toml
	case ".html", ".htm":
		return icons.Html
	case ".css", ".scss", ".less":
		return icons.Css
	case ".sql":
		return icons.Sql
	case ".sh", ".bash", ".zsh", ".fish":
		return icons.Shell
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".bmp":
		return icons.Image
	case ".gradle", ".kts":
		return icons.Gradle
	case ".groovy":
		return icons.Groovy
	case ".c", ".h":
		return icons.C
	case ".cpp", ".cc", ".cxx", ".hpp":
		return icons.Cpp
	case ".cs":
		return icons.Csharp
	case ".swift":
		return icons.SwiftLang
	case ".vue":
		return icons.Vue
	case ".graphql", ".gql":
		return icons.Graphql
	case ".csv", ".tsv":
		return icons.Csv
	case ".pl", ".pm":
		return icons.Perl
	case ".properties", ".ini", ".cfg", ".conf", ".env":
		return icons.Properties
	case ".patch", ".diff":
		return icons.Patch
	case ".zip", ".jar", ".tar", ".gz", ".tgz", ".7z", ".rar":
		return icons.Archive
	case ".txt", ".log", ".rst":
		return icons.Text
	case ".bin", ".exe", ".dll", ".so", ".dylib", ".class", ".o", ".a", ".pdf":
		return icons.BinaryData
	}
	return icons.Text
}

// openExternal opens a working-tree file in the configured editor.
func (w *Window) openExternal(rel string, line int) {
	rs := w.repo()
	if rs == nil {
		return
	}
	full := path.Join(rs.Repo.Root, rel)
	if err := platform.OpenWith(w.ws.Settings.ExternalEditor, full, line); err != nil {
		w.ws.NotifyError("Cannot open "+domain.FileName(rel), err)
	}
}

func inArea(entries []domain.StatusEntry, p string) bool {
	for _, e := range entries {
		if e.Path == p {
			return true
		}
	}
	return false
}

func (w *Window) closeTabOf(t *diffTab) {
	for i, o := range w.ed.tabs {
		if o == t {
			w.closeTab(i)
			return
		}
	}
}
