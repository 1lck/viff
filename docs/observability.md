# Viff 日志、崩溃与诊断导出设计

- 状态：提案（2026-11），待评审；决策记录见 [0021](decisions/0021-observability.md)。
- 目标读者：Viff 维护者。实现前先评审本设计，再按第 12 节分阶段落地。

## 0. 要解决的问题与取舍

用户现在遇到问题时，唯一能提供的是截图和口述；Viff 没有落盘日志、没有版本号、没有崩溃记录、Console 退出即丢，Issue 模板却要求"版本 + Console 输出"。结果是：**作者无法复现，用户无法自证，来回沟通消耗掉开源维护最贵的资源。**

设计目标：

1. **能定位**：拿到一份诊断包，5 分钟内判断"哪个 Git 命令、失败在哪一步、当时环境如何"。
2. **能精准**：导出的是"出事那段时间的事件 + 因果链"，不是几十 MB 的原始日志。
3. **能安全地交给作者**：默认脱敏，用户导出前可预览，不把 token、家目录、私有仓库地址带出去。
4. **零服务端**：不新增常驻服务、不默认联网、不自动上传。

非目标：不做产品分析/遥测、不做远程实时日志、不记录仓库内容（diff、提交说明、文件内容）。

核心决策：**采集端做厚（结构化事件 + 内存环 + 崩溃文件），传输端做薄（用户主动导出 zip），后端最后再说。**

## 1. 从排查场景倒推

先明确"作者到底要看到什么"，再决定记录什么。否则只会记一堆用不上的日志。

| 用户症状 | 作者需要的关键证据 | 本设计提供 |
|---|---|---|
| 双击没反应 / 窗口闪一下消失 / 启动崩溃 | 启动到哪一步、失败原因、栈、版本 | `app.start`/`app.fatal` 事件 + crash 文件 + 系统消息框里给日志路径 |
| "push/commit 失败了" | 哪条命令、退出码、Git 原始错误、错误分类、耗时 | `git.fail` 事件（op/exit/kind/dur/stderr 摘要）+ 稳定错误码 |
| 说不清在哪个操作出问题 | 时间线、操作链、最近错误列表 | `summary.md` 自动摘要 + `events.jsonl` 按时间排序 |
| 卡顿 / 内存高 / 大仓库慢 | 任务耗时、刷新频率、内存采样、慢操作 | `task.*`、`repo.refresh`、`perf.slow`、`perf.mem`（debug） |
| 界面状态不对 / 该刷新没刷新 | 刷新代次、监听器状态、外部改动 | `repo.refresh`（gen/dur/统计）、`repo.watch`（start/overflow/error） |
| 认证失败 / 凭据问题 | 用的哪个 remote 类型、Git 的原始报错 | `git.fail` + auth 分类 + remote 只保留 host（默认） |
| 配置坏了 / 升级后设置异常 | schema 版本、迁移结果、损坏告警 | `settings.load`（ok/warn/corrupt/migrated） |
| 特定平台问题（字体/缩放/输入法） | OS、架构、缩放、主题、字体、locale、渲染后端 | `env.txt`（启动时一次性收集） |
| 崩溃 / 闪退 | panic 栈、崩溃前最后几条事件 | crash 文件（栈 + 内存环最近 1000 事件） |

## 2. 体系结构

```mermaid
flowchart LR
    A[应用代码<br/>app / gitcli / platform / ui] -->|obs.Event| B[Observability]
    B --> C[异步 writer<br/>有界队列]
    C --> D[滚动日志<br/>logs/viff-YYYYMMDD.jsonl<br/>2 MiB × 5，14 天]
    B --> E[内存事件环<br/>1000 条 / 256 KiB]
    B --> F[Console 镜像<br/>最近 400 条命令]
    B --> G[诊断会话<br/>incident=... 时开 debug]
    G --> H[导出器]
    D --> H
    E --> H
    F --> H
    I[crash 文件<br/>panic / fault] --> H
    H --> J[诊断包 zip<br/>summary.md + events.jsonl + ...]
    J --> K[用户预览 ✓] --> L[GitHub Issue 附件 / 私密安全报告]
```

