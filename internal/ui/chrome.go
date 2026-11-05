package viffui

import (
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/i18n"
	"viff/internal/platform"
	"viff/internal/ui/icons"
)

// paintMain lays out the IDE frame: toolbar, stripes, islands, status bar.
func (w *Window) paintMain(full ui.Rect) {
	area := full
	bar := splitTop(&area, toolbarH)
	status := splitBottom(&area, statusH)
	left := splitLeft(&area, stripeW)
	rightStripe := splitRight(&area, stripeW)
	w.paintToolbar(bar)
	w.paintStatus(status)
	w.paintStripes(left, rightStripe)

	s := w.ws.Settings
	if w.notifyOpen {
		nr := splitRight(&area, min(360, area.Width/3))
		nr.X += islandGap
		nr.Width -= islandGap
		w.paintNotificationsPanel(nr)
	}
	// The bottom dock holds one tool window at a time, like IntelliJ's bottom
	// stripe: the terminal replaces the Git window while it is shown, and
	// closing it brings Git back if that was open.
	switch {
	case w.termOpen:
		h := clampF(s.TerminalHeight, terminalMinHeight, max(terminalMinHeight, area.Height-160))
		tr := splitBottom(&area, h)
		area.Height -= islandGap
		w.paintTerminal(tr)
		start := s.TerminalHeight
		w.splitter(idSplitTerminal, rect(tr.X, tr.Y-islandGap, tr.Width, islandGap), false,
			func() { start = h },
			func(d float32) {
				s.TerminalHeight = clampF(start-d, terminalMinHeight, max(terminalMinHeight, full.Height-toolbarH-statusH-160))
			},
			w.saveSettings)
	case w.gitOpen:
		h := clampF(s.GitPanelHeight, 120, max(120, area.Height-160))
		gr := splitBottom(&area, h)
		area.Height -= islandGap
		w.paintGitWindow(gr)
		start := s.GitPanelHeight
		w.splitter(idSplitGit, rect(gr.X, gr.Y-islandGap, gr.Width, islandGap), false,
			func() { start = h },
			func(d float32) { s.GitPanelHeight = clampF(start-d, 120, max(120, full.Height-toolbarH-statusH-160)) },
			w.saveSettings)
	}
	// The left dock holds one tool window at a time, like IntelliJ's left
	// stripe: the Project tree replaces the Commit panel instead of squeezing
	// the editor with two fixed panels.
	if w.projectOpen {
		pw := clampF(s.ProjectPanelWidth, 240, max(240, area.Width-320))
		pr := splitLeft(&area, pw)
		area.X += islandGap
		area.Width -= islandGap
		w.paintProject(pr)
		start := pw
		w.splitter(idSplitProject, rect(right(pr), pr.Y, islandGap, pr.Height), true,
			func() { start = pw },
			func(d float32) { s.ProjectPanelWidth = clampF(start+d, 240, max(240, full.Width-2*stripeW-300)) },
			w.saveSettings)
	} else if w.commitOpen {
		cw := clampF(s.CommitPanelWidth, 240, max(240, area.Width-300))
		cr := splitLeft(&area, cw)
		area.X += islandGap
		area.Width -= islandGap
		w.paintCommitWindow(cr)
		start := cw
		w.splitter(idSplitCommit, rect(right(cr), cr.Y, islandGap, cr.Height), true,
			func() { start = cw },
			func(d float32) { s.CommitPanelWidth = clampF(start+d, 240, max(240, full.Width-2*stripeW-300)) },
			w.saveSettings)
	}
	w.paintEditor(area)
	w.applyPendingRepositoryFile()
	if w.pendingRebase != "" && !w.ws.Busy() {
		onto := w.pendingRebase
		w.after(func() {
			w.pendingRebase = ""
			if w.repo().CurrentBranch() != onto {
				w.ws.Rebase(onto)
			}
		})
	}
}

// toggleGitWindow shows or hides the Git tool window. While the terminal has
// the bottom dock, the first press brings Git back rather than hiding both.
func (w *Window) toggleGitWindow() {
	if w.termOpen {
		w.termOpen = false
		w.gitOpen = true
		w.Invalidate()
		return
	}
	w.gitOpen = !w.gitOpen
	w.Invalidate()
}

