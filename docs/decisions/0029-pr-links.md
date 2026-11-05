# 0029 仓库、提交与分支比较的网页链接

- 状态：已采用（2026-11）

## 背景

0.2 的第一项要求：在应用内给出 GitHub、GitLab、Bitbucket、Gitea 的仓库、提交、分支比较与创建 PR／合并请求的入口。此前用户只能复制远程地址、再到浏览器里自己拼路径；Log 里看到一个提交、分支弹窗里看到一个分支，都没有「在网页上打开」的动作。

约束是不做 OAuth、不调用服务 API、不依赖 `gh`／`glab`，只做 URL 生成加系统浏览器打开。这既符合「系统 Git 是唯一运行时依赖」的项目事实，也避免为四个服务各写一套令牌、刷新与错误处理。

## 替代项

- **调用服务 API（OAuth 或访问令牌）**：能读 PR 状态、评论与 CI，也能直接创建 PR；代价是凭据存储、令牌刷新、四套 API、网络错误与限流，还要处理「离线时入口不可用」。本项只需要入口，不值得先把凭据边界打开。
- **依赖 `gh`／`glab` 命令行**：实现快，但要求用户另外安装并登录，只覆盖 GitHub 与 GitLab，且与「系统 Git 是运行时依赖，其他都要探测」的约定冲突。
- **内嵌 WebView 显示提交或 PR 页面**：项目明确不使用 WebView；引入它会带来第二套渲染、Cookie 与登录态边界。
- **只给仓库根链接**：最省事，但提交、比较和创建 PR 仍要用户自己拼；这三类地址的布局恰好是各服务最稳定的部分。
- **解析 `.git/config` 取远程地址**：会在应用层再造一条读 `.git` 的路径；`git remote -v` 已经随刷新查过，`RepoState.Remotes` 里就有地址。
- **把基准分支固定成 `main`**：多数仓库如此，但对 `develop`、`trunk` 或改了默认分支的仓库会生成错误的比较与 PR 地址。

## 决定

- URL 生成放在 `internal/app/hosturl.go`，是纯函数：`ParseRemoteURL` 把 `git remote -v` 打印的地址解析成 `RemoteLink`（服务、host、项目路径），`RemoteLink` 的 `RepoURL`／`CommitURL`／`BranchURL`／`CompareURL`／`NewPullRequestURL` 生成各页面。没有 Git 调用，没有网络，不做 I/O。
- 解析接受 https、`ssh://`、`git://` 与 scp 风格（`git@host:owner/repo.git`）：host 统一小写，凭据（`user:token@`）被丢弃，http(s) 远程的端口保留，ssh 的端口不当作网页端口；路径去掉结尾 `.git`，至少要有 owner 与仓库两段，GitLab 的子组保留全部层级。读不懂的地址（本地路径、`file://`、缺少 owner/repo）返回可展示的错误，不 panic、不返回半截地址。
- 服务按主机名识别：`github.com`／含 `github` 为 GitHub，`gitlab.com`／含 `gitlab` 为 GitLab，`bitbucket.org`／含 `bitbucket` 为 Bitbucket，`codeberg.org`、`gitea.com` 或含 `gitea`／`forgejo` 为 Gitea；其余为未知实例，页面按 GitHub 布局生成，仓库根地址在任何情况下都成立。
- 页面布局：

  | 页面 | GitHub | GitLab | Gitea | Bitbucket |
  | --- | --- | --- | --- | --- |
  | 仓库 | `/owner/repo` | `/group/sub/repo` | 同 GitHub | `/owner/repo` |
  | 提交 | `/commit/<sha>` | `/-/commit/<sha>` | `/commit/<sha>` | `/commits/<sha>` |
  | 分支 | `/tree/<branch>` | `/-/tree/<branch>` | `/src/branch/<branch>` | `/branch/<branch>` |
  | 比较 | `/compare/<base>...<head>` | `/-/compare/<base>...<head>` | 同 GitHub | 不生成 |
  | 创建 PR | `/compare/<base>...<head>?expand=1` | `/-/merge_requests/new?merge_request[source_branch]=<head>&merge_request[target_branch]=<base>` | 同 GitHub | 不生成 |

  分支名按路径段转义，`/` 保留（服务把 `feature/x` 当作一个分支），`#`、空格等字符转义。
