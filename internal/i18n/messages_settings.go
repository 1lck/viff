package i18n

// Message keys of the settings dialog. Later stages add their own file beside
// this one; each file registers its own map from an init, so the files never
// share a literal.
const (
	SettingsTitle = Key("settings.title")

	SettingsTabGeneral = Key("settings.tab.general")
	SettingsTabGit     = Key("settings.tab.git")
	SettingsTabAI      = Key("settings.tab.ai")

	SettingsGroupAppearance  = Key("settings.group.appearance")
	SettingsGroupGitExec     = Key("settings.group.gitExec")
	SettingsGroupTools       = Key("settings.group.tools")
	SettingsGroupDiagnostics = Key("settings.group.diagnostics")
	SettingsGroupUpdates     = Key("settings.group.updates")
	SettingsGroupFonts       = Key("settings.group.fonts")
	SettingsGroupCommands    = Key("settings.group.commands")
	SettingsGroupRemote      = Key("settings.group.remote")

	SettingsLabelTheme        = Key("settings.label.theme")
	SettingsLabelFontSize     = Key("settings.label.fontSize")
	SettingsLabelGitPath      = Key("settings.label.gitPath")
	SettingsLabelEditor       = Key("settings.label.editor")
	SettingsLabelLanguage     = Key("settings.label.language")
	SettingsLabelFetchMins    = Key("settings.label.fetchMinutes")
	SettingsLabelUpdateMode   = Key("settings.label.updateMethod")
	SettingsLabelInterface    = Key("settings.label.interfaceFont")
	SettingsLabelDiffFont     = Key("settings.label.diffFont")
	SettingsLabelSettingsFile = Key("settings.label.settingsFile")

	SettingsHintEditor    = Key("settings.hint.editor")
	SettingsHintDebugLog  = Key("settings.hint.debugLog")
	SettingsHintInterface = Key("settings.hint.interfaceFont")
	SettingsHintDiffFont  = Key("settings.hint.diffFont")

	SettingsCheckDebugLog   = Key("settings.check.debugLog")
	SettingsCheckAutoUpdate = Key("settings.check.autoUpdate")
	SettingsCheckPrune      = Key("settings.check.prune")
	SettingsCheckAutoFetch  = Key("settings.check.autoFetch")

	SettingsThemeDark   = Key("settings.theme.dark")
	SettingsThemeLight  = Key("settings.theme.light")
	SettingsThemeSystem = Key("settings.theme.system")

	SettingsPullMerge  = Key("settings.pull.merge")
	SettingsPullRebase = Key("settings.pull.rebase")
	SettingsPullFFOnly = Key("settings.pull.ffOnly")

	SettingsFontDefaultInterface = Key("settings.font.defaultInterface")
	SettingsFontDefaultDiff      = Key("settings.font.defaultDiff")

	SettingsLangEnglish = Key("settings.lang.english")
	SettingsLangChinese = Key("settings.lang.chinese")

	SettingsPlaceholderAuto  = Key("settings.placeholder.auto")
	SettingsPlaceholderTheme = Key("settings.placeholder.theme")

	CommonOK     = Key("common.ok")
	CommonCancel = Key("common.cancel")
	CommonClose  = Key("common.close")

	AboutTitle   = Key("about.title")
	AboutTagline = Key("about.tagline")
	AboutCredits = Key("about.credits")
)

