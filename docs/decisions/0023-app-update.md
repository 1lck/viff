# 0023 应用内更新

- 状态：已采用（2026-11）

## 背景

Viff 之前没有任何应用内更新能力：升级必须打开 GitHub Releases 页面，手动下载对应平台的 DMG、安装器或压缩包并覆盖安装。`docs/design.md` 的维护规则只写了“自动更新另立设计”，发布说明也明确声明“本次未提供 Homebrew 安装方式或自动更新”。用户想要的是留在应用内的完整闭环：检查更新 → 应用内下载 → 校验 → 点“Restart to Update”替换并重启，不需要打开浏览器。

应用自身已经有 `internal/version`（版本号由 `-ldflags` 注入，About、`--version` 和诊断导出都用它），发布流程由 tag 触发的 GitHub Actions 构建并上传产物、生成 `SHA256SUMS`，这为版本比较和完整性校验提供了现成基础。

## 替代项

- **接入 Sparkle / WinSparkle 等平台更新框架**：成熟，但引入平台专用二进制依赖与 appcast 签名体系（EdDSA），macOS 还需公证；Windows 方案与 macOS 不统一，和“纯 Go、无额外运行时”的定位冲突。
- **依赖系统包管理器**：macOS 无统一包管理器；Windows 没有默认包管理器；Linux DEB 需要 root。用户仍要离开应用并处理提权，达不到“应用内一键更新”。
- **静默调用现有安装器**：Windows NSIS 可用 `/S`，但运行中的 `viff.exe` 被占用，安装器需要先结束进程，且无法可靠保证安装后自动重启；macOS 仍需挂载 DMG、替换 `.app`。安装器内部布局不是稳定的更新协议，升级安装器后容易失效。
- **GitHub Releases API + 自带更新器（采用）**：只依赖标准库；版本与资产信息来自公开 Releases API；完整性由 `SHA256SUMS` 校验；替换逻辑由 Viff 自己控制，能给出明确的进度、错误和“重启以更新”。

## 决定

- 新增 `internal/updater`：纯标准库实现，不新增第三方依赖；`app` 通过窄接口 `UpdateService` 使用它，`cmd/viff` 负责装配，`ui` 只渲染 `app` 的状态。
- 版本来源：`GET https://api.github.com/repos/1lck/viff/releases?per_page=30`，请求带 `User-Agent: Viff/<version>`；只读取公开 release 元数据。
- 选版规则：跳过 draft；当前版本是正式版时跳过 prerelease，当前版本是预发布时允许更新到更新的预发布；按语义化版本比较，取严格大于当前版本的最新候选；`dev` 构建不检查更新。
- 资产规则：按 `GOOS/GOARCH` 精确选择 `viff-<tag>-<goos>-<goarch>` 加平台后缀——macOS `.dmg`、Windows `.zip`、Linux `.tar.gz`。为此发布流程新增 Windows 便携 ZIP 资产；DEB 仍然保留给系统包管理器。
- 完整性：下载后必须与同一 release 的 `SHA256SUMS` 中对应条目一致，并校验大小；校验失败不安装。网络请求只允许 HTTPS（本地测试服务器除外），重定向限制次数。
- 替换方式：下载到用户缓存目录（`Viff/updates`，权限 0700）。安装时写一个平台专用 helper 并分离启动，helper 等 Viff 退出后替换文件、再启动新版本：
  - macOS：`hdiutil` 挂载 DMG → `ditto` 复制新的 `Viff.app` → 替换整个 bundle（失败回滚）→ `open` 新应用。
  - Windows：ZIP 内提取 `viff.exe` → `move` 旧 exe 为 `.old`、换入新 exe → `start` 新 exe → 删除备份。
  - Linux 便携安装：tar.gz 内提取 `viff` → `install -m 0755` 覆盖 → `exec` 新程序。
  - Linux 系统安装（`/usr/bin/viff`）：识别为包管理器管理，改为下载同一版本的 DEB 并返回 `ManualInstallError`，界面显示验证过的下载路径与 `sudo apt install <deb>` 命令，不尝试提权，也不让 dpkg 记录与磁盘内容脱节。
- 触发与交互：设置项“Check for updates at startup”默认开启，启动时后台静默检查；Help 菜单提供“Check for Updates…”；发现更新时用通知气泡（Update 动作）或菜单入口打开对话框。对话框状态：检查中 → 有更新（版本、说明、Update）→ 下载中（百分比、可取消）→ 已下载（Restart to Update）→ 安装并重启；另含已最新、手动安装、失败重试状态。
- 退出：helper 需要等 Viff 退出，而 Viva v0.2.3 的 `RunCanvas` 没有程序化关闭窗口的 API（已核实模块源码）。`Restart to Update` 在刷新设置、停止监听并关闭工作区后调用 `os.Exit(0)`，由 helper 完成替换和重启。
- 隐私：更新检查是 Viff 第一个默认网络请求，只发送版本号（User-Agent）和公开 release 查询，不上传仓库内容、路径、设置或使用数据；可在设置中关闭。`docs/privacy.md` 披露请求地址、时机、缓存位置与删除方式。
- 观测：新增 `update.check`、`update.download`、`update.install` 事件，只记录版本、是否手动、字节数、耗时和结果，不记录下载 URL 之外的敏感信息。
- 兼容：v0.1.0 及更早版本没有更新器，升级到第一个带更新器的版本仍需手动下载一次；之后的应用内升级才能自举。

## 代价

- **没有系统签名校验**：macOS 仍是 ad-hoc 签名、Windows 无 Authenticode，更新器只做 HTTPS + SHA-256 校验；能防止传输损坏和明显的资产替换，但不能替代 Developer ID/Authenticode。补签名后应把签名校验加入安装前检查。
- **Windows/Linux 只替换可执行文件**：随包的 `docs/`、`licenses/` 保持旧版本，直到下一次完整安装；macOS 替换整个 `.app`，没有这个不一致。
- **DEB 无法一键更新**：root 所有的 `/usr/bin/viff` 不能由普通用户替换；更新器会下载 DEB 并给出 `sudo apt install` 命令，但最后一步仍需要用户提权。
- **`os.Exit(0)` 跳过 Viva 资源释放**：进程退出后由操作系统回收 GPU/窗口资源；若 Viva 未来暴露程序化退出 API，应改回正常关闭路径。
- **GitHub API 限额**：未认证请求 60 次/小时/IP；自动检查失败时静默，手动检查才显示错误。每次更新下载完整安装包，没有增量更新。
- **平台 helper 首次真实替换无法在 CI 中验证**：CI 只能覆盖选版、校验、下载、脚本生成等无副作用部分；macOS/Windows/Linux 的真实替换与重启需要人工桌面验收。

## 复审条件

- Viva 提供程序化关闭窗口或退出应用的能力时，移除 `os.Exit(0)` 路径。
- macOS 获得 Developer ID 签名与公证、Windows 获得 Authenticode 后，把签名校验加入安装前检查，并更新发布说明口径。
- GitHub Releases API 地址、鉴权或速率限制策略变化，导致自动检查不可用时。
- 用户反馈替换失败、helper 未重启、杀毒软件拦截或 macOS 安全提示增加时，重新评估 helper 方案。
- 需要灰度渠道、增量更新、静默自动安装或回滚时，另立设计，不继续往当前实现加开关。
