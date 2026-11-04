package viffui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/ui/icons"
)

// validRefName applies the main rules of git check-ref-format.
func validRefName(s string) string {
	switch {
	case s == "":
		return "Enter a name"
	case strings.ContainsAny(s, " ~^:?*[\\\t"):
		return "Names cannot contain spaces or any of ~ ^ : ? * [ \\"
	case strings.HasPrefix(s, "-") || strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") || strings.HasSuffix(s, "."):
		return "Names cannot start with - or /, or end with / or ."
	case strings.Contains(s, "..") || strings.Contains(s, "@{") || strings.Contains(s, "//"):
		return "Names cannot contain .., @{ or //"
	case strings.HasSuffix(s, ".lock"):
		return "Names cannot end with .lock"
	case s == "@" || s == "HEAD":
		return "This name is reserved"
	}
	return ""
}

func (w *Window) branchExists(name string) bool {
	for _, r := range w.repo().RefsOf(domain.RefLocal) {
		if r.Short == name {
			return true
		}
	}
	return false
}

// openNewBranch is IntelliJ's Create New Branch dialog.
func (w *Window) openNewBranch(start, startLabel string) {
	name := text.NewEditor("")
	checkout, overwrite := true, false
	problem := func() string {
		n := strings.TrimSpace(name.String())
		if p := validRefName(n); p != "" {
			return p
		}
		if w.branchExists(n) && !overwrite {
			return "Branch '" + n + "' already exists. Select Overwrite to reset it here."
		}
		return ""
	}
	title := "Create New Branch"
	if startLabel != "" {
		title += " from " + startLabel
	}
	w.openDialog(&dialog{
		title: title, ok: "Create", width: 460,
		body: func(f *form) {
			n := strings.TrimSpace(name.String())
			id := f.field("New branch name:", name, "", n != "" && problem() != "")
			f.w.dialog.focus = id
			f.check("Checkout branch", &checkout)
			f.check("Overwrite existing branch", &overwrite)
			if n != "" {
				if p := problem(); p != "" {
					f.message(p, w.pal.Error)
				}
			}
		},
		okDisabled: func() bool { return problem() != "" },
		onOK: func() bool {
			w.ws.NewBranch(strings.TrimSpace(name.String()), start, checkout, overwrite)
			return true
		},
	})
}

func (w *Window) openRename(r domain.Ref) {
	name := text.NewEditor(r.Short)
	w.openDialog(&dialog{
		title: "Rename Branch " + r.Short, ok: "Rename", width: 440,
		body: func(f *form) {
			n := strings.TrimSpace(name.String())
			p := validRefName(n)
			if p == "" && n != r.Short && w.branchExists(n) {
				p = "Branch '" + n + "' already exists"
			}
			id := f.field("New name:", name, "", p != "")
			f.w.dialog.focus = id
			if p != "" && n != "" {
				f.message(p, w.pal.Error)
			}
		},
		okDisabled: func() bool {
			n := strings.TrimSpace(name.String())
			return validRefName(n) != "" || n == r.Short || w.branchExists(n)
		},
		onOK: func() bool { w.ws.RenameBranch(r.Short, strings.TrimSpace(name.String())); return true },
	})
}

func (w *Window) openSetUpstream(r domain.Ref) {
	choices := w.repo().RefsOf(domain.RefRemote)
	sel := w.defaultRemote() + "/" + r.Short
	w.openDialog(&dialog{
		title: "Set Upstream for " + r.Short, ok: "Set", width: 440,
		body: func(f *form) {
			f.combo("Track remote branch:", sel, func() []item {
				var items []item
				for _, c := range choices {
					c := c
					items = append(items, item{title: c.Short, checked: c.Short == sel, run: func() { sel = c.Short }})
				}
				return items
			})
		},
		onOK: func() bool { w.ws.SetUpstream(r.Short, sel); return true },
	})
}

