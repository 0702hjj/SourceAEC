# Go agent integration

The Eino agent calls Go domain tools. Model operations use the gateway's native
editing service; no Python HTTP application is started. Project type selects
CAD/IFC roles, while source changes follow get → stage → run → save.

A shared SourceAEC integration skill is loaded from `skills/`; format references
are read on demand and planning is optional data. Operators can install upstream
SDK references in the configured catalog. These declarations contain no
SDK, hook or executable package. The optional Go MCP executable connects to the
same authenticated gateway and does not bypass business validation or storage.

Generated model source may call external geometry libraries inside bubblewrap.
Go owns environment preparation, limits, cancellation, revisions and publication.
