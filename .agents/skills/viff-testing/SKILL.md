---
name: viff-testing
description: "写或改 Go 测试、修 flaky、跑无头 UI 套件与准备 CI 证据时加载；不负责生产代码实现、CI 工作流编辑与截图视觉基准设计。"
metadata:
  version: "1.0.0"
---

# Viff 测试规范

## 何时使用

- 新增或修改 `*_test.go`，修 bug 前要写一个能失败的测试。
- 跑 `go test`、无头 UI 套件、生成截图或排查 flaky。
- 准备 PR 里要写明的命令、平台与 CI 证据。
- 加需要临时 Git 仓库、裸远端或隔离配置的测试。

## 前置读取

1. `AGENTS.md` 的「测试规则」：全局约束只在那里定义，本 skill 只给流程与本领域细节。
2. `docs/testing.md`：复现命令、截图开关、发布前验收记录、CI 与平台说明。
3. 现有夹具，优先复制而不是新造：`internal/gitcli/gitcli_test.go`（`newTestRepo`、`isolatedEnv`）、`internal/app/app_test.go`（`newFixture`、`settle`）、`internal/ui/shots_test.go`（`uiFixture`、`settle`、`settleTab`、`assertIdle`、`snap`）。
4. `.github/workflows/ci.yml` 与 `.github/actions/go-test-shard/action.yml`：确认超时、标签、分片与证据路径后再声称「CI 会跑」。

## 必须遵守

- 临时仓库用 `t.TempDir()`，并先 `filepath.EvalSymlinks`（macOS `/var` 会解析成 `/private/var`，`newTestRepo` 与 `newFixture` 都这么做）。
- 隔离配置靠显式环境数组，不靠改进程全局：`HOME`、`XDG_CONFIG_HOME`、`GIT_CONFIG_NOSYSTEM=1`、`GIT_CONFIG_GLOBAL=<home>/gitconfig`（需要系统级隔离时用 `GIT_CONFIG_SYSTEM` 指向临时文件）；Git 子进程再拼 `GIT_EDITOR=true`，并在仓库里固定 `user.name`、`user.email`、`commit.gpgsign=false`。仓库测试不使用 `t.Setenv`。
- 远端只在本机：`git init -q --bare -b main <tmp>/remote.git`，测试不访问公网、不依赖登录态或凭据。
- 标签用途固定：`//go:build vivahead` 启用 Viva 离屏 UI 测试；`internal/ui/lithe_issues_test.go` 用 `//go:build vivahead && litheaudit` 隔离外部 issue 回归；平台差异用文件名后缀（如 `runner_windows_test.go` 的 `//go:build windows`）。`internal/gitcli/lithe_issues_test.go`、`internal/app/lithe_issues_test.go` 没有标签、始终运行，不要误以为它们 opt-in。
- 命令与仓库/CI 一致：`go test -count=1 ./...`；`go test -count=1 -tags 'vivahead litheaudit' ./internal/ui`；`go vet -tags 'vivahead litheaudit' ./...` 覆盖测试代码。
- 截图只在显式设置时产生：`f.shot = os.Getenv("VIFF_SHOTS")`，为空就完全不写盘；`VIFF_SHOTS_BY_TEST=1` 时按 `t.Name()` 建子目录，并在 cleanup 里追加 `99-final-state.png`。默认跑测试不打开窗口。
- 修 bug 先写失败测试：断言先红后绿；外部 issue 回归在注释里写编号与期望行为（如 `// Lithe #1445:`）。
- 平台差异按 `runtime.GOOS` 改前置与断言：Windows 用 `core.symlinks`/`core.filemode`、`update-index --chmod=+x` 代替 chmod；大小写不敏感用 `strings.EqualFold`；能力不成立时 `t.Skipf` 并写原因（symlink 创建失败、Git 缺失、原生 watcher 不可用）。
- 断言看状态而不是像素：写操作后用 `f.git(...)` 重查（`diff --cached`、`rev-parse`、`log -1 --format=%s`、`worktree list --porcelain`）并断言 `ws.Repo` 字段；截图只作为补充证据。
- 等待用条件而不是长 sleep：app 层用 `settle(cond)` 反复 `Drain`，UI 层用 `s.WaitFor(cond, timeout)` + `settle()`；空闲回归用 `assertIdle()` 检查重绘次数。
- flaky 先定位再修：常见原因是点击同步、后台任务未 settle、任务结束与刷新竞争；改夹具或等待条件，不靠加大 sleep 或 `-count` 掩盖。重复验证同一用例多轮（如 `docs/testing.md` 记录的 5 轮）再声称稳定。
- 时间预算：每个分片 `go test -timeout=10m`（`.github/actions/go-test-shard/action.yml` 默认值；macOS 单作业并发跑无头 UI 用 15m）；顶层测试按索引分片，Windows core 2 片、UI 3 片，Ubuntu core 1 片、UI 2 片，新增慢测试要留余量。
- 证据可上传：CI 把 `VIFF_SHOTS` 指向 `.viff-test/ci/screenshots` 并设 `VIFF_SHOTS_BY_TEST=1`，JSON 流写 `.viff-test/ci/logs/*.jsonl`，上传 `.viff-test/ci/` 保留 14 天；本地临时产物也只放 `.viff-test/`（已被忽略）。
- 回归测试覆盖同类边界，不只复现单个输入：修并发/平台/路径/取消类缺陷时同时覆盖相邻场景（大小写、空列表、冲突块、中断后状态）。

