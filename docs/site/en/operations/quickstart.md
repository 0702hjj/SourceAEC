# Requirements and startup

Use Go 1.27.3+, PostgreSQL, Git, Linux bubblewrap and util-linux prlimit. Node.js 22 is needed for the client/converter.
Install uv for pinned managed Python 3.12 environments, or configure an external geometry interpreter.

Copy `server/server_config.example.json` to `server/server_config.json`; check dataDir, webDist, converterScript and database configuration.
Paths are relative to the process working directory.

```bash
export VIEWER_PG_DSN='postgres://localhost/sourceaec'
cd converter && npm ci
cd ../web && npm ci && npm run build
cd ../server
go run ./cmd/server
```

The gateway defaults to 127.0.0.1:8090 and runs all business services in-process. No Python HTTP services are needed.
For development, run `npm run dev` separately in `web`.
Set `VIEWER_API_TOKEN` and `VIEWER_CORS_ORIGINS` before public access.
For durable workflows, a configured interpreter needs an isolated managed base, not a venv linked to system Python.

See [configuration](/en/operations/configuration), [deployment](/en/operations/deployment) and [first project](/en/guide/first-project).
