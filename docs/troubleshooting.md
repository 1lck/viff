# 故障排查指南

本页帮助你把问题变成"可定位、可复现、维护者能直接处理"的报告。Viff 的日志与诊断包默认留在本机，只有你主动导出并分享时维护者才能看到，详见 [privacy.md](privacy.md)。

## 遇到问题先做什么

1. **确认版本**：打开 `Help → About Viff`，复制其中的版本信息（版本号、commit、构建日期、Go/Viva/Git 版本）。发布版和源码构建的排查方式不同。
2. **用最小仓库复现**：尽量新建一个临时仓库（`git init`）复现，不要把私有仓库内容带进 Issue。记录最少的操作步骤。
3. **看错误码和 Console**：错误通知里通常会写 `Code: …`；Git 工具窗口的 Console 会保留失败命令、退出码和 Git 原始输出。两者是定位问题的第一手证据。
4. **先搜现有 Issue**：可能已经有人报告或已有临时办法。
5. **安全问题走私密渠道**：凭据泄露、命令注入、越界读写等不要开公开 Issue，见 [SECURITY.md](../SECURITY.md)。
6. **生成诊断包并附到 Issue**：按本文"如何生成诊断包"操作，上传前先预览内容。

## 错误码表

错误通知和诊断包里会出现稳定错误码（`Code: …`）。报告问题时直接写出错误码，能省掉一轮追问。错误码与 Git 失败原因对应，原始 Git 输出不会被覆盖，仍保留在通知详情和 Console 中。

| 错误码 | 含义 | 用户下一步 | 需要什么证据 |
| --- | --- | --- | --- |
| `auth.failed` | 认证失败：HTTPS 令牌/凭据助手/SSH key 不被远端接受 | 检查 remote 地址、凭据助手或 SSH agent；重新登录/更新令牌后重试。**不要在 Issue 里贴令牌** | 错误码、remote 主机名（不要完整 URL）、Git 原始错误摘要、`git --version`、操作系统 |
| `repo.locked` | 仓库被锁：`.git/index.lock` 等锁文件存在，或另一个 Git 进程正在操作 | 关闭 Viff 和终端里的其他 Git 命令/编辑器；确认没有 Git 进程后再删除对应锁文件（不要盲删） | 错误码、锁文件名、是否有其他 Git 工具同时操作、诊断包 |
| `repo.not_a_repository` | 路径不是 Git 仓库，或 `.git` 损坏、被移动、权限不可读 | 确认目录里有有效 `.git`；从正确路径重新打开；用命令行 `git status` 检查仓库是否正常 | 打开方式、目录结构说明（可脱敏）、`git status` 输出、诊断包 |
| `git.conflict` | 合并/变基/拣选/应用补丁产生冲突 | 在冲突查看器逐块解决并标记，然后用横幅的 Continue/Abort 收尾；中途不要关闭仓库 | 冲突文件列表、当前操作状态（Merge/Rebase/Cherry-pick）、诊断包 |
| `git.not_found` | Git 对象、引用、分支、stash 或路径不存在（可能被删除、gc、外部工具改动） | 刷新仓库后重试；确认对象是否仍存在；外部操作后重新打开仓库 | 操作对象名（分支/短哈希/相对路径）、操作类型、`git fsck` 输出（若有） |
| `push.rejected` | 远端拒绝推送：非快进、分支保护、权限不足或 force-with-lease 过期 | 先 fetch/pull 并处理后，再推送；确需强推时确认 lease 保护对象。不要直接强推覆盖他人提交 | Git 原始错误（non-fast-forward / protected branch 等）、远端主机、是否有人同时推送 |
| `worktree.untracked_overwrite` | 未跟踪文件会被操作覆盖（checkout/switch/pull 等目标位置已有同名未跟踪文件） | 把该文件移走、改名、加入 Git 或删除后再操作；确认该文件是否还需要 | 冲突的相对路径、操作类型、诊断包 |
| `worktree.local_changes` | 本地修改会阻止操作（checkout/merge/rebase/reset 等会覆盖这些修改） | 先提交、stash 或丢弃修改再重试；丢弃前确认文件内容 | 修改文件列表、操作类型、是否已 stash |
| `branch.not_merged` | 删除分支被拒：分支上还有未合并的提交 | 先合并/变基，或确认无需保留后强制删除；删除前记录分支名与未合并提交 | 分支名、未合并提交数或短哈希、`git log --oneline <分支>` 片段 |
| `commit.empty` | 没有可提交的改动（暂存区为空，或改动被忽略规则排除） | 先暂存文件；检查 `.gitignore` 与文件状态；amend 场景确认是否本来就无改动 | 暂存区状态、文件列表、操作类型（普通提交/amend） |
| `hook.failed` | Git hook 失败（pre-commit、commit-msg、pre-push 等） | 阅读 hook 输出；修复 hook 或在提交对话框勾选跳过 hooks（跳过前确认代价）。检查 hook 脚本权限与解释器 | hook 名称、原始输出、操作系统、是否通过终端可复现 |
| `git.missing` | 找不到 `git` 可执行文件 | 安装 Git：macOS 安装 Git 或 Xcode Command Line Tools；Windows 安装 Git for Windows；Linux 用包管理器。确认 Git 在 `PATH` 中，重启 Viff | 操作系统、安装方式、终端 `git --version` 输出、`PATH` 大致情况（不含敏感路径） |
| `wsl.unavailable` | 打开 WSL 仓库，但本机没有可用的 WSL | 在终端运行 `wsl --install`，安装完成后重启；确认 `wsl.exe` 在 `PATH` 中且 `wsl --status` 可用 | Windows 版本、`wsl --status` 输出、`wsl.exe --version` |
| `wsl.distro_missing` | 路径里的发行版名不存在 | 检查路径写法：`\\wsl.localhost\<发行版>\home\<用户>\<仓库>`；用 `wsl --list` 确认发行版名称 | 完整的仓库路径、`wsl --list --verbose` 输出（可脱敏用户名） |
| `wsl.git_missing` | 发行版里没有安装 Git | 在发行版内安装：`wsl -d <发行版> -- sudo apt install git`（或对应发行版的包管理器），然后重试 | 发行版名称、发行版内 `git --version` 输出、发行版类型与版本 |
| `git.cancelled` | 操作被取消：手动取消、切换/关闭仓库、退出，或任务超时 | 重新执行操作；若反复取消，检查是否仓库过大、是否有外部操作/退出触发；看 Console 最后一条命令 | 什么操作、是否手动取消、诊断包（含时间线） |
| `git.failed` | 其他 Git 失败：权限不足、磁盘满、网络/代理异常、Git 版本过旧、仓库损坏等（未分类兜底） | 按 Console 的 Git 原始错误判断：检查磁盘空间、目录写权限、代理设置、`git --version`；仍无法解决时导出诊断包 | Git 原始错误、`git --version`、磁盘剩余空间、代理设置（不要贴凭据） |

