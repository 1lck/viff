# 0038 在 Windows 版直接打开 WSL 仓库

- 状态：已采用（2026-11）

## 背景

Windows 版 Viff 原先只能打开 Windows 本地路径的仓库。用户希望直接打开 WSL 发行版内已有的仓库（`\\wsl.localhost\<发行版>\...`，旧写法 `\\wsl$\...`）。初步检查确认现状没有 WSL 执行与路径适配，而且**仅放宽目录选择器不够**：

- Windows 版 Git 经 9P 打开 `\\wsl$` 仓库会做 CRLF/换行转换（`core.autocrlf` 在 Windows 安装器里默认开启），`core.filemode` 也按 Windows 语义处理；同一个 index 被两套语义写过之后，Linux 侧看到的是错误的文件模式与行尾。
- 9P 的往返延迟让每次 `git status` 变成数十次远程过程调用，刷新成本比本地磁盘高一个数量级。
- 用户在 WSL 里配置的凭据、SSH agent、签名密钥都在 Linux 侧；Windows Git 看不到它们，表现为“同一个仓库在终端能提交、在 Viff 里不能”。

因此决定：**WSL 仓库的每一条 Git 命令都改由发行版内自己的 git 执行。**

### 本机实测事实

测量环境：Windows 11 Pro 10.0.26300，WSL2，Ubuntu，发行版内 `git 2.53.0`，Windows 侧 `git 2.56.0.windows.4.niugit`。

| 问题 | 实测结果 | 对设计的影响 |
| --- | --- | --- |
| 杀掉 `wsl.exe` 后 Linux 侧进程 | 跟随退出（`sleep 30` 在 Stop-Process 后不再存在） | 取消语义可原样映射到 `context` 取消，无需额外清理手段 |
| 发行版不存在 | 退出码 255，stderr 含 `Wsl/Service/WSL_E_DISTRO_NOT_FOUND` | 用语言无关标记分类，不用本地化文案 |
| WSL 未安装 | stderr 含 `Wsl/Service/WSL_E_WSL_NOT_INSTALLED`，或 `wsl.exe` 不在 PATH | 同上 |
| 发行版内无 git | 退出码 1，stderr 含 `execvpe(git) failed: No such file or directory` | 同上 |
| `wsl.exe` 自身诊断的输出编码 | 写管道时为 **UTF-16LE 且无 BOM**；`WSL_UTF8=1` 后为 UTF-8 | 命令显式设 `WSL_UTF8=1`，另留无 BOM 启发式兜底 |
| `--exec` 的子进程输出 | 原字节透传（Linux git 的 UTF-8 原样到达） | 只有 `wsl.exe` 自身的消息需要解码 |

## 替代项

- **Windows git 直接打开 UNC 路径。** 不改执行层，改动最小；但 CRLF/文件模式语义、9P 性能、跨系统 index 与锁文件三类问题都在，且无法使用 Linux 侧凭据。否决。
- **每条命令用 `wslpath` 转换路径。** 能省掉自己写转换，但每条 Git 命令要多起一个进程（`wsl.exe ... wslpath ...`），在 9P 上把延迟翻倍；转换规则本身是纯词法的。否决。
- **保留 `\\wsl$` 原样、按输入拼写区分仓库。** `\\wsl$` 与 `\\wsl.localhost` 是同一文件系统，按拼写区分会让最近项目重复、写锁 key 分裂到两个互不相斥的锁上。否决。
- **通过 SSH 转发到发行版执行 Git。** 需要发行版内另起 sshd，超出“打开本机仓库”的范围，且新增常驻服务。否决。
- **只支持 `\\wsl.localhost`、不支持 `\\wsl$`。** 用户从文件管理器复制到的往往是旧写法；归一化成本很低。否决。

## 决定

### 1. 双路径域，转换收口在 gitcli

| 路径域 | 形态 | 使用者 |
| --- | --- | --- |
| 身份域 | `\\wsl.localhost\<distro>\<path>`（归一化） | `domain.Repo.Root/GitDir/CommonDir`、`app`、`ui`、`platform`、`storage`、最近项目去重、写锁 key |
| 执行域 | `/home/user/repo` | 只存在于 `internal/gitcli` 边界：`--cd` 参数、Git 输出的绝对路径、传给 Git 的绝对路径参数 |

`app`/`ui`/`platform`/`storage` 的路径语义零改动：`os.*` 对 `\\wsl.localhost` 经 9P 重定向器透明工作，文件树、diff 读取、冲突文件读写、`.gitignore` 写入、`explorer /select` 都直接可用。

转换点（新增 `internal/wslpath` 纯词法包，`internal/gitcli/wsl.go` 收口）：

