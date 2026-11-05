# AGENTS.md

Viff 是一个用 Go 和 Viva 编写的原生 Git 桌面客户端：一个 Go 进程、无 Electron/WebView、Git 通过本机 CLI 子进程执行。
本文件是所有编码 agent 的工作约定，也是本仓库规范的**唯一事实来源**；`CLAUDE.md` 只负责导入本文件。

按任务加载 `.agents/skills/` 里的细则（索引见文末）。skill 是「怎么做」的步骤，不能覆盖本文件的硬约束；两者冲突时以本文件为准。

## 工作原则：长期维护优先

- 标准是「以后还能改」，不是「现在能跑」：错误路径、取消路径、平台差异、空/边界状态、文档与测试一次做全，不留只覆盖正常流程的实现。
- 设计先于编码：新增公共类型、接口、事件、文件格式或命令前，先想清半年后的调用方与升级路径；新抽象要能说明现有代码为什么不够。
- 允许延后，不允许遗忘：确实要晚点做的，用 TODO 记录并附 issue/ADR 与复审条件；不留临时开关、静默降级、注释掉的代码和无主实验代码。
- 复用既有模式：夹具、错误分类、事件模型、刷新代次、设置迁移都已有约定，优先扩展而不是另起一套。
- 资源与性能受控：不引入常驻 goroutine、无界缓存或无上限重试；数量级变化要能测量，不靠感觉。
- 取舍写进 ADR：影响架构、兼容性或维护成本的选择，按 `docs/decisions` 记录背景、替代项、代价与复审条件，让后来者知道为什么这么设计。
- 「改动最小化」指范围而不是标准：只改该改的地方，但改到能长期维护的完成度；不为省事违反分层、跳过测试或欠文档。

## 开始前必读

| 主题 | 文件 |
| --- | --- |
| 产品定位、架构、性能与冲突边界 | [docs/design.md](docs/design.md) |
| 已采纳的架构决策（含代价与复审条件） | [docs/decisions/README.md](docs/decisions/README.md) |
| 测试与平台说明 | [docs/testing.md](docs/testing.md) |
| 日志、崩溃、诊断与隐私边界 | [docs/observability.md](docs/observability.md)、[docs/privacy.md](docs/privacy.md) |
| 发布流程与发布说明格式 | [docs/releases.md](docs/releases.md) |

改一块代码前，先用 `grep`/codebase 搜索确认现状，再看相关 `docs/decisions` 记录：已采纳的取舍可能来自真实回归，不要在没读理由时直接推翻。

## 项目事实

- Go 版本与依赖以 `go.mod` 为准（当前 Go 1.27.3，`github.com/vivaui/viva v0.2.3`）；模块名 `viff`。
- 入口 `cmd/viff`；界面在 `internal/ui`（Viva 视图）；构建产物在 `build/`、安装包在 `dist/`，**都不提交**。
- 界面文案当前为**英文**，与既有 UI 保持一致。仓库文档的双语布局按文件而定，**沿用该文件既有布局，不要重排**：README 分成英文 `README.md` 与中文 `README.zh-CN.md`；`CONTRIBUTING.md`、`SECURITY.md` 在同一文件内分 `## 中文` / `## English`；`docs/testing.md` 等专题文档按段落中英对照；发布说明必须是「中文 → `---` → English」。
- 系统 `git` 是运行时依赖，不是 vendor 的库；不假设 Git 版本，能力要探测。
- 提交消息、分支、PR 可以用中文；面向外部贡献者的 README/CONTRIBUTING 保持中英双语。
- 本机未安装 Go 时不要声称跑过测试；写清「未运行 + 原因」。

## 目录与依赖方向（硬约束）

| 目录 | 职责 | 约束 |
| --- | --- | --- |
| `cmd/viff` | 启动、装配、退出与资源回收 | 不承载 Git 业务规则 |
| `internal/domain` | 仓库身份、引用、提交、差异、冲突、操作状态 | 不导入 Viva，不启动进程 |
| `internal/app` | 用例、查询、调度、状态版本、缓存失效 | 只经窄接口访问外部资源 |
| `internal/gitcli` | Git 参数构造、机器输出解析、进程生命周期、能力探测 | 不返回 UI 节点，不直接改 UI |
| `internal/ui` | Viva 视图、视图模型、主题、可复用交互组件 | 不拼 Git 命令，不直接读 `.git` |
| `internal/platform` | 文件事件、外部打开、窗口与平台集成 | OS 差异集中在此 |
| `internal/process` | 子进程平台属性（Windows 隐藏控制台等） | 其他包起子进程必须经过它 |
| `internal/storage` | 设置、迁移、缓存、日志配额 | 不保存另一份权威 Git 状态 |
| `internal/observability` | 结构化事件日志、崩溃记录、诊断导出 | 脱敏先于落盘 |
| `internal/diagnostics` | 诊断包组装 | 不含仓库内容、凭据、令牌 |

