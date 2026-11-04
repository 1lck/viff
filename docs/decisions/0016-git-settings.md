# 0016 Git 命令、签名与凭据设置

- 状态：已采用（2026-11）

## 背景

IDE 的 Git 设置里有 SSH 可执行程序、GPG 程序、签名密钥与格式、默认签名提交、credential helper。Viff 之前只能选择 Git 可执行文件，其余都要求用户改全局 Git 配置；而 Viff 会继承那套配置，改了会影响终端和其他工具。设置需要成为 Viff 自己的偏好，而不是对用户 Git 配置的编辑。

## 决定

- 设置作为**每次调用**的参数传入，不写入仓库配置，也不改全局配置：`gpg.program`、`user.signingkey`、`gpg.format`、`credential.helper` 用 `-c key=value`，SSH 命令用环境变量 `GIT_SSH_COMMAND`。`-c` 的优先级高于系统、全局和仓库配置，正好表达“Viff 内的设置覆盖 Git 默认值”。
- 值为空就不传。旧版 `settings.json` 没有这些字段，读出来是空值，行为与升级前完全一致；新增字段用 `omitempty`，未设置时不会往文件里写空键。格式版本仍是 1，不需要迁移。
- Runner 持有一份配置快照（互斥锁保护）。保存设置时由 `Workspace.ApplyGitSettings` 整体替换；已经开始执行的命令保持旧值，不会在中途换配置。工作区创建时也应用一次，所以启动时读到的设置直接生效。
- 签名默认关闭：只有勾选“默认签名提交”才注入 `commit.gpgsign=true`。签名格式只接受 `openpgp` 或 `ssh`，其他值在加载设置时清空，交给 Git 的默认值。
- 命令路径是否存在的检查只提示、不阻止保存：`ssh`、`gpg` 可能只在用户的 shell PATH 里，GUI 进程的 PATH 看不到；提示行显示找不到的程序名。
- 设置对话框分成 General 与 Git 两页。签名格式用单选按钮而不是下拉：对话框内的弹层会被焦点陷阱挡住（现有框架的普遍限制，push 对话框同样如此），单选按钮既可直接点击也可用键盘。
- 每次保存都会把设置推给 Runner，所以改完设置后下一次 fetch、commit、push 就用新的命令与签名参数，不需要重启。

## 代价

- 每条命令多几个 `-c` 参数和一次环境变量拼接，开销可以忽略。
- 开启签名后每次 commit、tag 都会调用 `gpg` 或 `ssh-keygen`；失败信息来自签名工具本身。Viff 不提供 passphrase 输入（`GIT_TERMINAL_PROMPT=0`），需要 gpg-agent、ssh-agent 或 credential helper 已经解锁。
- credential helper 通过 `-c` 追加，与全局已配置的 helper 同时存在，Git 会按顺序尝试；用户若想完全替换，需要清空全局设置。
- 设置按应用全局生效，不能按仓库覆盖。

## 复审条件

需要按仓库覆盖这些设置、需要在应用内管理凭据（askpass）、需要在提交前预览签名，或需要为 SSH 命令提供参数编辑与文件选择器时，重新设计这一块，而不是继续往设置页加输入框。
