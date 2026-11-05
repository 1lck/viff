# 功能指南

用 Go 和 [Viva](https://github.com/vivaui/viva) 编写的 Git 桌面客户端。界面与交互参照最新版 IntelliJ IDEA 的新 UI（Islands 主题），目标是低内存，并保证 Git 状态正确。

设计文档见 [docs/design.md](design.md)，架构决策见 [docs/decisions](decisions/README.md)。

## 功能

- **欢迎页**：最近项目（支持搜索和移除，不存在的路径标红）、打开、克隆、新建仓库，也可以把文件夹拖进窗口。
- **多仓库工作区**：欢迎页或项目菜单的 Repositories 可添加仓库、统一查看分支与改动、切换和移除条目。点击文件会先切到所属仓库再查看 Diff；移除不删除磁盘文件，切换保留当前窗口内各仓库的提交说明草稿。
- **Commit 工具窗口**
  - 分组：Merge Conflicts / Staged / Changes / Unversioned Files，可按目录分组。
  - ChangeList：创建、重命名、删除清单，右键分配文件并按清单筛选。筛选仅影响未暂存和未跟踪文件；始终显示全部已暂存文件，提交仍包含全部暂存内容。筛选后的批量暂存只作用于可见文件。
  - 暂存：悬停行上的 +/−，空格键，多选（Shift、⌘）后批量暂存，回滚，加入 .gitignore。
  - 提交：Amend，Sign-off，跳过钩子，⌘↩ 提交，“Commit and Push…”。
  - 正在进行 merge、rebase 等操作时显示横幅，带 Continue / Abort。
- **Project 面板（只读）**：左侧 stripe 的文件夹图标或 ⌘1 打开，与 Commit 面板共用停靠区。顶部是仓库根节点（名称 + 路径）、工具条（Refresh、Collapse All、Expand All、Open File、Hide）和过滤框；下面是工作树的索引文件与未跟踪文件（ignored 除外），按目录折叠，按 Git 状态给文件名着色。单击文件在预览页签中只读打开，双击固定；右键可外部编辑、看 blame、看文件历史、复制路径或在 Finder 中显示。⇧⌘O 打开 Quick Open，输入路径片段后回车打开，见 [0024](decisions/0024-file-browser.md)。
- **Stash 页签**：暂存（可包含未跟踪文件、保留索引）、应用、Pop、删除、转为分支，并可查看 stash 中的文件 diff。
- **补丁**：变更列表右键“Create Patch…”把选中的改动保存为 `.patch`（二进制和 CRLF 原样保留，重命名仍是 rename）；Git 工具窗口菜单可以把补丁应用到工作树或只应用到索引，应用失败时文件不变并显示 Git 的原始错误。
- **Diff**
  - 并排与统一两种视图，同步滚动，曲线连接相同块，词级高亮，按语言语法着色。
  - 部分暂存：块按钮（→ ← + ⟲）可暂存、取消暂存或回滚单个块，也可以在行号栏选中几行后只暂存这些行。
  - F7 / ⇧F7 在差异之间跳转，⌘F 搜索，可忽略空白差异，F4 用外部编辑器打开。
  - 预览页签跟随选中的文件；双击或 ⌘D 固定页签。
  - 图片预览：PNG / JPEG / GIF（仅首帧）在 diff 中并排显示，等比缩放并标注格式、像素尺寸与文件大小；超过内容读取上限的图片仍按“文件过大”处理。
- **冲突查看器**：Ours / Result / Theirs 三栏，在冲突块间跳转，可逐块接受 Ours / Theirs / Both；修改后先检查结果，再标记为已解决。支持在外部编辑器中自由编辑。
- **文件查看**：只读打开某个提交里的文件（“Open Repository Version”），显示行数、换行符，以及二进制、超大文件和子模块的说明；可定位到当前行用外部编辑器打开。
  - **Git blame 注解**：按提交新旧着色，显示作者与日期；点击注解在 Log 中定位该提交，右键可看该提交的 diff 或注解上一个版本。
- **浏览提交的仓库树**（“Browse Repository at Revision…”）：在 Log 里右键一个提交，列出该提交里的全部文件（路径、大小，并标出可执行、符号链接和子模块），可按路径过滤，双击或回车以只读方式打开该版本。
- **子模块**：Git 工具窗口的 ⋯ 菜单列出子模块的状态（未初始化 / 已修改 / 冲突）、当前提交和 describe，可 Update、Sync 或打开子模块；变更列表里子模块条目的右键菜单也有 Update Submodule 与 Open Submodule。递归子模块一并列出；可以添加子模块，或确认后移除、停用（deinit）当前仓库的直接子模块，脏内容需要明确确认强制操作。
- **Git 工具窗口**
  - Log：分支树；提交图；引用标签；按文本或哈希、分支、作者、日期、路径过滤；懒加载。
  - 提交详情：文件列表和可滚动的完整提交信息；多选提交显示受影响文件的去重并集，点击文件可查看来源提交的 diff。
  - 提交右键菜单：cherry-pick、revert、reset（Soft/Mixed/Hard/Keep）、撤销提交、检出、新建分支或标签、与工作树比较；选中两个提交可比较它们之间的文件。
  - 直接编辑历史：修改提交信息、把提交 squash 进父提交、删除提交（drop），冲突时自动 abort 并保持分支不变。
  - Console 显示所有执行过的 Git 命令及其输出，支持长行横向滚动；Reflog 页签可以查看 reflog。
- **分支弹窗**（⌥⇧`）：速搜、收藏、本地和远程分支的子菜单，可执行检出、新建、重命名、合并、rebase、比较、设置上游、推送、删除；“Show Changed Files”列出两个分支 tip 之间的文件，双击打开 diff。
- **Smart Checkout**：工作区有未提交改动时切换分支或检出修订不再直接失败——Git 因改动会被覆盖而拒绝后弹出确认，选“Stash and Checkout”会连同未跟踪文件一起 stash，检出后自动恢复；恢复冲突时改动留在 stash 并提示，检出失败则恢复改动、当前分支不变。
- **工作树（Worktrees）**：分支弹窗底部的“Worktrees…”列出仓库的所有工作树（分支、短 HEAD、路径，当前工作树有标记），可以新建（创建新分支或选择未被占用的已有本地分支，路径按分支名建议）、删除（可强制丢弃改动，分支保留）、打开为当前仓库、复制路径；已被其他工作树检出的分支在分支菜单里标注，检出会打开那个工作树。
- **推送和更新**：推送对话框列出待推送的提交，可选推送标签和 force-with-lease；更新支持 merge 或 rebase 方式；也支持 Fetch、后台定时 Fetch 和管理远程仓库。
- **推送变体**：Log 右键“Push Up to Commit…”把某个历史提交及其之前的提交推到远端分支；分支弹窗“Push to Another Remote…”把 HEAD 推到别的远端或分支名；标签菜单可以只删除远端标签（本地标签保留）。非 fast-forward 默认被 Git 拒绝并显示原文，强制推送需显式勾选 force-with-lease。
- **更新未检出的分支**：分支弹窗里非当前分支的本地分支有“Update from Remote”，快进到远端同名分支并说明前进了几个提交；分叉时拒绝且分支不变。浅克隆时 Git 工具窗口的 ⋯ 菜单会多出“Fetch Full History (Unshallow)”。
- **Git 设置**：设置对话框的 Git 页可以指定 SSH 命令、GPG 程序、签名格式（Git 默认 / OpenPGP / SSH）、签名密钥、默认签名提交与 credential helper；保存后对下一次 Git 调用立即生效，不改写仓库或全局 Git 配置。
- **通知**：右下角气泡（成功消息 10 秒后自动隐藏），以及通知工具窗口。
- **应用内更新**：`Help → Check for Updates…` 或启动时自动检查；发现新版本后应用内下载、用 `SHA256SUMS` 校验，点“Restart to Update”自动替换并重启，不需要打开浏览器。Windows 使用便携 ZIP，Linux 便携包使用 tar.gz；DEB 安装需要 `sudo apt install` 完成。
- **主题**：Islands Dark、Islands Light，或跟随系统。

## 构建与运行

需要 Go 1.27 或更高版本，以及本机安装的 `git`。在 macOS 上编译需要 Xcode Command Line Tools，因为字体注册和 FSEvents 用到了 cgo。

```sh
go run ./cmd/viff                 # 打开上次的项目，没有则显示欢迎页
go run ./cmd/viff ~/code/project  # 打开指定仓库
go build -o viff ./cmd/viff
```

设置保存在用户配置目录的 `Viff/settings.json`（macOS 上为 `~/Library/Application Support/Viff/settings.json`） 中，也可以通过设置对话框（⌘,）查看路径。

## 常用快捷键

| 快捷键        | 动作                             |
| ------------- | -------------------------------- |
| ⌘K / ⇧⌘K / ⌘T | 提交 / 推送 / 更新项目           |
| ⌥⇧`           | 分支弹窗                         |
| ⌘D            | 显示选中项的 diff                |
| F7 / ⇧F7      | 下一个 / 上一个差异              |
| ⌘F            | 在 diff 中搜索，或聚焦日志搜索   |
| ⌘0 / ⌘9       | 显示或隐藏 Commit / Git 工具窗口 |
| ⌥⌘Z           | 回滚                             |
| ⌘W、⇧⌘[ / ⇧⌘] | 关闭或切换编辑器页签             |
| ⌘R            | 刷新                             |

## 测试

```sh
go test ./...                                            # 领域、Git 解析与命令、工作区、监听器
go test -tags vivahead ./internal/ui/                    # 无界面 UI 测试：提交流程、日志搜索、stash、冲突等
VIFF_SHOTS=/some/dir go test -tags vivahead ./internal/ui/  # 同时把截图写到指定目录
```

所有测试都在 `t.TempDir()` 中创建临时仓库，并隔离 HOME 和 Git 全局配置，不会读写本机的仓库或设置。

## 已知限制

- 多仓库写入操作只针对当前仓库；非活动仓库在打开总览或手动刷新时更新。ChangeList 是文件分组，不提供独立索引或跨仓库提交，见 [0020](decisions/0020-repositories-changelists.md)。
- 已在 macOS，以及 Windows Server 2022 x64（Git 2.56.0.windows.4）上验证 Git 功能和 Viva 无头界面；Windows 10/11 的实际桌面交互、输入法及安装分发尚未验收。Linux 和 Windows 的文件监听仍是轮询，见 [0003](decisions/0003-file-watching.md)。
- 界面字体默认为系统字体（SF Pro），没有使用 Inter；设置里可分别选择「界面字体」（全部已安装族）与「diff 字体」（仅等宽族，用于 diff、控制台与哈希），见 [0002](decisions/0002-fonts.md)。
- 冲突支持逐块接受两侧内容；自由文本编辑、二进制或不可靠冲突标记仍需要外部编辑器，见 [0019](decisions/0019-git-issue-gaps.md)。
- 已有分支创建工作树见 [0018](decisions/0018-existing-branch-worktree.md)；prunable 工作树只能查看、不能删除。嵌套子模块的移除和 deinit 需要先打开其所属仓库，见 [0019](decisions/0019-git-issue-gaps.md)。
- Project 面板与 Commit 共用左侧停靠区，不能同时显示；默认显示 Commit，按 ⌘1 打开 Project。ignored 文件默认不显示，折叠状态不持久化；图片仍只显示二进制说明；文本支持鼠标拖选、Shift+方向键与 ⌘A/⌘C 复制（制表符按显示展开为空格），但不支持编辑与拖选自动滚动。见 [0024](decisions/0024-file-browser.md)。
- 应用内更新没有系统签名校验（macOS ad-hoc 签名、Windows 无 Authenticode），只做 HTTPS + SHA-256；宿主的系统安全提示仍可能出现。DEB 安装受权限限制只能提示手动安装；随包文档在 Windows/Linux 自替换后保持旧版本，见 [0023](decisions/0023-app-update.md)。旧版本（v0.1.0 及更早）没有更新器，升级到第一个带更新器的版本需要手动下载一次。
