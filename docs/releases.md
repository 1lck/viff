# Release process / 发布流程

1. Merge changes into `main` and wait for the **CI** check to pass.
2. Choose a version. Pre-release tags include a suffix, such as `v0.1.0-alpha.1`.
3. Push the tag pointing to the tested commit.
4. The Release workflow verifies successful CI for that exact commit, builds
   on macOS and Windows, and packages the native executable with license notices.
5. The workflow publishes archives and `SHA256SUMS`. Review and expand the
   generated release notes with changes, verification, and remaining limitations.

6. 合并至 `main`，等待 **CI** 通过。
7. 选择版本；预发布使用后缀，例如 `v0.1.0-alpha.1`。
8. 将指向已验收提交的标签推送到 GitHub。
9. Release 工作流核实该提交的 CI 成功后，在 macOS 和 Windows 构建，并打包可执行文件及许可声明。
10. 工作流发布压缩包与 `SHA256SUMS`；维护者补充具体改动、验证情况和已知限制。

Current binaries are unsigned and are not installers. The archive name records
the actual build OS and architecture. Git is a separate prerequisite. Do not
describe these artifacts as notarized, signed, or verified on untested platforms.

当前产物未签名，也不是安装器。文件名标明实际构建平台和架构，Git 需单独安装。
不得声称已签名、公证，或在未测试的平台完成验证。

Local packaging after building `build/viff` (or `build/viff.exe`):

```sh
python scripts/package.py v0.1.0-alpha.1
```

GitHub repository settings require pull requests, a passing CI check, resolved
review conversations, and linear history on `main`. Force pushes and branch
deletion are disabled. Owner/admin bypass remains available for recovery; no
extra reviewer is required for a single-maintainer project. Dependency updates
are proposed weekly by Dependabot.

GitHub 仓库主分支要求 PR、CI 通过、解决评审讨论及线性历史，禁止强推和删除。
保留所有者/管理员应急绕过权限；单维护者项目不强制额外审批人数。Dependabot 每周提出依赖更新。
