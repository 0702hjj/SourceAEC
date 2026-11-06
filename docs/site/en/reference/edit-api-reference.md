# Machine-readable schemas

The Go gateway implements editing natively; no Python HTTP service is deployed.
Use `/api/v1/models/{id}/...` with the standard envelope. Private legacy `/models/...` service routes are not public contracts.

- [Gateway OpenAPI](/go-server.openapi.json)
- [Gateway route inventory](/go-rest-api.routes.json)
- [AI tool OpenAPI](/ai-tools.openapi.json)
- [Human-readable editing contract](/en/reference/edit-api)
- [Durable workflows](/en/reference/workflows)

These assets are shared across languages and versioned with source.
