# 0014 推送变体：推到某个提交、别的远端、删除远端标签

- 状态：已采用（2026-11）

## 背景
普通 Push 只处理一个场景：当前分支 → 它的上游（或对话框里改过的远端分支名）。IntelliJ 还有三种日常动作 Viff 没有：把一个历史提交及其之前的提交推到远端（不带后续提交）、把 HEAD 推到另一个远端或另一个分支名、删除远端标签（原来只能删本地）。它们都不改变本地分支，也不该顺手 force。

## 决定
- 三种动作共用一个底层原语 `PushRef(remote, src, branch)`，执行 `git push --progress [--force-with-lease] <remote> <src>:refs/heads/<branch>`。`src` 可以是提交哈希或 `HEAD`，因此“推到某个提交”和“HEAD 推到别的名字”不是两套实现。
- 目标分支名由用户输入，用现有的 ref 名校验，默认当前分支名；`--set-upstream` 不参与，因为这些动作不改变本地分支的跟踪关系。
- 非 fast-forward 默认交给 Git 拒绝，界面保留 Git 原文并给出“先更新再推”的提示；强制推送必须显式勾选 force-with-lease，与普通 Push 对话框一致。不使用 `--force`。
- 删除远端标签执行 `git push <remote> :refs/tags/<tag>`，只动远端；本地标签要删仍走原来的 “Delete Tag”。确认文案明确说明本地标签保留。
- 入口：Log 提交右键 “Push Up to Commit…”（仅单选、非 merge 提交，因为 merge 提交的“之前”不唯一）；分支弹窗当前分支 “Push to Another Remote…”；标签子菜单 “Delete Remote Tag…”。新代码集中在 `internal/ui/pushops.go`，不改 `ids.go`。

## 代价
- 推到某个提交只发送该提交可达的历史，界面只有一个提交卡片，不预先列出远端缺哪些提交；数量需要用户自己判断。
- `--force-with-lease` 依赖本地 remote-tracking ref，没 fetch 时会以 “stale info” 被拒；这是有意的保护，但用户需要先 fetch。
- 删除远端不存在的标签时，当前 Git 只打印 `warning: deleting a non-existent ref` 并返回 0，界面会按成功处理；不做额外探测。
- 每次动作都走 `Run`：串行、写锁、完成后刷新状态，因此推送期间其他写操作会等待。

## 复审条件
需要“推送指定区间（A..B）”或“推送时同时设置上游”时，再扩展对话框；如果 Git 改变删除不存在远端标签的退出码，重新评估成功/失败的判定。
