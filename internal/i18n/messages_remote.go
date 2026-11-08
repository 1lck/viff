package i18n

// Messages for repositories opened over SSH. The first version is read-only:
// the wording says so wherever a write action would otherwise be expected, so
// a missing button reads as a deliberate limit rather than a broken feature.
const (
	AppNoticeRemoteRefreshFailed = Key("app.notice.remoteRefreshFailed")
	AppNoticeReadOnlyRemote      = Key("app.notice.readOnlyRemote")
	AppNoticeReadOnlyRemoteBody  = Key("app.notice.readOnlyRemoteBody")

	AppErrorReadOnlyRemote = Key("app.error.readOnlyRemote")
	AppErrorSSHMissing     = Key("app.error.sshMissing")
	AppErrorSSHConnect     = Key("app.error.sshConnect")
	AppErrorSSHAuth        = Key("app.error.sshAuth")
	AppErrorSSHHostKey     = Key("app.error.sshHostKey")
	AppErrorSSHGitMissing  = Key("app.error.sshGitMissing")

	RemoteDialogTitle           = Key("remote.dialog.title")
	RemoteDialogHost            = Key("remote.dialog.host")
	RemoteDialogHostPlaceholder = Key("remote.dialog.hostPlaceholder")
	RemoteDialogPath            = Key("remote.dialog.path")
	RemoteDialogPathPlaceholder = Key("remote.dialog.pathPlaceholder")
	RemoteDialogNote            = Key("remote.dialog.note")
	RemoteDialogOpen            = Key("remote.dialog.open")
	RemoteDialogInvalidHost     = Key("remote.dialog.invalidHost")
	RemoteDialogInvalidPath     = Key("remote.dialog.invalidPath")

	ChromeMenuOpenRemote    = Key("chrome.menu.openRemote")
	WelcomeOpenRemote       = Key("welcome.openRemote")
	ChromeStatusReadOnly    = Key("chrome.status.readOnlyRemote")
	ChromeTipReadOnlyRemote = Key("chrome.tip.readOnlyRemote")

	WorkspaceBannerRemoteTitle = Key("workspace.banner.remoteTitle")
	WorkspaceBannerRemoteBody  = Key("workspace.banner.remoteBody")
	WorkspaceTipReadOnly       = Key("workspace.tip.readOnlyRemote")
	WorkspaceRemoteCommitHint  = Key("workspace.remote.commitHint")
	WorkspaceRemoteCommitBody  = Key("workspace.remote.commitBody")
)

