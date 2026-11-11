# REST API

Go server（默认 `http://localhost:8090`）是平台唯一对外入口，对外路径统一 `/api/v1/{resource}/{id}`。本页讲跨资源共用的约定：端点地图、响应 envelope、错误码、鉴权与 CORS、机器可消费 schema。

## 端点地图

| 分组 | 路径前缀 | 契约 |
| --- | --- | --- |
| 模型 | `/api/v1/models` | [模型与审查 API](/reference/api-model) |
| Issue | `/api/v1/models/{id}/issues` | [模型与审查 API](/reference/api-model) |
| 属性 override 与修改记录 | `/api/v1/models/{id}/overrides`、`/changes`、`/entities/{entityId}/properties` | [模型与审查 API](/reference/api-model) |
| 脚本编辑（代理） | `/api/v1/models/{id}/script/...` | [IFC 编辑 API](/reference/edit-api) |
| 编辑只读与对比（代理） | `/api/v1/models/{id}/edit/...` | [IFC 编辑 API](/reference/edit-api) |
| 对话与项目 | `/api/v1/chat/...`、`/api/v1/projects/...` | [对话 API](/reference/api-chat) |
| 静态资源 | `/v1/models/{id}/...` | [模型与审查 API](/reference/api-model) |

模型 id 格式是 `^m_[0-9a-f]{16}$`，服务端据此校验以防路径穿越。

## 响应与错误码

除静态文件端点外，所有响应统一 envelope，`code=0` 表示成功：

```json
{"code": 0, "message": "ok", "data": {...}}
```

| code | 含义 |
| --- | --- |
| `40001` | 参数或校验错误。编辑服务的 422 经代理也映射为此 |
| `40002` | 超限，如上传大小 |
| `40100` | 鉴权失败 |
| `40400` | 模型或资源不存在 |
| `40900` | 冲突 |
| `42900` | 工作或字节预算容量不足，归还后可重试 |
| `50000` | 服务器内部错误 |
| `50200` | 编辑调用或兼容传输错误 |
| `50300` | 所需运行时或隔离能力不可用 |
| `50400` | 编辑服务超时 |

代理映射的完整规则见 [Go Server](/architecture/gateway)。

## 鉴权与 CORS

鉴权默认关闭。设置 `apiToken`（或 `VIEWER_API_TOKEN`）后，除 OPTIONS 预检外，所有端点都必须携带有效 token。普通 API 使用 `Authorization: Bearer <token>`；以下四类只读资源因为 xeokit 和 `<img>` 无法添加请求头，允许使用 `?token=`，但不允许匿名访问：

- `GET /v1/models/{id}/model.xkt`
- `GET /v1/models/{id}/metadata.json`
- `GET /v1/models/{id}/render.json`
- `GET /v1/models/{id}/issues/{file}`

浏览器端把 token 存在 localStorage 的 `aiifc_token` 键，遇到 401 会弹输入框，保存后自动重试原请求。chat 的事件流走 EventSource，没法带自定义头，此 SSE 路径也允许 `?token=`，普通业务与工作流 API 仍必须使用请求头。

CORS 使用配置的来源白名单。编辑领域在 Go 网关进程内运行，统一受其鉴权保护。

## 机器可消费 OpenAPI

### 编辑领域

编辑请求由 Go 原生实现，见[编辑 API](/reference/edit-api)。没有 FastAPI 或
独立服务的 openapi.json。公共 schema 以 Go 网关接口为准。

### Go server

请求与响应 schema 见 [go-server.openapi.json](/go-server.openapi.json)，路由见
[端点清单](/go-rest-api.routes.json)。两者随文档版本发布；`npm test` 对照 Go 注册检查
method/path 覆盖、引用和 operation ID，并校验关键原生编辑契约。schema 的字段语义仍需人工
对照实现和运行时测试审阅，不能仅凭路由覆盖认定完整。修改 API 后同步更新这两份文件和参考页，
运行 `npm test` 与 `npm run docs:build`。

对接方要自研前端或接自己的存储时，看[存储与前端对接](/integrations/overview)；外部 Agent 接入看 [Agent 接入](/integrations/agents)。

## 持久脚本执行

[工作流 API](/reference/workflows)提供显式创建、查询、取消与恢复。202 表示固定定义与 run 已提交，
原同步编辑接口保持现有语义。MCP 通过同一鉴权网关使用这些接口。
