# 0024 增加 Subversion 日常客户端支持

- 状态：已采用（2026-11；架构设计已定，实现未开始）
- 相关：[0001](0001-git-cli.md)、[0005](0005-read-only-conflicts.md)、[0020](0020-repositories-changelists.md)

## 背景

Viff 当前只支持 Git：`internal/app/workspace.go` 用类型别名 `type Git = *gitcli.Runner` 直接绑定具体适配器，`gitcli.Runner` 有 86 个公开方法，`app` 层有 79 处直接调用点、40 余个 `gitcli.*` 类型依赖。`docs/design.md` 声称的「源码依赖由应用层定义的小接口约束」在实现中并不存在。

需求是为 Subversion 增加**日常客户端**支持（非只读查看器）：检出、状态、历史、diff、blame、update、commit、revert、分支切换。

Subversion 与 Git 的模型差异不是实现细节，而是**与本项目既有核心前提相冲突**。仓库反复强调的「Git 是仓库事实来源，UI 缓存只能加速，不能替代重新查询」成立的前提是本地查询廉价。Git 的 `.git` 含完整对象库，`git status`/`git log` 是毫秒级本地子进程；SVN 的 `.svn` **只有 pristine 基线文件、没有历史**，`svn log` 必须访问仓库（网络或 `file://`）。因此 SVN 后端必须引入显式的持久缓存与离线降级状态，这是对既有原则的一次有理由的偏离，不能悄悄复用 Git 的「永远重查」策略。

本文的 SVN 行为均在本机实测得到，不是凭记忆书写：Windows 11 x64，`svn` 1.15.0 (r1937767) amd64-windows，`ra_local`/`ra_serf`/`ra_svn` 均可用。

## 实测：与 Git 的关键差异

以下每条都在临时仓库中验证过，直接决定实现方式。

| 编号 | 实测事实 | 后果 |
| --- | --- | --- |
| S1 | `svn log` 不带 `-r` 时只回溯到工作副本的 BASE 版本。提交后工作副本目录仍是 `revision="1"`，`svn log -l 3` 只返回 r1，**静默漏掉 r2** | 历史查询必须显式 `-r 1:HEAD` 并针对仓库 URL；默认调用会静默给出过期历史 |
| S2 | 工作副本是**混合修订版**：`svn info` 的 `revision` 是该路径的 BASE，不是仓库 HEAD | 判断「是否有远程更新」必须 `svn info -r HEAD <URL> --show-item revision` |
| S3 | **没有暂存区**。`svn status --xml` 只有单列 `<wc-status item="modified"/>`，不存在 index 与工作树两列 | 现有 `AreaStaged`/`AreaUnstaged` 双栏模型在 SVN 下无意义，UI 必须按能力位退化 |
| S4 | 但**支持部分提交**：`svn commit -m msg f1.txt` 只提交 f1，其余保持待提交 | 用「显式提交目标集」替代暂存区是可行的 |
| S5 | `svn status` 在存在冲突时**退出码仍为 0** | 错误分类不能依赖退出码，必须解析 stderr 的 `E` 编号 |
| S6 | 冲突标记是标准 diff3，含 `\|\|\|\|\|\|\|` base 段，7 个标记字符 | 现有 `internal/domain/markers.go` 的 `ParseConflictMarkers` 已支持，可直接复用 |
| S7 | 冲突产生 `file.mine`/`file.r2`/`file.r3` 辅助文件，在 status 中显示为 `unversioned` | 不过滤会在未跟踪列表里出现三个垃圾项 |
| S8 | 属性变更单独表达为 `props="modified"`，与文本修改分离 | 状态模型需要独立字段，不能塞进文件模式字段 |
| S9 | Windows 下 `svn status --xml` 的 `path` 用反斜杠（`docs\a.txt`），而 `svn info` 的 `wcroot-abspath` 用正斜杠 | 解析层必须处理混合分隔符；操作时保留原样，显示时归一化 |
| S10 | 提交时目录过期会失败：`svn: E155011: Directory ... is out of date` | SVN 自身强制「先 update 再 commit」，与既有「写前重查」规则方向一致但更严格 |
| S11 | 交互模式下 `svn` 可能等待输入而挂起（本次探测中曾导致命令超时） | 所有调用**必须**带 `--non-interactive`，否则会卡死 UI 线程 |
| S12 | Windows 配置目录是 `%APPDATA%\Subversion`，**不是** `~/.subversion` | 测试隔离必须设 `APPDATA`，只隔离 `HOME` 在 Windows 上无效 |
| S13 | `svnversion` 不接受 `--non-interactive` | 混合版本判断改用 `svn info` 组合，不依赖 `svnversion` |
| S14 | `svn --version` 的 WC 格式为 31/32 | 能力探测看 WC 格式与 RA 模块，不写死版本号 |

