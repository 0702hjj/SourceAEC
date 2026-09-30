# 对话 API

进程内 chat agent 的 REST 与 SSE 接口。项目是入口，一个项目绑定唯一会话；前端的历史项目列表就是会话列表。agent 的行为与工具面见 [AI 接入](/reference/ai)。

> 项目创建和删除在 `/api/v1/chat/projects`，方案读写在 `/api/v1/projects/{id}/...`。同一个项目 id 出现在两个前缀下是历史原因。

## 项目

### POST /api/v1/chat/projects

创建项目。body 是 `{"title", "kind"}`，kind 必填，取值 `ifc`、`cad`、`cad->ifc`，它决定 agent 的派发方向。

模型初始化按 kind 分化：`ifc` 建项目时就生成骨架模型，跑一遍最小脚本产出 v1；`cad` 和 `cad->ifc` 先留空，agent 会话内用 init_model 按需建。

```json
{"code":0,"message":"ok","data":{"projectId":"p_xxxx","title":"我的项目","kind":"cad","createdAt":"2026-08-21T06:00:00Z","models":[]}}
```

错误：`40001` kind 缺失或非法。

### GET /api/v1/chat/projects/{id}

项目详情：元信息加项目下模型列表，data 形状与创建响应一致。

```json
{"code":0,"message":"ok","data":{
  "projectId":"p_xxxx","title":"我的项目","kind":"cad->ifc","createdAt":"2026-08-21T06:00:00Z",
  "models":[{"id":"m_xxxx","kind":"ifc","name":"骨架.ifc","status":"ready"}]
}}
```

models 每项是 `{id, kind, name, status}`，与 agent 的 get_project_models 工具同语义——`status` 反查真实状态（由转换队列更新），不是 project.json 快照；已删除的模型保留快照引用。

kind 创建时定死，不可演进；如需换管线就是删除项目重建（删除会级联清理模型与方案产物，需要旧产出先下载源文件）。

错误：`40400` 项目不存在。

### DELETE /api/v1/chat/projects/{id} {#delete-project}

删除项目并级联清理：会话、事件日志、方案文件、项目下所有模型及其附属数据。注意和单模型删除的区别——删项目就是删全套。

## 会话

### GET /api/v1/chat/sessions

```json
{"code":0,"message":"ok","data":[
  {"chatSessionId":"c_xxxx","opencodeSessionId":"s_xxxx","modelId":"","projectId":"p_xxxx","title":"我的项目","createdAt":"2026-08-21T06:00:00Z"}
]}
```

`opencodeSessionId` 是历史遗留字段名，契约如此。

### POST /api/v1/chat/sessions

创建或复用会话。传 `{"title", "projectId"}` 是项目级语义，projectId 幂等，一个项目只有一个会话；传 `{"title", "modelId"}` 是旧的单模型语义。错误：`40001` 项目不存在。

### POST /api/v1/chat/sessions/{cid}/messages

发消息，异步处理。body 是 `{"text"}`，响应 `{"accepted":true}`，事件经 SSE 推送。

### GET /api/v1/chat/sessions/{cid}/messages

会话历史，前端按消息 id 去重合并。

### GET /api/v1/chat/sessions/{cid}/events {#sse-events}

SSE 事件流。帧类型：

| event | data | 说明 |
| --- | --- | --- |
| `session.status` | busy 或 idle | 一轮对话的边界 |
| `message.updated` | 消息骨架 | |
| `message.part.updated` | part 定型 | 文本、推理、工具卡片 |
| `message.part.delta` | 流式增量 | |
| `subagent.status` | subagentId、persona、status、task | 子 agent 边界 |
| `question.ask` | interruptId、question | agent 向用户提问 |
| `viewer.staged` | modelId、kind | 试运行成功的中途预览 |
| `viewer.committed` | 保存成功 | 驱动前端刷新 |
| `model.created` | modelId、kind、title、projectId | agent 创建了新模型 |
| `session.error` | error | 错误 |
| `session.idle` | 空 | 本轮结束 |

支持 `Last-Event-ID` 断线重同步，缓冲最近 64 条。

### POST /api/v1/chat/sessions/{cid}/answer

回答 agent 的提问，body 是 `{"interruptId", "answer"}`，回答后 agent 继续执行。

### POST /api/v1/chat/sessions/{cid}/abort

中止当前执行。

## 项目级方案产物

plan→cad→ifc 管线的中间产物随项目版本化，`name` 取值 `plan`、`bim_supplement`、`building`。管线本身见 [AI Skill](/reference/ai-skill)。

### GET / PUT /api/v1/projects/{projectID}/{name} {#plan-file}

方案文件读写。PUT 全量替换并写入版本历史。

> 三个方案文件走 PlanStore 版本化。skill 的中间产物如 design.json 不版本化，落在 skill 工作区，随项目删除清理。

### GET /api/v1/projects/{projectID}/plan_history

方案版本历史。

### GET /api/v1/projects/{projectID}/plan_history/{base}/{target}/diff

两个方案版本的 diff，base 和 target 可以是 `v{n}` 或 `current`。

### POST /api/v1/projects/{projectID}/deliver

方案交付，body 是 `{"plan", "bimSupplement"}`。
