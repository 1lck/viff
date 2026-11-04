package viffui

import (
	"context"
	"strings"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/ui/icons"
)

// The multi-commit detail panel is a list of the union of the selected
// commits' files, not a diff: every file row remembers which selected commits
// changed it, and opening a row opens the real diff of one of those commits
// against its own first parent.
var (
	idMultiCommitFiles  = tree.ID(fLog, "multi-commit-files")
	idMultiCommitScroll = tree.ID(fLog, "multi-commit-scroll")
	scMultiCommitRows   = tree.Sub(fLog, tree.Name("multi-commit-rows"))
)

// multiCommitView is the log detail panel's state while more than one commit
// is selected. key identifies the selection and the repository generation the
// request was made for: results for another key are stale and dropped. cancel
// stops the per-commit Git processes of the request in flight when the
// selection moves on.
type multiCommitView struct {
	key     string
	seq     uint64
	loading bool
	cancel  context.CancelFunc
	summary *vapp.MultiCommitSummary
	sel     int
	scroll  scroller
}

// begin stops the previous request and marks a new one, returning its sequence
// number.
func (v *multiCommitView) begin(key string) uint64 {
	v.cancelRequest()
	v.key = key
	v.seq++
	v.loading = true
	v.summary = nil
	v.sel = -1
	v.scroll = scroller{}
	return v.seq
}

// stop cancels a request in flight and clears the aggregate, for a selection
// that is no longer a multi-selection.
func (v *multiCommitView) stop() {
	v.cancelRequest()
	if v.key == "" && v.summary == nil && !v.loading {
		return
	}
	seq := v.seq + 1
	*v = multiCommitView{seq: seq}
}

// cancelRequest stops the background Git queries of the current request.
func (v *multiCommitView) cancelRequest() {
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
}

// apply installs a result only when it belongs to the current selection. A
// result that arrives after the selection or the repository changed is
// discarded instead of overwriting the newer view.
func (v *multiCommitView) apply(key string, seq uint64, summary vapp.MultiCommitSummary) bool {
	if v.key != key || v.seq != seq {
		return false
	}
	v.loading = false
	v.cancel = nil
	v.summary = &summary
	v.sel = -1
	return true
}

// multiCommitSelectionKey names the selection for one repository generation.
func multiCommitSelectionKey(rs *vapp.RepoState, commits []domain.Commit) string {
	var b strings.Builder
	for _, c := range commits {
		b.WriteString(c.Hash)
		b.WriteByte('|')
	}
	b.WriteString("#")
	b.WriteString(itoa(int(rs.Gen)))
	return b.String()
}

// paintMultiCommitDetails draws an aggregated changed-file list for the
// selected commits. Files are the union of each commit's changes against its
// first parent; the panel never shows the net difference between the oldest and
// the newest selection.
func (w *Window) paintMultiCommitDetails(r ui.Rect, commits []domain.Commit) {
	rs := w.repo()
	v := &w.lg
	key := multiCommitSelectionKey(rs, commits)
	if v.multi.key != key {
		seq := v.multi.begin(key)
		// Starting a background query does not block the paint; the callback
		// arrives through the workspace queue and invalidates the window.
		v.multi.cancel = w.ws.MultiCommitChanges(commits, func(sum vapp.MultiCommitSummary) {
			if w.repo() != rs {
				return
			}
			v.multi.apply(key, seq, sum)
		})
	}
	// Files on top, the selected commits below, as in the single-commit panel.
	msgH := clampF(r.Height*0.42, 110, 260)
	files := rect(r.X, r.Y, r.Width, r.Height-msgH)
	msg := rect(r.X, bottom(files), r.Width, msgH)
	w.p.hline(msg.X, msg.Y, msg.Width, w.pal.Border)
	w.paintMultiCommitFiles(files, &v.multi)
	w.paintMultiCommitSelection(msg, commits)
}