## 替代项

- **只做 Git，不引入 SVN。** 维护成本最低，但不满足需求。
- **把 SVN 塞进现有 `gitcli.Runner`，用 `Kind` 分支。** 改动看似最小，但 86 个方法里约半数（rebase/stash/worktree/reflog/submodule）SVN 完全没有对应物，会产生大量空实现与运行期分支；且违反「接口围绕用例划分、避免巨型接口」的既有约定。
- **为大接口统一加 `ErrNotSupported`。** 相比上一项只是把分支换成错误，仍要求 SVN 实现 40 个无意义方法，没人愿意维护也无人敢删。
- **用第三方纯 Go SVN 库替代 CLI。** 与 [0001](0001-git-cli.md) 拒绝 go-git/libgit2 的理由相同：认证、`svn:ignore`、属性、冲突语义与官方实现一致的成本高，且 WC 格式私有。
- **为 SVN 另开一个独立应用。** 放弃共享的状态、diff、冲突、可观测性与 UI 资产，日常使用要开两个程序。

## 决定

### 1. 新增契约层 `internal/vcs`，依赖方向只增一条边

```
ui → app → vcs → domain
              ↑
        gitcli / svncli（各自实现）
              ↑
          cmd/viff（组合根，探测后选择后端）
```

`internal/vcs` 只依赖 `domain` 与标准库：不导入 Viva、不起子进程、不读 `.git`/`.svn`。这与 `domain` 同一档约束。

选择后端**只发生在组合根一处**，`app` 不感知具体实现。

### 2. 接口按用例切分，可选能力用 nil 表示「不支持」

必备能力作为 `NewWorkspace` 的构造参数：

```go
type Backend interface {
	Opener          // Discover / Init / Checkout(Clone)
	Reader          // Status / Operation / Conflicts / Refs / Remotes / OpenLog / CommitDetail / Read / Blame / DiffNames
	WorkspaceWriter // 改工作副本：标记增删、Discard、Revert
	CommitWriter    // Commit（含部分提交）/ LastCommitMessage / CreatePatch / ApplyPatch
	Syncer          // Fetch / Pull(PullUpdate) / Push(Commit) / Update
}
```

可选能力挂在结构体字段上，`nil` 即不支持：

```go
type Capabilities struct {
	RefWriter       RefWriter        // 分支创建/切换/删除；SVN 可由 switch 部分实现
	HistoryWriter   HistoryWriter    // Git: merge/rebase/cherry-pick/reset；SVN: nil
	Stash           StashService     // SVN: nil
	Worktrees       WorktreeService  // SVN: nil
	Submodules      SubmoduleService // SVN: nil
	Reflog          ReflogService    // SVN: nil
	ConflictResolve ConflictResolver
	Indexable       bool             // 有暂存区（Git true，SVN false）
	PropertyEdits   bool             // 支持属性编辑（SVN true）
}
```

理由：nil 判断可在编译期穷尽，比运行时 `ErrNotSupported` 清晰；避免 `svncli` 出现 40 个空方法。同时**构造函数签名不变**——`*gitcli.Runner` 天然满足必备接口，现有 20 余处 `NewWorkspace(&gitcli.Runner{...})` 测试调用无需改动。

### 3. 共享值类型与错误分类下沉，用类型别名分步迁移

`app` 依赖的 40 余个 `gitcli.*` 类型（`CommitOptions`、`PushOptions`、`ResetMode`、`Content`、`Revision`、`LogFilter`、25 个以上哨兵错误、`KindOf`/`CodeOf`）下沉到 `vcs`。迁移期在 `gitcli` 留别名：

```go
type ErrorKind = vcs.ErrorKind
var ErrConflict = vcs.ErrConflict
```

这样 79 处调用点继续编译，再按文件逐步改为 `vcs.`，最后删别名。分步而非一次改完，是因为错误码被 `docs/observability.md` §7 与诊断脱敏引用，一次性改动无法定位回归。

### 4. 仓库身份：`domain.Repo` 区分后端

```go
type Kind uint8 // KindGit | KindSVN

type Repo struct {
	Root      string
	Kind      Kind
	MetaDir   string // .git 或 .svn；替代原 GitDir
	CommonDir string // 仅 Git worktree 有意义；SVN 为空
	// 以下仅 SVN 使用：仓库身份由 UUID + 相对 URL 确定，而不是路径
	URL      string // 工作副本根对应的仓库 URL
	RelURL   string // ^/trunk
	RepoRoot string // 仓库根 URL
	UUID     string
}
```

