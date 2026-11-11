# Third-party notices / 第三方声明

Viff's original code is licensed under MIT. The components below retain their
own licenses; the root MIT license does not replace them.

Viff 原创代码采用 MIT。下列组件保留各自的许可证，不由根目录的 MIT 许可证替代。

| Component / 组件            | License / 许可                                    | Source / 来源                                                         |
| --------------------------- | ------------------------------------------------- | --------------------------------------------------------------------- |
| Viva v0.2.4                 | [Apache-2.0](licenses/Viva-Apache-2.0.txt)        | [vivaui/viva](https://github.com/vivaui/viva)                         |
| purego v0.11.1              | [Apache-2.0](licenses/purego-Apache-2.0.txt)      | [ebitengine/purego](https://github.com/ebitengine/purego)             |
| go-toml v2.4.3              | [MIT](licenses/go-toml-MIT.txt)                   | [pelletier/go-toml](https://github.com/pelletier/go-toml)             |
| JetBrains Mono              | [SIL OFL 1.1](licenses/JetBrainsMono-OFL-1.1.txt) | [JetBrains/JetBrainsMono](https://github.com/JetBrains/JetBrainsMono) |
| IntelliJ Platform SVG icons | [Apache-2.0](assets/icons/intellij/LICENSE)       | [Original attribution](assets/icons/intellij/NOTICE.md)               |
| Motty (terminal emulator, pty session) | [Apache-2.0](licenses/Motty-Apache-2.0.txt) | [t8y2/motty](https://github.com/t8y2/motty) at `fbd26b8`; see [NOTICE](licenses/Motty-NOTICE.txt) |
| creack/pty v1.1.24          | [MIT](licenses/creack-pty-MIT.txt)                | [creack/pty](https://github.com/creack/pty)                           |
| golang.org/x/sys v0.50.0    | [BSD-3-Clause](licenses/golang-x-sys-BSD-3-Clause.txt) | [golang/sys](https://github.com/golang/sys)                      |

The generated icon code in `internal/ui/icons` is derived from the attributed
IntelliJ Platform SVGs and remains subject to their license. UI screenshots
contain those icons. Viff is an independent project, not affiliated with or
endorsed by JetBrains.

`internal/ui/icons` 中生成的图标代码来源于上述 SVG，仍遵循其许可。界面截图包含这些图标。
Viff 是独立项目，与 JetBrains 没有关联，也未获得其背书。

The terminal emulator in `internal/ui/term` and the pty session in
`internal/platform/pty` are derived from Motty and remain under Apache-2.0;
the notice above states the changes. `creack/pty` is used on Unix and
`golang.org/x/sys` on Windows for the pseudo console.

`internal/ui/term` 与 `internal/platform/pty` 派生自 Motty，仍遵循 Apache-2.0；
修改说明见上方 NOTICE。Unix 侧使用 `creack/pty`，Windows 侧用
`golang.org/x/sys` 驱动 ConPTY。

Viva's own [NOTICE](licenses/Viva-NOTICE.txt) and [transitive notices](licenses/Viva-THIRD-PARTY-NOTICES.md) are also included.
