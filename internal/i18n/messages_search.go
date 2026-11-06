package i18n

// Message keys of the search dialog and the search settings. The dialog has
// two pages: file paths (the old Quick Open) and file contents.
const (
	SearchTitle       = Key("search.title")
	SearchTabFiles    = Key("search.tab.files")
	SearchTabContents = Key("search.tab.contents")

	SearchPlaceholderFiles    = Key("search.placeholder.files")
	SearchPlaceholderContents = Key("search.placeholder.contents")
	SearchEmptyContents       = Key("search.empty.contents")
	SearchNoMatches           = Key("search.noMatches")
	SearchLoading             = Key("search.loading")
	SearchResultsOne          = Key("search.results.one")
	SearchResultsMany         = Key("search.results.many")
	SearchTruncated           = Key("search.truncated")
	SearchHintFiles           = Key("search.hint.files")
	SearchHintContents        = Key("search.hint.contents")
	SearchOpenButton          = Key("search.button.open")
	SearchSettingsButton      = Key("search.button.settings")
	SearchRipgrepMissing      = Key("search.ripgrep.missing")
	SearchNoRepository        = Key("search.noRepository")

	SearchMenuFindInFiles = Key("search.menu.findInFiles")

	SettingsGroupSearch      = Key("settings.group.search")
	SettingsLabelSearchTool  = Key("settings.label.searchTool")
	SettingsLabelRipgrepPath = Key("settings.label.ripgrepPath")
	SettingsSearchAuto       = Key("settings.search.auto")
	SettingsSearchGit        = Key("settings.search.git")
	SettingsSearchRipgrep    = Key("settings.search.ripgrep")
	SettingsHintSearchTool   = Key("settings.hint.searchTool")
	SettingsHintRipgrep      = Key("settings.hint.ripgrep")
	SettingsRipgrepFound     = Key("settings.ripgrep.found")
	SettingsRipgrepMissing   = Key("settings.ripgrep.missing")
	SettingsRipgrepDownload  = Key("settings.ripgrep.download")
)

func init() {
	register(EN, map[Key]string{
		SearchTitle:       "Search",
		SearchTabFiles:    "Files",
		SearchTabContents: "Contents",

		SearchPlaceholderFiles:    "Search files by path",
		SearchPlaceholderContents: "Search file contents",
		SearchEmptyContents:       "Type to search the working tree",
		SearchNoMatches:           "No matches in the working tree",
		SearchLoading:             "Searching…",
		SearchResultsOne:          "%d match",
		SearchResultsMany:         "%d matches",
		SearchTruncated:           "Showing the first %d matches; refine the search",
		SearchHintFiles:           "Enter opens the file read-only; Esc closes",
		SearchHintContents:        "Enter opens the match read-only; Esc closes",
		SearchOpenButton:          "Open",
		SearchSettingsButton:      "Settings…",
		SearchRipgrepMissing:      "ripgrep is not installed; Git grep is used instead. Set it up in Settings.",
		SearchNoRepository:        "Open a repository to search",

		SearchMenuFindInFiles: "Find in Files…",

		SettingsGroupSearch:      "Search",
		SettingsLabelSearchTool:  "Search tool:",
		SettingsLabelRipgrepPath: "ripgrep path:",
		SettingsSearchAuto:       "Auto (ripgrep if installed)",
		SettingsSearchGit:        "Git grep",
		SettingsSearchRipgrep:    "ripgrep",
		SettingsHintSearchTool:   "Content search reads the working tree on every query; no index is built.",
		SettingsHintRipgrep:      "Not bundled. Leave the path empty to use ripgrep from PATH.",
		SettingsRipgrepFound:     "ripgrep found: %s",
		SettingsRipgrepMissing:   "ripgrep not found; Git grep is used.",
		SettingsRipgrepDownload:  "Download ripgrep",
	})

	register(ZHCN, map[Key]string{
		SearchTitle:       "搜索",
		SearchTabFiles:    "文件",
		SearchTabContents: "内容",

		SearchPlaceholderFiles:    "按路径搜索文件",
		SearchPlaceholderContents: "搜索文件内容",
		SearchEmptyContents:       "输入关键字搜索工作树",
		SearchNoMatches:           "工作树中没有匹配项",
		SearchLoading:             "正在搜索…",
		SearchResultsOne:          "%d 个匹配",
		SearchResultsMany:         "%d 个匹配",
		SearchTruncated:           "只显示前 %d 个匹配，请缩小搜索范围",
		SearchHintFiles:           "Enter 以只读方式打开文件；Esc 关闭",
		SearchHintContents:        "Enter 以只读方式打开匹配行；Esc 关闭",
		SearchOpenButton:          "打开",
		SearchSettingsButton:      "设置…",
		SearchRipgrepMissing:      "未安装 ripgrep，已改用 Git grep。可在设置里配置。",
		SearchNoRepository:        "请先打开一个仓库再搜索",

		SearchMenuFindInFiles: "在文件中查找…",

		SettingsGroupSearch:      "搜索",
		SettingsLabelSearchTool:  "搜索工具：",
		SettingsLabelRipgrepPath: "ripgrep 路径：",
		SettingsSearchAuto:       "自动（已安装 ripgrep 时优先）",
		SettingsSearchGit:        "Git grep",
		SettingsSearchRipgrep:    "ripgrep",
		SettingsHintSearchTool:   "内容搜索每次直接读取工作树，不建立索引。",
		SettingsHintRipgrep:      "不随应用打包。留空则从 PATH 中查找 ripgrep。",
		SettingsRipgrepFound:     "已找到 ripgrep：%s",
		SettingsRipgrepMissing:   "未找到 ripgrep；将使用 Git grep。",
		SettingsRipgrepDownload:  "下载 ripgrep",
	})
}
