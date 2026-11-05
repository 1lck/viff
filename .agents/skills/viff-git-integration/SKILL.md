---
name: viff-git-integration
description: "新增 Git 命令、解析机器输出、处理进程/取消/写操作与错误分类时加载；不负责 UI 呈现、日志脱敏与测试夹具搭建。"
metadata:
  version: "1.0.0"
---

# Viff Git 集成规范

## 何时使用

- 在 `internal/gitcli` 新增或修改 Git 命令、参数构造与输出解析。
- 处理写操作、取消、中断后的 continue/skip/abort 入口。
- 排查 Git 失败分类、Console 输出、能力差异（Git 版本、Windows）。
- 改 worktree、submodule、hooks、签名或凭据相关命令。

## 前置读取

1. `AGENTS.md` 的「代码规则」中 Git 执行、取消与串行写条款：全局约束只在那里定义。
2. `docs/decisions/0001-git-cli.md`（为什么用系统 Git、只读机器格式）、`0022-child-process-console.md`（子进程隐藏控制台）、`0010-submodule-status.md`、`0012-worktrees.md`（共享目录与路径注意事项）。
3. `docs/design.md` 中「官方 Git CLI 是首选执行引擎」：输出流、取消语义、串行写、preflight。
4. 代码入口：`internal/gitcli/runner.go`（执行、环境、错误分类）、`internal/gitcli/ops.go`（写命令与补丁）、`internal/gitcli/status.go`（porcelain v2 解析）、`internal/app/commands.go`（任务与取消）、`internal/app/conflict_resolve.go`（冲突重查）。

## 必须遵守

- 参数一律传数组，不经 shell：`exec.CommandContext(ctx, git, full...)`；commit 说明用 `--file=-` 从 stdin 读，路径用 `--pathspec-from-file=- --pathspec-file-nul`（`writePaths`），不拼命令字符串。
- 所有命令前置 `-c core.quotepath=false -c color.ui=false -c log.showSignature=false`（`runner.go` 的 `baseArgs`），避免用户配置改变输出；额外的 `-c` 只用于设置项（见签名/凭据条）或明确的输出控制（如子模块命令再带 `core.quotePath=false`）。
- 固定环境：`GIT_TERMINAL_PROMPT=0`（GUI 没有终端可提示）、`GIT_OPTIONAL_LOCKS=0`（只读查询不抢 index 锁）、`LC_ALL=C`、`LANGUAGE=C`；`Runner.Env` 追加在 `os.Environ()` 之后，`GIT_SSH_COMMAND` 再追加在最后以覆盖同名变量。
- 只用机器可读格式：`status --porcelain=v2 -z --branch --show-stash --untracked-files=all --ignore-submodules=dirty`、`log -z`、`diff --name-status -z`、`worktree list --porcelain -z`、`for-each-ref --format`、`ls-files -u -z`。
- NUL 分段后路径保持原始字节转成的字符串：不 unquote、不 re-escape、不改换行与特殊前缀；显示转义不能反过来改操作路径。
- 不假设对象 ID 长度：`domain.ShortHash` 只截 8 个字符用于展示，解析时按 Git 输出原样传回（SHA-256 仓库同样成立，见 0010）。
- stderr 必须持续消费且限量：`limitedBuffer` 上限 `stderrLimit`（64 KiB），`RunProgress` 边读边回调进度行；Console 单条输出上限 8 KiB，`git.fail` 事件 stderr 上限 2 KiB。失败信息取 stderr，空时退回 stdout 再退回 `Err`。
- 进程生命周期完整：`cmd.WaitDelay = 2*time.Second`；长任务走 `Stream` 且 `Close` 必须 `cancel` + `Wait`；每条 Git 命令都经 `process.HideConsoleWindow(cmd)`（Windows 隐藏控制台，见 0022），其他包不要自行起 Git 进程。
- 写操作串行且按 common dir 协调：`Workspace.Run` 对非 `Read` 任务取 `writeLock(repo)`，key 是 `repo.CommonDir`（为空退回 `GitDir`），所以同一仓库的 worktree 共享 refs 也串行。
- 写前重查，不用旧结果覆盖 Git：`ApplyBlock` 重读 old/new 两侧内容，不等就返回 `ErrStale`；`ApplyPatch` 先 `git apply --check` 再应用；`AddWorktreeExisting` 先检查分支存在、未被检出、路径未占用且从不 `--force`；冲突解析写前重读文件并确认该路径仍是未合并项，否则返回 `ErrStale`。
- 取消不等于撤销：`CancelTask` 停进程；`Workspace.Run` 在任务结束（含失败、取消）后若仓库未切换就 `Refresh(true)` 重查。操作中断后按 `rs.Operation.Kind` 给 continue/skip/abort 入口（`OperationAction`；并非每种操作都支持全部动作），不自动重试 commit/push。
- 避免交互式编辑器卡死：`--file=-`、`-c core.editor=true`（rebase 与 operation continue），不给 GUI 留终端编辑器。
- worktree/submodule 的路径按字面处理：批量路径写命令经 `writePaths` 前置 `--literal-pathspecs`，子模块命令也显式加该选项；子模块路径先经 `CleanSubmodulePath` 拒绝绝对路径、`..`、`.git` 组件；Windows 的 `submodule deinit` 改调 Git 自身 `submodule--helper`，不要为通过安全检查而加 `--force`；嵌套子模块拒绝操作并指向其所属仓库。
- 版本差异靠能力与格式，不要引入版本号比较分支代替能力判断：`Runner.Version` 只用于展示与诊断。Git 行为差异（如 Windows 子模块包装脚本丢 `--literal-pathspecs`）在 `internal/gitcli` 内按平台或探测结果分支处理并写注释。
- hooks、签名、凭据交给系统 Git：用 `-c gpg.program`、`-c user.signingkey`、`-c gpg.format`、`-c credential.helper` 与 `GIT_SSH_COMMAND` 表达 Viff 设置，不改仓库或全局配置（0016）；不静默禁 hooks 或签名，`--no-verify` 只作为显式选项（`CommitOptions.NoVerify`）。
- 错误分类统一在 `internal/gitcli/runner.go`：14 类 `ErrorKind` + 稳定 `Code()`（如 `push.rejected`、`repo.locked`）；新增失败模式先扩充 `classify` 与 `Code`，不要在 UI 里匹配字符串。Console 只在命令失败或非静默命令（写操作与普通读）时追加一条；成功读由 `quietReads` 静音，慢读只额外写事件日志。