// island paints a rounded tool-window surface and returns it.
func (w *Window) island(r ui.Rect) ui.Rect {
	w.p.round(r, islandRadius, w.pal.Island)
	return r
}

// ---- main toolbar -----------------------------------------------------------

func (w *Window) paintToolbar(r ui.Rect) {
	w.c.MoveRegion(r)
	lead, trail := app.TitlebarInsets()
	x := r.X + lead + 8
	if lead == 0 {
		x = r.X + 10
	}
	rs := w.repo()
	cy := r.Y + (r.Height-28)/2

	// Project widget.
	name := rs.Repo.Name()
	pw := 20 + 8 + w.p.width(name, fontUI, semibold) + 26
	pr := rect(x, cy, pw+8, 28)
	w.toolbarHover(idProjectWidget, pr)
	w.projectBadge(rect(x+6, cy+4, 20, 20), name, 5)
	w.p.text(name, rect(x+34, cy, pw, 28), fontUI, semibold, w.pal.Text)
	w.p.iconIn(icons.ChevronDown, rect(right(pr)-22, cy, 16, 28))
	w.c.HitWithOptions(idProjectWidget, pr, w.t(i18n.ChromeA11yProject, name), ui.HitOptions{Role: ui.RoleButton, SkipFocus: true})
	w.tip(idProjectWidget, rs.Repo.Root)
	if w.clicked(idProjectWidget, nil) {
		w.after(func() { w.openMenuBelow(pr, w.projectMenu(), 280) })
	}
	x = right(pr) + 4

	// Branch widget.
	branch := rs.HeadLabel()
	bw := 16 + 6 + w.p.width(branch, fontUI, regular) + 24
	if rs.Operation.Active() {
		bw += w.p.width(rs.Operation.Kind.Title(), fontUI, regular) + 8
	}
	br := rect(x, cy, bw+12, 28)
	w.toolbarHover(idBranchWidget, br)
	w.p.iconIn(icons.Vcs, rect(x+4, cy, 18, 28))
	tx := x + 26
	if rs.Operation.Active() {
		t := rs.Operation.Kind.Title()
		tw := w.p.text(t, rect(tx, cy, 200, 28), fontUI, regular, w.pal.Warning)
		tx += tw + 8
	}
	w.p.text(branch, rect(tx, cy, bw, 28), fontUI, regular, w.pal.Text)
	w.p.iconIn(icons.ChevronDown, rect(right(br)-22, cy, 16, 28))
	w.c.HitWithOptions(idBranchWidget, br, w.t(i18n.ChromeA11yBranch, branch), ui.HitOptions{Role: ui.RoleButton, SkipFocus: true})
	w.tip(idBranchWidget, w.branchTip())
	if w.clicked(idBranchWidget, nil) {
		w.after(func() { w.openBranchPopup(br) })
	}

	// Right-hand actions.
	ax := right(r) - trail - 10 - actionSize
	ay := r.Y + (r.Height-actionSize)/2
	if w.action(idTbSettings, ax, ay, icons.Settings, w.t(i18n.ChromeTbSettings), actOpts{}) {
		ar := rect(ax-200+actionSize, ay, 200, actionSize)
		w.after(func() { w.openMenuBelow(ar, w.settingsMenu(), 220) })
	}
	ax -= actionSize + 4
	if w.action(idTbHelp, ax, ay, icons.Help, w.t(i18n.ChromeTbHelp), actOpts{}) {
		ar := rect(ax-260+actionSize, ay, 260, actionSize)
		w.after(func() { w.openMenuBelow(ar, w.helpMenu(), 260) })
	}
	ax -= actionSize + 4
	if w.action(idTbSearch, ax, ay, icons.Search, w.t(i18n.ChromeTbSearch)+"\t⌘F", actOpts{}) {
		w.after(w.focusLogSearch)
	}
	ax -= actionSize + 14
	w.p.vline(ax+actionSize+7, ay+5, actionSize-10, w.pal.Separator)
	busy := w.ws.Busy()
	// A backend whose commit is itself the server operation has nothing to
	// publish afterwards, so Push and Fetch are not drawn at all instead of
	// being shown and doing nothing.
	separatePush := w.ws.Caps.SeparatePush
	if separatePush {
		if w.action(idTbPush, ax, ay, icons.Push, w.t(i18n.ChromeTbPush)+"\t⇧⌘K", actOpts{disabled: !rs.HasHead()}) {
			w.after(w.openPush)
		}
		ax -= actionSize + 4
	}
	if w.action(idTbCommit, ax, ay, icons.Commit, w.t(i18n.ChromeTbCommit)+"\t⌘K", actOpts{}) {
		w.after(w.focusCommit)
	}
	ax -= actionSize + 4
	if w.action(idTbUpdate, ax, ay, icons.Update, w.t(i18n.ChromeTbUpdate)+"\t⌘T", actOpts{disabled: len(rs.Remotes) == 0}) {
		w.after(w.openUpdate)
	}
	if separatePush {
		ax -= actionSize + 4
		if w.action(idTbFetch, ax, ay, icons.Fetch, w.t(i18n.ChromeTbFetch), actOpts{disabled: len(rs.Remotes) == 0 || busy && w.taskRunning("Fetch")}) {
			w.after(func() { w.ws.Fetch(false) })
		}
	}
}

