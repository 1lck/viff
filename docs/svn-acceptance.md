# Subversion acceptance record / Subversion 验收记录

- Status / 状态：界面手工验收完成；2 项未修（见「仍未修复」），其余通过或已记录原因
- Branch / 分支：`feat/svn-support`（已把 `main` 合并进来）
- Placeholder PR / 占位 PR：<https://github.com/1lck/viff/pull/41>（明确标注请勿合并）
- Related decision / 相关决策：[0025](decisions/0025-svn-support.md)

## What this record is / 本文档的用途

A working record of the Subversion support work: what was checked, what still
has to be checked, what broke and how it was fixed, and which decisions the
agent took on its own while the maintainer was away. It is deliberately a log
rather than a specification: the behaviour itself is specified in
[0025](decisions/0025-svn-support.md) and [features.zh-CN.md](features.zh-CN.md).

本文档记录 Subversion 支持的工作过程：已经检查了什么、还需要检查什么、
发现了哪些问题以及如何修复、以及在维护者不在场时 agent 自行做了哪些决定。
它是过程日志而非规格说明：行为本身由 [0025](decisions/0025-svn-support.md)
与 [features.zh-CN.md](features.zh-CN.md) 规定。

## Environment / 环境

| 项目 Item | 值 Value |
| --- | --- |
| OS | Windows 11 x64 |
| Go | go1.27.3 windows/amd64 |
| Git | 本机 CLI（`git --version`） |
| Subversion | Apache Subversion 1.15.0 (r1937767)，`C:\a-tools\Apache-Subversion-1.15.0-x64\bin\svn.exe`，同时在 `PATH` 上 |
| Test working copy / 演示工作副本 | `C:\Users\momo\Desktop\viff-svn-demo\work`（`file:///C:/Users/momo/Desktop/viff-svn-demo/repo/trunk`，UUID `a969c761-0e34-c745-ba60-1e4630a627da`） |
| UI automation / 界面自动化 | `agent-desktop` 1.2.0（Windows UI Automation，`accessibility: granted`） |
| Build / 构建产物 | `build/viff.exe`（`go build -trimpath`） |
| Config isolation / 配置隔离 | `APPDATA`/`LOCALAPPDATA`/`HOME` 指向 `.viff-test/uiconf`，不读写维护者的真实设置与 SVN 凭据 |

所有 SVN 调用遵循 [0025](decisions/0025-svn-support.md) 的约定：`--non-interactive`、
`file://` 临时仓库、不访问公网、不读写用户真实凭据。演示工作副本是维护者在任务中明确指定的例外。

## Automated checks / 自动化检查

Run from the branch worktree / 在分支工作目录执行：

| 命令 Command | 结果 Result |
| --- | --- |
| `gofmt -l .` | 干净 clean |
| `go vet -tags 'vivahead litheaudit' ./...` | 通过 pass |
| `go build -tags 'vivahead litheaudit' ./...` | 通过 pass |
| `go test -count=1 ./internal/domain/... ./internal/svncli/... ./internal/storage/...` | 通过 pass |
| `go test -count=1 ./internal/app/...` | 仅 `TestScenarioRenameModeSymlinkBinaryDelete` 失败（环境限制） |
| `go test -count=1 -tags 'vivahead litheaudit' ./internal/ui` | 通过 pass（`ok 525.431s`，0 失败）。中途曾让 `Update Project` 对 SVN 跳过对话框，导致 `TestSVNUIUpdateThroughTheWindow` 失败；该改动与分支设计意图冲突，已回退后复跑通过 |
| `go test -count=1 ./internal/gitcli/...` | 仅 `TestTreeAt` 失败（环境限制） |

Two failures are environment limitations, not regressions / 两处失败均为环境限制而非回归：

- `gitcli.TestTreeAt` 与 `app.TestScenarioRenameModeSymlinkBinaryDelete` 都失败在
  `A required privilege is not held by the client`：Windows 创建符号链接需要开发者模式
  或管理员权限，本机未开启。失败点在 `os.Symlink`，与合并改动无关，Git 行为未变。