// paintMultiCommitFiles draws the deduplicated file rows.
func (w *Window) paintMultiCommitFiles(r ui.Rect, v *multiCommitView) {
	w.list(idMultiCommitFiles, scMultiCommitRows, r, "Changed files")
	if v.summary == nil {
		w.p.textCenter("Loading changed files…", rect(r.X, r.Y+r.Height*0.28, r.Width, 22), fontUI, regular, w.pal.TextSecond)
		return
	}
	files := v.summary.Files
	headerH := multiCommitListHeaderH(v)
	content := float32(len(files))*rowH + headerH + 8
	v.scroll.clamp(content, r.Height)
	w.onScrollIn(r, func(dx, dy float32) { v.scroll.Y += dy; v.scroll.clamp(content, r.Height) })
	w.onKeys(idMultiCommitFiles, func(e ui.KeyEvent) bool {
		switch e.Key {
		case ui.KeyDown:
			v.sel = min(len(files)-1, v.sel+1)
			w.revealMultiCommitRow(v, len(files))
		case ui.KeyUp:
			v.sel = max(0, v.sel-1)
			w.revealMultiCommitRow(v, len(files))
		case ui.KeyEnter:
			if v.sel >= 0 && v.sel < len(files) {
				w.openMultiCommitFile(v.sel)
			}
		default:
			return false
		}
		return true
	})
	w.c.Clip(r)
	y := r.Y + 4 - v.scroll.Y
	header := plural(len(files), "file", "files") + " changed by " + plural(len(v.summary.Commits), "commit", "commits")
	w.p.text(header, rect(r.X+12, y, r.Width-24, 24), fontSmall, regular, w.pal.TextSecond)
	y += 24
	// Show the failure reason above the rows. The selection panel also keeps
	// a fixed partial-results label visible while this list scrolls.
	if err := v.summary.Err; err != nil {
		msg := w.p.truncateMiddle("Partial results: "+multiCommitErrorLine(err), fontSmall, regular, max(0, r.Width-24))
		w.p.text(msg, rect(r.X+12, y, r.Width-24, 20), fontSmall, regular, w.pal.Error)
		y += 20
	}
	focused := w.focused(idMultiCommitFiles)
	for i, f := range files {
		rr := rect(r.X, y, r.Width, rowH)
		y += rowH
		if bottom(rr) < r.Y || rr.Y > bottom(r) {
			continue
		}
		id := scMultiCommitRows.At(i)
		hl := rect(rr.X+6, rr.Y, rr.Width-12, rr.Height)
		if v.sel == i {
			c := w.pal.SelectionInac
			if focused {
				c = w.pal.Selection
			}
			w.p.round(hl, 4, c)
		} else if w.hovered(id) {
			w.p.round(hl, 4, w.pal.Hover)
		}
		x := rr.X + 14
		lw := w.p.text(f.Kind.Letter(), rect(x, rr.Y, 16, rowH), fontUI, semibold, w.statusColor(f.Kind))
		x += max(lw, 14) + 6
		w.p.iconIn(fileIcon(f.Path), rect(x, rr.Y, 16, rowH))
		x += 22
		rightLimit := right(rr) - 12
		// A path changed by several selected commits stays one row; the count
		// says that its sources were merged instead of duplicated.
		if n := len(f.Sources); n > 1 {
			badge := plural(n, "commit", "commits")
			bw := w.p.width(badge, fontSmall, regular) + 12
			w.p.text(badge, rect(rightLimit-bw+8, rr.Y, bw-8, rowH), fontSmall, regular, w.pal.TextMuted)
			rightLimit -= bw
		}
		tw := w.p.text(domain.FileName(f.Path), rect(x, rr.Y, rightLimit-x-10, rowH), fontUI, regular, w.pal.Text)
		if hint := multiCommitFileHint(f); hint != "" {
			hx := x + tw + 8
			if hx < rightLimit-16 {
				w.p.text(w.p.truncateMiddle(hint, fontUI, regular, max(0, rightLimit-hx-12)), rect(hx, rr.Y, rightLimit-hx-10, rowH), fontUI, regular, w.pal.TextSecond)
			}
		}
		w.c.HitWithOptions(id, hl, f.Path, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: v.sel == i})
		if w.c.Clicked(id) {
			i := i
			w.after(func() { w.clickMultiCommitFile(i) })
		}
		ii := i
		w.onDouble(id, func() { w.clickMultiCommitFile(ii) })
		w.onContext(id, func(x, y float32) {
			v.sel = ii
			w.openMenu(x, y, w.multiCommitFileMenu(ii))
		})
	}
	if len(files) == 0 {
		w.p.textCenter("The selected commits change no files", rect(r.X, r.Y+r.Height*0.3, r.Width, 22), fontUI, regular, w.pal.TextSecond)
	}
	w.c.Unclip()
	w.vbar(idMultiCommitScroll, r, &v.scroll, content)
}

