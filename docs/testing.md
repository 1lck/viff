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

The Subversion adapter's tests need an `svn` client on `PATH`, plus `svnadmin` for
the `file://` fixtures they create; `VIFF_SVN` and `VIFF_SVNADMIN` override the
lookup, which is what a Windows install outside `PATH` needs. Without a client
they skip, so a machine without Subversion proves nothing about that adapter: the
Linux CI shard installs `subversion` and fails if the package skips.

Subversion 适配器的测试需要 `PATH` 上有 `svn`，以及为 `file://` 仓库服务的 `svnadmin`；
`VIFF_SVN` 与 `VIFF_SVNADMIN` 可覆盖查找结果（Windows 装在 `PATH` 之外时用它们）。
没有 svn 客户端时这些用例会跳过，所以只跑跳过等于没有验证这一层：CI 的 Linux 分片
会安装 `subversion`，并在该包出现跳过时报失败。

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

### Coalesced refresh scope / 合并刷新的范围

`go test -count=1 -run '^TestRefresh' ./internal/app` covers fork issue #1.
Channel-controlled status queries verify that overlapping ordinary requests
produce one ordinary follow-up and reuse the freshly published LFS snapshot.
Metadata requests are OR-ed into the pending scope; the active pass's scope
does not carry forward. Each pass consumes the pending state before starting,
so requests during that pass can form one further pending refresh. Failure,
cancellation, repository removal, switching, and close/reopen scenarios check
that pending requests and late results stay within their repository lifetime.
Lifetime assertions use the UI-thread query sequence and busy state, so a stale
callback cannot hide a newly started refresh behind an unscheduled goroutine.
Queued metadata also invalidates the loaded log before the follow-up queries:
local shallow-clone tests fetch with `--unshallow` and `--deepen=1`, keeping
HEAD and refs unchanged, and check that the fetched parent appears in the log.
The existing LFS attribute fingerprint and 30-second recheck rules still apply.

`go test -count=1 -run '^TestRefresh' ./internal/app` 覆盖 fork issue #1。
测试通过 channel 控制状态查询，确认重叠的普通请求只产生一次普通补刷，并复用刚发布的
LFS 快照。待处理范围按元数据请求取 OR，不继承在途刷新的范围；每轮开始前消费待处理
状态，该轮期间到达的新请求仍可合并为下一次补刷。失败、取消、仓库消失、切换及关闭后
重开场景检查待处理请求和迟到结果始终属于原仓库生命周期。生命周期断言检查 UI 线程
同步维护的查询序号和刷新状态，避免新刷新的 goroutine 尚未调度而漏报。排队的元数据
请求还会在补刷查询前使已加载日志失效：本地浅克隆测试分别执行 `--unshallow` 和
`--deepen=1`，保持 HEAD、refs 不变，检查新取得的父提交出现在日志中。
LFS 原有的属性指纹和 30 秒重查规则继续生效。

The November 11, 2026 comparison used Windows 11 Pro x64, Go 1.27.3,
Git 2.45.1.windows.1, and Git LFS 3.5.1. Both versions opened the same
isolated fixture shape: 4,096 committed LFS pointers, a status result held for
31 seconds, and ten ordinary requests delivered at 30 seconds. A temporary
`go test -overlay` harness ran `TestRefreshSlowAcceptance` against `cc76616`
and the fix. Required and optional services share one metered runner; the
harness asserts two stash and two worktree queries in each run. A Windows Job
Object around each Git command in the measured open/refresh workload accounted
for its entire process tree, including git-lfs and its Git children; CPU time
excludes fixture setup and the artificial wait. Each version was measured once.

2026 年 11 月 11 日的对照环境为 Windows 11 Pro x64、Go 1.27.3、
Git 2.45.1.windows.1、Git LFS 3.5.1。两版使用同样的隔离夹具结构：
4,096 个已提交的 LFS 指针、延迟 31 秒返回的状态结果，以及第 30 秒到达的十个普通请求。
临时 `go test -overlay` 夹具通过 `TestRefreshSlowAcceptance` 分别运行 `cc76616`
和修复版。必备接口和可选能力共用一个计量 Runner，夹具断言每版各包含两条 stash、
两条 worktree 查询。被测打开与刷新流程的每条 Git 命令置于 Windows Job Object 内，
统计完整进程树，含 git-lfs 及其 Git 子进程；CPU 时间不含夹具搭建和人为等待。
每版各测一次。

