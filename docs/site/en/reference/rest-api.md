# REST conventions

The Go gateway (default `http://localhost:8090`) is the public entry point. Business resources use `/api/v1/`;
viewer files use `/v1/models/{id}/...`. Models match `^m_[0-9a-f]{16}$`.

| Prefix | Contract |
| --- | --- |
| `/api/v1/models` | [Models/review](/en/reference/api-model) |
| `/api/v1/models/{id}/script`, `/edit`, `/git` | [Editing](/en/reference/edit-api) |
| `/api/v1/models/{id}/workflows` | [Durable workflows](/en/reference/workflows) |
| `/api/v1/chat`, `/api/v1/projects` | [Projects/chat](/en/reference/api-chat) |
| `/v1/models/{id}` | [Viewer assets](/en/reference/api-model#static-assets) |

## Envelope and errors

Except for raw file endpoints, responses use:

```json
{"code":0,"message":"ok","data":{}}
```

| Code | Meaning |
| --- | --- |
| 40001 | Invalid request/validation; compatibility editing 422 may map here |
| 40002 | Size limit exceeded |
| 40100 | Authentication failed |
| 40400 | Resource not found |
| 40900 | Conflict |
| 42900 | Capacity exhausted |
| 50000 | Internal error |
| 50200 | Upstream/compatibility transport failure |
| 50400 | Execution/transport timeout |

See each endpoint's HTTP contract: durable validation uses 422 and admission uses 429.

## Authentication and CORS

Authentication is enabled by apiToken/VIEWER_API_TOKEN. Ordinary APIs require `Authorization: Bearer <token>`;
OPTIONS preflight is exempt. Read-only XKT, metadata, render JSON and issue screenshots accept `?token=` for viewer clients.
Chat SSE also accepts a query token for EventSource. This does not permit query-token authentication for ordinary API/workflow requests.
CORS uses an origin allowlist. The browser stores its token as `aiifc_token` and prompts/retries on 401.

## Schemas

[Go OpenAPI](/go-server.openapi.json), [route inventory](/go-rest-api.routes.json) and [AI tools](/ai-tools.openapi.json)
are shared by both languages. The gateway implements editing in-process; there is no separate FastAPI schema/service.
Use the schema from your deployed source revision. See [integration](/en/integrations/overview).
