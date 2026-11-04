<div align="center">

# Viff

**轻量、原生的 Git 桌面客户端。**

Go + Viva。无需 Electron，无需 WebView，不捆绑浏览器运行时。

[English](README.md) · [简体中文](README.zh-CN.md)

[![CI](https://github.com/1lck/viff/actions/workflows/ci.yml/badge.svg)](https://github.com/1lck/viff/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27.3%2B-00ADD8.svg)](https://go.dev/dl/)

[快速开始](#快速开始) · [功能](#轻量也能覆盖完整的-git-工作流) · [参与贡献](CONTRIBUTING.md) · [下载](https://github.com/1lck/viff/releases)

</div>

![Viff 深色界面：逐行暂存、并排 Diff 与提交历史](docs/screenshots/diff-dark.png)

## 让工具轻一点，让工作流完整一点

Viff 将工作区、代码差异和提交历史放进一个原生桌面窗口。
它使用 Go 和 [Viva](https://github.com/vivaui/viva) 编写，不依赖浏览器引擎绘制界面，
通过本机安装的 Git 执行操作。

- **分发轻量**：一个应用可执行文件，加上许可证文件，不需要安装独立浏览器运行时或本地服务。
- **原生桌面基础**：Viva 负责绘制与平台集成，Viff 不采用 Electron 或 WebView。
- **沿用你的 Git**：使用现有仓库、远端、钩子及凭据配置，在 Console 中查看实际执行的命令和输出。
- **按需加载**：历史分页读取，Diff 按需打开，避免启动时把大仓库的全部内容一次加载进来。

作为可复现的体积参考：2026 年 11 月 4 日，使用 Go 1.27.3 构建并去除调试符号后，
**macOS arm64 可执行文件为 7.37 MiB，Windows amd64 为 7.65 MiB**
（`-trimpath -ldflags="-s -w"`，仅统计可执行文件）。压缩包大小、本机 Git 和运行内存不在这个数字内。

“轻量”是设计目标，不是未经测量的性能承诺。实际内存与启动耗时取决于仓库、平台和渲染环境，
目前不宣称具体内存占用，也不使用“比某产品快几倍”的对比数字。

## 轻量，也能覆盖完整的 Git 工作流

| 场景       | 功能                                                                      |
| ---------- | ------------------------------------------------------------------------- |
| 查看改动   | 并排 / 统一 Diff、语法与词级高亮、搜索、同步滚动、多页签                  |
| 整理提交   | 按文件、块或选中行暂存，取消暂存、回滚、Amend、Sign-off、提交并推送       |
| 管理工作区 | 多仓库、各仓库独立提交草稿、目录分组、命名 ChangeList                     |
| 阅读历史   | 提交图、分支 / 作者 / 路径筛选、多提交文件汇总、Blame、Reflog、历史仓库树 |
| 操作分支   | Checkout、Merge、Rebase、Cherry-pick、Revert、Reset、借助 Stash 切换分支  |
| 解决冲突   | Ours / Result / Theirs 三栏，逐块接受 Ours / Theirs / Both，显式标记解决  |
| 多工作目录 | 从新分支或已有分支创建 Worktree，管理子模块，保护未提交改动               |
| 同步协作   | Fetch、Pull、Push、force-with-lease、远端、补丁、签名与凭据设置           |

**ChangeList 是文件分组，不是独立的 Git 索引。** 即使开启清单筛选，提交仍包含
**全部已暂存文件**；筛选后的批量暂存只处理可见文件。

<details>
<summary>查看浅色主题</summary>

![Viff 浅色界面：Diff 与所选提交详情](docs/screenshots/diff-light.png)

</details>

截图来自真实 Viva 无头测试，使用可销毁的演示仓库。

## 快速开始

### 从源码构建

安装 **Go 1.27.3 或更高版本**和 **Git**。macOS 还需要 Xcode Command Line Tools
（`xcode-select --install`），字体注册与文件监听使用了 cgo。

```sh
git clone https://github.com/1lck/viff.git
cd viff
go build -trimpath -ldflags="-s -w" -o build/viff ./cmd/viff
./build/viff /path/to/repository
```

Windows 克隆后，在 PowerShell 中执行：

```powershell
go build -trimpath -ldflags="-s -w -H=windowsgui" -o build/viff.exe ./cmd/viff
.\build\viff.exe C:\path\to\repository
```

不传入路径时，会重新打开上次的项目或显示欢迎页；也可以在欢迎页打开、克隆或初始化仓库。

### 下载发布版本

在 [GitHub Releases](https://github.com/1lck/viff/releases) 下载对应平台的包。
从 **v0.1.0-alpha.2** 起，提供原生安装格式：

| 平台      | 架构                   | 格式         | 安装方式                                             |
| --------- | ---------------------- | ------------ | ---------------------------------------------------- |
| macOS 12+ | Apple Silicon（arm64） | `.dmg`       | 打开磁盘映像，将 **Viff** 拖入 **Applications**。    |
| Windows   | x64（amd64）           | `-setup.exe` | 运行安装器，安装到当前用户目录。                     |
| Linux     | x64（amd64）           | `.deb`       | Debian/Ubuntu 执行 `sudo apt install ./viff-*.deb`。 |
| Linux     | x64（amd64）           | `.tar.gz`    | 解压便携包，在解压目录执行 `./viff`。                |

Git 需单独安装并加入 `PATH`。Linux 还需要[发布指南](docs/releases.md)中列出的系统渲染库。
发布页提供 `SHA256SUMS`，可用于核对下载文件；最初的 `alpha.1` 版本仅提供便携 ZIP。

目前仍是**预发布版本**。macOS 应用使用 ad-hoc 签名，未经 Developer ID 签名或公证；
Windows 安装器未经 Authenticode 签名，因此操作系统可能显示安全提示。

## 平台支持进度

| 平台    | 验证情况                                                        |
| ------- | --------------------------------------------------------------- |
| macOS   | 原生开发环境，Git 与无头 UI 测试                                |
| Windows | Windows Server 2022 x64 实机 Git、Viva 无头 UI 验收，Windows CI |
| Linux   | Ubuntu Git 与 Viva 无头 UI CI；交互式桌面验收尚未完成           |

Windows 10/11 的桌面交互、输入法及签名仍需专门验收；部分快捷键文案仍使用 macOS 符号。
Windows 与 Linux 文件监听采用轮询。

公开发布前的验收覆盖 **298 个顶层测试**，其中 **80 个无头 UI 测试**，并收集
**194 张 Windows 截图**。这些是特定日期和环境的验证结果，不代表覆盖了所有平台与边界情况。
详见[测试与平台说明](docs/testing.md)。

## 开发与测试

```sh
go test ./...
go vet -tags "vivahead litheaudit" ./...
go test -tags "vivahead litheaudit" ./internal/ui
```

UI 测试使用**无头模式，不打开应用窗口**。需要保存截图时：

```sh
VIFF_SHOTS="$PWD/.viff-test/shots" VIFF_SHOTS_BY_TEST=1 \
  go test -count=1 -tags "vivahead litheaudit" ./internal/ui
```

测试创建临时仓库并隔离 Git 配置。CI 在 macOS、Windows、Linux 上执行，并将验收证据保存为工作流附件。
原生安装包的检查同样不打开应用窗口。

- [贡献指南](CONTRIBUTING.md)
- [安全问题报告](SECURITY.md)
- [完整功能指南](docs/features.zh-CN.md)
- [设计文档](docs/design.md)与[架构决策](docs/decisions/README.md)

设置保存在系统用户配置目录的 `Viff/settings.json`，可在设置对话框查看实际路径。
写操作仅作用于当前仓库，非活动仓库按需刷新。自由文本冲突编辑及不支持的二进制冲突需要外部编辑器。

## 许可与致谢

Viff 原创代码采用 [MIT 许可证](LICENSE)。Viva、JetBrains Mono、IntelliJ Platform 图标及其它依赖
保留各自许可，详见[第三方声明](THIRD_PARTY_NOTICES.md)。

界面参考了 IntelliJ IDEA 的 Git 工作流。Viff 是独立项目，与 JetBrains 没有关联，也未获得其背书。
