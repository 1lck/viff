# 0022 子进程不弹控制台窗口

- 状态：已采用（2026-11）

## 背景
Windows 发布构建是 GUI 子系统（`-H=windowsgui`），进程本身没有控制台。Windows 会为这种进程启动的每个控制台子进程新建一个控制台窗口：修复前，Viff 启动时的版本探测与仓库刷新会连续执行多条 `git` 命令，之后轮询监听每 6 秒再触发一轮刷新（`status`、操作状态、冲突、引用、远端、stash、worktree、浅克隆检查），于是打开应用后黑色控制台窗口连续闪现，还会不断把焦点抢走，界面几乎无法操作。v0.1.0-alpha.2 及更早的 Windows 构建受此影响；macOS 没有这个问题，Linux 也不创建控制台窗口。

## 决定
- 新增 `internal/process`：Windows 下统一为子进程设置 `SysProcAttr.HideWindow` 与 `CREATE_NO_WINDOW`，其他平台为 no-op，OS 差异仍然集中在一处。
- `internal/gitcli` 的每条 Git 命令都经过该入口；Git 的 hooks、凭据助手等孙进程继承同一个隐藏控制台。
- `internal/platform` 里 `cmd /c start` 包装的外部打开与终端入口也隐藏包装进程的控制台；`start` 打开的目标窗口不受影响。
- 不改变 `OpenWith` 自定义命令的启动方式：它可能是需要可见控制台的编辑器或工具。

## 代价
- 子进程没有可见控制台，依赖控制台输出的排查只能看 Viff 的 Console 与日志。
- `CREATE_NO_WINDOW` 让子进程拿到一个没有窗口的控制台；依赖真实控制台句柄的第三方工具行为可能不同，因此目前只用于 Git 与 `cmd /c start` 包装。

## 复审条件
- 需要内置终端或可交互的控制台工具时，为该命令单独选择可见控制台，而不是整体撤销。
- Git、凭据助手或 hooks 在隐藏控制台下出现新问题时重新评估。
