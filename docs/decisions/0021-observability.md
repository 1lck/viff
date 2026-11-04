# 0021 可观测性、崩溃记录与诊断导出

- 状态：已采用（2026-11）；v1 已实现，实现范围与偏差见 [docs/observability.md](../observability.md) 第 15 节
- 详细规格：[docs/observability.md](../observability.md)

## 背景

用户遇到问题时，Viff 无法提供任何可供排查的证据：没有落盘日志（唯一的 Console 只存在于内存，400 条上限，退出即丢）、没有版本号（About 只显示 Git 版本）、没有 panic/崩溃记录、没有诊断导出入口。Issue 模板要求"版本 + Console 输出"，但产品并不具备提供这些信息的能力。`docs/design.md` 第 152 行已经承诺"有界日志，记录任务时长、取消和缓存命中；诊断导出脱敏；不默认上传代码、路径或 Git 输出"，`SECURITY.md` 也要求漏洞报告附脱敏日志——目前都是悬空承诺。

结果是维护成本被推到最高：每个反馈都要多轮追问版本、系统、步骤、错误原文，且多数问题无法复现。

## 替代项

- **只写本地日志文件，不做导出**：实现最省，但用户找不到、不会打包、不知道给什么；作者仍要远程指导，达不到"能排查"。
- **接 Sentry 等自动崩溃上报**：能力完整，但引入第三方服务、数据出境与隐私政策负担，且当前应用零网络请求，与"本地优先、默认不联网"的定位冲突。
- **本地结构化事件 + 崩溃记录 + 用户主动导出诊断包（采用）**：无服务端、用户可控、体积小、可脱敏；维护者拿到的是可读摘要 + 结构化明细。

## 决定

- 新增 `internal/observability` 包，记录**结构化事件**（固定事件名 + 稳定字段，JSONL），而不是自由文本日志；三条通道：滚动日志文件、内存事件环、Console 镜像。
- 日志有界：默认 info，按天 + 2 MiB 轮转，最多 5 个文件/10 MiB/14 天；单 writer goroutine 异步写，阻塞 UI 不允许；退出与 panic 前 Flush。
- **脱敏是落盘的前置条件**：单一 `Redact` 入口；仓库路径哈希、remote 只留 host、家目录替换、凭据与私钥永不记录、argv 默认不记、Git 输出截断脱敏。standard/strict 两档 + 导出时用户勾选敏感项。
- 崩溃处理：`main` 拆 `run()` 并在 `defer` 中 recover 写 crash 文件；接 `app.SetFaultHandler`（Viva 默认 handler 为 nil，回调边界 panic 会被静默吞掉）；启动失败改用 `app.ShowFatalError`（Windows GUI 构建下 stderr 不可见）；下次启动检测 crash 文件并提示。
- 注入版本：`internal/version` + `-X` + `--version` + About 显示版本/commit/构建时间，打通"用户装的是哪个构建"。
- 统一错误码：`gitcli.ErrorKind` 映射为稳定字符串（如 `push.rejected`），进入通知、日志与导出，并链接 `docs/troubleshooting.md`。
- 诊断导出：`Help > Report a Problem…` 先录制（临时 debug + incident ID），再导出 zip（`summary.md` 自动摘要 + `manifest.json` + `env.txt` + `settings-summary.json` + `events.jsonl` + `console.txt` + `crash/`，≤5 MiB），导出前预览与勾选，保存后提供复制问题模板。
- **不新增服务端、不做自动上传**：v0.1 交付方式为"用户保存 zip → 贴到 GitHub Issue/私密安全报告"。未来若引入上传端点，另立 ADR，且必须逐份显式同意。
- 隐私：诊断包不含仓库内容、提交说明、diff、文件内容；提供清除诊断数据入口；发布 `docs/privacy.md` 说明采集项、保留期与删除方式。

## 代价

- 约 4.5–5 人日的实现成本（版本 0.5 + 日志/崩溃 2 + 导出 1.5–2 + 测试文档 0.5–1），以及后续维护事件目录的纪律。
- 每次 Git 命令增加一次事件构造与脱敏开销（微秒级，异步写）；debug 会话期间写盘量增加，靠时间/条数上限约束。
- 磁盘占用最多 10 MiB；需要新增设置字段与 Help 菜单，UI 面积略增。
- 脱敏做得越严，排查信息越少；standard/strict 两档与导出勾选是折中，仍需用真实故障验证"包够不够用"。

## 复审条件

- 真实 triage 中发现诊断包缺少关键证据（事件目录不足或脱敏过度）时，先扩事件字段，再考虑放宽脱敏。
- 需要自动崩溃上报/遥测时：用户量与 triage 成本证明必要，且完成隐私政策后，另立 ADR 选型（优先自托管 GlitchTip 或最小自建端点），不直接接第三方 SaaS。
- Viva 提供原生日志/诊断 API，或 `SetFaultHandler` 语义变化时，重新评估崩溃捕获实现。
- 诊断包超过 5 MiB 或用户普遍无法完成导出流程时，简化包内容与流程。