func (w *Window) toolbarHover(id ui.ID, r ui.Rect) {
	if w.pressed(id) {
		w.p.round(r, 6, w.pal.ToolPressed)
	} else if w.hovered(id) {
		w.p.round(r, 6, w.pal.ToolHover)
	}
}

var badgeColors = [][2]uint32{
	{0xE36D74, 0xC8495A}, {0xE5A54B, 0xD27E2F}, {0x5FAD65, 0x3F8F4C}, {0x3D7DEB, 0x2A5FC9},
	{0xC77DBB, 0xA35AA0}, {0x2AACB8, 0x1A8A99}, {0x9C7BDB, 0x7A56C2}, {0xD5B778, 0xB8964F},
}

// projectBadge paints the IntelliJ project icon: initials on a gradient.
func (w *Window) projectBadge(r ui.Rect, name string, radius float32) {
	h := fnv.New32a()
	h.Write([]byte(name))
	cs := badgeColors[h.Sum32()%uint32(len(badgeColors))]
	w.c.Rounded(r, radius, ui.RGB24(cs[0]))
	w.p.textCenter(initials(name), r, r.Height*0.5, semibold, ui.RGB24(0xFFFFFF))
}

func (w *Window) branchTip() string {
	rs := w.repo()
	b := rs.Status.Branch
	if b.Detached || b.Branch == "" {
		return w.t(i18n.ChromeTipDetached, rs.HeadLabel())
	}
	s := w.t(i18n.ChromeTipBranch, b.Branch)
	if b.Upstream != "" {
		s += "\n" + w.t(i18n.ChromeTipTracking, b.Upstream)
		if b.Ahead > 0 || b.Behind > 0 {
			s += w.t(i18n.ChromeTipAheadBehind, itoa(b.Ahead), itoa(b.Behind))
		}
	}
	return s
}

func (w *Window) taskRunning(prefix string) bool {
	for _, t := range w.ws.Tasks {
		if strings.HasPrefix(t.Title, prefix) {
			return true
		}
	}
	return false
}

