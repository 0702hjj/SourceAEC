# Go agent integration

The Eino agent calls Go domain tools. Model operations use the gateway's native
editing service; no Python HTTP application is started. Project type selects
CAD/IFC roles, while source changes follow get → stage → run → save.

Skills are short declarations loaded directly from `skills/`. They contain no
SDK, hook or executable package. The optional Go MCP executable connects to the
same authenticated gateway and does not bypass business validation or storage.

Generated model source may call external geometry libraries inside bubblewrap.
Go owns environment preparation, limits, cancellation, revisions and publication.
