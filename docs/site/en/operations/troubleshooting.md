# Troubleshooting

| Symptom | Check |
| --- | --- |
| Missing database at startup | Set VIEWER_PG_DSN and verify PostgreSQL connectivity |
| Missing bubblewrap/prlimit | Install required isolation tools; production has no fallback |
| Geometry import fails | Check SOURCEAEC_PYTHON/pinned packages or uv preparation logs |
| Frozen environment rejected | Use a managed isolated base; a system-linked venv is insufficient |
| Staging conflict | Read source again and submit against the new baseRevision |
| Script execution fails | Inspect bounded error output, fix source and retry; do not edit runtime uploads |
| Incomplete DXF preview | Check unsupported entities and export supported ASCII DXF |
| Missing web assets | Build web/dist and configure webDist |
| MCP unavailable | Check Go bridge, gateway URL and token; do not start a Python MCP service |
| Documentation unavailable | Check Pages source/workflow using the hosting guide |

See [configuration](/en/operations/configuration), [sandbox](/en/architecture/sandbox) and [documentation hosting](/en/operations/documentation).
