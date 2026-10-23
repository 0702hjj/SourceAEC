# SourceAEC server binaries

The Linux server-only distribution contains `bin/sourceaec-server`,
`bin/sourceaec-runtime`, and `bin/sourceaec-mcp`. All business code is Go;
there are no Python HTTP services or maintained Python packages to deploy.

Use the matching source tag for frontend assets and Node.js converter dependencies.
PostgreSQL and Git are required. Linux script execution requires bubblewrap and
util-linux prlimit; missing isolation fails closed.

1. Extract `bin/` into a matching source checkout or configure equivalent paths.
2. Copy `server/server_config.example.json` to `server/server_config.json` and
   configure dataDir, converterScript, webDist and pgDSN. Relative defaults assume
   the gateway runs from the checkout's `server/` directory.
3. Set `VIEWER_PG_DSN`, `VIEWER_API_TOKEN`, and `VIEWER_CORS_ORIGINS`.
4. Install uv for Go-managed Python 3.12 geometry environments, or set an absolute
   `SOURCEAEC_PYTHON` interpreter path with required geometry packages installed.
   Python is only the sandbox geometry runtime, never a business service.
5. Run `../bin/sourceaec-server` from `server/`. The optional Go stdio MCP bridge
   uses `SOURCEAEC_API_URL` and `VIEWER_API_TOKEN` to access the same gateway.

Verify downloads with `sha256sum -c SHA256SUMS`. Adjacent build information files
record the compiler and module versions. The CGO-dependent legacy SQLite
migration utility remains available from source.

Deployment: https://0702hjj.github.io/SourceAEC/guide/quickstart