// projectMenu lists recent repositories and the project actions.
func (w *Window) projectMenu() []item {
	var out []item
	out = append(out, item{title: w.t(i18n.ChromeMenuOpen), icon: icons.Open, shortcut: "⌘O", run: w.chooseRepoNow},
		item{title: w.t(i18n.ChromeMenuClone), icon: icons.Download, run: w.openClone},
		item{title: w.t(i18n.ChromeMenuNewRepository), icon: icons.Add, run: w.openInit},
		item{title: w.t(i18n.ChromeMenuRepositories), icon: icons.Project, run: w.openRepositories})
	cur := ""
	if w.ws.Repo != nil {
		cur = w.ws.Repo.Repo.Root
	}
	recent := w.ws.Settings.Recent
	if len(recent) > 0 {
		out = append(out, sep(), header(w.t(i18n.ChromeMenuRecentProjects)))
		for i, r := range recent {
			if i >= 12 {
				break
			}
			path := r.Path
			name := filepath.Base(path)
			it := item{title: name, hint: shortenHome(filepath.Dir(path)), run: func() { w.openRepo(path) }, filter: path}
			if path == cur {
				it.checked = true
			}
			out = append(out, it)
		}
	}
	if w.ws.Repo != nil {
		// The newest recorded operation, named as the user performed it. The
		// same action is on Command-Z; the menu is the discoverable copy and
		// stays disabled while another command is changing the history.
		undo := item{title: w.t(i18n.ChromeMenuUndo), icon: icons.Undo, shortcut: "⌘Z", disabled: true}
		if name := w.ws.UndoName(); name != "" {
			undo.title = w.t(i18n.ChromeMenuUndoOperation, name)
			undo.disabled = w.ws.Busy()
			undo.run = w.ws.Undo
		}
		out = append(out, sep(), undo,
			item{title: w.t(i18n.ChromeMenuOpenFile), icon: icons.Search, shortcut: "⇧⌘O", run: w.openQuickOpen},
			item{title: w.t(i18n.ChromeMenuShowInFinder), icon: icons.Folder, run: func() { revealPath(cur) }},
			item{title: w.t(i18n.ChromeMenuOpenTerminal), icon: icons.Shell, run: w.openTerminal},
			item{title: w.t(i18n.ChromeMenuOpenSystemTerm), icon: icons.Shell, run: func() { openTerminal(cur) }},
			sep(),
			item{title: w.t(i18n.ChromeMenuCloseProject), run: w.closeProject})
	}
	return out
}

// toggleProject shows the Project tree in the left dock. The dock holds one
// tool window at a time, so opening it hides the Commit panel; the stripe
// icons switch back and forth.
func (w *Window) toggleProject() {
	w.projectOpen = !w.projectOpen
	if w.projectOpen {
		w.commitOpen = false
	}
}

// toggleCommit shows the Commit panel in the left dock.
func (w *Window) toggleCommit() {
	w.commitOpen = !w.commitOpen
	if w.commitOpen {
		w.projectOpen = false
	}
}

// chooseRepoNow is chooseRepo outside a paint (menu actions run after it).
func (w *Window) chooseRepoNow() {
	dir, err := app.OpenDirectory(app.OpenDirectoryOptions{Title: w.t(i18n.ChromeDialogOpenRepository)})
	if err != nil || dir == "" {
		return
	}
	w.openRepo(dir)
}

// ---- stripes ------------------------------------------------------------------

func (w *Window) paintStripes(left, rightS ui.Rect) {
	y := left.Y + 6
	if w.stripeButton(idStripeProject, rect(left.X+5, y, 30, 30), icons.Project, w.t(i18n.ChromeStripeProject)+"\t⌘1", w.projectOpen) {
		w.after(w.toggleProject)
	}
	y += 36
	if w.stripeButton(idStripeCommit, rect(left.X+5, y, 30, 30), icons.Commit, w.t(i18n.ChromeStripeCommit)+"\t⌘0", w.commitOpen) {
		w.after(w.toggleCommit)
	}
	by := bottom(left) - 36
	if w.stripeButton(idStripeGit, rect(left.X+5, by, 30, 30), icons.Vcs, w.t(i18n.ChromeStripeGit)+"\t⌘9", w.gitOpen && !w.termOpen) {
		w.after(w.toggleGitWindow)
	}
	by -= 36
	if w.stripeButton(idStripeTerm, rect(left.X+5, by, 30, 30), icons.Shell, w.t(i18n.TermStripe), w.termOpen) {
		w.after(w.toggleTerminal)
	}
	unread := w.nt.unread(w.ws)
	if w.stripeButton(idStripeNotify, rect(rightS.X+5, rightS.Y+6, 30, 30), icons.Notifications, w.t(i18n.ChromeStripeNotifications), w.notifyOpen) {
		w.after(func() {
			w.notifyOpen = !w.notifyOpen
			w.nt.markRead(w.ws)
		})
	}
	if unread > 0 {
		w.p.round(rect(rightS.X+24, rightS.Y+10, 8, 8), 4, w.pal.Link)
	}
}

