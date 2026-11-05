---
name: viff-ui-viva
description: "改 internal/ui 的 Viva 视图、绘制、主题、图标、键盘焦点或 UI 截图时加载；不负责 Git 命令与解析、日志事件与诊断包定义。"
metadata:
  version: "1.0.0"
---

# Viff 界面（Viva）规范

## 何时使用

- 修改 `internal/ui` 的视图、视图模型、绘制、主题、可复用组件、菜单与对话框。
- 新增或调整界面文案、按键、焦点、弹窗、提示与状态栏行为。
- 改图标、截图、无头 UI 测试，或排查重绘/卡顿/主题不可读问题。

## 前置读取

1. [AGENTS.md](../../../AGENTS.md) 的「目录与依赖方向」「代码规则」「测试规则」。
2. [docs/design.md](../../../docs/design.md)：性能预算、视觉还原、可访问性与文本能力要求。
3. [docs/decisions/0004-icons.md](../../../docs/decisions/0004-icons.md)、[docs/decisions/0006-deferred-ui-work.md](../../../docs/decisions/0006-deferred-ui-work.md)。
4. [docs/testing.md](../../../docs/testing.md)：无头套件与截图命令、未验证项。
5. 代码：`internal/ui/window.go`、`chrome.go`、`theme.go`、`draw.go`、`widgets.go`、`dialog.go`、`popup.go`、`keys.go`、`ids.go`、`shots_test.go`。

## 必须遵守

- 只调用 app：状态从 `app.RepoState`/`Workspace` 查询，动作调用 app 用例；不拼 Git 业务命令、不解析 Git 输出、不读 `.git`。现存例外仅打开/初始化对话框的路径存在性判断（`dialogs.go`、`welcome.go`、`worktrees.go`）和设置的 `exec.LookPath` 可用性检查（`gitsettings.go`），改动保持轻量，不要扩散成第二套 Git 层。
- 渲染路径不阻塞：`Window.Paint` 及其调用的 `paintXxx` 不做阻塞 IO、网络、Git 调用；需要新数据时向 app 发起查询/刷新，等 `Invalidate`/`OnWake` 后重画。
- 绘制期副作用走 `w.after`：`runDeferred` 在绘制结束后执行（`window.go`）。`Canvas.AfterPaint` 在普通绘制中被静默丢弃，理由与替代见 [ADR 0006](../../../docs/decisions/0006-deferred-ui-work.md)。
- 状态带代次：`RepoState.Gen` 单调递增，换回同一仓库也不复用（`internal/app/repo.go`）；app 内部用 `w.Repo != rs` 与请求序号丢弃过期结果，界面用 `Window.syncRepo` 在 Gen 变化时重置视图模型。新增缓存必须按 Gen/根路径失效；缓存只能加速，Git 仍是事实来源。
- 颜色只用 token：一律取 `Palette` 字段（`theme.go`），不写死颜色；新增 token 同时补 `DarkPalette` 与 `LightPalette`；Viva 组件颜色经 `Palette.viewTheme()` 映射，不要绕过。
- 字体与度量集中在 `theme.go` 常量（`fontUI=13`、`rowH=24`、`iconSize=16` 等）；等宽字体 macOS 用 `platform.EditorFontFamily`，其他平台用 `ui.MonoSystem`，不要向未注册该字族的平台请求它。编辑器字号取设置 `FontSize`（`diffview.go`）。
- 缩放照实做：设置字段 `UIScale`（0.75–2，`internal/storage/settings.go`）目前只进设置与诊断环境快照；要让界面真实缩放，必须接入 Viva 的度量/像素缩放，不要在局部乘系数冒充全局缩放。
- 文案英文：按钮、菜单、对话框、通知统一英文并沿用既有措辞（如 `Report a Problem…`、`Open Log Folder`、`Viff closed unexpectedly last time`）；省略号用法与句首大写保持一致。
- 图标走生成：图标源在 `assets/icons/intellij`，运行 `go generate ./internal/ui/icons`（`tools/genicons`），提交生成物与版权声明；明暗各一套 `icons.Pair`，多色图标按色拆 Part。
- 交互可测试：可点击元素用 `tree.ID`/`tree.Sub` 在 `ids.go` 声明稳定 ID，不按索引或临时字符串拼；对话框用 `PushFocusScope`/`TrapFocus`/`SetEscapeTarget` 并遵循 `Enter` 提交、`Esc` 取消、textarea 内 `Enter` 换行（`dialog.go`）；弹窗遵循 `Esc` 关闭/退层、上下移动、`←`/`→` 子菜单、`Enter`/`Space` 激活（`popup.go`）。改键位同步无头测试。
- UI 测试用 `vivahead`：文件头 `//go:build vivahead`，先 `platform.RegisterFonts()`，再 `headless.Open(cfg, nil, headless.Options{Scale: 2})`；用 `WaitFor` 等应用状态、`Settle` 等绘制；断言用 `HasText`/`Texts()`/剪贴板/字段值，不只比像素；空闲用 `assertIdle` 防重绘循环。

