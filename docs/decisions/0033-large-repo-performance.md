# 0033 大仓库的 Log 首屏与提交详情

- 状态：已采用（2026-11）

## 背景

0.2 第 3 项要求十万提交、几千引用的仓库里 Log 首屏、翻页、提交详情和 diff 打开仍然可用，并且有可复现的测量。改动前 Viff 已经有两处正确的基础设施：`gitlc.LogReader` 用一个常驻 `git log` 进程按页读取（翻页不重新从头遍历），`domain.GraphLayout.Append` 跨页保留 lane 状态（图在页边界不断线）。问题出在它们的组合方式。

1. **首屏会卡在加载态。** `setRepo` 先启动 `Refresh(true)`，随即 `ReloadLog()` 读第一页。刷新完成得更早时，`Refresh` 的 `post` 发现 refs 指纹变化，又调用一次 `ReloadLog()`。旧实现里 `ReloadLog` 取消旧 reader、递增 `seq`，但没有清掉 `LogState.Loading`；新的 `LoadMoreLog` 看到 `Loading` 仍为 true 直接返回。旧 goroutine 的结果因 `seq` 过期被丢弃，于是 `Loading` 永远为 true、`Commits` 为空，界面停在 “Loading”。小仓库通常在刷新结果到达前就读完第一页，所以只在较大的仓库或较慢的磁盘上暴露。
2. **提交详情被 branch 查询拖住。** `SelectCommit` 在同一个 goroutine 里先 `CommitDetail`，再 `ContainingBranches`，最后一次性 `post`。`git for-each-ref --contains=<hash>` 在没有 commit-graph 的仓库里要为每个引用走一遍历史；在 10 万提交、4021 个引用的夹具上约 2.1 秒，而 `CommitDetail` 只要十几毫秒，详情面板于是等满了整个 branch 查询。

### 测量口径

同一台机器、同一份合成夹具，只比较改动前后的**同一条命令**：

- 机器：macOS 26.6.2，Apple M4，32 GB；`go1.27.3 darwin/arm64`；`git 2.53.0`。
- 夹具：`git fast-import` 现场生成，不提交、不联网，放在 `t.TempDir()`；默认 20,000 提交 / 3,000 引用，用 `VIFF_PERF_COMMITS` / `VIFF_PERF_REFS` 放大。
  - `internal/gitcli`：100,000 条主线提交 + 每 100 条插入一个 side 提交和 merge（共 101,000 提交），3,000 个本地分支、600 个轻量标签、300 个远端分支、120 个附注标签，共 4,021 个引用。
  - `internal/app`：100,000 提交、3,000 个本地分支，共 3,001 个引用。
- 仓库状态：默认**没有 commit-graph**（fast-import 后直接测），引用已 `git pack-refs --all` 打包；文件内容很小，测量的是 Git 遍历/进程开销，不是 diff 文本计算。
- 每个 benchmark 自己建临时仓库；`-benchtime=3x`，数字是单次运行的 `ns/op`。Git 进程受页缓存影响，同一命令重复运行会有 5%–20% 波动；表中同时给出两次未改动项的读数作为波动参照。
- 命令（`PATH` 省略本机 Go 工具链前缀）：

```sh
VIFF_PERF_COMMITS=100000 VIFF_PERF_REFS=3000 \
  go test -run '^$' -bench BenchmarkLargeRepo -benchtime=3x -benchmem ./internal/gitcli

VIFF_PERF_COMMITS=100000 VIFF_PERF_REFS=3000 \
  go test -run '^$' -bench BenchmarkLargeRepo -benchtime=3x -benchmem ./internal/app
```

### 结果

用户可见的两个等待：

| 场景（app 层） | 改动前 | 改动后 | 基准 |
| --- | --- | --- | --- |
| Log 首屏有第一页 | 卡在 `Loading`，等待条件 2 分钟后超时；20k 与 100k 夹具都复现过，但不是每次打开都触发 | 417–494 ms | `BenchmarkLargeRepo/LogFirstScreen` |
| 提交详情可见 | 1,873 ms | 8.3–8.4 ms | `BenchmarkLargeRepo/DetailOnly` |

`DetailOnly` 在旧实现下等价于“详情 + branch 查询”（同一次 `post`），1,873 ms 是直接对旧 `SelectCommit` 跑同一基准得到的；修复后同一基准只等详情，8.3–8.4 ms 为详情本身。`DetailAndBranches` 仍为 1.81–1.88 s，说明慢的是 branch 查询，且它已不在关键路径上。

Git 原语（`internal/gitcli`，两次运行的对照；本轮没有改这个包，差异属于波动）：

| 测量 | 第一次 | 第二次 | 备注 |
| --- | --- | --- | --- |
| 首屏 400 条 | 325 ms | 317 ms | `--date-order` 在无 commit-graph 时全量遍历 |
| 101k 提交全量翻页 | 525 ms | 448 ms | 首屏之后约 250 页共 ~200 ms，翻页本身已增量 |
| refs（4,021） | 42 ms | 38 ms | `for-each-ref` 带 subject/date |
| `CommitDetail` | 14.3 ms | 8.6 ms | 两个 Git 进程 |
| `ContainingBranches` | 2,125 ms | 2,219 ms | `--contains` 全历史 |
| `ReadFileAtCommit`（diff 一侧） | 14.5 ms | 15.6 ms | 每侧 4 个 Git 进程 |

