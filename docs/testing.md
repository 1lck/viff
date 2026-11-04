# Testing and platform notes / 测试与平台说明

## Reproduce the checks / 复现检查

Use the Go version in `go.mod`, a working Git executable, and the platform's
native build prerequisites. Run from the repository root:

使用 `go.mod` 指定的 Go 版本、本机 Git 和平台构建依赖，在仓库根目录执行：

```sh
go test -count=1 ./...
go vet -tags "vivahead litheaudit" ./...
go test -count=1 -tags "vivahead litheaudit" ./internal/ui
```

`vivahead` enables Viva's offscreen UI tests. `litheaudit` includes the
external-issue regression scenarios. Neither command launches the app window.
The renderer still needs its platform rendering backend. Git fixtures use
temporary repositories and isolated configuration, including local temporary
remotes; they do not authenticate to public Git hosting.

`vivahead` 启用 Viva 离屏 UI 测试，`litheaudit` 加入外部 issue 回归场景。
不会启动应用窗口，但仍需要平台渲染后端。测试使用临时仓库、隔离配置和本地临时远端，
不测试公网 Git 托管服务的登录过程。

For screenshots on macOS / macOS 保存截图：

```sh
VIFF_SHOTS="$PWD/.viff-test/shots" VIFF_SHOTS_BY_TEST=1 \
  go test -count=1 -tags "vivahead litheaudit" ./internal/ui
```

On Windows PowerShell / Windows PowerShell：

```powershell
$env:VIFF_SHOTS = "$PWD/.viff-test/shots"
$env:VIFF_SHOTS_BY_TEST = "1"
go test -count=1 -tags "vivahead litheaudit" ./internal/ui
```

## Pre-publication acceptance / 发布前验收

The November 4, 2026 acceptance used macOS development builds and native Windows
test executables running on Windows Server 2022 Datacenter x64, Git
2.56.0.windows.4, and Microsoft's Basic Display Adapter. The latest result for
each of 298 unique top-level tests passed: domain 25, storage 13, gitcli 83,
app 95, platform 2, UI 80. The Windows run produced 194 final PNGs.

2026 年 11 月 4 日验收使用 macOS 开发构建，以及运行在 Windows Server 2022 Datacenter
x64 上的原生 Windows 测试程序（Git 2.56.0.windows.4、Microsoft Basic Display Adapter）。
298 个唯一顶层测试的最终结果全部通过：domain 25、storage 13、gitcli 83、app 95、
platform 2、UI 80；生成 194 张最终 Windows PNG。

This was not a single all-green first run: 27 initial failures led to fixes for
Windows submodule literal paths, monospace font fallback, and Unix-specific test
assumptions. Full package reruns followed; two UI click synchronization cases
were then verified five times each. Repetitions are not counted as extra tests.
Screenshots in this repository are selected demo images; CI uploads its own
current logs and screenshots as artifacts for 14 days.

首次运行并非全绿：最初的 27 项失败促成 Windows 子模块字面路径、等宽字体回退和 Unix
测试假设的修复。之后完整复跑相关包，再将两个 UI 点击同步用例各验证五轮。重复运行不增加
测试总数。仓库只保留精选演示截图；CI 将当次日志和截图保存为附件，保留 14 天。

## What remains unverified / 尚未验证

- Windows 10/11 desktop input, IME, and native window interaction / Windows 10/11 桌面输入、输入法及原生窗口交互。
- Signed installers, notarization, and auto-update / 签名安装器、公证和自动更新。
- Public SSH/HTTPS credential prompts and external editor integrations / 公网 SSH/HTTPS 凭据弹窗及外部编辑器集成。
- Linux desktop behavior / Linux 桌面行为。
- Comparative memory or startup benchmarks / 内存和启动速度对比基准。

Windows headless screenshots do not paint every native text-edit overlay;
tests also assert field values and application state. Windows submodule deinit
uses Git's own helper to preserve literal path handling; other Git versions
still need broader compatibility testing.

Windows 无头截图不绘制所有原生文本编辑覆盖层，因此测试也检查字段值和应用状态。
Windows 子模块 deinit 使用 Git 自身 helper 保留字面路径语义；其它 Git 版本仍需更多兼容性测试。
