# 配置

Go 网关读取 server_config.json，可由命令行 `-config` 选择文件。
示例配置在 server/server_config.example.json。相对路径以进程工作目录为基准。

| 配置/环境变量 | 用途 |
| --- | --- |
| pgDSN / VIEWER_PG_DSN | PostgreSQL 连接，必需 |
| dataDir | 运行时数据投影目录 |
| VIEWER_STORAGE_NAMESPACE | 数据库业务空间隔离 |
| webDist / VIEWER_WEB_DIST | 前端构建目录 |
| SOURCEAEC_PYTHON | 外部几何解释器绝对路径，未设置则由 uv 准备 |
| apiToken / VIEWER_API_TOKEN | 网关 Bearer token |
| corsOrigins / VIEWER_CORS_ORIGINS | 允许的浏览器来源列表 |
| skillsDir / VIEWER_SKILLS_DIR | 声明性 skill 根目录，可留空 |
| mcpBinary / VIEWER_MCP_BIN | 可选 Go stdio MCP 可执行文件 |
| VIEWER_LLM_API_KEY | 模型 API key，留空使用确定性离线模型 |
| VIEWER_LLM_BASE_URL | 模型 API 地址 |
| VIEWER_LLM_MODEL | 模型名 |
| VIEWER_PPROF_ADDR | 观察端口；设为 disable 关闭 |

Go MCP 独立运行时读取 SOURCEAEC_API_URL 和 VIEWER_API_TOKEN。
没有 Python 服务 URL、skill venv 或 Python 应用部署配置。
未设置 SOURCEAEC_PYTHON 时，缓存位于用户 cache 下的 sourceaec/environments。