四个部分：

1. **常驻滚动日志**：info 级别、默认脱敏、有界（≤10 MiB/≤14 天）。解决"exit 之前什么都没留下"。
2. **内存事件环**：最近 1000 条事件常驻内存（约 256 KiB），崩溃或导出时使用。解决"崩溃前最后发生了什么"。
3. **诊断会话**：用户点 "Report a Problem…" 后临时开 debug、给事件打 `incident` 标记；导出时按时间窗和关联 ID 过滤。解决"精准"——不导出无关的一天。
4. **导出器**：生成带自动摘要的 zip，用户预览后自己上传。解决"用户不知道怎么给"。

**关键认知：导出不是拷贝日志文件，而是按事件模型重新组织一份"可读的诊断报告 + 结构化明细"。**

## 3. 事件模型

### 3.1 格式

`events.jsonl`，一行一个 JSON 对象。文件名 `.jsonl`，日志文件与导出包同构，方便工具复用。

```json
{"ts":"2026-11-04T13:05:51.231Z","seq":412,"session":"7f3c1a2b","level":"error","event":"git.fail","incident":"inc-8f21","repo":"9f3a1c2b","task":12,"op":"push","write":true,"dur_ms":3001,"exit":128,"kind":"rejected","code":"push.rejected","msg":"failed to push some refs","data":{"remote_kind":"https","stderr_bytes":412}}
```

公共字段（每条事件都有）：

| 字段 | 说明 |
|---|---|
| `ts` | RFC 3339，毫秒精度，UTC |
| `seq` | 进程内单调递增，用于排序与发现丢失 |
| `session` | 每次启动随机 8 位十六进制 |
| `level` | `error` / `warn` / `info` / `debug` |
| `event` | 稳定的事件名（见 3.2），不是自由文本 |
| `incident` | 诊断会话 ID，只在录制期间出现 |
| `repo` | 仓库绝对路径的 `sha256[:8]`，**绝不写真实路径** |
| `task` | 关联的任务 ID（与状态栏任务对应） |
| `op` | Git 子命令；非 Git 事件为操作名（open/refresh/export…） |
| `code` | 稳定错误码（见第 7 节），错误类事件必填 |
| `dur_ms` | 结束类事件的耗时 |
| `msg` | 已脱敏的一句话，长度上限 512 字节 |
| `data` | 事件专属小对象，上限 4 KiB，禁止大字段 |

### 3.2 事件目录（固定小集合，便于 grep 与聚合）

