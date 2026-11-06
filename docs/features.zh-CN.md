# 功能指南

用 Go 和 [Viva](https://github.com/vivaui/viva) 编写的原生 Git 客户端，内置只读项目浏览、中英双语界面与可选的 AI 提交说明。界面与交互参照最新版 IntelliJ IDEA 的新 UI（Islands 主题），目标是低内存，并保证 Git 状态正确。

设计文档见 [docs/design.md](design.md)，架构决策见 [docs/decisions](decisions/README.md)。

## 功能

- **欢迎页**：最近项目（支持搜索和移除，不存在的路径标红）、打开、克隆、新建仓库，也可以把文件夹拖进窗口。
- **多仓库工作区**：欢迎页或项目菜单的 Repositories 可添加仓库、统一查看分支与改动、切换和移除条目。打开不是仓库的父目录时，Viff 会提供发现对话框，扫描并勾选其中的子仓库批量导入（包括仓库内嵌套的仓库、submodule 与 linked worktree）；扫描在后台执行、可取消，结果不会自动改变工作区，见 [0036](decisions/0036-repository-discovery.md)。点击文件会先切到所属仓库再查看 Diff；移除不删除磁盘文件，切换保留当前窗口内各仓库的提交说明草稿。
- **Commit 工具窗口**
  - 分组：Merge Conflicts / Staged / Changes / Unversioned Files，可按目录分组。
  - ChangeList：创建、重命名、删除清单，右键分配文件并按清单筛选。筛选仅影响未暂存和未跟踪文件；始终显示全部已暂存文件，提交仍包含全部暂存内容。筛选后的批量暂存只作用于可见文件。
  - 暂存：悬停行上的 +/−，空格键，多选（Shift、⌘）后批量暂存，回滚，加入 .gitignore。
  - 复选框：每行前的复选框可勾选单个文件、目录或整组，暂存、取消暂存、回滚、查看 diff 等批量操作作用于勾选的文件；点框只切换勾选，不触发行本身的单击行为。
  - 提交：Amend，Sign-off，跳过钩子，⌘↩ 提交，“Commit and Push…”。
  - 折叠说明：说明输入区左侧的箭头可把输入框、拖拽条与两个提交按钮一起收起，把空间让给变更树；折叠状态在会话内保留。
  - AI 生成说明：Amend 行右侧的 AI 按钮按当前生效的规则生成一条提交说明并填入输入框，不自动提交；端点、密钥与模型在设置的 AI 页配置，默认关闭，见 [0027](decisions/0027-ai-commit-message.md)。右键该按钮可切换提交信息规则、按项目分别选择或自行编写规则，并有一个「生成前先确认」开关；该开关默认开启，点击按钮先确认再发请求，关闭后点一下即生成，见 [0035](decisions/0035-ai-prompt-rules.md)。按钮提示会显示当前生效的规则名，便于确认这次用的是哪一条。
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
- **冲突查看器**：Ours / Result / Theirs 三栏，在冲突块间跳转，可逐块接受 Ours / Theirs / Both；修改后先检查结果，再标记为已解决。支持在外部编辑器中自由编辑；横幅的 Copy for AI（以及每块的 Copy This Block）把仓库、文件、操作、双方来源与冲突块原文复制到剪贴板，交给外部 AI 修改——Viff 不调用模型、不联网，外部保存后自动刷新，仍由你手动标记已解决。
- **文件查看**：只读打开某个提交里的文件（“Open Repository Version”），显示行数、换行符，以及二进制、超大文件和子模块的说明；可定位到当前行用外部编辑器打开。
  - **Git blame 注解**：按提交新旧着色，显示作者与日期；点击注解在日志中定位该提交，右键可看该提交的 diff 或注解上一个版本。
- **浏览提交的仓库树**（“Browse Repository at Revision…”）：在日志里右键一个提交，列出该提交里的全部文件（路径、大小，并标出可执行、符号链接和子模块），可按路径过滤，双击或回车以只读方式打开该版本。
- **子模块**：Git 工具窗口的 ⋯ 菜单列出子模块的状态（未初始化 / 已修改 / 冲突）、当前提交和 describe，可 Update、Sync 或打开子模块；变更列表里子模块条目的右键菜单也有 Update Submodule 与 Open Submodule。递归子模块一并列出；可以添加子模块，或确认后移除、停用（deinit）当前仓库的直接子模块，脏内容需要明确确认强制操作。
- **Git 工具窗口**
  - 日志：分支树；提交图；引用标签；按文本或哈希、分支、作者、日期、路径过滤；懒加载。点开「分支」后可直接输入名称搜索本地分支、远端分支和标签，用 ↑/↓ 选择、Enter 应用，或按 Esc 取消。
  - 提交详情：文件列表和可滚动的完整提交信息；多选提交显示受影响文件的去重并集，点击文件可查看来源提交的 diff。
  - 提交右键菜单：cherry-pick、revert、reset（Soft/Mixed/Hard/Keep）、撤销提交、检出、新建分支或标签、与工作树比较；选中两个提交可比较它们之间的文件。
  - 直接编辑历史：修改提交信息、把提交 squash 进父提交、删除提交（drop），冲突时自动 abort 并保持分支不变。
  - 控制台显示所有执行过的 Git 命令及其输出，支持长行横向滚动；引用日志页签可以查看 reflog。
- **分支弹窗**（⌥⇧`）：速搜、收藏、本地和远程分支的子菜单，可执行检出、新建、重命名、合并、rebase、比较、设置上游、推送、删除；“Show Changed Files”列出两个分支 tip 之间的文件，双击打开 diff。
- **网页入口**：Log 单提交右键“Open Commit on GitHub/GitLab/…”，分支弹窗里本地分支可打开分支页、与基准分支比较、打开创建 PR / 合并请求的页面。只生成服务地址并用系统浏览器打开，不登录、不调用 API；Bitbucket 不生成比较与创建 PR 页面，改为打开分支页并提示。
- **撤销**：⌘Z 撤销本会话内最近一次本地操作（提交、Amend、merge、rebase、cherry-pick、revert、reset、检出、分支新建/删除/重命名）。撤销前重新校验 HEAD、分支与工作区，状态不匹配就拒绝；保留最近 20 条，切仓库清空，不持久化。
- **Git LFS**：检测仓库的 LFS 规则与受管文件，状态栏显示 LFS 数量；本机缺少 git-lfs 时在提交面板给出警告并提供安装命令（只复制，不执行）。Viff 不管理 LFS 对象，只报告状态。
- **Smart Checkout**：工作区有未提交改动时切换分支或检出修订不再直接失败——Git 因改动会被覆盖而拒绝后弹出确认，选“Stash and Checkout”会连同未跟踪文件一起 stash，检出后自动恢复；恢复冲突时改动留在 stash 并提示，检出失败则恢复改动、当前分支不变。
- **工作树（Worktrees）**：分支弹窗底部的“Worktrees…”列出仓库的所有工作树（分支、短 HEAD、路径，当前工作树有标记），可以新建（创建新分支或选择未被占用的已有本地分支，路径按分支名建议）、删除（可强制丢弃改动，分支保留）、打开为当前仓库、复制路径；已被其他工作树检出的分支在分支菜单里标注，检出会打开那个工作树。
- **推送和更新**：推送对话框列出待推送的提交，可选推送标签和 force-with-lease；更新支持 merge、rebase 或只允许快进，Update Project 对话框里的“不再询问”勾选后，⌘T 直接用记住的方式更新并以通知反馈（设置 Git 页的“询问更新方式”可改回，恢复每次询问）；也支持 Fetch、后台定时 Fetch 和管理远程仓库。
- **推送变体**：日志右键“Push Up to Commit…”把某个历史提交及其之前的提交推到远端分支；分支弹窗“Push to Another Remote…”把 HEAD 推到别的远端或分支名；标签菜单可以只删除远端标签（本地标签保留）。非 fast-forward 默认被 Git 拒绝并显示原文，强制推送需显式勾选 force-with-lease。
- **更新未检出的分支**：分支弹窗里非当前分支的本地分支有“Update from Remote”，快进到远端同名分支并说明前进了几个提交；分叉时拒绝且分支不变。浅克隆时 Git 工具窗口的 ⋯ 菜单会多出“Fetch Full History (Unshallow)”。
- **Git 设置**：设置对话框的 Git 页可以指定 SSH 命令、GPG 程序、签名格式（Git 默认 / OpenPGP / SSH）、签名密钥、默认签名提交与 credential helper，也可以关掉“询问更新方式”让更新不再逐次询问；保存后对下一次 Git 调用立即生效，不改写仓库或全局 Git 配置。
- **终端工具窗**：左侧 stripe 的终端图标或 ⌥F12 在底部打开，当前目录是打开的仓库。支持 5000 行回看、滚轮与滚动条、拖选复制、粘贴（含 bracketed paste）、清屏、重启与拖动高度；隐藏工具窗或切换仓库不结束 shell；重启、关闭项目（有运行中的 shell 时先确认）或关闭窗口才结束进程。实现与边界见 [0028](decisions/0028-integrated-terminal.md)。
- **通知**：右下角气泡（成功消息 10 秒后自动隐藏），以及通知工具窗口。
- **应用内更新**：`Help → Check for Updates…` 或启动时自动检查；发现新版本后应用内下载、用 `SHA256SUMS` 校验，点“Restart to Update”自动替换并重启，不需要打开浏览器。Windows 使用便携 ZIP，Linux 便携包使用 tar.gz；DEB 安装需要 `sudo apt install` 完成。
- **界面语言**：默认英文，设置 → General → Language 可切到简体中文；菜单、对话框、工具窗口与通知随语言切换，Git 命令与输出、路径与哈希、快捷键与日志字段保持英文。
- **主题**：Islands Dark、Islands Light，或跟随系统。
- **窗口位置**：记住上次关闭时的位置和尺寸，包括放在副屏上的位置，下次启动就在原处打开；位置对应的显示器不存在时回落到主屏，见 [0034](decisions/0034-window-geometry.md)。

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
| ⌥F12          | 显示或隐藏终端工具窗             |
| ⌘1            | 打开 Project 面板（与 Commit 共用停靠区） |
| ⇧⌘O           | Quick Open                       |
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

- 多仓库写入操作只针对当前仓库；非活动仓库在打开总览或手动刷新时更新。ChangeList 是文件分组，不提供独立索引或跨仓库提交，见 [0020](decisions/0020-repositories-changelists.md)。从父目录发现仓库按名称跳过依赖/构建目录，默认深度 4、目录 2000、仓库 200；超出或跳过时对话框会提示，见 [0036](decisions/0036-repository-discovery.md)。
- 已在 macOS，以及 Windows Server 2022 x64（Git 2.56.0.windows.4）上验证 Git 功能和 Viva 无头界面；Windows 10/11 的实际桌面交互、输入法及安装分发尚未验收。Linux 和 Windows 的文件监听仍是轮询，见 [0003](decisions/0003-file-watching.md)。
- 界面字体默认为系统字体（SF Pro），没有使用 Inter；设置里可分别选择「界面字体」（全部已安装族）与「diff 字体」（仅等宽族，用于 diff、控制台与哈希），见 [0002](decisions/0002-fonts.md)。
- 冲突支持逐块接受两侧内容；自由文本编辑、二进制或不可靠冲突标记仍需要外部编辑器，见 [0019](decisions/0019-git-issue-gaps.md)。Copy for AI 只写系统剪贴板，不调用模型、不发网络请求；单块 200 行 / 20 KB、全文 200 KB 截断，见 [0032](decisions/0032-ai-conflict-context.md)。
- 撤销只在当前会话、当前仓库生效，保留最近 20 条；首次提交、reword/squash/drop、冲突后 continue、reset --soft/--mixed 的撤销不支持，也没有 redo，见 [0030](decisions/0030-undo.md)。
- 网页链接不做 OAuth/API：未知主机的页面布局按 GitHub 猜测，Bitbucket 不生成比较与创建 PR 页面，未推送的分支由服务端提示不存在，见 [0029](decisions/0029-pr-links.md)。
- LFS 只检测不管理：不安装 git-lfs、不执行 git lfs install/track/pull/push；未暂存的 LFS 指针文件不计入列表，见 [0031](decisions/0031-lfs.md)。
- 已有分支创建工作树见 [0018](decisions/0018-existing-branch-worktree.md)；prunable 工作树只能查看、不能删除。嵌套子模块的移除和 deinit 需要先打开其所属仓库，见 [0019](decisions/0019-git-issue-gaps.md)。
- Project 面板与 Commit 共用左侧停靠区，不能同时显示；默认显示 Commit，按 ⌘1 打开 Project。ignored 文件默认不显示，折叠状态不持久化；Project 中打开的图片仍只显示二进制说明（diff 的图片预览不适用于单版本查看）；文本支持鼠标拖选、Shift+方向键与 ⌘A/⌘C 复制（制表符按显示展开为空格），但不支持编辑与拖选自动滚动。见 [0024](decisions/0024-file-browser.md)。
- 界面语言覆盖主要窗口；少量相对时间提示（如“Moments ago”“Today 14:30”）与诊断 issue 模板仍是英文。
- AI 生成提交说明默认关闭；启用后会把当前改动的分支名、最近五条提交、变更文件与 diff 发到你自己配置的端点，范围与隐私边界见 [privacy.md](privacy.md)。自定义的规则文本同样会随请求发送；规则只影响模型的指令，不会改变发送范围与截断上限。模型返回的文本原样填入提交框，Viff 不解析结构。规则选择：仓库自己的选择优先，没选过的仓库沿用你上一次在任意项目选中的规则，两者都没有时用内置规则；选中内置规则会同时清空「上一次使用」。
- 应用内更新没有系统签名校验（macOS ad-hoc 签名、Windows 无 Authenticode），只做 HTTPS + SHA-256；宿主的系统安全提示仍可能出现。DEB 安装受权限限制只能提示手动安装；随包文档在 Windows/Linux 自替换后保持旧版本，见 [0023](decisions/0023-app-update.md)。旧版本（v0.1.0 及更早）没有更新器，升级到第一个带更新器的版本需要手动下载一次。
- 终端目前只有一个本地会话：不连 SSH、不支持多标签/分屏、搜索、链接、内联图片与鼠标上报；隐藏工具窗时 shell 继续运行，会话不跨重启恢复。切换仓库不会改写已在运行的 shell 的当前目录，新会话才从新仓库根目录开始。Windows 使用 ConPTY（需要 Windows 10 1809+），本机未做 Windows 实机验收。见 [0028](decisions/0028-integrated-terminal.md)。
