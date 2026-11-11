# Agents and MCP

The built-in Eino agent uses Go domain tools and coordinates IFC/CAD tasks by project kind.
Roles share optional SourceAEC integration declarations and configured upstream SDK references. Planning is optional input.
Without an LLM API key, the deterministic offline model is not a full natural-language modeling agent.

Run the stdio bridge from `server` with `go run ./cmd/sourceaec-mcp`; configure SOURCEAEC_API_URL and VIEWER_API_TOKEN.

| Tool | Purpose |
| --- | --- |
| model_current_context | Current version and staging state |
| ifc_upload_modified | Compare a locally edited IFC and record USER provenance through the gateway |
| dxf_upload_modified | Compare a locally edited DXF and record USER provenance through the gateway |
| workflow_submit | Freeze source/inputs and submit durable run/save; AI source by default |
| workflow_get | Query a run, or list recent summaries when workflow_id is omitted |
| workflow_control | Cancel/resume with frozen definitions/completed checkpoints |

MCP does not bypass gateway authentication/storage boundaries. Geometry always enters the sandbox.
See [skills](/en/integrations/skills), [editing API](/en/reference/edit-api) and [durable workflows](/en/reference/workflows).
