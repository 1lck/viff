# 0041 提交时间的时区与时钟格式

- 状态：已采纳
- 日期：2026-11-07
- 相关：[#65](https://github.com/1lck/viff/issues/65)、[#66](https://github.com/1lck/viff/issues/66)
- 触及领域：`internal/svncli`、`internal/gitcli`、`internal/ui`、`internal/storage`

## 背景

用户报告 Windows 上的 SVN 仓库提交时间"用了 12 小时制且没有 AM/PM 标记"：列表显示 `Today 04:51`，而那条提交实际发生在当地 12:51。排查后确认这不是格式偏好：

- `svncli` 用 `time.Parse(time.RFC3339Nano, ...)` 解析 `svn log --xml` 的时间戳，得到的是 UTC，之后从未转换；
- Git 侧 `time.Unix(t, 0)` 返回的是本地时间；
- 界面直接 `Format("15:04")`，于是 SVN 仓库比 Git 仓库早/晚一个月时区，且 `ago()` 用 `YearDay()` 比较两个不同时区的时间，把"今天"判成"昨天"。

同一个 issue 还报告了中文提交信息乱码。取证结论是：乱码字节在仓库里就已经是错的（客户端用遗留编码写入后又被当作 UTF-8 提交），Viff 没有也不可能把 UTF-8 当 GBK 解——它没有任何字符集转换代码。但 `encoding/xml` 遇到非法 UTF-8 会拒绝整份文档，导致一条坏 revision 让整个日志视图报错，这是真实缺陷。

## 决定

1. **时间戳在解码边界转到本地时区。** `svncli.parseTime` 返回 `date.Local()`，与 Git 侧已有的本地时间对齐；`ago()` 在比较前把两个时间统一到 `now` 的时区，日历日判断因此只在同一时区内进行。
2. **新增 `timeFormat` 设置**（`storage.TimeFormat24Hour` 为默认，空值等价于它；`storage.TimeFormat12Hour` 为可选）。布局集中在 `Window.clockLayout()` / `consoleClockLayout()`，提交列表、提交详情、blame 提示、通知与命令控制台共用，日期部分不随格式变化。
3. **解析层对非法 UTF-8 宽容。** `svncli` 在 `xml.Unmarshal` 前把非法字节替换为 U+FFFD（`unmarshalXML`），`gitcli` 对提交文本字段做同样处理（`validUTF8`）。损坏的字符只损失自己，不再让整条命令失败，也不会把非法字节带进诊断包与事件日志。
4. **最近项目上限收敛到 `storage.MaxRecentProjects`（20 → 50）**，删除 `AddRecent` 里重复的硬编码；欢迎页搜索在有查询词时同时覆盖工作区仓库列表（`workspace.json`），因为 Recent 只是便利列表，工作区才是用户显式添加的完整集合。

## 替代项

- **域层统一存 UTC、界面统一转换。** 语义更干净，但要同时改 Git 路径、所有比较与排序点，并新增一层"何时转换"的约定；当前 Git 与 SVN 在解析处就统一更省事，风险也更小。若将来出现跨时区排程或时间线聚合，再按此方案重做。
- **跟随操作系统的 12/24 小时设置。** 需要在 `platform` 里分别探测 Windows 与 macOS 的区域格式，属于独立改动；当前提供 24/12 两档显式选择。
- **自动修复仓库里已双重编码的乱码。** 需要 `golang.org/x/text`（引入依赖要更新 `licenses/` 与 notices）并做往返校验，且必须先确认目标仓库的字节形态。留待单独 issue 处理，见下。
- **欢迎页列表默认展示全部工作区仓库。** 保留"最近项目"的空查询语义，避免把欢迎页变成第二个仓库总览；搜索是这次要解决的入口。

## 代价

- 界面显示的是用户本地时间；跨国协作者看到的是各自本地时间，与 `git log` 的默认行为一致。
- 非法 UTF-8 被替换成 U+FFFD 后无法还原，原字节也不会保留在日志或诊断包里——这是有意的隐私与健壮性取舍。
- 12/24 小时是全局设置，不区分仓库；同一用户在 SVN 与 Git 仓库看到同一个时钟格式。

## 复审条件

- 若引入需要绝对时间比较的功能（提交时间线聚合、多人协作时间线、按时间排程），改为域层统一 UTC。
- 若用户要求时钟跟随系统，补 `platform` 的区域格式探测并把 `timeFormat` 扩展为三档（system / 24h / 12h）。
- 若确认某个仓库的乱码是遗留编码字节（`svn log --xml | xxd` 显示非 UTF-8）而非已双重编码的脏数据，再评估引入 `x/text` 做按仓库的编码修复；两种形态的修复方向相反，必须先取证。
