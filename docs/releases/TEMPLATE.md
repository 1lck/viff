<!--
复制为 docs/releases/{{tag}}.md；tag 与 previous_tag 包含 v 前缀。
替换所有双花括号占位符，删除本注释；逐版核实下载文件、升级步骤和平台限制。
每条只写一个用户可感知的变化、一句话、一行 Markdown，不嵌套列表。
按“新功能 → 改进 → 修复”排序，删除空分类，中英文保持一致。
不要列内部类名、测试数量或原始日志；在底部集中感谢已核实身份的贡献者。
本模板仅列 Viff 当前实际提供的六种安装包；不要加入 Homebrew 或功能承诺。
先提交说明，再创建标签；运行 python3 scripts/release_notes.py docs/releases/{{tag}}.md --tag {{tag}}。
-->

## 中文

{{summary_zh}}

### 下载

- [项目主页](https://github.com/1lck/viff)
- [macOS Apple Silicon · DMG](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-darwin-arm64.dmg)
- [macOS Intel · DMG](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-darwin-amd64.dmg)
- [Windows x64 · 安装 EXE](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-windows-amd64-setup.exe)
- [Windows x64 · 便携 ZIP](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-windows-amd64.zip)
- [Linux x64 · DEB](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-linux-amd64.deb)
- [Linux x64 · 便携包](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-linux-amd64.tar.gz)
- [SHA-256 校验值](https://github.com/1lck/viff/releases/download/{{tag}}/SHA256SUMS)
- [全部下载文件](https://github.com/1lck/viff/releases/tag/{{tag}})

### 重点更新

{{overview_zh}}

#### ✨ 新功能

- {{new_feature_zh}}

#### ⚡ 改进

- {{improvement_zh}}

#### 🛠 修复

- {{fix_zh}}

### 升级说明

- macOS：按机型选择 Apple Silicon 或 Intel 的 DMG，退出旧版后打开 DMG，将 Viff.app 拖入“应用程序”并替换旧版。
- Windows：退出旧版后运行 x64 安装器。
- Windows 便携包：解压后进入新目录执行 `viff.exe`，不写入安装信息。
- Debian/Ubuntu：执行 `sudo apt install ./viff-{{tag}}-linux-amd64.deb`。
- Linux 便携包：解压后进入新目录执行 `./viff`，并自行安装所需系统库。
- Git 需单独安装并加入 `PATH`，Linux 系统库要求见[发布指南](https://github.com/1lck/viff/blob/{{tag}}/docs/releases.md)。

### 兼容性与已知问题

- {{compatibility_zh}}
- {{signing_and_known_issues_zh}}

{{trusted_download_note_zh}}

[查看完整变更](https://github.com/1lck/viff/compare/{{previous_tag}}...{{tag}})

---

## English

{{summary_en}}

### Downloads

- [Project homepage](https://github.com/1lck/viff)
- [macOS Apple Silicon · DMG](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-darwin-arm64.dmg)
- [macOS Intel · DMG](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-darwin-amd64.dmg)
- [Windows x64 · Setup EXE](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-windows-amd64-setup.exe)
- [Windows x64 · Portable ZIP](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-windows-amd64.zip)
- [Linux x64 · DEB](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-linux-amd64.deb)
- [Linux x64 · Portable archive](https://github.com/1lck/viff/releases/download/{{tag}}/viff-{{tag}}-linux-amd64.tar.gz)
- [SHA-256 checksums](https://github.com/1lck/viff/releases/download/{{tag}}/SHA256SUMS)
- [All downloads](https://github.com/1lck/viff/releases/tag/{{tag}})

### Highlights

{{overview_en}}

#### ✨ New features

- {{new_feature_en}}

#### ⚡ Improvements

- {{improvement_en}}

#### 🛠 Fixes

- {{fix_en}}

### Upgrade instructions

- macOS: choose the Apple Silicon or Intel DMG, quit the old version, open the DMG, and replace Viff.app in Applications.
- Windows: quit the old version and run the x64 installer.
- Windows portable: extract the archive, enter its directory, and run `viff.exe` without installing.
- Debian/Ubuntu: run `sudo apt install ./viff-{{tag}}-linux-amd64.deb`.
- Linux portable: extract the archive, enter its directory, and run `./viff` after installing the required system libraries.
- Install Git separately on `PATH`, and check the [release guide](https://github.com/1lck/viff/blob/{{tag}}/docs/releases.md) for Linux dependencies.

### Compatibility and known issues

- {{compatibility_en}}
- {{signing_and_known_issues_en}}

{{trusted_download_note_en}}

[Full changelog](https://github.com/1lck/viff/compare/{{previous_tag}}...{{tag}})

### 🙌 感谢贡献者

{{contributors_zh}}

### 🙌 Contributors

{{contributors_en}}
