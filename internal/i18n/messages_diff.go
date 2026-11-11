package i18n

// Message keys of the diff viewer, the conflict viewer, the picture diff and
// the console. Like the settings keys beside them, this file registers its own
// maps from an init, so the message files never share a literal.
const (
	DiffNoticeIdentical          = Key("diff.notice.identical")
	DiffNoticeModeChanged        = Key("diff.notice.modeChanged")
	DiffNoticeModeChangedContent = Key("diff.notice.modeChangedContent")

	DiffViewSideBySide = Key("diff.view.sideBySide")
	DiffViewUnified    = Key("diff.view.unified")

	// The whitespace chip and the menu behind it.
	DiffWhitespaceExact  = Key("diff.whitespace.exact")
	DiffWhitespaceTrim   = Key("diff.whitespace.trim")
	DiffWhitespaceIgnore = Key("diff.whitespace.ignore")

	DiffActionPrevDifference = Key("diff.action.previousDifference")
	DiffActionNextDifference = Key("diff.action.nextDifference")
	DiffActionHighlightWords = Key("diff.action.highlightWords")
	DiffActionSyncScrolling  = Key("diff.action.syncScrolling")
	DiffActionCollapse       = Key("diff.action.collapse")
	DiffResizePanes          = Key("diff.resizePanes")
	DiffFoldLines            = Key("diff.fold.lines")
	DiffFoldExpand           = Key("diff.fold.expand")
	DiffActionOpenEditor     = Key("diff.action.openEditor")
	DiffActionJumpSource     = Key("diff.action.jumpToSource")
	DiffActionStageFile      = Key("diff.action.stageFile")
	DiffActionUnstageFile    = Key("diff.action.unstageFile")

	DiffActionMarks  = Key("diff.action.marks")
	DiffMarksLabel   = Key("diff.marks.label")
	DiffMarksTip     = Key("diff.marks.tip")
	DiffCodemapLabel = Key("diff.codemap.label")
	DiffCodemapTip   = Key("diff.codemap.tip")

	DiffTipStageChange         = Key("diff.tip.stageChange")
	DiffTipStageSelectedOne    = Key("diff.tip.stageSelectedLine")
	DiffTipStageSelectedMany   = Key("diff.tip.stageSelectedLines")
	DiffTipRevertChange        = Key("diff.tip.revertChange")
	DiffTipRevertSelectedOne   = Key("diff.tip.revertSelectedLine")
	DiffTipRevertSelectedMany  = Key("diff.tip.revertSelectedLines")
	DiffTipUnstageChange       = Key("diff.tip.unstageChange")
	DiffTipUnstageSelectedOne  = Key("diff.tip.unstageSelectedLine")
	DiffTipUnstageSelectedMany = Key("diff.tip.unstageSelectedLines")
	DiffTipSelectLines         = Key("diff.tip.selectLines")

	DiffCountNone    = Key("diff.count.none")
	DiffCountOne     = Key("diff.count.one")
	DiffCountMany    = Key("diff.count.many")
	DiffCountOfTotal = Key("diff.count.ofTotal")

	DiffSearchPlaceholder = Key("diff.search.placeholder")
	DiffSearchNoResults   = Key("diff.search.noResults")
	DiffSearchResultOne   = Key("diff.search.resultOne")
	DiffSearchResultMany  = Key("diff.search.resultMany")
	DiffSearchPrev        = Key("diff.search.previousOccurrence")
	DiffSearchNext        = Key("diff.search.nextOccurrence")
	DiffSearchClose       = Key("diff.search.close")

	ConflictLabelResult    = Key("conflict.label.result")
	ConflictLabelEditable  = Key("conflict.label.editable")
	ConflictTitleMerge     = Key("conflict.title.merge")
	ConflictSubResolve     = Key("conflict.sub.resolve")
	ConflictTitleResolved  = Key("conflict.title.resolved")
	ConflictSubResolved    = Key("conflict.sub.resolved")
	ConflictSubFoundOne    = Key("conflict.sub.foundOne")
	ConflictSubFoundMany   = Key("conflict.sub.foundMany")
	ConflictSubAcceptOne   = Key("conflict.sub.acceptOne")
	ConflictSubAcceptMany  = Key("conflict.sub.acceptMany")
	ConflictSubUnreliable  = Key("conflict.sub.unreliable")
	ConflictSubNoText      = Key("conflict.sub.noText")
	ConflictSubResultClean = Key("conflict.sub.resultClean")

	ConflictActionMarkAsResolved = Key("conflict.action.markAsResolved")
	ConflictTipMarkAsResolved    = Key("conflict.tip.markAsResolved")
	ConflictTipSaveFirst         = Key("conflict.tip.saveFirst")
	ConflictActionSave           = Key("conflict.action.save")
	ConflictTipSave              = Key("conflict.tip.save")
	ConflictActionRevert         = Key("conflict.action.revert")
	ConflictTipRevert            = Key("conflict.tip.revert")
	ConflictEditLabel            = Key("conflict.label.editor")
	ConflictActionOpenEditor     = Key("conflict.action.openEditor")
	ConflictActionCopyPath       = Key("conflict.action.copyPath")
	ConflictActionPrevious       = Key("conflict.action.previousConflict")
	ConflictActionNext           = Key("conflict.action.nextConflict")
	ConflictCount                = Key("conflict.count")
	ConflictAcceptTitle          = Key("conflict.accept.title")
	ConflictAcceptOurs           = Key("conflict.action.acceptOurs")
	ConflictAcceptTheirs         = Key("conflict.action.acceptTheirs")
	ConflictAcceptBoth           = Key("conflict.action.acceptBoth")
	ConflictTipKeepSide          = Key("conflict.tip.keepSide")
	ConflictTipKeepBoth          = Key("conflict.tip.keepBoth")
	ConflictActionCopyForAI      = Key("conflict.action.copyForAI")
	ConflictTipCopyForAI         = Key("conflict.tip.copyForAI")
	ConflictActionCopyBlock      = Key("conflict.action.copyBlock")
	ConflictTipCopyBlock         = Key("conflict.tip.copyBlock")
	ConflictTipCopyNothing       = Key("conflict.tip.copyNothing")
	ConflictNoticeCopied         = Key("conflict.notice.copied")
	ConflictNoticeCopiedBody     = Key("conflict.notice.copiedBody")
	ConflictNoticeSaved          = Key("conflict.notice.saved")
	ConflictNoticeSavedBody      = Key("conflict.notice.savedBody")

	PictureNoNewVersion   = Key("picture.notice.noNewVersion")
	PictureNoOldVersion   = Key("picture.notice.noPreviousVersion")
	PictureNoSide         = Key("picture.notice.noSide")
	PictureFirstFrameOnly = Key("picture.label.firstFrameOnly")
	PictureCannotDecode   = Key("picture.error.cannotDecode")

	ConsoleActionClear       = Key("console.action.clearAll")
	ConsoleConfirmClearTitle = Key("console.confirm.clearTitle")
	ConsoleConfirmClearBody  = Key("console.confirm.clearBody")
	ConsoleConfirmClearOK    = Key("console.confirm.clearOK")
	ConsoleActionSave        = Key("console.action.save")
	ConsoleActionCopyAll     = Key("console.action.copyAll")
	ConsoleHeader            = Key("console.header")
	ConsoleEmpty             = Key("console.empty")
	ConsoleSaveTitle         = Key("console.save.title")
	ConsoleSaveFilter        = Key("console.save.filter")
	ConsoleSaveError         = Key("console.save.error")
	ConsoleSaved             = Key("console.save.done")
	ConsoleActionReveal      = Key("console.action.reveal")
	ConsoleOutputTruncated   = Key("console.output.truncated")
)

