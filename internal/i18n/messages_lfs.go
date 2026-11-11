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
	LFSRefreshStats     = Key("lfs.refreshStats")
	LFSCopyFileList     = Key("lfs.copyFileList")
	LFSStatsUnqueried   = Key("lfs.stats.unqueried")
	LFSStatsLoading     = Key("lfs.stats.loading")
	LFSStatsStale       = Key("lfs.stats.stale")
	LFSStatsFailed      = Key("lfs.stats.failed")
	LFSStatsUpdated     = Key("lfs.stats.updated")
	LFSStatsErrorCode   = Key("lfs.stats.errorCode")
	LFSStatsUnconfirmed = Key("lfs.stats.unconfirmed")
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
		LFSRefreshStats:           "Refresh LFS Statistics",
		LFSCopyFileList:           "Copy LFS File List",
		LFSStatsUnqueried:         "Managed files have not been counted",
		LFSStatsLoading:           "Counting…",
		LFSStatsStale:             "Out of date",
		LFSStatsFailed:            "Count failed",
		LFSStatsUpdated:           "Last counted: %s",
		LFSStatsErrorCode:         "Error code: %s",
		LFSStatsUnconfirmed:       "The query's exit could not be confirmed. Check Git/LFS processes on the repository host, then restart Viff to try again.",

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
		LFSRefreshStats:           "更新 LFS 统计",
		LFSCopyFileList:           "复制 LFS 文件清单",
		LFSStatsUnqueried:         "尚未统计受管文件",
		LFSStatsLoading:           "统计中…",
		LFSStatsStale:             "已过期",
		LFSStatsFailed:            "统计失败",
		LFSStatsUpdated:           "上次统计：%s",
		LFSStatsErrorCode:         "错误码：%s",
		LFSStatsUnconfirmed:       "未收到查询退出确认。请检查仓库所在机器上的 Git/LFS 进程，再重启 Viff 重试。",

		WorkspaceLFSMissingTitle: "未安装 Git LFS",
		WorkspaceLFSRuleOne:      "%d 条 LFS 规则",
		WorkspaceLFSRuleMany:     "%d 条 LFS 规则",
		WorkspaceLFSCopyInstall:  "复制安装命令",
		WorkspaceLFSNotifyBody:   "这个仓库使用 Git LFS，但没有找到 git-lfs。\n安装前，受 LFS 管理的文件无法下载或推送。\n可用 %s 安装，然后在终端执行一次 git lfs install。",
		WorkspaceLFSCopiedTitle:  "Git LFS",
		WorkspaceLFSCopiedBody:   "已复制安装命令：%s",
	})
}
