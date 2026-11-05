package i18n

// Message keys of the worktree and submodule windows.
const (
	WorktreeTitle             = Key("worktree.title")
	WorktreeNewTitle          = Key("worktree.newTitle")
	WorktreeDeleteTitle       = Key("worktree.deleteTitle")
	WorktreeCannotList        = Key("worktree.cannotList")
	WorktreeDoubleClickHint   = Key("worktree.doubleClick")
	WorktreeNewCheckoutHint   = Key("worktree.newCheckoutHint")
	WorktreeNewBranchHint     = Key("worktree.newBranchHint")
	WorktreeBranchName        = Key("worktree.branchName")
	WorktreeBranch            = Key("worktree.branch")
	WorktreePath              = Key("worktree.path")
	WorktreeStartPoint        = Key("worktree.startPoint")
	WorktreeSelectBranch      = Key("worktree.selectBranch")
	WorktreePathRequired      = Key("worktree.pathRequired")
	WorktreePathAbsolute      = Key("worktree.pathAbsolute")
	WorktreeThisCheckout      = Key("worktree.thisCheckout")
	WorktreeAlreadyCheckedOut = Key("worktree.alreadyCheckedOut")
	WorktreeNotLocal          = Key("worktree.notLocal")
	WorktreeNoBranches        = Key("worktree.noBranches")
	WorktreeEveryBranch       = Key("worktree.everyBranch")
	WorktreeDeleteQuestion    = Key("worktree.deleteQuestion")
	WorktreeDeleteHint        = Key("worktree.deleteHint")
	WorktreeForceDelete       = Key("worktree.forceDelete")

	SubmoduleTitle          = Key("submodule.title")
	SubmoduleMenu           = Key("submodule.menu")
	SubmoduleCannotOpen     = Key("submodule.cannotOpen")
	SubmoduleLoading        = Key("submodule.loading")
	SubmoduleNone           = Key("submodule.none")
	SubmoduleAddHint        = Key("submodule.addHint")
	SubmoduleMore           = Key("submodule.more")
	SubmoduleCannotRead     = Key("submodule.cannotRead")
	SubmoduleNestedHint     = Key("submodule.nestedHint")
	SubmoduleCannotReadOne  = Key("submodule.cannotReadOne")
	SubmoduleRemoveQuestion = Key("submodule.removeQuestion")
	SubmoduleRemoveHint     = Key("submodule.removeHint")
	SubmoduleDeinitQuestion = Key("submodule.deinitQuestion")
	SubmoduleDeinitHint     = Key("submodule.deinitHint")
	SubmoduleForceHint      = Key("submodule.forceHint")
	SubmoduleAddTitle       = Key("submodule.addTitle")
	SubmoduleAddBody        = Key("submodule.addBody")
	SubmoduleLocalPathHint  = Key("submodule.localPathHint")
)

