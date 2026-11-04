# 0015 更新未检出的分支与反浅化

- 状态：已采用（2026-11）

## 背景
分支弹窗只能更新当前分支（走 Pull），别的本地分支落后远端时，得先检出再 pull，工作区还会被迫切换。浅克隆同样没有入口补齐历史：用户只看到历史截断，不知道可以在应用里 `git fetch --unshallow`。IntelliJ 的 Update 对未检出的分支直接做 fast-forward，并在 Git 工具窗口提供 Fetch Full History。

## 决定
- 更新未检出的分支时，先把远端同名分支抓进它的 remote-tracking ref（`+refs/heads/<branch>:refs/remotes/<remote>/<branch>`），再用 `merge-base --is-ancestor` 判断远端 tip 是否为本地分支的后代。只有是才用 `update-ref` 带旧值 CAS 移动本地 ref；否则一个字节都不写，报告“分叉”，让用户去 worktree 里 merge/rebase。
- 这样不需要检出、不会留下半个状态，也不会覆盖并发改动：`update-ref` 的旧值不匹配就直接失败，`fetch` 只动 remote-tracking ref。
- 已检出的分支拒绝更新，并说明它在哪个 worktree。一个分支只能在一个 worktree 检出，移动 ref 会让那个 checkout 的索引落后于 HEAD，静默破坏它的状态。
- 通知说明前进的提交数（`rev-list --count old..new`），没有新提交时显示“已是最新”，而不是笼统的成功。
- 浅克隆判定用 `git rev-parse --is-shallow-repository`，随状态、引用、工作树一起在后台刷新时读取，存进 `RepoState.Shallow`。Git 窗口 ⋯ 菜单只在浅克隆时显示 “Fetch Full History (Unshallow)”，执行 `git fetch --unshallow`，进度走统一的任务条。
- 在完整仓库上调用反浅化时给 info 提示（“历史已经是完整的”），不当作错误。
- 更新来源优先用分支配置的上游 remote，没有上游时用默认 remote；两者都没有时菜单项不出现。

## 代价
- 每次仓库刷新多一次 `rev-parse --is-shallow-repository`。
- 未检出分支的更新只做 fast-forward；分叉必须到 worktree 里处理。
- `--unshallow` 会把远端全部历史下载下来，大仓库耗时且流量大，界面只能等它完成。

## 复审条件
需要支持非 fast-forward 的未检出分支更新（自动 merge/rebase）、或需要按 remote 批量更新多个分支时，重新设计这几条，而不是往菜单里再加一项。