| event | 级别 | 触发点 | 主要字段 |
|---|---|---|---|
| `app.start` | info | 进程启动 | 版本/commit/构建时间、goos/goarch、Go/Viva 版本、Git 版本与来源、配置目录（`<cfg>`）、设置 schema、locale、缩放、主题、字体 |
| `app.ready` | info | 首帧绘制完成 | `startup_ms` |
| `app.stop` | info | 正常退出 | `uptime_ms`、`tasks_running`、`reason` |
| `app.fatal` | error | 启动失败/无法继续 | `phase`、`msg`、`stack`（写 crash 文件） |
| `app.recover` | warn | 检测到上次崩溃文件 | `age_h`、`crash_path` |
| `settings.load` | info/warn | 读取设置 | `schema`、`status`（ok/warn/corrupt/migrated）、`readonly`、`from`/`to` |
| `settings.save` | warn | 保存失败 | `err`（成功不记，避免噪声） |
| `repo.open` | info | 打开/切换仓库 | `dur_ms`、`status`、`worktree`、`bare`、`op_state`、`error_code` |
| `repo.close` | info | 关闭仓库 | 原因 |
| `repo.refresh` | debug/info | 刷新完成 | `gen`、`dur_ms`、`trigger`（watch/manual/auto）、`staged`/`unstaged`/`untracked`/`conflicted` 计数、`op_state` |
| `repo.watch` | info/warn | 监听器启停/异常 | `backend`（fsevents/poll）、`status`、`overflow`、`error` |
| `task.start` | info | 开始写操作或长任务 | `title_key`、`write` |
| `task.cancel` | info | 用户取消 | `task`、`elapsed_ms` |
| `task.end` | info | 任务结束 | `task`、`dur_ms`、`status` |
| `git.start` | debug | 命令开始 | `op`、`write`、脱敏后的参数摘要 |
| `git.end` | info | 写操作成功 / 慢读（>500ms） | `op`、`write`、`dur_ms`、`exit` |
| `git.fail` | error | 命令失败 | `op`、`write`、`dur_ms`、`exit`、`kind`、`code`、`stderr`（脱敏摘要，≤2 KiB）、`args`（debug 时脱敏后） |
| `ui.notice` | warn/info | 通知中心新增错误/警告 | `notice_level`、`code`、`title` |
| `conflict.detected` | info | 检测到冲突 | `files` 计数、`op_state` |
| `perf.slow` | warn | 任务/刷新超过阈值 | `op`、`dur_ms`、`threshold_ms` |
| `perf.mem` | debug | 每 60s 采样 | `alloc_mb`、`heap_mb`、`goroutines` |
| `diagnostics.export` | info | 生成诊断包 | `incident`、`files`、`bytes`、`profile`、`truncated` |
| `update.check` | info/warn | 启动时自动检查或手动检查更新 | `from`/`to`、`manual`、`result`（up-to-date/available/error）、`dur_ms` |
| `update.download` | info/warn | 下载更新安装包 | `to`、`bytes`、`result`（downloaded/cancelled/error）、`dur_ms` |
| `update.install` | info/warn | 应用更新（写 helper 并退出替换） | `to`、`result`（scheduled/manual/error）、`dur_ms` |
| `panic` / `fault` | error | Go panic / Viva 回调边界 panic | `where`、`msg`、crash 文件路径 |

### 3.3 噪声控制

- **读命令不记 `git.start`**（debug 会话除外）；成功读命令也不写 Console（沿用现有 `workspace.go:184-213` 的静默名单，但补全 `rev-list`/`ls-tree`/`blame`，避免刷屏挤掉失败上下文）。
- 重复事件 1 秒内合并：`{"count":N,"first_seq":…}`，防止错误风暴写满磁盘。
- `perf.mem` 只在 debug 下采样；`perf.slow` 阈值初始：任务 >5s、刷新 >2s、单条 Git 命令 >10s。
- 单条事件序列化后 >4 KiB 直接截断，记 `truncated:true` 与原始字节数。
- 典型会话预算：info 级别 <200 KB；debug 会话 10 分钟 <2 MiB。

## 4. 脱敏（必须先于日志落盘实现）

单一入口 `observability.Redact(...)`，日志、Console、导出包全部经过它，禁止旁路。三档配置：

| 数据 | standard（默认） | strict | 规则 |
|---|---|---|---|
| 仓库绝对路径 | `sha256[:8]` + 可选 basename | 仅 `sha256[:8]` | 永不写绝对路径 |
| 家目录路径 | `~` | `~` | `$HOME` 前缀替换 |
| remote URL | 保留 scheme+host | 保留 scheme+host | 去掉 path 与 `user:token@` |
| 分支/tag 名 | 保留 | 保留 | 排查必需；用户可在 strict 关闭 |
| 提交哈希 | 保留短哈希 | 保留短哈希 | 非敏感 |
| 仓库相对文件路径 | 保留 | 仅扩展名/计数 | standard 下不含家目录，风险可控 |
| Git stderr/stdout | 脱敏后前 2 KiB | 脱敏后前 512 B | 标记 `truncated` |
| 完整 argv | 默认不记 | 不记 | 只记子命令与白名单参数 |
| 环境变量 | 不记 | 不记 | 需要时只记白名单键 |
| 凭据/私钥 | 永不 | 永不 | — |
| commit message / diff / 文件内容 | 永不 | 永不 | — |
| 设置密钥字段 | 只记字段名 | 只记字段名 | `sshCommand`/`signingKey`/`credentialHelper` 不导出值 |

