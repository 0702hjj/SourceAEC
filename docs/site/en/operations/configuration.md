# Configuration

The gateway reads `server_config.json`; `-config` selects another file. Copy `server/server_config.example.json`.
Relative paths resolve from the process working directory.

| Setting / environment | Purpose |
| --- | --- |
| pgDSN / VIEWER_PG_DSN | Required PostgreSQL connection |
| dataDir | Runtime projection directory |
| VIEWER_STORAGE_NAMESPACE | Database business namespace |
| webDist / VIEWER_WEB_DIST | Web build directory |
| SOURCEAEC_PYTHON | Absolute external geometry interpreter; otherwise Go uses uv |
| apiToken / VIEWER_API_TOKEN | Gateway Bearer token |
| corsOrigins / VIEWER_CORS_ORIGINS | Browser origin allowlist |
| skillsDir / VIEWER_SKILLS_DIR | Optional declarative skill root |
| mcpBinary / VIEWER_MCP_BIN | Optional Go stdio MCP binary |
| VIEWER_LLM_API_KEY | LLM API key; absent uses deterministic offline model |
| VIEWER_LLM_BASE_URL | LLM API URL |
| VIEWER_LLM_MODEL | Model name |
| VIEWER_PPROF_ADDR | Profiling listener; `disable` turns it off |
| SOURCEAEC_NOTEBOOK_AS_BYTES | Administrator notebook address-space bytes; default 1 GiB, 64 MiB–8 GiB |

The external Go MCP bridge reads `SOURCEAEC_API_URL` and `VIEWER_API_TOKEN`.
There is no Python service URL or skill venv deployment setting. Default environments live under the user cache in `sourceaec/environments`.
Durable workflows require a managed isolated Python base. See [sandbox](/en/architecture/sandbox).