| Metric / 指标 | Before / 修复前 | After / 修复后 |
| --- | --- | --- |
| Refresh scopes / 刷新范围 | `true, true` | `true, false` |
| Status queries / 状态查询 | 2 | 2 |
| Git commands / Git 命令 | 24 | 21 |
| Stash queries / stash 查询 | 2 | 2 |
| Worktree queries / worktree 查询 | 2 | 2 |
| LFS commands / LFS 命令 | 5 | 3 |
| LFS listing attempts / LFS 枚举命令 | 4 | 2 |
| Git process-tree CPU / Git 进程树 CPU | 18,734 ms | 11,203 ms |
| LFS process-tree CPU (included above) / LFS 进程树 CPU（已含在上项） | 14,484 ms | 6,938 ms |
| Follow-up duration / 补刷耗时 | 15,580 ms | 2,778 ms |

The old version started a second metadata refresh about 34.0 seconds after
the first; the fixed version completed one metadata pass followed by an
ordinary pass. LFS 3.5.1 rejects `ls-files -z`, so each enumeration includes
the existing newline-format fallback: two listing commands, not two scans.
The fixture measures coalescing; actual checkout size, cache state, and disk
load determine timings in a real repository.

旧版在首轮开始约 34.0 秒后启动第二次元数据刷新；修复版完成一次元数据刷新后按普通
范围补刷。LFS 3.5.1 拒绝 `ls-files -z`，每次枚举包含已有的换行格式回退，因此会有
两条枚举命令。此夹具验证合并行为；实际仓库耗时取决于规模、缓存和磁盘负载。

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

## Cross-platform CI and packaging / 跨平台 CI 与安装包

CI runs the Git and Viva headless UI suites on macOS, Windows Server 2022, and
Ubuntu. It also checks native packages: macOS DMG application bundles, Windows
NSIS installers and portable ZIPs, and Linux DEB and portable archives. These checks do not launch
application windows. Review the workflow results for the exact commit before
claiming a release has passed.

CI 在 macOS、Windows Server 2022 和 Ubuntu 运行 Git 与 Viva 无头 UI 测试，
并检查 macOS DMG 应用包、Windows NSIS 安装器与便携 ZIP、Linux DEB 和便携包。
这些检查不打开应用窗口。声明某个版本通过验收前，须检查对应提交的工作流结果。

Ubuntu headless results verify application behavior in the CI renderer. They do
not establish compatibility with every Linux distribution, display server,
graphics driver, input method, or desktop environment. Package validation and
interactive desktop acceptance are separate checks.

Ubuntu 无头结果验证的是 CI 渲染环境下的应用行为，不代表覆盖所有 Linux 发行版、
显示服务器、显卡驱动、输入法或桌面环境。安装包验证与交互式桌面验收分别记录。

## What remains unverified / 尚未验证

The updater has hermetic unit tests (release selection, download, checksum, helper scripts) and headless dialog tests, but the actual file replacement and relaunch on each desktop platform is not exercised in CI.

更新器有隔离的单测（选版、下载、校验、helper 脚本）和无头对话框测试，但各桌面平台的
真实文件替换与重启不在 CI 中执行。

- Windows 10/11 desktop input, IME, and native window interaction / Windows 10/11 桌面输入、输入法及原生窗口交互。
- Signed installers and notarization; real in-app update replacement and restart / 签名安装器与公证；应用内更新的真实替换与重启。
- Public SSH/HTTPS credential prompts and external editor integrations / 公网 SSH/HTTPS 凭据弹窗及外部编辑器集成。
- Linux desktop behavior / Linux 桌面行为。
- Comparative memory or startup benchmarks / 内存和启动速度对比基准。

Windows headless screenshots do not paint every native text-edit overlay;
tests also assert field values and application state. Windows submodule deinit
uses Git's own helper to preserve literal path handling; other Git versions
still need broader compatibility testing.

Windows 无头截图不绘制所有原生文本编辑覆盖层，因此测试也检查字段值和应用状态。
Windows 子模块 deinit 使用 Git 自身 helper 保留字面路径语义；其它 Git 版本仍需更多兼容性测试。
