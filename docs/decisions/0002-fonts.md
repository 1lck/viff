# 0002 界面字体与等宽字体

- 状态：已采用（2026-11）

## 背景
IntelliJ 新 UI 的界面字体是 Inter，编辑器字体是 JetBrains Mono。Viva v0.2.3 只能通过平台字体系统查找字体族。

## 决定
- 界面文字使用平台系统字体（macOS 上是 SF Pro）。它的度量接近 Inter，避免再打包、注册一套界面字体。
- 打包 JetBrains Mono（OFL 许可，见 `internal/platform/fonts/OFL.txt`），用于 diff、日志哈希等等宽内容。macOS 上以进程范围注册，不会安装到系统。
- 提交信息输入框使用界面字体，与 IDEA 的默认设置一致。

## 代价
界面文字与 IDEA 并非逐像素一致，字宽差异约 1–3%。

## 复审条件
Viva 支持从内存加载字体，或需要在三个平台上统一字体度量时，改为打包 Inter。