// multiCommitFileHint explains a deleted or renamed aggregated file next to its
// path. A plain modification shows only its directory.
func multiCommitFileHint(f vapp.MultiCommitFile) string {
	hint := domain.DirName(f.Path)
	if f.RenamedFrom != "" {
		hint = "← " + f.RenamedFrom
	}
	if f.Kind == domain.ChangeDeleted {
		if hint == "" {
			return "deleted"
		}
		return hint + " · deleted"
	}
	return hint
}

// paintMultiCommitSelection draws the selected commits below the file list, so
// the panel shows what the union was built from.
func (w *Window) paintMultiCommitSelection(r ui.Rect, commits []domain.Commit) {
	w.c.Clip(r)
	y := r.Y + 10
	caption, color := "Selected commits ("+itoa(len(commits))+")", w.pal.TextSecond
	if sum := w.lg.multi.summary; sum != nil && sum.Err != nil {
		caption, color = "Partial results · "+plural(len(commits), "commit", "commits"), w.pal.Error
	}
	w.p.text(caption, rect(r.X+14, y, r.Width-28, 18), fontSmall, regular, color)
	y += 20
	_, _, fs := w.metrics()
	lineH := float32(20)
	hintY := bottom(r) - 22
	for i, c := range commits {
		if y+lineH > hintY-2 {
			w.p.text("+"+itoa(len(commits)-i)+" more", rect(r.X+14, y, r.Width-28, lineH), fontSmall, regular, w.pal.TextSecond)
			break
		}
		w.c.Label(domain.ShortHash(c.Hash), rect(r.X+14, y, 76, lineH), ui.TextStyle{Size: fs - 1, Color: w.pal.Link, Mono: true, MonoFace: w.mono})
		w.p.text(c.Subject, rect(r.X+96, y, r.Width-112, lineH), fontUI, regular, w.pal.Text)
		y += lineH
	}
	w.p.text("Choose a file to view its changes", rect(r.X+14, hintY, r.Width-28, 18), fontSmall, regular, w.pal.TextMuted)
	w.c.Unclip()
}

// clickMultiCommitFile opens the real diff of a file's source commit. One
// source opens directly; several sources ask which commit to show, so a merged
// net difference is never invented.
func (w *Window) clickMultiCommitFile(i int) {
	v := &w.lg.multi
	if v.summary == nil || i < 0 || i >= len(v.summary.Files) {
		return
	}
	v.sel = i
	f := v.summary.Files[i]
	if len(f.Sources) == 1 {
		w.showMultiCommitSource(f.Sources[0], false)
		return
	}
	w.openMultiCommitSourceMenu(f, w.multiCommitRowRect(i))
}

// openMultiCommitFile handles the keyboard's Enter on a row.
func (w *Window) openMultiCommitFile(i int) { w.clickMultiCommitFile(i) }