func (w *Window) openNewTag(rev, label string) {
	name := text.NewEditor("")
	msg := text.NewEditor("")
	w.openDialog(&dialog{
		title: "Create Tag on " + label, ok: "Create Tag", width: 460,
		body: func(f *form) {
			n := strings.TrimSpace(name.String())
			p := validRefName(n)
			id := f.field("Tag name:", name, "v1.0.0", n != "" && p != "")
			f.w.dialog.focus = id
			f.field("Message (optional, makes an annotated tag):", msg, "", false)
			if n != "" && p != "" {
				f.message(p, w.pal.Error)
			}
		},
		okDisabled: func() bool { return validRefName(strings.TrimSpace(name.String())) != "" },
		onOK: func() bool {
			w.ws.CreateTag(strings.TrimSpace(name.String()), rev, strings.TrimSpace(msg.String()))
			return true
		},
	})
}

func (w *Window) openCheckoutRevision() {
	rev := text.NewEditor("")
	w.openDialog(&dialog{
		title: "Checkout Tag or Revision", ok: "Checkout", width: 460,
		body: func(f *form) {
			id := f.field("Tag, commit hash or reference:", rev, "v1.2.0, a1b2c3d, HEAD~2", false)
			f.w.dialog.focus = id
			f.muted("HEAD will be detached. Create a branch to keep commits you make there.")
		},
		okDisabled: func() bool { return strings.TrimSpace(rev.String()) == "" },
		onOK:       func() bool { w.ws.CheckoutRevision(strings.TrimSpace(rev.String())); return true },
	})
}

// openReset is IntelliJ's Git Reset dialog.
func (w *Window) openReset(c domain.Commit) {
	rs := w.repo()
	mode := 1
	modes := []struct {
		title, desc string
		m           gitcli.ResetMode
	}{
		{"Soft", "Files are untouched; the differences are staged for commit.", gitcli.ResetSoft},
		{"Mixed", "Files are untouched; the differences are not staged.", gitcli.ResetMixed},
		{"Hard", "Files are reverted to the commit. Local changes are lost.", gitcli.ResetHard},
		{"Keep", "Files are reverted to the commit, but local changes are kept. Fails if they touch the same files.", gitcli.ResetKeep},
	}
	target := rs.HeadLabel()
	w.openDialog(&dialog{
		title: "Git Reset", ok: "Reset", width: 520,
		body: func(f *form) {
			f.text("Reset " + target + " to " + domain.ShortHash(c.Hash) + " \"" + firstLine(c.Subject) + "\"")
			f.gap(4)
			for i, m := range modes {
				id := scDialogRadios.At(i)
				if w.radio(id, f.x, f.y, 24, m.title, mode == i) {
					i := i
					w.after(func() { mode = i })
				}
				f.y += 24
				w.p.text(m.desc, rect(f.x+22, f.y, f.width-22, 18), fontSmall, regular, w.pal.TextSecond)
				f.y += 26
			}
			if mode == 2 && !rs.Status.Clean() {
				f.message("Your uncommitted changes will be discarded.", w.pal.Warning)
			}
		},
		onOK: func() bool { w.ws.Reset(modes[mode].m, c.Hash); return true },
	})
}

// ---- push -------------------------------------------------------------------

func (w *Window) openPush() {
	rs := w.repo()
	if r, ok := rs.CurrentRef(); ok {
		w.openPushFor(r)
		return
	}
	w.ws.Notify(vapp.NoticeWarning, "Nothing to push", "HEAD is detached. Create a branch to push your commits.")
}

