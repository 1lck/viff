# 安全政策 / Security Policy

[中文](#中文) · [English](#english)

## 中文

### 支持范围

Viff 尚未发布稳定版本，安全修复只面向 `main` 分支，不维护旧版本或 LTS。

### 报告漏洞

请使用 GitHub 私密漏洞报告，不要开公开 Issue 或公开讨论细节：

<https://github.com/1lck/viff/security/advisories/new>

报告中请尽量包含：

- 受影响的版本或提交哈希，以及构建方式（源码运行或二进制）；
- 操作系统与 `git --version`；
- 复现步骤或最小 PoC，以及实际影响；
- 必要的脱敏日志。不要粘贴令牌、口令、私有仓库内容或其他凭据。

### 在范围内的示例

- 凭据或令牌泄露，例如被 Viff 写入日志、通知、Console 或设置文件；
- 命令或参数注入，例如不可信的分支名、路径、远程 URL、补丁内容或 Git 输出；
- 恶意仓库内容导致越界读写文件、执行外部程序，或绕过临时仓库与隔离环境的边界；
- reset、删除分支、强推等破坏性 Git 操作在未确认时被执行。

### 不在范围内

- 依赖本机已被攻破或已具备代码执行能力的问题；
- 一般功能缺陷、界面或性能问题，请走公开 Issue；
- Viva 上游框架自身的问题，请同时参考其仓库的安全政策。

### 处理方式

维护者会尽力评估和修复每个报告，但不承诺响应时限。确认后通过 GitHub Security Advisory 沟通修复与披露安排。

### 安全测试

请在临时仓库或隔离环境中复现，不要对真实仓库执行破坏性操作，也不要在报告中暴露他人的数据。

## English

### Supported versions

Viff has no stable release yet. Security fixes target the `main` branch only; there are no maintained old versions or LTS releases.

### Reporting a vulnerability

Use GitHub private vulnerability reporting. Do not open a public issue or discuss details publicly:

<https://github.com/1lck/viff/security/advisories/new>

Please include where possible:

- the affected version or commit hash and how it was built (source or binary);
- your operating system and `git --version`;
- reproduction steps or a minimal proof of concept, and the actual impact;
- relevant logs with secrets removed. Do not paste tokens, passwords, private repository content or other credentials.

### In scope, for example

- Credential or token disclosure, for example through Viff logs, notifications, the console or settings files;
- command or argument injection from untrusted input such as branch names, paths, remote URLs, patch content or Git output;
- malicious repository content causing out-of-bounds file access, execution of external programs, or escaping the temporary-repository and isolation boundaries;
- destructive Git operations (reset, branch deletion, force push and similar) running without confirmation.

### Out of scope

- Problems that require an already compromised machine or existing code execution;
- ordinary bugs, UI issues or performance problems, which belong in a public issue;
- issues in the upstream Viva framework itself; also check that repository's security policy.

### What to expect

Maintainers will make a best effort to assess and fix every report, but no response time is promised. Once a report is confirmed, fixes and disclosure are coordinated through the GitHub Security Advisory.

### Security testing

Reproduce in a temporary repository or an isolated environment. Do not run destructive operations against real repositories, and do not expose other people's data in a report.
