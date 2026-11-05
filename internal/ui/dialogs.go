package viffui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/assets/branding"
	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
	"viff/internal/version"
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
			return w.t(i18n.DialogBranchExistsOverwrite, n)
		}
		return ""
	}
	title := w.t(i18n.DialogNewBranchTitle)
	if startLabel != "" {
		title = w.t(i18n.DialogNewBranchFrom, startLabel)
	}
	w.openDialog(&dialog{
		title: title, ok: w.t(i18n.DialogCreate), width: 460,
		body: func(f *form) {
			n := strings.TrimSpace(name.String())
			id := f.field(w.t(i18n.DialogNewBranchName), name, "", n != "" && problem() != "")
			f.w.dialog.focus = id
			f.check(w.t(i18n.DialogCheckoutBranch), &checkout)
			f.check(w.t(i18n.DialogOverwriteBranch), &overwrite)
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
		title: w.t(i18n.DialogRenameTitle, r.Short), ok: w.t(i18n.DialogRename), width: 440,
		body: func(f *form) {
			n := strings.TrimSpace(name.String())
			p := validRefName(n)
			if p == "" && n != r.Short && w.branchExists(n) {
				p = w.t(i18n.DialogBranchExists, n)
			}
			id := f.field(w.t(i18n.DialogNewName), name, "", p != "")
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
		title: w.t(i18n.DialogSetUpstreamTitle, r.Short), ok: w.t(i18n.DialogSet), width: 440,
		body: func(f *form) {
			f.combo(w.t(i18n.DialogTrackRemote), sel, func() []item {
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
		title: w.t(i18n.DialogCreateTagTitle, label), ok: w.t(i18n.DialogCreateTag), width: 460,
		body: func(f *form) {
			n := strings.TrimSpace(name.String())
			p := validRefName(n)
			id := f.field(w.t(i18n.DialogTagName), name, "v1.0.0", n != "" && p != "")
			f.w.dialog.focus = id
			f.field(w.t(i18n.DialogTagMessage), msg, "", false)
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
		title: w.t(i18n.DialogCheckoutRevTitle), ok: w.t(i18n.DialogCheckout), width: 460,
		body: func(f *form) {
			id := f.field(w.t(i18n.DialogRevField), rev, "v1.2.0, a1b2c3d, HEAD~2", false)
			f.w.dialog.focus = id
			f.muted(w.t(i18n.DialogDetachedHint))
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
		{w.t(i18n.DialogResetSoft), w.t(i18n.DialogResetSoftDesc), gitcli.ResetSoft},
		{w.t(i18n.DialogResetMixed), w.t(i18n.DialogResetMixedDesc), gitcli.ResetMixed},
		{w.t(i18n.DialogResetHard), w.t(i18n.DialogResetHardDesc), gitcli.ResetHard},
		{w.t(i18n.DialogResetKeep), w.t(i18n.DialogResetKeepDesc), gitcli.ResetKeep},
	}
	target := rs.HeadLabel()
	w.openDialog(&dialog{
		title: w.t(i18n.DialogResetTitle), ok: w.t(i18n.DialogReset), width: 520,
		body: func(f *form) {
			f.text(w.t(i18n.DialogResetBody, target, domain.ShortHash(c.Hash), firstLine(c.Subject)))
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
				f.message(w.t(i18n.DialogResetWarning), w.pal.Warning)
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
	w.ws.Notify(vapp.NoticeWarning, w.t(i18n.NotifyNothingToPushTitle), w.t(i18n.NotifyNothingToPushBody))
}

// openPushFor is IntelliJ's Push dialog for one local branch.
func (w *Window) openPushFor(r domain.Ref) {
	rs := w.repo()
	if len(rs.Remotes) == 0 {
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.NotifyNoRemoteTitle), w.t(i18n.NotifyNoRemoteBody))
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
		title: w.t(i18n.DialogPushTitle, w.repo().Repo.Name()), ok: w.t(i18n.DialogPush), width: 640,
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
				w.field(scDialogFields.At(0), target, rect(x, rr.Y+1, fw, 28), fieldOpts{label: w.t(i18n.DialogRemoteBranch)})
				if upstream == "" || strings.TrimSpace(target.String()) != upstreamBranch(upstream) {
					w.p.text(w.t(i18n.DialogNewLabel), rect(x+fw+8, rr.Y, 60, rr.Height), fontSmall, semibold, w.pal.Success)
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
					w.p.textCenter(w.t(i18n.DialogLoadingCommits), inner, fontUI, regular, w.pal.TextSecond)
					return
				case loadErr != nil:
					w.p.textCenter(vapp.ErrorText(loadErr), inner, fontUI, regular, w.pal.Error)
					return
				case len(commits) == 0:
					w.p.textCenter(w.t(i18n.DialogNothingToPush), inner, fontUI, regular, w.pal.TextSecond)
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
				f.muted(w.pluralT(len(commits), i18n.DialogWillBePushedOne, i18n.DialogWillBePushedMany, len(commits)))
			}
			f.check(w.t(i18n.DialogPushTags), &tags)
			f.check(w.t(i18n.DialogPushForce), &force)
		},
		okDisabled: func() bool { return validRefName(strings.TrimSpace(target.String())) != "" },
		onOK: func() bool {
			if force {
				w.dialog = nil
				w.confirm(w.t(i18n.DialogForcePushTitle), w.t(i18n.DialogForcePushBody, r.Short, remote, strings.TrimSpace(target.String())), w.t(i18n.DialogForcePushTitle), func() {
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
		w.ws.Notify(vapp.NoticeWarning, w.t(i18n.NotifyCannotUpdateTitle), w.t(i18n.NotifyCannotUpdateBody))
		return
	}
	mode := int(w.pullMode())
	w.openDialog(&dialog{
		title: w.t(i18n.DialogUpdateTitle), ok: w.t(i18n.CommonOK), width: 460,
		body: func(f *form) {
			f.bold(w.t(i18n.DialogUpdateType))
			f.radio([]string{w.t(i18n.DialogPullMerge), w.t(i18n.DialogPullRebase), w.t(i18n.DialogPullFFOnly)}, &mode)
			f.gap(4)
			f.muted(w.t(i18n.DialogUpdateBody, rs.CurrentBranch(), rs.Status.Branch.Upstream))
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
		title: w.t(i18n.DialogStashTitle), ok: w.t(i18n.DialogCreateStash), width: 460,
		body: func(f *form) {
			f.muted(w.t(i18n.DialogBranchLabel, w.repo().HeadLabel()))
			id := f.field(w.t(i18n.DialogMessage), msg, w.t(i18n.DialogOptional), false)
			f.w.dialog.focus = id
			f.check(w.t(i18n.DialogIncludeUntracked), &untracked)
			f.check(w.t(i18n.DialogKeepStaged), &keep)
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
		title: w.t(i18n.DialogCloneTitle), ok: w.t(i18n.DialogClone), width: 560,
		body: func(f *form) {
			f.labelW = 90
			id := f.field(w.t(i18n.DialogURL), url, "https://github.com/user/repo.git", false)
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
				w.p.text(w.t(i18n.DialogDirectory), rect(r.X, r.Y, 90, fieldH), fontUI, regular, w.pal.Text)
				bw := float32(90)
				w.field(scDialogFields.At(10), parent, rect(r.X+90, r.Y, r.Width-90-bw-8, fieldH), fieldOpts{label: w.t(i18n.DialogParentDir)})
				if w.button(idDialogExtra, rect(right(r)-bw, r.Y, bw, fieldH), w.t(i18n.DialogBrowse), btnNormal, false) {
					w.after(func() {
						if d, err := app.OpenDirectory(app.OpenDirectoryOptions{Title: w.t(i18n.DialogCloneInto)}); err == nil && d != "" {
							setText(parent, d)
						}
					})
				}
			})
			f.field(w.t(i18n.DialogName), dirEd, "repo", false)
			p := filepath.Join(expandHome(strings.TrimSpace(parent.String())), strings.TrimSpace(dirEd.String()))
			if _, err := os.Stat(p); err == nil && strings.TrimSpace(dirEd.String()) != "" {
				f.message(w.t(i18n.DialogExists, p), w.pal.Error)
			} else if strings.TrimSpace(dirEd.String()) != "" {
				f.muted(w.t(i18n.DialogClonesInto, shortenHome(p)))
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
	dir, err := app.OpenDirectory(app.OpenDirectoryOptions{Title: w.t(i18n.DialogInitTitle)})
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
		title: w.t(i18n.DialogRemotesTitle), ok: w.t(i18n.CommonClose), width: 600, noCancel: true,
		body: func(f *form) {
			if len(rs.Remotes) == 0 {
				f.muted(w.t(i18n.DialogNoRemotes))
			}
			for i, r := range rs.Remotes {
				r := r
				f.custom(30, func(rr ui.Rect) {
					w.p.text(r.Name, rect(rr.X, rr.Y, 100, rr.Height), fontUI, semibold, w.pal.Text)
					w.p.text(r.FetchURL, rect(rr.X+110, rr.Y, rr.Width-110-80, rr.Height), fontUI, regular, w.pal.TextMuted)
					if w.button(scDialogLinks.At(i), rect(right(rr)-72, rr.Y+2, 72, 26), w.t(i18n.DialogRemove), btnBorderless, false) {
						w.after(func() {
							w.confirm(w.t(i18n.DialogRemoveRemoteTitle), w.t(i18n.DialogRemoveRemoteBody, r.Name), w.t(i18n.DialogRemove), func() { w.ws.RemoveRemote(r.Name) })
						})
					}
				})
			}
			f.gap(10)
			f.bold(w.t(i18n.DialogAddRemote))
			f.labelW = 60
			f.field(w.t(i18n.DialogName), name, "origin", false)
			f.field(w.t(i18n.DialogURL), url, "https://github.com/user/repo.git", false)
			f.custom(30, func(rr ui.Rect) {
				ok := validRefName(strings.TrimSpace(name.String())) == "" && strings.TrimSpace(url.String()) != ""
				if w.button(scDialogLinks.At(99), rect(rr.X+60, rr.Y, 100, 28), w.t(i18n.DialogAddRemoteButton), btnNormal, !ok) {
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
		{title: w.t(i18n.MenuSettings), icon: icons.Settings, shortcut: "⌘,", run: w.openSettings},
		{title: w.t(i18n.MenuTheme), sub: []item{
			theme("dark", w.t(i18n.SettingsThemeDark)),
			theme("light", w.t(i18n.SettingsThemeLight)),
			theme("system", w.t(i18n.SettingsThemeSystem)),
		}},
		sep(),
	}
	if w.ws.Repo != nil {
		items = append(items,
			item{title: w.t(i18n.MenuManageRemotes), run: w.openRemotes},
			item{title: w.t(i18n.MenuFetch), icon: icons.Fetch, disabled: len(w.repo().Remotes) == 0, run: func() { w.ws.Fetch(false) }},
			sep(),
		)
	}
	items = append(items, item{title: w.t(i18n.AboutTitle), icon: icons.Info, run: w.openAbout})
	return items
}

// openAbout shows the build identity plus the Go, Viva and Git versions. The
// build identity comes from internal/version, which release builds fill in
// with -ldflags.
func (w *Window) openAbout() {
	git := w.gitVersion()
	detail := "Go " + version.GoVersion() + " · Viva " + version.VivaVersion()
	if git != "" {
		detail += " · Git " + git
	}
	w.openDialog(&dialog{
		title: w.t(i18n.AboutTitle), ok: w.t(i18n.CommonClose), width: 480, noCancel: true,
		body: func(f *form) {
			f.custom(56, func(r ui.Rect) {
				w.c.DrawBitmap(branding.Bitmap, rect(r.X, r.Y+4, 44, 44))
				w.p.text("Viff", rect(r.X+58, r.Y+4, 200, 24), 18, semibold, w.pal.Text)
				w.p.text(w.t(i18n.AboutTagline), rect(r.X+58, r.Y+28, r.Width-58, 20), fontUI, regular, w.pal.TextMuted)
			})
			f.text("Viff " + version.String())
			f.muted(detail)
			f.muted(w.t(i18n.AboutCredits))
		},
	})
}

func (w *Window) openSettings() {
	s := w.ws.Settings
	theme := s.Theme
	editorFont := s.EditorFont
	diffFont := s.DiffFont
	language := s.Language
	// app.FontFamilies and app.MonospaceFamilies read the system on every call
	// and take a moment, so they are collected once when the dialog opens rather
	// than while painting.
	fonts := app.FontFamilies()
	monoFonts := app.MonospaceFamilies()
	font := text.NewEditor(strconv.Itoa(int(s.FontSize)))
	gitPath := text.NewEditor(s.GitPath)
	editor := text.NewEditor(s.ExternalEditor)
	autoFetch, prune := s.AutoFetch, s.FetchPrune
	autoUpdate, autoUpdateWas := s.AutoUpdateCheck, s.AutoUpdateCheck
	debugLog := s.LogLevel == "debug"
	mins := text.NewEditor(strconv.Itoa(s.AutoFetchMins))
	pull := s.PullMode
	gitDraft := newGitSettingsDraft(s)
	page := 0
	themeTitle := map[string]string{
		"dark":   w.t(i18n.SettingsThemeDark),
		"light":  w.t(i18n.SettingsThemeLight),
		"system": w.t(i18n.SettingsThemeSystem),
	}
	pullTitle := map[string]string{
		"merge":   w.t(i18n.SettingsPullMerge),
		"rebase":  w.t(i18n.SettingsPullRebase),
		"ff-only": w.t(i18n.SettingsPullFFOnly),
	}
	w.openDialog(&dialog{
		title: w.t(i18n.SettingsTitle), ok: w.t(i18n.CommonOK), cancel: w.t(i18n.CommonCancel), width: 600,
		body: func(f *form) {
			f.labelW = 170
			settingsTabs(f, &page, []string{w.t(i18n.SettingsTabGeneral), w.t(i18n.SettingsTabGit)})
			if page == 1 {
				f.bold(w.t(i18n.SettingsGroupCommands))
				gitDraft.form(f)
				f.gap(6)
				f.bold(w.t(i18n.SettingsGroupRemote))
				f.combo(w.t(i18n.SettingsLabelUpdateMode), pullTitle[pull], func() []item {
					var items []item
					for _, k := range []string{"merge", "rebase", "ff-only"} {
						k := k
						items = append(items, item{title: pullTitle[k], checked: pull == k, run: func() { pull = k }})
					}
					return items
				})
				f.check(w.t(i18n.SettingsCheckPrune), &prune)
				f.check(w.t(i18n.SettingsCheckAutoFetch), &autoFetch)
				if autoFetch {
					m, err := strconv.Atoi(strings.TrimSpace(mins.String()))
					f.field(w.t(i18n.SettingsLabelFetchMins), mins, "10", err != nil || m < 1)
				}
				return
			}
			f.bold(w.t(i18n.SettingsGroupAppearance))
			langCurrent := w.t(i18n.SettingsLangEnglish)
			if language == "zh-CN" {
				langCurrent = w.t(i18n.SettingsLangChinese)
			}
			f.combo(w.t(i18n.SettingsLabelLanguage), langCurrent, func() []item {
				return []item{
					{title: w.t(i18n.SettingsLangEnglish), checked: language != "zh-CN", run: func() { language = "en" }},
					{title: w.t(i18n.SettingsLangChinese), checked: language == "zh-CN", run: func() { language = "zh-CN" }},
				}
			})
			f.combo(w.t(i18n.SettingsLabelTheme), themeTitle[theme], func() []item {
				var items []item
				for _, k := range []string{"dark", "light", "system"} {
					k := k
					items = append(items, item{title: themeTitle[k], checked: theme == k, run: func() { theme = k }})
				}
				return items
			})
			n, err := strconv.Atoi(strings.TrimSpace(font.String()))
			f.field(w.t(i18n.SettingsLabelFontSize), font, "13", err != nil || n < 8 || n > 32)
			f.gap(6)
			f.bold(w.t(i18n.SettingsGroupGitExec))
			f.field(w.t(i18n.SettingsLabelGitPath), gitPath, w.t(i18n.SettingsPlaceholderAuto), false)
			f.gap(6)
			f.bold(w.t(i18n.SettingsGroupTools))
			f.field(w.t(i18n.SettingsLabelEditor), editor, "code -g {file}:{line}", false)
			f.y -= 8
			f.custom(22, func(r ui.Rect) {
				w.p.text(w.t(i18n.SettingsHintEditor), rect(r.X+170, r.Y, r.Width-170, 20), fontSmall, regular, w.pal.TextSecond)
			})
			f.gap(4)
			f.custom(22, func(r ui.Rect) {
				w.p.text(w.t(i18n.SettingsLabelSettingsFile, shortenHome(s.Path())), rect(r.X, r.Y, r.Width, 20), fontSmall, regular, w.pal.TextDisable)
			})
			f.gap(6)
			f.bold(w.t(i18n.SettingsGroupDiagnostics))
			f.check(w.t(i18n.SettingsCheckDebugLog), &debugLog)
			f.custom(22, func(r ui.Rect) {
				w.p.text(w.t(i18n.SettingsHintDebugLog), rect(r.X+170, r.Y, r.Width-170, 20), fontSmall, regular, w.pal.TextSecond)
			})
			f.gap(6)
			f.bold(w.t(i18n.SettingsGroupUpdates))
			f.check(w.t(i18n.SettingsCheckAutoUpdate), &autoUpdate)
			f.gap(6)
			f.bold(w.t(i18n.SettingsGroupFonts))
			interfaceDefault := w.t(i18n.SettingsFontDefaultInterface)
			current := interfaceDefault
			if editorFont != "" {
				current = editorFont
			}
			f.combo(w.t(i18n.SettingsLabelInterface), current, func() []item {
				items := []item{{title: interfaceDefault, checked: editorFont == "", run: func() { editorFont = "" }}}
				for _, name := range fonts {
					name := name
					items = append(items, item{title: name, checked: editorFont == name, run: func() { editorFont = name }})
				}
				return items
			})
			f.custom(22, func(r ui.Rect) {
				w.p.text(w.t(i18n.SettingsHintInterface), rect(r.X+170, r.Y, r.Width-170, 20), fontSmall, regular, w.pal.TextSecond)
			})
			diffDefault := w.t(i18n.SettingsFontDefaultDiff)
			diffCurrent := diffDefault
			if diffFont != "" {
				diffCurrent = diffFont
			}
			f.combo(w.t(i18n.SettingsLabelDiffFont), diffCurrent, func() []item {
				items := []item{{title: diffDefault, checked: diffFont == "", run: func() { diffFont = "" }}}
				for _, name := range monoFonts {
					name := name
					items = append(items, item{title: name, checked: diffFont == name, run: func() { diffFont = name }})
				}
				return items
			})
			f.custom(22, func(r ui.Rect) {
				w.p.text(w.t(i18n.SettingsHintDiffFont), rect(r.X+170, r.Y, r.Width-170, 20), fontSmall, regular, w.pal.TextSecond)
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
			w.ws.Git.SetPath(s.GitPath)
			s.ExternalEditor = strings.TrimSpace(editor.String())
			gitDraft.commit(s)
			s.AutoFetch, s.FetchPrune, s.PullMode = autoFetch, prune, pull
			s.AutoUpdateCheck = autoUpdate
			if m > 0 {
				s.AutoFetchMins = m
			}
			s.LogLevel = ""
			if debugLog {
				s.LogLevel = "debug"
			}
			w.ws.SetLogLevel(s.LogLevel)
			s.EditorFont = editorFont
			s.DiffFont = diffFont
			s.Language = language
			w.applyTheme()
			w.applyFont()
			w.applyLanguage()
			w.Invalidate()
			w.flushSettings()
			w.startAutoFetch()
			if autoUpdate && !autoUpdateWas {
				w.ws.CheckForUpdates(false)
			}
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