- 依赖只从外向内：`ui → app → {domain, storage, gitcli, platform, process, observability, diagnostics}`；反向导入不允许。
- `domain`、`app`、`gitcli`、`storage`、`process` **不得导入 Viva**；Viva 类型留在 `internal/ui`。
- `ui` 不把 Git 业务规则搬进界面：不拼 Git 业务命令、不解析 Git 输出。现存例外只有打开/初始化对话框里的 `.git` 存在性判断和设置里的 `exec.LookPath` 可用性检查，改动这类代码时保持轻量，不要扩散成第二套 Git 层。
- 箭头是调用/通知关系；源码依赖由 `app` 定义的小接口约束，适配器在 `cmd/viff` 组装。
- 不为每个结构体造接口，不引入通用消息总线或自研组件框架：单进程、单模块、模块化单体。

**Git 是仓库事实来源。** UI 缓存只能加速，不能替代重新查询：外部 AI、hooks、其他终端都会改仓库；执行写操作前重查 HEAD、index 与涉及内容，发现变化就刷新并要求重新选择。

## 常用命令

```sh
gofmt -w .                                  # Go 代码必须 gofmt 干净（CI 检查）
go vet -tags 'vivahead litheaudit' ./...     # 静态检查（含无头测试代码）
go test -count=1 ./...                       # domain / gitcli / storage / app / platform
go test -count=1 -tags 'vivahead litheaudit' ./internal/ui   # 无头 UI 测试

# 截图（仅显式开启；无头测试默认不打开窗口）
VIFF_SHOTS="$PWD/.viff-test/shots" VIFF_SHOTS_BY_TEST=1 \
  go test -count=1 -tags 'vivahead litheaudit' ./internal/ui

go build -trimpath -ldflags="-s -w" -o build/viff ./cmd/viff
python scripts/package.py v0.1.0-ci --smoke   # 打包与安装冒烟（各目标 OS 上跑）
python3 scripts/release_notes.py docs/releases/vTAG.md --tag vTAG  # 发布说明校验
```

在 CI、SSH 等无图形环境**不要启动 `cmd/viff`**：用 Viva 离屏后端；`vivahead` 只启用测试，不保证真窗口行为。

## 代码规则

- `gofmt` 干净是门槛；`go vet`（含 `vivahead litheaudit` 标签）必须通过；`go mod tidy -diff` 必须为空。
- 新增依赖需要回归依据：说明为什么标准库/现有代码不够用，更新 `go.mod`/`go.sum`、`licenses/` 与 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)，并记录影响。
- 错误处理：包装用 `fmt.Errorf("...: %w", err)`，保留 `errors.Is/As` 链；不吞错，不用日志代替返回；用户可见错误要带稳定错误码（见 `docs/observability.md` §7）。
- `context` 沿调用链传递；**读**操作可取消且必须丢弃过期结果；**写**操作取消不等于撤销——中止进程后重新查询 Git 状态，明确是否 continue/abort，不自动重试。
- 执行 Git：参数数组、绝不经 shell 拼接；机器可读格式（`--porcelain=v2 -z` 等），路径按 NUL 分隔保留原样；stderr 持续消费且限量保留；对象 ID 不写死 40 位。
- 同一仓库的写操作串行；改共享引用（refs、worktree 共享元数据）按 common Git dir 协调。
- 不记录完整 argv、环境变量、仓库内容、diff、提交说明、凭据；日志与诊断包一律先脱敏。
- Windows：所有子进程经 `internal/process`（`SysProcAttr.HideWindow`/`CREATE_NO_WINDOW`），不要新增会弹控制台的调用。
- UI：渲染路径不做阻塞 IO 和 Git 调用；状态从 `app` 查询，交互文案英文，明暗主题都要可用。
- 图标由 `go generate ./internal/ui/icons` 从 `assets/icons/intellij` 生成，生成物提交；新增图标保留版权声明。

## 测试规则