New tests added by this work / 本次新增的测试：

- `svncli.TestEdgesStageResolvesConflict`：冲突路径上的 `Stage` 必须是 `svn resolve
  --accept working`，否则 `svn add` 报 E155015，界面「标记已解决」永远失败。
- `svncli.TestBranchWrites`：分支创建/重命名/删除是服务端目录操作，trunk 不可删除。

## Manual GUI acceptance / 界面手工验收

Driven through `agent-desktop` against `build/viff.exe` with the demo working
copy above. Status values: `通过` verified in the running app and cross-checked
with the `svn` CLI; `待测试` not yet run; `未复现` the scenario could not be
reproduced in this environment (reason stated).

用 `agent-desktop` 驱动 `build/viff.exe`，对演示工作副本逐项验收。状态含义：
`通过`＝在运行的应用里验证过，并用 `svn` 命令行交叉核对；`待测试`＝尚未执行；
`未复现`＝本机无法构造该场景（写明原因）。

| # | 区域 Area | 检查项 Check | 结果 Result | 状态 Status |
| --- | --- | --- | --- | --- |
| 1 | 打开 Open | 打开 SVN 工作副本，状态栏与标题识别后端 | 仓库打开，顶部显示 `work`/`trunk`，历史 4 条提交可读 | 通过 |
| 2 | 状态 Changes | 修改、新增、缺失、未版本化各自落位 | `notes.txt` 修改、`staged.txt` 新增、`README.md` 删除（修复 D2 后可见）、`loose.txt` 未版本化 | 通过 |
| 3 | 状态 Changes | 冲突项独立分组，`.mine`/`.r5`/`.r6` 被过滤 | `Merge Conflicts` 分组只有 `notes.txt`，未跟踪列表无辅助文件 | 通过 |
| 4 | 状态 Changes | 属性变更（`props=modified`）被列出 | 修复 D9 前「只有属性变更」的文件完全不在列表里；修复后 `README.md`（`svn propset svn:eol-style`）出现在 Changes | 通过 |
| 5 | 差异 Diff | 工作副本 diff 与 `svn diff` 一致 | `beta` → `beta CHANGED` 单块差异，三栏视图正常 | 通过 |
| 6 | 历史 Log | 打开历史、最新提交、线性顺序 | r6 在顶、r1 在底，6 条提交按时间倒序 | 通过 |
| 7 | 历史 Log | 离线/不可达时显示缓存并标注陈旧 | 把仓库目录改名后刷新：日志仍显示全部提交，**没有任何「缓存/陈旧」标注**，等于冒充最新（D11，未修） | 失败 |
| 8 | Blame | 每行带修订与作者，未提交行无归属 | 与 `svn blame` 一致：r4/r5/未提交/r6 | 通过 |
| 9 | 更新 Update | 执行 update 拉入远程提交 | 拉入 r6 并产生冲突（双方都改了同一行），符合 svn 语义 | 通过 |
| 10 | 提交 Commit | 部分提交（只提交勾选的文件） | 只提交 `notes.txt` → r5，`loose.txt` 保持未版本化 | 通过 |
| 11 | 提交 Commit | 无索引时提交按钮可用 | 只填消息即可提交（不需要 stage） | 通过 |
| 12 | 提交 Commit | 目录过期时提交被拒绝并给出可读错误 | 第二个工作副本提交 r8 后，从过期工作副本提交：仓库没有新修订，弹出 `Commit failed / svn: E155011 … is out of date / E160028` | 通过 |
| 13 | 还原 Revert | 还原单个文件 | `Rollback…` 恢复被删除的 `README.md`，其他改动保留 | 通过 |
| 14 | 添加/删除 Add/Delete | 添加未版本化文件、删除已跟踪文件 | `staged.txt` 经 `svn add` 进入提交；删除走 `!` → 提交即删除 | 通过 |
| 15 | 分支 Branches | 列出、切换、创建、删除 | 列表显示 `trunk` + `acceptance-branch`；创建落盘为 `branches/acceptance-branch`（修复 D5），切换后工作副本 URL 变更；删除由 `svncli.TestBranchWrites` 覆盖 | 通过 |
| 16 | 冲突 Conflicts | 只读展示 + 标记已解决 | 三栏展示 + 标记已解决把 `C` 变 `M`（修复 D4）；接受某一侧在 SVN 下已隐藏（修复 D6） | 通过 |
| 17 | 能力位 Caps | rebase/stash/worktree/reflog/submodule 入口隐藏 | 界面无这些入口；Push/Fetch/Commit and Push 也已隐藏（修复 D3） | 通过 |
| 18 | 设置 Settings | Subversion 可执行文件路径可配置 | 设置 → General 有 `Path to svn:` 输入并写入 `settings.json` 的 `svnPath` | 通过 |
| 19 | 错误提示 Errors | svn 缺失/过期/冲突类错误有可读提示 | 过期提交弹出可读错误（见 #12）；但把 `svnPath` 指到不存在的程序后，启动打开仓库失败，应用静默退回欢迎页，没有任何提示（D8，未修） | 部分失败（D8） |
| 20 | 主题 Theme | 明暗主题下界面可用 | 浅色（`Islands Light`）下提交面板、变更列表、历史、空状态提示都可读，按钮门控一致；备注：界面上切换主题后没有立即生效，需重启才应用（非 SVN 问题，未修） | 通过 |