> 提示：日志与诊断包里同时记录 `code` 和 `kind`，按错误码搜索 Issue 或日志都能对应上。

## 常见场景

### 启动闪退 / 双击没反应

1. 启动失败时 Viff 会尽量弹出系统消息框，里面通常包含日志路径与下一步建议；先截图或复制这段文字。
2. 再次启动 Viff：如果上次是异常退出，会看到"上次异常退出"的通知，可点 `Report a Problem…` 或 `Open Log Folder`。
3. 打开日志目录上一级的 `crash/`，找到最新的 `last-crash-*.txt`（包含 panic 栈）；把它和诊断包一起附上。
4. macOS 若提示"无法打开 Viff.app"（Gatekeeper），先按 [README.zh-CN.md](../README.zh-CN.md) 的 macOS 一节处理，再判断是否为程序问题。
5. Windows 的 GUI 构建没有可见的 stderr，启动失败优先看系统消息框、`%AppData%\Viff\crash\` 与日志目录。

### Windows 打开后一直闪黑框

- 现象：Windows 上打开 Viff 后，黑色控制台窗口连续闪现，空闲时也周期性出现，界面几乎无法操作。
- 原因：Windows GUI 构建自身没有控制台，系统会为它启动的每条控制台子命令新建一个控制台窗口。Viff 的仓库刷新会连续执行多条 `git` 命令，轮询监听每 6 秒触发一次刷新，所以黑框会反复出现。
- 处理：升级到包含该修复的版本。v0.1.0-alpha.2 及更早的 Windows 构建存在此问题；升级后如果仍持续闪黑框，按"如何生成诊断包"导出诊断包，并说明是否有外部终端、编辑器或 hook 参与。

### 提示找不到 Git（`git.missing`）

- 在终端运行 `git --version`。如果终端也没有，先安装 Git 并重启 Viff。
- macOS 可安装 Xcode Command Line Tools 或 Git 官方安装包；Windows 用 Git for Windows；Linux 用发行版包管理器。
- 如果终端有而 Viff 找不到，可能是从桌面图标启动时 `PATH` 不同：用终端启动一次 Viff，或修正系统 `PATH` 后重启。
- 附上操作系统、安装方式和 `git --version`。

### 认证失败（`auth.failed`）

- HTTPS：检查凭据助手（Git 设置里配置的 credential helper）或令牌是否过期；私有仓库需要有效令牌。
- SSH：确认 `ssh -T git@<主机>` 能通过、SSH agent 里有可用 key、remote 用的是 SSH 地址。
- **不要**把令牌、口令、完整 remote URL 贴进 Issue；诊断包默认已隐藏这类信息，但仍请预览。
- 附上错误码、remote 主机名和 Git 原始错误摘要。

### Git hooks 失败（`hook.failed`）

- 在终端手动执行同一条 Git 命令，确认是哪个 hook 失败；hook 输出通常就是原因（格式检查、提交信息规则、签名等）。
- 确认 hook 文件可执行、解释器（如 `#!/usr/bin/env python3`）存在。
- 提交对话框可以勾选跳过 hooks（临时绕过时使用，确认不会破坏项目规则）。
- 附上 hook 名称、原始输出和操作系统。

