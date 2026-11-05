package i18n

// Message keys of the push variants: pushing an older commit, pushing HEAD to
// another remote, and deleting a remote tag.
const (
	PushRemote         = Key("push.remote")
	PushBranch         = Key("push.branch")
	PushForceLease     = Key("push.forceLease")
	PushUpToTitle      = Key("push.upToTitle")
	PushUpToBody       = Key("push.upToBody")
	PushForceUpToBody  = Key("push.forceUpToBody")
	PushAnotherTitle   = Key("push.anotherTitle")
	PushAnotherBody    = Key("push.anotherBody")
	PushAnotherHint    = Key("push.anotherHint")
	PushForceHeadBody  = Key("push.forceHeadBody")
	PushDeleteTagTitle = Key("push.deleteTagTitle")
	PushDeleteTagBody  = Key("push.deleteTagBody")
	PushNoRemoteForTag = Key("push.noRemoteForTag")
)

func init() {
	register(EN, map[Key]string{
		PushRemote:         "Remote",
		PushBranch:         "Branch",
		PushForceLease:     "Force push (with lease: protects unseen remote commits)",
		PushUpToTitle:      "Push Up to %s",
		PushUpToBody:       "Pushes %s and every commit before it to %s/%s",
		PushForceUpToBody:  "Force-push up to %s to %s/%s? Commits on the remote that are not in this history will be removed.",
		PushAnotherTitle:   "Push HEAD to Another Remote",
		PushAnotherBody:    "Pushes the current HEAD (%s) to the remote and branch below.",
		PushAnotherHint:    "The local branch and its upstream are unchanged.",
		PushForceHeadBody:  "Force-push HEAD to %s/%s? Commits on the remote branch that are not in HEAD will be removed.",
		PushDeleteTagTitle: "Delete Remote Tag",
		PushDeleteTagBody:  "Deletes tag '%s' on the remote. The local tag stays.",
		PushNoRemoteForTag: "Add a remote before deleting remote tags.",
	})
	register(ZHCN, map[Key]string{
		PushRemote:         "远程",
		PushBranch:         "分支",
		PushForceLease:     "强制推送（带 lease 保护：不会覆盖尚未抓取的远端提交）",
		PushUpToTitle:      "推送至 %s",
		PushUpToBody:       "把 %s 及其之前的提交推送到 %s/%s",
		PushForceUpToBody:  "把 %s 及其之前的提交强制推送到 %s/%s？远端上不在此历史中的提交会被删除。",
		PushAnotherTitle:   "把 HEAD 推送到其他远程",
		PushAnotherBody:    "把当前 HEAD（%s）推送到下面的远程与分支。",
		PushAnotherHint:    "本地分支及其上游保持不变。",
		PushForceHeadBody:  "把 HEAD 强制推送到 %s/%s？远端分支上不在 HEAD 中的提交会被删除。",
		PushDeleteTagTitle: "删除远程标签",
		PushDeleteTagBody:  "删除远端上的标签 '%s'，本地标签会保留。",
		PushNoRemoteForTag: "删除远程标签前请先添加远程。",
	})
}
