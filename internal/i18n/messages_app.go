package i18n

// Message keys of the application layer: the notifications it hands to the
// interface and the errors it shows to the user. Technical text is not here:
// Git's own messages, paths and hashes stay as they come from Git.
//
// app.notice.fileChangedBody repeats the text of the app's ErrStale on purpose:
// the same sentence is notified from two places, and English must read as it
// did before.
const (
	AppNoticeFailed = Key("app.notice.failed")

	AppNoticeFileChanged     = Key("app.notice.fileChanged")
	AppNoticeFileChangedBody = Key("app.notice.fileChangedBody")

	AppNoticeCheckedOut       = Key("app.notice.checkedOut")
	AppNoticeHeadDetachedBody = Key("app.notice.headDetachedBody")

	AppNoticeCreatedBranch          = Key("app.notice.createdBranch")
	AppNoticeCreatedBranchFromStash = Key("app.notice.createdBranchFromStash")
	AppNoticeRenamedBranch          = Key("app.notice.renamedBranch")
	AppNoticeDeletedRemoteBranch    = Key("app.notice.deletedRemoteBranch")

	AppNoticeBranchNotMerged     = Key("app.notice.branchNotMerged")
	AppNoticeBranchNotMergedBody = Key("app.notice.branchNotMergedBody")
	AppNoticeDeletedBranch       = Key("app.notice.deletedBranch")
	AppNoticeDeletedBranchBody   = Key("app.notice.deletedBranchBody")

	AppNoticeCreatedTag       = Key("app.notice.createdTag")
	AppNoticeDeletedTag       = Key("app.notice.deletedTag")
	AppNoticePushedTag        = Key("app.notice.pushedTag")
	AppNoticeDeletedRemoteTag = Key("app.notice.deletedRemoteTag")
	AppNoticeTrackingUpstream = Key("app.notice.trackingUpstream")

	AppNoticeNothingToFetch     = Key("app.notice.nothingToFetch")
	AppNoticeNothingToFetchBody = Key("app.notice.nothingToFetchBody")
	AppNoticeFetchSuccessful    = Key("app.notice.fetchSuccessful")

	AppNoticeCannotUpdate         = Key("app.notice.cannotUpdate")
	AppNoticeCannotUpdateBody     = Key("app.notice.cannotUpdateBody")
	AppNoticeUpdateConflicts      = Key("app.notice.updateConflicts")
	AppNoticeUpdateConflictsBody  = Key("app.notice.updateConflictsBody")
	AppNoticeAllUpToDate          = Key("app.notice.allUpToDate")
	AppNoticeProjectUpdated       = Key("app.notice.projectUpdated")
	AppNoticeProjectUpdatedBody   = Key("app.notice.projectUpdatedBody")
	AppNoticeCannotUpdateBranch   = Key("app.notice.cannotUpdateBranch")
	AppNoticeBranchCheckedOutBody = Key("app.notice.branchCheckedOutBody")
	AppNoticeCannotFastForward    = Key("app.notice.cannotFastForward")
	AppNoticeBranchDivergedBody   = Key("app.notice.branchDivergedBody")
	AppNoticeBranchUpToDate       = Key("app.notice.branchUpToDate")
	AppNoticeBranchUpToDateBody   = Key("app.notice.branchUpToDateBody")
	AppNoticeUpdatedBranch        = Key("app.notice.updatedBranch")
	AppNoticeUpdatedBranchBody    = Key("app.notice.updatedBranchBody")

	AppNoticeHistoryComplete        = Key("app.notice.historyComplete")
	AppNoticeHistoryCompleteBody    = Key("app.notice.historyCompleteBody")
	AppNoticeFullHistoryFetched     = Key("app.notice.fullHistoryFetched")
	AppNoticeFullHistoryFetchedBody = Key("app.notice.fullHistoryFetchedBody")

	AppNoticePushedTo              = Key("app.notice.pushedTo")
	AppNoticePushRejected          = Key("app.notice.pushRejected")
	AppNoticePushRejectedBody      = Key("app.notice.pushRejectedBody")
	AppNoticePushRejectedStaleBody = Key("app.notice.pushRejectedStaleBody")

	AppNoticeStoppedWithConflicts     = Key("app.notice.stoppedWithConflicts")
	AppNoticeStoppedWithConflictsBody = Key("app.notice.stoppedWithConflictsBody")

	AppNoticeMerged           = Key("app.notice.merged")
	AppNoticeRebasedOnto      = Key("app.notice.rebasedOnto")
	AppNoticeCherryPicked     = Key("app.notice.cherryPicked")
	AppNoticeCherryPickedMany = Key("app.notice.cherryPickedMany")
	AppNoticeReverted         = Key("app.notice.reverted")
	AppNoticeRevertedMany     = Key("app.notice.revertedMany")

	AppNoticeResetTo   = Key("app.notice.resetTo")
	AppNoticeResetBody = Key("app.notice.resetBody")

	AppNoticeChangesStashed   = Key("app.notice.changesStashed")
	AppNoticeStashSucceeded   = Key("app.notice.stashSucceeded")
	AppNoticeStashDropped     = Key("app.notice.stashDropped")
	AppNoticeStashDroppedBody = Key("app.notice.stashDroppedBody")

	AppNoticeAddedRemote   = Key("app.notice.addedRemote")
	AppNoticeRemovedRemote = Key("app.notice.removedRemote")
	AppNoticeUpdatedRemote = Key("app.notice.updatedRemote")

	AppNoticeAllSubmodules    = Key("app.notice.allSubmodules")
	AppNoticeUpdatedSubmodule = Key("app.notice.updatedSubmodule")
	AppNoticeSyncedSubmodule  = Key("app.notice.syncedSubmodule")

	AppNoticePatchApplied        = Key("app.notice.patchApplied")
	AppNoticePatchAppliedToIndex = Key("app.notice.patchAppliedToIndex")
	AppNoticeCannotApplyPatch    = Key("app.notice.cannotApplyPatch")

	AppNoticeCannotCreateWorktree = Key("app.notice.cannotCreateWorktree")
	AppNoticeWorktreeCreated      = Key("app.notice.worktreeCreated")
	AppNoticeCannotDeleteWorktree = Key("app.notice.cannotDeleteWorktree")
	AppNoticeWorktreeDeleted      = Key("app.notice.worktreeDeleted")

	AppNoticeCloneFailed    = Key("app.notice.cloneFailed")
	AppNoticeInitFailed     = Key("app.notice.initFailed")
	AppNoticeRebaseFinished = Key("app.notice.rebaseFinished")

	AppNoticeMessageUpdated       = Key("app.notice.messageUpdated")
	AppNoticeMessageUpdatedBody   = Key("app.notice.messageUpdatedBody")
	AppNoticeSquashedIntoParent   = Key("app.notice.squashedIntoParent")
	AppNoticeSquashedBody         = Key("app.notice.squashedBody")
	AppNoticeDroppedCommit        = Key("app.notice.droppedCommit")
	AppNoticeDroppedCommitBody    = Key("app.notice.droppedCommitBody")
	AppNoticeRewriteAbortLeftBody = Key("app.notice.rewriteAbortLeftBody")
	AppNoticeWasAborted           = Key("app.notice.wasAborted")
	AppNoticeEditHitConflictBody  = Key("app.notice.editHitConflictBody")

	AppNoticeChangeListsUnavailable = Key("app.notice.changeListsUnavailable")
	AppNoticeChangeLists            = Key("app.notice.changeLists")
	AppNoticeCancelled              = Key("app.notice.cancelled")

	AppNoticeRepositoryGone   = Key("app.notice.repositoryGone")
	AppNoticeCannotReadStatus = Key("app.notice.cannotReadStatus")
	AppNoticeCannotLoadCommit = Key("app.notice.cannotLoadCommit")

	AppNoticeWorkspace           = Key("app.notice.workspace")
	AppNoticeWorkspaceSaveFailed = Key("app.notice.workspaceSaveFailed")

	AppNoticeCheckoutFailed              = Key("app.notice.checkoutFailed")
	AppNoticeCheckoutBlockedBody         = Key("app.notice.checkoutBlockedBody")
	AppNoticeCheckoutStashNotCreatedBody = Key("app.notice.checkoutStashNotCreatedBody")
	AppNoticeCheckoutRestoreFailedBody   = Key("app.notice.checkoutRestoreFailedBody")
	AppNoticeCheckoutStashKeptBody       = Key("app.notice.checkoutStashKeptBody")
	AppNoticeCheckoutRestoredBody        = Key("app.notice.checkoutRestoredBody")
	AppNoticeCheckoutStashedBody         = Key("app.notice.checkoutStashedBody")

	AppNoticeUpdateAvailable     = Key("app.notice.updateAvailable")
	AppNoticeUpdateAvailableBody = Key("app.notice.updateAvailableBody")

	AppNoticeResolveConflictFailed = Key("app.notice.resolveConflictFailed")

	AppNoticeSubmoduleAdded         = Key("app.notice.submoduleAdded")
	AppNoticeSubmoduleRemoved       = Key("app.notice.submoduleRemoved")
	AppNoticeSubmoduleDeinitialized = Key("app.notice.submoduleDeinitialized")

	AppNoticeCommitted         = Key("app.notice.committed")
	AppNoticeCommittedBody     = Key("app.notice.committedBody")
	AppNoticeCommittedBodyMany = Key("app.notice.committedBodyMany")
	AppNoticeAmendedBody       = Key("app.notice.amendedBody")

	// Session undo: the operation names, the notification wording, and the
	// reasons an undo refuses because the repository moved on.
	AppUndoOpCommit       = Key("app.undo.op.commit")
	AppUndoOpAmend        = Key("app.undo.op.amend")
	AppUndoOpMerge        = Key("app.undo.op.merge")
	AppUndoOpRebase       = Key("app.undo.op.rebase")
	AppUndoOpCherryPick   = Key("app.undo.op.cherryPick")
	AppUndoOpRevert       = Key("app.undo.op.revert")
	AppUndoOpReset        = Key("app.undo.op.reset")
	AppUndoOpCheckout     = Key("app.undo.op.checkout")
	AppUndoOpCreateBranch = Key("app.undo.op.createBranch")
	AppUndoOpDeleteBranch = Key("app.undo.op.deleteBranch")
	AppUndoOpRenameBranch = Key("app.undo.op.renameBranch")
	AppUndoOpUnknown      = Key("app.undo.op.unknown")

	AppUndoDone    = Key("app.undo.done")
	AppUndoFailed  = Key("app.undo.failed")
	AppUndoNothing = Key("app.undo.nothing")
	AppUndoBusy    = Key("app.undo.busy")

	AppUndoNothingBody = Key("app.undo.nothingBody")
	AppUndoBusyBody    = Key("app.undo.busyBody")

	AppUndoBlockedRepository    = Key("app.undo.blocked.repository")
	AppUndoBlockedOperation     = Key("app.undo.blocked.operation")
	AppUndoBlockedMoved         = Key("app.undo.blocked.moved")
	AppUndoBlockedDirty         = Key("app.undo.blocked.dirty")
	AppUndoBlockedBranch        = Key("app.undo.blocked.branch")
	AppUndoBlockedBranchExists  = Key("app.undo.blocked.branchExists")
	AppUndoResetBody            = Key("app.undo.body.reset")
	AppUndoCommitBody           = Key("app.undo.body.commit")
	AppUndoAmendBody            = Key("app.undo.body.amend")
	AppUndoCheckoutBranchBody   = Key("app.undo.body.checkoutBranch")
	AppUndoCheckoutRevisionBody = Key("app.undo.body.checkoutRevision")
	AppUndoCreateBranchBody     = Key("app.undo.body.createBranch")
	AppUndoDeleteBranchBody     = Key("app.undo.body.deleteBranch")
	AppUndoRenameBranchBody     = Key("app.undo.body.renameBranch")

	AppActionDeleteAnyway              = Key("app.action.deleteAnyway")
	AppActionRestore                   = Key("app.action.restore")
	AppActionUpdate                    = Key("app.action.update")
	AppActionCreateBranchAtOldPosition = Key("app.action.createBranchAtOldPosition")

	AppErrorOpenRepositoryFirst      = Key("app.error.openRepositoryFirst")
	AppErrorNoConflictView           = Key("app.error.noConflictView")
	AppErrorNoTextToMerge            = Key("app.error.noTextToMerge")
	AppErrorNoRepositoryOpen         = Key("app.error.noRepositoryOpen")
	AppErrorRecordingRunning         = Key("app.error.recordingRunning")
	AppErrorNoDiagnosticsDestination = Key("app.error.noDiagnosticsDestination")
	AppErrorChangeListNameEmpty      = Key("app.error.changeListNameEmpty")
	AppErrorChangeListNamedExists    = Key("app.error.changeListNamedExists")
	AppErrorChangeListDefaultRename  = Key("app.error.changeListDefaultRename")
	AppErrorChangeListDefaultDelete  = Key("app.error.changeListDefaultDelete")
	AppErrorChangeListUnknown        = Key("app.error.changeListUnknown")
	AppErrorChangeListEmpty          = Key("app.error.changeListEmpty")

	// Titles of the two sides of a comparison.
	AppSideStaged      = Key("app.side.staged")
	AppSideYourVersion = Key("app.side.yourVersion")
)

