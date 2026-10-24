# 快速部署

需要 Go 1.27.3+、PostgreSQL、Git、Linux bubblewrap、util-linux prlimit。
前端与 IFC 转换器需要 Node.js 22。安装 uv，让 Go 准备固定依赖的 Python 3.12
几何环境，或通过 `SOURCEAEC_PYTHON` 选择已配置的解释器。

```bash
export VIEWER_PG_DSN='postgres://localhost/sourceaec'
cd converter && npm ci
cd ../web && npm ci && npm run build
cd ../server
go run ./cmd/server
```

复制 `server/server_config.example.json` 为 `server/server_config.json`，
核对 dataDir、webDist、converterScript 与 PostgreSQL 配置。
Go 网关默认运行于 127.0.0.1:8090；不需要 Python HTTP 服务。

开发时可在另一个终端从 web 运行 `npm run dev`。
生产访问需配置 `VIEWER_API_TOKEN` 和 `VIEWER_CORS_ORIGINS`。

[配置详情](/guide/configuration)、[生产部署](/guide/deploy)。