func (w *Window) stripeButton(id ui.ID, r ui.Rect, pair icons.Pair, tooltip string, selected bool) bool {
	switch {
	case selected:
		w.p.round(r, 6, w.pal.SelectedSoft)
	case w.pressed(id):
		w.p.round(r, 6, w.pal.ToolPressed)
	case w.hovered(id):
		w.p.round(r, 6, w.pal.ToolHover)
	}
	w.p.icon(pair, r.X+(r.Width-20)/2, r.Y+(r.Height-20)/2, 20)
	w.c.HitWithOptions(id, r, tooltip, ui.HitOptions{Role: ui.RoleButton, SkipFocus: true, Checked: selected})
	w.tip(id, tooltip)
	return w.c.Clicked(id)
}

// ---- status bar ---------------------------------------------------------------

func (w *Window) paintStatus(r ui.Rect) {
	rs := w.repo()
	x := r.X + 12
	// Breadcrumb: repository › branch.
	name := rs.Repo.Name()
	tw := w.p.text(name, rect(x, r.Y, 300, r.Height), fontUI, regular, w.pal.TextMuted)
	x += tw + 4
	w.p.iconIn(icons.ChevronRight, rect(x, r.Y, 16, r.Height))
	x += 18
	tw = w.p.text(rs.HeadLabel(), rect(x, r.Y, 300, r.Height), fontUI, regular, w.pal.TextMuted)
	x += tw

	// A repository that uses Git LFS gets a quiet marker with its managed file
	// count. It turns into a warning colour when the LFS rules exist but
	// git-lfs is missing, the case the Commit panel and notification explain.
	x += 20
	if rs.LFS.Managed {
		label := w.lfsStatusLabel(rs)
		lw := w.p.width(label, fontUI, regular)
		lr := rect(x, r.Y, lw+8, r.Height)
		color := w.pal.Link
		if rs.LFS.NeedsInstall() {
			color = w.pal.Warning
		}
		w.p.text(label, lr, fontUI, regular, color)
		tip := w.lfsStatusTip(rs)
		w.c.HitWithOptions(idStatusLFS, lr, tip, ui.HitOptions{Role: ui.RoleStatus, SkipFocus: true})
		w.tip(idStatusLFS, tip)
		x = right(lr) + 12
	}

	// Diagnostics recording is a quiet status marker, not a dialog: the user
	// keeps working while it lasts. Clicking it stops the recording and saves
	// the bundle, the same as Help > Stop Recording and Save Diagnostics.
	if w.ws.ProblemRecording() {
		label := w.t(i18n.ChromeStatusRecording)
		lw := w.p.width(label, fontUI, regular)
		rr := rect(x, r.Y, lw+10, r.Height)
		w.p.text(label, rr, fontUI, regular, w.pal.Warning)
		w.c.HitWithOptions(idStatusRecording, rr, w.t(i18n.ChromeStatusRecordingLabel), ui.HitOptions{Role: ui.RoleButton, SkipFocus: true})
		w.tip(idStatusRecording, w.t(i18n.ChromeTipRecording))
		w.clicked(idStatusRecording, w.stopRecordingAndSave)
	}

	rx := right(r) - 12
	// Branch widget with sync state.
	b := rs.Status.Branch
	label := rs.HeadLabel()
	sync := ""
	if b.Ahead > 0 {
		sync += "↑" + itoa(b.Ahead)
	}
	if b.Behind > 0 {
		if sync != "" {
			sync += " "
		}
		sync += "↓" + itoa(b.Behind)
	}
	bw := 18 + w.p.width(label, fontUI, regular) + 12
	if sync != "" {
		bw += w.p.width(sync, fontUI, regular) + 8
	}
	br := rect(rx-bw, r.Y+3, bw, r.Height-6)
	w.toolbarHover(idStatusBranch, br)
	w.p.iconIn(icons.Vcs, rect(br.X+4, br.Y, 16, br.Height))
	lx := br.X + 24
	lw := w.p.text(label, rect(lx, br.Y, 400, br.Height), fontUI, regular, w.pal.TextMuted)
	if sync != "" {
		w.p.text(sync, rect(lx+lw+8, br.Y, 100, br.Height), fontUI, regular, w.pal.Link)
	}
	w.c.HitWithOptions(idStatusBranch, br, w.t(i18n.ChromeA11yBranchButton), ui.HitOptions{Role: ui.RoleButton, SkipFocus: true})
	w.tip(idStatusBranch, w.branchTip())
	if w.clicked(idStatusBranch, nil) {
		w.after(func() { w.openBranchPopup(rect(br.X, br.Y-2, br.Width, 0)) })
	}
	rx = br.X - 12

	// Running task with progress.
	if len(w.ws.Tasks) > 0 {
		t := w.ws.Tasks[len(w.ws.Tasks)-1]
		title := t.Title
		if t.Progress != "" {
			title += ": " + t.Progress
		}
		title = w.p.truncateMiddle(title, fontUI, regular, 320)
		cw := float32(22)
		pw := float32(120)
		tw := w.p.width(title, fontUI, regular)
		x0 := rx - cw - pw - 10 - tw
		w.p.text(title, rect(x0, r.Y, tw, r.Height), fontUI, regular, w.pal.TextMuted)
		w.paintProgress(rect(x0+tw+10, r.Y+r.Height/2-2, pw, 4))
		if w.action(idStatusCancel, rx-cw, r.Y+3, icons.CloseSmall, w.t(i18n.CommonCancel), actOpts{size: 22}) {
			id := t.ID
			w.after(func() { w.ws.CancelTask(id) })
		}
		w.c.RequestFrameIn(16 * time.Millisecond)
	}
}

