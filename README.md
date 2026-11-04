# Viff

用 Go 和 [Viva](https://github.com/vivaui/viva) 编写的 Git 桌面客户端。界面与交互参照最新版 IntelliJ IDEA 的新 UI（Islands 主题），目标是低内存，并保证 Git 状态正确。

设计文档见 [docs/design.md](docs/design.md)，架构决策见 [docs/decisions](docs/decisions/README.md)。

## 功能

- **欢迎页**：最近项目（支持搜索和移除，不存在的路径标红）、打开、克隆、新建仓库，也可以把文件夹拖进窗口。
- **Commit 工具窗口**
  - 分组：Merge Conflicts / Staged / Changes / Unversioned Files，可按目录分组。
  - 暂存：悬停行上的 +/−，空格键，多选（Shift、⌘）后批量暂存，回滚，加入 .gitignore。
  - 提交：Amend，Sign-off，跳过钩子，⌘↩ 提交，“Commit and Push…”。
  - 正在进行 merge、rebase 等操作时显示横幅，带 Continue / Abort。
- **Stash 页签**：暂存（可包含未跟踪文件、保留索引）、应用、Pop、删除、转为分支，并可查看 stash 中的文件 diff。
- **Diff**
  - 并排与统一两种视图，同步滚动，曲线连接相同块，词级高亮，按语言语法着色。
  - 部分暂存：块按钮（→ ← + ⟲）可暂存、取消暂存或回滚单个块，也可以在行号栏选中几行后只暂存这些行。
  - F7 / ⇧F7 在差异之间跳转，⌘F 搜索，可忽略空白差异，F4 用外部编辑器打开。
  - 预览页签跟随选中的文件；双击或 ⌘D 固定页签。
- **冲突查看器**：Ours / Result / Theirs 三栏，在冲突块间跳转，在编辑器中打开，确认无冲突标记后才能标记为已解决。
- **Git 工具窗口**
  - Log：分支树；提交图；引用标签；按文本或哈希、分支、作者、日期、路径过滤；懒加载。
  - 提交详情：文件列表和完整提交信息。
  - 提交右键菜单：cherry-pick、revert、reset（Soft/Mixed/Hard/Keep）、撤销提交、检出、新建分支或标签、与工作树比较。
  - Console 显示所有执行过的 Git 命令及其输出；Reflog 页签可以查看 reflog。
- **分支弹窗**（⌥⇧`）：速搜、收藏、本地和远程分支的子菜单，可执行检出、新建、重命名、合并、rebase、比较、设置上游、推送、删除。
- **推送和更新**：推送对话框列出待推送的提交，可选推送标签和 force-with-lease；更新支持 merge 或 rebase 方式；也支持 Fetch、后台定时 Fetch 和管理远程仓库。
- **通知**：右下角气泡（成功消息 10 秒后自动隐藏），以及通知工具窗口。
- **主题**：Islands Dark、Islands Light，或跟随系统。

## 构建与运行

需要 Go 1.27 或更高版本，以及本机安装的 `git`。在 macOS 上编译需要 Xcode Command Line Tools，因为字体注册和 FSEvents 用到了 cgo。

```sh
go run ./cmd/viff                 # 打开上次的项目，没有则显示欢迎页
go run ./cmd/viff ~/code/project  # 打开指定仓库
go build -o viff ./cmd/viff
```

设置保存在用户配置目录的 `Viff/settings.json`（macOS 上为 `~/Library/Application Support/Viff/settings.json`） 中，也可以通过设置对话框（⌘,）查看路径。

## 常用快捷键

| 快捷键 | 动作 |
| --- | --- |
| ⌘K / ⇧⌘K / ⌘T | 提交 / 推送 / 更新项目 |
| ⌥⇧` | 分支弹窗 |
| ⌘D | 显示选中项的 diff |
| F7 / ⇧F7 | 下一个 / 上一个差异 |
| ⌘F | 在 diff 中搜索，或聚焦日志搜索 |
| ⌘0 / ⌘9 | 显示或隐藏 Commit / Git 工具窗口 |
| ⌥⌘Z | 回滚 |
| ⌘W、⇧⌘[ / ⇧⌘] | 关闭或切换编辑器页签 |
| ⌘R | 刷新 |

## 测试

```sh
go test ./...                                            # 领域、Git 解析与命令、工作区、监听器
go test -tags vivahead ./internal/ui/                    # 无界面 UI 测试：提交流程、日志搜索、stash、冲突等
VIFF_SHOTS=/some/dir go test -tags vivahead ./internal/ui/  # 同时把截图写到指定目录
```

所有测试都在 `t.TempDir()` 中创建临时仓库，并隔离 HOME 和 Git 全局配置，不会读写本机的仓库或设置。

## 已知限制

- 目前只在 macOS 上验证过。Linux 和 Windows 的文件监听仍是轮询，见 [0003](docs/decisions/0003-file-watching.md)。
- 界面字体使用系统字体（SF Pro），没有使用 Inter，见 [0002](docs/decisions/0002-fonts.md)。
- 冲突需要在外部编辑器里解决，见 [0005](docs/decisions/0005-read-only-conflicts.md)。
- 还不支持子模块和多 worktree 的专门界面，也没有交互式 rebase 编辑器。