// openPushFor is IntelliJ's Push dialog for one local branch.
func (w *Window) openPushFor(r domain.Ref) {
	rs := w.repo()
	if len(rs.Remotes) == 0 {
		w.ws.Notify(vapp.NoticeWarning, "No remote", "Add a remote before pushing (Settings → Manage Remotes).")
		return
	}
	remote := w.defaultRemote()
	target := text.NewEditor(r.Short)
	upstream := r.Upstream
	if upstream != "" {
		if i := strings.IndexByte(upstream, '/'); i > 0 {
			remote = upstream[:i]
			setText(target, upstream[i+1:])
		}
	}
	var commits []domain.Commit
	loading, loadErr := true, error(nil)
	w.ws.Outgoing(r.Short, upstream, func(cs []domain.Commit, err error) {
		commits, loading, loadErr = cs, false, err
	})
	tags, force := false, false
	var sel int = -1
	listScroll := &scroller{}
	w.openDialog(&dialog{
		title: "Push Commits to " + w.repo().Repo.Name(), ok: "Push", width: 640,
		body: func(f *form) {
			// Branch → remote : target
			f.custom(30, func(rr ui.Rect) {
				x := rr.X
				w.p.iconTinted(icons.CurrentBranchLabel, rect(x, rr.Y, 16, rr.Height), w.pal.RefHead)
				x += 22
				x += w.p.text(r.Short, rect(x, rr.Y, 200, rr.Height), fontUI, semibold, w.pal.Text) + 8
				x += w.p.text("→", rect(x, rr.Y, 20, rr.Height), fontUI, regular, w.pal.TextMuted) + 8
				rid := scDialogCombos.At(0)
				rw := w.p.width(remote, fontUI, regular) + 40
				rr2 := rect(x, rr.Y+1, rw, 28)
				if w.combo(rid, rr2, remote, len(rs.Remotes) < 2) {
					w.after(func() {
						var items []item
						for _, rm := range rs.Remotes {
							rm := rm
							items = append(items, item{title: rm.Name, hint: rm.FetchURL, checked: rm.Name == remote, run: func() { remote = rm.Name }})
						}
						w.openMenuBelow(rr2, items, 220)
					})
				}
				x += rw + 6
				x += w.p.text(":", rect(x, rr.Y, 10, rr.Height), fontUI, regular, w.pal.TextMuted) + 6
				fw := min(240, right(rr)-x-80)
				w.field(scDialogFields.At(0), target, rect(x, rr.Y+1, fw, 28), fieldOpts{label: "Remote branch"})
				if upstream == "" || strings.TrimSpace(target.String()) != upstreamBranch(upstream) {
					w.p.text("New", rect(x+fw+8, rr.Y, 60, rr.Height), fontSmall, semibold, w.pal.Success)
				}
			})
			f.gap(10)
			// Outgoing commits.
			listH := float32(220)
			f.custom(listH, func(lr ui.Rect) {
				w.p.outline(lr, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
				inner := inset(lr, 1, 1)
				switch {
				case loading:
					w.p.textCenter("Loading commits…", inner, fontUI, regular, w.pal.TextSecond)
					return
				case loadErr != nil:
					w.p.textCenter(vapp.ErrorText(loadErr), inner, fontUI, regular, w.pal.Error)
					return
				case len(commits) == 0:
					w.p.textCenter("Nothing to push", inner, fontUI, regular, w.pal.TextSecond)
					return
				}
				content := float32(len(commits))*rowH + 8
				listScroll.clamp(content, inner.Height)
				w.onScrollIn(inner, func(dx, dy float32) { listScroll.Y += dy; listScroll.clamp(content, inner.Height) })
				w.c.Clip(inner)
				for i, c := range commits {
					y := inner.Y + 4 + float32(i)*rowH - listScroll.Y
					if y+rowH < inner.Y || y > bottom(inner) {
						continue
					}
					row := rect(inner.X+4, y, inner.Width-8, rowH)
					id := scDialogRows.At(i)
					if sel == i {
						w.p.round(row, 4, w.pal.Selection)
					} else if w.hovered(id) {
						w.p.round(row, 4, w.pal.Hover)
					}
					w.p.iconIn(icons.Commit, rect(row.X+4, y, 16, rowH))
					w.p.text(c.Subject, rect(row.X+26, y, row.Width-26-220, rowH), fontUI, regular, w.pal.Text)
					w.p.text(c.Author, rect(right(row)-216, y, 110, rowH), fontUI, regular, w.pal.TextMuted)
					w.p.text(domain.ShortHash(c.Hash), rect(right(row)-100, y, 90, rowH), fontUI, regular, w.pal.TextSecond)
					w.c.HitWithOptions(id, row, c.Subject, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true})
					if w.c.Clicked(id) {
						i := i
						w.after(func() { sel = i })
					}
				}
				w.c.Unclip()
				w.vbar(scLogScrollbars.At(20), inner, listScroll, content)
			})
			f.gap(10)
			if !loading && len(commits) > 0 {
				f.muted(plural(len(commits), "commit", "commits") + " will be pushed")
			}
			f.check("Push tags (annotated tags on the pushed commits)", &tags)
			f.check("Force push (with lease: refuses if the remote has commits you have not fetched)", &force)
		},
		okDisabled: func() bool { return validRefName(strings.TrimSpace(target.String())) != "" },
		onOK: func() bool {
			if force {
				w.dialog = nil
				w.confirm("Force Push", "Force-push "+r.Short+" to "+remote+"/"+strings.TrimSpace(target.String())+"? Commits on the remote that are not in your branch will be removed.", "Force Push", func() {
					w.doPush(r, remote, strings.TrimSpace(target.String()), tags, true)
				})
				return false
			}
			w.doPush(r, remote, strings.TrimSpace(target.String()), tags, false)
			return true
		},
	})
}