## Defects found and fixed / 发现并修复的问题

A defect is only listed here once a fix is in the branch (or explicitly marked
still open). 只在修复已进分支（或明确标注仍未解决）时列出。

| # | 现象 Symptom | 原因 Cause | 修复 Fix | 状态 Status |
| --- | --- | --- | --- | --- |
| D1 | 合并 `main` 后 `internal/app` 无法编译：`w.Git.Head`、`w.Git.LFS`、`w.Git.RemoteHeads`、`w.Git.Reset`、`w.Git.CreateBranch` 等未定义 | `main` 新增的撤销与 LFS 代码直接调用具体 Git 方法，而本分支已把 `app.Git` 收窄成 `vcs.Backend` | 新增 `internal/app/gitonly.go` 的窄接口（`headReader`/`remoteHeadReader`/`lfsReporter`）与 `HEAD`/`hasUndo`/`remoteHeads`/`lfsStatus` 访问器；撤销记录改走 `w.history()`/`w.branches()` 能力位；Subversion 没有本地 HEAD 时不记录撤销 | 已修复 |
| D2 | `svn status` 的 `!`（磁盘上已删除）在界面上完全消失 | `statusLetter` 把 `missing` 映射成 `!`，而 `domain.StatusEntry.Ignored()` 把 `!` 当作忽略项过滤；Git 从不产出 `!`（status 不请求 ignored） | `missing` 改为映射 `D`：两者都表示「文件不在了，提交会删除它」；更新 `TestEdgesDeletedAndMissing` 的断言并写明理由 | 已修复 |
| D3 | SVN 下仍显示 `Push…`、`Fetch`（三处）、`Commit and Push…`、分支菜单的 `Push` | 这些入口是 Git 的「本地提交 + 独立发布」模型，界面没有按能力位判断 | `vcs.Capabilities` 新增 `SeparatePush`（Git 默认 true，Subversion false），工具栏、日志工具栏、分支面板、提交面板与分支菜单统一门控 | 已修复 |
| D4 | 「标记已解决」在 SVN 下无效：`svn status` 仍是 `C` | 该按钮走 `StagePaths` → `svn add`，对冲突路径报 `E155015 is an existing item in conflict` | `svncli.Stage` 先读 `svn status --xml`，把冲突路径交给 `svn resolve --accept working`；新增 `TestEdgesStageResolvesConflict` | 已修复 |
| D5 | 「新建分支」对话框弹出后什么也没发生 | `svncli` 没有实现分支写能力（`Caps.References == nil`），界面却照样显示入口 | 新增窄接口 `vcs.BranchWriter`（创建/重命名/删除），`svncli` 用 `svn copy`/`svn mv`/`svn delete` 实现（`internal/svncli/refs.go`），界面按 `Caps.Branches` 门控；新增 `TestBranchWrites` | 已修复 |
| D6 | 冲突视图在 SVN 下画出「接受某一侧」按钮，点击无效果 | 按钮只按 diff 内容判断，没有看后端是否实现 `ConflictResolver` | 冲突视图的接受行改为 `w.ws.Caps.Conflicts != nil` 才绘制；`Copy for AI` 仍可用（只写剪贴板，不需要解析后端能力） | 已修复 |
| D7 | 打开仓库对话框标题硬编码 `Open Git Repository`，且文案只提 Git | `internal/ui/window.go` 直接写字面量，没走 i18n；文案本身也过时 | 改走 `i18n.ChromeDialogOpenRepository`，中英文案改为「Open Repository」/「打开仓库」 | 已修复 |
| D8 | `svnPath` 指向不存在的程序时，启动打开仓库静默失败、退回欢迎页 | `svncli` 已把二进制缺失分类成 `ErrNoGit`（`runner.go`，有测试覆盖），`AppErrorToolMissingSVN` 文案也在；失败发生在 `Window.Start` → `openRepo` 阶段，此时欢迎页没有通知面板入口，提示等于没显示 | 未修复。修法应是在欢迎页给失败一个可见位置（或阻止启动时静默丢弃），属跨界面改动 | 未修复 |

