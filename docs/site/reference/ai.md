# AI 与 MCP

内置 Eino Agent 使用 Go 领域工具修改模型，按项目类型协调 IFC/CAD 任务。
所有角色共享可选的 SourceAEC 接入 skill 和配置的上游 SDK 参考。规划是可选输入，
执行与业务规则由 Go 实现。
没有模型 API key 时使用确定性离线模型，不能视为完整自然语言建模能力。

Go stdio MCP：`go run ./cmd/sourceaec-mcp`（从 server 目录执行）。
配置 SOURCEAEC_API_URL 与 VIEWER_API_TOKEN 访问网关。

| 工具 | 作用 |
| --- | --- |
| model_current_context | 当前版本及脚本暂存状态 |
| ifc_upload_modified | 比较本地修改后的 IFC，并经网关记录 USER 来源 |
| dxf_upload_modified | 比较本地修改后的 DXF，并经网关记录 USER 来源 |

MCP 不直接连接持久化存储，也不启动 Python 服务。
模型源码与普通业务文件分离，生成几何必须经沙箱。

[skills](/reference/ai-skill)、[编辑 API](/reference/edit-api)。
