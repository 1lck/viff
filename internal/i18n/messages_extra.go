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
		ShowInFileManager: "Show in Finder",

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
		ShowInFileManager: "在文件管理器中显示",

		CompareTitle:   "比较 %s → %s",
		CompareLoading: "正在加载变更文件…",
		CompareNoDiff:  "这些提交之间没有差异",
		CompareHint:    "双击文件查看它的 diff",

		FileviewWorkingTree:      "工作区",
		FileviewCannotAnnotate:   "无法注解：%s",
		FileviewAnnotate:         "用 Git Blame 注解",
		FileviewCloseAnnotations: "关闭注解",
		FileviewShowDiff:         "显示 diff",
		FileviewShowInLog:        "在日志中显示",
		FileviewAnnotateRev:      "注解提交 %s",
		FileviewCopyRev:          "复制提交哈希",
		FileviewOpenExternal:     "在外部编辑器中打开",
		FileviewShowHistory:      "显示历史",

		RevisionTitle:   "%s 的仓库内容",
		RevisionFilter:  "筛选文件",
		RevisionLoading: "正在加载该提交的文件…",
		RevisionEmpty:   "该提交没有文件",
		RevisionNoMatch: "没有匹配筛选条件的文件",
		RevisionHint:    "双击文件，以该提交的版本打开",
	})
}
