# 故障排查

- 启动提示数据库缺失：配置 VIEWER_PG_DSN，确认 PostgreSQL 可达。
- 执行提示 bubblewrap/prlimit 缺失：安装所需系统工具；没有生产降级模式。
- 几何包导入失败：核对 SOURCEAEC_PYTHON 的包环境或 uv 准备日志，确认依赖精确版本。
- 暂存冲突：重新 get_script，基于最新 revision 修改后提交 baseRevision。
- 脚本执行失败：查看有限长度的错误尾部；修复源脚本后重试，不要手改 uploads。
- CAD 预览不完整：查看 unsupported；导出 ASCII DXF 并使用支持的实体类型。
- 页面资源缺失：单独构建 web/dist 并配置 webDist。
- MCP 不可用：配置 Go MCP 可执行文件及网关地址/token，不要启动 Python MCP 服务。

[配置](/guide/configuration)、[沙箱](/development/sandbox)。