- `GitDir` 引用 19 处、`CommonDir` 16 处、`domain.Repo{}` 构造 10 处，改名集中在少量文件。
- 身份用 `UUID + RelURL`，因为工作副本可被移动，路径不是身份。
- `writeLock` 的键改为 `repo.LockKey()`：Git 返回 `CommonDir`，SVN 返回 `Root`（无共享引用，按工作副本串行）。
- `platform.Watch` 内部硬编码的 `.git` 改用 `repo.MetaDir`。

### 5. 状态模型映射

`StatusEntry` 的 `Index`/`Worktree` 两列保留（Git 语义），SVN 映射为单列：`Index` 恒为 `'.'`，`Worktree` 取 `wc-status` 的 `item` 首字母。新增字段：

```go
// Props 是非 Git 后端的属性变更（'M'/'C'/' '）。Git 恒为 ' '。
Props byte
```

`item` → 字母映射：`modified`→`M`、`added`→`A`、`deleted`→`D`、`missing`→`!`、`unversioned`→`?`、`conflicted`→`C`、`ignored`→`I`、`normal`→` `、`replaced`→`R`、`external`→`X`。

**必须修**：`domain.StatusEntry.Conflicted()` 当前只认 `'U'` 与 `AA`/`DD`，不认 `'C'`。不改则 SVN 冲突文件会被当成普通修改，落在错误的列表里。

按 S7，`*.mine`/`*.r2`/`*.r3` 必须从 `unversioned` 列表中过滤，否则未跟踪列表出现垃圾项。

### 6. 历史：显式区间 + 持久缓存 + 显式陈旧态

- 查询一律用 `svn log --xml -r 1:HEAD <URL>`（S1），分页用 `-l`。**禁止**不带 `-r` 的默认调用。
- SVN 历史是线性序列，`domain.Commit.Parents` 只放**该路径日志中的前一条**，不得合成 `revision-1`——仓库修订号是全局自增，路径日志会出现 r3、r7、r9 这样的跳号。
- `domain/graph.go` 对 SVN 退化为单车道渲染，不引入第二套绘图算法。
- 缓存落 `internal/storage`，带 schema 版本、字节配额、LRU 淘汰、TTL；损坏可重建，不阻止打开仓库。
- 视图必须能表达三态：`Live`（刚查）、`Cache`（未核对）、`Stale`（超时/离线）。复用并扩展 `RepoState.StatusTime`。**离线时显示缓存必须显式标注**，不能冒充最新。

### 7. 写操作

- **全部调用带 `--non-interactive`**（S11）。冲突交给 `--accept postpone`，产生冲突文件后由只读冲突视图展示（延续 [0005](0005-read-only-conflicts.md)）。
- 提交用**显式目标集**替代暂存区（S4）：`svn commit <paths...>`。目标集由 UI 复选框选择，不复用 ChangeList 机制，避免与 [0020](0020-repositories-changelists.md) 的两套语义混淆。
- 提交前先 `svn update` 或检查 `E155011`（S10）：SVN 自己就会拒绝过期提交。失败后**重新查询状态并提示用户**，不自动重试。
- 取消 ≠ 撤销：提交是原子远程操作，中止进程后必须重新查询工作副本，明确是否已提交，不自动重试。
- 无本地提交，因此 `Ahead` 恒为 0；「有远程更新」由 `BASE` 与 HEAD 修订号比较得出。

### 8. 错误分类

`ErrorKind` 增加 SVN 需要的分类，映射基于 stderr 的 `E` 编号（S5，退出码不可靠）：

| 实测 E 码 | 含义 | 映射 |
| --- | --- | --- |
| `E155007` | 不是工作副本 | `ErrNotRepository` |
| `E155011` | 提交/操作时目录过期 | 新增 `ErrOutOfDate` |
| `E170013` / `E180001` | 无法连接仓库、无法打开 | 新增 `ErrNetwork` |
| `E160006` | 不存在的修订版 | 新增 `ErrNoRevision` |
| `E170001` | 认证失败 | `ErrAuth` |
| `E155004` | 工作副本被锁 | `ErrLocked` |

未实测的编号必须在实现时用真实命令补齐测试后再入表，不凭文档猜测。每个新增分类都要有稳定错误码（`docs/observability.md` §7）。

### 9. 功能矩阵（日常客户端范围）

| 能力 | Git | SVN |
| --- | --- | --- |
| 检出/打开、状态、diff、blame、文件内容 | 有 | 有 |
| 历史（线性） | 有（DAG） | 有（线性，需缓存） |
| update / commit（含部分提交）/ revert / add / delete | 有 | 有 |
| 分支（切换、创建、删除） | 有 | 有（目录 + `switch`，语义不同） |
| 属性编辑（`svn:ignore`、`svn:eol-style`） | 不适用 | 有 |
| lock / unlock | 不适用 | 有 |
| 冲突只读展示 + 标记已解决 | 有 | 有（复用 markers 解析） |
| **rebase / cherry-pick / reset / 交互式 rebase** | 有 | **不适用**（无提交改写） |
| **stash** | 有 | **不适用**（`revert` 不是 stash） |
| **worktree / reflog / submodule** | 有 | **不适用** |
| **暂存区（部分暂存）** | 有 | **不适用**（改用提交目标集） |

