# 集成

所有业务集成通过 Go 网关 REST API，统一返回 `{code,message,data}`，成功 code=0。
模型 ID 形如 `m_0123456789abcdef`。人和 Agent 使用相同的编辑及版本入口。

外部几何能力属于执行依赖：IfcOpenShell 用于 IFC，ezdxf 用于 DXF；
SimpleCADAPI 可用于其支持的 CAD 工作流。源码与包版本需匹配上游公开 API，
SourceAEC 不复制这些 SDK，也不以 skill 作为运行库。

Go MCP 桥通过 `SOURCEAEC_API_URL` 和 `VIEWER_API_TOKEN` 访问网关，
不会绕过网关直接读取业务存储或连接 Python 服务。

[REST 参考](/reference/rest-api)、[编辑 API](/reference/edit-api)、
[集成边界](/development/integration-boundaries)。