func init() {
	register(EN, map[Key]string{
		DiffNoticeIdentical:          "Contents are identical",
		DiffNoticeModeChanged:        "File mode changed: %s",
		DiffNoticeModeChangedContent: "File mode changed: %s (content changed too)",

		DiffViewSideBySide: "Side-by-side viewer",
		DiffViewUnified:    "Unified viewer",

		DiffWhitespaceExact:  "Do not ignore",
		DiffWhitespaceTrim:   "Trim whitespaces",
		DiffWhitespaceIgnore: "Ignore whitespaces",

		DiffActionPrevDifference: "Previous Difference\t⇧F7",
		DiffActionNextDifference: "Next Difference\tF7",
		DiffActionHighlightWords: "Highlight Words",
		DiffActionSyncScrolling:  "Synchronize Scrolling",
		DiffActionCollapse:       "Collapse Unchanged Fragments",
		DiffResizePanes:          "Resize diff panes (Left / Right)",
		DiffFoldLines:            "%d unchanged lines",
		DiffFoldExpand:           "Show %d unchanged lines",
		DiffActionOpenEditor:     "Open in External Editor\tF4",
		DiffActionJumpSource:     "Jump to Source",
		DiffActionStageFile:      "Stage File",
		DiffActionUnstageFile:    "Unstage File",

		DiffActionMarks:  "Change Marks",
		DiffMarksLabel:   "Change marks",
		DiffMarksTip:     "Every change in this file on one ruler; drag to scroll",
		DiffCodemapLabel: "Code map",
		DiffCodemapTip:   "The whole file at a glance; drag to scroll",

		DiffTipStageChange:         "Stage Change",
		DiffTipStageSelectedOne:    "Stage %d Selected Line",
		DiffTipStageSelectedMany:   "Stage %d Selected Lines",
		DiffTipRevertChange:        "Revert Change",
		DiffTipRevertSelectedOne:   "Revert %d Selected Line",
		DiffTipRevertSelectedMany:  "Revert %d Selected Lines",
		DiffTipUnstageChange:       "Unstage Change",
		DiffTipUnstageSelectedOne:  "Unstage %d Selected Line",
		DiffTipUnstageSelectedMany: "Unstage %d Selected Lines",
		DiffTipSelectLines:         "Click changed lines to select them for a partial stage or revert",

		DiffCountNone:    "No differences",
		DiffCountOne:     "%d difference",
		DiffCountMany:    "%d differences",
		DiffCountOfTotal: "%s of %s differences",

		DiffSearchPlaceholder: "Search in the new version",
		DiffSearchNoResults:   "0 results",
		DiffSearchResultOne:   "%d result",
		DiffSearchResultMany:  "%d results",
		DiffSearchPrev:        "Previous Occurrence\t⇧⏎",
		DiffSearchNext:        "Next Occurrence\t⏎",
		DiffSearchClose:       "Close\t⎋",

		ConflictLabelResult:          "Result (working tree)",
		ConflictLabelEditable:        "editable",
		ConflictTitleMerge:           "Merge conflict: %s",
		ConflictSubResolve:           "Resolve it in your editor; Viff shows both sides and the file on disk as it changes.",
		ConflictTitleResolved:        "No conflict markers left in the file on disk",
		ConflictSubResolved:          "Review the result, then mark the file as resolved to stage it.",
		ConflictSubFoundOne:          "%d conflict in the file on disk. Edit it in your editor; this view updates when it is saved.",
		ConflictSubFoundMany:         "%d conflicts in the file on disk. Edit it in your editor; this view updates when it is saved.",
		ConflictSubAcceptOne:         "%d conflict in the file on disk. Accept a side below, or edit the result directly.",
		ConflictSubAcceptMany:        "%d conflicts in the file on disk. Accept a side below, or edit the result directly.",
		ConflictSubUnreliable:        "The conflict markers look unusual; check the result before continuing.",
		ConflictSubNoText:            "There is no text to merge. Keep or delete the file on disk, then mark it as resolved.",
		ConflictSubResultClean:       "No conflict markers left in the result pane. Save it, then mark the file as resolved.",
		ConflictActionMarkAsResolved: "Mark as Resolved",
		ConflictTipMarkAsResolved:    "Remove all conflict markers first",
		ConflictTipSaveFirst:         "Save the merge result first",
		ConflictActionSave:           "Save Merge Result",
		ConflictTipSave:              "Write the merged result to the working tree (⌘S)",
		ConflictActionRevert:         "Revert",
		ConflictTipRevert:            "Discard the edits in the result pane",
		ConflictEditLabel:            "Merge result editor",
		ConflictActionOpenEditor:     "Open in Editor",
		ConflictActionCopyPath:       "Copy Path",
		ConflictActionPrevious:       "Previous Conflict",
		ConflictActionNext:           "Next Conflict",
		ConflictCount:                "Conflict %s of %s",
		ConflictAcceptTitle:          "Conflict %s of %s: accept",
		ConflictAcceptOurs:           "Accept Ours",
		ConflictAcceptTheirs:         "Accept Theirs",
		ConflictAcceptBoth:           "Accept Both",
		ConflictTipKeepSide:          "Keep %s for this block",
		ConflictTipKeepBoth:          "Keep %s and then %s for this block",
		ConflictActionCopyForAI:      "Copy for AI",
		ConflictTipCopyForAI:         "Copy the conflict and file details for an external AI tool; Viff calls no model",
		ConflictActionCopyBlock:      "Copy This Block",
		ConflictTipCopyBlock:         "Copy only this conflict block for an external AI tool",
		ConflictTipCopyNothing:       "Available while the file has conflict markers",
		ConflictNoticeCopied:         "Conflict context copied",
		ConflictNoticeCopiedBody:     "Copied. Viff refreshes the file automatically after it is edited.",
		ConflictNoticeSaved:          "Merge result saved",
		ConflictNoticeSavedBody:      "The working-tree file was updated and the view reloaded.",

		PictureNoNewVersion:   "No new version",
		PictureNoOldVersion:   "No previous version",
		PictureNoSide:         "The file does not exist on this side.",
		PictureFirstFrameOnly: "  · first frame only",
		PictureCannotDecode:   "Cannot decode picture",

		ConsoleActionClear:       "Clear All",
		ConsoleConfirmClearTitle: "Clear Console",
		ConsoleConfirmClearBody:  "Remove all Git commands and their output from the Console?\n\nThis only clears the view; Git is not affected.",
		ConsoleConfirmClearOK:    "Clear",
		ConsoleActionSave:        "Save Console Output…",
		ConsoleActionCopyAll:     "Copy All",
		ConsoleHeader:            "Commands run by Viff",
		ConsoleEmpty:             "No commands yet",
		ConsoleSaveTitle:         "Save Console Output",
		ConsoleSaveFilter:        "Text files",
		ConsoleSaveError:         "Cannot save console output",
		ConsoleSaved:             "Console saved",
		ConsoleActionReveal:      revealLabel(EN),
		ConsoleOutputTruncated:   "… (output truncated)",
	})

	register(ZHCN, map[Key]string{
		DiffNoticeIdentical:          "内容相同",
		DiffNoticeModeChanged:        "文件模式已变更：%s",
		DiffNoticeModeChangedContent: "文件模式已变更：%s（内容也有变更）",

		DiffViewSideBySide: "并排视图",
		DiffViewUnified:    "统一视图",

		DiffWhitespaceExact:  "不忽略",
		DiffWhitespaceTrim:   "忽略行尾空白",
		DiffWhitespaceIgnore: "忽略空白",

		DiffActionPrevDifference: "上一处差异\t⇧F7",
		DiffActionNextDifference: "下一处差异\tF7",
		DiffActionHighlightWords: "高亮单词",
		DiffActionSyncScrolling:  "同步滚动",
		DiffActionCollapse:       "折叠未更改的片段",
		DiffResizePanes:          "调整差异分栏宽度（左 / 右方向键）",
		DiffFoldLines:            "未更改的 %d 行",
		DiffFoldExpand:           "展开 %d 行未更改内容",
		DiffActionOpenEditor:     "在外部编辑器中打开\tF4",
		DiffActionJumpSource:     "跳转到源",
		DiffActionStageFile:      "暂存文件",
		DiffActionUnstageFile:    "取消暂存文件",

		DiffActionMarks:  "改动标记",
		DiffMarksLabel:   "改动标记",
		DiffMarksTip:     "整份文件的改动都在这一条上；拖动可滚动",
		DiffCodemapLabel: "代码缩略图",
		DiffCodemapTip:   "整份文件一览；拖动可滚动",

		DiffTipStageChange:         "暂存此更改",
		DiffTipStageSelectedOne:    "暂存所选 %d 行",
		DiffTipStageSelectedMany:   "暂存所选 %d 行",
		DiffTipRevertChange:        "还原此更改",
		DiffTipRevertSelectedOne:   "还原所选 %d 行",
		DiffTipRevertSelectedMany:  "还原所选 %d 行",
		DiffTipUnstageChange:       "取消暂存此更改",
		DiffTipUnstageSelectedOne:  "取消暂存所选 %d 行",
		DiffTipUnstageSelectedMany: "取消暂存所选 %d 行",
		DiffTipSelectLines:         "点击已更改的行即可选中，用于部分暂存或还原",

		DiffCountNone:    "没有差异",
		DiffCountOne:     "%d 处差异",
		DiffCountMany:    "%d 处差异",
		DiffCountOfTotal: "%s / %s 处差异",

		DiffSearchPlaceholder: "在新版本中搜索",
		DiffSearchNoResults:   "0 个结果",
		DiffSearchResultOne:   "%d 个结果",
		DiffSearchResultMany:  "%d 个结果",
		DiffSearchPrev:        "上一处匹配\t⇧⏎",
		DiffSearchNext:        "下一处匹配\t⏎",
		DiffSearchClose:       "关闭\t⎋",

		ConflictLabelResult:          "结果（工作区）",
		ConflictLabelEditable:        "可编辑",
		ConflictTitleMerge:           "合并冲突：%s",
		ConflictSubResolve:           "在编辑器中解决冲突；Viff 会显示两侧内容，磁盘上的文件也会随编辑更新。",
		ConflictTitleResolved:        "磁盘上的文件中已无冲突标记",
		ConflictSubResolved:          "检查结果，然后把文件标记为已解决以便暂存。",
		ConflictSubFoundOne:          "磁盘上的文件中有 %d 处冲突。请在编辑器中修改；保存后此视图会更新。",
		ConflictSubFoundMany:         "磁盘上的文件中有 %d 处冲突。请在编辑器中修改；保存后此视图会更新。",
		ConflictSubAcceptOne:         "磁盘上的文件中有 %d 处冲突。可在下方接受某一侧，或直接编辑中间的结果。",
		ConflictSubAcceptMany:        "磁盘上的文件中有 %d 处冲突。可在下方接受某一侧，或直接编辑中间的结果。",
		ConflictSubUnreliable:        "这些冲突标记看起来不常见，请检查结果后再继续。",
		ConflictSubNoText:            "没有可合并的文本。保留或删除磁盘上的文件，然后将其标记为已解决。",
		ConflictSubResultClean:       "结果栏中已无冲突标记。请先保存，再把文件标记为已解决。",
		ConflictActionMarkAsResolved: "标记为已解决",
		ConflictTipMarkAsResolved:    "请先移除所有冲突标记",
		ConflictTipSaveFirst:         "请先保存合并结果",
		ConflictActionSave:           "保存合并结果",
		ConflictTipSave:              "将合并结果写入工作区文件（⌘S）",
		ConflictActionRevert:         "还原",
		ConflictTipRevert:            "放弃结果栏中的修改",
		ConflictEditLabel:            "合并结果编辑器",
		ConflictActionOpenEditor:     "在编辑器中打开",
		ConflictActionCopyPath:       "复制路径",
		ConflictActionPrevious:       "上一处冲突",
		ConflictActionNext:           "下一处冲突",
		ConflictCount:                "第 %s 处冲突，共 %s 处",
		ConflictAcceptTitle:          "第 %s 处冲突，共 %s 处：接受",
		ConflictAcceptOurs:           "接受我方",
		ConflictAcceptTheirs:         "接受对方",
		ConflictAcceptBoth:           "接受双方",
		ConflictTipKeepSide:          "此块保留 %s",
		ConflictTipKeepBoth:          "此块先保留 %s，再保留 %s",
		ConflictActionCopyForAI:      "复制给 AI",
		ConflictTipCopyForAI:         "把冲突内容与文件信息复制给外部 AI 工具；Viff 不调用模型",
		ConflictActionCopyBlock:      "复制此冲突块",
		ConflictTipCopyBlock:         "只复制当前冲突块，供外部 AI 工具使用",
		ConflictTipCopyNothing:       "文件中有冲突标记时可用",
		ConflictNoticeCopied:         "冲突上下文已复制",
		ConflictNoticeCopiedBody:     "已复制，外部修改后 Viff 会自动刷新",
		ConflictNoticeSaved:          "合并结果已保存",
		ConflictNoticeSavedBody:      "工作区文件已更新，视图已重新加载。",

		PictureNoNewVersion:   "没有新版本",
		PictureNoOldVersion:   "没有旧版本",
		PictureNoSide:         "该文件在此侧不存在。",
		PictureFirstFrameOnly: "  · 仅显示第一帧",
		PictureCannotDecode:   "无法解码图片",

		ConsoleActionClear:       "全部清除",
		ConsoleConfirmClearTitle: "清除控制台",
		ConsoleConfirmClearBody:  "要从控制台移除所有 Git 命令及其输出吗？\n\n仅清除视图，不影响 Git。",
		ConsoleConfirmClearOK:    "清除",
		ConsoleActionSave:        "保存控制台输出…",
		ConsoleActionCopyAll:     "全部复制",
		ConsoleHeader:            "Viff 运行的命令",
		ConsoleEmpty:             "暂无命令",
		ConsoleSaveTitle:         "保存控制台输出",
		ConsoleSaveFilter:        "文本文件",
		ConsoleSaveError:         "无法保存控制台输出",
		ConsoleSaved:             "控制台已保存",
		ConsoleActionReveal:      revealLabel(ZHCN),
		ConsoleOutputTruncated:   "…（输出已截断）",
	})
}
