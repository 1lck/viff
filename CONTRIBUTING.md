# 贡献指南 / Contributing

[中文](#中文) · [English](#english)

Viff 是一个用 Go 和 [Viva](https://github.com/vivaui/viva) 编写的 Git 桌面客户端。开始之前请先读 [README](README.md)、[设计文档](docs/design.md) 和 [架构决策记录](docs/decisions/README.md)。

Viff is a Git desktop client written in Go with [Viva](https://github.com/vivaui/viva). Please read the [README](README.md), the [design document](docs/design.md) and the [architecture decision records](docs/decisions/README.md) first.

## 中文

### 参与方式

Issue 和 Pull Request 都可以用中文或英文提交。提交问题前请先搜索已有 Issue；讨论请聚焦技术内容。安全漏洞不要开公开 Issue，请按 [SECURITY.md](SECURITY.md) 私密报告。

### 环境要求

- Go 1.27.3 或更高版本，以及本机安装的 `git`。
- macOS 上编译需要 Xcode Command Line Tools，因为字体注册和 FSEvents 使用 cgo。

### 测试

在仓库根目录运行：

```sh
go test ./...
go test -tags 'vivahead litheaudit' ./internal/ui
```

第一条覆盖领域、Git 解析与命令、工作区与监听器；第二条运行无头 UI 测试（提交、日志、diff、冲突、stash、worktree 等）。`litheaudit` 是外部 issue 回归审计的 opt-in 标签。

CI 在 macOS 与 Windows 上运行同一组测试，并额外检查 `gofmt`、`go vet -tags 'vivahead litheaudit' ./...`、`go mod verify` 和 `go mod tidy -diff`。

UI 测试在无头模式下运行，不打开应用窗口。在 CI、SSH 等没有图形界面的环境中不要启动 `cmd/viff`，使用 Viva 离屏渲染后端；需要生成截图时显式设置 `VIFF_SHOTS`。

测试必须在 `t.TempDir()` 中创建临时仓库，并隔离 `HOME` 与 Git 全局配置，不能读写本机的真实仓库或设置。

### 改动约定

- Git 是仓库事实来源：保持 Git 状态正确，不要用 UI 缓存替代重新查询。
- domain、app、gitcli 不依赖 Viva；Viva 类型不进入这些包。
- 行为变化请同步更新 README 的“已知限制”、相关文档，或在 `docs/decisions` 新增记录。
- 一个 PR 聚焦一件事，避免混入无关格式化、重命名或依赖升级；Go 代码保持 `gofmt` 格式。
- 提交信息用简洁的祈使句，说明改动的原因。
- 不要提交构建产物、测试临时目录、凭据或令牌；截图只在确有必要时提交。

### Pull Request

1. 从 `main` 创建分支。
2. 完成改动，并运行上面的测试。
3. 在 PR 中说明问题、做法、实际运行的命令与平台；界面变化附前后对比。
4. 由维护者审阅，可能需要修改后再合并。

### 许可

本项目以 MIT 许可证发布（见 [LICENSE](LICENSE)）。提交贡献即表示你同意按同一许可证授权。

## English

### Ways to contribute

Issues and pull requests are welcome in Chinese or English. Search existing issues first, and keep discussion technical. Do not open a public issue for a security vulnerability — report it privately as described in [SECURITY.md](SECURITY.md).

### Requirements

- Go 1.27.3 or newer, plus a local `git`.
- On macOS, Xcode Command Line Tools are required for cgo (font registration and FSEvents).

### Tests

Run from the repository root:

```sh
go test ./...
go test -tags 'vivahead litheaudit' ./internal/ui
```

The first command covers the domain, Git parsing and commands, the workspace and the watchers. The second runs the headless UI suite (commit, log, diff, conflict, stash, worktree and more). `litheaudit` is an opt-in tag for the external-issue audit.

CI runs the same tests on macOS and Windows, and additionally checks `gofmt`, `go vet -tags 'vivahead litheaudit' ./...`, `go mod verify` and `go mod tidy -diff`.

The UI tests run headless and never open an application window. In headless environments such as CI or an SSH session, do not launch `cmd/viff` use the Viva offscreen renderer, and set `VIFF_SHOTS` explicitly when screenshots are needed.

Tests must create temporary repositories under `t.TempDir()` and isolate `HOME` and the global Git configuration. They must not touch the user's real repositories or settings.

### Change rules

- Git is the source of truth for repository state: keep Git state correct and re-query instead of trusting UI caches.
- `domain`, `app` and `gitcli` must not depend on Viva; Viva types must not enter those packages.
- For behaviour changes, update the README's known limitations or the relevant docs, or add a record under `docs/decisions`.
- Keep one concern per PR and avoid unrelated formatting, renames or dependency bumps. Keep Go code `gofmt`-clean.
- Write imperative commit messages that explain why the change is needed.
- Do not commit build artifacts, test temporary directories, credentials or tokens; commit screenshots only when they are genuinely needed.

### Pull requests

1. Branch from `main`.
2. Make the change and run the tests above.
3. In the PR, describe the problem, the approach, the commands you actually ran and your platform; attach before/after images for UI changes.
4. A maintainer reviews the PR, and changes may be requested before it is merged.

### License

Released under the [MIT License](LICENSE). By contributing, you agree to license your contribution under the same terms.