正则黑名单（进入 `Redact` 的测试语料）：`ghp_/gho_/ghu_/ghs_/ghr_`、`github_pat_`、`glpat-`、`AKIA[0-9A-Z]{16}`、`xox[baprs]-`、`-----BEGIN … PRIVATE KEY-----`、`(?i)(token|password|secret|apikey|authorization)\s*[=:]\S+`、URL userinfo。

验收要求：用带 `https://user:ghp_xxx@host/org/repo.git` 的临时仓库故意失败一次，日志与导出包中 `grep` 不到用户名、`ghp_`、家目录绝对路径。

## 5. 存储与运行

| 项 | 决定 |
|---|---|
| 路径 | macOS `~/Library/Application Support/Viff/logs/`；Windows `%AppData%\Viff\logs\`；Linux `~/.config/Viff/logs/`（复用 `storage` 的 `UserConfigDir` 策略，失败回退临时目录并记 WARN） |
| 权限 | 目录 `0700`，文件 `0600` |
| 文件 | `viff-YYYYMMDD.jsonl`，按天 + 单文件 2 MiB 轮转 |
| 保留 | 最多 5 个文件、总量 ≤10 MiB、超过 14 天清理；启动时清理 |
| 写入 | 单 writer goroutine + 有界队列（4096 条）；队列满丢新事件并累加 `dropped`，退出/panic 前 Flush |
| 级别 | 默认 `info`；`VIFF_LOG=debug` 或设置项 `logLevel` 开启 debug；诊断会话期间强制 debug |
| 开销 | 关闭 debug 时每事件约数十微秒，写日志不占用 UI 线程；实测目标：开启日志后帧时间无可测变化 |
| 崩溃安全 | panic/fault 路径同步写 `crash/last-crash.txt`（O_SYNC），内容含栈与内存环快照 |

设置新增字段（schema 保持 1，`omitempty`，旧文件行为不变）：

```go
LogLevel string `json:"logLevel,omitempty"` // "", "info", "debug"
```

## 6. 崩溃处理

1. `main()` 拆为 `run() (exitCode int)`：`defer` 中 `recover()` → 写 crash 文件 → `app.ShowFatalError` 显示系统消息框（含日志路径与"Report a Problem"指引）；`os.Exit(code)`。
2. 启动最早阶段调用 `app.SetFaultHandler`，把 Viva 在 AppKit/Win32/COM 回调边界捕获的 panic 写同一份 crash 文件并记 `fault` 事件（当前 Viva 默认 handler 为 nil，这类 panic **会被静默吞掉**）。
3. 下次启动检测 7 天内的 crash 文件 → 非阻塞通知"Viff 上次异常退出"，动作：`[Report a Problem…] [Open Log Folder]`，然后归档到 `crash/history/`。
4. Windows 可选 `app.EnableCrashDumps`：**必须先弹窗告知"dump 是内存副本，可能包含敏感内容"** 再启用（Viva 文档要求），路径记入 `env.txt`。
5. 启动失败不再只写 stderr（`-H=windowsgui` 下不可见），改走 `ShowFatalError`。

## 7. 错误码（让用户能"说清"错误）

现有 `gitcli.ErrorKind` 14 类（`internal/gitcli/runner.go:23-40`）是良好基础，但没有稳定字符串进 UI 与日志。增加映射：

| Kind | code |
|---|---|
| ErrAuth | `auth.failed` |
| ErrLocked | `repo.locked` |
| ErrNotRepository | `repo.not_a_repository` |
| ErrConflict | `git.conflict` |
| ErrNotFound | `git.not_found` |
| ErrRejected | `push.rejected` |
| ErrUntrackedOverwrite | `worktree.untracked_overwrite` |
| ErrLocalChangesOverwrite | `worktree.local_changes` |
| ErrNotMerged | `branch.not_merged` |
| ErrNothingToCommit | `commit.empty` |
| ErrHookFailed | `hook.failed` |
| ErrNoGit | `git.missing` |
| ErrNoWSL | `wsl.unavailable` |
| ErrNoDistro | `wsl.distro_missing` |
| ErrNoWSLGit | `wsl.git_missing` |
| ErrCancelled | `git.cancelled` |
| ErrOther | `git.failed` |

错误通知正文末尾追加 `Code: push.rejected · Help`（Help 打开 `docs/troubleshooting.md` 对应锚点）。日志/导出里 `code` 与 `kind` 同时出现，便于按 code 聚合统计。Git 原始输出仍然完整保留在通知详情里，不遮挡原文。

## 8. 诊断导出（"精准"的核心）

### 8.1 入口

- **Help → Report a Problem…**：录制复现过程（主入口）。
- **错误通知上的 `Report Problem…` 动作**：带着错误码和上下文直接进入录制/导出。
- **Help → Save Diagnostics…**：不录制，直接导出当前会话（最近 30 分钟）。
- **崩溃后的下次启动通知**：自动带上 crash 文件。
- About / 设置里的 **Open Log Folder**。

### 8.2 两阶段：录制 → 导出

**为什么录制**：平时日志是 info 级、按噪声控制裁剪；只有知道"要复现什么问题"时，才能临时打开 debug、记录 UI 动作面包屑（可选），并给事件打 `incident` 标记。这样导出包只包含相关问题，不包含用户几小时无关操作。

流程：

1. 用户点 Report a Problem… → 对话框：问题描述（可选）、复现步骤（可选）、脱敏档位（standard/strict/custom）、"开始记录"。
2. 开始后：`incident` ID 生成，级别临时提到 debug（10 分钟或 10000 条事件后自动停止），状态栏显示安静提示"正在记录诊断信息…"，不弹窗、不打断操作。
3. 用户复现问题 → 回到对话框点"停止并保存"。
4. 导出器冻结 `[incident_start - 60s, incident_stop + 5s]` 时间窗，生成诊断包。
5. 预览对话框：文件清单/大小/脱敏摘要/包含的敏感项开关；用户可取消勾选。
6. 保存 zip → 显示路径 + "复制问题模板"（版本、环境摘要、错误码列表、复现步骤模板）→ 用户粘贴到 GitHub Issue 并附上 zip。

崩溃场景不需要录制：crash 文件 + 内存环快照已保存，下次启动走"导出上次崩溃"。

### 8.3 诊断包结构

```
viff-diagnostics-20261104-132501.zip     # ≤5 MiB
├── summary.md               # 自动生成的人读摘要（作者先看这个）
├── manifest.json            # 机器可读：版本、时间范围、文件清单、sha256、脱敏档位、截断标记
├── env.txt                  # 版本/commit/构建时间、OS/arch、locale、缩放、主题、字体、Git 版本与来源、Viva 版本、配置路径（脱敏）
├── settings-summary.json    # 设置摘要（不含密钥值；只含非敏感项与字段名）
├── events.jsonl             # 时间窗内的结构化事件（主数据，≤5000 条或 2 MiB）
├── console.txt              # 最近 Git 命令与输出（脱敏、截断，≤1 MiB）
├── repo.txt                 # 最近仓库摘要：hash、可选名字、当前分支类型、操作状态、最近刷新
└── crash/                   # 如有：last-crash.txt、history/ 最近 3 份
```

### 8.4 `summary.md` 模板（作者 5 分钟定位的关键）

```markdown
# Viff 诊断摘要
- 版本：0.1.0-alpha.3 (abc1234，2026-11-03 构建)
- 平台：macOS 15.1 arm64 · 缩放 1.00 · 主题 dark · locale zh-CN
- Git：2.53.0（来自 PATH）· Viva 0.2.3 · 设置 schema 1
- 导出时间：2026-11-04T13:25:01Z · 窗口：13:20:01–13:24:59（诊断会话 inc-8f21）
- 用户描述：推送时一直失败