## 步骤

1. 先 `grep` 现状并读相关 ADR、现有测试，确认状态来源、ID 与可复用组件，再动代码。
2. 改视图/视图模型：数据从 `RepoState`/设置读；新控件优先复用 `widgets.go`、`dialog.go`、`popup.go`，不引入通用消息总线或自研组件框架。
3. 改绘制：用 `pen` 与 palette token；列表按可见区虚拟化、按需展开（`docs/design.md` 性能约束），避免每帧复制大字符串或全量重算。
4. 需要加载、防抖或延迟副作用时放进 `w.after`，或调用 app 用例并在回调里 `Invalidate`。
5. 补/改无头测试：覆盖新行为与字段值；界面变化需要证据时加 `snap`，截图仅在显式设置 `VIFF_SHOTS` 时生成（`VIFF_SHOTS_BY_TEST=1` 时按测试名分目录），输出到 `.viff-test/`。
6. 本地门禁：`gofmt -w .`、`go vet -tags 'vivahead litheaudit' ./...`、`go test -count=1 -tags 'vivahead litheaudit' ./internal/ui`；本机无 Go 工具链时写明「未运行 + 原因」，不得声称跑过。
7. 行为或文案变化同步文档：README 已知限制、`docs/features.zh-CN.md`，或按 `docs/decisions` 新增 ADR。

## 检查清单

- [ ] 没有新增 Git 命令拼接、Git 输出解析或 `.git` 读取；既有例外未扩散。
- [ ] `Paint` 路径无阻塞 IO/网络/Git；绘制期副作用走 `w.after`。
- [ ] 明暗两套主题都检查过，颜色全部来自 token。
- [ ] 新文案为英文且与既有风格一致。
- [ ] 交互元素 ID 稳定并登记在 `ids.go`；键盘、焦点、弹窗与对话框行为正确。
- [ ] 新增图标已 `go generate` 并提交生成物与版权声明。
- [ ] 无头测试断言状态/字段值，空闲无重绘循环。
- [ ] 截图只在需要时生成、输出到 `.viff-test/`，未入库。

## 常见错误

- 在 `Paint` 里同步查询 app 或刷新仓库：卡顿、重入、掉帧。
- 用 `Canvas.AfterPaint` 排绘制期任务：普通绘制下被静默丢弃，功能看似随机失效。
- 缓存 `RepoState` 数据却不随 Gen 失效：换仓库后仍显示旧状态。
- 只改暗色或写死颜色：亮色主题不可读，且违反 token 约定。
- UI 文案写成中文或自由措辞：与既有英文界面不一致。
- 用像素截图代替状态断言：Windows 无头不绘原生文本覆盖层，容易误报。
- 测试用固定 sleep 等结果而不 `WaitFor`/`Settle`：套件 flaky 且拖慢 CI。

## 相关文件

- [internal/ui/window.go](../../../internal/ui/window.go)、[chrome.go](../../../internal/ui/chrome.go)、[draw.go](../../../internal/ui/draw.go)、[theme.go](../../../internal/ui/theme.go)、[widgets.go](../../../internal/ui/widgets.go)、[dialog.go](../../../internal/ui/dialog.go)、[popup.go](../../../internal/ui/popup.go)、[keys.go](../../../internal/ui/keys.go)、[ids.go](../../../internal/ui/ids.go)
- [internal/ui/icons/icons.go](../../../internal/ui/icons/icons.go)、[internal/ui/diagnostics.go](../../../internal/ui/diagnostics.go)、[internal/ui/help.go](../../../internal/ui/help.go)
- [internal/ui/shots_test.go](../../../internal/ui/shots_test.go)：`uiFixture`、`settle`、`assertIdle`、`snap`
- [internal/app/repo.go](../../../internal/app/repo.go)、[internal/app/workspace.go](../../../internal/app/workspace.go)、[internal/storage/settings.go](../../../internal/storage/settings.go)
- [docs/decisions/0004-icons.md](../../../docs/decisions/0004-icons.md)、[docs/decisions/0006-deferred-ui-work.md](../../../docs/decisions/0006-deferred-ui-work.md)
- [assets/icons/intellij/NOTICE.md](../../../assets/icons/intellij/NOTICE.md)、`tools/genicons`