func init() {
	register(EN, map[Key]string{
		WorktreeTitle:             "Worktrees",
		WorktreeNewTitle:          "New Worktree",
		WorktreeDeleteTitle:       "Delete Worktree",
		WorktreeCannotList:        "Cannot list worktrees",
		WorktreeDoubleClickHint:   "Double-click a worktree to open it.",
		WorktreeNewCheckoutHint:   "Checks out a local branch that no worktree has yet. A branch can be checked out in one worktree at a time.",
		WorktreeNewBranchHint:     "Creates the branch from the start point and checks it out in a new worktree.",
		WorktreeBranchName:        "Branch name:",
		WorktreeBranch:            "Branch:",
		WorktreePath:              "Path:",
		WorktreeStartPoint:        "Start point:",
		WorktreeSelectBranch:      "Select a local branch.",
		WorktreePathRequired:      "Enter a path for the worktree.",
		WorktreePathAbsolute:      "Use an absolute path, for example ~/Projects/repo-branch.",
		WorktreeThisCheckout:      "this checkout",
		WorktreeAlreadyCheckedOut: "Branch '%s' is already checked out in %s. A branch can be checked out in one worktree at a time; open that worktree instead.",
		WorktreeNotLocal:          "Branch '%s' is not a local branch any more. Pick another one.",
		WorktreeNoBranches:        "There are no local branches to check out. Create a new branch instead.",
		WorktreeEveryBranch:       "Every local branch is already checked out in a worktree. Create a new branch instead.",
		WorktreeDeleteQuestion:    "Delete the worktree at %s?",
		WorktreeDeleteHint:        "The branch '%s' and its commits stay in the repository. Uncommitted changes in that worktree are lost.",
		WorktreeForceDelete:       "Force delete (discard modified and untracked files)",

		SubmoduleTitle:          "Submodules",
		SubmoduleMenu:           "Submodules…",
		SubmoduleCannotOpen:     "Cannot open submodule",
		SubmoduleLoading:        "Loading submodules…",
		SubmoduleNone:           "This repository has no submodules.",
		SubmoduleAddHint:        "Add one with Add Submodule…; Git records it in .gitmodules and stages the change.",
		SubmoduleMore:           "%s more submodules are not shown. Update All still applies to them.",
		SubmoduleCannotRead:     "Cannot read .gitmodules (%s), so Deinit and Remove are unavailable.",
		SubmoduleNestedHint:     "A nested submodule belongs to the repository that records it; deinit and remove it there, in that repository's own Submodules dialog.",
		SubmoduleCannotReadOne:  "Cannot read submodule %s",
		SubmoduleRemoveQuestion: "Remove the submodule '%s'?",
		SubmoduleRemoveHint:     "Its working tree and its entry in .gitmodules are removed together, and the change is staged. Commits in the submodule's own repository are not deleted, so it can be added again.",
		SubmoduleDeinitQuestion: "Deinit the submodule '%s'?",
		SubmoduleDeinitHint:     "Its working tree is emptied, but its entry in .gitmodules and the recorded commit stay. Update checks it out again.",
		SubmoduleForceHint:      "Git needs force here: local changes in the submodule, or a staged change to its recorded commit, are discarded. This cannot be undone.",
		SubmoduleAddTitle:       "Add Submodule",
		SubmoduleAddBody:        "Git writes .gitmodules and stages the submodule. Commit the change when you are ready.",
		SubmoduleLocalPathHint:  "A local path needs the file transport; configure that in Git if you intend to use one.",
	})
	register(ZHCN, map[Key]string{
		WorktreeTitle:             "工作树",
		WorktreeNewTitle:          "新建工作树",
		WorktreeDeleteTitle:       "删除工作树",
		WorktreeCannotList:        "无法列出工作树",
		WorktreeDoubleClickHint:   "双击工作树即可打开。",
		WorktreeNewCheckoutHint:   "检出一个还没有工作树的本地分支。同一分支同一时间只能在一个工作树里检出。",
		WorktreeNewBranchHint:     "从起点创建分支，并在新的工作树里检出。",
		WorktreeBranchName:        "分支名：",
		WorktreeBranch:            "分支：",
		WorktreePath:              "路径：",
		WorktreeStartPoint:        "起点：",
		WorktreeSelectBranch:      "请选择一个本地分支。",
		WorktreePathRequired:      "请填写工作树的路径。",
		WorktreePathAbsolute:      "请使用绝对路径，例如 ~/Projects/repo-branch。",
		WorktreeThisCheckout:      "当前检出",
		WorktreeAlreadyCheckedOut: "分支 '%s' 已在 %s 检出。同一分支同一时间只能在一个工作树里检出，请改为打开那个工作树。",
		WorktreeNotLocal:          "分支 '%s' 已不是本地分支，请另选一个。",
		WorktreeNoBranches:        "没有可检出的本地分支，请改为新建分支。",
		WorktreeEveryBranch:       "所有本地分支都已在一个工作树里检出，请改为新建分支。",
		WorktreeDeleteQuestion:    "删除位于 %s 的工作树？",
		WorktreeDeleteHint:        "分支 '%s' 及其提交仍保留在仓库里；该工作树中未提交的更改会丢失。",
		WorktreeForceDelete:       "强制删除（丢弃已修改与未跟踪的文件）",

		SubmoduleTitle:          "子模块",
		SubmoduleMenu:           "子模块…",
		SubmoduleCannotOpen:     "无法打开子模块",
		SubmoduleLoading:        "正在加载子模块…",
		SubmoduleNone:           "这个仓库还没有子模块。",
		SubmoduleAddHint:        "可以用「添加子模块…」加入；Git 会把它记录到 .gitmodules 并暂存这次改动。",
		SubmoduleMore:           "还有 %s 个子模块未显示，「全部更新」仍会对它们生效。",
		SubmoduleCannotRead:     "无法读取 .gitmodules（%s），因此「停用」与「移除」不可用。",
		SubmoduleNestedHint:     "嵌套子模块属于记录它的那个仓库，请到那个仓库的子模块对话框里停用或移除。",
		SubmoduleCannotReadOne:  "无法读取子模块 %s",
		SubmoduleRemoveQuestion: "移除子模块 '%s'？",
		SubmoduleRemoveHint:     "它的工作树与 .gitmodules 中的记录会被一起移除，并暂存这次改动。子模块自身仓库里的提交不会被删除，之后可以重新添加。",
		SubmoduleDeinitQuestion: "停用子模块 '%s'？",
		SubmoduleDeinitHint:     "它的工作树会被清空，但 .gitmodules 中的记录与所记录的提交仍然保留；执行「更新」会重新检出。",
		SubmoduleForceHint:      "这里需要强制：子模块里的本地更改，或对记录提交的已暂存改动会被丢弃，且无法撤销。",
		SubmoduleAddTitle:       "添加子模块",
		SubmoduleAddBody:        "Git 会写入 .gitmodules 并暂存该子模块；准备好后提交这次改动。",
		SubmoduleLocalPathHint:  "本地路径需要 file 传输协议，如果你要用，请在 Git 里配置。",
	})
}
