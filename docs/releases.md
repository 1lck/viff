# Release process / 发布流程

## Publish a tested version / 发布已验证版本

1. Merge changes into `main` and wait for the **CI** check to pass.
2. Choose a version. Pre-release tags include a suffix, such as `v0.1.0-alpha.2`.
3. Push the tag pointing to the tested commit.
4. The Release workflow verifies successful CI for that exact commit, builds on
   macOS, Windows, and Linux, and packages the executable with license notices.
5. Review the published packages and `SHA256SUMS`, and expand the release notes
   with changes, verification, and remaining limitations.

6. 合并至 `main`，等待 **CI** 通过。
7. 选择版本；预发布使用后缀，例如 `v0.1.0-alpha.2`。
8. 将指向已验收提交的标签推送到 GitHub。
9. Release 工作流核实该提交的 CI 成功后，在 macOS、Windows、Linux 构建，并打包程序及许可声明。
10. 检查发布的安装包与 `SHA256SUMS`，补充具体改动、验证情况和已知限制。

## Choose the package / 选择安装包

For `v0.1.0-alpha.2`, the artifact names are:

| Platform / 平台             | Artifact / 文件                               | Installation / 安装                                           |
| --------------------------- | --------------------------------------------- | ------------------------------------------------------------- |
| macOS 13+, Apple Silicon    | `viff-v0.1.0-alpha.2-darwin-arm64.dmg`        | Drag Viff.app to Applications / 将 Viff.app 拖入 Applications |
| Windows x64                 | `viff-v0.1.0-alpha.2-windows-amd64-setup.exe` | Per-user installation / 安装到当前用户目录                    |
| Debian/Ubuntu x64           | `viff-v0.1.0-alpha.2-linux-amd64.deb`         | `sudo apt install ./viff-v0.1.0-alpha.2-linux-amd64.deb`      |
| Linux x64 portable / 便携包 | `viff-v0.1.0-alpha.2-linux-amd64.tar.gz`      | Extract, then run `./viff` / 解压后执行 `./viff`              |

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
