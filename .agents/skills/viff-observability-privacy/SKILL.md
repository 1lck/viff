---
name: viff-observability-privacy
description: "加日志事件、错误码、崩溃记录、脱敏规则或诊断导出时加载；不负责 Git 命令实现、界面布局与打包发版。"
metadata:
  version: "1.0.0"
---

# Viff 可观测性与隐私规范

## 何时使用

- 新增或修改结构化事件、事件字段、级别、错误码。
- 调整日志轮转/保留配额、内存环、崩溃记录、诊断会话。
- 改脱敏规则、诊断包内容与选择逻辑、隐私文档或 Issue 模板。
- 排查「日志里多出敏感信息」「诊断包不够用/太大」类问题。

## 前置读取

1. [AGENTS.md](../../../AGENTS.md) 的「目录与依赖方向」「代码规则」「测试规则」。
2. [docs/observability.md](../../../docs/observability.md)：权威规格；第 15 节列出 v1 实现与设计偏差。
3. [docs/privacy.md](../../../docs/privacy.md)：用户可见承诺，改动必须与实现一致。
4. [docs/decisions/0021-observability.md](../../../docs/decisions/0021-observability.md)：已采纳的取舍与复审条件。
5. 代码：`internal/observability/{event,logger,writer,redact,crash,ring}.go`、`internal/diagnostics/{types,bundle,summary}.go`、`internal/app/diagnostics.go`、`cmd/viff/main.go`。

## 必须遵守

- 事件是固定小集合：新增事件先加到 `internal/observability/event.go` 的 `Event*` 常量（现有 23 个），并在 `docs/observability.md` §3.2 目录补「级别 / 触发点 / 主要字段」；不得用自由文本当事件名。落盘格式为 JSONL，公共字段见 `Record`（`event.go`）：`ts`、`seq`、`session`、`level`、`event`、`incident`、`repo`、`op`、`code`、`dur_ms`、`msg`、`data`。
- 脱敏必须先于落盘：唯一入口 `Redact` / `RedactPath` / `RepoHash`（`redact.go`）；日志、崩溃文件、诊断包都走它，禁止旁路。仓库根只写 `sha256[:8]`，路径写 `<repo>`/`<path>`/`~`，凭据写 `***`，remote URL 只留 scheme+host。
- 永不记录：完整 argv、环境变量、仓库内容/diff/提交说明、凭据与私钥、remote 完整 URL、绝对路径、设置中的密钥字段值（只记字段名）。`Event.Fields` 的注释已写明这条边界（`event.go`）。
- 级别纪律：`LevelError` 是零值，漏设级别的事件仍按 error 保留、不会被过滤；默认 `LevelInfo`；debug 仅在 `VIFF_LOG=debug`、设置 `logLevel` 或诊断会话期间开启。`StartIncident` 强制 debug，`StopIncident` 恢复录制前级别（`logger.go`）。
- 配额是契约：单文件 2 MiB 轮转、最多 5 个文件、总量 10 MiB、14 天保留、目录 `0700`、文件 `0600`、文件名 `viff-YYYYMMDD-HHMMSS-<pid>.jsonl`（`logger.go` 包注释与 `writer.go`）。改这些常量必须同步 `docs/observability.md` §5 与 `docs/privacy.md`。
- 不阻塞调用方：`Event` 在调用方 goroutine 完成脱敏与序列化，投递 4096 条有界队列；队列满时丢新事件并累加 `Dropped`，不得阻塞 UI（`logger.go`）。文件写入只在单个 writer goroutine；`Flush` 最多等 2 秒，导出前允许短暂 Flush。
- 单条记录上限 4 KiB、`msg` 512 字节，超限截断并写 `data.truncated`/`data.bytes`；不要把大字段塞进 `Fields` 指望能落盘。
- 内存环默认 4096 条（`RingSize`）；诊断会话每场保留 20000 条、最多记住最近 5 场（`logger.go` 常量）。崩溃文件从内存环取最近 200 条，不要依赖操作系统 core。
- 崩溃路径：`main` 只做 `os.Exit(run())`，`run()` 的 defer 里 recover 并同步写 `crash/last-crash-<ts>-<pid>.txt`（0600，写临时文件后 rename），内容含 panic 栈与最近 200 条事件快照；接 `app.SetFaultHandler` 捕获 Viva 回调边界 panic；启动失败与崩溃都走 `app.ShowFatalError`（GUI 无 stderr）。下次启动用 `PreviousCrash(crashDir, 7*24h)` 提示（`.seen` 后缀会被忽略，但 v1 main 尚未执行改名，重复提示属已知偏差，改动前先核实）。
- 错误码：`gitcli.ErrorKind.Code()` 产出稳定码（`runner.go`，13 个显式映射，其余回退 `git.failed`），`git.fail` 事件与错误通知同时带 `code` 与 `kind`；新增 kind 同步 `docs/observability.md` §7 与 `docs/troubleshooting.md` 锚点，不写死 40 位对象 ID。
- 诊断导出两阶段：录制（`StartIncident`，ID 形如 `inc-<8 hex>`）→ 导出。导出先 `Flush`；事件窗口优先取 incident 起止，否则 `Options.Since/Until`，再退到首末事件与当前时间（`bundle.go`）；无录制时 app 层取最近 30 分钟（`internal/app/diagnostics.go`）。指定 incident 时只取该会话事件。
- 包结构与预算固定：`summary.md`、`manifest.json`、`env.txt`、`settings-summary.json`、`events.jsonl`、`console.txt`（可选）、`repo.txt`、`crash/`（最多 3 份）；zip ≤5 MiB、events ≤5000 条或 2 MiB、console ≤1 MiB、crash ≤1 MiB（`bundle.go` 常量）。超限按 console→crash→events→settings 顺序裁剪，并在 `manifest.truncations` 写明原因。
- 导出端保留兜底脱敏：`internal/diagnostics` 的 sanitizer 会丢弃仍像绝对家目录/凭据的文本行、替换可疑 JSON 值，并把 `redaction_reviewed` 置 false；不要因为上层已脱敏就删掉这层防线。
- 零服务端：不新增上传、遥测、崩溃上报 SDK 或第三方分析；诊断包只在用户选择的路径落盘。未来若加端点，必须另立 ADR 并逐份显式同意（ADR 0021 的复审条件）。
- v1 偏差照实说：导出前预览与敏感项勾选、录制 10 分钟自动停止、`perf.slow`/`perf.mem` 接入、Windows crash dump、清除诊断数据设置项尚未实现（`docs/observability.md` §15）。写文档或汇报时不得声称已有。