- **入（cwd）**：`Runner.command` 是唯一的进程构造点。dir 为 WSL 路径时包装为
  `wsl.exe -d <distro> --cd <linuxDir> --exec git <baseArgs> <cfg.args> <args>`。
  `--exec` 走 execv 不经 Linux shell，参数数组纪律与注入面不变；`process.HideConsoleWindow` 照常生效。
- **入（绝对路径参数）**：worktree 增删、`clone` 目标、rebase/rewrite 的 `sequence.editor`/`core.editor`（`cp <绝对路径>`）。同发行版 UNC → Linux 路径；`C:\x` → `/mnt/c/x`；已是 Linux 绝对路径或相对路径 → 原样。
- **出**：`rev-parse --show-toplevel/--git-dir/--git-common-dir` 与 `worktree list --porcelain` 的绝对路径经 `FromLinux` 转回身份域；相对路径输出（porcelain v2、`log -z`、`diff -z`）不转换，维持“NUL 分段路径按原始字节保留”。

### 2. 环境、编码与错误分类

- 显式设置 `WSL_UTF8=1`；固定 Git 环境经 `WSLENV` 转发（`GIT_TERMINAL_PROMPT`、`GIT_OPTIONAL_LOCKS`、`LC_ALL`、`LANGUAGE`、`GIT_SSH_COMMAND`），用户已有的 `WSLENV` 条目与标志位保留。
- 新增三个稳定错误码，按语言无关标记分类：`wsl.unavailable`、`wsl.distro_missing`、`wsl.git_missing`。设置项 `GitPath` 不作用于 WSL 仓库。
- 能力探测（`wsl.exe -d <distro> --exec git --version`）收口在 `Discover`，成功结果按发行版缓存、失败不缓存。

### 3. 复用既有机制

- 取消：沿用 `context` 取消杀进程；写操作结束（含取消）后 `Refresh(true)` 重查。
- 写锁：key 为身份域 CommonDir，两种别名归一化后天然同 key。
- 去重：`app.canonicalPath` 对 WSL 路径改走 `wslpath.Normalize`（别名、发行版大小写归一），跳过 `EvalSymlinks`（9P 上慢且 Linux 符号链接语义不同）。
- LFS 能力探测按仓库上下文（`wsl:<distro>`）缓存，避免把 Windows 侧 `git-lfs` 结果用于 WSL 仓库。
- watcher：WSL 仓库轮询间隔从 2 秒放宽到 5 秒（9P 往返成本），间隔本身是 `startPolling` 的参数，不做新机制。

## 代价

- WSL 仓库的文件 IO 全部经过 9P，diff、文件树、状态刷新比本地磁盘慢一个量级；本轮不做缓存层，靠既有的刷新合批与代次丢弃控制，**没有做性能测量**，间隔 5 秒是保守值而非实测最优。
- 每次 Git 命令多一个 `wsl.exe` 进程（约几十毫秒）；只读刷新本来就是后台任务，未做批量合并。
- 发行版内 git 版本与 Windows 侧可能不同（实测 2.53.0 对 2.56.0），某些选项在这两个版本上行为不同时，WSL 仓库以发行版内的行为为准；能力探测目前只判断“能跑 `--version`”，没有按版本分支。
- 内置终端仍以 Windows 侧 `cmd.exe` 打开，`cmd.exe` 在 UNC 工作目录下会退化到 `C:\Windows`；WSL 仓库的终端体验未经适配（后续 issue）。
- 内容搜索的 `rg` 分支仍用 Windows 侧 `rg.exe` 读 9P，能用但慢；`git grep` 分支自动走 WSL。
- 转换要求仓库位于发行版文件系统或 `/mnt/<盘符>`；同一发行版之外的 UNC（例如另一个发行版）不转换，由 Git 报错。
- 非 Windows 平台上 `wslpath` 仍会识别这类字符串并返回 `wsl.unavailable`；正常路径不会触发。

## 复审条件

- WSL 版本演进改变以下任一实测行为时，重跑 `internal/gitcli/wsl_windows_test.go` 并更新本 ADR：杀 `wsl.exe` 后 Linux 进程是否跟随退出；错误标记字符串；管道输出编码。
- 发行版内 git 与 Windows git 的版本差异导致同一操作结果不一致（例如某个选项新版本才有）时，把能力探测从“能跑”扩展为按选项探测。
- 在真实 WSL 仓库上做性能测量后，重新确定 watcher 间隔；如果 diff/文件树成为可感知瓶颈，再评估缓存层，并单独记录。
- 内置终端要支持 WSL 仓库时，另起 ADR 说明是转发到发行版 shell 还是继续用 Windows shell。
- 若需要支持“同一 Windows 进程打开多个发行版”，检查 `wslReady` 缓存与 `WSLENV` 合并是否仍按发行版隔离。