func init() {
	register(EN, map[Key]string{
		SettingsTitle: "Settings",

		SettingsTabGeneral: "General",
		SettingsTabGit:     "Git",
		SettingsTabAI:      "AI",

		SettingsGroupAppearance:  "Appearance",
		SettingsGroupGitExec:     "Git executable",
		SettingsGroupTools:       "Tools",
		SettingsGroupDiagnostics: "Diagnostics",
		SettingsGroupUpdates:     "Updates",
		SettingsGroupFonts:       "Fonts",
		SettingsGroupCommands:    "Commands and signing",
		SettingsGroupRemote:      "Remote",

		SettingsLabelTheme:        "Theme:",
		SettingsLabelFontSize:     "Editor font size:",
		SettingsLabelGitPath:      "Path to Git:",
		SettingsLabelEditor:       "External editor command:",
		SettingsLabelLanguage:     "Language:",
		SettingsLabelFetchMins:    "Every (minutes):",
		SettingsLabelUpdateMode:   "Update method:",
		SettingsLabelInterface:    "Interface font:",
		SettingsLabelDiffFont:     "Diff font:",
		SettingsLabelSettingsFile: "Settings file: %s",

		SettingsHintEditor:    "Empty: system default. {file} and {line} are filled in.",
		SettingsHintDebugLog:  "Writes detailed events for troubleshooting. Logs stay on this computer.",
		SettingsHintInterface: "Replaces the font of the whole interface. Diffs and the console keep a fixed-pitch face.",
		SettingsHintDiffFont:  "Only fixed-pitch families: diffs, console and hashes lay their text out on a character grid.",

		SettingsCheckDebugLog:   "Debug logging",
		SettingsCheckAutoUpdate: "Check for updates at startup",
		SettingsCheckPrune:      "Prune remote branches on fetch",
		SettingsCheckAutoFetch:  "Fetch in the background",

		SettingsThemeDark:   "Islands Dark",
		SettingsThemeLight:  "Islands Light",
		SettingsThemeSystem: "Sync with OS",

		SettingsPullMerge:  "Merge",
		SettingsPullRebase: "Rebase",
		SettingsPullFFOnly: "Fast-forward only",

		SettingsFontDefaultInterface: "Default (system interface font)",
		SettingsFontDefaultDiff:      "Default (bundled or system fixed-pitch)",

		SettingsLangEnglish: "English",
		SettingsLangChinese: "简体中文",

		SettingsPlaceholderAuto:  "Detected automatically",
		SettingsPlaceholderTheme: "Islands Dark",

		CommonOK:     "OK",
		CommonCancel: "Cancel",
		CommonClose:  "Close",

		AboutTitle:   "About Viff",
		AboutTagline: "A Git client in the style of IntelliJ IDEA",
		AboutCredits: "Icons: IntelliJ Platform (Apache 2.0). Editor font: JetBrains Mono (OFL).",
	})

	register(ZHCN, map[Key]string{
		SettingsTitle: "设置",

		SettingsTabGeneral: "常规",
		SettingsTabGit:     "Git",
		SettingsTabAI:      "AI",

		SettingsGroupAppearance:  "外观",
		SettingsGroupGitExec:     "Git 可执行文件",
		SettingsGroupTools:       "工具",
		SettingsGroupDiagnostics: "诊断",
		SettingsGroupUpdates:     "更新",
		SettingsGroupFonts:       "字体",
		SettingsGroupCommands:    "命令与签名",
		SettingsGroupRemote:      "远程",

		SettingsLabelTheme:        "主题：",
		SettingsLabelFontSize:     "编辑器字号：",
		SettingsLabelGitPath:      "Git 路径：",
		SettingsLabelEditor:       "外部编辑器命令：",
		SettingsLabelLanguage:     "语言：",
		SettingsLabelFetchMins:    "间隔（分钟）：",
		SettingsLabelUpdateMode:   "更新方式：",
		SettingsLabelInterface:    "界面字体：",
		SettingsLabelDiffFont:     "diff 字体：",
		SettingsLabelSettingsFile: "设置文件：%s",

		SettingsHintEditor:    "留空则用系统默认。{file} 与 {line} 会被替换。",
		SettingsHintDebugLog:  "记录详细事件用于排查问题。日志只保存在本机。",
		SettingsHintInterface: "替换整个界面的字体。diff 与控制台仍保持等宽。",
		SettingsHintDiffFont:  "只列等宽字体：diff、控制台与哈希按字符网格排版。",

		SettingsCheckDebugLog:   "调试日志",
		SettingsCheckAutoUpdate: "启动时检查更新",
		SettingsCheckPrune:      "抓取时清理远程分支",
		SettingsCheckAutoFetch:  "后台自动抓取",

		SettingsThemeDark:   "Islands 深色",
		SettingsThemeLight:  "Islands 浅色",
		SettingsThemeSystem: "跟随系统",

		SettingsPullMerge:  "合并",
		SettingsPullRebase: "变基",
		SettingsPullFFOnly: "仅快进",

		SettingsFontDefaultInterface: "默认（系统界面字体）",
		SettingsFontDefaultDiff:      "默认（内置或系统等宽字体）",

		SettingsLangEnglish: "English",
		SettingsLangChinese: "简体中文",

		SettingsPlaceholderAuto:  "自动检测",
		SettingsPlaceholderTheme: "Islands 深色",

		CommonOK:     "确定",
		CommonCancel: "取消",
		CommonClose:  "关闭",

		AboutTitle:   "关于 Viff",
		AboutTagline: "IntelliJ IDEA 风格的 Git 客户端",
		AboutCredits: "图标来自 IntelliJ Platform（Apache 2.0），编辑器字体 JetBrains Mono（OFL）。",
	})
}
