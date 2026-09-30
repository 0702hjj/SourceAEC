# 测试与调试

开发采用 TDD：先写失败测试再写实现，测试文件和源码放同一个目录。涉及异步写盘的测试——转换队列、SSE、后台 goroutine——必须用条件等待确认落盘，禁止固定 sleep。

## 各模块测试

| 模块 | 框架 | 运行命令 |
| --- | --- | --- |
| server | go test（含 httptest 与并发 `-race`） | `cd server && go test ./... && go vet ./...` |
| web | vitest + jsdom | `cd web && npm test` |
| services/ifc | pytest | `cd services/ifc && uv run --group dev pytest` |
| services/cad | pytest | `cd services/cad && uv run --group dev pytest` |
| services/sandbox | pytest（用例配两套服务适配器） | `cd services/sandbox && uv run --group dev pytest` |
| services/editapi | pytest（契约套件，两个服务环境各跑一遍） | `cd services/ifc && uv run --group dev pytest ../editapi/tests -q`（cad 同理） |
| converter | node:test | `cd converter && npm test` |
| mcp | pytest | `cd mcp && uv run --group dev pytest` |
| skill 打包 | pytest | `python -m pytest tests/skill/ -q` |
| agent TUI | textual Pilot | `tools/agent/agent --test` |
| 端到端 | bash 冒烟 | `./scripts/smoke.sh`，需 server 运行 |

server 的 18 个 PG 测试需要 `VIEWER_TEST_PG_DSN`（指向专用测试库，测试会删表），未设时自动跳过。各组件当前的用例规模随迭代变动，以仓库根 `AGENTS.md` 的组件表为准。

## 端到端冒烟

前提是 server 已经在 8090 运行。编辑服务可达时冒烟会追加脚本编辑链路，不可达时自动跳过。

```bash
cd server && go run ./cmd/server &
./scripts/smoke.sh    # 成功以 smoke OK 结尾
```

覆盖内容：上传样例 IFC，轮询到 ready，检查 XKT、metadata 和下载接口；Issue 的创建、列表、截图、状态流转和删除；override 写入与生效值断言；change log 的旧值新值断言；脚本管线的暂存、沙箱运行、保存 v1 和版本记录断言；最后清理。

## 手工验收清单

1. 打开 `http://localhost:5173`，新建项目并选类型，在 AI 对话栏让 agent 生成模型。
2. 模型列表状态从 converting 变 ready；failed 有错误提示且可重试。
3. 进查看器：IFC 模型能渲染，可切 web-ifc 引擎，轨道旋转、缩放、NavCube 正常；DXF 模型 Canvas 渲染正常。
4. 模型树默认展开一层，搜索和类型过滤可用，节点能显隐，点击节点相机飞行并高亮。
5. 属性面板折叠、搜索、复制正常；script-backed 模型点定位脚本能跳到编辑器对应行。
6. 可见性工具、剖切滑杆、距离测量都可用。
7. Issue 全流程：选中构件新建，自动截图，3D 钉显示并可点击，状态流转，删除。
8. Diff：选 base 和 target，着色正确，展开看字段明细，清除后复位。

跑出来不对的现象查[故障排查](/guide/troubleshooting)。

## 文档与提交纪律

- 公开文档源在 `docs/site/`，是唯一信息源。改动后必须跑 `cd docs && npm run docs:build`，死链会让构建失败。
- 改了 API 时要同步更新 `docs/site/public/` 下的公开 schema、对应参考页和契约测试，再运行文档构建。
- 文档涉及未交付能力时必须标注为规划，不得写不可执行的步骤。移动或删除文档后，全仓的相对链接要同步更新。
- commit 用中文前缀：`feat:`、`fix:`、`docs:`、`ci:`、`chore:`。不要提交本机路径、密钥和运行时数据 `data/`。
