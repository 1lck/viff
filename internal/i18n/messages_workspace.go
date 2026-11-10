package i18n

// Message keys of the workspace panels: the Commit tool window and its Stash
// tab, and the editor's empty state. Later stages add their own file beside
// this one; each file registers its own map from an init, so the files never
// share a literal.
const (
	// Panel titles of the Commit tool window and the lists it shows.
	WorkspacePanelCommit  = Key("workspace.panel.commit")
	WorkspacePanelStash   = Key("workspace.panel.stash")
	WorkspacePanelChanges = Key("workspace.panel.changes")

	// Change-tree group headings.
	WorkspaceAreaStaged    = Key("workspace.area.staged")
	WorkspaceAreaChanges   = Key("workspace.area.changes")
	WorkspaceAreaUntracked = Key("workspace.area.untracked")
	WorkspaceAreaConflicts = Key("workspace.area.conflicts")

	// Stash panel and the smart-checkout confirmation.
	WorkspaceStashChanges        = Key("workspace.stash.changes")
	WorkspaceStashApply          = Key("workspace.stash.apply")
	WorkspaceStashPop            = Key("workspace.stash.pop")
	WorkspaceStashDrop           = Key("workspace.stash.drop")
	WorkspaceStashAsBranch       = Key("workspace.stash.asBranch")
	WorkspaceStashEmpty          = Key("workspace.stash.empty")
	WorkspaceStashEmptyHint      = Key("workspace.stash.emptyHint")
	WorkspaceStashList           = Key("workspace.stash.list")
	WorkspaceStashApplyIndex     = Key("workspace.stash.applyIndex")
	WorkspaceStashPopMenu        = Key("workspace.stash.popMenu")
	WorkspaceStashCopyRev        = Key("workspace.stash.copyRev")
	WorkspaceStashDropMenu       = Key("workspace.stash.dropMenu")
	WorkspaceStashDropTitle      = Key("workspace.stash.dropTitle")
	WorkspaceStashDropBody       = Key("workspace.stash.dropBody")
	WorkspaceStashDropButton     = Key("workspace.stash.dropButton")
	WorkspaceSmartCheckoutTitle  = Key("workspace.smartCheckout.title")
	WorkspaceSmartCheckoutBody   = Key("workspace.smartCheckout.body")
	WorkspaceSmartCheckoutButton = Key("workspace.smartCheckout.button")

	// Dropping a commit, and the stash-to-branch dialog.
	WorkspaceDropCommitTitle = Key("workspace.log.dropCommit")
	WorkspaceDropCommitBody  = Key("workspace.log.dropCommitBody")
	WorkspaceStashBranchName = Key("workspace.stash.branchName")

	// Labels of the values a panel prints.
	WorkspaceLabelCommitMessage = Key("workspace.label.commitMessage")
	WorkspaceLabelSubmodule     = Key("workspace.label.submodule")
	WorkspaceLabelAndMore       = Key("workspace.label.andMore")

	WorkspaceStateLoading = Key("workspace.state.loading")

	// Empty states.
	WorkspaceEmptyNoChanges        = Key("workspace.empty.noChanges")
	WorkspaceEmptyWorkingTreeClean = Key("workspace.empty.workingTreeClean")
	WorkspaceEmptyEditorHint       = Key("workspace.empty.editorHint")
	WorkspaceEmptyNoFiles          = Key("workspace.empty.noFiles")
	WorkspaceLoadingFiles          = Key("workspace.loading.files")
	WorkspaceHintProject           = Key("workspace.hint.project")
	WorkspaceHintOpenFile          = Key("workspace.hint.openFile")
	WorkspaceProjectRoot           = Key("workspace.project.root")
	WorkspaceRepoOpenToLoad        = Key("workspace.repo.openToLoad")

	// The Project tool window's own menu and tooltips, where the wording differs
	// from the toolbar action it repeats.
	WorkspaceMenuOpen          = Key("workspace.menu.open")
	WorkspaceMenuExpand        = Key("workspace.menu.expand")
	WorkspaceMenuCollapse      = Key("workspace.menu.collapse")
	WorkspaceMenuHideProject   = Key("workspace.menu.hideProject")
	WorkspaceTipProjectOptions = Key("workspace.tip.projectOptions")

	// Toolbar actions and the menu items that repeat them.
	WorkspaceActionRefresh  = Key("workspace.action.refresh")
	WorkspaceActionHide     = Key("workspace.action.hide")
	WorkspaceActionShowDiff = Key("workspace.action.showDiff")
	WorkspaceActionRollback = Key("workspace.action.rollback")

	WorkspaceActionStageAll   = Key("workspace.action.stageAll")
	WorkspaceActionStageAllIn = Key("workspace.action.stageAllIn")
	WorkspaceActionUnstageAll = Key("workspace.action.unstageAll")

	WorkspaceActionGroupByDirectory = Key("workspace.action.groupByDirectory")
	WorkspaceActionExpandAll        = Key("workspace.action.expandAll")
	WorkspaceActionCollapseAll      = Key("workspace.action.collapseAll")

	WorkspaceActionStashChanges = Key("workspace.action.stashChanges")

	WorkspaceActionCopyPath         = Key("workspace.action.copyPath")
	WorkspaceActionCopyRelativePath = Key("workspace.action.copyRelativePath")

	// Tooltips, hints and the shortcut hints of the empty editor.
	WorkspaceTipResolveFirst        = Key("workspace.tip.resolveFirst")
	WorkspaceTipResolveBeforeCommit = Key("workspace.tip.resolveBeforeCommit")
	WorkspaceTipStageToCommit       = Key("workspace.tip.stageToCommit")
	WorkspaceTipNoChangesToCommit   = Key("workspace.tip.noChangesToCommit")
	WorkspaceTipEnterMessage        = Key("workspace.tip.enterMessage")
	WorkspaceTipCommitOptions       = Key("workspace.tip.commitOptions")
	WorkspaceTipCollapseMessage     = Key("workspace.tip.collapseMessage")
	WorkspaceTipExpandMessage       = Key("workspace.tip.expandMessage")
	WorkspaceTipAddRemoteToPush     = Key("workspace.tip.addRemoteToPush")
	WorkspaceTipAIGenerate          = Key("workspace.tip.aiGenerate")
	WorkspaceAIGenerating           = Key("workspace.ai.generating")
	WorkspaceAIGenerateFailed       = Key("workspace.ai.generateFailed")
	WorkspaceAINotConfigured        = Key("workspace.ai.notConfigured")
	WorkspaceAIOpenSettings         = Key("workspace.ai.openSettings")

	// The AI button's right-click menu picks the rules the generated message
	// follows, and edits the rules themselves.
	WorkspaceAIRulesTitle           = Key("workspace.aiRules.title")
	WorkspaceAIRulesNew             = Key("workspace.aiRules.new")
	WorkspaceAIRulesNewTitle        = Key("workspace.aiRules.newTitle")
	WorkspaceAIRulesCreate          = Key("workspace.aiRules.create")
	WorkspaceAIRulesEdit            = Key("workspace.aiRules.edit")
	WorkspaceAIRulesEditTitle       = Key("workspace.aiRules.editTitle")
	WorkspaceAIRulesSave            = Key("workspace.aiRules.save")
	WorkspaceAIRulesRename          = Key("workspace.aiRules.rename")
	WorkspaceAIRulesRenameTitle     = Key("workspace.aiRules.renameTitle")
	WorkspaceAIRulesDelete          = Key("workspace.aiRules.delete")
	WorkspaceAIRulesDeleteTitle     = Key("workspace.aiRules.deleteTitle")
	WorkspaceAIRulesDeleteBody      = Key("workspace.aiRules.deleteBody")
	WorkspaceAIRulesRestore         = Key("workspace.aiRules.restore")
	WorkspaceAIRulesBuiltinName     = Key("workspace.aiRules.builtinName")
	WorkspaceAIRulesName            = Key("workspace.aiRules.name")
	WorkspaceAIRulesNamePlaceholder = Key("workspace.aiRules.namePlaceholder")
	WorkspaceAIRulesRules           = Key("workspace.aiRules.rules")
	WorkspaceAIRulesPlaceholder     = Key("workspace.aiRules.placeholder")
	WorkspaceAIRulesNameRequired    = Key("workspace.aiRules.nameRequired")
	WorkspaceAIRulesTextRequired    = Key("workspace.aiRules.textRequired")
	WorkspaceAIRulesCannotSelect    = Key("workspace.aiRules.cannotSelect")
	WorkspaceAIRulesCannotDelete    = Key("workspace.aiRules.cannotDelete")
	WorkspaceAIRulesRightClickHint  = Key("workspace.aiRules.rightClickHint")
	WorkspaceAITipWithRule          = Key("workspace.ai.tipWithRule")
	WorkspaceAIConfirmToggle        = Key("workspace.ai.confirmToggle")
	WorkspaceAIConfirmAsk           = Key("workspace.ai.confirmAsk")
	WorkspaceAIConfirmGenerate      = Key("workspace.ai.confirmGenerate")

	// The commit scope above the buttons, and the prompt that offers to stage
	// checked files that are not staged yet. The scope depends on the backend:
	// Git commits the index, a backend without one commits the checked paths.
	WorkspaceCommitScopeHint      = Key("workspace.commit.scopeHint")
	WorkspaceCommitScopeHintPaths = Key("workspace.commit.scopeHintPaths")

	WorkspaceHintPush          = Key("workspace.hint.push")
	WorkspaceHintUpdateProject = Key("workspace.hint.updateProject")
	WorkspaceHintBranches      = Key("workspace.hint.branches")
	WorkspaceHintDiffSelection = Key("workspace.hint.diffSelection")

	// The banner of a merge, rebase, cherry-pick or revert in progress.
	WorkspaceBannerInProgress = Key("workspace.banner.inProgress")
	WorkspaceBannerRebasing   = Key("workspace.banner.rebasing")
	WorkspaceBannerMerging    = Key("workspace.banner.merging")

	// Buttons.
	WorkspaceButtonContinue = Key("workspace.button.continue")
	WorkspaceButtonSkip     = Key("workspace.button.skip")
	WorkspaceButtonAbort    = Key("workspace.button.abort")
	WorkspaceButtonRollback = Key("workspace.button.rollback")

	WorkspaceButtonCommit             = Key("workspace.button.commit")
	WorkspaceButtonAmendCommit        = Key("workspace.button.amendCommit")
	WorkspaceButtonCommitAndPush      = Key("workspace.button.commitAndPush")
	WorkspaceButtonAmendCommitAndPush = Key("workspace.button.amendCommitAndPush")
	WorkspaceButtonStageSelected      = Key("workspace.button.stageSelected")

	WorkspaceButtonClose = Key("workspace.button.close")

	// Context menu items.
	WorkspaceMenuCloseOtherTabs = Key("workspace.menu.closeOtherTabs")
	WorkspaceMenuCloseAllTabs   = Key("workspace.menu.closeAllTabs")
	WorkspaceMenuKeepTabOpen    = Key("workspace.menu.keepTabOpen")

	WorkspaceMenuUpdateSubmodule = Key("workspace.menu.updateSubmodule")
	WorkspaceMenuOpenSubmodule   = Key("workspace.menu.openSubmodule")
	WorkspaceMenuStage           = Key("workspace.menu.stage")
	WorkspaceMenuUnstage         = Key("workspace.menu.unstage")

	WorkspaceMenuMarkResolved     = Key("workspace.menu.markResolved")
	WorkspaceMenuMoveToChangeList = Key("workspace.menu.moveToChangeList")
	WorkspaceMenuAddToGitignore   = Key("workspace.menu.addToGitignore")

	WorkspaceMenuShowHistory    = Key("workspace.menu.showHistory")
	WorkspaceMenuAnnotate       = Key("workspace.menu.annotate")
	WorkspaceMenuOpenInEditor   = Key("workspace.menu.openInEditor")
	WorkspaceMenuRevealInFinder = Key("workspace.menu.revealInFinder")

	WorkspaceMenuCreatePatch  = Key("workspace.menu.createPatch")
	WorkspaceMenuCommitChecks = Key("workspace.menu.commitChecks")

	// Checkboxes of the commit options.
	WorkspaceCheckAmend           = Key("workspace.check.amend")
	WorkspaceCheckRunHooks        = Key("workspace.check.runHooks")
	WorkspaceCheckSignOff         = Key("workspace.check.signOff")
	WorkspaceCheckDeleteUntracked = Key("workspace.check.deleteUntracked")

	// Dialogs, confirmations and muted notes.
	WorkspaceDialogRollback = Key("workspace.dialog.rollback")

	WorkspaceConfirmAbort        = Key("workspace.confirm.abortTitle")
	WorkspaceConfirmAbortBody    = Key("workspace.confirm.abortBody")
	WorkspaceConfirmRollbackBody = Key("workspace.confirm.rollbackBody")

	WorkspaceMutedStagedReset = Key("workspace.muted.stagedReset")

	// Errors.
	WorkspaceErrorCannotCopy     = Key("workspace.error.cannotCopy")
	WorkspaceErrorCannotShowDiff = Key("workspace.error.cannotShowDiff")

	// Counts. The one form is used for one in English; Chinese has one form for
	// every count and repeats its text in the many form.
	WorkspaceCountFilesOne         = Key("workspace.count.files.one")
	WorkspaceCountFilesMany        = Key("workspace.count.files.many")
	WorkspaceCountRepositoriesOne  = Key("workspace.count.repositories.one")
	WorkspaceCountRepositoriesMany = Key("workspace.count.repositories.many")
	WorkspaceCountConflictsOne     = Key("workspace.count.conflicts.one")
	WorkspaceCountConflictsMany    = Key("workspace.count.conflicts.many")

	WorkspaceCountUnversionedOne  = Key("workspace.count.unversionedFiles.one")
	WorkspaceCountUnversionedMany = Key("workspace.count.unversionedFiles.many")

	WorkspaceCountNotStagedOne  = Key("workspace.count.notStaged.one")
	WorkspaceCountNotStagedMany = Key("workspace.count.notStaged.many")
)

