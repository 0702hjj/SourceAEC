# External engine boundaries

SourceAEC owns project/model business logic: API, state, validation, orchestration, versions, Git, persistence, agents and MCP.
All maintained business implementation is Go. External engines own geometry operations and format export.
User/agent model scripts enter the sandbox as runtime input; they do not implement SourceAEC state or publication rules.

IfcOpenShell and ezdxf are external execution dependencies. SimpleCADAPI can supply upstream SDK/workflow guidance where its actual
installed format/export contract fits. SourceAEC does not vendor these SDKs or claim every upstream capability.
The product formats remain IFC and ASCII DXF; upstream STEP/assembly support does not introduce new platform formats.

The repository maintains one declarative integration skill with optional IFC/DXF references. Roles and tool permissions are configured
separately. There is no required planning gate, Python CLI, hook framework or bundled drawing DSL.
See [skills](/en/integrations/skills) and [format adapters](/en/architecture/formats).
