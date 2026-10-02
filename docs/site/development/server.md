# Go Server

`server/` 是 Go 1.26 服务，默认端口 8090，是平台唯一对外入口。它承担 REST API、上传与转换编排、存储抽象、进程内 chat agent，并托管 `web/dist` 静态产物。

## 命令

```bash
cd server
go run ./cmd/server          # 默认读取 ./server_config.json
go test ./...                # 单元 + httptest + 并发；18 个 PG 测试需 VIEWER_TEST_PG_DSN，未设自动跳过
go vet ./...
```

## 包结构

```
cmd/server/main.go        config（json + env 覆盖）+ 依赖装配
internal/
├── api/                  全部 handler（api.go 核心 + edit.go 编辑编排 + auth.go 鉴权），envelope {code,message,data}
├── agent/                进程内 Eino chat agent：react loop + 领域工具 + 主子编排（AgentAsTool）+ 会话事件日志
├── chat/                 chat REST/SSE 端点（项目/会话/消息/事件流/answer/abort）
├── project/              项目与方案产物（PlanStore：plan/bim_supplement/building 三文件版本化）
├── store/                模型元数据/文件存储：Create/Get/List/SetStatus/Delete/Recover
├── convert/              转换队列：Runner 接口、Queue（2 worker、dedup、dirty 重跑、重启 Recover）
├── issue/ change/ override/   各 Store 接口 + FileStore + PgStore（构造时自动建表）
└── editsvc/              edit-service / cad-edit-service HTTP 客户端（按模型 kind 分流）
```

## 端点概览

- 模型、Issue、override 与修改记录：`/api/v1/models/...`。
- 脚本编辑代理：`/api/v1/models/{id}/script/...`。run、save、rollback 成功后自动排队重转 XKT，按模型 kind 分流到 8100 或 8200。`script/edit-call` 不经代理，仅直连可用。
- chat：`/api/v1/chat/projects|sessions` 及其子路径，加上方案产物的 `/api/v1/projects/{id}/...`。
- 静态产物：`GET /v1/models/{id}/model.xkt|metadata.json|render.json`，无 envelope。

完整路由与分组见 [REST API](/reference/rest-api)，机器可读 schema 见 [go-server.openapi.json](/go-server.openapi.json)。

## 代理错误映射

`internal/api/edit.go` 的 `writeEditErr` 透传 Python 的状态码语义：

| Python 侧 | Go 侧 |
| --- | --- |
| 404 | 404 / `40400` |
| 409 | 409 / `40900` |
| 422 | 400 / `40001` |
| 504（diff 超时，默认 60 秒） | 504 / `50400` |
| 其余（含不可达） | 502 / `50200` |

模型 id 校验 `^m_[0-9a-f]{16}$`，防止路径穿越。

## chat agent

进程内 Eino Agent，详见 [Agent 接入](/reference/ai)；LLM 三参与 Skill 三参的配置见[配置说明](/guide/configuration)。key 为空时回退离线 mock。

## 静态托管

`webDist` 存在就由 server 托管前端产物，带 SPA fallback 和指纹资源长缓存；目录缺失时静态路径返回 503，API 照常工作。生产环境浏览器只访问 8090。

## 鉴权与存储

鉴权与 CORS 的配置项见[配置说明](/guide/configuration)，协议行为见 [REST API](/reference/rest-api#鉴权与-cors)。

三个领域 store 各有文件和 PostgreSQL 两套实现，用 `pgDSN` 切换，建表自动完成。模型文件本身始终在文件系统，数据目录布局与第三方整合方式见[存储与前端对接](/development/integration)。

## Converter 子进程

`converter/` 是 Node CLI，基于 web-ifc 和 xeokit-convert，把 IFC 转成 XKT 几何加元数据。server 用 `nodeBin` 和 `converterScript` 配置以子进程方式调用，不需要常驻：

```bash
node convert.js <input.ifc> <outDir>   # 产出 model.xkt + metadata.json
```

- `metadata.json` 是 xeokit 标准元模型，`metaObject id` 就是 IFC GlobalId。convert.js 内置校验 XKT 实体 id 和元模型 id 一致，不一致直接报错退出。
- 重转时机：上传、重试、脚本 run/save/rollback。对同一个模型的在途任务做 dirty 重跑，保证最终转换的是最新内容。
- 测试：`cd converter && npm install && npm test`，用真实 IFC 样例做转换快照。
