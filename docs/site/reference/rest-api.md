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
| `50000` | 服务器内部错误 |
| `50200` | 编辑服务不可达等代理错误 |
| `50400` | 编辑服务超时 |

代理映射的完整规则见 [Go Server](/development/server)。

## 鉴权与 CORS

鉴权默认关闭。设置 `apiToken`（或 `VIEWER_API_TOKEN`）后，除 OPTIONS 预检外，所有端点都必须携带有效 token。普通 API 使用 `Authorization: Bearer <token>`；以下四类只读资源因为 xeokit 和 `<img>` 无法添加请求头，允许使用 `?token=`，但不允许匿名访问：

- `GET /v1/models/{id}/model.xkt`
- `GET /v1/models/{id}/metadata.json`
- `GET /v1/models/{id}/render.json`
- `GET /v1/models/{id}/issues/{file}`

浏览器端把 token 存在 localStorage 的 `aiifc_token` 键，遇到 401 会弹输入框，保存后自动重试原请求。chat 的事件流走 EventSource，没法带自定义头，**这是唯一放行 `?token=` 查询参数的路径**。

CORS 使用配置的来源白名单。编辑领域在 Go 网关进程内运行，统一受其鉴权保护。

## 机器可消费 OpenAPI

### 编辑领域

编辑请求由 Go 原生实现，见[编辑 API](/reference/edit-api)。没有 FastAPI 或
独立服务的 openapi.json。公共 schema 以 Go 网关接口为准。

### Go server

完整 schema 见 [go-server.openapi.json](/go-server.openapi.json)，可以直接喂给 LLM、工具或代码生成器。路由清单与请求响应 schema 随公开文档版本发布；修改 Go API 时，请同步更新 `docs/site/public/` 下的 schema、路由清单和对应参考页，并运行 `npm run docs:build` 检查站点。

对接方要自研前端或接自己的存储时，看[存储与前端对接](/development/integration)；外部 Agent 接入看 [Agent 接入](/reference/ai)。
