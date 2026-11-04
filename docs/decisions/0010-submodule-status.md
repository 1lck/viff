# 0010 子模块的状态、更新与同步

- 状态：部分被 [0019](0019-git-issue-gaps.md) 替代（2026-11）；以下保留原决策背景。

## 背景
设计稿把子模块放在 P3，只要求“状态和基本更新”。此前 Viff 只在状态列表和 diff 里认出子模块，不能初始化、更新或同步；仓库里的 `.gitmodules` 改了 URL 也只能去终端处理。

## 决定
- 状态来自一次 `git submodule status --recursive`，不读 `.gitmodules` 也不拼 `git ls-files --stage`。首字符（`' '`/`-`/`+`/`U`）、SHA、路径和 describe 都按 Git 的输出解释；`core.quotePath=false` 让非 ASCII 路径保持可读。
- 路径可能含空格，describe 是行尾最后一个括号；解析只依赖这两个规则，不假设 SHA-1 长度（SHA-256 仓库同样可用）。
- 只提供三种写操作：Update（`git submodule update --recursive [--init] [-- <path>]`）、Sync（`git submodule sync --recursive [-- <path>]`）和把子模块作为项目打开。Update 默认带 `--init`：未初始化时这是唯一合理的动作，已初始化时没有副作用。
- 应用内不做 add、remove、deinit。它们要同时改 `.gitmodules`、索引和工作树，失败时容易留下半完成状态；先交给终端，等有真实需求再设计。
- 每次写操作后重新查询 Git；列表不做本地状态缓存。子模块指针 diff 的说明沿用现有 “Submodule” 提示，不另写一套。
- 界面有两个入口：Git 工具窗口选项菜单里的 “Submodules…” 对话框（Update / Sync / Open，含 Update All / Sync All），以及变更列表里子模块条目的右键菜单（Update Submodule / Open Submodule）。

## 代价
- `submodule status` 会为每个已初始化子模块跑一次 `git describe`；子模块很多时首次打开对话框有等待。
- 打开子模块会替换当前项目窗口，而不是新开窗口；Viff 目前是单仓库窗口。
- 本地路径子模块需要 Git 允许 file 传输（测试里用 `GIT_ALLOW_PROTOCOL=file`），生产界面不替用户放宽这个限制。

## 复审条件
需要添加、删除子模块，或需要在同一窗口并列查看父子仓库时，重新设计这部分，而不是继续往对话框里加按钮。
