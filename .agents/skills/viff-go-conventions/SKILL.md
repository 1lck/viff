---
name: viff-go-conventions
description: "写或改 Go 代码、加包、加依赖、做重构与错误处理时加载；不负责具体 Git 命令语义、测试用例设计与 Viva 界面绘制。"
metadata:
  version: "1.0.0"
---

# Viff Go 代码规范

## 何时使用

- 新增或修改 `internal/`、`cmd/viff` 下的 Go 代码，或拆分包、移动文件。
- 引入第三方依赖，改 `go.mod`/`go.sum`，升级 Go 或 Viva。
- 调整错误处理、`context` 传播、后台调度、缓存与资源关闭。
- 重构公共类型、函数或接口。

## 前置读取

1. `AGENTS.md` 的「目录与依赖方向」「代码规则」「常用命令」：全局硬约束只在那里定义，本 skill 不重复。
2. `docs/design.md` 的架构小节与「低内存来自受控的数据生命周期」表。
3. 改动触及已采纳取舍时，读对应 `docs/decisions/NNNN-*.md` 的「代价」与「复审条件」。
4. `go.mod`：模块 `viff`、Go 1.27.3、直接依赖只有 `github.com/vivaui/viva`。
5. 参考实现：`internal/app/workspace.go`（线程模型）、`internal/app/repo.go`（代次与写锁）、`internal/gitcli/runner.go`（进程）、`internal/process/doc.go`（平台差异）。

## 必须遵守

- 门槛由 CI 执行：quality job 跑 `gofmt -l`、`go mod verify`、`go mod tidy -diff`，测试分片跑 `go vet -tags 'vivahead litheaudit' ./...`（见 `.github/workflows/ci.yml`）。改完本地按 `AGENTS.md`「常用命令」跑。
- 依赖方向只从外向内：`ui → app → {domain, storage, gitcli, platform, process, observability, diagnostics}`。`domain`、`app`、`gitcli`、`storage`、`process` 不得导入 `github.com/vivaui/viva`；Viva 类型留在 `internal/ui`。
- `internal/ui` 里的例外保持最小：只有打开/初始化对话框的 `.git` 存在性判断和设置里的 `exec.LookPath` 可用性检查，不要扩散成第二套 Git 层。
- 外部资源经组合根注入：`cmd/viff/main.go` 构造 `&gitcli.Runner{Path: settings.GitPath}` 传给 `vapp.NewWorkspace`，文件监听经 `Workspace.Watch` 接到 `platform.Watch`。不为每个结构体造接口；`Workspace.Watcher`（只要 `Close`）是「需要关闭语义」的窄接口范例。
- 错误包装用 `fmt.Errorf("...: %w", err)` 并保留 `errors.Is/As` 链；分类沿用 `internal/gitcli/runner.go` 的 `ErrorKind` 与 `KindOf`，不要用 `strings.Contains(err.Error(), ...)` 判断。用户可见错误附带稳定错误码：`gitcli.CodeOf(err)`，通知走 `Workspace.NotifyError`。
- `context` 沿调用链传递：后台任务用 `context.WithCancel`（`Workspace.startTask`），刷新用 `context.WithTimeout` 限时（`Refresh` 为 2 分钟）。读结果带代次/序号（`RepoState.Gen`、`statusSeq`），过期结果在 `post` 回主线程后直接丢弃。
- 线程模型固定：`Workspace`/`RepoState` 只在 UI 线程读写；goroutine 只调用 Git，再把闭包交给 `Workspace.post`，由 `Drain` 应用。`setProgress` 也必须走 `post`。
- 写操作串行：`Workspace.Run` 的写任务取 `writeLock(repo)`（按 `CommonDir`），`RunOptions.Read` 是已有的跳过锁开关（当前调用点都是写命令；只读查询走各自的后台刷新，不要塞进 `Run`）。写任务结束后无论成败都 `Refresh(true)` 重查 Git。
- 资源要有明确归宿：`defer cancel()`；`exec.Cmd.WaitDelay` 防止管道未关导致挂起；`gitcli.Stream.Close/Wait` 停进程；`Workspace.Close` 关监听与日志读取器（`Repo.Log.close()`）。不要留下无人取消的 goroutine。
- 新增依赖必须有回归依据：说明标准库或现有代码为何不够用，同步 `go.mod`/`go.sum`、`licenses/<Component>-<License>.txt` 与 `THIRD_PARTY_NOTICES.md` 表格行（照 Viva/purego/go-toml 现有行格式）。
- 生成物提交：`internal/ui/icons/icons.go` 的 `//go:generate go run ../../../tools/genicons ../../../assets/icons/intellij .` 是唯一入口，改图标后跑 `go generate ./internal/ui/icons` 并提交生成代码；SVG 版权声明留在 `assets/icons/intellij/NOTICE.md`。
- 包布局：包名短小写、无下划线；平台差异用 `_windows.go` / `_other.go` 加 build tag；包说明写在 `doc.go` 或包内一个文件顶部（`internal/process/doc.go`、`internal/app/workspace.go`）；同一包按主题分文件（`status.go`、`ops.go`、`history.go`），不要按类型堆文件或为每个方法造接口。
- 不记录完整 argv、环境变量、仓库内容与凭据；日志与错误码细则见 `viff-observability-privacy`。
- 实现标准是长期维护而非「先跑通」：错误/取消/平台/空边界一次做全；不为赶进度留临时开关、静默降级或注释掉的代码；确实延后的 TODO 必须带 issue/ADR 与复审条件；新抽象要能说明现有代码为何不够。