func upstreamBranch(up string) string {
	if i := strings.IndexByte(up, '/'); i > 0 {
		return up[i+1:]
	}
	return up
}

func (w *Window) doPush(r domain.Ref, remote, target string, tags, force bool) {
	w.ws.Push(gitcli.PushOptions{
		Remote: remote, Branch: r.Short, RemoteName: target,
		SetUpstream: r.Upstream == "", ForceWithLease: force, Tags: tags,
	}, nil)
}

// ---- update -----------------------------------------------------------------

func (w *Window) pullMode() gitcli.PullMode {
	switch w.ws.Settings.PullMode {
	case "rebase":
		return gitcli.PullRebase
	case "ff-only":
		return gitcli.PullFastForwardOnly
	}
	return gitcli.PullMerge
}

// openUpdate is IntelliJ's Update Project dialog.
func (w *Window) openUpdate() {
	rs := w.repo()
	if len(rs.Remotes) == 0 {
		return
	}
	if rs.Status.Branch.Upstream == "" {
		w.ws.Notify(vapp.NoticeWarning, "Cannot update", "The current branch has no upstream. Push it first or set an upstream in the branch popup.")
		return
	}
	mode := int(w.pullMode())
	w.openDialog(&dialog{
		title: "Update Project", ok: "OK", width: 460,
		body: func(f *form) {
			f.bold("Update Type")
			f.radio([]string{"Merge incoming changes into the current branch", "Rebase the current branch on top of incoming changes", "Fast-forward only"}, &mode)
			f.gap(4)
			f.muted("Updates " + rs.CurrentBranch() + " from " + rs.Status.Branch.Upstream + ".")
		},
		onOK: func() bool {
			w.ws.Settings.PullMode = []string{"merge", "rebase", "ff-only"}[mode]
			w.saveSettings()
			w.ws.Pull(gitcli.PullMode(mode))
			return true
		},
	})
}

// ---- stash ------------------------------------------------------------------

func (w *Window) openStashDialog() {
	msg := text.NewEditor("")
	untracked, keep := false, false
	w.openDialog(&dialog{
		title: "Stash Changes", ok: "Create Stash", width: 460,
		body: func(f *form) {
			f.muted("Branch: " + w.repo().HeadLabel())
			id := f.field("Message:", msg, "Optional", false)
			f.w.dialog.focus = id
			f.check("Include unversioned files", &untracked)
			f.check("Keep staged changes in the index", &keep)
		},
		onOK: func() bool { w.ws.StashPush(strings.TrimSpace(msg.String()), untracked, keep); return true },
	})
}

