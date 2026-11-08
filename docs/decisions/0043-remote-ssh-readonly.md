# 0043 通过 SSH 只读观察远程仓库

- 状态：已采用（2026-11）

## 背景

issue #70 的场景是代码在远程开发服务器上，用户希望用 Viff 看服务器上的 Git 状态，而不是只能在本机克隆一份再看。此前的 SSH 支持只覆盖远端地址（clone/fetch/push 由系统 `git` 自己调 `ssh`），打开的工作区仍然必须是本机路径。

把服务器目录挂到本机（SSHFS）看起来最省事，但会让两套 Git 语义碰同一个 index：换行、文件模式、hooks、凭据和签名都以客户端为准；每次 `stat` 还要付网络往返。WSL 支持当时就是为了避免同类问题，才改成用发行版内的 Git（[0038](0038-wsl-repositories.md)）。远程仓库没有 9P 那样的本地路径可用，所以需要另一条边界。

## 替代项

- **SSHFS/网络挂载后用本机 Git。** 不承诺。语义分叉、性能与 FUSE 依赖都在，无法保证“看到的”就是服务器 Git 看到的状态。否决。
- **在 Go 里自己实现 SSH 与 SFTP。** 会绕开用户的 `~/.ssh/config`、跳板机、agent 与 known_hosts，还要引入新的加密/文件协议依赖。改用系统 `ssh` 后这些全部继承，也不需要新依赖。否决。
- **在服务器上运行 Viff 的远端 agent。** 需要安装、升级、鉴权和常驻进程，违背单进程桌面应用的前提。否决。
- **只支持本机 clone。** 当前已有，但看不到服务器工作区里未提交的改动，正是本条需求的缺口。

## 决定

### 1. 身份与传输

远程仓库的身份是 `ssh://[user@]host[:port]/绝对路径`，路径保持用户输入的原始字节（允许空格与非 ASCII），只规范 authority 与结尾斜杠。`domain.Repo.Root/MetaDir/CommonDir` 使用该身份；写锁、最近项目、缓存与 `samePath` 因此天然按主机区分。`internal/sshpath` 只做词法转换，和 `wslpath` 同一位置。

每条 Git 命令由系统 `ssh` 执行：

```
ssh -T -o BatchMode=yes -o ConnectTimeout=10 ... host \
  env GIT_TERMINAL_PROMPT=0 GIT_OPTIONAL_LOCKS=0 LC_ALL=C LANGUAGE=C \
  git -C '<服务器路径>' '<参数>'...
```

`BatchMode` 让缺少密钥、口令或未知主机立即失败，而不是等待 GUI 无法提供的提示；`StrictHostKeyChecking` 保持用户配置，不静默信任主机。参数逐个单引号转义，本地不经过 shell。连接、主机密钥、认证与服务器缺少 Git 使用独立的错误码（`ssh.connect`、`ssh.host_key`、`ssh.auth`、`ssh.git_missing`、`ssh.missing`）。

服务器上的 Git 使用它自己的配置：`user.name/email`、签名、hooks、credential helper 与远端凭据都取自服务器。本机设置里的 Git 路径、签名、凭据助手与 SSH 命令不注入远程命令，这与 WSL 仓库的规则一致。

### 2. 只读

这一版是观察模式，**任何写操作都不执行**：

- `Runner.command` 对 `ssh://` 仓库运行命令白名单，写命令在启动 `ssh` 前就返回 `repo.read_only`。白名单按子命令与参数形态判断（`branch`/`tag`/`stash`/`remote`/`config`/`worktree`/`submodule`/`lfs` 等只允许读形态），缺项一律拒绝。
- 应用层的写任务入口（`Workspace.run`）再次拒绝 `ssh://` 仓库，避免某条快捷键绕过界面。
- 界面隐藏或禁用写入口：暂存/提交/回滚、分支与标签写操作、merge/rebase/cherry-pick/revert/reset、stash、fetch/pull/push、worktree 创建/删除、submodule 增删/更新、应用内编辑、冲突接受某侧、交互式 rebase、终端、外部打开、ripgrep 搜索与后台自动 fetch。worktree/submodule 列表面板仍可打开，面板内的写按钮禁用。
- 没有只读的 fetch：fetch 会更新远端跟踪引用，属于写操作，留给服务器或终端。

### 3. 工作区文件读取

diff、历史、引用、提交详情与文件树都来自 Git 输出，不需要额外通道。工作区当前文件（文件查看、图片 diff 的新侧、冲突原文、未跟踪文件 diff）通过一次 `sh -c` 脚本读取：脚本以路径为 `$1`，先判断符号链接/目录/缺失/普通文件，输出一行头（`L`/`D`/`M`/`F <size> <mode>`），再输出内容；超过 `ContentLimit` 时只取前 8000 字节用于“过大/二进制”判断。文件只读，不存在远程写入路径。

### 4. 刷新

SSH 没有本地文件事件。活动远程仓库每 5 秒跑一次 `git status`，结果字符串变化时触发完整刷新；另外每第 6 次轮询（约 30 秒）触发一次完整元数据刷新，以便看到服务器上新建的分支/标签或操作状态变化，因为 `git status` 看不到纯引用变化。失败只通知一次，不自动重试写操作。5 秒是保守值，未做性能测量；连接复用（ControlMaster）留到实测后再加。

### 5. 本机专属能力

远程模式下禁用：应用内编辑、冲突逐块接受、终端、外部编辑器/文件管理器、ripgrep（`git grep` 仍可用）、worktree 创建/删除、submodule 增删、交互式 rebase/history rewrite、fetch/pull/push。worktree/submodule 列表面板仍可打开，写按钮禁用。“只读”指 Viff 不发送写命令、不写服务器上的文件；服务器自己的 Git 配置/hook 仍可能执行，这是用户服务器环境的一部分。服务器需要是 POSIX 系统（Linux/macOS）并安装 Git；Windows 的 OpenSSH 服务器不在这一版范围。认证只支持已加载的 key/agent，不提供密码或口令弹窗。

## 代价

- 每次命令至少一个 SSH 往返；大仓库的首次状态、diff 与文件查看明显慢于本地，且没有连接复用。
- 没有文件事件，外部修改最多晚 5 秒出现；轮询本身也消耗网络。
- 不能登录密码认证；主机密钥需要先在终端信任，macOS 从 Dock 启动时可能拿不到终端里的 `SSH_AUTH_SOCK`。
- 服务器上的 Git 版本、hooks、签名与凭据决定行为；需要 TTY 的 hook 或签名口令可能失败。
- 只能观察，不能在本机改文件；冲突需要在服务器上解决后再回 Viff 标记。
- 传输脚本使用 POSIX `sh` 与 `head -c`，不支持 Windows 服务器。

## 复审条件

- 需要密码/口令或一键信任主机时，先设计独立的 askpass/主机密钥确认交互，不把密码放进参数或日志。
- 实测轮询或首次刷新明显影响交互时，评估 SSH ControlMaster 或批量查询，并记录测量环境。
- 需要远程编辑、冲突接受、终端或 fetch/push 时，先补安全写入与取消语义，再单独评估是否仍保持只读默认。
- 需要支持 Windows 服务器时，重新设计远程 shell 的引号与脚本层，而不是在 POSIX 路径上打补丁。
- 需要批量读取工作区文件（例如整树 diff）时，评估 SFTP 或批量 `cat`，避免每条命令一个连接。
