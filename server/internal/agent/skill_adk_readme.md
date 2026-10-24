# Skill integration

The official Eino middleware reads `skills/*/SKILL.md`. SourceAEC maintains one
integration entry with optional IFC/DXF references. Operators may install
upstream SDK declaration bundles in the configured catalog without vendoring
them into SourceAEC. All project kinds and model roles share that catalog.

Skills are optional references; business rules remain in Go tools/runtime.
Role-specific tool permissions and filesystem roots remain enforced regardless
of which reference is loaded. Host commands are disabled by default; generated
geometry source runs only through the Go sandbox executor.
