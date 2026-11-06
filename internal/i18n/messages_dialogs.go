package i18n

// Message keys of the dialogs and the project menu that opens them.
const (
	MenuSettings      = Key("dialog.menu.settings")
	MenuTheme         = Key("dialog.menu.theme")
	MenuManageRemotes = Key("dialog.menu.manageRemotes")
	MenuFetch         = Key("dialog.menu.fetch")

	// Shared field labels and buttons.
	DialogName            = Key("dialog.field.name")
	DialogURL             = Key("dialog.field.url")
	DialogDirectory       = Key("dialog.field.directory")
	DialogBrowse          = Key("dialog.button.browse")
	DialogCreate          = Key("dialog.button.create")
	DialogRename          = Key("dialog.button.rename")
	DialogSet             = Key("dialog.button.set")
	DialogCreateTag       = Key("dialog.button.createTag")
	DialogCheckout        = Key("dialog.button.checkout")
	DialogReset           = Key("dialog.button.reset")
	DialogPush            = Key("dialog.button.push")
	DialogCreateStash     = Key("dialog.button.createStash")
	DialogClone           = Key("dialog.button.clone")
	DialogDelete          = Key("dialog.button.delete")
	DialogAdd             = Key("dialog.button.add")
	DialogRemove          = Key("dialog.remotes.remove")
	DialogAddRemote       = Key("dialog.remotes.add")
	DialogAddRemoteButton = Key("dialog.remotes.addButton")

	// Branch dialogs.
	DialogNewBranchTitle        = Key("dialog.branch.title")
	DialogNewBranchFrom         = Key("dialog.branch.titleFrom")
	DialogNewBranchName         = Key("dialog.branch.name")
	DialogCheckoutBranch        = Key("dialog.branch.checkout")
	DialogOverwriteBranch       = Key("dialog.branch.overwrite")
	DialogBranchExistsOverwrite = Key("dialog.branch.existsOverwrite")
	DialogRenameTitle           = Key("dialog.rename.title")
	DialogNewName               = Key("dialog.rename.name")
	DialogBranchExists          = Key("dialog.rename.exists")
	DialogSetUpstreamTitle      = Key("dialog.upstream.title")
	DialogTrackRemote           = Key("dialog.upstream.track")

	// Tag dialog.
	DialogCreateTagTitle = Key("dialog.tag.title")
	DialogTagName        = Key("dialog.tag.name")
	DialogTagMessage     = Key("dialog.tag.message")

	// Checkout revision.
	DialogCheckoutRevTitle = Key("dialog.rev.title")
	DialogRevField         = Key("dialog.rev.field")
	DialogDetachedHint     = Key("dialog.rev.detached")

	// Reset.
	DialogResetTitle     = Key("dialog.reset.title")
	DialogResetBody      = Key("dialog.reset.body")
	DialogResetWarning   = Key("dialog.reset.warning")
	DialogResetSoft      = Key("dialog.reset.soft")
	DialogResetMixed     = Key("dialog.reset.mixed")
	DialogResetHard      = Key("dialog.reset.hard")
	DialogResetKeep      = Key("dialog.reset.keep")
	DialogResetSoftDesc  = Key("dialog.reset.softDesc")
	DialogResetMixedDesc = Key("dialog.reset.mixedDesc")
	DialogResetHardDesc  = Key("dialog.reset.hardDesc")
	DialogResetKeepDesc  = Key("dialog.reset.keepDesc")

	NotifyNothingToPushTitle = Key("dialog.notify.nothingToPush")
	NotifyNothingToPushBody  = Key("dialog.notify.nothingToPushBody")
	NotifyNoRemoteTitle      = Key("dialog.notify.noRemote")
	NotifyNoRemoteBody       = Key("dialog.notify.noRemoteBody")
	NotifyCannotUpdateTitle  = Key("dialog.notify.cannotUpdate")
	NotifyCannotUpdateBody   = Key("dialog.notify.cannotUpdateBody")

	// Push.
	DialogPushTitle        = Key("dialog.push.title")
	DialogRemoteBranch     = Key("dialog.push.remoteBranch")
	DialogNewLabel         = Key("dialog.push.new")
	DialogLoadingCommits   = Key("dialog.push.loading")
	DialogNothingToPush    = Key("dialog.push.nothing")
	DialogWillBePushedOne  = Key("dialog.push.countOne")
	DialogWillBePushedMany = Key("dialog.push.countMany")
	DialogPushTags         = Key("dialog.push.tags")
	DialogPushForce        = Key("dialog.push.force")
	DialogForcePushTitle   = Key("dialog.push.forceTitle")
	DialogForcePushBody    = Key("dialog.push.forceBody")

	// Update project.
	DialogUpdateTitle   = Key("dialog.update.title")
	DialogUpdateType    = Key("dialog.update.type")
	DialogPullMerge     = Key("dialog.update.merge")
	DialogPullRebase    = Key("dialog.update.rebase")
	DialogPullFFOnly    = Key("dialog.update.ffOnly")
	DialogUpdateBody    = Key("dialog.update.body")
	DialogUpdateDontAsk = Key("dialog.update.dontAsk")

	// Stash.
	DialogStashTitle       = Key("dialog.stash.title")
	DialogBranchLabel      = Key("dialog.stash.branch")
	DialogMessage          = Key("dialog.stash.message")
	DialogOptional         = Key("dialog.stash.optional")
	DialogIncludeUntracked = Key("dialog.stash.untracked")
	DialogKeepStaged       = Key("dialog.stash.keep")

	// Clone and init.
	DialogCloneTitle = Key("dialog.clone.title")
	DialogParentDir  = Key("dialog.clone.parent")
	DialogCloneInto  = Key("dialog.clone.into")
	DialogClonesInto = Key("dialog.clone.result")
	DialogExists     = Key("dialog.clone.exists")
	DialogInitTitle  = Key("dialog.init.title")

	DialogRemotesTitle      = Key("dialog.remotes.title")
	DialogNoRemotes         = Key("dialog.remotes.empty")
	DialogRemoveRemoteTitle = Key("dialog.remotes.removeTitle")
	DialogRemoveRemoteBody  = Key("dialog.remotes.removeBody")
)

