# 0028 集成终端工具窗

- 状态：已采用（2026-11；MVP 单会话，仅本地 shell）

## 背景
使用者希望在 Viff 内直接开终端，交互参照 IntelliJ IDEA 的 Terminal 工具窗：点击或快捷键展开，shell 的当前目录是当前仓库。现有菜单里的 “Open Terminal” 调用系统终端，切走窗口后无法与仓库状态并排查看，也不能复用 Viff 的字体、主题与键位。

同一 UI 栈上的 [Motty](https://github.com/t8y2/motty) 已经有一台成熟终端：VT/xterm 模拟器约 7 千行、4 千余行测试；本地 shell 后端同时支持 Unix pty 与 Windows ConPTY；Apache-2.0。它的绘制层与 Motty 的应用状态绑定，不能整块搬；模拟器与 pty 会话层可以。

## 决定

- **位置与开关**：底部工具窗，左侧 stripe 的终端按钮或 `⌥F12` 开关。与 Git 工具窗共用同一段底部停靠区：终端显示时 Git 让出位置（按钮高亮切换），关闭终端后 Git 回到原来的高度；两者不同时占两段高度，避免把左侧面板压到无法使用。隐藏工具窗或切换仓库不结束 shell；关闭项目（有运行中的 shell 时先确认）、关闭窗口或 Restart 才结束进程。标题下显示的是该会话的启动目录，不是当前仓库根目录，避免切换仓库后显示错位。
- **分层**：
  - `internal/ui/term`：从 Motty 复制的 VT/xterm 模拟器（含测试与 fuzz 语料）。它只依赖标准库与 `viva/ui`（键编码使用 `ui.Key`/`ui.KeyEvent`），因此放在 UI 层内，不进入 domain/app。
  - `internal/platform/pty`：本地 shell 会话与平台后端（Unix pty、Windows ConPTY）；不导入 Viva。读取有 1 MiB 背压，输入队列 4 MiB，`Notify` 只负责唤醒窗口。
  - `internal/ui/terminal*.go`：工具窗状态、行缓存绘制、选择、滚动、键盘与 IME；只调用 pty 与模拟器，不拼 Git 命令。
- **进程创建**：pty/ConPTY 不走 `internal/process` 的隐藏控制台路径（0022）。终端需要一个真实控制台宿主：Unix 侧 `pty.StartWithSize` 设置 `setsid` 与控制终端；Windows 侧 ConPTY 本身不创建可见窗口，`CREATE_NO_WINDOW` 反而会破坏伪控制台。这是 0022 复审条件里写明的“内置终端单独选择”的落地，不改变 Git 子进程的既有规则。
- **线程与帧**：shell 读取 goroutine 只做缓冲与 `Invalidate`；输出解析在窗口线程的 `onWake` 里，每次最多 128 KiB，剩余留到下一帧；尺寸变化在绘制后用 `after` 应用到模拟器与 pty，绘制路径不做阻塞 IO。光标闪烁 530 ms 半周期、无输入 20 秒后停止请求帧。
- **绘制**：沿用 Motty 的行缓存：按行 ID + version + 光标列 + IME 覆盖区 + 调色板（明/暗）+ 列数缓存文本、`ui.TextRun` 与背景 span；未变的行、只是滚动的行不重建。字体用设置里的等宽字体与字号，光标块、下划线、竖线三种样式。
- **键盘**：网格以 `ui.RoleTerminal` 聚焦，Viva 负责 raw 键盘、IME 组合与粘贴事件。应用快捷键与 shell 的冲突按平台处理：macOS 上 Command 归应用、Control 归 shell，Command+←/→/⌫ 映射为 readline 的 Ctrl+A/E/U；Windows/Linux 上终端聚焦时普通 Control+字母与 Ctrl+数字归 shell（应用自己的 Ctrl 命令键暂时让位），Control+Shift+C/V 仍为复制/粘贴，Windows 另用 Ctrl+V 粘贴。`⌥F12` 始终归应用。
- **来源与维护**：模拟器是 Motty `fbd26b8` 的只读快照，文件不改内容；`internal/ui/term/NOTICE.md` 与 `THIRD_PARTY_NOTICES.md` 记录来源、许可与修改。上游同步用 diff 而不是长期补丁堆；需要改模拟器时在 Viff 侧单独记录。
- **隐私**：终端内容、命令、路径不进日志与诊断包；OSC 52 只在程序主动请求时写剪贴板；粘贴经 bracketed paste 交给程序自己判断。
- **本期范围**：一个会话、本地 shell、滚动回看、选择复制、粘贴、缩放、清屏、重启、错误提示与重试。SSH、多标签、分屏、搜索、链接、内联图片、鼠标上报与 DECSET 2026 的整屏快照不在本期；它们不影响模拟器解析，只是没有对应 UI。## 代价
- 引入两份第三方代码：`creack/pty`（MIT）与 `golang.org/x/sys`（BSD-3-Clause，仅 Windows 编译），需随包维护许可证。
- Viff 从此维护约 7 千行模拟器的快照；上游修复不会自动进来，需要定期 diff。
- 单会话意味着同一窗口只有一条 shell；切换仓库不会重启已在跑的 shell（避免杀掉用户进程），新会话才用新仓库根目录。
- 隐藏工具窗保留进程，这是 IDEA 的行为，但用户可能误以为已结束；Restart、关闭项目与关闭窗口是明确的结束路径。
- 尺寸变化有一帧延迟：绘制先按旧网格画，应用尺寸后下一帧补齐。避免在绘制中做 ioctl。
- Windows ConPTY 需要 Windows 10 1809+；本机无法实机验证，只有 CI 编译与平台测试覆盖。

## 复审条件
- 上游 Motty 的 `internal/term` 或 `localshell` 有安全/正确性修复时，安排一次 diff 同步。
- 增加多标签/分屏、鼠标上报、内联图片、find 或 DECSET 2026 快照时，重新评估会话模型与绘制缓存键。
- Windows 实机出现 IME、Ctrl 键位或 ConPTY 退出码问题，按平台分支修正并补测试。
- 模拟器维护成本超过收益（例如上游 API 分叉、连续两个版本需要手工合并），改为 fork 成独立模块或换用维护中的库。
- Viva 的 `RoleTerminal`/`app.TextInput` 行为变化时，重跑终端无头测试并更新本文。
