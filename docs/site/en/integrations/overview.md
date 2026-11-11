# Gateway integration contract

Every business integration enters through the Go gateway REST API. Responses use `{code,message,data}` with success `code=0`;
model IDs match `^m_[0-9a-f]{16}$`. People and agents use the same editing/version contracts.

Geometry packages are external execution dependencies: IfcOpenShell for IFC, ezdxf for DXF and other SDKs only where an installed export
contract fits. Follow the actual upstream package API; skills are declarations, not runtime libraries.

The Go stdio MCP bridge uses SOURCEAEC_API_URL and VIEWER_API_TOKEN and calls the authenticated gateway.
It neither accesses persistence directly nor connects to Python HTTP services.

See [REST](/en/reference/rest-api), [editing](/en/reference/edit-api), [agents/MCP](/en/integrations/agents)
and [engine boundaries](/en/architecture/boundaries).
