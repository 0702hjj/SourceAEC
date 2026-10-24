# Go 网关

`server/` 使用 Go 1.27.3+。网关默认端口 8090，在进程内装配编辑领域、
PostgreSQL 存储、版本、方案交付和 Eino Agent，并托管前端静态文件。

```bash
export VIEWER_PG_DSN='postgres://localhost/sourceaec'
cd server
go run ./cmd/server
```

复制示例配置为 `server_config.json`，按部署目录设置 dataDir、webDist 和
converterScript。生产公开访问需配置 token 与 CORS。没有 Python 服务地址配置。

`go build ./cmd/server` 构建网关；`go build ./cmd/sourceaec-mcp` 构建独立 Go
stdio MCP。`scripts/build_runtime.sh` 可构建用于运行时协议兼容的 Go 可执行程序，
网关原生编辑不通过 Python 适配器或子进程调用业务运行时。

[配置](/guide/configuration)、[开发验证](/development/testing)。
