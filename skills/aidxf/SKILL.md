---
name: aidxf
description: Create or revise CAD drawings through SourceAEC's script API using external geometry libraries.
license: MIT
---

# CAD model authoring

Read the project's existing drawings and requirements. Author runtime model
source with a literal `PARAMS` dictionary, stage with `baseRevision`, run it,
inspect the drawing, and save only a validated result. The first process
argument names the output ASCII DXF file.

Use external geometry libraries directly. For DXF, use `ezdxf`. For part and
assembly geometry, SimpleCADAPI can supply the external modeling runtime and
its own workflow/API references; consult the documentation for the installed
version. Do not copy an upstream SDK into this skill or assume a library exists
in the selected execution environment. Additional dependencies require pinned
registry versions in the source's PEP 723 block or an administrator-provided
runtime environment.

Keep units, dimensions, layers, entity identity and required deliverables
explicit. Validate exported geometry and drawing extents. Go generates the web
render payload; unsupported entity types are reported there. Optional stable
selection keys use AIDXF or SOURCEAEC XDATA. There are no bundled DSLs, hooks,
Python tools or `aidxfv3` commands.

Business validation, script state, execution isolation, versioning and artifact
publication belong to Go. Use the same API as human edits.