func init() {
	register(EN, map[Key]string{
		WorkspacePanelCommit:  "Commit",
		WorkspacePanelStash:   "Stash",
		WorkspacePanelChanges: "Changes",

		WorkspaceAreaStaged:    "Staged",
		WorkspaceAreaChanges:   "Changes",
		WorkspaceAreaUntracked: "Unversioned Files",
		WorkspaceAreaConflicts: "Merge Conflicts",

		WorkspaceStashChanges:        "Stash Changes…",
		WorkspaceStashApply:          "Apply Stash",
		WorkspaceStashPop:            "Pop Stash (apply and drop)",
		WorkspaceStashDrop:           "Drop Stash…",
		WorkspaceStashAsBranch:       "Unstash as Branch…",
		WorkspaceStashEmpty:          "No stashes",
		WorkspaceStashEmptyHint:      "Stash changes to set them aside and get back to them later",
		WorkspaceStashList:           "Stashes",
		WorkspaceStashApplyIndex:     "Apply with Index",
		WorkspaceStashPopMenu:        "Pop Stash",
		WorkspaceStashCopyRev:        "Copy Revision Number",
		WorkspaceStashDropMenu:       "Drop…",
		WorkspaceStashDropTitle:      "Drop Stash",
		WorkspaceStashDropBody:       "Drop %s \"%s\"? You can still find its commit in the reflog for a while, but not in Viff.",
		WorkspaceStashDropButton:     "Drop",
		WorkspaceSmartCheckoutTitle:  "Stash Changes and Checkout",
		WorkspaceSmartCheckoutBody:   "Your local changes would be overwritten by checking out %s.\n\nViff can stash them, including untracked files, check out, and restore them afterwards. If restoring them conflicts, they stay in the stash.",
		WorkspaceSmartCheckoutButton: "Stash and Checkout",

		WorkspaceDropCommitTitle: "Drop Commit",
		WorkspaceDropCommitBody:  "Drop %s \"%s\"?\n\nIts changes disappear from the branch, and the commits after it are replayed without them. The old commits stay in the reflog until it expires.",
		WorkspaceStashBranchName: "Branch name:",

		WorkspaceLabelCommitMessage: "Commit message",
		WorkspaceLabelSubmodule:     "submodule",
		WorkspaceLabelAndMore:       "and %d more",

		WorkspaceStateLoading: "Loading…",

		WorkspaceEmptyNoChanges:        "No changes",
		WorkspaceEmptyWorkingTreeClean: "Working tree is clean",
		WorkspaceEmptyEditorHint:       "Select a file in the Project panel, the Commit tool window or the Git log to see its changes",
		WorkspaceEmptyNoFiles:          "No files in the working tree",
		WorkspaceLoadingFiles:          "Loading files…",
		WorkspaceHintProject:           "Project",
		WorkspaceHintOpenFile:          "Open File",
		WorkspaceProjectRoot:           "Workspace",
		WorkspaceRepoOpenToLoad:        "Open to load status",

		WorkspaceMenuOpen:          "Open",
		WorkspaceMenuExpand:        "Expand",
		WorkspaceMenuCollapse:      "Collapse",
		WorkspaceMenuHideProject:   "Hide Project",
		WorkspaceTipProjectOptions: "Project options",

		WorkspaceActionRefresh:  "Refresh",
		WorkspaceActionHide:     "Hide",
		WorkspaceActionShowDiff: "Show Diff",
		WorkspaceActionRollback: "Rollback…",

		WorkspaceActionStageAll:   "Stage All",
		WorkspaceActionStageAllIn: "Stage All in %s",
		WorkspaceActionUnstageAll: "Unstage All",

		WorkspaceActionGroupByDirectory: "Group by Directory",
		WorkspaceActionExpandAll:        "Expand All",
		WorkspaceActionCollapseAll:      "Collapse All",

		WorkspaceActionStashChanges: "Stash Changes…",

		WorkspaceActionCopyPath:         "Copy Path",
		WorkspaceActionCopyRelativePath: "Copy Relative Path",

		WorkspaceTipResolveFirst:        "Resolve and stage all conflicts first",
		WorkspaceTipResolveBeforeCommit: "Resolve conflicts before committing",
		WorkspaceTipStageToCommit:       "Stage changes to commit",
		WorkspaceTipNoChangesToCommit:   "There are no changes to commit",
		WorkspaceTipEnterMessage:        "Enter a commit message",
		WorkspaceTipCommitOptions:       "Commit Options",
		WorkspaceTipCollapseMessage:     "Collapse the commit message",
		WorkspaceTipExpandMessage:       "Expand the commit message",
		WorkspaceTipAddRemoteToPush:     "Add a remote to push",
		WorkspaceTipAIGenerate:          "Write the message with AI",
		WorkspaceAIGenerating:           "Writing the message…",
		WorkspaceAIGenerateFailed:       "The AI could not write the message",
		WorkspaceAINotConfigured:        "Set the AI endpoint and model in the settings first",
		WorkspaceAIOpenSettings:         "Open settings…",

		WorkspaceAIRulesTitle:           "Commit message rules",
		WorkspaceAIRulesNew:             "New Rules…",
		WorkspaceAIRulesNewTitle:        "New Commit Message Rules",
		WorkspaceAIRulesCreate:          "Create",
		WorkspaceAIRulesEdit:            "Edit Rules…",
		WorkspaceAIRulesEditTitle:       "Edit Commit Message Rules",
		WorkspaceAIRulesSave:            "Save",
		WorkspaceAIRulesRename:          "Rename…",
		WorkspaceAIRulesRenameTitle:     "Rename Rules",
		WorkspaceAIRulesDelete:          "Delete…",
		WorkspaceAIRulesDeleteTitle:     "Delete Rules",
		WorkspaceAIRulesDeleteBody:      "Delete the rules \"%s\"? Repositories that use them go back to the built-in rules. Your other rules are kept.",
		WorkspaceAIRulesRestore:         "Use Built-in Rules",
		WorkspaceAIRulesBuiltinName:     "Default (read-only)",
		WorkspaceAIRulesName:            "Name:",
		WorkspaceAIRulesNamePlaceholder: "Enter a name",
		WorkspaceAIRulesRules:           "Rules:",
		WorkspaceAIRulesPlaceholder:     "Instructions the model receives: the format, the wording, what to ignore.",
		WorkspaceAIRulesNameRequired:    "A name is required.",
		WorkspaceAIRulesTextRequired:    "The rules text cannot be empty.",
		WorkspaceAIRulesCannotSelect:    "Cannot change the commit message rules",
		WorkspaceAIRulesCannotDelete:    "Cannot delete the commit message rules",
		WorkspaceAIRulesRightClickHint:  "AI commit message; right-click to pick the rules",
		WorkspaceAITipWithRule:          "AI commit message · rules: %s; right-click to pick the rules",
		WorkspaceAIConfirmToggle:        "Ask before generating",
		WorkspaceAIConfirmAsk:           "Generate a commit message with AI?",
		WorkspaceAIConfirmGenerate:      "Generate",

		WorkspaceCommitScopeHint:      "Commit includes only staged files",
		WorkspaceCommitScopeHintPaths: "Commit includes the checked files",

		WorkspaceHintPush:          "Push",
		WorkspaceHintUpdateProject: "Update Project",
		WorkspaceHintBranches:      "Branches",
		WorkspaceHintDiffSelection: "Show Diff of Selection",

		WorkspaceBannerInProgress: "%s in progress",
		WorkspaceBannerRebasing:   "Rebasing %s onto %s",
		WorkspaceBannerMerging:    "Merging into %s",

		WorkspaceButtonContinue: "Continue",
		WorkspaceButtonSkip:     "Skip",
		WorkspaceButtonAbort:    "Abort",
		WorkspaceButtonRollback: "Rollback",

		WorkspaceButtonCommit:             "Commit",
		WorkspaceButtonAmendCommit:        "Amend Commit",
		WorkspaceButtonCommitAndPush:      "Commit and Push…",
		WorkspaceButtonAmendCommitAndPush: "Amend Commit and Push…",
		WorkspaceButtonStageSelected:      "Stage Selected",

		WorkspaceButtonClose: "Close",

		WorkspaceMenuCloseOtherTabs: "Close Other Tabs",
		WorkspaceMenuCloseAllTabs:   "Close All Tabs",
		WorkspaceMenuKeepTabOpen:    "Keep Tab Open",

		WorkspaceMenuUpdateSubmodule: "Update Submodule",
		WorkspaceMenuOpenSubmodule:   "Open Submodule",
		WorkspaceMenuStage:           "Stage",
		WorkspaceMenuUnstage:         "Unstage",

		WorkspaceMenuMarkResolved:     "Mark Resolved",
		WorkspaceMenuMoveToChangeList: "Move to Change List",
		WorkspaceMenuAddToGitignore:   "Add to .gitignore",

		WorkspaceMenuShowHistory:    "Show History",
		WorkspaceMenuAnnotate:       "Annotate with Git Blame",
		WorkspaceMenuOpenInEditor:   "Open in External Editor",
		WorkspaceMenuRevealInFinder: revealLabel(EN),

		WorkspaceMenuCreatePatch:  "Create Patch…",
		WorkspaceMenuCommitChecks: "Commit Checks",

		WorkspaceCheckAmend:           "Amend",
		WorkspaceCheckRunHooks:        "Run Git hooks",
		WorkspaceCheckSignOff:         "Sign-off commit",
		WorkspaceCheckDeleteUntracked: "Delete %s from disk",

		WorkspaceDialogRollback: "Rollback Changes",

		WorkspaceConfirmAbort:        "Abort %s",
		WorkspaceConfirmAbortBody:    "Abort the %s and restore the state before it started? Changes made while resolving are lost.",
		WorkspaceConfirmRollbackBody: "Discard local changes in %s? This cannot be undone.",

		WorkspaceMutedStagedReset: "Staged changes are reset to HEAD.",

		WorkspaceErrorCannotCopy:     "Cannot copy",
		WorkspaceErrorCannotShowDiff: "Cannot show diff",

		WorkspaceCountFilesOne:         "%d file",
		WorkspaceCountFilesMany:        "%d files",
		WorkspaceCountRepositoriesOne:  "%d repository",
		WorkspaceCountRepositoriesMany: "%d repositories",
		WorkspaceCountConflictsOne:     "%d conflict",
		WorkspaceCountConflictsMany:    "%d conflicts",

		WorkspaceCountUnversionedOne:  "%d unversioned file",
		WorkspaceCountUnversionedMany: "%d unversioned files",

		WorkspaceCountNotStagedOne:  "%d selected file is not staged",
		WorkspaceCountNotStagedMany: "%d selected files are not staged",
	})

	register(ZHCN, map[Key]string{
		WorkspacePanelCommit:  "提交",
		WorkspacePanelStash:   "储藏",
		WorkspacePanelChanges: "变更",

		WorkspaceAreaStaged:    "已暂存",
		WorkspaceAreaChanges:   "变更",
		WorkspaceAreaUntracked: "未跟踪文件",
		WorkspaceAreaConflicts: "合并冲突",

		WorkspaceStashChanges:        "储藏更改…",
		WorkspaceStashApply:          "应用储藏",
		WorkspaceStashPop:            "弹出储藏（应用并删除）",
		WorkspaceStashDrop:           "删除储藏…",
		WorkspaceStashAsBranch:       "从储藏创建分支…",
		WorkspaceStashEmpty:          "没有储藏",
		WorkspaceStashEmptyHint:      "把当前更改储藏起来，稍后再处理",
		WorkspaceStashList:           "储藏",
		WorkspaceStashApplyIndex:     "应用（含索引）",
		WorkspaceStashPopMenu:        "弹出储藏",
		WorkspaceStashCopyRev:        "复制修订号",
		WorkspaceStashDropMenu:       "删除…",
		WorkspaceStashDropTitle:      "删除储藏",
		WorkspaceStashDropBody:       "删除 %s \"%s\"？一段时间内仍可在引用日志里找到它的提交，但 Viff 里不再显示。",
		WorkspaceStashDropButton:     "删除",
		WorkspaceSmartCheckoutTitle:  "储藏更改并检出",
		WorkspaceSmartCheckoutBody:   "检出 %s 会覆盖你的本地更改。\n\nViff 可以把这些更改（含未跟踪文件）储藏起来，检出后再恢复；若恢复时冲突，它们会留在储藏里。",
		WorkspaceSmartCheckoutButton: "储藏并检出",

		WorkspaceDropCommitTitle: "丢弃提交",
		WorkspaceDropCommitBody:  "丢弃 %s \"%s\"？\n\n它的更改会从分支上消失，其后的提交会在去掉这些更改后重放。旧提交会留在引用日志里直到过期。",
		WorkspaceStashBranchName: "分支名：",

		WorkspaceLabelCommitMessage: "提交说明",
		WorkspaceLabelSubmodule:     "子模块",
		WorkspaceLabelAndMore:       "以及另外 %d 个",

		WorkspaceStateLoading: "加载中…",

		WorkspaceEmptyNoChanges:        "没有变更",
		WorkspaceEmptyWorkingTreeClean: "工作树是干净的",
		WorkspaceEmptyEditorHint:       "在项目面板、提交工具窗口或 Git 日志中选择一个文件，即可查看它的改动",
		WorkspaceEmptyNoFiles:          "工作树里没有文件",
		WorkspaceLoadingFiles:          "正在加载文件…",
		WorkspaceHintProject:           "项目",
		WorkspaceHintOpenFile:          "打开文件",
		WorkspaceProjectRoot:           "工作区",
		WorkspaceRepoOpenToLoad:        "打开后加载状态",

		WorkspaceMenuOpen:          "打开",
		WorkspaceMenuExpand:        "展开",
		WorkspaceMenuCollapse:      "折叠",
		WorkspaceMenuHideProject:   "隐藏项目",
		WorkspaceTipProjectOptions: "项目选项",

		WorkspaceActionRefresh:  "刷新",
		WorkspaceActionHide:     "隐藏",
		WorkspaceActionShowDiff: "显示 diff",
		WorkspaceActionRollback: "回滚…",

		WorkspaceActionStageAll:   "全部暂存",
		WorkspaceActionStageAllIn: "在 %s 中全部暂存",
		WorkspaceActionUnstageAll: "全部取消暂存",

		WorkspaceActionGroupByDirectory: "按目录分组",
		WorkspaceActionExpandAll:        "全部展开",
		WorkspaceActionCollapseAll:      "全部折叠",

		WorkspaceActionStashChanges: "储藏更改…",

		WorkspaceActionCopyPath:         "复制路径",
		WorkspaceActionCopyRelativePath: "复制相对路径",

		WorkspaceTipResolveFirst:        "请先解决并暂存全部冲突",
		WorkspaceTipResolveBeforeCommit: "提交前请先解决冲突",
		WorkspaceTipStageToCommit:       "暂存改动后再提交",
		WorkspaceTipNoChangesToCommit:   "没有可提交的改动",
		WorkspaceTipEnterMessage:        "请输入提交说明",
		WorkspaceTipCommitOptions:       "提交选项",
		WorkspaceTipCollapseMessage:     "折叠提交说明",
		WorkspaceTipExpandMessage:       "展开提交说明",
		WorkspaceTipAddRemoteToPush:     "添加远端后才能推送",
		WorkspaceTipAIGenerate:          "用 AI 生成提交说明",
		WorkspaceAIGenerating:           "正在生成提交说明…",
		WorkspaceAIGenerateFailed:       "AI 没能生成提交说明",
		WorkspaceAINotConfigured:        "请先在设置里填写 AI 端点和模型",
		WorkspaceAIOpenSettings:         "打开设置…",

		WorkspaceAIRulesTitle:           "提交信息规则",
		WorkspaceAIRulesNew:             "新建规则…",
		WorkspaceAIRulesNewTitle:        "新建提交信息规则",
		WorkspaceAIRulesCreate:          "创建",
		WorkspaceAIRulesEdit:            "编辑规则…",
		WorkspaceAIRulesEditTitle:       "编辑提交信息规则",
		WorkspaceAIRulesSave:            "保存",
		WorkspaceAIRulesRename:          "重命名…",
		WorkspaceAIRulesRenameTitle:     "重命名规则",
		WorkspaceAIRulesDelete:          "删除…",
		WorkspaceAIRulesDeleteTitle:     "删除规则",
		WorkspaceAIRulesDeleteBody:      "删除规则“%s”？使用它的仓库会回到内置规则，你的其他规则不受影响。",
		WorkspaceAIRulesRestore:         "使用内置规则",
		WorkspaceAIRulesBuiltinName:     "默认（不可修改）",
		WorkspaceAIRulesName:            "名称：",
		WorkspaceAIRulesNamePlaceholder: "输入名称",
		WorkspaceAIRulesRules:           "规则：",
		WorkspaceAIRulesPlaceholder:     "模型收到的指令：格式、措辞、需要忽略的内容。",
		WorkspaceAIRulesNameRequired:    "名称不能为空。",
		WorkspaceAIRulesTextRequired:    "规则内容不能为空。",
		WorkspaceAIRulesCannotSelect:    "无法切换提交信息规则",
		WorkspaceAIRulesCannotDelete:    "无法删除提交信息规则",
		WorkspaceAIRulesRightClickHint:  "AI 生成提交信息；右键选择规则",
		WorkspaceAITipWithRule:          "AI 生成提交信息 · 当前规则：%s；右键选择规则",
		WorkspaceAIConfirmToggle:        "生成前先确认",
		WorkspaceAIConfirmAsk:           "确认使用 AI 生成提交说明？",
		WorkspaceAIConfirmGenerate:      "生成",

		WorkspaceCommitScopeHint:      "提交只包含已暂存的文件",
		WorkspaceCommitScopeHintPaths: "提交包含勾选的文件",

		WorkspaceHintPush:          "推送",
		WorkspaceHintUpdateProject: "更新项目",
		WorkspaceHintBranches:      "分支",
		WorkspaceHintDiffSelection: "显示所选内容的 diff",

		WorkspaceBannerInProgress: "%s 进行中",
		WorkspaceBannerRebasing:   "正在把 %s 变基到 %s",
		WorkspaceBannerMerging:    "正在合并到 %s",

		WorkspaceButtonContinue: "继续",
		WorkspaceButtonSkip:     "跳过",
		WorkspaceButtonAbort:    "中止",
		WorkspaceButtonRollback: "回滚",

		WorkspaceButtonCommit:             "提交",
		WorkspaceButtonAmendCommit:        "修正提交",
		WorkspaceButtonCommitAndPush:      "提交并推送…",
		WorkspaceButtonAmendCommitAndPush: "修正提交并推送…",
		WorkspaceButtonStageSelected:      "暂存所选",

		WorkspaceButtonClose: "关闭",

		WorkspaceMenuCloseOtherTabs: "关闭其他页签",
		WorkspaceMenuCloseAllTabs:   "关闭全部页签",
		WorkspaceMenuKeepTabOpen:    "保持页签打开",

		WorkspaceMenuUpdateSubmodule: "更新子模块",
		WorkspaceMenuOpenSubmodule:   "打开子模块",
		WorkspaceMenuStage:           "暂存",
		WorkspaceMenuUnstage:         "取消暂存",

		WorkspaceMenuMarkResolved:     "标记为已解决",
		WorkspaceMenuMoveToChangeList: "移到变更列表",
		WorkspaceMenuAddToGitignore:   "添加到 .gitignore",

		WorkspaceMenuShowHistory:    "显示历史",
		WorkspaceMenuAnnotate:       "用 Git blame 注解",
		WorkspaceMenuOpenInEditor:   "用外部编辑器打开",
		WorkspaceMenuRevealInFinder: revealLabel(ZHCN),

		WorkspaceMenuCreatePatch:  "创建补丁…",
		WorkspaceMenuCommitChecks: "提交检查",

		WorkspaceCheckAmend:           "修正",
		WorkspaceCheckRunHooks:        "运行 Git 钩子",
		WorkspaceCheckSignOff:         "添加 sign-off",
		WorkspaceCheckDeleteUntracked: "从磁盘删除 %s",

		WorkspaceDialogRollback: "回滚变更",

		WorkspaceConfirmAbort:        "中止 %s",
		WorkspaceConfirmAbortBody:    "中止 %s 并恢复到它开始前的状态？解决冲突期间的改动会丢失。",
		WorkspaceConfirmRollbackBody: "丢弃 %s 中的本地改动？此操作无法撤销。",

		WorkspaceMutedStagedReset: "已暂存的改动会重置到 HEAD。",

		WorkspaceErrorCannotCopy:     "无法复制",
		WorkspaceErrorCannotShowDiff: "无法显示 diff",

		WorkspaceCountFilesOne:         "%d 个文件",
		WorkspaceCountFilesMany:        "%d 个文件",
		WorkspaceCountRepositoriesOne:  "%d 个仓库",
		WorkspaceCountRepositoriesMany: "%d 个仓库",
		WorkspaceCountConflictsOne:     "%d 处冲突",
		WorkspaceCountConflictsMany:    "%d 处冲突",

		WorkspaceCountUnversionedOne:  "%d 个未跟踪文件",
		WorkspaceCountUnversionedMany: "%d 个未跟踪文件",

		WorkspaceCountNotStagedOne:  "%d 个已勾选的文件尚未暂存",
		WorkspaceCountNotStagedMany: "%d 个已勾选的文件尚未暂存",
	})
}
