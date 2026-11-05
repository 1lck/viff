<div align="center">

<img src="assets/branding/icon.png" width="128" height="128" alt="Viff" />

# Viff

**A lightweight, native Git desktop client.**

Go + Viva. No Electron. No WebView. No bundled browser.

[English](README.md) · [简体中文](README.zh-CN.md)

[![CI](https://github.com/1lck/viff/actions/workflows/ci.yml/badge.svg)](https://github.com/1lck/viff/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27.3%2B-00ADD8.svg)](https://go.dev/dl/)

[Features](#a-full-git-workflow) · [Community](#join-the-community) · [Contribute](CONTRIBUTING.md) · [Releases](https://github.com/1lck/viff/releases)

</div>

![Viff in dark mode: line staging, side-by-side diff, and commit history](docs/screenshots/diff-dark.png)

## Keep the tool lightweight. Keep the workflow complete.

Viff puts your working tree, diffs, and history in one native desktop window.
It is written in Go with [Viva](https://github.com/vivaui/viva), renders without a
browser engine, and runs your installed Git executable.

- **A small distribution:** one application executable, plus license files. No separate browser runtime or local server to install.
- **A native desktop foundation:** Viva handles rendering and platform integration; Viff does not ship an Electron or WebView application.
- **Git stays Git:** use your repositories, remotes, hooks, and configured credentials. Inspect executed commands and output in the Console.
- **Work is loaded as needed:** paginated history and on-demand diffs keep large repositories from becoming a single eager load.

For a concrete size reference, stripped Go 1.27.3 builds on November 4, 2026
were **7.37 MiB on macOS arm64** and **7.65 MiB on Windows amd64**
(`-trimpath -ldflags="-s -w"`, executable only). Archive size, installed Git,
and runtime memory are separate measurements.

Lightweight is a design goal, not an unmeasured performance claim. Memory usage
and startup time vary with the repository, platform, and renderer; no comparative
RAM or startup benchmark is claimed.

## Join the community

Welcome to join the Viff community — share your experience, ask questions, and
follow the latest updates.

<p align="center">
  <img src="docs/assets/community/wechat-group.png" width="240" alt="Viff WeChat group QR code">
</p>

<p align="center"><sub>Scan with WeChat to join the group. The group QR code expires
periodically; if it no longer works, open an issue and we will refresh it.</sub></p>

## A full Git workflow

| Work                  | What Viff provides                                                                                                   |
| --------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Review changes        | Side-by-side and unified diffs, syntax and word highlighting, search, synchronized scrolling, and multiple tabs      |
| Build a commit        | Stage files, hunks, or selected lines; unstage, roll back, amend, sign off, and commit/push                          |
| Organize work         | Multiple repositories, per-repository commit drafts, directory grouping, and named ChangeLists                       |
| Read history          | Commit graph, branch/author/path filters, multi-commit changed-file views, blame, reflog, and revision tree browsing |
| Browse files          | Project tool window with folders, status colours, Quick Open, read-only viewing, blame, and file history |
| Manage branches       | Checkout, merge, rebase, cherry-pick, revert, reset, and stash-assisted checkout                                     |
| Resolve conflicts     | Ours / Result / Theirs views, per-block Ours/Theirs/Both actions, and explicit mark-as-resolved                      |
| Work across checkouts | Create worktrees from new or existing branches; manage submodules with dirty-work protection                         |
| Exchange changes      | Fetch, pull, push, force-with-lease, remotes, patches, and signing/credential settings                               |

**ChangeLists group files; they do not create separate Git indexes.** A commit
includes **all staged files**, even when a list filter is active. Filtered bulk
staging only stages visible files.

<details>
<summary>Light theme</summary>

![Viff in light mode with a diff and selected commit](docs/screenshots/diff-light.png)

</details>

Screenshots come from real Viva headless tests using disposable demo repositories.

## If macOS says it cannot open `Viff.app`

If macOS says that Apple cannot verify whether `Viff.app` contains malware, the
manually downloaded package may not yet be notarized by Apple. First confirm that
the app came from the trusted [GitHub Releases](https://github.com/1lck/viff/releases),
then use one of these methods:

<p align="center">
  <img src="./docs/assets/screenshots/macos-app-verification-warning.png" width="492" alt="macOS warning that Viff.app cannot be opened">
</p>

1. In **Applications**, Control-click `Viff.app`, choose **Open**, and choose **Open** again in the confirmation dialog.
2. If macOS still blocks it, open **System Settings > Privacy & Security**, click **Open Anyway** next to the security warning, and launch the app again.
3. You can also remove the quarantine attribute in Terminal:

   ```bash
   sudo xattr -dr com.apple.quarantine /Applications/Viff.app
   ```

Only use these steps for an app whose source you trust.

## ⭐ Special thanks

<p align="center">
  <a href="https://linux.do/">
    <img src="./docs/assets/special-thanks/linux-do.png" width="42%" alt="LINUX DO">
  </a>
</p>

<p align="center">
  <strong>For all things AI, head to <a href="https://linux.do/">LINUX DO</a>. Wishing the community ever greater success.</strong>
</p>

## Contributors

Thank you to everyone who contributes to and improves Viff.

<a href="https://github.com/1lck/viff/graphs/contributors">
  <img src="https://raw.githubusercontent.com/1lck/viff/chart-assets/contributors.svg" alt="Contributors">
</a>

## License and credits

Viff's original code is [MIT licensed](LICENSE). Viva, JetBrains Mono, IntelliJ
Platform icons, and other dependencies retain their own licenses; see
[third-party notices](THIRD_PARTY_NOTICES.md).

The interface takes inspiration from IntelliJ IDEA's Git workflow. Viff is an
independent project and is not affiliated with or endorsed by JetBrains.

## Star History

<a href="https://www.star-history.com/#1lck/viff&Date">
  <img alt="Star History Chart" src="https://raw.githubusercontent.com/1lck/viff/chart-assets/star-history-light.svg" width="100%" />
</a>
