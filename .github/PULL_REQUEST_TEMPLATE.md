## 概要

<!-- 简述本 PR 做了什么、为什么 -->

## 勾选清单

- [ ] 相关组件测试全绿（`go test ./...` / `uv run --group dev pytest` / `npm test`，见 AGENTS.md 组件与命令表）
- [ ] 新增/修改代码配有测试，且新增测试量 ≥ 新增实现量（AGENTS.md 测试纪律）
- [ ] 修 bug 已先写复现该 bug 的失败测试
- [ ] 改了 API：已同步更新公开 schema、参考页和契约测试
- [ ] 改了文档：已执行 `cd docs && npm run docs:build`
- [ ] Commit 信息为中文前缀式（`feat(server): ...` / `fix(web): ...` / `docs: ...` / `chore: ...`）
- [ ] 未提交本机路径、密钥、运行时数据（`data/`）