## 步骤

1. 先查 §3.2 是否已有可复用事件；没有才新增常量与目录行，字段用小对象、snake_case、总长受 4 KiB 约束。
2. 在现成边界构造 `observability.Event` 并填入 `Op`/`Code`/`Dur`/`Fields`：`internal/app/workspace.go`（`logCommand`、`notify`、`startTask`）、`internal/app/repo.go`（刷新与 watcher）、`internal/gitcli/runner.go`（Log 回调）。业务层只描述事实，不格式化日志行。
3. 所有进入 `Fields`、`msg`、崩溃文本的字符串确认会被 `Redact` 处理；路径用 `RedactPath` 并传入仓库根。
4. 更新文档：`docs/observability.md` 事件表；涉及用户承诺改 `docs/privacy.md`；行为变化同步 README 已知限制或新增 ADR。
5. 补测试：脱敏语料、级别过滤、轮转与保留、导出选择与截断、summary 内容（验收清单见 `docs/observability.md` §11）。
6. 本地门禁：`go test -count=1 ./internal/observability ./internal/diagnostics ./internal/app`、`go vet -tags 'vivahead litheaudit' ./...`；本机无 Go 工具链时写明「未运行 + 原因」，不得声称跑过。

## 检查清单

- [ ] 事件名来自 `Event*` 常量，且已写进 §3.2 目录。
- [ ] 所有字符串经 `Redact`/`RedactPath`；没有绝对路径、URL path、token、私钥。
- [ ] 没有新增完整 argv、环境变量、diff、提交说明、仓库内容的记录。
- [ ] 级别正确，错误事件带稳定 `code`；debug 事件不会在默认级别落盘。
- [ ] 配额/保留常量改动已同步两份文档。
- [ ] 导出选择符合 §8.5，超出预算时有 manifest 截断标记。
- [ ] 崩溃路径仍能写文件、不 panic，且下次启动能提示。
- [ ] 没有新增网络请求或自动上传。

## 常见错误

- 把完整命令、stderr 原文无限长、绝对路径直接塞进 `Fields` 或 `msg`。
- 新增事件却不更新事件目录，工具与文档从此漂移。
- 在 UI 线程同步 Flush 或写文件，造成掉帧；导出才允许短暂 Flush。
- 认为「只有 debug 才导出」就放松脱敏，忽略默认 info 日志同样落盘。
- 删掉 `internal/diagnostics` 的兜底 sanitizer，只信上层脱敏。
- 顺手加入上传/遥测/第三方 SDK，违反零服务端与 ADR 0021。
- 把 `docs/observability.md` §15 的「未实现」写成已实现。

## 相关文件

- [internal/observability/event.go](../../../internal/observability/event.go)、[logger.go](../../../internal/observability/logger.go)、[writer.go](../../../internal/observability/writer.go)、[redact.go](../../../internal/observability/redact.go)、[crash.go](../../../internal/observability/crash.go)、[ring.go](../../../internal/observability/ring.go)
- [internal/diagnostics/types.go](../../../internal/diagnostics/types.go)、[bundle.go](../../../internal/diagnostics/bundle.go)、[summary.go](../../../internal/diagnostics/summary.go)
- [internal/app/diagnostics.go](../../../internal/app/diagnostics.go)、[internal/app/workspace.go](../../../internal/app/workspace.go)、[cmd/viff/main.go](../../../cmd/viff/main.go)、[internal/gitcli/runner.go](../../../internal/gitcli/runner.go)
- [internal/ui/diagnostics.go](../../../internal/ui/diagnostics.go)、[internal/ui/help.go](../../../internal/ui/help.go)
- [docs/observability.md](../../../docs/observability.md)、[docs/privacy.md](../../../docs/privacy.md)、[docs/troubleshooting.md](../../../docs/troubleshooting.md)、[docs/decisions/0021-observability.md](../../../docs/decisions/0021-observability.md)