## 最近错误
1. 13:24:41 push 失败 exit=128 kind=rejected code=push.rejected（3.0s）
   stderr: non-fast-forward ...
2. 13:22:10 fetch 失败 exit=128 kind=auth code=auth.failed（1.2s）

## 时间线（错误与写操作）
13:20:03 app.start
13:21:55 git push → fail
13:24:41 git push → fail
...

## 环境与设置摘要
（表格，不含敏感值）

## 脱敏说明
本包按 standard 档脱敏：仓库路径已哈希、remote 仅保留 host、Git 输出截断且已脱敏。
```

### 8.5 选择算法（精准而不是全量）

```
events = 诊断会话窗内事件
       + 窗内前后各 20 条 error/warn
       + 本次会话的 app.start / settings.load / repo.open
       + 导出时刻的 env 快照
排序：seq；去重；超过 5000 条或 2 MiB 时保留最新并记 truncated/lost_counts
console.txt = 窗内失败的 Git 命令（全量输出，脱敏后 ≤2 KiB/条）+ 最近 50 条成功写操作
```

### 8.6 用户可选项（默认最安全）

| 选项 | 默认 | 说明 |
|---|---|---|
| 包含仓库名（basename） | 关 | 打开后日志里出现项目名 |
| 包含完整 remote URL | 关 | 打开后 path 也保留 |
| 包含完整 argv | 关 | 仍会脱敏凭据，但会暴露参数 |
| 包含仓库相对文件路径 | 开 | 排查冲突/路径问题需要 |
| 包含 Git 输出原文 | 开（已脱敏、截断） | 排查 Git 故障最重要的证据 |

### 8.7 导出后的交付

- 保存 zip 后显示：路径、大小、`复制问题模板` 按钮（生成可直接粘贴到 Issue 的标题/正文：版本、平台、错误码、时间线摘要、复现步骤）。
- `Help → Report a Problem…` 同时提供 `Open GitHub Issue`（打开预填正文的新 Issue 页面），用户自己附加 zip。
- **不做自动上传**。未来若做，另立 ADR，且必须是"每份诊断包显式同意"而不是全局开关。

## 9. 用户界面文案（当前 UI 为英文，先保持一致）

- `Help` 菜单：`Report a Problem…`、`Save Diagnostics…`、`Open Log Folder`、`About Viff`。
- 录制中状态栏：`Recording diagnostics… (stop)`
- 导出预览：`Diagnostics bundle (4.2 MiB)`、文件清单、`Include…` 勾选项、`Save…`、`Copy issue template`。
- 隐私说明弹层：`Diagnostics stay on this computer unless you save and share them. No repository contents are included.`

## 10. 接入点（现有代码）

| 位置 | 改动 |
|---|---|
| `cmd/viff/main.go:20-56` | 初始化 observability、装 fault handler、拆 `run()`、`--version`、启动失败走 `ShowFatalError`、退出 flush |
| `internal/gitcli/runner.go:167-174` | `Runner.Log` 回调已有 `dir/args/took/err`，直接转发到 observability（补 `Error.ExitCode`/`Kind`） |
| `internal/app/workspace.go:184-213` | `logCommand` 同时写 Console 与事件；补全读命令静默名单；`Notify/NotifyError`、`startTask/CancelTask`、`Refresh`、`OpenRepo/Close` 打点 |
| `internal/app/repo.go:167-178` | watcher 启停/溢出事件；Close 时记录进行中任务数 |
| `internal/app/workspace.go:272-291` | `ErrorText` 产出错误码；新增 `docs/troubleshooting.md` 锚点 |
| `internal/storage/settings.go` | `LogLevel` 字段；`settings.load` 打点（损坏/只读/迁移） |
| `internal/ui/dialogs.go:562-581` | About 显示版本/commit/构建时间/Go/Viva/Git；增加 Report/Save Diagnostics/Open Log Folder |
| `internal/ui/console.go:120-188` | Copy All / Save…、显示耗时、单条输出 8 KiB 封顶、Clear All 确认 |
| `internal/ui/window.go:207-217` | 退出前 flush、记录未完成任务；崩溃文件检测通知 |
| 新包 `internal/observability/` | `event.go`（模型与级别）、`logger.go`（异步 writer/轮转/级别）、`ring.go`、`redact.go`、`crash.go`、`export.go`、`summary.go`、`env.go` |

接口草案（不引入第三方依赖，标准库足够）：

```go
package observability

