package i18n

// Message keys of the rebase plan, patches, the compare view, the read-only
// file view and the revision browser.
const (
	RebaseCannotFrom    = Key("rebase.cannotFrom")
	RebaseInteractive   = Key("rebase.interactive")
	RebaseAllDropped    = Key("rebase.allDropped")
	RebaseHint          = Key("rebase.hint")
	RebasePlan          = Key("rebase.plan")
	RebaseMoveUp        = Key("rebase.moveUp")
	RebaseMoveDown      = Key("rebase.moveDown")
	RebaseMessageOf     = Key("rebase.messageOf")
	RebaseCommitMessage = Key("rebase.commitMessage")

	PatchFilter       = Key("patch.filter")
	PatchCannotCreate = Key("patch.cannotCreate")
	PatchNoneCreated  = Key("patch.noneCreated")
	PatchNoneBody     = Key("patch.noneBody")
	PatchCannotWrite  = Key("patch.cannotWrite")
	PatchCreated      = Key("patch.created")
	PatchApply        = Key("patch.apply")
	PatchApplyIndex   = Key("patch.applyIndex")
	PatchFetchFull    = Key("patch.fetchFull")
	ShowInFileManager = Key("common.showInFileManager")

	CompareTitle   = Key("compare.title")
	CompareLoading = Key("compare.loading")
	CompareNoDiff  = Key("compare.noDiff")
	CompareHint    = Key("compare.hint")

	FileviewWorkingTree      = Key("fileview.workingTree")
	FileviewCannotAnnotate   = Key("fileview.cannotAnnotate")
	FileviewAnnotate         = Key("fileview.annotate")
	FileviewCloseAnnotations = Key("fileview.closeAnnotations")
	FileviewShowDiff         = Key("fileview.showDiff")
	FileviewShowInLog        = Key("fileview.showInLog")
	FileviewAnnotateRev      = Key("fileview.annotateRevision")
	FileviewCopyRev          = Key("fileview.copyRevision")
	FileviewOpenExternal     = Key("fileview.openExternal")
	FileviewShowHistory      = Key("fileview.showHistory")
	FileviewCodemap          = Key("fileview.codemap")

	FileviewEdit            = Key("fileview.edit")
	FileviewSave            = Key("fileview.save")
	FileviewRevert          = Key("fileview.revert")
	FileviewDone            = Key("fileview.done")
	FileviewEditing         = Key("fileview.editing")
	FileviewModified        = Key("fileview.modified")
	FileviewEditLabel       = Key("fileview.editLabel")
	FileviewDiscardTitle    = Key("fileview.discardTitle")
	FileviewDiscardBody     = Key("fileview.discardBody")
	FileviewDiscardButton   = Key("fileview.discardButton")
	FileviewSaveFailed      = Key("fileview.saveFailed")
	FileviewChangedTitle    = Key("fileview.changedTitle")
	FileviewChangedBody     = Key("fileview.changedBody")
	FileviewReload          = Key("fileview.reload")
	FileviewEditNotUTF8     = Key("fileview.editNotUTF8")
	FileviewEditTooLarge    = Key("fileview.editTooLarge")
	FileviewEditReadOnly    = Key("fileview.editReadOnly")
	FileviewEditUnavailable = Key("fileview.editUnavailable")
	FileviewSaveTooLarge    = Key("fileview.saveTooLarge")
	FileviewFileMissing     = Key("fileview.fileMissing")

	RevisionTitle   = Key("revision.title")
	RevisionFilter  = Key("revision.filter")
	RevisionLoading = Key("revision.loading")
	RevisionEmpty   = Key("revision.empty")
	RevisionNoMatch = Key("revision.noMatch")
	RevisionHint    = Key("revision.hint")
)