## 步骤

1. 先确认 Git 是否已有机器可读输出可用；优先 `-z` 与 `--porcelain`，不要解析人读文本。
2. 在 `internal/gitcli` 加方法：参数数组、`r.Run`/`r.RunInput`/`r.RunProgress`、必要时 `r.write`；解析函数独立（如 `ParseStatus`）便于单测。
3. 写操作接入 `app`：`Workspace.Run(title, op, RunOptions{...})`，成功后由 `Run` 统一 `Refresh(true)`；只读查询留在各自的后台刷新里，不要为图省事塞进 `Run`。
4. 需要用户确认、continue/abort 或 stale 重选时返回稳定错误（`ErrStale`、`ErrSubmoduleNeedsForce`），由 UI 给动作，不自动重试。
5. 用真实临时仓库写测试（见 `viff-testing`），覆盖失败路径与中断路径；本机无 Go 就说明未运行。
6. 行为变化同步文档或 ADR（如新增操作入口、改变失败分类）。

## 检查清单

- [ ] 没有 shell 拼接，参数都是数组，含空格的路径走 stdin 或 NUL 列表。
- [ ] 命令带机器可读格式与 `baseArgs`；固定环境变量齐全。
- [ ] stderr 被持续消费且限量；失败信息保留 Git 原文。
- [ ] 每条命令都经 `process.HideConsoleWindow`。
- [ ] 写操作走写锁；写前重查 HEAD/index/内容，变化返回 stale 类错误。
- [ ] 取消后重查状态并暴露 continue/skip/abort，无自动重试。
- [ ] 对象 ID 未写死 40 位；NUL 路径未做二次转义。
- [ ] worktree/submodule 路径按字面处理，未用 `--force` 绕过安全检查。
- [ ] 未修改用户 Git 配置，未静默禁用 hooks/签名。
- [ ] 失败分类沿用 `ErrorKind`，用户可见错误有 code。

## 常见错误

- 用 `exec.Command("sh", "-c", ...)` 或 `strings.Join` 拼路径：空格、换行、Unicode、`[` 等字符立刻出错。
- 加 `--force` 让被安全检查拦下的操作「成功」，例如子模块 deinit 或 worktree 删除。
- 把 stderr 直接 `cmd.Stderr = nil` 或只读一次，长任务在管道写满时挂住。
- 只读刷新也取写锁，或反过来写操作不取锁，导致 index 并发损坏。
- 取消进程后直接重建视图而不查状态，用户看到的与实际 HEAD/index 不一致。
- 用 `git status --porcelain`（v1）或 `--format` 文本解析路径，遇到重命名/非 ASCII 就错。
- 假设 SHA-1 40 位截断，SHA-256 仓库上标题被截错。
- 为通过测试在代码里硬编码 Git 版本判断，而真正差异是 Windows 包装脚本或配置。

## 相关文件

- `internal/gitcli/runner.go`、`internal/gitcli/status.go`、`internal/gitcli/ops.go`、`internal/gitcli/history.go`、`internal/gitcli/submodule.go`
- `internal/app/commands.go`、`internal/app/repo.go`、`internal/app/conflict_resolve.go`、`internal/app/smartcheckout.go`
- `internal/process/`、`internal/domain/domain.go`
- `docs/decisions/0001-git-cli.md`、`docs/decisions/0010-submodule-status.md`、`docs/decisions/0012-worktrees.md`、`docs/decisions/0016-git-settings.md`、`docs/decisions/0022-child-process-console.md`