// multiCommitListHeaderH is the room above the first file row: the count line
// and, when a commit could not be read, the partial-results line that must stay
// visible.
func multiCommitListHeaderH(v *multiCommitView) float32 {
	if v.summary != nil && v.summary.Err != nil {
		return 44
	}
	return 24
}

func (w *Window) revealMultiCommitRow(v *multiCommitView, n int) {
	area := w.areas[idMultiCommitFiles]
	if n == 0 {
		return
	}
	v.scroll.reveal(multiCommitListHeaderH(v)+float32(max(0, v.sel))*rowH, rowH, area.Height)
}

// multiCommitRowRect is the painted bounds of a file row, used to anchor the
// source-commit popup.
func (w *Window) multiCommitRowRect(i int) ui.Rect {
	area := w.areas[idMultiCommitFiles]
	headerH := multiCommitListHeaderH(&w.lg.multi)
	return rect(area.X, area.Y+4+headerH+float32(i)*rowH-w.lg.multi.scroll.Y, area.Width, rowH)
}

// multiCommitErrorLine is the first line of a query failure, short enough to
// sit in the list header.
func multiCommitErrorLine(err error) string {
	msg := vapp.ErrorText(err)
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return msg
}

// openMultiCommitSourceMenu lists the selected commits that changed one file.
// Each item opens that commit's own first-parent diff.
func (w *Window) openMultiCommitSourceMenu(f vapp.MultiCommitFile, anchor ui.Rect) {
	items := make([]item, 0, len(f.Sources))
	for _, src := range f.Sources {
		src := src
		items = append(items, item{
			title: domain.ShortHash(src.Commit.Hash) + "  " + src.Commit.Subject,
			hint:  multiCommitSourceLabel(src.Change),
			icon:  icons.Diff,
			run:   func() { w.showMultiCommitSource(src, true) },
		})
	}
	w.openMenuBelow(anchor, items, 460)
}

// showMultiCommitSource opens a source commit's real change of one file. The
// diff viewer compares that commit with its first parent; files are never
// stitched into a synthetic combined patch.
func (w *Window) showMultiCommitSource(src vapp.MultiCommitSource, pin bool) {
	w.showDiff(vapp.DiffSource{
		Path:    src.Change.Path,
		OldPath: src.Change.OldPath,
		Commit:  src.Commit.Hash,
		Parent:  src.Parent,
		Kind:    src.Change.Kind,
	}, pin)
}

// multiCommitFileMenu is the context menu of an aggregated file row.
func (w *Window) multiCommitFileMenu(i int) []item {
	v := &w.lg.multi
	if v.summary == nil || i < 0 || i >= len(v.summary.Files) {
		return nil
	}
	f := v.summary.Files[i]
	items := make([]item, 0, len(f.Sources)+3)
	for _, src := range f.Sources {
		src := src
		items = append(items, item{
			title: "Show Diff from " + domain.ShortHash(src.Commit.Hash),
			hint:  multiCommitSourceLabel(src.Change),
			icon:  icons.Diff,
			run:   func() { w.showMultiCommitSource(src, true) },
		})
	}
	if len(items) > 0 {
		items = append(items, sep())
	}
	items = append(items,
		item{title: "Show History", icon: icons.History, run: func() { w.showPathHistory(f.Path) }},
		item{title: "Copy Relative Path", icon: icons.Copy, run: func() { w.copyText(f.Path) }},
	)
	return items
}

// multiCommitSourceLabel names one commit's change to a file in the source
// picker, so a deletion or rename is attributed to the commit that made it.
func multiCommitSourceLabel(ch domain.Change) string {
	switch ch.Kind {
	case domain.ChangeAdded:
		return "added"
	case domain.ChangeDeleted:
		return "deleted"
	case domain.ChangeRenamed:
		return "renamed from " + ch.OldPath
	case domain.ChangeCopied:
		return "copied from " + ch.OldPath
	case domain.ChangeTypeChanged:
		return "type changed"
	case domain.ChangeConflicted:
		return "conflicted"
	default:
		return "modified"
	}
}
