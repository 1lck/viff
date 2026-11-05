package i18n

// Message keys of the window chrome: the main toolbar, the stripes, the status
// bar, the project and branch menus, the Git tool window's log, branch tree and
// reflog, and the popup menus they open. Terms follow docs/features.zh-CN.md;
// "diff" and the tool window names (Log, Console, Reflog) stay as they are.
const (
	// Toolbar actions (chrome.go).
	ChromeTbSettings = Key("chrome.tb.settings")
	ChromeTbHelp     = Key("chrome.tb.help")
	ChromeTbSearch   = Key("chrome.tb.search")
	ChromeTbPush     = Key("chrome.tb.push")
	ChromeTbCommit   = Key("chrome.tb.commit")
	ChromeTbUpdate   = Key("chrome.tb.update")
	ChromeTbFetch    = Key("chrome.tb.fetch")

	// Side stripes.
	ChromeStripeCommit        = Key("chrome.stripe.commit")
	ChromeStripeProject       = Key("chrome.stripe.project")
	ChromeStripeGit           = Key("chrome.stripe.git")
	ChromeStripeNotifications = Key("chrome.stripe.notifications")

	// Project menu.
	ChromeMenuOpen           = Key("chrome.menu.open")
	ChromeMenuClone          = Key("chrome.menu.clone")
	ChromeMenuNewRepository  = Key("chrome.menu.newRepository")
	ChromeMenuRepositories   = Key("chrome.menu.repositories")
	ChromeMenuRecentProjects = Key("chrome.menu.recentProjects")
	ChromeMenuShowInFinder   = Key("chrome.menu.showInFinder")
	ChromeMenuOpenFile       = Key("chrome.menu.openFile")
	ChromeMenuOpenTerminal   = Key("chrome.menu.openTerminal")
	ChromeMenuCloseProject   = Key("chrome.menu.closeProject")

	// Accessibility names of chrome widgets and generic controls.
	ChromeA11yProject      = Key("chrome.a11y.project")
	ChromeA11yBranch       = Key("chrome.a11y.branch")
	ChromeA11yBranchButton = Key("chrome.a11y.branchButton")
	ChromeA11yScrollBar    = Key("chrome.a11y.scrollBar")
	ChromeA11yResize       = Key("chrome.a11y.resize")

	// Tooltips of the status bar and branch widget.
	ChromeTipDetached    = Key("chrome.tip.detached")
	ChromeTipBranch      = Key("chrome.tip.branch")
	ChromeTipTracking    = Key("chrome.tip.tracking")
	ChromeTipAheadBehind = Key("chrome.tip.aheadBehind")
	ChromeTipRecording   = Key("chrome.tip.recording")

	// Status bar markers.
	ChromeStatusRecording      = Key("chrome.status.recording")
	ChromeStatusRecordingLabel = Key("chrome.status.recordingLabel")

	// Native dialog titles.
	ChromeDialogOpenRepository = Key("chrome.dialog.openRepository")

	// Popup menus (popup.go).
	ChromePopupCloseMenu = Key("chrome.popup.closeMenu")
	ChromePopupSearch    = Key("chrome.popup.search")
	ChromePopupEmpty     = Key("chrome.popup.empty")
	ChromePopupFavorite  = Key("chrome.popup.favorite")
	ChromePopupChanges   = Key("chrome.popup.changes")
	ChromePopupGitBranch = Key("chrome.popup.gitBranches")

	// Git tool window header and shared actions.
	ChromeGitTabLog     = Key("chrome.git.tab.log")
	ChromeGitTabConsole = Key("chrome.git.tab.console")
	ChromeGitTabReflog  = Key("chrome.git.tab.reflog")
	ChromeGitActions    = Key("chrome.git.actions")
	ChromeGitHide       = Key("chrome.git.hide")

	ChromeActionRefresh  = Key("chrome.action.refresh")
	ChromeActionCheckout = Key("chrome.action.checkout")
	ChromeActionDelete   = Key("chrome.action.delete")
	ChromeActionApply    = Key("chrome.action.apply")
	ChromeActionClear    = Key("chrome.action.clear")

	// Log filters.
	ChromeLogPlaceholder   = Key("chrome.log.placeholder")
	ChromeLogFilterBranch  = Key("chrome.log.filter.branch")
	ChromeLogFilterUser    = Key("chrome.log.filter.user")
	ChromeLogFilterDate    = Key("chrome.log.filter.date")
	ChromeLogFilterPaths   = Key("chrome.log.filter.paths")
	ChromeLogFilterAll     = Key("chrome.log.filter.all")
	ChromeLogFilterMe      = Key("chrome.log.filter.me")
	ChromeLogClearFilters  = Key("chrome.log.clearFilters")
	ChromeLogEmpty         = Key("chrome.log.empty")
	ChromeLogEmptyFiltered = Key("chrome.log.emptyFiltered")
	ChromeLogEmptyNoHead   = Key("chrome.log.emptyNoHead")
	ChromeLogLoadingMore   = Key("chrome.log.loadingMore")
	ChromeLogSinceDay      = Key("chrome.log.since.day")
	ChromeLogSinceWeek     = Key("chrome.log.since.week")
	ChromeLogSinceMonth    = Key("chrome.log.since.month")
	ChromeLogSinceYear     = Key("chrome.log.since.year")

	ChromeCommonLoading = Key("chrome.common.loading")

	// Reference tree and log filters.
	ChromeRefsLocal       = Key("chrome.refs.local")
	ChromeRefsRemote      = Key("chrome.refs.remote")
	ChromeRefsTags        = Key("chrome.refs.tags")
	ChromeRefsHeadCurrent = Key("chrome.refs.headCurrent")
	ChromeRefsHeadHash    = Key("chrome.refs.headHash")
	ChromeRefsGoneWord    = Key("chrome.refs.goneWord")
	ChromeRefsGone        = Key("chrome.refs.gone")

	// List accessibility names.
	ChromeListCommits      = Key("chrome.list.commits")
	ChromeListBranches     = Key("chrome.list.branches")
	ChromeListChangedFiles = Key("chrome.list.changedFiles")

	// Commit details panel.
	ChromeDetailsEmpty        = Key("chrome.details.empty")
	ChromeDetailsFilesChanged = Key("chrome.details.filesChanged")

	// Reflog tab.
	ChromeReflogHint  = Key("chrome.reflog.hint")
	ChromeReflogEmpty = Key("chrome.reflog.empty")

	// Dialogs and confirmations opened from the chrome.
	ChromeDialogFilterPath              = Key("chrome.dialog.filterPath")
	ChromeDialogFilterPathHint          = Key("chrome.dialog.filterPathHint")
	ChromeDialogUndoCommit              = Key("chrome.dialog.undoCommit")
	ChromeDialogUndoCommitMessage       = Key("chrome.dialog.undoCommitMessage")
	ChromeDialogCheckoutRevision        = Key("chrome.dialog.checkoutRevision")
	ChromeDialogCheckoutRevisionMessage = Key("chrome.dialog.checkoutRevisionMessage")
	ChromeDialogDeleteTag               = Key("chrome.dialog.deleteTag")
	ChromeDialogDeleteRemoteBranch      = Key("chrome.dialog.deleteRemoteBranch")
	ChromeDialogDeleteRemoteBranchMsg   = Key("chrome.dialog.deleteRemoteBranchMessage")

	// Commit context menu and file rows.
	ChromeMenuCopyRevision      = Key("chrome.menu.copyRevision")
	ChromeMenuCherryPick        = Key("chrome.menu.cherryPick")
	ChromeMenuCheckoutRevision  = Key("chrome.menu.checkoutRevision")
	ChromeMenuShowDiffWorking   = Key("chrome.menu.showDiffWorkingTree")
	ChromeMenuCompareCommits    = Key("chrome.menu.compareCommits")
	ChromeMenuResetHere         = Key("chrome.menu.resetHere")
	ChromeMenuRevert            = Key("chrome.menu.revert")
	ChromeMenuInteractiveRebase = Key("chrome.menu.interactiveRebase")
	ChromeMenuUndoCommit        = Key("chrome.menu.undoCommit")
	ChromeMenuEditCommitMessage = Key("chrome.menu.editCommitMessage")
	ChromeMenuSquashIntoParent  = Key("chrome.menu.squashIntoParent")
	ChromeMenuDropCommit        = Key("chrome.menu.dropCommit")
	ChromeMenuNewBranch         = Key("chrome.menu.newBranch")
	ChromeMenuNewTag            = Key("chrome.menu.newTag")
	ChromeMenuPushUpToCommit    = Key("chrome.menu.pushUpToCommit")
	ChromeMenuBrowseAtRevision  = Key("chrome.menu.browseAtRevision")
	ChromeMenuShowDiff          = Key("chrome.menu.showDiff")
	ChromeMenuOpenRepoVersion   = Key("chrome.menu.openRepositoryVersion")
	ChromeMenuAnnotateRevision  = Key("chrome.menu.annotateRevision")
	ChromeMenuShowHistory       = Key("chrome.menu.showHistory")
	ChromeMenuCopyRelativePath  = Key("chrome.menu.copyRelativePath")

	// Branch popup and reference actions.
	ChromeBranchContinue       = Key("chrome.branch.continue")
	ChromeBranchSkip           = Key("chrome.branch.skip")
	ChromeBranchAbort          = Key("chrome.branch.abort")
	ChromeBranchInWorktree     = Key("chrome.branch.inWorktree")
	ChromeMenuCheckoutTagRev   = Key("chrome.menu.checkoutTagOrRevision")
	ChromeMenuWorktrees        = Key("chrome.menu.worktrees")
	ChromeMenuPushTagTo        = Key("chrome.menu.pushTagTo")
	ChromeMenuMergeInto        = Key("chrome.menu.mergeInto")
	ChromeMenuNewBranchFrom    = Key("chrome.menu.newBranchFrom")
	ChromeMenuDeleteTag        = Key("chrome.menu.deleteTag")
	ChromeMenuDeleteRemoteTag  = Key("chrome.menu.deleteRemoteTag")
	ChromeMenuUpdate           = Key("chrome.menu.update")
	ChromeMenuPushAnother      = Key("chrome.menu.pushAnotherRemote")
	ChromeMenuRename           = Key("chrome.menu.rename")
	ChromeMenuSetUpstream      = Key("chrome.menu.setUpstream")
	ChromeMenuCheckoutRebase   = Key("chrome.menu.checkoutAndRebaseOnto")
	ChromeMenuCompareWith      = Key("chrome.menu.compareWith")
	ChromeMenuShowChanged      = Key("chrome.menu.showChangedFiles")
	ChromeMenuRebaseOnto       = Key("chrome.menu.rebaseOnto")
	ChromeMenuUpdateFromRemote = Key("chrome.menu.updateFromRemote")

	// Notifications raised by these panels.
	ChromeNotifyCannotCompare            = Key("chrome.notify.cannotCompare")
	ChromeNotifyNoDifferences            = Key("chrome.notify.noDifferences")
	ChromeNotifyNoDifferencesMessage     = Key("chrome.notify.noDifferencesMessage")
	ChromeNotifyCannotReadReflog         = Key("chrome.notify.cannotReadReflog")
	ChromeNotifyComparingBranches        = Key("chrome.notify.comparingBranches")
	ChromeNotifyComparingBranchesMessage = Key("chrome.notify.comparingBranchesMessage")
)

