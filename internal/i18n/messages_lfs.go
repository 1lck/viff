package i18n

// Message keys of Git LFS detection (ADR 0031). They live in their own file,
// like the other feature modules, so parallel work on the status bar or the
// Commit panel does not merge-conflict inside a shared message literal. Viff
// only detects LFS; the install hint is text the user can copy, never a
// command Viff runs.
const (
	// Status-bar marker and its tooltip.
	ChromeStatusLFS     = Key("chrome.status.lfs")
	ChromeTipLFSFiles   = Key("chrome.tip.lfsFiles")
	ChromeTipLFSRules   = Key("chrome.tip.lfsRules")
	ChromeTipLFSMissing = Key("chrome.tip.lfsMissing")
	// The server's git-lfs is missing; the install command belongs to the
	// server, not to the client this status bar is running on.
	ChromeTipLFSMissingRemote = Key("chrome.tip.lfsMissingRemote")

	// Commit-panel warning and the notification shown once per repository.
	WorkspaceLFSMissingTitle = Key("workspace.lfs.missingTitle")
	WorkspaceLFSRuleOne      = Key("workspace.lfs.ruleOne")
	WorkspaceLFSRuleMany     = Key("workspace.lfs.ruleMany")
	WorkspaceLFSCopyInstall  = Key("workspace.lfs.copyInstall")
	WorkspaceLFSNotifyBody   = Key("workspace.lfs.notifyBody")
	WorkspaceLFSCopiedTitle  = Key("workspace.lfs.copiedTitle")
	WorkspaceLFSCopiedBody   = Key("workspace.lfs.copiedBody")
)

func init() {
	register(EN, map[Key]string{
		ChromeStatusLFS:           "LFS",
		ChromeTipLFSFiles:         "Git LFS: %d managed files",
		ChromeTipLFSRules:         "Git LFS: %d rules",
		ChromeTipLFSMissing:       "git-lfs is not installed",
		ChromeTipLFSMissingRemote: "git-lfs is not installed on the server",

		WorkspaceLFSMissingTitle: "Git LFS is not installed",
		WorkspaceLFSRuleOne:      "%d LFS rule",
		WorkspaceLFSRuleMany:     "%d LFS rules",
		WorkspaceLFSCopyInstall:  "Copy Install Command",
		WorkspaceLFSNotifyBody:   "This repository uses Git LFS, but git-lfs was not found.\nFiles tracked by LFS cannot be downloaded or pushed until it is installed.\nInstall it with %s, then run git lfs install once.",
		WorkspaceLFSCopiedTitle:  "Git LFS",
		WorkspaceLFSCopiedBody:   "Install command copied: %s",
	})

	register(ZHCN, map[Key]string{
		ChromeStatusLFS:           "LFS",
		ChromeTipLFSFiles:         "Git LFS：%d 个受管文件",
		ChromeTipLFSRules:         "Git LFS：%d 条规则",
		ChromeTipLFSMissing:       "未安装 git-lfs",
		ChromeTipLFSMissingRemote: "服务器上未安装 git-lfs",

		WorkspaceLFSMissingTitle: "未安装 Git LFS",
		WorkspaceLFSRuleOne:      "%d 条 LFS 规则",
		WorkspaceLFSRuleMany:     "%d 条 LFS 规则",
		WorkspaceLFSCopyInstall:  "复制安装命令",
		WorkspaceLFSNotifyBody:   "这个仓库使用 Git LFS，但没有找到 git-lfs。\n安装前，受 LFS 管理的文件无法下载或推送。\n可用 %s 安装，然后在终端执行一次 git lfs install。",
		WorkspaceLFSCopiedTitle:  "Git LFS",
		WorkspaceLFSCopiedBody:   "已复制安装命令：%s",
	})
}
