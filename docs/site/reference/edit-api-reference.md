# 机器可读 Schema

编辑接口由 Go 网关原生实现，没有独立 Python HTTP 服务。
集成使用 `/api/v1/models/{id}/...` 和 `{code,message,data}`；旧内部 `/models/...` 路径不是公共接口。

- [网关 OpenAPI](/go-server.openapi.json)
- [路由清单](/go-rest-api.routes.json)
- [AI 工具 OpenAPI](/ai-tools.openapi.json)
- [脚本编辑 API](/reference/edit-api)
- [持久工作流 API](/reference/workflows)

两种语言共享这些版本化文件；请使用与部署源码匹配的版本。
