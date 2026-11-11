# Declarative skills

The repository provides one entry, `skills/sourceaec/SKILL.md`. Configure skillsDir as its containing `skills/` root.
It describes platform editing, runtime I/O and capability boundaries, with optional `references/ifc.md` and `references/dxf.md`.
Planning is optional declarative input, not a separate mandatory skill/gate.

Modeling methods and precise package APIs belong to upstream SDK documentation/skills, including SimpleCADAPI.
Administrators can install upstream declarations in the configured root. Roles load references as needed;
role count and permissions are independent of the number of skills. References cannot expand file access or bypass isolation.
An empty skillsDir disables skill loading while Go tools remain usable.

No Python SDK, hooks, CLI or automatic design gate is bundled. Supported products remain IFC and ASCII DXF;
upstream STEP/assembly support is not an automatic product capability. Integration tests do not establish natural-language design quality.
See [agents](/en/integrations/agents) and [engine boundaries](/en/architecture/boundaries).
