# API contract sources / API 契约维护源

维护 Go 合同的 `base.json`（元数据）、`paths/index.json`、`schemas/index.json` 和
`responses/index.json`。索引把路径或组件名映射到同目录下的单个 JSON 模块。
每个维护源文件必须少于 500 行；按路由、schema 或 response 的职责拆分，不压缩排版。

AI 子集的 `base.json` 保留其元数据，`source.json` 选择 Go 合同中的既有路径与 schema。
装配器补齐引用所需的组件，不自动扩大路径范围；不要手抄共享定义到第二套源模块。

从 `docs` 运行：

```bash
npm run api:build
npm test
npm run docs:build
```

`api:build` 确定性装配 `site/public/{go-server,ai-tools}.openapi.json`。
这两份文件是保持既有下载 URL 的生成产物，不直接手改；组件引用在装配后的单文件内解析。
`api:check`、测试和文档构建拒绝过期产物。两个生成文件的行数例外不适用于维护源。
路由清单仍由 Go 注册覆盖测试校验；字段语义仍须结合 Go 实现和运行时测试人工审阅。

Maintain the Go contract's `base.json`, path/schema indexes and response index.
Each index maps one path or component name to a JSON module in the same directory.
Keep every source below 500 lines, splitting by responsibility rather than compressing formatting.

The agent subset keeps its metadata in `base.json` and selects canonical Go paths/schemas in `source.json`.
Assembly includes referenced components transitively without expanding the exposed path surface.
Do not duplicate shared declarations in a second set of source modules.

Run the commands above from `docs`. `api:build` deterministically assembles the existing public single-file
snapshots, preserving their download URLs and local component references. Do not edit generated snapshots
by hand. `api:check`, tests and documentation builds reject stale output. Generated-file size exceptions do
not apply to source modules. Go route coverage and runtime tests complement semantic review; the assembler
does not infer every schema field from Go implementations.