// ---- clone and init -------------------------------------------------------------

func (w *Window) openClone() {
	url := text.NewEditor("")
	parent := text.NewEditor(defaultProjectsDir())
	dirEd := text.NewEditor("")
	lastAuto := ""
	w.openDialog(&dialog{
		title: "Get from Version Control", ok: "Clone", width: 560,
		body: func(f *form) {
			f.labelW = 90
			id := f.field("URL:", url, "https://github.com/user/repo.git", false)
			if !w.dialog.focused {
				w.dialog.focus = id
			}
			// Suggest the folder name from the URL until the user edits it.
			auto := repoNameFromURL(strings.TrimSpace(url.String()))
			if dirEd.String() == lastAuto && auto != lastAuto {
				lastAuto = auto
				w.after(func() { setText(dirEd, auto) })
			}
			f.custom(fieldH+12, func(r ui.Rect) {
				w.p.text("Directory:", rect(r.X, r.Y, 90, fieldH), fontUI, regular, w.pal.Text)
				bw := float32(90)
				w.field(scDialogFields.At(10), parent, rect(r.X+90, r.Y, r.Width-90-bw-8, fieldH), fieldOpts{label: "Parent directory"})
				if w.button(idDialogExtra, rect(right(r)-bw, r.Y, bw, fieldH), "Browse…", btnNormal, false) {
					w.after(func() {
						if d, err := app.OpenDirectory(app.OpenDirectoryOptions{Title: "Clone into"}); err == nil && d != "" {
							setText(parent, d)
						}
					})
				}
			})
			f.field("Name:", dirEd, "repo", false)
			p := filepath.Join(expandHome(strings.TrimSpace(parent.String())), strings.TrimSpace(dirEd.String()))
			if _, err := os.Stat(p); err == nil && strings.TrimSpace(dirEd.String()) != "" {
				f.message(p+" already exists", w.pal.Error)
			} else if strings.TrimSpace(dirEd.String()) != "" {
				f.muted("Clones into " + shortenHome(p))
			}
		},
		okDisabled: func() bool {
			p := filepath.Join(expandHome(strings.TrimSpace(parent.String())), strings.TrimSpace(dirEd.String()))
			_, err := os.Stat(p)
			return strings.TrimSpace(url.String()) == "" || strings.TrimSpace(dirEd.String()) == "" || err == nil
		},
		onOK: func() bool {
			w.ws.Clone(strings.TrimSpace(url.String()), expandHome(strings.TrimSpace(parent.String())), strings.TrimSpace(dirEd.String()), nil)
			return true
		},
	})
}

func repoNameFromURL(u string) string {
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, ".git")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		u = u[i+1:]
	}
	return u
}

func defaultProjectsDir() string {
	return filepath.Join(homeDir(), "Projects")
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		return homeDir() + p[1:]
	}
	return p
}

func (w *Window) openInit() {
	dir, err := app.OpenDirectory(app.OpenDirectoryOptions{Title: "Create Git Repository in"})
	if err != nil || dir == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		w.openRepo(dir)
		return
	}
	w.ws.Init(dir, nil)
}

// ---- remotes ------------------------------------------------------------------