Also observed but not fixed / 一并观察到但未修复（属文案而非功能）：

- D9 已修：`svn propset` 造成的**纯属性变更**原本完全不进任何分组。`StatusEntry.Unstaged()`
  只认 `Worktree` 列，而属性变更的文本列是空格（`svn status` 显示 ` M`），条目因此落在
  所有分组之外——既看不见也进不了提交。现在 `Props` 非空即视为工作副本改动。
  新增断言在 `domain.TestStatusEntryAreas`。
- D10 已修：Console 把所有命令都记成 `git …`，SVN 命令也一样；标题写「Git commands run
  by Viff」。现在命令前缀取自后端（新增 `CLIName()`，`git`/`svn`），标题改为
  「Commands run by Viff」/「Viff 运行的命令」。
- D11 未修：仓库不可达时，历史区仍然显示完整缓存且**没有任何陈旧标注**。这与
  [0025](decisions/0025-svn-support.md) §6 的「离线时显示缓存必须显式标注，不能冒充最新」
  直接冲突，属数据可信度问题，优先于本表其余文案项。
- 界面切换主题（`General → Theme`）当次不生效、需要重启；与 SVN 无关，未修。
- 工具栏的 `Git ⌘9` 工具窗与 `Git Actions` 面板标题对 SVN 仍是 Git 品牌文案；两者承载的是
  共用的分支/历史面板，改名会牵动 i18n 与截图，本次未动。
- 文件视图的 `Annotate with Git Blame` 按钮在 SVN 下功能正常（走 `svn blame`），文案仍写 Git。
- `Update Project` 对话框对 SVN 也列出 merge/rebase/fast-forward 三种 Git 集成方式，后端会忽略
  该选择。这**不改**：分支自带的 `TestSVNUIUpdateThroughTheWindow` 明确要求这条用户路径，
  且 [0025](decisions/0025-svn-support.md) 已写明「mode 是无 rebase 后端忽略的 Git 集成选择」。
  曾尝试让它对 SVN 直接更新，与该测试意图冲突，已回退。

## Decisions taken without the maintainer / 维护者不在场时的决策

The maintainer delegated these while away; each one states the alternative that
was rejected and how to revisit it.

维护者明确授权自行决策。每条记录被放弃的替代项与复审方式。

