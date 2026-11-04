# 0004 图标来源与编译

- 状态：已采用（2026-11）

## 背景
界面需要尽量还原最新版 IntelliJ IDEA。IDEA 的新 UI 图标以 Apache 2.0 许可开源。

## 决定
- 从 intellij-community 的 `platform/icons/src/expui` 与 `platform/dvcs-impl` 原样复制所需 SVG 到 `assets/icons/intellij`，保留版权声明（见该目录的 NOTICE.md）。
- `tools/genicons` 用 Viva 的图标编译器把 SVG 转成 Go 值并提交到仓库，运行时不解析 SVG。
- 多色图标按填充色拆成多个部件（Part），每个部件分别着色，以支持明暗两套主题。
- 标签在日志与分支树中复用 `branchLabel` 形状并按引用类型着色，与 IDEA 日志的标签画法一致；名为 `tag` 的 expui 图标实际是 HTML 标签 `<>`，不再使用。

## 代价
新增图标要重新运行 `go generate ./internal/ui/icons`。

## 复审条件
Viva 支持运行时加载 SVG 时，可以改为按需加载。