func (w *Window) openRemotes() {
	rs := w.repo()
	name := text.NewEditor("")
	url := text.NewEditor("")
	w.openDialog(&dialog{
		title: "Git Remotes", ok: "Close", width: 600, noCancel: true,
		body: func(f *form) {
			if len(rs.Remotes) == 0 {
				f.muted("This repository has no remotes.")
			}
			for i, r := range rs.Remotes {
				r := r
				f.custom(30, func(rr ui.Rect) {
					w.p.text(r.Name, rect(rr.X, rr.Y, 100, rr.Height), fontUI, semibold, w.pal.Text)
					w.p.text(r.FetchURL, rect(rr.X+110, rr.Y, rr.Width-110-80, rr.Height), fontUI, regular, w.pal.TextMuted)
					if w.button(scDialogLinks.At(i), rect(right(rr)-72, rr.Y+2, 72, 26), "Remove", btnBorderless, false) {
						w.after(func() {
							w.confirm("Remove Remote", "Remove remote '"+r.Name+"'? Its remote-tracking branches are deleted.", "Remove", func() { w.ws.RemoveRemote(r.Name) })
						})
					}
				})
			}
			f.gap(10)
			f.bold("Add Remote")
			f.labelW = 60
			f.field("Name:", name, "origin", false)
			f.field("URL:", url, "https://github.com/user/repo.git", false)
			f.custom(30, func(rr ui.Rect) {
				ok := validRefName(strings.TrimSpace(name.String())) == "" && strings.TrimSpace(url.String()) != ""
				if w.button(scDialogLinks.At(99), rect(rr.X+60, rr.Y, 100, 28), "Add Remote", btnNormal, !ok) {
					w.after(func() {
						w.ws.AddRemote(strings.TrimSpace(name.String()), strings.TrimSpace(url.String()))
						setText(name, "")
						setText(url, "")
					})
				}
			})
		},
		onOK: func() bool { return true },
	})
}

// ---- settings -----------------------------------------------------------------

func (w *Window) settingsMenu() []item {
	s := w.ws.Settings
	theme := func(name, title string) item {
		return item{title: title, checked: s.Theme == name, run: func() { s.Theme = name; w.applyTheme(); w.saveSettings() }}
	}
	items := []item{
		{title: "Settings…", icon: icons.Settings, shortcut: "⌘,", run: w.openSettings},
		{title: "Theme", sub: []item{theme("dark", "Islands Dark"), theme("light", "Islands Light"), theme("system", "Sync with OS")}},
		sep(),
	}
	if w.ws.Repo != nil {
		items = append(items,
			item{title: "Manage Remotes…", run: w.openRemotes},
			item{title: "Fetch", icon: icons.Fetch, disabled: len(w.repo().Remotes) == 0, run: func() { w.ws.Fetch(false) }},
			sep(),
		)
	}
	items = append(items, item{title: "About Viff", icon: icons.Info, run: w.openAbout})
	return items
}

func (w *Window) openAbout() {
	ver := ""
	if v, err := w.ws.Git.Version(nil); err == nil {
		ver = v
	}
	w.openDialog(&dialog{
		title: "About Viff", ok: "Close", width: 420, noCancel: true,
		body: func(f *form) {
			f.custom(56, func(r ui.Rect) {
				w.projectBadge(rect(r.X, r.Y+4, 44, 44), "Viff", 10)
				w.p.text("Viff", rect(r.X+58, r.Y+4, 200, 24), 18, semibold, w.pal.Text)
				w.p.text("A Git client in the style of IntelliJ IDEA", rect(r.X+58, r.Y+28, r.Width-58, 20), fontUI, regular, w.pal.TextMuted)
			})
			if ver != "" {
				f.muted("Using " + ver)
			}
			f.muted("Icons: IntelliJ Platform (Apache 2.0). Editor font: JetBrains Mono (OFL).")
		},
	})
}

