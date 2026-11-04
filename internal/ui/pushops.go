package viffui

import (
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/ui/icons"
)

// ---- push variants ----------------------------------------------------------
//
// Pushing an older commit, pushing HEAD to another remote or branch name, and
// deleting a remote tag. The dialogs reuse the shared combo and field scopes,
// so this file does not add IDs to ids.go.

// openPushUpToCommit pushes c and every commit before it to a remote branch.
func (w *Window) openPushUpToCommit(c domain.Commit) {
	rs := w.repo()
	if len(rs.Remotes) == 0 {
		w.ws.Notify(vapp.NoticeWarning, "No remote", "Add a remote before pushing (Settings → Manage Remotes).")
		return
	}
	remote := w.defaultRemote()
	target := text.NewEditor(rs.CurrentBranch())
	force := false
	short := domain.ShortHash(c.Hash)
	w.openDialog(&dialog{
		title: "Push Up to " + short, ok: "Push", width: 540,
		body: func(f *form) {
			f.labelW = 76
			f.custom(50, func(rr ui.Rect) {
				w.p.round(rr, 6, w.pal.ControlBg)
				w.p.iconIn(icons.Commit, rect(rr.X+10, rr.Y+7, 16, 16))
				w.p.text(c.Subject, rect(rr.X+32, rr.Y+5, rr.Width-42, 20), fontUI, semibold, w.pal.Text)
				w.p.text(short+" · "+c.Author, rect(rr.X+32, rr.Y+25, rr.Width-42, 18), fontSmall, regular, w.pal.TextMuted)
			})
			f.gap(10)
			f.combo("Remote", remote, func() []item {
				var items []item
				for _, rm := range rs.Remotes {
					rm := rm
					items = append(items, item{title: rm.Name, hint: rm.FetchURL, checked: rm.Name == remote, run: func() { remote = rm.Name }})
				}
				return items
			})
			f.field("Branch", target, "Remote branch", validRefName(strings.TrimSpace(target.String())) != "")
			f.check("Force push (with lease: protects unseen remote commits)", &force)
			f.gap(2)
			f.muted("Pushes " + short + " and every commit before it to " + remote + "/" + branchOrDash(target))
		},
		okDisabled: func() bool {
			return validRefName(strings.TrimSpace(target.String())) != ""
		},
		onOK: func() bool {
			name := strings.TrimSpace(target.String())
			if name == "" {
				return false
			}
			if force {
				w.dialog = nil
				w.confirm("Force Push", "Force-push up to "+short+" to "+remote+"/"+name+"? Commits on the remote that are not in this history will be removed.", "Force Push", func() {
					w.ws.PushUpTo(remote, name, c.Hash, true)
				})
				return false
			}
			w.ws.PushUpTo(remote, name, c.Hash, false)
			return true
		},
	})
}

// openPushAnother pushes HEAD to a different remote or under another branch
// name, leaving the local branch and its upstream alone.
func (w *Window) openPushAnother(r domain.Ref) {
	rs := w.repo()
	if len(rs.Remotes) == 0 {
		w.ws.Notify(vapp.NoticeWarning, "No remote", "Add a remote before pushing (Settings → Manage Remotes).")
		return
	}
	remote := w.defaultRemote()
	target := text.NewEditor(r.Short)
	force := false
	w.openDialog(&dialog{
		title: "Push HEAD to Another Remote", ok: "Push", width: 520,
		body: func(f *form) {
			f.labelW = 76
			f.muted("Pushes the current HEAD (" + rs.HeadLabel() + ") to the remote and branch below.")
			f.gap(8)
			f.combo("Remote", remote, func() []item {
				var items []item
				for _, rm := range rs.Remotes {
					rm := rm
					items = append(items, item{title: rm.Name, hint: rm.FetchURL, checked: rm.Name == remote, run: func() { remote = rm.Name }})
				}
				return items
			})
			f.field("Branch", target, "Remote branch", validRefName(strings.TrimSpace(target.String())) != "")
			f.check("Force push (with lease: protects unseen remote commits)", &force)
			f.gap(2)
			f.muted("The local branch and its upstream are unchanged.")
		},
		okDisabled: func() bool {
			return validRefName(strings.TrimSpace(target.String())) != ""
		},
		onOK: func() bool {
			name := strings.TrimSpace(target.String())
			if name == "" {
				return false
			}
			if force {
				w.dialog = nil
				w.confirm("Force Push", "Force-push HEAD to "+remote+"/"+name+"? Commits on the remote branch that are not in HEAD will be removed.", "Force Push", func() {
					w.ws.PushHeadTo(remote, name, true)
				})
				return false
			}
			w.ws.PushHeadTo(remote, name, false)
			return true
		},
	})
}

// openDeleteRemoteTag deletes one tag on the chosen remote. The local tag is
// kept, so the confirmation says so explicitly.
func (w *Window) openDeleteRemoteTag(name string) {
	rs := w.repo()
	if len(rs.Remotes) == 0 {
		w.ws.Notify(vapp.NoticeWarning, "No remote", "Add a remote before deleting remote tags.")
		return
	}
	remote := w.defaultRemote()
	w.openDialog(&dialog{
		title: "Delete Remote Tag", ok: "Delete", width: 480,
		body: func(f *form) {
			f.labelW = 76
			f.muted("Deletes tag '" + name + "' on the remote. The local tag stays.")
			f.gap(8)
			f.combo("Remote", remote, func() []item {
				var items []item
				for _, rm := range rs.Remotes {
					rm := rm
					items = append(items, item{title: rm.Name, hint: rm.FetchURL, checked: rm.Name == remote, run: func() { remote = rm.Name }})
				}
				return items
			})
		},
		onOK: func() bool {
			w.ws.DeleteRemoteTag(remote, name)
			return true
		},
	})
}

// branchOrDash keeps the hint line readable while the branch field is empty.
func branchOrDash(ed *text.Editor) string {
	if s := strings.TrimSpace(ed.String()); s != "" {
		return s
	}
	return "…"
}