## 步骤

1. 先 `grep -rn` 找现有实现与调用点，再读相关 `docs/decisions`；确认这层是否已有约定。
2. 定层：纯规则进 `domain`；用例、调度、缓存失效进 `app`；Git 参数与解析进 `gitcli`；OS 差异进 `platform`/`process`；设置与配额进 `storage`。
3. 写最小改动；行为变化同时补测试（见 `viff-testing`）。
4. 本机有 Go 工具链就跑门槛命令并记录退出码；没有就写清「未运行 + 原因」，不要声称跑过。
5. 涉及界面文本、依赖、行为变化时同步文档与 ADR。
6. 报告列出改动文件、命令、平台、结果。

## 检查清单

- [ ] `gofmt -l` 无输出；`go vet -tags 'vivahead litheaudit' ./...` 通过。
- [ ] `go mod tidy -diff` 为空，`go mod verify` 通过。
- [ ] 新导入不违反依赖方向；被改的非 ui 包仍没有 Viva 依赖。
- [ ] 每个新错误都 `%w` 包装；用户可见错误带 `CodeOf` 结果。
- [ ] 新 goroutine 都有取消路径，结果带代次/序号并在过期时丢弃。
- [ ] 写操作走 `Run` 的写锁；没有把只读查询塞进 `Run`。
- [ ] 新依赖已更新 `licenses/` 与 `THIRD_PARTY_NOTICES.md`。
- [ ] 图标等生成物已重新生成并纳入提交。
- [ ] 没有顺手格式化、无关重命名或依赖升级。

- [ ] 没有 happy path 实现、临时开关或无记录的 TODO。

## 常见错误

- 在 goroutine 里直接写 `Workspace`/`RepoState` 字段，导致数据竞争或视图撕裂；应 `post` 回 UI 线程。
- 用 `errors.New` 重新造错、丢掉下层原因，或在上层用字符串匹配 Git 报错。
- 给只读查询加写锁或全局串行，拖慢刷新；锁的粒度是 common Git dir，不是进程全局。
- 为「方便」把 Viva 类型（颜色、节点）带进 `app`/`gitcli`，破坏可替换 UI 的边界。
- 只改 `go.mod` 加依赖，不更新 notices 与许可证文件。
- 用 `time.Sleep` 等后台结果；应用条件轮询（测试夹具的 `settle`/`WaitFor`）。
- 缓存不带代次，切仓库或切提交后旧结果覆盖新视图。

## 相关文件

- `AGENTS.md`、`docs/design.md`、`docs/decisions/`
- `.github/workflows/ci.yml`
- `internal/app/workspace.go`、`internal/app/repo.go`、`internal/app/commands.go`
- `internal/gitcli/runner.go`、`internal/process/`
- `internal/ui/icons/icons.go`、`tools/genicons/`
- `go.mod`、`licenses/`、`THIRD_PARTY_NOTICES.md`
