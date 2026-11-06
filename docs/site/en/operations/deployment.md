# Deployment

Deploy one Go gateway with PostgreSQL and a shared runtime workspace. Install Git, bubblewrap, prlimit and the external geometry runtime.
Python is not an HTTP application. Build the web client and install converter dependencies from the matching source revision.

`scripts/build_server_release.sh VERSION` packages Linux amd64/arm64 gateway, Go runtime adapter, MCP bridge, build metadata,
example configuration and licenses. Check SHA256SUMS. Geometry packages and web/converter dependencies are separate deployment inputs.

Configure PostgreSQL, workspace/static/converter paths, Bearer token and CORS before startup; use a reverse proxy for HTTPS.
Use `SOURCEAEC_PYTHON` or Go-managed uv environments. Durable environment sealing requires an isolated managed base and checks content drift.

Back up database and runtime data, stop old writers, then upgrade matching binaries/static assets.
Do not mix old and new storage protocols. Release v0.1.0 instructions describe that historical architecture, not the current runtime.
For limits and capacity, see [execution](/en/architecture/execution), [storage](/en/architecture/storage) and
[performance verification](/en/operations/performance). Static documentation hosting is [separate](/en/operations/documentation).
