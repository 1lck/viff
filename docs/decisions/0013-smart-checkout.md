# 0013 Smart Checkout：先 stash 再检出

- 状态：已采用（2026-11）

## 背景
工作区有未提交改动时，`git switch` 会拒绝切换：改动会被目标分支覆盖，或者未跟踪文件会被覆盖。之前 Viff 只把这类错误显示成“先提交、暂存或回滚”，用户必须自己 stash、切分支、再 pop。IntelliJ 把这三步合并成一次确认，这里采用同样的流程。

## 决定
- 不在检出前主动 stash。先执行普通检出，只有 Git 以“本地改动会被覆盖”或“未跟踪文件会被覆盖”拒绝时，才把错误交给 UI；其它失败（例如分支已被别的 worktree 检出）保持原来的错误提示，不提供 stash。
- 是否 stash 由用户确认，绝不自作主张。确认框写明会连同未跟踪文件一起 stash，并说明恢复冲突时改动会留在 stash。
- 编排放在 app 层（`Workspace.SmartCheckout`）：`git stash push --include-untracked -m "viff: checkout <ref>"` → 检出 → `git stash pop`。UI 只负责确认框，Git 层的三步没有拆到界面里。
- 检出失败必须把改动放回工作区：pop 之后再报告原始检出错误，当前分支保持不变。pop 失败时改动留在 stash 里，通知明确说“改动仍在 stash”，用户可以用 Stash 页签取回。
- `git stash pop` 在冲突时不会删除 stash 条目，正好满足“冲突时改动必须留在 stash”；因此不需要先 `apply` 再手动 `drop`。
- 分支检出和分离 HEAD 的修订检出（`git switch --detach`）共用同一流程与确认框。

## 代价
- 需要为 Workspace 增加一个 UI 回调（`ConfirmCheckout`）；没有 UI 的环境（测试、无界面调用）不会提供 stash，只报原始错误。
- stash 期间若用户取消或进程崩溃，改动留在 stash 里，需要用户自己 pop；消息里带 `viff: checkout <ref>` 便于识别。
- 冲突恢复后工作区带冲突标记，仍需按既有冲突流程处理（ADR [0051](0051-editable-conflict-result.md)；SVN 仍只读，见 0005）。
- 检出后 pop 的改动不再保留暂存区状态：`git stash pop` 默认把改动放回工作区，暂存的文件会变成未暂存。需要保留索引时要改用 `pop --index`，但那会在更多情况下制造冲突，暂不采用。

## 复审条件
需要保留暂存区状态、或需要“把当前改动直接提交到目标分支”这类更复杂的迁移时，再扩展。若 Git 增加原生的“检出时自动带改动”能力，优先改用它而不是继续维护这套三步流程。
