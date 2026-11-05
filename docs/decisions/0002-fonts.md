# 0002 界面字体与等宽字体

- 状态：已采用（2026-11）；2026-11 增补可选界面字体

## 背景
IntelliJ 新 UI 的界面字体是 Inter，编辑器字体是 JetBrains Mono。Viva v0.2.3 只能通过平台字体系统查找字体族；v0.2.4 增加了 `app.FontFamilies` 与 `app.SetInterfaceFontFamily`，界面字体因此可选，项目随之升到 v0.2.4。

## 决定
- 界面文字使用平台系统字体（macOS 上是 SF Pro）。它的度量接近 Inter，避免再打包、注册一套界面字体。
- 打包 JetBrains Mono（OFL 许可，见 `internal/platform/fonts/OFL.txt`），用于 diff、日志哈希等等宽内容。macOS 上以进程范围注册，不会安装到系统。
- 提交信息输入框使用界面字体，与 IDEA 的默认设置一致。
- 设置页提供「界面字体」选择：`app.FontFamilies()` 列出系统已安装的字体族，选中的族交给 `app.SetInterfaceFontFamily`，`""` 表示平台默认。该项是进程级的，只影响 Viva 自己绘制的比例文字；系统绘制的菜单、文件对话框等仍用系统字体，重绘由应用自己请求。
- 选中的族同时出现在 `app.MonospaceFamilies()` 中时，diff 与控制台也改用它；否则保持内置 JetBrains Mono，因为等宽内容依赖字符网格。

## 代价
界面文字与 IDEA 并非逐像素一致，字宽差异约 1–3%。改选非等宽族时，比例文字的度量随族变化，固定宽度的面板与列需要按新字体回归视觉基准。

## 复审条件
Viva 支持从内存加载字体，或需要在三个平台上统一字体度量时，改为打包 Inter。