表中「不适用」不是暂缓，是 SVN 模型没有对应物；对应入口按 `Capabilities` 隐藏，而不是显示后报错。

## 代价

- 新增一层抽象与一套后端，`internal/svncli` 的读取子集（status/log/diff/blame/属性）解析工作量与 Git 侧同量级。
- 历史必须缓存，引入缓存失效、配额、schema 迁移与「陈旧态」展示，这是 Git 侧没有的复杂度；错误处理不当会让用户看到过期数据。
- 状态与应用层需要同时承载两套语义（双列/单列、DAG/线性、Ahead-Behind/仅 Behind），测试矩阵扩大，CI 需增加安装 `svn` 的分片。
- 依赖本机 SVN 可执行文件（设置里可配路径），与 Git 一样成为运行时依赖；未安装时功能需明确降级而不是崩溃。
- 双语文档、README 定位（「Git 桌面客户端」）、`docs/features.zh-CN.md` 已知限制都需同步改动。
- 依赖 SVN 的 auth cache（`%APPDATA%\Subversion\auth`、`~/.subversion/auth`），Viff 不自建凭据存储；凭据交互失败时的提示质量受 SVN 自身输出限制。

## 复审条件

- SVN 历史缓存出现用户可见的错误（展示过期数据且未标注、或缓存污染导致状态错误）时，重新评估是否保留该缓存层。
- `internal/vcs` 接口数量持续增长、或出现第三个后端时，重新划分接口边界，而不是继续加可选字段。
- SVN 的日常使用中「无暂存区」被证实影响效率时，重新评估提交目标集的交互方式。
- `svn` 未安装或版本过低成为主要支持负担时，重新评估是否随包分发或收窄支持范围。
- 网络往返成为可测量的交互瓶颈（需给出平台、版本与测量命令）时，再讨论预取策略。

## 实施阶段

| 阶段 | 内容 | 完成判据 |
| --- | --- | --- |
| P0 契约化 | 新增 `internal/vcs`；`type Git = *gitcli.Runner` 换成 `Backend` + `Capabilities`；错误类型下沉留别名；`domain.Repo` 加 `Kind`/`MetaDir` | **行为零变化**，`go test ./...` 与 `vivahead` 无头套件全绿，`gofmt`/`go vet` 干净 |
| P0.5 能力位贯通 | `Indexable`/`PropertyEdits` 等从 app 透传到 UI；修 `Conflicted()` 认 `'C'`；`writeLock` 改 `LockKey()`；`platform.Watch` 用 `MetaDir` | 单测覆盖能力位到渲染分支；Git 行为不变 |
| P1 SVN 只读 | `internal/svncli`：Discover/Status/Info/Log(含缓存)/Cat/Blame/Diff；缓存进 `storage`（schema 升版） | 本地 `file://` 仓库可打开、看历史与 diff；断网走缓存且显式标注陈旧 |
| P2 SVN 写入 | update/commit(含部分)/revert/add/delete/switch/lock/propset | 每个写操作都有失败与中断后重查状态的测试 |
| P3 文档与发布 | README 双语定位、`docs/features.zh-CN.md`、`docs/design.md`、本 ADR 状态更新、兼容矩阵、错误码表 | 双语一一对应；链接可达；发布说明通过 `scripts/release_notes.py` |

P0 的价值独立于 SVN：它修掉「app 依赖具体适配器」这一既存缺陷，即使 SVN 最终不做也应完成。

## 测试约定

- 用 `svnadmin create` 建临时仓库，经 **`file://` URL** 访问（Windows 下必须是三斜杠形式 `file:///C:/path`，两斜杠会被解析成主机名 `c` 而失败），满足「不访问公网、不依赖登录态」。
- 隔离 `APPDATA`（Windows 真实配置目录，S12）与 `HOME`/`XDG_CONFIG_HOME`；不得读写用户真实 SVN 配置与凭据。
- 所有测试调用带 `--non-interactive`，避免挂起（S11）。
- 未安装 `svn` 时相关测试 `t.Skip`，但 CI 必须有一个**强制执行** SVN 套件的分片，否则会和被跳过一起掩盖回归。
- 断言状态与字段值，不只依赖截图；冲突场景断言 `C` 项与 `.mine/.r2/.r3` 过滤结果。