func init() {
	register(EN, map[Key]string{
		AppNoticeFailed: "%s failed",

		AppNoticeFileChanged:     "File changed",
		AppNoticeFileChangedBody: "the file changed after the diff was shown; review the new diff and try again",

		AppNoticeCheckedOut:       "Checked out %s",
		AppNoticeHeadDetachedBody: "HEAD is detached. Create a branch to keep new commits.",

		AppNoticeCreatedBranch:          "Created branch %s",
		AppNoticeCreatedBranchFromStash: "Created branch %s from %s",
		AppNoticeRenamedBranch:          "Renamed %s to %s",
		AppNoticeDeletedRemoteBranch:    "Deleted remote branch %s",

		AppNoticeBranchNotMerged:     "Branch %s is not fully merged",
		AppNoticeBranchNotMergedBody: "Its commits are not on the current branch. Deleting it anyway leaves them reachable only through the reflog.",
		AppNoticeDeletedBranch:       "Deleted branch %s",
		AppNoticeDeletedBranchBody:   "It pointed at %s.",

		AppNoticeCreatedTag:       "Created tag %s",
		AppNoticeDeletedTag:       "Deleted tag %s",
		AppNoticePushedTag:        "Pushed tag %s to %s",
		AppNoticeDeletedRemoteTag: "Deleted tag %s on %s",
		AppNoticeTrackingUpstream: "Tracking %s",

		AppNoticeNothingToFetch:     "Nothing to fetch",
		AppNoticeNothingToFetchBody: "This repository has no remotes.",
		AppNoticeFetchSuccessful:    "Fetch successful",

		AppNoticeCannotUpdate:         "Cannot update",
		AppNoticeCannotUpdateBody:     "The current branch has no upstream branch. Push it with an upstream or set one in the branches popup.",
		AppNoticeUpdateConflicts:      "Update has conflicts",
		AppNoticeUpdateConflictsBody:  "Resolve them with your tool; Viff shows the conflicts and follows the repository state.",
		AppNoticeAllUpToDate:          "All files are up-to-date",
		AppNoticeProjectUpdated:       "Project updated",
		AppNoticeProjectUpdatedBody:   "Updated to %s",
		AppNoticeCannotUpdateBranch:   "Cannot update %s",
		AppNoticeBranchCheckedOutBody: "The branch is checked out in a worktree. Update it there, where its files are.",
		AppNoticeCannotFastForward:    "Cannot fast-forward %s",
		AppNoticeBranchDivergedBody:   "It has commits the remote does not. Merge or rebase it in a worktree first; Viff leaves the branch unchanged.",
		AppNoticeBranchUpToDate:       "%s is up to date",
		AppNoticeBranchUpToDateBody:   "No new commits on %s/%s.",
		AppNoticeUpdatedBranch:        "Updated %s",
		AppNoticeUpdatedBranchBody:    "%s → %s, %d new commits",

		AppNoticeHistoryComplete:        "History is complete",
		AppNoticeHistoryCompleteBody:    "This repository is not a shallow clone.",
		AppNoticeFullHistoryFetched:     "Full history fetched",
		AppNoticeFullHistoryFetchedBody: "The repository is no longer a shallow clone.",

		AppNoticePushedTo:              "Pushed %s to %s/%s",
		AppNoticePushRejected:          "Push rejected",
		AppNoticePushRejectedBody:      "The remote has commits you do not have. Update first, then push again.",
		AppNoticePushRejectedStaleBody: "The remote branch changed since your last fetch, so the force push was refused to protect that work. Fetch and review before trying again.",

		AppNoticeStoppedWithConflicts:     "%s stopped with conflicts",
		AppNoticeStoppedWithConflictsBody: "Resolve the conflicts with your external tool. Viff shows them read-only, refreshes when files change, and offers Continue or Abort once Git is ready.",

		AppNoticeMerged:           "Merged %s",
		AppNoticeRebasedOnto:      "Rebased onto %s",
		AppNoticeCherryPicked:     "Cherry-picked %d commit",
		AppNoticeCherryPickedMany: "Cherry-picked %d commits",
		AppNoticeReverted:         "Reverted %d commit",
		AppNoticeRevertedMany:     "Reverted %d commits",

		AppNoticeResetTo:   "Reset to %s",
		AppNoticeResetBody: "The branch pointed at %s before; it stays in the reflog.",

		AppNoticeChangesStashed:   "Changes stashed",
		AppNoticeStashSucceeded:   "%s succeeded",
		AppNoticeStashDropped:     "Dropped %s",
		AppNoticeStashDroppedBody: "It was commit %s; git stash store can restore it while the object exists.",

		AppNoticeAddedRemote:   "Added remote %s",
		AppNoticeRemovedRemote: "Removed remote %s",
		AppNoticeUpdatedRemote: "Updated remote %s",

		AppNoticeAllSubmodules:    "all submodules",
		AppNoticeUpdatedSubmodule: "Updated %s",
		AppNoticeSyncedSubmodule:  "Synced %s",

		AppNoticePatchApplied:        "Patch applied",
		AppNoticePatchAppliedToIndex: ", applied to the index only",
		AppNoticeCannotApplyPatch:    "Cannot apply patch",

		AppNoticeCannotCreateWorktree: "Cannot create worktree",
		AppNoticeWorktreeCreated:      "Worktree created",
		AppNoticeCannotDeleteWorktree: "Cannot delete worktree",
		AppNoticeWorktreeDeleted:      "Worktree deleted",

		AppNoticeCloneFailed:    "Clone failed",
		AppNoticeInitFailed:     "Init failed",
		AppNoticeRebaseFinished: "Rebase finished",

		AppNoticeMessageUpdated:       "Message of %s updated",
		AppNoticeMessageUpdatedBody:   "The commits after it were replayed on top.",
		AppNoticeSquashedIntoParent:   "Squashed %s into its parent",
		AppNoticeSquashedBody:         "The combined commit keeps both changes.",
		AppNoticeDroppedCommit:        "Dropped %s",
		AppNoticeDroppedCommitBody:    "The commits after it were replayed without it.",
		AppNoticeRewriteAbortLeftBody: "The rebase is still in progress. Run `git rebase --abort` in a terminal before doing anything else.\n\n%s",
		AppNoticeWasAborted:           "%s was aborted",
		AppNoticeEditHitConflictBody:  "The edit hit a conflict. Viff aborted the rebase and left the branch as it was.",

		AppNoticeChangeListsUnavailable: "Change lists are unavailable",
		AppNoticeChangeLists:            "Change lists",
		AppNoticeCancelled:              "Cancelled",

		AppNoticeRepositoryGone:   "Repository is gone",
		AppNoticeCannotReadStatus: "Cannot read status",
		AppNoticeCannotLoadCommit: "Cannot load commit",

		AppNoticeWorkspace:           "Workspace",
		AppNoticeWorkspaceSaveFailed: "Cannot save the workspace list: %s",

		AppNoticeCheckoutFailed:              "Checkout failed",
		AppNoticeCheckoutBlockedBody:         "%s\n\nCommit, stash or roll back your changes first.",
		AppNoticeCheckoutStashNotCreatedBody: "%s\n\nNothing was stashed.",
		AppNoticeCheckoutRestoreFailedBody:   "%s\n\nRestoring the changes failed too; they are still in the stash and the branch did not change.",
		AppNoticeCheckoutStashKeptBody:       "Your local changes conflict with the target. They are still in the stash; apply them when ready.",
		AppNoticeCheckoutRestoredBody:        "%s\n\nYour local changes were restored and the branch did not change.",
		AppNoticeCheckoutStashedBody:         "Local changes were stashed and restored.",

		AppNoticeUpdateAvailable:     "Update available",
		AppNoticeUpdateAvailableBody: "Viff %s is available (you have %s).",

		AppNoticeResolveConflictFailed: "Resolve conflict failed",

		AppNoticeSubmoduleAdded:         "Submodule added",
		AppNoticeSubmoduleRemoved:       "Submodule removed",
		AppNoticeSubmoduleDeinitialized: "Submodule deinitialized",

		AppNoticeCommitted:         "Committed",
		AppNoticeCommittedBody:     "%d file committed: %s",
		AppNoticeCommittedBodyMany: "%d files committed: %s",
		AppNoticeAmendedBody:       "Amended: %s",

		AppUndoOpCommit:       "Commit",
		AppUndoOpAmend:        "Amend",
		AppUndoOpMerge:        "Merge",
		AppUndoOpRebase:       "Rebase",
		AppUndoOpCherryPick:   "Cherry-pick",
		AppUndoOpRevert:       "Revert",
		AppUndoOpReset:        "Reset",
		AppUndoOpCheckout:     "Checkout",
		AppUndoOpCreateBranch: "Create branch",
		AppUndoOpDeleteBranch: "Delete branch",
		AppUndoOpRenameBranch: "Rename branch",
		AppUndoOpUnknown:      "Operation",

		AppUndoDone:    "Undid %s",
		AppUndoFailed:  "Cannot undo %s",
		AppUndoNothing: "Nothing to undo",
		AppUndoBusy:    "A command is running",

		AppUndoNothingBody: "Viff remembers the local operations of this session only.",
		AppUndoBusyBody:    "Wait for it to finish before undoing an earlier operation.",

		AppUndoBlockedRepository:    "The repository changed since this operation; its history is no longer available.",
		AppUndoBlockedOperation:     "Finish or abort the %s in progress first.",
		AppUndoBlockedMoved:         "HEAD or the current branch changed since this operation. Undoing now would overwrite newer work.",
		AppUndoBlockedDirty:         "Commit, stash or discard your changes first: this undo replaces the working tree.",
		AppUndoBlockedBranch:        "Branch %s changed since this operation.",
		AppUndoBlockedBranchExists:  "Branch %s exists again; remove or rename it first.",
		AppUndoResetBody:            "The branch is back at %s.",
		AppUndoCommitBody:           "The commit is gone and its changes are staged again.",
		AppUndoAmendBody:            "The previous commit is back; the amended changes are staged again.",
		AppUndoCheckoutBranchBody:   "Back on %s.",
		AppUndoCheckoutRevisionBody: "HEAD is detached at %s again.",
		AppUndoCreateBranchBody:     "Branch %s was removed.",
		AppUndoDeleteBranchBody:     "Branch %s is back at %s.",
		AppUndoRenameBranchBody:     "Branch %s is called %s again.",

		AppActionDeleteAnyway:              "Delete anyway",
		AppActionRestore:                   "Restore",
		AppActionUpdate:                    "Update",
		AppActionCreateBranchAtOldPosition: "Create branch at old position",

		AppErrorOpenRepositoryFirst:      "open a repository first",
		AppErrorNoConflictView:           "no conflict view to resolve; reload the diff",
		AppErrorNoTextToMerge:            "there is no text to merge",
		AppErrorNoRepositoryOpen:         "no repository is open",
		AppErrorRecordingRunning:         "a problem recording is already running",
		AppErrorNoDiagnosticsDestination: "no diagnostics destination chosen",
		AppErrorChangeListNameEmpty:      "change list name is empty",
		AppErrorChangeListNamedExists:    "a change list named %q already exists",
		AppErrorChangeListDefaultRename:  "the default change list cannot be renamed",
		AppErrorChangeListDefaultDelete:  "the default change list cannot be deleted",
		AppErrorChangeListUnknown:        "unknown change list %q",
		AppErrorChangeListEmpty:          "change list is empty",

		AppSideStaged:      "Staged",
		AppSideYourVersion: "Your version",
	})

	register(ZHCN, map[Key]string{
		AppNoticeFailed: "%s 失败",

		AppNoticeFileChanged:     "文件已更改",
		AppNoticeFileChangedBody: "显示 diff 之后文件又发生了变化；请查看新的 diff 后重试。",

		AppNoticeCheckedOut:       "已切换到 %s",
		AppNoticeHeadDetachedBody: "HEAD 已分离。新建分支以保留新的提交。",

		AppNoticeCreatedBranch:          "已创建分支 %s",
		AppNoticeCreatedBranchFromStash: "已创建分支 %s（基于 %s）",
		AppNoticeRenamedBranch:          "已将 %s 重命名为 %s",
		AppNoticeDeletedRemoteBranch:    "已删除远程分支 %s",

		AppNoticeBranchNotMerged:     "分支 %s 未完全合并",
		AppNoticeBranchNotMergedBody: "它的提交不在当前分支上。强行删除后，这些提交只能通过 reflog 找回。",
		AppNoticeDeletedBranch:       "已删除分支 %s",
		AppNoticeDeletedBranchBody:   "它原本指向 %s。",

		AppNoticeCreatedTag:       "已创建标签 %s",
		AppNoticeDeletedTag:       "已删除标签 %s",
		AppNoticePushedTag:        "已将标签 %s 推送到 %s",
		AppNoticeDeletedRemoteTag: "已删除标签 %s（位于 %s）",
		AppNoticeTrackingUpstream: "正在跟踪 %s",

		AppNoticeNothingToFetch:     "没有可抓取的内容",
		AppNoticeNothingToFetchBody: "此仓库没有远程。",
		AppNoticeFetchSuccessful:    "抓取成功",

		AppNoticeCannotUpdate:         "无法更新",
		AppNoticeCannotUpdateBody:     "当前分支没有上游分支。推送时指定上游，或在分支弹出菜单中设置一个。",
		AppNoticeUpdateConflicts:      "更新时发生冲突",
		AppNoticeUpdateConflictsBody:  "请用你的工具解决冲突；Viff 会展示冲突并跟随仓库状态。",
		AppNoticeAllUpToDate:          "所有文件均为最新",
		AppNoticeProjectUpdated:       "项目已更新",
		AppNoticeProjectUpdatedBody:   "已更新到 %s",
		AppNoticeCannotUpdateBranch:   "无法更新 %s",
		AppNoticeBranchCheckedOutBody: "该分支已在某个工作树中检出。请在那里更新，它的文件在那里。",
		AppNoticeCannotFastForward:    "无法快进 %s",
		AppNoticeBranchDivergedBody:   "它含有远程没有的提交。请先在工作树中合并或变基；Viff 不会改动该分支。",
		AppNoticeBranchUpToDate:       "%s 已是最新",
		AppNoticeBranchUpToDateBody:   "%s/%s 上没有新的提交。",
		AppNoticeUpdatedBranch:        "已更新 %s",
		AppNoticeUpdatedBranchBody:    "%s → %s，新增 %d 个提交",

		AppNoticeHistoryComplete:        "历史已完整",
		AppNoticeHistoryCompleteBody:    "此仓库不是浅克隆。",
		AppNoticeFullHistoryFetched:     "已抓取完整历史",
		AppNoticeFullHistoryFetchedBody: "此仓库已不再是浅克隆。",

		AppNoticePushedTo:              "已推送 %s 到 %s/%s",
		AppNoticePushRejected:          "推送被拒绝",
		AppNoticePushRejectedBody:      "远程有你没有的提交。请先更新，然后再推送。",
		AppNoticePushRejectedStaleBody: "远程分支在你上次抓取后发生了变化，因此为保护那部分工作，强制推送被拒绝。请先抓取并查看，再重试。",

		AppNoticeStoppedWithConflicts:     "%s 因冲突而停止",
		AppNoticeStoppedWithConflictsBody: "请用外部工具解决冲突。Viff 以只读方式展示冲突，文件变化时自动刷新，并在 Git 就绪后提供继续或中止。",

		AppNoticeMerged:           "已合并 %s",
		AppNoticeRebasedOnto:      "已变基到 %s",
		AppNoticeCherryPicked:     "已拣选 %d 个提交",
		AppNoticeCherryPickedMany: "已拣选 %d 个提交",
		AppNoticeReverted:         "已回滚 %d 个提交",
		AppNoticeRevertedMany:     "已回滚 %d 个提交",

		AppNoticeResetTo:   "已重置到 %s",
		AppNoticeResetBody: "该分支此前指向 %s；它仍保留在 reflog 中。",

		AppNoticeChangesStashed:   "更改已贮藏",
		AppNoticeStashSucceeded:   "%s 成功",
		AppNoticeStashDropped:     "已丢弃 %s",
		AppNoticeStashDroppedBody: "它曾是提交 %s；只要该对象还在，git stash store 就能恢复它。",

		AppNoticeAddedRemote:   "已添加远程 %s",
		AppNoticeRemovedRemote: "已移除远程 %s",
		AppNoticeUpdatedRemote: "已更新远程 %s",

		AppNoticeAllSubmodules:    "所有子模块",
		AppNoticeUpdatedSubmodule: "已更新 %s",
		AppNoticeSyncedSubmodule:  "已同步 %s",

		AppNoticePatchApplied:        "补丁已应用",
		AppNoticePatchAppliedToIndex: "，仅应用到索引",
		AppNoticeCannotApplyPatch:    "无法应用补丁",

		AppNoticeCannotCreateWorktree: "无法创建工作树",
		AppNoticeWorktreeCreated:      "工作树已创建",
		AppNoticeCannotDeleteWorktree: "无法删除工作树",
		AppNoticeWorktreeDeleted:      "工作树已删除",

		AppNoticeCloneFailed:    "克隆失败",
		AppNoticeInitFailed:     "初始化失败",
		AppNoticeRebaseFinished: "变基完成",

		AppNoticeMessageUpdated:       "已更新 %s 的提交信息",
		AppNoticeMessageUpdatedBody:   "其后的提交已重新应用到它之上。",
		AppNoticeSquashedIntoParent:   "已将 %s 压缩进其父提交",
		AppNoticeSquashedBody:         "合并后的提交保留了双方的更改。",
		AppNoticeDroppedCommit:        "已丢弃 %s",
		AppNoticeDroppedCommitBody:    "其后的提交已在没有它的情况下重新应用。",
		AppNoticeRewriteAbortLeftBody: "变基仍在进行中。请先在终端中执行 `git rebase --abort`，再做其他操作。\n\n%s",
		AppNoticeWasAborted:           "%s 已中止",
		AppNoticeEditHitConflictBody:  "该编辑遇到冲突。Viff 已中止变基，分支保持原状。",

		AppNoticeChangeListsUnavailable: "变更列表不可用",
		AppNoticeChangeLists:            "变更列表",
		AppNoticeCancelled:              "已取消",

		AppNoticeRepositoryGone:   "仓库已不存在",
		AppNoticeCannotReadStatus: "无法读取状态",
		AppNoticeCannotLoadCommit: "无法加载提交",

		AppNoticeWorkspace:           "工作区",
		AppNoticeWorkspaceSaveFailed: "无法保存工作区列表：%s",

		AppNoticeCheckoutFailed:              "检出失败",
		AppNoticeCheckoutBlockedBody:         "%s\n\n请先提交、贮藏或回滚你的更改。",
		AppNoticeCheckoutStashNotCreatedBody: "%s\n\n没有贮藏任何内容。",
		AppNoticeCheckoutRestoreFailedBody:   "%s\n\n恢复更改也失败了；更改仍在贮藏中，分支没有变化。",
		AppNoticeCheckoutStashKeptBody:       "你的本地更改与目标冲突。它们仍在贮藏中；准备好后再应用。",
		AppNoticeCheckoutRestoredBody:        "%s\n\n你的本地更改已恢复，分支没有变化。",
		AppNoticeCheckoutStashedBody:         "本地更改已贮藏并恢复。",

		AppNoticeUpdateAvailable:     "有可用更新",
		AppNoticeUpdateAvailableBody: "Viff %s 已发布（当前为 %s）。",

		AppNoticeResolveConflictFailed: "解决冲突失败",

		AppNoticeSubmoduleAdded:         "子模块已添加",
		AppNoticeSubmoduleRemoved:       "子模块已移除",
		AppNoticeSubmoduleDeinitialized: "子模块已取消初始化",

		AppNoticeCommitted:         "已提交",
		AppNoticeCommittedBody:     "已提交 %d 个文件：%s",
		AppNoticeCommittedBodyMany: "已提交 %d 个文件：%s",
		AppNoticeAmendedBody:       "已修订：%s",

		AppUndoOpCommit:       "提交",
		AppUndoOpAmend:        "修订",
		AppUndoOpMerge:        "合并",
		AppUndoOpRebase:       "变基",
		AppUndoOpCherryPick:   "拣选",
		AppUndoOpRevert:       "回滚",
		AppUndoOpReset:        "重置",
		AppUndoOpCheckout:     "检出",
		AppUndoOpCreateBranch: "创建分支",
		AppUndoOpDeleteBranch: "删除分支",
		AppUndoOpRenameBranch: "重命名分支",
		AppUndoOpUnknown:      "操作",

		AppUndoDone:    "已撤销%s",
		AppUndoFailed:  "无法撤销%s",
		AppUndoNothing: "没有可撤销的操作",
		AppUndoBusy:    "有命令正在运行",

		AppUndoNothingBody: "Viff 只记住本次会话中的本地操作。",
		AppUndoBusyBody:    "请等它结束后再撤销更早的操作。",

		AppUndoBlockedRepository:    "仓库已变化，该操作的历史不再可用。",
		AppUndoBlockedOperation:     "请先完成或中止进行中的%s。",
		AppUndoBlockedMoved:         "该操作之后 HEAD 或当前分支已变化；现在撤销会覆盖更新的工作。",
		AppUndoBlockedDirty:         "请先提交、贮藏或回滚你的更改：这次撤销会替换工作区。",
		AppUndoBlockedBranch:        "分支 %s 在该操作之后已变化。",
		AppUndoBlockedBranchExists:  "分支 %s 又被创建了；请先删除或重命名它。",
		AppUndoResetBody:            "分支已回到 %s。",
		AppUndoCommitBody:           "提交已移除，其中的更改重新处于已暂存状态。",
		AppUndoAmendBody:            "上一次提交已恢复，修订的更改重新处于已暂存状态。",
		AppUndoCheckoutBranchBody:   "已切回 %s。",
		AppUndoCheckoutRevisionBody: "HEAD 已重新分离到 %s。",
		AppUndoCreateBranchBody:     "分支 %s 已移除。",
		AppUndoDeleteBranchBody:     "分支 %s 已恢复到 %s。",
		AppUndoRenameBranchBody:     "分支 %s 已改回 %s。",

		AppActionDeleteAnyway:              "仍要删除",
		AppActionRestore:                   "恢复",
		AppActionUpdate:                    "更新",
		AppActionCreateBranchAtOldPosition: "在原位置创建分支",

		AppErrorOpenRepositoryFirst:      "请先打开一个仓库",
		AppErrorNoConflictView:           "没有可解决的冲突视图；请重新加载 diff",
		AppErrorNoTextToMerge:            "没有可合并的文本",
		AppErrorNoRepositoryOpen:         "没有已打开的仓库",
		AppErrorRecordingRunning:         "已在记录问题",
		AppErrorNoDiagnosticsDestination: "未选择诊断信息的保存位置",
		AppErrorChangeListNameEmpty:      "变更列表名称为空",
		AppErrorChangeListNamedExists:    "名为 %q 的变更列表已存在",
		AppErrorChangeListDefaultRename:  "默认变更列表无法重命名",
		AppErrorChangeListDefaultDelete:  "默认变更列表无法删除",
		AppErrorChangeListUnknown:        "未知的变更列表 %q",
		AppErrorChangeListEmpty:          "变更列表为空",

		AppSideStaged:      "已暂存",
		AppSideYourVersion: "你的版本",
	})
}
