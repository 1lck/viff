---
name: viff-release-ci
description: "改 Viff 的 CI、发布流程、打包脚本、版本号或依赖升级时加载；不负责功能实现、文档双语写作，也不代替用户打 tag 发版。"
metadata:
  version: "1.0.0"
---

# Viff 发布与 CI 规范

## 何时使用

- 改 `.github/workflows/ci.yml`、`release.yml`、`.github/actions/*`、`dependabot.yml`。
- 调整 `scripts/package.py`、`scripts/release_notes.py`、`scripts/publish-release.py` 的调用方式或产物。
- 准备 `vTAG` 发布、排查 CI 必过项、给构建注入版本戳。
- 升级 Go 模块、工具链或 GitHub Actions 版本。

## 前置读取

1. `AGENTS.md` 的「常用命令」「测试规则」「文档规则」「Agent 行为约定」。
2. `docs/releases.md`（流程、安装包矩阵、签名与兼容性口径）。
3. `.github/workflows/ci.yml`、`.github/workflows/release.yml`、`.github/actions/go-test-shard/action.yml`。
4. `scripts/package.py`、`scripts/release_notes.py`、`scripts/publish-release.py`。
5. 涉及依赖：`go.mod`、`THIRD_PARTY_NOTICES.md`、`licenses/`。

## 必须遵守

- 没有 Go 工具链或没有打 tag/发布权限时只做静态核实；不运行发布流程、不 `git commit`/推送、不自行打 tag 或创建 Release，并写清「未运行 + 原因」。
- CI 门禁是 `required` job（`name: CI`）：`changes` 必须为 success；docs-only 时 `quality`/`test`/`ui`/`macos` 必须为 skipped，非 docs-only 时四者必须 success。`changes` 无条件运行，避免被跳过的 required check 长期 pending；不要把它改名或拆散。
- docs-only 只在 `pull_request` 生效：分类列表为 `README.md`、`README.zh-CN.md`、`CONTRIBUTING.md`、`SECURITY.md`、`THIRD_PARTY_NOTICES.md`、`LICENSE`、`docs/*`、`AGENTS.md`、`CLAUDE.md`、`.agents/*`；`main` push 始终全量跑，因为发布要求待 tag 提交有成功 CI。
- 保留并发语义：PR 的旧 run 可取消（`cancel-in-progress: ${{ github.event_name == 'pull_request' }}`），`main` 与发布 run 不取消。
- 超时预算：`changes`/`required` 3 分钟，`quality` 10 分钟，`test`/`ui`/`macos` 20 分钟；分片 `go test` 10 分钟（macOS UI 15 分钟）。超过预算先缩小或拆分测试，不盲目加时长。
- 发布前置两条同时满足：`docs/releases/vTAG.md` 已提交并通过 `python3 scripts/release_notes.py docs/releases/vTAG.md --tag vTAG`；待 tag 提交存在成功的 CI run。之后才推 `v*` tag 触发 `release.yml`。
- 产物集合固定 6 个：以 tag 为 `v0.1.0` 为例，`viff-v0.1.0-darwin-arm64.dmg`、`viff-v0.1.0-darwin-amd64.dmg`、`viff-v0.1.0-windows-amd64-setup.exe`、`viff-v0.1.0-linux-amd64.deb`、`viff-v0.1.0-linux-amd64.tar.gz`、`SHA256SUMS`；`publish-release.py` 会校验集合完全一致。
- 工作流、打包脚本变更独立成 PR，与功能改动分开；`AGENTS.md`、`CLAUDE.md`、`.agents/skills/` 的修改单独成一个 docs-only PR，不要混进 CI 或发布改动里。

## 步骤

### 改 CI

1. 先定位职责：`changes`（PR 分类，仅 pull_request）、`quality`（gofmt、发布说明与 Python 单测、`go mod verify`、`go mod tidy -diff`）、`test`（核心测试；shard 1 额外跑 vet、build、package 冒烟）、`ui`（无头 UI 套件）、`macos`（单 job 内并发跑 vet/核心/UI/构建）、`required`（汇总门禁）。
2. 分片用 `.github/actions/go-test-shard`：按 `go test -list '^Test'` 的顶层测试序号取模分配，新增测试自动纳入；不要手写测试名单。
3. 每个 job 显式设置 `timeout-minutes`；新增 job 也要设。证据上传走 `test-evidence-*` 工件（保留 14 天），失败时也上传。
4. 改 docs-only 分类时同步更新 `changes` job 的 `case` 与注释；agent 规范文件属于文档。
5. 无法本地运行 Go 时，在 PR 中写清只做了静态核实，并列出未执行项。

### 本地构建与打包

