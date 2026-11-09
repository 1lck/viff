package i18n

// Message keys of the welcome page, the help menu, the multi-commit view and
// the repositories window.
const (
	WelcomeGitClient    = Key("welcome.gitClient")
	WelcomeNoProjects   = Key("welcome.noProjects")
	WelcomeOpen         = Key("welcome.open")
	WelcomeCopyPath     = Key("welcome.copyPath")
	WelcomeRemoveRecent = Key("welcome.removeRecent")
	WelcomeDropHint     = Key("welcome.dropHint")
	WelcomeCustomize    = Key("welcome.customize")

	WelcomeSearchName     = Key("welcome.search.name")
	WelcomeSearchPath     = Key("welcome.search.path")
	WelcomeSearchNameHint = Key("welcome.search.nameHint")
	WelcomeSearchPathHint = Key("welcome.search.pathHint")

	HelpReportProblem   = Key("help.reportProblem")
	HelpStopRecording   = Key("help.stopRecording")
	HelpSaveDiagnostics = Key("help.saveDiagnostics")
	HelpOpenLogFolder   = Key("help.openLogFolder")

	MultiNoFileChanges = Key("multi.noFileChanges")
	MultiChooseFile    = Key("multi.chooseFile")
	MultiShowDiffFrom  = Key("multi.showDiffFrom")
	MultiShowHistory   = Key("multi.showHistory")
	MultiCopyPath      = Key("multi.copyPath")

	ReposTitle            = Key("repos.title")
	ReposSwitch           = Key("repos.switch")
	ReposNone             = Key("repos.none")
	ReposActiveOnly       = Key("repos.activeOnly")
	ReposSelectHint       = Key("repos.selectHint")
	ReposCannotReadStatus = Key("repos.cannotReadStatus")
	ReposClean            = Key("repos.clean")
	ReposNoCached         = Key("repos.noCached")
	ReposPathRequired     = Key("repos.pathRequired")
	ReposCannotAdd        = Key("repos.cannotAdd")
	ReposCannotRemove     = Key("repos.cannotRemove")
	ReposChangedFiles     = Key("repos.changedFiles")
	ReposLoadingStatus    = Key("repos.loadingStatus")
	ReposAddTitle         = Key("repos.addTitle")
)

func init() {
	register(EN, map[Key]string{
		WelcomeGitClient:    "Git client",
		WelcomeNoProjects:   "No projects match",
		WelcomeOpen:         "Open",
		WelcomeCopyPath:     "Copy Path",
		WelcomeRemoveRecent: "Remove from Recent Projects",
		WelcomeDropHint:     "You can also drop a repository folder on this window.",
		WelcomeCustomize:    "Customize",

		WelcomeSearchName:     "Name",
		WelcomeSearchPath:     "Path",
		WelcomeSearchNameHint: "Search repository names",
		WelcomeSearchPathHint: "Search full paths",

		HelpReportProblem:   "Report a Problem…",
		HelpStopRecording:   "Stop Recording and Save Diagnostics…",
		HelpSaveDiagnostics: "Save Diagnostics…",
		HelpOpenLogFolder:   "Open Log Folder",

		MultiNoFileChanges: "The selected commits change no files",
		MultiChooseFile:    "Choose a file to view its changes",
		MultiShowDiffFrom:  "Show Diff from %s",
		MultiShowHistory:   "Show History",
		MultiCopyPath:      "Copy Relative Path",

		ReposTitle:            "Repositories",
		ReposSwitch:           "Switch to Repository",
		ReposNone:             "No repositories yet. Type a repository path and choose Add.",
		ReposActiveOnly:       "Only the active repository takes Git actions; stage and commit in the Commit panel.",
		ReposSelectHint:       "Select a repository to see its branch and changes",
		ReposCannotReadStatus: "Cannot read status: %s",
		ReposClean:            "Working tree clean",
		ReposNoCached:         "No cached changes",
		ReposPathRequired:     "Enter the path of a repository, or choose Browse….",
		ReposCannotAdd:        "Cannot add repository: %s",
		ReposCannotRemove:     "Cannot remove repository: %s",
		ReposChangedFiles:     "Changed files",
		ReposLoadingStatus:    "Loading status…",
		ReposAddTitle:         "Add Repository",
	})
	register(ZHCN, map[Key]string{
		WelcomeGitClient:    "Git 客户端",
		WelcomeNoProjects:   "没有匹配的项目",
		WelcomeOpen:         "打开",
		WelcomeCopyPath:     "复制路径",
		WelcomeRemoveRecent: "从最近项目中移除",
		WelcomeDropHint:     "也可以把仓库文件夹拖到这个窗口。",
		WelcomeCustomize:    "自定义",

		WelcomeSearchName:     "名称",
		WelcomeSearchPath:     "路径",
		WelcomeSearchNameHint: "按仓库名称搜索",
		WelcomeSearchPathHint: "按完整路径搜索",

		HelpReportProblem:   "报告问题…",
		HelpStopRecording:   "停止记录并保存诊断…",
		HelpSaveDiagnostics: "保存诊断…",
		HelpOpenLogFolder:   "打开日志文件夹",

		MultiNoFileChanges: "所选提交没有改动任何文件",
		MultiChooseFile:    "选择一个文件查看它的更改",
		MultiShowDiffFrom:  "显示自 %s 起的 diff",
		MultiShowHistory:   "查看历史",
		MultiCopyPath:      "复制相对路径",

		ReposTitle:            "仓库",
		ReposSwitch:           "切换到该仓库",
		ReposNone:             "还没有仓库。输入仓库路径后点「添加」。",
		ReposActiveOnly:       "只有当前活动的仓库会执行 Git 操作；暂存与提交请在提交面板里做。",
		ReposSelectHint:       "选择一个仓库，查看它的分支与更改",
		ReposCannotReadStatus: "无法读取状态：%s",
		ReposClean:            "工作区干净",
		ReposNoCached:         "没有已缓存的更改",
		ReposPathRequired:     "请输入仓库路径，或点「浏览…」。",
		ReposCannotAdd:        "无法添加仓库：%s",
		ReposCannotRemove:     "无法移除仓库：%s",
		ReposChangedFiles:     "更改的文件",
		ReposLoadingStatus:    "正在加载状态…",
		ReposAddTitle:         "添加仓库",
	})
}
