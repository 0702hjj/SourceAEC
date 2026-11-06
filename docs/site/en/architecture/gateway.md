# Go gateway

`server/` uses Go 1.27.3+. The gateway defaults to port 8090, wires editing, PostgreSQL, versions,
optional planning delivery and the Eino agent in-process, and serves the production web build.

```bash
export VIEWER_PG_DSN='postgres://localhost/sourceaec'
cd server
go run ./cmd/server
```

Copy the example configuration to `server_config.json`; set dataDir, webDist and converterScript for your working directory.
Configure the API token and CORS before public access. There is no Python HTTP service URL.

`go build ./cmd/server` builds the gateway; `go build ./cmd/sourceaec-mcp` builds the external stdio bridge.
The private Go edit-runtime adapter shares storage rules; native gateway editing calls domain code in-process.
See [configuration](/en/operations/configuration) and [verification](/en/operations/testing).
