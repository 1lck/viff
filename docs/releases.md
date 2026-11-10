# Release process / 发布流程

## Write the release notes first / 先写发布说明

Use [Viff's release template](releases/TEMPLATE.md) for every stable or preview
release. Its structure follows the [Lithe-IDEA template](https://github.com/1lck/Lithe-IDEA/blob/main/docs/releases/TEMPLATE.md)
and [release writing guidelines](https://github.com/1lck/Lithe-IDEA/blob/main/.agents/skills/release-lithe/SKILL.md),
adapted to Viff's actual packages and platform support.

稳定版和预发布都使用 [Viff 发布模板](releases/TEMPLATE.md)。格式参考
[Lithe-IDEA 模板](https://github.com/1lck/Lithe-IDEA/blob/main/docs/releases/TEMPLATE.md)与
[发布写作规范](https://github.com/1lck/Lithe-IDEA/blob/main/.agents/skills/release-lithe/SKILL.md)，
内容以 Viff 实际产物和平台支持为准，不照搬其他项目的功能、贡献者、Intel 包或 Homebrew 安装说明。

1. Choose a version and its comparison baseline, such as `v0.1.0-alpha.2` and `v0.1.0-alpha.1`.
   A release request means a patch bump (`vX.Y.Z` to `vX.Y.Z+1`); raising the
   minor or major number is a separate decision the maintainer states
   explicitly, never something the size of the change set decides.
2. Copy the template to `docs/releases/vTAG.md`, where `vTAG` is the full tag, and replace every placeholder.
3. Derive changes from the release's commits and pull requests, verifying download names, compatibility, and contributors.
4. Validate and commit the notes with the release changes before creating a tag.
5. Merge into `main` and wait for CI to pass for the exact commit to be tagged.
6. Push the tag; the Release workflow builds packages and publishes the committed notes verbatim.
7. Verify the public description, downloads, and `SHA256SUMS` after publication.

中文步骤：

1. 选择版本和对比基线，例如 `v0.1.0-alpha.2` 与 `v0.1.0-alpha.1`。
   用户要求发版时默认只递增补丁位（`vX.Y.Z` → `vX.Y.Z+1`）；升次版本或主版本是维护者
   单独给出的决定，不凭「这次改动多」自行进位。
2. 将模板复制为 `docs/releases/vTAG.md`，其中 `vTAG` 为完整标签，并替换全部占位符。
3. 从本次提交和 PR 提炼变化，核实下载文件名、兼容性和贡献者身份。
4. 验证说明，并在创建标签前将说明与发布改动一同提交。
5. 合并至 `main`，等待待打标签的确切提交通过 CI。
6. 推送标签；Release 工作流构建安装包并原样发布已提交的说明。
7. 发布后检查公开说明、下载文件和 `SHA256SUMS`。

Run the validator before committing / 提交前运行验证：

```sh
python3 scripts/release_notes.py docs/releases/v0.1.0-alpha.2.md --tag v0.1.0-alpha.2
```

For another version, use `python3 scripts/release_notes.py docs/releases/vTAG.md --tag vTAG`
with the actual full tag. CI validates every `docs/releases/v*.md` file. Missing,
empty, or invalid notes block publication; there is no fallback to GitHub-generated
notes or a raw commit list. Historical notes added after a release document that
release; they do not retroactively change its tag.

其他版本使用 `python3 scripts/release_notes.py docs/releases/vTAG.md --tag vTAG`，
将 `vTAG` 替换为实际完整标签。CI 校验全部 `docs/releases/v*.md`。
说明缺失、为空或格式不合格时阻止发布，不回退到 GitHub 自动生成的说明或原始提交列表。
历史版本补录说明只记录该版本，不回改已发布标签。

### Keep the two languages equivalent / 保持双语一致

Write Chinese first and English second, separated by `---`. Keep the template's
literal headings and order: summary, downloads, highlights with an overview,
optional new features / improvements / fixes, upgrade instructions, compatibility,
and a comparison link. End with the Chinese and English contributor sections.
Omit empty categories in both languages. Keep each bullet to one user-facing
sentence on one Markdown source line, without nested lists, internal class names,
test counts, or raw logs. Include only verified changes and contributors.

中文在前、英文在后，以 `---` 分隔。保留模板标题和顺序：摘要、下载、先有概览的重点更新、
可选的新功能 / 改进 / 修复、升级说明、兼容性、版本对比链接，底部集中列出中英文贡献者致谢。
中英文同步删除空分类。每条只写一个用户可感知的变化，一句话、一行 Markdown，不嵌套列表，
不列内部类名、测试数量或原始日志。只写已核实的改动和贡献者。

Keep Viff's lightweight positioning factual: Go and Viva, without Electron or a
bundled browser runtime. Do not invent memory, startup, or performance numbers.
Verify signing and platform requirements for each release; headless checks do
not establish full desktop compatibility.

“轻量”围绕事实表达：Go 与 Viva，不采用 Electron，不捆绑浏览器运行时。
不编造内存、启动或性能数字。逐版核实签名和系统要求，不将无头检查等同于完整桌面兼容性。

## Choose the package / 选择安装包

For `v0.1.0`, the artifact names are:
`v0.1.0` was published before the Windows portable ZIP existed; the next release
adds it, so the ZIP row applies to that release onward.

| Platform / 平台                 | Artifact / 文件                            | Installation / 安装                                           |
| ------------------------------- | ------------------------------------------ | ------------------------------------------------------------- |
| macOS 13+, Apple Silicon        | `viff-v0.1.0-darwin-arm64.dmg`             | Drag Viff.app to Applications / 将 Viff.app 拖入 Applications |
| macOS 13+, Intel                | `viff-v0.1.0-darwin-amd64.dmg`             | Drag Viff.app to Applications / 将 Viff.app 拖入 Applications |
| Windows x64                     | `viff-v0.1.0-windows-amd64-setup.exe`      | Per-user installation / 安装到当前用户目录                    |
| Windows x64 portable / 便携 ZIP | `viff-v0.1.0-windows-amd64.zip`            | Extract, then run `viff.exe` / 解压后执行 `viff.exe`          |
| Debian/Ubuntu x64               | `viff-v0.1.0-linux-amd64.deb`              | `sudo apt install ./viff-v0.1.0-linux-amd64.deb`              |
| Linux x64 portable / 便携包     | `viff-v0.1.0-linux-amd64.tar.gz`           | Extract, then run `./viff` / 解压后执行 `./viff`              |

In-app updates read these releases from GitHub and download the DMG, the Windows
portable ZIP, or the Linux portable `tar.gz`, checking every download against
`SHA256SUMS`. The DEB stays a manual install because it needs elevation.

应用内更新从 GitHub Releases 读取上述产物，下载 DMG、Windows 便携 ZIP 或 Linux 便携包
`tar.gz`，并用 `SHA256SUMS` 校验每次下载；DEB 安装需要提权，仍保持手动安装。
`v0.1.0` 发布时还没有 Windows 便携 ZIP，从下一个版本开始提供，因此 ZIP 一行从该版本起适用。

The macOS arm64 package is built natively on the Apple Silicon runner. GitHub no
longer provides an Intel macOS runner, so the amd64 package is cross-compiled on
that runner with `CC="clang -arch x86_64"`; `scripts/package.py --arch amd64
--binary build/viff-amd64` verifies the Mach-O architecture before it builds the
DMG. Both packages target macOS 13 or later.
macOS arm64 包在 Apple Silicon runner 上原生构建；GitHub 已不再提供 Intel macOS
runner，因此 amd64 包在同一 runner 上用 `CC="clang -arch x86_64"` 交叉编译，
由 `scripts/package.py --arch amd64 --binary build/viff-amd64` 校验 Mach-O 架构后再生成 DMG。
两个包都面向 macOS 13 及以上。

Git must be available on `PATH`. The Linux DEB declares dependencies on `git`,
`libvulkan1`, `libxcb1`, `libcairo2`, `libfontconfig1`, `libfreetype6`,
`libharfbuzz0b`, `libpango-1.0-0`, and `libpangocairo-1.0-0`;
`apt install ./…deb` resolves those packages. The portable archive needs equivalent libraries installed separately.
Linux desktop use also needs a working graphical session and rendering backend.
The DEB installs `/usr/bin/viff` and a desktop menu entry. Windows installs under
`%LOCALAPPDATA%\Programs\Viff`, adds a Start menu shortcut and an uninstall entry,
and does not require administrator access.

Git 必须在 `PATH` 中。Linux DEB 声明了 `git`、`libvulkan1`、`libxcb1`、`libcairo2`、
`libfontconfig1`、`libfreetype6`、`libharfbuzz0b`、`libpango-1.0-0`、`libpangocairo-1.0-0` 依赖，使用 `apt install ./…deb` 可解析安装；便携包需要自行安装对应系统库。
Linux 桌面运行还需要正常的图形会话与渲染后端。DEB 安装 `/usr/bin/viff` 和桌面菜单项。
Windows 安装位置为 `%LOCALAPPDATA%\Programs\Viff`，添加开始菜单快捷方式和卸载入口，
无需管理员权限。

The macOS app uses an ad-hoc signature, without Developer ID signing or
notarization. Windows installers are not Authenticode signed. Operating-system security prompts are expected. Do not describe
native installation formats as signed releases, or headless CI as full desktop
acceptance. The older `v0.1.0-alpha.1` release contains portable ZIPs only.

macOS 应用使用 ad-hoc 签名，未经 Developer ID 签名或公证；Windows 安装器未经
Authenticode 签名。系统可能显示安全提示。
不要将原生安装格式等同于签名发布，也不要将无头 CI 等同于完整桌面验收。
旧版 `v0.1.0-alpha.1` 仅提供便携 ZIP。

## Package locally / 本地打包

Build `build/viff` (or `build/viff.exe`) on the target platform, then run:

在目标平台构建 `build/viff`（Windows 为 `build/viff.exe`），再执行：

```sh
python scripts/package.py v0.1.0-alpha.2
```

Packaging uses macOS `hdiutil`, Windows NSIS (`makensis`), or Linux `dpkg-deb`.
Windows release builds use `-ldflags="-s -w -H=windowsgui"` to avoid a console
window. Package checks must inspect and install/uninstall disposable test packages
without launching application windows. Preserve all bundled license notices.

打包分别使用 macOS 的 `hdiutil`、Windows 的 NSIS（`makensis`）或 Linux 的 `dpkg-deb`。
Windows 发布构建使用 `-ldflags="-s -w -H=windowsgui"`，避免弹出控制台窗口。
安装包检查使用可清理的临时环境，不启动应用窗口；所有分发包都须保留许可声明。

GitHub repository settings require pull requests, a passing CI check, resolved
review conversations, and linear history on `main`. Force pushes and branch
deletion are disabled. Owner/admin bypass remains available for recovery; no
extra reviewer is required for a single-maintainer project. Dependency updates
are proposed weekly by Dependabot.

GitHub 仓库主分支要求 PR、CI 通过、解决评审讨论及线性历史，禁止强推和删除。
保留所有者/管理员应急绕过权限；单维护者项目不强制额外审批人数。Dependabot 每周提出依赖更新。

## 应用图标

应用统一使用圆角白底、四角透明、无文字的橙色[品牌图标](../assets/branding/README.md)。
底板四周保留约 4% 画布的透明安全边距，避免桌面在缩放或裁切时切到图形。
原始图稿 `source.png` 保持整幅白底不变；修改图稿后运行 `go run ./cmd/brandassets`
会重新生成带圆角 alpha 的各平台资源。
打包脚本会配置 macOS ICNS、Windows 程序和安装器 ICO，以及 Linux 桌面图标。
