package i18n

// Message keys of the diagnostics flows and the update dialog.
const (
	DiagTitle = Key("diag.title")

	DiagNoticeRecording = Key("diag.notice.recording")
	DiagNoticeStarted   = Key("diag.notice.started")

	DiagReportTitle       = Key("diag.report.title")
	DiagReportStart       = Key("diag.report.start")
	DiagReportLabel       = Key("diag.report.label")
	DiagReportPlaceholder = Key("diag.report.placeholder")
	DiagReportBody        = Key("diag.report.body")
	DiagReportPrivacy     = Key("diag.report.privacy")
	DiagReportHint        = Key("diag.report.hint")

	DiagSaveTitle     = Key("diag.save.title")
	DiagSaveFilterZip = Key("diag.save.filterZip")

	DiagErrorStart      = Key("diag.error.start")
	DiagErrorChoosePath = Key("diag.error.choosePath")
	DiagErrorSave       = Key("diag.error.save")

	DiagSavedTitle  = Key("diag.saved.title")
	DiagSavedBody   = Key("diag.saved.body")
	DiagSavedCopy   = Key("diag.saved.copyTemplate")
	DiagLogsTitle   = Key("diag.logs.title")
	DiagLogsNone    = Key("diag.logs.none")
	DiagErrorLogDir = Key("diag.error.openLogFolder")

	UpdateTitle = Key("update.title")

	UpdateButtonClose = Key("update.button.close")

	UpdateActionUpdate        = Key("update.action.update")
	UpdateActionDownloading   = Key("update.action.downloading")
	UpdateActionRestart       = Key("update.action.restart")
	UpdateActionInstalling    = Key("update.action.installing")
	UpdateActionShowDownload  = Key("update.action.showDownload")
	UpdateActionRetryInstall  = Key("update.action.retryInstall")
	UpdateActionRetryDownload = Key("update.action.retryDownload")
	UpdateActionCheckAgain    = Key("update.action.checkAgain")
	UpdateActionChecking      = Key("update.action.checking")

	UpdateNoticeChecking       = Key("update.notice.checking")
	UpdateNoticeUpToDate       = Key("update.notice.upToDate")
	UpdateNoticeNewest         = Key("update.notice.newest")
	UpdateNoticeAvailable      = Key("update.notice.available")
	UpdateNoticeYouHave        = Key("update.notice.youHave")
	UpdateNoticeDownloading    = Key("update.notice.downloading")
	UpdateNoticeProgress       = Key("update.notice.progress")
	UpdateNoticeDownloaded     = Key("update.notice.downloaded")
	UpdateNoticeReady          = Key("update.notice.ready")
	UpdateNoticeRestartHint    = Key("update.notice.restartHint")
	UpdateNoticeInstalling     = Key("update.notice.installing")
	UpdateNoticeInstallHint    = Key("update.notice.installHint")
	UpdateNoticeManualDownload = Key("update.notice.manualDownloaded")
	UpdateNoticeManualHint     = Key("update.notice.manualHint")
	UpdateNoticeFailed         = Key("update.notice.failed")
	UpdateNoticeFailedHint     = Key("update.notice.failedHint")
	UpdateNoticeDefault        = Key("update.notice.default")

	UpdateLabelReleaseNotes = Key("update.label.releaseNotes")
	UpdateLabelPackage      = Key("update.label.package")
)