1. 构建：`go build -trimpath -ldflags="-s -w" -o build/viff ./cmd/viff`；Windows 加 `-H=windowsgui`。
2. 版本戳：`VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)`、`COMMIT=$(git rev-parse --short HEAD)`、`BUILD_DATE=$(date -u +%Y-%m-%d)`，拼成 `LD="-X viff/internal/version.Version=$VERSION -X viff/internal/version.Commit=$COMMIT -X viff/internal/version.BuildDate=$BUILD_DATE"`，再 `go build -trimpath -ldflags="-s -w $LD" -o build/viff ./cmd/viff`；发布构建改用 `$GITHUB_REF_NAME` 作为 `VERSION`。
3. 打包：`python scripts/package.py v0.1.0`；CI 与发布用 `python scripts/package.py vTAG --smoke`。`--smoke` 要求 `GITHUB_ACTIONS=true`，只允许一次性 runner。
4. macOS Intel：`GOARCH=amd64 CC="clang -arch x86_64" CGO_ENABLED=1 CGO_CFLAGS="-arch x86_64 -mmacosx-version-min=13.0" CGO_LDFLAGS="-arch x86_64 -mmacosx-version-min=13.0" MACOSX_DEPLOYMENT_TARGET=13.0 go build -trimpath -ldflags="-s -w $LD" -o build/viff-amd64 ./cmd/viff`，再 `python scripts/package.py vTAG --smoke --arch amd64 --binary build/viff-amd64`；`package.py` 用 `lipo` 校验 Mach-O 架构。
5. 检查 `dist/` 产物、包内 `LICENSE`、`THIRD_PARTY_NOTICES.md`、`licenses/`、`assets/icons/intellij/{LICENSE,NOTICE.md}`（`require_notices`），并运行 `./build/viff --version` 确认版本戳可见。

### 发版（需用户明确授权）

1. 按 `viff-docs-adr` 写并校验 `docs/releases/vTAG.md`，合并进 `main`。
2. 等该提交的 CI 全绿（Actions 页面或 `gh run list --commit <SHA>`）。
3. 由用户推 tag；`release.yml` 依次执行 `verify`（说明 + CI success）、`build`（macos-15、windows-2022、ubuntu-24.04）、`publish`（下载工件、生成 `SHA256SUMS`、`publish-release.py`）。
4. publish 用提交的说明原样建 Release；tag 含 `-` 时自动标 prerelease，上传 6 个资产并核对 digest，最后取消 draft。
5. 发布后核验：`gh release view vTAG` 检查说明与资产；下载 `SHA256SUMS` 后 `sha256sum -c SHA256SUMS`（macOS 用 `shasum -a 256 -c`）；签名与兼容性口径必须与 `docs/releases.md` 一致。

### 依赖升级

1. Dependabot 每周为 `gomod` 与 `github-actions` 各开 PR（上限 5）；一条 PR 只升一类，不混功能改动。
2. 新增依赖先说明标准库/现有代码为何不够，更新 `go.mod`/`go.sum`、`licenses/`、`THIRD_PARTY_NOTICES.md`，并记录影响。
3. 必须通过 `quality` 的 `go mod verify` 与 `go mod tidy -diff`；升级后跑三端构建与 UI 证据。
4. 大版本升级在 PR 写回归依据与回退方式；无依据不升级。

## 检查清单

- [ ] `required` 门禁语义完整；docs-only 时重活 skipped 仍能变绿。
- [ ] `changes` 分类列表与注释一致，agent 规范文件仍归 docs-only。
- [ ] 每个 job 有合理 `timeout-minutes`；分片逻辑仍覆盖全部顶层测试。
- [ ] 构建含 `-trimpath`、`-s -w` 与三个 `-X` 版本字段；Windows 含 `-H=windowsgui`。
- [ ] 发布说明通过 validator；tag 与产物文件名一致（tag 自带 `v`）。
- [ ] 6 个产物齐全；`SHA256SUMS` 由 publish 生成，不手写。
- [ ] 未自行打 tag、未推送、未创建 Release；报告写明未运行的命令与原因。

## 常见错误

- 删掉或条件化 `changes` job，导致 required 无法汇聚、PR 永远 pending。
- 只校验发布说明、不校验该提交 CI，发布未经测试的构建。
- docs-only 漏掉 `AGENTS.md`/`.agents/*`，规范 PR 触发全量构建。
- 手工放置或漏掉产物，`publish-release.py` 的资产集合校验直接失败。
- 在本地跑 `--smoke`，因 `GITHUB_ACTIONS != true` 被 `package.py` 拒绝。
- 在 Apple Silicon 上用普通 `go build` 冒充 Intel 包，`lipo` 架构校验失败。
- 改 workflow 时顺手改 `cancel-in-progress`，导致 main 的 CI 被取消、发布 verify 找不到成功 run。

## 相关文件

- [AGENTS.md](../../../AGENTS.md)
- [.github/workflows/ci.yml](../../../.github/workflows/ci.yml)
- [.github/workflows/release.yml](../../../.github/workflows/release.yml)
- [.github/actions/go-test-shard/action.yml](../../../.github/actions/go-test-shard/action.yml)
- [.github/dependabot.yml](../../../.github/dependabot.yml)
- [scripts/package.py](../../../scripts/package.py)
- [scripts/release_notes.py](../../../scripts/release_notes.py)
- [scripts/publish-release.py](../../../scripts/publish-release.py)
- [docs/releases.md](../../../docs/releases.md)
