## 说明 / Summary

<!-- 这个 PR 解决什么问题？为什么这样改？中文或英文均可。 / What does this PR solve, and why this way? Chinese or English is welcome. -->

## 关联 / Related

<!-- 关联的 Issue，例如 Fixes #123 / Related issue, e.g. Fixes #123 -->

## 变更类型 / Type of change

- [ ] Bug 修复 / Bug fix
- [ ] 新功能 / Feature
- [ ] 文档 / Documentation
- [ ] 重构或测试 / Refactor or tests
- [ ] 构建或 CI / Build or CI

## 验证 / Verification

```sh
go test ./...
go test -tags 'vivahead litheaudit' ./internal/ui
```

- 实际运行的命令与结果（未运行的说明原因） / Commands actually run and results (explain anything not run):
- 平台 / Platform (OS, `git --version`):
- 界面变化？如有请附截图；无头测试不会打开窗口 / UI change? Attach screenshots if so; headless tests do not open a window:

## 自查 / Checklist

- [ ] 上面的测试通过；未通过的部分已说明原因 / The tests above pass, or failures are explained
- [ ] `gofmt` 与 CI 通过 / `gofmt` is clean and CI passes
- [ ] 改动聚焦单一问题，没有混入无关格式化、重命名或依赖升级 / Focused change, no unrelated formatting, renames or dependency bumps
- [ ] 行为变化已同步更新 README、docs 或 docs/decisions / Behaviour changes are reflected in README, docs or docs/decisions
- [ ] 未提交凭据、令牌、私有仓库内容或构建产物 / No credentials, tokens, private repository content or build artifacts
- [ ] 测试使用临时仓库并隔离 HOME 与 Git 配置；无头环境不启动 GUI / Tests use temporary repositories with isolated HOME and Git config; no GUI in headless environments
- [ ] 我同意贡献按 MIT 许可证授权 / I agree my contribution is licensed under the MIT License

## 其他 / Notes