- Bitbucket 是 best effort：仓库、分支、提交页面可用，比较与创建 PR 的地址 Viff 拿不准，返回 `ErrLinkUnsupported`；界面改为打开分支页面，并明确提示「不为这个服务生成该页面」，不静默打开一个别的地址。
- 基准分支（比较与 PR 的目标）按顺序解析：当前分支的 upstream 去掉远程前缀；该远程的默认分支 `refs/remotes/<remote>/HEAD`；`main`，再 `master`（优先远程跟踪分支）。分支不能是自己的基准（`/compare/main...main` 没有意义），这种情况按顺序落到下一项；四项都没有时弹出候选分支列表让用户选，不猜。
- 默认分支来自新增的 `gitcli.RemoteHeads`：一次本地的 `for-each-ref refs/remotes`（读 `%(symref)`），随其他引用一起刷新；不使用会联网的 `git remote show`。
- `internal/platform.OpenURL` 只接受 http 与 https：地址来自仓库的远程，克隆内容可以控制它，把 `file:` 或自定义协议交给系统打开器等于让远程执行别的东西。darwin 用 `open`，windows 用 `rundll32 url.dll,FileProtocolHandler`（经 `process.HideConsoleWindow` 不弹控制台），其他平台用 `xdg-open`；地址作为单个参数传入，绝不经 shell。
- 界面入口：Log 单提交右键菜单一项「Open Commit on <服务>」；分支弹窗里本地分支的子菜单增加「Open <branch> on <服务>」「Compare with <base>」「Create Pull Request…」。创建 PR 只对当前分支或已有 upstream 的分支显示；远程没有网页地址（本地裸仓库）时整项不显示，而不是留一个点了没反应的禁用项。打开失败用 `NotifyError` 通知，不吞错。
- 明确不做：OAuth 与令牌、服务 API、在应用内创建 PR 或推送分支、PR 状态与评论、Bitbucket 的比较与 PR 页面。取舍与后续入口见复审条件。

## 代价

- 未知主机（多为自建实例）按 GitHub 布局生成提交与比较地址；实例若用别的布局，用户会看到服务自己的 404。仓库根地址不受影响。
- 分支没有推送到远程时，创建 PR 的页面仍会打开，由服务端提示分支不存在；Viff 不联网检查，以避免为一个菜单项引入网络失败模式。
- ssh 远程不携带网页端口：自建实例的网页不在 443 且远程是 `ssh://` 时，生成的地址会错；把该远程的 fetch URL 写成 https 地址即可修正。
- 比较与 PR 页面依赖服务的分支解析规则；分支名含特殊字符时由服务端解码，Viff 只做路径段转义，无法在本地验证服务端行为。
- 每次刷新多一次本地的 `for-each-ref refs/remotes`；代价与现有的 `Refs` 查询同一数量级，尚未在超大仓库上单独测量。
- 未知主机的菜单文案显示主机名（如 `git.example.com`），比服务名长；托盘或窄面板里可能被截断。
- 不做 API 意味着 Viff 不知道 PR 是否已存在、是否已合并，也没有评论与 CI 入口。

## 复审条件

- 用户要求创建 PR 之后跟踪状态、评论或 CI，或要求在应用内完成登录时，重新评估服务 API 与凭据存储方案（会新增依赖与隐私边界，需要新的 ADR）。
- 自建实例的 404 反馈变多，或用户要求自定义 URL 模板时，增加按仓库配置的页面模板，而不是继续扩大主机名猜测表。
- `RemoteHeads` 的额外查询在大型仓库上造成可测量的刷新开销时，改为按需查询或按代次缓存，并先补基准。
- 任一服务的布局改变（如 GitLab 调整 `/-/` 前缀、Gitea 改分支地址）时，修订上面的布局表并补对应表驱动测试。