func init() {
	register(EN, map[Key]string{
		RebaseCannotFrom:    "Cannot rebase from %s",
		RebaseInteractive:   "Interactive rebase",
		RebaseAllDropped:    "Every commit is dropped: the branch will point at the base commit.",
		RebaseHint:          "Local changes are stashed and restored automatically. Conflicts stop the rebase; continue or abort it from the Commit tool window.",
		RebasePlan:          "Rebase plan",
		RebaseMoveUp:        "Move Up",
		RebaseMoveDown:      "Move Down",
		RebaseMessageOf:     "Message of %s",
		RebaseCommitMessage: "Commit message",

		PatchFilter:       "Patch files",
		PatchCannotCreate: "Cannot create patch",
		PatchNoneCreated:  "No patch created",
		PatchNoneBody:     "The selection has no changes to include.",
		PatchCannotWrite:  "Cannot write patch",
		PatchCreated:      "Patch created",
		PatchApply:        "Apply Patch…",
		PatchApplyIndex:   "Apply Patch to Index…",
		PatchFetchFull:    "Fetch Full History (Unshallow)",
		ShowInFileManager: revealLabel(EN),

		CompareTitle:   "Compare %s → %s",
		CompareLoading: "Loading changed files…",
		CompareNoDiff:  "The commits have no differences",
		CompareHint:    "Double-click a file to open its diff",

		FileviewWorkingTree:      "Working tree",
		FileviewCannotAnnotate:   "Cannot annotate: %s",
		FileviewAnnotate:         "Annotate with Git Blame",
		FileviewCloseAnnotations: "Close Annotations",
		FileviewShowDiff:         "Show Diff",
		FileviewShowInLog:        "Show in Git Log",
		FileviewAnnotateRev:      "Annotate Revision %s",
		FileviewCopyRev:          "Copy Revision Number",
		FileviewOpenExternal:     "Open in External Editor",
		FileviewShowHistory:      "Show History",
		FileviewCodemap:          "Code Map",

		FileviewEdit:            "Edit",
		FileviewSave:            "Save",
		FileviewRevert:          "Revert",
		FileviewDone:            "Done",
		FileviewEditing:         "Editing",
		FileviewModified:        "Modified",
		FileviewEditLabel:       "File editor",
		FileviewDiscardTitle:    "Discard unsaved changes?",
		FileviewDiscardBody:     "%s has unsaved changes. Discard them?",
		FileviewDiscardButton:   "Discard",
		FileviewSaveFailed:      "Cannot save %s",
		FileviewChangedTitle:    "File changed on disk",
		FileviewChangedBody:     "Your changes were not saved. Reload the file to see the current contents.",
		FileviewReload:          "Reload",
		FileviewEditNotUTF8:     "This file is not UTF-8 text. Open it in an external editor.",
		FileviewEditTooLarge:    "This file is too large to edit in Viff. Open it in an external editor.",
		FileviewEditReadOnly:    "This file is read-only.",
		FileviewEditUnavailable: "This file cannot be edited in Viff.",
		FileviewSaveTooLarge:    "The edited text is larger than the 1 MiB editing limit. Shorten it before saving.",
		FileviewFileMissing:     "The file no longer exists. Reload the view to see the current state.",

		RevisionTitle:   "Repository at %s",
		RevisionFilter:  "Filter files",
		RevisionLoading: "Loading the files of this revision…",
		RevisionEmpty:   "The revision has no files",
		RevisionNoMatch: "No files match the filter",
		RevisionHint:    "Double-click a file to open it at this revision",
	})
	register(ZHCN, map[Key]string{
		RebaseCannotFrom:    "无法从此提交变基：%s",
		RebaseInteractive:   "交互式变基",
		RebaseAllDropped:    "所有提交都会被丢弃：分支将指向基点提交。",
		RebaseHint:          "本地更改会自动储藏并在之后恢复；遇到冲突会停下变基，可在提交窗口里继续或中止。",
		RebasePlan:          "变基计划",
		RebaseMoveUp:        "上移",
		RebaseMoveDown:      "下移",
		RebaseMessageOf:     "%s 的提交说明",
		RebaseCommitMessage: "提交说明",

		PatchFilter:       "补丁文件",
		PatchCannotCreate: "无法创建补丁",
		PatchNoneCreated:  "没有创建补丁",
		PatchNoneBody:     "所选内容里没有可包含的改动。",
		PatchCannotWrite:  "无法写入补丁",
		PatchCreated:      "补丁已创建",
		PatchApply:        "应用补丁…",
		PatchApplyIndex:   "应用补丁到暂存区…",
		PatchFetchFull:    "抓取完整历史（补全浅克隆）",
		ShowInFileManager: revealLabel(ZHCN),

		CompareTitle:   "比较 %s → %s",
		CompareLoading: "正在加载变更文件…",
		CompareNoDiff:  "这些提交之间没有差异",
		CompareHint:    "双击文件查看它的 diff",

		FileviewWorkingTree:      "工作区",
		FileviewCannotAnnotate:   "无法注解：%s",
		FileviewAnnotate:         "用 Git blame 注解",
		FileviewCloseAnnotations: "关闭注解",
		FileviewShowDiff:         "显示 diff",
		FileviewShowInLog:        "在日志中显示",
		FileviewAnnotateRev:      "注解提交 %s",
		FileviewCopyRev:          "复制提交哈希",
		FileviewOpenExternal:     "用外部编辑器打开",
		FileviewShowHistory:      "显示历史",
		FileviewCodemap:          "代码缩略图",

		FileviewEdit:            "编辑",
		FileviewSave:            "保存",
		FileviewRevert:          "还原",
		FileviewDone:            "完成",
		FileviewEditing:         "正在编辑",
		FileviewModified:        "已修改",
		FileviewEditLabel:       "文件编辑器",
		FileviewDiscardTitle:    "放弃未保存的修改？",
		FileviewDiscardBody:     "%s 有未保存的修改，确定放弃？",
		FileviewDiscardButton:   "放弃",
		FileviewSaveFailed:      "无法保存 %s",
		FileviewChangedTitle:    "文件已在磁盘上更改",
		FileviewChangedBody:     "你的修改未保存。请重新加载文件后再试。",
		FileviewReload:          "重新加载",
		FileviewEditNotUTF8:     "该文件不是 UTF-8 文本，请用外部编辑器打开。",
		FileviewEditTooLarge:    "该文件过大，无法在 Viff 中编辑，请用外部编辑器打开。",
		FileviewEditReadOnly:    "该文件是只读的。",
		FileviewEditUnavailable: "该文件无法在 Viff 中编辑。",
		FileviewSaveTooLarge:    "编辑后的文本超过 1 MiB 上限，请缩短后再保存。",
		FileviewFileMissing:     "文件已不存在。请重新加载视图查看当前状态。",

		RevisionTitle:   "%s 的仓库内容",
		RevisionFilter:  "筛选文件",
		RevisionLoading: "正在加载该提交的文件…",
		RevisionEmpty:   "该提交没有文件",
		RevisionNoMatch: "没有匹配筛选条件的文件",
		RevisionHint:    "双击文件，以该提交的版本打开",
	})
}