## 替代项

- **把 `--date-order` 换成默认顺序。** 实测默认顺序首屏 3.8 ms，`--date-order` 270 ms，但默认顺序不保证父提交在子提交之后。现有 lane 布局按“子先出现”设计，时钟偏移的历史会出现悬挂 lane；这属于语义变化，不采用。
- **改用 `--topo-order`。** 与 `--date-order` 一样要全量遍历（实测 269 ms），没有收益。
- **由 Viff 写 commit-graph。** 会向用户的 `.git/objects/info` 写缓存文件，超出“只读查询不改仓库”的边界；有 commit-graph 的仓库 Git 会自动加速，属于用户/仓库侧的选择，不采用。
- **缓存 `for-each-ref` 结果。** 4,021 个引用约 40 ms，省下的绝对时间有限，却要为 refs 失效再维护一层缓存；本轮不做。
- **branch 查询只查本地分支 / 直接不显示。** 语义变化，用户会少看到远端分支信息，不采用。
- **并行做详情和 branch 查询但仍等两者。** 关键路径仍是 2.1 s，满足不了“详情先可用”。
- **合并 `Read` 的 4 个 Git 进程（`cat-file -t/-s/blob` + `ls-tree`）。** 单侧约 14.5 ms，双侧约 30 ms；收益小于详情问题，且要重写 batch 协议解析，本轮不做。

## 决定

1. `ReloadLog` 在取消旧 reader 后显式把 `Loading` 置 false，再启动新的 `LoadMoreLog`；旧结果继续由 `seq` 丢弃。清空选择（`SelectCommit("")`）同时复位 `DetailLoading`，避免同类卡住。
2. `SelectCommit` 拆成两段：`CommitDetail` 完成就 `post` 详情；同一个带超时的 context 继续查 `ContainingBranches`，完成后再单独 `post`。新增 `detailSeq` 丢弃同一 commit 的迟到结果（A→B→A 也不会被旧结果覆盖），`detailCancel` 在切换选择、切仓库和关闭时取消慢查询。
3. 新增可复现基准：
   - `internal/gitcli/perf_bench_test.go`：fast-import 夹具 + `BenchmarkLargeRepo`（首屏、全量翻页、refs、详情、`--contains`、diff 读取一侧），`TestSyntheticPerfFixture` 校验夹具形状。
   - `internal/app/perf_bench_test.go`：端到端 `BenchmarkLargeRepo`（首屏、`DetailOnly`、`DetailAndBranches`）。
   - 基准只在 `-bench` 下运行，普通 `go test` 不生成大仓库；夹具永远在 `t.TempDir()`。
4. 测试覆盖：`TestLogPaginationBoundaryAndGraph` 用 451 个提交跨页读取，并检查页边界两侧 lane 集合完全一致（merge 的第二父 lane 从第一页最后一行连到第二页第一行）；`TestReloadLogWhileLoading` 复现“加载中再次 Reload”的竞态；`TestSelectCommitClearStopsLoading` 覆盖清空选择。

行为保持：分页顺序与边界、merge 图连通、refs 标签、Log 过滤、外部修改后的 `Refresh`/`ReloadLog` 失效路径都没有改；改动只是让已取消的加载不再挡住新加载，并把 branch 查询移出详情的关键路径。

## 代价

- 详情面板会先显示正文，`In N branches` 稍后出现；UI 没有单独的“branch 加载中”状态，这段时间显示为空。最终数据与改动前一致。
- `ContainingBranches` 仍要消耗约 2 秒的 CPU/IO（后台执行）；快速连续选择时旧的 Git 进程会被取消，但取消能力和其他读命令一样是“停止进程并丢弃结果”，不会回滚任何东西。
- 新增三份测试/基准文件（约 600 行），需要维护 fast-import 流格式；100k 规模必须显式设置环境变量，默认 20k 以免拖慢本地基准。
- 首屏在**没有 commit-graph** 的十万提交仓库仍需约 320 ms（Git 的 `--date-order` 全量遍历），本轮没有消除；有 commit-graph 的仓库会明显更快，但基准没有覆盖这种情况。
- 基准数字只代表这套合成历史和这台机器；不同 Git 版本、文件大小、引用分布与页缓存都会改变绝对值，不能当作所有仓库的承诺。

## 复审条件

- 真实十万提交仓库上首屏超过 1 秒，或再次出现 Log 永久 `Loading`：重跑 app 基准并检查 `ReloadLog`/`LoadMoreLog` 的取消路径。
- 在带 commit-graph 的仓库上重跑基准，确认首屏是否已不是瓶颈；如果 Git 以后为 `--date-order` 提供更便宜的增量顺序，重新评估首屏。
- 详情 branch 查询仍超过 1 秒且用户反馈明显卡顿时，考虑按需/缓存/限制范围，并单独记录语义变化。
- 连续快速选择提交出现可感知的 CPU 峰值时，评估在 `SelectCommit` 加去抖，而不是继续加大取消范围。
- 修改 `GraphLayout`、分页大小或 Log 过滤时，同时跑 `TestLogPaginationBoundaryAndGraph` 与 `internal/domain` 的图连通测试，并重跑本 ADR 的两条基准命令。