type Level int // Error, Warn, Info, Debug

type Event struct {
    Event  string         // "git.fail"
    Level  Level
    Repo   string         // 仓库根，内部转 hash
    Op     string
    Code   string
    Dur    time.Duration
    Fields map[string]any // 序列化前统一脱敏与长度检查
}

type Logger interface {
    Event(e Event)
    StartIncident(kind string) (IncidentID, error)
    StopIncident(IncidentID)
    Flush()
    Close()
}
```

是否用 `log/slog`：不采用。需要的轮转、内存环、脱敏、incident 过滤都不是 slog Handler 目标，自建小包更简单、可控且零依赖。

## 11. 测试与验收

**单元测试**
- 脱敏语料：token/URL/家目录/私钥/长行全部命中；standard 与 strict 行为不同。
- 轮转与保留：写 20 MiB 后剩余 ≤10 MiB、14 天前文件被清理。
- 级别与异步：debug 关闭时 debug 事件不落盘；队列满时计 dropped 且不阻塞；Flush 后文件完整。
- 导出选择：给定 2 小时事件，只导出 incident 窗口 ± 上下文；截断时 manifest 有标记。
- summary 生成：含版本、错误列表、时间线、脱敏声明。

**集成测试**
- 用现有"测试二进制伪装 git"stub（`internal/gitcli/config_test.go:141-190`）制造 push 失败 → 导出包内 `git.fail` 含 op/exit/kind、summary 列出该错误。
- `kill -9` 场景：crash 文件存在、下次启动产生 `app.recover` 与通知。
- 脱敏端到端：带 `user:ghp_xxx@` 的 clone 失败 → 导出包 grep 不到 token/用户名/家目录。
- Headless UI：Help 菜单存在；Report a Problem 流程可走通；About 显示版本。

**人工验收（发布前）**
三个真实场景各做一次"只把诊断包给另一个维护者"，要求 5 分钟内能回答：什么操作、失败在哪、环境是什么、下一步该查什么。做不到就说明事件目录或 summary 缺字段。

## 12. 分阶段实施

| 阶段 | 内容 | 估算 |
|---|---|---|
| Stage 0 | `internal/version` + `-X` + `--version` + About 显示版本；顺手修 About 的 nil context panic | 0.5 人日 |
| Stage 1 | `internal/observability`：事件模型、异步日志、轮转、脱敏、内存环；main 生命周期/崩溃/fault handler；git/task/refresh/watcher 打点；ErrorKind→code | 2 人日 |
| Stage 2 | Report a Problem 录制/导出、summary/manifest、预览与勾选项、Console Copy/Save/耗时、Help 菜单、错误通知动作 | 1.5–2 人日 |
| Stage 3 | 测试与文档：脱敏语料、kill -9 集成、`docs/privacy.md`、`docs/troubleshooting.md` 骨架、Issue 模板更新 | 0.5–1 人日 |

合计约 4.5–5 人日。P1 后续：全量错误码提示、`perf.slow`/`perf.mem`、Windows crash dump、导出包上传端点的 ADR。

## 13. 隐私与合规

- 默认不联网、不上传；诊断包只在用户点击"保存"后写到用户选择的路径。
- 导出前必须预览；敏感项默认关闭；包内不含仓库内容、提交说明、diff、文件内容。
- 日志与导出包设保留期与删除入口（Help → Open Log Folder，用户自行删除；设置页提供"清除诊断数据"）。
- 若未来做上传：单独 ADR + 隐私政策 + 保留期（建议 30 天）+ 按 ID 删除；国内用户按《个人信息保护法》最小必要与告知同意评估。
- 公开 Issue 是公开场所：导出后提醒用户"你将公开上传这个文件"。

## 14. 待评审问题

1. standard 档默认包含"仓库相对文件路径"是否可接受？（排查冲突/路径问题需要，但会暴露项目内部结构）
2. 诊断会话上限 10 分钟 / 10000 条是否合适？
3. `logLevel` 放在设置页还是仅用环境变量（避免普通用户误开 debug 写盘）？
4. 是否需要 `viff --export-diagnostics` 命令行入口（便于远程指导用户操作）？
5. 是否值得额外持久化"最近一条错误快照"（跨重启导出），代价是增加磁盘写？
6. Windows crash dump 默认关闭、仅在崩溃提示里引导，是否足够？

> 第 3 题的 v1 取舍：`logLevel` 已放在设置页 General 的 Debug logging 复选框（同时支持 `VIFF_LOG` 环境变量），后续如发现用户误开会改回仅环境变量。

## 15. 实现状态（v1，2026-11）

v1 已实现，与本文的偏差如下，后续改动请同步本节：

| 能力 | 状态 | 说明 |
|---|---|---|
| 结构化事件日志 + 滚动轮转 + 级别 + 脱敏 + 内存环 + incident 录制 | 已实现 | `internal/observability`；默认 info，录制期 debug |
| 崩溃文件 + Viva fault handler + 启动失败系统弹窗 + 上次崩溃提示 | 已实现 | `cmd/viff/main.go`；Windows crash dump 仍为 P1 |
| 版本号注入（`-X`）+ `--version` + About 显示版本/Go/Viva/Git | 已实现 | CI/release 工作流已传入 `-X` |
| `gitcli.ErrorKind` 稳定错误码 + 通知/日志携带 code | 已实现 | 14 类映射；`docs/troubleshooting.md` 覆盖 |
| 诊断包导出（summary/manifest/env/events/console/repo/crash） | 已实现 | `internal/diagnostics`；≤5 MiB，事件 ≤5000 条或 2 MiB |
| Report a Problem 录制 → 停止 → 保存 zip → Copy Issue Template | 已实现 | Help 菜单 + 状态栏"Recording diagnostics…" |
| Console Copy All / Save / 显示耗时 / Clear 确认 | 已实现 | 单条输出上限 8 KiB |
| UI 导出前预览与敏感项勾选 | **未实现**（P1） | v1 导出包固定 standard 档；用户可用压缩工具自查 |
| 诊断会话 10 分钟自动停止 | **未实现**（P1） | v1 需用户手动停止；事件/体量上限仍由导出器兜底 |
| `perf.slow` / `perf.mem` 等度量事件 | **未实现**（P1） | 事件常量已保留，接入点待补 |
| 一键打开预填 GitHub Issue / 自动上传 | **未实现**（P1/P2） | 当前复制模板 + 用户自行附加 zip；上传需另立 ADR |
| Windows crash dumps (`app.EnableCrashDumps`) | **未实现**（P2） | 需先有用户同意弹窗 |
| 清除诊断数据设置项 | **未实现**（P2） | 当前通过 Open Log Folder 手动删除 |

