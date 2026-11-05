---
name: viff-docs-adr
description: "写或改 Viff 文档、双语同步、ADR 与发布说明时加载；不负责代码实现、打包、CI 工作流与真正发版。"
metadata:
  version: "1.0.0"
---

# Viff 文档与 ADR 规范

## 何时使用

- 改 `README.md`/`README.zh-CN.md`、`docs/` 专题文档、`docs/features.zh-CN.md`。
- 新增或修订 `docs/decisions/NNNN-slug.md`，更新决策索引。
- 写 `docs/releases/vTAG.md` 发布说明。
- 只改文档时，确认 PR 能否命中 CI 的 docs-only 分类。

## 前置读取

1. `AGENTS.md` 的「文档规则」「提交与 PR」；全局硬约束不在此重复。
2. `docs/releases.md`、`docs/releases/TEMPLATE.md`（发布说明格式）。
3. 本次相关专题文档与最近 ADR；先 `grep -rn "<主题>" docs README*.md` 找全同步点。
4. `.github/pull_request_template.md` 的自查项。

## 必须遵守

- 发布说明是唯一强制「中文在前、单独一行 `---`、英文在后」的文档：标题、分类与链接结构由 `scripts/release_notes.py` 校验，分隔符按 `TEMPLATE.md` 保留。其他文档按下方既有布局各自同步，两侧标题顺序与含义一一对应，不各写一套内容。
- 沿用既有布局，不为统一而重排：`README.md`（英文）与 `README.zh-CN.md`（中文）是两份文件，标题一一对应；`CONTRIBUTING.md`、`SECURITY.md` 用 `## 中文` 与 `## English` 两段；`docs/testing.md` 按段落中英对照。
- 行为变化同步：`README.md` 与 `README.zh-CN.md` 的相关段落（安装、平台提示、体积口径等）、`docs/features.zh-CN.md` 的「功能」「已知限制」、对应专题文档，或新增 ADR；不能只改代码注释。
- ADR 文件名 `NNNN-kebab-slug.md`，编号 4 位递增；正文写「背景、替代项、决定、代价、复审条件」，状态行形如 `- 状态：已采用（2026-11）`；被替代只改状态并双向链接，正文不删；同步 `docs/decisions/README.md` 索引行。
- 链接用相对路径（如 `[design.md](design.md)`、`[0021](decisions/0021-observability.md)`）；外链给可访问的 HTTPS 地址；图片放 `docs/assets/` 或 `docs/screenshots/` 并用相对路径引用。
- 不复制版权文本；许可证原文放 `licenses/`，第三方组件登记到 `THIRD_PARTY_NOTICES.md` 并给来源链接；图标许可另见 `assets/icons/intellij/NOTICE.md`。
- 不写未核实的性能、内存、兼容性数字；实测必须写清平台、版本与命令/口径（参照 README 的体积参考段落）。
- 只改文档，不改 `.github/`、`scripts/`；需要动工作流或打包脚本时转 `viff-release-ci`。
- 取舍与延后项写进 ADR 或专题文档（含复审条件），不把「以后再改」留在代码注释、PR 描述或口头约定里。
- 文档与代码同一次交付：代码行为变化时，文档更新属于该改动的一部分，不是收尾工作；不在 PR 里留「文档待补」，确需分批时开 issue 并写明剩余项与复审条件。

## 步骤

### 改既有文档

1. 用 `grep`/`glob` 确认中文、英文与专题文档的所有出现位置。
2. 一次只改一个关注点；同步英文侧对应段落与标题，避免只改一侧。
3. 逐个 `test -f` 检查相对链接与图片目标存在。
4. 若改了平台支持或已知限制，同步 `README.md`、`README.zh-CN.md` 的相关段落与 `docs/features.zh-CN.md` 的「已知限制」。

### 新增或替代 ADR

1. `ls docs/decisions` 取最大编号加一（当前到 `0022`）。
2. 复制最近一条 ADR 的结构，写清五段；「代价」写真实维护成本，「复审条件」写可观察的触发条件。
3. 替代旧决策时：旧文件只改状态行（如 `- 状态：被 NNNN 替代（YYYY-MM）`）并互加链接，正文保留。
4. 在 `docs/decisions/README.md` 表格末尾加 `| [NNNN](NNNN-slug.md) | 决策标题 |`。
5. `grep -n '状态' docs/decisions/*.md` 复核状态与互链，`grep -n 'NNNN' docs/decisions/README.md` 复核索引。

### 写发布说明

1. 复制 `docs/releases/TEMPLATE.md` 为 `docs/releases/vTAG.md`，替换全部双花括号占位符并删除模板注释。
2. 按实际发布核实下载文件名与对比基线；改动类别为空时中英文同步删除该分类。
3. 每条一句话、一行 Markdown，不嵌套列表，不写内部类名、测试数量、原始日志或未核实贡献者。
4. 运行 `python3 scripts/release_notes.py docs/releases/vTAG.md --tag vTAG`，通过后再提交。
5. 说明需在打 tag 前进入 `main`；不自行打 tag。只改文档的 PR 会跳过 quality，因此发布说明最终还会被 `release.yml` 的 verify 再校验一次。

## 检查清单

- [ ] 中英文标题数量、顺序、含义一致；发布说明的 `---` 位置正确。
- [ ] 相对链接与图片路径存在，外链可访问。
- [ ] 行为变化已同步 README 两侧、`docs/features.zh-CN.md` 或 ADR。
- [ ] ADR 编号不重复、五段齐全、索引已更新、被替代项未删正文。
- [ ] 发布说明通过 `release_notes.py`，无 `{{ }}`、`TODO`、`TBD`。
- [ ] 无未核实数字、无复制版权文本、许可证登记在 `licenses/` 与 notices。
- [ ] 纯文档 PR 的文件全部落在 docs-only 分类内：`README.md`、`README.zh-CN.md`、`CONTRIBUTING.md`、`SECURITY.md`、`THIRD_PARTY_NOTICES.md`、`LICENSE`、`docs/*`、`AGENTS.md`、`CLAUDE.md`、`.agents/*`。

- [ ] 取舍与延后项有记录（ADR/文档 + 复审条件），没有无主 TODO。

## 常见错误

- 只改中文侧或英文侧，`release_notes.py` 报分类不匹配、标题乱序或基线不一致。
- 直接改写被替代 ADR 的正文，历史丢失；或漏加索引行。
- 发布说明保留占位符，或用 GitHub 自动生成的提交列表代替已提交说明。
- 写「快几倍」「占用多少 MiB」却没有平台、版本与测量命令。
- 改文档时顺手改 `.github/workflows` 或 `scripts/`，PR 不再命中 docs-only，触发全量 CI。

## 相关文件

- [AGENTS.md](../../../AGENTS.md)
- [docs/releases.md](../../../docs/releases.md)、[TEMPLATE.md](../../../docs/releases/TEMPLATE.md)
- [docs/decisions/README.md](../../../docs/decisions/README.md)
- [docs/features.zh-CN.md](../../../docs/features.zh-CN.md)
- [scripts/release_notes.py](../../../scripts/release_notes.py)
- [.github/pull_request_template.md](../../../.github/pull_request_template.md)
- [.github/workflows/ci.yml](../../../.github/workflows/ci.yml)
