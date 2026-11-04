# 0011 在 Log 里直接编辑历史

- 状态：已采用（2026-11）

## 背景
IntelliJ 的 Log 右键可以直接改提交信息、把提交 squash 进父提交、或删除提交，不必先打开完整的交互式 rebase 计划。Viff 已经有交互式 rebase，但只想改一句提交信息时，走整套计划对话框太重。

## 决定
- reword / squash / drop 都实现为一次范围很小的交互式 rebase：todo 由 Viff 生成（目标行改成 reword、squash 或 drop，其余 pick），提交信息写到文件并把 `core.editor` 指向它，任何编辑器都不会打开。
- 使用 `--autostash`：本地改动先 stash，结束后恢复。
- 使用 `--empty=keep`：只删目标提交；目标之后的提交如果因此变成空提交也保留，不会连带消失。
- 以下情况直接拒绝并说明原因，不尝试任何变通：
  - 仓库里有进行中的操作（merge、rebase、cherry-pick 等）；
  - 目标是根提交，没有可重放的父提交；
  - 目标是 merge 提交，或重放范围里含 merge 提交——普通 interactive rebase 会把这些 merge 拉平，属于静默改写；
  - 提交不在当前分支上。
- squash 从目标提交的父提交开始重放（父提交是根时用 `--root`），所以目标至少是第二个提交；父提交的作者和两边的改动都保留。
- 任何失败（包括冲突）都执行 `git rebase --abort`，之后再次检查操作确实结束；分支回到原位置，旧提交留在 reflog 里。

## 代价
- 带 merge 提交的长历史里，较早的提交不能这样编辑；要么先用交互式 rebase，要么用命令行。
- 每次编辑都会重写目标之后的所有提交：哈希和 committer 时间都会变。
- 如果 autostash 恢复时冲突，Git 会把改动留在 stash 里但命令仍算成功；界面按成功提示，用户需要自己再 stash pop。

## 复审条件
需要跨 merge 编辑时，改用 `--rebase-merges` 并重写含 label/reset 的 todo；或者换成 commit-tree/update-ref 的定向重写（只重建目标之后的提交）。