### 锁文件（`repo.locked`）

- 常见原因：另一个 Git 进程、IDE、终端命令或 Viff 自身的前一个操作未结束。
- 先关闭其他 Git 工具并等待操作结束；确认没有 `git` 进程后再删除 `.git/index.lock`。
- 如果在 NFS/网络盘、iCloud/Dropbox 同步目录里，锁问题会更频繁；可把仓库移到本地磁盘复现。
- 附上锁文件名、是否多工具同时操作、诊断包。

### 权限不足 / 磁盘满

- 检查仓库目录与 `.git` 是否可写（文件属主、只读挂载、受保护目录）。
- 检查磁盘剩余空间；Git 的临时对象和 `gc` 需要额外空间。
- 错误通常落在 Git 原始输出里、错误码可能是 `git.failed`；附上原始错误、磁盘剩余空间和仓库位置的大致说明（不要贴完整私有路径）。

### 网络 / 代理问题

- 确认 `git ls-remote <remote>` 能在终端成功；失败时按 Git 的网络错误排查代理、VPN、DNS。
- 代理变量（`http_proxy`/`https_proxy`/`no_proxy`）只写变量名与是否启用，不要贴带凭据的值。
- 错误码可能是 `auth.failed`、`push.rejected` 或 `git.failed`；附上 Git 原始错误与网络环境说明。

### 卡顿 / 内存占用高 / 大仓库慢

1. `Help → Report a Problem…`，在描述里写明仓库规模（提交数、文件数、是否浅克隆/子模块/worktree）。
2. 复现卡顿（滚动、切换仓库、打开 diff 等）约 10–60 秒。
3. `Help → Stop Recording and Save Diagnostics…` 保存；诊断包含任务与刷新耗时、错误时间线，便于判断是 Git 慢还是界面慢。
4. 若问题只在特定仓库出现，说明该仓库特征，并尽量用脱敏后的临时仓库复现。

### 仓库打不开 / 仓库损坏（`repo.not_a_repository`、`git.not_found`）

- 在终端进入仓库执行 `git status`、`git fsck`，确认 Git 本身能否读取；先用 Git 自身修复，再回 Viff 打开。
- 如果是外部工具正在执行操作（变基、合并、gc），等它结束并刷新。
- 附上 `git status` / `git fsck` 输出与诊断包。

## 如何生成诊断包

### 方式一：录制复现过程（推荐）

1. 打开 `Help → Report a Problem…`；若错误通知上提供 `Report Problem…` 按钮，也可以从那里开始。
2. 在对话框里填写问题描述（可选，但写清"哪个操作失败"很有帮助），确认开始记录。
3. 回到界面复现问题；期间 `Help` 菜单会变成 `Stop Recording and Save Diagnostics…`，表示正在记录。
4. 复现后打开 `Help → Stop Recording and Save Diagnostics…`。
5. 在保存对话框选择位置和文件名（默认类似 `viff-diagnostics-<日期>.zip`），保存。
6. 导出成功后界面会给出 zip 路径；若提供 `Copy Issue Template`，可把模板粘贴到 Issue 正文。