func init() {
	register(EN, map[Key]string{
		ChromeTbSettings: "Settings",
		ChromeTbHelp:     "Help",
		ChromeTbSearch:   "Search Commits",
		ChromeTbPush:     "Push…",
		ChromeTbCommit:   "Commit…",
		ChromeTbUpdate:   "Update Project…",
		ChromeTbFetch:    "Fetch",

		ChromeStripeCommit:        "Commit",
		ChromeStripeProject:       "Project",
		ChromeStripeGit:           "Git",
		ChromeStripeNotifications: "Notifications",

		ChromeMenuOpen:           "Open…",
		ChromeMenuClone:          "Clone Repository…",
		ChromeMenuNewRepository:  "New Repository…",
		ChromeMenuRepositories:   "Repositories…",
		ChromeMenuRecentProjects: "Recent Projects",
		ChromeMenuShowInFinder:   "Show in Finder",
		ChromeMenuOpenFile:       "Open File…",
		ChromeMenuOpenTerminal:   "Open in Terminal",
		ChromeMenuCloseProject:   "Close Project",

		ChromeA11yProject:      "Project: %s",
		ChromeA11yBranch:       "Branch: %s",
		ChromeA11yBranchButton: "Branch",
		ChromeA11yScrollBar:    "Scroll bar",
		ChromeA11yResize:       "Resize",

		ChromeTipDetached:    "Detached HEAD at %s",
		ChromeTipBranch:      "Git Branch: %s",
		ChromeTipTracking:    "Tracking %s",
		ChromeTipAheadBehind: " (↑%s ↓%s)",
		ChromeTipRecording:   "Recording diagnostics — click to stop and save",

		ChromeStatusRecording:      "● Recording diagnostics…",
		ChromeStatusRecordingLabel: "Recording diagnostics",

		ChromeDialogOpenRepository: "Open Git Repository",

		ChromePopupCloseMenu: "Close menu",
		ChromePopupSearch:    "Search",
		ChromePopupEmpty:     "Nothing found",
		ChromePopupFavorite:  "Favorite",
		ChromePopupChanges:   "Changes",
		ChromePopupGitBranch: "Git Branches",

		ChromeGitTabLog:     "Log",
		ChromeGitTabConsole: "Console",
		ChromeGitTabReflog:  "Reflog",
		ChromeGitActions:    "Git Actions",
		ChromeGitHide:       "Hide",

		ChromeActionRefresh:  "Refresh",
		ChromeActionCheckout: "Checkout",
		ChromeActionDelete:   "Delete",
		ChromeActionApply:    "Apply",
		ChromeActionClear:    "Clear",

		ChromeLogPlaceholder:   "Text or hash",
		ChromeLogFilterBranch:  "Branch",
		ChromeLogFilterUser:    "User",
		ChromeLogFilterDate:    "Date",
		ChromeLogFilterPaths:   "Paths",
		ChromeLogFilterAll:     "All",
		ChromeLogFilterMe:      "me",
		ChromeLogClearFilters:  "Clear Filters",
		ChromeLogEmpty:         "No commits",
		ChromeLogEmptyFiltered: "No commits matching filters",
		ChromeLogEmptyNoHead:   "No commits yet",
		ChromeLogLoadingMore:   "Loading more…",
		ChromeLogSinceDay:      "Last 24 hours",
		ChromeLogSinceWeek:     "Last 7 days",
		ChromeLogSinceMonth:    "Last 30 days",
		ChromeLogSinceYear:     "Last year",

		ChromeCommonLoading: "Loading…",

		ChromeRefsLocal:       "Local",
		ChromeRefsRemote:      "Remote",
		ChromeRefsTags:        "Tags",
		ChromeRefsHeadCurrent: "HEAD (Current Branch)",
		ChromeRefsHeadHash:    "HEAD (%s)",
		ChromeRefsGoneWord:    "gone",
		ChromeRefsGone:        " (gone)",

		ChromeListCommits:      "Commits",
		ChromeListBranches:     "Branches",
		ChromeListChangedFiles: "Changed files",

		ChromeDetailsEmpty:        "Select a commit to see its changes",
		ChromeDetailsFilesChanged: "Files changed: %d",

		ChromeReflogHint:  "Where HEAD has been. Use it to find commits after a reset or rebase.",
		ChromeReflogEmpty: "No reflog entries",

		ChromeDialogFilterPath:              "Filter by Path",
		ChromeDialogFilterPathHint:          "Show only commits that change files under this path (relative to the repository root).",
		ChromeDialogUndoCommit:              "Undo Commit",
		ChromeDialogUndoCommitMessage:       "Undo the last commit '%s'? Its changes stay staged so you can commit them again.",
		ChromeDialogCheckoutRevision:        "Checkout Revision",
		ChromeDialogCheckoutRevisionMessage: "Check out %s in detached HEAD state? Commits made there belong to no branch until you create one.",
		ChromeDialogDeleteTag:               "Delete tag %s locally?",
		ChromeDialogDeleteRemoteBranch:      "Delete Remote Branch",
		ChromeDialogDeleteRemoteBranchMsg:   "Delete %s on %s? Everyone who fetches will lose this branch.",

		ChromeMenuCopyRevision:      "Copy Revision Number",
		ChromeMenuCherryPick:        "Cherry-Pick",
		ChromeMenuCheckoutRevision:  "Checkout Revision '%s'",
		ChromeMenuShowDiffWorking:   "Show Diff with Working Tree",
		ChromeMenuCompareCommits:    "Compare Commits",
		ChromeMenuResetHere:         "Reset Current Branch to Here…",
		ChromeMenuRevert:            "Revert Commit",
		ChromeMenuInteractiveRebase: "Interactively Rebase from Here…",
		ChromeMenuUndoCommit:        "Undo Commit…",
		ChromeMenuEditCommitMessage: "Edit Commit Message…",
		ChromeMenuSquashIntoParent:  "Squash into Parent…",
		ChromeMenuDropCommit:        "Drop Commit…",
		ChromeMenuNewBranch:         "New Branch…",
		ChromeMenuNewTag:            "New Tag…",
		ChromeMenuPushUpToCommit:    "Push Up to Commit…",
		ChromeMenuBrowseAtRevision:  "Browse Repository at Revision…",
		ChromeMenuShowDiff:          "Show Diff",
		ChromeMenuOpenRepoVersion:   "Open Repository Version",
		ChromeMenuAnnotateRevision:  "Annotate Revision",
		ChromeMenuShowHistory:       "Show History",
		ChromeMenuCopyRelativePath:  "Copy Relative Path",

		ChromeBranchContinue:       "Continue %s",
		ChromeBranchSkip:           "Skip Commit",
		ChromeBranchAbort:          "Abort %s",
		ChromeBranchInWorktree:     "in %s",
		ChromeMenuCheckoutTagRev:   "Checkout Tag or Revision…",
		ChromeMenuWorktrees:        "Worktrees…",
		ChromeMenuPushTagTo:        "Push Tag to %s",
		ChromeMenuMergeInto:        "Merge %s into %s",
		ChromeMenuNewBranchFrom:    "New Branch from %s…",
		ChromeMenuDeleteTag:        "Delete Tag",
		ChromeMenuDeleteRemoteTag:  "Delete Remote Tag…",
		ChromeMenuUpdate:           "Update",
		ChromeMenuPushAnother:      "Push to Another Remote…",
		ChromeMenuRename:           "Rename…",
		ChromeMenuSetUpstream:      "Set Upstream…",
		ChromeMenuCheckoutRebase:   "Checkout and Rebase onto %s",
		ChromeMenuCompareWith:      "Compare with %s",
		ChromeMenuShowChanged:      "Show Changed Files",
		ChromeMenuRebaseOnto:       "Rebase %s onto %s",
		ChromeMenuUpdateFromRemote: "Update from Remote",

		ChromeNotifyCannotCompare:            "Cannot compare",
		ChromeNotifyNoDifferences:            "No differences",
		ChromeNotifyNoDifferencesMessage:     "The working tree matches %s.",
		ChromeNotifyCannotReadReflog:         "Cannot read the reflog",
		ChromeNotifyComparingBranches:        "Comparing branches",
		ChromeNotifyComparingBranchesMessage: "The log shows commits in '%s' that are not in '%s'. Clear the Branch filter to return.",
	})

	register(ZHCN, map[Key]string{
		ChromeTbSettings: "设置",
		ChromeTbHelp:     "帮助",
		ChromeTbSearch:   "搜索提交",
		ChromeTbPush:     "推送…",
		ChromeTbCommit:   "提交…",
		ChromeTbUpdate:   "更新项目…",
		ChromeTbFetch:    "抓取",

		ChromeStripeCommit:        "提交",
		ChromeStripeProject:       "项目",
		ChromeStripeGit:           "Git",
		ChromeStripeNotifications: "通知",

		ChromeMenuOpen:           "打开…",
		ChromeMenuClone:          "克隆仓库…",
		ChromeMenuNewRepository:  "新建仓库…",
		ChromeMenuRepositories:   "仓库…",
		ChromeMenuRecentProjects: "最近项目",
		ChromeMenuShowInFinder:   "在文件管理器中显示",
		ChromeMenuOpenFile:       "打开文件…",
		ChromeMenuOpenTerminal:   "在终端中打开",
		ChromeMenuCloseProject:   "关闭项目",

		ChromeA11yProject:      "项目：%s",
		ChromeA11yBranch:       "分支：%s",
		ChromeA11yBranchButton: "分支",
		ChromeA11yScrollBar:    "滚动条",
		ChromeA11yResize:       "调整大小",

		ChromeTipDetached:    "游离 HEAD 位于 %s",
		ChromeTipBranch:      "Git 分支：%s",
		ChromeTipTracking:    "跟踪 %s",
		ChromeTipAheadBehind: "（↑%s ↓%s）",
		ChromeTipRecording:   "正在记录诊断信息 — 点击停止并保存",

		ChromeStatusRecording:      "● 正在记录诊断信息…",
		ChromeStatusRecordingLabel: "正在记录诊断信息",

		ChromeDialogOpenRepository: "打开 Git 仓库",

		ChromePopupCloseMenu: "关闭菜单",
		ChromePopupSearch:    "搜索",
		ChromePopupEmpty:     "没有匹配项",
		ChromePopupFavorite:  "收藏",
		ChromePopupChanges:   "更改",
		ChromePopupGitBranch: "Git 分支",

		ChromeGitTabLog:     "Log",
		ChromeGitTabConsole: "Console",
		ChromeGitTabReflog:  "Reflog",
		ChromeGitActions:    "Git 操作",
		ChromeGitHide:       "隐藏",

		ChromeActionRefresh:  "刷新",
		ChromeActionCheckout: "检出",
		ChromeActionDelete:   "删除",
		ChromeActionApply:    "应用",
		ChromeActionClear:    "清除",

		ChromeLogPlaceholder:   "文本或哈希",
		ChromeLogFilterBranch:  "分支",
		ChromeLogFilterUser:    "用户",
		ChromeLogFilterDate:    "日期",
		ChromeLogFilterPaths:   "路径",
		ChromeLogFilterAll:     "全部",
		ChromeLogFilterMe:      "我",
		ChromeLogClearFilters:  "清除筛选",
		ChromeLogEmpty:         "没有提交",
		ChromeLogEmptyFiltered: "没有符合筛选的提交",
		ChromeLogEmptyNoHead:   "还没有提交",
		ChromeLogLoadingMore:   "正在加载更多…",
		ChromeLogSinceDay:      "最近 24 小时",
		ChromeLogSinceWeek:     "最近 7 天",
		ChromeLogSinceMonth:    "最近 30 天",
		ChromeLogSinceYear:     "最近一年",

		ChromeCommonLoading: "加载中…",

		ChromeRefsLocal:       "本地",
		ChromeRefsRemote:      "远程",
		ChromeRefsTags:        "标签",
		ChromeRefsHeadCurrent: "HEAD（当前分支）",
		ChromeRefsHeadHash:    "HEAD（%s）",
		ChromeRefsGoneWord:    "已失效",
		ChromeRefsGone:        "（已失效）",

		ChromeListCommits:      "提交",
		ChromeListBranches:     "分支",
		ChromeListChangedFiles: "已更改的文件",

		ChromeDetailsEmpty:        "选择一个提交以查看它的改动",
		ChromeDetailsFilesChanged: "已更改文件：%d",

		ChromeReflogHint:  "HEAD 曾经的位置。reset 或 rebase 之后可以用它找回提交。",
		ChromeReflogEmpty: "没有 reflog 记录",

		ChromeDialogFilterPath:              "按路径筛选",
		ChromeDialogFilterPathHint:          "只显示改动此路径下文件的提交（路径相对于仓库根目录）。",
		ChromeDialogUndoCommit:              "撤销提交",
		ChromeDialogUndoCommitMessage:       "撤销最后一次提交 '%s'？其中的改动会保持已暂存，方便你再次提交。",
		ChromeDialogCheckoutRevision:        "检出修订",
		ChromeDialogCheckoutRevisionMessage: "以游离 HEAD 状态检出 %s？在那里做的提交不属于任何分支，除非你新建一个。",
		ChromeDialogDeleteTag:               "从本地删除标签 %s？",
		ChromeDialogDeleteRemoteBranch:      "删除远端分支",
		ChromeDialogDeleteRemoteBranchMsg:   "删除 %s（位于 %s）？所有抓取的人都会失去这个分支。",

		ChromeMenuCopyRevision:      "复制修订号",
		ChromeMenuCherryPick:        "Cherry-Pick",
		ChromeMenuCheckoutRevision:  "检出修订 '%s'",
		ChromeMenuShowDiffWorking:   "与工作树比较 diff",
		ChromeMenuCompareCommits:    "比较提交",
		ChromeMenuResetHere:         "将当前分支重置到此处…",
		ChromeMenuRevert:            "还原提交",
		ChromeMenuInteractiveRebase: "从此处交互式变基…",
		ChromeMenuUndoCommit:        "撤销提交…",
		ChromeMenuEditCommitMessage: "编辑提交信息…",
		ChromeMenuSquashIntoParent:  "压缩进父提交…",
		ChromeMenuDropCommit:        "删除提交…",
		ChromeMenuNewBranch:         "新建分支…",
		ChromeMenuNewTag:            "新建标签…",
		ChromeMenuPushUpToCommit:    "推送至该提交…",
		ChromeMenuBrowseAtRevision:  "浏览此修订的仓库树…",
		ChromeMenuShowDiff:          "显示 diff",
		ChromeMenuOpenRepoVersion:   "打开仓库中的版本",
		ChromeMenuAnnotateRevision:  "注解修订",
		ChromeMenuShowHistory:       "显示历史",
		ChromeMenuCopyRelativePath:  "复制相对路径",

		ChromeBranchContinue:       "继续 %s",
		ChromeBranchSkip:           "跳过提交",
		ChromeBranchAbort:          "中止 %s",
		ChromeBranchInWorktree:     "位于 %s",
		ChromeMenuCheckoutTagRev:   "检出标签或修订…",
		ChromeMenuWorktrees:        "工作树…",
		ChromeMenuPushTagTo:        "推送标签到 %s",
		ChromeMenuMergeInto:        "把 %s 合并到 %s",
		ChromeMenuNewBranchFrom:    "从 %s 新建分支…",
		ChromeMenuDeleteTag:        "删除标签",
		ChromeMenuDeleteRemoteTag:  "删除远端标签…",
		ChromeMenuUpdate:           "更新",
		ChromeMenuPushAnother:      "推送到其他远端…",
		ChromeMenuRename:           "重命名…",
		ChromeMenuSetUpstream:      "设置上游…",
		ChromeMenuCheckoutRebase:   "检出并变基到 %s",
		ChromeMenuCompareWith:      "与 %s 比较",
		ChromeMenuShowChanged:      "显示更改的文件",
		ChromeMenuRebaseOnto:       "把 %s 变基到 %s",
		ChromeMenuUpdateFromRemote: "从远端更新",

		ChromeNotifyCannotCompare:            "无法比较",
		ChromeNotifyNoDifferences:            "没有差异",
		ChromeNotifyNoDifferencesMessage:     "工作树与 %s 一致。",
		ChromeNotifyCannotReadReflog:         "无法读取 reflog",
		ChromeNotifyComparingBranches:        "正在比较分支",
		ChromeNotifyComparingBranchesMessage: "Log 只显示在 '%s' 中而不在 '%s' 中的提交。清除分支筛选即可返回。",
	})
}
