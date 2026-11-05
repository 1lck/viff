package i18n

// Message keys of the remote links: the log's commit menu, the branch menu and
// the notices shown when a link cannot be built or opened. The service name in
// the menu comes from the remote's host, not from this table, so a self-hosted
// instance shows its own name.
const (
	// LinksOpenCommit is the log's entry; the argument is the service.
	LinksOpenCommit = Key("links.openCommit")
	// LinksOpenBranch is the branch menu's entry; the arguments are the branch
	// and the service. Chinese names them in the other order, which is why its
	// text uses fmt's explicit argument indexes.
	LinksOpenBranch = Key("links.openBranch")
	// LinksCompare is the branch menu's comparison; the argument is the base.
	LinksCompare = Key("links.compare")
	// LinksCompareChoose is the same entry when the base branch is not known
	// yet; the click asks which one to use.
	LinksCompareChoose = Key("links.compareChoose")
	// LinksCreatePR is the branch menu's pull request entry.
	LinksCreatePR = Key("links.createPullRequest")
	// LinksChooseBase titles the popup that picks the base branch.
	LinksChooseBase = Key("links.chooseBase")
	// LinksNoBase is shown when no branch can act as the base at all.
	LinksNoBase = Key("links.noBase")
	// LinksFallback explains that the service has no such page and the branch
	// page was opened instead; the argument is the service.
	LinksFallback = Key("links.fallback")
	// LinksCannotOpen titles a notice whose body is the opener's error.
	LinksCannotOpen = Key("links.cannotOpen")
)

func init() {
	register(EN, map[Key]string{
		LinksOpenCommit:    "Open Commit on %s",
		LinksOpenBranch:    "Open %s on %s",
		LinksCompare:       "Compare with %s",
		LinksCompareChoose: "Compare with…",
		LinksCreatePR:      "Create Pull Request…",
		LinksChooseBase:    "Choose Base Branch",
		LinksNoBase:        "No branch to compare with yet. Commit on a branch first.",
		LinksFallback:      "Viff does not build this page for %s; opened the branch page instead.",
		LinksCannotOpen:    "Cannot open the link",
	})
	register(ZHCN, map[Key]string{
		LinksOpenCommit:    "在 %s 上打开提交",
		LinksOpenBranch:    "在 %s 上打开 %s",
		LinksCompare:       "与 %s 比较",
		LinksCompareChoose: "与…比较",
		LinksCreatePR:      "创建拉取请求…",
		LinksChooseBase:    "选择基准分支",
		LinksNoBase:        "还没有可比对的分支，先在分支上提交。",
		LinksFallback:      "Viff 不为 %s 生成这个页面，改为打开分支页面。",
		LinksCannotOpen:    "无法打开链接",
	})
}