### 方式二：直接导出当前会话

没有提前录制时，用 `Help → Save Diagnostics…` 直接导出**最近 30 分钟**的事件与 Console 摘要，然后选择保存位置。

### 诊断包内容与限制

- 总大小上限约 5 MiB；事件、Console、崩溃记录超限时截断，并在 `manifest.json` 中标记。
- 包含 `summary.md`（人读摘要）、`manifest.json`、`env.txt`、`settings-summary.json`、`events.jsonl`、`console.txt`、`repo.txt`，以及 `crash/`（如有）。
- 不包含仓库文件内容、diff、提交说明；路径与 remote 默认脱敏。详细说明见 [privacy.md](privacy.md)。

### 把诊断包附加到 Issue

1. 先解压或用系统压缩工具打开 zip，查看 `summary.md` 与 `events.jsonl`，确认没有不想公开的信息。
2. 在 GitHub 创建 Issue 后，把 zip 拖进 Issue 正文即可上传附件（Issue 表单一提交后也可以继续编辑添加附件）。
3. **公开 Issue 是公开场所**：附件会长期公开可见；敏感内容请先删除或改用私密渠道。
4. 若 zip 太大或不便公开，至少附上 `summary.md` 的相关片段、错误码、操作步骤和 Console 中的失败命令。
5. 安全漏洞不要附在公开 Issue，走 [私密漏洞报告](https://github.com/1lck/viff/security/advisories/new)。

## 如何打开日志目录

- 应用内：`Help → Open Log Folder`，会打开日志目录；若提示目录为空或不存在，先重启 Viff 并操作一次再打开。
- 手动路径：

| 平台 | 日志目录 |
| --- | --- |
| macOS | `~/Library/Application Support/Viff/logs/` |
| Windows | `%AppData%\Viff\logs\` |
| Linux | `~/.config/Viff/logs/`（遵循 `$XDG_CONFIG_HOME`） |

- 日志文件为 `viff-*.jsonl`；崩溃记录在同级 `Viff/crash/` 目录。
- Git 工具窗口的 Console 只保存在内存中，退出即丢失；需要 Console 内容时请先导出诊断包或用 Console 的复制/保存功能。

## 平台差异

- **路径**：日志、崩溃记录、设置都在各平台的用户配置目录（见上表和 [privacy.md](privacy.md)），不在仓库目录里。
- **macOS**：首次打开可能遇到 Gatekeeper 提示，按 [README.zh-CN.md](../README.zh-CN.md) 的 macOS 一节处理；无头/远程环境不能代替真实桌面验证。
- **Windows**：GUI 构建不显示 stderr，启动失败没有终端输出；请依赖系统消息框、`crash/` 与日志。路径分隔符和长路径、非 ASCII 路径问题请在报告中注明。
- **Linux**：需要可用的图形/渲染环境与依赖（见 README 的平台支持进度）；报告时请写清发行版、桌面环境（GNOME/KDE…）与显示服务器（X11/Wayland）。
- **语言与编码**：仓库路径含中文、空格、emoji 时请说明；日志中的仓库路径是哈希，Issue 里可以用脱敏后的相对路径描述。

## 提交 Issue 前的检查清单

- [ ] 已从 `Help → About Viff` 复制版本信息（版本号、commit、构建日期）。
- [ ] 已用临时仓库写出最少复现步骤，并说明是发布版还是源码构建。
- [ ] 已写明错误码（如 `push.rejected`）和 Git 原始错误的关键行。
- [ ] 已附上诊断包，或 `summary.md` 的相关片段；上传前已预览。
- [ ] 已搜索现有 Issue。
- [ ] 已移除令牌、口令、完整私有 URL、私有仓库内容。
- [ ] 如果是安全问题，已改用私密漏洞报告而不是公开 Issue。

## 相关文档

- 隐私与数据说明：[privacy.md](privacy.md)
- 安全政策与私密报告：[SECURITY.md](../SECURITY.md)
- 构建、平台支持与已知限制：[README.zh-CN.md](../README.zh-CN.md)
- 维护者设计说明：[observability.md](observability.md)