- 用 `t.TempDir()` 建临时仓库；隔离 `HOME`/`XDG_CONFIG_HOME` 与 Git 全局/系统配置（`GIT_CONFIG_GLOBAL`、`GIT_CONFIG_SYSTEM` 等）；**不得**读写本机真实仓库、真实设置或用户凭据。
- 远端用本地临时裸仓库；测试不访问公网、不依赖登录态。
- 无头 UI 测试用 `vivahead`；外部 issue 回归场景用 `litheaudit`（opt-in）。测试不得打开真实应用窗口。
- 截图只在 `VIFF_SHOTS` 显式设置时生成；仓库只保留精选演示截图（`docs/screenshots/`），CI 产物不入库。
- 修 bug 先补一个能失败的测试；测试自身不依赖长 sleep 或网络超时；CI 每个分片默认 10 分钟 `go test` 超时，套件按顶层测试分片并行。
- 平台相关断言按 `runtime.GOOS` 分支，不假设 Unix 路径、大小写敏感性或换行符；Windows 子模块等场景优先用 Git 自身 helper。
- 测试断言「Git 状态 / 应用状态 / 字段值」，不只依赖像素截图。

## 文档规则

- 行为变化必须同步：README（含 `README.zh-CN.md`）的已知限制、`docs/features.zh-CN.md`、相关专题文档，或新增 `docs/decisions` 记录。
- 架构/取舍类决策写 `docs/decisions/NNNN-slug.md`：背景、替代项、决定、代价、复审条件；编号递增并更新索引。被替代的决策只改状态，**不删内容**。
- 双语内容一一对应：改一侧必须同步另一侧，标题顺序与含义保持一致；不要为「统一格式」重排既有文档结构。
- 链接用相对路径；引用外部项目/规范时给可访问链接；不复制版权文本，许可证声明放 `licenses/` 与 notices。
- 发布说明严格按 `docs/releases/TEMPLATE.md` 与 `scripts/release_notes.py` 校验；发布说明在打 tag 前提交到 `main`。
- 不写未经验证的性能、内存、兼容性数字；报告实测时写清平台、版本与命令。

## 提交与 PR

- 分支从 `main` 切出；沿用仓库既有命名（`feat/`、`fix/`、`docs/`、`codex/` 等短横线风格）。
- 提交信息用祈使句说明**为什么**改；一个提交一个关注点；不混入无关格式化、重命名、依赖升级。
- 一个 PR 聚焦一件事；用 [`.github/pull_request_template.md`](.github/pull_request_template.md)：问题、做法、实际运行的命令与结果、平台（OS + `git --version`）、界面变化前后截图、自查清单。
- 不提交 `build/`、`dist/`、`.viff-test/`、`.lithe/`、`output/`、凭据、令牌、私有仓库内容或构建产物。
- 不 force push `main`，不擅自改写他人分支；需要 force-with-lease 时说明原因。
- CI 的 docs-only 分类包含 `AGENTS.md`、`CLAUDE.md`、`.agents/*`：规范与 agent 文档变更不触发全量构建。

## Agent 行为约定

- 改动最小化指范围而不是标准：只改与任务相关的代码，不顺手重构、不批量格式化、不升级依赖换「顺便」；范围内的实现按「工作原则」做完整。
- 不自行发版、不打 tag、不推送、不创建 Release/Issue；这些需要用户明确授权。
- 不启动 GUI，不读写用户真实仓库、设置、日志目录（除非任务明确要求并在临时环境验证）。
- 报告要可核对：命令、退出码、平台、结果；失败就说失败，不编造输出。
- 临时脚本与验证产物放 `.viff-test/`（已被忽略），不留在仓库树里。
- 需要并行开发多个任务时用 `git worktree`（本仓库实践：`.agents/worktrees/<branch>/`），每条分支独立、写完各自验证。

## Skill 索引

| skill | 何时加载 |
| --- | --- |
| [viff-go-conventions](.agents/skills/viff-go-conventions/SKILL.md) | 写/改 Go 代码、加包、加依赖、做重构或错误处理时 |
| [viff-testing](.agents/skills/viff-testing/SKILL.md) | 写测试、修 flaky、跑无头 UI 套件、准备证据时 |
| [viff-ui-viva](.agents/skills/viff-ui-viva/SKILL.md) | 改界面、主题、绘制、交互、图标或 UI 截图时 |
| [viff-git-integration](.agents/skills/viff-git-integration/SKILL.md) | 新增 Git 命令、解析机器输出、处理进程/取消/写操作时 |
| [viff-observability-privacy](.agents/skills/viff-observability-privacy/SKILL.md) | 加日志事件、错误码、崩溃处理、诊断导出、脱敏时 |
| [viff-docs-adr](.agents/skills/viff-docs-adr/SKILL.md) | 改文档、写 ADR、同步双语、更新 README/features 时 |
| [viff-release-ci](.agents/skills/viff-release-ci/SKILL.md) | 动 CI/发布/打包/发布说明/版本号/依赖升级时 |
