package viffui

import (
	"runtime"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/gitcli"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// lfsInstallHint is the sentence fragment and the copyable command for this
// platform. Viff never runs the command: installing a Git extension is the
// user's decision, so the interface only offers the text (ADR 0031).
func lfsInstallHint() (hint, command string) {
	switch runtime.GOOS {
	case "darwin":
		return "Homebrew: brew install git-lfs", "brew install git-lfs"
	case "windows":
		return "winget install GitHub.GitLFS", "winget install GitHub.GitLFS"
	}
	// Linux distributions disagree on the package manager, so the official
	// page is what the copy action offers; the hint names the usual ones.
	return "your package manager (apt, dnf, pacman) or https://git-lfs.com", "https://git-lfs.com"
}

func (w *Window) lfsStatusLabel(rs *vapp.RepoState) string {
	label := w.t(i18n.ChromeStatusLFS)
	if !rs.LFSStats.UpdatedAt.IsZero() {
		label += " " + itoa(len(rs.LFSStats.Files))
	}
	if state := w.lfsStatsState(rs); state != "" {
		label += " · " + state
	}
	return label
}

func (w *Window) lfsStatsState(rs *vapp.RepoState) string {
	switch rs.LFSStats.Status {
	case vapp.LFSStatsLoading:
		return w.t(i18n.LFSStatsLoading)
	case vapp.LFSStatsStale:
		return w.t(i18n.LFSStatsStale)
	case vapp.LFSStatsFailed:
		return w.t(i18n.LFSStatsFailed)
	}
	return ""
}

// lfsStatusTip explains the marker and names the missing dependency.
func (w *Window) lfsStatusTip(rs *vapp.RepoState) string {
	var lines []string
	if rs.LFS.Installed {
		lines = append(lines, w.t(i18n.LFSRefreshStats))
	}
	if rs.LFS.Managed {
		lines = append(lines, w.t(i18n.ChromeTipLFSRules, len(rs.LFS.Patterns)))
	}
	if rs.LFS.Err != nil {
		lines = append(lines, w.t(i18n.ChromeTipLFSRulesFailed, gitcli.CodeOf(rs.LFS.Err)))
	}
	if state := w.lfsStatsState(rs); state != "" {
		lines = append(lines, state)
	}
	if !rs.LFSStats.UpdatedAt.IsZero() {
		lines = append(lines, w.t(i18n.ChromeTipLFSFiles, len(rs.LFSStats.Files)),
			w.t(i18n.LFSStatsUpdated, rs.LFSStats.UpdatedAt.Local().Format("2006-01-02 "+w.clockLayout())))
	} else if rs.LFSStats.Status == vapp.LFSStatsUnqueried {
		lines = append(lines, w.t(i18n.LFSStatsUnqueried))
	}
	if rs.LFSStats.Code != "" {
		lines = append(lines, w.t(i18n.LFSStatsErrorCode, rs.LFSStats.Code))
		if rs.LFSStats.Code == "lfs.cleanup_unconfirmed" {
			lines = append(lines, w.t(i18n.LFSStatsUnconfirmed))
		}
	}
	if rs.LFS.NeedsInstall() {
		if w.readOnly() {
			lines = append(lines, w.t(i18n.ChromeTipLFSMissingRemote))
		} else {
			lines = append(lines, w.t(i18n.ChromeTipLFSMissing))
		}
	}
	return strings.Join(lines, "\n")
}

func (w *Window) copyLFSFileList() {
	rs := w.repo()
	if rs == nil || rs.LFSStats.Status != vapp.LFSStatsCurrent || len(rs.LFSStats.Files) == 0 {
		return
	}
	w.copyText(strings.Join(rs.LFSStats.Files, "\n"))
}

// paintLFSBanner warns in the Commit panel that LFS files cannot be fetched or
// pushed without git-lfs. It is deliberately small: the full instructions and
// the copy action also arrive as a notification when the repository opens.
func (w *Window) paintLFSBanner(r ui.Rect) ui.Rect {
	rs := w.repo()
	const h = float32(76)
	b := splitTop(&r, h+8)
	br := rect(b.X+8, b.Y+4, b.Width-16, h)
	w.p.outline(br, 8, 1, w.pal.BannerWarnBorder, w.pal.BannerWarn)
	w.p.iconIn(icons.Warning, rect(br.X+8, br.Y+6, 20, 22))
	w.p.text(w.t(i18n.WorkspaceLFSMissingTitle), rect(br.X+32, br.Y+6, br.Width-40, 20), fontUI, semibold, w.pal.Text)
	detail := w.pluralT(len(rs.LFS.Patterns), i18n.WorkspaceLFSRuleOne, i18n.WorkspaceLFSRuleMany)
	w.p.text(detail, rect(br.X+32, br.Y+26, br.Width-40, 16), fontSmall, regular, w.pal.TextMuted)
	label := w.t(i18n.WorkspaceLFSCopyInstall)
	bw := w.buttonWidth(label)
	if w.button(idCmLFSCopy, rect(br.X+32, br.Y+h-30, bw, 26), label, btnNormal, false) {
		w.after(w.copyLFSInstall)
	}
	return r
}

// warnLFSSetup tells the user once per repository that this checkout needs
// git-lfs. onWake calls it after a refresh result arrives, so it never runs on
// the paint path.
func (w *Window) warnLFSSetup() {
	rs := w.ws.Repo
	if rs == nil || w.readOnly() || !rs.LFS.NeedsInstall() || w.lfsWarnedGen == rs.Gen {
		return
	}
	w.lfsWarnedGen = rs.Gen
	hint, _ := lfsInstallHint()
	w.ws.Notify(vapp.NoticeWarning, w.t(i18n.WorkspaceLFSMissingTitle), w.t(i18n.WorkspaceLFSNotifyBody, hint),
		vapp.NoticeAction{Title: w.t(i18n.WorkspaceLFSCopyInstall), Run: w.copyLFSInstall})
}

// copyLFSInstall puts the platform install command on the clipboard and says
// what was copied. A failure reuses the existing copy error path.
func (w *Window) copyLFSInstall() {
	_, command := lfsInstallHint()
	if err := app.SetClipboardText(command); err != nil {
		w.ws.NotifyError(w.t(i18n.WorkspaceErrorCannotCopy), err)
		return
	}
	w.ws.Notify(vapp.NoticeInfo, w.t(i18n.WorkspaceLFSCopiedTitle), w.t(i18n.WorkspaceLFSCopiedBody, command))
}