func init() {
	register(EN, map[Key]string{
		DiagTitle: "Diagnostics",

		DiagNoticeRecording: "A problem is already being recorded. Reproduce it, then use Help > Stop Recording and Save Diagnostics…",
		DiagNoticeStarted:   "Recording started. Reproduce the problem, then use Help > Stop Recording and Save Diagnostics…",

		DiagReportTitle:       "Report a Problem",
		DiagReportStart:       "Start Recording",
		DiagReportLabel:       "What happened?",
		DiagReportPlaceholder: "What were you doing when the problem appeared? (optional)",
		DiagReportBody:        "Start a detailed diagnostics recording, reproduce the problem, then save a bundle to attach to your report.",
		DiagReportPrivacy:     "Diagnostics stay on this computer unless you save and share them. No repository contents are included.",
		DiagReportHint:        "The description is optional. Recording keeps going until you stop it from the Help menu.",

		DiagSaveTitle:     "Save Diagnostics",
		DiagSaveFilterZip: "Zip archives",

		DiagErrorStart:      "Cannot start diagnostics recording",
		DiagErrorChoosePath: "Cannot choose where to save diagnostics",
		DiagErrorSave:       "Cannot save diagnostics",

		DiagSavedTitle:  "Diagnostics saved",
		DiagSavedBody:   "Attach this file to your report.\n%s",
		DiagSavedCopy:   "Copy Issue Template",
		DiagLogsTitle:   "Logs",
		DiagLogsNone:    "This build writes no log files, so there is no log folder to open.",
		DiagErrorLogDir: "Cannot open log folder",

		UpdateTitle: "Update Viff",

		UpdateButtonClose: "Close",

		UpdateActionUpdate:        "Update",
		UpdateActionDownloading:   "Downloading…",
		UpdateActionRestart:       "Restart to Update",
		UpdateActionInstalling:    "Installing…",
		UpdateActionShowDownload:  "Show Download",
		UpdateActionRetryInstall:  "Retry Install",
		UpdateActionRetryDownload: "Retry Download",
		UpdateActionCheckAgain:    "Check Again",
		UpdateActionChecking:      "Checking…",

		UpdateNoticeChecking:       "Checking for updates…",
		UpdateNoticeUpToDate:       "Viff is up to date.",
		UpdateNoticeNewest:         "Version %s is the newest release.",
		UpdateNoticeAvailable:      "Viff %s is available.",
		UpdateNoticeYouHave:        "You have %s.",
		UpdateNoticeDownloading:    "Downloading Viff %s…",
		UpdateNoticeProgress:       "%d%% · %s of %s",
		UpdateNoticeDownloaded:     "%s downloaded",
		UpdateNoticeReady:          "Viff %s has been downloaded.",
		UpdateNoticeRestartHint:    "Restart Viff to finish the update. Your repositories and settings are kept.",
		UpdateNoticeInstalling:     "Installing update…",
		UpdateNoticeInstallHint:    "Viff will close and start the new version automatically.",
		UpdateNoticeManualDownload: "Viff %s was downloaded.",
		UpdateNoticeManualHint:     "Install it with your package manager, then start Viff again.",
		UpdateNoticeFailed:         "Update failed.",
		UpdateNoticeFailedHint:     "You can retry, or download the release from the project page.",
		UpdateNoticeDefault:        "Check for updates",

		UpdateLabelReleaseNotes: "Release notes",
		UpdateLabelPackage:      "Package: %s",
	})

	register(ZHCN, map[Key]string{
		DiagTitle: "诊断",

		DiagNoticeRecording: "已经在记录问题。请复现问题，然后用 帮助 > 停止记录并保存诊断…",
		DiagNoticeStarted:   "已开始记录。请复现问题，然后用 帮助 > 停止记录并保存诊断…",

		DiagReportTitle:       "报告问题",
		DiagReportStart:       "开始记录",
		DiagReportLabel:       "发生了什么？",
		DiagReportPlaceholder: "问题出现时你在做什么？（可选）",
		DiagReportBody:        "开始详细的诊断记录，复现问题，然后保存诊断包附在你的报告中。",
		DiagReportPrivacy:     "除非你保存并分享，诊断信息只留在本机。诊断包不包含仓库内容。",
		DiagReportHint:        "描述是可选的。记录会一直进行，直到你在帮助菜单里停止它。",

		DiagSaveTitle:     "保存诊断",
		DiagSaveFilterZip: "Zip 压缩包",

		DiagErrorStart:      "无法开始诊断记录",
		DiagErrorChoosePath: "无法选择诊断的保存位置",
		DiagErrorSave:       "无法保存诊断",

		DiagSavedTitle:  "诊断已保存",
		DiagSavedBody:   "请将此文件附在你的报告中。\n%s",
		DiagSavedCopy:   "复制问题模板",
		DiagLogsTitle:   "日志",
		DiagLogsNone:    "此版本不写日志文件，因此没有可打开的日志文件夹。",
		DiagErrorLogDir: "无法打开日志文件夹",

		UpdateTitle: "更新 Viff",

		UpdateButtonClose: "关闭",

		UpdateActionUpdate:        "更新",
		UpdateActionDownloading:   "正在下载…",
		UpdateActionRestart:       "重启以更新",
		UpdateActionInstalling:    "正在安装…",
		UpdateActionShowDownload:  "显示下载包",
		UpdateActionRetryInstall:  "重试安装",
		UpdateActionRetryDownload: "重试下载",
		UpdateActionCheckAgain:    "重新检查",
		UpdateActionChecking:      "正在检查…",

		UpdateNoticeChecking:       "正在检查更新…",
		UpdateNoticeUpToDate:       "Viff 已是最新版本。",
		UpdateNoticeNewest:         "版本 %s 是最新发布。",
		UpdateNoticeAvailable:      "Viff %s 已发布。",
		UpdateNoticeYouHave:        "你当前是 %s。",
		UpdateNoticeDownloading:    "正在下载 Viff %s…",
		UpdateNoticeProgress:       "%d%% · %s / %s",
		UpdateNoticeDownloaded:     "已下载 %s",
		UpdateNoticeReady:          "Viff %s 已下载完成。",
		UpdateNoticeRestartHint:    "重启 Viff 即可完成更新。仓库与设置都会保留。",
		UpdateNoticeInstalling:     "正在安装更新…",
		UpdateNoticeInstallHint:    "Viff 将自动关闭并启动新版本。",
		UpdateNoticeManualDownload: "Viff %s 已下载。",
		UpdateNoticeManualHint:     "请用包管理器安装，然后重新启动 Viff。",
		UpdateNoticeFailed:         "更新失败。",
		UpdateNoticeFailedHint:     "你可以重试，或从项目主页下载该发布。",
		UpdateNoticeDefault:        "检查更新",

		UpdateLabelReleaseNotes: "发布说明",
		UpdateLabelPackage:      "安装包：%s",
	})
}