func init() {
	register(EN, map[Key]string{
		MenuSettings:      "Settings…",
		MenuTheme:         "Theme",
		MenuManageRemotes: "Manage Remotes…",
		MenuFetch:         "Fetch",

		DialogName:            "Name:",
		DialogURL:             "URL:",
		DialogDirectory:       "Directory:",
		DialogBrowse:          "Browse…",
		DialogCreate:          "Create",
		DialogRename:          "Rename",
		DialogSet:             "Set",
		DialogCreateTag:       "Create Tag",
		DialogCheckout:        "Checkout",
		DialogReset:           "Reset",
		DialogPush:            "Push",
		DialogCreateStash:     "Create Stash",
		DialogClone:           "Clone",
		DialogDelete:          "Delete",
		DialogAdd:             "Add",
		DialogRemove:          "Remove",
		DialogAddRemote:       "Add Remote",
		DialogAddRemoteButton: "Add Remote",

		DialogNewBranchTitle:        "Create New Branch",
		DialogNewBranchFrom:         "Create New Branch from %s",
		DialogNewBranchName:         "New branch name:",
		DialogCheckoutBranch:        "Checkout branch",
		DialogOverwriteBranch:       "Overwrite existing branch",
		DialogBranchExistsOverwrite: "Branch '%s' already exists. Select Overwrite to reset it here.",
		DialogRenameTitle:           "Rename Branch %s",
		DialogNewName:               "New name:",
		DialogBranchExists:          "Branch '%s' already exists",
		DialogSetUpstreamTitle:      "Set Upstream for %s",
		DialogTrackRemote:           "Track remote branch:",

		DialogCreateTagTitle: "Create Tag on %s",
		DialogTagName:        "Tag name:",
		DialogTagMessage:     "Message (optional, makes an annotated tag):",

		DialogCheckoutRevTitle: "Checkout Tag or Revision",
		DialogRevField:         "Tag, commit hash or reference:",
		DialogDetachedHint:     "HEAD will be detached. Create a branch to keep commits you make there.",

		DialogResetTitle:     "Git Reset",
		DialogResetBody:      "Reset %s to %s \"%s\"",
		DialogResetWarning:   "Your uncommitted changes will be discarded.",
		DialogResetSoft:      "Soft",
		DialogResetMixed:     "Mixed",
		DialogResetHard:      "Hard",
		DialogResetKeep:      "Keep",
		DialogResetSoftDesc:  "Files are untouched; the differences are staged for commit.",
		DialogResetMixedDesc: "Files are untouched; the differences are not staged.",
		DialogResetHardDesc:  "Files are reverted to the commit. Local changes are lost.",
		DialogResetKeepDesc:  "Files are reverted to the commit, but local changes are kept. Fails if they touch the same files.",

		NotifyNothingToPushTitle: "Nothing to push",
		NotifyNothingToPushBody:  "HEAD is detached. Create a branch to push your commits.",
		NotifyNoRemoteTitle:      "No remote",
		NotifyNoRemoteBody:       "Add a remote before pushing (Settings → Manage Remotes).",
		NotifyCannotUpdateTitle:  "Cannot update",
		NotifyCannotUpdateBody:   "The current branch has no upstream. Push it first or set an upstream in the branch popup.",

		DialogPushTitle:        "Push Commits to %s",
		DialogRemoteBranch:     "Remote branch",
		DialogNewLabel:         "New",
		DialogLoadingCommits:   "Loading commits…",
		DialogNothingToPush:    "Nothing to push",
		DialogWillBePushedOne:  "%d commit will be pushed",
		DialogWillBePushedMany: "%d commits will be pushed",
		DialogPushTags:         "Push tags (annotated tags on the pushed commits)",
		DialogPushForce:        "Force push (with lease: refuses if the remote has commits you have not fetched)",
		DialogForcePushTitle:   "Force Push",
		DialogForcePushBody:    "Force-push %s to %s/%s? Commits on the remote that are not in your branch will be removed.",

		DialogUpdateTitle:   "Update Project",
		DialogUpdateType:    "Update Type",
		DialogPullMerge:     "Merge incoming changes into the current branch",
		DialogPullRebase:    "Rebase the current branch on top of incoming changes",
		DialogPullFFOnly:    "Fast-forward only",
		DialogUpdateBody:    "Updates %s from %s.",
		DialogUpdateDontAsk: "Don't ask again",

		DialogStashTitle:       "Stash Changes",
		DialogBranchLabel:      "Branch: %s",
		DialogMessage:          "Message:",
		DialogOptional:         "Optional",
		DialogIncludeUntracked: "Include unversioned files",
		DialogKeepStaged:       "Keep staged changes in the index",

		DialogCloneTitle: "Get from Version Control",
		DialogParentDir:  "Parent directory",
		DialogCloneInto:  "Clone into",
		DialogClonesInto: "Clones into %s",
		DialogExists:     "%s already exists",
		DialogInitTitle:  "Create Git Repository in",

		DialogRemotesTitle:      "Git Remotes",
		DialogNoRemotes:         "This repository has no remotes.",
		DialogRemoveRemoteTitle: "Remove Remote",
		DialogRemoveRemoteBody:  "Remove remote '%s'? Its remote-tracking branches are deleted.",
	})
	register(ZHCN, map[Key]string{
		MenuSettings:      "设置…",
		MenuTheme:         "主题",
		MenuManageRemotes: "管理远程…",
		MenuFetch:         "抓取",

		DialogName:            "名称：",
		DialogURL:             "URL：",
		DialogDirectory:       "目录：",
		DialogBrowse:          "浏览…",
		DialogCreate:          "创建",
		DialogRename:          "重命名",
		DialogSet:             "设置",
		DialogCreateTag:       "创建标签",
		DialogCheckout:        "检出",
		DialogReset:           "重置",
		DialogPush:            "推送",
		DialogCreateStash:     "创建储藏",
		DialogClone:           "克隆",
		DialogDelete:          "删除",
		DialogAdd:             "添加",
		DialogRemove:          "移除",
		DialogAddRemote:       "添加远程",
		DialogAddRemoteButton: "添加远程",

		DialogNewBranchTitle:        "创建新分支",
		DialogNewBranchFrom:         "基于 %s 创建新分支",
		DialogNewBranchName:         "新分支名：",
		DialogCheckoutBranch:        "检出该分支",
		DialogOverwriteBranch:       "覆盖已存在的分支",
		DialogBranchExistsOverwrite: "分支 '%s' 已存在。勾选「覆盖」可把它重置到这里。",
		DialogRenameTitle:           "重命名分支 %s",
		DialogNewName:               "新名称：",
		DialogBranchExists:          "分支 '%s' 已存在",
		DialogSetUpstreamTitle:      "为 %s 设置上游",
		DialogTrackRemote:           "跟踪的远程分支：",

		DialogCreateTagTitle: "在 %s 上创建标签",
		DialogTagName:        "标签名：",
		DialogTagMessage:     "说明（可选，填写后创建附注标签）：",

		DialogCheckoutRevTitle: "检出标签或修订",
		DialogRevField:         "标签、提交哈希或引用：",
		DialogDetachedHint:     "HEAD 将处于分离状态。要保留新建的提交，请创建分支。",

		DialogResetTitle:     "Git 重置",
		DialogResetBody:      "把 %s 重置到 %s \"%s\"",
		DialogResetWarning:   "未提交的更改会被丢弃。",
		DialogResetSoft:      "Soft",
		DialogResetMixed:     "Mixed",
		DialogResetHard:      "Hard",
		DialogResetKeep:      "Keep",
		DialogResetSoftDesc:  "文件不动，差异留在暂存区等待提交。",
		DialogResetMixedDesc: "文件不动，差异不进入暂存区。",
		DialogResetHardDesc:  "文件回到该提交，本地更改会丢失。",
		DialogResetKeepDesc:  "文件回到该提交，但保留本地更改；若两者改了同一文件则失败。",

		NotifyNothingToPushTitle: "没有可推送的内容",
		NotifyNothingToPushBody:  "HEAD 处于分离状态。请先创建分支再推送提交。",
		NotifyNoRemoteTitle:      "没有远程",
		NotifyNoRemoteBody:       "推送前请先添加远程（设置 → 管理远程）。",
		NotifyCannotUpdateTitle:  "无法更新",
		NotifyCannotUpdateBody:   "当前分支没有上游。请先推送，或在分支弹窗里设置上游。",

		DialogPushTitle:        "推送提交到 %s",
		DialogRemoteBranch:     "远程分支",
		DialogNewLabel:         "新分支",
		DialogLoadingCommits:   "正在加载提交…",
		DialogNothingToPush:    "没有可推送的提交",
		DialogWillBePushedOne:  "%d 个提交将被推送",
		DialogWillBePushedMany: "%d 个提交将被推送",
		DialogPushTags:         "推送标签（被推送提交上的附注标签）",
		DialogPushForce:        "强制推送（带 lease：远端有你未抓取的提交时拒绝）",
		DialogForcePushTitle:   "强制推送",
		DialogForcePushBody:    "把 %s 强制推送到 %s/%s？远端上不在你分支里的提交会被删除。",

		DialogUpdateTitle:   "更新项目",
		DialogUpdateType:    "更新方式",
		DialogPullMerge:     "把远端更改合并进当前分支",
		DialogPullRebase:    "把当前分支变基到远端更改之上",
		DialogPullFFOnly:    "仅快进",
		DialogUpdateBody:    "从 %s 更新 %s。",
		DialogUpdateDontAsk: "不再询问",

		DialogStashTitle:       "储藏更改",
		DialogBranchLabel:      "分支：%s",
		DialogMessage:          "说明：",
		DialogOptional:         "可选",
		DialogIncludeUntracked: "包含未跟踪文件",
		DialogKeepStaged:       "保留索引中的已暂存更改",

		DialogCloneTitle: "从版本控制获取",
		DialogParentDir:  "父目录",
		DialogCloneInto:  "克隆到",
		DialogClonesInto: "克隆到 %s",
		DialogExists:     "%s 已存在",
		DialogInitTitle:  "在此创建 Git 仓库",

		DialogRemotesTitle:      "Git 远程",
		DialogNoRemotes:         "这个仓库还没有远程。",
		DialogRemoveRemoteTitle: "移除远程",
		DialogRemoveRemoteBody:  "移除远程 '%s'？它的远程跟踪分支会一并删除。",
	})
}
