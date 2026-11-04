# 0003 文件监听

- 状态：已采用（2026-11）

## 背景
外部编辑器、终端里执行的 Git 命令都会改变仓库，界面需要及时刷新，同时空闲时不能占用 CPU 与内存。

## 决定
- macOS 使用 FSEvents（`internal/platform/watch_darwin.go`）。它按目录树订阅，不需要每个文件一个描述符；同时对 Git 目录做低频指纹校验，以防事件丢失。
- 其他平台暂用有上限的轮询：Git 目录每 2 秒、工作树每 6 秒检查一次。
- 事件合并后再触发一次 `status`；`.git` 内的变化同时刷新引用和日志。

## 代价
Linux 和 Windows 上外部变化最多延迟约 6 秒才刷新，大仓库轮询会有可感知的 I/O。

## 复审条件
在 Linux/Windows 上正式支持时，分别接入 inotify 和 ReadDirectoryChangesW。
