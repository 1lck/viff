# 0050 勾选文件直接提交（工作树范围）

- 状态：已采用（2026-11）

## 背景

Issue [#84](https://github.com/1lck/viff/issues/84) 要求：在 Changes 里勾选文件、输入提交信息后，**Commit / Commit and Push…** 可以直接提交所选内容，不必先 Stage；未勾选的文件不进入提交；未勾选但已经暂存的内容必须保留；提交范围必须与勾选一致，并在界面上清楚表达。该 issue 关联已关闭的 [#82](https://github.com/1lck/viff/issues/82)。

#82 已经做了勾选与 Stage Selected，但提交仍然只取索引：按钮启用条件只看 `CommittableCount()`（Git 下等于已暂存文件数），勾选只决定批量操作范围。用户必须先点 Stage Selected 或逐行暂存，才能在提交里带上勾选文件；而“先 add 再提交整个索引”又会把未勾选的暂存内容一起提交，违反 #84 的保留要求。

Git 自身提供了 `git commit --only -- <pathspec>`：它按 HEAD 的树加上列出的路径的工作树内容构造提交，不改变未列路径的索引。这个语义正好对应“勾选文件是本次提交范围”，但需要先验证边界，再决定是否采用。

## 替代项

- **先 `git add` 勾选文件，再执行普通 `git commit`。** 实现最简单，但会把未勾选但已暂存的内容一起提交，直接违反 #84；提交失败、取消或钩子报错后索引还留下了用户没要求的暂存状态。不采用。
- **临时索引（`GIT_INDEX_FILE`）：从 HEAD `read-tree`，把勾选路径 `add` 进临时索引，再用它提交。** 能保留真实索引，但要自己处理 merge/cherry-pick 的父提交、amend、hooks 看到的索引、split index/fsmonitor/sparse-checkout 等差异；提交成功后还要把勾选路径的索引同步到新 HEAD，同时不能动未勾选内容。失败和取消路径更多，且容易在 Git 已内建 `--only` 的地方造第二套机制。作为 `--only` 不满足要求时的后备方案记录，不采用。
- **stash 未勾选内容 → add 勾选 → 提交 → stash pop。** 失败、冲突、钩子和取消都会把仓库留在 stash 流程中间，恢复冲突时还要用户手工处理；成本远高于 `--only`。不采用。
- **提交整个工作树（`commit -a`）。** 会带入未勾选文件，与 #84 相反。不采用。
- **勾选只作为“建议范围”，实际仍提交索引。** 界面和行为不一致，issue 明确要求范围与勾选一致。不采用。

## 决定

- **有勾选时，提交范围就是勾选文件当前的工作树内容。** 修改、新增/未跟踪、删除都直接提交，不需要先暂存。
- **没有勾选时保持旧行为。** Git 提交索引，Subversion 提交全部已调度改动；“提交全部暂存”的旧路径和精细暂存（单行 +/−、Stage All、块/行暂存、Stage Selected）都保留。
- **Git 实现用 `git commit --only`，不改用户索引中未勾选的部分。** 路径经 `--literal-pathspecs --pathspec-from-file=<临时文件> --pathspec-file-nul` 传入，保留空格、引号、`[`、前导 `-` 等原样字符；提交说明仍走 stdin（`--file=-`）。两者不能同时用 stdin，因此只有路径列表落到系统临时文件，命令结束立即删除。
- **未跟踪文件先记录 intent-to-add。** `--only` 对未知 pathspec 直接报错，所以提交前对本次勾选中的未跟踪路径执行 `git add --intent-to-add`；提交失败、取消或临时文件创建失败时，用 `git rm --cached -q` 把这些条目恢复为未跟踪。清理使用 `context.WithoutCancel` + 10 秒上限：取消写操作不等于撤销，不能把索引留在半途。
- **重命名按一个文件处理，两个路径一起提交。** Status 的 staged rename 入口带 `OrigPath`；勾选该行时把新旧路径都加入 pathspec，否则只会把新路径作为新增提交，把旧路径的删除留在暂存区。删除与未跟踪同样走工作树内容语义。
- **部分暂存的勾选文件提交工作树版本。** 例如索引里是 a2、工作树是 a3，提交 a3，索引随提交更新为 a3，不会留下 a2/a3 的夹层。这是用户可见的“范围与 index 不一致”，因此面板显示提示，并在点击提交时二次确认。
- **Amend 显式按勾选范围执行。** 勾选时用 `git commit --amend --only`：以 HEAD 的树为基础，只把勾选路径的工作树内容叠加上去，未勾选的暂存内容保持暂存，不会被静默扩大进 amended commit；没有勾选时沿用原来的 amend（提交索引）。
- **写前重查。** `app` 在提交任务开始、拿到写锁后重新查询 status 与 operation：勾选路径已不再变化、出现新冲突、或进入 merge/cherry-pick 时返回 stale/blocked，刷新后要求重新选择，不用界面里的旧行提交。
- **合并与 cherry-pick 中禁止部分提交。** Git 会报 `cannot do a partial commit during a merge` / `during a cherry-pick`；界面在有勾选时禁提交并提示取消勾选或先完成操作，不静默退化成“提交整个索引”。rebase、revert、`git am` 实测允许带 pathspec 提交，因此不额外禁止。
- **失败不推送。** UI 的 Push 只在 `Workspace.Commit` 的成功回调里打开；提交失败、钩子失败或取消都不会进入推送。推送失败沿用现有流程，此时本地提交已经完成。
- 范围说明、警告与确认文案走 `internal/i18n`（英文 + 简体中文）；`docs/features.zh-CN.md`、`README.md`、`README.zh-CN.md` 同批更新。

### 实测记录

2026-11，macOS arm64，`git version 2.53.0`，全部在 `GIT_CONFIG_NOSYSTEM=1`、临时 HOME 与 `t.TempDir()` 仓库中验证：

1. **保留未勾选暂存：** `git commit --only -m msg -- checked` 只提交 checked 的工作树内容；另一个文件无论只是已暂存，还是“已暂存 + 工作树又改过”，索引与工作树都保持原样。未勾选的未跟踪文件也仍是未跟踪。
2. **未跟踪路径：** 直接 `--only` 报 `pathspec ... did not match any file(s) known to git`；`git add --intent-to-add` 后可以提交，成功后索引记录新 blob。失败（pre-commit hook 拒绝）时索引留下 `.A` 条目，`git rm --cached` 可恢复为未跟踪；带空格/方括号的路径同样成立。
3. **删除：** 工作树删除的已跟踪文件直接提交为删除，不需要先 `git add -A`。
4. **无 HEAD：** 初始仓库对勾选的未跟踪文件先 intent-to-add，可以生成只含勾选文件的 root commit，其他未跟踪文件保持未跟踪。
5. **重命名：** staged rename 同时传新旧路径时提交为 rename（`R100`）；只传新路径会把旧路径的删除留在暂存区，因此界面按两个路径处理。rename 后又有工作树修改时提交新内容。
6. **部分暂存：** 提交工作树版本；提交后该路径的索引与 HEAD 一致，没有残留的已暂存半份。
7. **Amend：** `git commit --amend --only -m msg -- a` 把 a 的工作树内容写进 amended HEAD，未勾选的 b 暂存内容保持不动；勾选路径与 HEAD 无差异但只改说明时也能正常 amend。
8. **操作进行中：** merge（含冲突解决后）、cherry-pick 报 `fatal: cannot do a partial commit during a merge/cherry-pick`；rebase 冲突解决后、revert 冲突解决后、`git am` 中断后都允许带 pathspec 提交。因此只禁止前两者。
9. **无变化：** 勾选路径的工作树内容与该路径在 HEAD 一致（例如只有旧的暂存版本）时，`--only` 报 `no changes added to commit`，索引保持原样；提交失败，不会悄悄提交别的内容。
10. **消息与路径分开：** `--file=-` 与 `--pathspec-from-file=-` 不能同时读 stdin；改为路径写临时 NUL 文件、消息仍走 stdin，实测带空格路径提交正确。

## 代价

- 一次带勾选的提交在 `app` 预检和 `gitcli` 判定未跟踪路径时各查一次 status。大仓库上这是可感知的额外开销；换来的是写前重查与“不把未跟踪文件当 pathspec 错误”的可靠行为。若以后成为瓶颈，可让 UI 把一次新的 status 快照传给 app，但要保证它仍来自写锁获取之后。
- intent-to-add 会短暂改变索引；进程在 add 与 cleanup 之间被强杀时可能留下 `.A` 条目。下一次刷新能看到它，用户可以取消暂存；它不会让未勾选内容进入提交。取消路径已用 detached context 清理，但进程被 SIGKILL 无法纠正。
- 路径列表临时文件放在系统临时目录，包含仓库相对路径（不含文件内容），命令结束即删。崩溃可能留下空文件，属于操作系统临时目录清理范围。
- 勾选沿用现有的 `cm.sel`：单击行也会勾选该行，因此“点开一个文件看 diff”会把提交范围缩到这一行。范围行会明确显示“Working-tree commit of N checked file(s)”，但这是与旧版“点行只影响批量操作”的可感知差异；彻底分开“光标”和“提交勾选”需要重做选择模型，不在本 issue 范围内。
- 勾选文件同时有已暂存与未暂存改动时，已暂存版本在提交后被工作树版本取代，不能再从索引里找回；界面用二次确认说明这一点。
- merge/cherry-pick 中不能部分提交是 Git 的限制；Viff 只能禁按钮，不能提供等价能力。
- 子模块、sparse-checkout、split index 没有单独设计：它们仍走 `--only` 的标准路径，但本轮测试只覆盖普通文件、未跟踪、删除、rename、部分暂存与 amend。

## 复审条件

- Git 改变 `--only`、intent-to-add 或 `--pathspec-from-file` 的行为，或支持在 merge/cherry-pick 中做部分提交时，先补回归测试再调整禁用规则。
- 用户反馈“单击文件查看 diff 却缩小了提交范围”成为主要误操作来源时，把行光标与提交勾选拆成两套状态，并重新评估 #82 的整组/目录勾选交互。
- 带勾选提交在真实大仓库上的写前 status 查询被测量为明显卡顿时，改为复用写锁内的状态快照，同时保留残留变化检测。
- 出现子模块、sparse-checkout 或 split index 的范围错误时，为这些场景补临时仓库测试，并决定是在 UI 排除还是在 `gitcli` 特殊处理；不能用 `--force` 绕过。
- 用户需要“勾选文件只提交索引中已暂存的那一部分，而不带入工作树新改动”时，需要新的范围模型（例如逐文件选择 index/worktree），不能复用当前 `--only` 语义。
- 需要跨仓库或跨 worktree 一次提交多个勾选集合时，先按 [0020](0020-repositories-changelists.md) 与 [0012](0012-worktrees.md) 重新设计写队列，当前范围仍属于单个活动仓库。