// paintProgress draws an indeterminate IntelliJ progress bar.
func (w *Window) paintProgress(r ui.Rect) {
	w.p.round(r, r.Height/2, w.pal.Separator)
	t := float32((w.c.Now() % (1400 * time.Millisecond)).Milliseconds()) / 1400
	seg := r.Width * 0.35
	x := r.X - seg + (r.Width+seg)*t
	w.c.Clip(r)
	w.p.round(rect(x, r.Y, seg, r.Height), r.Height/2, w.pal.Brand)
	w.c.Unclip()
}

// ---- tool window header ---------------------------------------------------------

// toolHeader paints a tool window's header with tabs; it returns the content
// rect below and the x where header actions may start (right to left).
func (w *Window) toolHeader(r ui.Rect, scope ui.Scope, tabs []string, sel int, onSel func(int)) (ui.Rect, float32) {
	hr := splitTop(&r, headerH)
	x := hr.X + 10
	for i, t := range tabs {
		id := scope.At(i)
		tw := w.p.width(t, fontUI, semibold)
		tr := rect(x, hr.Y+(hr.Height-26)/2, tw+16, 26)
		if i == sel {
			if len(tabs) > 1 {
				w.p.round(tr, 6, w.pal.ToolPressed)
			}
			w.p.text(t, rect(tr.X+8, tr.Y, tw+2, tr.Height), fontUI, semibold, w.pal.Text)
		} else {
			if w.hovered(id) {
				w.p.round(tr, 6, w.pal.ToolHover)
			}
			w.p.text(t, rect(tr.X+8, tr.Y, tw+2, tr.Height), fontUI, regular, w.pal.TextMuted)
		}
		if len(tabs) > 1 {
			w.c.HitWithOptions(id, tr, t, ui.HitOptions{Role: ui.RoleTab, Selected: i == sel, SkipFocus: true})
			if w.c.Clicked(id) {
				i := i
				w.after(func() { onSel(i) })
			}
		}
		x = right(tr) + 2
	}
	return r, right(hr) - 8
}

// ---- helpers ------------------------------------------------------------------

func revealPath(p string)   { _ = platform.Reveal(p) }
func openTerminal(p string) { _ = platform.OpenTerminal(p) }

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func shortenHome(p string) string {
	if home := homeDir(); home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func ago(t time.Time, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "Moments ago"
	case d < time.Hour:
		m := int(d.Minutes())
		return plural(m, "minute", "minutes") + " ago"
	case d < 24*time.Hour && now.YearDay() == t.YearDay():
		return "Today " + t.Format("15:04")
	case d < 48*time.Hour && now.AddDate(0, 0, -1).YearDay() == t.YearDay():
		return "Yesterday " + t.Format("15:04")
	case now.Year() == t.Year():
		return t.Format("Jan 2, 15:04")
	}
	return t.Format("Jan 2, 2006")
}

var _ = vapp.LogPage
