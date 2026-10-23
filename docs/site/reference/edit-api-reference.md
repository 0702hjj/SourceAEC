# 编辑接口参考

编辑接口由 Go 网关原生实现，业务不再部署为 Python HTTP 服务。
完整路由和请求约定见[编辑 API](/reference/edit-api)。

旧服务内部 `/models/...` 路径不是公共 API；集成方使用
`/api/v1/models/{id}/...` 并读取 `{code,message,data}` 响应。