1. **合并 `main` 前先把本地 `main` 快进到 `upstream/main`。**
   本地 `main` 落后 8 个提交，直接合并会得到一份基于旧主线的 PR。
   替代项是只合并本地旧 `main`，被否决：PR 仍需要二次合并。
   复审：若 `upstream/main` 在验收期间继续前移，需要再合并一次。

2. **在既有 worktree `.viff-test/worktrees/svn-p0` 上操作，而不是切换主工作目录。**
   `feat/svn-support` 已被该 worktree 检出，Git 不允许同一分支被两处检出；
   主工作目录又在 `feat/ai-prompt-rules` 上有未提交改动。
   替代项是丢掉主目录的改动去切换，被否决（属破坏性操作）。

3. **撤销（Undo）在 Subversion 下整体关闭，而不是做「半套」撤销。**
   SVN 没有本地 HEAD，提交本身就是远端原子操作，本地没有可回退的状态。
   替代项是让撤销记录在 SVN 下静默失败，被否决（隐藏失败比不提供更糟）。
   复审：若 SVN 需要可撤销，应实现 `svn merge -c -REV` 这类显式新功能。

4. **新增两个能力位而不是继续加 `ErrNotSupported` 空实现。**
   `SeparatePush` 与 `BranchWriter` 各解决一个真实差异：前者是「提交是否即发布」，
   后者是「能否写本地分支」。替代项是让 `svncli` 实现整个 `ReferenceWriter`
   （tag/remote/push 全为空实现），与 [0025](decisions/0025-svn-support.md) 拒绝
   「40 个无意义方法」的决定冲突，被否决。
   复审：出现第三个后端时，按 [0025](decisions/0025-svn-support.md) 的复审条件
   重新划分接口边界，而不是继续加字段。

5. **`missing` 映射为 `D` 而不是保留 `!` 并改 `domain` 语义。**
   改 `Ignored()` 会让 Git 侧一个从不触发的路径变成「没有忽略语义」，
   而 `D` 在 SVN 里就是提交会删除该路径，两种状态对用户是同一件事。
   复审：若将来需要区分「计划删除」与「磁盘丢失」，应在状态模型里增加独立字段。

6. **界面手工验收用隔离配置文件运行，不碰维护者的真实设置。**
   `APPDATA`/`LOCALAPPDATA`/`HOME` 指向 `.viff-test/uiconf`。
   替代项是直接跑真实配置，被否决（会在用户设置里写下演示仓库与临时 svn 路径）。

7. **GitHub 推送被网络阻断时，先完成本地验收而不是反复重试。**
   `github.com` 的 TLS 握手连续失败（`api.github.com`/`codeload` 可用），
   属本机网络/代理状态而非仓库问题。网络恢复后已推送分支并创建占位 PR
   <https://github.com/1lck/viff/pull/41>，PR 正文明确写了「请勿合并」。
   复审：维护者验收后再决定是否转为可评审。

8. **`Update Project` 对话框对 SVN 保留（改动已回退）。**
   看起来该对 SVN 隐藏三种 Git 集成方式，但分支自带的
   `TestSVNUIUpdateThroughTheWindow` 明确把「打开对话框 → 确定」当作既有用户路径，
   [0025](decisions/0025-svn-support.md) 也写明 mode 会被无 rebase 的后端忽略。
   替代项（SVN 直接更新）会让该测试失败并改变已设计行为，被否决。

## Still to verify / 未完成项

- D8：`svnPath` 指错时启动失败没有可见提示（欢迎页没有通知入口）。
- D11：历史缓存在仓库不可达时没有任何「缓存/陈旧」标注，与
  [0025](decisions/0025-svn-support.md) §6 冲突，是本项目前最该修的一项。
- 属文案的 Git 品牌残留（`Git` 工具窗、`Git Actions`、`Annotate with Git Blame`）。
- 界面切换主题需重启才生效（非 SVN 问题）。
- 另有两处 Windows 环境限制导致的测试失败（创建符号链接权限不足），与改动无关：
  `gitcli.TestTreeAt`、`app.TestScenarioRenameModeSymlinkBinaryDelete`。