func (w *Window) openSettings() {
	s := w.ws.Settings
	theme := s.Theme
	font := text.NewEditor(strconv.Itoa(int(s.FontSize)))
	gitPath := text.NewEditor(s.GitPath)
	editor := text.NewEditor(s.ExternalEditor)
	autoFetch, prune := s.AutoFetch, s.FetchPrune
	mins := text.NewEditor(strconv.Itoa(s.AutoFetchMins))
	pull := s.PullMode
	themeTitle := map[string]string{"dark": "Islands Dark", "light": "Islands Light", "system": "Sync with OS"}
	pullTitle := map[string]string{"merge": "Merge", "rebase": "Rebase", "ff-only": "Fast-forward only"}
	w.openDialog(&dialog{
		title: "Settings", ok: "OK", width: 600,
		body: func(f *form) {
			f.labelW = 170
			f.bold("Appearance")
			f.combo("Theme:", themeTitle[theme], func() []item {
				var items []item
				for _, k := range []string{"dark", "light", "system"} {
					k := k
					items = append(items, item{title: themeTitle[k], checked: theme == k, run: func() { theme = k }})
				}
				return items
			})
			n, err := strconv.Atoi(strings.TrimSpace(font.String()))
			f.field("Editor font size:", font, "13", err != nil || n < 8 || n > 32)
			f.gap(6)
			f.bold("Git")
			f.field("Path to Git executable:", gitPath, "Detected automatically", false)
			f.combo("Update method:", pullTitle[pull], func() []item {
				var items []item
				for _, k := range []string{"merge", "rebase", "ff-only"} {
					k := k
					items = append(items, item{title: pullTitle[k], checked: pull == k, run: func() { pull = k }})
				}
				return items
			})
			f.check("Prune remote branches on fetch", &prune)
			f.check("Fetch in the background", &autoFetch)
			if autoFetch {
				m, err := strconv.Atoi(strings.TrimSpace(mins.String()))
				f.field("Every (minutes):", mins, "10", err != nil || m < 1)
			}
			f.gap(6)
			f.bold("Tools")
			f.field("External editor command:", editor, "code -g {file}:{line}", false)
			f.y -= 8
			f.custom(22, func(r ui.Rect) {
				w.p.text("Empty opens files with the system default. {file} and {line} are replaced.", rect(r.X+170, r.Y, r.Width-170, 20), fontSmall, regular, w.pal.TextSecond)
			})
			f.gap(4)
			f.custom(22, func(r ui.Rect) {
				w.p.text("Settings file: "+shortenHome(s.Path()), rect(r.X, r.Y, r.Width, 20), fontSmall, regular, w.pal.TextDisable)
			})
		},
		okDisabled: func() bool {
			n, err := strconv.Atoi(strings.TrimSpace(font.String()))
			if err != nil || n < 8 || n > 32 {
				return true
			}
			if autoFetch {
				m, err := strconv.Atoi(strings.TrimSpace(mins.String()))
				return err != nil || m < 1
			}
			return false
		},
		onOK: func() bool {
			n, _ := strconv.Atoi(strings.TrimSpace(font.String()))
			m, _ := strconv.Atoi(strings.TrimSpace(mins.String()))
			s.Theme = theme
			s.FontSize = float32(n)
			s.GitPath = strings.TrimSpace(gitPath.String())
			w.ws.Git.Path = s.GitPath
			s.ExternalEditor = strings.TrimSpace(editor.String())
			s.AutoFetch, s.FetchPrune, s.PullMode = autoFetch, prune, pull
			if m > 0 {
				s.AutoFetchMins = m
			}
			w.applyTheme()
			w.flushSettings()
			w.startAutoFetch()
			return true
		},
	})
}

// addToGitignore appends untracked paths to the repository's .gitignore.
func (w *Window) addToGitignore(rows []changeRow) {
	root := w.repo().Repo.Root
	p := filepath.Join(root, ".gitignore")
	var b strings.Builder
	old, _ := os.ReadFile(p)
	if len(old) > 0 && old[len(old)-1] != '\n' {
		b.WriteByte('\n')
	}
	for _, r := range rows {
		b.WriteString("/" + r.entry.Path + "\n")
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		_, err = f.WriteString(b.String())
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		w.ws.NotifyError("Cannot update .gitignore", err)
		return
	}
	w.ws.Refresh(false)
}

var _ = icons.Add