## 步骤

1. 确定测试层：Git 解析与命令进 `internal/gitcli`，用例与调度进 `internal/app`，界面行为进 `internal/ui`。
2. 复制同层夹具：Git 仓库（`newTestRepo`/`newFixture`）或 UI（`newUIFixture` + `start`），不另起一套环境拼装。
3. 搭数据：临时仓库、裸远端、需要的分支/提交/冲突；不要提交大仓库或用户数据。
4. 先写断言（修 bug 时断言应失败），再驱动到断言；UI 用 `ClickLabel`/`Type`/`Keys`/`WaitFor`，app 用 `Drain`。
5. 本机有 Go 才执行命令并记录结果；没有就写「未运行 + 原因」。
6. 需要截图时显式设 `VIFF_SHOTS`，精选图才进 `docs/screenshots/`。
7. PR 写命令、OS、`git --version`、结果；CI 失败附 `.viff-test/ci/` 里的日志或截图。

## 检查清单

- [ ] 测试只写 `t.TempDir()`，没有触碰真实仓库、设置、日志或凭据。
- [ ] Git 环境已隔离：`HOME`、`XDG_CONFIG_HOME`、`GIT_CONFIG_NOSYSTEM`、`GIT_CONFIG_GLOBAL`。
- [ ] 远端是本地裸仓库，无网络与登录依赖。
- [ ] UI 测试带 `vivahead`；外部 issue 回归才用 `litheaudit`。
- [ ] 等待条件是 `WaitFor`/`settle`，没有长的固定 sleep。
- [ ] 断言同时覆盖 Git 状态与应用状态，截图不是唯一依据。
- [ ] 平台相关部分有 `runtime.GOOS` 分支或带原因的 `t.Skip`。
- [ ] 新增慢测试仍能在 10 分钟分片内完成。
- [ ] 本机未跑的命令已注明，没有编造输出。

- [ ] 回归测试覆盖同类边界，不是只让单个用例通过。

## 常见错误

- 直接 `t.Setenv("HOME", ...)` 或依赖 `os.Environ` 里的真实 Git 配置，导致本机全局配置污染结果。
- 忘记 `EvalSymlinks`，在 macOS 上路径比较失败。
- 在测试里创建真实窗口或启动 `cmd/viff`，或依赖图形环境；应走 Viva 离屏后端。
- 把 `litheaudit` 当成普通回归的开关，把日常测试塞进 opt-in 分组。
- 用 `time.Sleep(2 * time.Second)` 等提交/刷新完成，既慢又不稳。
- 只看截图断言成功，不查 `git status`/`Rev`/字段值，Windows 无头截图缺覆盖层时尤其致命。
- 修 bug 时先改代码再补测试，事后无法证明测试能抓住该缺陷。
- 在测试里创建真实远端（GitHub 等）或提交凭据。

## 相关文件

- `AGENTS.md`、`docs/testing.md`
- `.github/workflows/ci.yml`、`.github/actions/go-test-shard/action.yml`
- `internal/gitcli/gitcli_test.go`、`internal/gitcli/lithe_issues_test.go`
- `internal/app/app_test.go`、`internal/app/scenarios_test.go`
- `internal/ui/shots_test.go`、`internal/ui/lithe_issues_test.go`
- `docs/screenshots/`、`.viff-test/`（忽略）
