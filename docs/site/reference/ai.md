# AI 接入

面向 AI agent 的接入指南。script-as-source 模式下，AI 的修改流程和设计师完全一致：**暂存脚本 → 沙箱试运行 → 保存大版本**。概念与版本语义见[编辑与版本](/guide/editing)，端点契约见 [IFC 编辑 API](/reference/edit-api)，机器可读 schema 见 [REST API 的 OpenAPI 一节](/reference/rest-api#机器可消费-openapi)。本页讲怎么接。

## 平台内置 chat agent

除了 REST 直连，平台自带进程内 chat agent，由 cloudwego/eino 驱动，跑在 Go server 里。网页的 AI 对话栏就是它。要点：

- **LLM 配置**：`llmAPIKey`、`llmBaseURL`、`llmModel` 三项，OpenAI 兼容端点。key 留空进入离线模式，用确定性脚本模型，不产生真实回复。
- **领域工具**：agent 只见平台工具，没有 shell 和任意文件写权限。工具面覆盖建项目、起模型、读脚本、定位、暂存、试运行、保存、看版本、看 diff，方案级还有 deliver_plan、deliver_building 和把上游产物桥接进工作区的工具。按模型 kind 自动路由到对应服务；工具报错以文本返回给模型自纠。
- **主子编排**：orchestrator 负责对话和路由，按任务派 ifc-agent 或 cad-agent 子 agent，子 agent 事件带 subagentId 标签，前端分组展示。
- **skill 接入**：从 `skills/dist` 加载正式集合。文件工具只允许读参考文档、执行白名单命令、写项目工作区。skill 中间产物落在 `{DATA}/skill-work/{projectID}/`，不版本化。
- **会话连续性**：跨轮次回填历史，超出预算时压缩。agent 随时可以读当前脚本，保证修改是增量而不是重写。
- **向用户提问**：ask_user 工具触发 SSE 提问帧，用户回答后续跑。

### 中途预览

run_script 试运行成功即推 `viewer.staged` 事件，保存之前人就能看到中间结果。行为细节见[编辑与版本](/guide/editing#中途预览)。工具结果末尾还会附上 staging diff 摘要，优先给构件级计数，供 AI 对照预期自纠。

## 双角色同一 API

人和 AI 用同一套编辑端点，只是入口不同：

```
浏览器（人）──► Go server :8090 ──代理──► Python 编辑服务 :8100
                  /api/v1/models/{id}/script/...  │  /models/{id}/script/...
AI agent ────────► REST 直连 ──────────────────────┘  （或经 Go 代理，端点一一对应）
```

人走 Go 代理，保存后自动重转 XKT。AI 可以直连编辑服务，也可以走代理；`script/edit-call` 只有直连可用。Python 服务自带 Swagger UI 在 `/docs`。

## 快速开始

```bash
# 1) Python 编辑服务（默认端口 8100）
cd services/ifc
uv sync
uv run uvicorn app.main:app --port 8100

# 2) Go server（默认 127.0.0.1:8090）
cd server
go run ./cmd/server
```

`VIEWER_DATA_DIR` 必须和 Go 配置的 `dataDir` 指向同一目录，两边都按它定位模型文件。

AI agent 可以不装 Go server、web、converter、PostgreSQL，只用 `services/ifc` 就能完成脚本编辑、版本和 diff。独立部署步骤见 [Edit Service 的独立部署一节](/development/edit-service#独立部署与移植)。

## AI 直连全流程

前提是已有一个模型，文件在 `{VIEWER_DATA_DIR}/uploads/{id}.ifc`。

### 上传 IFC 的参考生成

```bash
BASE=http://127.0.0.1:8100
MID=m_0123456789abcdef

# 1. 用 aiifc skill 写复现脚本后暂存；首次暂存自动保留 bootstrap.ifc
curl -X PUT "$BASE/models/$MID/script" \
  -H 'Content-Type: application/json' \
  -d '{"script": "PARAMS = {...}\n\ndef build(params, out_path):\n    ...\n"}'

# 2. 沙箱试运行，预览，无版本
curl -X POST "$BASE/models/$MID/script/run"

# 3. 保存大版本 v1，响应带 alignment 计数
curl -X POST "$BASE/models/$MID/script/save" \
  -H 'Content-Type: application/json' -d '{"note": "bootstrap v1"}'
```

### 既有脚本的定向修改

```bash
# 1. 按 guid 定位调用点
curl "$BASE/models/$MID/script/locate?guid=2O2Fr\$t4X7ZfFPoeewFlqU"

# 2a. 参数来自 PARAMS：只改键值，暂存一步
curl -X PUT "$BASE/models/$MID/script" \
  -H 'Content-Type: application/json' \
  -d '{"params": {"wall_height": 3.2}}'

# 2b. 参数是字面量：libcst 标量改写，一步完成
curl -X POST "$BASE/models/$MID/script/edit-call" \
  -H 'Content-Type: application/json' \
  -d '{"designKey": "L1:wall:1", "argument": "height", "value": 3.2}'

# 3. 保存大版本
curl -X POST "$BASE/models/$MID/script/save"

# 4. 版本对比：脚本 diff 加语义 diff
curl -X POST "$BASE/models/$MID/script/diff" \
  -H 'Content-Type: application/json' -d '{"base": "v1", "target": "v2"}'
curl -X POST "$BASE/models/$MID/diff" \
  -H 'Content-Type: application/json' -d '{"base": "v1", "target": "current"}'
```

直连的 run 和 save **不触发** Go 侧的 XKT 重转。需要前端自动刷新时改走 Go 代理，把 base 换成 `http://127.0.0.1:8090/api/v1` 即可。

## 契约要点

- **脚本契约**：PARAMS 是顶层字面量 dict；构件经契约工厂创建，自动写确定性 GlobalId 和 designKey；可编辑参数必须是标量；入口 `build(params, out_path)`；出口过 validate。逐条见 [IFC 编辑 API](/reference/edit-api#构建脚本契约)。
- **失败语义**：契约校验或沙箱构建失败返回 422，零副作用；edit-call 遇到不可改写的参数返回 422；locate 查不到返回 200 加 `found: false`，不是 5xx。状态码归属见[故障排查](/guide/troubleshooting#状态码去哪查)。
- **版本语义**：脚本和 map 全量保留、编号同步；产物只物化最新，历史从脚本重建，对比走语义 diff 不做字节断言。
- **provenance**：来源字段由调用方自报，服务端只校验枚举。

## 当前限制

单机单用户；编辑服务无鉴权，勿暴露公网；数据目录必须一致；IFC 侧 diff 只到属性级。

## 与 skill 的分工

REST 编辑 API 适合在既有脚本上做定向修改和版本管理。从零建模型或大改几何用 [AI Skill](/reference/ai-skill)：agent 直接写符合契约的构建脚本，再交给平台执行、存版本、算 diff。