func init() {
	register(EN, map[Key]string{
		AppNoticeRemoteRefreshFailed: "Cannot refresh the remote repository",
		AppNoticeReadOnlyRemote:      "Remote repository is read-only",
		AppNoticeReadOnlyRemoteBody:  "Viff opened this repository over SSH for viewing. Edit and commit on the server or in another terminal.",

		AppErrorReadOnlyRemote: "This remote repository is read-only in Viff. Change it on the server or in another terminal.",
		AppErrorSSHMissing:     "SSH was not found. Install an OpenSSH client (Windows: Settings > Apps > Optional Features > OpenSSH Client) and try again.",
		AppErrorSSHConnect:     "Viff could not reach the server. Check the host, port and network, then try again.",
		AppErrorSSHAuth:        "SSH authentication failed. Load a key into ssh-agent or set IdentityFile in ~/.ssh/config; Viff does not show password prompts.",
		AppErrorSSHHostKey:     "The server's host key is not trusted yet. Run `ssh <host>` once in a terminal, confirm the key, then try again.",
		AppErrorSSHGitMissing:  "Git is not installed on the server, or the non-interactive SSH PATH cannot find it.",

		RemoteDialogTitle:           "Open Remote Repository",
		RemoteDialogHost:            "Host",
		RemoteDialogHostPlaceholder: "dev or me@host:2222",
		RemoteDialogPath:            "Repository path",
		RemoteDialogPathPlaceholder: "/srv/app",
		RemoteDialogNote:            "Read-only over SSH. Viff uses the system ssh and your ~/.ssh/config; the host key must be trusted and a key must be loaded into ssh-agent.",
		RemoteDialogOpen:            "Open",
		RemoteDialogInvalidHost:     "Enter a host such as dev or me@host:2222.",
		RemoteDialogInvalidPath:     "Enter an absolute path on the server, such as /srv/app.",

		ChromeMenuOpenRemote:    "Open Remote Repository…",
		WelcomeOpenRemote:       "Open Remote",
		ChromeStatusReadOnly:    "SSH · read-only",
		ChromeTipReadOnlyRemote: "Only viewing is available for a repository opened over SSH.\nChanges are made on the server.",

		WorkspaceBannerRemoteTitle: "Remote repository (read-only)",
		WorkspaceBannerRemoteBody:  "Git runs on %s. Editing, staging and pushing are disabled in this version; use the server or another terminal.",
		WorkspaceTipReadOnly:       "Not available for a read-only remote repository.",
		WorkspaceRemoteCommitHint:  "This repository is open read-only.",
		WorkspaceRemoteCommitBody:  "Viff shows the server's Git state but does not change it. Use the server or another terminal to edit and commit.",
	})
	register(ZHCN, map[Key]string{
		AppNoticeRemoteRefreshFailed: "无法刷新远程仓库",
		AppNoticeReadOnlyRemote:      "远程仓库为只读",
		AppNoticeReadOnlyRemoteBody:  "Viff 通过 SSH 以只读方式打开了此仓库供查看。请在服务器上或其他终端里修改和提交。",

		AppErrorReadOnlyRemote: "此远程仓库在 Viff 中为只读。请在服务器上或其他终端里修改。",
		AppErrorSSHMissing:     "找不到 ssh。请安装 OpenSSH 客户端（Windows：设置 > 应用 > 可选功能 > OpenSSH 客户端）后重试。",
		AppErrorSSHConnect:     "Viff 无法连接到服务器。请检查主机、端口和网络后重试。",
		AppErrorSSHAuth:        "SSH 认证失败。请把密钥加载到 ssh-agent，或在 ~/.ssh/config 里设置 IdentityFile；Viff 不提供密码输入框。",
		AppErrorSSHHostKey:     "服务器的主机密钥尚未被信任。请先在终端执行一次 `ssh <host>`，确认密钥后重试。",
		AppErrorSSHGitMissing:  "服务器上没有安装 Git，或非交互式 SSH 的 PATH 找不到 git。",

		RemoteDialogTitle:           "打开远程仓库",
		RemoteDialogHost:            "主机",
		RemoteDialogHostPlaceholder: "dev 或 me@host:2222",
		RemoteDialogPath:            "仓库路径",
		RemoteDialogPathPlaceholder: "/srv/app",
		RemoteDialogNote:            "通过 SSH 只读打开。Viff 使用系统 ssh 和你的 ~/.ssh/config；主机密钥需已信任，密钥需已加载到 ssh-agent。",
		RemoteDialogOpen:            "打开",
		RemoteDialogInvalidHost:     "请输入主机，例如 dev 或 me@host:2222。",
		RemoteDialogInvalidPath:     "请输入服务器上的绝对路径，例如 /srv/app。",

		ChromeMenuOpenRemote:    "打开远程仓库…",
		WelcomeOpenRemote:       "打开远程",
		ChromeStatusReadOnly:    "SSH · 只读",
		ChromeTipReadOnlyRemote: "通过 SSH 打开的仓库在当前版本只能查看。\n修改请在服务器上或其他终端里进行。",

		WorkspaceBannerRemoteTitle: "远程仓库（只读）",
		WorkspaceBannerRemoteBody:  "Git 在 %s 上执行。当前版本禁用编辑、暂存和推送；请使用服务器上的终端或其他工具。",
		WorkspaceTipReadOnly:       "只读远程仓库不支持此操作。",
		WorkspaceRemoteCommitHint:  "此仓库以只读方式打开。",
		WorkspaceRemoteCommitBody:  "Viff 只显示服务器上的 Git 状态，不会修改它。编辑和提交请在服务器上或其他终端里完成。",
	})
}
